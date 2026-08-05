// Package ingestion implements the Ingestion Workers module.
//
// Interface: Ingest(tenantID, sourceID) → IngestionResult.
// Connector resolution, chunking, embedding, and dedup are internal seams
// hidden behind this single entry point.
package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/google/uuid"
)

// IngestWorker orchestrates the full ingestion pipeline.
//
// Connectors are not indexed statically; a ConnectorResolver turns the
// tenant-scoped DataSource into a Connector at ingest time.
type IngestWorker struct {
	resolver       ConnectorResolver
	chunker        knowledge.Chunker
	store          knowledge.KnowledgeStore
	emitter        usage.UsageEmitter
	dedup          *Dedup
	embeddingModel string
}

func NewIngestWorker(
	resolver ConnectorResolver,
	chunker knowledge.Chunker,
	store knowledge.KnowledgeStore,
	emitter usage.UsageEmitter,
	embeddingModel string,
) *IngestWorker {
	return &IngestWorker{
		resolver:       resolver,
		chunker:        chunker,
		store:          store,
		emitter:        emitter,
		dedup:          NewDedup(),
		embeddingModel: embeddingModel,
	}
}

func (w *IngestWorker) Ingest(ctx context.Context, tenantID domain.TenantID, sourceID string) (domain.IngestionResult, error) {
	start := time.Now()

	// 1. Resolve the connector for this source. Resolution is tenant-scoped and
	// marks the source error on failure, so a broken source does not retry.
	connector, err := w.resolver.Resolve(ctx, tenantID, sourceID)
	if err != nil {
		return domain.IngestionResult{}, fmt.Errorf("resolve source %s: %w", sourceID, err)
	}

	// 2. Extract raw documents.
	docs, err := connector.Extract(ctx)
	if err != nil {
		return domain.IngestionResult{}, fmt.Errorf("extract from %s: %w", sourceID, err)
	}

	// 3. Chunk documents.
	chunks := w.chunker.Chunk(docs)

	// 4. Deduplicate.
	unique := make([]domain.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		if w.dedup.Seen(chunk.Content) {
			continue
		}
		unique = append(unique, chunk)
	}

	// 5. Assign IDs and timestamps.
	for i := range unique {
		unique[i].ID = uuid.New().String()
		unique[i].Date = time.Now()
	}

	// Build full documents so the store can persist and link them to chunks.
	documents := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		documents = append(documents, domain.Document{
			ID:       d.ID,
			Source:   d.Metadata["source"],
			Content:  d.Content,
			Metadata: d.Metadata,
		})
	}

	// 6. Replace this source's prior documents and chunks atomically, so
	// re-ingesting a source never accumulates duplicates. The source name is
	// taken from the extracted documents (e.g. the uploaded filename).
	if len(documents) == 0 {
		return domain.IngestionResult{}, fmt.Errorf("no documents extracted from %s", sourceID)
	}
	if err := w.store.ReplaceSource(ctx, tenantID, documents[0].Source, documents, unique); err != nil {
		return domain.IngestionResult{}, fmt.Errorf("store chunks: %w", err)
	}

	// 7. Emit embedding usage event.
	_ = w.emitter.EmitUsage(ctx, usage.UsageEvent{
		Type:     usage.EventEmbedding,
		TenantID: tenantID,
		Embedding: &usage.EmbeddingUsage{
			Model:           w.embeddingModel,
			ChunksProcessed: len(unique),
		},
	})

	return domain.IngestionResult{
		ChunksProcessed: len(unique),
		Duration:        time.Since(start),
	}, nil
}
