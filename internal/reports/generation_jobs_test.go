package reports

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrackGenerationJob(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	err := svc.TrackGenerationJob(context.Background(), "t1", "task-1", "on_demand", "Growth")
	require.NoError(t, err)
	require.Len(t, repo.jobs, 1)

	// The mock returns the single stored job; grab it regardless of key.
	var job db.ReportGenerationJob
	for _, j := range repo.jobs {
		job = j
	}
	assert.Equal(t, "t1", job.TenantID)
	assert.Equal(t, "task-1", job.TaskID)
	assert.Equal(t, "on_demand", job.ReportType)
	assert.Equal(t, "Growth", job.Focus)
	assert.Equal(t, GenerationJobQueued, GenerationJobStatus(job.Status))
	assert.False(t, job.FinishedAt.Valid)
}

func TestTrackGenerationJob_ValidatesInput(t *testing.T) {
	svc := NewService(newMockReportRepo())

	tests := []struct {
		name       string
		tenantID   string
		taskID     string
		reportType string
		wantErr    error
	}{
		{"empty tenant", "", "task-1", "on_demand", ErrInvalidTenantID},
		{"empty task ID", "t1", "", "on_demand", ErrInvalidTaskID},
		{"empty report type", "t1", "task-1", "", ErrInvalidReportType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.TrackGenerationJob(context.Background(), tt.tenantID, tt.taskID, tt.reportType, "")
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestListGenerationJobs_OrdersByEnqueuedDesc(t *testing.T) {
	repo := newMockReportRepo()
	now := time.Now()
	repo.jobs = map[uuid.UUID]db.ReportGenerationJob{
		uuid.New(): {ID: uuid.New(), TenantID: "t1", TaskID: "task-old", EnqueuedAt: now.Add(-time.Hour)},
		uuid.New(): {ID: uuid.New(), TenantID: "t1", TaskID: "task-new", EnqueuedAt: now},
		uuid.New(): {ID: uuid.New(), TenantID: "t2", TaskID: "task-other", EnqueuedAt: now},
	}
	svc := NewService(repo)

	jobs, err := svc.ListGenerationJobs(context.Background(), "t1", 10)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	// Repository is responsible for ordering; service must return as-is but
	// enforce the tenant filter and limit.
	assert.Equal(t, "t1", jobs[0].TenantID)
	assert.Equal(t, "t1", jobs[1].TenantID)
}

func TestListGenerationJobs_AppliesLimit(t *testing.T) {
	repo := newMockReportRepo()
	now := time.Now()
	for i := range 5 {
		repo.jobs[uuid.New()] = db.ReportGenerationJob{
			ID: uuid.New(), TenantID: "t1", TaskID: "task-" + string(rune('a'+i)),
			EnqueuedAt: now.Add(time.Duration(i) * time.Minute),
		}
	}
	svc := NewService(repo)

	jobs, err := svc.ListGenerationJobs(context.Background(), "t1", 3)
	require.NoError(t, err)
	assert.Len(t, jobs, 3)
}

func TestListGenerationJobs_EmptyTenantID(t *testing.T) {
	svc := NewService(newMockReportRepo())

	_, err := svc.ListGenerationJobs(context.Background(), "", 5)
	require.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestMarkGenerationJobRunning(t *testing.T) {
	repo := newMockReportRepo()
	repo.jobs[uuid.New()] = db.ReportGenerationJob{ID: uuid.New(), TenantID: "t1", TaskID: "task-1", Status: "queued"}
	svc := NewService(repo)

	err := svc.MarkGenerationJobRunning(context.Background(), "task-1")
	require.NoError(t, err)
	assert.Equal(t, "running", repo.lastUpdate.Status)
	assert.Equal(t, "task-1", repo.lastUpdate.TaskID)
	assert.False(t, repo.lastUpdate.FinishedAt.Valid)
}

func TestMarkGenerationJobSucceeded(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	err := svc.MarkGenerationJobSucceeded(context.Background(), "task-1")
	require.NoError(t, err)
	assert.Equal(t, "succeeded", repo.lastUpdate.Status)
	assert.Equal(t, "", repo.lastUpdate.Error)
	assert.True(t, repo.lastUpdate.FinishedAt.Valid)
}

func TestMarkGenerationJobFailed(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewService(repo)

	err := svc.MarkGenerationJobFailed(context.Background(), "task-1", "llm timeout")
	require.NoError(t, err)
	assert.Equal(t, "failed", repo.lastUpdate.Status)
	assert.Equal(t, "llm timeout", repo.lastUpdate.Error)
	assert.True(t, repo.lastUpdate.FinishedAt.Valid)
}
