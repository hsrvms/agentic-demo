package storage

import (
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
)

// tenantKeyPrefix returns the storage prefix for a tenant's objects.
func tenantKeyPrefix(tenantID domain.TenantID) string {
	return "tenant/" + string(tenantID) + "/"
}

// objectKey builds the full storage key for a tenant-scoped object.
//
// The caller supplies a key relative to the tenant; the store prefixes it with
// tenant/{tenantID}/ so cross-tenant access is impossible by construction. Keys
// that could escape the tenant namespace (empty, absolute, tenant-prefixed, or
// containing a path-traversal segment) are rejected.
//
// This is the single enforcement point for tenant scoping: every Put, Get, and
// Delete call runs through it.
func objectKey(tenantID domain.TenantID, key string) (string, error) {
	if key == "" {
		return "", ErrInvalidKey
	}
	if strings.HasPrefix(key, "/") || strings.HasPrefix(key, "tenant/") {
		return "", ErrInvalidKey
	}
	for _, part := range strings.Split(key, "/") {
		if part == ".." {
			return "", ErrInvalidKey
		}
	}
	return tenantKeyPrefix(tenantID) + key, nil
}
