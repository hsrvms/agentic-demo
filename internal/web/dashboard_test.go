package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/agentic-demo/platform/internal/webui"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock services ---

type mockUsageService struct {
	currentUsage *usage.CurrentUsage
	err          error
}

func (m *mockUsageService) GetSummary(_ context.Context, _, _, _ string) (*usage.UsageSummary, error) {
	return nil, nil
}

func (m *mockUsageService) GetCurrentUsage(_ context.Context, _ string) (*usage.CurrentUsage, error) {
	return m.currentUsage, m.err
}

func (m *mockUsageService) ListEvents(_ context.Context, _ string, _, _ int) (*usage.UsageEventPage, error) {
	return nil, nil
}

type mockReportService struct {
	page reports.ReportPage
	err  error
}

func (m *mockReportService) Create(_ context.Context, _ *reports.CreateReportParams) (reports.StoredReport, error) {
	return reports.StoredReport{}, nil
}

func (m *mockReportService) GetByID(_ context.Context, _ uuid.UUID) (reports.StoredReport, error) {
	return reports.StoredReport{}, nil
}

func (m *mockReportService) ListByTenant(_ context.Context, _ string, _, _ int) (reports.ReportPage, error) {
	return m.page, m.err
}

func (m *mockReportService) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

type mockSourceService struct {
	page         sources.DataSourcePage
	err          error
	detailSource *sources.DataSource
	detailErr    error
	createResult sources.DataSource
	createErr    error
	updateResult sources.DataSource
	updateErr    error
	deleteErr    error
	testResult   sources.ConnectionTestResult
	testErr      error
	syncErr      error
}

func (m *mockSourceService) Create(_ context.Context, _ *sources.CreateDataSourceParams) (sources.DataSource, error) {
	return m.createResult, m.createErr
}

func (m *mockSourceService) GetByID(_ context.Context, _ uuid.UUID) (sources.DataSource, error) {
	if m.detailSource != nil {
		return *m.detailSource, m.detailErr
	}
	return sources.DataSource{}, m.detailErr
}

func (m *mockSourceService) ListByTenant(_ context.Context, _ string, _, _ int) (sources.DataSourcePage, error) {
	return m.page, m.err
}

func (m *mockSourceService) Update(_ context.Context, _ uuid.UUID, _ sources.UpdateDataSourceParams) (sources.DataSource, error) {
	return m.updateResult, m.updateErr
}

func (m *mockSourceService) Delete(_ context.Context, _ uuid.UUID) error {
	return m.deleteErr
}

func (m *mockSourceService) TestConnection(_ context.Context, _ uuid.UUID) (sources.ConnectionTestResult, error) {
	return m.testResult, m.testErr
}

func (m *mockSourceService) Sync(_ context.Context, _ uuid.UUID) error {
	return m.syncErr
}

type mockBudgetService struct {
	status *budget.BudgetStatus
	err    error
}

func (m *mockBudgetService) GetBudgetStatus(_ context.Context, _ domain.TenantID) (*budget.BudgetStatus, error) {
	return m.status, m.err
}

func (m *mockBudgetService) SetMonthlyBudget(_ context.Context, _ domain.TenantID, _ float64) error {
	return nil
}

func (m *mockBudgetService) ListInvoices(_ context.Context, _ domain.TenantID, _, _ int) (budget.InvoicePage, error) {
	return budget.InvoicePage{}, nil
}

func (m *mockBudgetService) GetInvoice(_ context.Context, _ domain.TenantID, _ uuid.UUID) (budget.Invoice, error) {
	return budget.Invoice{}, nil
}

func (m *mockBudgetService) GenerateInvoice(_ context.Context, _ domain.TenantID, _, _ time.Time) (budget.Invoice, error) {
	return budget.Invoice{}, nil
}

// Compile-time interface checks.
var _ usage.UsageService = (*mockUsageService)(nil)
var _ reports.ReportService = (*mockReportService)(nil)
var _ sources.Service = (*mockSourceService)(nil)
var _ budget.BudgetService = (*mockBudgetService)(nil)

// --- BudgetIntent tests ---

