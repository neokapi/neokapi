package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The MCP endpoint is routed for a fixed set of methods; the router answers
// any other method 405 before the MCP handler sees it.
func TestRegisterRoutesMethods(t *testing.T) {
	ms, err := NewMCPServer(&memVoiceStore{}, Config{})
	require.NoError(t, err)
	// A stand-in handler, so a 405 can only come from the router.
	ms.handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	e := echo.New()
	ms.RegisterRoutes(e)

	for _, m := range mcpMethods {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(m, "/mcp/", nil))
		assert.Equal(t, http.StatusOK, rec.Code, "%s reaches the MCP handler", m)
	}
	for _, m := range []string{echo.QUERY, "FOO"} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(m, "/mcp/", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, "%s is not routed", m)
	}
}
