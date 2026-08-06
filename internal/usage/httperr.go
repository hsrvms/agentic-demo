package usage

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/httperr"
)

func init() {
	httperr.Register(ErrInvalidTenantID, http.StatusBadRequest, "usage: invalid tenant ID")
	httperr.Register(ErrInvalidDateRange, http.StatusBadRequest, "usage: invalid date range")
}
