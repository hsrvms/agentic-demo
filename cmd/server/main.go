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
	"crypto/sha256"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/config"
	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/httperr"
	"github.com/agentic-demo/platform/internal/queue"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/scheduling"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/agentic-demo/platform/internal/web"
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

	scheduleRepo := scheduling.NewRepository(queries)
	scheduleService := scheduling.NewService(scheduleRepo)

	reportRepo := reports.NewRepository(queries)
	reportService := reports.NewService(reportRepo)

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

	// Schedule routes (tenant-scoped).
	scheduleHandler := scheduling.NewHandler(scheduleService)
	scheduleHandler.Register(api, tenantMw)

	// Report routes (tenant-scoped).
	reportHandler := reports.NewHandler(reportService)
	reportHandler.Register(api, tenantMw)

	// Data source routes (tenant-scoped).
	encryptionKey := sha256.Sum256([]byte(cfg.EncryptionKey))
	cryptSvc, err := sources.NewAESGCMService(encryptionKey[:])
	if err != nil {
		log.Fatalf("crypto service: %v", err)
	}
	sourceRepo := sources.NewRepository(queries)
	connectionTester := sources.NewConnectionTester()
	jobQueue, err := queue.NewAsynqQueue(cfg.RedisURL)
	if err != nil {
		log.Fatalf("job queue: %v", err)
	}
	sourceService := sources.NewService(sourceRepo, cryptSvc, connectionTester, jobQueue)
	sourceHandler := sources.NewHandler(sourceService)
	sourceHandler.Register(api, tenantMw)

	// Usage routes (tenant-scoped).
	usageRepo := usage.NewRepository(queries)
	usageReader, err := usage.NewRedisReader(cfg.RedisURL)
	if err != nil {
		log.Fatalf("usage reader: %v", err)
	}
	usageService := usage.NewService(usageRepo, usageReader)
	usageHandler := usage.NewHandler(usageService)
	usageHandler.Register(api, tenantMw)

	// Budget routes (tenant-scoped).
	budgetRepo := budget.NewRepository(queries)
	budgetService := budget.NewService(budgetRepo, usageReader, usageRepo)
	budgetHandler := budget.NewHandler(budgetService)
	budgetHandler.Register(api, tenantMw)

	// Web (HTML) routes — cookie-based auth, CSRF, server-rendered pages.
	webServer := web.NewServer(authService, tenantService,
		web.WithDashboard(usageService, reportService, sourceService, budgetService),
		web.WithSources(sourceService),
	)
	webServer.Register(e.Group(""))

	// Static files (embedded from web/static/).
	e.GET("/static/*", web.StaticHandler())

	// Set error handler that routes /api/* to JSON, everything else to web redirects.
	e.HTTPErrorHandler = web.MakeErrorHandler(e.HTTPErrorHandler)

	// Deferred cleanup.
	defer usageReader.Close()
	defer jobQueue.Close()

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
		log.Printf("shutdown error: %v", shutdownErr)
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
			return echo.NewHTTPError(httperr.MapHTTP(err))
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
			return echo.NewHTTPError(httperr.MapHTTP(err))
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
			return echo.NewHTTPError(httperr.MapHTTP(err))
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

// mapAuthError and mapTenantError have been migrated to internal/httperr/mapper.go.
// Use httperr.MapHTTP(err) instead.
