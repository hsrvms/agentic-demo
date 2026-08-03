package storage

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestMemoryObjectStore_RoundTrip(t *testing.T) {
	store := NewMemoryObjectStore()
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

func TestMemoryObjectStore_GetMissingReturnsNotFound(t *testing.T) {
	store := NewMemoryObjectStore()
	ctx := context.Background()

	_, err := store.Get(ctx, domain.TenantID("tenant-a"), "sources/s1/file")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryObjectStore_DeleteRemovesObject(t *testing.T) {
	store := NewMemoryObjectStore()
	ctx := context.Background()
	tenant := domain.TenantID("tenant-a")

	content := []byte("data")
	err := store.Put(ctx, tenant, "sources/s1/file", bytes.NewReader(content), int64(len(content)))
	require.NoError(t, err)

	require.NoError(t, store.Delete(ctx, tenant, "sources/s1/file"))
	_, err = store.Get(ctx, tenant, "sources/s1/file")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryObjectStore_DeleteMissingIsNoop(t *testing.T) {
	store := NewMemoryObjectStore()
	ctx := context.Background()
	require.NoError(t, store.Delete(ctx, domain.TenantID("tenant-a"), "sources/s1/file"))
}

// TestMemoryObjectStore_TenantIsolation verifies that one tenant cannot read
// another tenant's object — the core cross-tenant guarantee.
func TestMemoryObjectStore_TenantIsolation(t *testing.T) {
	store := NewMemoryObjectStore()
	ctx := context.Background()

	secret := []byte("A's secret")
	err := store.Put(ctx, domain.TenantID("tenant-a"), "sources/s1/file", bytes.NewReader(secret), int64(len(secret)))
	require.NoError(t, err)

	// Reading the same relative key under a different tenant must not leak.
	_, err = store.Get(ctx, domain.TenantID("tenant-b"), "sources/s1/file")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryObjectStore_RejectsInvalidKey(t *testing.T) {
	store := NewMemoryObjectStore()
	ctx := context.Background()

	err := store.Put(ctx, domain.TenantID("tenant-a"), "sources/../../etc/passwd", bytes.NewReader([]byte("x")), 1)
	require.ErrorIs(t, err, ErrInvalidKey)

	_, err = store.Get(ctx, domain.TenantID("tenant-a"), "tenant/tenant-b/secret")
	require.ErrorIs(t, err, ErrInvalidKey)
}
