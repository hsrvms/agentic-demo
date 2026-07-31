package web

import (
	"context"
	"testing"

	"github.com/agentic-demo/platform/internal/webui"
	"github.com/stretchr/testify/assert"
)

func TestUserEmail(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, GetUserEmail(ctx))

	ctx = SetUserEmail(ctx, "alice@example.com")
	assert.Equal(t, "alice@example.com", GetUserEmail(ctx))
}

func TestTenantName(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, GetTenantName(ctx))

	ctx = SetTenantName(ctx, "Acme Corp")
	assert.Equal(t, "Acme Corp", GetTenantName(ctx))
}

func TestFlashMessages(t *testing.T) {
	ctx := context.Background()
	assert.Nil(t, GetFlashMessages(ctx))

	msgs := []webui.Flash{
		{Intent: "success", Message: "Saved!"},
		{Intent: "error", Message: "Failed!"},
	}
	ctx = SetFlashMessages(ctx, msgs)
	got := GetFlashMessages(ctx)
	assert.Len(t, got, 2)
	assert.Equal(t, "success", got[0].Intent)
	assert.Equal(t, "Failed!", got[1].Message)
}

func TestCSRFToken(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, GetCSRFToken(ctx))

	ctx = SetCSRFToken(ctx, "abc123")
	assert.Equal(t, "abc123", GetCSRFToken(ctx))
}
