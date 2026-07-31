package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Auth-specific mocks ---

type authMockService struct {
	registerUser domain.User
	registerErr  error
	loginToken   string
	loginErr     error
	claims       *domain.AuthClaims
	validateErr  error
}

func (m *authMockService) Register(_ context.Context, _, _ string) (domain.User, error) {
	return m.registerUser, m.registerErr
}

func (m *authMockService) Login(_ context.Context, _, _ string) (string, error) {
	return m.loginToken, m.loginErr
}

func (m *authMockService) ValidateToken(_ context.Context, _ string) (*domain.AuthClaims, error) {
	return m.claims, m.validateErr
}

type tenantMockService struct {
	tenants    []domain.Tenant
	listErr    error
	membership map[string]bool // "userID:tenantID" → bool
	memberErr  error
	tenantMap  map[domain.TenantID]domain.Tenant
}

func newTenantMockService() *tenantMockService {
	return &tenantMockService{
		membership: make(map[string]bool),
		tenantMap:  make(map[domain.TenantID]domain.Tenant),
	}
}

func (m *tenantMockService) Create(_ context.Context, _ uuid.UUID, _ string) (domain.Tenant, error) {
	return domain.Tenant{}, nil
}

func (m *tenantMockService) AddMember(_ context.Context, _ domain.TenantID, _ uuid.UUID, _ domain.Role) (domain.TenantMembership, error) {
	return domain.TenantMembership{}, nil
}

func (m *tenantMockService) ListByUser(_ context.Context, _ uuid.UUID) ([]domain.Tenant, error) {
	return m.tenants, m.listErr
}

func (m *tenantMockService) GetByID(_ context.Context, id domain.TenantID) (domain.Tenant, error) {
	t, ok := m.tenantMap[id]
	if !ok {
		return domain.Tenant{}, tenant.ErrTenantNotFound
	}
	return t, nil
}

func (m *tenantMockService) IsMember(_ context.Context, tenantID domain.TenantID, userID uuid.UUID) (bool, error) {
	if m.memberErr != nil {
		return false, m.memberErr
	}
	key := userID.String() + ":" + string(tenantID)
	return m.membership[key], nil
}

// Compile-time checks.
var _ auth.AuthService = (*authMockService)(nil)
var _ tenant.TenantService = (*tenantMockService)(nil)

// --- Helpers ---

func setupEcho() *echo.Echo {
	e := echo.New()
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))
	return e
}

func postForm(e *echo.Echo, path string, values url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func getCSRFCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookieName {
			return c
		}
	}
	return nil
}

func extractCSRFToken(rec *httptest.ResponseRecorder) string {
	c := getCSRFCookie(rec)
	if c == nil {
		return ""
	}
	return c.Value
}

// getCSRFAndCookies performs a GET to obtain a CSRF token and returns it
// along with all cookies from the response.
func getCSRFAndCookies(e *echo.Echo, path string) (string, []*http.Cookie) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	token := extractCSRFToken(rec)
	return token, rec.Result().Cookies()
}

// --- Login tests ---

func TestLoginSubmit_Success(t *testing.T) {
	svc := &authMockService{loginToken: "valid-jwt-token"}
	ts := newTenantMockService()
	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/login", h.loginSubmit)

	// Get CSRF token first.
	csrfToken, cookies := getCSRFAndCookies(e, "/login")

	form := url.Values{
		"email":    {"alice@example.com"},
		"password": {"securepass"},
		"_csrf":    {csrfToken},
	}
	rec := postForm(e, "/login", form, cookies...)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/select-tenant", rec.Header().Get("Location"))

	// JWT cookie should be set.
	var jwtCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == jwtCookieName {
			jwtCookie = c
		}
	}
	require.NotNil(t, jwtCookie)
	assert.Equal(t, "valid-jwt-token", jwtCookie.Value)
	assert.True(t, jwtCookie.HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, jwtCookie.SameSite)
}

