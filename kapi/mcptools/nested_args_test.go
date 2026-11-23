package mcptools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// callThroughRoots calls tool on a server as `kapi mcp` serves it, the roots
// middleware in front, and returns the result's text and whether it failed.
func callThroughRoots(t *testing.T, tool string, args map[string]any) (string, bool) {
	t.Helper()
	a := testApp()
	a.MCPSurface.AllTools = true
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi-test", Version: "0"}, nil)
	registerKapiTools(server, a)
	registerReviewTools(server, a)
	require.NoError(t, a.ResolveMCPProject(host.NewEnvCommand(t.Context(), "mcp")))
	server.AddReceivingMiddleware(a.MCPRootsMiddleware())

	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := client.Connect(ctx, ct, &mcp.ClientSessionOptions{ProtocolVersion: rootsProtocol})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return text.String(), res.IsError
}

// The roots middleware checks a document named inside a structure like one
// passed on its own. Here that is review_block's at.doc; host tests
// apply_edits' ops[].at.doc. A path that leaves the project, or an absolute
// path outside the directories the call may reach, is refused before the tool
// runs, and the refusal names the argument.
func TestMCPChecksADocumentNamedInsideAStructure(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.json")
	for _, doc := range []string{"../../outside.json", outside} {
		calls := map[string]map[string]any{
			"review_block": {"at": map[string]any{"doc": doc, "block": "k1_x"}},
		}
		for tool, args := range calls {
			msg, failed := callThroughRoots(t, tool, args)
			require.True(t, failed, "%s with doc %q: %s", tool, doc, msg)
			assert.Regexp(t, `doc: .*(leaves the project|lies outside the directories this server may reach)`, msg,
				"%s: the argument check refused it, naming the argument", tool)
		}
	}
}
