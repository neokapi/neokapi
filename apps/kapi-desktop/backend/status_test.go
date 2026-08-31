package backend

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCoverageProject scaffolds a tiny native-format project (one JSON source
// collection, two target locales) on disk and opens it. Returns the tab and
// the project root dir.
func newCoverageProject(t *testing.T, app *App) (*TabInfo, string) {
	t.Helper()
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "en.json"),
		[]byte(`{"greeting":"Hello","farewell":"Goodbye","cta":"Buy now"}`),
		0o644,
	))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Coverage",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
		},
		Collections: []project.Collection{
			{
				Name: "App",
				Content: []project.ContentItem{
					{Path: "locales/en.json", Target: "locales/{lang}.json"},
				},
			},
		},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
	}
	path := filepath.Join(root, "project.kapi")
	require.NoError(t, project.Save(path, proj))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab, root
}

func TestGetProjectStatusNoDataYet(t *testing.T) {
	app := NewApp()
	tab, _ := newCoverageProject(t, app)

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, "Coverage", status.ProjectName)
	assert.False(t, status.HasData, "no extraction yet → HasData false")
	require.Len(t, status.Collections, 1)

	coll := status.Collections[0]
	assert.Equal(t, "App", coll.Name)
	assert.Equal(t, 0, coll.BlockCount)
	assert.ElementsMatch(t, []string{"fr-FR", "de-DE"}, coll.TargetLanguages)
	assert.Equal(t, 0, coll.Coverage["fr-FR"])
	assert.Equal(t, 0, coll.Coverage["de-DE"])
}

func TestGetProjectStatusUnknownTab(t *testing.T) {
	app := NewApp()
	_, err := app.GetProjectStatus("nope")
	assert.Error(t, err)
}

// A fresh extract stamps the store with the running kapi version, so status is
// not stale; the "no data yet" shell is never stale (nothing to mismatch).
func TestGetProjectStatusStaleVersionStamp(t *testing.T) {
	app := NewApp()
	tab, _ := newCoverageProject(t, app)

	// Before any extract: nothing extracted, so not stale.
	pre, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.False(t, pre.HasData)
	assert.False(t, pre.Stale, "no data yet ⇒ never stale")

	_, err = app.RunExtract(tab.ID)
	require.NoError(t, err)

	// The stamp lives in the store's own metadata table now, not in a sidecar
	// named after a database file — a filename-keyed stamp says nothing once the
	// block cache is one schema inside a shared store.
	ctx := context.Background()
	db, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)

	// Extract wrote the stamp and status reports fresh.
	stamped, ok, err := db.Meta(ctx, projectdb.MetaBlocksSchemaVersion)
	require.NoError(t, err)
	require.True(t, ok, "extract should record the schema version")
	assert.Equal(t, project.BlockStoreSchemaVersion, stamped)
	fresh, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.True(t, fresh.HasData)
	assert.False(t, fresh.Stale, "cache stamped by the running version ⇒ not stale")

	// A stamp from a different kapi version ⇒ stale.
	require.NoError(t, db.PutMeta(ctx, projectdb.MetaBlocksSchemaVersion, "v0.0.0-old"))
	old, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.True(t, old.HasData)
	assert.True(t, old.Stale, "version mismatch ⇒ stale")

	// A missing stamp (a cache written before the stamp existed) ⇒ stale.
	_, err = db.Raw().ExecContext(ctx, `DELETE FROM store_meta WHERE key = ?`, projectdb.MetaBlocksSchemaVersion)
	require.NoError(t, err)
	missing, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.True(t, missing.HasData)
	assert.True(t, missing.Stale, "missing stamp ⇒ stale")
}

func TestRunExtractPopulatesStoreAndCoverage(t *testing.T) {
	app := NewApp()
	tab, root := newCoverageProject(t, app)

	res, err := app.RunExtract(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Files)
	assert.Equal(t, 3, res.Blocks, "three JSON values extracted")
	assert.Empty(t, res.Skipped)

	// The blocks landed in the project's one store. Presence is a row question:
	// the file exists from the store's first open, so its existence would say
	// only that some command ran here.
	layout, err := project.LayoutFor(filepath.Join(root, "project.kapi"))
	require.NoError(t, err)
	require.FileExists(t, layout.StorePath())
	db, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)
	extracted, err := db.HasBlocks(context.Background())
	require.NoError(t, err)
	assert.True(t, extracted, "the block cache holds blocks after extract")

	// Status now reports real totals, zero translated.
	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.True(t, status.HasData)
	require.Len(t, status.Collections, 1)
	coll := status.Collections[0]
	assert.Equal(t, 3, coll.BlockCount)
	assert.Equal(t, 0, coll.Coverage["fr-FR"])
	assert.Equal(t, 0, coll.Coverage["de-DE"])
}

