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
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/jackc/pgx/v5/pgtype"
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
	queries  *db.Queries
	embedder Embedder
}

// Embedder generates vector embeddings for text.
// This is an internal seam — swap DashScope for local ONNX later.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

func NewPgVectorStore(pool *pgxpool.Pool, embedder Embedder) *PgVectorStore {
	return &PgVectorStore{queries: db.New(pool), embedder: embedder}
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

	for _, chunk := range chunks {
		metadataJSON, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for chunk %s: %w", chunk.ID, err)
		}

		err = s.queries.InsertChunk(ctx, db.InsertChunkParams{
			ID:           chunk.ID,
			TenantID:     string(tenantID),
			Content:      chunk.Content,
			Embedding:    pgvector.NewVector(chunk.Embedding),
			Source:       chunk.Source,
			DocumentType: chunk.DocumentType,
			Date:         chunk.Date,
			Metadata:     metadataJSON,
		})
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

	params := db.QueryChunksParams{
		TenantID:  string(tenantID),
		Embedding: pgvector.NewVector(embeddings[0]),
		Limit:     int32(topK),
	}

	if filters.Source != "" {
		params.Source = pgtype.Text{String: filters.Source, Valid: true}
	}
	if !filters.DateFrom.IsZero() {
		params.DateFrom = pgtype.Timestamptz{Time: filters.DateFrom, Valid: true}
	}
	if !filters.DateTo.IsZero() {
		params.DateTo = pgtype.Timestamptz{Time: filters.DateTo, Valid: true}
	}

	rows, err := s.queries.QueryChunks(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("similarity query: %w", err)
	}

	results := make([]domain.RankedChunk, 0, len(rows))
	for _, row := range rows {
		var metadata map[string]string
		_ = json.Unmarshal(row.Metadata, &metadata)

		results = append(results, domain.RankedChunk{
			Chunk: domain.Chunk{
				ID:           row.ID,
				Content:      row.Content,
				Source:       row.Source,
				DocumentType: row.DocumentType,
				Date:         row.Date,
				Metadata:     metadata,
			},
			Distance: row.Distance,
		})
	}

	return results, nil
}

func (s *PgVectorStore) DeleteTenantData(ctx context.Context, tenantID domain.TenantID) error {
	err := s.queries.DeleteTenantChunks(ctx, string(tenantID))
	if err != nil {
		return fmt.Errorf("delete tenant data: %w", err)
	}
	return nil
}

func (s *PgVectorStore) GetStats(ctx context.Context, tenantID domain.TenantID) (map[string]int, error) {
	rows, err := s.queries.GetChunkStats(ctx, string(tenantID))
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	stats := make(map[string]int, len(rows))
	for _, row := range rows {
		stats[row.Source] = int(row.ChunkCount)
	}
	return stats, nil
}
