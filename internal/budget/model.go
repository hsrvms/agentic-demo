// Package budget implements budget enforcement and invoicing.
//
// BudgetChecker is the cross-cutting seam: it is injected into the LLM Client
// and rejects calls that would exceed the tenant's monthly budget.
// The Service handles invoice generation and listing.
package budget

import (
	"time"

	"github.com/google/uuid"
)

// InvoiceStatus is the lifecycle state of an invoice.
type InvoiceStatus string

const (
	InvoiceDraft   InvoiceStatus = "draft"
	InvoiceIssued  InvoiceStatus = "issued"
	InvoicePaid    InvoiceStatus = "paid"
	InvoiceOverdue InvoiceStatus = "overdue"
)

// Invoice is a generated monthly billing record.
type Invoice struct {
	ID           uuid.UUID
	TenantID     string
	PeriodStart  time.Time
	PeriodEnd    time.Time
	TotalCostUSD float64
	LineItems    []InvoiceLineItem
	Status       InvoiceStatus
	CreatedAt    time.Time
}

// InvoiceLineItem is a single line on an invoice, driven by usage_daily aggregates.
type InvoiceLineItem struct {
	Model            string  `json:"model"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	ToolCalls        int32   `json:"tool_calls"`
	EmbeddingTokens  int64   `json:"embedding_tokens"`
	ReportsGenerated int32   `json:"reports_generated"`
	CostUSD          float64 `json:"cost_usd"`
}

// InvoicePage is a paginated list of invoices.
type InvoicePage struct {
	Invoices   []Invoice
	TotalCount int
	Page       int
	PageSize   int
}

// BudgetStatus describes the current budget state for a tenant.
type BudgetStatus struct {
	TenantID        string  `json:"tenant_id"`
	MonthlyBudget   float64 `json:"monthly_budget_usd"`
	MonthToDateCost float64 `json:"month_to_date_cost_usd"`
	RemainingBudget float64 `json:"remaining_budget_usd"`
	PercentUsed     float64 `json:"percent_used"`
	IsExceeded      bool    `json:"is_exceeded"`
}

// BudgetCheckResult is returned by BudgetChecker.CheckBudget.
type BudgetCheckResult struct {
	Allowed           bool
	BudgetStatus      *BudgetStatus
	EstimatedCallCost float64
	ThresholdReached  string // "80%", "95%", or empty if below thresholds
}
