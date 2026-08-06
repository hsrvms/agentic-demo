package budget

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/httperr"
)

func init() {
	httperr.Register(ErrNotFound, http.StatusNotFound, "invoice not found")
	httperr.Register(ErrInvalidTenantID, http.StatusBadRequest, "tenant_id must not be empty")
	httperr.Register(ErrInvalidBudget, http.StatusBadRequest, "monthly_budget_usd must be >= 0")
	httperr.Register(ErrInvalidPeriod, http.StatusBadRequest, "invalid billing period")
}
