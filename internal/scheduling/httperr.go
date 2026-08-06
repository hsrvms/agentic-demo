package scheduling

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/httperr"
)

func init() {
	httperr.Register(ErrScheduleNotFound, http.StatusNotFound, "schedule not found")
	httperr.Register(ErrScheduleAlreadyExists, http.StatusConflict, "schedule already exists for this tenant and type")
	httperr.Register(ErrInvalidCronExpr, http.StatusBadRequest, "invalid cron expression")
	httperr.Register(ErrInvalidScheduleType, http.StatusBadRequest, "invalid schedule type: must be 'daily', 'weekly', or 'monthly'")
	httperr.Register(ErrInvalidTenantID, http.StatusBadRequest, "tenant_id must not be empty")
}
