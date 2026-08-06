package web

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/labstack/echo/v4"
)

const usageEventPageSize = 20

// UsageHandler serves the platform usage page: a current-month snapshot from
// Redis, a historical summary by date range, and a paginated event log.
type UsageHandler struct {
	service usage.UsageService
}

// NewUsageHandler creates a UsageHandler.
func NewUsageHandler(service usage.UsageService) *UsageHandler {
	return &UsageHandler{service: service}
}

// Register mounts usage routes on the authenticated web group.
func (h *UsageHandler) Register(g *echo.Group) {
	g.GET("/usage", h.Page)
	g.GET("/usage/summary", h.Summary)
	g.GET("/usage/events", h.Events)
}

// Page handles GET /usage. It renders the full usage page with the Redis-backed
// current-month stat cards, per-model breakdown, and the first page of events.
func (h *UsageHandler) Page(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))
	tenantName := GetTenantName(ctx)

	currentUsage, err := h.service.GetCurrentUsage(ctx, tenantID)
	if err != nil {
		log.Printf("usage page error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load usage")
	}

	events, err := h.service.ListEvents(ctx, tenantID, 1, usageEventPageSize)
	if err != nil {
		log.Printf("usage events error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load usage events")
	}

	data := webui.MapUsage(currentUsage, events)
	data.TenantName = tenantName

	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.Usage(data, flashes))
}

// Summary handles GET /usage/summary. It returns a summary fragment with a
// per-model cost table for the requested date range. The fragment is swapped
// into the page by the HTMX date-range form.
func (h *UsageHandler) Summary(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	from := strings.TrimSpace(c.QueryParam("from"))
	to := strings.TrimSpace(c.QueryParam("to"))

	summary, err := h.service.GetSummary(ctx, tenantID, from, to)
	if err != nil {
		log.Printf("usage summary error: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, "invalid date range")
	}

	return Render(c, http.StatusOK, webpages.UsageSummaryFragment(webui.MapUsageSummary(summary)))
}

// Events handles GET /usage/events. It returns just the next page of event
// rows; the page's reveal sentinel swaps them into the event table body.
func (h *UsageHandler) Events(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	result, err := h.service.ListEvents(ctx, tenantID, page, usageEventPageSize)
	if err != nil {
		log.Printf("usage events error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load usage events")
	}

	items := webui.MapUsageEvents(result.Events)
	return Render(c, http.StatusOK, webpages.UsageEventRowsFragment(items))
}
