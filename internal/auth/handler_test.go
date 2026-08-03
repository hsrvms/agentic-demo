package auth

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

func TestHandler_HandleRegister(t *testing.T) {
	svc := &stubAuthService{
		registerResult: domain.User{ID: uuid.New(), Email: "test@example.com"},
		loginResult:    "test-token",
	}
	handler := NewHandler(svc)
	e := echo.New()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email": "test@example.com", "password": "password123"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.HandleRegister(c))
	assert.Equal(t, http.StatusCreated, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "test@example.com", body["email"])
	assert.Equal(t, "test-token", body["token"])
	assert.NotEmpty(t, body["user_id"])
}

func TestHandler_HandleRegister_BadRequest(t *testing.T) {
	handler := NewHandler(&stubAuthService{})
	e := echo.New()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/register",
		strings.NewReader(`not json`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleRegister(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestHandler_HandleRegister_UserExists(t *testing.T) {
	svc := &stubAuthService{registerErr: ErrUserExists}
	handler := NewHandler(svc)
	e := echo.New()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email": "taken@example.com", "password": "password123"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleRegister(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusConflict, httpErr.Code)
}

func TestHandler_HandleRegister_WeakPassword(t *testing.T) {
	svc := &stubAuthService{registerErr: ErrWeakPassword}
	handler := NewHandler(svc)
	e := echo.New()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email": "test@example.com", "password": "short"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleRegister(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestHandler_HandleLogin(t *testing.T) {
	svc := &stubAuthService{loginResult: "test-token"}
	handler := NewHandler(svc)
	e := echo.New()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email": "test@example.com", "password": "password123"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.HandleLogin(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "test-token", body["token"])
}

func TestHandler_HandleLogin_InvalidCredentials(t *testing.T) {
	svc := &stubAuthService{loginErr: ErrInvalidCredentials}
	handler := NewHandler(svc)
	e := echo.New()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email": "test@example.com", "password": "wrong"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleLogin(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

func TestHandler_HandleLogin_BadRequest(t *testing.T) {
	handler := NewHandler(&stubAuthService{})
	e := echo.New()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/login",
		strings.NewReader(`not json`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleLogin(c)
	require.Error(t, err)
	var httpErr *echo.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestHandler_HandleMe(t *testing.T) {
	handler := NewHandler(&stubAuthService{})
	e := echo.New()

	userID := uuid.New()
	tenantID := domain.TenantID("t_test123")

	ctx := SetUserID(context.Background(), userID)
	ctx = SetTenantID(ctx, tenantID)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/auth/me", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.HandleMe(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, userID.String(), body["user_id"])
	assert.Equal(t, string(tenantID), body["tenant_id"])
}

func TestHandler_HandleMe_NoContext(t *testing.T) {
	handler := NewHandler(&stubAuthService{})
	e := echo.New()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/auth/me", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.HandleMe(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	// uuid.Nil serializes to "00000000-0000-0000-0000-000000000000"
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", body["user_id"])
	assert.Equal(t, "", body["tenant_id"])
}

func TestHandler_Register_Routes(t *testing.T) {
	handler := NewHandler(&stubAuthService{})
	e := echo.New()
	g := e.Group("/api")
	handler.Register(g)

	routes := e.Routes()
	paths := make(map[string]string)
	for _, r := range routes {
		paths[r.Path] = r.Method
	}
	assert.Equal(t, "POST", paths["/api/auth/register"])
	assert.Equal(t, "POST", paths["/api/auth/login"])
	assert.Equal(t, "GET", paths["/api/auth/me"])
}

// --- stub ---

type stubAuthService struct {
	registerResult domain.User
	registerErr    error
	loginResult    string
	loginErr       error
}

func (m *stubAuthService) Register(ctx context.Context, email, password string) (domain.User, error) {
	if m.registerErr != nil {
		return domain.User{}, m.registerErr
	}
	return m.registerResult, nil
}

func (m *stubAuthService) Login(ctx context.Context, email, password string) (string, error) {
	if m.loginErr != nil {
		return "", m.loginErr
	}
	return m.loginResult, nil
}

func (m *stubAuthService) ValidateToken(ctx context.Context, tokenString string) (*domain.AuthClaims, error) {
	return nil, nil
}