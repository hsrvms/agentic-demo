package reports

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/httperr"
)

func init() {
	httperr.Register(ErrReportNotFound, http.StatusNotFound, "report not found")
	httperr.Register(ErrInvalidTenantID, http.StatusBadRequest, "tenant_id must not be empty")
}