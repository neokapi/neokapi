//go:build !js

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apply_edits names its documents inside each operation (ops[].at.doc). The
// roots middleware checks each one, so a document that leaves the project, or
// an absolute path outside the directories the call may reach, is refused
// before the change service sees the call.
func TestMCPChecksTheDocumentOfEveryEditOperation(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	t.Chdir(alpha)
	outside := filepath.Join(t.TempDir(), "outside.json")
	session, ctx := fullSurfaceSession(t, scopeApp(t, alpha))

	for _, doc := range []string{"../outside.json", outside} {
		res := callTool(t, ctx, session, "apply_edits", map[string]any{"ops": []any{
			map[string]any{"op": "set_content", "at": map[string]any{"doc": "docs/guide.md", "block": "k1_x"}, "if_match": "*", "text": "x"},
			map[string]any{"op": "set_content", "at": map[string]any{"doc": doc, "block": "k1_x"}, "if_match": "*", "text": "x"},
		}})
		require.True(t, res.IsError, doc)
		assert.Regexp(t, `doc: .*(leaves the project|lies outside the directories this server may reach)`, resultText(res),
			"the argument check refused it, naming the argument (%s)", doc)
	}
}

// An MCP call names files one by one: stats reads exactly the paths it is
// given and expands no pattern, so a glob a person would type on the command
// line (host/inputs.go expandGlobArg, which lists a link wherever it leads)
// never runs for an agent. A link out of the project is refused as the path
// it resolves to.
func TestMCPStatsExpandsNoPatternAndFollowsNoLinkOut(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	t.Chdir(alpha)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.md"), []byte("# Held outside\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(alpha, "docs", "linked")))
	session, ctx := fullSurfaceSession(t, scopeApp(t, alpha))

	res := callTool(t, ctx, session, "stats", map[string]any{"files": []string{filepath.Join(alpha, "docs", "*.md")}})
	require.True(t, res.IsError, "a pattern is not expanded: %s", resultText(res))

	res = callTool(t, ctx, session, "stats", map[string]any{"files": []string{filepath.Join(alpha, "docs", "linked", "secret.md")}})
	require.True(t, res.IsError, resultText(res))
	assert.Contains(t, resultText(res), "leads outside the root")
	assert.NotContains(t, resultText(res), "Held outside")
}
