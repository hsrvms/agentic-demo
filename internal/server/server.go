// Package server owns the HTTP composition root.
package server

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"

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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Server is the HTTP application and the owner of its infrastructure resources.
type Server struct {
	echo *echo.Echo
	pool *pgxpool.Pool

	usageReader usage.UsageReader
	jobQueue    queue.JobQueue

	closeOnce sync.Once
	closeErr  error
}

// New builds the HTTP application and all of its dependencies.
//
//nolint:gocritic // Config is copied once at the composition boundary.
func New(cfg config.Config) (*Server, error) {
	pool, err := db.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	queries := db.New(pool)
	usageReader, jobQueue, err := buildServices(cfg.RedisURL)
	if err != nil {
		pool.Close()
		return nil, err
	}

	cleanupOnError := func() {
		_ = usageReader.Close()
		_ = jobQueue.Close()
		pool.Close()
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:    true,
		LogStatus: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log.Printf("%s %s %d", c.Request().Method, v.URI, v.Status)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.GET("/health", health)

	api := e.Group("/api")
	authService := auth.NewService(auth.NewRepository(queries), []byte(cfg.JWTSecret))
	tenantService := tenant.NewService(tenant.NewRepository(queries))
	authMw := auth.AuthMiddleware(authService)
	tenantMw := auth.JWTMiddleware(authService, tenantService)

	auth.NewHandler(authService).Register(api, tenantMw)
	tenant.NewHandler(tenantService).Register(api, authMw)

	scheduleService := scheduling.NewService(scheduling.NewRepository(queries))
	scheduling.NewHandler(scheduleService).Register(api, tenantMw)

	reportService := reports.NewService(reports.NewRepository(queries))
	reports.NewHandler(reportService).Register(api, tenantMw)

	encryptionKey := sha256.Sum256([]byte(cfg.EncryptionKey))
	cryptService, err := sources.NewAESGCMService(encryptionKey[:])
	if err != nil {
		cleanupOnError()
		return nil, fmt.Errorf("create data source crypto service: %w", err)
	}
	sourceService := sources.NewService(
		sources.NewRepository(queries),
		cryptService,
		sources.NewConnectionTester(),
		jobQueue,
	)
	sourceCore := sources.NewHandlerCore(sourceService)
	sources.NewHandler(sourceCore).Register(api, tenantMw)

	usageRepo := usage.NewRepository(queries)
	usageService := usage.NewService(usageRepo, usageReader)
	usage.NewHandler(usageService).Register(api, tenantMw)

	budgetService := budget.NewService(budget.NewRepository(queries), usageReader, usageRepo)
	budget.NewHandler(budgetService).Register(api, tenantMw)

	web.NewServer(authService, tenantService,
		web.WithDashboard(usageService, reportService, sourceService, budgetService),
		web.WithSources(sourceCore),
	).Register(e.Group(""))
	e.GET("/static/*", web.StaticHandler())
	e.HTTPErrorHandler = web.MakeErrorHandler(e.HTTPErrorHandler)

	return &Server{
		echo:        e,
		pool:        pool,
		usageReader: usageReader,
		jobQueue:    jobQueue,
	}, nil
}

// buildServices creates infrastructure shared by the HTTP handlers. Keeping
// this separate makes cleanup of constructor failures explicit.
func buildServices(redisURL string) (usage.UsageReader, queue.JobQueue, error) {
	usageReader, err := usage.NewRedisReader(redisURL)
	if err != nil {
		return nil, nil, fmt.Errorf("create usage reader: %w", err)
	}

	jobQueue, err := queue.NewAsynqQueue(redisURL)
	if err != nil {
		_ = usageReader.Close()
		return nil, nil, fmt.Errorf("create job queue: %w", err)
	}

	return usageReader, jobQueue, nil
}

func health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Start binds addr and serves HTTP until the server is shut down.
func (s *Server) Start(addr string) error {
	err := s.echo.Start(addr)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Addr returns the address currently bound by the HTTP listener. It is empty
// until Start has created the listener.
func (s *Server) Addr() string {
	addr := s.echo.ListenerAddr()
	if addr == nil {
		return ""
	}
	return addr.String()
}

// Shutdown gracefully stops HTTP traffic and releases owned resources.
func (s *Server) Shutdown(ctx context.Context) error {
	return errors.Join(s.echo.Shutdown(ctx), s.Close())
}

// Close releases resources owned by the Server. It is safe to call more than
// once, which allows callers to defer Close and also use Shutdown.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = errors.Join(
			s.usageReader.Close(),
			s.jobQueue.Close(),
		)
		s.pool.Close()
	})
	return s.closeErr
}
