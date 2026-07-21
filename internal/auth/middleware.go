package auth

import (
	"net/http"
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/labstack/echo/v4"
)

// JWTMiddleware returns an Echo middleware that authenticates requests.
//
// Flow:
//  1. Extract Bearer token from Authorization header
//  2. Validate the JWT (signature, expiry)
//  3. Extract X-Tenant-ID header
//  4. Verify the user is a member of the requested tenant
//  5. Set tenant ID and user ID in the request context
func JWTMiddleware(authService AuthService, tenantService tenant.TenantService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract token.
			header := c.Request().Header.Get("Authorization")
			if header == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing authorization header")
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization header format")
			}
			tokenString := parts[1]

			// Validate token.
			claims, err := authService.ValidateToken(c.Request().Context(), tokenString)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
			}

			// Extract tenant ID.
			tenantIDStr := c.Request().Header.Get("X-Tenant-ID")
			if tenantIDStr == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "missing X-Tenant-ID header")
			}
			tenantID := domain.TenantID(tenantIDStr)

			// Verify tenant exists.
			_, err = tenantService.GetByID(c.Request().Context(), tenantID)
			if err != nil {
				return echo.NewHTTPError(http.StatusNotFound, "tenant not found")
			}

			// Verify membership.
			isMember, err := tenantService.IsMember(c.Request().Context(), tenantID, claims.UserID)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to verify membership")
			}
			if !isMember {
				return echo.NewHTTPError(http.StatusForbidden, "user is not a member of this tenant")
			}

			// Set context values.
			ctx := c.Request().Context()
			ctx = SetTenantID(ctx, tenantID)
			ctx = SetUserID(ctx, claims.UserID)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}
