//go:build unix

package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// replaceFile puts data at path as an archive tool unpacking over a checkout
// does: the old file is removed and a new one written in its place.
func replaceFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

// overwriteFile writes data into the file at path in place, as a copy onto the
// file does: the file keeps its identity.
func overwriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

// Each state entry is bound on its own, so state unpacked or copied over a
// checkout kapi made is not used, though the checkout's directories keep
// their identity.
func TestStateSwappedUnderAMarkedCheckoutIsNotUsed(t *testing.T) {
	t.Run("a project store unpacked over the work directory", func(t *testing.T) {
		l := ownedProject(t)
		w, err := BeginStateWrite(l, WorkDirName)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(l.StorePath(), []byte("this checkout's store"), 0o600))
		require.NoError(t, w.Commit())
		require.NoError(t, CheckStateOwned(t.Context(), l))

		replaceFile(t, l.StorePath(), []byte("another project's store"))
		notices := captureNotices(t)
		require.NoError(t, CheckStateOwned(t.Context(), l))
		require.Len(t, *notices, 1)
		assert.Contains(t, (*notices)[0], ".kapi/work was not written by kapi on this machine")
		assert.NoFileExists(t, l.StorePath(), "the store that arrived is set aside")
	})

	for _, tc := range []struct {
		name  string
		entry string
		file  func(Layout) string
	}{
		{"the sync cache, claim token and all", SyncDirName, func(l Layout) string { return filepath.Join(l.SyncDir(), "kept") }},
		{"the vault", VaultDirName, func(l Layout) string { return filepath.Join(l.VaultDir(), "kept") }},
		{"the saved filters", LocalFiltersFilename, func(l Layout) string { return l.LocalFiltersPath() }},
	} {
		t.Run(tc.name+", copied over in place", func(t *testing.T) {
			l := ownedProject(t)
			overwriteFile(t, tc.file(l), []byte(`{"from":"elsewhere"}`))
			var unverified *UnverifiedStateError
			require.ErrorAs(t, CheckStateOwned(t.Context(), l), &unverified)
			assert.Equal(t, []string{".kapi/" + tc.entry}, unverified.Entries)
		})
	}

	t.Run("a file added to the vault", func(t *testing.T) {
		l := ownedProject(t)
		require.NoError(t, os.WriteFile(filepath.Join(l.VaultDir(), "redaction.json"), []byte(`{"from":"elsewhere"}`), 0o600))
		var unverified *UnverifiedStateError
		require.ErrorAs(t, CheckStateOwned(t.Context(), l), &unverified)
		assert.Equal(t, []string{".kapi/vault"}, unverified.Entries)
	})

	t.Run("a vault unpacked into a marked checkout that had none", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
		l := LayoutAt(root)
		require.NoError(t, EnsureLayout(l))
		require.NoError(t, os.MkdirAll(l.VaultDir(), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(l.VaultDir(), "redaction.json"), []byte(`{"from":"elsewhere"}`), 0o600))
		var unverified *UnverifiedStateError
		require.ErrorAs(t, CheckStateOwned(t.Context(), l), &unverified, "the directory's mark records no entry that was not there")
	})

	t.Run("a file an earlier layout kept, unpacked into the work directory", func(t *testing.T) {
		l := ownedProject(t)
		require.NoError(t, os.WriteFile(filepath.Join(l.CacheDir(), "sync-cache.json"), []byte(`{"claim_token":"elsewhere"}`), 0o600))
		var unverified *UnverifiedStateError
		require.ErrorAs(t, CheckStateOwned(t.Context(), l), &unverified)
		assert.Contains(t, unverified.Entries, ".kapi/work", "it would be folded into the sync state, so it is refused, not rebuilt")
	})

	t.Run("kapi's own writes keep verifying", func(t *testing.T) {
		l := ownedProject(t)
		require.NoError(t, WriteStateFile(filepath.Join(l.SyncDir(), "kept"), []byte("written again"), 0o600))
		require.NoError(t, WriteStateFile(filepath.Join(l.VaultDir(), "batches", "b-1.json"), []byte("{}"), 0o600))
		require.NoError(t, WriteStateFile(l.LocalFiltersPath(), []byte(`{"active":"again"}`), 0o600))
		require.NoError(t, CheckStateOwned(t.Context(), l))
	})

	t.Run("a write interrupted between the mark and the file", func(t *testing.T) {
		l := ownedProject(t)
		key, err := stateKey()
		require.NoError(t, err)
		mark, err := readMark(l)
		require.NoError(t, err)
		after, err := entryBinding(l, SyncDirName, map[string][]byte{"sync/kept": []byte("next")})
		require.NoError(t, err)
		rec := mark.Entries[SyncDirName]
		rec.MACs = append(rec.MACs, entryMAC(key, l, mark, SyncDirName, after))
		mark.Entries[SyncDirName] = rec
		require.NoError(t, writeMark(key, l, mark))
		require.NoError(t, CheckStateOwned(t.Context(), l), "the content before the write verifies")
		overwriteFile(t, filepath.Join(l.SyncDir(), "kept"), []byte("next"))
		require.NoError(t, CheckStateOwned(t.Context(), l), "and the content after it")
	})
}

