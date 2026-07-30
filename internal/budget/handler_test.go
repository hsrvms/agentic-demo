package budget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBudgetService implements BudgetService for testing.
type mockBudgetService struct {
	budgetStatus    *BudgetStatus
	budgetStatusErr error
	invoices        InvoicePage
	invoicesErr     error
	invoice         Invoice
	invoiceErr      error
}

func (m *mockBudgetService) GetBudgetStatus(ctx context.Context, tenantID domain.TenantID) (*BudgetStatus, error) {
	return m.budgetStatus, m.budgetStatusErr
}

func (m *mockBudgetService) SetMonthlyBudget(ctx context.Context, tenantID domain.TenantID, budget float64) error {
	return nil
}

func (m *mockBudgetService) ListInvoices(ctx context.Context, tenantID domain.TenantID, page, pageSize int) (InvoicePage, error) {
	return m.invoices, m.invoicesErr
}

func (m *mockBudgetService) GetInvoice(ctx context.Context, tenantID domain.TenantID, id uuid.UUID) (Invoice, error) {
	return m.invoice, m.invoiceErr
}

func (m *mockBudgetService) GenerateInvoice(ctx context.Context, tenantID domain.TenantID, periodStart, periodEnd time.Time) (Invoice, error) {
	return Invoice{}, nil
}

func TestHandler_GetBudgetStatus(t *testing.T) {
	svc := &mockBudgetService{
		budgetStatus: &BudgetStatus{
			TenantID:        "t_abc12345",
			MonthlyBudget:   100.0,
			MonthToDateCost: 30.0,
			RemainingBudget: 70.0,
			PercentUsed:     30.0,
			IsExceeded:      false,
		},
	}
	handler := NewHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/budget/status", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setTenantContext(c)

	err := handler.GetBudgetStatus(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp BudgetStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "t_abc12345", resp.TenantID)
	assert.Equal(t, 100.0, resp.MonthlyBudget)
	assert.Equal(t, 30.0, resp.MonthToDateCost)
}

func TestHandler_ListInvoices(t *testing.T) {
	invID := uuid.New()
	now := time.Now()

	svc := &mockBudgetService{
		invoices: InvoicePage{
			Invoices: []Invoice{
				{
					ID:           invID,
					TenantID:     "t_abc12345",
					PeriodStart:  time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
					PeriodEnd:    time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC),
					TotalCostUSD: 12.50,
					LineItems:    []InvoiceLineItem{},
					Status:       InvoiceDraft,
					CreatedAt:    now,
				},
			},
			TotalCount: 1,
			Page:       1,
			PageSize:   20,
		},
	}
	handler := NewHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/invoices", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setTenantContext(c)

	err := handler.ListInvoices(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["total_count"])

	invoices := resp["invoices"].([]interface{})
	assert.Len(t, invoices, 1)
}

func TestHandler_GetInvoice(t *testing.T) {
	invID := uuid.New()
	now := time.Now()

	svc := &mockBudgetService{
		invoice: Invoice{
			ID:           invID,
			TenantID:     "t_abc12345",
			PeriodStart:  time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:    time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC),
			TotalCostUSD: 12.50,
			LineItems: []InvoiceLineItem{
				{Model: "qwen-max", InputTokens: 1000, OutputTokens: 500, CostUSD: 0.005},
			},
			Status:    InvoiceDraft,
			CreatedAt: now,
		},
	}
	handler := NewHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/invoices/"+invID.String(), http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(invID.String())
	setTenantContext(c)

	err := handler.GetInvoice(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, invID.String(), resp["id"])
	assert.Equal(t, float64(12.5), resp["total_cost_usd"])
}

func TestHandler_GetInvoice_NotFound(t *testing.T) {
	svc := &mockBudgetService{
		invoiceErr: ErrNotFound,
	}
	handler := NewHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/invoices/"+uuid.New().String(), http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	setTenantContext(c)

	err := handler.GetInvoice(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

func TestHandler_GetInvoice_InvalidID(t *testing.T) {
	svc := &mockBudgetService{}
	handler := NewHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/invoices/not-a-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("not-a-uuid")
	setTenantContext(c)

	err := handler.GetInvoice(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

// setTenantContext sets the tenant ID in the request context
// so that auth.GetTenantID returns the expected value.
func setTenantContext(c echo.Context) {
	ctx := auth.SetTenantID(c.Request().Context(), domain.TenantID("t_abc12345"))
	c.SetRequest(c.Request().WithContext(ctx))
}