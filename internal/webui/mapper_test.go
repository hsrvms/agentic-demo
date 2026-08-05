package webui

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- MapSourceItem ---

func TestMapSourceItem(t *testing.T) {
	now := time.Now()
	id := uuid.New()

	ds := &sources.DataSource{
		ID:             id,
		Name:           "My Source",
		SourceType:     sources.SourceTypeWebsite,
		Status:         sources.StatusActive,
		LastSyncAt:     &now,
		LastSyncStatus: "success",
	}

	item := MapSourceItem(ds)
	assert.Equal(t, id.String(), item.ID)
	assert.Equal(t, "My Source", item.Name)
	assert.Equal(t, "website", item.SourceType)
	assert.Equal(t, "Website", item.SourceLabel)
	assert.Equal(t, "active", item.Status)
	assert.Equal(t, "success", item.StatusIntent)
	assert.Equal(t, "success", item.LastSyncStatus)
	assert.Equal(t, "just now", item.LastSyncAt)
}

func TestMapSourceItem_NilLastSyncAt(t *testing.T) {
	ds := &sources.DataSource{
		ID:     uuid.New(),
		Name:   "No Sync",
		Status: sources.StatusInactive,
	}

	item := MapSourceItem(ds)
	assert.Equal(t, "", item.LastSyncAt)
	assert.Equal(t, "muted", item.StatusIntent)
}

func TestMapSourceItem_UnknownSourceType(t *testing.T) {
	ds := &sources.DataSource{
		ID:         uuid.New(),
		Name:       "Unknown",
		SourceType: "custom_type",
		Status:     sources.StatusError,
	}

	item := MapSourceItem(ds)
	assert.Equal(t, "custom_type", item.SourceLabel)
	assert.Equal(t, "error", item.StatusIntent)
}

// --- MapSourceList ---

func TestMapSourceList(t *testing.T) {
	page := sources.DataSourcePage{
		Sources: []sources.DataSource{
			{ID: uuid.New(), Name: "A", Status: sources.StatusActive},
			{ID: uuid.New(), Name: "B", Status: sources.StatusInactive},
		},
		TotalCount: 42,
		Page:       2,
		PageSize:   20,
	}

	data := MapSourceList(page)
	assert.Len(t, data.Sources, 2)
	assert.Equal(t, "A", data.Sources[0].Name)
	assert.Equal(t, "B", data.Sources[1].Name)
	assert.Equal(t, 42, data.TotalCount)
	assert.Equal(t, 2, data.Page)
	assert.Equal(t, 20, data.PageSize)
}

func TestMapSourceList_Empty(t *testing.T) {
	data := MapSourceList(sources.DataSourcePage{})
	assert.Len(t, data.Sources, 0)
	assert.Equal(t, 0, data.TotalCount)
}

// --- MapSourceDetail ---

func TestMapSourceDetail(t *testing.T) {
	ds := &sources.DataSource{
		ID:      uuid.New(),
		Name:    "Detail Source",
		Config:  json.RawMessage(`{"url":"https://example.com"}`),
		Status:  sources.StatusActive,
		Credentials: []byte("secret"),
	}

	detail := MapSourceDetail(ds)
	assert.Equal(t, "Detail Source", detail.Source.Name)
	assert.Contains(t, detail.ConfigJSON, "https://example.com")
	assert.True(t, detail.HasCredentials)
}

func TestMapSourceDetail_NoCredentials(t *testing.T) {
	ds := &sources.DataSource{
		ID:     uuid.New(),
		Name:   "No Creds",
		Config: nil,
		Status: sources.StatusInactive,
	}

	detail := MapSourceDetail(ds)
	assert.False(t, detail.HasCredentials)
	assert.Equal(t, "", detail.ConfigJSON)
}

// --- MapDashboard ---

func TestMapDashboard_Full(t *testing.T) {
	usg := &usage.CurrentUsage{
		TotalCostUSD:      12.34,
		TotalInputTokens:  5_000,
		TotalOutputTokens: 3_000,
	}
	bgt := &budget.BudgetStatus{
		PercentUsed: 62,
		IsExceeded:  false,
	}

	data := MapDashboard(usg, 7, 3, bgt)
	assert.Equal(t, "$12.34", data.CostFormatted)
	assert.Equal(t, int64(8_000), data.TotalTokens)
	assert.Equal(t, "8.0K", data.TokensFormatted)
	assert.Equal(t, 7, data.ReportsCount)
	assert.Equal(t, 3, data.ActiveSources)
	assert.Equal(t, 62.0, data.BudgetPercent)
	assert.Equal(t, "success", data.BudgetIntent)
	assert.False(t, data.BudgetExceeded)
}

