package sources

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler holds the dependencies for data source HTTP endpoints.
type Handler struct {
	service Service
}

// NewHandler creates a data source Handler.
func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// Register wires data source routes under the given Echo group with the provided middleware.
func (h *Handler) Register(g *echo.Group, mw ...echo.MiddlewareFunc) {
	g.GET("/sources", h.List, mw...)
	g.POST("/sources", h.Create, mw...)
	g.GET("/sources/:id", h.Get, mw...)
	g.PUT("/sources/:id", h.Update, mw...)
	g.DELETE("/sources/:id", h.Delete, mw...)
	g.POST("/sources/:id/test", h.TestConnection, mw...)
	g.POST("/sources/:id/sync", h.Sync, mw...)
}

// Create handles POST /api/sources.
func (h *Handler) Create(c echo.Context) error {
	tenantID := getTenantID(c)

	var req struct {
		SourceType  string          `json:"source_type"`
		Name        string          `json:"name"`
		Config      json.RawMessage `json:"config"`
		Credentials json.RawMessage `json:"credentials,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	var creds []byte
	if req.Credentials != nil {
		creds = []byte(req.Credentials)
	}

	ds, err := h.service.Create(c.Request().Context(), &CreateDataSourceParams{
		TenantID:    tenantID,
		SourceType:  SourceType(req.SourceType),
		Name:        req.Name,
		Config:      req.Config,
		Credentials: creds,
	})
	if err != nil {
		return mapError(err)
	}

	return c.JSON(http.StatusCreated, toResponse(&ds))
}

// List handles GET /api/sources.
func (h *Handler) List(c echo.Context) error {
	tenantID := getTenantID(c)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("limit"))

	result, err := h.service.ListByTenant(c.Request().Context(), tenantID, page, pageSize)
	if err != nil {
		return mapError(err)
	}

	items := make([]map[string]interface{}, len(result.Sources))
	for i := range result.Sources {
		items[i] = toResponse(&result.Sources[i])
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"sources":     items,
		"total_count": result.TotalCount,
		"page":        result.Page,
		"page_size":   result.PageSize,
	})
}

// Get handles GET /api/sources/:id.
func (h *Handler) Get(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	ds, err := h.service.GetByID(c.Request().Context(), id)
	if err != nil {
		return mapError(err)
	}

	return c.JSON(http.StatusOK, toResponseWithCredentials(&ds))
}

// Update handles PUT /api/sources/:id.
func (h *Handler) Update(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	var req struct {
		Name        *string          `json:"name,omitempty"`
		Config      *json.RawMessage `json:"config,omitempty"`
		Credentials *json.RawMessage `json:"credentials,omitempty"`
		Status      *string          `json:"status,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	params := UpdateDataSourceParams{
		Name:   req.Name,
		Config: req.Config,
		Status: (*Status)(req.Status),
	}
	if req.Credentials != nil {
		creds := []byte(*req.Credentials)
		params.Credentials = &creds
	}

	ds, err := h.service.Update(c.Request().Context(), id, params)
	if err != nil {
		return mapError(err)
	}

	return c.JSON(http.StatusOK, toResponse(&ds))
}

// Delete handles DELETE /api/sources/:id.
func (h *Handler) Delete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		return mapError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// TestConnection handles POST /api/sources/:id/test.
func (h *Handler) TestConnection(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	result, err := h.service.TestConnection(c.Request().Context(), id)
	if err != nil {
		return mapError(err)
	}

	status := http.StatusOK
	if !result.Success {
		status = http.StatusUnprocessableEntity
	}

	return c.JSON(status, result)
}

// Sync handles POST /api/sources/:id/sync.
func (h *Handler) Sync(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	if err := h.service.Sync(c.Request().Context(), id); err != nil {
		return mapError(err)
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "sync job enqueued",
	})
}

// --- helpers ---

func getTenantID(c echo.Context) string {
	return string(auth.GetTenantID(c.Request().Context()))
}

func mapError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalidTenantID):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidName):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidSourceType):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidConfig):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrEncryptionFailed):
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	case errors.Is(err, ErrDecryptionFailed):
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}

func toResponse(ds *DataSource) map[string]interface{} {
	resp := map[string]interface{}{
		"id":          ds.ID.String(),
		"tenant_id":   ds.TenantID,
		"source_type": string(ds.SourceType),
		"name":        ds.Name,
		"config":      ds.Config,
		"status":      string(ds.Status),
		"created_at":  ds.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated_at":  ds.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if ds.LastSyncAt != nil {
		resp["last_sync_at"] = ds.LastSyncAt.Format("2006-01-02T15:04:05Z")
	}
	if ds.LastSyncStatus != "" {
		resp["last_sync_status"] = ds.LastSyncStatus
	}
	return resp
}

func toResponseWithCredentials(ds *DataSource) map[string]interface{} {
	resp := toResponse(ds)
	if len(ds.Credentials) > 0 {
		resp["credentials"] = json.RawMessage(ds.Credentials)
	}
	return resp
}