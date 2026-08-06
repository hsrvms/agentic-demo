package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// --- Mocks ---

type mockAuthService struct {
	claims *domain.AuthClaims
	err    error
}

func (m *mockAuthService) Register(_ context.Context, _, _ string) (domain.User, error) {
	return domain.User{}, nil
}

func (m *mockAuthService) Login(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (m *mockAuthService) ValidateToken(_ context.Context, _ string) (*domain.AuthClaims, error) {
	return m.claims, m.err
}

type mockTenantService struct {
	tenants map[domain.TenantID]domain.Tenant
	members map[string]bool // "userID:tenantID" → bool
	isAdmin bool
}

func newMockTenantService() *mockTenantService {
	return &mockTenantService{
		tenants: make(map[domain.TenantID]domain.Tenant),
		members: make(map[string]bool),
		isAdmin: true,
	}
}

func (m *mockTenantService) Create(_ context.Context, _ uuid.UUID, _ string) (domain.Tenant, error) {
	return domain.Tenant{}, nil
}

func (m *mockTenantService) AddMember(_ context.Context, _ domain.TenantID, _ uuid.UUID, _ domain.Role) (domain.TenantMembership, error) {
	return domain.TenantMembership{}, nil
}

func (m *mockTenantService) ListByUser(_ context.Context, _ uuid.UUID) ([]domain.Tenant, error) {
	return nil, nil
}

func (m *mockTenantService) GetByID(_ context.Context, id domain.TenantID) (domain.Tenant, error) {
	t, ok := m.tenants[id]
	if !ok {
		return domain.Tenant{}, tenant.ErrTenantNotFound
	}
	return t, nil
}

func (m *mockTenantService) IsMember(_ context.Context, tenantID domain.TenantID, userID uuid.UUID) (bool, error) {
	key := userID.String() + ":" + string(tenantID)
	return m.members[key], nil
}

func (m *mockTenantService) Delete(_ context.Context, _ domain.TenantID) error {
	return nil
}

func (m *mockTenantService) IsAdmin(_ context.Context, _ domain.TenantID, _ uuid.UUID) (bool, error) {
	return m.isAdmin, nil
}

// Compile-time checks.
var _ auth.AuthService = (*mockAuthService)(nil)
var _ tenant.TenantService = (*mockTenantService)(nil)

// --- Tests ---

func TestCookieAuthMiddleware_CookieSuccess(t *testing.T) {
	userID := uuid.New()
	svc := &mockAuthService{
		claims: &domain.AuthClaims{UserID: userID, Email: "alice@example.com"},
	}

	e := echo.New()
	e.Use(CookieAuthMiddleware(svc))
	e.GET("/test", func(c echo.Context) error {
		gotID := auth.GetUserID(c.Request().Context())
		gotEmail := GetUserEmail(c.Request().Context())
		return c.JSON(http.StatusOK, map[string]string{
			"user_id": gotID.String(),
			"email":   gotEmail,
		})
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	req.AddCookie(&http.Cookie{Name: jwtCookieName, Value: "valid-token"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	assert.Equal(t, userID.String(), body["user_id"])
	assert.Equal(t, "alice@example.com", body["email"])
}

func TestCookieAuthMiddleware_CookieInvalid(t *testing.T) {
	svc := &mockAuthService{
		err: auth.ErrInvalidToken,
	}

	e := echo.New()
	e.Use(CookieAuthMiddleware(svc))
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	req.AddCookie(&http.Cookie{Name: jwtCookieName, Value: "bad-token"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCookieAuthMiddleware_FallsBackToHeader(t *testing.T) {
	userID := uuid.New()
	svc := &mockAuthService{
		claims: &domain.AuthClaims{UserID: userID, Email: "bob@example.com"},
	}

	e := echo.New()
	e.Use(CookieAuthMiddleware(svc))
	e.GET("/test", func(c echo.Context) error {
		gotID := auth.GetUserID(c.Request().Context())
		return c.JSON(http.StatusOK, map[string]string{"user_id": gotID.String()})
	})

	// No cookie, but Authorization header present.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCookieTenantMiddleware_CookieSuccess(t *testing.T) {
	tenantID := domain.TenantID("t_abc12345")
	userID := uuid.New()

	ts := newMockTenantService()
	ts.tenants[tenantID] = domain.Tenant{ID: tenantID, Name: "Acme Corp"}
	ts.members[userID.String()+":"+string(tenantID)] = true

	e := echo.New()
	// Must set userID in context first (simulating auth middleware).
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := auth.SetUserID(c.Request().Context(), userID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(CookieTenantMiddleware(ts))
	e.GET("/test", func(c echo.Context) error {
		gotTenant := auth.GetTenantID(c.Request().Context())
		gotName := GetTenantName(c.Request().Context())
		return c.JSON(http.StatusOK, map[string]string{
			"tenant_id": string(gotTenant),
			"name":      gotName,
		})
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	req.AddCookie(&http.Cookie{Name: tenantCookieName, Value: string(tenantID)})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	assert.Equal(t, string(tenantID), body["tenant_id"])
	assert.Equal(t, "Acme Corp", body["name"])
}

func TestCookieTenantMiddleware_NotMember(t *testing.T) {
	tenantID := domain.TenantID("t_abc12345")
	userID := uuid.New()

	ts := newMockTenantService()
	ts.tenants[tenantID] = domain.Tenant{ID: tenantID, Name: "Acme Corp"}
	// No membership added.

	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := auth.SetUserID(c.Request().Context(), userID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(CookieTenantMiddleware(ts))
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	req.AddCookie(&http.Cookie{Name: tenantCookieName, Value: string(tenantID)})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