func TestRunExtractThenCoverageWithTargets(t *testing.T) {
	app := NewApp()
	tab, root := newCoverageProject(t, app)

	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)

	// A translation pass materializes the target beside its source, which is
	// what a run writes and what the coverage derivation reads. Two of the
	// three units carry French; the third has no target value at all, which is
	// what "untranslated" looks like on disk — a target repeating its source is
	// a translation that happens to match, and counts.
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "fr-FR.json"),
		[]byte(`{"greeting":"Bonjour","farewell":"Au revoir"}`),
		0o644,
	))

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	require.Len(t, status.Collections, 1)
	coll := status.Collections[0]
	assert.Equal(t, 3, coll.BlockCount)
	assert.Equal(t, 2, coll.Coverage["fr-FR"], "two fr-FR units carry a translation")
	assert.Equal(t, 0, coll.Coverage["de-DE"], "no de-DE target yet")
}

// A target committed in the working tree counts before any run has carried it
// into the block store: the file is the content, and a reading that waited for
// the store reported a translated project as untouched (#2265).
func TestCoverageCountsACommittedTargetWithoutARun(t *testing.T) {
	app := NewApp()
	tab, root := newCoverageProject(t, app)

	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "fr-FR.json"),
		[]byte(`{"greeting":"Bonjour"}`),
		0o644,
	))
	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	require.Len(t, status.Collections, 1)
	assert.Equal(t, 1, status.Collections[0].Coverage["fr-FR"])
}

func TestRunExtractUnknownTab(t *testing.T) {
	app := NewApp()
	_, err := app.RunExtract("nope")
	assert.Error(t, err)
}

func TestRunExtractSkipsUnreadableFiles(t *testing.T) {
	app := NewApp()
	root := t.TempDir()

	// A .json that extracts fine plus an unknown-extension file the project
	// declares but no reader can handle.
	require.NoError(t, os.WriteFile(filepath.Join(root, "ok.json"), []byte(`{"a":"A"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "mystery.zzz"), []byte("???"), 0o644))

	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "Skip",
		Defaults: project.Defaults{SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"fr-FR"}},
		Collections: []project.Collection{
			{Name: "C", Content: []project.ContentItem{
				{Path: "ok.json"},
				{Path: "mystery.zzz"},
			}},
		},
	}
	path := filepath.Join(root, "project.kapi")
	require.NoError(t, project.Save(path, proj))
	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	res, err := app.RunExtract(tab.ID)
	require.NoError(t, err, "extraction is best-effort, must not fail on one bad file")
	assert.Equal(t, 1, res.Files)
	assert.Equal(t, 1, res.Blocks)
	require.Len(t, res.Skipped, 1)
	assert.Equal(t, "mystery.zzz", res.Skipped[0].Path)
}

// One handle per project, and it serves concurrent readers. A tab-cached block
// store used to be the desktop's answer to "database is locked" — opening a
// fresh pool per call let two operations collide on the file. The merged store
// answers it once, for every subsystem, on the engine: one pool, whose writers
// this process can order. Verify the identity, the concurrency, and that
// CloseProject hands the project back.
func TestProjectStoreIsOneHandleAndServesConcurrentReaders(t *testing.T) {
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "test.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: project.CurrentVersion, Name: "Test"}))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)
	op := app.getOpenProject(tab.ID)
	require.NotNil(t, op)

	db1, err := app.projectStore(op)
	require.NoError(t, err)
	db2, err := app.projectStore(op)
	require.NoError(t, err)
	require.Same(t, db1, db2, "the project store is memoized per root")

	// The autocommit block store is what every read-side surface uses; concurrent
	// sessions on the one pool must not deadlock or report a locked database.
	store := db1.BlocksAutocommit()
	require.NotNil(t, store)
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for range 24 {
		wg.Go(func() {
			sess, err := store.Begin(context.Background())
			if err != nil {
				errs <- err
				return
			}
			_ = sess.Close()
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e, "concurrent block-store session")
	}

	// Closing the tab releases the store, so the directory is free to be moved
	// and the next open builds a fresh handle rather than one on the old inode.
	app.CloseProject(tab.ID)
	assert.Nil(t, db1.Raw(), "CloseProject releases the project store")
}

// Both surfaces that only look at a project — the status panel and the plan
// preview — must not bring its store into being. Opening is what creates the
// file, and a read that created one would leave the next real command a store it
// never wrote.
func TestReadOnlySurfacesDoNotCreateTheStore(t *testing.T) {
	app := NewApp()
	tab, root := newCoverageProject(t, app)

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.False(t, status.HasData)

	plan, err := app.GetConvergePlan(tab.ID)
	require.NoError(t, err)
	assert.True(t, plan.StoreMissing, "a project with no store has drifted from nothing")

	layout, err := project.LayoutFor(filepath.Join(root, "project.kapi"))
	require.NoError(t, err)
	assert.NoFileExists(t, layout.StorePath(), "looking at a project must not write to it")
}