// A copied checkout whose vault stays refused rebuilds its work directory once,
// and the state directory keeps one set-aside copy of it.
func TestRefusedStateDoesNotRebuildTheWorkDirectoryEachRun(t *testing.T) {
	src := ownedProject(t)
	dst := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dst, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	copyTree(t, src.StateDir, filepath.Join(dst, StateDirName))
	l := LayoutAt(dst)
	notices := captureNotices(t)
	for range 3 {
		var unverified *UnverifiedStateError
		require.ErrorAs(t, EnsureLayout(l), &unverified)
		require.ErrorAs(t, CheckStateOwned(t.Context(), l), &unverified)
	}
	assert.Len(t, *notices, 1, "the work directory is set aside once")
	entries, err := os.ReadDir(filepath.Join(l.StateDir, SetAsideDirName))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// The set-aside directory keeps the latest copy of an entry and no older one.
func TestOnlyTheLatestSetAsideCopyIsKept(t *testing.T) {
	l := ownedProject(t)
	captureNotices(t)
	for range 3 {
		require.NoError(t, os.MkdirAll(l.WorkDir(), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(l.WorkDir(), "store.db"), []byte("arrived"), 0o600))
		require.NoError(t, SetAsideWork(l))
	}
	entries, err := os.ReadDir(filepath.Join(l.StateDir, SetAsideDirName))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Name(), WorkDirName+setAsideSuffix)
}

// The key is refused unless only this user can read and replace it.
func TestTheStateKeyMustBePrivate(t *testing.T) {
	setup := func(t *testing.T) (Layout, string) {
		t.Helper()
		t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
		stateKeys.Clear()
		l := ownedProject(t)
		dir, err := stateKeyDir()
		require.NoError(t, err)
		stateKeys.Clear()
		return l, dir
	}
	t.Run("a key others can read", func(t *testing.T) {
		l, dir := setup(t)
		require.NoError(t, os.Chmod(filepath.Join(dir, stateKeyFile), 0o644))
		require.ErrorContains(t, CheckStateOwned(t.Context(), l), "chmod 600")
	})
	t.Run("a key that is a link", func(t *testing.T) {
		l, dir := setup(t)
		path := filepath.Join(dir, stateKeyFile)
		require.NoError(t, os.Rename(path, path+".real"))
		require.NoError(t, os.Symlink(path+".real", path))
		require.ErrorContains(t, CheckStateOwned(t.Context(), l), "must be a regular file")
	})
	t.Run("a key directory others can write", func(t *testing.T) {
		l, dir := setup(t)
		require.NoError(t, os.Chmod(dir, 0o777))
		require.ErrorContains(t, CheckStateOwned(t.Context(), l), "chmod 700")
	})
	t.Run("a configuration directory inside the project", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
		t.Setenv("KAPI_CONFIG_DIR", filepath.Join(root, "config"))
		stateKeys.Clear()
		require.ErrorContains(t, EnsureLayout(LayoutAt(root)), "is inside the project")
	})
	t.Run("a key created just now says so", func(t *testing.T) {
		l, _ := setup(t)
		t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
		stateKeys.Clear()
		captureNotices(t)
		require.ErrorContains(t, CheckStateOwned(t.Context(), l), "This machine's key was created just now, so state made earlier is not recognised.")
	})
}

