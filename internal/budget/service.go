package budget

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/usage"
	"github.com/google/uuid"
)

// BudgetService is the public interface for budget operations.
type BudgetService interface {
	GetBudgetStatus(ctx context.Context, tenantID domain.TenantID) (*BudgetStatus, error)
	SetMonthlyBudget(ctx context.Context, tenantID domain.TenantID, budget float64) error
	ListInvoices(ctx context.Context, tenantID domain.TenantID, page, pageSize int) (InvoicePage, error)
	GetInvoice(ctx context.Context, tenantID domain.TenantID, id uuid.UUID) (Invoice, error)
	GenerateInvoice(ctx context.Context, tenantID domain.TenantID, periodStart, periodEnd time.Time) (Invoice, error)
}

type budgetService struct {
	repo      Repository
	reader    usage.UsageReader
	usageRepo usage.Repository
}

// NewService creates a BudgetService.
func NewService(repo Repository, reader usage.UsageReader, usageRepo usage.Repository) BudgetService {
	return &budgetService{
		repo:      repo,
		reader:    reader,
		usageRepo: usageRepo,
	}
}

func (s *budgetService) GetBudgetStatus(ctx context.Context, tenantID domain.TenantID) (*BudgetStatus, error) {
	tenantIDStr := string(tenantID)
	if strings.TrimSpace(tenantIDStr) == "" {
		return nil, ErrInvalidTenantID
	}

	budget, err := s.repo.GetMonthlyBudget(ctx, tenantIDStr)
	if err != nil {
		return nil, fmt.Errorf("get budget: %w", err)
	}

	current, err := s.reader.GetCurrentUsage(ctx, tenantIDStr)
	if err != nil {
		return nil, fmt.Errorf("get current usage: %w", err)
	}

	monthToDate := current.TotalCostUSD
	remaining := budget - monthToDate
	if remaining < 0 {
		remaining = 0
	}

	var pct float64
	if budget > 0 {
		pct = (monthToDate / budget) * 100
	}

	return &BudgetStatus{
		TenantID:        tenantIDStr,
		MonthlyBudget:   budget,
		MonthToDateCost: monthToDate,
		RemainingBudget: remaining,
		PercentUsed:     pct,
		IsExceeded:      budget > 0 && monthToDate >= budget,
	}, nil
}

func (s *budgetService) SetMonthlyBudget(ctx context.Context, tenantID domain.TenantID, budget float64) error {
	tenantIDStr := string(tenantID)
	if strings.TrimSpace(tenantIDStr) == "" {
		return ErrInvalidTenantID
	}
	if budget < 0 {
		return ErrInvalidBudget
	}
	return s.repo.SetMonthlyBudget(ctx, tenantIDStr, budget)
}

func (s *budgetService) ListInvoices(ctx context.Context, tenantID domain.TenantID, page, pageSize int) (InvoicePage, error) {
	tenantIDStr := string(tenantID)
	if strings.TrimSpace(tenantIDStr) == "" {
		return InvoicePage{}, ErrInvalidTenantID
	}

	return s.repo.ListByTenant(ctx, tenantIDStr, page, pageSize)
}

func (s *budgetService) GetInvoice(ctx context.Context, tenantID domain.TenantID, id uuid.UUID) (Invoice, error) {
	inv, err := s.repo.GetInvoiceByID(ctx, id)
	if err != nil {
		return Invoice{}, err
	}

	// Verify tenant ownership.
	if inv.TenantID != string(tenantID) {
		return Invoice{}, ErrNotFound
	}

	return inv, nil
}

func (s *budgetService) GenerateInvoice(ctx context.Context, tenantID domain.TenantID, periodStart, periodEnd time.Time) (Invoice, error) {
	tenantIDStr := string(tenantID)
	if strings.TrimSpace(tenantIDStr) == "" {
		return Invoice{}, ErrInvalidTenantID
	}

	if periodStart.IsZero() || periodEnd.IsZero() || !periodEnd.After(periodStart) {
		return Invoice{}, ErrInvalidPeriod
	}

	// Read usage_daily aggregates for the billing period.
	rows, err := s.usageRepo.GetDailySummary(ctx, &db.GetUsageDailySummaryParams{
		TenantID: tenantIDStr,
		Column2:  toPgDate(periodStart),
		Column3:  toPgDate(periodEnd),
	})
	if err != nil {
		return Invoice{}, fmt.Errorf("get usage daily summary: %w", err)
	}

	// Convert to line items and compute total.
	lineItems := make([]InvoiceLineItem, len(rows))
	var totalCost float64
	for i, row := range rows {
		li := InvoiceLineItem{
			Model:            row.Model,
			InputTokens:      row.InputTokens,
			OutputTokens:     row.OutputTokens,
			ToolCalls:        row.ToolCalls,
			EmbeddingTokens:  row.EmbeddingTokens,
			ReportsGenerated: row.ReportsGenerated,
			CostUSD:          row.EstimatedCostUSD,
		}
		lineItems[i] = li
		totalCost += row.EstimatedCostUSD
	}

	return s.repo.CreateInvoice(ctx, tenantIDStr, periodStart, periodEnd, totalCost, lineItems, InvoiceDraft)
}
