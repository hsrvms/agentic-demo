// Package auth provides context helpers that delegate to the domain package.
// These are kept for backward compatibility; new code should use domain.GetUserID
// and domain.GetTenantID directly.
package auth

import (
	"context"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
)

// SetTenantID stores the tenant ID in the context.
func SetTenantID(ctx context.Context, tenantID domain.TenantID) context.Context {
	return domain.SetTenantID(ctx, tenantID)
}

// GetTenantID extracts the tenant ID from the context.
func GetTenantID(ctx context.Context) domain.TenantID {
	return domain.GetTenantID(ctx)
}

// SetUserID stores the user ID in the context.
func SetUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return domain.SetUserID(ctx, userID)
}

// GetUserID extracts the user ID from the context.
func GetUserID(ctx context.Context) uuid.UUID {
	return domain.GetUserID(ctx)
}