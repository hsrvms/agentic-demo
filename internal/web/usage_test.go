package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const usageTenant = "t_usage"

func newUsageContext(t *testing.T, path string, htmx bool) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	ctx := auth.SetTenantID(req.Context(), domain.TenantID(usageTenant))
	ctx = SetTenantName(ctx, "Acme Corp")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	e := echo.New()
	return e.NewContext(req, rec), rec
}

// --- Page ---

func TestUsageHandler_Page_RendersStatsAndEvents(t *testing.T) {
	now := time.Now().UTC()
	svc := &mockUsageService{
		currentUsage: &usage.CurrentUsage{
			TenantID:             usageTenant,
			PeriodStart:          time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:            time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC),
			TotalCostUSD:         12.5,
			TotalInputTokens:     1_000_000,
			TotalOutputTokens:    500_000,
			TotalToolCalls:       42,
			TotalEmbeddingTokens: 2_000_000,
			ReportsGenerated:     3,
			ByModel: []usage.ModelUsage{
				{Model: "qwen-turbo", InputTokens: 1_000_000, OutputTokens: 500_000, ToolCalls: 42, EmbeddingTokens: 2_000_000, CostUSD: 12.5},
			},
		},
		events: &usage.UsageEventPage{
			Events: []usage.UsageEventRecord{
				{ID: uuid.New(), TenantID: usageTenant, EventType: string(usage.EventLLM), Payload: json.RawMessage(`{"Type":"llm_usage","LLM":{"Model":"qwen-turbo","InputTokens":100,"OutputTokens":50}}`), CreatedAt: now},
			},
			TotalCount: 21,
			Page:       1,
			PageSize:   20,
		},
	}
	handler := NewUsageHandler(svc)

	c, rec := newUsageContext(t, "/usage", false)
	err := handler.Page(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// Page shell.
	assert.Contains(t, body, "<html")
	assert.Contains(t, body, "Usage")
	assert.Contains(t, body, "Acme Corp")
	assert.Contains(t, body, "August 2026")
	// Stat cards.
	assert.Contains(t, body, "Total Cost MTD")
	assert.Contains(t, body, "Tool Calls")
	assert.Contains(t, body, "Reports Generated")
	assert.Contains(t, body, "$12.50")
	// Per-model breakdown.
	assert.Contains(t, body, "qwen-turbo")
	// Date-range form submits via HTMX to the summary endpoint.
	assert.Contains(t, body, `hx-get="/usage/summary"`)
	assert.Contains(t, body, `name="from"`)
	assert.Contains(t, body, `name="to"`)
	// Event log with infinite-scroll sentinel.
	assert.Contains(t, body, "Event Log")
	assert.Contains(t, body, `/usage/events?page=2`)
	assert.Contains(t, body, `hx-trigger="revealed"`)
}

func TestUsageHandler_Page_EmptyState(t *testing.T) {
	svc := &mockUsageService{
		currentUsage: &usage.CurrentUsage{TenantID: usageTenant, ByModel: []usage.ModelUsage{}},
		events:       &usage.UsageEventPage{Events: []usage.UsageEventRecord{}, TotalCount: 0, Page: 1, PageSize: 20},
	}
	handler := NewUsageHandler(svc)

	c, rec := newUsageContext(t, "/usage", false)
	err := handler.Page(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "No usage recorded for the current month yet.")
	assert.Contains(t, body, "No usage events recorded yet.")
	assert.NotContains(t, body, "hx-trigger=\"revealed\"")
}

