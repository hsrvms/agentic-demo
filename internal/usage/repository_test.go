package usage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRepository implements Repository for testing.
type mockRepository struct {
	events      []UsageEventRecord
	dailyRows   []UsageDailyRecord
	createErr   error
	listErr     error
	upsertErr   error
	summaryErr  error
}

func (m *mockRepository) CreateEvent(ctx context.Context, params *db.CreateUsageEventParams) (UsageEventRecord, error) {
	if m.createErr != nil {
		return UsageEventRecord{}, m.createErr
	}
	rec := UsageEventRecord{
		ID:        uuid.New(),
		TenantID:  params.TenantID,
		EventType: params.EventType,
		Payload:   json.RawMessage(params.Payload),
		CreatedAt: time.Now(),
	}
	m.events = append(m.events, rec)
	return rec, nil
}

func (m *mockRepository) ListEvents(ctx context.Context, params *db.ListUsageEventsParams) ([]UsageEventRecord, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.events, nil
}

func (m *mockRepository) CountEvents(ctx context.Context, params *db.CountUsageEventsParams) (int32, error) {
	return int32(len(m.events)), nil
}

func (m *mockRepository) UpsertDaily(ctx context.Context, params *db.UpsertUsageDailyParams) (UsageDailyRecord, error) {
	if m.upsertErr != nil {
		return UsageDailyRecord{}, m.upsertErr
	}
	rec := UsageDailyRecord{
		ID:               uuid.New(),
		TenantID:         params.TenantID,
		Date:             pgDateToTime(params.Date),
		Model:            params.LlmModel,
		InputTokens:      params.InputTokens,
		OutputTokens:     params.OutputTokens,
		ToolCalls:        params.ToolCalls,
		EmbeddingTokens:  params.EmbeddingTokens,
		EstimatedCostUSD: pgNumericToFloat64(params.EstimatedCostUsd),
		ReportsGenerated: params.ReportsGenerated,
	}
	m.dailyRows = append(m.dailyRows, rec)
	return rec, nil
}

func (m *mockRepository) GetDailySummary(ctx context.Context, params *db.GetUsageDailySummaryParams) ([]UsageDailyRecord, error) {
	if m.summaryErr != nil {
		return nil, m.summaryErr
	}
	return m.dailyRows, nil
}

func (m *mockRepository) CountDaily(ctx context.Context, params *db.CountUsageDailyParams) (int32, error) {
	return int32(len(m.dailyRows)), nil
}

func TestRepositoryInterface(t *testing.T) {
	// Compile-time check that mockRepository satisfies Repository.
	var _ Repository = (*mockRepository)(nil)
}

func TestMockRepository_CreateEvent(t *testing.T) {
	repo := &mockRepository{}
	ctx := context.Background()

	payload := json.RawMessage(`{"model":"qwen-max","input_tokens":100}`)
	rec, err := repo.CreateEvent(ctx, &db.CreateUsageEventParams{
		TenantID:  "tenant-1",
		EventType: "llm_usage",
		Payload:   []byte(payload),
	})

	require.NoError(t, err)
	assert.Equal(t, "tenant-1", rec.TenantID)
	assert.Equal(t, "llm_usage", rec.EventType)
	assert.Equal(t, payload, rec.Payload)
	assert.NotEqual(t, uuid.Nil, rec.ID)
	assert.Len(t, repo.events, 1)
}

func TestMockRepository_ListEvents(t *testing.T) {
	repo := &mockRepository{
		events: []UsageEventRecord{
			{TenantID: "tenant-1", EventType: "llm_usage"},
			{TenantID: "tenant-1", EventType: "tool_usage"},
		},
	}

	events, err := repo.ListEvents(context.Background(), &db.ListUsageEventsParams{
		TenantID: "tenant-1",
		Limit:    10,
	})

	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestMockRepository_UpsertDaily(t *testing.T) {
	repo := &mockRepository{}
	ctx := context.Background()

	rec, err := repo.UpsertDaily(ctx, &db.UpsertUsageDailyParams{
		TenantID:        "tenant-1",
		Date:            toPgDate(time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)),
		LlmModel:        "qwen-max",
		InputTokens:     1000,
		OutputTokens:    500,
		ToolCalls:       3,
		EmbeddingTokens: 0,
		EstimatedCostUsd: toPgNumeric(0.005),
		ReportsGenerated: 1,
	})

	require.NoError(t, err)
	assert.Equal(t, "tenant-1", rec.TenantID)
	assert.Equal(t, "qwen-max", rec.Model)
	assert.Equal(t, int64(1000), rec.InputTokens)
	assert.Equal(t, int64(500), rec.OutputTokens)
	assert.Equal(t, int32(3), rec.ToolCalls)
	assert.InDelta(t, 0.005, rec.EstimatedCostUSD, 0.0001)
	assert.Len(t, repo.dailyRows, 1)
}

func TestMockRepository_GetDailySummary(t *testing.T) {
	repo := &mockRepository{
		dailyRows: []UsageDailyRecord{
			{TenantID: "tenant-1", Model: "qwen-max", InputTokens: 100, Date: time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)},
		},
	}

	rows, err := repo.GetDailySummary(context.Background(), &db.GetUsageDailySummaryParams{
		TenantID: "tenant-1",
	})

	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.Equal(t, "qwen-max", rows[0].Model)
}

func TestMockRepository_CountEvents(t *testing.T) {
	repo := &mockRepository{
		events: []UsageEventRecord{{}, {}, {}},
	}

	count, err := repo.CountEvents(context.Background(), &db.CountUsageEventsParams{
		TenantID: "tenant-1",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(3), count)
}

func TestPgDateToTime_Null(t *testing.T) {
	result := pgDateToTime(pgtype.Date{Valid: false})
	assert.True(t, result.IsZero())
}

func TestPgDateToTime_Valid(t *testing.T) {
	d := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	result := pgDateToTime(pgtype.Date{Time: d, Valid: true})
	assert.Equal(t, d, result)
}

func TestPgNumericToFloat64_Null(t *testing.T) {
	result := pgNumericToFloat64(pgtype.Numeric{Valid: false})
	assert.Equal(t, float64(0), result)
}

func TestToPgDate_Zero(t *testing.T) {
	result := toPgDate(time.Time{})
	assert.False(t, result.Valid)
}

func TestToPgDate_Valid(t *testing.T) {
	d := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	result := toPgDate(d)
	assert.True(t, result.Valid)
	assert.Equal(t, d, result.Time)
}

func TestToPgTimestamptz_Zero(t *testing.T) {
	result := toPgTimestamptz(time.Time{})
	assert.True(t, result.IsZero())
}

func TestToPgTimestamptz_Valid(t *testing.T) {
	now := time.Now()
	result := toPgTimestamptz(now)
	assert.Equal(t, now, result)
}