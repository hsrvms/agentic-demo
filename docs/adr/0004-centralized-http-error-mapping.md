# Centralized HTTP error mapping via `internal/httperr` package

## Status

Proposed

## Context

Seven locations in the codebase implement the same pattern for mapping domain
errors to HTTP status codes and messages:

| File | Function | Errors covered |
|---|---|---|
| `internal/sources/handler.go` | `mapError` | 6 (NotFound, InvalidTenantID, InvalidName, InvalidSourceType, InvalidConfig, EncryptionFailed, DecryptionFailed) |
| `internal/budget/handler.go` | `mapError` | 4 (NotFound, InvalidTenantID, InvalidBudget, InvalidPeriod) |
| `internal/usage/handler.go` | `mapUsageError` | 2 (InvalidTenantID, InvalidDateRange) |
| `internal/reports/handler.go` | `mapReportError` | 2 (ReportNotFound, InvalidTenantID) |
| `internal/scheduling/handler.go` | `mapError` | 5 (ScheduleNotFound, ScheduleAlreadyExists, InvalidCronExpr, InvalidScheduleType, InvalidTenantID) |
| `cmd/server/main.go` | `mapAuthError`, `mapTenantError` | 5 (UserExists, InvalidCredentials, InvalidEmail, WeakPassword) + 4 (InvalidName, TenantNotFound, AlreadyExists, InvalidRole) |
| `internal/web/errors.go` | `webErrorHandler` | auth/tenant only (partial coverage) |

All follow the same pattern:

```go
func mapError(err error) *echo.HTTPError {
    switch {
    case errors.Is(err, ErrNotFound):
        return echo.NewHTTPError(http.StatusNotFound, err.Error())
    case errors.Is(err, ErrInvalidX):
        return echo.NewHTTPError(http.StatusBadRequest, err.Error())
    default:
        return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
    }
}
```

The web error handler (`internal/web/errors.go`) partially duplicates this
for auth/tenant errors but misses sources, budget, usage, reports, and
scheduling errors. When a sources error reaches the web error handler, it
falls through to the default 500 case.

Adding a new error type means updating 2–3 places: the domain's API handler,
the web handler (if it has one), and the centralized web error handler.

## Decision

Create a new leaf package `internal/httperr` with a single function:

```go
// MapHTTP converts a domain error to an HTTP status code and user-facing
// message. Every domain error the platform can produce has a case here.
func MapHTTP(err error) (status int, message string)
```

All API handlers and the web error handler call `httperr.MapHTTP(err)` instead
of their own inline `mapError` functions.

### Design choices

**Option A: Extend `web/errors.go`** — add more cases to the existing web
error handler.

Rejected: API handlers still need their own `mapError` functions. The web
error handler is only called for unhandled errors that propagate up to Echo's
error handler, not for errors the handler catches and converts inline.

**Option B: New `internal/httperr` package** — single registry function.

Chosen. One source of truth. Both API handlers and the web error handler use
the same function. Adding a new error type means updating one `case` in one
file.

**Option C: Domain errors implement an HTTPStatus interface** — each error
type carries its own HTTP status code.

Rejected. This couples domain errors to HTTP, violating the principle that
domain logic should not know about transport. A CLI tool or gRPC handler
wouldn't want HTTP status codes on domain errors.

### Package dependencies

`internal/httperr` imports every domain package (sources, budget, usage,
reports, scheduling, auth, tenant). It is a leaf package — nothing depends
on it. This is acceptable because:

- The imports are compile-time only — `httperr` has no runtime dependencies
  beyond the standard library and the domain error types.
- The alternative (each domain registering its errors with `httperr` via
  `init()`) is more complex and harder to trace.

### Convention

Every domain package defines sentinel errors (e.g., `sources.ErrNotFound`).
The `httperr` package maps each sentinel to a status code and message. When
a new sentinel error is added to a domain package, a corresponding case must
be added to `httperr.MapHTTP`. Tests in `httperr` verify that every known
sentinel maps to a non-500 status.

## Consequences

### Positive

- **Locality**: add an error type → update one case in one file
- **Leverage**: one function, 7+ call sites (API handlers + web error handler)
- **Consistency**: the same domain error always produces the same HTTP status
  code, regardless of which handler path it takes
- **Deletion test**: delete the 7 `mapError` functions; the logic lives in
  `httperr.MapHTTP`

### Negative

- `httperr` imports every domain package. If a domain package adds a
  dependency that `httperr` cannot import (e.g., a CGo library), the mapping
  for that domain must live elsewhere. This is hypothetical — no such
  dependency exists today.
- The `switch` statement grows linearly with error types. At 30+ cases,
  consider splitting by domain in internal helper functions while keeping the
  single `MapHTTP` entry point.

### Neutral

- The web error handler (`internal/web/errors.go`) is simplified: its inline
  switch on auth/tenant errors is replaced by a call to `httperr.MapHTTP`.
  The HTMX vs redirect logic remains unchanged.