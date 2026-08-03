package tenant

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/httperr"
)

func init() {
	httperr.Register(ErrInvalidName, http.StatusBadRequest, "tenant name must not be empty")
	httperr.Register(ErrTenantNotFound, http.StatusNotFound, "tenant not found")
	httperr.Register(ErrAlreadyExists, http.StatusConflict, "membership already exists")
	httperr.Register(ErrInvalidRole, http.StatusBadRequest, "invalid role: must be 'admin' or 'viewer'")
	httperr.Register(ErrMembershipNotFound, http.StatusNotFound, "membership not found")
}