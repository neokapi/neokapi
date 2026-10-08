//go:build !js

package host

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContextReadRetiredVerbNamesTheTool: an assistant that learned the old
// verbs asks context_read for "observe" and gets an answer for a planned file
// at the project root. The tool refuses the bare verb and names the tool or
// command that does the job now.
func TestContextReadRetiredVerbNamesTheTool(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "retired-read")

	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerContextMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "retired-test", Version: "test"}, nil).
		Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	for path, want := range map[string]string{
		"observe":  "the context_note tool",
		"withdraw": "context_note with withdraw",
		"import":   "kapi store import",
		"keep":     "kapi context review --keep",
	} {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name: "context_read", Arguments: map[string]any{"path": path, "project": recipeOf(root)},
		})
		require.NoError(t, err)
		require.Truef(t, res.IsError, "context_read %s is refused", path)
		assert.Containsf(t, resultText(res), want, "context_read %s", path)
	}
}

// TestRetiredContextVerbErrorLeavesPathsAlone: only the bare verb is refused.
func TestRetiredContextVerbErrorLeavesPathsAlone(t *testing.T) {
	for _, path := range []string{"./import", "docs/import", "import.md", "notes", "README.md"} {
		require.NoErrorf(t, RetiredContextVerbError(path), "%s is a path", path)
	}
	err := RetiredContextVerbError("settle")
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
}