// Vouching records the state that was shown, and refuses state that changed
// after it was shown.
func TestVouchingRecordsWhatWasShown(t *testing.T) {
	src := ownedProject(t)
	dst := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dst, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	copyTree(t, src.StateDir, filepath.Join(dst, StateDirName))
	l := LayoutAt(dst)
	captureNotices(t)
	require.Error(t, CheckStateOwned(t.Context(), l))
	shown, err := DescribeState(l)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(l.VaultDir(), "redaction.json"), []byte(`{"added":"after"}`), 0o600))
	require.ErrorContains(t, AdoptState(l, shown), "changed after it was shown")
	require.Error(t, CheckStateOwned(t.Context(), l), "nothing was recorded")

	shown, err = DescribeState(l)
	require.NoError(t, err)
	require.NoError(t, AdoptState(l, shown))
	require.NoError(t, CheckStateOwned(t.Context(), l))
}

// An entry's MAC is written only for an entry kapi created, or one that
// verified before kapi's own write; never for state that does not verify, and
// never while the trust override is set.
func TestOnlyStateThatVerifiesIsRecorded(t *testing.T) {
	t.Run("a vault from elsewhere present when kapi records", func(t *testing.T) {
		l := ownedProject(t)
		require.NoError(t, os.RemoveAll(l.VaultDir()))
		require.NoError(t, os.MkdirAll(l.VaultDir(), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(l.VaultDir(), "redaction.json"), []byte(`{"from":"elsewhere"}`), 0o600))
		_, err := BeginStateWrite(l, WorkDirName, VaultDirName, SyncDirName)
		var unverified *UnverifiedStateError
		require.ErrorAs(t, err, &unverified)
		assert.Equal(t, []string{".kapi/vault"}, unverified.Entries)
		require.ErrorAs(t, CheckStateOwned(t.Context(), l), &unverified, "nothing was recorded")
	})

	t.Run("an entry swapped after it was verified, before a record", func(t *testing.T) {
		l := ownedProject(t)
		require.NoError(t, CheckStateOwned(t.Context(), l))
		require.NoError(t, os.WriteFile(l.StorePath(), []byte("another project's store"), 0o600))
		_, err := BeginStateWrite(l, WorkDirName)
		var unverified *UnverifiedStateError
		require.ErrorAs(t, err, &unverified)
		notices := captureNotices(t)
		require.NoError(t, CheckStateOwned(t.Context(), l))
		assert.Len(t, *notices, 1, "the store that arrived is set aside, not recorded")
	})

	t.Run("nothing while the trust override is set", func(t *testing.T) {
		l := ownedProject(t)
		before, err := os.ReadFile(filepath.Join(l.StateDir, MarkFilename))
		require.NoError(t, err)
		t.Setenv(TrustStateEnv, "1")
		require.NoError(t, os.WriteFile(l.StorePath(), []byte("a store"), 0o600))
		w, err := BeginStateWrite(l, WorkDirName)
		require.NoError(t, err)
		require.NoError(t, w.Commit())
		require.NoError(t, RecordCreatedEntry(l, WorkDirName))
		require.NoError(t, WriteStateFile(filepath.Join(l.SyncDir(), "kept"), []byte("under the override"), 0o600))
		after, err := os.ReadFile(filepath.Join(l.StateDir, MarkFilename))
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after), "the mark is as it was")
		t.Setenv(TrustStateEnv, "")
		var unverified *UnverifiedStateError
		captureNotices(t)
		require.ErrorAs(t, CheckStateOwned(t.Context(), l), &unverified)
		assert.Equal(t, []string{".kapi/sync"}, unverified.Entries)
	})

	t.Run("kapi's own rewrite of the sync cache", func(t *testing.T) {
		l := ownedProject(t)
		require.NoError(t, WriteStateFile(filepath.Join(l.SyncDir(), "sync-cache.json"), []byte(`{"claim_token":"mine"}`), 0o600))
		require.NoError(t, WriteStateFile(filepath.Join(l.SyncDir(), "sync-cache.json"), []byte(`{"claim_token":"mine, again"}`), 0o600))
		require.NoError(t, CheckStateOwned(t.Context(), l))
	})

	t.Run("a store kapi creates in a work directory that verified", func(t *testing.T) {
		l := ownedProject(t)
		w, err := BeginStateWrite(l, WorkDirName)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(l.StorePath(), []byte("kapi's store"), 0o600))
		require.NoError(t, w.Commit())
		require.NoError(t, CheckStateOwned(t.Context(), l))
		assert.FileExists(t, l.StorePath())
	})
}
