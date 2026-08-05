package knowledge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// stubEmbedder returns configured vectors for known texts, zero vectors
// otherwise. This avoids calling the DashScope API in tests and makes
// similarity ranking deterministic.
type stubEmbedder struct {
	dim     int
	model   string
	vectors map[string][]float32
}

func (e *stubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		if v, ok := e.vectors[text]; ok {
			results[i] = v
		} else {
			results[i] = make([]float32, e.dim)
		}
	}
	return results, nil
}

func (e *stubEmbedder) Dimension() int { return e.dim }
func (e *stubEmbedder) Model() string {
	if e.model != "" {
		return e.model
	}
	return "test-embedding"
}

// unitVector returns a vector with 1.0 at index idx and 0.0 elsewhere.
// Two identical unit vectors have cosine distance 0; two orthogonal unit
// vectors have cosine distance 1.
func unitVector(dim, idx int) []float32 {
	v := make([]float32, dim)
	v[idx] = 1.0
	return v
}

// migrationSQL reads and concatenates all migration files in order.
func migrationSQL() ([]byte, error) {
	entries, err := os.ReadDir("../../sql/migrations")
	if err != nil {
		return nil, err
	}

	var buf []byte
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := os.ReadFile("../../sql/migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		buf = append(buf, b...)
		buf = append(buf, '\n')
	}
	return buf, nil
}

// setupStore spins up a Postgres+pgvector environment, runs migrations,
// and returns a PgVectorStore and a cleanup function.
//
// It first attempts testcontainers (for CI with Docker). If Docker is
// unavailable, it falls back to a running Postgres instance (e.g. in a
// devcontainer) by creating an ephemeral test database.
func setupStore(t *testing.T) (*PgVectorStore, func()) {
	t.Helper()
	ctx := context.Background()

	// Try testcontainers first.
	store, cleanup, err := setupStoreContainer(t, ctx)
	if err == nil {
		return store, cleanup
	}
	t.Logf("testcontainers unavailable: %v — falling back to running Postgres", err)

	// Fall back to running Postgres instance.
	store, cleanup, err = setupStoreLocal(t, ctx)
	if err != nil {
		t.Fatalf("failed to set up test store: %v", err)
	}
	return store, cleanup
}

// setupStoreContainer uses testcontainers to start an ephemeral Postgres.
func setupStoreContainer(t *testing.T, ctx context.Context) (*PgVectorStore, func(), error) {
	req := testcontainers.ContainerRequest{
		Image: "pgvector/pgvector:pg16",
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
			"POSTGRES_DB":       "testdb",
		},
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		),
	}

	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start postgres container: %w", err)
	}

	host, err := pgContainer.Host(ctx)
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, nil, fmt.Errorf("get host: %w", err)
	}
	port, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, nil, fmt.Errorf("get port: %w", err)
	}

	connStr := fmt.Sprintf("postgres://testuser:testpass@%s:%s/testdb?sslmode=disable", host, port.Port())
	t.Logf("connection string: %s", connStr)

	store, cleanup, err := connectAndMigrate(t, ctx, connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, nil, err
	}

	wrappedCleanup := func() {
		cleanup()
		pgContainer.Terminate(ctx)
	}
	return store, wrappedCleanup, nil
}

// setupStoreLocal creates an ephemeral database on a running Postgres instance.
// It uses DATABASE_URL or falls back to the default devcontainer connection.
func setupStoreLocal(t *testing.T, ctx context.Context) (*PgVectorStore, func(), error) {
	baseConnStr := os.Getenv("DATABASE_URL")
	if baseConnStr == "" {
		baseConnStr = "postgres://app:app@postgres:5432/app?sslmode=disable"
	}

	// Connect to the base database to create the test database.
	basePool, err := pgxpool.New(ctx, baseConnStr)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to base database: %w", err)
	}
	defer basePool.Close()

	// Create a unique, short database name (stays under PostgreSQL's 63-char limit).
	var randBytes [4]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		return nil, nil, fmt.Errorf("generate random bytes: %w", err)
	}
	dbName := fmt.Sprintf("test_know_%s", hex.EncodeToString(randBytes[:]))

	if _, err := basePool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName)); err != nil {
		return nil, nil, fmt.Errorf("create test database %s: %w", dbName, err)
	}

	// Build connection string for the test database by parsing the URL.
	connStr, err := withDBName(baseConnStr, dbName)
	if err != nil {
		basePool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		return nil, nil, fmt.Errorf("build connection string: %w", err)
	}
	t.Logf("connection string: %s", connStr)

	store, cleanup, err := connectAndMigrate(t, ctx, connStr)
	if err != nil {
		basePool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		return nil, nil, err
	}

	wrappedCleanup := func() {
		cleanup()
		p, err := pgxpool.New(context.Background(), baseConnStr)
		if err == nil {
			p.Exec(context.Background(),
				"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
				dbName)
			p.Exec(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			p.Close()
		}
	}
	return store, wrappedCleanup, nil
}

