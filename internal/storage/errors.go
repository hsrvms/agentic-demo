// Package storage implements the Object Store module.
//
// TenantScopedObjectStore sits behind every byte that crosses the ingestion
// pipeline. Every operation is scoped to a TenantID and stores objects under
// the prefix tenant/{tenantID}/, so no tenant can reach another tenant's
// objects by construction.
package storage

import "errors"

// Sentinel errors returned by ObjectStore implementations.
var (
	// ErrNotFound is returned when an object does not exist for the tenant.
	ErrNotFound = errors.New("object not found")

	// ErrInvalidKey is returned when a key is empty or could resolve outside
	// the tenant's namespace.
	ErrInvalidKey = errors.New("invalid object key")
)
