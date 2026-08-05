package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storeProject scaffolds a saved project whose store already exists, and returns
// its tab and root.
func storeProject(t *testing.T, app *App) (*TabInfo, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(path, &project.KapiProject{Version: "v1", Name: "Engine"}))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab, root
}

// The desktop used to build a second host.App for a converge run while a tab
// held the project's stores open. Two Apps meant two connection pools on one
// `.kapi/work/store.db`, and the in-process write gate is per pool: it could order
// neither set of writers against the other, which is the starvation the merged
// store exists to remove. A run-scoped App now borrows the engine's stores, so
// the run, the tab and the review loop reach one handle.
func TestBorrowEngine_RunAppReachesTheTabsStore(t *testing.T) {
	app := NewApp()
	tab, root := storeProject(t, app)

	fromTab, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)

	run := app.borrowEngine(&host.App{})
	fromRun, err := run.ProjectDB(context.Background(), root)
	require.NoError(t, err)

	assert.Same(t, fromTab, fromRun, "a run reads and writes the tab's handle")
	assert.Same(t, fromTab.Memory(), fromRun.Memory())
	assert.Same(t, fromTab.Work(), fromRun.Work())
}

// The checks App is a second App for a real reason — it owns the document cache
// a checks run opens — but it is not a second store owner.
func TestChecksApp_BorrowsTheEnginesStores(t *testing.T) {
	app := NewApp()
	tab, root := storeProject(t, app)

	fromTab, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)

	app.checksMu.Lock()
	checks := app.checksCLI()
	app.checksMu.Unlock()
	fromChecks, err := checks.ProjectDB(context.Background(), root)
	require.NoError(t, err)

	assert.Same(t, fromTab, fromChecks)
}

// Service teardown releases the project stores. Nothing else may: every other
// host.App the desktop builds borrows these handles.
func TestServiceShutdown_ReleasesTheProjectStores(t *testing.T) {
	app := NewApp()
	tab, _ := storeProject(t, app)
	db, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)
	require.NotNil(t, db.Raw())

	require.NoError(t, app.ServiceShutdown())
	assert.Nil(t, db.Raw(), "the store's pool is released on the way out")
}

// A borrowed handle is forgotten on Close, not closed: a project's content
// memory and terms are two schemas of one store, so closing either through the
// handle store would take the block cache and the working set with it.
func TestHandleStore_AdoptedHandlesAreNotClosed(t *testing.T) {
	app := NewApp()
	tab, _ := storeProject(t, app)
	db, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)
	require.NoError(t, db.Memory().Add(t.Context(), sampleMemoryEntry()))

	op := app.getOpenProject(tab.ID)
	app.autoOpenProjectResources(op)
	require.NotEmpty(t, op.memoryHandle, "a store holding entries yields a handle")

	require.NoError(t, app.memoryHandles.Close(op.memoryHandle))
	_, ok := app.memoryHandles.Get(op.memoryHandle)
	assert.False(t, ok, "the handle is forgotten")

	// The store behind it is untouched: the engine still owns the pool.
	n, cerr := db.Memory().Count(t.Context())
	require.NoError(t, cerr, "closing a borrowed handle must not close the pool")
	assert.Equal(t, 1, n)
}

// Recovering a PROJECT store moves the whole file aside — every subsystem at
// once, the documented trade of merging them — and releases the handle first, so
// the replacement is not written through a pool sitting on the moved inode.
func TestRecoverResource_ProjectStoreReleasesTheHandle(t *testing.T) {
	app := NewApp()
	tab, root := storeProject(t, app)
	db, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)
	require.NotNil(t, db.Raw())

	storePath := filepath.Join(root, project.StateDirName, project.StoreFileName)
	bak, err := app.RecoverResource(storePath)
	require.NoError(t, err)

	assert.Equal(t, storePath+".bak", bak)
	assert.NoFileExists(t, storePath)
	assert.FileExists(t, bak)
	assert.Nil(t, db.Raw(), "the handle is released before the move")

	// And the project opens again, on a fresh handle over a fresh store.
	reopened, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)
	assert.NotSame(t, db, reopened)
	assert.FileExists(t, storePath)
}

// A standalone store keeps the per-file behaviour: it is one content memory or
// one terms store, nothing else rides on it, and no project handle is involved.
func TestRecoverResource_StandaloneStoreMovesOnlyItsOwnFile(t *testing.T) {
	app := NewApp()
	path := filepath.Join(t.TempDir(), "corp.db")
	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o644))

	bak, err := app.RecoverResource(path)
	require.NoError(t, err)
	assert.Equal(t, path+".bak", bak)
	assert.NoFileExists(t, path)
	assert.FileExists(t, bak)
}

// projectStoreRoot must recognize a project's own store and nothing else: a
// named store that happens to be called store.db is still standalone.
func TestProjectStoreRoot(t *testing.T) {
	root, ok := projectStoreRoot(filepath.Join("/p", project.StateDirName, project.StoreFileName))
	assert.True(t, ok)
	assert.Equal(t, "/p", root)

	_, ok = projectStoreRoot(filepath.Join("/p", "stores", project.StoreFileName))
	assert.False(t, ok, "the store must sit in the state directory")

	_, ok = projectStoreRoot(filepath.Join("/p", project.StateDirName, "corp.db"))
	assert.False(t, ok, "a differently-named file in .kapi is not the project store")
}

func sampleMemoryEntry() memory.Entry {
	return memory.Entry{
		ID:          "e1",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello"}}},
			"fr": {{Text: &model.TextRun{Text: "Bonjour"}}},
		},
	}
}
