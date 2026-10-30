package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

func TestADraftProjectKeepsItsStoreOutsideTheFolder(t *testing.T) {
	ctx := t.Context()
	a := &App{}
	a.SetWorkspaceRoot(filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(a.closeProjectStores)

	root := t.TempDir()
	state := filepath.Join(t.TempDir(), "draft")
	key := workspace.ProjectKey(project.NewID())
	require.NoError(t, a.BindDraftProject(root, key, state))
	assert.Equal(t, project.Layout{StateDir: state}.StorePath(), a.ProjectStorePath(root))

	_, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	assert.FileExists(t, project.Layout{StateDir: state}.StorePath())
	assert.NoDirExists(t, filepath.Join(root, project.StateDirName))
	p, err := a.Projector(ctx, root)
	require.NoError(t, err)
	assert.Equal(t, key, p.Key(), "the draft binds the context under the key it was given")

	ws, err := a.Workspace(ctx)
	require.NoError(t, err)
	regs, err := ws.Projects(ctx)
	require.NoError(t, err)
	assert.Empty(t, regs, "a draft registers nothing")

	// With the recipe written, the store moves into the folder and registers
	// under the recipe's id.
	require.NoError(t, project.Save(filepath.Join(root, project.RecipeFileName),
		&project.KapiProject{Version: project.CurrentVersion, ID: string(key)}))
	require.NoError(t, a.UnbindDraftProject(root))
	_, err = a.ProjectDB(ctx, root)
	require.NoError(t, err)
	assert.FileExists(t, project.LayoutAt(root).StorePath())
	regs, err = ws.Projects(ctx)
	require.NoError(t, err)
	require.Len(t, regs, 1)
	assert.Equal(t, key, regs[0].Key)
}

func TestBindDraftProjectRefusesAnOpenStore(t *testing.T) {
	a := &App{}
	a.SetWorkspaceRoot(filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(a.closeProjectStores)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\nname: x\n"), 0o644))
	_, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	require.Error(t, a.BindDraftProject(root, "prj_aaaaaaaaaaaaaaaaaaaaaaaaaa", t.TempDir()))
}
