package usage

import (
	"net/http"
	"strconv"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/httperr"
	"github.com/labstack/echo/v4"
)

// Handler holds the dependencies for usage HTTP endpoints.
type Handler struct {
	service UsageService
}

// NewHandler creates a usage Handler.
func NewHandler(service UsageService) *Handler {
	return &Handler{service: service}
}

// Register wires usage routes under the given Echo group with the provided middleware.
func (h *Handler) Register(g *echo.Group, mw ...echo.MiddlewareFunc) {
	g.GET("/usage/summary", h.GetSummary, mw...)
	g.GET("/usage/current", h.GetCurrent, mw...)
	g.GET("/usage/events", h.ListEvents, mw...)
}

// GetSummary returns aggregated usage data for a date range.
func (h *Handler) GetSummary(c echo.Context) error {
	tenantID := getTenantID(c)

	from := c.QueryParam("from")
	to := c.QueryParam("to")

	summary, err := h.service.GetSummary(c.Request().Context(), tenantID, from, to)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	models := make([]map[string]interface{}, len(summary.Models))
	for i, m := range summary.Models {
		models[i] = map[string]interface{}{
			"model":            m.Model,
			"input_tokens":     m.InputTokens,
			"output_tokens":    m.OutputTokens,
			"tool_calls":       m.ToolCalls,
			"embedding_tokens": m.EmbeddingTokens,
			"cost_usd":         m.CostUSD,
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"tenant_id":      summary.TenantID,
		"from":           summary.From.Format("2006-01-02"),
		"to":             summary.To.Format("2006-01-02"),
		"total_cost_usd": summary.TotalCostUSD,
		"models":         models,
	})
}

// GetCurrent returns real-time usage for the current month.
func (h *Handler) GetCurrent(c echo.Context) error {
	tenantID := getTenantID(c)

	current, err := h.service.GetCurrentUsage(c.Request().Context(), tenantID)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	byModel := make([]map[string]interface{}, len(current.ByModel))
	for i, m := range current.ByModel {
		byModel[i] = map[string]interface{}{
			"model":            m.Model,
			"input_tokens":     m.InputTokens,
			"output_tokens":    m.OutputTokens,
			"tool_calls":       m.ToolCalls,
			"embedding_tokens": m.EmbeddingTokens,
			"cost_usd":         m.CostUSD,
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"tenant_id":              current.TenantID,
		"period_start":           current.PeriodStart.Format("2006-01-02"),
		"period_end":             current.PeriodEnd.Format("2006-01-02"),
		"total_input_tokens":     current.TotalInputTokens,
		"total_output_tokens":    current.TotalOutputTokens,
		"total_tool_calls":       current.TotalToolCalls,
		"total_embedding_tokens": current.TotalEmbeddingTokens,
		"total_cost_usd":         current.TotalCostUSD,
		"reports_generated":      current.ReportsGenerated,
		"by_model":               byModel,
	})
}

// ListEvents returns the raw usage event log.
func (h *Handler) ListEvents(c echo.Context) error {
	tenantID := getTenantID(c)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("limit"))

	result, err := h.service.ListEvents(c.Request().Context(), tenantID, page, pageSize)
	if err != nil {
		return echo.NewHTTPError(httperr.MapHTTP(err))
	}

	events := make([]map[string]interface{}, len(result.Events))
	for i, e := range result.Events {
		events[i] = map[string]interface{}{
			"id":         e.ID.String(),
			"event_type": e.EventType,
			"payload":    e.Payload,
			"created_at": e.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"events":      events,
		"total_count": result.TotalCount,
		"page":        result.Page,
		"page_size":   result.PageSize,
	})
}

// --- helpers ---

func getTenantID(c echo.Context) string {
	return string(auth.GetTenantID(c.Request().Context()))
}

