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
	endpoint, accessKey, secretKey, useSSL := testutil.StartMinIO(t)

	cfg := config.Config{
		DatabaseURL:   testutil.StartPostgres(t),
		RedisURL:      redisServer.Addr(),
		JWTSecret:     "test-jwt-secret",
		EncryptionKey: "test-encryption-key",
		S3Endpoint:    endpoint,
		S3AccessKey:   accessKey,
		S3SecretKey:   secretKey,
		S3Region:      "us-east-1",
		S3Bucket:      "test-objects",
		S3UseSSL:      useSSL,
	}

	worker, err := New(cfg)
	require.NoError(t, err)

	require.NoError(t, worker.Shutdown())
	require.NoError(t, worker.Close())
}
