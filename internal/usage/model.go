package usage

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// UsageEventRecord is a persisted usage event retrieved from the database.
type UsageEventRecord struct {
	ID        uuid.UUID
	TenantID  string
	EventType string
	Payload   json.RawMessage
	CreatedAt time.Time
}

// UsageDailyRecord is a persisted daily aggregate.
type UsageDailyRecord struct {
	ID               uuid.UUID
	TenantID         string
	Date             time.Time
	Model            string
	InputTokens      int64
	OutputTokens     int64
	ToolCalls        int32
	EmbeddingTokens  int64
	EstimatedCostUSD float64
	ReportsGenerated int32
}

// UsageSummary is an aggregated usage response for the API.
type UsageSummary struct {
	TenantID     string
	From         time.Time
	To           time.Time
	TotalCostUSD float64
	Models       []ModelUsageSummary
}

// ModelUsageSummary is a per-model breakdown within a UsageSummary.
type ModelUsageSummary struct {
	Model           string
	InputTokens     int64
	OutputTokens    int64
	ToolCalls       int32
	EmbeddingTokens int64
	CostUSD         float64
}

// UsageEventPage is a paginated list of usage events.
type UsageEventPage struct {
	Events     []UsageEventRecord
	TotalCount int
	Page       int
	PageSize   int
}

// CurrentUsage is a real-time usage snapshot from Redis for the current month.
type CurrentUsage struct {
	TenantID             string
	PeriodStart          time.Time
	PeriodEnd            time.Time
	TotalCostUSD         float64
	TotalInputTokens     int64
	TotalOutputTokens    int64
	TotalToolCalls       int64
	TotalEmbeddingTokens int64
	ReportsGenerated     int64
	ByModel              []ModelUsage
}

// ModelUsage is a per-model breakdown within CurrentUsage.
type ModelUsage struct {
	Model           string
	InputTokens     int64
	OutputTokens    int64
	ToolCalls       int64
	EmbeddingTokens int64
	CostUSD         float64
}
