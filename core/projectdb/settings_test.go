package projectdb_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/workspace"
)

// A project setting is written in one checkout and read in another: it lives
// in the project's context store, and never in a checkout's projection.
func TestSetting_SharedByEveryCheckoutOfTheProject(t *testing.T) {
	base := t.TempDir()
	ws, err := workspace.OpenLocal(t.Context(), filepath.Join(base, "workspaces", "default"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	ctx := t.Context()
	first := openWorkspaceProject(t, ws, checkout(t, base, "clone-a"), "prj_settings")
	second := openWorkspaceProject(t, ws, checkout(t, base, "clone-b"), "prj_settings")

	_, ok, err := second.Setting(ctx, projectdb.SettingSavedFilters)
	require.NoError(t, err)
	assert.False(t, ok, "an unwritten setting reads as absent")

	require.NoError(t, first.PutSetting(ctx, projectdb.SettingSavedFilters, `[{"id":"a"}]`))
	got, ok, err := second.Setting(ctx, projectdb.SettingSavedFilters)
	require.NoError(t, err)
	require.True(t, ok)
	assert.JSONEq(t, `[{"id":"a"}]`, got)

	require.NoError(t, second.PutSetting(ctx, projectdb.SettingSavedFilters, `[]`))
	got, _, err = first.Setting(ctx, projectdb.SettingSavedFilters)
	require.NoError(t, err)
	assert.Equal(t, `[]`, got, "a write replaces the value")

	_, inProjection, err := first.Meta(ctx, "setting."+projectdb.SettingSavedFilters)
	require.NoError(t, err)
	assert.False(t, inProjection, "the checkout's projection holds no setting")
}
