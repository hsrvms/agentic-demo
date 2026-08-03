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
	svc := &mockService{
		createResult: DataSource{
			ID:         uuid.New(),
			Name:       "My Website",
			SourceType: SourceTypeWebsite,
			Status:     StatusInactive,
		},
	}
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(&mockService{}))
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
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(svc))
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
	handler := NewHandler(NewHandlerCore(&mockService{}))
	e := echo.New()
	g := e.Group("/api")
	handler.Register(g)
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