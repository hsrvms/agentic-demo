// Package webui holds shared types for the web UI layer.
// It is a leaf package with no dependencies on internal/web or templates,
// avoiding import cycles.
package webui

import "errors"

// ErrInvalidTime is returned when a schedule form supplies an unparseable
// "HH:MM" time of day.
var ErrInvalidTime = errors.New("invalid time of day")

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

// ReportTypeOption is an option in the report type selector.
type ReportTypeOption struct {
	Value string
	Label string
}

// GenerateReportForm is the view model for the generate-report modal form.
type GenerateReportForm struct {
	CSRFToken   string
	TypeOptions []ReportTypeOption
}

// SourceFormData is the view model for the create/edit source form.
type SourceFormData struct {
	Editing      bool
	SourceID     string
	Name         string
	SourceType   string
	ConfigURL    string
	ConfigAPIKey string
	CSRFToken    string
	TypeOptions  []SourceTypeOption
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
	Title  string
	URL    string
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

// ScheduleItem is a single row in the schedules list table.
type ScheduleItem struct {
	ID          string
	Type        string
	TypeLabel   string
	TypeIntent  string
	CronExpr    string
	CronHuman   string
	Focus       string
	Format      string
	FormatLabel string
	Enabled     bool
}

// ScheduleListData is the view model for the schedules list page.
type ScheduleListData struct {
	Schedules []ScheduleItem
}

// ScheduleTypeOption is an option in the schedule type selector.
type ScheduleTypeOption struct {
	Value string
	Label string
}

// ReportFormatOption is an option in the report format selector.
type ReportFormatOption struct {
	Value string
	Label string
}

// ScheduleFormData is the view model for the create/edit schedule form.
type ScheduleFormData struct {
	Editing       bool
	ScheduleID    string
	Type          string
	Time          string // "HH:MM" time of day
	DayOfWeek     string // cron weekday (0-6), only used for weekly
	DayOfMonth    string // "1"-"31", only used for monthly
	Focus         string
	Format        string
	CSRFToken     string
	TypeOptions   []ScheduleTypeOption
	FormatOptions []ReportFormatOption
}

// WeekdayOption is an option in the weekly day-of-week selector.
type WeekdayOption struct {
	Value string
	Label string
}

// UsageStatData is a single stat card on the usage page.
type UsageStatData struct {
	Label  string
	Value  string
	Intent string // "primary", "info", "warning", "success"
}

// UsageModelItem is a single per-model row in the usage breakdown table.
type UsageModelItem struct {
	Model           string
	InputTokens     string
	OutputTokens    string
	ToolCalls       string
	EmbeddingTokens string
	CostUSD         string
}

// UsageEventItem is a single row in the usage event log.
type UsageEventItem struct {
	ID         string
	EventType  string
	TypeLabel  string
	TypeIntent string // "info", "warning", "success"
	Summary    string
	CreatedAt  string
}

// UsageData is the view model for the usage page.
type UsageData struct {
	TenantName    string
	PeriodLabel   string
	HasData       bool
	Stats         []UsageStatData
	Models        []UsageModelItem
	Events        []UsageEventItem
	HasMoreEvents bool
	NextEventPage int
}

// UsageSummaryData is the view model for the usage summary fragment.
type UsageSummaryData struct {
	HasData       bool
	FromLabel     string
	ToLabel       string
	CostFormatted string
	Models        []UsageModelItem
}

// InvoiceItem is a single row in the invoices list table.
type InvoiceItem struct {
	ID           string
	PeriodLabel  string
	Status       string
	StatusLabel  string
	StatusIntent string // "muted", "info", "success", "error"
	TotalCostUSD string
}

// InvoiceListData is the view model for the invoices list page.
type InvoiceListData struct {
	TenantName string
	Invoices   []InvoiceItem
	TotalCount int
	Page       int
	PageSize   int
	HasMore    bool
	NextPage   int
}

// InvoiceLineItem is a single per-model line on the invoice detail page.
type InvoiceLineItem struct {
	Model            string
	InputTokens      string
	OutputTokens     string
	ToolCalls        string
	EmbeddingTokens  string
	ReportsGenerated string
	CostUSD          string
}

// InvoiceDetailData is the view model for the invoice detail page.
type InvoiceDetailData struct {
	ID           string
	PeriodLabel  string
	PeriodStart  string
	PeriodEnd    string
	Status       string
	StatusLabel  string
	StatusIntent string
	TotalCostUSD string
	LineItems    []InvoiceLineItem
}

// SettingsData is the view model for the settings page.
type SettingsData struct {
	TenantName         string
	TenantID           string
	TenantStatus       string
	TenantStatusLabel  string
	TenantStatusIntent string // "success", "warning", "error"
	CreatedAt          string
	IsAdmin            bool
	MonthlyBudget      string
	MTDCost            string
	Remaining          string
	PercentUsed        float64
	BudgetIntent       string // "success", "warning", "error"
	BudgetExceeded     bool
	CSRFToken          string
}