// withDBName returns a copy of the postgres connection string with the
// database name replaced.
func withDBName(connStr, dbName string) (string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", err
	}
	u.Path = "/" + dbName
	return u.String(), nil
}

// connectAndMigrate connects to a database, runs the initial migration,
// and returns a PgVectorStore and cleanup function.
func connectAndMigrate(t *testing.T, ctx context.Context, connStr string) (*PgVectorStore, func(), error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, nil, fmt.Errorf("create connection pool: %w", err)
	}

	sqlBytes, err := migrationSQL()
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("read migration: %w", err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("run migration: %w", err)
	}

	store := &PgVectorStore{
		pool:     pool,
		queries:  db.New(pool),
		embedder: &stubEmbedder{dim: 1024},
	}

	cleanup := func() {
		pool.Close()
	}
	return store, cleanup, nil
}

// setupStoreWithEmbedder is like setupStore but uses the provided embedder.
func setupStoreWithEmbedder(t *testing.T, emb Embedder) (*PgVectorStore, func()) {
	t.Helper()
	store, cleanup := setupStore(t)
	store.embedder = emb
	return store, cleanup
}

// storeChunks stores the given chunks, deriving one document per chunk so the
// document_id FK is satisfied. Documents are the full content behind a chunk;
// here each chunk is its own document for simplicity.
func storeChunks(store *PgVectorStore, ctx context.Context, tenantID domain.TenantID, chunks []domain.Chunk) error {
	docs := make([]domain.Document, len(chunks))
	for i, c := range chunks {
		if c.DocumentID == "" {
			c.DocumentID = c.ID
		}
		chunks[i] = c
		docs[i] = domain.Document{
			ID:       c.DocumentID,
			Source:   c.Source,
			Content:  c.Content,
			Metadata: c.Metadata,
		}
	}
	return store.Store(ctx, tenantID, docs, chunks)
}

