package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenProjectAutoOpensTM(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()

	// Scaffold a sample project into a temp directory.
	require.NoError(t, sample.Scaffold("kapimart", dir))

	// Open the project — should auto-detect .kapi/tm.db and .kapi/termbase.db.
	tab, err := app.OpenProject(filepath.Join(dir, "project.kapi"))
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	app.mu.RLock()
	op := app.projects[tab.ID]
	app.mu.RUnlock()
	require.NotNil(t, op)

	assert.NotEmpty(t, op.tmHandle, "project-scoped TM should be auto-opened")
	assert.NotEmpty(t, op.tbHandle, "project-scoped termbase should be auto-opened")

	// Handles should be valid.
	tm, ok := app.tmHandles.Get(op.tmHandle)
	assert.True(t, ok)
	tmCount, err := tm.Count(t.Context())
	require.NoError(t, err)
	assert.Greater(t, tmCount, 0)

	tb, ok := app.tbHandles.Get(op.tbHandle)
	assert.True(t, ok)
	tbCount, err := tb.Count(t.Context())
	require.NoError(t, err)
	assert.Greater(t, tbCount, 0)
}

func TestGetProjectHandles(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))

	tab, err := app.OpenProject(filepath.Join(dir, "project.kapi"))
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	h := app.GetProjectHandles(tab.ID)
	assert.Equal(t, tab.ID, h.TabID)
	assert.NotEmpty(t, h.TMHandle, "project TM handle should be reachable")
	assert.NotEmpty(t, h.TermbaseHandle, "project termbase handle should be reachable")

	// The bundled ids match the single-getter accessors and resolve to live
	// handles the frontend can preselect.
	assert.Equal(t, app.GetProjectTMHandle(tab.ID), h.TMHandle)
	assert.Equal(t, app.GetProjectTermbaseHandle(tab.ID), h.TermbaseHandle)
	_, ok := app.tmHandles.Get(h.TMHandle)
	assert.True(t, ok)
	_, ok = app.tbHandles.Get(h.TermbaseHandle)
	assert.True(t, ok)
}

func TestGetProjectHandlesUnknownTab(t *testing.T) {
	app := NewApp()
	h := app.GetProjectHandles("nope")
	assert.Equal(t, "nope", h.TabID)
	assert.Empty(t, h.TMHandle)
	assert.Empty(t, h.TermbaseHandle)
}

func TestOpenProjectNoAutoOpenWithoutDotKapi(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "plain")

	app.mu.RLock()
	op := app.projects[tab.ID]
	app.mu.RUnlock()

	assert.Empty(t, op.tmHandle, "no TM handle when .kapi/tm.db doesn't exist")
	assert.Empty(t, op.tbHandle, "no termbase handle when .kapi/termbase.db doesn't exist")
}

func TestCloseProjectClosesHandles(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))

	tab, err := app.OpenProject(filepath.Join(dir, "project.kapi"))
	require.NoError(t, err)

	app.mu.RLock()
	tmHandle := app.projects[tab.ID].tmHandle
	tbHandle := app.projects[tab.ID].tbHandle
	app.mu.RUnlock()

	app.CloseProject(tab.ID)

	// Handles should be closed.
	_, ok := app.tmHandles.Get(tmHandle)
	assert.False(t, ok, "TM handle should be closed")
	_, ok = app.tbHandles.Get(tbHandle)
	assert.False(t, ok, "termbase handle should be closed")
}

func TestCreateSampleProjectIdempotent(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()

	// Scaffold manually first to simulate existing project.
	require.NoError(t, sample.Scaffold("kapimart", dir))

	// Opening the same project twice should return the same tab.
	kapiPath := filepath.Join(dir, "project.kapi")
	tab1, err := app.OpenProject(kapiPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab1.ID) })
	tab2, err := app.OpenProject(kapiPath)
	require.NoError(t, err)
	assert.Equal(t, tab1.ID, tab2.ID)
}

func TestCreateSampleProjectInvalidName(t *testing.T) {
	app := NewApp()
	_, err := app.CreateSampleProject("nonexistent")
	assert.Error(t, err)
}

// A sample left on disk by an older app version may carry a recipe that no
// longer parses (legacy top-level languages, list-form `plugins:`). Opening it
// must recover by re-scaffolding rather than failing (issue #4 follow-up).
func TestCreateSampleProjectRecoversStaleRecipe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAPI_HOME_DIR", home)

	targetDir := filepath.Join(home, "KapiProjects", sample.DisplayName["okapimart"])
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	// Legacy schema: top-level languages + list-form plugins (the exact shape
	// that triggered "cannot unmarshal !!seq into map[string]project.PluginSpec").
	stale := "version: v1\nname: OkapiMart\nsource_language: en-US\ntarget_languages:\n  - fr-FR\nplugins:\n  - okapi-bridge\ncontent:\n  - path: \"input/*.json\"\n    format: okf_json\n"
	kapiPath := filepath.Join(targetDir, "project.kapi")
	require.NoError(t, os.WriteFile(kapiPath, []byte(stale), 0o644))

	// Plant a stale/corrupt state dir: a tm.db with an incompatible schema would
	// break re-seeding ("apply migration N: no such table ..."). Recovery must
	// wipe .kapi so Scaffold reseeds cleanly. A non-DB file reproduces the class.
	staleKapi := filepath.Join(targetDir, ".kapi")
	require.NoError(t, os.MkdirAll(staleKapi, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staleKapi, "tm.db"), []byte("not a database"), 0o644))

	// The stale recipe must not parse...
	_, err := project.Load(kapiPath)
	require.Error(t, err, "precondition: stale recipe should fail to parse")

	app := NewApp()
	tab, err := app.CreateSampleProject("okapimart")
	require.NoError(t, err, "CreateSampleProject should recover a stale sample recipe")
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	// ...and after recovery the on-disk recipe parses cleanly.
	_, err = project.Load(kapiPath)
	require.NoError(t, err, "recipe should parse after re-scaffold")
}

func TestSampleProjectFilesExist(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("okapimart", dir))

	// Verify all expected files.
	for _, path := range []string{
		"project.kapi",
		"input/store-ui.json",
		"input/changelog.md",
		".kapi/tm.db",
		".kapi/termbase.db",
		"output",
	} {
		_, err := os.Stat(filepath.Join(dir, path))
		assert.NoError(t, err, "missing: %s", path)
	}
}