func TestMapDashboard_BudgetExceeded(t *testing.T) {
	bgt := &budget.BudgetStatus{
		PercentUsed: 110,
		IsExceeded:  true,
	}

	data := MapDashboard(nil, 0, 0, bgt)
	assert.Equal(t, "error", data.BudgetIntent)
	assert.True(t, data.BudgetExceeded)
}

func TestMapDashboard_AllNil(t *testing.T) {
	data := MapDashboard(nil, 0, 0, nil)
	assert.Equal(t, "$0.00", data.CostFormatted)
	assert.Equal(t, "0", data.TokensFormatted)
	assert.Equal(t, "success", data.BudgetIntent)
	assert.Equal(t, 0, data.ReportsCount)
	assert.Equal(t, 0, data.ActiveSources)
}

// --- SourceTypeOptions ---

func TestSourceTypeOptions(t *testing.T) {
	opts := SourceTypeOptions()
	assert.Len(t, opts, 4)
	labels := make(map[string]string, len(opts))
	for _, o := range opts {
		labels[o.Value] = o.Label
	}
	assert.Equal(t, "File Upload", labels["file_upload"])
	assert.Equal(t, "Website", labels["website"])
	assert.Equal(t, "HubSpot CRM", labels["crm_hubspot"])
	assert.Equal(t, "Salesforce CRM", labels["crm_salesforce"])
}

// --- BudgetIntent ---

func TestBudgetIntent(t *testing.T) {
	tests := []struct {
		name    string
		percent float64
		want    string
	}{
		{"zero", 0, "success"},
		{"under_80", 50, "success"},
		{"exactly_79", 79.9, "success"},
		{"exactly_80", 80, "warning"},
		{"between_80_95", 87, "warning"},
		{"exactly_95", 95, "warning"},
		{"over_95", 95.1, "error"},
		{"exactly_100", 100, "error"},
		{"over_100", 120, "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BudgetIntent(tt.percent))
		})
	}
}

// --- FormatTokens ---

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{"zero", 0, "0"},
		{"hundreds", 500, "500"},
		{"thousands", 1500, "1.5K"},
		{"millions", 2_500_000, "2.5M"},
		{"large", 15_750_000, "15.8M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatTokens(tt.n))
		})
	}
}

// --- CountActiveSources ---

func TestCountActiveSources(t *testing.T) {
	srcs := []sources.DataSource{
		{Status: sources.StatusActive},
		{Status: sources.StatusInactive},
		{Status: sources.StatusActive},
		{Status: sources.StatusError},
	}
	assert.Equal(t, 2, CountActiveSources(srcs))
	assert.Equal(t, 0, CountActiveSources(nil))
	assert.Equal(t, 0, CountActiveSources([]sources.DataSource{}))
}

// --- FormatTimeAgo ---

func TestFormatTimeAgo(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"just now", 10 * time.Second, "just now"},
		{"1 minute", 1 * time.Minute, "1 minute ago"},
		{"5 minutes", 5 * time.Minute, "5 minutes ago"},
		{"1 hour", 1 * time.Hour, "1 hour ago"},
		{"3 hours", 3 * time.Hour, "3 hours ago"},
		{"1 day", 24 * time.Hour, "1 day ago"},
		{"5 days", 5 * 24 * time.Hour, "5 days ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTimeAgo(time.Now().Add(-tt.d))
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- PrettyJSON ---

func TestPrettyJSON(t *testing.T) {
	assert.Contains(t, PrettyJSON(json.RawMessage(`{"url":"https://example.com"}`)), "https://example.com")
	assert.Equal(t, "", PrettyJSON(nil))
	assert.Equal(t, "", PrettyJSON(json.RawMessage("")))
	assert.Equal(t, "invalid", PrettyJSON(json.RawMessage("invalid")))
}

// --- ReportTypeLabel ---

func TestReportTypeLabel(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"daily", "Daily"},
		{"weekly", "Weekly"},
		{"monthly", "Monthly"},
		{"on_demand", "On Demand"},
		{"custom", "custom"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, ReportTypeLabel(tt.in))
		})
	}
}

// --- ReportTypeIntent ---

