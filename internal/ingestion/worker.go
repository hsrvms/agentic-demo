// Package ingestion implements the Ingestion Workers module.
//
// Interface: Ingest(tenantID, sourceID) → IngestionResult.
// Connectors, chunking, embedding, and dedup are internal seams
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
type IngestWorker struct {
	connectors     map[string]Connector
	chunker        knowledge.Chunker
	embedder       knowledge.Embedder
	store          knowledge.KnowledgeStore
	emitter        usage.UsageEmitter
	dedup          *Dedup
	embeddingModel string
}

func NewIngestWorker(
	connectors map[string]Connector,
	chunker knowledge.Chunker,
	embedder knowledge.Embedder,
	store knowledge.KnowledgeStore,
	emitter usage.UsageEmitter,
	embeddingModel string,
) *IngestWorker {
	return &IngestWorker{
		connectors:     connectors,
		chunker:        chunker,
		embedder:       embedder,
		store:          store,
		emitter:        emitter,
		dedup:          NewDedup(),
		embeddingModel: embeddingModel,
	}
}

func (w *IngestWorker) Ingest(ctx context.Context, tenantID domain.TenantID, sourceID string) (domain.IngestionResult, error) {
	start := time.Now()

	// 1. Find the connector for this source type.
	connector, ok := w.connectors[sourceID]
	if !ok {
		return domain.IngestionResult{}, fmt.Errorf("unknown source: %s", sourceID)
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

	// 6. Store chunks (embeddings generated internally by the store).
	if err := w.store.Store(ctx, tenantID, unique); err != nil {
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
