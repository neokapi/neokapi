package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncProjectStore_ExtractsOnlyWhenTheCacheHasDrifted(t *testing.T) {
	app := NewApp()
	tab, root := newCoverageProject(t, app)
	var events []string
	InjectEventSink(app, func(name string, _ any) { events = append(events, name) })

	first, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	assert.True(t, first.Extracted, "a project with no cache is read")
	assert.Equal(t, 1, first.Files)
	assert.Equal(t, 3, first.Blocks)
	assert.Equal(t, []string{"project:store-syncing", "project:extracted"}, events)

	db, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)
	has, err := db.HasBlocks(context.Background())
	require.NoError(t, err)
	assert.True(t, has)

	events = nil
	again, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	assert.False(t, again.Extracted, "a current cache is left alone")
	assert.Empty(t, events)

	// An edit to a source file makes the cache drift; the next call reads it.
	src := filepath.Join(root, "locales", "en.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello","farewell":"Goodbye","cta":"Buy now","more":"More"}`), 0o644))
	later := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(src, later, later))
	edited, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	assert.True(t, edited.Extracted)
	assert.Equal(t, 4, edited.Blocks)

	plan, err := app.GetConvergePlan(tab.ID)
	require.NoError(t, err)
	assert.False(t, plan.StoreMissing)
	assert.Zero(t, plan.ChangedFiles)
}

func TestSyncProjectStore_LeavesTheCacheToARunningRun(t *testing.T) {
	app := NewApp()
	tab, _ := newCoverageProject(t, app)
	app.runState = newRunner()
	app.runState.running = true

	res, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	assert.False(t, res.Extracted)
	_, ok := app.existingProjectStore(app.getOpenProject(tab.ID))
	assert.False(t, ok, "no store is created while a run holds the project")
}

func TestSyncProjectStore_UnknownTab(t *testing.T) {
	_, err := NewApp().SyncProjectStore("nope")
	require.Error(t, err)
}
