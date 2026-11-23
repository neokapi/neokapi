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

	"github.com/neokapi/neokapi/core/dataroot"
)

// newCheckout writes a recipe into a fresh directory at root and returns its
// layout.
func newCheckout(t *testing.T, root string) Layout {
	t.Helper()
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\nid: shared-project-id\n"), 0o644))
	return LayoutAt(root)
}

// withVault gives the checkout's state a redaction vault, as kapi writes one.
func withVault(t *testing.T, l Layout, content string) {
	t.Helper()
	_, err := EnsureLocalDir(l, VaultDirName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(l.RedactionVaultPath(), []byte(content), 0o600))
}

// captureStateNotices collects the one-time lines kapi prints.
func captureStateNotices(t *testing.T) *[]string {
	t.Helper()
	var got []string
	prev := StateNotice
	StateNotice = func(msg string) { got = append(got, msg) }
	t.Cleanup(func() { StateNotice = prev })
	return &got
}

func within(t *testing.T, p, dir string) bool {
	t.Helper()
	rel, err := filepath.Rel(dir, p)
	require.NoError(t, err)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}

// A checkout's state lives under the data root, and kapi writes nothing into
// the checkout to keep it.
func TestACheckoutsStateLivesOutsideIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	l := newCheckout(t, root)
	require.NoError(t, EnsureLayout(l))
	withVault(t, l, "{}")

	for _, p := range []string{l.LocalStateDir(), l.WorkDir(), l.StorePath(), l.SyncDir(), l.VaultDir(), l.LocalFiltersPath()} {
		assert.True(t, within(t, p, dataroot.CheckoutsDir()), "%s is under the data root", p)
		assert.False(t, within(t, p, root), "%s is not in the checkout", p)
	}
	assert.NoDirExists(t, filepath.Join(root, StateDirName), "nothing is written in the checkout")
}

// Two checkouts of one project, with the same recipe and id, keep separate
// state.
func TestTwoCheckoutsOfOneProjectKeepSeparateState(t *testing.T) {
	a := newCheckout(t, filepath.Join(t.TempDir(), "a"))
	b := newCheckout(t, filepath.Join(t.TempDir(), "b"))
	withVault(t, a, `{"from":"a"}`)
	require.NoError(t, EnsureLayout(b))
	assert.NotEqual(t, a.LocalStateDir(), b.LocalStateDir())
	assert.NoFileExists(t, b.RedactionVaultPath())
}

// A repository cloned where another checkout used to be is another directory
// at the same location: it sees none of that checkout's state, the person is
// told once, and the earlier state is kept for a relink a person asks for.
func TestACheckoutClonedWhereAnotherWasSeesNoneOfItsState(t *testing.T) {
	notices := captureStateNotices(t)
	root := filepath.Join(t.TempDir(), "app")
	a := newCheckout(t, root)
	withVault(t, a, `{"originals":"of project A"}`)
	_, err := EnsureLocalDir(a, SyncDirName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(a.SyncDir(), "sync-cache.json"), []byte(`{"claim_token":"clm_a"}`), 0o600))
	earlier := a.LocalStateDir()

	require.NoError(t, os.RemoveAll(root))
	b := newCheckout(t, root)
	require.NoError(t, EnsureLayout(b))
	assert.NotEqual(t, earlier, b.LocalStateDir())
	assert.NoFileExists(t, b.RedactionVaultPath())
	assert.NoFileExists(t, filepath.Join(b.SyncDir(), "sync-cache.json"))
	require.Len(t, *notices, 1)
	assert.Contains(t, (*notices)[0], "kapi state relink --from "+root)

	require.NoError(t, EnsureLayout(b))
	assert.Len(t, *notices, 1, "told once")
	assert.FileExists(t, filepath.Join(earlier, VaultDirName, "redaction.json"), "the earlier state is kept")
}

// A checkout renamed on its volume keeps its state; a copy starts without it.
func TestARenamedCheckoutKeepsItsStateAndACopyDoesNot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	l := newCheckout(t, root)
	withVault(t, l, `{"mine":true}`)

	copied := filepath.Join(t.TempDir(), "copy")
	out, err := exec.Command("cp", "-R", root, copied).CombinedOutput()
	require.NoError(t, err, string(out))
	assert.NoFileExists(t, LayoutAt(copied).RedactionVaultPath(), "a copy is another directory")

	renamed := filepath.Join(t.TempDir(), "renamed")
	require.NoError(t, os.Rename(root, renamed))
	moved := LayoutAt(renamed)
	data, err := os.ReadFile(moved.RedactionVaultPath())
	require.NoError(t, err, "the state follows the rename")
	assert.JSONEq(t, `{"mine":true}`, string(data))
}

