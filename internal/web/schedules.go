package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/scheduling"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// SchedulesHandler serves the report schedule management pages.
type SchedulesHandler struct {
	service scheduling.ScheduleService
}

// NewSchedulesHandler creates a SchedulesHandler.
func NewSchedulesHandler(service scheduling.ScheduleService) *SchedulesHandler {
	return &SchedulesHandler{service: service}
}

// Register mounts schedule routes on the authenticated web group.
func (h *SchedulesHandler) Register(g *echo.Group) {
	g.GET("/schedules", h.List)
	g.GET("/schedules/new", h.NewForm)
	g.GET("/schedules/:id/edit", h.EditForm)
	g.POST("/schedules", h.Create)
	g.PUT("/schedules/:id", h.Update)
	g.DELETE("/schedules/:id", h.Delete)
	g.POST("/schedules/:id/toggle", h.Toggle)
}

// List handles GET /schedules.
func (h *SchedulesHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	schedules, err := h.service.ListByTenant(ctx, tenantID)
	if err != nil {
		log.Printf("schedules list error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load schedules")
	}

	data := webui.MapScheduleList(schedules)
	csrf := GetCSRFToken(ctx)
	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.SchedulesList(data, csrf, flashes))
}

// NewForm handles GET /schedules/new.
func (h *SchedulesHandler) NewForm(c echo.Context) error {
	ctx := c.Request().Context()

	data := webui.ScheduleFormData{
		CSRFToken:     GetCSRFToken(ctx),
		TypeOptions:   webui.ScheduleTypeOptions(),
		FormatOptions: webui.ReportFormatOptions(),
	}
	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.ScheduleForm(data, flashes))
}

// EditForm handles GET /schedules/:id/edit.
func (h *SchedulesHandler) EditForm(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid schedule ID")
	}

	s, err := h.ownedSchedule(ctx, tenantID, id)
	if err != nil {
		return h.scheduleError(err)
	}

	// Decompose the stored cron back into the friendly form fields.
	timeOfDay, dayOfWeek, dayOfMonth := webui.CronParts(s.CronExpr)
	data := webui.ScheduleFormData{
		Editing:       true,
		ScheduleID:    s.ID.String(),
		Type:          string(s.Type),
		Time:          timeOfDay,
		DayOfWeek:     dayOfWeek,
		DayOfMonth:    dayOfMonth,
		Focus:         s.Focus,
		Format:        s.Format,
		CSRFToken:     GetCSRFToken(ctx),
		TypeOptions:   webui.ScheduleTypeOptions(),
		FormatOptions: webui.ReportFormatOptions(),
	}
	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.ScheduleForm(data, flashes))
}

// Create handles POST /schedules. It composes the cron expression from the
// friendly form inputs (type, time, and cadence-specific day) so the user
// never has to author cron syntax directly.
func (h *SchedulesHandler) Create(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	cronExpr, err := h.formCronExpr(c)
	if err != nil {
		return h.formError(c, err)
	}

	params := &scheduling.CreateScheduleParams{
		TenantID: tenantID,
		Type:     scheduling.ScheduleType(c.FormValue("type")),
		CronExpr: cronExpr,
		Focus:    strings.TrimSpace(c.FormValue("focus")),
		Format:   strings.TrimSpace(c.FormValue("format")),
	}

	if _, err := h.service.Create(ctx, params); err != nil {
		return h.formError(c, err)
	}

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/schedules")
		return c.NoContent(http.StatusOK)
	}
	setFlashCookie(c, webui.Flash{Intent: "success", Message: "Schedule created"})
	return c.Redirect(http.StatusSeeOther, "/schedules")
}

// Update handles PUT /schedules/:id.
func (h *SchedulesHandler) Update(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid schedule ID")
	}

	if _, err := h.ownedSchedule(ctx, tenantID, id); err != nil {
		return h.formError(c, err)
	}

	cronExpr, err := h.formCronExpr(c)
	if err != nil {
		return h.formError(c, err)
	}

	params := &scheduling.UpdateScheduleParams{
		ID:       id,
		Type:     scheduling.ScheduleType(c.FormValue("type")),
		CronExpr: cronExpr,
		Focus:    strings.TrimSpace(c.FormValue("focus")),
		Format:   strings.TrimSpace(c.FormValue("format")),
	}

	if _, err := h.service.Update(ctx, params); err != nil {
		return h.formError(c, err)
	}

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/schedules")
		return c.NoContent(http.StatusOK)
	}
	setFlashCookie(c, webui.Flash{Intent: "success", Message: "Schedule updated"})
	return c.Redirect(http.StatusSeeOther, "/schedules")
}

