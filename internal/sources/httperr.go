package sources

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/httperr"
)

func init() {
	httperr.Register(ErrNotFound, http.StatusNotFound, "data source not found")
	httperr.Register(ErrInvalidTenantID, http.StatusBadRequest, "tenant_id must not be empty")
	httperr.Register(ErrInvalidName, http.StatusBadRequest, "name must not be empty")
	httperr.Register(ErrInvalidSourceType, http.StatusBadRequest, "invalid source type")
	httperr.Register(ErrInvalidConfig, http.StatusBadRequest, "config must be valid JSON")
	httperr.Register(ErrEncryptionFailed, http.StatusInternalServerError, "internal server error")
	httperr.Register(ErrDecryptionFailed, http.StatusInternalServerError, "internal server error")
}
