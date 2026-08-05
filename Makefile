.PHONY: dev dev-server dev-worker build build-server build-worker build-all run run-server run-worker \
        test test-fails test-cover test-short lint lint-fix generate tidy vet clean \
        migrate-up db-ping db-psql db-reset

# ── Environment ────────────────────────────────────────────────────
# Source .env into every recipe's shell, but never override variables
# already set in the environment (e.g. by docker-compose in devcontainer).
define ENV
set -a; [ -f .env ] && while IFS= read -r l; do \
	case "$$l" in \#*|"") continue;; esac; \
	eval "$$l"; \
done < .env; set +a;
endef

# ── Configuration ───────────────────────────────────────────────────
# These are fallbacks when no .env or environment variable is present.
PGHOST     ?= localhost
PGPORT     ?= 5432
PGUSER     ?= platform
PGPASSWORD ?= platform
PGDATABASE ?= platform

DATABASE_URL ?= postgres://$(PGUSER):$(PGPASSWORD)@$(PGHOST):$(PGPORT)/$(PGDATABASE)?sslmode=disable

BIN_DIR      := tmp
SERVER_BIN   := $(BIN_DIR)/server
WORKER_BIN   := $(BIN_DIR)/worker
MIGRATE_DIR  := sql/migrations

# ── Development (live reload) ───────────────────────────────────────
dev: dev-server

dev-server:
	air -c .air.server.toml

dev-worker:
	air -c .air.worker.toml

dev-all:
	@echo "Starting server and worker in parallel..."
	air -c .air.server.toml & air -c .air.worker.toml & wait

# ── Build ───────────────────────────────────────────────────────────
build: build-server build-worker

build-server:
	go build -o $(SERVER_BIN) ./cmd/server

build-worker:
	go build -o $(WORKER_BIN) ./cmd/worker

build-all:
	go build ./...

# ── Run ─────────────────────────────────────────────────────────────
run-server: build-server
	$(ENV) $(SERVER_BIN)

run-worker: build-worker
	$(ENV) $(WORKER_BIN)

# ── Test ────────────────────────────────────────────────────────────
test:
	gotestsum --format pkgname-and-test-fails --rerun-fails --packages="./..." -- -race ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "coverage report → coverage.html"

test-short:
	go test -short ./...

test-fails:
	gotestsum --format pkgname-and-test-fails -- ./internal/...

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
	$(ENV) for f in $$(ls $(MIGRATE_DIR)/*.sql 2>/dev/null | sort); do \
		echo "▶ $$(basename $$f)"; \
		psql "$${DATABASE_URL:-$(DATABASE_URL)}" -f "$$f" || exit 1; \
	done

db-ping:
	$(ENV) pg_isready -h "$${PGHOST:-$(PGHOST)}" -p "$${PGPORT:-$(PGPORT)}" -U "$${PGUSER:-$(PGUSER)}" -d "$${PGDATABASE:-$(PGDATABASE)}"

db-psql:
	$(ENV) psql "$${DATABASE_URL:-$(DATABASE_URL)}"

db-reset:
	@echo "This will drop and recreate the public schema."
	@read -p "Continue? [y/N] " ans && [ "$$ans" = "y" ] || exit 0
	$(ENV) psql "$${DATABASE_URL:-$(DATABASE_URL)}" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	$(MAKE) migrate-up

# ── Clean ───────────────────────────────────────────────────────────
clean:
	rm -rf $(BIN_DIR)/ coverage.out coverage.html
