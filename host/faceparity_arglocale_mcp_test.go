//go:build !js

package host_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The MCP face refuses a locale that names no language, rather than searching
// for one.
func TestFaceParity_MCPArgumentLocaleIsCanonicalized(t *testing.T) {
	p := facetest.Write(t)

	a := &host.App{}
	a.InitRegistries()
	session := mcpSession(t, a)

	t.Run("posix locale narrows the same search", func(t *testing.T) {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name: "context_search",
			Arguments: map[string]any{
				"query":  p.SearchQuery,
				"locale": "en_US",
				"limit":  p.SearchLimit,
			},
		})
		require.NoError(t, err)
		require.False(t, res.IsError, "a POSIX locale is a locale: %+v", res.Content)

		var out host.ContextSearchResult
		require.NoError(t, json.Unmarshal(structuredJSON(t, res), &out))
		assert.Equal(t, p.SearchQuery, out.Query)
	})

	t.Run("a locale that is not one is refused", func(t *testing.T) {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name: "context_search",
			Arguments: map[string]any{
				"query":  p.SearchQuery,
				"locale": "xx_YY",
			},
		})
		// The SDK reports a handler error either as a transport error or as an
		// error result; both are the refusal this asserts.
		if err == nil {
			assert.True(t, res.IsError, "xx_YY names no language and must be refused")
		}
	})
}
