package web

import (
	"bytes"
	"context"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

// Render writes a templ component to the response.
func Render(c echo.Context, status int, component templ.Component) error {
	c.Response().Status = status
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// IsHTMX reports whether the request is an HTMX partial update.
func IsHTMX(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

// RenderComponent renders the fragment for HTMX requests, or the full
// page for regular browser navigation.
func RenderComponent(c echo.Context, status int, page, fragment templ.Component) error {
	if IsHTMX(c) {
		return Render(c, status, fragment)
	}
	return Render(c, status, page)
}

// RenderToString renders a templ component to a string.
func RenderToString(ctx context.Context, component templ.Component) (string, error) {
	var buf bytes.Buffer
	if err := component.Render(ctx, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
