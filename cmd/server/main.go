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

	// Auth and tenant handlers.
	api := e.Group("/api")

	authMw := auth.AuthMiddleware(authService)
	tenantMw := auth.JWTMiddleware(authService, tenantService)

	authHandler := auth.NewHandler(authService)
	authHandler.Register(api, tenantMw) // tenantMw only applied to /auth/me

	tenantHandler := tenant.NewHandler(tenantService)
	tenantHandler.Register(api, authMw) // authMw applied to all tenant routes

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

