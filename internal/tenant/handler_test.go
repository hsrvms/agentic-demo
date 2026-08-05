package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Create(t *testing.T) {
	svc := &stubTenantService{
		createResult: domain.Tenant{
			ID:     "t_test123",
			Name:   "My Tenant",
			Status: domain.TenantActive,
		},
	}
	handler := NewHandler(svc)
	e := echo.New()

	userID := uuid.New()
	ctx := domain.SetUserID(context.Background(), userID)

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/tenants",
		strings.NewReader(`{"name": "My Tenant"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.Create(c))
	assert.Equal(t, http.StatusCreated, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "t_test123", body["id"])
	assert.Equal(t, "My Tenant", body["name"])
	assert.Equal(t, "active", body["status"])
}

func TestHandler_Create_InvalidName(t *testing.T) {
	svc := &stubTenantService{createErr: ErrInvalidName}
	handler := NewHandler(svc)
	e := echo.New()

	userID := uuid.New()
	ctx := domain.SetUserID(context.Background(), userID)

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/tenants",
		strings.NewReader(`{"name": ""}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Create(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestHandler_Create_BadRequest(t *testing.T) {
	handler := NewHandler(&stubTenantService{})
	e := echo.New()

	userID := uuid.New()
	ctx := domain.SetUserID(context.Background(), userID)

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/tenants",
		strings.NewReader(`not json`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Create(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestHandler_List(t *testing.T) {
	svc := &stubTenantService{
		listResult: []domain.Tenant{
			{ID: "t_abc123", Name: "Tenant A", Status: domain.TenantActive},
			{ID: "t_def456", Name: "Tenant B", Status: domain.TenantSuspended},
		},
	}
	handler := NewHandler(svc)
	e := echo.New()

	userID := uuid.New()
	ctx := domain.SetUserID(context.Background(), userID)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/tenants", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.List(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body []map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Len(t, body, 2)
	assert.Equal(t, "t_abc123", body[0]["id"])
	assert.Equal(t, "Tenant A", body[0]["name"])
	assert.Equal(t, "active", body[0]["status"])
	assert.Equal(t, "t_def456", body[1]["id"])
	assert.Equal(t, "suspended", body[1]["status"])
}

func TestHandler_List_Empty(t *testing.T) {
	svc := &stubTenantService{}
	handler := NewHandler(svc)
	e := echo.New()

	userID := uuid.New()
	ctx := domain.SetUserID(context.Background(), userID)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/tenants", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.List(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body []map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Len(t, body, 0)
}

func TestHandler_Register_Routes(t *testing.T) {
	handler := NewHandler(&stubTenantService{})
	e := echo.New()
	g := e.Group("/api")
	handler.Register(g)

	routes := e.Routes()
	methods := make(map[string][]string)
	for _, r := range routes {
		methods[r.Path] = append(methods[r.Path], r.Method)
	}
	assert.Contains(t, methods["/api/tenants"], "POST")
	assert.Contains(t, methods["/api/tenants"], "GET")
	assert.Contains(t, methods["/api/tenants/:id"], "DELETE")
}

func TestHandler_Delete(t *testing.T) {
	handler := NewHandler(&stubTenantService{isAdmin: true})
	e := echo.New()

	userID := uuid.New()
	ctx := domain.SetUserID(context.Background(), userID)
	req := httptest.NewRequestWithContext(ctx, http.MethodDelete, "/api/tenants/t_abc123", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("t_abc123")

	require.NoError(t, handler.Delete(c))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_Delete_NotAdmin(t *testing.T) {
	handler := NewHandler(&stubTenantService{isAdmin: false})
	e := echo.New()

	userID := uuid.New()
	ctx := domain.SetUserID(context.Background(), userID)
	req := httptest.NewRequestWithContext(ctx, http.MethodDelete, "/api/tenants/t_abc123", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("t_abc123")

	err := handler.Delete(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusForbidden, httpErr.Code)
}

func TestHandler_Delete_NotFound(t *testing.T) {
	handler := NewHandler(&stubTenantService{isAdmin: true, deleteErr: ErrTenantNotFound})
	e := echo.New()

	userID := uuid.New()
	ctx := domain.SetUserID(context.Background(), userID)
	req := httptest.NewRequestWithContext(ctx, http.MethodDelete, "/api/tenants/t_missing", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("t_missing")

	err := handler.Delete(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

// --- stub ---

type stubTenantService struct {
	createResult domain.Tenant
	createErr    error
	listResult   []domain.Tenant
	listErr      error
	deleteErr    error
	isAdmin      bool
}

func (s *stubTenantService) Create(ctx context.Context, ownerID uuid.UUID, name string) (domain.Tenant, error) {
	if s.createErr != nil {
		return domain.Tenant{}, s.createErr
	}
	return s.createResult, nil
}

func (s *stubTenantService) AddMember(ctx context.Context, tenantID domain.TenantID, userID uuid.UUID, role domain.Role) (domain.TenantMembership, error) {
	return domain.TenantMembership{}, nil
}

func (s *stubTenantService) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Tenant, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult == nil {
		return []domain.Tenant{}, nil
	}
	return s.listResult, nil
}

func (s *stubTenantService) GetByID(ctx context.Context, tenantID domain.TenantID) (domain.Tenant, error) {
	return domain.Tenant{}, nil
}

func (s *stubTenantService) IsMember(ctx context.Context, tenantID domain.TenantID, userID uuid.UUID) (bool, error) {
	return s.isAdmin, nil
}

func (s *stubTenantService) IsAdmin(ctx context.Context, tenantID domain.TenantID, userID uuid.UUID) (bool, error) {
	return s.isAdmin, nil
}

func (s *stubTenantService) Delete(ctx context.Context, tenantID domain.TenantID) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return nil
}
