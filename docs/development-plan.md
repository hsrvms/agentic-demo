# Development Plan: Handler Deepening + Error Mapping + WebUI Mapper

> Covers ADRs: [0003](0003-dual-handler-collapse.md), [0004](0004-centralized-http-error-mapping.md), [0005](0005-deepen-webui-mapper.md), [0006](0006-decompose-main-into-server-package.md)
>
> Created: 2026-08-03

## Overview

Four architectural deepenings that reduce duplication, improve testability,
and prevent the pattern from compounding as 6 new web pages are added. Steps
1–5 are designed to be implemented together; Step 6 is a standalone cleanup
that can happen before or after.

```
Before:                           After:
┌──────────┐ ┌──────────┐        ┌──────────────────────┐
│ API      │ │ Web      │        │ HandlerCore          │
│ Handler  │ │ Handler  │        │ ┌──────────────────┐ │
│ ┌──────┐ │ │ ┌──────┐ │        │ │ Error mapping    │ │ ← httperr.MapHTTP
│ │mapErr│ │ │ │frmErr│ │        │ │ Pagination       │ │
│ │pagin.│ │ │ │pagin.│ │   →    │ │ UUID parsing     │ │
│ │UUID  │ │ │ │UUID  │ │        │ │ Validation       │ │
│ │serial│ │ │ │serial│ │        │ └──────────────────┘ │
│ └──────┘ │ │ └──────┘ │        │          ↓           │
│    ↓     │ │    ↓     │        │   JSONAdapter  TemplAdapter
│  Service │ │  Service │        └──────────────────────┘
└──────────┘ └──────────┘
```

## Sequence

### Step 1: Create `internal/httperr` package → [#31](https://github.com/hsrvms/agentic-demo/issues/31)

**Deliverable:** `internal/httperr/mapper.go` with `MapHTTP(err) → (status, message)`

1. Create the package with one function.
2. Migrate every case from the 7 `mapError` functions into `MapHTTP`.
3. Write `internal/httperr/mapper_test.go` that verifies every known sentinel
   error maps to a non-500 status.
4. Replace all `mapError` calls in API handlers with `httperr.MapHTTP`.
5. Replace the inline switch in `web/errors.go`'s `webErrorHandler` with
   `httperr.MapHTTP`.
6. Delete the 7 `mapError` functions.

**Files touched:**

- NEW: `internal/httperr/mapper.go`, `internal/httperr/mapper_test.go`
- MODIFY: `internal/sources/handler.go`, `internal/budget/handler.go`,
  `internal/usage/handler.go`, `internal/reports/handler.go`,
  `internal/scheduling/handler.go`, `cmd/server/main.go`,
  `internal/web/errors.go`

**Verification:** `go test ./internal/httperr/...` + `go test ./...` (all
existing handler tests pass with the new error mapping).

---

### Step 2: Create handler core for `sources` domain → [#34](https://github.com/hsrvms/agentic-demo/issues/34) (combined with Step 3)

**Deliverable:** `internal/sources/handler_core.go` with transport-agnostic
handler logic returning result structs.

1. Define result types:
    - `ListResult` — paginated list with total count
    - `GetResult` — single data source (with credentials)
    - `CreateResult` — created data source
    - `UpdateResult` — updated data source
    - `DeleteResult` — empty (success)
    - `TestConnectionResult` — connection test outcome
    - `SyncResult` — sync triggered

2. Implement core methods: `List(ctx, tenantID, page, pageSize) → ListResult, error`,
   `Get(ctx, id) → GetResult, error`, `Create(ctx, tenantID, params) → CreateResult, error`,
   `Update(ctx, id, params) → UpdateResult, error`, `Delete(ctx, id) → error`,
   `TestConnection(ctx, id) → TestConnectionResult, error`, `Sync(ctx, id) → error`.

3. Each method calls `sources.Service`, handles errors via `httperr.MapHTTP`,
   and returns a result struct. No `echo.Context`, no `http.ResponseWriter`,
   no `templ.Component`.

