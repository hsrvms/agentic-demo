.PHONY: dev build run test test-cover test-short lint lint-fix generate tidy vet clean \
        migrate-up db-ping db-psql db-reset

# ── Configuration ───────────────────────────────────────────────────
# In the devcontainer, PGHOST=postgres is set automatically via remoteEnv
# and DATABASE_URL is provided by docker-compose.  Locally, either set
# DATABASE_URL directly or override the PG* variables below.
PGHOST     ?= localhost
PGPORT     ?= 5432
PGUSER     ?= platform
PGPASSWORD ?= platform
PGDATABASE ?= platform

DATABASE_URL ?= postgres://$(PGUSER):$(PGPASSWORD)@$(PGHOST):$(PGPORT)/$(PGDATABASE)?sslmode=disable

BINARY      := bin/agentic-demo
CMD_DIR     := ./cmd/cli
MIGRATE_DIR := sql/migrations

# ── Development ─────────────────────────────────────────────────────
dev:
	air

# ── Build ───────────────────────────────────────────────────────────
build:
	go build -o $(BINARY) $(CMD_DIR)

build-all:
	go build ./...

# ── Run ─────────────────────────────────────────────────────────────
run:
	go run $(CMD_DIR)

# ── Test ────────────────────────────────────────────────────────────
test:
	go test -race ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "coverage report → coverage.html"

test-short:
	go test -short ./...

# ── Lint ────────────────────────────────────────────────────────────
lint:
	golangci-lint run
	go vet ./...

lint-fix:
	golangci-lint run --fix

# ── Code Generation ─────────────────────────────────────────────────
generate:
	templ generate
	sqlc generate

# ── Dependencies ────────────────────────────────────────────────────
tidy:
	go mod tidy

# ── Type Check ──────────────────────────────────────────────────────
vet:
	go vet ./...

# ── Database Migrations ─────────────────────────────────────────────
migrate-up:
	@for f in $$(ls $(MIGRATE_DIR)/*.sql 2>/dev/null | sort); do \
		echo "▶ $$(basename $$f)"; \
		psql "$$DATABASE_URL" -f "$$f" || exit 1; \
	done

db-ping:
	pg_isready -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) -d $(PGDATABASE)

db-psql:
	psql "$(DATABASE_URL)"

db-reset:
	@echo "This will drop and recreate the public schema."
	@read -p "Continue? [y/N] " ans && [ "$$ans" = "y" ] || exit 0
	psql "$(DATABASE_URL)" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	$(MAKE) migrate-up

# ── Clean ───────────────────────────────────────────────────────────
clean:
	rm -rf bin/ coverage.out coverage.html