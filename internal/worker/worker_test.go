package worker

import (
	"testing"

	"github.com/agentic-demo/platform/internal/config"
	"github.com/agentic-demo/platform/internal/testutil"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestNewWorker_ShutdownIsIdempotent(t *testing.T) {
	redisServer := miniredis.RunT(t)

	cfg := config.Config{
		DatabaseURL:   testutil.StartPostgres(t),
		RedisURL:      redisServer.Addr(),
		JWTSecret:     "test-jwt-secret",
		EncryptionKey: "test-encryption-key",
	}

	worker, err := New(cfg)
	require.NoError(t, err)

	require.NoError(t, worker.Shutdown())
	require.NoError(t, worker.Close())
}