func TestBudgetIntent(t *testing.T) {
	tests := []struct {
		name    string
		percent float64
		want    string
	}{
		{"zero", 0, "success"},
		{"under_80", 50, "success"},
		{"exactly_79", 79.9, "success"},
		{"exactly_80", 80, "warning"},
		{"between_80_95", 87, "warning"},
		{"exactly_95", 95, "warning"},
		{"over_95", 95.1, "error"},
		{"exactly_100", 100, "error"},
		{"over_100", 120, "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BudgetIntent(tt.percent)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- formatTokens tests ---

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{"zero", 0, "0"},
		{"hundreds", 500, "500"},
		{"thousands", 1500, "1.5K"},
		{"millions", 2_500_000, "2.5M"},
		{"large", 15_750_000, "15.8M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTokens(tt.n)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- countActiveSources tests ---

func TestCountActiveSources(t *testing.T) {
	srcs := []sources.DataSource{
		{Status: sources.StatusActive},
		{Status: sources.StatusInactive},
		{Status: sources.StatusActive},
		{Status: sources.StatusError},
	}
	assert.Equal(t, 2, countActiveSources(srcs))
	assert.Equal(t, 0, countActiveSources(nil))
	assert.Equal(t, 0, countActiveSources([]sources.DataSource{}))
}

// --- Dashboard handler integration test ---

func TestDashboardHandler_AssemblesData(t *testing.T) {
	tenantID := domain.TenantID("t_test123")
	userID := uuid.New()

	usg := &mockUsageService{
		currentUsage: &usage.CurrentUsage{
			TenantID:          string(tenantID),
			TotalCostUSD:      12.34,
			TotalInputTokens:  5000,
			TotalOutputTokens: 3000,
		},
	}
	rpt := &mockReportService{
		page: reports.ReportPage{TotalCount: 7},
	}
	src := &mockSourceService{
		page: sources.DataSourcePage{
			Sources: []sources.DataSource{
				{Status: sources.StatusActive},
				{Status: sources.StatusActive},
				{Status: sources.StatusInactive},
			},
		},
	}
	bgt := &mockBudgetService{
		status: &budget.BudgetStatus{
			PercentUsed: 62,
			IsExceeded:  false,
		},
	}

	handler := NewDashboardHandler(usg, rpt, src, bgt)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = auth.SetUserID(ctx, userID)
	ctx = SetTenantName(ctx, "Test Corp")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.dashboardPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "$12.34")
	assert.Contains(t, body, "8.0K tokens used")
	assert.Contains(t, body, "7")  // reports count
	assert.Contains(t, body, "2")  // active sources count
	assert.Contains(t, body, "62%")
}

func TestDashboardHandler_ServiceErrorsDegradedGracefully(t *testing.T) {
	tenantID := domain.TenantID("t_test123")
	userID := uuid.New()

	usg := &mockUsageService{err: assert.AnError}
	rpt := &mockReportService{err: assert.AnError}
	src := &mockSourceService{err: assert.AnError}
	bgt := &mockBudgetService{err: assert.AnError}

	handler := NewDashboardHandler(usg, rpt, src, bgt)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = auth.SetUserID(ctx, userID)
	ctx = SetTenantName(ctx, "Test Corp")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.dashboardPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "$0.00") // fallback cost
	assert.Contains(t, body, "0")     // fallback counts
}

func TestDashboardHandler_BudgetExceeded(t *testing.T) {
	tenantID := domain.TenantID("t_test123")
	userID := uuid.New()

	usg := &mockUsageService{currentUsage: &usage.CurrentUsage{}}
	rpt := &mockReportService{page: reports.ReportPage{}}
	src := &mockSourceService{page: sources.DataSourcePage{}}
	bgt := &mockBudgetService{
		status: &budget.BudgetStatus{
			PercentUsed: 110,
			IsExceeded:  true,
		},
	}

	handler := NewDashboardHandler(usg, rpt, src, bgt)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = auth.SetUserID(ctx, userID)
	ctx = SetTenantName(ctx, "Test Corp")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.dashboardPage(c)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "110%")
	assert.Contains(t, body, "Budget exceeded")
}

func TestDashboardHandler_FlashesPassedToTemplate(t *testing.T) {
	tenantID := domain.TenantID("t_test123")
	userID := uuid.New()

	usg := &mockUsageService{currentUsage: &usage.CurrentUsage{}}
	rpt := &mockReportService{page: reports.ReportPage{}}
	src := &mockSourceService{page: sources.DataSourcePage{}}
	bgt := &mockBudgetService{status: &budget.BudgetStatus{}}

	handler := NewDashboardHandler(usg, rpt, src, bgt)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = auth.SetUserID(ctx, userID)
	ctx = SetTenantName(ctx, "Test Corp")
	ctx = SetFlashMessages(ctx, []webui.Flash{{Intent: "success", Message: "Welcome!"}})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.dashboardPage(c)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "Welcome!")
}


