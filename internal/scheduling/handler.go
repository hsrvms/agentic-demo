package scheduling

import (
	"errors"
	"net/http"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler holds the dependencies for schedule HTTP endpoints.
type Handler struct {
	service ScheduleService
}

// NewHandler creates a schedule Handler.
func NewHandler(service ScheduleService) *Handler {
	return &Handler{service: service}
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
	schedules, err := h.service.ListByTenant(c.Request().Context(), tenantID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list schedules")
	}

	type response struct {
		ID        string `json:"id"`
		TenantID  string `json:"tenant_id"`
		Type      string `json:"type"`
		CronExpr  string `json:"cron_expr"`
		Focus     string `json:"focus,omitempty"`
		Format    string `json:"format"`
		Enabled   bool   `json:"enabled"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	result := make([]response, len(schedules))
	for i := range schedules {
		s := &schedules[i]
		result[i] = response{
			ID:        s.ID.String(),
			TenantID:  s.TenantID,
			Type:      string(s.Type),
			CronExpr:  s.CronExpr,
			Focus:     s.Focus,
			Format:    s.Format,
			Enabled:   s.Enabled,
			CreatedAt: s.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return c.JSON(http.StatusOK, result)
}

func (h *Handler) Create(c echo.Context) error {
	tenantID := getTenantID(c)

	var req createScheduleRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	s, err := h.service.Create(c.Request().Context(), &CreateScheduleParams{
		TenantID: tenantID,
		Type:     ScheduleType(req.Type),
		CronExpr: req.CronExpr,
		Focus:    req.Focus,
		Format:   req.Format,
	})
	if err != nil {
		return mapError(err)
	}

	return c.JSON(http.StatusCreated, scheduleResponse(&s))
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

	s, err := h.service.Update(c.Request().Context(), &UpdateScheduleParams{
		ID:       id,
		Type:     ScheduleType(req.Type),
		CronExpr: req.CronExpr,
		Focus:    req.Focus,
		Format:   req.Format,
	})
	if err != nil {
		return mapError(err)
	}

	return c.JSON(http.StatusOK, scheduleResponse(&s))
}

func (h *Handler) Delete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid schedule ID")
	}

	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		return mapError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) Toggle(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid schedule ID")
	}

	s, err := h.service.Toggle(c.Request().Context(), id)
	if err != nil {
		return mapError(err)
	}

	return c.JSON(http.StatusOK, scheduleResponse(&s))
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

func mapError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, ErrScheduleNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, ErrScheduleAlreadyExists):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidCronExpr):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidScheduleType):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidTenantID):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}