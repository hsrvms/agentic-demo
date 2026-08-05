package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

// integrationDeps builds mock domain dependencies for integration tests.
func integrationDeps(t *testing.T) (HandlerDeps, *mockConnector, *mockKnowledgeStore, *mockLLMClient) {
	t.Helper()
	conn := &mockConnector{}
	ks := &mockKnowledgeStore{}
	mockLLM := &mockLLMClient{}

	return HandlerDeps{
		IngestWorker: ingestion.NewIngestWorker(
			&fakeResolver{conn: conn},
			knowledge.NewRecursiveChunker(100, 20),
			ks,
			usage.NoOpEmitter{},
			"text-embedding-v4",
		),
		ReportWorker: reports.NewReportWorker(
			ks,
			mockLLM,
			tools.NewRegistry(usage.NoOpEmitter{}),
			usage.NoOpEmitter{},
			5, 5, 5*time.Minute,
		),
		Logger: slog.Default(),
	}, conn, ks, mockLLM
}

func TestIntegration_IngestionJob(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	deps, conn, ks, _ := integrationDeps(t)

	cfg := ServerConfig{
		RedisAddr:   mr.Addr(),
		Concurrency: 2,
		Queues:      map[string]int{QueueIngestion: 1},
		MaxRetry:    0,
	}

	srv := NewWorkerServer(cfg, deps)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	// Enqueue ingestion job through the JobQueue interface.
	q, err := NewAsynqQueue(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqQueue: %v", err)
	}
	defer q.Close()

	result, err := q.Enqueue(context.Background(), Job{
		Type:    TypeIngestionManual,
		Queue:   QueueIngestion,
		Payload: IngestionPayload{TenantID: "tenant-1", SourceID: "crm"},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if result.ID == "" {
		t.Fatal("expected non-empty job ID")
	}

	// Wait for processing.
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for ingestion job processing")
		case <-ticker.C:
			if conn.called.Load() {
				if ks.storeCalls.Load() < 1 {
					t.Fatal("expected at least 1 Store call")
				}
				return // success
			}
		}
	}
}

func TestIntegration_ReportJob(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	deps, _, ks, mockLLM := integrationDeps(t)

	cfg := ServerConfig{
		RedisAddr:   mr.Addr(),
		Concurrency: 2,
		Queues:      map[string]int{QueueReport: 1},
		MaxRetry:    0,
	}

	srv := NewWorkerServer(cfg, deps)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	q, err := NewAsynqQueue(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqQueue: %v", err)
	}
	defer q.Close()

	result, err := q.Enqueue(context.Background(), Job{
		Type:  TypeReportDaily,
		Queue: QueueReport,
		Payload: ReportPayload{
			TenantID:   "tenant-1",
			ReportType: "daily",
			FocusAreas: []string{"revenue"},
		},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if result.ID == "" {
		t.Fatal("expected non-empty job ID")
	}

	// Wait for processing.
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for report job processing")
		case <-ticker.C:
			if mockLLM.called.Load() && ks.queryCalls.Load() > 0 {
				return // success
			}
		}
	}
}

func TestIntegration_DeadLetter(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	// Use a custom mux with a handler that always fails.
	failType := "test:always_fail"
	maxRetries := 1

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: mr.Addr()},
		asynq.Config{
			Concurrency: 1,
			Queues:      map[string]int{"test": 1},
			RetryDelayFunc: func(n int, err error, t *asynq.Task) time.Duration {
				return 10 * time.Millisecond // fast retries for testing
			},
		},
	)

	var attempts atomic.Int32
	mux := asynq.NewServeMux()
	mux.HandleFunc(failType, func(ctx context.Context, task *asynq.Task) error {
		attempts.Add(1)
		return errAlwaysFail
	})

	if err := srv.Start(mux); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Shutdown()

	// Enqueue a task with max retries = 2.
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
	task := asynq.NewTask(failType, []byte(`{}`))
	_, err = client.Enqueue(task, asynq.Queue("test"), asynq.MaxRetry(maxRetries))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	client.Close()

	// Wait for all retries to exhaust. With MaxRetry(1): initial + 1 retry = 2 attempts.
	// Fast-forward miniredis clock to trigger scheduled retries.
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("timed out; attempts=%d, expected %d", attempts.Load(), maxRetries+1)
		case <-ticker.C:
			mr.FastForward(5 * time.Second) // advance clock to trigger scheduled retries
			if int(attempts.Load()) >= maxRetries+1 {
				// All retries exhausted — task should be in the archive (dead-letter).
				time.Sleep(200 * time.Millisecond)
				mr.FastForward(1 * time.Second)

				verifyArchived(t, mr.Addr())
				return // success
			}
		}
	}
}

func verifyArchived(t *testing.T, redisAddr string) {
	t.Helper()
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redisAddr})
	defer inspector.Close()

	archived, err := inspector.ListArchivedTasks("test", asynq.PageSize(10))
	if err != nil {
		t.Fatalf("list archived tasks: %v", err)
	}
	if len(archived) < 1 {
		t.Fatalf("expected at least 1 archived task, got %d", len(archived))
	}
}

var errAlwaysFail = errors.New("intentional failure for dead-letter test")
