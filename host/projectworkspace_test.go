package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/terms"
)

// seedProjectStore opens a project's store the way a command does, through an
// App, and hands it to seed.
//
// Tests that put terms, voice profiles or decisions into a project before
// running a command have to go through here. A store opened with no workspace
// keeps its context tables beside the projection in the checkout, which is the
// embedded layout and not where any command looks.
func seedProjectStore(t *testing.T, root string, seed func(db *projectdb.DB)) {
	t.Helper()
	a := &App{}
	defer a.Shutdown()
	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	seed(db)
}

// writeWorkspaceProject scaffolds a checkout with a recipe carrying the given
// identity, and returns its root.
func writeWorkspaceProject(t *testing.T, dir, id, name string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	recipe := "version: 1\n"
	if id != "" {
		recipe += "id: " + id + "\n"
	}
	recipe += "name: " + name + "\ndefaults:\n  source_language: en\n  target_languages: [nb]\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, project.RecipeFileName), []byte(recipe), 0o644))
	require.NoError(t, project.EnsureLayout(project.LayoutAt(dir)))
	return dir
}

func TestWorkspace_TwoCheckoutsOfOneProjectShareTheirContext(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspaces", "shared")

	const id = "prj_aaaabbbbccccddddeeee"
	first := writeWorkspaceProject(t, filepath.Join(base, "clone-a"), id, "Docs")
	second := writeWorkspaceProject(t, filepath.Join(base, "clone-b"), id, "Docs")

	a := &App{}
	a.SetWorkspaceRoot(root)
	defer a.Shutdown()

	ctx := t.Context()
	dbA, err := a.ProjectDB(ctx, first)
	require.NoError(t, err)
	dbB, err := a.ProjectDB(ctx, second)
	require.NoError(t, err)

	assert.Equal(t, dbA.ContextPath(), dbB.ContextPath(), "one project, one context store")
	assert.NotEqual(t, dbA.Path(), dbB.Path(), "each checkout keeps its own projection")

	require.NoError(t, dbA.Terms().AddConcept(ctx, terms.Concept{
		ID:    "c1",
		Terms: []terms.Term{{Text: "content memory", Locale: model.LocaleEnglish, Status: model.TermPreferred}},
	}))
	has, err := dbB.HasTerms(ctx)
	require.NoError(t, err)
	assert.True(t, has, "the second checkout reads what was authored in the first")
}

func TestWorkspace_RegistersEveryProjectItOpens(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspaces", "registry")
	docs := writeWorkspaceProject(t, filepath.Join(base, "docs"), "prj_aaaabbbbccccddddeeee", "Docs")
	app := writeWorkspaceProject(t, filepath.Join(base, "app"), "prj_ffffgggghhhhiiiijjjj", "App")

	a := &App{}
	a.SetWorkspaceRoot(root)
	defer a.Shutdown()

	ctx := t.Context()
	_, err := a.ProjectDB(ctx, docs)
	require.NoError(t, err)
	_, err = a.ProjectDB(ctx, app)
	require.NoError(t, err)

	ws, err := a.Workspace(ctx)
	require.NoError(t, err)
	projects, err := ws.Projects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 2, "two projects register in one workspace")

	byKey := map[workspace.ProjectKey]workspace.Registration{}
	for _, p := range projects {
		byKey[p.Key] = p
	}
	require.Contains(t, byKey, workspace.ProjectKey("prj_aaaabbbbccccddddeeee"))
	reg := byKey["prj_aaaabbbbccccddddeeee"]
	assert.Equal(t, "Docs", reg.Name)
	assert.Equal(t, []string{NormalizeCheckoutPath(docs)}, reg.Checkouts)
	assert.False(t, reg.LastActive.IsZero())
}

func TestWorkspace_ARecipeWithNoIdentityIsKeyedByItsCheckout(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspaces", "anonymous")
	// Two checkouts, neither stating an id or a name.
	one := writeWorkspaceProject(t, filepath.Join(base, "one"), "", "")
	two := writeWorkspaceProject(t, filepath.Join(base, "two"), "", "")

	a := &App{}
	a.SetWorkspaceRoot(root)
	defer a.Shutdown()

	ctx := t.Context()
	dbOne, err := a.ProjectDB(ctx, one)
	require.NoError(t, err)
	dbTwo, err := a.ProjectDB(ctx, two)
	require.NoError(t, err)
	assert.NotEqual(t, dbOne.ContextPath(), dbTwo.ContextPath(),
		"a project stating no identity claims no other checkout's context")
}

func TestWorkspace_TheContextStoreIsOutsideTheCheckout(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspaces", "outside")
	checkout := writeWorkspaceProject(t, filepath.Join(base, "docs"), "prj_aaaabbbbccccddddeeee", "Docs")

	a := &App{}
	a.SetWorkspaceRoot(root)
	defer a.Shutdown()

	db, err := a.ProjectDB(t.Context(), checkout)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(db.ContextPath(), root),
		"the context store is in the workspace, at %s", db.ContextPath())
	assert.False(t, strings.HasPrefix(db.ContextPath(), checkout),
		"and nothing of it is left in the checkout")
	assert.True(t, strings.HasPrefix(db.Path(), checkout),
		"the projection stays with the tree it describes")
}

func TestDataDirNeverResolvesToTheRealUserRootInATest(t *testing.T) {
	// The isolation contract's variables, minus KAPI_DATA_DIR: the case a test
	// that has not named a data root falls into.
	t.Setenv(EnvDataDir, "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	dir := DataDir()
	assert.True(t, strings.HasPrefix(dir, os.TempDir()),
		"a test binary's data root is under the system temporary directory, got %s", dir)

	real := dataDir(func(string) string { return "" }, "darwin")
	assert.NotEqual(t, real, dir)
	assert.NotEqual(t, dataDir(func(string) string { return "" }, "linux"), dir)

	assert.True(t, strings.HasPrefix(DefaultWorkspaceDir(), dir),
		"and the default workspace is inside it")

	// Naming a root is still honoured, which is how the isolation env pins one.
	named := t.TempDir()
	t.Setenv(EnvDataDir, named)
	assert.Equal(t, named, DataDir())
}

// TestIsolationEnvironmentsCarryTheDataDir reads the three isolation
// environments the repository maintains and asserts each pins $KAPI_DATA_DIR.
//
// $KAPI_DATA_DIR wins over $XDG_DATA_HOME, so an environment that sets only the
// latter leaves a developer's exported KAPI_DATA_DIR in force and an in-repo
// kapi writing into their own workspace. The other three surfaces launch a
// released binary, which is not a test binary and resolves the platform
// default, so the variable is the only thing standing between them and the
// developer's context.
func TestIsolationEnvironmentsCarryTheDataDir(t *testing.T) {
	// Relative to the host module, which sits one level under the repository
	// root.
	files := []struct {
		name string
		path string
	}{
		{"the Makefile's KAPI_ISO_ENV", filepath.Join("..", "Makefile")},
		{"kapi/e2e's isoEnv", filepath.Join("..", "kapi", "e2e", "e2e_test.go")},
		{"the harness's kapiIsolationEnv", filepath.Join("..", "harness", "src", "lib", "paths.ts")},
	}
	for _, f := range files {
		t.Run(f.name, func(t *testing.T) {
			body, err := os.ReadFile(f.path)
			if os.IsNotExist(err) {
				t.Skipf("%s is not in this checkout", f.path)
			}
			require.NoError(t, err)
			assert.Contains(t, string(body), EnvDataDir,
				"%s must pin %s; see the isolation contract in CLAUDE.md", f.name, EnvDataDir)
		})
	}
}
