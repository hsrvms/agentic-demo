// Package knowledge implements the Knowledge Store module.
//
// Interface: store, query, delete_tenant_data, get_stats.
// Every query is scoped to a TenantID — a query without a tenant_id
// is impossible by construction, not by convention.
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// KnowledgeStore is the module interface.
type KnowledgeStore interface {
	Store(ctx context.Context, tenantID domain.TenantID, chunks []domain.Chunk) error
	Query(ctx context.Context, tenantID domain.TenantID, text string, topK int, filters QueryFilters) ([]domain.RankedChunk, error)
	DeleteTenantData(ctx context.Context, tenantID domain.TenantID) error
	GetStats(ctx context.Context, tenantID domain.TenantID) (map[string]int, error)
}

// QueryFilters are optional metadata filters for similarity search.
type QueryFilters struct {
	Source   string
	DateFrom time.Time
	DateTo   time.Time
}

// PgVectorStore implements KnowledgeStore backed by Postgres + pgvector.
type PgVectorStore struct {
	pool     *pgxpool.Pool
	embedder Embedder
}

// Embedder generates vector embeddings for text.
// This is an internal seam — swap DashScope for local ONNX later.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

func NewPgVectorStore(pool *pgxpool.Pool, embedder Embedder) *PgVectorStore {
	return &PgVectorStore{pool: pool, embedder: embedder}
}

// vecToString converts a float32 slice to a pgvector-compatible string: "[1,2,3]".
// This is the text format Postgres accepts with the ::vector cast.
func vecToString(v []float32) string {
	return pgvector.NewVector(v).String()
}

// parseVecString parses a pgvector text representation back to float32.
func parseVecString(s string) ([]float32, error) {
	var v pgvector.Vector
	if err := v.Parse(s); err != nil {
		return nil, err
	}
	return v.Slice(), nil
}

func (s *PgVectorStore) Store(ctx context.Context, tenantID domain.TenantID, chunks []domain.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	// Generate embeddings for all chunks in one batch.
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	embeddings, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("generate embeddings: %w", err)
	}

	// Assign embeddings back to chunks.
	for i := range chunks {
		chunks[i].Embedding = embeddings[i]
	}

	// Batch insert. Each chunk is inserted with the vector as a text parameter
	// cast to ::vector — Postgres handles the implicit text→vector conversion.
	for _, chunk := range chunks {
		metadataJSON, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for chunk %s: %w", chunk.ID, err)
		}

		_, err = s.pool.Exec(ctx, `
			INSERT INTO chunks (id, tenant_id, content, embedding, source, document_type, date, metadata)
			VALUES ($1, $2, $3, $4::vector, $5, $6, $7, $8::jsonb)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				embedding = EXCLUDED.embedding,
				metadata = EXCLUDED.metadata,
				date = EXCLUDED.date`,
			chunk.ID, string(tenantID), chunk.Content, vecToString(chunk.Embedding),
			chunk.Source, chunk.DocumentType, chunk.Date, string(metadataJSON),
		)
		if err != nil {
			return fmt.Errorf("store chunk %s: %w", chunk.ID, err)
		}
	}

	return nil
}

func (s *PgVectorStore) Query(ctx context.Context, tenantID domain.TenantID, text string, topK int, filters QueryFilters) ([]domain.RankedChunk, error) {
	// Embed the query text.
	embeddings, err := s.embedder.Embed(ctx, []string{text})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	queryVec := vecToString(embeddings[0])

	// Build dynamic WHERE clause.
	where := []string{"c.tenant_id = $2"}
	args := []interface{}{queryVec, string(tenantID), topK}
	argIdx := 4 // $1, $2, $3 already used

	if filters.Source != "" {
		where = append(where, fmt.Sprintf("c.source = $%d", argIdx))
		args = append(args, filters.Source)
		argIdx++
	}
	if !filters.DateFrom.IsZero() {
		where = append(where, fmt.Sprintf("c.date >= $%d", argIdx))
		args = append(args, filters.DateFrom)
		argIdx++
	}
	if !filters.DateTo.IsZero() {
		where = append(where, fmt.Sprintf("c.date <= $%d", argIdx))
		args = append(args, filters.DateTo)
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT c.id, c.content, c.source, c.document_type, c.date, c.metadata::text,
		       (c.embedding <=> $1::vector) AS distance
		FROM chunks c
		WHERE %s
		ORDER BY c.embedding <=> $1::vector
		LIMIT $3`, strings.Join(where, " AND "))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("similarity query: %w", err)
	}
	defer rows.Close()

	var results []domain.RankedChunk
	for rows.Next() {
		var rc domain.RankedChunk
		var metadataJSON string
		var date time.Time

		err := rows.Scan(
			&rc.Chunk.ID, &rc.Chunk.Content, &rc.Chunk.Source,
			&rc.Chunk.DocumentType, &date, &metadataJSON, &rc.Distance,
		)
		if err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}

		rc.Chunk.Date = date
		_ = json.Unmarshal([]byte(metadataJSON), &rc.Chunk.Metadata)
		results = append(results, rc)
	}

	return results, rows.Err()
}

func (s *PgVectorStore) DeleteTenantData(ctx context.Context, tenantID domain.TenantID) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM chunks WHERE tenant_id = $1", string(tenantID))
	if err != nil {
		return fmt.Errorf("delete tenant data: %w", err)
	}
	return nil
}

func (s *PgVectorStore) GetStats(ctx context.Context, tenantID domain.TenantID) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source, COUNT(*) as chunk_count
		FROM chunks
		WHERE tenant_id = $1
		GROUP BY source`, string(tenantID))
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return nil, fmt.Errorf("scan stat: %w", err)
		}
		stats[source] = count
	}
	return stats, rows.Err()
}
