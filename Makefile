.PHONY: dev-db stop-db migrate build run-cli test

dev-db:
	docker compose up -d

stop-db:
	docker compose down

migrate:
	PGPASSWORD=platform psql -h localhost -U platform -d platform -f internal/db/migrations/001_initial.sql

build:
	go build ./...

run-cli:
	go run ./cmd/cli -file examples/demo.txt

test:
	go test ./... -v

tidy:
	go mod tidy

vet:
	go vet ./...
