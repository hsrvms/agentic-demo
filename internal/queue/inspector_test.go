package queue

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

func TestMapTaskState(t *testing.T) {
	tests := []struct {
		state asynq.TaskState
		want  JobStatus
	}{
		{asynq.TaskStatePending, JobQueued},
		{asynq.TaskStateScheduled, JobQueued},
		{asynq.TaskStateRetry, JobQueued},
		{asynq.TaskStateAggregating, JobQueued},
		{asynq.TaskStateActive, JobRunning},
		{asynq.TaskStateCompleted, JobSucceeded},
		{asynq.TaskStateArchived, JobFailed},
		{asynq.TaskState(99), JobUnknown},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.state), func(t *testing.T) {
			if got := mapTaskState(tt.state); got != tt.want {
				t.Errorf("mapTaskState(%d) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestAsynqInspector_GetJobState(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	insp, err := NewAsynqInspector(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqInspector: %v", err)
	}
	defer insp.Close()

	q, err := NewAsynqQueue(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqQueue: %v", err)
	}
	defer q.Close()

	// A freshly enqueued task is pending → queued.
	result, err := q.Enqueue(context.Background(), Job{
		Type:    TypeReportOnDemand,
		Queue:   QueueReport,
		Payload: ReportPayload{TenantID: "t1", ReportType: "on_demand"},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	state, err := insp.GetJobState(context.Background(), result.ID)
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if state.ID != result.ID {
		t.Errorf("state.ID = %q, want %q", state.ID, result.ID)
	}
	if state.Status != JobQueued {
		t.Errorf("state.Status = %q, want %q", state.Status, JobQueued)
	}
}

func TestAsynqInspector_GetJobState_UnknownTask(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	insp, err := NewAsynqInspector(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqInspector: %v", err)
	}
	defer insp.Close()

	_, err = insp.GetJobState(context.Background(), "no-such-task")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("GetJobState = %v, want ErrJobNotFound", err)
	}
}

func TestAsynqInspector_GetJobState_Completed(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	insp, err := NewAsynqInspector(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqInspector: %v", err)
	}
	defer insp.Close()

	// Enqueue and process the task so it lands in the completed state.
	var processed atomic.Bool
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: mr.Addr()},
		asynq.Config{Concurrency: 1, Queues: map[string]int{QueueReport: 1}},
	)
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeReportOnDemand, func(ctx context.Context, task *asynq.Task) error {
		processed.Store(true)
		return nil
	})
	if err := srv.Start(mux); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Shutdown()

	q, err := NewAsynqQueue(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqQueue: %v", err)
	}
	defer q.Close()

	result, err := q.Enqueue(context.Background(), Job{
		Type:    TypeReportOnDemand,
		Queue:   QueueReport,
		Payload: ReportPayload{TenantID: "t1", ReportType: "on_demand"},
		Options: []asynq.Option{asynq.Retention(time.Hour)},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !processed.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !processed.Load() {
		t.Fatal("timed out waiting for task processing")
	}

	state, err := insp.GetJobState(context.Background(), result.ID)
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if state.Status != JobSucceeded {
		t.Errorf("state.Status = %q, want %q", state.Status, JobSucceeded)
	}
}

func TestAsynqInspector_GetJobState_Archived(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	insp, err := NewAsynqInspector(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqInspector: %v", err)
	}
	defer insp.Close()

	var processed atomic.Bool
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: mr.Addr()},
		asynq.Config{Concurrency: 1, Queues: map[string]int{QueueReport: 1}},
	)
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeReportOnDemand, func(ctx context.Context, task *asynq.Task) error {
		processed.Store(true)
		return errors.New("boom")
	})
	if err := srv.Start(mux); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Shutdown()

	q, err := NewAsynqQueue(mr.Addr())
	if err != nil {
		t.Fatalf("NewAsynqQueue: %v", err)
	}
	defer q.Close()

	result, err := q.Enqueue(context.Background(), Job{
		Type:    TypeReportOnDemand,
		Queue:   QueueReport,
		Payload: ReportPayload{TenantID: "t1", ReportType: "on_demand"},
		Options: []asynq.Option{asynq.MaxRetry(0)},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !processed.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !processed.Load() {
		t.Fatal("timed out waiting for task processing")
	}

	state, err := insp.GetJobState(context.Background(), result.ID)
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if state.Status != JobFailed {
		t.Errorf("state.Status = %q, want %q", state.Status, JobFailed)
	}
	if state.Error == "" {
		t.Error("expected error message on failed job")
	}
}
