package project_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLayout_walksUpFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte("name: my-app\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "deep"), 0o755))

	layout, err := project.ResolveLayout(filepath.Join(root, "src", "deep"))
	require.NoError(t, err)
	assert.Equal(t, root, layout.Root)
	assert.Equal(t, filepath.Join(root, "kapi.yaml"), layout.RecipePath)
	assert.Equal(t, filepath.Join(root, ".kapi"), layout.StateDir)
}

func TestResolveLayout_noProjectFound(t *testing.T) {
	root := t.TempDir()
	_, err := project.ResolveLayout(root)
	assert.ErrorIs(t, err, project.ErrNoProject)
}

// A YAML file that is not named kapi.yaml is not a recipe: discovery keys on
// the fixed basename, so an unrelated config.yaml does not make a project.
func TestResolveLayout_ignoresNonRecipeYAML(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("id: x\n"), 0o644))

	_, err := project.ResolveLayout(root)
	assert.ErrorIs(t, err, project.ErrNoProject)
}

func TestResolveLayout_stateWithoutRecipe(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	_, err := project.ResolveLayout(root)
	assert.ErrorIs(t, err, project.ErrRecipeMissing)
}

func TestResolveLayout_startIsAFile(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("name: my-app\n"), 0o644))

	layout, err := project.ResolveLayout(recipe)
	require.NoError(t, err)
	assert.Equal(t, recipe, layout.RecipePath)
}

func TestLayoutFor_explicitRecipePath(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("name: my-app\n"), 0o644))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	assert.Equal(t, recipe, layout.RecipePath)
	assert.Equal(t, filepath.Join(root, ".kapi"), layout.StateDir)
}

// An explicit -p path is trusted even when the file is not named kapi.yaml —
// pointing at a variant recipe is taken at its word (cf. `docker compose -f`).
func TestLayoutFor_acceptsExplicitNonStandardName(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "variant.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: x\n"), 0o644))

	layout, err := project.LayoutFor(path)
	require.NoError(t, err)
	assert.Equal(t, path, layout.RecipePath)
	assert.Equal(t, filepath.Join(root, ".kapi"), layout.StateDir)
}

// Pointing -p at a project directory resolves the kapi.yaml inside it.
func TestLayoutFor_directoryResolvesRecipe(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("name: x\n"), 0o644))

	layout, err := project.LayoutFor(root)
	require.NoError(t, err)
	assert.Equal(t, recipe, layout.RecipePath)

	// A directory with no kapi.yaml is an error.
	_, err = project.LayoutFor(t.TempDir())
	assert.Error(t, err)
}

// EnsureLayout creates the checkout's state directory under the data root and
// writes nothing into the checkout.
func TestEnsureLayout_createsTheCheckoutsStateDir(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("name: my-app\n"), 0o644))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	info, err := os.Stat(layout.LocalStateDir())
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.NoDirExists(t, layout.StateDir)
}

func testLayout(t *testing.T) project.Layout {
	t.Helper()
	root := t.TempDir()
	return project.Layout{
		Root:       root,
		RecipePath: filepath.Join(root, project.RecipeFileName),
		StateDir:   filepath.Join(root, project.StateDirName),
	}
}

// The store sits at the TOP of the work directory, not under work/cache/: it
// carries staged decisions, so `rm -rf .kapi/work/cache` must not take it.
func TestLayout_StorePathIsNotUnderCache(t *testing.T) {
	layout := testLayout(t)
	work := filepath.Join(layout.LocalStateDir(), "work")

	assert.Equal(t, filepath.Join(work, "store.db"), layout.StorePath())
	assert.Equal(t, work, filepath.Dir(layout.StorePath()))
	assert.NotEqual(t, layout.CacheDir(), filepath.Dir(layout.StorePath()))
}

// Every path kapi derives from the working tree sits under work/, so deleting
// work/ is always a re-extraction and nothing more, and nothing else does. The context files an
// import reads and a snapshot writes sit one segment inside `.kapi/`.
func TestLayout_WorkHoldsEveryDerivedPath(t *testing.T) {
	layout := testLayout(t)
	work := layout.WorkDir() + string(filepath.Separator)

	for name, path := range map[string]string{
		"store":       layout.StorePath(),
		"cache":       layout.CacheDir(),
		"extractions": layout.ExtractionsDir(),
		"collections": layout.CollectionsDir(),
	} {
		assert.True(t, strings.HasPrefix(path, work), "%s must live under work/: %s", name, path)
	}

	// What no source reproduces sits beside work/, so deleting work/ loses none
	// of it: the vault and its sidecars, and the venue sync state.
	for name, path := range map[string]string{
		"vault":             layout.VaultDir(),
		"redaction vault":   layout.RedactionVaultPath(),
		"redaction sidecar": layout.RedactionSidecarPath("b-1"),
		"sync state":        layout.SyncDir(),
	} {
		assert.False(t, strings.HasPrefix(path, work), "%s must live outside work/: %s", name, path)
		assert.True(t, strings.HasPrefix(path, layout.LocalStateDir()+string(filepath.Separator)), "%s lives in the checkout's state directory: %s", name, path)
	}

	assert.Equal(t, filepath.Join(layout.StateDir, "state"), layout.Export().UnitStateDir())
	assert.Equal(t, filepath.Join(layout.StateDir, "memory"), layout.Export().MemoryDir())
	assert.Equal(t, filepath.Join(layout.StateDir, "profiles"), layout.Export().ProfilesDir())
	assert.Equal(t, filepath.Join(layout.Export().ProfilesDir(), "bowrain"), layout.Export().ProfileDir("bowrain"))
	assert.Equal(t, filepath.Join(layout.LocalStateDir(), "filters.local.json"), layout.LocalFiltersPath())
}

// EnsureLayout leaves a `.kapi/` that holds files a project commits as it is:
// no rule added, no file written.
func TestEnsureLayout_LeavesTheCommittedDirectoryAlone(t *testing.T) {
	layout := testLayout(t)
	require.NoError(t, os.MkdirAll(filepath.Join(layout.StateDir, "state"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(layout.StateDir, "terms.json"), []byte("{}"), 0o644))

	require.NoError(t, project.EnsureLayout(layout))

	entries, err := os.ReadDir(layout.StateDir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"state", "terms.json"}, names)
	assert.DirExists(t, layout.CacheDir())
}

// EnsureLayout scaffolds the derived state, so a fresh project has somewhere to
// put it before it is written.
func TestEnsureLayout_createsTheWorkDirs(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("name: my-app\n"), 0o644))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	assert.DirExists(t, layout.CacheDir())
	assert.DirExists(t, layout.WorkDir())
	// A scaffold creates no place for a context file: the context lives in the
	// workspace, and an export creates what it writes.
	assert.NoDirExists(t, layout.Export().MemoryDir())
	assert.NoDirExists(t, layout.Export().UnitStateDir())
}

// EnsureLocalDir creates a directory of the checkout's state owner-only.
func TestEnsureLocalDir_ProtectsTheDirectory(t *testing.T) {
	layout := testLayout(t)
	dir, err := project.EnsureLocalDir(layout, project.VaultDirName)
	require.NoError(t, err)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	assert.Equal(t, layout.VaultDir(), dir)
	assert.NoDirExists(t, layout.StateDir)
}
