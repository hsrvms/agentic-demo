package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Repository abstracts data source persistence.
type Repository interface {
	Create(ctx context.Context, params *db.CreateDataSourceParams) (db.DataSourceConfig, error)
	GetByID(ctx context.Context, id uuid.UUID) (db.DataSourceConfig, error)
	ListByTenant(ctx context.Context, params *db.ListDataSourcesByTenantParams) ([]db.DataSourceConfig, error)
	CountByTenant(ctx context.Context, tenantID string) (int32, error)
	Update(ctx context.Context, params *db.UpdateDataSourceParams) (db.DataSourceConfig, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateSyncStatus(ctx context.Context, params *db.UpdateDataSourceSyncStatusParams) (db.DataSourceConfig, error)
}

// pgRepository wraps sqlc-generated queries.
type pgRepository struct {
	queries *db.Queries
}

// NewRepository creates a data source Repository backed by PostgreSQL.
func NewRepository(queries *db.Queries) Repository {
	return &pgRepository{queries: queries}
}

func (r *pgRepository) Create(ctx context.Context, params *db.CreateDataSourceParams) (db.DataSourceConfig, error) {
	row, err := r.queries.CreateDataSource(ctx, *params)
	if err != nil {
		return db.DataSourceConfig{}, fmt.Errorf("create data source: %w", err)
	}
	return row, nil
}

func (r *pgRepository) GetByID(ctx context.Context, id uuid.UUID) (db.DataSourceConfig, error) {
	row, err := r.queries.GetDataSourceByID(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return db.DataSourceConfig{}, err
		}
		return db.DataSourceConfig{}, ErrNotFound
	}
	return row, nil
}

func (r *pgRepository) ListByTenant(ctx context.Context, params *db.ListDataSourcesByTenantParams) ([]db.DataSourceConfig, error) {
	rows, err := r.queries.ListDataSourcesByTenant(ctx, *params)
	if err != nil {
		return nil, fmt.Errorf("list data sources: %w", err)
	}
	return rows, nil
}

func (r *pgRepository) CountByTenant(ctx context.Context, tenantID string) (int32, error) {
	count, err := r.queries.CountDataSourcesByTenant(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("count data sources: %w", err)
	}
	return count, nil
}

func (r *pgRepository) Update(ctx context.Context, params *db.UpdateDataSourceParams) (db.DataSourceConfig, error) {
	row, err := r.queries.UpdateDataSource(ctx, *params)
	if err != nil {
		return db.DataSourceConfig{}, fmt.Errorf("update data source: %w", err)
	}
	return row, nil
}

func (r *pgRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteDataSource(ctx, id); err != nil {
		return fmt.Errorf("delete data source: %w", err)
	}
	return nil
}

func (r *pgRepository) UpdateSyncStatus(ctx context.Context, params *db.UpdateDataSourceSyncStatusParams) (db.DataSourceConfig, error) {
	row, err := r.queries.UpdateDataSourceSyncStatus(ctx, *params)
	if err != nil {
		return db.DataSourceConfig{}, fmt.Errorf("update sync status: %w", err)
	}
	return row, nil
}

// --- domain conversion helpers ---

func toDomain(row *db.DataSourceConfig) DataSource {
	var ds DataSource
	ds.ID = row.ID
	ds.TenantID = row.TenantID
	ds.SourceType = SourceType(row.SourceType)
	ds.Name = row.Name
	ds.Config = json.RawMessage(row.Config)
	ds.Credentials = row.Credentials
	ds.Status = Status(row.Status)
	if row.LastSyncAt.Valid {
		ds.LastSyncAt = &row.LastSyncAt.Time
	}
	if row.LastSyncStatus.Valid {
		ds.LastSyncStatus = row.LastSyncStatus.String
	}
	ds.CreatedAt = row.CreatedAt
	ds.UpdatedAt = row.UpdatedAt
	return ds
}

func toPgSourceType(t SourceType) string {
	return string(t)
}

func toPgConfig(raw json.RawMessage) []byte {
	if raw == nil {
		return []byte("{}")
	}
	return []byte(raw)
}

func toPgTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func toPgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: s, Valid: true}
}
