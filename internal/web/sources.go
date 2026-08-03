package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/sources"
	"github.com/agentic-demo/platform/internal/webui"
	webpages "github.com/agentic-demo/platform/web/templates/pages"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const sourcesPageSize = 20

// SourcesHandler serves the data source management pages.
type SourcesHandler struct {
	core *sources.HandlerCore
}

// NewSourcesHandler creates a SourcesHandler.
func NewSourcesHandler(core *sources.HandlerCore) *SourcesHandler {
	return &SourcesHandler{core: core}
}

// Register mounts sources routes on the authenticated web group.
func (h *SourcesHandler) Register(g *echo.Group) {
	g.GET("/sources", h.List)
	g.GET("/sources/new", h.NewForm)
	g.GET("/sources/:id", h.Detail)
	g.GET("/sources/:id/edit", h.EditForm)
	g.POST("/sources", h.Create)
	g.PUT("/sources/:id", h.Update)
	g.DELETE("/sources/:id", h.Delete)
	g.POST("/sources/:id/test", h.TestConnection)
	g.POST("/sources/:id/sync", h.Sync)
}

// List handles GET /sources.
func (h *SourcesHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	result, err := h.core.List(ctx, tenantID, page, sourcesPageSize)
	if err != nil {
		log.Printf("sources list error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load sources")
	}

	items := make([]webui.SourceItem, len(result.Sources))
	for i := range result.Sources {
		items[i] = webui.MapSourceItem(&result.Sources[i])
	}

	csrf := GetCSRFToken(ctx)

	hasMore := (page * sourcesPageSize) < result.TotalCount
	nextPage := page + 1

	if IsHTMX(c) {
		return Render(c, http.StatusOK, webpages.SourceRowsFragment(items, csrf))
	}

	data := webui.MapSourceList(sources.DataSourcePage(result))
	data.HasMore = hasMore
	data.NextPage = nextPage

	flashes := GetFlashMessages(ctx)
	return Render(c, http.StatusOK, webpages.SourcesList(data, csrf, flashes))
}

// NewForm handles GET /sources/new.
func (h *SourcesHandler) NewForm(c echo.Context) error {
	csrf := GetCSRFToken(c.Request().Context())
	flashes := GetFlashMessages(c.Request().Context())

	data := webui.SourceFormData{
		CSRFToken:   csrf,
		TypeOptions: webui.SourceTypeOptions(),
	}

	return Render(c, http.StatusOK, webpages.SourceForm(data, flashes))
}

// Detail handles GET /sources/:id.
func (h *SourcesHandler) Detail(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	result, err := h.core.Get(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, sources.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "source not found")
		}
		log.Printf("sources detail error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load source")
	}

	csrf := GetCSRFToken(c.Request().Context())
	flashes := GetFlashMessages(c.Request().Context())

	data := webui.MapSourceDetail(&result.DataSource)

	return Render(c, http.StatusOK, webpages.SourceDetail(data, csrf, flashes))
}

// EditForm handles GET /sources/:id/edit.
func (h *SourcesHandler) EditForm(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	result, err := h.core.Get(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, sources.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "source not found")
		}
		log.Printf("sources edit error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load source")
	}

	ds := result.DataSource
	csrf := GetCSRFToken(c.Request().Context())
	flashes := GetFlashMessages(c.Request().Context())

	formData := webui.SourceFormData{
		Editing:     true,
		SourceID:    ds.ID.String(),
		Name:        ds.Name,
		SourceType:  string(ds.SourceType),
		ConfigURL:   extractConfigURL(ds.Config),
		CSRFToken:   csrf,
		TypeOptions: webui.SourceTypeOptions(),
	}

	return Render(c, http.StatusOK, webpages.SourceForm(formData, flashes))
}

// Create handles POST /sources (including multipart file uploads).
func (h *SourcesHandler) Create(c echo.Context) error {
	ctx := c.Request().Context()
	tenantID := string(auth.GetTenantID(ctx))

	name := c.FormValue("name")
	sourceType := c.FormValue("source_type")

	config, creds, err := buildConfigAndCreds(sourceType, c)
	if err != nil {
		return h.formError(c, err)
	}

	result, err := h.core.Create(ctx, tenantID, &sources.CreateDataSourceParams{
		TenantID:    tenantID,
		SourceType:  sources.SourceType(sourceType),
		Name:        name,
		Config:      config,
		Credentials: creds,
	})
	if err != nil {
		return h.formError(c, err)
	}

	// File uploads redirect to the list with a processing flash.
	if sources.SourceType(sourceType) == sources.SourceTypeFileUpload {
		if IsHTMX(c) {
			c.Response().Header().Set("HX-Redirect", "/sources")
			return c.NoContent(http.StatusOK)
		}
		setFlashCookie(c, webui.Flash{Intent: "success", Message: "File uploaded, processing…"})
		return c.Redirect(http.StatusSeeOther, "/sources")
	}

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/sources/"+result.DataSource.ID.String())
		return c.NoContent(http.StatusOK)
	}

	setFlashCookie(c, webui.Flash{Intent: "success", Message: "Source created successfully"})
	return c.Redirect(http.StatusSeeOther, "/sources/"+result.DataSource.ID.String())
}

