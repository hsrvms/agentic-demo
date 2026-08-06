package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/scheduling"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockScheduleService implements scheduling.ScheduleService for tests.
type mockScheduleService struct {
	schedules  []scheduling.ReportSchedule
	schedule   scheduling.ReportSchedule
	err        error
	findErr    error
	createErr  error
	updateErr  error
	lastCreate *scheduling.CreateScheduleParams
	lastUpdate *scheduling.UpdateScheduleParams
}

func (m *mockScheduleService) Create(_ context.Context, p *scheduling.CreateScheduleParams) (scheduling.ReportSchedule, error) {
	m.lastCreate = p
	if m.createErr != nil {
		return scheduling.ReportSchedule{}, m.createErr
	}
	return scheduling.ReportSchedule{
		ID:       uuid.New(),
		TenantID: p.TenantID,
		Type:     p.Type,
		CronExpr: p.CronExpr,
		Focus:    p.Focus,
		Format:   p.Format,
		Enabled:  true,
	}, nil
}

func (m *mockScheduleService) GetByID(_ context.Context, _ uuid.UUID) (scheduling.ReportSchedule, error) {
	if m.findErr != nil {
		return scheduling.ReportSchedule{}, m.findErr
	}
	return m.schedule, nil
}

func (m *mockScheduleService) ListByTenant(_ context.Context, _ string) ([]scheduling.ReportSchedule, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.schedules, nil
}

func (m *mockScheduleService) Update(_ context.Context, p *scheduling.UpdateScheduleParams) (scheduling.ReportSchedule, error) {
	m.lastUpdate = p
	if m.updateErr != nil {
		return scheduling.ReportSchedule{}, m.updateErr
	}
	return scheduling.ReportSchedule{
		ID:       p.ID,
		TenantID: m.schedule.TenantID,
		Type:     p.Type,
		CronExpr: p.CronExpr,
		Focus:    p.Focus,
		Format:   p.Format,
		Enabled:  m.schedule.Enabled,
	}, nil
}

func (m *mockScheduleService) Delete(context.Context, uuid.UUID) error { return nil }

func (m *mockScheduleService) Toggle(_ context.Context, id uuid.UUID) (scheduling.ReportSchedule, error) {
	if m.findErr != nil {
		return scheduling.ReportSchedule{}, m.findErr
	}
	s := m.schedule
	s.ID = id
	s.Enabled = !s.Enabled
	return s, nil
}

func (m *mockScheduleService) ListAllEnabled(context.Context) ([]scheduling.ReportSchedule, error) {
	return m.schedules, nil
}

func newScheduleContext(t *testing.T, method, path string, tenantID domain.TenantID) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	e := echo.New()
	return e.NewContext(req, rec), rec
}

const scheduleTenant = "t_test"

// --- List ---

func TestSchedulesHandler_List_RendersTable(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{
		schedules: []scheduling.ReportSchedule{
			{ID: id, TenantID: scheduleTenant, Type: scheduling.ScheduleDaily, CronExpr: "0 9 * * *", Focus: "Revenue signals", Format: "standard", Enabled: true},
			{ID: uuid.New(), TenantID: scheduleTenant, Type: scheduling.ScheduleWeekly, CronExpr: "0 8 * * 1", Format: "concise", Enabled: false},
		},
	}
	handler := NewSchedulesHandler(svc)

	c, rec := newScheduleContext(t, http.MethodGet, "/schedules", scheduleTenant)
	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// Type badges.
	assert.Contains(t, body, "Daily")
	assert.Contains(t, body, "Weekly")
	// Human-readable cron.
	assert.Contains(t, body, "0 9 * * *")
	assert.Contains(t, body, "Every day at 09:00")
	assert.Contains(t, body, "Every Monday at 08:00")
	// Focus.
	assert.Contains(t, body, "Revenue signals")
	// Format labels.
	assert.Contains(t, body, "Standard")
	assert.Contains(t, body, "Concise")
	// Toggle switch reflects enabled state.
	assert.Contains(t, body, `role="switch"`)
	assert.Contains(t, body, `translate-x-6`)
	assert.Contains(t, body, `translate-x-1`)
	// The enabled/disabled state is labelled so color is never the sole indicator.
	assert.Contains(t, body, ">Enabled</span>")
	assert.Contains(t, body, ">Disabled</span>")
	// Edit and delete actions.
	assert.Contains(t, body, "/schedules/"+id.String()+"/edit")
	assert.Contains(t, body, `hx-delete="/schedules/`+id.String()+`"`)
	assert.Contains(t, body, "Delete this schedule?")
	// Toggle endpoint.
	assert.Contains(t, body, "/schedules/"+id.String()+"/toggle")
}

func TestSchedulesHandler_List_EmptyState(t *testing.T) {
	svc := &mockScheduleService{schedules: []scheduling.ReportSchedule{}}
	handler := NewSchedulesHandler(svc)

	c, rec := newScheduleContext(t, http.MethodGet, "/schedules", scheduleTenant)
	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "No schedules configured")
	assert.Contains(t, rec.Body.String(), "Create one to automate report generation.")
}

