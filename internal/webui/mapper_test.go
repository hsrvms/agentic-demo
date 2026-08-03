package webui

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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