// Update handles PUT /sources/:id.
func (h *SourcesHandler) Update(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	name := c.FormValue("name")
	sourceType := c.FormValue("source_type")

	params := sources.UpdateDataSourceParams{}
	if name != "" {
		params.Name = &name
	}

	config, creds, err := buildConfigAndCreds(sourceType, c)
	if err != nil {
		return h.formError(c, err)
	}
	if len(config) > 0 {
		params.Config = &config
	}
	if len(creds) > 0 {
		params.Credentials = &creds
	}

	_, err = h.core.Update(c.Request().Context(), id, params)
	if err != nil {
		return h.formError(c, err)
	}

	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/sources/"+id.String())
		return c.NoContent(http.StatusOK)
	}

	setFlashCookie(c, webui.Flash{Intent: "success", Message: "Source updated successfully"})
	return c.Redirect(http.StatusSeeOther, "/sources/"+id.String())
}

// Delete handles DELETE /sources/:id.
func (h *SourcesHandler) Delete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	if _, err := h.core.Delete(c.Request().Context(), id); err != nil {
		if errors.Is(err, sources.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "source not found")
		}
		log.Printf("sources delete error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete source")
	}

	if IsHTMX(c) {
		return c.NoContent(http.StatusOK)
	}

	setFlashCookie(c, webui.Flash{Intent: "success", Message: "Source deleted"})
	return c.Redirect(http.StatusSeeOther, "/sources")
}

// TestConnection handles POST /sources/:id/test.
func (h *SourcesHandler) TestConnection(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	result, err := h.core.TestConnection(c.Request().Context(), id)
	if err != nil {
		log.Printf("sources test error: %v", err)
		return Render(c, http.StatusOK, webpages.StatusFragment(false, "Failed to test connection", ""))
	}

	return Render(c, http.StatusOK, webpages.StatusFragment(result.Success, result.Message, result.Latency))
}

// Sync handles POST /sources/:id/sync.
func (h *SourcesHandler) Sync(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source ID")
	}

	if _, err := h.core.Sync(c.Request().Context(), id); err != nil {
		log.Printf("sources sync error: %v", err)
		return Render(c, http.StatusOK, webpages.StatusFragment(false, "Failed to trigger sync", ""))
	}

	return Render(c, http.StatusOK, webpages.SyncTriggeredFragment())
}

// --- helpers ---

// formError maps a service error to an HTMX error fragment or redirects with flash.
func (h *SourcesHandler) formError(c echo.Context, err error) error {
	message := "Something went wrong"
	switch {
	case errors.Is(err, sources.ErrInvalidName):
		message = "Please enter a name for the source"
	case errors.Is(err, sources.ErrInvalidSourceType):
		message = "Please select a valid source type"
	case errors.Is(err, sources.ErrInvalidConfig):
		message = "Configuration is invalid"
	case errors.Is(err, sources.ErrNotFound):
		message = "Source not found"
	}

	if IsHTMX(c) {
		return Render(c, http.StatusOK, webpages.ErrorFragment(message))
	}

	setFlashCookie(c, webui.Flash{Intent: "error", Message: message})
	referer := c.Request().Referer()
	if referer == "" {
		referer = "/sources"
	}
	return c.Redirect(http.StatusSeeOther, referer)
}

// buildConfigAndCreds extracts type-specific config and credentials from form values
// or multipart file uploads.
func buildConfigAndCreds(sourceType string, c echo.Context) (config json.RawMessage, credentials []byte, err error) {
	switch sources.SourceType(sourceType) {
	case sources.SourceTypeFileUpload:
		file, ferr := c.FormFile("file")
		if ferr != nil {
			return nil, nil, fmt.Errorf("file is required")
		}
		src, ferr := file.Open()
		if ferr != nil {
			return nil, nil, fmt.Errorf("failed to open uploaded file")
		}
		defer src.Close()
		content, ferr := io.ReadAll(src)
		if ferr != nil {
			return nil, nil, fmt.Errorf("failed to read uploaded file")
		}
		cfg := map[string]string{
			"filename": file.Filename,
			"size":     fmt.Sprintf("%d", file.Size),
		}
		raw, _ := json.Marshal(cfg)
		return raw, content, nil
	case sources.SourceTypeWebsite:
		url := c.FormValue("config_url")
		if url != "" {
			cfg := map[string]string{"url": url}
			raw, _ := json.Marshal(cfg)
			return raw, nil, nil
		}
	case sources.SourceTypeCRMHubSpot, sources.SourceTypeCRMSalesforce:
		apiKey := c.FormValue("config_api_key")
		if apiKey != "" {
			cfg := map[string]string{"api_key": apiKey}
			raw, _ := json.Marshal(cfg)
			return raw, []byte(apiKey), nil
		}
	}
	return nil, nil, nil
}

// extractConfigURL pulls the URL from a website source's config JSON.
func extractConfigURL(config json.RawMessage) string {
	if len(config) == 0 {
		return ""
	}
	var cfg struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return ""
	}
	return cfg.URL
}