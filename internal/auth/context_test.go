package auth

import (
	"context"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
)

func TestGetTenantID_SetAndRetrieve(t *testing.T) {
	ctx := SetTenantID(context.Background(), "t-abc")
	got := GetTenantID(ctx)
	if got != "t-abc" {
		t.Errorf("GetTenantID = %q, want %q", got, "t-abc")
	}
}

func TestGetTenantID_NotSet(t *testing.T) {
	got := GetTenantID(context.Background())
	if got != "" {
		t.Errorf("GetTenantID = %q, want empty", got)
	}
}

func TestGetUserID_SetAndRetrieve(t *testing.T) {
	id := uuid.New()
	ctx := SetUserID(context.Background(), id)
	got := GetUserID(ctx)
	if got != id {
		t.Errorf("GetUserID = %v, want %v", got, id)
	}
}

func TestGetUserID_NotSet(t *testing.T) {
	got := GetUserID(context.Background())
	if got != uuid.Nil {
		t.Errorf("GetUserID = %v, want nil", got)
	}
}

func TestContextKeys_Independent(t *testing.T) {
	id := uuid.New()
	ctx := context.Background()
	ctx = SetTenantID(ctx, "t-xyz")
	ctx = SetUserID(ctx, id)

	if got := GetTenantID(ctx); got != "t-xyz" {
		t.Errorf("GetTenantID = %q, want %q", got, "t-xyz")
	}
	if got := GetUserID(ctx); got != id {
		t.Errorf("GetUserID = %v, want %v", got, id)
	}
	// Verify domain.TenantID type.
	_ = domain.TenantID("test")
}