func TestLoginSubmit_HTMX_Success(t *testing.T) {
	svc := &authMockService{loginToken: "valid-jwt-token"}
	ts := newTenantMockService()
	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/login", h.loginSubmit)

	csrfToken, cookies := getCSRFAndCookies(e, "/login")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/login",
		strings.NewReader(url.Values{
			"email":    {"alice@example.com"},
			"password": {"securepass"},
			"_csrf":    {csrfToken},
		}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/select-tenant", rec.Header().Get("HX-Redirect"))
}

func TestLoginSubmit_Failure(t *testing.T) {
	svc := &authMockService{loginErr: auth.ErrInvalidCredentials}
	ts := newTenantMockService()
	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/login", h.loginSubmit)

	csrfToken, cookies := getCSRFAndCookies(e, "/login")

	form := url.Values{
		"email":    {"bad@example.com"},
		"password": {"wrong"},
		"_csrf":    {csrfToken},
	}
	rec := postForm(e, "/login", form, cookies...)

	// Non-HTMX: redirects back to /login with flash.
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

func TestLoginSubmit_HTMX_Failure(t *testing.T) {
	svc := &authMockService{loginErr: auth.ErrInvalidCredentials}
	ts := newTenantMockService()
	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/login", h.loginSubmit)

	csrfToken, cookies := getCSRFAndCookies(e, "/login")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/login",
		strings.NewReader(url.Values{
			"email":    {"bad@example.com"},
			"password": {"wrong"},
			"_csrf":    {csrfToken},
		}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid email or password")
}

// --- Register tests ---

func TestRegisterSubmit_Success(t *testing.T) {
	svc := &authMockService{
		registerUser: domain.User{ID: uuid.New(), Email: "new@example.com"},
		loginToken:   "new-jwt-token",
	}
	ts := newTenantMockService()
	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/register", h.registerSubmit)

	csrfToken, cookies := getCSRFAndCookies(e, "/register")

	form := url.Values{
		"email":           {"new@example.com"},
		"password":        {"securepass"},
		"confirmPassword": {"securepass"},
		"_csrf":           {csrfToken},
	}
	rec := postForm(e, "/register", form, cookies...)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/select-tenant", rec.Header().Get("Location"))

	var jwtCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == jwtCookieName {
			jwtCookie = c
		}
	}
	require.NotNil(t, jwtCookie)
	assert.Equal(t, "new-jwt-token", jwtCookie.Value)
}

func TestRegisterSubmit_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		regErr     error
		wantMsg    string
	}{
		{"user exists", auth.ErrUserExists, "already exists"},
		{"invalid email", auth.ErrInvalidEmail, "valid email"},
		{"weak password", auth.ErrWeakPassword, "at least 8 characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &authMockService{registerErr: tt.regErr}
			ts := newTenantMockService()
			h := NewAuthHandler(svc, ts, false)

			e := setupEcho()
			e.POST("/register", h.registerSubmit)

			csrfToken, cookies := getCSRFAndCookies(e, "/register")

			form := url.Values{
				"email":    {"test@example.com"},
				"password": {"password"},
				"_csrf":    {csrfToken},
			}

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/register",
				strings.NewReader(form.Encode()))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			req.Header.Set("HX-Request", "true")
			for _, c := range cookies {
				req.AddCookie(c)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantMsg)
		})
	}
}

// --- Logout tests ---

func TestLogout_ClearsCookies(t *testing.T) {
	svc := &authMockService{}
	ts := newTenantMockService()
	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/logout", h.logout)

	csrfToken, cookies := getCSRFAndCookies(e, "/login")

	form := url.Values{"_csrf": {csrfToken}}
	rec := postForm(e, "/logout", form, cookies...)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))

	// Both cookies should be cleared (MaxAge=-1).
	cleared := make(map[string]bool)
	for _, c := range rec.Result().Cookies() {
		if c.Name == jwtCookieName || c.Name == tenantCookieName {
			assert.Equal(t, -1, c.MaxAge, "cookie %s should be cleared", c.Name)
			cleared[c.Name] = true
		}
	}
	assert.True(t, cleared[jwtCookieName], "JWT cookie should be cleared")
	assert.True(t, cleared[tenantCookieName], "tenant cookie should be cleared")
}

