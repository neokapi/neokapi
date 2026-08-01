package server

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDesktopCallbackEscapesProviderError pins that the identity provider's
// error text reaches the page as text.
//
// The page is reached by an unauthenticated GET on the app's own origin, and
// both parameters it renders come from the query string, so whoever sends the
// link chooses the content. The sibling device-flow callback already routes the
// same two parameters through url.QueryEscape; this page was the one that did
// not.
func TestDesktopCallbackEscapesProviderError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)
	e := srv.GetEcho()

	tests := []struct {
		name  string
		param string
		value string
	}{
		{"error_description with markup", "error_description", `<script>alert(1)</script>`},
		{"error with markup", "error", `<img src=x onerror=alert(1)>`},
		{"attribute break-out", "error_description", `" onmouseover="alert(1)`},
		{"tag break-out", "error_description", `</p><script>alert(1)</script><p>`},
		{"svg vector", "error_description", `<svg/onload=alert(1)>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := url.Values{tt.param: {tt.value}}
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/auth/desktop/callback?"+q.Encode(), nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			body := rec.Body.String()

			// The provider's text is shown, but only as text. Note that an
			// escaped payload still *reads* as "onerror=alert(1)" in the body —
			// what matters is that no tag or attribute boundary survives, so
			// the substring is inert prose rather than markup.
			assert.NotContains(t, body, tt.value, "the raw value must not reach the document")
			assert.Contains(t, body, html.EscapeString(tt.value))

			// The document contains exactly the markup the handler authored,
			// and no angle bracket or quote from the parameter.
			rendered := body[strings.Index(body, "<p>")+len("<p>") : strings.Index(body, "</p>")]
			assert.NotContains(t, rendered, "<", "no tag may be opened from the parameter")
			assert.NotContains(t, rendered, ">", "no tag may be closed from the parameter")
			assert.NotContains(t, rendered, `"`, "no attribute may be broken out of")

			// And the response is not a document that could act on the origin
			// even if something later slipped through.
			assert.Equal(t, defaultCSP, rec.Header().Get(headerCSP))
		})
	}
}

// TestDesktopCallbackStillShowsAMessage guards the escaping against being
// mistaken for suppression: an ordinary provider message is still rendered, and
// the no-parameter case keeps its default.
func TestDesktopCallbackStillShowsAMessage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)
	e := srv.GetEcho()

	q := url.Values{"error_description": {"The user denied the request"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/callback?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Contains(t, rec.Body.String(), "The user denied the request")

	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/callback", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Contains(t, rec.Body.String(), "missing code or state")
}
