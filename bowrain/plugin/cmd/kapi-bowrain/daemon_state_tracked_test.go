//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/cli"
	coreproj "github.com/neokapi/neokapi/core/project"
)

// The daemon serves a project for as long as it runs, so every call asks
// whether git tracks the project's state: a project tracked before the first
// call is refused, and one tracked after the daemon opened it is refused at
// the next call.
func TestDaemonRefusesTrackedStateAtEveryCall(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	prev := app
	app = &cli.App{}
	app.InitRegistries()
	t.Cleanup(func() {
		app.Shutdown()
		app = prev
	})
	git := func(root string, args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	for _, st := range []struct{ entry, file string }{
		{coreproj.WorkDirName, "work/store.db"},
		{coreproj.SyncDirName, "sync/sync-cache.json"},
		{coreproj.VaultDirName, "vault/originals.json"},
		{coreproj.LocalFiltersFilename, coreproj.LocalFiltersFilename},
	} {
		t.Run(st.entry, func(t *testing.T) {
			root := daemonProjectRoot(t)
			p := filepath.Join(root, coreproj.StateDirName, filepath.FromSlash(st.file))
			// Written as kapi writes it, so the state is this machine's own.
			if _, err := os.Stat(p); os.IsNotExist(err) {
				require.NoError(t, coreproj.WriteStateFile(p, []byte(`{"claim_token":"planted"}`), 0o600))
			}
			require.NoError(t, coreproj.RecordStateEntry(coreproj.LayoutAt(root), coreproj.WorkDirName))
			git(root, "init", "-q")
			d := newDaemonService(nil)
			_, err := d.projectFor(t.Context(), root)
			require.NoError(t, err, "nothing tracked yet")

			git(root, "add", "-f", coreproj.StateDirName+"/"+st.file)
			_, err = d.projectFor(t.Context(), root)
			require.ErrorContains(t, err, "git tracks "+coreproj.StateDirName+"/"+st.entry)
		})
	}
}