func TestSchedulesHandler_List_ServiceError(t *testing.T) {
	svc := &mockScheduleService{err: assert.AnError}
	handler := NewSchedulesHandler(svc)

	c, _ := newScheduleContext(t, http.MethodGet, "/schedules", scheduleTenant)
	err := handler.List(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}

// --- Form rendering ---

func TestSchedulesHandler_NewForm_RendersFields(t *testing.T) {
	handler := NewSchedulesHandler(&mockScheduleService{})

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/schedules/new", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), scheduleTenant)
	ctx = SetCSRFToken(ctx, "tok-123")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.NewForm(c)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "Add Schedule")
	assert.Contains(t, body, `name="_csrf" value="tok-123"`)
	// Type selector options.
	assert.Contains(t, body, `value="daily"`)
	assert.Contains(t, body, `value="weekly"`)
	assert.Contains(t, body, `value="monthly"`)
	// Format selector options.
	assert.Contains(t, body, `value="concise"`)
	assert.Contains(t, body, `value="detailed"`)
	// Friendly inputs instead of a raw cron field.
	assert.Contains(t, body, `name="time"`)
	assert.Contains(t, body, `name="day_of_week"`)
	assert.Contains(t, body, `name="day_of_month"`)
	assert.NotContains(t, body, `name="cron_expr"`)
	// Live schedule preview is present.
	assert.Contains(t, body, "Schedule Preview")
	// Submits via HTMX.
	assert.Contains(t, body, `hx-post="/schedules"`)
}

func TestSchedulesHandler_EditForm_PrefillsValues(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{
		schedule: scheduling.ReportSchedule{
			ID: id, TenantID: scheduleTenant, Type: scheduling.ScheduleMonthly,
			CronExpr: "0 9 1 * *", Focus: "Expansion", Format: "detailed",
		},
	}
	handler := NewSchedulesHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/schedules/"+id.String()+"/edit", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), scheduleTenant)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.EditForm(c)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "Edit Schedule")
	assert.Contains(t, body, `hx-put="/schedules/`+id.String()+`"`)
	// Prefilled friendly fields derived from the stored cron.
	assert.Contains(t, body, `type="time"`)
	assert.Contains(t, body, `value="09:00"`)
	assert.Contains(t, body, `value="monthly" selected`)
	assert.Contains(t, body, `value="1" selected`) // day of month
	assert.Contains(t, body, "Expansion")
	assert.Contains(t, body, `value="detailed" selected`)
}

func TestSchedulesHandler_EditForm_NotFound(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{findErr: scheduling.ErrScheduleNotFound}
	handler := NewSchedulesHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/schedules/"+id.String()+"/edit", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), scheduleTenant)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.EditForm(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

func TestSchedulesHandler_EditForm_CrossTenantDenied(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{
		schedule: scheduling.ReportSchedule{ID: id, TenantID: "t_other"},
	}
	handler := NewSchedulesHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/schedules/"+id.String()+"/edit", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), scheduleTenant)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.EditForm(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

// --- Create ---

func postScheduleForm(t *testing.T, method, path, body string, htmx bool) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody)
	} else {
		req = httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	ctx := auth.SetTenantID(req.Context(), scheduleTenant)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	e := echo.New()
	return e.NewContext(req, rec), rec
}

