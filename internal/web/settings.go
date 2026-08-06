package web

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/labstack/echo/v4"
)

// SettingsHandler serves the tenant settings page: read-only tenant info plus,
// for admins only, the monthly budget configuration form.
type SettingsHandler struct {
	tenantService tenant.TenantService
	budgetService budget.BudgetService
}

// NewSettingsHandler creates a SettingsHandler.
func NewSettingsHandler(tenantService tenant.TenantService, budgetService budget.BudgetService) *SettingsHandler {
	return &SettingsHandler{
		tenantService: tenantService,
		budgetService: budgetService,
	}
}

// Register mounts settings routes on the authenticated web group.
func (h *SettingsHandler) Register(g *echo.Group) {
	g.GET("/settings", h.Page)
	g.POST("/settings/budget", h.BudgetSubmit)
}

// Page handles GET /settings. It renders the tenant's identity read-only and
// the live budget status; the budget form is only shown to admins.
func (h *SettingsHandler) Page(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := auth.GetTenantID(ctx)
	userID := auth.GetUserID(ctx)

	t, err := h.tenantService.GetByID(ctx, tenantID)
	if err != nil {
		log.Printf("settings tenant error: %v", err)
		return echo.NewHTTPError(http.StatusNotFound, "tenant not found")
	}

	// Budget status degrades gracefully (like the dashboard): if it cannot be
	// loaded the page still renders tenant info and the (empty) status cards.
	status, err := h.budgetService.GetBudgetStatus(ctx, tenantID)
	if err != nil {
		log.Printf("settings budget error: %v", err)
	}

	isAdmin, err := h.tenantService.IsAdmin(ctx, tenantID, userID)
	if err != nil {
		log.Printf("settings is-admin error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load settings")
	}

	data := webui.MapSettings(&t, status)
	data.IsAdmin = isAdmin
	data.CSRFToken = GetCSRFToken(ctx)

	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.Settings(data, flashes))
}

// BudgetSubmit handles POST /settings/budget. Only admins may change the
// monthly cap. On success it flashes "Budget updated" and returns to the
// settings page (via HX-Redirect for HTMX requests).
func (h *SettingsHandler) BudgetSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := auth.GetTenantID(ctx)
	userID := auth.GetUserID(ctx)

	isAdmin, err := h.tenantService.IsAdmin(ctx, tenantID, userID)
	if err != nil {
		log.Printf("settings is-admin error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save budget")
	}
	if !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "only admins can change the monthly budget")
	}

	raw := strings.TrimSpace(c.FormValue(budgetFieldName))
	budgetVal, err := strconv.ParseFloat(raw, 64)
	if err != nil || budgetVal < 0 {
		setFlashCookie(c, webui.Flash{Intent: "error", Message: "Enter a valid monthly budget"})
		return h.budgetRedirect(c)
	}

	if err := h.budgetService.SetMonthlyBudget(ctx, tenantID, budgetVal); err != nil {
		log.Printf("settings set budget error: %v", err)
		setFlashCookie(c, webui.Flash{Intent: "error", Message: "Failed to update budget"})
		return h.budgetRedirect(c)
	}

	setFlashCookie(c, webui.Flash{Intent: "success", Message: "Budget updated"})
	return h.budgetRedirect(c)
}

// budgetRedirect returns the caller to /settings, using HX-Redirect for HTMX
// requests so the browser navigates to the freshly re-rendered page.
func (h *SettingsHandler) budgetRedirect(c echo.Context) error {
	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/settings")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/settings")
}

// budgetFieldName is the form field carrying the monthly budget cap in USD.
const budgetFieldName = "monthly_budget_usd"
