// Package testutil provides shared infrastructure for integration tests.
package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartPostgres starts an ephemeral pgvector-enabled PostgreSQL instance,
// applies every migration, and registers container cleanup with t.
//
// Tests are skipped only when Docker is unavailable. A running PostgreSQL
// instance is deliberately not used as a fallback: integration tests must not
// depend on developer credentials or mutate a shared database.
func StartPostgres(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "pgvector/pgvector:pg16",
			Env: map[string]string{
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpass",
				"POSTGRES_DB":       "testdb",
			},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
				wait.ForListeningPort("5432/tcp"),
			),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("integration test requires Docker: %v", err)
	}

	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Errorf("terminate PostgreSQL container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get PostgreSQL container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("get PostgreSQL container port: %v", err)
	}

	connString := fmt.Sprintf("postgres://testuser:testpass@%s:%s/testdb?sslmode=disable", host, port.Port())
	applyMigrations(t, connString)
	return connString
}

func applyMigrations(t *testing.T, connString string) {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test helper")
	}
	migrationDir := filepath.Join(filepath.Dir(sourceFile), "..", "..", "sql", "migrations")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		t.Fatalf("connect to PostgreSQL container: %v", err)
	}
	defer pool.Close()

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		migrationPath := filepath.Join(migrationDir, entry.Name())
		sql, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", entry.Name(), err)
		}
	}
}
