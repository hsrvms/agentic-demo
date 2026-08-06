package budget

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUsageRepository implements usage.Repository for testing.
type mockUsageRepository struct {
	dailySummary []usage.UsageDailyRecord
}

func (m *mockUsageRepository) CreateEvent(ctx context.Context, params *db.CreateUsageEventParams) (usage.UsageEventRecord, error) {
	return usage.UsageEventRecord{}, nil
}
func (m *mockUsageRepository) ListEvents(ctx context.Context, params *db.ListUsageEventsParams) ([]usage.UsageEventRecord, error) {
	return nil, nil
}
func (m *mockUsageRepository) CountEvents(ctx context.Context, params *db.CountUsageEventsParams) (int32, error) {
	return 0, nil
}
func (m *mockUsageRepository) UpsertDaily(ctx context.Context, params *db.UpsertUsageDailyParams) (usage.UsageDailyRecord, error) {
	return usage.UsageDailyRecord{}, nil
}
func (m *mockUsageRepository) GetDailySummary(ctx context.Context, params *db.GetUsageDailySummaryParams) ([]usage.UsageDailyRecord, error) {
	return m.dailySummary, nil
}
func (m *mockUsageRepository) CountDaily(ctx context.Context, params *db.CountUsageDailyParams) (int32, error) {
	return int32(len(m.dailySummary)), nil
}

func TestService_GetBudgetStatus(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 30.0,
		},
	}

	svc := NewService(repo, reader, &mockUsageRepository{})

	status, err := svc.GetBudgetStatus(context.Background(), "t_abc12345")
	require.NoError(t, err)
	assert.Equal(t, "t_abc12345", status.TenantID)
	assert.Equal(t, 100.0, status.MonthlyBudget)
	assert.Equal(t, 30.0, status.MonthToDateCost)
	assert.Equal(t, 70.0, status.RemainingBudget)
	assert.Equal(t, 30.0, status.PercentUsed)
	assert.False(t, status.IsExceeded)
}

func TestService_GetBudgetStatus_Exceeded(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 150.0,
		},
	}

	svc := NewService(repo, reader, &mockUsageRepository{})

	status, err := svc.GetBudgetStatus(context.Background(), "t_abc12345")
	require.NoError(t, err)
	assert.True(t, status.IsExceeded)
	assert.Equal(t, 0.0, status.RemainingBudget)
}

func TestService_GetBudgetStatus_NoBudget(t *testing.T) {
	repo := newMockRepository()
	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 30.0,
		},
	}

	svc := NewService(repo, reader, &mockUsageRepository{})

	status, err := svc.GetBudgetStatus(context.Background(), "t_abc12345")
	require.NoError(t, err)
	assert.Equal(t, 0.0, status.MonthlyBudget)
	assert.False(t, status.IsExceeded)
}

