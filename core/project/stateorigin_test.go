//go:build unix

package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ownedProject is a project whose state directory kapi on this machine
// created, holding every entry of StateEntries.
func ownedProject(t *testing.T) Layout {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	l := LayoutAt(root)
	require.NoError(t, EnsureLayout(l))
	plantState(t, l)
	require.NoError(t, CheckStateOwned(t.Context(), l), "state this machine wrote is used")
	return l
}

// plantState writes a file into every entry of StateEntries.
func plantState(t *testing.T, l Layout) {
	t.Helper()
	for _, e := range StateEntries {
		p := filepath.Join(l.StateDir, e)
		if e == LocalFiltersFilename {
			require.NoError(t, os.MkdirAll(l.StateDir, 0o755))
			require.NoError(t, os.WriteFile(p, []byte(`{"active":"planted"}`), 0o644))
			continue
		}
		require.NoError(t, os.MkdirAll(p, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(p, "kept"), []byte("planted"), 0o644))
	}
}

// copyTree copies dir to dst as an archive extraction or `cp -r` would.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	out, err := exec.Command("cp", "-R", src, dst).CombinedOutput()
	require.NoError(t, err, string(out))
}

// captureNotices collects what CheckStateOwned prints when it sets state aside.
func captureNotices(t *testing.T) *[]string {
	t.Helper()
	var got []string
	prev := StateNotice
	StateNotice = func(msg string) { got = append(got, msg) }
	t.Cleanup(func() { StateNotice = prev })
	return &got
}

// requireRefusedUntilAdopted asserts the layout's state is refused, naming the
// command that vouches for it, that the work directory was set aside, and
// that adopting it lets it be used.
func requireRefusedUntilAdopted(t *testing.T, l Layout) {
	t.Helper()
	err := CheckStateOwned(t.Context(), l)
	var unverified *UnverifiedStateError
	require.ErrorAs(t, err, &unverified)
	assert.Contains(t, err.Error(), "kapi trust allow --state")
	assert.ElementsMatch(t, []string{".kapi/sync", ".kapi/vault", ".kapi/" + LocalFiltersFilename}, unverified.Entries)
	_, werr := os.Stat(filepath.Join(l.WorkDir(), "kept"))
	require.ErrorIs(t, werr, os.ErrNotExist, "the work directory this machine did not write is set aside")
	require.NoError(t, AdoptState(l))
	require.NoError(t, CheckStateOwned(t.Context(), l))
}

