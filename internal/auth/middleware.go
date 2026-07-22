package auth

import (
	"net/http"
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/labstack/echo/v4"
)

// AuthMiddleware returns Echo middleware that validates the JWT and sets
// the user ID in the request context. This middleware does NOT check
// tenant membership — use TenantMiddleware for tenant-scoped routes.
func AuthMiddleware(authService AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if header == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing authorization header")
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization header format")
			}

			claims, err := authService.ValidateToken(c.Request().Context(), parts[1])
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
			}

			ctx := SetUserID(c.Request().Context(), claims.UserID)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

// TenantMiddleware returns Echo middleware that verifies the user is a
// member of the tenant specified in the X-Tenant-ID header. It must be
// used after AuthMiddleware so that the user ID is already in the context.
func TenantMiddleware(tenantService tenant.TenantService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tenantIDStr := c.Request().Header.Get("X-Tenant-ID")
			if tenantIDStr == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "missing X-Tenant-ID header")
			}
			tenantID := domain.TenantID(tenantIDStr)

			// Verify tenant exists.
			_, err := tenantService.GetByID(c.Request().Context(), tenantID)
			if err != nil {
				return echo.NewHTTPError(http.StatusNotFound, "tenant not found")
			}

			// Get user ID from context (set by AuthMiddleware).
			userID := GetUserID(c.Request().Context())

			// Verify membership.
			isMember, err := tenantService.IsMember(c.Request().Context(), tenantID, userID)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to verify membership")
			}
			if !isMember {
				return echo.NewHTTPError(http.StatusForbidden, "user is not a member of this tenant")
			}

			ctx := SetTenantID(c.Request().Context(), tenantID)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

// JWTMiddleware combines AuthMiddleware and TenantMiddleware for
// convenience on routes that require both auth and tenant scoping.
func JWTMiddleware(authService AuthService, tenantService tenant.TenantService) echo.MiddlewareFunc {
	auth := AuthMiddleware(authService)
	tenantMw := TenantMiddleware(tenantService)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return auth(tenantMw(next))
	}
}
