package usage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockReader implements UsageReader for testing.
type mockReader struct {
	currentUsage *CurrentUsage
	err          error
}

func (m *mockReader) GetCurrentUsage(ctx context.Context, tenantID string) (*CurrentUsage, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.currentUsage, nil
}

func (m *mockReader) Close() error { return nil }

func TestService_GetSummary(t *testing.T) {
	repo := &mockRepository{
		dailyRows: []UsageDailyRecord{
			{
				TenantID:         "tenant-1",
				Model:            "qwen-max",
				InputTokens:      1000,
				OutputTokens:     500,
				ToolCalls:        3,
				EstimatedCostUSD: 0.005,
				Date:             time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	reader := &mockReader{}
	svc := NewService(repo, reader)

	summary, err := svc.GetSummary(context.Background(), "tenant-1", "2026-07-01", "2026-07-31")
	require.NoError(t, err)

	assert.Equal(t, "tenant-1", summary.TenantID)
	assert.InDelta(t, 0.005, summary.TotalCostUSD, 0.0001)
	require.Len(t, summary.Models, 1)
	assert.Equal(t, "qwen-max", summary.Models[0].Model)
	assert.Equal(t, int64(1000), summary.Models[0].InputTokens)
}

func TestService_GetSummary_InvalidTenantID(t *testing.T) {
	svc := NewService(&mockRepository{}, &mockReader{})

	_, err := svc.GetSummary(context.Background(), "", "", "")
	require.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestService_GetSummary_InvalidDateRange(t *testing.T) {
	svc := NewService(&mockRepository{}, &mockReader{})

	_, err := svc.GetSummary(context.Background(), "tenant-1", "2026-07-31", "2026-07-01")
	require.ErrorIs(t, err, ErrInvalidDateRange)
}

func TestService_GetCurrentUsage(t *testing.T) {
	expected := &CurrentUsage{
		TenantID:      "tenant-1",
		TotalCostUSD:  0.005,
		ByModel:       []ModelUsage{{Model: "qwen-max", InputTokens: 100}},
	}
	reader := &mockReader{currentUsage: expected}
	svc := NewService(&mockRepository{}, reader)

	result, err := svc.GetCurrentUsage(context.Background(), "tenant-1")
	require.NoError(t, err)

	assert.Equal(t, "tenant-1", result.TenantID)
	assert.InDelta(t, 0.005, result.TotalCostUSD, 0.0001)
}

func TestService_GetCurrentUsage_InvalidTenantID(t *testing.T) {
	svc := NewService(&mockRepository{}, &mockReader{})

	_, err := svc.GetCurrentUsage(context.Background(), "")
	require.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestService_ListEvents(t *testing.T) {
	repo := &mockRepository{
		events: []UsageEventRecord{
			{TenantID: "tenant-1", EventType: "llm_usage"},
			{TenantID: "tenant-1", EventType: "tool_usage"},
		},
	}
	svc := NewService(repo, &mockReader{})

	page, err := svc.ListEvents(context.Background(), "tenant-1", 1, 20)
	require.NoError(t, err)

	assert.Equal(t, 2, page.TotalCount)
	assert.Len(t, page.Events, 2)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 20, page.PageSize)
}

func TestService_ListEvents_InvalidTenantID(t *testing.T) {
	svc := NewService(&mockRepository{}, &mockReader{})

	_, err := svc.ListEvents(context.Background(), "", 1, 20)
	require.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestService_ListEvents_DefaultPagination(t *testing.T) {
	repo := &mockRepository{}
	svc := NewService(repo, &mockReader{})

	page, err := svc.ListEvents(context.Background(), "tenant-1", 0, 0)
	require.NoError(t, err)

	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 20, page.PageSize)
}

func TestParseDateRange(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{"valid range", "2026-07-01", "2026-07-31", false},
		{"empty range", "", "", false},
		{"from only", "2026-07-01", "", false},
		{"to only", "", "2026-07-31", false},
		{"invalid from", "bad", "2026-07-31", true},
		{"invalid to", "2026-07-01", "bad", true},
		{"from after to", "2026-07-31", "2026-07-01", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseDateRange(tt.from, tt.to)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}