package tenant

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/httperr"
	"github.com/labstack/echo/v4"
)

// Handler holds the dependencies for tenant HTTP endpoints.
type Handler struct {
	service TenantService
}

// NewHandler creates a tenant Handler.
func NewHandler(service TenantService) *Handler {
	return &Handler{service: service}
}

// Register wires tenant routes under the given Echo group with the provided middleware.
func (h *Handler) Register(g *echo.Group, mw ...echo.MiddlewareFunc) {
	g.POST("/tenants", h.Create, mw...)
	g.GET("/tenants", h.List, mw...)
}

// --- request types ---

type createTenantRequest struct {
	Name string `json:"name"`
}

// --- handlers ---

// Create handles POST /tenants.
func (h *Handler) Create(c echo.Context) error {
	var req createTenantRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	userID := domain.GetUserID(c.Request().Context())

	t, err := h.service.Create(c.Request().Context(), userID, req.Name)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id":     t.ID,
		"name":   t.Name,
		"status": t.Status,
	})
}

// List handles GET /tenants.
func (h *Handler) List(c echo.Context) error {
	userID := domain.GetUserID(c.Request().Context())

	tenants, err := h.service.ListByUser(c.Request().Context(), userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list tenants")
	}

	type tenantResponse struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	result := make([]tenantResponse, len(tenants))
	for i, t := range tenants {
		result[i] = tenantResponse{
			ID:     string(t.ID),
			Name:   t.Name,
			Status: string(t.Status),
		}
	}

	return c.JSON(http.StatusOK, result)
}