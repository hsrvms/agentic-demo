package web

import (
	"log"
	"net/http"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/labstack/echo/v4"
)

// DashboardHandler serves the dashboard page.
type DashboardHandler struct {
	usageService  usage.UsageService
	reportService reports.ReportService
	sourceService sources.Service
	budgetService budget.BudgetService
}

// NewDashboardHandler creates a DashboardHandler.
func NewDashboardHandler(
	usageService usage.UsageService,
	reportService reports.ReportService,
	sourceService sources.Service,
	budgetService budget.BudgetService,
) *DashboardHandler {
	return &DashboardHandler{
		usageService:  usageService,
		reportService: reportService,
		sourceService: sourceService,
		budgetService: budgetService,
	}
}

// dashboardPage serves GET /dashboard.
func (h *DashboardHandler) dashboardPage(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))
	tenantName := GetTenantName(ctx)

	// Fetch usage data.
	currentUsage, err := h.usageService.GetCurrentUsage(ctx, tenantID)
	if err != nil {
		log.Printf("dashboard: usage error: %v", err)
	}

	// Fetch reports count.
	var reportsCount int
	reportPage, err := h.reportService.ListByTenant(ctx, tenantID, 1, 1)
	if err != nil {
		log.Printf("dashboard: reports error: %v", err)
	} else {
		reportsCount = reportPage.TotalCount
	}

	// Fetch active sources count.
	var activeSources int
	sourcePage, err := h.sourceService.ListByTenant(ctx, tenantID, 1, 100)
	if err != nil {
		log.Printf("dashboard: sources error: %v", err)
	} else {
		activeSources = webui.CountActiveSources(sourcePage.Sources)
	}

	// Fetch budget status.
	budgetStatus, err := h.budgetService.GetBudgetStatus(ctx, domain.TenantID(tenantID))
	if err != nil {
		log.Printf("dashboard: budget error: %v", err)
	}

	data := webui.MapDashboard(currentUsage, reportsCount, activeSources, budgetStatus)
	data.TenantName = tenantName

	flashes := GetFlashMessages(c.Request().Context())
	return Render(c, http.StatusOK, webpages.Dashboard(data, flashes))
}
