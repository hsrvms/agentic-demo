package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/queue"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockJobQueue records the last enqueued job and returns a configurable error.
type mockJobQueue struct {
	lastJob queue.Job
	err     error
}

func (m *mockJobQueue) Enqueue(_ context.Context, job queue.Job) (*queue.JobResult, error) {
	m.lastJob = job
	if m.err != nil {
		return nil, m.err
	}
	return &queue.JobResult{ID: "job-1", Queue: job.Queue}, nil
}

func (m *mockJobQueue) EnqueueAt(_ context.Context, job queue.Job, _ time.Time) (*queue.JobResult, error) {
	m.lastJob = job
	if m.err != nil {
		return nil, m.err
	}
	return &queue.JobResult{ID: "job-1", Queue: job.Queue}, nil
}

func (m *mockJobQueue) Close() error {
	return nil
}

// --- List tests ---

func TestReportsHandler_List_RendersTable(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	generated := time.Now().Add(-24 * time.Hour)

	svc := &mockReportService{
		page: reports.ReportPage{
			Reports: []reports.StoredReport{
				{ID: uuid.New(), Title: "Daily Brief", Type: "daily", Focus: "Signals", GeneratedAt: generated},
				{ID: uuid.New(), Title: "Weekly Review", Type: "weekly", GeneratedAt: generated},
			},
			TotalCount: 2,
			Page:       1,
			PageSize:   20,
		},
	}

	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Daily Brief")
	assert.Contains(t, body, "Weekly Review")
	assert.Contains(t, body, "/reports/")
	assert.Contains(t, body, "Daily")
	assert.Contains(t, body, "Weekly")
	assert.Contains(t, body, "Signals")
	// Type badges use the correct intent colors.
	assert.Contains(t, body, "bg-intent-info/10 text-intent-info")
	assert.Contains(t, body, "bg-primary-subtle text-primary")
}

func TestReportsHandler_List_EmptyState(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockReportService{
		page: reports.ReportPage{
			Reports:    []reports.StoredReport{},
			TotalCount: 0,
			Page:       1,
			PageSize:   20,
		},
	}

	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "No reports yet. Generate one to get started.")
}

func TestReportsHandler_List_HTMXPaginationFragment(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockReportService{
		page: reports.ReportPage{
			Reports: []reports.StoredReport{
				{ID: uuid.New(), Title: "Page 2 Report", Type: "monthly", GeneratedAt: time.Now()},
			},
			TotalCount: 25,
			Page:       2,
			PageSize:   20,
		},
	}

	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports?page=2", http.NoBody)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Page 2 Report")
	// Fragment must not render the full page shell.
	assert.Contains(t, body, "<tr")
	assert.NotContains(t, body, "<html")
}

func TestReportsHandler_List_ServiceError(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockReportService{err: assert.AnError}
	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}

// --- Detail tests ---

func TestReportsHandler_Detail_RendersContentAndCitations(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	id := uuid.New()

	svc := &mockReportService{
		report: reports.StoredReport{
			ID:          id,
			TenantID:    string(tenantID),
			Title:       "Monthly Review",
			Type:        "monthly",
			Content:     "# Revenue\n\nRevenue is **up 12%**.",
			Focus:       "Growth",
			Citations:   json.RawMessage(`[{"title":"Q3 Report","url":"https://example.com/q3","source":"Internal"}]`),
			GeneratedAt: time.Now(),
		},
	}

	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/"+id.String(), http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Detail(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Monthly Review")
	assert.Contains(t, body, "<h1>Revenue</h1>")
	assert.Contains(t, body, "<strong>up 12%</strong>")
	assert.Contains(t, body, "Q3 Report")
	assert.Contains(t, body, "https://example.com/q3")
	assert.Contains(t, body, "Back to Reports")
}

