package web

import (
	"errors"
	"net/http"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/labstack/echo/v4"
)

// AuthHandler groups browser-based authentication handlers.
type AuthHandler struct {
	authService   auth.AuthService
	tenantService tenant.TenantService
	secureCookies bool
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(authService auth.AuthService, tenantService tenant.TenantService, secureCookies bool) *AuthHandler {
	return &AuthHandler{
		authService:   authService,
		tenantService: tenantService,
		secureCookies: secureCookies,
	}
}

// loginPage renders the login form.
func (h *AuthHandler) loginPage(c echo.Context) error {
	flashes := GetFlashMessages(c.Request().Context())
	csrf := GetCSRFToken(c.Request().Context())
	return Render(c, http.StatusOK, webpages.Login(csrf, flashes))
}

// loginSubmit processes the login form.
func (h *AuthHandler) loginSubmit(c echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")

	token, err := h.authService.Login(c.Request().Context(), email, password)
	if err != nil {
		return h.authError(c, "Invalid email or password")
	}

	h.setJWTCookie(c, token)

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/select-tenant")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/select-tenant")
}

// registerPage renders the registration form.
func (h *AuthHandler) registerPage(c echo.Context) error {
	flashes := GetFlashMessages(c.Request().Context())
	csrf := GetCSRFToken(c.Request().Context())
	return Render(c, http.StatusOK, webpages.Register(csrf, flashes))
}

// registerSubmit processes the registration form, then auto-logs-in.
func (h *AuthHandler) registerSubmit(c echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")

	_, err := h.authService.Register(c.Request().Context(), email, password)
	if err != nil {
		return h.registerError(c, err)
	}

	token, err := h.authService.Login(c.Request().Context(), email, password)
	if err != nil {
		return h.authError(c, "Account created but sign-in failed. Please sign in manually.")
	}

	h.setJWTCookie(c, token)

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/select-tenant")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/select-tenant")
}

// logout clears the JWT and tenant cookies, then redirects to login.
func (h *AuthHandler) logout(c echo.Context) error {
	h.clearCookies(c)

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/login")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/login")
}

// selectTenantPage renders the tenant selector.
func (h *AuthHandler) selectTenantPage(c echo.Context) error {
	userID := auth.GetUserID(c.Request().Context())
	flashes := GetFlashMessages(c.Request().Context())
	csrf := GetCSRFToken(c.Request().Context())

	tenants, err := h.tenantService.ListByUser(c.Request().Context(), userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load workspaces")
	}

	return Render(c, http.StatusOK, webpages.SelectTenant(tenants, csrf, flashes))
}

// selectTenantSubmit verifies membership and sets the tenant cookie.
func (h *AuthHandler) selectTenantSubmit(c echo.Context) error {
	tenantIDStr := c.FormValue("tenant_id")
	if tenantIDStr == "" {
		return h.authError(c, "Please select a workspace")
	}

	tenantID := domain.TenantID(tenantIDStr)
	userID := auth.GetUserID(c.Request().Context())

	isMember, err := h.tenantService.IsMember(c.Request().Context(), tenantID, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to verify membership")
	}
	if !isMember {
		return h.authError(c, "You are not a member of this workspace")
	}

	h.setTenantCookie(c, tenantIDStr)

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/dashboard")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/dashboard")
}

// --- helpers ---

func (h *AuthHandler) setJWTCookie(c echo.Context, token string) {
	c.SetCookie(&http.Cookie{ //nolint:gosec // Secure is configurable via secureCookies.
		Name:     jwtCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.secureCookies,
		MaxAge:   86400, // 24 hours
	})
}

func (h *AuthHandler) setTenantCookie(c echo.Context, tenantID string) {
	c.SetCookie(&http.Cookie{ //nolint:gosec // Tenant cookie is not security-critical.
		Name:     tenantCookieName,
		Value:    tenantID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.secureCookies,
		MaxAge:   86400,
	})
}

func (h *AuthHandler) clearCookies(c echo.Context) {
	c.SetCookie(&http.Cookie{ //nolint:gosec // Clearing cookies.
		Name:   jwtCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	c.SetCookie(&http.Cookie{ //nolint:gosec // Clearing cookies.
		Name:   tenantCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// createTenantSubmit processes the "create workspace" form from the
// select-tenant page. On success it sets the tenant cookie and redirects
// to /dashboard.
func (h *AuthHandler) createTenantSubmit(c echo.Context) error {
	name := c.FormValue("name")
	if name == "" {
		return h.authError(c, "Workspace name is required")
	}

	userID := auth.GetUserID(c.Request().Context())

	t, err := h.tenantService.Create(c.Request().Context(), userID, name)
	if err != nil {
		return h.authError(c, h.mapTenantError(err))
	}

	h.setTenantCookie(c, string(t.ID))

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/dashboard")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/dashboard")
}

// mapTenantError converts a tenant service error to a user-facing message.
func (h *AuthHandler) mapTenantError(err error) string {
	switch {
	case errors.Is(err, tenant.ErrInvalidName):
		return "Please enter a valid workspace name"
	case errors.Is(err, tenant.ErrAlreadyExists):
		return "A workspace with that name already exists"
	default:
		return "Failed to create workspace"
	}
}

// authError returns an error fragment for HTMX or redirects with flash.
func (h *AuthHandler) authError(c echo.Context, message string) error {
	if IsHTMX(c) {
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
		c.Response().Status = http.StatusOK
		_, err := c.Response().Write([]byte(`<div class="rounded-lg border border-intent-error/20 bg-intent-error/5 p-3 text-sm text-intent-error" role="alert">` + message + `</div>`))
		return err
	}
	setFlashCookie(c, webui.Flash{Intent: "error", Message: message})
	return c.Redirect(http.StatusSeeOther, c.Request().URL.Path)
}

// registerError maps a registration error to a user-facing message.
func (h *AuthHandler) registerError(c echo.Context, err error) error {
	message := "Registration failed"
	switch {
	case errors.Is(err, auth.ErrUserExists):
		message = "An account with that email already exists"
	case errors.Is(err, auth.ErrInvalidEmail):
		message = "Please enter a valid email address"
	case errors.Is(err, auth.ErrWeakPassword):
		message = "Password must be at least 8 characters"
	}
	return h.authError(c, message)
}
