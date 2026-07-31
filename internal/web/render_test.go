package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubComponent is a minimal templ.Component for testing.
type stubComponent struct {
	html string
}

func (s stubComponent) Render(ctx context.Context, w io.Writer) error {
	_, err := io.WriteString(w, s.html)
	return err
}

func TestIsHTMX(t *testing.T) {
	e := echo.New()

	t.Run("regular request", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		c := e.NewContext(req, httptest.NewRecorder())
		assert.False(t, IsHTMX(c))
	})

	t.Run("htmx request", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		req.Header.Set("HX-Request", "true")
		c := e.NewContext(req, httptest.NewRecorder())
		assert.True(t, IsHTMX(c))
	})
}

func TestRender(t *testing.T) {
	e := echo.New()
	comp := stubComponent{html: "<p>hello</p>"}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := Render(c, http.StatusOK, comp)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, echo.MIMETextHTMLCharsetUTF8, rec.Header().Get(echo.HeaderContentType))
	assert.Equal(t, "<p>hello</p>", rec.Body.String())
}

func TestRenderComponent_BrowserRequest(t *testing.T) {
	e := echo.New()
	page := stubComponent{html: "<html>full page</html>"}
	frag := stubComponent{html: "<div>fragment</div>"}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := RenderComponent(c, http.StatusOK, page, frag)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "full page")
}

func TestRenderComponent_HTMXRequest(t *testing.T) {
	e := echo.New()
	page := stubComponent{html: "<html>full page</html>"}
	frag := stubComponent{html: "<div>fragment</div>"}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := RenderComponent(c, http.StatusOK, page, frag)
	require.NoError(t, err)
	assert.Contains(t, rec.Body.String(), "fragment")
	assert.NotContains(t, rec.Body.String(), "full page")
}

func TestRenderToString(t *testing.T) {
	comp := stubComponent{html: "<span>text</span>"}
	got, err := RenderToString(context.Background(), comp)
	require.NoError(t, err)
	assert.Equal(t, "<span>text</span>", got)
}

// Verify stubComponent satisfies the interface at compile time.
var _ templ.Component = stubComponent{}