func TestService_GetBudgetStatus_EmptyTenantID(t *testing.T) {
	svc := NewService(newMockRepository(), &mockUsageReader{}, &mockUsageRepository{})

	_, err := svc.GetBudgetStatus(context.Background(), "")
	require.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestService_SetMonthlyBudget(t *testing.T) {
	repo := newMockRepository()
	svc := NewService(repo, &mockUsageReader{}, &mockUsageRepository{})

	err := svc.SetMonthlyBudget(context.Background(), "t_abc12345", 200.0)
	require.NoError(t, err)
	assert.Equal(t, 200.0, repo.budgets["t_abc12345"])
}

func TestService_SetMonthlyBudget_Negative(t *testing.T) {
	svc := NewService(newMockRepository(), &mockUsageReader{}, &mockUsageRepository{})

	err := svc.SetMonthlyBudget(context.Background(), "t_abc12345", -1.0)
	require.ErrorIs(t, err, ErrInvalidBudget)
}

func TestService_SetMonthlyBudget_EmptyTenantID(t *testing.T) {
	svc := NewService(newMockRepository(), &mockUsageReader{}, &mockUsageRepository{})

	err := svc.SetMonthlyBudget(context.Background(), "", 100.0)
	require.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestService_ListInvoices(t *testing.T) {
	repo := newMockRepository()
	svc := NewService(repo, &mockUsageReader{}, &mockUsageRepository{})

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	_, err := repo.CreateInvoice(context.Background(), "t_abc12345", periodStart, periodEnd, 10.0, nil, InvoiceDraft)
	require.NoError(t, err)

	page, err := svc.ListInvoices(context.Background(), "t_abc12345", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, page.TotalCount)
}

func TestService_ListInvoices_EmptyTenantID(t *testing.T) {
	svc := NewService(newMockRepository(), &mockUsageReader{}, &mockUsageRepository{})

	_, err := svc.ListInvoices(context.Background(), "", 1, 20)
	require.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestService_GetInvoice(t *testing.T) {
	repo := newMockRepository()
	svc := NewService(repo, &mockUsageReader{}, &mockUsageRepository{})

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	created, err := repo.CreateInvoice(context.Background(), "t_abc12345", periodStart, periodEnd, 10.0, nil, InvoiceDraft)
	require.NoError(t, err)

	inv, err := svc.GetInvoice(context.Background(), "t_abc12345", created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, inv.ID)
}

func TestService_GetInvoice_WrongTenant(t *testing.T) {
	repo := newMockRepository()
	svc := NewService(repo, &mockUsageReader{}, &mockUsageRepository{})

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	created, err := repo.CreateInvoice(context.Background(), "tenant-a", periodStart, periodEnd, 10.0, nil, InvoiceDraft)
	require.NoError(t, err)

	// Try to access as a different tenant.
	_, err = svc.GetInvoice(context.Background(), "tenant-b", created.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_GenerateInvoice(t *testing.T) {
	repo := newMockRepository()
	usageRepo := &mockUsageRepository{
		dailySummary: []usage.UsageDailyRecord{
			{Model: "qwen-max", InputTokens: 1000, OutputTokens: 500, EstimatedCostUSD: 0.005},
			{Model: "qwen-plus", InputTokens: 2000, OutputTokens: 1000, EstimatedCostUSD: 0.005},
		},
	}

	svc := NewService(repo, &mockUsageReader{}, usageRepo)

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	inv, err := svc.GenerateInvoice(context.Background(), "t_abc12345", periodStart, periodEnd)
	require.NoError(t, err)
	assert.Equal(t, "t_abc12345", inv.TenantID)
	assert.Equal(t, periodStart, inv.PeriodStart)
	assert.Equal(t, periodEnd, inv.PeriodEnd)
	assert.Equal(t, 0.01, inv.TotalCostUSD)
	assert.Equal(t, InvoiceDraft, inv.Status)
	assert.Len(t, inv.LineItems, 2)
}

func TestService_GenerateInvoice_EmptyTenantID(t *testing.T) {
	svc := NewService(newMockRepository(), &mockUsageReader{}, &mockUsageRepository{})

	_, err := svc.GenerateInvoice(context.Background(), "", time.Now(), time.Now().AddDate(0, 1, 0))
	require.ErrorIs(t, err, ErrInvalidTenantID)
}

func TestService_GenerateInvoice_InvalidPeriod(t *testing.T) {
	svc := NewService(newMockRepository(), &mockUsageReader{}, &mockUsageRepository{})

	// Start after end.
	_, err := svc.GenerateInvoice(context.Background(), "t_abc12345",
		time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC))
	require.ErrorIs(t, err, ErrInvalidPeriod)
}

func TestService_GenerateInvoice_NoUsage(t *testing.T) {
	repo := newMockRepository()
	usageRepo := &mockUsageRepository{
		dailySummary: []usage.UsageDailyRecord{},
	}

	svc := NewService(repo, &mockUsageReader{}, usageRepo)

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	inv, err := svc.GenerateInvoice(context.Background(), "t_abc12345", periodStart, periodEnd)
	require.NoError(t, err)
	assert.Equal(t, 0.0, inv.TotalCostUSD)
	assert.Empty(t, inv.LineItems)
}

func TestService_DomainTenantID(t *testing.T) {
	repo := newMockRepository()
	repo.budgets["t_abc12345"] = 100.0

	reader := &mockUsageReader{
		currentUsage: &usage.CurrentUsage{
			TenantID:     "t_abc12345",
			TotalCostUSD: 30.0,
		},
	}

	svc := NewService(repo, reader, &mockUsageRepository{})

	status, err := svc.GetBudgetStatus(context.Background(), domain.TenantID("t_abc12345"))
	require.NoError(t, err)
	assert.Equal(t, "t_abc12345", status.TenantID)
}

func TestService_GetInvoice_NotFound(t *testing.T) {
	svc := NewService(newMockRepository(), &mockUsageReader{}, &mockUsageRepository{})

	_, err := svc.GetInvoice(context.Background(), "t_abc12345", uuid.New())
	require.ErrorIs(t, err, ErrNotFound)
}