// State an earlier kapi left inside a checkout is not read: the checkout's
// state directory is elsewhere, and the person is told once what to do.
func TestStateInsideACheckoutIsNotRead(t *testing.T) {
	notices := captureStateNotices(t)
	root := filepath.Join(t.TempDir(), "app")
	l := newCheckout(t, root)
	in := filepath.Join(root, StateDirName)
	for rel, data := range map[string]string{
		"work/store.db":              "a store from elsewhere",
		"vault/redaction.json":       `{"planted":true}`,
		"sync/sync-cache.json":       `{"claim_token":"clm_planted","server_url":"https://elsewhere.example"}`,
		LocalFiltersFilename:         `{"active":"planted"}`,
		"work/cache/sync-cache.json": `{"claim_token":"clm_legacy"}`,
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(in, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(in, rel), []byte(data), 0o600))
	}
	require.NoError(t, EnsureLayout(l))
	for _, p := range []string{l.StorePath(), l.RedactionVaultPath(), filepath.Join(l.SyncDir(), "sync-cache.json"), l.LocalFiltersPath()} {
		assert.NoFileExists(t, p)
	}
	require.Len(t, *notices, 2)
	assert.Contains(t, (*notices)[0], ".kapi/work in this checkout is no longer used")
	assert.Contains(t, (*notices)[1], "kapi state import -p "+root)
	require.NoError(t, EnsureLayout(l))
	assert.Len(t, *notices, 2, "each is told once")
}

// kapi state import shows what it takes and takes exactly that; it refuses a
// link, state that changed after it was shown, and a target that holds other
// content, and leaves the copies in the checkout unless asked to remove them.
func TestImportTakesExactlyWhatItShowed(t *testing.T) {
	captureStateNotices(t)
	setup := func(t *testing.T) Layout {
		t.Helper()
		l := newCheckout(t, filepath.Join(t.TempDir(), "app"))
		in := filepath.Join(l.Root, StateDirName)
		require.NoError(t, os.MkdirAll(filepath.Join(in, "vault"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(in, "vault", "redaction.json"), []byte(`{"v":1}`), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(in, "sync"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(in, "sync", "sync-cache.json"), []byte(`{"claim_token":"clm","server_url":"https://app.example"}`), 0o600))
		return l
	}

	t.Run("shown, then taken", func(t *testing.T) {
		l := setup(t)
		plan, err := PlanImport(l)
		require.NoError(t, err)
		require.Len(t, plan.Files, 2)
		assert.Equal(t, []string{"https://app.example"}, plan.Servers)
		assert.Equal(t, int64(len(`{"v":1}`)), plan.Files[0].Size)
		require.NoError(t, ApplyTransfer(l, plan, false))
		data, err := os.ReadFile(l.RedactionVaultPath())
		require.NoError(t, err)
		assert.JSONEq(t, `{"v":1}`, string(data))
		assert.FileExists(t, filepath.Join(l.StateDir, "vault", "redaction.json"), "the copy in the checkout stays")
	})

	t.Run("with --remove the copies go", func(t *testing.T) {
		l := setup(t)
		plan, err := PlanImport(l)
		require.NoError(t, err)
		require.NoError(t, ApplyTransfer(l, plan, true))
		assert.NoFileExists(t, filepath.Join(l.StateDir, "vault", "redaction.json"))
	})

	t.Run("changed after it was shown", func(t *testing.T) {
		l := setup(t)
		plan, err := PlanImport(l)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(l.StateDir, "vault", "added.json"), []byte("{}"), 0o600))
		require.ErrorContains(t, ApplyTransfer(l, plan, false), "changed after it was shown")
		assert.NoFileExists(t, l.RedactionVaultPath(), "nothing was taken")
	})

	t.Run("a link", func(t *testing.T) {
		l := setup(t)
		outside := filepath.Join(t.TempDir(), "secret")
		require.NoError(t, os.WriteFile(outside, []byte("x"), 0o600))
		require.NoError(t, os.Symlink(outside, filepath.Join(l.StateDir, "vault", "link.json")))
		_, err := PlanImport(l)
		require.ErrorContains(t, err, "is a link or not a regular file")
	})

	t.Run("a target that holds other content", func(t *testing.T) {
		l := setup(t)
		withVault(t, l, `{"already":"here"}`)
		plan, err := PlanImport(l)
		require.NoError(t, err)
		require.ErrorContains(t, ApplyTransfer(l, plan, false), "already keeps vault/redaction.json")
	})
}

// kapi state relink moves the state recorded for another location into this
// checkout, exactly as shown, and removes the directory it came from.
func TestRelinkMovesTheStateOfAnEarlierLocation(t *testing.T) {
	captureStateNotices(t)
	old := filepath.Join(t.TempDir(), "old")
	a := newCheckout(t, old)
	withVault(t, a, `{"mine":true}`)
	earlier := a.LocalStateDir()

	b := newCheckout(t, filepath.Join(t.TempDir(), "new"))
	require.NoError(t, EnsureLayout(b))
	plan, err := PlanRelink(b, old)
	require.NoError(t, err)
	assert.Equal(t, earlier, plan.From)
	require.NoError(t, ApplyTransfer(b, plan, false))
	data, err := os.ReadFile(b.RedactionVaultPath())
	require.NoError(t, err)
	assert.JSONEq(t, `{"mine":true}`, string(data))
	assert.NoDirExists(t, earlier)

	_, err = PlanRelink(b, filepath.Join(t.TempDir(), "nowhere"))
	require.ErrorContains(t, err, "recorded no state")
}
