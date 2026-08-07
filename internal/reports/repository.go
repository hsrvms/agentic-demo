package reports

import (
	"context"
	"errors"
	"fmt"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Repository abstracts report data access.
type Repository interface {
	Create(ctx context.Context, params *db.CreateReportParams) (db.Report, error)
	GetByID(ctx context.Context, id uuid.UUID) (db.Report, error)
	ListByTenant(ctx context.Context, params *db.ListReportsByTenantParams) ([]db.Report, error)
	CountByTenant(ctx context.Context, tenantID string) (int32, error)
	Delete(ctx context.Context, id uuid.UUID) error

	CreateGenerationJob(ctx context.Context, params *db.CreateGenerationJobParams) (db.ReportGenerationJob, error)
	ListGenerationJobsByTenant(ctx context.Context, params *db.ListGenerationJobsByTenantParams) ([]db.ReportGenerationJob, error)
	UpdateGenerationJob(ctx context.Context, params *db.UpdateGenerationJobParams) error
}

// pgRepository wraps sqlc-generated queries.
type pgRepository struct {
	queries *db.Queries
}

// NewRepository creates a report Repository backed by PostgreSQL.
func NewRepository(queries *db.Queries) Repository {
	return &pgRepository{queries: queries}
}

func (r *pgRepository) Create(ctx context.Context, params *db.CreateReportParams) (db.Report, error) {
	row, err := r.queries.CreateReport(ctx, *params)
	if err != nil {
		return db.Report{}, fmt.Errorf("create report: %w", err)
	}
	return row, nil
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (db.Report, error) {
	row, err := r.queries.GetReportByID(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return db.Report{}, err
		}
		return db.Report{}, ErrReportNotFound
	}
	return row, nil
}

func (r *pgRepository) ListByTenant(ctx context.Context, params *db.ListReportsByTenantParams) ([]db.Report, error) {
	rows, err := r.queries.ListReportsByTenant(ctx, *params)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	return rows, nil
}

func (r *pgRepository) CountByTenant(ctx context.Context, tenantID string) (int32, error) {
	count, err := r.queries.CountReportsByTenant(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("count reports: %w", err)
	}
	return count, nil
}

func (r *pgRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteReport(ctx, id); err != nil {
		return fmt.Errorf("delete report: %w", err)
	}
	return nil
}

func (r *pgRepository) CreateGenerationJob(ctx context.Context, params *db.CreateGenerationJobParams) (db.ReportGenerationJob, error) {
	row, err := r.queries.CreateGenerationJob(ctx, *params)
	if err != nil {
		return db.ReportGenerationJob{}, fmt.Errorf("create generation job: %w", err)
	}
	return row, nil
}

func (r *pgRepository) ListGenerationJobsByTenant(ctx context.Context, params *db.ListGenerationJobsByTenantParams) ([]db.ReportGenerationJob, error) {
	rows, err := r.queries.ListGenerationJobsByTenant(ctx, *params)
	if err != nil {
		return nil, fmt.Errorf("list generation jobs: %w", err)
	}
	return rows, nil
}

func (r *pgRepository) UpdateGenerationJob(ctx context.Context, params *db.UpdateGenerationJobParams) error {
	if err := r.queries.UpdateGenerationJob(ctx, *params); err != nil {
		return fmt.Errorf("update generation job: %w", err)
	}
	return nil
}

// --- helpers for nullable DB fields ---

// toPgText converts a string to pgtype.Text (null if empty).
func toPgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: s, Valid: true}
}

// toPgUUID converts a uuid.UUID to pgtype.UUID (null if zero).
func toPgUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// pgTextToString converts pgtype.Text to a plain string.
func pgTextToString(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

// pgUUIDToUUID converts pgtype.UUID to uuid.UUID.
func pgUUIDToUUID(u pgtype.UUID) uuid.UUID {
	if u.Valid {
		return uuid.UUID(u.Bytes)
	}
	return uuid.Nil
}
