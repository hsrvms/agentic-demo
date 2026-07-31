package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestStaticHandler_ServesRobots(t *testing.T) {
	e := echo.New()
	e.GET("/static/*", StaticHandler())

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/robots.txt", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "User-agent")
}

func TestStaticHandler_NotFound(t *testing.T) {
	e := echo.New()
	e.GET("/static/*", StaticHandler())

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/nonexistent.txt", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
