package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockService implements UsageService for testing.
type mockService struct {
	summary       *UsageSummary
	current       *CurrentUsage
	events        *UsageEventPage
	summaryErr    error
	currentErr    error
	eventsErr     error
}

func (m *mockService) GetSummary(ctx context.Context, tenantID string, from, to string) (*UsageSummary, error) {
	if m.summaryErr != nil {
		return nil, m.summaryErr
	}
	return m.summary, nil
}

func (m *mockService) GetCurrentUsage(ctx context.Context, tenantID string) (*CurrentUsage, error) {
	if m.currentErr != nil {
		return nil, m.currentErr
	}
	return m.current, nil
}

func (m *mockService) ListEvents(ctx context.Context, tenantID string, page, pageSize int) (*UsageEventPage, error) {
	if m.eventsErr != nil {
		return nil, m.eventsErr
	}
	return m.events, nil
}

func setupEcho(t *testing.T) (*echo.Echo, echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Set tenant ID in context.
	ctx := auth.SetTenantID(req.Context(), "tenant-1")
	c.SetRequest(req.WithContext(ctx))

	return e, c, rec
}

func TestHandler_GetSummary(t *testing.T) {
	_, c, rec := setupEcho(t)
	c.SetPath("/api/usage/summary")

	svc := &mockService{
		summary: &UsageSummary{
			TenantID:     "tenant-1",
			From:         time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			To:           time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
			TotalCostUSD: 0.005,
			Models: []ModelUsageSummary{
				{Model: "qwen-max", InputTokens: 1000, OutputTokens: 500, ToolCalls: 3, CostUSD: 0.005},
			},
		},
	}

	h := NewHandler(svc)
	err := h.GetSummary(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "tenant-1", body["tenant_id"])
	assert.InDelta(t, 0.005, body["total_cost_usd"], 0.0001)

	models := body["models"].([]interface{})
	require.Len(t, models, 1)
}

func TestHandler_GetSummary_InvalidDateRange(t *testing.T) {
	_, c, _ := setupEcho(t)
	c.SetPath("/api/usage/summary")

	svc := &mockService{summaryErr: ErrInvalidDateRange}
	h := NewHandler(svc)

	err := h.GetSummary(c)
	require.Error(t, err)

	he, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, he.Code)
}

func TestHandler_GetCurrent(t *testing.T) {
	_, c, rec := setupEcho(t)
	c.SetPath("/api/usage/current")

	svc := &mockService{
		current: &CurrentUsage{
			TenantID:              "tenant-1",
			PeriodStart:           time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:             time.Date(2026, 7, 31, 23, 59, 59, 0, time.UTC),
			TotalInputTokens:      1000,
			TotalOutputTokens:     500,
			TotalToolCalls:        3,
			TotalEmbeddingTokens:  100,
			TotalCostUSD:          0.00501,
			ReportsGenerated:      1,
			ByModel: []ModelUsage{
				{Model: "qwen-max", InputTokens: 1000, OutputTokens: 500, ToolCalls: 3, CostUSD: 0.005},
			},
		},
	}

	h := NewHandler(svc)
	err := h.GetCurrent(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "tenant-1", body["tenant_id"])
	assert.Equal(t, float64(1000), body["total_input_tokens"])
	assert.Equal(t, float64(1), body["reports_generated"])
}

func TestHandler_ListEvents(t *testing.T) {
	_, c, rec := setupEcho(t)
	c.SetPath("/api/usage/events")

	payload := json.RawMessage(`{"model":"qwen-max"}`)
	svc := &mockService{
		events: &UsageEventPage{
			Events: []UsageEventRecord{
				{TenantID: "tenant-1", EventType: "llm_usage", Payload: payload},
			},
			TotalCount: 1,
			Page:       1,
			PageSize:   20,
		},
	}

	h := NewHandler(svc)
	err := h.ListEvents(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["total_count"])

	events := body["events"].([]interface{})
	require.Len(t, events, 1)
}

func TestHandler_ListEvents_InvalidTenantID(t *testing.T) {
	_, c, _ := setupEcho(t)
	c.SetPath("/api/usage/events")

	svc := &mockService{eventsErr: ErrInvalidTenantID}
	h := NewHandler(svc)

	err := h.ListEvents(c)
	require.Error(t, err)

	he, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, he.Code)
}

func TestHandler_InternalError(t *testing.T) {
	_, c, _ := setupEcho(t)
	c.SetPath("/api/usage/current")

	svc := &mockService{currentErr: assert.AnError}
	h := NewHandler(svc)

	err := h.GetCurrent(c)
	require.Error(t, err)

	he, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, he.Code)
}