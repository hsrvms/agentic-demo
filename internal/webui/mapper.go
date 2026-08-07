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
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/queue"
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

// MapGenerationActivity converts tracked generation jobs into activity panel
// view models. states holds the live queue state for jobs the Inspector could
// still find; a live state overrides the durable DB status. Jobs absent from
// states keep their recorded outcome.
func MapGenerationActivity(jobs []reports.GenerationJob, states map[string]queue.JobState) GenerationActivityData {
	items := make([]GenerationActivityItem, len(jobs))
	for i := range jobs {
		job := &jobs[i]
		item := GenerationActivityItem{
			ID:         job.ID.String(),
			TypeLabel:  ReportTypeLabel(job.ReportType),
			TypeIntent: ReportTypeIntent(job.ReportType),
			Focus:      job.Focus,
			EnqueuedAt: FormatTimeAgo(job.EnqueuedAt),
		}

		status := queue.JobStatus(job.Status)
		errMsg := job.Error
		if state, ok := states[job.TaskID]; ok {
			status = state.Status
			errMsg = state.Error
		}
		item.Status, item.StatusLabel, item.StatusIntent, item.Error = generationStatusMeta(status, errMsg)
		items[i] = item
	}
	return GenerationActivityData{Items: items}
}

// generationStatusMeta maps a job status to its display label and design
// intent. Failed jobs carry their error message for inline display.
func generationStatusMeta(status queue.JobStatus, errMsg string) (statusKey, label, intent, errOut string) {
	switch status {
	case queue.JobQueued:
		return string(queue.JobQueued), "Queued", "info", ""
	case queue.JobRunning:
		return string(queue.JobRunning), "Running", "info", ""
	case queue.JobSucceeded:
		return string(queue.JobSucceeded), "Succeeded", "success", ""
	case queue.JobFailed:
		return string(queue.JobFailed), "Failed", "error", errMsg
	default:
		return string(queue.JobUnknown), "Unknown", "muted", ""
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

// --- Usage mappers ---

// MapUsage assembles a UsageData view model from the Redis-backed current
// usage snapshot and the first page of usage events. The event page is
// optional (nil) — missing data yields an empty event log.
func MapUsage(current *usage.CurrentUsage, events *usage.UsageEventPage) UsageData {
	data := UsageData{
		Stats: []UsageStatData{
			{Label: "Total Cost MTD", Value: "$0.00", Intent: "primary"},
			{Label: "Input Tokens", Value: "0", Intent: "info"},
			{Label: "Output Tokens", Value: "0", Intent: "info"},
			{Label: "Tool Calls", Value: "0", Intent: "warning"},
			{Label: "Embedding Tokens", Value: "0", Intent: "success"},
			{Label: "Reports Generated", Value: "0", Intent: "primary"},
		},
		Models: make([]UsageModelItem, 0),
		Events: make([]UsageEventItem, 0),
	}

	if current != nil {
		data.HasData = true
		data.PeriodLabel = formatPeriodLabel(current.PeriodStart)
		data.Stats = []UsageStatData{
			{Label: "Total Cost MTD", Value: fmt.Sprintf("$%.2f", current.TotalCostUSD), Intent: "primary"},
			{Label: "Input Tokens", Value: FormatTokens(current.TotalInputTokens), Intent: "info"},
			{Label: "Output Tokens", Value: FormatTokens(current.TotalOutputTokens), Intent: "info"},
			{Label: "Tool Calls", Value: strconv.FormatInt(current.TotalToolCalls, 10), Intent: "warning"},
			{Label: "Embedding Tokens", Value: FormatTokens(current.TotalEmbeddingTokens), Intent: "success"},
			{Label: "Reports Generated", Value: strconv.FormatInt(current.ReportsGenerated, 10), Intent: "primary"},
		}
		for i := range current.ByModel {
			data.Models = append(data.Models, MapUsageModel(&current.ByModel[i]))
		}
	}

	if events != nil {
		data.Events = MapUsageEvents(events.Events)
		data.HasMoreEvents = events.Page*events.PageSize < events.TotalCount
		data.NextEventPage = events.Page + 1
	}

	return data
}

// MapUsageModel converts a usage.ModelUsage into a UsageModelItem view model.
func MapUsageModel(m *usage.ModelUsage) UsageModelItem {
	return UsageModelItem{
		Model:           m.Model,
		InputTokens:     FormatTokens(m.InputTokens),
		OutputTokens:    FormatTokens(m.OutputTokens),
		ToolCalls:       strconv.FormatInt(m.ToolCalls, 10),
		EmbeddingTokens: FormatTokens(m.EmbeddingTokens),
		CostUSD:         fmt.Sprintf("$%.2f", m.CostUSD),
	}
}

// MapUsageSummary maps a domain UsageSummary into a UsageSummaryData fragment
// view model.
func MapUsageSummary(summary *usage.UsageSummary) UsageSummaryData {
	data := UsageSummaryData{
		Models: make([]UsageModelItem, 0),
	}
	if summary == nil {
		return data
	}

	data.HasData = len(summary.Models) > 0
	data.FromLabel = formatDateOnly(summary.From)
	data.ToLabel = formatDateOnly(summary.To)
	data.CostFormatted = fmt.Sprintf("$%.2f", summary.TotalCostUSD)
	for i := range summary.Models {
		m := &summary.Models[i]
		data.Models = append(data.Models, UsageModelItem{
			Model:           m.Model,
			InputTokens:     FormatTokens(m.InputTokens),
			OutputTokens:    FormatTokens(m.OutputTokens),
			ToolCalls:       strconv.FormatInt(int64(m.ToolCalls), 10),
			EmbeddingTokens: FormatTokens(m.EmbeddingTokens),
			CostUSD:         fmt.Sprintf("$%.2f", m.CostUSD),
		})
	}
	return data
}

// MapUsageEvents converts domain usage event records into event log view
// models, deriving a human-readable summary from each event's payload.
func MapUsageEvents(records []usage.UsageEventRecord) []UsageEventItem {
	items := make([]UsageEventItem, len(records))
	for i := range records {
		r := &records[i]
		label, intent := eventTypeMeta(r.EventType)
		items[i] = UsageEventItem{
			ID:         r.ID.String(),
			EventType:  r.EventType,
			TypeLabel:  label,
			TypeIntent: intent,
			Summary:    eventSummary(r),
			CreatedAt:  FormatDate(r.CreatedAt),
		}
	}
	return items
}

// formatPeriodLabel renders a month label like "August 2026".
func formatPeriodLabel(from time.Time) string {
	if from.IsZero() {
		return ""
	}
	year, month, _ := from.Date()
	return fmt.Sprintf("%s %d", month, year)
}

// formatDateOnly renders a date as "Jan 02, 2006", or an empty string when
// the time is zero (unbounded range edge).
func formatDateOnly(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Jan 02, 2006")
}

// eventTypeMeta maps a usage event type to a label and design-system intent.
func eventTypeMeta(eventType string) (label, intent string) {
	switch eventType {
	case string(usage.EventLLM):
		return "LLM", "info"
	case string(usage.EventTool):
		return "Tool", "warning"
	case string(usage.EventEmbedding):
		return "Embedding", "success"
	default:
		return eventType, "info"
	}
}

// eventSummary renders a one-line human-readable description of a usage
// event from its type-specific payload. Malformed payloads fall back to the
// bare event type.
func eventSummary(r *usage.UsageEventRecord) string {
	type llmPayload struct {
		Model        string `json:"Model"`
		InputTokens  int    `json:"InputTokens"`
		OutputTokens int    `json:"OutputTokens"`
	}
	type toolPayload struct {
		ToolName string `json:"ToolName"`
		Model    string `json:"Model"`
		Success  bool   `json:"Success"`
	}
	type embeddingPayload struct {
		Model           string `json:"Model"`
		ChunksProcessed int    `json:"ChunksProcessed"`
	}
	type payload struct {
		Type      string            `json:"Type"`
		LLM       *llmPayload       `json:"LLM"`
		Tool      *toolPayload      `json:"Tool"`
		Embedding *embeddingPayload `json:"Embedding"`
	}

	var p payload
	if err := json.Unmarshal(r.Payload, &p); err != nil {
		return eventTypeLabel(r.EventType)
	}

	switch p.Type {
	case string(usage.EventLLM):
		if p.LLM != nil {
			tokens := p.LLM.InputTokens + p.LLM.OutputTokens
			return fmt.Sprintf("%s · %d tokens", p.LLM.Model, tokens)
		}
	case string(usage.EventTool):
		if p.Tool != nil {
			status := "succeeded"
			if !p.Tool.Success {
				status = "failed"
			}
			return fmt.Sprintf("%s · %s", p.Tool.ToolName, status)
		}
	case string(usage.EventEmbedding):
		if p.Embedding != nil {
			return fmt.Sprintf("%s · %d chunks", p.Embedding.Model, p.Embedding.ChunksProcessed)
		}
	}
	return eventTypeLabel(p.Type)
}

// eventTypeLabel returns a plain label for an event type, falling back to the
// raw type string.
func eventTypeLabel(eventType string) string {
	label, _ := eventTypeMeta(eventType)
	return label
}

// --- Invoice mappers ---

// MapInvoiceItem converts a domain Invoice into an InvoiceItem view model.
func MapInvoiceItem(inv *budget.Invoice) InvoiceItem {
	return InvoiceItem{
		ID:           inv.ID.String(),
		PeriodLabel:  invoicePeriodLabel(inv.PeriodStart, inv.PeriodEnd),
		Status:       string(inv.Status),
		StatusLabel:  InvoiceStatusLabel(inv.Status),
		StatusIntent: InvoiceStatusIntent(inv.Status),
		TotalCostUSD: fmt.Sprintf("$%.2f", inv.TotalCostUSD),
	}
}

// MapInvoiceList converts a paginated InvoicePage into an InvoiceListData
// view model.
func MapInvoiceList(result budget.InvoicePage) InvoiceListData {
	items := make([]InvoiceItem, len(result.Invoices))
	for i := range result.Invoices {
		items[i] = MapInvoiceItem(&result.Invoices[i])
	}
	return InvoiceListData{
		Invoices:   items,
		TotalCount: result.TotalCount,
		Page:       result.Page,
		PageSize:   result.PageSize,
	}
}

// MapInvoiceDetail converts a domain Invoice into an InvoiceDetailData view
// model with a per-model line-item breakdown.
func MapInvoiceDetail(inv *budget.Invoice) InvoiceDetailData {
	lineItems := make([]InvoiceLineItem, len(inv.LineItems))
	for i := range inv.LineItems {
		li := &inv.LineItems[i]
		lineItems[i] = InvoiceLineItem{
			Model:            li.Model,
			InputTokens:      FormatTokens(li.InputTokens),
			OutputTokens:     FormatTokens(li.OutputTokens),
			ToolCalls:        strconv.FormatInt(int64(li.ToolCalls), 10),
			EmbeddingTokens:  FormatTokens(li.EmbeddingTokens),
			ReportsGenerated: strconv.FormatInt(int64(li.ReportsGenerated), 10),
			CostUSD:          fmt.Sprintf("$%.2f", li.CostUSD),
		}
	}
	return InvoiceDetailData{
		ID:           inv.ID.String(),
		PeriodLabel:  invoicePeriodLabel(inv.PeriodStart, inv.PeriodEnd),
		PeriodStart:  invoiceDate(inv.PeriodStart),
		PeriodEnd:    invoiceDate(inv.PeriodEnd),
		Status:       string(inv.Status),
		StatusLabel:  InvoiceStatusLabel(inv.Status),
		StatusIntent: InvoiceStatusIntent(inv.Status),
		TotalCostUSD: fmt.Sprintf("$%.2f", inv.TotalCostUSD),
		LineItems:    lineItems,
	}
}

// InvoiceStatusLabel returns the display label for an invoice status.
func InvoiceStatusLabel(s budget.InvoiceStatus) string {
	switch s {
	case budget.InvoiceDraft:
		return "Draft"
	case budget.InvoiceIssued:
		return "Issued"
	case budget.InvoicePaid:
		return "Paid"
	case budget.InvoiceOverdue:
		return "Overdue"
	default:
		return string(s)
	}
}

// InvoiceStatusIntent maps an invoice status to a design-system intent.
// draft=muted, issued=info, paid=success, overdue=error.
func InvoiceStatusIntent(s budget.InvoiceStatus) string {
	switch s {
	case budget.InvoiceIssued:
		return "info"
	case budget.InvoicePaid:
		return "success"
	case budget.InvoiceOverdue:
		return "error"
	case budget.InvoiceDraft:
		return "muted"
	default:
		return "muted"
	}
}

// invoicePeriodLabel renders an invoice billing period in a compact form,
// e.g. "Jul 01 — Jul 31, 2025". Different years render both years.
func invoicePeriodLabel(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return ""
	}
	if start.Year() == end.Year() {
		return fmt.Sprintf("%s %02d — %s %02d, %d",
			start.Month().String()[:3], start.Day(),
			end.Month().String()[:3], end.Day(), end.Year())
	}
	return fmt.Sprintf("%s %02d, %d — %s %02d, %d",
		start.Month().String()[:3], start.Day(), start.Year(),
		end.Month().String()[:3], end.Day(), end.Year())
}

// invoiceDate renders a date as "Jan 02, 2006", or an empty string for a
// zero time.
func invoiceDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Jan 02, 2006")
}

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
//
// BudgetIntent returns the semantic intent for a budget usage percentage.
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

