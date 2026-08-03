# Decompose cmd/server/main.go into a structured composition root

## Status

Proposed

## Context

`cmd/server/main.go` is a 287-line file containing three things that the
AGENTS.md says should not live in `cmd/`:

1. **Raw handler functions** — `registerHandler`, `loginHandler`, `meHandler`,
   `createTenantHandler`, `listTenantsHandler`. These are API handlers that
   belong in their domain packages (`internal/auth/handler.go`,
   `internal/tenant/handler.go`). The AGENTS.md states: "cmd/ contains only
   main.go files — one per binary. No business logic."

2. **Request/response types** — `registerRequest`, `loginRequest`,
   `createTenantRequest`. These should be in the handler packages.

3. **Error mapping functions** — `mapAuthError`, `mapTenantError`. These are
   addressed separately by ADR-0004 (centralized `httperr` package).

The `main()` function itself is a shallow module — its interface (the function
body) is nearly as complex as its implementation (the wiring it performs).
The deletion test: if you delete `main()`, the wiring logic reappears in
`internal/server.NewServer()`, not in the callers.

Additionally, `cmd/worker/main.go` has the same pattern: a ~200-line `run()`
function with all worker wiring inline. It should follow the same pattern.

## Decision

### 1. Move raw handler functions to domain packages

Auth endpoints (`register`, `login`, `me`) move to `internal/auth/handler.go`:

```go
// internal/auth/handler.go
type Handler struct {
    service AuthService
}

func NewHandler(service AuthService) *Handler { ... }

func (h *Handler) Register(g *echo.Group, mw ...echo.MiddlewareFunc) {
    g.POST("/auth/register", h.Register, mw...)
    g.POST("/auth/login", h.Login, mw...)
    g.GET("/auth/me", h.Me, mw...)
}
```

Tenant endpoints (`create`, `list`) move to `internal/tenant/handler.go`:

```go
// internal/tenant/handler.go
type Handler struct {
    service TenantService
}

func NewHandler(service TenantService) *Handler { ... }

func (h *Handler) Register(g *echo.Group, mw ...echo.MiddlewareFunc) {
    g.POST("/tenants", h.Create, mw...)
    g.GET("/tenants", h.List, mw...)
}
```

This brings auth and tenant into the same handler pattern as every other
domain (sources, reports, scheduling, usage, budget).

### 2. Extract composition root into `internal/server`

Create `internal/server/server.go` with a `New(cfg) → *echo.Echo` constructor:

```go
// internal/server/server.go
package server

type Server struct {
    echo *echo.Echo
    cfg  config.Config
    pool *pgxpool.Pool
}

func New(cfg config.Config) (*Server, error) {
    // 1. Database
    // 2. Repositories
    // 3. Services
    // 4. Handlers
    // 5. Middleware
    // 6. Route registration
    // 7. Error handler
    return &Server{...}, nil
}

func (s *Server) Start(addr string) error { ... }
func (s *Server) Shutdown(ctx context.Context) error { ... }
```

`cmd/server/main.go` reduces to:

```go
func main() {
    cfg, err := config.Load()
    if err != nil { log.Fatal(err) }

    srv, err := server.New(cfg)
    if err != nil { log.Fatal(err) }
    defer srv.Close()

    go func() { _ = srv.Start(":3000") }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    _ = srv.Shutdown(ctx)
}
```

### 3. Apply the same pattern to `cmd/worker/main.go`

Extract `internal/worker` package with `New(cfg) → *Worker` constructor.
`cmd/worker/main.go` reduces to the same ~15-line main function.

## Consequences

### Positive

- **Locality**: wiring logic lives in one testable module (`internal/server`).
  `New(cfg)` can be tested by starting the server on a random port, hitting
  `/health`, and verifying routes are registered.
- **Interface**: `New(cfg) → (*Server, error)` — a small interface for a large
  amount of wiring.
- **Consistency**: every domain package has a `handler.go` with a `Handler`
  struct and `Register` method. Auth and tenant are no longer exceptions.
- **Deletion test**: `cmd/server/main.go` becomes a thin shell. The wiring
  lives in `internal/server.New()`.
- **Worker symmetry**: `cmd/worker/main.go` follows the same pattern as
  `cmd/server/main.go`. Both are thin shells over a composition root.

### Negative

- Adds a new package (`internal/server`) with dependencies on every domain
  package. This is acceptable — it's the composition root; it's supposed to
  depend on everything.
- `internal/server` is not testable without a database and Redis. This is
  inherent to a composition root — integration tests are the right level.

### Neutral

- `internal/auth` and `internal/tenant` gain `handler.go` files. This is
  consistent with every other domain package and with the AGENTS.md convention.
- `internal/server` may grow large. If it exceeds ~300 lines, consider
  splitting route registration into `internal/server/routes.go`.