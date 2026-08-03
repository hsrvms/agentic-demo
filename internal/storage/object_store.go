package storage

import (
	"context"
	"io"

	"github.com/agentic-demo/platform/internal/domain"
)

// ObjectStore is a tenant-scoped object store.
//
// Every operation is scoped to a TenantID. Keys are relative to the tenant and
// are stored under the prefix tenant/{tenantID}/. The store derives the full
// key from the tenant ID and rejects keys that would escape the prefix, so a
// caller cannot read or write another tenant's objects.
type ObjectStore interface {
	// Put stores the object at the tenant-scoped key. size is the exact length
	// of r in bytes (0 when unknown).
	Put(ctx context.Context, tenantID domain.TenantID, key string, r io.Reader, size int64) error

	// Get returns the object at the tenant-scoped key. The caller must close the
	// returned reader. Returns ErrNotFound when the object does not exist.
	Get(ctx context.Context, tenantID domain.TenantID, key string) (io.ReadCloser, error)

	// Delete removes the object at the tenant-scoped key. Removing a
	// non-existent object is not an error.
	Delete(ctx context.Context, tenantID domain.TenantID, key string) error
}
