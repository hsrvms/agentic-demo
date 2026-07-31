package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSRF_GETSetsToken(t *testing.T) {
	e := echo.New()
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))
	e.GET("/test", func(c echo.Context) error {
		token := GetCSRFToken(c.Request().Context())
		return c.String(http.StatusOK, token)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Body.String())

	// Should set a cookie.
	cookies := rec.Result().Cookies()
	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == csrfCookieName {
			csrfCookie = c
		}
	}
	require.NotNil(t, csrfCookie, "CSRF cookie should be set")
	assert.Equal(t, rec.Body.String(), csrfCookie.Value)
}

func TestCSRF_POSTWithoutToken(t *testing.T) {
	e := echo.New()
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))
	e.POST("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRF_POSTWithMatchingToken(t *testing.T) {
	e := echo.New()
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))
	e.POST("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	token := "test-token-" + strings.Repeat("a", 50)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test", http.NoBody)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

func TestCSRF_POSTWithMismatchedToken(t *testing.T) {
	e := echo.New()
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))
	e.POST("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test", http.NoBody)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "correct-token"})
	req.Header.Set(csrfHeaderName, "wrong-token")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRF_POSTWithFormField(t *testing.T) {
	e := echo.New()
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))
	e.POST("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	token := "form-token-value"
	form := url.Values{}
	form.Set(csrfFieldName, token)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRF_PassThroughSafeMethods(t *testing.T) {
	e := echo.New()
	e.Use(CSRFMiddleware(DefaultCSRFConfig()))
	e.OPTIONS("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
