// Package webui maps domain types into view model types for the web UI layer.
// It is a leaf package — no dependencies on echo, templ, net/http, or internal/web.
package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"time"

	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/yuin/goldmark"
)

// --- Source mappers ---

// MapSourceItem converts a domain DataSource into a view model SourceItem.
func MapSourceItem(ds *sources.DataSource) SourceItem {
	item := SourceItem{
		ID:             ds.ID.String(),
		Name:           ds.Name,
		SourceType:     string(ds.SourceType),
		SourceLabel:    sourceTypeLabel(ds.SourceType),
		Status:         string(ds.Status),
		StatusIntent:   statusIntent(ds.Status),
		LastSyncStatus: ds.LastSyncStatus,
	}
	if ds.LastSyncAt != nil {
		item.LastSyncAt = FormatTimeAgo(*ds.LastSyncAt)
	}
	return item
}

// MapSourceList converts a paginated DataSourcePage into a SourcesListData view model.
func MapSourceList(result sources.DataSourcePage) SourcesListData {
	items := make([]SourceItem, len(result.Sources))
	for i := range result.Sources {
		items[i] = MapSourceItem(&result.Sources[i])
	}
	return SourcesListData{
		Sources:    items,
		TotalCount: result.TotalCount,
		Page:       result.Page,
		PageSize:   result.PageSize,
	}
}

// MapSourceDetail converts a single DataSource into a SourceDetailData view model.
func MapSourceDetail(ds *sources.DataSource) SourceDetailData {
	return SourceDetailData{
		Source:         MapSourceItem(ds),
		ConfigJSON:     PrettyJSON(ds.Config),
		HasCredentials: len(ds.Credentials) > 0,
	}
}

// --- Report mappers ---

// MapReportItem converts a domain StoredReport into a ReportItem view model.
func MapReportItem(r *reports.StoredReport) ReportItem {
	return ReportItem{
		ID:          r.ID.String(),
		Type:        r.Type,
		TypeLabel:   ReportTypeLabel(r.Type),
		TypeIntent:  ReportTypeIntent(r.Type),
		Title:       r.Title,
		Focus:       r.Focus,
		GeneratedAt: FormatDate(r.GeneratedAt),
	}
}

// MapReportList converts a paginated ReportPage into a ReportListData view model.
func MapReportList(result reports.ReportPage) ReportListData {
	items := make([]ReportItem, len(result.Reports))
	for i := range result.Reports {
		items[i] = MapReportItem(&result.Reports[i])
	}
	return ReportListData{
		Reports:    items,
		TotalCount: result.TotalCount,
		Page:       result.Page,
		PageSize:   result.PageSize,
	}
}

// MapReportDetail converts a domain StoredReport into a ReportDetailData view
// model, rendering the markdown content to HTML and parsing its citations.
func MapReportDetail(r *reports.StoredReport) ReportDetailData {
	return ReportDetailData{
		ID:          r.ID.String(),
		Title:       r.Title,
		Type:        r.Type,
		TypeLabel:   ReportTypeLabel(r.Type),
		TypeIntent:  ReportTypeIntent(r.Type),
		Focus:       r.Focus,
		GeneratedAt: FormatDate(r.GeneratedAt),
		ContentHTML: RenderMarkdown(r.Content),
		Citations:   ParseCitations(r.Citations),
	}
}

// ReportTypeLabel returns a human-readable label for a report type.
func ReportTypeLabel(t string) string {
	switch t {
	case "daily":
		return "Daily"
	case "weekly":
		return "Weekly"
	case "monthly":
		return "Monthly"
	case "on_demand":
		return "On Demand"
	default:
		return t
	}
}

// ReportTypeIntent maps a report type to a design-system intent.
// daily=info, weekly=primary, monthly=primary, on_demand=warning.
func ReportTypeIntent(t string) string {
	switch t {
	case "daily":
		return "info"
	case "weekly", "monthly":
		return "primary"
	case "on_demand":
		return "warning"
	default:
		return "primary"
	}
}

// ParseCitations parses a report's citations JSON into a slice of Citation.
// It tolerates malformed or empty JSON, returning an empty slice in that case.
func ParseCitations(raw json.RawMessage) []Citation {
	if len(raw) == 0 {
		return nil
	}

	// Citations are an array of objects. Unmarshal generically so unsupported
	// key shapes lose no data, then map the common fields.
	var generic []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil
	}
	out := make([]Citation, 0, len(generic))
	for _, g := range generic {
		out = append(out, Citation{
			Title:  firstString(g, "title", "name", "reference"),
			URL:    firstString(g, "url", "link"),
			Source: firstString(g, "source", "snippet"),
		})
	}
	return out
}

