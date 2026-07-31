package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	csrfCookieName = "_csrf"
	csrfHeaderName = "X-CSRF-Token"
	csrfFieldName  = "_csrf"
	csrfTokenLen   = 32
)

// CSRFConfig configures CSRF protection.
type CSRFConfig struct {
	// CookiePath sets the cookie path. Default "/".
	CookiePath string
	// Secure sets the Secure flag on the cookie. Default false (set true in production).
	Secure bool
}

// DefaultCSRFConfig returns sensible defaults.
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		CookiePath: "/",
		Secure:     false,
	}
}

// CSRFMiddleware generates a CSRF token on GET requests and validates it
// on state-changing methods (POST, PUT, DELETE, PATCH).
//
// The token is stored in a SameSite=Lax cookie and must be echoed back
// via the X-CSRF-Token header or a _csrf form field.
func CSRFMiddleware(cfg CSRFConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			switch c.Request().Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				// Safe methods: issue/refresh token.
				token := getOrGenerateToken(c, cfg)
				ctx := SetCSRFToken(c.Request().Context(), token)
				c.SetRequest(c.Request().WithContext(ctx))
				return next(c)

			case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
				// State-changing: validate token.
				cookieToken := getCookieToken(c)
				if cookieToken == "" {
					return echo.NewHTTPError(http.StatusForbidden, "missing CSRF cookie")
				}

				requestToken := extractRequestToken(c)
				if requestToken == "" {
					return echo.NewHTTPError(http.StatusForbidden, "missing CSRF token")
				}

				if !secureCompare(cookieToken, requestToken) {
					return echo.NewHTTPError(http.StatusForbidden, "invalid CSRF token")
				}

				// Set token in context for downstream use.
				ctx := SetCSRFToken(c.Request().Context(), cookieToken)
				c.SetRequest(c.Request().WithContext(ctx))
				return next(c)

			default:
				return next(c)
			}
		}
	}
}

// getOrGenerateToken reads an existing CSRF cookie or generates a new one.
func getOrGenerateToken(c echo.Context, cfg CSRFConfig) string {
	if token := getCookieToken(c); token != "" {
		return token
	}

	token := generateToken()
	c.SetCookie(&http.Cookie{ //nolint:gosec // JS needs to read CSRF cookie for AJAX; Secure is configurable.
		Name:     csrfCookieName,
		Value:    token,
		Path:     cfg.CookiePath,
		HttpOnly: false, // JS needs to read it for AJAX
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.Secure,
	})
	return token
}

func getCookieToken(c echo.Context) string {
	cookie, err := c.Cookie(csrfCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func extractRequestToken(c echo.Context) string {
	// Check header first (for HTMX/AJAX).
	if token := c.Request().Header.Get(csrfHeaderName); token != "" {
		return token
	}
	// Check form field.
	if token := c.FormValue(csrfFieldName); token != "" {
		return token
	}
	return ""
}

func generateToken() string {
	b := make([]byte, csrfTokenLen)
	if _, err := rand.Read(b); err != nil {
		panic("csrf: failed to generate token: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(a)), []byte(strings.TrimSpace(b))) == 1
}
