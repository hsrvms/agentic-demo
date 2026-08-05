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
	Store(ctx context.Context, tenantID domain.TenantID, docs []domain.Document, chunks []domain.Chunk) error
	// ReplaceSource atomically replaces a source's prior documents and chunks
	// with the given ones. Re-ingesting a source must not accumulate
	// duplicates, so the old rows for (tenantID, source) are deleted and the
	// new ones inserted in a single transaction.
	ReplaceSource(ctx context.Context, tenantID domain.TenantID, source string, docs []domain.Document, chunks []domain.Chunk) error
	Query(ctx context.Context, tenantID domain.TenantID, text string, topK int, filters QueryFilters) ([]domain.RankedChunk, error)
	GetDocument(ctx context.Context, tenantID domain.TenantID, documentID string) (domain.Document, error)
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
	queries  *db.Queries
	embedder Embedder
}

// Embedder generates vector embeddings for text.
// This is an internal seam — swap DashScope for local ONNX later.
// Model returns the identifier of the embedding model in use, which the store
// records on every chunk it writes for provenance.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
	Model() string
}

func NewPgVectorStore(pool *pgxpool.Pool, embedder Embedder) *PgVectorStore {
	return &PgVectorStore{pool: pool, queries: db.New(pool), embedder: embedder}
}

func (s *PgVectorStore) Store(ctx context.Context, tenantID domain.TenantID, docs []domain.Document, chunks []domain.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	return s.insert(ctx, s.queries, tenantID, docs, chunks)
}

// ReplaceSource atomically replaces a source's prior documents and chunks. The
// delete and the insert share one transaction, so re-ingestion never leaves a
// window of partial state: either the old data is fully replaced or nothing
// changes. Deleting the source's documents cascades to its chunks via the
// document_id FK.
func (s *PgVectorStore) ReplaceSource(ctx context.Context, tenantID domain.TenantID, source string, docs []domain.Document, chunks []domain.Chunk) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin replace tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	qtx := s.queries.WithTx(tx)

	if err := qtx.DeleteSourceDocuments(ctx, db.DeleteSourceDocumentsParams{
		TenantID: string(tenantID),
		Source:   source,
	}); err != nil {
		return fmt.Errorf("delete prior documents for source %s: %w", source, err)
	}

	if err := s.insert(ctx, qtx, tenantID, docs, chunks); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit replace tx: %w", err)
	}
	return nil
}

// insert persists documents and their chunks through the given queries
// handle (either the pool-backed queries or a transaction-scoped one).
// Documents are always persisted, independent of whether any chunks survive
// dedup, so a source that yields documents but no chunks still replaces its
// prior data cleanly instead of being wiped.
func (s *PgVectorStore) insert(ctx context.Context, q *db.Queries, tenantID domain.TenantID, docs []domain.Document, chunks []domain.Chunk) error {
	// Persist documents first so the document_id FK on chunks can be satisfied.
	for _, doc := range docs {
		metadataJSON, err := json.Marshal(doc.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for document %s: %w", doc.ID, err)
		}

		if err := q.InsertDocument(ctx, db.InsertDocumentParams{
			ID:       doc.ID,
			TenantID: string(tenantID),
			Source:   doc.Source,
			Content:  doc.Content,
			Metadata: metadataJSON,
		}); err != nil {
			return fmt.Errorf("store document %s: %w", doc.ID, err)
		}
	}

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

	model := s.embedder.Model()

	for i, chunk := range chunks {
		metadataJSON, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for chunk %s: %w", chunk.ID, err)
		}

		err = q.InsertChunk(ctx, db.InsertChunkParams{
			ID:             chunk.ID,
			TenantID:       string(tenantID),
			Content:        chunk.Content,
			Embedding:      pgvector.NewVector(embeddings[i]),
			Source:         chunk.Source,
			DocumentType:   chunk.DocumentType,
			Date:           chunk.Date,
			Metadata:       metadataJSON,
			DocumentID:     chunk.DocumentID,
			EmbeddingModel: model,
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
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata for chunk %s: %w", row.ID, err)
		}

		results = append(results, domain.RankedChunk{
			Chunk: domain.Chunk{
				ID:           row.ID,
				Content:      row.Content,
				Source:       row.Source,
				DocumentType: row.DocumentType,
				DocumentID:   row.DocumentID,
				Date:         row.Date,
				Metadata:     metadata,
			},
			Distance: row.Distance,
		})
	}

	return results, nil
}

func (s *PgVectorStore) GetDocument(ctx context.Context, tenantID domain.TenantID, documentID string) (domain.Document, error) {
	row, err := s.queries.GetDocument(ctx, db.GetDocumentParams{
		ID:       documentID,
		TenantID: string(tenantID),
	})
	if err != nil {
		return domain.Document{}, fmt.Errorf("get document %s: %w", documentID, err)
	}

	var metadata map[string]string
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return domain.Document{}, fmt.Errorf("unmarshal metadata for document %s: %w", row.ID, err)
	}

	return domain.Document{
		ID:        row.ID,
		Source:    row.Source,
		Content:   row.Content,
		Metadata:  metadata,
		CreatedAt: row.CreatedAt,
	}, nil
}

func (s *PgVectorStore) DeleteTenantData(ctx context.Context, tenantID domain.TenantID) error {
	// Deleting documents cascades to their chunks via the document_id FK.
	err := s.queries.DeleteTenantDocuments(ctx, string(tenantID))
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
