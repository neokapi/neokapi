//go:build unix

package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// The connector a push, a pull or a listing builds reads the sync state, and
// with it the claim token a push authenticates with. Both constructors refuse
// a checkout whose git tracks any entry of kapi's state, before the sync
// state is read.
func TestSourceConnectorsRefuseTrackedState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	for _, st := range []struct{ entry, file string }{
		{coreproj.WorkDirName, "work/store.db"},
		{coreproj.SyncDirName, "sync/sync-cache.json"},
		{coreproj.VaultDirName, "vault/originals.json"},
		{coreproj.LocalFiltersFilename, coreproj.LocalFiltersFilename},
	} {
		t.Run(st.entry, func(t *testing.T) {
			root := t.TempDir()
			proj, err := bproject.InitProject(root, &bproject.Recipe{
				Defaults: coreproj.Defaults{SourceLanguage: "en"},
			})
			require.NoError(t, err)
			p := filepath.Join(root, coreproj.StateDirName, filepath.FromSlash(st.file))
			require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
			require.NoError(t, os.WriteFile(p, []byte(`{"claim_token":"planted"}`), 0o644))
			for _, args := range [][]string{{"init", "-q"}, {"add", "-f", coreproj.StateDirName + "/" + st.file}} {
				cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
				cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
				out, cerr := cmd.CombinedOutput()
				require.NoError(t, cerr, string(out))
			}
			want := "git tracks " + coreproj.StateDirName + "/" + st.entry
			_, err = NewSourceConnector(t.Context(), &host.App{}, proj, registry.NewFormatRegistry())
			require.ErrorContains(t, err, want)
			_, err = NewLocalConnector(t.Context(), &host.App{}, proj, registry.NewFormatRegistry())
			require.ErrorContains(t, err, want)
		})
	}
}
