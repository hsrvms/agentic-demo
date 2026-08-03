# Collapse dual-handler pattern into shared handler core with transport adapters

## Status

Proposed

## Context

Every domain module that exposes both a JSON API and an HTML web page today
has two separate handlers:

- `internal/sources/handler.go` — JSON API handler
- `internal/web/sources.go` — HTML web handler

Both call the same `sources.Service` interface but duplicate:

- Error mapping (switch on `errors.Is` → `echo.NewHTTPError` or flash message)
- Pagination normalization (page/pageSize defaults, offset calculation)
- UUID parsing from path parameters
- Tenant ID extraction from context
- Serialization (JSON `map[string]interface{}` vs templ component rendering)

This pattern is about to compound. Six open issues (#16–#22) will add web pages
for settings, invoices, usage, reports, schedules, and file upload — each
creating a new `web/*.go` file that duplicates the same orchestration from the
domain's API handler.

The DashboardHandler (`internal/web/dashboard.go`) exhibits a related but
distinct pattern: it calls four services (usage, reports, sources, budget),
swallows errors from each, and assembles a view model. Its formatting helpers
(`formatTokens`, `BudgetIntent`, `sourceTypeLabel`, `countActiveSources`) are
inline in the handler and will be addressed separately (see ADR-0005).

## Decision

Extract a **shared handler core** for each domain that sits between the
service interface and the transport. The core owns:

- Input validation and parsing (UUID, pagination, tenant ID)
- Service orchestration (calling the service, handling errors)
- Result construction (a structured, transport-agnostic result type)

Two **transport adapters** then render the result:

- **JSONAdapter** — renders the result as JSON via `echo.Context.JSON`
- **TemplAdapter** — renders the result as HTML via `templ.Component`

```
Echo routes → HandlerCore → domain.Service → repository → DB
                  ↓
           JSONAdapter / TemplAdapter
                  ↓
           echo.Context (JSON or HTML)
```

The handler core is transport-agnostic: it does not import `echo`, `templ`,
or `net/http`. It returns a result struct. Errors are mapped to HTTP status
codes by a centralized `httperr` package (see ADR-0004).

### What goes in the core vs adapter

| Concern | Handler Core | Transport Adapter |
|---|---|---|
| UUID parsing from path params | ✓ | |
| Pagination normalization | ✓ | |
| Tenant ID extraction | ✓ | |
| Service call + error handling | ✓ | |
| Error → HTTP status mapping | ✓ (via httperr) | |
| JSON serialization | | ✓ (JSONAdapter) |
| HTML rendering | | ✓ (TemplAdapter) |
| HTMX fragment vs full page | | ✓ (TemplAdapter) |
| Form parsing (multipart, urlencoded) | | ✓ (TemplAdapter) |
| Flash messages | | ✓ (TemplAdapter) |
| CSRF token injection | | ✓ (TemplAdapter) |

### Scope

Apply to `sources` first as the pattern has the most duplication there.
Then apply to `schedules`, `reports`, `usage`, `budget`, `invoices`, and
`settings` as each web page is added (per the open issues).

The `DashboardHandler` is excluded from this ADR — it calls multiple services
and its formatting helpers are addressed in ADR-0005.

### What the handler core does NOT own

- **Form-specific logic** (`buildConfigAndCreds`, `extractConfigURL`) — these
  live in the web adapter. The API adapter receives JSON, the web adapter
  receives form values. The core operates on already-parsed domain types.
- **HTMX detection** — the adapter checks `IsHTMX(c)` and renders the
  appropriate fragment or full page. The core doesn't know about HTMX.
- **Flash messages and CSRF** — these are web-only concerns and stay in the
  web adapter.

## Consequences

### Positive

- **Leverage**: one handler core, N transport adapters. Adding a new transport
  (e.g., XML, gRPC, CLI) requires only a new adapter, not a new handler.
- **Locality**: error mapping, pagination, and validation live in one place
  per domain instead of two.
- **Testability**: the handler core is testable without an HTTP server. Tests
  exercise the core through its result struct, not through `httptest.ResponseRecorder`.
- **Deletion test**: delete the API handler and web handler; the orchestration
  logic reappears in the core, not in two separate files.

### Negative

- Adds one abstraction layer. The core + adapter pattern is more files than
  a single handler, though the total line count decreases (duplication removed).
- The core's result struct is an additional type to learn. Callers must
  understand both the domain types and the result type.
- For simple handlers (e.g., `GET /health`), the pattern is overkill. Apply
  judgment — not every handler needs to be deepened.

### Neutral

- Existing `service_test.go` tests survive unchanged. Existing `handler_test.go`
  tests need to be rewritten to exercise the core instead of `httptest`.
- The `web/sources_test.go` tests need to adapt to the new adapter signature.