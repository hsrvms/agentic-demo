// Package scheduling manages report schedules — cron-driven periodic
// report generation for each tenant.
package scheduling

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ScheduleType is the cadence of a scheduled report.
type ScheduleType string

const (
	ScheduleDaily   ScheduleType = "daily"
	ScheduleWeekly  ScheduleType = "weekly"
	ScheduleMonthly ScheduleType = "monthly"
)

// ReportSchedule is a recurring report generation rule for a tenant.
type ReportSchedule struct {
	ID        uuid.UUID
	TenantID  string
	Type      ScheduleType
	CronExpr  string
	Focus     string
	Format    string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateScheduleParams holds the inputs needed to create a schedule.
type CreateScheduleParams struct {
	TenantID string
	Type     ScheduleType
	CronExpr string
	Focus    string
	Format   string
}

// UpdateScheduleParams holds the inputs for updating an existing schedule.
type UpdateScheduleParams struct {
	ID       uuid.UUID
	Type     ScheduleType
	CronExpr string
	Focus    string
	Format   string
}

// ScheduleService is the module's public interface. Handlers and the
// worker use this to manage report schedules.
type ScheduleService interface {
	Create(ctx context.Context, params *CreateScheduleParams) (ReportSchedule, error)
	GetByID(ctx context.Context, id uuid.UUID) (ReportSchedule, error)
	ListByTenant(ctx context.Context, tenantID string) ([]ReportSchedule, error)
	Update(ctx context.Context, params *UpdateScheduleParams) (ReportSchedule, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Toggle(ctx context.Context, id uuid.UUID) (ReportSchedule, error)
	ListAllEnabled(ctx context.Context) ([]ReportSchedule, error)
}
