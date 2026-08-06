// Package webui maps domain types into view model types for the web UI layer.
// It is a leaf package — no dependencies on echo, templ, net/http, or internal/web.
package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/scheduling"
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

// --- Schedule mappers ---

// MapScheduleItem converts a domain ReportSchedule into a ScheduleItem view
// model, computing a human-readable cron description for display.
func MapScheduleItem(s *scheduling.ReportSchedule) ScheduleItem {
	return ScheduleItem{
		ID:          s.ID.String(),
		Type:        string(s.Type),
		TypeLabel:   ScheduleTypeLabel(s.Type),
		TypeIntent:  ScheduleTypeIntent(s.Type),
		CronExpr:    s.CronExpr,
		CronHuman:   DescribeCron(s.CronExpr),
		Focus:       s.Focus,
		Format:      s.Format,
		FormatLabel: ScheduleFormatLabel(s.Format),
		Enabled:     s.Enabled,
	}
}

// MapScheduleList converts a slice of domain schedules into a
// ScheduleListData view model.
func MapScheduleList(schedules []scheduling.ReportSchedule) ScheduleListData {
	items := make([]ScheduleItem, len(schedules))
	for i := range schedules {
		items[i] = MapScheduleItem(&schedules[i])
	}
	return ScheduleListData{Schedules: items}
}

// ScheduleTypeLabel returns a human-readable label for a schedule type.
func ScheduleTypeLabel(t scheduling.ScheduleType) string {
	switch t {
	case scheduling.ScheduleDaily:
		return "Daily"
	case scheduling.ScheduleWeekly:
		return "Weekly"
	case scheduling.ScheduleMonthly:
		return "Monthly"
	default:
		return string(t)
	}
}

// ScheduleTypeIntent maps a schedule type to a design-system intent.
// daily=info, weekly=primary, monthly=primary.
func ScheduleTypeIntent(t scheduling.ScheduleType) string {
	switch t {
	case scheduling.ScheduleDaily:
		return "info"
	case scheduling.ScheduleWeekly, scheduling.ScheduleMonthly:
		return "primary"
	default:
		return "primary"
	}
}

// ScheduleTypeOptions returns the available schedule types for the form.
func ScheduleTypeOptions() []ScheduleTypeOption {
	return []ScheduleTypeOption{
		{Value: string(scheduling.ScheduleDaily), Label: "Daily"},
		{Value: string(scheduling.ScheduleWeekly), Label: "Weekly"},
		{Value: string(scheduling.ScheduleMonthly), Label: "Monthly"},
	}
}

// ScheduleFormatLabel returns a human-readable label for a report format.
func ScheduleFormatLabel(f string) string {
	switch f {
	case "standard":
		return "Standard"
	case "concise":
		return "Concise"
	case "detailed":
		return "Detailed"
	default:
		if f == "" {
			return "Standard"
		}
		return f
	}
}

// ReportFormatOptions returns the available report formats for the form.
func ReportFormatOptions() []ReportFormatOption {
	return []ReportFormatOption{
		{Value: "standard", Label: "Standard"},
		{Value: "concise", Label: "Concise"},
		{Value: "detailed", Label: "Detailed"},
	}
}

// WeekdayOptions returns the day-of-week options for weekly schedules, using
// the cron convention (0 = Sunday … 6 = Saturday).
func WeekdayOptions() []WeekdayOption {
	return []WeekdayOption{
		{Value: "0", Label: "Sunday"},
		{Value: "1", Label: "Monday"},
		{Value: "2", Label: "Tuesday"},
		{Value: "3", Label: "Wednesday"},
		{Value: "4", Label: "Thursday"},
		{Value: "5", Label: "Friday"},
		{Value: "6", Label: "Saturday"},
	}
}

