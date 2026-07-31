package web

import (
	"fmt"
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

	data := webui.DashboardData{
		TenantName:      tenantName,
		CostFormatted:   "$0.00",
		TokensFormatted: "0",
		BudgetIntent:    "success",
	}

	// Fetch usage data.
	currentUsage, err := h.usageService.GetCurrentUsage(ctx, tenantID)
	if err != nil {
		log.Printf("dashboard: usage error: %v", err)
	} else if currentUsage != nil {
		data.TotalCostUSD = currentUsage.TotalCostUSD
		data.TotalTokens = currentUsage.TotalInputTokens + currentUsage.TotalOutputTokens
		data.CostFormatted = fmt.Sprintf("$%.2f", currentUsage.TotalCostUSD)
		data.TokensFormatted = formatTokens(data.TotalTokens)
	}

	// Fetch reports count.
	reportPage, err := h.reportService.ListByTenant(ctx, tenantID, 1, 1)
	if err != nil {
		log.Printf("dashboard: reports error: %v", err)
	} else {
		data.ReportsCount = reportPage.TotalCount
	}

	// Fetch active sources count.
	sourcePage, err := h.sourceService.ListByTenant(ctx, tenantID, 1, 100)
	if err != nil {
		log.Printf("dashboard: sources error: %v", err)
	} else {
		data.ActiveSources = countActiveSources(sourcePage.Sources)
	}

	// Fetch budget status.
	budgetStatus, err := h.budgetService.GetBudgetStatus(ctx, domain.TenantID(tenantID))
	if err != nil {
		log.Printf("dashboard: budget error: %v", err)
	} else if budgetStatus != nil {
		data.BudgetPercent = budgetStatus.PercentUsed
		data.BudgetIntent = BudgetIntent(budgetStatus.PercentUsed)
		data.BudgetExceeded = budgetStatus.IsExceeded
	}

	flashes := GetFlashMessages(c.Request().Context())
	return Render(c, http.StatusOK, webpages.Dashboard(data, flashes))
}

// BudgetIntent returns the intent CSS class based on budget usage percentage.
//
//	green  (< 80%)  → "success"
//	yellow (80–95%) → "warning"
//	red    (> 95%)  → "error"
func BudgetIntent(percent float64) string {
	switch {
	case percent > 95:
		return "error"
	case percent >= 80:
		return "warning"
	default:
		return "success"
	}
}

// formatTokens formats a token count for human-readable display.
func formatTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// countActiveSources counts sources with Status == "active".
func countActiveSources(srcs []sources.DataSource) int {
	n := 0
	for i := range srcs {
		if srcs[i].Status == sources.StatusActive {
			n++
		}
	}
	return n
}


