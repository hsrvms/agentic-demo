package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-demo/platform/internal/auth"
	"github.com/agentic-demo/platform/internal/tenant"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestWebErrorHandler_InvalidCredentials(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	webErrorHandler(auth.ErrInvalidCredentials, c)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/login")
}

func TestWebErrorHandler_UserExists(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/register", http.NoBody)
	req.Header.Set("Referer", "/register")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	webErrorHandler(auth.ErrUserExists, c)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/register", rec.Header().Get("Location"))
}

func TestWebErrorHandler_TenantNotFound(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	webErrorHandler(tenant.ErrTenantNotFound, c)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

func TestWebErrorHandler_HTMXRequest(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test", http.NoBody)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	webErrorHandler(auth.ErrInvalidCredentials, c)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "sign in")
	assert.Equal(t, echo.MIMETextHTMLCharsetUTF8, rec.Header().Get(echo.HeaderContentType))
}

func TestWebErrorHandler_EchoHTTPError(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/missing", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	webErrorHandler(echo.NewHTTPError(http.StatusNotFound, "page not found"), c)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

func TestWebErrorHandler_UnknownError(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	webErrorHandler(assert.AnError, c)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

func TestWebErrorHandler_CommittedResponse(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Commit the response.
	rec.WriteHeader(http.StatusOK)
	_, _ = rec.WriteString("done")

	// Should not panic or modify the response.
	webErrorHandler(auth.ErrInvalidCredentials, c)
}

func TestMakeErrorHandler_APIPathDelegatesToDefault(t *testing.T) {
	e := echo.New()
	handler := MakeErrorHandler(e.HTTPErrorHandler)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/register", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler(auth.ErrUserExists, c)

	// API should get JSON with the correct status code, not a redirect.
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	assert.NotEmpty(t, body)
}

func TestMakeErrorHandler_WebPathUsesRedirects(t *testing.T) {
	e := echo.New()
	handler := MakeErrorHandler(e.HTTPErrorHandler)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler(auth.ErrInvalidCredentials, c)

	// Web path should redirect to login.
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/login")
}

func TestMakeErrorHandler_CommittedResponse(t *testing.T) {
	e := echo.New()
	handler := MakeErrorHandler(e.HTTPErrorHandler)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	rec.WriteHeader(http.StatusOK)
	_, _ = rec.WriteString("done")

	// Should not panic.
	handler(auth.ErrInvalidCredentials, c)
}

func TestReadFlashCookie(t *testing.T) {
	e := echo.New()

	t.Run("no cookie", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		assert.Nil(t, ReadFlashCookie(c))
	})

	t.Run("valid cookie", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		req.AddCookie(&http.Cookie{Name: "flash", Value: "success|Item saved"})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		msgs := ReadFlashCookie(c)
		assert.Len(t, msgs, 1)
		assert.Equal(t, "success", msgs[0].Intent)
		assert.Equal(t, "Item saved", msgs[0].Message)
	})

	t.Run("malformed cookie", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		req.AddCookie(&http.Cookie{Name: "flash", Value: "no-separator"})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		msgs := ReadFlashCookie(c)
		assert.Nil(t, msgs)
	})
}
