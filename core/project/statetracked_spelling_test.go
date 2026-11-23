//go:build unix

package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// sameOnThisVolume reports whether two names beneath dir are one directory on
// this volume.
func sameOnThisVolume(t *testing.T, dir, a, b string) bool {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, a), 0o755))
	ia, err := os.Stat(filepath.Join(dir, a))
	require.NoError(t, err)
	ib, err := os.Stat(filepath.Join(dir, b))
	same := err == nil && os.SameFile(ia, ib)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, strings.SplitN(a, string(filepath.Separator), 2)[0])))
	return same
}

// A repository that tracks kapi's state under another spelling that this
// volume resolves to the same directory (case, Unicode case folding, or
// composed and decomposed forms) is refused, naming the spelling git holds.
// On a case-sensitive volume (the Linux CI leg) those spellings are other
// directories kapi never opens, and the subtests say so and stop.
func TestCheckStateTrackedJudgesSpellingsAsTheVolumeResolvesThem(t *testing.T) {
	for _, tc := range []struct{ name, stateDir, entry, canonical string }{
		{"entry in upper case", StateDirName, "WORK", WorkDirName},
		{"state directory in upper case", ".KAPI", "work", WorkDirName},
		{"Kelvin sign for k", StateDirName, "worK", WorkDirName},
		{"sync in mixed case", StateDirName, "SyNc", SyncDirName},
		{"vault in upper case", ".Kapi", "VAULT", VaultDirName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := exec.LookPath("git"); err != nil {
				t.Skip("git is not installed")
			}
			root := t.TempDir()
			if !sameOnThisVolume(t, root, filepath.Join(StateDirName, tc.canonical), filepath.Join(tc.stateDir, tc.entry)) {
				t.Skipf("this volume keeps %s/%s apart from %s/%s, so kapi never opens it", tc.stateDir, tc.entry, StateDirName, tc.canonical)
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
			planted := filepath.Join(root, tc.stateDir, tc.entry, "store.db")
			require.NoError(t, os.MkdirAll(filepath.Dir(planted), 0o755))
			require.NoError(t, os.WriteFile(planted, []byte("planted"), 0o644))
			gitIn(t, root, "init", "-q")
			gitIn(t, root, "add", "-f", tc.stateDir+"/"+tc.entry+"/store.db")
			err := CheckStateTracked(t.Context(), LayoutAt(root))
			require.ErrorContains(t, err, "git tracks "+tc.stateDir+"/"+tc.entry)
		})
	}
}

// A state directory or entry that is a submodule, or a repository of its
// own, is refused: the parent's index lists a submodule as one entry and a
// nested repository not at all, so their contents arrive unseen.
func TestCheckStateTrackedRefusesASubmoduleOrANestedRepository(t *testing.T) {
	const sha = "1111111111111111111111111111111111111111"
	t.Run("the state directory is a submodule", func(t *testing.T) {
		l := stateRepo(t)
		gitIn(t, l.Root, "update-index", "--add", "--cacheinfo", "160000,"+sha+","+StateDirName)
		require.ErrorContains(t, CheckStateTracked(t.Context(), l), "git tracks "+StateDirName)
	})
	t.Run("a state entry is a submodule", func(t *testing.T) {
		l := stateRepo(t)
		gitIn(t, l.Root, "update-index", "--add", "--cacheinfo", "160000,"+sha+","+StateDirName+"/"+WorkDirName)
		require.ErrorContains(t, CheckStateTracked(t.Context(), l), "git tracks "+StateDirName+"/"+WorkDirName)
	})
	for _, dir := range []string{"", SyncDirName} {
		t.Run("a repository of its own at "+StateDirName+"/"+dir, func(t *testing.T) {
			l := stateRepo(t)
			gitIn(t, filepath.Join(l.StateDir, dir), "init", "-q")
			require.ErrorContains(t, CheckStateTracked(t.Context(), l), "is a git repository of its own")
		})
	}
}
