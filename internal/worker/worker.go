// Package worker owns the asynchronous worker composition root.
package worker

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/config"
	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/delivery"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/queue"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/scheduling"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/storage"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Worker owns the asynchronous job server, scheduler, usage collector, and
// their database and Redis resources.
type Worker struct {
	server          *queue.WorkerServer
	scheduler       *asynq.Scheduler
	usageCollector  *usage.UsageCollector
	collectorCancel context.CancelFunc
	collectorDone   chan struct{}
	pool            *pgxpool.Pool
	usageReader     usage.UsageReader
	usageEmitter    *usage.RedisEmitter
	jobQueue        queue.JobQueue
	rateLimiter     queue.TenantRateLimiter

	closeOnce sync.Once
	closeErr  error
}

// New builds the worker object graph and starts its background components.
//
//nolint:gocritic // Config is copied once at the composition boundary.
func New(cfg config.Config) (*Worker, error) {
	logger := slog.Default()

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	cleanup := func() {
		pool.Close()
	}

	queries := db.New(pool)
	embedder := llm.NewDashScopeNativeEmbedder(
		cfg.DashScopeEmbeddingAPIKey,
		cfg.DashScopeEmbeddingBaseURL,
		cfg.EmbeddingModel,
	)
	knowledgeStore := knowledge.NewPgVectorStore(pool, embedder)
	provider := llm.NewDashScopeProvider(cfg.DashScopeAPIKey, cfg.DashScopeBaseURL, cfg.LLMModel)

	usageReader, err := usage.NewRedisReader(cfg.RedisURL)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("usage reader: %w", err)
	}
	cleanupWithReader := func() {
		_ = usageReader.Close()
		cleanup()
	}

	budgetRepo := budget.NewRepository(queries)
	budgetChecker := budget.NewBudgetChecker(budgetRepo, usageReader)
	llmClient := llm.NewClientWithBudget(provider, nil, budgetChecker)

	usageEmitter, err := usage.NewRedisEmitter(cfg.RedisURL)
	if err != nil {
		cleanupWithReader()
		return nil, fmt.Errorf("usage emitter: %w", err)
	}
	cleanupWithEmitter := func() {
		_ = usageEmitter.Close()
		cleanupWithReader()
	}

	toolRegistry := tools.NewRegistry(usageEmitter)

	// Object store backing uploaded file bytes (MinIO in development).
	objectStore, err := storage.NewS3ObjectStore(&storage.S3Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Region:    cfg.S3Region,
		Bucket:    cfg.S3Bucket,
		UseSSL:    cfg.S3UseSSL,
	})
	if err != nil {
		cleanupWithEmitter()
		return nil, fmt.Errorf("create object store: %w", err)
	}

	// The ConnectorResolver turns a tenant-scoped DataSource into a Connector
	// at ingest time, using the Sources module's reader and the object store.
	encryptionKey := sha256.Sum256([]byte(cfg.EncryptionKey))
	cryptService, err := sources.NewAESGCMService(encryptionKey[:])
	if err != nil {
		cleanupWithEmitter()
		return nil, fmt.Errorf("create data source crypto service: %w", err)
	}
	sourceReader := sources.NewReader(sources.NewRepository(queries), cryptService)
	resolver := ingestion.NewConnectorResolver(sourceReader, objectStore)

	ingestWorker := ingestion.NewIngestWorker(
		resolver,
		knowledge.NewRecursiveChunker(1000, 200),
		knowledgeStore,
		usageEmitter,
		cfg.EmbeddingModel,
	)
	reportWorker := reports.NewReportWorker(
		knowledgeStore,
		llmClient,
		toolRegistry,
		usageEmitter,
		cfg.MaxToolCalls,
		cfg.MaxLLMCalls,
		cfg.MaxExecutionDuration(),
	)

	rateLimiter := queue.NewRedisRateLimiter(cfg.RedisURL, cfg.MaxActiveJobsPerTenant)
	cleanupWithRateLimiter := func() {
		_ = rateLimiter.Close()
		cleanupWithEmitter()
	}
	scheduleService := scheduling.NewService(scheduling.NewRepository(queries))
	reportService := reports.NewService(reports.NewRepository(queries))

	var deliveryService delivery.DeliveryService
	if cfg.SMTPHost != "" {
		deliveryService = delivery.NewSMTPService(delivery.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}, logger)
	} else {
		deliveryService = delivery.NewLogService(logger)
	}

	jobQueue, err := queue.NewAsynqQueue(cfg.RedisURL)
	if err != nil {
		cleanupWithRateLimiter()
		return nil, fmt.Errorf("job queue: %w", err)
	}
	cleanupWithQueue := func() {
		_ = jobQueue.Close()
		cleanupWithRateLimiter()
	}

	deps := queue.HandlerDeps{
		IngestWorker:    ingestWorker,
		ReportWorker:    reportWorker,
		ReportService:   reportService,
		DeliveryService: deliveryService,
		Queue:           jobQueue,
		RateLimiter:     rateLimiter,
		Logger:          logger,
	}
	workerServer := queue.NewWorkerServer(queue.ServerConfig{
		RedisAddr:   cfg.RedisURL,
		Concurrency: cfg.QueueConcurrency,
		Queues:      cfg.QueueWeights(),
		MaxRetry:    cfg.QueueMaxRetry,
	}, deps)
	if err := workerServer.Start(); err != nil {
		cleanupWithQueue()
		return nil, fmt.Errorf("worker server start: %w", err)
	}

	usageRepo := usage.NewRepository(queries)
	usageCollector, err := usage.NewCollector(cfg.RedisURL, usageRepo, logger)
	if err != nil {
		workerServer.Stop()
		cleanupWithQueue()
		return nil, fmt.Errorf("usage collector: %w", err)
	}
	collectorCtx, collectorCancel := context.WithCancel(context.Background())
	collectorDone := make(chan struct{})
	go func() {
		defer close(collectorDone)
		usageCollector.Start(collectorCtx)
	}()

	scheduler, err := startScheduler(logger, cfg.RedisURL, scheduleService)
	if err != nil {
		collectorCancel()
		<-collectorDone
		_ = usageCollector.Close()
		workerServer.Stop()
		cleanupWithQueue()
		return nil, fmt.Errorf("scheduler start: %w", err)
	}

	return &Worker{
		server:          workerServer,
		scheduler:       scheduler,
		usageCollector:  usageCollector,
		collectorCancel: collectorCancel,
		collectorDone:   collectorDone,
		pool:            pool,
		usageReader:     usageReader,
		usageEmitter:    usageEmitter,
		jobQueue:        jobQueue,
		rateLimiter:     rateLimiter,
	}, nil
}

