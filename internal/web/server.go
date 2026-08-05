package web

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/agentic-demo/platform/internal/usage"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/labstack/echo/v4"
)

// Server holds the web (HTML) route group and its dependencies.
type Server struct {
	authHandler      *AuthHandler
	dashboardHandler *DashboardHandler
	sourcesHandler   *SourcesHandler
	reportsHandler   *ReportsHandler
	authService      auth.AuthService
	tenantService    tenant.TenantService
	secureCookies    bool
}

// ServerOption configures the web server.
type ServerOption func(*Server)

// WithSecureCookies enables the Secure flag on all cookies.
func WithSecureCookies() ServerOption {
	return func(s *Server) {
		s.secureCookies = true
	}
}

// WithDashboard wires the dashboard handler with its service dependencies.
func WithDashboard(
	usageService usage.UsageService,
	reportService reports.ReportService,
	sourceService sources.Service,
	budgetService budget.BudgetService,
) ServerOption {
	return func(s *Server) {
		s.dashboardHandler = NewDashboardHandler(usageService, reportService, sourceService, budgetService)
	}
}

// WithSources wires the sources management handler.
func WithSources(sourceCore *sources.HandlerCore) ServerOption {
	return func(s *Server) {
		s.sourcesHandler = NewSourcesHandler(sourceCore)
	}
}

// WithReports wires the report browsing handler.
func WithReports(reportService reports.ReportService) ServerOption {
	return func(s *Server) {
		s.reportsHandler = NewReportsHandler(reportService)
	}
}

// NewServer creates a web Server.
func NewServer(authService auth.AuthService, tenantService tenant.TenantService, opts ...ServerOption) *Server {
	s := &Server{
		authService:   authService,
		tenantService: tenantService,
	}
	for _, opt := range opts {
		opt(s)
	}
	s.authHandler = NewAuthHandler(authService, tenantService, s.secureCookies)
	return s
}

// Register mounts the web routes on the given Echo instance.
// Web routes live under / (root) and use cookie-based auth, CSRF
// protection, and HTML rendering.
func (s *Server) Register(e *echo.Group) {
	// Apply web middleware stack.
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))

	// Public web routes (no auth required).
	e.GET("/login", s.authHandler.loginPage)
	e.POST("/login", s.authHandler.loginSubmit)
	e.GET("/register", s.authHandler.registerPage)
	e.POST("/register", s.authHandler.registerSubmit)

	// Auth-only routes (JWT required, no tenant context yet).
	authOnly := e.Group("",
		CookieAuthMiddleware(s.authService),
		FlashMiddleware(),
	)
	authOnly.GET("/select-tenant", s.authHandler.selectTenantPage)
	authOnly.POST("/select-tenant", s.authHandler.selectTenantSubmit)
	authOnly.POST("/create-tenant", s.authHandler.createTenantSubmit)
	authOnly.POST("/logout", s.authHandler.logout)

	// Authenticated web routes (JWT + tenant required).
	authGroup := e.Group("",
		CookieAuthMiddleware(s.authService),
		CookieTenantMiddleware(s.tenantService),
		FlashMiddleware(),
	)

	authGroup.GET("/", s.homePage)
	authGroup.GET("/dashboard", s.dashboardOrHome)

	// Sources management routes.
	if s.sourcesHandler != nil {
		s.sourcesHandler.Register(authGroup)
	}

	// Report browsing routes.
	if s.reportsHandler != nil {
		s.reportsHandler.Register(authGroup)
	}
}

// FlashMiddleware reads flash cookies on GET requests and injects them
// into the context for templates to render.
func FlashMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Method == http.MethodGet {
				flashes := ReadFlashCookie(c)
				if len(flashes) > 0 {
					ctx := SetFlashMessages(c.Request().Context(), flashes)
					c.SetRequest(c.Request().WithContext(ctx))
				}
			}
			return next(c)
		}
	}
}

// --- Page handlers ---

func (s *Server) homePage(c echo.Context) error {
	flashes := GetFlashMessages(c.Request().Context())
	return Render(c, http.StatusOK, webpages.Home(flashes))
}

// dashboardOrHome delegates to the DashboardHandler if wired, otherwise
// falls back to the generic home page.
func (s *Server) dashboardOrHome(c echo.Context) error {
	if s.dashboardHandler != nil {
		return s.dashboardHandler.dashboardPage(c)
	}
	return s.homePage(c)
}
