package budget

import "errors"

var (
	ErrNotFound        = errors.New("invoice not found")
	ErrInvalidTenantID = errors.New("tenant_id must not be empty")
	ErrBudgetExceeded  = errors.New("budget exceeded: monthly limit would be surpassed")

	ErrInvalidPeriod     = errors.New("invalid billing period")
	ErrInvoiceExists     = errors.New("invoice already exists for this period")
	ErrInvalidStatus     = errors.New("invalid invoice status transition")
	ErrInvalidBudget     = errors.New("monthly_budget_usd must be >= 0")
)