4. Write `internal/sources/handler_core_test.go` — tests the core directly
   with a mock service. No HTTP test server needed.

**Files touched:**

- NEW: `internal/sources/handler_core.go`, `internal/sources/handler_core_test.go`

**Verification:** `go test ./internal/sources/...`

---

### Step 3: Create JSON and Templ adapters for `sources` → [#34](https://github.com/hsrvms/agentic-demo/issues/34) (combined with Step 2)

**Deliverable:** `internal/sources/handler.go` (rewritten) and
`internal/web/sources.go` (rewritten) as thin adapters.

1. Rewrite `internal/sources/handler.go`:
    - Each handler method extracts params from `echo.Context` (UUID, form values),
      calls the handler core, and renders the result via `c.JSON`.
    - No error mapping, no pagination logic, no serialization logic.
    - ~30 lines per handler method → ~10 lines.

2. Rewrite `internal/web/sources.go`:
    - Each handler method extracts params from `echo.Context` (UUID, form values,
      CSRF token, flash messages), calls the handler core, and renders the result
      via `Render(c, status, templComponent)`.
    - HTMX detection stays in the adapter — checks `IsHTMX(c)` and renders
      fragment or full page.
    - Form-specific logic (`buildConfigAndCreds`, `extractConfigURL`) stays here.
    - ~60 lines per handler method → ~20 lines.

3. Update existing tests:
    - `internal/sources/handler_test.go` — adapt to new adapter signature.
    - `internal/web/sources_test.go` — adapt to new adapter signature.

**Files touched:**

- MODIFY: `internal/sources/handler.go`, `internal/sources/handler_test.go`,
  `internal/web/sources.go`, `internal/web/sources_test.go`

**Verification:** `go test ./internal/sources/... ./internal/web/...` +
`go build ./cmd/server` (server starts and routes work).

---

### Step 4: Deepen `internal/webui` package → [#32](https://github.com/hsrvms/agentic-demo/issues/32)

**Deliverable:** `internal/webui/mapper.go` and `internal/webui/mapper_test.go`

1. Move all conversion functions into `webui`:
    - `MapSourceItem(ds *sources.DataSource) SourceItem`
    - `MapSourceList(result sources.DataSourcePage) SourcesListData`
    - `MapDashboard(usage, reports, sources, budget) DashboardData`
    - Internal helpers: `sourceTypeLabel`, `statusIntent`, `formatTokens`,
      `BudgetIntent`, `countActiveSources`, `formatTimeAgo`, `prettyJSON`

2. Update `internal/web/sources.go` and `internal/web/dashboard.go` to call
   the mapper instead of inline conversion.

3. Write `internal/webui/mapper_test.go` — pure function tests, no mocks.

**Files touched:**

- NEW: `internal/webui/mapper.go`, `internal/webui/mapper_test.go`
- MODIFY: `internal/web/sources.go`, `internal/web/dashboard.go`

**Verification:** `go test ./internal/webui/... ./internal/web/...`

---

### Step 5: Apply pattern to next domain (scheduling) → [#36](https://github.com/hsrvms/agentic-demo/issues/36)

**Deliverable:** `internal/scheduling/handler_core.go` + updated adapters

