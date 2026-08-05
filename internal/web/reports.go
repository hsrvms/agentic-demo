package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const reportsPageSize = 20

// ReportsHandler serves the report browsing pages.
type ReportsHandler struct {
	service reports.ReportService
}

// NewReportsHandler creates a ReportsHandler.
func NewReportsHandler(service reports.ReportService) *ReportsHandler {
	return &ReportsHandler{service: service}
}

// Register mounts report routes on the authenticated web group.
func (h *ReportsHandler) Register(g *echo.Group) {
	g.GET("/reports", h.List)
	g.GET("/reports/:id", h.Detail)
}

// List handles GET /reports.
func (h *ReportsHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	result, err := h.service.ListByTenant(ctx, tenantID, page, reportsPageSize)
	if err != nil {
		log.Printf("reports list error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load reports")
	}

	data := webui.MapReportList(result)
	data.HasMore = (page * reportsPageSize) < result.TotalCount
	data.NextPage = page + 1

	if IsHTMX(c) {
		return Render(c, http.StatusOK, webpages.ReportRowsFragment(data.Reports))
	}

	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.ReportsList(data, flashes))
}

// Detail handles GET /reports/:id.
func (h *ReportsHandler) Detail(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid report ID")
	}

	r, err := h.service.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, reports.ErrReportNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "report not found")
		}
		log.Printf("reports detail error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load report")
	}

	// Enforce tenant isolation: a report belongs to exactly one tenant.
	if r.TenantID != tenantID {
		return echo.NewHTTPError(http.StatusNotFound, "report not found")
	}

	data := webui.MapReportDetail(&r)
	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.ReportDetail(data, flashes))
}