func TestSchedulesHandler_Create_Success_HTMX(t *testing.T) {
	svc := &mockScheduleService{}
	handler := NewSchedulesHandler(svc)
	c, rec := postScheduleForm(t, http.MethodPost, "/schedules", "type=daily&time=09:00&focus=Revenue&format=standard", true)

	err := handler.Create(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/schedules", rec.Header().Get("HX-Redirect"))

	// The cron is composed from the friendly inputs, not typed by the user.
	require.NotNil(t, svc.lastCreate)
	assert.Equal(t, "0 9 * * *", svc.lastCreate.CronExpr)
	assert.Equal(t, scheduling.ScheduleDaily, svc.lastCreate.Type)
	assert.Equal(t, "Revenue", svc.lastCreate.Focus)
}

func TestSchedulesHandler_Create_WeeklyComposesDayOfWeek(t *testing.T) {
	svc := &mockScheduleService{}
	handler := NewSchedulesHandler(svc)
	c, _ := postScheduleForm(t, http.MethodPost, "/schedules", "type=weekly&time=08:00&day_of_week=1", true)

	err := handler.Create(c)
	require.NoError(t, err)
	require.NotNil(t, svc.lastCreate)
	assert.Equal(t, "0 8 * * 1", svc.lastCreate.CronExpr)
}

func TestSchedulesHandler_Create_MonthlyComposesDayOfMonth(t *testing.T) {
	svc := &mockScheduleService{}
	handler := NewSchedulesHandler(svc)
	c, _ := postScheduleForm(t, http.MethodPost, "/schedules", "type=monthly&time=09:00&day_of_month=15", true)

	err := handler.Create(c)
	require.NoError(t, err)
	require.NotNil(t, svc.lastCreate)
	assert.Equal(t, "0 9 15 * *", svc.lastCreate.CronExpr)
}

func TestSchedulesHandler_Create_Success_BrowserRedirect(t *testing.T) {
	handler := NewSchedulesHandler(&mockScheduleService{})
	c, rec := postScheduleForm(t, http.MethodPost, "/schedules", "type=weekly&time=08:00&day_of_week=1", false)

	err := handler.Create(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/schedules", rec.Header().Get("Location"))
}

func TestSchedulesHandler_Create_InvalidTime(t *testing.T) {
	handler := NewSchedulesHandler(&mockScheduleService{})
	c, rec := postScheduleForm(t, http.MethodPost, "/schedules", "type=daily&time=not-a-time", true)

	err := handler.Create(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Please select a valid time of day")
}

func TestSchedulesHandler_Create_AlreadyExists(t *testing.T) {
	svc := &mockScheduleService{createErr: scheduling.ErrScheduleAlreadyExists}
	handler := NewSchedulesHandler(svc)
	c, rec := postScheduleForm(t, http.MethodPost, "/schedules", "type=daily&time=09:00", true)

	err := handler.Create(c)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "already exists")
}

func TestSchedulesHandler_Create_InvalidType(t *testing.T) {
	handler := NewSchedulesHandler(&mockScheduleService{})
	c, rec := postScheduleForm(t, http.MethodPost, "/schedules", "type=bogus&time=09:00", true)

	err := handler.Create(c)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "valid report type")
}

// --- Update ---

func TestSchedulesHandler_Update_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{
		schedule: scheduling.ReportSchedule{ID: id, TenantID: scheduleTenant, Enabled: true},
	}
	handler := NewSchedulesHandler(svc)

	c, rec := postScheduleForm(t, http.MethodPut, "/schedules/"+id.String(), "type=daily&time=10:00&format=concise", true)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Update(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/schedules", rec.Header().Get("HX-Redirect"))

	require.NotNil(t, svc.lastUpdate)
	assert.Equal(t, "0 10 * * *", svc.lastUpdate.CronExpr)
}

func TestSchedulesHandler_Update_CrossTenantDenied(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{
		schedule: scheduling.ReportSchedule{ID: id, TenantID: "t_other"},
	}
	handler := NewSchedulesHandler(svc)

	c, rec := postScheduleForm(t, http.MethodPut, "/schedules/"+id.String(), "type=daily&time=10:00", true)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Update(c)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "Schedule not found")
}

// --- Delete ---

func TestSchedulesHandler_Delete_Success_HTMX(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{
		schedule: scheduling.ReportSchedule{ID: id, TenantID: scheduleTenant},
	}
	handler := NewSchedulesHandler(svc)

	c, rec := postScheduleForm(t, http.MethodDelete, "/schedules/"+id.String(), "", true)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Delete(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestSchedulesHandler_Delete_Success_BrowserRedirect(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{
		schedule: scheduling.ReportSchedule{ID: id, TenantID: scheduleTenant},
	}
	handler := NewSchedulesHandler(svc)

	c, rec := postScheduleForm(t, http.MethodDelete, "/schedules/"+id.String(), "", false)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Delete(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/schedules", rec.Header().Get("Location"))
}

func TestSchedulesHandler_Delete_NotFound_HTMX(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{findErr: scheduling.ErrScheduleNotFound}
	handler := NewSchedulesHandler(svc)

	c, rec := postScheduleForm(t, http.MethodDelete, "/schedules/"+id.String(), "", true)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Delete(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
	assert.Empty(t, rec.Body.String())
}

// --- Toggle ---

func TestSchedulesHandler_Toggle_ReturnsUpdatedRow(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{
		schedule: scheduling.ReportSchedule{
			ID: id, TenantID: scheduleTenant, Type: scheduling.ScheduleDaily,
			CronExpr: "0 9 * * *", Enabled: true,
		},
	}
	handler := NewSchedulesHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/schedules/"+id.String()+"/toggle", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), scheduleTenant)
	ctx = SetCSRFToken(ctx, "tok-123")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Toggle(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// Toggled to disabled: knob at off position, aria-checked=false.
	assert.Contains(t, body, `aria-checked="false"`)
	assert.Contains(t, body, `translate-x-1`)
	// A clear text label communicates the disabled state.
	assert.Contains(t, body, ">Disabled</span>")
	assert.NotContains(t, body, ">Enabled</span>")
	// Row fragment only, no page shell.
	assert.NotContains(t, body, "<html")
}

func TestSchedulesHandler_Toggle_NotFound(t *testing.T) {
	id := uuid.New()
	svc := &mockScheduleService{findErr: scheduling.ErrScheduleNotFound}
	handler := NewSchedulesHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/schedules/"+id.String()+"/toggle", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), scheduleTenant)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Toggle(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}
