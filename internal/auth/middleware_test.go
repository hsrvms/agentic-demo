package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// mockAuthService implements AuthService for middleware tests.
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
	if m.err != nil {
		return nil, m.err
	}
	return m.claims, nil
}

// mockTenantService implements tenant.TenantService for middleware tests.
type mockTenantService struct {
	tenant    domain.Tenant
	isMember  bool
	tenantErr error
	memberErr error
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

func (m *mockTenantService) GetByID(_ context.Context, _ domain.TenantID) (domain.Tenant, error) {
	if m.tenantErr != nil {
		return domain.Tenant{}, m.tenantErr
	}
	return m.tenant, nil
}

func (m *mockTenantService) IsMember(_ context.Context, _ domain.TenantID, _ uuid.UUID) (bool, error) {
	if m.memberErr != nil {
		return false, m.memberErr
	}
	return m.isMember, nil
}

func (m *mockTenantService) Delete(_ context.Context, _ domain.TenantID) error {
	return nil
}

func (m *mockTenantService) IsAdmin(_ context.Context, _ domain.TenantID, _ uuid.UUID) (bool, error) {
	return true, nil
}

// Verify mockTenantService implements the interface.
var _ tenant.TenantService = (*mockTenantService)(nil)

func newTestContext() (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// --- AuthMiddleware tests ---

func TestAuthMiddleware_ValidToken(t *testing.T) {
	userID := uuid.New()
	auth := &mockAuthService{
		claims: &domain.AuthClaims{UserID: userID, Email: "alice@example.com"},
	}

	middleware := AuthMiddleware(auth)
	var gotUserID uuid.UUID

	handler := middleware(func(c echo.Context) error {
		gotUserID = GetUserID(c.Request().Context())
		return c.String(http.StatusOK, "ok")
	})

	c, rec := newTestContext()
	c.Request().Header.Set("Authorization", "Bearer valid-token")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if gotUserID != userID {
		t.Errorf("UserID = %v, want %v", gotUserID, userID)
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	auth := &mockAuthService{}
	middleware := AuthMiddleware(auth)
	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	c, _ := newTestContext()
	err := handler(c)
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if he.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want %d", he.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	auth := &mockAuthService{err: ErrInvalidToken}
	middleware := AuthMiddleware(auth)
	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	c, _ := newTestContext()
	c.Request().Header.Set("Authorization", "Bearer bad-token")

	err := handler(c)
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if he.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want %d", he.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_InvalidFormat(t *testing.T) {
	auth := &mockAuthService{}
	middleware := AuthMiddleware(auth)
	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	c, _ := newTestContext()
	c.Request().Header.Set("Authorization", "NotBearer token")

	err := handler(c)
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if he.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want %d", he.Code, http.StatusUnauthorized)
	}
}

// --- TenantMiddleware tests ---

func TestTenantMiddleware_Valid(t *testing.T) {
	userID := uuid.New()
	tenantSvc := &mockTenantService{
		tenant:   domain.Tenant{ID: "t-abc", Name: "Acme", Status: domain.TenantActive},
		isMember: true,
	}

	auth := AuthMiddleware(&mockAuthService{
		claims: &domain.AuthClaims{UserID: userID, Email: "alice@example.com"},
	})
	tenantMw := TenantMiddleware(tenantSvc)

	var gotTenantID domain.TenantID
	var gotUserID uuid.UUID

	handler := auth(tenantMw(func(c echo.Context) error {
		gotTenantID = GetTenantID(c.Request().Context())
		gotUserID = GetUserID(c.Request().Context())
		return c.String(http.StatusOK, "ok")
	}))

	c, rec := newTestContext()
	c.Request().Header.Set("Authorization", "Bearer valid-token")
	c.Request().Header.Set("X-Tenant-ID", "t-abc")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if gotTenantID != "t-abc" {
		t.Errorf("TenantID = %q, want %q", gotTenantID, "t-abc")
	}
	if gotUserID != userID {
		t.Errorf("UserID = %v, want %v", gotUserID, userID)
	}
}

func TestTenantMiddleware_MissingHeader(t *testing.T) {
	tenantSvc := &mockTenantService{}
	auth := AuthMiddleware(&mockAuthService{
		claims: &domain.AuthClaims{UserID: uuid.New(), Email: "alice@example.com"},
	})
	tenantMw := TenantMiddleware(tenantSvc)

	handler := auth(tenantMw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}))

	c, _ := newTestContext()
	c.Request().Header.Set("Authorization", "Bearer valid-token")

	err := handler(c)
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if he.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want %d", he.Code, http.StatusBadRequest)
	}
}

