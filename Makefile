.PHONY: dev dev-server dev-cli build build-server build-cli build-all run run-server run-cli \
        test test-cover test-short lint lint-fix generate tidy vet clean \
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

BIN_DIR     := tmp
SERVER_BIN  := $(BIN_DIR)/server
CLI_BIN     := $(BIN_DIR)/cli
MIGRATE_DIR := sql/migrations

# ── Development (live reload) ───────────────────────────────────────
dev: dev-server

dev-server:
	air --build.cmd "go build -o $(SERVER_BIN) ./cmd/server" --build.bin "$(SERVER_BIN)"

dev-cli:
	air --build.cmd "go build -o $(CLI_BIN) ./cmd/cli" --build.bin "$(CLI_BIN)"

# ── Build ───────────────────────────────────────────────────────────
build: build-server build-cli

build-server:
	go build -o $(SERVER_BIN) ./cmd/server

build-cli:
	go build -o $(CLI_BIN) ./cmd/cli

build-all:
	go build ./...

# ── Run ─────────────────────────────────────────────────────────────
run-server: build-server
	$(SERVER_BIN)

run-cli: build-cli
	$(CLI_BIN) $(ARGS)

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
	rm -rf $(BIN_DIR)/ coverage.out coverage.html