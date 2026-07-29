package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Repository abstracts usage data access.
type Repository interface {
	CreateEvent(ctx context.Context, params *db.CreateUsageEventParams) (UsageEventRecord, error)
	ListEvents(ctx context.Context, params *db.ListUsageEventsParams) ([]UsageEventRecord, error)
	CountEvents(ctx context.Context, params *db.CountUsageEventsParams) (int32, error)
	UpsertDaily(ctx context.Context, params *db.UpsertUsageDailyParams) (UsageDailyRecord, error)
	GetDailySummary(ctx context.Context, params *db.GetUsageDailySummaryParams) ([]UsageDailyRecord, error)
	CountDaily(ctx context.Context, params *db.CountUsageDailyParams) (int32, error)
}

// pgRepository wraps sqlc-generated queries.
type pgRepository struct {
	queries *db.Queries
}

// NewRepository creates a usage Repository backed by PostgreSQL.
func NewRepository(queries *db.Queries) Repository {
	return &pgRepository{queries: queries}
}

func (r *pgRepository) CreateEvent(ctx context.Context, params *db.CreateUsageEventParams) (UsageEventRecord, error) {
	row, err := r.queries.CreateUsageEvent(ctx, *params)
	if err != nil {
		return UsageEventRecord{}, fmt.Errorf("create usage event: %w", err)
	}
	return toEventRecord(&row), nil
}

func (r *pgRepository) ListEvents(ctx context.Context, params *db.ListUsageEventsParams) ([]UsageEventRecord, error) {
	rows, err := r.queries.ListUsageEvents(ctx, *params)
	if err != nil {
		return nil, fmt.Errorf("list usage events: %w", err)
	}
	records := make([]UsageEventRecord, len(rows))
	for i := range rows {
		records[i] = toEventRecord(&rows[i])
	}
	return records, nil
}

func (r *pgRepository) CountEvents(ctx context.Context, params *db.CountUsageEventsParams) (int32, error) {
	count, err := r.queries.CountUsageEvents(ctx, *params)
	if err != nil {
		return 0, fmt.Errorf("count usage events: %w", err)
	}
	return count, nil
}

func (r *pgRepository) UpsertDaily(ctx context.Context, params *db.UpsertUsageDailyParams) (UsageDailyRecord, error) {
	row, err := r.queries.UpsertUsageDaily(ctx, *params)
	if err != nil {
		return UsageDailyRecord{}, fmt.Errorf("upsert usage daily: %w", err)
	}
	return toDailyRecord(&row), nil
}

func (r *pgRepository) GetDailySummary(ctx context.Context, params *db.GetUsageDailySummaryParams) ([]UsageDailyRecord, error) {
	rows, err := r.queries.GetUsageDailySummary(ctx, *params)
	if err != nil {
		return nil, fmt.Errorf("get usage daily summary: %w", err)
	}
	records := make([]UsageDailyRecord, len(rows))
	for i := range rows {
		records[i] = toDailySummaryRecord(&rows[i])
	}
	return records, nil
}

func (r *pgRepository) CountDaily(ctx context.Context, params *db.CountUsageDailyParams) (int32, error) {
	count, err := r.queries.CountUsageDaily(ctx, *params)
	if err != nil {
		return 0, fmt.Errorf("count usage daily: %w", err)
	}
	return count, nil
}

// --- domain conversion ---

func toEventRecord(row *db.UsageEvent) UsageEventRecord {
	return UsageEventRecord{
		ID:        row.ID,
		TenantID:  row.TenantID,
		EventType: row.EventType,
		Payload:   json.RawMessage(row.Payload),
		CreatedAt: row.CreatedAt,
	}
}

func toDailyRecord(row *db.UsageDaily) UsageDailyRecord {
	return UsageDailyRecord{
		ID:               row.ID,
		TenantID:         row.TenantID,
		Date:             pgDateToTime(row.Date),
		Model:            row.LlmModel,
		InputTokens:      row.InputTokens,
		OutputTokens:     row.OutputTokens,
		ToolCalls:        row.ToolCalls,
		EmbeddingTokens:  row.EmbeddingTokens,
		EstimatedCostUSD: pgNumericToFloat64(row.EstimatedCostUsd),
		ReportsGenerated: row.ReportsGenerated,
	}
}

func toDailySummaryRecord(row *db.GetUsageDailySummaryRow) UsageDailyRecord {
	return UsageDailyRecord{
		TenantID:         row.TenantID,
		Date:             pgDateToTime(row.Date),
		Model:            row.LlmModel,
		InputTokens:      row.InputTokens,
		OutputTokens:     row.OutputTokens,
		ToolCalls:        row.ToolCalls,
		EmbeddingTokens:  row.EmbeddingTokens,
		EstimatedCostUSD: pgNumericToFloat64(row.EstimatedCostUsd),
		ReportsGenerated: row.ReportsGenerated,
	}
}

// --- pgtype helpers ---

func pgDateToTime(d pgtype.Date) time.Time {
	if d.Valid {
		return d.Time
	}
	return time.Time{}
}

func pgNumericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, _ := n.Float64Value()
	return f.Float64
}

// toPgDate converts a time.Time to pgtype.Date.
func toPgDate(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}

// toPgNumeric converts a float64 to pgtype.Numeric.
func toPgNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(f, 'f', -1, 64))
	return n
}

// toPgTimestamptz converts a time.Time to a nullable timestamptz.
func toPgTimestamptz(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t
}

// pgUUIDBytes converts a uuid.UUID to [16]byte for pgtype.UUID.
func pgUUIDBytes(id uuid.UUID) [16]byte {
	return [16]byte(id)
}