# Repository-wide lint report

**Generated:** 2026-08-03
**Command:** `golangci-lint run`
**Configuration:** `.golangci.yaml`
**Result:** failed with **28 findings**

This report records the baseline repository-wide findings observed while
validating the server and worker composition-root changes. The findings are
outside the changed production files unless noted otherwise. The focused
changed-file check passed:

```text
golangci-lint run --new-from-rev 119f30c
0 issues.
```

## Summary by linter

| Linter | Findings |
|---|---:|
| `errcheck` | 2 |
| `gocritic` | 16 |
| `gosec` | 3 |
| `staticcheck` | 1 |
| `unparam` | 5 |
| `unused` | 1 |
| **Total** | **28** |

## Findings

### `errcheck` — 2

- `internal/usage/collector.go:272:13` — The return value of `fmt.Sscanf` is not checked while parsing an integer field.
- `internal/usage/collector.go:281:13` — The return value of `fmt.Sscanf` is not checked while parsing a floating-point field.

**Suggested action:** check the scan count and error, or centralize parsing in helpers that return an explicit parse failure.

### `gocritic` — 16

- `internal/knowledge/store_test.go:62:1` — `setupStore` has unnamed result types.
- `internal/knowledge/store_test.go:227:1` — `setupStoreWithEmbedder` has unnamed result types.
- `internal/llm/client.go:49:1` — `NewClient` has adjacent parameters with the same type that can be combined.
- `internal/llm/client.go:58:1` — `NewClientWithBudget` has adjacent parameters with the same type that can be combined.
- `internal/queue/handler.go:31:44` — `HandlerDeps` is passed by value despite being a large parameter.
- `internal/queue/server.go:32:40` — `HandlerDeps` is passed by value despite being a large parameter.
- `internal/reports/agent_loop.go:161:15` — An `append` result is assigned to a different variable instead of the original slice.
- `internal/reports/prompts.go:41:2` — A range loop copies a 128-byte value on each iteration.
- `internal/reports/worker.go:62:2` — A local variable named `context` shadows the imported package.
- `internal/usage/handler_test.go:27:1` — Adjacent same-type parameters can be combined.
- `internal/usage/handler_test.go:51:9` — `http.NoBody` should be used instead of a nil request body.
- `internal/usage/redis_emitter_test.go:278:44` — A test comment is detected as commented-out code.
- `internal/usage/redis_emitter_test.go:279:45` — A test comment is detected as commented-out code.
- `internal/usage/redis_emitter_test.go:280:45` — A test comment is detected as commented-out code.
- `internal/usage/service.go:28:1` — Adjacent same-type parameters can be combined.
- `internal/usage/service.go:120:1` — `parseDateRange` has unnamed result types.

**Suggested action:** address straightforward style findings first; review the
`append` and by-value dependency findings for semantic/performance impact
before changing public or package-level APIs.

### `gosec` — 3

- `internal/knowledge/store.go:108:19` — Conversion from `int` to `int32` may overflow when assigning the vector query limit.
- `internal/usage/collector.go:202:27` — Conversion from `int64` to `int32` may overflow for `ToolCalls`.
- `internal/usage/collector.go:205:28` — Conversion from `int64` to `int32` may overflow for `ReportsGenerated`.

**Suggested action:** validate or clamp values before narrowing conversions, or
use a wider domain/database type where appropriate.

### `staticcheck` — 1

- `internal/reports/prompts.go:42:3` — Prefer `fmt.Fprintf` over `WriteString(fmt.Sprintf(...))`.

**Suggested action:** replace the formatting/write sequence with `fmt.Fprintf` and handle its returned error according to the writer contract.

### `unparam` — 5

- `internal/knowledge/store_test.go:45:17` — `unitVector`'s `dim` parameter is always `1024`.
- `internal/knowledge/store_test.go:199:24` — `connectAndMigrate` receives an unused `*testing.T` parameter.
- `internal/reports/agent_loop.go:156:94` — `AgentLoop.synthesize` receives an unused `tenantID` parameter.
- `internal/usage/handler_test.go:48:31` — `setupEcho` returns an unused `*echo.Echo` value.
- `internal/usage/redis_emitter_test.go:15:62` — `setupRedis` returns an unused `*miniredis.Miniredis` value.

**Suggested action:** remove unused parameters/results where they do not
serve an intentional test or interface seam. Preserve parameters required by
an interface or planned extension and document/suppress the finding narrowly.

### `unused` — 1

- `internal/usage/repository.go:173:6` — `pgUUIDBytes` is unused.

**Suggested action:** remove the dead helper unless it is intended for an
upcoming query implementation.

## Priority and ownership

### High priority

1. The three `gosec` narrowing-conversion findings, because malformed or
   unexpectedly large values could truncate.
2. The two unchecked `fmt.Sscanf` errors, because malformed usage data is
   currently handled implicitly.
3. The unused `pgUUIDBytes` helper, because it is dead code and has no callers.

### Medium priority

- The `appendAssign` finding should be reviewed to confirm whether the current
  copy-on-append behavior is intentional.
- The `HandlerDeps` by-value findings should be evaluated together before
  changing constructor signatures.
- The unused `tenantID` parameter should be checked against the report-agent
  contract before removal.

### Low priority / mechanical cleanup

- Combined same-type parameters.
- Named result types.
- `http.NoBody`.
- Variable shadowing.
- `fmt.Fprintf` simplification.
- Test helper result/parameter cleanup.
- Comments flagged as commented-out code.

## Scope note

This is a baseline report, not a claim that all findings are defects. Several
are style or maintainability suggestions, while the `gosec` and `errcheck`
findings warrant behavioral review. No repository-wide lint findings were
introduced by the composition-root changes according to the focused
`--new-from-rev` run.
