package reports

import (
	"net/http"
	"strconv"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/httperr"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler holds the dependencies for report HTTP endpoints.
type Handler struct {
	service ReportService
}

// NewHandler creates a report Handler.
func NewHandler(service ReportService) *Handler {
	return &Handler{service: service}
}

// Register wires report routes under the given Echo group with the provided middleware.
func (h *Handler) Register(g *echo.Group, mw ...echo.MiddlewareFunc) {
	g.GET("/reports", h.List, mw...)
	g.GET("/reports/:id", h.Get, mw...)
}

// --- handlers ---

func (h *Handler) List(c echo.Context) error {
	tenantID := getTenantID(c)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("limit"))

	result, err := h.service.ListByTenant(c.Request().Context(), tenantID, page, pageSize)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	type reportItem struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		Title       string `json:"title"`
		Focus       string `json:"focus,omitempty"`
		ScheduleID  string `json:"schedule_id,omitempty"`
		GeneratedAt string `json:"generated_at"`
	}

	items := make([]reportItem, len(result.Reports))
	for i := range result.Reports {
		r := &result.Reports[i]
		item := reportItem{
			ID:          r.ID.String(),
			Type:        r.Type,
			Title:       r.Title,
			Focus:       r.Focus,
			GeneratedAt: r.GeneratedAt.Format("2006-01-02T15:04:05Z"),
		}
		if r.ScheduleID != uuid.Nil {
			item.ScheduleID = r.ScheduleID.String()
		}
		items[i] = item
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"reports":     items,
		"total_count": result.TotalCount,
		"page":        result.Page,
		"page_size":   result.PageSize,
	})
}

func (h *Handler) Get(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid report ID")
	}

	r, err := h.service.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	resp := map[string]interface{}{
		"id":           r.ID.String(),
		"tenant_id":    r.TenantID,
		"type":         r.Type,
		"title":        r.Title,
		"content":      r.Content,
		"citations":    r.Citations,
		"focus":        r.Focus,
		"generated_at": r.GeneratedAt.Format("2006-01-02T15:04:05Z"),
		"created_at":   r.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if r.ScheduleID != uuid.Nil {
		resp["schedule_id"] = r.ScheduleID.String()
	}

	return c.JSON(http.StatusOK, resp)
}

// --- helpers ---

func getTenantID(c echo.Context) string {
	return string(auth.GetTenantID(c.Request().Context()))
}
