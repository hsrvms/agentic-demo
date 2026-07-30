package knowledge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
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

// unitVector returns a vector with 1.0 at index idx and 0.0 elsewhere.
// Two identical unit vectors have cosine distance 0; two orthogonal unit
// vectors have cosine distance 1.
func unitVector(dim, idx int) []float32 {
	v := make([]float32, dim)
	v[idx] = 1.0
	return v
}

// migrationSQL reads and returns the initial migration file content.
func migrationSQL() ([]byte, error) {
	return os.ReadFile("../../sql/migrations/001_initial.sql")
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

	if err := store.Store(ctx, tenantID, chunks); err != nil {
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

	if err := store.Store(ctx, "tenant-a", chunkA); err != nil {
		t.Fatalf("Store tenant-a failed: %v", err)
	}
	if err := store.Store(ctx, "tenant-b", chunkB); err != nil {
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

	if err := store.Store(ctx, "tenant-del", chunks); err != nil {
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

	if err := store.Store(ctx, "tenant-stats", chunks); err != nil {
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

	if err := store.Store(ctx, "tenant-filter", chunks); err != nil {
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
