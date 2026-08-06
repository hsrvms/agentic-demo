package budget

import (
	"net/http"
	"strconv"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/httperr"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler holds the dependencies for budget and invoice HTTP endpoints.
type Handler struct {
	service BudgetService
}

// NewHandler creates a budget Handler.
func NewHandler(service BudgetService) *Handler {
	return &Handler{service: service}
}

// Register wires budget and invoice routes under the given Echo group with the provided middleware.
func (h *Handler) Register(g *echo.Group, mw ...echo.MiddlewareFunc) {
	g.GET("/budget/status", h.GetBudgetStatus, mw...)
	g.GET("/invoices", h.ListInvoices, mw...)
	g.GET("/invoices/:id", h.GetInvoice, mw...)
}

// GetBudgetStatus returns the current budget status for the tenant.
func (h *Handler) GetBudgetStatus(c echo.Context) error {
	tenantID := getTenantID(c)

	status, err := h.service.GetBudgetStatus(c.Request().Context(), domain.TenantID(tenantID))
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	return c.JSON(http.StatusOK, status)
}

// ListInvoices returns paginated invoices for the tenant.
func (h *Handler) ListInvoices(c echo.Context) error {
	tenantID := getTenantID(c)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("limit"))

	result, err := h.service.ListInvoices(c.Request().Context(), domain.TenantID(tenantID), page, pageSize)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	items := make([]map[string]interface{}, len(result.Invoices))
	for i := range result.Invoices {
		items[i] = invoiceToResponse(&result.Invoices[i])
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"invoices":    items,
		"total_count": result.TotalCount,
		"page":        result.Page,
		"page_size":   result.PageSize,
	})
}

// GetInvoice returns a single invoice by ID.
func (h *Handler) GetInvoice(c echo.Context) error {
	tenantID := getTenantID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid invoice ID")
	}

	inv, err := h.service.GetInvoice(c.Request().Context(), domain.TenantID(tenantID), id)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	return c.JSON(http.StatusOK, invoiceToResponse(&inv))
}

// --- helpers ---

func getTenantID(c echo.Context) string {
	return string(auth.GetTenantID(c.Request().Context()))
}

func invoiceToResponse(inv *Invoice) map[string]interface{} {
	lineItems := make([]map[string]interface{}, len(inv.LineItems))
	for i, li := range inv.LineItems {
		lineItems[i] = map[string]interface{}{
			"model":             li.Model,
			"input_tokens":      li.InputTokens,
			"output_tokens":     li.OutputTokens,
			"tool_calls":        li.ToolCalls,
			"embedding_tokens":  li.EmbeddingTokens,
			"reports_generated": li.ReportsGenerated,
			"cost_usd":          li.CostUSD,
		}
	}

	return map[string]interface{}{
		"id":             inv.ID.String(),
		"tenant_id":      inv.TenantID,
		"period_start":   inv.PeriodStart.Format("2006-01-02"),
		"period_end":     inv.PeriodEnd.Format("2006-01-02"),
		"total_cost_usd": inv.TotalCostUSD,
		"line_items":     lineItems,
		"status":         string(inv.Status),
		"created_at":     inv.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
