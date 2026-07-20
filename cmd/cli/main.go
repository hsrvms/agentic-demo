// Phase 0 walking skeleton: file in → chunk → embed → store → query → LLM → report out.
//
// Usage:
//
//	go run ./cmd/cli -file examples/demo.txt
//	go run ./cmd/cli -query "What are the key business metrics?"
//	go run ./cmd/cli -file examples/demo.txt -query "Summarize the key findings"
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/agentic-demo/platform/internal/config"
	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
)

func main() {
	filePath := flag.String("file", "", "file to ingest")
	query := flag.String("query", "What are the key business metrics and trends?", "report focus query")
	tenantID := flag.String("tenant", "demo", "tenant ID")
	flag.Parse()

	if *filePath == "" && *query == "" {
		fmt.Fprintln(os.Stderr, "usage: cli -file <path> [-query <text>] [-tenant <id>]")
		os.Exit(1)
	}

	ctx := context.Background()

	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// Connect to Postgres.
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Build the object graph.
	emitter := usage.NoOpEmitter{}
	embedder := llm.NewDashScopeEmbedder(cfg.DashScopeAPIKey, cfg.DashScopeBaseURL, cfg.EmbeddingModel)
	ks := knowledge.NewPgVectorStore(pool, embedder)

	llmClient := llm.NewClient(
		llm.NewDashScopeProvider(cfg.DashScopeAPIKey, cfg.DashScopeBaseURL, cfg.LLMModel),
		nil, // No fallback in Phase 0.
	)

	chunker := knowledge.NewRecursiveChunker(1000, 200)
	toolRegistry := tools.NewRegistry(emitter)

	tid := domain.TenantID(*tenantID)

	// Ingestion path.
	if *filePath != "" {
		connector := &ingestion.FileConnector{Path: *filePath}
		worker := ingestion.NewIngestWorker(
			map[string]ingestion.Connector{"file": connector},
			chunker, embedder, ks, emitter,
		)

		fmt.Printf("Ingesting: %s\n", *filePath)
		result, err := worker.Ingest(ctx, tid, "file")
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingestion failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Ingested %d chunks in %v\n\n", result.ChunksProcessed, result.Duration)
	}

	// Report generation path.
	reportWorker := reports.NewReportWorker(
		ks, llmClient, toolRegistry, emitter,
		cfg.MaxToolCalls, cfg.MaxLLMCalls, cfg.MaxExecutionDuration(),
	)

	reportConfig := domain.ReportConfig{
		Type:       domain.ReportDaily,
		FocusAreas: []string{*query},
	}

	fmt.Println("Generating report...")
	report, err := reportWorker.GenerateReport(ctx, tid, reportConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "report generation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(strings.Repeat("=", 72))
	fmt.Println(report.Content)
	fmt.Println(strings.Repeat("=", 72))

	if report.Metadata != nil {
		fmt.Printf("\n[memory stats] llm_calls=%s tool_calls=%s tokens=%s/%s\n",
			report.Metadata["llm_calls"], report.Metadata["tool_calls"],
			report.Metadata["input_tokens"], report.Metadata["output_tokens"])
	}
}
