package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Job describes a unit of work to be enqueued.
type Job struct {
	Type    string
	Queue   string
	Payload any
	Options []asynq.Option
}

// JobResult is returned after a job is successfully enqueued.
type JobResult struct {
	ID    string
	Queue string
}

// JobQueue is the public enqueue interface. HTTP handlers and the
// scheduler use this to submit work. The asynq client is hidden
// behind it.
type JobQueue interface {
	Enqueue(ctx context.Context, job Job) (*JobResult, error)
	EnqueueAt(ctx context.Context, job Job, processAt time.Time) (*JobResult, error)
	Close() error
}

// asynqQueue implements JobQueue using an asynq.Client.
type asynqQueue struct {
	client *asynq.Client
	closed bool
}

// NewAsynqQueue creates a JobQueue backed by the given Redis address.
func NewAsynqQueue(redisAddr string) (JobQueue, error) {
	if redisAddr == "" {
		return nil, fmt.Errorf("redis address is required")
	}
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	return &asynqQueue{client: client}, nil
}

func (q *asynqQueue) Enqueue(ctx context.Context, job Job) (*JobResult, error) {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	task := asynq.NewTask(job.Type, payload)
	opts := append([]asynq.Option{asynq.Queue(job.Queue)}, job.Options...)

	info, err := q.client.EnqueueContext(ctx, task, opts...)
	if err != nil {
		return nil, fmt.Errorf("enqueue task %s: %w", job.Type, err)
	}

	return &JobResult{
		ID:    info.ID,
		Queue: info.Queue,
	}, nil
}

func (q *asynqQueue) EnqueueAt(ctx context.Context, job Job, processAt time.Time) (*JobResult, error) {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	task := asynq.NewTask(job.Type, payload)
	opts := append([]asynq.Option{
		asynq.Queue(job.Queue),
		asynq.ProcessAt(processAt),
	}, job.Options...)

	info, err := q.client.EnqueueContext(ctx, task, opts...)
	if err != nil {
		return nil, fmt.Errorf("enqueue task %s at %s: %w", job.Type, processAt, err)
	}

	return &JobResult{
		ID:    info.ID,
		Queue: info.Queue,
	}, nil
}

func (q *asynqQueue) Close() error {
	if q.closed {
		return nil
	}
	q.closed = true
	return q.client.Close()
}
