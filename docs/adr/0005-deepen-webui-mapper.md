# Deepen the webui package into a view model mapper

## Status

Proposed

## Context

`internal/webui/types.go` defines view model types (`SourcesListData`,
`SourceItem`, `DashboardData`, `SourceFormData`, `Flash`) but no behavior.
The conversion logic — domain types → view model types — is spread across
the web handlers:

| File | Conversion logic |
|---|---|
| `internal/web/sources.go` | `toSourceItem`, `sourceTypeLabel`, `sourceTypeOptions`, `statusIntent`, `buildConfigAndCreds`, `extractConfigURL`, `prettyJSON`, `formatTimeAgo` |
| `internal/web/dashboard.go` | `formatTokens`, `BudgetIntent`, `countActiveSources` |
| `internal/web/auth.go` | `mapTenantError` (auth-specific, stays) |

The `webui` package is a pass-through: it defines types that are renamed
copies of domain types. Callers must learn about `webui.SourceItem` AND
`sources.DataSource`. The package adds indirection without leverage.

The deletion test: if `webui` were deleted, the conversion logic would
reappear in the web handlers where it already lives. The types are used
exactly once — in the templ template that renders them.

## Decision

**Deepen the `webui` package** by moving all domain-to-view-model conversion
logic into it. The package becomes a mapper module:

```go
// internal/webui/mapper.go
package webui

func MapSourceItem(ds *sources.DataSource) SourceItem { ... }
func MapSourceList(result sources.DataSourcePage) SourcesListData { ... }
func MapDashboard(usage *usage.CurrentUsage, reports int, sources int, budget *budget.BudgetStatus) DashboardData { ... }
```

The web handlers call the mapper instead of doing inline conversion. The
package now earns its keep: it hides the conversion complexity behind a
small interface of mapper functions.

### What moves in

- `toSourceItem` → `MapSourceItem`
- `sourceTypeLabel` → stays internal to `webui`
- `sourceTypeOptions` → `SourceTypeOptions()` (public)
- `statusIntent` → stays internal to `webui`
- `formatTokens` → stays internal to `webui`
- `BudgetIntent` → stays internal to `webui`
- `countActiveSources` → stays internal to `webui`
- `formatTimeAgo` → stays internal to `webui`
- `prettyJSON` → stays internal to `webui`

### What stays in the web handler

- `buildConfigAndCreds` — form parsing is a transport concern, not a mapping
  concern. It stays in the web adapter.
- `extractConfigURL` — also form-specific, stays in the web adapter.
- `mapTenantError` — auth-specific error mapping, stays in the auth handler
  (and is superseded by `httperr.MapHTTP` per ADR-0004).

### Why not inline the types into the web handlers?

The web handlers are already large. Moving the conversion logic into the
handlers would make them larger. The `webui` package provides a natural
home for the conversion logic — it's a leaf package with no dependencies
on `echo`, `templ`, or `net/http`, so it's testable in isolation.

The types themselves (`SourceItem`, `DashboardData`, etc.) are used by
templ templates. They need to exist somewhere. Keeping them in `webui`
with the mapper functions that produce them is cohesive.

## Consequences

### Positive

- **Leverage**: one `MapSourceItem` function, N call sites (list, detail,
  edit, create handlers)
- **Locality**: formatting logic concentrates in one module. Changing how
  token counts are formatted updates one function, not every handler.
- **Testability**: mapper functions are pure Go — inputs → outputs. No HTTP
  context, no templates, no database. Trivially testable.
- **Deletion test**: delete `webui`; the mapper logic reappears in callers,
  not in the package.

### Negative

- `webui` imports domain packages (`sources`, `budget`, `usage`). This is
  acceptable — `webui` is a leaf package at the presentation layer; nothing
  depends on it except templ templates.
- The mapper functions add one more hop in the call chain: handler → mapper
  → template instead of handler → template. The hop is justified by the
  consolidation of logic.

### Neutral

- The `webui` types (`SourceItem`, `DashboardData`) are unchanged. Templ
  templates continue to reference the same types.