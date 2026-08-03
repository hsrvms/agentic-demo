package web

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/agentic-demo/platform/internal/httperr"
	"github.com/agentic-demo/platform/internal/webui"
	"github.com/labstack/echo/v4"
)

// MakeErrorHandler returns an Echo error handler that routes errors
// differently based on the request path:
//   - /api/* requests get the default Echo JSON error handler
//   - All other requests get web-style redirects with flash messages
//
// This preserves the existing JSON API contract while adding HTML error
// handling for the web UI.
func MakeErrorHandler(defaultHandler echo.HTTPErrorHandler) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		// API requests always use the default JSON error handler.
		if strings.HasPrefix(c.Request().URL.Path, "/api") {
			defaultHandler(err, c)
			return
		}

		webErrorHandler(err, c)
	}
}

// webErrorHandler handles errors for web (HTML) requests.
func webErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	code, message := httperr.MapHTTP(err)

	// If the error is an echo.HTTPError from a non-domain source, use its code/message.
	var he *echo.HTTPError
	if errors.As(err, &he) {
		code = he.Code
		if s, ok := he.Message.(string); ok {
			message = s
		}
	}

	// For HTMX requests, return a simple error fragment.
	if IsHTMX(c) {
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
		c.Response().Status = code
		_, _ = c.Response().Write([]byte(`<div class="text-intent-error text-sm">` + message + `</div>`))
		return
	}

	// For browser requests: if it's a 401, redirect to login.
	// Otherwise, redirect to the error page with a flash message.
	if code == http.StatusUnauthorized {
		setFlashCookie(c, webui.Flash{Intent: "error", Message: message})
		_ = c.Redirect(http.StatusSeeOther, "/login")
		return
	}

	// For other errors on browser requests, try to redirect back with a flash.
	setFlashCookie(c, webui.Flash{Intent: "error", Message: message})

	referer := c.Request().Referer()
	if referer == "" {
		referer = "/"
	}
	_ = c.Redirect(http.StatusSeeOther, referer)

	log.Printf("web error [%d]: %v", code, err)
}

// setFlashCookie stores a flash message as a short-lived cookie.
// In a full implementation this would use a session store; here we use
// a simple cookie that the middleware reads and clears.
func setFlashCookie(c echo.Context, flash webui.Flash) {
	// Store in context for the current request cycle.
	// The redirect target reads this from the session/cookie on next request.
	existing := GetFlashMessages(c.Request().Context())
	existing = append(existing, flash)
	ctx := SetFlashMessages(c.Request().Context(), existing)
	c.SetRequest(c.Request().WithContext(ctx))

	// Also set a simple cookie for cross-request flash.
	c.SetCookie(&http.Cookie{ //nolint:gosec // Short-lived flash cookie; not security-critical.
		Name:     "flash",
		Value:    flash.Intent + "|" + flash.Message,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   10, // Expires quickly after redirect.
	})
}

// ReadFlashCookie extracts and clears the flash cookie.
func ReadFlashCookie(c echo.Context) []webui.Flash {
	cookie, err := c.Cookie("flash")
	if err != nil || cookie.Value == "" {
		return nil
	}

	// Clear the cookie.
	c.SetCookie(&http.Cookie{ //nolint:gosec // Clearing cookie.
		Name:   "flash",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	parts := splitFirst(cookie.Value, "|")
	if len(parts) != 2 {
		return nil
	}

	return []webui.Flash{{Intent: parts[0], Message: parts[1]}}
}

func splitFirst(s, sep string) []string {
	for i := 0; i < len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return []string{s[:i], s[i+len(sep):]}
		}
	}
	return []string{s}
}