// MapSettings assembles a SettingsData view model from the current tenant and
// its live budget status. tenant is required; budgetStatus is optional (nil)
// and yields zero-value budget fields.
func MapSettings(tenant *domain.Tenant, budgetStatus *budget.BudgetStatus) SettingsData {
	data := SettingsData{
		TenantName:         tenant.Name,
		TenantID:           string(tenant.ID),
		TenantStatus:       string(tenant.Status),
		TenantStatusLabel:  tenantStatusLabel(tenant.Status),
		TenantStatusIntent: intentForTenantStatus(tenant.Status),
		CreatedAt:          FormatDate(tenant.CreatedAt),
		BudgetIntent:       "success",
	}

	if budgetStatus != nil {
		data.MonthlyBudget = formatUSD(budgetStatus.MonthlyBudget)
		data.MTDCost = formatUSD(budgetStatus.MonthToDateCost)
		data.Remaining = formatUSD(budgetStatus.RemainingBudget)
		data.PercentUsed = budgetStatus.PercentUsed
		data.BudgetIntent = BudgetIntent(budgetStatus.PercentUsed)
		data.BudgetExceeded = budgetStatus.IsExceeded
	} else {
		data.MonthlyBudget = "$0.00"
		data.MTDCost = "$0.00"
		data.Remaining = "$0.00"
	}

	return data
}

// formatUSD renders a dollar amount like "$250.00".
func formatUSD(v float64) string {
	return fmt.Sprintf("$%.2f", v)
}

// tenantStatusLabel maps a tenant lifecycle status to a human label.
func tenantStatusLabel(s domain.TenantStatus) string {
	switch s {
	case domain.TenantActive:
		return "Active"
	case domain.TenantSuspended:
		return "Suspended"
	case domain.TenantDeleted:
		return "Deleted"
	default:
		return string(s)
	}
}

// intentForTenantStatus maps a tenant status to a semantic intent.
func intentForTenantStatus(s domain.TenantStatus) string {
	switch s {
	case domain.TenantActive:
		return "success"
	case domain.TenantSuspended:
		return "warning"
	case domain.TenantDeleted:
		return "error"
	default:
		return "muted"
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
