package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/budget"
	"github.com/agentic-demo/platform/internal/domain"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockInvoiceService implements budget.BudgetService for invoicing handler
// tests. It embeds the interface so only the methods under test need to be
// defined; any other method call panics on the nil embedded interface.
type mockInvoiceService struct {
	budget.BudgetService
	page    budget.InvoicePage
	pageErr error
	invoice *budget.Invoice
	invErr  error
}

func (m *mockInvoiceService) ListInvoices(_ context.Context, _ domain.TenantID, page, pageSize int) (budget.InvoicePage, error) {
	if m.pageErr != nil {
		return budget.InvoicePage{}, m.pageErr
	}
	return m.page, nil
}

func (m *mockInvoiceService) GetInvoice(_ context.Context, tenantID domain.TenantID, _ uuid.UUID) (budget.Invoice, error) {
	if m.invErr != nil {
		return budget.Invoice{}, m.invErr
	}
	if m.invoice == nil {
		return budget.Invoice{}, budget.ErrNotFound
	}
	// Mirror the real service: tenant ownership is enforced before returning.
	if m.invoice.TenantID != string(tenantID) {
		return budget.Invoice{}, budget.ErrNotFound
	}
	return *m.invoice, nil
}

func newInvoiceContext(t *testing.T, target string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
	ctx := auth.SetTenantID(req.Context(), domain.TenantID("t_test"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func sampleInvoice(id uuid.UUID, status budget.InvoiceStatus, tenantID string) *budget.Invoice {
	return &budget.Invoice{
		ID:           id,
		TenantID:     tenantID,
		PeriodStart:  time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:    time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC),
		TotalCostUSD: 42.50,
		Status:       status,
		LineItems: []budget.InvoiceLineItem{
			{Model: "qwen-max", InputTokens: 1000, OutputTokens: 500, ToolCalls: 3, EmbeddingTokens: 2000, ReportsGenerated: 5, CostUSD: 30.00},
			{Model: "qwen-plus", InputTokens: 2000, OutputTokens: 1000, ToolCalls: 1, EmbeddingTokens: 0, ReportsGenerated: 2, CostUSD: 12.50},
		},
	}
}

func invoiceListPage(invoices ...*budget.Invoice) budget.InvoicePage {
	items := make([]budget.Invoice, len(invoices))
	for i := range invoices {
		items[i] = *invoices[i]
	}
	return budget.InvoicePage{Invoices: items, TotalCount: len(items), Page: 1, PageSize: 20}
}

// --- List tests ---

func TestInvoicesHandler_List_RendersTable(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	svc := &mockInvoiceService{
		page: invoiceListPage(
			sampleInvoice(id1, budget.InvoicePaid, "t_test"),
			sampleInvoice(id2, budget.InvoiceOverdue, "t_test"),
		),
	}
	handler := NewInvoicesHandler(svc)

	c, rec := newInvoiceContext(t, "/invoices")
	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// Row links render.
	assert.Contains(t, body, "/invoices/"+id1.String())
	assert.Contains(t, body, "/invoices/"+id2.String())
	// Period and cost render.
	assert.Contains(t, body, "Jul 01 — Jul 31, 2025")
	assert.Contains(t, body, "$42.50")
	// Status labels and their intent badge colors.
	assert.Contains(t, body, "Paid")
	assert.Contains(t, body, "Overdue")
	assert.Contains(t, body, "bg-intent-success/10 text-intent-success")
	assert.Contains(t, body, "bg-intent-error/10 text-intent-error")
}

func TestInvoicesHandler_List_StatusBadgeIntents(t *testing.T) {
	svc := &mockInvoiceService{
		page: invoiceListPage(
			sampleInvoice(uuid.New(), budget.InvoiceDraft, "t_test"),
			sampleInvoice(uuid.New(), budget.InvoiceIssued, "t_test"),
			sampleInvoice(uuid.New(), budget.InvoicePaid, "t_test"),
			sampleInvoice(uuid.New(), budget.InvoiceOverdue, "t_test"),
		),
	}
	handler := NewInvoicesHandler(svc)

	c, rec := newInvoiceContext(t, "/invoices")
	err := handler.List(c)
	require.NoError(t, err)
	body := rec.Body.String()

	// draft=gray(muted), issued=info, paid=success, overdue=error.
	assert.Contains(t, body, "Draft")
	assert.Contains(t, body, "Issued")
	assert.Contains(t, body, "bg-content-muted/10 text-content-muted")
	assert.Contains(t, body, "bg-intent-info/10 text-intent-info")
	assert.Contains(t, body, "bg-intent-success/10 text-intent-success")
	assert.Contains(t, body, "bg-intent-error/10 text-intent-error")
}

func TestInvoicesHandler_List_EmptyState(t *testing.T) {
	svc := &mockInvoiceService{page: budget.InvoicePage{Invoices: []budget.Invoice{}, TotalCount: 0, Page: 1, PageSize: 20}}
	handler := NewInvoicesHandler(svc)

	c, rec := newInvoiceContext(t, "/invoices")
	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "No invoices yet. Invoices are generated at the start of each billing period.")
}

func TestInvoicesHandler_List_HTMXPaginationFragment(t *testing.T) {
	svc := &mockInvoiceService{
		page: budget.InvoicePage{
			Invoices:   []budget.Invoice{*sampleInvoice(uuid.New(), budget.InvoiceIssued, "t_test")},
			TotalCount: 25,
			Page:       2,
			PageSize:   20,
		},
	}
	handler := NewInvoicesHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/invoices?page=2", http.NoBody)
	req.Header.Set("HX-Request", "true")
	ctx := auth.SetTenantID(req.Context(), domain.TenantID("t_test"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "Jul 01 — Jul 31, 2025")
	// Fragment must not render the full page shell.
	assert.Contains(t, body, "<tr")
	assert.NotContains(t, body, "<html")
}

func TestInvoicesHandler_List_ServiceError(t *testing.T) {
	svc := &mockInvoiceService{pageErr: assert.AnError}
	handler := NewInvoicesHandler(svc)

	c, _ := newInvoiceContext(t, "/invoices")
	err := handler.List(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}

// --- Detail tests ---

func TestInvoicesHandler_Detail_RendersLineItems(t *testing.T) {
	id := uuid.New()
	svc := &mockInvoiceService{invoice: sampleInvoice(id, budget.InvoiceIssued, "t_test")}
	handler := NewInvoicesHandler(svc)

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/invoices/"+id.String(), http.NoBody)
	ctx := auth.SetTenantID(req.Context(), domain.TenantID("t_test"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	err := handler.Detail(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// Period dates, status, and total.
	assert.Contains(t, body, "Jul 01, 2025")
	assert.Contains(t, body, "Jul 31, 2025")
	assert.Contains(t, body, "Issued")
	assert.Contains(t, body, "$42.50")
	// Line-item breakdown.
	assert.Contains(t, body, "qwen-max")
	assert.Contains(t, body, "qwen-plus")
	assert.Contains(t, body, "1.0K") // 1000 input tokens formatted
	assert.Contains(t, body, "3")    // tool calls
	assert.Contains(t, body, "2.0K") // embedding tokens
	assert.Contains(t, body, "5")    // reports generated
	assert.Contains(t, body, "$30.00")
	assert.Contains(t, body, "$12.50")
	// Back link to the list.
	assert.Contains(t, body, "Back to Invoices")
}

func TestInvoicesHandler_Detail_InvalidID(t *testing.T) {
	handler := NewInvoicesHandler(&mockInvoiceService{})

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/invoices/bad", http.NoBody)
	ctx := auth.SetTenantID(req.Context(), domain.TenantID("t_test"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("bad")

	err := handler.Detail(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestInvoicesHandler_Detail_NotFound(t *testing.T) {
	svc := &mockInvoiceService{invErr: budget.ErrNotFound}
	handler := NewInvoicesHandler(svc)

	c, _ := newInvoiceContext(t, "/invoices/"+uuid.New().String())
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())

	err := handler.Detail(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

func TestInvoicesHandler_Detail_CrossTenantDenied(t *testing.T) {
	// The service enforces tenant ownership by returning ErrNotFound when the
	// invoice belongs to another tenant. The handler must surface a 404.
	other := sampleInvoice(uuid.New(), budget.InvoiceDraft, "t_other")
	svc := &mockInvoiceService{invoice: other}
	handler := NewInvoicesHandler(svc)

	c, _ := newInvoiceContext(t, "/invoices/"+other.ID.String())
	c.SetParamNames("id")
	c.SetParamValues(other.ID.String())

	err := handler.Detail(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
	assert.NotContains(t, c.Response().Writer.(*httptest.ResponseRecorder).Body.String(), "qwen-max")
}
