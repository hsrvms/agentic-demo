package auth

import (
	"net/http"

	"github.com/agentic-demo/platform/internal/httperr"
	"github.com/labstack/echo/v4"
)

// Handler holds the dependencies for auth HTTP endpoints.
type Handler struct {
	service AuthService
}

// NewHandler creates an auth Handler.
func NewHandler(service AuthService) *Handler {
	return &Handler{service: service}
}

// Register wires auth routes under the given Echo group.
// Public routes (register, login) are registered without middleware.
// The /auth/me endpoint is registered with the provided middleware.
func (h *Handler) Register(g *echo.Group, mw ...echo.MiddlewareFunc) {
	g.POST("/auth/register", h.HandleRegister)
	g.POST("/auth/login", h.HandleLogin)
	g.GET("/auth/me", h.HandleMe, mw...)
}

// --- request types ---

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// --- handlers ---

// HandleRegister handles POST /auth/register.
func (h *Handler) HandleRegister(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	user, err := h.service.Register(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	token, err := h.service.Login(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to issue token after registration")
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"user_id": user.ID,
		"email":   user.Email,
		"token":   token,
	})
}

// HandleLogin handles POST /auth/login.
func (h *Handler) HandleLogin(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	token, err := h.service.Login(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	return c.JSON(http.StatusOK, map[string]string{
		"token": token,
	})
}

// HandleMe handles GET /auth/me.
func (h *Handler) HandleMe(c echo.Context) error {
	tenantID := GetTenantID(c.Request().Context())
	userID := GetUserID(c.Request().Context())

	return c.JSON(http.StatusOK, map[string]interface{}{
		"user_id":   userID,
		"tenant_id": tenantID,
	})
}