package scheduling

import (
	"context"
	"errors"
	"fmt"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
)

// Repository abstracts schedule data access.
type Repository interface {
	Create(ctx context.Context, params *db.CreateScheduleParams) (db.ReportSchedule, error)
	GetByID(ctx context.Context, id uuid.UUID) (db.ReportSchedule, error)
	ListByTenant(ctx context.Context, tenantID string) ([]db.ReportSchedule, error)
	Update(ctx context.Context, params *db.UpdateScheduleParams) (db.ReportSchedule, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Toggle(ctx context.Context, id uuid.UUID) (db.ReportSchedule, error)
	ListAllEnabled(ctx context.Context) ([]db.ReportSchedule, error)
}

// pgRepository wraps sqlc-generated queries.
type pgRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &pgRepository{queries: queries}
}

func (r *pgRepository) Create(ctx context.Context, params *db.CreateScheduleParams) (db.ReportSchedule, error) {
	row, err := r.queries.CreateSchedule(ctx, *params)
	if err != nil {
		return db.ReportSchedule{}, fmt.Errorf("create schedule: %w", err)
	}
	return row, nil
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (db.ReportSchedule, error) {
	row, err := r.queries.GetScheduleByID(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return db.ReportSchedule{}, err
		}
		return db.ReportSchedule{}, ErrScheduleNotFound
	}
	return row, nil
}

func (r *pgRepository) ListByTenant(ctx context.Context, tenantID string) ([]db.ReportSchedule, error) {
	rows, err := r.queries.ListSchedulesByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	return rows, nil
}

func (r *pgRepository) Update(ctx context.Context, params *db.UpdateScheduleParams) (db.ReportSchedule, error) {
	row, err := r.queries.UpdateSchedule(ctx, *params)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return db.ReportSchedule{}, err
		}
		return db.ReportSchedule{}, ErrScheduleNotFound
	}
	return row, nil
}

func (r *pgRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.queries.DeleteSchedule(ctx, id)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	return nil
}

func (r *pgRepository) Toggle(ctx context.Context, id uuid.UUID) (db.ReportSchedule, error) {
	row, err := r.queries.ToggleSchedule(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return db.ReportSchedule{}, err
		}
		return db.ReportSchedule{}, ErrScheduleNotFound
	}
	return row, nil
}

func (r *pgRepository) ListAllEnabled(ctx context.Context) ([]db.ReportSchedule, error) {
	rows, err := r.queries.ListAllEnabledSchedules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled schedules: %w", err)
	}
	return rows, nil
}