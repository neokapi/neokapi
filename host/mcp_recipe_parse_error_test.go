//go:build !js

package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An MCP tool called on a project whose recipe has a YAML error reports the
// parser's error with its line, as the CLI and the desktop do.
func TestMCPReportsTheRecipeParseError(t *testing.T) {
	isolateCheckExecution(t)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("# Hello\n"), 0o644))
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: dup\ndefaults:\n  target_languages: [fr]\n  target_languages: [de]\n"), 0o644))
	app := scopeApp(t, scopeProject(t, "alpha", "translation memory", "content memory", "nb"))
	t.Chdir(t.TempDir())
	session, ctx := mcpSession(t, app)

	res := callTool(t, ctx, session, "check_file", map[string]any{"file": filepath.Join(root, "docs", "a.md"), "project": recipe})
	require.True(t, res.IsError, "%+v", res.Content)
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	assert.Contains(t, text.String(), "line 5")
	assert.Contains(t, text.String(), "already defined")
	assert.NotContains(t, text.String(), "not a kapi recipe")
}
