package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/ingestion"
	"github.com/agentic-demo/platform/internal/knowledge"
	"github.com/agentic-demo/platform/internal/llm"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/tools"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
)

func testHandlerDeps(t *testing.T) (HandlerDeps, *mockConnector) {
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
	}, conn
}

func TestWorkerServer_StartStop(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	deps, conn := testHandlerDeps(t)
	cfg := ServerConfig{
		RedisAddr:   mr.Addr(),
		Concurrency: 1,
		Queues:      map[string]int{QueueIngestion: 1},
		MaxRetry:    0,
	}

	srv := NewWorkerServer(cfg, &deps)

	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Enqueue a task.
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
	payload := IngestionPayload{TenantID: "t1", SourceID: "crm"}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeIngestionManual, data)

	_, err = client.Enqueue(task, asynq.Queue(QueueIngestion))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	client.Close()

	// Wait for the handler to be called.
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			srv.Stop()
			t.Fatal("timed out waiting for task processing")
		case <-ticker.C:
			if conn.called.Load() {
				srv.Stop()
				return // success
			}
		}
	}
}

func TestWorkerServer_GracefulShutdown(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	started := make(chan struct{})
	var completed atomic.Bool

	slowType := "test:slow"

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: mr.Addr()},
		asynq.Config{
			Concurrency: 1,
			Queues:      map[string]int{"default": 1},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(slowType, func(ctx context.Context, task *asynq.Task) error {
		close(started) // signal that processing has begun
		time.Sleep(300 * time.Millisecond)
		completed.Store(true)
		return nil
	})

	if err := srv.Start(mux); err != nil {
		t.Fatalf("start server: %v", err)
	}

	// Enqueue slow task.
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
	task := asynq.NewTask(slowType, []byte(`{}`))
	_, err = client.Enqueue(task, asynq.Queue("default"))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	client.Close()

	// Wait for handler to start processing.
	select {
	case <-started:
		// Handler is running. Call Shutdown immediately.
	case <-time.After(5 * time.Second):
		srv.Shutdown()
		t.Fatal("task was never picked up")
	}

	// Shutdown should block until the in-flight task finishes.
	srv.Shutdown()

	if !completed.Load() {
		t.Fatal("expected slow task to complete during graceful shutdown")
	}
}

func TestWorkerServer_ErrorHandler_LogsFinalFailure(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	logBuf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelError}))

	// A generic (non-resolution) error so the queue retries and, once retries
	// are exhausted, the ErrorHandler logs the failure.
	deps := HandlerDeps{
		IngestWorker: ingestion.NewIngestWorker(
			&fakeResolver{err: errors.New("connect refused")},
			knowledge.NewRecursiveChunker(100, 20),
			&mockKnowledgeStore{},
			usage.NoOpEmitter{},
			"text-embedding-v4",
		),
		Logger: logger,
	}
	cfg := ServerConfig{
		RedisAddr:   mr.Addr(),
		Concurrency: 1,
		Queues:      map[string]int{QueueIngestion: 1},
		MaxRetry:    0, // no retries -> the ErrorHandler runs immediately
		Logger:      logger,
	}

	srv := NewWorkerServer(cfg, &deps)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
	payload := IngestionPayload{TenantID: "t1", SourceID: "src-1"}
	data, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeIngestionManual, data)
	if _, err := client.Enqueue(task, asynq.Queue(QueueIngestion)); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	client.Close()

	// Wait for the error handler to log the failure.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logBuf.String(), "task processing failed") {
			srv.Stop()
			assert.Contains(t, logBuf.String(), TypeIngestionManual)
			assert.Contains(t, logBuf.String(), "connect refused")
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	srv.Stop()
	t.Fatal("timed out waiting for error handler to log failure")
}

func TestNewWorkerServer_DefaultQueues(t *testing.T) {
	deps, _ := testHandlerDeps(t)
	cfg := ServerConfig{
		RedisAddr:   "localhost:6379",
		Concurrency: 5,
		MaxRetry:    3,
	}

	srv := NewWorkerServer(cfg, &deps)
	if srv == nil {
		t.Fatal("expected non-nil WorkerServer")
	}
}

// syncBuffer is a bytes.Buffer that is safe for concurrent write/read, used
// to capture slog output produced on the worker goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Compile-time interface assertions.
var _ llm.LLMClient = (*mockLLMClient)(nil)
var _ domain.TenantID = ""