func TestReportsHandler_Detail_InvalidID(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	handler := NewReportsHandler(&mockReportService{}, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/bad", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("bad")

	err := handler.Detail(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestReportsHandler_Detail_NotFound(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	id := uuid.New()

	svc := &mockReportService{detailErr: reports.ErrReportNotFound}
	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/"+id.String(), http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Detail(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

func TestReportsHandler_Detail_CrossTenantDenied(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	otherTenant := domain.TenantID("t_other")
	id := uuid.New()

	// Report belongs to a different tenant.
	svc := &mockReportService{
		report: reports.StoredReport{
			ID:       id,
			TenantID: string(otherTenant),
			Title:    "Secret Report",
			Type:     "daily",
		},
	}
	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/"+id.String(), http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Detail(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
	assert.NotContains(t, rec.Body.String(), "Secret Report")
}

// --- Generate tests ---

func postGenerateRequest(t *testing.T, body string, htmx bool) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/reports/generate", http.NoBody)
	} else {
		req = httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/reports/generate", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	ctx := auth.SetTenantID(req.Context(), domain.TenantID("t_test"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	e := echo.New()
	return e.NewContext(req, rec), rec
}

func TestReportsHandler_Generate_Success_HTMX(t *testing.T) {
	q := &mockJobQueue{}
	handler := NewReportsHandler(&mockReportService{}, q, nil)

	c, rec := postGenerateRequest(t, "report_type=on_demand&focus=Growth", true)
	err := handler.Generate(c)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	// The job was enqueued with the correct type and tenant.
	require.Equal(t, queue.TypeReportOnDemand, q.lastJob.Type)
	require.Equal(t, queue.QueueReport, q.lastJob.Queue)
	payload, ok := q.lastJob.Payload.(*queue.ReportPayload)
	require.True(t, ok)
	assert.Equal(t, "t_test", payload.TenantID)
	assert.Equal(t, "on_demand", payload.ReportType)
	assert.Equal(t, []string{"Growth"}, payload.FocusAreas)
	assert.Equal(t, "web", payload.DeliveryMethod)

	// Success confirmation is returned and a toast is triggered.
	assert.Contains(t, rec.Header().Get("HX-Trigger"), "Report generation started")
	assert.Contains(t, rec.Body.String(), "Report generation started")
}

func TestReportsHandler_Generate_WeeklyWithSchedule(t *testing.T) {
	q := &mockJobQueue{}
	handler := NewReportsHandler(&mockReportService{}, q, nil)

	scheduleID := uuid.New().String()
	body := "report_type=weekly&schedule_id=" + scheduleID
	c, _ := postGenerateRequest(t, body, true)
	err := handler.Generate(c)
	require.NoError(t, err)

	require.Equal(t, queue.TypeReportWeekly, q.lastJob.Type)
	payload, ok := q.lastJob.Payload.(*queue.ReportPayload)
	require.True(t, ok)
	assert.Equal(t, scheduleID, payload.ScheduleID)
	assert.Len(t, payload.FocusAreas, 0) // empty focus not sent
}

func TestReportsHandler_Generate_DefaultsToOnDemand(t *testing.T) {
	q := &mockJobQueue{}
	handler := NewReportsHandler(&mockReportService{}, q, nil)

	c, _ := postGenerateRequest(t, "", true)
	err := handler.Generate(c)
	require.NoError(t, err)

	payload, ok := q.lastJob.Payload.(*queue.ReportPayload)
	require.True(t, ok)
	assert.Equal(t, "on_demand", payload.ReportType)
}

func TestReportsHandler_Generate_InvalidScheduleID(t *testing.T) {
	q := &mockJobQueue{}
	handler := NewReportsHandler(&mockReportService{}, q, nil)

	c, rec := postGenerateRequest(t, "schedule_id=not-a-uuid", true)
	err := handler.Generate(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Schedule reference must be a valid schedule ID")
	assert.Empty(t, q.lastJob.Type, "no job should be enqueued on invalid schedule ID")
}

func TestReportsHandler_Generate_InvalidReportType(t *testing.T) {
	q := &mockJobQueue{}
	handler := NewReportsHandler(&mockReportService{}, q, nil)

	c, rec := postGenerateRequest(t, "report_type=bogus", true)
	err := handler.Generate(c)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "Please select a valid report type")
	assert.Empty(t, q.lastJob.Type, "no job should be enqueued on invalid report type")
}

func TestReportsHandler_Generate_EnqueueError(t *testing.T) {
	q := &mockJobQueue{err: assert.AnError}
	handler := NewReportsHandler(&mockReportService{}, q, nil)

	c, rec := postGenerateRequest(t, "report_type=daily", true)
	err := handler.Generate(c)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "Failed to start report generation")
}

func TestReportsHandler_Generate_NonHTMXRedirect(t *testing.T) {
	q := &mockJobQueue{}
	handler := NewReportsHandler(&mockReportService{}, q, nil)

	c, rec := postGenerateRequest(t, "report_type=daily", false)
	err := handler.Generate(c)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/reports", rec.Header().Get("Location"))

	payload, ok := q.lastJob.Payload.(*queue.ReportPayload)
	require.True(t, ok)
	assert.Equal(t, "daily", payload.ReportType)
}

func TestReportsHandler_List_RendersGenerateControls(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	svc := &mockReportService{
		page: reports.ReportPage{
			Reports:    []reports.StoredReport{{ID: uuid.New(), Title: "Daily Brief", Type: "daily", GeneratedAt: time.Now()}},
			TotalCount: 1,
			Page:       1,
			PageSize:   20,
		},
	}
	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = SetCSRFToken(ctx, "tok-123")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	body := rec.Body.String()

	// Generate Report button in the header.
	assert.Contains(t, body, "Generate Report")
	// CSRF token is embedded in the modal form.
	assert.Contains(t, body, `name="_csrf" value="tok-123"`)
	// Report type selector offers all four types.
	assert.Contains(t, body, `value="daily"`)
	assert.Contains(t, body, `value="weekly"`)
	assert.Contains(t, body, `value="monthly"`)
	assert.Contains(t, body, `value="on_demand"`)
	// Modal posts to /reports/generate.
	assert.Contains(t, body, `hx-post="/reports/generate"`)
	// Loading spinner indicator is wired up.
	assert.Contains(t, body, "gen-submit-spinner")
	// HTMX polling refreshes the report list on page one.
	assert.Contains(t, body, `hx-trigger="every 10s"`)
}

// --- Generation job tracking & activity tests ---

func TestReportsHandler_Generate_TracksJob(t *testing.T) {
	q := &mockJobQueue{}
	svc := &mockReportService{}
	handler := NewReportsHandler(svc, q, nil)

	c, _ := postGenerateRequest(t, "report_type=on_demand&focus=Growth", true)
	err := handler.Generate(c)
	require.NoError(t, err)

	// The enqueued task ID is retained on the tracking record.
	assert.Equal(t, "job-1", svc.trackedTaskID)
}

func TestReportsHandler_Generate_TrackingErrorStillSucceeds(t *testing.T) {
	q := &mockJobQueue{}
	svc := &mockReportService{trackErr: assert.AnError}
	handler := NewReportsHandler(svc, q, nil)

	c, rec := postGenerateRequest(t, "report_type=on_demand", true)
	err := handler.Generate(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Report generation started")
}

func TestReportsHandler_List_RendersActivityPanel(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	svc := &mockReportService{
		page: reports.ReportPage{
			Reports:    []reports.StoredReport{{ID: uuid.New(), Title: "Daily Brief", Type: "daily", GeneratedAt: time.Now()}},
			TotalCount: 1,
			Page:       1,
			PageSize:   20,
		},
		jobs: []reports.GenerationJob{
			{ID: uuid.New(), TenantID: "t_test", TaskID: "task-1", ReportType: "on_demand", Focus: "Growth", Status: reports.GenerationJobQueued, EnqueuedAt: time.Now().Add(-time.Minute)},
			{ID: uuid.New(), TenantID: "t_test", TaskID: "task-2", ReportType: "weekly", Status: reports.GenerationJobFailed, Error: "llm timeout", EnqueuedAt: time.Now().Add(-time.Hour)},
		},
	}
	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	body := rec.Body.String()

	assert.Contains(t, body, "Recent Activity")
	assert.Contains(t, body, "Queued")
	assert.Contains(t, body, "On Demand")
	assert.Contains(t, body, "Failed")
	// Failed jobs surface their error message.
	assert.Contains(t, body, "llm timeout")
	// The activity panel polls the fragment endpoint.
	assert.Contains(t, body, `hx-get="/reports/activity"`)
	assert.Contains(t, body, `hx-trigger="every 10s`)
}

func TestReportsHandler_Activity_LiveStateOverridesDB(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	svc := &mockReportService{
		jobs: []reports.GenerationJob{
			{ID: uuid.New(), TenantID: "t_test", TaskID: "task-1", ReportType: "on_demand", Status: reports.GenerationJobQueued, EnqueuedAt: time.Now()},
		},
	}
	insp := &mockJobInspector{states: map[string]queue.JobState{
		"task-1": {ID: "task-1", Status: queue.JobRunning},
	}}
	handler := NewReportsHandler(svc, &mockJobQueue{}, insp)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/activity", http.NoBody)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Activity(c)
	require.NoError(t, err)
	body := rec.Body.String()

	// Live queue state wins over the recorded DB status.
	assert.Contains(t, body, "Running")
	assert.NotContains(t, body, "Queued")
	// Fragment only — no page shell.
	assert.NotContains(t, body, "<html")
}

func TestReportsHandler_Activity_FailedJobShowsError(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	svc := &mockReportService{
		jobs: []reports.GenerationJob{
			{ID: uuid.New(), TenantID: "t_test", TaskID: "task-1", ReportType: "monthly", Status: reports.GenerationJobQueued, EnqueuedAt: time.Now()},
		},
	}
	insp := &mockJobInspector{states: map[string]queue.JobState{
		"task-1": {ID: "task-1", Status: queue.JobFailed, Error: "archived: llm quota exceeded"},
	}}
	handler := NewReportsHandler(svc, &mockJobQueue{}, insp)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/activity", http.NoBody)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Activity(c)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "archived: llm quota exceeded")
}

func TestReportsHandler_Activity_InspectorErrorFallsBackToDB(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	svc := &mockReportService{
		jobs: []reports.GenerationJob{
			{ID: uuid.New(), TenantID: "t_test", TaskID: "task-1", ReportType: "daily", Status: reports.GenerationJobSucceeded, EnqueuedAt: time.Now()},
		},
	}
	// Inspector unavailable — the recorded DB outcome is shown.
	handler := NewReportsHandler(svc, &mockJobQueue{}, &mockJobInspector{err: assert.AnError})

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/activity", http.NoBody)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Activity(c)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "Succeeded")
}

func TestReportsHandler_Activity_Empty(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	handler := NewReportsHandler(&mockReportService{}, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/activity", http.NoBody)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Activity(c)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "No generation activity yet")
}

func TestReportsHandler_Activity_ServiceError(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	svc := &mockReportService{jobsErr: assert.AnError}
	handler := NewReportsHandler(svc, &mockJobQueue{}, nil)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports/activity", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Activity(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "No generation activity yet")
}
