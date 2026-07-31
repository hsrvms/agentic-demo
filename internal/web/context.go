// Package web provides the web application layer: rendering, middleware,
// CSRF protection, and error handling for server-rendered HTML pages.
package web

import (
	"context"

	"github.com/agentic-demo/platform/internal/webui"
)

type contextKey int

const (
	userEmailKey contextKey = iota
	tenantNameKey
	flashMessagesKey
	csrfTokenKey
)

// SetUserEmail stores the current user's email in the context.
func SetUserEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, userEmailKey, email)
}

// GetUserEmail extracts the user email from the context.
func GetUserEmail(ctx context.Context) string {
	v, ok := ctx.Value(userEmailKey).(string)
	if !ok {
		return ""
	}
	return v
}

// SetTenantName stores the current tenant's display name in the context.
func SetTenantName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, tenantNameKey, name)
}

// GetTenantName extracts the tenant name from the context.
func GetTenantName(ctx context.Context) string {
	v, ok := ctx.Value(tenantNameKey).(string)
	if !ok {
		return ""
	}
	return v
}

// SetFlashMessages stores flash messages in the context.
func SetFlashMessages(ctx context.Context, msgs []webui.Flash) context.Context {
	return context.WithValue(ctx, flashMessagesKey, msgs)
}

// GetFlashMessages extracts flash messages from the context.
func GetFlashMessages(ctx context.Context) []webui.Flash {
	v, ok := ctx.Value(flashMessagesKey).([]webui.Flash)
	if !ok {
		return nil
	}
	return v
}

// SetCSRFToken stores the CSRF token in the context.
func SetCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfTokenKey, token)
}

// GetCSRFToken extracts the CSRF token from the context.
func GetCSRFToken(ctx context.Context) string {
	v, ok := ctx.Value(csrfTokenKey).(string)
	if !ok {
		return ""
	}
	return v
}
