package knowledge

import (
	"context"
	"fmt"
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

// setupStore spins up a Postgres+pgvector container, runs migrations,
// and returns a PgVectorStore and a cleanup function.
func setupStore(t *testing.T) (*PgVectorStore, func()) {
	t.Helper()
	ctx := context.Background()

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
		t.Fatalf("failed to start postgres container: %v", err)
	}

	host, err := pgContainer.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}
	port, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get port: %v", err)
	}

	connStr := fmt.Sprintf("postgres://testuser:testpass@%s:%s/testdb?sslmode=disable", host, port.Port())
	t.Logf("connection string: %s", connStr)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		t.Fatalf("failed to create connection pool: %v", err)
	}

	// Run migration.
	migration, err := os.ReadFile("../../internal/db/migrations/001_initial.sql")
	if err != nil {
		pool.Close()
		pgContainer.Terminate(ctx)
		t.Fatalf("failed to read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		pool.Close()
		pgContainer.Terminate(ctx)
		t.Fatalf("failed to run migration: %v", err)
	}

	store := &PgVectorStore{
		queries:  db.New(pool),
		embedder: &stubEmbedder{dim: 1024},
	}

	cleanup := func() {
		pool.Close()
		pgContainer.Terminate(ctx)
	}

	return store, cleanup
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
