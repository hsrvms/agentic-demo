package auth

import (
	"context"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
)

type contextKey int

const (
	tenantIDKey contextKey = iota
	userIDKey
)

// SetTenantID stores the tenant ID in the context.
func SetTenantID(ctx context.Context, tenantID domain.TenantID) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// GetTenantID extracts the tenant ID from the context.
// Returns empty string if not set.
func GetTenantID(ctx context.Context) domain.TenantID {
	v, ok := ctx.Value(tenantIDKey).(domain.TenantID)
	if !ok {
		return ""
	}
	return v
}

// SetUserID stores the user ID in the context.
func SetUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// GetUserID extracts the user ID from the context.
// Returns uuid.Nil if not set.
func GetUserID(ctx context.Context) uuid.UUID {
	v, ok := ctx.Value(userIDKey).(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return v
}
