package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/queue"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const reportsPageSize = 20

// ReportsHandler serves the report browsing pages and on-demand generation.
type ReportsHandler struct {
	service reports.ReportService
	queue   queue.JobQueue
}

// NewReportsHandler creates a ReportsHandler. The JobQueue is used to
// enqueue on-demand report generation jobs.
func NewReportsHandler(service reports.ReportService, q queue.JobQueue) *ReportsHandler {
	return &ReportsHandler{service: service, queue: q}
}

// Register mounts report routes on the authenticated web group.
func (h *ReportsHandler) Register(g *echo.Group) {
	g.GET("/reports", h.List)
	g.GET("/reports/:id", h.Detail)
	g.POST("/reports/generate", h.Generate)
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

	genForm := webui.GenerateReportForm{
		CSRFToken:   GetCSRFToken(ctx),
		TypeOptions: webui.ReportTypeOptions(),
	}
	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.ReportsList(data, genForm, flashes))
}

// Generate handles POST /reports/generate. It validates the form input and
// enqueues an on-demand report job on the queue; the job runs asynchronously
// in the worker. The handler returns immediately with a success confirmation.
func (h *ReportsHandler) Generate(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	reportType := strings.TrimSpace(c.FormValue("report_type"))
	if reportType == "" {
		reportType = "on_demand"
	}
	focus := strings.TrimSpace(c.FormValue("focus"))
	scheduleID := strings.TrimSpace(c.FormValue("schedule_id"))

	payload := &queue.ReportPayload{
		TenantID:       tenantID,
		ReportType:     reportType,
		DeliveryMethod: "web",
	}
	if focus != "" {
		payload.FocusAreas = []string{focus}
	}
	if scheduleID != "" {
		if _, err := uuid.Parse(scheduleID); err != nil {
			return h.generateError(c, "Schedule reference must be a valid schedule ID")
		}
		payload.ScheduleID = scheduleID
	}

	job, err := queue.NewReportJob(payload)
	if err != nil {
		return h.generateError(c, "Please select a valid report type")
	}

	if _, err := h.queue.Enqueue(ctx, job); err != nil {
		log.Printf("reports generate enqueue error: %v", err)
		return h.generateError(c, "Failed to start report generation")
	}

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Trigger", `{"show-toast":{"message":"Report generation started","intent":"success"}}`)
		return Render(c, http.StatusOK, webpages.GenerateSuccessFragment())
	}

	setFlashCookie(c, webui.Flash{Intent: "success", Message: "Report generation started"})
	return c.Redirect(http.StatusSeeOther, "/reports")
}

// generateError renders an inline error fragment for HTMX requests, or sets a
// flash and redirects back to the reports list for browser requests.
func (h *ReportsHandler) generateError(c echo.Context, message string) error {
	if IsHTMX(c) {
		return Render(c, http.StatusOK, webpages.ErrorFragment(message))
	}
	setFlashCookie(c, webui.Flash{Intent: "error", Message: message})
	return c.Redirect(http.StatusSeeOther, "/reports")
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
