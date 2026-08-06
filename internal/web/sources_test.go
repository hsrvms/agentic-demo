package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/webui"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mockSourceService is defined in dashboard_test.go and reused here ---

// --- List tests ---

func TestSourcesHandler_List_RendersTable(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	lastSync := time.Now().Add(-2 * time.Hour)

	svc := &mockSourceService{
		page: sources.DataSourcePage{
			Sources: []sources.DataSource{
				{
					ID:         uuid.New(),
					Name:       "Website Alpha",
					SourceType: sources.SourceTypeWebsite,
					Status:     sources.StatusActive,
					LastSyncAt: &lastSync,
				},
				{
					ID:         uuid.New(),
					Name:       "CRM Beta",
					SourceType: sources.SourceTypeCRMHubSpot,
					Status:     sources.StatusInactive,
				},
			},
			TotalCount: 2,
			Page:       1,
			PageSize:   20,
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sources", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = SetCSRFToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Website Alpha")
	assert.Contains(t, body, "CRM Beta")
	assert.Contains(t, body, "Website")
	assert.Contains(t, body, "HubSpot CRM")
	assert.Contains(t, body, "active")
	assert.Contains(t, body, "inactive")
}

func TestSourcesHandler_List_EmptyState(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockSourceService{
		page: sources.DataSourcePage{
			Sources:    []sources.DataSource{},
			TotalCount: 0,
			Page:       1,
			PageSize:   20,
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sources", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = SetCSRFToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "No data sources yet")
}

func TestSourcesHandler_List_HTMXPagination(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockSourceService{
		page: sources.DataSourcePage{
			Sources: []sources.DataSource{
				{
					ID:         uuid.New(),
					Name:       "Page 2 Source",
					SourceType: sources.SourceTypeFileUpload,
					Status:     sources.StatusActive,
				},
			},
			TotalCount: 25,
			Page:       2,
			PageSize:   20,
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sources?page=2", http.NoBody)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = SetCSRFToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// HTMX should return just the rows fragment, not the full page.
	assert.Contains(t, body, "Page 2 Source")
	assert.NotContains(t, body, "No data sources yet")
	// Should not contain full page layout.
	assert.NotContains(t, body, "<!DOCTYPE")
}

func TestSourcesHandler_List_FlashesRendered(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockSourceService{
		page: sources.DataSourcePage{
			Sources:    []sources.DataSource{},
			TotalCount: 0,
			Page:       1,
			PageSize:   20,
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sources", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = SetCSRFToken(ctx, "test-csrf-token")
	ctx = SetFlashMessages(ctx, []webui.Flash{{Intent: "success", Message: "Source deleted"}})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "Source deleted")
}

// --- NewForm tests ---

func TestSourcesHandler_NewForm_RendersForm(t *testing.T) {
	handler := NewSourcesHandler(sources.NewHandlerCore(&mockSourceService{}))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sources/new", http.NoBody)
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.NewForm(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Add Data Source")
	assert.Contains(t, body, "test-csrf-token")
	assert.Contains(t, body, "File Upload")
	assert.Contains(t, body, "Website")
	assert.Contains(t, body, "HubSpot CRM")
	assert.Contains(t, body, "Salesforce CRM")
}

// --- Detail tests ---

func TestSourcesHandler_Detail_RendersSource(t *testing.T) {
	id := uuid.New()
	lastSync := time.Now().Add(-30 * time.Minute)

	svc := &mockSourceService{
		detailSource: &sources.DataSource{
			ID:         id,
			Name:       "My Website",
			SourceType: sources.SourceTypeWebsite,
			Status:     sources.StatusActive,
			Config:     json.RawMessage(`{"url":"https://example.com"}`),
			LastSyncAt: &lastSync,
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sources/"+id.String(), http.NoBody)
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Detail(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "My Website")
	assert.Contains(t, body, "Website")
	assert.Contains(t, body, "active")
	assert.Contains(t, body, "https://example.com")
}

func TestSourcesHandler_Detail_NotFound(t *testing.T) {
	svc := &mockSourceService{
		detailErr: sources.ErrNotFound,
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sources/bad-id", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("bad-id")

	err := handler.Detail(c)
	var he *echo.HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusBadRequest, he.Code) // bad UUID format
}

func TestSourcesHandler_Detail_InvalidUUID(t *testing.T) {
	svc := &mockSourceService{}
	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sources/not-a-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("not-a-uuid")

	err := handler.Detail(c)
	var he *echo.HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusBadRequest, he.Code)
}

// --- EditForm tests ---

func TestSourcesHandler_EditForm_RendersPrepopulatedForm(t *testing.T) {
	id := uuid.New()

	svc := &mockSourceService{
		detailSource: &sources.DataSource{
			ID:         id,
			Name:       "Edit Me",
			SourceType: sources.SourceTypeWebsite,
			Status:     sources.StatusActive,
			Config:     json.RawMessage(`{"url":"https://example.com"}`),
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sources/"+id.String()+"/edit", http.NoBody)
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.EditForm(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Edit Data Source")
	assert.Contains(t, body, "Edit Me")
	assert.Contains(t, body, "https://example.com")
}

// --- Create tests ---

func TestSourcesHandler_Create_HTMXRedirect(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockSourceService{
		createResult: sources.DataSource{
			ID:         uuid.New(),
			Name:       "New Source",
			SourceType: sources.SourceTypeWebsite,
			Status:     sources.StatusInactive,
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	form := url.Values{}
	form.Set("name", "New Source")
	form.Set("source_type", "website")
	form.Set("config_url", "https://example.com")
	form.Set("_csrf", "test-csrf-token")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = SetCSRFToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Create(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("HX-Redirect"))
}

func TestSourcesHandler_Create_FullPageRedirect(t *testing.T) {
	tenantID := domain.TenantID("t_test")
	newID := uuid.New()

	svc := &mockSourceService{
		createResult: sources.DataSource{
			ID:         newID,
			Name:       "New Source",
			SourceType: sources.SourceTypeWebsite,
			Status:     sources.StatusInactive,
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	form := url.Values{}
	form.Set("name", "New Source")
	form.Set("source_type", "website")
	form.Set("config_url", "https://example.com")
	form.Set("_csrf", "test-csrf-token")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = SetCSRFToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Create(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/sources/"+newID.String())
}

func TestSourcesHandler_Create_FileUploadPassesBytesAsFile(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockSourceService{
		createResult: sources.DataSource{
			ID:         uuid.New(),
			Name:       "Upload",
			SourceType: sources.SourceTypeFileUpload,
			Status:     sources.StatusInactive,
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.WriteField("name", "Upload"))
	require.NoError(t, mw.WriteField("source_type", "file_upload"))
	require.NoError(t, mw.WriteField("_csrf", "test-csrf-token"))
	part, err := mw.CreateFormFile("file", "notes.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("hello world"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources", &buf)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = SetCSRFToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	err = handler.Create(c)
	require.NoError(t, err)

	require.NotNil(t, svc.createdParams)
	// Bytes travel as File, not as credentials.
	assert.Equal(t, []byte("hello world"), svc.createdParams.File)
	assert.Empty(t, svc.createdParams.Credentials)

	var cfg map[string]string
	require.NoError(t, json.Unmarshal(svc.createdParams.Config, &cfg))
	assert.Equal(t, "notes.txt", cfg["filename"])
}

func TestSourcesHandler_Create_HTMXValidationError(t *testing.T) {
	tenantID := domain.TenantID("t_test")

	svc := &mockSourceService{
		createErr: sources.ErrInvalidName,
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	form := url.Values{}
	form.Set("name", "")
	form.Set("source_type", "website")
	form.Set("_csrf", "test-csrf-token")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), tenantID)
	ctx = SetCSRFToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Create(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "name")
}

// --- Update tests ---

func TestSourcesHandler_Update_HTMXRedirect(t *testing.T) {
	id := uuid.New()

	svc := &mockSourceService{
		updateResult: sources.DataSource{
			ID:   id,
			Name: "Updated",
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	form := url.Values{}
	form.Set("name", "Updated")
	form.Set("source_type", "website")
	form.Set("_csrf", "test-csrf-token")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/sources/"+id.String(), strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Update(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("HX-Redirect"))
}

// --- Delete tests ---

func TestSourcesHandler_Delete_HTMXReturnsNoContent(t *testing.T) {
	id := uuid.New()

	svc := &mockSourceService{}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/sources/"+id.String(), http.NoBody)
	req.Header.Set("HX-Request", "true")
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Delete(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSourcesHandler_Delete_FullPageRedirect(t *testing.T) {
	id := uuid.New()

	svc := &mockSourceService{}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/sources/"+id.String(), http.NoBody)
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Delete(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/sources", rec.Header().Get("Location"))
}

func TestSourcesHandler_Delete_NotFound(t *testing.T) {
	id := uuid.New()

	svc := &mockSourceService{
		deleteErr: sources.ErrNotFound,
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/sources/"+id.String(), http.NoBody)
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Delete(c)
	var he *echo.HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusNotFound, he.Code)
}

// --- TestConnection tests ---

func TestSourcesHandler_TestConnection_Success(t *testing.T) {
	id := uuid.New()

	svc := &mockSourceService{
		testResult: sources.ConnectionTestResult{
			Success: true,
			Message: "connection successful (HTTP 200)",
			Latency: "142ms",
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources/"+id.String()+"/test", http.NoBody)
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.TestConnection(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "connection successful")
	assert.Contains(t, body, "142ms")
}

func TestSourcesHandler_TestConnection_Failure(t *testing.T) {
	id := uuid.New()

	svc := &mockSourceService{
		testResult: sources.ConnectionTestResult{
			Success: false,
			Message: "connection refused",
			Latency: "5s",
		},
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources/"+id.String()+"/test", http.NoBody)
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.TestConnection(c)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "connection refused")
}

// --- Sync tests ---

func TestSourcesHandler_Sync_Triggered(t *testing.T) {
	id := uuid.New()

	svc := &mockSourceService{}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources/"+id.String()+"/sync", http.NoBody)
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Sync(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Sync job has been queued")
}

func TestSourcesHandler_Sync_Error(t *testing.T) {
	id := uuid.New()

	svc := &mockSourceService{
		syncErr: assert.AnError,
	}

	handler := NewSourcesHandler(sources.NewHandlerCore(svc))

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources/"+id.String()+"/sync", http.NoBody)
	ctx := SetCSRFToken(req.Context(), "test-csrf-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Sync(c)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "Failed to trigger sync")
}

// --- Helper function tests ---

func TestBuildConfigAndCreds(t *testing.T) {
	t.Run("website", func(t *testing.T) {
		e := echo.New()
		form := url.Values{}
		form.Set("config_url", "https://example.com")
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		c := e.NewContext(req, httptest.NewRecorder())

		config, creds, _, err := buildConfigAndCreds("website", c)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Nil(t, creds)

		var parsed map[string]string
		require.NoError(t, json.Unmarshal(config, &parsed))
		assert.Equal(t, "https://example.com", parsed["url"])
	})

	t.Run("crm", func(t *testing.T) {
		e := echo.New()
		form := url.Values{}
		form.Set("config_api_key", "test-key-123")
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		c := e.NewContext(req, httptest.NewRecorder())

		config, creds, _, err := buildConfigAndCreds("crm_hubspot", c)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, []byte("test-key-123"), creds)
	})

	t.Run("file_upload_no_file", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sources", http.NoBody)
		req.Header.Set(echo.HeaderContentType, echo.MIMEMultipartForm)
		c := e.NewContext(req, httptest.NewRecorder())

		_, _, _, err := buildConfigAndCreds("file_upload", c)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "file is required")
	})
}

func TestExtractConfigURL(t *testing.T) {
	assert.Equal(t, "https://example.com", extractConfigURL(json.RawMessage(`{"url":"https://example.com"}`)))
	assert.Equal(t, "", extractConfigURL(nil))
	assert.Equal(t, "", extractConfigURL(json.RawMessage(`{}`)))
	assert.Equal(t, "", extractConfigURL(json.RawMessage(`invalid`)))
}
