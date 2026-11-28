//go:build unix

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/project"
)

// stateDialogCheckout writes a checkout whose `.kapi/` holds a vault and a
// sync state an earlier kapi left there, and returns its recipe.
func stateDialogCheckout(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: legacy\n"), 0o644))
	for rel, data := range map[string]string{
		".kapi/vault/redaction.json":  `{"version":3,"values":[]}`,
		".kapi/sync/sync-cache.json":  `{"claim_token":"clm","server_url":"https://app.example","project_id":"p1"}`,
		".kapi/work/cache/stale.json": `{}`,
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(data), 0o600))
	}
	return recipe
}

// A dialog is given the lines a checkout owes a person, each with the
// transfer it offers, and asking writes nothing.
func TestPendingCheckoutStateListsTheLinesAndTheirTransfers(t *testing.T) {
	recipe := stateDialogCheckout(t)
	root := filepath.Dir(recipe)

	pending, err := PendingCheckoutState(recipe)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, project.NoticeWorkInTree, pending[0].Name)
	assert.Nil(t, pending[0].Transfer, "the work directory offers no transfer")
	assert.Equal(t, project.NoticeStateInTree, pending[1].Name)
	require.NotNil(t, pending[1].Transfer)
	assert.Equal(t, "import", pending[1].Transfer.Action)
	assert.Len(t, pending[1].Transfer.Files, 2)
	assert.Equal(t, []string{"https://app.example"}, pending[1].Transfer.Servers)
	assert.Equal(t, "https://app.example", pending[1].Transfer.SyncServer)
	assert.Equal(t, "p1", pending[1].Transfer.SyncProject, "the dialog names the server project the state is bound to")
	assert.NotEmpty(t, pending[1].Digest)
	assert.False(t, dataroot.CheckoutExists(root), "asking creates no record")

	require.NoError(t, RecordCheckoutStateShown(recipe, []string{project.NoticeWorkInTree, project.NoticeStateInTree}))
	pending, err = PendingCheckoutState(recipe)
	require.NoError(t, err)
	assert.Empty(t, pending, "a line a person was shown is not offered again")

	require.ErrorContains(t, RecordCheckoutStateShown(recipe, []string{"other"}), `no state notice is named "other"`)
}

// A transfer is taken only as the dialog showed it, and only by a person.
func TestApplyCheckoutStateTransferTakesWhatWasShown(t *testing.T) {
	recipe := stateDialogCheckout(t)
	root := filepath.Dir(recipe)
	pending, err := PendingCheckoutState(recipe)
	require.NoError(t, err)
	shown := pending[1]
	require.Equal(t, project.NoticeStateInTree, shown.Name)
	a := &App{}

	t.Run("an agent cannot take it", func(t *testing.T) {
		asAgent(t, "claude", "s-1")
		_, err := a.ApplyCheckoutStateTransfer(recipe, shown.Name, shown.Digest, false)
		require.ErrorContains(t, err, "only a person can bring state into a checkout")
	})

	t.Run("a plan that changed after it was shown is refused", func(t *testing.T) {
		added := filepath.Join(root, ".kapi", "vault", "added.json")
		require.NoError(t, os.WriteFile(added, []byte("{}"), 0o600))
		_, err := a.ApplyCheckoutStateTransfer(recipe, shown.Name, shown.Digest, false)
		require.ErrorContains(t, err, "changed after it was shown")
		require.NoError(t, os.Remove(added))
	})

	t.Run("a line that offers no transfer", func(t *testing.T) {
		_, err := a.ApplyCheckoutStateTransfer(recipe, project.NoticeWorkInTree, shown.Digest, false)
		require.ErrorContains(t, err, "offers no transfer")
	})

	out, err := a.ApplyCheckoutStateTransfer(recipe, shown.Name, shown.Digest, true)
	require.NoError(t, err)
	assert.True(t, out.Taken)
	assert.True(t, out.SourceRemoved)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	data, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":3,"values":[]}`, string(data))
	assert.NoFileExists(t, filepath.Join(root, ".kapi", "vault", "redaction.json"), "with remove the copies go")
}

// A checkout of the same project cloned where another was is offered the
// relink of the earlier state, and taking it brings that state across.
func TestApplyCheckoutStateTransferRelinksAnEarlierRecord(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	root := filepath.Join(parent, "app")
	require.NoError(t, os.MkdirAll(root, 0o755))
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: first\nid: prj_aaaaaaaaaaaaaaaaaaaaaaaa\n"), 0o644))
	first, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	_, err = project.EnsureLocalDir(first, project.VaultDirName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(first.RedactionVaultPath(), []byte(`{"mine":true}`), 0o600))

	require.NoError(t, os.RemoveAll(root))
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: second\nid: prj_aaaaaaaaaaaaaaaaaaaaaaaa\n"), 0o644))

	pending, err := PendingCheckoutState(recipe)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, project.NoticeEarlierAtPath, pending[0].Name)
	require.NotNil(t, pending[0].Transfer)
	assert.Equal(t, "relink", pending[0].Transfer.Action)

	out, err := (&App{}).ApplyCheckoutStateTransfer(recipe, pending[0].Name, pending[0].Digest, true)
	require.NoError(t, err)
	assert.True(t, out.Taken)
	assert.True(t, pending[0].Transfer.RemovesSource, "the plan says the earlier directory goes")
	second, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	data, err := os.ReadFile(second.RedactionVaultPath())
	require.NoError(t, err)
	assert.JSONEq(t, `{"mine":true}`, string(data))
}

