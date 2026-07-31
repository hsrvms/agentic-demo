package web

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/labstack/echo/v4"
)

// Server holds the web (HTML) route group and its dependencies.
type Server struct {
	authService   auth.AuthService
	tenantService tenant.TenantService
	secureCookies bool
}

// ServerOption configures the web server.
type ServerOption func(*Server)

// WithSecureCookies enables the Secure flag on all cookies.
func WithSecureCookies() ServerOption {
	return func(s *Server) {
		s.secureCookies = true
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
	return s
}

// Register mounts the web routes on the given Echo instance.
// Web routes live under / (root) and use cookie-based auth, CSRF
// protection, and HTML rendering.
func (s *Server) Register(e *echo.Group) {
	// Apply web middleware stack.
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))

	// Public web routes (no auth required).
	e.GET("/login", s.loginPage)
	e.POST("/login", s.loginSubmit)

	// Authenticated web routes.
	authGroup := e.Group("",
		CookieAuthMiddleware(s.authService),
		CookieTenantMiddleware(s.tenantService),
		FlashMiddleware(),
	)

	authGroup.GET("/", s.homePage)
	authGroup.GET("/dashboard", s.homePage)
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

func (s *Server) loginPage(c echo.Context) error {
	// Minimal login page — a proper implementation would use a dedicated template.
	return c.HTML(http.StatusOK, loginHTML)
}

func (s *Server) loginSubmit(c echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")

	token, err := s.authService.Login(c.Request().Context(), email, password)
	if err != nil {
		setFlashCookie(c, webui.Flash{Intent: "error", Message: "Invalid email or password"})
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	// Set JWT as httpOnly cookie.
	c.SetCookie(&http.Cookie{ //nolint:gosec // Secure is configurable via WithSecureCookies.
		Name:     jwtCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookies,
		MaxAge:   86400, // 24 hours
	})

	setFlashCookie(c, webui.Flash{Intent: "success", Message: "Welcome back!"})
	return c.Redirect(http.StatusSeeOther, "/")
}

func (s *Server) homePage(c echo.Context) error {
	flashes := GetFlashMessages(c.Request().Context())
	return Render(c, http.StatusOK, webpages.Home(flashes))
}

// loginHTML is a minimal login form. A full implementation would use templ.
const loginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sign In — Platform</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<script src="https://cdn.tailwindcss.com"></script>
<script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
<style>
:root { --color-surface-page: #f8fafc; --color-surface-card: #ffffff; --color-content: #0f172a; --color-content-secondary: #475569; --color-primary: #4f46e5; --color-primary-hover: #4338ca; --color-primary-on: #ffffff; --color-border: #e2e8f0; --color-intent-error: #e11d48; }
</style>
</head>
<body class="bg-[var(--color-surface-page)] text-[var(--color-content)] font-['Inter',sans-serif] min-h-screen flex items-center justify-center">
<div class="w-full max-w-sm mx-4">
<div class="bg-[var(--color-surface-card)] border border-[var(--color-border)] rounded-lg shadow-sm p-8">
<h1 class="text-2xl font-bold mb-1">Sign in</h1>
<p class="text-sm text-[var(--color-content-secondary)] mb-6">Welcome back to Platform</p>
<form method="POST" action="/login" class="space-y-4">
<div>
<label for="email" class="block text-sm font-medium mb-1">Email</label>
<input id="email" name="email" type="email" required class="block w-full rounded-lg border border-[var(--color-border)] px-3 py-2 text-sm focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20 outline-none transition-colors" placeholder="you@example.com">
</div>
<div>
<label for="password" class="block text-sm font-medium mb-1">Password</label>
<input id="password" name="password" type="password" required class="block w-full rounded-lg border border-[var(--color-border)] px-3 py-2 text-sm focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20 outline-none transition-colors" placeholder="••••••••">
</div>
<button type="submit" class="w-full rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-on)] hover:bg-[var(--color-primary-hover)] focus:ring-2 focus:ring-[var(--color-primary)] focus:ring-offset-2 transition-colors">Sign in</button>
</form>
</div>
</div>
</body>
</html>`
