package backend

import (
	"os"
	"path/filepath"
	"testing"

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