// BuildCronExpr composes a 5-field cron expression from friendly form inputs.
// The schedule type determines the cadence structure; only the relevant day
// field is used (day of week for weekly, day of month for monthly). Empty day
// fields fall back to sensible defaults (Monday for weekly, the 1st for
// monthly). It returns ErrInvalidScheduleType for an unknown type and
// ErrInvalidTime for an unparseable time.
func BuildCronExpr(scheduleType, timeOfDay, dayOfWeek, dayOfMonth string) (string, error) {
	hour, minute, err := parseClock(timeOfDay)
	if err != nil {
		return "", err
	}

	switch scheduling.ScheduleType(scheduleType) {
	case scheduling.ScheduleDaily:
		return fmt.Sprintf("%d %d * * *", minute, hour), nil
	case scheduling.ScheduleWeekly:
		dow := dayOfWeek
		if dow == "" {
			dow = "1" // Monday
		}
		return fmt.Sprintf("%d %d * * %s", minute, hour, dow), nil
	case scheduling.ScheduleMonthly:
		dom := dayOfMonth
		if dom == "" {
			dom = "1"
		}
		return fmt.Sprintf("%d %d %s * *", minute, hour, dom), nil
	default:
		return "", scheduling.ErrInvalidScheduleType
	}
}

// CronParts decomposes a 5-field cron expression back into the friendly form
// fields (time of day, day of week, day of month) so the edit form can prefill
// its selections. Unparseable expressions return all empty strings.
func CronParts(expr string) (timeOfDay, dayOfWeek, dayOfMonth string) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return "", "", ""
	}
	return pad2(parts[1]) + ":" + pad2(parts[0]), parts[4], parts[2]
}

// parseClock parses an "HH:MM" (24-hour) time string into hour and minute
// integers. It returns ErrInvalidTime for malformed input.
func parseClock(t string) (hour, minute int, err error) {
	t = strings.TrimSpace(t)
	if t == "" {
		return 0, 0, ErrInvalidTime
	}
	parsed, perr := time.Parse("15:04", t)
	if perr != nil {
		return 0, 0, ErrInvalidTime
	}
	return parsed.Hour(), parsed.Minute(), nil
}

// DescribeCron renders a 5-field cron expression as a human-readable phrase.
// Unparseable expressions fall back to the raw expression.
func DescribeCron(expr string) string {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return expr
	}
	minute, hour, dom, month, dow := parts[0], parts[1], parts[2], parts[3], parts[4]

	if isEvery(parts...) {
		return "Every minute"
	}

	when := cronTime(hour, minute)

	// Daily: every day at HH:MM (e.g. "0 9 * * *").
	if isStar(dom) && isStar(month) && isStar(dow) {
		return "Every day" + when
	}

	// Weekly: on a specific weekday (e.g. "0 9 * * 1").
	if isStar(dom) && isStar(month) && !isStar(dow) {
		if day, ok := weekdayName(dow); ok {
			return "Every " + day + when
		}
	}

	// Monthly: on a specific day of month (e.g. "0 9 1 * *").
	if !isStar(dom) && isStar(month) && isStar(dow) {
		return "Monthly" + ordinal(dom) + when
	}

	return "Custom schedule" + when
}

// cronTime renders " at HH:MM" when both hour and minute are fixed.
func cronTime(hour, minute string) string {
	if isStar(hour) || isStar(minute) {
		return ""
	}
	return " at " + pad2(hour) + ":" + pad2(minute)
}

// weekdayName maps a cron weekday (0-7, both 0 and 7 are Sunday) to its name.
func weekdayName(dow string) (string, bool) {
	if strings.ContainsAny(dow, "*/,") {
		return "", false
	}
	switch dow {
	case "0", "7":
		return "Sunday", true
	case "1":
		return "Monday", true
	case "2":
		return "Tuesday", true
	case "3":
		return "Wednesday", true
	case "4":
		return "Thursday", true
	case "5":
		return "Friday", true
	case "6":
		return "Saturday", true
	default:
		return "", false
	}
}

// ordinal returns a number with its English ordinal suffix and a leading
// "on the " phrase (e.g. "1" -> "on the 1st").
func ordinal(day string) string {
	n, err := strconv.Atoi(day)
	if err != nil {
		return ""
	}
	suffix := "th"
	switch n % 100 {
	case 11, 12, 13:
		suffix = "th"
	default:
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf(" on the %d%s", n, suffix)
}

// pad2 zero-pads a numeric field to two digits, falling back to the raw
// value when it is not a plain integer.
func pad2(v string) string {
	n, err := strconv.Atoi(v)
	if err != nil {
		return v
	}
	return fmt.Sprintf("%02d", n)
}

func isEvery(fields ...string) bool {
	for _, f := range fields {
		if !isStar(f) {
			return false
		}
	}
	return true
}

func isStar(f string) bool {
	return f == "*"
}

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
