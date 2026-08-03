package scheduling

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/httperr"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler holds the dependencies for schedule HTTP endpoints.
type Handler struct {
	core *HandlerCore
}

// NewHandler creates a schedule Handler.
func NewHandler(core *HandlerCore) *Handler {
	return &Handler{core: core}
}

// Register wires schedule routes under the given Echo group with the provided middleware.
func (h *Handler) Register(g *echo.Group, mw ...echo.MiddlewareFunc) {
	g.GET("/schedules", h.List, mw...)
	g.POST("/schedules", h.Create, mw...)
	g.PUT("/schedules/:id", h.Update, mw...)
	g.DELETE("/schedules/:id", h.Delete, mw...)
	g.POST("/schedules/:id/toggle", h.Toggle, mw...)
}

// --- request types ---

type createScheduleRequest struct {
	Type     string `json:"type"`
	CronExpr string `json:"cron_expr"`
	Focus    string `json:"focus,omitempty"`
	Format   string `json:"format,omitempty"`
}

type updateScheduleRequest struct {
	Type     string `json:"type"`
	CronExpr string `json:"cron_expr"`
	Focus    string `json:"focus,omitempty"`
	Format   string `json:"format,omitempty"`
}

// --- handlers ---

func (h *Handler) List(c echo.Context) error {
	tenantID := getTenantID(c)

	result, err := h.core.List(c.Request().Context(), tenantID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list schedules")
	}

	response := make([]map[string]interface{}, len(result.Schedules))
	for i := range result.Schedules {
		response[i] = scheduleResponse(&result.Schedules[i])
	}

	return c.JSON(http.StatusOK, response)
}

func (h *Handler) Create(c echo.Context) error {
	tenantID := getTenantID(c)

	var req createScheduleRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	result, err := h.core.Create(c.Request().Context(), tenantID, &CreateScheduleParams{
		TenantID: tenantID,
		Type:     ScheduleType(req.Type),
		CronExpr: req.CronExpr,
		Focus:    req.Focus,
		Format:   req.Format,
	})
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	return c.JSON(http.StatusCreated, scheduleResponse(&result.Schedule))
}

func (h *Handler) Update(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid schedule ID")
	}

	var req updateScheduleRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	result, err := h.core.Update(c.Request().Context(), &UpdateScheduleParams{
		ID:       id,
		Type:     ScheduleType(req.Type),
		CronExpr: req.CronExpr,
		Focus:    req.Focus,
		Format:   req.Format,
	})
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	return c.JSON(http.StatusOK, scheduleResponse(&result.Schedule))
}

func (h *Handler) Delete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid schedule ID")
	}

	if _, err := h.core.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) Toggle(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid schedule ID")
	}

	result, err := h.core.Toggle(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	return c.JSON(http.StatusOK, scheduleResponse(&result.Schedule))
}

// --- helpers ---

func getTenantID(c echo.Context) string {
	return string(auth.GetTenantID(c.Request().Context()))
}

func scheduleResponse(s *ReportSchedule) map[string]interface{} {
	return map[string]interface{}{
		"id":         s.ID.String(),
		"tenant_id":  s.TenantID,
		"type":       string(s.Type),
		"cron_expr":  s.CronExpr,
		"focus":      s.Focus,
		"format":     s.Format,
		"enabled":    s.Enabled,
		"created_at": s.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated_at": s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