// RenderMarkdown renders markdown to safe HTML. On parse errors it falls back
// to the escaped raw text so content is never lost.
func RenderMarkdown(md string) string {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		return html.EscapeString(md)
	}
	return buf.String()
}

// FormatDate renders a timestamp as a human-readable date and time.
func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Jan 02, 2006 15:04")
}

// firstString returns the first non-empty string value for the given keys.
func firstString(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		var s string
		if raw, ok := m[k]; ok && json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

// --- Dashboard mapper ---

// MapDashboard assembles a DashboardData view model from the four domain inputs.
// All inputs are optional (nil) — missing data gets zero-value defaults.
func MapDashboard(
	currentUsage *usage.CurrentUsage,
	reportsCount int,
	activeSources int,
	budgetStatus *budget.BudgetStatus,
) DashboardData {
	data := DashboardData{
		CostFormatted:   "$0.00",
		TokensFormatted: "0",
		BudgetIntent:    "success",
		ReportsCount:    reportsCount,
		ActiveSources:   activeSources,
	}

	if currentUsage != nil {
		data.TotalCostUSD = currentUsage.TotalCostUSD
		data.TotalTokens = currentUsage.TotalInputTokens + currentUsage.TotalOutputTokens
		data.CostFormatted = fmt.Sprintf("$%.2f", currentUsage.TotalCostUSD)
		data.TokensFormatted = FormatTokens(data.TotalTokens)
	}

	if budgetStatus != nil {
		data.BudgetPercent = budgetStatus.PercentUsed
		data.BudgetIntent = BudgetIntent(budgetStatus.PercentUsed)
		data.BudgetExceeded = budgetStatus.IsExceeded
	}

	return data
}

// --- Report type options ---

// ReportTypeOptions returns the available report types for the generate form.
func ReportTypeOptions() []ReportTypeOption {
	return []ReportTypeOption{
		{Value: "daily", Label: "Daily"},
		{Value: "weekly", Label: "Weekly"},
		{Value: "monthly", Label: "Monthly"},
		{Value: "on_demand", Label: "On Demand"},
	}
}

// --- Source type options ---

// SourceTypeOptions returns the available source types for the form selector.
func SourceTypeOptions() []SourceTypeOption {
	return []SourceTypeOption{
		{Value: string(sources.SourceTypeFileUpload), Label: "File Upload"},
		{Value: string(sources.SourceTypeWebsite), Label: "Website"},
		{Value: string(sources.SourceTypeCRMHubSpot), Label: "HubSpot CRM"},
		{Value: string(sources.SourceTypeCRMSalesforce), Label: "Salesforce CRM"},
	}
}

// --- Formatting helpers ---

// BudgetIntent returns the intent CSS class based on budget usage percentage.
//
//	green  (< 80%)  → "success"
//	yellow (80–95%) → "warning"
//	red    (> 95%)  → "error"
func BudgetIntent(percent float64) string {
	switch {
	case percent > 95:
		return "error"
	case percent >= 80:
		return "warning"
	default:
		return "success"
	}
}

// FormatTokens formats a token count for human-readable display.
func FormatTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// CountActiveSources counts sources with Status == "active".
func CountActiveSources(srcs []sources.DataSource) int {
	n := 0
	for i := range srcs {
		if srcs[i].Status == sources.StatusActive {
			n++
		}
	}
	return n
}

// FormatTimeAgo returns a human-readable "time ago" string.
func FormatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

func PrettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(b)
}

// --- internal helpers ---

// statusIntent maps a source status to a design-system intent.
func statusIntent(s sources.Status) string {
	switch s {
	case sources.StatusActive:
		return "success"
	case sources.StatusError:
		return "error"
	case sources.StatusInactive:
		return "muted"
	default:
		return "muted"
	}
}

// sourceTypeLabel returns a human-readable label for a source type.
func sourceTypeLabel(t sources.SourceType) string {
	switch t {
	case sources.SourceTypeFileUpload:
		return "File Upload"
	case sources.SourceTypeWebsite:
		return "Website"
	case sources.SourceTypeCRMHubSpot:
		return "HubSpot CRM"
	case sources.SourceTypeCRMSalesforce:
		return "Salesforce CRM"
	default:
		return string(t)
	}
}