func TestTenantMiddleware_TenantNotFound(t *testing.T) {
	tenantSvc := &mockTenantService{tenantErr: tenant.ErrTenantNotFound}
	auth := AuthMiddleware(&mockAuthService{
		claims: &domain.AuthClaims{UserID: uuid.New(), Email: "alice@example.com"},
	})
	tenantMw := TenantMiddleware(tenantSvc)

	handler := auth(tenantMw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}))

	c, _ := newTestContext()
	c.Request().Header.Set("Authorization", "Bearer valid-token")
	c.Request().Header.Set("X-Tenant-ID", "t-nonexistent")

	err := handler(c)
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if he.Code != http.StatusNotFound {
		t.Errorf("code = %d, want %d", he.Code, http.StatusNotFound)
	}
}

func TestTenantMiddleware_NotMember(t *testing.T) {
	tenantSvc := &mockTenantService{
		tenant:   domain.Tenant{ID: "t-abc", Name: "Acme", Status: domain.TenantActive},
		isMember: false,
	}
	auth := AuthMiddleware(&mockAuthService{
		claims: &domain.AuthClaims{UserID: uuid.New(), Email: "alice@example.com"},
	})
	tenantMw := TenantMiddleware(tenantSvc)

	handler := auth(tenantMw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}))

	c, _ := newTestContext()
	c.Request().Header.Set("Authorization", "Bearer valid-token")
	c.Request().Header.Set("X-Tenant-ID", "t-abc")

	err := handler(c)
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if he.Code != http.StatusForbidden {
		t.Errorf("code = %d, want %d", he.Code, http.StatusForbidden)
	}
}

// --- JWTMiddleware (combined) tests ---

func TestJWTMiddleware_ValidToken(t *testing.T) {
	userID := uuid.New()
	auth := &mockAuthService{
		claims: &domain.AuthClaims{UserID: userID, Email: "alice@example.com"},
	}
	tenantSvc := &mockTenantService{
		tenant:   domain.Tenant{ID: "t-abc", Name: "Acme", Status: domain.TenantActive},
		isMember: true,
	}

	middleware := JWTMiddleware(auth, tenantSvc)
	var gotTenantID domain.TenantID
	var gotUserID uuid.UUID

	handler := middleware(func(c echo.Context) error {
		gotTenantID = GetTenantID(c.Request().Context())
		gotUserID = GetUserID(c.Request().Context())
		return c.String(http.StatusOK, "ok")
	})

	c, rec := newTestContext()
	c.Request().Header.Set("Authorization", "Bearer valid-token")
	c.Request().Header.Set("X-Tenant-ID", "t-abc")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if gotTenantID != "t-abc" {
		t.Errorf("TenantID = %q, want %q", gotTenantID, "t-abc")
	}
	if gotUserID != userID {
		t.Errorf("UserID = %v, want %v", gotUserID, userID)
	}
}

func TestJWTMiddleware_InvalidToken(t *testing.T) {
	auth := &mockAuthService{err: ErrInvalidToken}
	tenantSvc := &mockTenantService{}

	middleware := JWTMiddleware(auth, tenantSvc)
	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	c, _ := newTestContext()
	c.Request().Header.Set("Authorization", "Bearer bad-token")
	c.Request().Header.Set("X-Tenant-ID", "t-abc")

	err := handler(c)
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if he.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want %d", he.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_NotMember(t *testing.T) {
	auth := &mockAuthService{
		claims: &domain.AuthClaims{UserID: uuid.New(), Email: "alice@example.com"},
	}
	tenantSvc := &mockTenantService{
		tenant:   domain.Tenant{ID: "t-abc", Name: "Acme", Status: domain.TenantActive},
		isMember: false,
	}

	middleware := JWTMiddleware(auth, tenantSvc)
	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	c, _ := newTestContext()
	c.Request().Header.Set("Authorization", "Bearer valid-token")
	c.Request().Header.Set("X-Tenant-ID", "t-abc")

	err := handler(c)
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if he.Code != http.StatusForbidden {
		t.Errorf("code = %d, want %d", he.Code, http.StatusForbidden)
	}
}
