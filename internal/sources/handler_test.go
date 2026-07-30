package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Create(t *testing.T) {
	svc := &mockService{}
	handler := NewHandler(svc)
	e := setupEcho()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/sources", strings.NewReader(`{
		"source_type": "website",
		"name": "My Website",
		"config": {"url": "https://example.com"},
		"credentials": {"api_key": "secret"}
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("tenant_id", "tenant-1")

	require.NoError(t, handler.Create(c))
	assert.Equal(t, http.StatusCreated, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "My Website", body["name"])
	assert.Equal(t, "website", body["source_type"])
}

func TestHandler_Create_BadRequest(t *testing.T) {
	svc := &mockService{createErr: ErrInvalidName}
	handler := NewHandler(svc)
	e := setupEcho()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/sources", strings.NewReader(`{"source_type": "website"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("tenant_id", "tenant-1")

	err := handler.Create(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestHandler_List(t *testing.T) {
	svc := &mockService{
		listResult: DataSourcePage{
			Sources:    []DataSource{{ID: uuid.New(), Name: "Source 1", TenantID: "tenant-1"}},
			TotalCount: 1,
			Page:       1,
			PageSize:   20,
		},
	}
	handler := NewHandler(svc)
	e := setupEcho()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/sources?page=1&limit=20", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("tenant_id", "tenant-1")

	require.NoError(t, handler.List(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["total_count"])
}

func TestHandler_Get(t *testing.T) {
	id := uuid.New()
	svc := &mockService{
		getResult: DataSource{
			ID:         id,
			TenantID:   "tenant-1",
			SourceType: SourceTypeWebsite,
			Name:       "Test Source",
			Config:     json.RawMessage(`{"url": "https://example.com"}`),
			Status:     StatusActive,
		},
	}
	handler := NewHandler(svc)
	e := setupEcho()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/sources/"+id.String(), http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	require.NoError(t, handler.Get(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "Test Source", body["name"])
}

func TestHandler_Get_NotFound(t *testing.T) {
	svc := &mockService{getErr: ErrNotFound}
	handler := NewHandler(svc)
	e := setupEcho()
	id := uuid.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/sources/"+id.String(), http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Get(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

func TestHandler_Get_InvalidID(t *testing.T) {
	handler := NewHandler(&mockService{})
	e := setupEcho()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/sources/bad-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("bad-uuid")

	err := handler.Get(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestHandler_Update(t *testing.T) {
	id := uuid.New()
	svc := &mockService{
		updateResult: DataSource{
			ID:     id,
			Name:   "Updated",
			Status: StatusActive,
		},
	}
	handler := NewHandler(svc)
	e := setupEcho()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/api/sources/"+id.String(), strings.NewReader(`{
		"name": "Updated",
		"status": "active"
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	require.NoError(t, handler.Update(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "Updated", body["name"])
}

func TestHandler_Delete(t *testing.T) {
	svc := &mockService{}
	handler := NewHandler(svc)
	e := setupEcho()
	id := uuid.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/sources/"+id.String(), http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	require.NoError(t, handler.Delete(c))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_TestConnection(t *testing.T) {
	svc := &mockService{
		testResult: ConnectionTestResult{Success: true, Message: "ok", Latency: "5ms"},
	}
	handler := NewHandler(svc)
	e := setupEcho()
	id := uuid.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/sources/"+id.String()+"/test", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	require.NoError(t, handler.TestConnection(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["success"])
}

func TestHandler_TestConnection_Failed(t *testing.T) {
	svc := &mockService{
		testResult: ConnectionTestResult{Success: false, Message: "connection refused"},
	}
	handler := NewHandler(svc)
	e := setupEcho()
	id := uuid.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/sources/"+id.String()+"/test", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	require.NoError(t, handler.TestConnection(c))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHandler_Sync(t *testing.T) {
	svc := &mockService{}
	handler := NewHandler(svc)
	e := setupEcho()
	id := uuid.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/sources/"+id.String()+"/sync", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	require.NoError(t, handler.Sync(c))
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestHandler_Register(t *testing.T) {
	handler := NewHandler(&mockService{})
	e := echo.New()
	g := e.Group("/api")
	handler.Register(g)
	// Verify routes are registered by checking the router.
	routes := e.Routes()
	paths := make(map[string]bool)
	for _, r := range routes {
		paths[r.Path] = true
	}
	assert.True(t, paths["/api/sources"])
	assert.True(t, paths["/api/sources/:id"])
	assert.True(t, paths["/api/sources/:id/test"])
	assert.True(t, paths["/api/sources/:id/sync"])
}

// --- helpers ---

func setupEcho() *echo.Echo {
	e := echo.New()
	// Set up tenant context for tests.
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tenantID := c.Get("tenant_id")
			if tenantID != nil {
				ctx := auth.SetTenantID(c.Request().Context(), domain.TenantID(tenantID.(string)))
				c.SetRequest(c.Request().WithContext(ctx))
			}
			return next(c)
		}
	})
	return e
}

// --- mock service ---

type mockService struct {
	createResult DataSource
	createErr    error
	getResult    DataSource
	getErr       error
	listResult   DataSourcePage
	listErr      error
	updateResult DataSource
	updateErr    error
	deleteErr    error
	testResult   ConnectionTestResult
	testErr      error
	syncErr      error
}

func (m *mockService) Create(ctx context.Context, params *CreateDataSourceParams) (DataSource, error) {
	if m.createErr != nil {
		return DataSource{}, m.createErr
	}
	if m.createResult.ID == uuid.Nil {
		m.createResult.ID = uuid.New()
	}
	m.createResult.TenantID = params.TenantID
	m.createResult.SourceType = params.SourceType
	m.createResult.Name = params.Name
	return m.createResult, nil
}

func (m *mockService) GetByID(ctx context.Context, id uuid.UUID) (DataSource, error) {
	if m.getErr != nil {
		return DataSource{}, m.getErr
	}
	return m.getResult, nil
}

func (m *mockService) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (DataSourcePage, error) {
	if m.listErr != nil {
		return DataSourcePage{}, m.listErr
	}
	return m.listResult, nil
}

func (m *mockService) Update(ctx context.Context, id uuid.UUID, params UpdateDataSourceParams) (DataSource, error) {
	if m.updateErr != nil {
		return DataSource{}, m.updateErr
	}
	return m.updateResult, nil
}

func (m *mockService) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteErr
}

func (m *mockService) TestConnection(ctx context.Context, id uuid.UUID) (ConnectionTestResult, error) {
	if m.testErr != nil {
		return ConnectionTestResult{}, m.testErr
	}
	return m.testResult, nil
}

func (m *mockService) Sync(ctx context.Context, id uuid.UUID) error {
	return m.syncErr
}