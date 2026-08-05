package tenant

import "errors"

var (
	ErrTenantNotFound     = errors.New("tenant not found")
	ErrAlreadyExists      = errors.New("membership already exists")
	ErrInvalidRole        = errors.New("invalid role: must be 'admin' or 'viewer'")
	ErrInvalidName        = errors.New("tenant name must not be empty")
	ErrMembershipNotFound = errors.New("membership not found")
	ErrRequiresAdmin      = errors.New("admin role required")
)
