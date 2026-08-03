package worker

import (
	"os"
	"testing"

	"github.com/agentic-demo/platform/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestNewWorker_ShutdownIsIdempotent(t *testing.T) {
	redisServer := miniredis.RunT(t)

	cfg := config.Config{
		DatabaseURL:   databaseURLForTest(),
		RedisURL:      redisServer.Addr(),
		JWTSecret:     "test-jwt-secret",
		EncryptionKey: "test-encryption-key",
	}

	worker, err := New(cfg)
	if err != nil {
		t.Skipf("integration test requires PostgreSQL: %v", err)
	}

	require.NoError(t, worker.Shutdown())
	require.NoError(t, worker.Close())
}

func databaseURLForTest() string {
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		return databaseURL
	}
	return "postgres://platform:platform@localhost:5432/platform?sslmode=disable"
}
