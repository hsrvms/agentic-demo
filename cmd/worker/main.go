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
	"github.com/agentic-demo/platform/internal/delivery"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/queue"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/scheduling"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/hibiken/asynq"
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

	queries := db.New(pool)

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

	// Build usage emitter. Uses Redis for real-time counters and flush queue.
	usageEmitter, err := usage.NewRedisEmitter(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("usage emitter: %w", err)
	}
	defer usageEmitter.Close()

	// Build tool registry.
	toolRegistry := tools.NewRegistry(usageEmitter)

	// Build domain workers.
	// Connectors are empty — configured in later workstreams.
	ingestWorker := ingestion.NewIngestWorker(
		map[string]ingestion.Connector{},
		knowledge.NewRecursiveChunker(1000, 200),
		embedder,
		ks,
		usageEmitter,
		cfg.EmbeddingModel,
	)

	reportWorker := reports.NewReportWorker(
		ks,
		llmClient,
		toolRegistry,
		usageEmitter,
		cfg.MaxToolCalls,
		cfg.MaxLLMCalls,
		cfg.MaxExecutionDuration(),
	)

	// Build rate limiter.
	rateLimiter := queue.NewRedisRateLimiter(cfg.RedisURL, cfg.MaxActiveJobsPerTenant)

	// Build scheduling service.
	scheduleRepo := scheduling.NewRepository(queries)
	scheduleService := scheduling.NewService(scheduleRepo)

	// Build report persistence service.
	reportRepo := reports.NewRepository(queries)
	reportService := reports.NewService(reportRepo)

	// Build delivery service. Use SMTP when configured, otherwise log mode.
	var deliveryService delivery.DeliveryService
	if cfg.SMTPHost != "" {
		deliveryService = delivery.NewSMTPService(delivery.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}, logger)
		logger.Info("delivery: using SMTP", "host", cfg.SMTPHost, "port", cfg.SMTPPort)
	} else {
		deliveryService = delivery.NewLogService(logger)
		logger.Info("delivery: using log mode (no SMTP_HOST configured)")
	}

	// Build job queue for enqueuing delivery tasks.
	jobQueue, err := queue.NewAsynqQueue(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("job queue: %w", err)
	}
	defer jobQueue.Close()

	// Build handler deps and start worker server.
	deps := queue.HandlerDeps{
		IngestWorker:    ingestWorker,
		ReportWorker:    reportWorker,
		ReportService:   reportService,
		DeliveryService: deliveryService,
		Queue:           jobQueue,
		RateLimiter:     rateLimiter,
		Logger:          logger,
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

	// Build usage repository and collector for persisting usage events.
	usageRepo := usage.NewRepository(queries)
	usageCollector, err := usage.NewCollector(cfg.RedisURL, usageRepo, logger)
	if err != nil {
		srv.Stop()
		return fmt.Errorf("usage collector: %w", err)
	}
	defer usageCollector.Close()

	collectorCtx, collectorCancel := context.WithCancel(context.Background())
	defer collectorCancel()
	go usageCollector.Start(collectorCtx)
	logger.Info("usage collector started")

	// Start the scheduler for periodic report generation.
	scheduler, err := startScheduler(logger, cfg.RedisURL, scheduleService)
	if err != nil {
		srv.Stop()
		return fmt.Errorf("scheduler start: %w", err)
	}

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("received signal, shutting down", "signal", sig)
	collectorCancel()
	logger.Info("usage collector stopped")
	scheduler.Shutdown()
	logger.Info("scheduler stopped")
	srv.Stop()
	logger.Info("worker stopped")
	return nil
}

// startScheduler loads enabled schedules from the database and registers
// them as periodic tasks on an asynq.Scheduler. Returns the running scheduler.
func startScheduler(logger *slog.Logger, redisAddr string, svc scheduling.ScheduleService) (*asynq.Scheduler, error) {
	redisOpt, err := parseRedisAddr(redisAddr)
	if err != nil {
		return nil, err
	}

	scheduler := asynq.NewScheduler(redisOpt, nil)

	ctx := context.Background()
	schedules, err := svc.ListAllEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled schedules: %w", err)
	}

	for i := range schedules {
		s := &schedules[i]
		payload := &queue.ReportPayload{
			TenantID:   s.TenantID,
			ReportType: string(s.Type),
			ScheduleID: s.ID.String(),
		}
		if s.Focus != "" {
			payload.FocusAreas = []string{s.Focus}
		}

		task, err := queue.NewReportTask(payload)
		if err != nil {
			logger.Warn("skipping schedule: invalid task",
				"schedule_id", s.ID,
				"tenant_id", s.TenantID,
				"error", err,
			)
			continue
		}

		entryID, err := scheduler.Register(s.CronExpr, task)
		if err != nil {
			logger.Warn("failed to register schedule",
				"schedule_id", s.ID,
				"tenant_id", s.TenantID,
				"cron", s.CronExpr,
				"error", err,
			)
			continue
		}

		logger.Info("registered periodic task",
			"entry_id", entryID,
			"schedule_id", s.ID,
			"tenant_id", s.TenantID,
			"type", s.Type,
			"cron", s.CronExpr,
		)
	}

	if err := scheduler.Start(); err != nil {
		return nil, fmt.Errorf("scheduler start: %w", err)
	}

	logger.Info("scheduler started", "registered", len(schedules))
	return scheduler, nil
}

// parseRedisAddr converts a Redis address string to asynq.RedisClientOpt.
func parseRedisAddr(addr string) (asynq.RedisClientOpt, error) {
	if len(addr) > 8 && addr[:8] == "redis://" {
		connOpt, err := asynq.ParseRedisURI(addr)
		if err != nil {
			return asynq.RedisClientOpt{}, fmt.Errorf("parse redis URL: %w", err)
		}
		clientOpt, ok := connOpt.(asynq.RedisClientOpt)
		if !ok {
			return asynq.RedisClientOpt{}, fmt.Errorf("unsupported redis URL type")
		}
		return clientOpt, nil
	}
	return asynq.RedisClientOpt{Addr: addr}, nil
}
