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

func TestEnsureLayout_createsStateDir(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("name: my-app\n"), 0o644))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	info, err := os.Stat(layout.StateDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// State manifest round-trip.
func TestStateManifest_roundTrip(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("name: my-app\n"), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)

	orig := &project.StateManifest{
		Generator: project.StateGenerator{ID: "kapi", Version: "0.5.0"},
		Project:   project.StateProjectRef{ID: "my-app", Path: "../kapi.yaml"},
		Blocks: map[string]project.StateBlockStats{
			"ui": {Count: 42, SHA256: "abc123"},
		},
	}
	require.NoError(t, project.SaveState(layout, orig))

	got, err := project.LoadState(layout)
	require.NoError(t, err)
	assert.Equal(t, "my-app", got.Project.ID)
	assert.Equal(t, 42, got.Blocks["ui"].Count)
	assert.Equal(t, "abc123", got.Blocks["ui"].SHA256)
	assert.NotEmpty(t, got.UpdatedAt)
}

func TestLoadState_missingFileReturnsNil(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("name: my-app\n"), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	got, err := project.LoadState(layout)
	require.NoError(t, err)
	assert.Nil(t, got)
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
	work := filepath.Join(layout.StateDir, "work")

	assert.Equal(t, filepath.Join(work, "store.db"), layout.StorePath())
	assert.Equal(t, filepath.Join(work, "store.json"), layout.StoreSidecarPath())
	assert.Equal(t, work, filepath.Dir(layout.StorePath()))
	assert.NotEqual(t, layout.CacheDir(), filepath.Dir(layout.StorePath()))
}

// `.kapi/` splits exactly once: `work/` is machine state, everything else is
// committed. That is the whole ignore rule, so it is asserted as one property —
// every derived path sits under work/, and every authored one does not.
func TestLayout_WorkHoldsEveryDerivedPath(t *testing.T) {
	layout := testLayout(t)
	work := layout.WorkDir() + string(filepath.Separator)

	for name, path := range map[string]string{
		"store":             layout.StorePath(),
		"store sidecar":     layout.StoreSidecarPath(),
		"cache":             layout.CacheDir(),
		"extractions":       layout.ExtractionsDir(),
		"collections":       layout.CollectionsDir(),
		"vault":             layout.VaultDir(),
		"redaction vault":   layout.RedactionVaultPath(),
		"redaction sidecar": layout.RedactionSidecarPath("b-1"),
	} {
		assert.True(t, strings.HasPrefix(path, work), "%s must live under work/: %s", name, path)
	}

	committed := layout.StateDir + string(filepath.Separator)
	for name, path := range map[string]string{
		"memory":     layout.MemoryDir(),
		"unit state": layout.UnitStateDir(),
		"profiles":   layout.ProfilesDir(),
		"profile":    layout.ProfileDir("bowrain"),
		"filters":    layout.FiltersPath(),
	} {
		assert.True(t, strings.HasPrefix(path, committed), "%s must live under .kapi/: %s", name, path)
		assert.False(t, strings.HasPrefix(path, work), "%s is committed and must not live under work/: %s", name, path)
	}

	// The one gitignored path outside work/: personal, user-edited, and named
	// individually by the ignore file for exactly that reason.
	assert.Equal(t, filepath.Join(layout.StateDir, "filters.local.json"), layout.LocalFiltersPath())
}

// The committed sources sit one segment inside `.kapi/`, not two: no umbrella
// directory groups what is already grouped by being committed at all.
func TestLayout_CommittedSourcesAreFlat(t *testing.T) {
	layout := testLayout(t)
	assert.Equal(t, filepath.Join(layout.StateDir, "state"), layout.UnitStateDir())
	assert.Equal(t, filepath.Join(layout.StateDir, "memory"), layout.MemoryDir())
	assert.Equal(t, filepath.Join(layout.StateDir, "profiles"), layout.ProfilesDir())
	assert.Equal(t, filepath.Join(layout.ProfilesDir(), "bowrain"), layout.ProfileDir("bowrain"))
	assert.Equal(t, filepath.Join(".kapi", "terms.json"), project.RelStatePath("terms.json"))
	assert.Equal(t, filepath.Join(".kapi", "memory", "memory.json"),
		project.RelStatePath(project.MemoryDirName, "memory.json"))
}

// EnsureLayout scaffolds both halves, so a fresh project has somewhere to put
// authored context and somewhere to put derived state before either is written.
func TestEnsureLayout_createsCommittedAndWork(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("name: my-app\n"), 0o644))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	assert.DirExists(t, layout.MemoryDir())
	assert.DirExists(t, layout.UnitStateDir())
	assert.DirExists(t, layout.CacheDir())
	assert.DirExists(t, layout.WorkDir())
}
