//go:build unix

package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// Sync state a repository commits in its .kapi/ is not kapi's: the connector
// reads the sync state from the checkout's state directory under the data
// root, so a claim token committed in the tree never reaches a push.
func TestSourceConnectorsReadNoSyncStateFromTheTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
	})
	require.NoError(t, err)
	p := filepath.Join(root, coreproj.StateDirName, "sync", "sync-cache.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(`{"claim_token":"planted"}`), 0o644))
	for _, args := range [][]string{{"init", "-q"}, {"add", "-f", coreproj.StateDirName + "/sync/sync-cache.json"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, cerr := cmd.CombinedOutput()
		require.NoError(t, cerr, string(out))
	}

	c, err := NewLocalConnector(t.Context(), &host.App{}, proj, registry.NewFormatRegistry())
	require.NoError(t, err)
	assert.Empty(t, c.cache.ClaimToken, "the committed claim token is not read")
}
