package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const settingsTenant = "t_settings"

func newSettingsHandler(ts *mockTenantService, bs *mockBudgetService) *SettingsHandler {
	return NewSettingsHandler(ts, bs)
}

func newSettingsContext(t *testing.T, method, path string, htmx bool, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody)
	} else {
		req = httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	ctx := auth.SetTenantID(req.Context(), domain.TenantID(settingsTenant))
	ctx = SetTenantName(ctx, "Acme Corp")
	ctx = SetCSRFToken(ctx, "csrf-token-123")
	ctx = auth.SetUserID(ctx, uuid.MustParse("11111111-2222-3333-4444-555555555555"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	e := echo.New()
	return e.NewContext(req, rec), rec
}

func settingsTenantFixture() domain.Tenant {
	return domain.Tenant{
		ID:        domain.TenantID(settingsTenant),
		Name:      "Acme Corp",
		Status:    domain.TenantActive,
		CreatedAt: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
	}
}

func settingsBudgetFixture() *budget.BudgetStatus {
	return &budget.BudgetStatus{
		TenantID:        settingsTenant,
		MonthlyBudget:   250.0,
		MonthToDateCost: 90.0,
		RemainingBudget: 160.0,
		PercentUsed:     36.0,
		IsExceeded:      false,
	}
}

// --- GET /settings ---

func TestSettingsHandler_Page_RendersTenantInfoAndBudget(t *testing.T) {
	ts := newMockTenantService()
	ts.tenants[domain.TenantID(settingsTenant)] = settingsTenantFixture()
	bs := &mockBudgetService{status: settingsBudgetFixture()}
	handler := newSettingsHandler(ts, bs)

	c, rec := newSettingsContext(t, http.MethodGet, "/settings", false, "")
	err := handler.Page(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// Tenant info (read-only).
	assert.Contains(t, body, "Acme Corp")
	assert.Contains(t, body, settingsTenant)
	assert.Contains(t, body, "Active")
	assert.Contains(t, body, "Jan 15, 2026")
	// Budget status inline.
	assert.Contains(t, body, "Monthly Budget")
	assert.Contains(t, body, "$250.00")
	assert.Contains(t, body, "$90.00")
	assert.Contains(t, body, "$160.00")
	assert.Contains(t, body, "36%")
	// Budget form pre-filled + CSRF.
	assert.Contains(t, body, `hx-post="/settings/budget"`)
	assert.Contains(t, body, `name="_csrf"`)
	assert.Contains(t, body, `value="csrf-token-123"`)
	assert.Contains(t, body, `value="250.00"`)
}

func TestSettingsHandler_Page_BudgetErrorStillRenders(t *testing.T) {
	// When budget status cannot be loaded, the page still renders tenant info
	// and the (empty) budget cards, matching the dashboard's graceful degrade.
	ts := newMockTenantService()
	ts.tenants[domain.TenantID(settingsTenant)] = settingsTenantFixture()
	bs := &mockBudgetService{err: assert.AnError}
	handler := newSettingsHandler(ts, bs)

	c, rec := newSettingsContext(t, http.MethodGet, "/settings", false, "")
	err := handler.Page(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Acme Corp")
	assert.Contains(t, body, "Monthly Budget")
	// Form still rendered for the admin.
	assert.Contains(t, body, `hx-post="/settings/budget"`)
}

func TestSettingsHandler_Page_NonAdminSeesReadOnly(t *testing.T) {
	ts := newMockTenantService()
	ts.tenants[domain.TenantID(settingsTenant)] = settingsTenantFixture()
	ts.isAdmin = false
	bs := &mockBudgetService{status: settingsBudgetFixture()}
	handler := newSettingsHandler(ts, bs)

	c, rec := newSettingsContext(t, http.MethodGet, "/settings", false, "")
	err := handler.Page(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// Budget status still shown.
	assert.Contains(t, body, "Monthly Budget")
	// No editable form for non-admins.
	assert.NotContains(t, body, `hx-post="/settings/budget"`)
	assert.NotContains(t, body, `name="_csrf"`)
}

// --- POST /settings/budget ---

func TestSettingsHandler_BudgetSubmit_Success(t *testing.T) {
	ts := newMockTenantService()
	bs := &mockBudgetService{}
	handler := newSettingsHandler(ts, bs)

	c, rec := newSettingsContext(t, http.MethodPost, "/settings/budget", false, "monthly_budget_usd=500")
	err := handler.BudgetSubmit(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.True(t, bs.setBudgetCalled)
	assert.Equal(t, 500.0, bs.lastBudget)
	assert.Equal(t, "/settings", rec.Header().Get("Location"))
}

func TestSettingsHandler_BudgetSubmit_HTMXRedirect(t *testing.T) {
	ts := newMockTenantService()
	bs := &mockBudgetService{}
	handler := newSettingsHandler(ts, bs)

	c, rec := newSettingsContext(t, http.MethodPost, "/settings/budget", true, "monthly_budget_usd=500")
	err := handler.BudgetSubmit(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/settings", rec.Header().Get("HX-Redirect"))
	assert.True(t, bs.setBudgetCalled)
	assert.Equal(t, 500.0, bs.lastBudget)
}

func TestSettingsHandler_BudgetSubmit_InvalidBudget(t *testing.T) {
	ts := newMockTenantService()
	bs := &mockBudgetService{}
	handler := newSettingsHandler(ts, bs)

	c, rec := newSettingsContext(t, http.MethodPost, "/settings/budget", false, "monthly_budget_usd=-10")
	err := handler.BudgetSubmit(c)
	require.NoError(t, err)
	// Redirected back with an error flash, service never called.
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.False(t, bs.setBudgetCalled)
}

func TestSettingsHandler_BudgetSubmit_NonAdminForbidden(t *testing.T) {
	ts := newMockTenantService()
	ts.isAdmin = false
	bs := &mockBudgetService{}
	handler := newSettingsHandler(ts, bs)

	c, _ := newSettingsContext(t, http.MethodPost, "/settings/budget", false, "monthly_budget_usd=500")
	err := handler.BudgetSubmit(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, httpErr.Code)
	assert.False(t, bs.setBudgetCalled)
}