// State recorded at the location for another project is named, and offers no
// transfer.
func TestPendingCheckoutStateOffersNoTransferOfAnotherProjectsState(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	root := filepath.Join(parent, "app")
	require.NoError(t, os.MkdirAll(root, 0o755))
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: billing\n"), 0o644))
	first, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	_, err = project.EnsureLocalDir(first, project.VaultDirName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(first.RedactionVaultPath(), []byte(`{"theirs":true}`), 0o600))

	require.NoError(t, os.RemoveAll(root))
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: docs\n"), 0o644))

	pending, err := PendingCheckoutState(recipe)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, project.NoticeEarlierAtPath, pending[0].Name)
	assert.Contains(t, pending[0].Message, `belongs to "billing", another project`)
	assert.Equal(t, `"billing"`, pending[0].OtherProject)
	assert.Nil(t, pending[0].Transfer)
	_, err = (&App{}).ApplyCheckoutStateTransfer(recipe, pending[0].Name, "", false)
	require.Error(t, err)
	second, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	assert.NoFileExists(t, second.RedactionVaultPath())
}

// An import that takes every file and cannot remove some of the copies it was
// asked to remove is reported as taken, with the copies that remain.
func TestApplyCheckoutStateTransferReportsTheCopiesThatRemain(t *testing.T) {
	recipe := stateDialogCheckout(t)
	root := filepath.Dir(recipe)
	pending, err := PendingCheckoutState(recipe)
	require.NoError(t, err)
	shown := pending[1]
	syncDir := filepath.Join(root, ".kapi", "sync")
	require.NoError(t, os.Chmod(syncDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(syncDir, 0o700) })

	out, err := (&App{}).ApplyCheckoutStateTransfer(recipe, shown.Name, shown.Digest, true)
	require.NoError(t, err)
	assert.True(t, out.Taken)
	assert.False(t, out.SourceRemoved)
	assert.Equal(t, []string{"sync/sync-cache.json"}, out.Remaining)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	assert.FileExists(t, layout.RedactionVaultPath())
	assert.FileExists(t, filepath.Join(layout.SyncDir(), "sync-cache.json"))
	assert.NoFileExists(t, filepath.Join(root, ".kapi", "vault", "redaction.json"))
}

// A relink that leaves the original text of hidden values and the saved
// sign-in, because kapi cannot confirm the state is this project's, names the
// terminal command a person who knows it is runs; nothing in the dialog takes
// them.
func TestPendingCheckoutStateNamesTheCommandThatStatesTheSameProject(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	root := filepath.Join(parent, "my app")
	require.NoError(t, os.MkdirAll(root, 0o755))
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: app\n"), 0o644))
	first, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	_, err = project.EnsureLocalDir(first, project.VaultDirName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(first.RedactionVaultPath(), []byte(`{"mine":true}`), 0o600))

	require.NoError(t, os.RemoveAll(root))
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: app\n"), 0o644))

	pending, err := PendingCheckoutState(recipe)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	n := pending[0]
	require.NotNil(t, n.Transfer, "a plan that only leaves files is still shown")
	assert.True(t, n.Transfer.Unverified)
	assert.Empty(t, n.Transfer.Files)
	assert.Len(t, n.Transfer.Withheld, 1)
	assert.Equal(t, "kapi state relink --from '"+root+"' --same-project -p '"+root+"'", n.SameProjectCommand)
	assert.False(t, n.SameProjectLeavesSignIn, "the command takes everything this plan leaves")
}

func TestPendingCheckoutStateSaysWhenTheCommandStillLeavesTheSignIn(t *testing.T) {
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	root := filepath.Join(parent, "app")
	require.NoError(t, os.MkdirAll(root, 0o755))
	recipe := filepath.Join(root, "kapi.yaml")
	// Neither recipe carries an id, so kapi cannot confirm the project, and
	// the two connect to different server projects.
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: app\nbowrain:\n  url: https://app.example/team/p1\n"), 0o644))
	first, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	_, err = project.EnsureLocalDir(first, project.VaultDirName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(first.RedactionVaultPath(), []byte(`{"mine":true}`), 0o600))
	_, err = project.EnsureLocalDir(first, project.SyncDirName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(first.SyncDir(), "sync-cache.json"),
		[]byte(`{"claim_token":"clm","server_url":"https://app.example","workspace":"team","project_id":"p1"}`), 0o600))

	require.NoError(t, os.RemoveAll(root))
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: app\nbowrain:\n  url: https://app.example/team/p2\n"), 0o644))

	pending, err := PendingCheckoutState(recipe)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	n := pending[0]
	require.NotNil(t, n.Transfer)
	assert.True(t, n.Transfer.Unverified)
	assert.Len(t, n.Transfer.Withheld, 2, "the vault and the sync state both stay")
	assert.NotEmpty(t, n.SameProjectCommand, "stating the project releases the vault")
	assert.True(t, n.SameProjectLeavesSignIn, "the sync state of another server project stays whatever is stated")
}
