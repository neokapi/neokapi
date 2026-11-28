//go:build !windows

package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A project opened with state an earlier kapi left in its checkout offers the
// import in a dialog: opening it consumes no line, the dialog's showing does,
// and confirming takes the files the dialog listed.
func TestCheckoutState_TheDesktopOffersTheImport(t *testing.T) {
	app := newWorkspaceApp(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipe := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: Legacy\n"), 0o644))
	vault := filepath.Join(dir, ".kapi", "vault", "redaction.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(vault), 0o700))
	require.NoError(t, os.WriteFile(vault, []byte(`{"version":3,"values":[]}`), 0o600))
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	pending, err := app.PendingCheckoutState(tab.ID)
	require.NoError(t, err)
	require.Len(t, pending, 1, "opening the project gave the line to nobody")
	require.Equal(t, project.NoticeStateInTree, pending[0].Name)
	require.NotNil(t, pending[0].Transfer)
	assert.Equal(t, "import", pending[0].Transfer.Action)

	require.NoError(t, app.MarkCheckoutStateShown(tab.ID, []string{pending[0].Name}))
	again, err := app.PendingCheckoutState(tab.ID)
	require.NoError(t, err)
	assert.Empty(t, again, "the dialog showed it")

	out, err := app.ApplyCheckoutStateTransfer(tab.ID, pending[0].Name, pending[0].Digest, false)
	require.NoError(t, err)
	assert.True(t, out.Taken)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	assert.FileExists(t, layout.RedactionVaultPath())
	assert.FileExists(t, vault, "the copy in the checkout stays unless removed")
}
