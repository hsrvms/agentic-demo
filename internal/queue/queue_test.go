package queue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

func startMiniredis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return mr
}

func TestAsynqQueue_Enqueue(t *testing.T) {
	mr := startMiniredis(t)

	q, err := NewAsynqQueue(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqQueue: %v", err)
	}
	defer q.Close()

	result, err := q.Enqueue(context.Background(), Job{
		Type:    TypeIngestionManual,
		Queue:   QueueIngestion,
		Payload: IngestionPayload{TenantID: "t1", SourceID: "crm"},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if result.ID == "" {
		t.Fatal("expected non-empty job ID")
	}
	if result.Queue != QueueIngestion {
		t.Fatalf("expected queue %q, got %q", QueueIngestion, result.Queue)
	}
}

func TestAsynqQueue_EnqueueAt(t *testing.T) {
	mr := startMiniredis(t)

	q, err := NewAsynqQueue(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqQueue: %v", err)
	}
	defer q.Close()

	processAt := time.Now().Add(1 * time.Hour)
	result, err := q.EnqueueAt(context.Background(), Job{
		Type:    TypeReportDaily,
		Queue:   QueueReport,
		Payload: ReportPayload{TenantID: "t1", ReportType: "daily"},
	}, processAt)
	if err != nil {
		t.Fatalf("EnqueueAt: %v", err)
	}
	if result.ID == "" {
		t.Fatal("expected non-empty job ID")
	}
}

func TestAsynqQueue_Close(t *testing.T) {
	mr := startMiniredis(t)

	q, err := NewAsynqQueue(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqQueue: %v", err)
	}

	// Close should be idempotent — calling twice must not panic or error.
	if err := q.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestAsynqMiniredis_EnqueueAndProcess verifies that asynq works with miniredis:
// a task enqueued via the client is processed by the server handler.
func TestAsynqMiniredis_EnqueueAndProcess(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	redisAddr := mr.Addr()

	// Create client and enqueue a task.
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer client.Close()

	const taskType = "test:sanity"
	task := asynq.NewTask(taskType, []byte(`{"ok":true}`))

	info, err := client.Enqueue(task, asynq.Queue("test"))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if info.Queue != "test" {
		t.Fatalf("expected queue 'test', got %q", info.Queue)
	}

	// Create server and register handler.
	var processed atomic.Int32

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 1,
			Queues:      map[string]int{"test": 1},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(taskType, func(ctx context.Context, task *asynq.Task) error {
		processed.Add(1)
		return nil
	})

	// Start server in background.
	if err := srv.Start(mux); err != nil {
		t.Fatalf("start server: %v", err)
	}

	// Wait for processing (with timeout).
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			srv.Shutdown()
			t.Fatal("timed out waiting for task processing")
		case <-ticker.C:
			if processed.Load() > 0 {
				srv.Shutdown()
				return // success
			}
		}
	}
}
