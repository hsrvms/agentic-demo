// Worker binary: processes async jobs from the Redis-backed queue.
//
// This is the composition root for the worker process. It wires up
// the full object graph (DB, domain workers, queue handlers) and runs
// the asynq worker server with graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/agentic-demo/platform/internal/config"
	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/queue"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("database ping: %w", err)
	}
	defer pool.Close()

	// Build embedder.
	embedder := llm.NewDashScopeNativeEmbedder(
		cfg.DashScopeEmbeddingAPIKey,
		cfg.DashScopeEmbeddingBaseURL,
		cfg.EmbeddingModel,
	)

	// Build knowledge store.
	ks := knowledge.NewPgVectorStore(pool, embedder)

	// Build LLM client.
	provider := llm.NewDashScopeProvider(cfg.DashScopeAPIKey, cfg.DashScopeBaseURL, cfg.LLMModel)
	llmClient := llm.NewClient(provider, nil)

	// Build tool registry.
	toolRegistry := tools.NewRegistry(usage.NoOpEmitter{})

	// Build domain workers.
	// Connectors are empty — configured in later workstreams.
	ingestWorker := ingestion.NewIngestWorker(
		map[string]ingestion.Connector{},
		knowledge.NewRecursiveChunker(1000, 200),
		embedder,
		ks,
		usage.NoOpEmitter{},
	)

	reportWorker := reports.NewReportWorker(
		ks,
		llmClient,
		toolRegistry,
		usage.NoOpEmitter{},
		cfg.MaxToolCalls,
		cfg.MaxLLMCalls,
		cfg.MaxExecutionDuration(),
	)

	// Build rate limiter.
	rateLimiter := queue.NewRedisRateLimiter(cfg.RedisURL, cfg.MaxActiveJobsPerTenant)

	// Build handler deps and start worker server.
	deps := queue.HandlerDeps{
		IngestWorker: ingestWorker,
		ReportWorker: reportWorker,
		RateLimiter:  rateLimiter,
		Logger:       logger,
	}

	serverCfg := queue.ServerConfig{
		RedisAddr:   cfg.RedisURL,
		Concurrency: cfg.QueueConcurrency,
		Queues:      cfg.QueueWeights(),
		MaxRetry:    cfg.QueueMaxRetry,
	}

	srv := queue.NewWorkerServer(serverCfg, deps)

	if err := srv.Start(); err != nil {
		return fmt.Errorf("worker server start: %w", err)
	}

	logger.Info("worker started",
		"concurrency", cfg.QueueConcurrency,
		"queues", cfg.QueueWeights(),
	)

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("received signal, shutting down", "signal", sig)
	srv.Stop()
	logger.Info("worker stopped")
	return nil
}