func TestUsageHandler_Page_CurrentUsageError(t *testing.T) {
	handler := NewUsageHandler(&mockUsageService{err: assert.AnError})
	c, _ := newUsageContext(t, "/usage", false)
	err := handler.Page(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}

func TestUsageHandler_Page_EventsError(t *testing.T) {
	handler := NewUsageHandler(&mockUsageService{
		currentUsage: &usage.CurrentUsage{TenantID: usageTenant, ByModel: []usage.ModelUsage{}},
		eventsErr:    assert.AnError,
	})
	c, _ := newUsageContext(t, "/usage", false)
	err := handler.Page(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}

// --- Summary ---

func TestUsageHandler_Summary_RendersFragment(t *testing.T) {
	svc := &mockUsageService{
		summary: &usage.UsageSummary{
			TenantID:     usageTenant,
			From:         time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			To:           time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
			TotalCostUSD: 8.0,
			Models: []usage.ModelUsageSummary{
				{Model: "qwen-max", InputTokens: 2000, OutputTokens: 1000, ToolCalls: 5, EmbeddingTokens: 0, CostUSD: 8.0},
			},
		},
	}
	handler := NewUsageHandler(svc)

	c, rec := newUsageContext(t, "/usage/summary?from=2026-07-01&to=2026-07-31", true)
	err := handler.Summary(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, "2026-07-01", svc.lastFrom)
	assert.Equal(t, "2026-07-31", svc.lastTo)

	body := rec.Body.String()
	// Fragment only, no page shell.
	assert.NotContains(t, body, "<html")
	assert.Contains(t, body, "qwen-max")
	assert.Contains(t, body, "$8.00")
	assert.Contains(t, body, "Jul 01, 2026")
	assert.Contains(t, body, "Jul 31, 2026")
}

func TestUsageHandler_Summary_Empty(t *testing.T) {
	svc := &mockUsageService{summary: &usage.UsageSummary{Models: []usage.ModelUsageSummary{}}}
	handler := NewUsageHandler(svc)

	c, rec := newUsageContext(t, "/usage/summary", true)
	err := handler.Summary(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "No usage recorded for the selected range.")
}

func TestUsageHandler_Summary_InvalidDateRange(t *testing.T) {
	handler := NewUsageHandler(&mockUsageService{summaryErr: usage.ErrInvalidDateRange})
	c, _ := newUsageContext(t, "/usage/summary?from=bad", true)
	err := handler.Summary(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

// --- Events ---

func TestUsageHandler_Events_RendersRows(t *testing.T) {
	now := time.Now().UTC()
	svc := &mockUsageService{
		events: &usage.UsageEventPage{
			Events: []usage.UsageEventRecord{
				{ID: uuid.New(), TenantID: usageTenant, EventType: string(usage.EventTool), Payload: json.RawMessage(`{"Type":"tool_usage","Tool":{"ToolName":"web_search","Success":true}}`), CreatedAt: now},
				{ID: uuid.New(), TenantID: usageTenant, EventType: string(usage.EventLLM), Payload: json.RawMessage(`{"Type":"llm_usage","LLM":{"Model":"qwen","InputTokens":10,"OutputTokens":5}}`), CreatedAt: now},
			},
			TotalCount: 25,
			Page:       2,
			PageSize:   20,
		},
	}
	handler := NewUsageHandler(svc)

	c, rec := newUsageContext(t, "/usage/events?page=2", true)
	err := handler.Events(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 2, svc.lastPage)

	body := rec.Body.String()
	// Rows only, no page shell.
	assert.NotContains(t, body, "<html")
	assert.Contains(t, body, "web_search · succeeded")
	assert.Contains(t, body, "qwen · 15 tokens")
}

func TestUsageHandler_Events_DefaultsPageOne(t *testing.T) {
	svc := &mockUsageService{events: &usage.UsageEventPage{Events: []usage.UsageEventRecord{}, TotalCount: 0, Page: 1, PageSize: 20}}
	handler := NewUsageHandler(svc)

	c, _ := newUsageContext(t, "/usage/events", true)
	err := handler.Events(c)
	require.NoError(t, err)
	assert.Equal(t, 1, svc.lastPage)
}

func TestUsageHandler_Events_Error(t *testing.T) {
	handler := NewUsageHandler(&mockUsageService{eventsErr: assert.AnError})
	c, _ := newUsageContext(t, "/usage/events", true)
	err := handler.Events(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}
