package budget

import (
	"context"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUsageReader implements usage.UsageReader for testing.
type mockUsageReader struct {
	currentUsage *usage.CurrentUsage
	err          error
}

func (m *mockUsageReader) GetCurrentUsage(ctx context.Context, tenantID string) (*usage.CurrentUsage, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.currentUsage, nil
}

func (m *mockUsageReader) Close() error { return nil }

func TestCheckBudget_NoBudget(t *testing.T) {
	repo := newMockRepository()
	reader := &mockUsageReader{}
	checker := NewBudgetChecker(repo, reader)

	// Budget is 0 (default) — no enforcement.
	result, err := checker.CheckBudget(context.Background(), "t_abc12345", "qwen-max", 1000, 500)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheckBudget_UnderBudget(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 10.0, // $10 used, $90 remaining
		},
	}

	checker := NewBudgetChecker(repo, reader)

	// qwen-max: 1000 input ($0.002) + 500 output ($0.003) = $0.005
	result, err := checker.CheckBudget(context.Background(), "t_abc12345", "qwen-max", 1000, 500)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, 0.005, result.EstimatedCallCost)
	require.NotNil(t, result.BudgetStatus)
	assert.Equal(t, 100.0, result.BudgetStatus.MonthlyBudget)
	assert.Equal(t, 10.0, result.BudgetStatus.MonthToDateCost)
	assert.Equal(t, 90.0, result.BudgetStatus.RemainingBudget)
	assert.False(t, result.BudgetStatus.IsExceeded)
}

func TestCheckBudget_Exceeded(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 99.999, // Very close to limit
		},
	}

	checker := NewBudgetChecker(repo, reader)

	// qwen-max: 1000 input ($0.002) + 500 output ($0.003) = $0.005
	// 99.999 + 0.005 = 100.004 > 100.0 → exceeded
	result, err := checker.CheckBudget(context.Background(), "t_abc12345", "qwen-max", 1000, 500)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	require.NotNil(t, result.BudgetStatus)
	assert.True(t, result.BudgetStatus.IsExceeded)
}

func TestCheckBudget_AlreadyExceeded(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 150.0, // Already over budget
		},
	}

	checker := NewBudgetChecker(repo, reader)

	result, err := checker.CheckBudget(context.Background(), "t_abc12345", "qwen-max", 100, 10)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	require.NotNil(t, result.BudgetStatus)
	assert.True(t, result.BudgetStatus.IsExceeded)
}

func TestCheckBudget_Threshold80(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 80.0, // Exactly at 80%
		},
	}

	checker := NewBudgetChecker(repo, reader)

	result, err := checker.CheckBudget(context.Background(), "t_abc12345", "qwen-max", 100, 10)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, "80%", result.ThresholdReached)
}

func TestCheckBudget_Threshold95(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 95.0, // Exactly at 95%
		},
	}

	checker := NewBudgetChecker(repo, reader)

	result, err := checker.CheckBudget(context.Background(), "t_abc12345", "qwen-max", 100, 10)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, "95%", result.ThresholdReached)
}

func TestCheckBudget_BelowThreshold(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 50.0, // 50% — no threshold
		},
	}

	checker := NewBudgetChecker(repo, reader)

	result, err := checker.CheckBudget(context.Background(), "t_abc12345", "qwen-max", 100, 10)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Empty(t, result.ThresholdReached)
}

func TestCheckBudget_UnknownModel(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 10.0,
		},
	}

	checker := NewBudgetChecker(repo, reader)

	// Unknown model has zero cost.
	result, err := checker.CheckBudget(context.Background(), "t_abc12345", "unknown-model", 1000000, 1000000)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, 0.0, result.EstimatedCallCost)
}

func TestCheckBudget_DomainTenantID(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 10.0,
		},
	}

	checker := NewBudgetChecker(repo, reader)

	// Use domain.TenantID type.
	result, err := checker.CheckBudget(context.Background(), domain.TenantID("t_abc12345"), "qwen-max", 1000, 500)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}
