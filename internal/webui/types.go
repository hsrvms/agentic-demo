// Package webui holds shared types for the web UI layer.
// It is a leaf package with no dependencies on internal/web or templates,
// avoiding import cycles.
package webui

// Flash represents a flash message shown once to the user.
type Flash struct {
	Intent  string // "success", "error", "warning", "info"
	Message string
}

// DashboardData is the view model passed to the dashboard template.
type DashboardData struct {
	TenantName      string
	TotalCostUSD    float64
	CostFormatted   string
	TotalTokens     int64
	TokensFormatted string
	ReportsCount    int
	ActiveSources   int
	BudgetPercent   float64
	BudgetIntent    string
	BudgetExceeded  bool
}
