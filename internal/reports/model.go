package reports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// StoredReport is a persisted report retrieved from the database.
type StoredReport struct {
	ID          uuid.UUID
	TenantID    string
	Type        string
	Title       string
	Content     string
	Citations   json.RawMessage
	Focus       string
	ScheduleID  uuid.UUID // zero value means no schedule
	GeneratedAt time.Time
	CreatedAt   time.Time
}

// CreateReportParams holds the inputs for persisting a new report.
type CreateReportParams struct {
	TenantID    string
	Type        string
	Title       string
	Content     string
	Citations   json.RawMessage
	Focus       string
	ScheduleID  uuid.UUID // zero value means no schedule
	GeneratedAt time.Time
}

// ReportPage is a paginated list of reports.
type ReportPage struct {
	Reports    []StoredReport
	TotalCount int
	Page       int
	PageSize   int
}

// GenerationJobStatus is the lifecycle status of a tracked report
// generation job.
type GenerationJobStatus string

// Generation job lifecycle statuses. "queued" and "running" are live states
// observed from the queue; "succeeded" and "failed" are durable outcomes
// recorded by the worker.
const (
	GenerationJobQueued    GenerationJobStatus = "queued"
	GenerationJobRunning   GenerationJobStatus = "running"
	GenerationJobSucceeded GenerationJobStatus = "succeeded"
	GenerationJobFailed    GenerationJobStatus = "failed"
)

// GenerationJob is a report generation job triggered from the web UI. The
// task ID links it to the asynq task for live status lookups.
type GenerationJob struct {
	ID         uuid.UUID
	TenantID   string
	TaskID     string
	ReportType string
	Focus      string
	Status     GenerationJobStatus
	Error      string
	EnqueuedAt time.Time
	FinishedAt *time.Time
}

// ReportService manages persisted reports and tracks report generation
// jobs. Handlers and the queue worker use this interface.
type ReportService interface {
	Create(ctx context.Context, params *CreateReportParams) (StoredReport, error)
	GetByID(ctx context.Context, id uuid.UUID) (StoredReport, error)
	ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (ReportPage, error)
	Delete(ctx context.Context, id uuid.UUID) error

	// TrackGenerationJob records a report generation job enqueued from the
	// web UI so its status can be shown on the reports page.
	TrackGenerationJob(ctx context.Context, tenantID, taskID, reportType, focus string) error
	// ListGenerationJobs returns the most recent tracked generation jobs for
	// a tenant, newest first.
	ListGenerationJobs(ctx context.Context, tenantID string, limit int) ([]GenerationJob, error)
	// MarkGenerationJobRunning/Succeeded/Failed record the worker's observed
	// outcome for a job. They are no-ops for task IDs that were never tracked.
	MarkGenerationJobRunning(ctx context.Context, taskID string) error
	MarkGenerationJobSucceeded(ctx context.Context, taskID string) error
	MarkGenerationJobFailed(ctx context.Context, taskID, errMsg string) error
}