// Delete handles DELETE /schedules/:id.
func (h *SchedulesHandler) Delete(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid schedule ID")
	}

	if _, err := h.ownedSchedule(ctx, tenantID, id); err != nil {
		return h.scheduleError(err)
	}

	if err := h.service.Delete(ctx, id); err != nil {
		log.Printf("schedules delete error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete schedule")
	}

	if IsHTMX(c) {
		return c.NoContent(http.StatusOK)
	}
	setFlashCookie(c, webui.Flash{Intent: "success", Message: "Schedule deleted"})
	return c.Redirect(http.StatusSeeOther, "/schedules")
}

// Toggle handles POST /schedules/:id/toggle. It flips the schedule's enabled
// state and returns the updated row fragment so the toggle reflects in place.
func (h *SchedulesHandler) Toggle(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid schedule ID")
	}

	if _, err := h.ownedSchedule(ctx, tenantID, id); err != nil {
		return h.scheduleError(err)
	}

	s, err := h.service.Toggle(ctx, id)
	if err != nil {
		if errors.Is(err, scheduling.ErrScheduleNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "schedule not found")
		}
		log.Printf("schedules toggle error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to toggle schedule")
	}

	csrf := GetCSRFToken(ctx)
	return Render(c, http.StatusOK, webpages.ScheduleRow(webui.MapScheduleItem(&s), csrf))
}

// --- helpers ---

// formCronExpr composes the cron expression from the form's friendly inputs.
func (h *SchedulesHandler) formCronExpr(c echo.Context) (string, error) {
	return webui.BuildCronExpr(
		c.FormValue("type"),
		c.FormValue("time"),
		c.FormValue("day_of_week"),
		c.FormValue("day_of_month"),
	)
}

// ownedSchedule fetches a schedule and verifies it belongs to the current
// tenant, enforcing tenant isolation before any mutation.
func (h *SchedulesHandler) ownedSchedule(ctx context.Context, tenantID string, id uuid.UUID) (scheduling.ReportSchedule, error) {
	s, err := h.service.GetByID(ctx, id)
	if err != nil {
		return scheduling.ReportSchedule{}, err
	}
	if s.TenantID != tenantID {
		return scheduling.ReportSchedule{}, scheduling.ErrScheduleNotFound
	}
	return s, nil
}

// formError maps a service error to an HTMX error fragment or a redirect with
// a flash message for browser requests.
func (h *SchedulesHandler) formError(c echo.Context, err error) error {
	message := "Something went wrong"
	switch {
	case errors.Is(err, webui.ErrInvalidTime):
		message = "Please select a valid time of day"
	case errors.Is(err, scheduling.ErrInvalidCronExpr):
		message = "The generated schedule is invalid"
	case errors.Is(err, scheduling.ErrInvalidScheduleType):
		message = "Please select a valid report type"
	case errors.Is(err, scheduling.ErrScheduleAlreadyExists):
		message = "A schedule of this type already exists for your tenant"
	case errors.Is(err, scheduling.ErrScheduleNotFound):
		message = "Schedule not found"
	}

	if IsHTMX(c) {
		return Render(c, http.StatusOK, webpages.ErrorFragment(message))
	}
	setFlashCookie(c, webui.Flash{Intent: "error", Message: message})
	referer := c.Request().Referer()
	if referer == "" {
		referer = "/schedules"
	}
	return c.Redirect(http.StatusSeeOther, referer)
}

// scheduleError maps a schedule fetch error to an HTTP response.
func (h *SchedulesHandler) scheduleError(err error) error {
	if errors.Is(err, scheduling.ErrScheduleNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "schedule not found")
	}
	log.Printf("schedules fetch error: %v", err)
	return echo.NewHTTPError(http.StatusInternalServerError, "failed to load schedule")
}
