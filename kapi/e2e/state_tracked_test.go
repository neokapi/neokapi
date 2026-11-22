//go:build e2e && unix

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trackedStateEntries are the entries of the state directory kapi keeps for
// one checkout (core/project.StateEntries), each with the file a repository
// plants there and commits. The planted content carries the word "planted",
// which no refusal may echo.
var trackedStateEntries = []struct{ entry, file, body string }{
	{"work", "work/store.db", "planted store"},
	{"sync", "sync/sync-cache.json", `{"claim_token":"planted-claim-token"}`},
	{"vault", "vault/originals.json", `{"planted":"original"}`},
	{"filters.local.json", "filters.local.json", `{"active":"planted"}`},
}

// trackedStateProject is a project in a git repository that commits one
// entry of kapi's state.
func trackedStateProject(t *testing.T, file, body string) (root, recipe string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root, recipe = writeBareProject(t, "tracked")
	p := filepath.Join(root, ".kapi", filepath.FromSlash(file))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	for _, args := range [][]string{{"init", "-q"}, {"add", "-f", ".kapi/" + file}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return root, recipe
}

// Every surface that reads kapi's state refuses a checkout whose git tracks
// any entry of it, naming the entry, and echoes nothing planted there: the
// CLI's status, a run and a merge. The MCP server, the desktop
// open, the source connector a push or pull builds, and the plugin daemon are
// tested where they live (apps/kapi-desktop/backend, host/venue/source,
// bowrain/plugin/cmd/kapi-bowrain).
func TestEveryCommandRefusesTrackedState(t *testing.T) {
	for _, st := range trackedStateEntries {
		t.Run(st.entry, func(t *testing.T) {
			for _, args := range [][]string{
				{"status"},
				{"run", "pseudo-translate"},
				{"merge"},
			} {
				_, recipe := trackedStateProject(t, st.file, st.body)
				out, err := kapiAllowFail(t, append(args, "-p", recipe)...)
				require.Error(t, err, "%v succeeded:\n%s", args, out)
				assert.Contains(t, out, "git tracks .kapi/"+st.entry, args)
				assert.NotContains(t, out, "planted", args)
			}
		})
	}
}

// The MCP server refuses a call that acts on a project whose git tracks any
// entry of kapi's state, and echoes nothing planted there.
func TestTheMCPServerRefusesTrackedState(t *testing.T) {
	for _, st := range trackedStateEntries {
		t.Run(st.entry, func(t *testing.T) {
			root, _ := trackedStateProject(t, st.file, st.body)
			session, ctx := mcpServerWithRoots(t, []string{root})
			res := rawCallTool(t, ctx, session, "context_search", map[string]any{"query": "gadget", "project": root})
			require.True(t, res.IsError, "the MCP call succeeded: %s", resultText(res))
			assert.Contains(t, resultText(res), "git tracks .kapi/"+st.entry)
			assert.NotContains(t, resultText(res), "planted")
		})
	}
}
