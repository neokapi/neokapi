package main

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	coreproj "github.com/neokapi/neokapi/core/project"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// daemonProjectRoot scaffolds a bowrain project the daemon can load.
func daemonProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	_, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
	})
	require.NoError(t, err)
	return root
}

// One process, one App, one store handle per project — however many projects the
// daemon serves and however many RPCs land on each. A connector that opened a
// store of its own would put a second connection pool on `.kapi/work/store.db`, and
// the in-process write gate is per pool: it could not order a push's decision
// staging against anything else the daemon writes to that project.
func TestDaemon_OneStoreHandlePerProject(t *testing.T) {
	prev := app
	app = &cli.App{}
	app.InitRegistries()
	t.Cleanup(func() {
		app.Shutdown()
		app = prev
	})

	rootA, rootB := daemonProjectRoot(t), daemonProjectRoot(t)
	d := newDaemonService(nil)

	// Two RPCs against one project reach one entry, and the second load does not
	// mint a second store.
	first, err := d.projectFor(rootA)
	require.NoError(t, err)
	again, err := d.projectFor(rootA)
	require.NoError(t, err)
	assert.Same(t, first, again, "the daemon caches one project entry per root")

	dbA, err := app.ProjectDB(t.Context(), rootA)
	require.NoError(t, err)
	sameA, err := app.ProjectDB(t.Context(), rootA)
	require.NoError(t, err)
	assert.Same(t, dbA, sameA, "one store handle per project root")

	// A second project is a second handle, not a shared one.
	_, err = d.projectFor(rootB)
	require.NoError(t, err)
	dbB, err := app.ProjectDB(t.Context(), rootB)
	require.NoError(t, err)
	assert.NotSame(t, dbA, dbB)

	// And the App is what releases them, which is why runDaemon defers Shutdown.
	app.Shutdown()
	assert.Nil(t, dbA.Raw())
	assert.Nil(t, dbB.Raw())
}

// The daemon reads formats through the process App's registry rather than one
// built beside it, so a single object answers "what can this daemon read" the
// same way the App answers "which store is this project's".
func TestDaemon_UsesTheProcessAppsFormatRegistry(t *testing.T) {
	prev := app
	app = &cli.App{}
	app.InitRegistries()
	t.Cleanup(func() {
		app.Shutdown()
		app = prev
	})

	d := newDaemonService(nil)
	assert.Same(t, app.FormatReg, d.formatReg)

	root := daemonProjectRoot(t)
	entry, err := d.projectFor(root)
	require.NoError(t, err)
	assert.Same(t, app.FormatReg, entry.formatReg)
	assert.FileExists(t, filepath.Join(root, coreproj.RecipeFileName))
}
