package storage

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/testutil"
	"github.com/stretchr/testify/require"
)

// setupS3 starts an ephemeral MinIO and returns a configured S3ObjectStore.
func setupS3(t *testing.T) *S3ObjectStore {
	t.Helper()
	endpoint, accessKey, secretKey, useSSL := testutil.StartMinIO(t)
	store, err := NewS3ObjectStore(&S3Config{
		Endpoint:  endpoint,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Region:    "us-east-1",
		Bucket:    "test-objects",
		UseSSL:    useSSL,
	})
	require.NoError(t, err)
	return store
}

func TestS3ObjectStore_RoundTrip(t *testing.T) {
	store := setupS3(t)
	ctx := context.Background()
	tenant := domain.TenantID("tenant-a")

	content := []byte("hello world")
	err := store.Put(ctx, tenant, "sources/s1/file", bytes.NewReader(content), int64(len(content)))
	require.NoError(t, err)

	rc, err := store.Get(ctx, tenant, "sources/s1/file")
	require.NoError(t, err)
	defer rc.Close()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, "hello world", string(data))
}

func TestS3ObjectStore_GetMissingReturnsNotFound(t *testing.T) {
	store := setupS3(t)
	ctx := context.Background()

	_, err := store.Get(ctx, domain.TenantID("tenant-a"), "sources/s1/file")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestS3ObjectStore_DeleteRemovesObject(t *testing.T) {
	store := setupS3(t)
	ctx := context.Background()
	tenant := domain.TenantID("tenant-a")

	content := []byte("data")
	err := store.Put(ctx, tenant, "sources/s1/file", bytes.NewReader(content), int64(len(content)))
	require.NoError(t, err)

	require.NoError(t, store.Delete(ctx, tenant, "sources/s1/file"))
	_, err = store.Get(ctx, tenant, "sources/s1/file")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestS3ObjectStore_TenantIsolation(t *testing.T) {
	store := setupS3(t)
	ctx := context.Background()

	secret := []byte("A's secret")
	err := store.Put(ctx, domain.TenantID("tenant-a"), "sources/s1/file", bytes.NewReader(secret), int64(len(secret)))
	require.NoError(t, err)

	// Reading the same relative key under a different tenant must not leak.
	_, err = store.Get(ctx, domain.TenantID("tenant-b"), "sources/s1/file")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestS3ObjectStore_RejectsInvalidKey(t *testing.T) {
	store := setupS3(t)
	ctx := context.Background()

	err := store.Put(ctx, domain.TenantID("tenant-a"), "sources/../../etc/passwd", bytes.NewReader([]byte("x")), 1)
	require.ErrorIs(t, err, ErrInvalidKey)

	_, err = store.Get(ctx, domain.TenantID("tenant-a"), "tenant/tenant-b/secret")
	require.ErrorIs(t, err, ErrInvalidKey)
}
