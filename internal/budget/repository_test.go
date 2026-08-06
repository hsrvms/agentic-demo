package budget

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository_GetMonthlyBudget(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	repo.budgets["t_abc12345"] = 100.0

	budget, err := repo.GetMonthlyBudget(ctx, "t_abc12345")
	require.NoError(t, err)
	assert.Equal(t, 100.0, budget)
}

func TestRepository_GetMonthlyBudget_DefaultZero(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	budget, err := repo.GetMonthlyBudget(ctx, "t_unknown")
	require.NoError(t, err)
	assert.Equal(t, 0.0, budget)
}

func TestRepository_SetMonthlyBudget(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	err := repo.SetMonthlyBudget(ctx, "t_abc12345", 200.0)
	require.NoError(t, err)
	assert.Equal(t, 200.0, repo.budgets["t_abc12345"])
}

func TestRepository_CreateInvoice(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	lineItems := []InvoiceLineItem{
		{Model: "qwen-max", InputTokens: 1000, OutputTokens: 500, CostUSD: 0.005},
	}

	inv, err := repo.CreateInvoice(ctx, "t_abc12345", periodStart, periodEnd, 0.005, lineItems, InvoiceDraft)
	require.NoError(t, err)
	assert.Equal(t, "t_abc12345", inv.TenantID)
	assert.Equal(t, periodStart, inv.PeriodStart)
	assert.Equal(t, periodEnd, inv.PeriodEnd)
	assert.Equal(t, 0.005, inv.TotalCostUSD)
	assert.Equal(t, InvoiceDraft, inv.Status)
	assert.Len(t, inv.LineItems, 1)
	assert.Equal(t, "qwen-max", inv.LineItems[0].Model)
}

func TestRepository_GetInvoiceByID(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	created, err := repo.CreateInvoice(ctx, "t_abc12345", periodStart, periodEnd, 12.50, nil, InvoiceIssued)
	require.NoError(t, err)

	inv, err := repo.GetInvoiceByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, inv.ID)
	assert.Equal(t, 12.50, inv.TotalCostUSD)
	assert.Equal(t, InvoiceIssued, inv.Status)
}

func TestRepository_GetInvoiceByID_NotFound(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	_, err := repo.GetInvoiceByID(ctx, uuid.New())
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_ListInvoicesByTenant(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	_, err := repo.CreateInvoice(ctx, "t_abc12345", periodStart, periodEnd, 5.00, nil, InvoiceDraft)
	require.NoError(t, err)

	page, err := repo.ListByTenant(ctx, "t_abc12345", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, page.TotalCount)
	assert.Len(t, page.Invoices, 1)
	assert.Equal(t, 5.0, page.Invoices[0].TotalCostUSD)
}

func TestRepository_ListInvoicesByTenant_Empty(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	page, err := repo.ListByTenant(ctx, "t_abc12345", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 0, page.TotalCount)
	assert.Empty(t, page.Invoices)
}

func TestRepository_ListInvoicesByTenant_Pagination(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		_, err := repo.CreateInvoice(ctx, "t_abc12345", periodStart, periodEnd, 1.0, nil, InvoiceDraft)
		require.NoError(t, err)
	}

	page, err := repo.ListByTenant(ctx, "t_abc12345", 1, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, page.TotalCount)
	assert.Len(t, page.Invoices, 2)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 2, page.PageSize)
}

func TestRepository_TenantIsolation(t *testing.T) {
	repo := newMockRepository()
	ctx := context.Background()

	periodStart := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)

	_, err := repo.CreateInvoice(ctx, "tenant-a", periodStart, periodEnd, 1.0, nil, InvoiceDraft)
	require.NoError(t, err)
	_, err = repo.CreateInvoice(ctx, "tenant-b", periodStart, periodEnd, 2.0, nil, InvoiceDraft)
	require.NoError(t, err)

	pageA, err := repo.ListByTenant(ctx, "tenant-a", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, pageA.TotalCount)
	assert.Equal(t, "tenant-a", pageA.Invoices[0].TenantID)

	pageB, err := repo.ListByTenant(ctx, "tenant-b", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, pageB.TotalCount)
	assert.Equal(t, "tenant-b", pageB.Invoices[0].TenantID)
}

// --- mock repository ---

type mockRepository struct {
	invoices []Invoice
	budgets  map[string]float64
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		budgets: make(map[string]float64),
	}
}

func (m *mockRepository) GetMonthlyBudget(ctx context.Context, tenantID string) (float64, error) {
	if budget, ok := m.budgets[tenantID]; ok {
		return budget, nil
	}
	return 0, nil
}

func (m *mockRepository) SetMonthlyBudget(ctx context.Context, tenantID string, budget float64) error {
	m.budgets[tenantID] = budget
	return nil
}

func (m *mockRepository) CreateInvoice(ctx context.Context, tenantID string, periodStart, periodEnd time.Time, totalCost float64, lineItems []InvoiceLineItem, status InvoiceStatus) (Invoice, error) {
	if lineItems == nil {
		lineItems = []InvoiceLineItem{}
	}
	inv := Invoice{
		ID:           uuid.New(),
		TenantID:     tenantID,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
		TotalCostUSD: totalCost,
		LineItems:    lineItems,
		Status:       status,
		CreatedAt:    time.Now(),
	}
	m.invoices = append(m.invoices, inv)
	return inv, nil
}

func (m *mockRepository) GetInvoiceByID(ctx context.Context, id uuid.UUID) (Invoice, error) {
	for i := range m.invoices {
		if m.invoices[i].ID == id {
			return m.invoices[i], nil
		}
	}
	return Invoice{}, ErrNotFound
}

func (m *mockRepository) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (InvoicePage, error) {
	var filtered []Invoice
	for i := range m.invoices {
		if m.invoices[i].TenantID == tenantID {
			filtered = append(filtered, m.invoices[i])
		}
	}

	total := len(filtered)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	start := (page - 1) * pageSize
	if start >= total {
		return InvoicePage{Invoices: []Invoice{}, TotalCount: total, Page: page, PageSize: pageSize}, nil
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	return InvoicePage{
		Invoices:   filtered[start:end],
		TotalCount: total,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}
