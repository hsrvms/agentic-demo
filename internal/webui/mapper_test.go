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
		ID:          uuid.New(),
		Name:        "Detail Source",
		Config:      json.RawMessage(`{"url":"https://example.com"}`),
		Status:      sources.StatusActive,
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

// --- Usage mappers ---

func TestMapUsage_WithCurrentAndEvents(t *testing.T) {
	now := time.Now().UTC()
	current := &usage.CurrentUsage{
		TenantID:             "t1",
		PeriodStart:          time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:            time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC),
		TotalCostUSD:         12.5,
		TotalInputTokens:     1_000_000,
		TotalOutputTokens:    500_000,
		TotalToolCalls:       42,
		TotalEmbeddingTokens: 2_000_000,
		ReportsGenerated:     3,
		ByModel: []usage.ModelUsage{
			{Model: "qwen-turbo", InputTokens: 1_000_000, OutputTokens: 500_000, ToolCalls: 42, EmbeddingTokens: 2_000_000, CostUSD: 12.5},
		},
	}
	events := &usage.UsageEventPage{
		Events: []usage.UsageEventRecord{
			{
				ID:        uuid.New(),
				TenantID:  "t1",
				EventType: string(usage.EventLLM),
				Payload:   json.RawMessage(`{"Type":"llm_usage","LLM":{"Model":"qwen-turbo","InputTokens":100,"OutputTokens":50}}`),
				CreatedAt: now,
			},
			{
				ID:        uuid.New(),
				TenantID:  "t1",
				EventType: string(usage.EventTool),
				Payload:   json.RawMessage(`{"Type":"tool_usage","Tool":{"ToolName":"web_search","Success":true}}`),
				CreatedAt: now,
			},
		},
		TotalCount: 22,
		Page:       1,
		PageSize:   20,
	}

	data := MapUsage(current, events)

	assert.True(t, data.HasData)
	assert.Equal(t, "August 2026", data.PeriodLabel)
	assert.Equal(t, "$12.50", data.Stats[0].Value)
	assert.Equal(t, "Total Cost MTD", data.Stats[0].Label)
	assert.Equal(t, "primary", data.Stats[0].Intent)
	assert.Equal(t, "1.0M", data.Stats[1].Value) // input tokens
	assert.Equal(t, "500.0K", data.Stats[2].Value)
	assert.Equal(t, "42", data.Stats[3].Value) // tool calls
	assert.Equal(t, "2.0M", data.Stats[4].Value)
	assert.Equal(t, "3", data.Stats[5].Value) // reports generated

	require.Len(t, data.Models, 1)
	assert.Equal(t, "qwen-turbo", data.Models[0].Model)
	assert.Equal(t, "$12.50", data.Models[0].CostUSD)

	require.Len(t, data.Events, 2)
	assert.Equal(t, "LLM", data.Events[0].TypeLabel)
	assert.Equal(t, "qwen-turbo · 150 tokens", data.Events[0].Summary)
	assert.Equal(t, "Tool", data.Events[1].TypeLabel)
	assert.Equal(t, "web_search · succeeded", data.Events[1].Summary)

	assert.True(t, data.HasMoreEvents)
	assert.Equal(t, 2, data.NextEventPage)
}

func TestMapUsage_NilCurrent(t *testing.T) {
	data := MapUsage(nil, nil)
	assert.False(t, data.HasData)
	assert.Len(t, data.Stats, 6) // zero-value default cards
	assert.Empty(t, data.Models)
	assert.Empty(t, data.Events)
	assert.False(t, data.HasMoreEvents)
}

func TestMapUsage_NoPaginationWhenSinglePage(t *testing.T) {
	events := &usage.UsageEventPage{
		Events:     []usage.UsageEventRecord{{ID: uuid.New(), TenantID: "t1", EventType: string(usage.EventLLM), CreatedAt: time.Now().UTC()}},
		TotalCount: 1,
		Page:       1,
		PageSize:   20,
	}
	data := MapUsage(nil, events)
	assert.False(t, data.HasMoreEvents)
}

func TestMapUsageSummary(t *testing.T) {
	summary := &usage.UsageSummary{
		TenantID:     "t1",
		From:         time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		To:           time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		TotalCostUSD: 8.0,
		Models: []usage.ModelUsageSummary{
			{Model: "qwen-max", InputTokens: 2000, OutputTokens: 1000, ToolCalls: 5, EmbeddingTokens: 0, CostUSD: 8.0},
		},
	}
	data := MapUsageSummary(summary)
	assert.True(t, data.HasData)
	assert.Equal(t, "Jul 01, 2026", data.FromLabel)
	assert.Equal(t, "Jul 31, 2026", data.ToLabel)
	assert.Equal(t, "$8.00", data.CostFormatted)
	require.Len(t, data.Models, 1)
	assert.Equal(t, "qwen-max", data.Models[0].Model)
	assert.Equal(t, "2.0K", data.Models[0].InputTokens)
	assert.Equal(t, "5", data.Models[0].ToolCalls)
}

func TestMapUsageSummary_Empty(t *testing.T) {
	data := MapUsageSummary(&usage.UsageSummary{Models: []usage.ModelUsageSummary{}})
	assert.False(t, data.HasData)
	assert.Empty(t, data.Models)
}

func TestMapUsageEvents_EmbeddingAndMalformed(t *testing.T) {
	records := []usage.UsageEventRecord{
		{
			ID:        uuid.New(),
			TenantID:  "t1",
			EventType: string(usage.EventEmbedding),
			Payload:   json.RawMessage(`{"Type":"embedding_usage","Embedding":{"Model":"bge","ChunksProcessed":7}}`),
			CreatedAt: time.Now().UTC(),
		},
		{
			ID:        uuid.New(),
			TenantID:  "t1",
			EventType: "unknown_type",
			Payload:   json.RawMessage(`not json`),
			CreatedAt: time.Now().UTC(),
		},
	}
	items := MapUsageEvents(records)
	require.Len(t, items, 2)
	assert.Equal(t, "Embedding", items[0].TypeLabel)
	assert.Equal(t, "success", items[0].TypeIntent)
	assert.Equal(t, "bge · 7 chunks", items[0].Summary)
	assert.Equal(t, "unknown_type", items[1].TypeLabel)
	assert.Equal(t, "unknown_type", items[1].Summary)
}