// State copied in from another checkout or an archive, whatever mark it
// carries, is not used: the work directory is set aside and rebuilt, the rest
// is refused until a person vouches for it.
func TestStateFromElsewhereIsNotUsed(t *testing.T) {
	t.Run("a state directory copied in, mark and all", func(t *testing.T) {
		src := ownedProject(t)
		dst := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dst, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
		copyTree(t, src.StateDir, filepath.Join(dst, StateDirName))
		notices := captureNotices(t)
		requireRefusedUntilAdopted(t, LayoutAt(dst))
		require.Len(t, *notices, 1)
		assert.Contains(t, (*notices)[0], "set aside")
	})

	t.Run("a mark copied from another checkout of this person", func(t *testing.T) {
		a := ownedProject(t)
		b := ownedProject(t)
		mark, err := os.ReadFile(filepath.Join(a.StateDir, MarkFilename))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(b.StateDir, MarkFilename), mark, 0o600))
		captureNotices(t)
		requireRefusedUntilAdopted(t, b)
	})

	t.Run("a claim token copied into a checkout kapi never marked", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
		l := LayoutAt(root)
		require.NoError(t, os.MkdirAll(l.SyncDir(), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(l.SyncDir(), "sync-cache.json"), []byte(`{"claim_token":"planted"}`), 0o644))
		err := CheckStateOwned(t.Context(), l)
		require.ErrorContains(t, err, ".kapi/sync was not written by kapi on this machine")
		require.Error(t, EnsureLayout(l), "a store open refuses it too")
	})

	t.Run("a store spelled another way that the volume resolves", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
		if !sameOnThisVolume(t, root, filepath.Join(StateDirName, WorkDirName), filepath.Join(".KAPI", "WORK")) {
			t.Skip("this volume keeps .KAPI/WORK apart from .kapi/work, so kapi never opens it")
		}
		planted := filepath.Join(root, ".KAPI", "WORK", "store.db")
		require.NoError(t, os.MkdirAll(filepath.Dir(planted), 0o755))
		require.NoError(t, os.WriteFile(planted, []byte("planted"), 0o644))
		captureNotices(t)
		require.NoError(t, CheckStateOwned(t.Context(), LayoutAt(root)))
		_, err := os.Stat(filepath.Join(root, StateDirName, WorkDirName, "store.db"))
		assert.ErrorIs(t, err, os.ErrNotExist, "the store this machine did not write is set aside")
	})

	t.Run("state in a repository with no index and none tracked", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git is not installed")
		}
		src := ownedProject(t)
		dst := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dst, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
		copyTree(t, src.StateDir, filepath.Join(dst, StateDirName))
		gitIn(t, dst, "init", "-q")
		require.NoFileExists(t, filepath.Join(dst, ".git", "index"))
		captureNotices(t)
		requireRefusedUntilAdopted(t, LayoutAt(dst))
	})

	t.Run("state made under another key: another user, or another machine", func(t *testing.T) {
		l := ownedProject(t)
		t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
		captureNotices(t)
		requireRefusedUntilAdopted(t, l)
	})
}

// The cases where state is this machine's own keep working.
func TestStateThisMachineWroteKeepsWorking(t *testing.T) {
	t.Run("a checkout moved or renamed on its volume", func(t *testing.T) {
		l := ownedProject(t)
		moved := filepath.Join(t.TempDir(), "renamed")
		require.NoError(t, os.Rename(l.Root, moved))
		require.NoError(t, CheckStateOwned(t.Context(), LayoutAt(moved)))
	})

	t.Run("a second process of the same user", func(t *testing.T) {
		l := ownedProject(t)
		stateKeys.Clear()
		require.NoError(t, CheckStateOwned(t.Context(), l), "the key is read from the configuration directory, not held in one process")
	})

	t.Run("KAPI_TRUST_STATE=1 vouches for this run only", func(t *testing.T) {
		src := ownedProject(t)
		dst := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dst, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
		copyTree(t, src.StateDir, filepath.Join(dst, StateDirName))
		t.Setenv(TrustStateEnv, "1")
		require.NoError(t, CheckStateOwned(t.Context(), LayoutAt(dst)))
		assert.NoFileExists(t, filepath.Join(dst, StateDirName, "set-aside"), "nothing is set aside or written while the person vouches")
		t.Setenv(TrustStateEnv, "")
		captureNotices(t)
		var unverified *UnverifiedStateError
		require.ErrorAs(t, CheckStateOwned(t.Context(), LayoutAt(dst)), &unverified, "the variable adopts nothing: a later run without it is refused")
	})

	t.Run("a state directory with nothing kapi keeps in it is marked, not refused", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
		l := LayoutAt(root)
		require.NoError(t, os.MkdirAll(filepath.Join(l.StateDir, "profiles"), 0o755))
		require.NoError(t, CheckStateOwned(t.Context(), l))
		assert.FileExists(t, filepath.Join(l.StateDir, MarkFilename))
	})
}

// The key lives in this user's configuration directory, readable by this user
// alone, in a directory only this user can open.
func TestStateKeyIsPrivateToTheUser(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)
	_, err := stateKey()
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(dir, stateKeyDirName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	info, err = os.Stat(filepath.Join(dir, stateKeyDirName, stateKeyFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.True(t, strings.HasPrefix(stateKeyDir(), dir))
}
