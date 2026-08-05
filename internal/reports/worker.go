// Package reports implements the Report Workers module.
//
// Interface: GenerateReport(tenantID, config) → Report.
// The agent loop (context gathering → bounded reasoning → synthesis)
// is an implementation detail hidden behind this single entry point.
package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/google/uuid"
)

// ReportWorker generates strategic reports.
type ReportWorker struct {
	knowledge    knowledge.KnowledgeStore
	llmClient    llm.LLMClient
	tools        tools.ToolRegistry
	emitter      usage.UsageEmitter
	maxToolCalls int
	maxLLMCalls  int
	maxDuration  time.Duration
}

func NewReportWorker(
	ks knowledge.KnowledgeStore,
	llmClient llm.LLMClient,
	tr tools.ToolRegistry,
	emitter usage.UsageEmitter,
	maxToolCalls, maxLLMCalls int,
	maxDuration time.Duration,
) *ReportWorker {
	return &ReportWorker{
		knowledge:    ks,
		llmClient:    llmClient,
		tools:        tr,
		emitter:      emitter,
		maxToolCalls: maxToolCalls,
		maxLLMCalls:  maxLLMCalls,
		maxDuration:  maxDuration,
	}
}

func (w *ReportWorker) GenerateReport(ctx context.Context, tenantID domain.TenantID, config domain.ReportConfig) (domain.Report, error) {
	ctx, cancel := context.WithTimeout(ctx, w.maxDuration)
	defer cancel()

	// Phase 1: Gather context from the knowledge store.
	prompt := BuildPrompt(config)
	chunks, err := w.knowledge.Query(ctx, tenantID, prompt, 10, knowledge.QueryFilters{})
	if err != nil {
		return domain.Report{}, fmt.Errorf("gather context: %w", err)
	}

	// Expand each matched chunk to its full document so the LLM reasons over
	// complete context. GetDocument is tenant-scoped, so a chunk can only ever
	// resolve to a document owned by this tenant.
	retrieved, err := expandDocuments(ctx, tenantID, chunks, w.knowledge)
	if err != nil {
		return domain.Report{}, fmt.Errorf("expand documents: %w", err)
	}

	gatheredContext := BuildContext(retrieved)

	// Phase 2: Run the agent loop.
	agent := &AgentLoop{
		llmClient:    w.llmClient,
		tools:        w.tools,
		emitter:      w.emitter,
		maxToolCalls: w.maxToolCalls,
		maxLLMCalls:  w.maxLLMCalls,
	}

	agentResult, err := agent.Run(ctx, tenantID, config, gatheredContext)
	if err != nil {
		return domain.Report{}, fmt.Errorf("agent loop: %w", err)
	}

	report := domain.Report{
		ID:       uuid.New().String(),
		Content:  agentResult.Content,
		Type:     config.Type,
		Date:     time.Now(),
		Metadata: agentResult.Metadata,
	}

	return report, nil
}

// expandDocuments resolves each matched chunk's DocumentID to its full document
// via the tenant-scoped GetDocument. Multiple chunks that reference the same
// document are fetched once (deduplicated). A chunk with no document reference
// falls back to its own content so context is never silently dropped.
func expandDocuments(ctx context.Context, tenantID domain.TenantID, chunks []domain.RankedChunk, ks knowledge.KnowledgeStore) ([]RetrievedContext, error) {
	results := make([]RetrievedContext, 0, len(chunks))
	seen := make(map[string]bool, len(chunks))

	for _, rc := range chunks {
		docID := rc.Chunk.DocumentID
		similarity := 1.0 - rc.Distance

		if docID == "" {
			results = append(results, RetrievedContext{
				Source:       rc.Chunk.Source,
				DocumentType: rc.Chunk.DocumentType,
				Similarity:   similarity,
				Document: domain.Document{
					ID:      rc.Chunk.ID,
					Source:  rc.Chunk.Source,
					Content: rc.Chunk.Content,
				},
			})
			continue
		}

		if seen[docID] {
			continue
		}
		seen[docID] = true

		doc, err := ks.GetDocument(ctx, tenantID, docID)
		if err != nil {
			return nil, fmt.Errorf("get document %s: %w", docID, err)
		}

		results = append(results, RetrievedContext{
			Source:       rc.Chunk.Source,
			DocumentType: rc.Chunk.DocumentType,
			Similarity:   similarity,
			Document:     doc,
		})
	}

	return results, nil
}