func TestReportTypeIntent(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"daily", "info"},
		{"weekly", "primary"},
		{"monthly", "primary"},
		{"on_demand", "warning"},
		{"custom", "primary"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, ReportTypeIntent(tt.in))
		})
	}
}

// --- MapReportItem / MapReportList ---

func TestMapReportItem(t *testing.T) {
	id := uuid.New()
	generated := time.Now().Add(-48 * time.Hour)
	r := &reports.StoredReport{
		ID:          id,
		Type:        "weekly",
		Title:       "Weekly Market Brief",
		Content:     "# Heading",
		Focus:       "Revenue growth",
		GeneratedAt: generated,
	}

	item := MapReportItem(r)
	assert.Equal(t, id.String(), item.ID)
	assert.Equal(t, "weekly", item.Type)
	assert.Equal(t, "Weekly", item.TypeLabel)
	assert.Equal(t, "primary", item.TypeIntent)
	assert.Equal(t, "Weekly Market Brief", item.Title)
	assert.Equal(t, "Revenue growth", item.Focus)
	assert.Equal(t, generated.Format("Jan 02, 2006 15:04"), item.GeneratedAt)
}

func TestMapReportList(t *testing.T) {
	page := reports.ReportPage{
		Reports: []reports.StoredReport{
			{ID: uuid.New(), Title: "A", Type: "daily"},
			{ID: uuid.New(), Title: "B", Type: "on_demand"},
		},
		TotalCount: 42,
		Page:       2,
		PageSize:   20,
	}

	data := MapReportList(page)
	assert.Len(t, data.Reports, 2)
	assert.Equal(t, "A", data.Reports[0].Title)
	assert.Equal(t, "info", data.Reports[0].TypeIntent)
	assert.Equal(t, "warning", data.Reports[1].TypeIntent)
	assert.Equal(t, 42, data.TotalCount)
	assert.Equal(t, 2, data.Page)
	assert.Equal(t, 20, data.PageSize)
}

// --- MapReportDetail / RenderMarkdown ---

func TestMapReportDetail_RendersMarkdownAndCitations(t *testing.T) {
	r := &reports.StoredReport{
		ID:          uuid.New(),
		Type:        "monthly",
		Title:       "Monthly Review",
		Content:     "# Revenue\n\nRevenue is **up 12%**.",
		Citations:   json.RawMessage(`[{"title":"Q3 Report","url":"https://example.com/q3","source":"Internal"}]`),
		GeneratedAt: time.Now(),
	}

	detail := MapReportDetail(r)
	assert.Equal(t, "Monthly Review", detail.Title)
	assert.Equal(t, "monthly", detail.Type)
	assert.Equal(t, "primary", detail.TypeIntent)
	assert.Contains(t, detail.ContentHTML, "<h1>Revenue</h1>")
	assert.Contains(t, detail.ContentHTML, "<strong>up 12%</strong>")
	require.Len(t, detail.Citations, 1)
	assert.Equal(t, "Q3 Report", detail.Citations[0].Title)
	assert.Equal(t, "https://example.com/q3", detail.Citations[0].URL)
	assert.Equal(t, "Internal", detail.Citations[0].Source)
}

func TestRenderMarkdown_Unknown(t *testing.T) {
	got := RenderMarkdown("**bold** and `code`")
	assert.Contains(t, got, "<strong>bold</strong>")
	assert.Contains(t, got, "<code>code</code>")
}

// --- ParseCitations ---

func TestParseCitations_Empty(t *testing.T) {
	assert.Nil(t, ParseCitations(nil))
	assert.Nil(t, ParseCitations(json.RawMessage("")))
	assert.Nil(t, ParseCitations(json.RawMessage("not json")))
}

func TestParseCitations_GenericShape(t *testing.T) {
	raw := json.RawMessage(`[{"source":"https://a.com","reference":"A headline"},{"name":"B","link":"https://b.com"}]`)
	cites := ParseCitations(raw)
	require.Len(t, cites, 2)
	assert.Equal(t, "A headline", cites[0].Title)
	assert.Equal(t, "https://a.com", cites[0].Source)
	assert.Equal(t, "B", cites[1].Title)
	assert.Equal(t, "https://b.com", cites[1].URL)
}

// --- FormatDate ---

func TestFormatDate(t *testing.T) {
	assert.Equal(t, "", FormatDate(time.Time{}))
	ts := time.Date(2026, 8, 5, 14, 30, 0, 0, time.UTC)
	assert.Equal(t, "Aug 05, 2026 14:30", FormatDate(ts))
}