//go:build unix

package project

import (
	"errors"
	"io/fs"
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

// captureStateNotices collects the one-time lines kapi shows, as a person at
// a terminal would see them.
func captureStateNotices(t *testing.T) *[]string {
	t.Helper()
	var got []string
	prev := StateNotice
	StateNotice = func(msg string) bool { got = append(got, msg); return true }
	t.Cleanup(func() { StateNotice = prev })
	return &got
}

// unseenStateNotices collects the lines kapi offers when nobody is there to
// see them: StateNotice reports each as not shown.
func unseenStateNotices(t *testing.T) *[]string {
	t.Helper()
	var got []string
	prev := StateNotice
	StateNotice = func(msg string) bool { got = append(got, msg); return false }
	t.Cleanup(func() { StateNotice = prev })
	return &got
}

// dataRootEntries lists what the data root holds under checkouts/.
func dataRootEntries(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(dataroot.CheckoutsDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// Looking at a checkout writes nothing: NoticeState on a checkout with no
// state and nothing to tell leaves the data root as it found it.
func TestNoticeStateOnACheckoutWithNothingToTellWritesNothing(t *testing.T) {
	notices := captureStateNotices(t)
	l := newCheckout(t, filepath.Join(t.TempDir(), "app"))
	before := dataRootEntries(t)
	NoticeState(l)
	assert.Empty(t, *notices)
	assert.Equal(t, before, dataRootEntries(t), "no checkout record was created")
	assert.False(t, dataroot.CheckoutExists(l.Root))
}

// A notice nobody was shown is not recorded as given, and creates no record;
// the next run with a person to tell shows it, once.
func TestANoticeNobodySawIsGivenOnTheNextShownRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	l := newCheckout(t, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, StateDirName, WorkDirName), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, StateDirName, VaultDirName), 0o700))

	unseen := unseenStateNotices(t)
	before := dataRootEntries(t)
	NoticeState(l)
	assert.Len(t, *unseen, 2, "both lines were offered")
	assert.Equal(t, before, dataRootEntries(t), "a notice nobody saw creates no record")

	shown := captureStateNotices(t)
	NoticeState(l)
	require.Len(t, *shown, 2)
	assert.Contains(t, (*shown)[0], ".kapi/work in this checkout is no longer used")
	assert.Contains(t, (*shown)[1], "kapi state import -p "+root)
	rec, err := dataroot.ReadRecord(dataroot.CheckoutDir(root))
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"work-in-tree": true, "state-in-tree": true}, rec.Told)

	NoticeState(l)
	assert.Len(t, *shown, 2, "each is shown once")
}

// A notice offered while the checkout's state is created, and not shown, stays
// unrecorded: the store opened for a `--json` run does not consume it.
func TestANoticeNotShownWhileStateIsCreatedStaysOwed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	l := newCheckout(t, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, StateDirName, WorkDirName), 0o755))

	unseenStateNotices(t)
	require.NoError(t, EnsureLayout(l))
	rec, err := dataroot.ReadRecord(dataroot.CheckoutDir(root))
	require.NoError(t, err)
	assert.Empty(t, rec.Told, "the record exists for the state, and holds no notice")

	shown := captureStateNotices(t)
	NoticeState(l)
	require.Len(t, *shown, 1)
	NoticeState(l)
	assert.Len(t, *shown, 1, "shown once")
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

// A repository cloned where another checkout was is another directory at the
// same location: it sees none of that checkout's state, the person is
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
		err = ApplyTransfer(l, plan, false)
		require.ErrorContains(t, err, filepath.Join("vault", "redaction.json")+", with other content, so nothing was taken")
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
	plan, err := PlanRelink(b, old, RelinkOptions{})
	require.NoError(t, err)
	assert.Equal(t, earlier, plan.From)
	require.NoError(t, ApplyTransfer(b, plan, false))
	data, err := os.ReadFile(b.RedactionVaultPath())
	require.NoError(t, err)
	assert.JSONEq(t, `{"mine":true}`, string(data))
	assert.NoDirExists(t, earlier)

	_, err = PlanRelink(b, filepath.Join(t.TempDir(), "nowhere"), RelinkOptions{})
	require.ErrorContains(t, err, "recorded no state")
}

// The line about state recorded for another folder at the location stays owed
// when the checkout's state is created with nobody to show it to (a desktop
// opening the project), and the next surface with a person shows it, once.
func TestTheEarlierAtPathLineSurvivesAnUnseenCreation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	a := newCheckout(t, root)
	withVault(t, a, `{"mine":true}`)
	require.NoError(t, os.RemoveAll(root))
	b := newCheckout(t, root)

	unseenStateNotices(t)
	require.NoError(t, EnsureLayout(b))
	pending := PendingStateNotices(b)
	require.Len(t, pending, 1)
	assert.Equal(t, NoticeEarlierAtPath, pending[0].Name)

	shown := captureStateNotices(t)
	NoticeState(b)
	require.Len(t, *shown, 1)
	assert.Contains(t, (*shown)[0], "kapi state relink --from "+root)
	NoticeState(b)
	assert.Len(t, *shown, 1, "shown once")
	assert.Empty(t, PendingStateNotices(b))
}
