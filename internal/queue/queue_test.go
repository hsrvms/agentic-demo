package queue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

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
