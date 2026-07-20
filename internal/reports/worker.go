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
	knowledge   knowledge.KnowledgeStore
	llmClient   llm.LLMClient
	tools       tools.ToolRegistry
	emitter     usage.UsageEmitter
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

	context := BuildContext(chunks)

	// Phase 2: Run the agent loop.
	agent := &AgentLoop{
		llmClient:    w.llmClient,
		tools:        w.tools,
		emitter:      w.emitter,
		maxToolCalls: w.maxToolCalls,
		maxLLMCalls:  w.maxLLMCalls,
	}

	agentResult, err := agent.Run(ctx, tenantID, config, context)
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
