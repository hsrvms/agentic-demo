package server

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestServer_StartsHealthAndRoutes(t *testing.T) {
	redisServer := miniredis.RunT(t)

	cfg := config.Config{
		DatabaseURL:   databaseURLForTest(),
		RedisURL:      redisServer.Addr(),
		JWTSecret:     "test-jwt-secret",
		EncryptionKey: "test-encryption-key",
	}

	srv, err := New(cfg)
	if err != nil {
		t.Skipf("integration test requires PostgreSQL: %v", err)
	}
	defer srv.Close()

	startErr := make(chan error, 1)
	go func() {
		startErr <- srv.Start("127.0.0.1:0")
	}()

	addr := waitForAddress(t, srv)
	client := &http.Client{Timeout: time.Second}

	t.Run("health", func(t *testing.T) {
		resp := getWithRetry(t, client, "http://"+addr+"/health")
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, `{"status":"ok"}
`, string(body))
	})

	t.Run("web route", func(t *testing.T) {
		resp := getWithRetry(t, client, "http://"+addr+"/login")
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("api route", func(t *testing.T) {
		resp := getWithRetry(t, client, "http://"+addr+"/api/tenants")
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, srv.Shutdown(shutdownCtx))

	select {
	case err := <-startErr:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("server did not stop after shutdown")
	}
}

func databaseURLForTest() string {
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		return databaseURL
	}
	return "postgres://platform:platform@localhost:5432/platform?sslmode=disable"
}

func waitForAddress(t *testing.T, srv *Server) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if addr := srv.Addr(); addr != "" {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not bind to an address")
	return ""
}

func getWithRetry(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
		if err != nil {
			lastErr = err
			time.Sleep(10 * time.Millisecond)
			continue
		}
		resp, err := client.Do(req)
		if err == nil {
			return resp
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("GET %s: %v", url, lastErr)
	return nil
}