func TestKnowledgeStore_StoreAndQuery(t *testing.T) {
	// Configure vectors so the query ranks the revenue chunk first:
	// query and revenue chunk share the same unit vector (distance 0),
	// the other chunks are orthogonal (distance 1).
	emb := &stubEmbedder{
		dim: 1024,
		vectors: map[string][]float32{
			"revenue growth":                                unitVector(1024, 0),
			"Quarterly revenue increased by 15%":            unitVector(1024, 0),
			"Customer satisfaction scores dropped 3 points": unitVector(1024, 1),
			"Employee headcount stable at 250":              unitVector(1024, 2),
		},
	}
	store, cleanup := setupStoreWithEmbedder(t, emb)
	defer cleanup()
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-a")

	chunks := []domain.Chunk{
		{
			ID:           "chunk-1",
			Content:      "Quarterly revenue increased by 15%",
			Source:       "financials",
			DocumentType: "report",
			Date:         time.Now(),
			Metadata:     map[string]string{"quarter": "Q1"},
		},
		{
			ID:           "chunk-2",
			Content:      "Customer satisfaction scores dropped 3 points",
			Source:       "support",
			DocumentType: "report",
			Date:         time.Now(),
			Metadata:     map[string]string{"quarter": "Q1"},
		},
		{
			ID:           "chunk-3",
			Content:      "Employee headcount stable at 250",
			Source:       "hr",
			DocumentType: "report",
			Date:         time.Now(),
			Metadata:     map[string]string{"quarter": "Q1"},
		},
	}

	if err := storeChunks(store, ctx, tenantID, chunks); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	results, err := store.Query(ctx, tenantID, "revenue growth", 3, QueryFilters{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// The revenue-related chunk should be first (closest match).
	if results[0].Chunk.Content != "Quarterly revenue increased by 15%" {
		t.Errorf("first result = %q, want revenue chunk", results[0].Chunk.Content)
	}
}

func TestKnowledgeStore_TenantIsolation(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()

	chunkA := []domain.Chunk{{
		ID: "ca-1", Content: "Tenant A data", Source: "src", DocumentType: "text", Date: time.Now(),
	}}
	chunkB := []domain.Chunk{{
		ID: "cb-1", Content: "Tenant B data", Source: "src", DocumentType: "text", Date: time.Now(),
	}}

	if err := storeChunks(store, ctx, "tenant-a", chunkA); err != nil {
		t.Fatalf("Store tenant-a failed: %v", err)
	}
	if err := storeChunks(store, ctx, "tenant-b", chunkB); err != nil {
		t.Fatalf("Store tenant-b failed: %v", err)
	}

	results, err := store.Query(ctx, "tenant-a", "data", 5, QueryFilters{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	for _, r := range results {
		if r.Chunk.Content == "Tenant B data" {
			t.Error("tenant-a query returned tenant-b data — tenant isolation violated")
		}
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for tenant-a, got %d", len(results))
	}
}

func TestKnowledgeStore_DeleteTenantData(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()

	chunks := []domain.Chunk{{
		ID: "d-1", Content: "Data to delete", Source: "src", DocumentType: "text", Date: time.Now(),
	}}

	if err := storeChunks(store, ctx, "tenant-del", chunks); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if err := store.DeleteTenantData(ctx, "tenant-del"); err != nil {
		t.Fatalf("DeleteTenantData failed: %v", err)
	}

	results, err := store.Query(ctx, "tenant-del", "data", 5, QueryFilters{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after deletion, got %d", len(results))
	}
}

func TestKnowledgeStore_GetStats(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()

	chunks := []domain.Chunk{
		{ID: "s-1", Content: "Source A content", Source: "source-a", DocumentType: "text", Date: time.Now()},
		{ID: "s-2", Content: "Source B content 1", Source: "source-b", DocumentType: "text", Date: time.Now()},
		{ID: "s-3", Content: "Source A content 2", Source: "source-a", DocumentType: "text", Date: time.Now()},
		{ID: "s-4", Content: "Source C content", Source: "source-c", DocumentType: "text", Date: time.Now()},
	}

	if err := storeChunks(store, ctx, "tenant-stats", chunks); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	stats, err := store.GetStats(ctx, "tenant-stats")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(stats))
	}
	if stats["source-a"] != 2 {
		t.Errorf("source-a count = %d, want 2", stats["source-a"])
	}
	if stats["source-b"] != 1 {
		t.Errorf("source-b count = %d, want 1", stats["source-b"])
	}
	if stats["source-c"] != 1 {
		t.Errorf("source-c count = %d, want 1", stats["source-c"])
	}
}

func TestKnowledgeStore_QueryWithSourceFilter(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()

	chunks := []domain.Chunk{
		{ID: "f-1", Content: "Alpha content from src1", Source: "src1", DocumentType: "text", Date: time.Now()},
		{ID: "f-2", Content: "Beta content from src2", Source: "src2", DocumentType: "text", Date: time.Now()},
		{ID: "f-3", Content: "Gamma content from src1", Source: "src1", DocumentType: "text", Date: time.Now()},
	}

	if err := storeChunks(store, ctx, "tenant-filter", chunks); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	results, err := store.Query(ctx, "tenant-filter", "content", 5, QueryFilters{Source: "src1"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results with src1 filter, got %d", len(results))
	}
	for _, r := range results {
		if r.Chunk.Source != "src1" {
			t.Errorf("result has source %q, want src1", r.Chunk.Source)
		}
	}
}

func TestKnowledgeStore_EmptyQuery(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()

	results, err := store.Query(ctx, "tenant-empty", "anything", 5, QueryFilters{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty store, got %d", len(results))
	}
}

func TestKnowledgeStore_GetDocument(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-doc")

	chunks := []domain.Chunk{
		{
			ID: "cd-1", DocumentID: "doc-1",
			Content: "First chunk of the full document", Source: "manual", DocumentType: "report",
			Date: time.Now(), Metadata: map[string]string{"title": "Quarterly Report"},
		},
		{
			ID: "cd-2", DocumentID: "doc-1",
			Content: "Second chunk of the full document", Source: "manual", DocumentType: "report",
			Date: time.Now(), Metadata: map[string]string{"title": "Quarterly Report"},
		},
	}

	if err := store.Store(ctx, tenantID, []domain.Document{{
		ID: "doc-1", Source: "manual", Content: "The full parsed document text", Metadata: map[string]string{"title": "Quarterly Report"},
	}}, chunks); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Query returns chunks carrying their document_id.
	results, err := store.Query(ctx, tenantID, "full document", 5, QueryFilters{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Chunk.DocumentID != "doc-1" {
			t.Errorf("chunk document_id = %q, want doc-1", r.Chunk.DocumentID)
		}
	}

	// Expand the matched chunk to its full document with one lookup.
	doc, err := store.GetDocument(ctx, tenantID, "doc-1")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc.Content != "The full parsed document text" {
		t.Errorf("document content = %q, want full parsed text", doc.Content)
	}
	if doc.Source != "manual" {
		t.Errorf("document source = %q, want manual", doc.Source)
	}
	if doc.Metadata["title"] != "Quarterly Report" {
		t.Errorf("document title = %q, want Quarterly Report", doc.Metadata["title"])
	}
}

func TestKnowledgeStore_GetDocument_TenantScoped(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()

	if err := store.Store(ctx, "tenant-a", []domain.Document{{
		ID: "shared-doc", Source: "src", Content: "Tenant A content",
	}}, []domain.Chunk{{
		ID: "ca-1", DocumentID: "shared-doc", Content: "Tenant A content", Source: "src", DocumentType: "text", Date: time.Now(),
	}}); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// The same document ID in another tenant must not resolve.
	_, err := store.GetDocument(ctx, "tenant-b", "shared-doc")
	if err == nil {
		t.Fatal("expected error fetching another tenant's document, got nil")
	}
}

func TestKnowledgeStore_EmbeddingModelRecorded(t *testing.T) {
	emb := &stubEmbedder{dim: 1024, model: "text-embedding-v4"}
	store, cleanup := setupStoreWithEmbedder(t, emb)
	defer cleanup()
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-model")

	chunks := []domain.Chunk{{
		ID: "cm-1", DocumentID: "dm-1", Content: "Content to embed", Source: "src", DocumentType: "text", Date: time.Now(),
	}}
	docs := []domain.Document{{
		ID: "dm-1", Source: "src", Content: "Content to embed",
	}}

	if err := store.Store(ctx, tenantID, docs, chunks); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// The recorded embedding model must match the model actually used by the store.
	model, err := store.queries.GetChunkEmbeddingModel(ctx, db.GetChunkEmbeddingModelParams{
		TenantID: string(tenantID),
		ID:       "cm-1",
	})
	if err != nil {
		t.Fatalf("query embedding_model: %v", err)
	}
	if model != "text-embedding-v4" {
		t.Errorf("embedding_model = %q, want text-embedding-v4", model)
	}
}

func TestKnowledgeStore_ReplaceSource_ReplacesPriorData(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-replace")
	source := "quarterly-report"

	// First ingest: one document, two chunks.
	firstChunks := []domain.Chunk{
		{
			ID: "c1", DocumentID: "doc-1", Content: "Revenue grew 15%",
			Source: source, DocumentType: "report", Date: time.Now(),
			Metadata: map[string]string{"run": "1"},
		},
		{
			ID: "c2", DocumentID: "doc-1", Content: "Costs fell 5%",
			Source: source, DocumentType: "report", Date: time.Now(),
			Metadata: map[string]string{"run": "1"},
		},
	}
	if err := store.ReplaceSource(ctx, tenantID, source, []domain.Document{{
		ID: "doc-1", Source: source, Content: "Full report v1", Metadata: map[string]string{"version": "1"},
	}}, firstChunks); err != nil {
		t.Fatalf("first ReplaceSource failed: %v", err)
	}

	// Re-ingest the same source: one document with one new chunk.
	secondChunks := []domain.Chunk{
		{
			ID: "c3", DocumentID: "doc-1", Content: "Revenue grew 20%",
			Source: source, DocumentType: "report", Date: time.Now(),
			Metadata: map[string]string{"run": "2"},
		},
	}
	if err := store.ReplaceSource(ctx, tenantID, source, []domain.Document{{
		ID: "doc-1", Source: source, Content: "Full report v2", Metadata: map[string]string{"version": "2"},
	}}, secondChunks); err != nil {
		t.Fatalf("second ReplaceSource failed: %v", err)
	}

	// The prior chunks must be gone — the source holds only the new chunk.
	stats, err := store.GetStats(ctx, tenantID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats[source] != 1 {
		t.Errorf("source %q chunk count = %d, want 1 (prior chunks replaced)", source, stats[source])
	}

	results, err := store.Query(ctx, tenantID, "revenue", 10, QueryFilters{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 chunk after re-ingest, got %d", len(results))
	}
	if got := results[0].Chunk.Content; got != "Revenue grew 20%" {
		t.Errorf("chunk content = %q, want %q", got, "Revenue grew 20%")
	}

	// The document behind the chunk reflects the latest ingest.
	doc, err := store.GetDocument(ctx, tenantID, "doc-1")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc.Content != "Full report v2" {
		t.Errorf("document content = %q, want %q", doc.Content, "Full report v2")
	}
}

func TestKnowledgeStore_ReplaceSource_OnlyAffectsMatchingSource(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-replace-t")

	// Two distinct sources exist for the tenant.
	for _, src := range []string{"source-a", "source-b"} {
		if err := store.ReplaceSource(ctx, tenantID, src, []domain.Document{{
			ID: src + "-doc", Source: src, Content: "Doc for " + src,
		}}, []domain.Chunk{{
			ID: src + "-1", DocumentID: src + "-doc", Content: "chunk of " + src,
			Source: src, DocumentType: "text", Date: time.Now(),
		}}); err != nil {
			t.Fatalf("ReplaceSource %s failed: %v", src, err)
		}
	}

	// Re-ingest source-a only; source-b must be untouched.
	if err := store.ReplaceSource(ctx, tenantID, "source-a", []domain.Document{{
		ID: "source-a-doc", Source: "source-a", Content: "Doc for source-a v2",
	}}, []domain.Chunk{{
		ID: "source-a-2", DocumentID: "source-a-doc", Content: "new chunk of source-a",
		Source: "source-a", DocumentType: "text", Date: time.Now(),
	}}); err != nil {
		t.Fatalf("ReplaceSource source-a failed: %v", err)
	}

	stats, err := store.GetStats(ctx, tenantID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats["source-a"] != 1 {
		t.Errorf("source-a chunk count = %d, want 1", stats["source-a"])
	}
	if stats["source-b"] != 1 {
		t.Errorf("source-b chunk count = %d, want 1 (untouched)", stats["source-b"])
	}
}

func TestKnowledgeStore_ReplaceSource_TenantIsolated(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()
	source := "shared-ops"

	// Both tenants ingest the same source name.
	for _, tid := range []domain.TenantID{"tenant-x", "tenant-y"} {
		docID := string(tid) + "-doc"
		if err := store.ReplaceSource(ctx, tid, source, []domain.Document{{
			ID: docID, Source: source, Content: "Doc for " + string(tid),
		}}, []domain.Chunk{{
			ID: docID + "-c1", DocumentID: docID, Content: "chunk for " + string(tid),
			Source: source, DocumentType: "text", Date: time.Now(),
		}}); err != nil {
			t.Fatalf("ReplaceSource for %s failed: %v", tid, err)
		}
	}

	// Replacing in tenant-x must not touch tenant-y's data.
	if err := store.ReplaceSource(ctx, "tenant-x", source, []domain.Document{{
		ID: "tenant-x-doc", Source: source, Content: "Doc for tenant-x v2",
	}}, []domain.Chunk{{
		ID: "tenant-x-c2", DocumentID: "tenant-x-doc", Content: "new chunk for tenant-x",
		Source: source, DocumentType: "text", Date: time.Now(),
	}}); err != nil {
		t.Fatalf("ReplaceSource tenant-x failed: %v", err)
	}

	results, err := store.Query(ctx, "tenant-y", "tenant-y", 10, QueryFilters{})
	if err != nil {
		t.Fatalf("Query tenant-y failed: %v", err)
	}
	if len(results) != 1 || results[0].Chunk.Content != "chunk for tenant-y" {
		t.Errorf("tenant-y data changed by tenant-x re-ingest: %+v", results)
	}
}

func TestKnowledgeStore_DeleteTenantData_CascadesDocuments(t *testing.T) {
	store, cleanup := setupStore(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := domain.TenantID("tenant-cascade")

	docs := []domain.Document{{
		ID: "dc-1", Source: "src", Content: "Full document to delete",
	}}
	chunks := []domain.Chunk{{
		ID: "cc-1", DocumentID: "dc-1", Content: "Chunk of the document", Source: "src", DocumentType: "text", Date: time.Now(),
	}}

	if err := store.Store(ctx, tenantID, docs, chunks); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if err := store.DeleteTenantData(ctx, tenantID); err != nil {
		t.Fatalf("DeleteTenantData failed: %v", err)
	}

	// Both the document and its cascaded chunks must be gone.
	if _, err := store.GetDocument(ctx, tenantID, "dc-1"); err == nil {
		t.Error("expected error fetching deleted document, got nil")
	}
	results, err := store.Query(ctx, tenantID, "document", 5, QueryFilters{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 chunks after tenant deletion, got %d", len(results))
	}
}
