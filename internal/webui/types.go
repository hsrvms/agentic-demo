// Package webui holds shared types for the web UI layer.
// It is a leaf package with no dependencies on internal/web or templates,
// avoiding import cycles.
package webui

// Flash represents a flash message shown once to the user.
type Flash struct {
	Intent  string // "success", "error", "warning", "info"
	Message string
}
