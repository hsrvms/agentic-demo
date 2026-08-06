package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const invoicesPageSize = 20

// InvoicesHandler serves the invoice browsing pages: a paginated list with
// HTMX infinite scroll and a detail view with per-model line items.
type InvoicesHandler struct {
	service budget.BudgetService
}

// NewInvoicesHandler creates an InvoicesHandler.
func NewInvoicesHandler(service budget.BudgetService) *InvoicesHandler {
	return &InvoicesHandler{service: service}
}

// Register mounts invoice routes on the authenticated web group.
func (h *InvoicesHandler) Register(g *echo.Group) {
	g.GET("/invoices", h.List)
	g.GET("/invoices/:id", h.Detail)
}

// List handles GET /invoices. It renders the full paginated invoice table, or
// just the next page of rows for HTMX infinite-scroll requests.
func (h *InvoicesHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := auth.GetTenantID(ctx)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	result, err := h.service.ListInvoices(ctx, tenantID, page, invoicesPageSize)
	if err != nil {
		log.Printf("invoices list error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load invoices")
	}

	data := webui.MapInvoiceList(result)
	data.HasMore = (page * invoicesPageSize) < result.TotalCount
	data.NextPage = page + 1

	if IsHTMX(c) {
		return Render(c, http.StatusOK, webpages.InvoiceRowsFragment(data.Invoices))
	}

	data.TenantName = GetTenantName(ctx)
	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.InvoicesList(data, flashes))
}

// Detail handles GET /invoices/:id. It renders the invoice period, status,
// total cost, and the per-model line-item breakdown. Tenant ownership is
// enforced by the service, which returns ErrNotFound for foreign invoices.
func (h *InvoicesHandler) Detail(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := auth.GetTenantID(ctx)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid invoice ID")
	}

	inv, err := h.service.GetInvoice(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, budget.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "invoice not found")
		}
		log.Printf("invoices detail error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load invoice")
	}

	data := webui.MapInvoiceDetail(&inv)
	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.InvoiceDetail(data, flashes))
}
