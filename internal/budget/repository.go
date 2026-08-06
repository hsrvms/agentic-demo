package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/agentic-demo/platform/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Repository abstracts budget and invoice data access.
type Repository interface {
	GetMonthlyBudget(ctx context.Context, tenantID string) (float64, error)
	SetMonthlyBudget(ctx context.Context, tenantID string, budget float64) error
	CreateInvoice(ctx context.Context, tenantID string, periodStart, periodEnd time.Time, totalCost float64, lineItems []InvoiceLineItem, status InvoiceStatus) (Invoice, error)
	GetInvoiceByID(ctx context.Context, id uuid.UUID) (Invoice, error)
	ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (InvoicePage, error)
}

// pgRepository wraps sqlc-generated queries.
type pgRepository struct {
	queries *db.Queries
}

// NewRepository creates a budget Repository backed by PostgreSQL.
func NewRepository(queries *db.Queries) Repository {
	return &pgRepository{queries: queries}
}

func (r *pgRepository) GetMonthlyBudget(ctx context.Context, tenantID string) (float64, error) {
	n, err := r.queries.GetTenantBudget(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("get tenant budget: %w", err)
	}
	f, _ := n.Float64Value()
	return f.Float64, nil
}

func (r *pgRepository) SetMonthlyBudget(ctx context.Context, tenantID string, budget float64) error {
	var n pgtype.Numeric
	err := n.Scan(fmt.Sprintf("%f", budget))
	if err != nil {
		return fmt.Errorf("set monthly budget: %w", err)
	}
	return r.queries.UpdateTenantBudget(ctx, db.UpdateTenantBudgetParams{
		ID:               tenantID,
		MonthlyBudgetUsd: n,
	})
}

func (r *pgRepository) CreateInvoice(ctx context.Context, tenantID string, periodStart, periodEnd time.Time, totalCost float64, lineItems []InvoiceLineItem, status InvoiceStatus) (Invoice, error) {
	liJSON, err := json.Marshal(lineItems)
	if err != nil {
		return Invoice{}, fmt.Errorf("marshal line items: %w", err)
	}

	var costN pgtype.Numeric
	_ = costN.Scan(fmt.Sprintf("%f", totalCost))

	row, err := r.queries.CreateInvoice(ctx, db.CreateInvoiceParams{
		TenantID:     tenantID,
		PeriodStart:  toPgDate(periodStart),
		PeriodEnd:    toPgDate(periodEnd),
		TotalCostUsd: costN,
		LineItems:    liJSON,
		Status:       string(status),
	})
	if err != nil {
		return Invoice{}, fmt.Errorf("create invoice: %w", err)
	}

	return toDomainInvoice(&row), nil
}

func (r *pgRepository) GetInvoiceByID(ctx context.Context, id uuid.UUID) (Invoice, error) {
	row, err := r.queries.GetInvoiceByID(ctx, id)
	if err != nil {
		return Invoice{}, ErrNotFound
	}
	return toDomainInvoice(&row), nil
}

func (r *pgRepository) ListByTenant(ctx context.Context, tenantID string, page, pageSize int) (InvoicePage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := int32((page - 1) * pageSize) //nolint:gosec // page/pageSize bounded above

	rows, err := r.queries.ListInvoicesByTenant(ctx, db.ListInvoicesByTenantParams{
		TenantID: tenantID,
		Limit:    int32(pageSize),
		Offset:   offset,
	})
	if err != nil {
		return InvoicePage{}, fmt.Errorf("list invoices: %w", err)
	}

	total, err := r.queries.CountInvoicesByTenant(ctx, tenantID)
	if err != nil {
		return InvoicePage{}, fmt.Errorf("count invoices: %w", err)
	}

	invoices := make([]Invoice, len(rows))
	for i := range rows {
		invoices[i] = toDomainInvoice(&rows[i])
	}

	return InvoicePage{
		Invoices:   invoices,
		TotalCount: int(total),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

// --- domain conversion ---

func toDomainInvoice(row *db.Invoice) Invoice {
	var lineItems []InvoiceLineItem
	if len(row.LineItems) > 0 {
		_ = json.Unmarshal(row.LineItems, &lineItems)
	}
	if lineItems == nil {
		lineItems = []InvoiceLineItem{}
	}

	cost, _ := pgNumericToFloat64(row.TotalCostUsd)

	return Invoice{
		ID:           row.ID,
		TenantID:     row.TenantID,
		PeriodStart:  pgDateToTime(row.PeriodStart),
		PeriodEnd:    pgDateToTime(row.PeriodEnd),
		TotalCostUSD: cost,
		LineItems:    lineItems,
		Status:       InvoiceStatus(row.Status),
		CreatedAt:    row.CreatedAt,
	}
}

// --- pgtype helpers ---

func pgDateToTime(d pgtype.Date) time.Time {
	if d.Valid {
		return d.Time
	}
	return time.Time{}
}

func pgNumericToFloat64(n pgtype.Numeric) (float64, bool) {
	if !n.Valid {
		return 0, false
	}
	f, _ := n.Float64Value()
	return f.Float64, true
}

func toPgDate(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}
