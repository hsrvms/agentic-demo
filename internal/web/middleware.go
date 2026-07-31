package web

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/labstack/echo/v4"
)

const (
	jwtCookieName    = "jwt"
	tenantCookieName = "tenant_id"
)

// CookieAuthMiddleware extracts a JWT from an httpOnly cookie (instead of
// the Authorization header) and sets the user claims in the request context.
//
// If the cookie is missing, it falls back to the Authorization header for
// backward compatibility with API clients.
func CookieAuthMiddleware(authService auth.AuthService) echo.MiddlewareFunc {
	headerAuth := auth.AuthMiddleware(authService)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Try cookie first.
			cookie, err := c.Cookie(jwtCookieName)
			if err == nil && cookie.Value != "" {
				claims, err := authService.ValidateToken(c.Request().Context(), cookie.Value)
				if err != nil {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired session")
				}

				ctx := auth.SetUserID(c.Request().Context(), claims.UserID)
				ctx = SetUserEmail(ctx, claims.Email)
				c.SetRequest(c.Request().WithContext(ctx))
				return next(c)
			}

			// Fall back to header-based auth (for API clients).
			return headerAuth(next)(c)
		}
	}
}

// CookieTenantMiddleware extracts the tenant ID from an httpOnly cookie
// (instead of the X-Tenant-ID header) and verifies membership.
//
// If the cookie is missing, it falls back to the X-Tenant-ID header.
func CookieTenantMiddleware(tenantService tenant.TenantService) echo.MiddlewareFunc {
	headerTenant := auth.TenantMiddleware(tenantService)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Try cookie first.
			cookie, err := c.Cookie(tenantCookieName)
			if err == nil && cookie.Value != "" {
				tenantID := domain.TenantID(cookie.Value)

				t, err := tenantService.GetByID(c.Request().Context(), tenantID)
				if err != nil {
					return echo.NewHTTPError(http.StatusNotFound, "tenant not found")
				}

				userID := auth.GetUserID(c.Request().Context())
				isMember, err := tenantService.IsMember(c.Request().Context(), tenantID, userID)
				if err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, "failed to verify membership")
				}
				if !isMember {
					return echo.NewHTTPError(http.StatusForbidden, "not a member of this tenant")
				}

				ctx := auth.SetTenantID(c.Request().Context(), tenantID)
				ctx = SetTenantName(ctx, t.Name)
				c.SetRequest(c.Request().WithContext(ctx))
				return next(c)
			}

			// Fall back to header-based tenant (for API clients).
			return headerTenant(next)(c)
		}
	}
}

// CookieJWTMiddleware combines CookieAuth and CookieTenant middleware.
func CookieJWTMiddleware(authService auth.AuthService, tenantService tenant.TenantService) echo.MiddlewareFunc {
	authMw := CookieAuthMiddleware(authService)
	tenantMw := CookieTenantMiddleware(tenantService)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return authMw(tenantMw(next))
	}
}