Apply the same pattern to `scheduling`, which is the next domain being
exposed as a web page (issue #19). This validates the pattern generalizes
beyond `sources`.

**Files touched:**

- NEW: `internal/scheduling/handler_core.go`, `internal/scheduling/handler_core_test.go`
- MODIFY: `internal/scheduling/handler.go`

**Verification:** `go test ./internal/scheduling/...`

---

## Open issues mapping

Each open issue that adds a web page will use the deepened pattern from the start:

| Issue                  | Domain          | Pattern                                 |
| ---------------------- | --------------- | --------------------------------------- |
| #22 — Settings page    | budget + tenant | ✅ HandlerCore + TemplAdapter           |
| #21 — Invoices page    | budget          | ✅ HandlerCore + TemplAdapter           |
| #20 — Usage page       | usage           | ✅ HandlerCore + TemplAdapter           |
| #19 — Schedules page   | scheduling      | ✅ Already done in Step 5               |
| #18 — Report trigger   | reports         | ✅ HandlerCore + TemplAdapter           |
| #17 — Reports page     | reports         | ✅ HandlerCore + TemplAdapter           |
| #16 — File upload flow | sources         | ✅ Extends existing sources web adapter |

---

### Step 1b: Add API handlers for auth and tenant domains → [#33](https://github.com/hsrvms/agentic-demo/issues/33)

**Deliverable:** `internal/auth/handler.go` and `internal/tenant/handler.go`

Move the raw handler functions from `cmd/server/main.go` into proper domain
handler packages. This is a prefactoring for Step 6 (main.go decomposition).

**Files touched:**
- NEW: `internal/auth/handler.go`, `internal/auth/handler_test.go`
- NEW: `internal/tenant/handler.go`, `internal/tenant/handler_test.go`
- MODIFY: `cmd/server/main.go`

**Verification:** `go test ./internal/auth/... ./internal/tenant/...`

---

### Step 6: Decompose `cmd/server/main.go` into `internal/server` → [#35](https://github.com/hsrvms/agentic-demo/issues/35)

**Deliverable:** `internal/server/server.go` with `New(cfg) → (*Server, error)`

This step is defined in [ADR-0006](0006-decompose-main-into-server-package.md).

1. Move raw handler functions to domain packages:
    - `registerHandler`, `loginHandler`, `meHandler` → `internal/auth/handler.go`
    - `createTenantHandler`, `listTenantsHandler` → `internal/tenant/handler.go`
    - Request/response types move with them.

2. Create `internal/server/server.go`:
    - `New(cfg) → (*Server, error)` — wires database, services, handlers, middleware, routes.
    - `Server.Start(addr)` — starts the Echo server.
    - `Server.Shutdown(ctx)` — graceful shutdown with cleanup.

3. Reduce `cmd/server/main.go` to ~15 lines: load config, call `server.New`, start, wait for signal, shutdown.

4. Apply the same pattern to `cmd/worker/main.go` — extract `internal/worker` package.

**Files touched:**

- NEW: `internal/server/server.go`, `internal/server/server_test.go`
- NEW: `internal/auth/handler.go`, `internal/auth/handler_test.go`
- NEW: `internal/tenant/handler.go`, `internal/tenant/handler_test.go`
- NEW: `internal/worker/worker.go` (optional, same pattern for worker binary)
- MODIFY: `cmd/server/main.go`, `cmd/worker/main.go`

**Verification:** `go build ./cmd/server ./cmd/worker` + `go test ./internal/server/...`

---

## Risk & rollback

- **Risk**: The handler core adds an abstraction that may not fit every domain.
  **Mitigation**: Apply to `sources` first, validate, then generalize. If a
  domain's handler is too simple to justify the core, skip it.
- **Risk**: `httperr` imports every domain package, creating a dependency hub.
  **Mitigation**: `httperr` is a leaf package — nothing depends on it. If a
  domain adds a problematic dependency, its error mapping can live in a
  separate internal function called by `MapHTTP`.
- **Risk**: `internal/server` depends on every domain package, making it a
  dependency hub. **Mitigation**: This is the composition root — it's supposed
  to depend on everything. It has no callers except `cmd/server/main.go`.
- **Rollback**: Each step is independently revertible. `httperr` can be
  deleted and the 7 `mapError` functions restored. The handler core can be
  inlined back into the adapters. The `webui` mapper can be inlined back
  into the web handlers. `internal/server` can be inlined back into `main.go`.

## Success criteria

1. `go test -race ./...` passes with no regressions.
2. `golangci-lint run` passes.
3. `go build ./cmd/server ./cmd/worker` succeeds.
4. Adding a new error type requires updating exactly one case in one file.
5. Adding a new web page for a domain that already has an API handler
   requires only a TemplAdapter — the core is reused.
6. The `webui` package has tests that exercise the mapper functions directly.
