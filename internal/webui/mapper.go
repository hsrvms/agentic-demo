// Package webui maps domain types into view model types for the web UI layer.
// It is a leaf package — no dependencies on echo, templ, net/http, or internal/web.
package webui

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/usage"
)

// --- Source mappers ---

// MapSourceItem converts a domain DataSource into a view model SourceItem.
func MapSourceItem(ds *sources.DataSource) SourceItem {
	item := SourceItem{
		ID:           ds.ID.String(),
		Name:         ds.Name,
		SourceType:   string(ds.SourceType),
		SourceLabel:  sourceTypeLabel(ds.SourceType),
		Status:       string(ds.Status),
		StatusIntent: statusIntent(ds.Status),
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

// --- Dashboard mapper ---

// MapDashboard assembles a DashboardData view model from the four domain inputs.
// All inputs are optional (nil) — missing data gets zero-value defaults.
func MapDashboard(
	currentUsage *usage.CurrentUsage,
	reports int,
	activeSources int,
	budgetStatus *budget.BudgetStatus,
) DashboardData {
	data := DashboardData{
		CostFormatted:   "$0.00",
		TokensFormatted: "0",
		BudgetIntent:    "success",
		ReportsCount:    reports,
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

// PrettyJSON formats JSON for display.
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