package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- List tests ---

func TestReportsHandler_List_RendersTable(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	generated := time.Now().Add(-24 * time.Hour)

	svc := &mockReportService{
		page: reports.ReportPage{
			Reports: []reports.StoredReport{
				{ID: uuid.New(), Title: "Daily Brief", Type: "daily", Focus: "Signals", GeneratedAt: generated},
				{ID: uuid.New(), Title: "Weekly Review", Type: "weekly", GeneratedAt: generated},
			},
			TotalCount: 2,
			Page:       1,
			PageSize:   20,
		},
	}

	handler := NewReportsHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Daily Brief")
	assert.Contains(t, body, "Weekly Review")
	assert.Contains(t, body, "/reports/")
	assert.Contains(t, body, "Daily")
	assert.Contains(t, body, "Weekly")
	assert.Contains(t, body, "Signals")
	// Type badges use the correct intent colors.
	assert.Contains(t, body, "bg-intent-info/10 text-intent-info")
	assert.Contains(t, body, "bg-primary-subtle text-primary")
}

func TestReportsHandler_List_EmptyState(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockReportService{
		page: reports.ReportPage{
			Reports:    []reports.StoredReport{},
			TotalCount: 0,
			Page:       1,
			PageSize:   20,
		},
	}

	handler := NewReportsHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "No reports yet. Generate one to get started.")
}

func TestReportsHandler_List_HTMXPaginationFragment(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockReportService{
		page: reports.ReportPage{
			Reports: []reports.StoredReport{
				{ID: uuid.New(), Title: "Page 2 Report", Type: "monthly", GeneratedAt: time.Now()},
			},
			TotalCount: 25,
			Page:       2,
			PageSize:   20,
		},
	}

	handler := NewReportsHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports?page=2", http.NoBody)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Page 2 Report")
	// Fragment must not render the full page shell.
	assert.Contains(t, body, "<tr")
	assert.NotContains(t, body, "<html")
}

func TestReportsHandler_List_ServiceError(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockReportService{err: assert.AnError}
	handler := NewReportsHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}

// --- Detail tests ---

func TestReportsHandler_Detail_RendersContentAndCitations(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	id := uuid.New()

	svc := &mockReportService{
		report: reports.StoredReport{
			ID:          id,
			TenantID:    string(tenantID),
			Title:       "Monthly Review",
			Type:        "monthly",
			Content:     "# Revenue\n\nRevenue is **up 12%**.",
			Focus:       "Growth",
			Citations:   json.RawMessage(`[{"title":"Q3 Report","url":"https://example.com/q3","source":"Internal"}]`),
			GeneratedAt: time.Now(),
		},
	}

	handler := NewReportsHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/"+id.String(), http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Detail(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Monthly Review")
	assert.Contains(t, body, "<h1>Revenue</h1>")
	assert.Contains(t, body, "<strong>up 12%</strong>")
	assert.Contains(t, body, "Q3 Report")
	assert.Contains(t, body, "https://example.com/q3")
	assert.Contains(t, body, "Back to Reports")
}

func TestReportsHandler_Detail_InvalidID(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	handler := NewReportsHandler(&mockReportService{})

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/bad", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("bad")

	err := handler.Detail(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestReportsHandler_Detail_NotFound(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	id := uuid.New()

	svc := &mockReportService{detailErr: reports.ErrReportNotFound}
	handler := NewReportsHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/"+id.String(), http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Detail(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

func TestReportsHandler_Detail_CrossTenantDenied(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	otherTenant := domain.TenantID("t_other")
	id := uuid.New()

	// Report belongs to a different tenant.
	svc := &mockReportService{
		report: reports.StoredReport{
			ID:       id,
			TenantID: string(otherTenant),
			Title:    "Secret Report",
			Type:     "daily",
		},
	}
	handler := NewReportsHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/"+id.String(), http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Detail(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
	assert.NotContains(t, rec.Body.String(), "Secret Report")
}