// Shutdown stops workers and releases all owned resources.
func (w *Worker) Shutdown() error {
	return w.Close()
}

// Close stops workers and releases all owned resources. It is idempotent.
func (w *Worker) Close() error {
	w.closeOnce.Do(func() {
		w.collectorCancel()
		<-w.collectorDone
		w.scheduler.Shutdown()
		w.server.Stop()
		w.closeErr = errors.Join(
			w.usageCollector.Close(),
			w.jobQueue.Close(),
			w.rateLimiter.Close(),
			w.usageEmitter.Close(),
			w.usageReader.Close(),
		)
		w.pool.Close()
	})
	return w.closeErr
}

// startScheduler loads enabled ReportSchedules and registers their periodic
// ReportGeneration jobs.
func startScheduler(logger *slog.Logger, redisAddr string, svc scheduling.ScheduleService) (*asynq.Scheduler, error) {
	redisOpt, err := parseRedisAddr(redisAddr)
	if err != nil {
		return nil, err
	}

	scheduler := asynq.NewScheduler(redisOpt, nil)
	schedules, err := svc.ListAllEnabled(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list enabled schedules: %w", err)
	}

	for i := range schedules {
		schedule := &schedules[i]
		payload := &queue.ReportPayload{
			TenantID:   schedule.TenantID,
			ReportType: string(schedule.Type),
			ScheduleID: schedule.ID.String(),
		}
		if schedule.Focus != "" {
			payload.FocusAreas = []string{schedule.Focus}
		}

		task, err := queue.NewReportTask(payload)
		if err != nil {
			logger.Warn("skipping schedule: invalid task", "schedule_id", schedule.ID, "error", err)
			continue
		}
		if _, err := scheduler.Register(schedule.CronExpr, task); err != nil {
			logger.Warn("failed to register schedule", "schedule_id", schedule.ID, "cron", schedule.CronExpr, "error", err)
		}
	}

	if err := scheduler.Start(); err != nil {
		return nil, fmt.Errorf("scheduler start: %w", err)
	}
	return scheduler, nil
}

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
