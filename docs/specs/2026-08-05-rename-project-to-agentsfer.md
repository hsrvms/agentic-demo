# Rename Project to "agentsfer"

## Problem Statement

The platform is currently identified by the name **agentic-demo** / **Agentic Demo**. This
identity is scattered across four distinct surfaces, each with different semantics:

1. **The Go module path** — `module github.com/agentic-demo/platform`, which all ~200 package
   imports across `internal/`, `cmd/`, and `web/templates/` reference.
2. **The API tooling brand** — the Bruno collection and environment are named
   "Agentic Demo - Auth + RLS" / "Agentic-Demo".
3. **The Git repository** — the remote is `git@github.com:hsrvms/agentic-demo.git`.
4. **Internal docs** — the design skill and agent conventions reference the old module path.

The user wants the project to be known as **agentsfer**. The current name is a demo placeholder
and no longer reflects the product's identity.

## Solution

Rename the platform to **agentsfer** across all surfaces where the name is a project identifier.
The rename is a mechanical refactor — it changes identity, not behaviour. No domain entities
(Tenant, DataSource, Report, AgentLoop, etc.) are affected, and no runtime behaviour changes.

The rename is executed in four parallel tracks mirroring the four surfaces above, each with its
own completion criteria, then verified by the full build/test suite.

## User Stories

1. As a maintainer, I want the Go module path to read `agentsfer`, so that the module's identity
   matches the product name and new imports resolve under it.
2. As a maintainer, I want every package import to be updated consistently, so that the codebase
   compiles and `go vet` / `golangci-lint` pass with no stale references.
3. As a maintainer, I want the generated `_templ.go` files to be regenerated (or bulk-edited) so
   that compiled template code does not import the old module path.
4. As a maintainer, I want the Bruno collection and environment renamed, so that the API tooling
   reflects the product name.
5. As a maintainer, I want the design skill and agent conventions updated, so that internal
   documentation stays consistent with the new name.
6. As a maintainer, I want `rg -i "agentic"` (excluding git history and the remote) to return no
   results, so that no stale project-identity reference remains.
7. As a maintainer, I want the Git remote to point at the renamed repository, so that pushes and
   pulls continue to work after the GitHub rename.
8. As a maintainer, I want the rename committed as a dedicated commit, so that it is atomic and
   easy to review/revert without entangling unrelated work.

## Implementation Decisions

The change is a rename, not a feature; there are no new modules or interfaces. The relevant
**module** is the platform itself (its Go module, its tooling, its docs), and the **seam** at
which correctness is verified is the build/verification boundary described under Testing.

### Module path (provisional)

- **Decided:** the new module path is **`github.com/hsrvms/agentsfer`** — derived from the Git
  remote (`git@github.com:hsrvms/agentic-demo.git`) with the repo segment renamed and the "demo"
  dropped. The last path segment is `agentsfer`.
- **Open question (confirm before implementation):** whether the module path should instead be
  `github.com/agentsfer/platform` (preserving the current `/<owner>/platform` shape) or a bare
  `agentsfer`. The current module path (`github.com/agentic-demo/platform`) does not match the
  repo owner (`hsrvms`), so either shape is defensible. This must be settled before any edits.

### Brand name

- **Decided:** the display/brand name is **agentsfer** (unchanged casing where the source is
  code-identifiers, e.g. the Bruno environment file stays `agentsfer`). The Bruno collection name
  becomes `agentsfer - Auth + RLS`, preserving the existing "Auth + RLS" suffix.
- **Open question:** whether the brand should be title-cased ("Agentsfer") in human-facing spots.
  When ambiguous, default to lowercase `agentsfer`.

### Scope of edits, by track

1. **Module path** — update `go.mod`, then replace the module path in every `.go` file's imports
   (`internal/`, `cmd/`), every `.templ` file, and every generated `*_templ.go` file under
   `web/templates/`. Regenerate templates with `templ generate` where the toolchain is available;
   otherwise bulk-replace in the generated files too, since those import under the module path.
2. **Tooling brand** — update `bruno/bruno.json`, `bruno/collection.bru`, `bruno/README.md`, and
   rename the environment file `bruno/environments/Agentic-Demo.bru` → `agentsfer.bru` (update the
   four references in `AGENTS.md` and `bruno/README.md` accordingly).
3. **Git remote** — rename the GitHub repository (manual, outside the repo) and update the remote
   URL to `git@github.com:hsrvms/agentsfer.git`. This is a separate, manual step.
4. **Internal docs** — update the module-path import example in
   `.pi/skills/web-design-guide/component-patterns.md`.

### Non-goals

- No domain vocabulary changes. Terms in `CONTEXT.md` (Tenant, Report, AgentLoop, KnowledgeBase,
  etc.) are unaffected.
- No runtime behaviour, configuration semantics, database schema, or API contract changes. The
  `S3_BUCKET=platform` value is a bucket name, not the project name, and is left unchanged.

## Testing Decisions

There is no new runtime behaviour, so there are no new behavioural seams to test. The relevant
**seam** is the verification boundary of the build itself: the codebase is correct iff it compiles
and behaves identically under the new identity.

- **Primary verification:** the whole verification suite passes after the rename —
  `go mod tidy`, `go build ./...`, `go vet ./...`, `golangci-lint run`, and `go test -race ./...`.
  These replace hand-written tests as the acceptance check, because a rename with no behaviour
  change has no independent behavioural expectation to assert.
- **Stale-reference check:** `rg -i "agentic"` over the working tree (excluding `.git` and the
  remote config) returns no results. This is the rename's equivalent of a tautology test — it
  asserts the one thing that must change, from an independent source (the old name preserved in
  git history).
- **Prior art:** the repo's existing `go test -race ./...` / `golangci-lint run` gate in the
  project conventions is the standard here; the rename must not regress it.

## Out of Scope

- Renaming any domain entities, database tables, S3 buckets, or infrastructure identifiers.
- Changing the externally-visible product behaviour or UI wording beyond the project's own
  identity.
- Migrating the GitHub repository contents or history (rename is done in place on the existing
  repo).
- Touching committed history — the rename is a new commit, and git history is not rewritten.

## Further Notes

- **Confirmation required:** the exact module path (see Implementation Decisions) must be agreed
  before the mechanical edit runs, because it drives every import rewrite.
- **External dependency:** the GitHub repository rename must happen (or a new remote be created)
  for the remote-update track to complete; this cannot be done purely from the repo.
- **Risk of partial edit:** bulk import rewrites are the main risk. The stale-reference grep and
  the full verification suite are the guards against a partial rename slipping through.
- **Generated files:** `*_templ.go` files are committed and checked into the repo, so they must be
  regenerated or edited in the same change; hand-editing them is acceptable here because the
  change is a uniform path replacement, but `templ generate` is preferred where available.