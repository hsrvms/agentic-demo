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

// SourceItem is a single row in the sources list table.
type SourceItem struct {
	ID             string
	Name           string
	SourceType     string
	SourceLabel    string
	Status         string
	StatusIntent   string // "success", "warning", "error", "muted"
	LastSyncAt     string
	LastSyncStatus string
}

// SourcesListData is the view model for the sources list page.
type SourcesListData struct {
	TenantName string
	Sources    []SourceItem
	TotalCount int
	Page       int
	PageSize   int
	HasMore    bool
	NextPage   int
}

// SourceTypeOption is an option in the source type selector.
type SourceTypeOption struct {
	Value string
	Label string
}

// SourceFormData is the view model for the create/edit source form.
type SourceFormData struct {
	Editing     bool
	SourceID    string
	Name        string
	SourceType  string
	ConfigURL   string
	ConfigAPIKey string
	CSRFToken   string
	TypeOptions []SourceTypeOption
}

// SourceDetailData is the view model for the source detail page.
type SourceDetailData struct {
	Source         SourceItem
	ConfigJSON     string
	HasCredentials bool
}

// ReportItem is a single row in the reports list table.
type ReportItem struct {
	ID          string
	Type        string
	TypeLabel   string
	TypeIntent  string // "info", "primary", "warning"
	Title       string
	Focus       string
	GeneratedAt string
}

// ReportListData is the view model for the reports list page.
type ReportListData struct {
	Reports    []ReportItem
	TotalCount int
	Page       int
	PageSize   int
	HasMore    bool
	NextPage   int
}

// Citation is a single source reference attached to a generated report.
type Citation struct {
	Title string
	URL   string
	Source string
}

// ReportDetailData is the view model for the report detail page.
type ReportDetailData struct {
	ID          string
	Title       string
	Type        string
	TypeLabel   string
	TypeIntent  string
	Focus       string
	GeneratedAt string
	ContentHTML string
	Citations   []Citation
}
