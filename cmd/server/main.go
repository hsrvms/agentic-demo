// Server entry point: minimal Echo server with auth endpoints for Phase 1.
//
// Routes:
//
//	POST /api/auth/register   — {email, password} → {user_id, email, token}
//	POST /api/auth/login      — {email, password} → {token}
//	GET  /api/auth/me         — (auth + tenant) → {user_id, email, tenant_id}
//	POST /api/tenants         — (auth only) → {id, name}
//	GET  /api/tenants         — (auth only) → [{id, name, status}]
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/config"
	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}

	// Ping to verify connectivity.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		log.Fatalf("database ping failed: %v", err)
	}

	queries := db.New(pool)

	// Wire services.
	authRepo := auth.NewRepository(queries)
	authService := auth.NewService(authRepo, []byte(cfg.JWTSecret))

	tenantRepo := tenant.NewRepository(queries)
	tenantService := tenant.NewService(tenantRepo)

	// Echo setup.
	e := echo.New()
	e.HideBanner = true

	// Global middleware.
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:    true,
		LogStatus: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log.Printf("%s %s %d", c.Request().Method, v.URI, v.Status)
			return nil
		},
	}))
	e.Use(middleware.Recover())

	// Health check.
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Public auth routes (no auth required).
	api := e.Group("/api")
	authGroup := api.Group("/auth")
	authGroup.POST("/register", registerHandler(authService))
	authGroup.POST("/login", loginHandler(authService))

	// Auth-only routes (JWT required, no tenant context needed).
	// These are used for bootstrapping: creating the first tenant
	// before any tenant membership exists.
	// Per-route middleware avoids group prefix conflicts.
	authMw := auth.AuthMiddleware(authService)
	api.POST("/tenants", createTenantHandler(tenantService), authMw)
	api.GET("/tenants", listTenantsHandler(tenantService), authMw)

	// Tenant-scoped routes (JWT + X-Tenant-ID required).
	tenantMw := auth.JWTMiddleware(authService, tenantService)
	api.GET("/auth/me", meHandler, tenantMw)

	// Graceful shutdown.
	go func() {
		if err := e.Start(":3000"); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	shutdownErr := e.Shutdown(ctx)
	cancel()
	pool.Close()
	if shutdownErr != nil {
		log.Fatalf("shutdown error: %v", shutdownErr)
	}
}

// --- Request/Response types ---

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createTenantRequest struct {
	Name string `json:"name"`
}

// --- Handlers ---

func registerHandler(authService auth.AuthService) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req registerRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}

		user, err := authService.Register(c.Request().Context(), req.Email, req.Password)
		if err != nil {
			return mapAuthError(err)
		}

		token, err := authService.Login(c.Request().Context(), req.Email, req.Password)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to issue token after registration")
		}

		return c.JSON(http.StatusCreated, map[string]interface{}{
			"user_id": user.ID,
			"email":   user.Email,
			"token":   token,
		})
	}
}

func loginHandler(authService auth.AuthService) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req loginRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}

		token, err := authService.Login(c.Request().Context(), req.Email, req.Password)
		if err != nil {
			return mapAuthError(err)
		}

		return c.JSON(http.StatusOK, map[string]string{
			"token": token,
		})
	}
}

func meHandler(c echo.Context) error {
	tenantID := auth.GetTenantID(c.Request().Context())
	userID := auth.GetUserID(c.Request().Context())

	return c.JSON(http.StatusOK, map[string]interface{}{
		"user_id":   userID,
		"tenant_id": tenantID,
	})
}

func createTenantHandler(tenantService tenant.TenantService) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req createTenantRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}

		userID := auth.GetUserID(c.Request().Context())

		t, err := tenantService.Create(c.Request().Context(), userID, req.Name)
		if err != nil {
			return mapTenantError(err)
		}

		return c.JSON(http.StatusCreated, map[string]interface{}{
			"id":     t.ID,
			"name":   t.Name,
			"status": t.Status,
		})
	}
}

func listTenantsHandler(tenantService tenant.TenantService) echo.HandlerFunc {
	return func(c echo.Context) error {
		userID := auth.GetUserID(c.Request().Context())

		tenants, err := tenantService.ListByUser(c.Request().Context(), userID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to list tenants")
		}

		type tenantResponse struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		}

		result := make([]tenantResponse, len(tenants))
		for i, t := range tenants {
			result[i] = tenantResponse{
				ID:     string(t.ID),
				Name:   t.Name,
				Status: string(t.Status),
			}
		}

		return c.JSON(http.StatusOK, result)
	}
}

// --- Error mapping ---

func mapAuthError(err error) *echo.HTTPError {
	switch err {
	case auth.ErrUserExists:
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case auth.ErrInvalidCredentials:
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	case auth.ErrInvalidEmail:
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case auth.ErrWeakPassword:
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}

func mapTenantError(err error) *echo.HTTPError {
	switch err {
	case tenant.ErrInvalidName:
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case tenant.ErrTenantNotFound:
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case tenant.ErrAlreadyExists:
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case tenant.ErrInvalidRole:
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}
