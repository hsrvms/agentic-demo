package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
)

// JobStatus is the user-facing lifecycle status of a queued job.
type JobStatus string

// Job lifecycle statuses surfaced to the UI.
const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobUnknown   JobStatus = "unknown"
)

// ErrJobNotFound is returned when the queue has no record of the task.
var ErrJobNotFound = errors.New("job not found")

// JobState is the observed state of a single queued job.
type JobState struct {
	ID     string
	Status JobStatus
	Error  string // last failure message, only set for failed jobs
}

// JobInspector is the read-only counterpart of JobQueue. It reports the
// lifecycle state of previously enqueued jobs by task ID. It is implemented
// with the asynq Inspector.
type JobInspector interface {
	GetJobState(ctx context.Context, taskID string) (JobState, error)
	Close() error
}

// asynqInspector implements JobInspector with an asynq.Inspector.
type asynqInspector struct {
	inspector *asynq.Inspector
}

// NewAsynqInspector creates a JobInspector backed by the given Redis address.
// The address can be a host:port (e.g. "localhost:6379") or a redis:// URL.
func NewAsynqInspector(redisAddr string) (JobInspector, error) {
	if redisAddr == "" {
		return nil, fmt.Errorf("redis address is required")
	}
	opt, err := parseRedisAddr(redisAddr)
	if err != nil {
		return nil, err
	}
	return &asynqInspector{inspector: asynq.NewInspector(opt)}, nil
}

// GetJobState reports the state of the task with the given ID in the report
// queue. It returns ErrJobNotFound when the queue has no record of the task
// (e.g. the job was completed and its retention period expired).
func (i *asynqInspector) GetJobState(ctx context.Context, taskID string) (JobState, error) {
	info, err := i.inspector.GetTaskInfo(QueueReport, taskID)
	if err != nil {
		if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
			return JobState{}, ErrJobNotFound
		}
		return JobState{}, fmt.Errorf("inspect task %s: %w", taskID, err)
	}

	return JobState{
		ID:     info.ID,
		Status: mapTaskState(info.State),
		Error:  info.LastErr,
	}, nil
}

func (i *asynqInspector) Close() error {
	return i.inspector.Close()
}

// mapTaskState converts an asynq task state into a user-facing JobStatus.
// Pending, scheduled, and retry tasks are all waiting to run (queued).
func mapTaskState(state asynq.TaskState) JobStatus {
	switch state {
	case asynq.TaskStatePending, asynq.TaskStateScheduled, asynq.TaskStateRetry, asynq.TaskStateAggregating:
		return JobQueued
	case asynq.TaskStateActive:
		return JobRunning
	case asynq.TaskStateCompleted:
		return JobSucceeded
	case asynq.TaskStateArchived:
		return JobFailed
	default:
		return JobUnknown
	}
}
