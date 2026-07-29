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

// ReportService manages persisted reports. Handlers and the queue
// worker use this interface.
type ReportService interface {
	Create(ctx context.Context, params *CreateReportParams) (StoredReport, error)
	GetByID(ctx context.Context, id uuid.UUID) (StoredReport, error)
	ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (ReportPage, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