func TestLogout_HTMX_ClearsCookies(t *testing.T) {
	svc := &authMockService{}
	ts := newTenantMockService()
	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/logout", h.logout)

	csrfToken, cookies := getCSRFAndCookies(e, "/login")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/logout",
		strings.NewReader(url.Values{"_csrf": {csrfToken}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("HX-Redirect"))
}

// --- Select tenant tests ---

func TestSelectTenantSubmit_Success(t *testing.T) {
	userID := uuid.New()
	tenantID := domain.TenantID("t_abc12345")

	svc := &authMockService{
		claims: &domain.AuthClaims{UserID: userID, Email: "alice@example.com"},
	}
	ts := newTenantMockService()
	ts.tenants = []domain.Tenant{{ID: tenantID, Name: "Acme Corp"}}
	ts.tenantMap[tenantID] = domain.Tenant{ID: tenantID, Name: "Acme Corp"}
	ts.membership[userID.String()+":"+string(tenantID)] = true

	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/select-tenant", func(c echo.Context) error {
		// Simulate auth middleware setting userID in context.
		ctx := auth.SetUserID(c.Request().Context(), userID)
		c.SetRequest(c.Request().WithContext(ctx))
		return h.selectTenantSubmit(c)
	})

	csrfToken, cookies := getCSRFAndCookies(e, "/login")

	form := url.Values{
		"tenant_id": {string(tenantID)},
		"_csrf":     {csrfToken},
	}
	rec := postForm(e, "/select-tenant", form, cookies...)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/dashboard", rec.Header().Get("Location"))

	var tenantCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == tenantCookieName {
			tenantCookie = c
		}
	}
	require.NotNil(t, tenantCookie)
	assert.Equal(t, string(tenantID), tenantCookie.Value)
}

func TestSelectTenantSubmit_NotMember(t *testing.T) {
	userID := uuid.New()
	tenantID := domain.TenantID("t_abc12345")

	svc := &authMockService{
		claims: &domain.AuthClaims{UserID: userID, Email: "alice@example.com"},
	}
	ts := newTenantMockService()
	ts.tenantMap[tenantID] = domain.Tenant{ID: tenantID, Name: "Acme Corp"}
	// No membership set — user is NOT a member.

	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/select-tenant", func(c echo.Context) error {
		ctx := auth.SetUserID(c.Request().Context(), userID)
		c.SetRequest(c.Request().WithContext(ctx))
		return h.selectTenantSubmit(c)
	})

	csrfToken, cookies := getCSRFAndCookies(e, "/login")

	form := url.Values{
		"tenant_id": {string(tenantID)},
		"_csrf":     {csrfToken},
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/select-tenant",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "not a member")
}

func TestSelectTenantSubmit_EmptyTenantID(t *testing.T) {
	userID := uuid.New()

	svc := &authMockService{}
	ts := newTenantMockService()
	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.POST("/select-tenant", func(c echo.Context) error {
		ctx := auth.SetUserID(c.Request().Context(), userID)
		c.SetRequest(c.Request().WithContext(ctx))
		return h.selectTenantSubmit(c)
	})

	csrfToken, cookies := getCSRFAndCookies(e, "/login")

	form := url.Values{
		"tenant_id": {""},
		"_csrf":     {csrfToken},
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/select-tenant",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Contains(t, rec.Body.String(), "select a workspace")
}

func TestSelectTenantPage_ListsUserTenants(t *testing.T) {
	userID := uuid.New()
	tenants := []domain.Tenant{
		{ID: "t_aaa", Name: "Acme Corp"},
		{ID: "t_bbb", Name: "Globex"},
	}

	svc := &authMockService{}
	ts := newTenantMockService()
	ts.tenants = tenants

	h := NewAuthHandler(svc, ts, false)

	e := setupEcho()
	e.GET("/select-tenant", func(c echo.Context) error {
		ctx := auth.SetUserID(c.Request().Context(), userID)
		c.SetRequest(c.Request().WithContext(ctx))
		return h.selectTenantPage(c)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/select-tenant", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Acme Corp")
	assert.Contains(t, body, "Globex")
	assert.Contains(t, body, "t_aaa")
	assert.Contains(t, body, "t_bbb")
}

// --- Integration-style test: full server route registration ---

func TestServer_RouteRegistration(t *testing.T) {
	svc := &authMockService{
		loginToken: "jwt-token",
		claims:     &domain.AuthClaims{UserID: uuid.New(), Email: "alice@example.com"},
	}
	ts := newTenantMockService()

	srv := NewServer(svc, ts)
	e := echo.New()
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))
	srv.Register(e.Group(""))

	// Verify public routes are accessible.
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/login"},
		{http.MethodGet, "/register"},
	}

	for _, r := range routes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), r.method, r.path, http.NoBody)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}
