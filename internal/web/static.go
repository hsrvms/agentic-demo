package web

import (
	"io/fs"
	"net/http"

	embedded "github.com/agentic-demo/platform/web"
	"github.com/labstack/echo/v4"
)

// StaticHandler returns an Echo handler that serves embedded static files
// from the web/static/ directory. Mount at /static/*.
func StaticHandler() echo.HandlerFunc {
	sub, err := fs.Sub(embedded.StaticFS, "static")
	if err != nil {
		panic("web: failed to create static sub-filesystem: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return func(c echo.Context) error {
		path := c.Param("*")
		c.Request().URL.Path = path
		fileServer.ServeHTTP(c.Response().Writer, c.Request())
		return nil
	}
}
