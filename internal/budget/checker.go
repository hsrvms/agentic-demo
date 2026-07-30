package budget

import (
	"context"
	"fmt"
	"log"
	"math"

	"github.com/agentic-demo/platform/internal/usage"
	"github.com/agentic-demo/platform/internal/domain"
)

// BudgetChecker checks whether an LLM call would exceed the tenant's budget.
// It is injected into the LLM Client and called before every API request.
type BudgetChecker interface {
	CheckBudget(ctx context.Context, tenantID domain.TenantID, model string, estimatedInputTokens, estimatedOutputTokens int) (BudgetCheckResult, error)
}

// checker implements BudgetChecker using Redis for real-time usage and a
// Repository for the monthly budget cap.
type checker struct {
	repo   Repository
	reader usage.UsageReader
}

// NewBudgetChecker creates a BudgetChecker.
func NewBudgetChecker(repo Repository, reader usage.UsageReader) BudgetChecker {
	return &checker{repo: repo, reader: reader}
}

func (c *checker) CheckBudget(ctx context.Context, tenantID domain.TenantID, model string, estimatedInputTokens, estimatedOutputTokens int) (BudgetCheckResult, error) {
	tenantIDStr := string(tenantID)

	// Get the monthly budget.
	budget, err := c.repo.GetMonthlyBudget(ctx, tenantIDStr)
	if err != nil {
		return BudgetCheckResult{}, fmt.Errorf("get monthly budget: %w", err)
	}

	// If budget is 0, no enforcement.
	if budget <= 0 {
		return BudgetCheckResult{Allowed: true}, nil
	}

	// Get current month-to-date usage from Redis.
	current, err := c.reader.GetCurrentUsage(ctx, tenantIDStr)
	if err != nil {
		return BudgetCheckResult{}, fmt.Errorf("get current usage: %w", err)
	}

	// Estimate the cost of this call.
	estimatedCallCost := computeCost(model, estimatedInputTokens, estimatedOutputTokens)

	// Check if this call would exceed the budget.
	monthToDate := current.TotalCostUSD
	if monthToDate+estimatedCallCost > budget {
		pct := (monthToDate / budget) * 100
		return BudgetCheckResult{
			Allowed:           false,
			EstimatedCallCost: estimatedCallCost,
			BudgetStatus: &BudgetStatus{
				TenantID:        tenantIDStr,
				MonthlyBudget:   budget,
				MonthToDateCost: monthToDate,
				RemainingBudget: budget - monthToDate,
				PercentUsed:     math.Round(pct*100) / 100,
				IsExceeded:      true,
			},
		}, nil
	}

	// Check alert thresholds.
	pct := (monthToDate / budget) * 100
	thresholdReached := ""
	if pct >= 95 {
		thresholdReached = "95%"
		log.Printf("BUDGET ALERT: tenant %s at %.1f%% of monthly budget ($%.2f / $%.2f) — 95%% threshold",
			tenantIDStr, pct, monthToDate, budget)
	} else if pct >= 80 {
		thresholdReached = "80%"
		log.Printf("BUDGET ALERT: tenant %s at %.1f%% of monthly budget ($%.2f / $%.2f) — 80%% threshold",
			tenantIDStr, pct, monthToDate, budget)
	}

	return BudgetCheckResult{
		Allowed:           true,
		EstimatedCallCost: estimatedCallCost,
		ThresholdReached:  thresholdReached,
		BudgetStatus: &BudgetStatus{
			TenantID:        tenantIDStr,
			MonthlyBudget:   budget,
			MonthToDateCost: monthToDate,
			RemainingBudget: budget - monthToDate,
			PercentUsed:     math.Round(pct*100) / 100,
			IsExceeded:      false,
		},
	}, nil
}

// computeCost estimates the cost of an LLM call using the usage module's pricing.
func computeCost(model string, inputTokens, outputTokens int) float64 {
	// Re-use the pricing table from the usage module.
	p, ok := modelPricing[model]
	if !ok {
		return 0
	}
	inputCost := (float64(inputTokens) / 1000.0) * p.InputPer1K
	outputCost := (float64(outputTokens) / 1000.0) * p.OutputPer1K
	return inputCost + outputCost
}

// modelPricing mirrors the usage module's pricing table.
// Shared here to avoid a circular dependency.
var modelPricing = map[string]struct {
	InputPer1K  float64
	OutputPer1K float64
}{
	"qwen-max":          {0.002, 0.006},
	"qwen-plus":         {0.001, 0.003},
	"qwen-turbo":        {0.0005, 0.0015},
	"text-embedding-v4": {0.0001, 0},
	"text-embedding-v3": {0.0001, 0},
}