# Go Project Conventions

These rules apply to this Go project. They supplement the global engineering rules in `~/.pi/agent/AGENTS.md`.

---

## Language & Runtime

- Target **Go 1.26+**.
- Use modern Go features: range over integers, weak aliases, `min`/`max` builtins, `slog`, and the `net/http` routing enhancements.
- Follow the [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) and [Effective Go](https://go.dev/doc/effective_go).

---

## Toolchain

| Tool               | Role                                      |
|--------------------|-------------------------------------------|
| `go`               | Build, test, vet, mod                     |
| `golangci-lint`    | Linter aggregator (replaces golint, etc.) |
| `sqlc`             | Type-safe SQL generation from queries     |
| `a-h/templ`            | Type-safe HTML template generation        |
| `tailwindcss`      | Utility-first CSS (via CLI or standalone) |
| `air`              | Live reload in development                |
| `staticcheck`      | Additional static analysis                |

---

## Project Structure

```
project-root/
    go.mod
    go.sum
    Makefile
    README.md
    cmd/
        server/
            main.go              # entry point
    internal/
        auth/                    # domain: auth
            handler.go           # HTTP handlers (Echo)
            service.go           # business logic
            repository.go        # database queries (sqlc-generated)
            model.go             # domain types
        users/                   # domain: users
            handler.go
            service.go
            repository.go
            model.go
        middleware/               # shared middleware
            auth.go
            logging.go
        server/                  # server setup & wiring
            server.go
            routes.go
    web/
        templates/               # .templ files (a-h/templ)
            layouts/
                base.templ
            components/
                navbar.templ
                alert.templ
            pages/
                home.templ
                login.templ
        static/
            js/                  # Alpine.js components
                app.js
            css/
                input.css        # Tailwind source
                output.css       # Tailwind build output
    sql/
        migrations/              # numbered migration files
            001_create_users.sql
            002_create_sessions.sql
        queries/                 # sqlc query files
            users.sql
            sessions.sql
    sqlc.yaml                    # sqlc configuration
    tailwind.config.js           # Tailwind configuration
    .air.toml                    # air live-reload config
    .golangci.yaml               # linter configuration
```

Rules:

- **`cmd/`** contains only `main.go` files — one per binary. No business logic.
- **`internal/`** is organized by **domain**, not by technical layer.
- **`web/`** holds templates, static assets, and frontend config.
- **`sql/`** separates migrations from queries. `sqlc` reads `queries/` and generates Go code.
- Never import from `cmd/` into `internal/`.

---

## golangci-lint Configuration

```yaml
# .golangci.yaml
run:
  timeout: 5m

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gosec
    - revive
    - gocritic
    - bodyclose
    - nilerr
    - noctx
    - rowserrcheck
    - sqlclosecheck
    - unconvert
    - unparam
    - wastedassign

linters-settings:
  revive:
    rules:
      - name: exported
        arguments:
          - disableStutteringCheck
      - name: unexported-return
        disabled: true
  gocritic:
    enabled-tags:
      - diagnostic
      - style
      - performance

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - errcheck
        - gosec
        - bodyclose
```

---

## sqlc Configuration

```yaml
# sqlc.yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "sql/queries/"
    schema: "sql/migrations/"
    gen:
      go:
        package: "db"
        out: "internal/db"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_empty_slices: true
        overrides:
          - db_type: "timestamptz"
            go_type: "time.Time"
          - db_type: "uuid"
            go_type: "github.com/google/uuid.UUID"
```

Rules:

- Write raw SQL in `sql/queries/*.sql` with `-- name:` annotations.
- Never hand-edit files in `internal/db/` — they are generated.
- Run `sqlc generate` after modifying queries or migrations.
- Wrap generated queries in domain repositories, don't call `db` package directly from handlers.

---

## Echo & HTTP Handlers

```go
// handler.go
func (h *UserHandler) Register(g *echo.Group) {
    g.GET("/users", h.List)
    g.GET("/users/:id", h.Get)
    g.POST("/users", h.Create)
}

func (h *UserHandler) List(c echo.Context) error {
    users, err := h.service.ListUsers(c.Request().Context())
    if err != nil {
        return err
    }
    return templ.New(200, pages.UserList(users)).Render(c)
}

func (h *UserHandler) Get(c echo.Context) error {
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "invalid user ID")
    }
    user, err := h.service.GetUser(c.Request().Context(), id)
    if err != nil {
        return err
    }
    return templ.New(200, pages.UserDetail(user)).Render(c)
}
```

Rules:

- Handlers parse input, call a service, return a response. No business logic in handlers.
- Always pass `c.Request().Context()` to services — never `context.Background()` in handlers.
- Use `echo.Group` to organize related routes.
- Use typed path parameters (`uuid.Parse`, `strconv.Atoi`) at the handler boundary.
- Return `error` from handlers and let a centralized error handler format responses.
- Use `echo.NewHTTPError` for client-facing errors with appropriate status codes.

### Templ Rendering Helper

```go
// internal/server/templ.go
package server

import (
    "github.com/a-h/templ"
    "github.com/labstack/echo/v4"
)

func Render(c echo.Context, status int, t templ.Component) error {
    c.Response().Status = status
    c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
    return t.Render(c.Request().Context(), c.Response().Writer)
}
```

---

## a-h/templ Templates

```templ
// web/templates/pages/home.templ
package pages

import "project/internal/users"

templ UserList(users []users.User) {
    @Layout("Users") {
        <div class="max-w-4xl mx-auto px-4 py-8">
            <h1 class="text-2xl font-bold mb-6">Users</h1>
            for _, u := range users {
                @UserCard(u)
            }
        </div>
    }
}
```

Rules:

- Templates live in `web/templates/` organized by `layouts/`, `components/`, and `pages/`.
- Templates receive typed Go values — no `any`, no `map[string]any`.
- Use `Layout` components for shared page structure (head, nav, footer).
- Keep logic in templates minimal: conditionals and loops only. No business logic.
- Run `templ generate` to produce `.go` files. Never hand-edit generated files.
- Commit generated `_templ.go` files so builds work without the `templ` CLI.

---

## HTMX Patterns

Rules:

- Use `hx-get`, `hx-post`, `hx-put`, `hx-delete`, `hx-patch` for server interactions.
- Use `hx-target` to specify which element receives the response.
- Use `hx-swap` to control how content is replaced (`innerHTML`, `outerHTML`, `beforebegin`, etc.).
- Return HTML fragments from handlers when the request has `HX-Request: true`.
- Use `hx-indicator` for loading states.
- Use `hx-confirm` for destructive actions.
- Avoid `hx-vals` with inline JS — pass data via query params or form fields.

```html
<!-- Good: clean HTMX -->
<button hx-delete="/users/123"
        hx-target="closest tr"
        hx-swap="outerHTML"
        hx-confirm="Delete this user?">
    Delete
</button>

<!-- Good: partial render for HTMX requests -->
<div id="user-list" hx-get="/users?page=2" hx-trigger="revealed" hx-swap="beforeend">
    ...
</div>
```

Detect HTMX requests in handlers:

```go
func isHTMX(c echo.Context) bool {
    return c.Request().Header.Get("HX-Request") == "true"
}

func (h *UserHandler) Delete(c echo.Context) error {
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "invalid user ID")
    }
    if err := h.service.DeleteUser(c.Request().Context(), id); err != nil {
        return err
    }
    if isHTMX(c) {
        return c.NoContent(http.StatusOK)
    }
    return c.Redirect(http.StatusSeeOther, "/users")
}
```

---

## Alpine.js

Rules:

- Use Alpine.js for client-side interactivity that doesn't need a server round-trip: dropdowns, modals, tabs, form validation feedback.
- Keep Alpine state local and small. If state grows complex, it belongs in a backend component.
- Use `x-data` with a function for reusable components.
- Use `x-ref` to reference DOM elements instead of `document.querySelector`.
- Avoid mixing HTMX and Alpine on the same element unless necessary.
- Place Alpine component definitions in `web/static/js/`.

```html
<!-- Good: Alpine for dropdown -->
<div x-data="{ open: false }" class="relative">
    <button @click="open = !open">Menu</button>
    <div x-show="open" @click.outside="open = false" class="absolute">
        <a href="/settings">Settings</a>
        <a href="/logout">Logout</a>
    </div>
</div>

<!-- Good: Alpine reusable component -->
<script>
document.addEventListener('alpine:init', () => {
    Alpine.data('notification', () => ({
        show: false,
        message: '',
        display(msg) {
            this.message = msg
            this.show = true
            setTimeout(() => this.show = false, 3000)
        }
    }))
})
</script>
```

---

## Tailwind CSS

Rules:

- Use the Tailwind CLI in watch mode during development.
- Write Tailwind classes directly in `.templ` files — the scanner picks them up.
- Extract repeated patterns into templ components, not `@apply` rules.
- Keep `input.css` minimal: only the `@tailwind` directives and rare custom rules.

```css
/* web/static/css/input.css */
@tailwind base;
@tailwind components;
@tailwind utilities;
```

```javascript
// tailwind.config.js
/** @type {import('tailwindcss').Config} */
module.exports = {
    content: [
        "./web/templates/**/*.templ",
        "./web/templates/**/*.html",
    ],
    theme: {
        extend: {},
    },
    plugins: [],
}
```

---

## PostgreSQL & Migrations

Rules:

- Use numbered migration files: `001_create_users.sql`, `002_add_email_index.sql`.
- Each migration has an `up` and `down` section, or separate `001_up.sql` / `001_down.sql` files.
- Migrations are applied by a tool like `goose`, `migrate`, or `atlas`. Pick one and stick with it.
- Never modify a migration that has been applied to production — create a new one.
- Use `timestamptz` (not `timestamp`), `text` (not `varchar`), and `uuid` for IDs when appropriate.
- Always add indexes for foreign keys and columns used in `WHERE` / `ORDER BY`.

```sql
-- sql/migrations/001_create_users.sql
CREATE TABLE IF NOT EXISTS users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email       text NOT NULL UNIQUE,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_email ON users (email);
```

---

## Repository Pattern

Wrap sqlc-generated code behind domain interfaces:

```go
// internal/users/repository.go
package users

import (
    "context"
    "github.com/google/uuid"
    "project/internal/db"
)

type Repository interface {
    FindByID(ctx context.Context, id uuid.UUID) (User, error)
    FindAll(ctx context.Context) ([]User, error)
    Create(ctx context.Context, params CreateUserParams) (User, error)
    Delete(ctx context.Context, id uuid.UUID) error
}

type pgRepository struct {
    queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
    return &pgRepository{queries: queries}
}

func (r *pgRepository) FindByID(ctx context.Context, id uuid.UUID) (User, error) {
    row, err := r.queries.GetUserByID(ctx, id)
    if err != nil {
        return User{}, err
    }
    return toDomain(row), nil
}
```

Rules:

- Handlers never import the `db` package directly.
- Repositories translate between sqlc types and domain types.
- Domain types live in `model.go`, not in the `db` package.
- This keeps sqlc changes isolated to the repository layer.

---

## Dependency Injection & Wiring

Wire everything in `cmd/server/main.go`:

```go
func main() {
    // 1. Configuration
    cfg := config.Load()

    // 2. Database
    pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()
    queries := db.New(pool)

    // 3. Repositories
    userRepo := users.NewRepository(queries)

    // 4. Services
    userService := users.NewService(userRepo)

    // 5. Handlers
    userHandler := users.NewHandler(userService)

    // 6. Server
    srv := server.New(cfg, userHandler)
    srv.Start()
}
```

Rules:

- Use constructor functions (`NewX`), not global variables or `init()`.
- Pass dependencies explicitly — no service locators.
- Use interfaces at consumption boundaries for testability.
- `main.go` is the composition root. It wires everything and starts the server.

---

## Error Handling

- Always check errors. Never use `_` for error returns.
- Wrap errors with context using `fmt.Errorf("description: %w", err)`.
- Use `errors.Is` and `errors.As` for error comparison, not string matching.
- Define sentinel errors in the domain package:

```go
// internal/users/errors.go
package users

import "errors"

var (
    ErrNotFound      = errors.New("user not found")
    ErrAlreadyExists = errors.New("user already exists")
)
```

- Use a centralized Echo error handler to map domain errors to HTTP responses:

```go
func ErrorHandler(err error, c echo.Context) {
    code := http.StatusInternalServerError
    message := "internal server error"

    switch {
    case errors.Is(err, users.ErrNotFound):
        code = http.StatusNotFound
        message = "not found"
    case errors.Is(err, users.ErrAlreadyExists):
        code = http.StatusConflict
        message = "already exists"
    default:
        var he *echo.HTTPError
        if errors.As(err, &he) {
            code = he.Code
            message = he.Message.(string)
        }
    }

    if !c.Response().Committed {
        _ = c.JSON(code, map[string]string{"error": message})
    }
}
```

- Log the full error at the handler level. The client sees only the mapped message.

---

## Testing

- Use the standard `testing` package.
- Table-driven tests for logic with multiple cases.
- Test files live alongside the code: `internal/users/service.go` → `internal/users/service_test.go`.
- Use `testify/assert` and `testify/require` for readable assertions if preferred, but the standard library is sufficient.
- Mock repositories via interfaces — no reflection-based mocking libraries.

```go
// internal/users/service_test.go
func TestGetUser_NotFound(t *testing.T) {
    repo := &mockRepository{findErr: users.ErrNotFound}
    svc := NewService(repo)

    _, err := svc.GetUser(context.Background(), uuid.New())
    require.ErrorIs(t, err, users.ErrNotFound)
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
    repo := &mockRepository{createErr: users.ErrAlreadyExists}
    svc := NewService(repo)

    _, err := svc.CreateUser(context.Background(), CreateUserParams{
        Email: "taken@example.com",
        Name:  "Test",
    })
    require.ErrorIs(t, err, users.ErrAlreadyExists)
}

// Mock implements Repository interface
type mockRepository struct {
    users     []User
    findErr   error
    createErr error
}

func (m *mockRepository) FindByID(ctx context.Context, id uuid.UUID) (User, error) {
    return User{}, m.findErr
}

func (m *mockRepository) FindAll(ctx context.Context) ([]User, error) {
    return m.users, nil
}

func (m *mockRepository) Create(ctx context.Context, p CreateUserParams) (User, error) {
    if m.createErr != nil {
        return User{}, m.createErr
    }
    return User{ID: uuid.New(), Email: p.Email, Name: p.Name}, nil
}

func (m *mockRepository) Delete(ctx context.Context, id uuid.UUID) error {
    return nil
}
```

Run tests:

```bash
go test ./...                          # all tests
go test ./internal/users/...           # specific package
go test -run TestGetUser ./...         # by name
go test -race ./...                    # with race detector
go test -cover ./...                   # with coverage
```

---

## Common Commands

```bash
# Install dependencies
go mod tidy

# Run the server (with live reload)
air

# Build
go build -o bin/server ./cmd/server

# Type check & vet
go vet ./...

# Lint
golangci-lint run

# Generate sqlc code
sqlc generate

# Generate templ templates
templ generate

# Run migrations
migrate -path sql/migrations -database "$DATABASE_URL" up

# Build Tailwind CSS
npx tailwindcss -i web/static/css/input.css -o web/static/css/output.css --watch

# Run tests
go test ./...

# Run tests with race detector and coverage
go test -race -cover ./...
```

---

## Makefile

```makefile
.PHONY: dev build test lint generate tailwind migrate

dev:
	air

build:
	go build -o bin/server ./cmd/server

test:
	go test -race ./...

lint:
	golangci-lint run
	go vet ./...

generate:
	sqlc generate
	templ generate

tailwind:
	npx tailwindcss -i web/static/css/input.css -o web/static/css/output.css --watch

migrate-up:
	migrate -path sql/migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path sql/migrations -database "$(DATABASE_URL)" down
```

---

## Before Completing a Task

Run all of these and ensure they pass:

```bash
go vet ./...
golangci-lint run
go test -race ./...
go build ./...
```

Do not report a task as complete if any of these fail. If a failure is unrelated to your changes, explain it explicitly.
