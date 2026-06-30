package backend

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
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
		Version: "v1",
		Name:    "Coverage",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
		},
		Content: []project.ContentCollection{
			{
				Name: "App",
				Items: []project.ContentItem{
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
	tab, root := newCoverageProject(t, app)

	// Before any extract: no store, so not stale.
	pre, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.False(t, pre.HasData)
	assert.False(t, pre.Stale, "no data yet ⇒ never stale")

	_, err = app.RunExtract(tab.ID)
	require.NoError(t, err)

	layout, err := project.LayoutFor(filepath.Join(root, "project.kapi"))
	require.NoError(t, err)
	stamp := blockStoreVersionStampPath(layout.BlockStorePath())

	// Extract wrote the stamp and status reports fresh.
	_, statErr := os.Stat(stamp)
	require.NoError(t, statErr, "extract should write the version stamp")
	fresh, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.True(t, fresh.HasData)
	assert.False(t, fresh.Stale, "store stamped by the running version ⇒ not stale")

	// A missing stamp (store predates this feature) ⇒ stale.
	require.NoError(t, os.Remove(stamp))
	missing, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.True(t, missing.HasData)
	assert.True(t, missing.Stale, "missing stamp ⇒ stale")

	// A stamp from a different kapi version ⇒ stale.
	require.NoError(t, os.WriteFile(stamp, []byte("v0.0.0-old"), 0o644))
	old, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.True(t, old.HasData)
	assert.True(t, old.Stale, "version mismatch ⇒ stale")
}

func TestRunExtractPopulatesStoreAndCoverage(t *testing.T) {
	app := NewApp()
	tab, root := newCoverageProject(t, app)

	res, err := app.RunExtract(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Files)
	assert.Equal(t, 3, res.Blocks, "three JSON values extracted")
	assert.Empty(t, res.Skipped)

	// The block store file now exists under .kapi/cache/.
	layout, err := project.LayoutFor(filepath.Join(root, "project.kapi"))
	require.NoError(t, err)
	_, statErr := os.Stat(layout.BlockStorePath())
	require.NoError(t, statErr, "blocks.db should exist after extract")

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

	layout, err := project.LayoutFor(filepath.Join(root, "project.kapi"))
	require.NoError(t, err)

	// Simulate a translation pass committing two fr-FR target overlays — the
	// same keys (targets/<locale> by block ID) a real `kapi run` writes.
	store, err := sqlitestore.New(layout.BlockStorePath())
	require.NoError(t, err)
	ctx := context.Background()
	sess, err := store.Begin(ctx)
	require.NoError(t, err)

	var ids []string
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Collection: "App"}) {
		require.NoError(t, berr)
		ids = append(ids, b.ID)
	}
	require.Len(t, ids, 3)

	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/fr-FR", BlockHash: ids[0], Payload: []byte(`{"text":"Bonjour"}`)}))
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/fr-FR", BlockHash: ids[1], Payload: []byte(`{"text":"Au revoir"}`)}))
	require.NoError(t, sess.Commit())
	require.NoError(t, store.Close())

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	require.Len(t, status.Collections, 1)
	coll := status.Collections[0]
	assert.Equal(t, 3, coll.BlockCount)
	assert.Equal(t, 2, coll.Coverage["fr-FR"], "two fr-FR targets committed")
	assert.Equal(t, 0, coll.Coverage["de-DE"], "no de-DE targets yet")
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
		Version:  "v1",
		Name:     "Skip",
		Defaults: project.Defaults{SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"fr-FR"}},
		Content: []project.ContentCollection{
			{Name: "C", Items: []project.ContentItem{
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

// Blocks from daemon-backed readers (okapi-bridge okf_*) arrive without an ID
// or a populated Identity. modelBlockToKLF must still produce a non-empty,
// content-addressed Hash so PutBlock accepts them (re-extract regression).
func TestModelBlockToKLF_EmptyIdentityGetsContentHash(t *testing.T) {
	b := &model.Block{Translatable: true}
	b.SetSourceText("Hello world")

	got := modelBlockToKLF(b)
	require.NotEmpty(t, got.Hash, "hash must be non-empty for PutBlock")
	assert.Equal(t, model.ComputeContentHash("Hello world"), got.Hash)
}

// A populated Identity.ContentHash is preferred over a derived one.
func TestModelBlockToKLF_PrefersIdentityContentHash(t *testing.T) {
	b := &model.Block{ID: "blk-1", Translatable: true, Identity: &model.BlockIdentity{ContentHash: "deadbeef"}}
	b.SetSourceText("ignored")

	got := modelBlockToKLF(b)
	assert.Equal(t, "deadbeef", got.Hash)
	assert.Equal(t, "blk-1", got.ID)
}

// The project block store is opened once and reused across calls (rather than a
// fresh connection pool per call, which let concurrent operations collide on
// blocks.db with "database is locked"). Verify the cached identity, that the
// shared pool serves concurrent sessions without error, and that CloseProject
// releases it.
func TestProjectBlockStoreCachedAndConcurrent(t *testing.T) {
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "test.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: "v1", Name: "Test"}))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)
	op := app.getOpenProject(tab.ID)
	require.NotNil(t, op)

	// sqlitestore.New doesn't create parent dirs — ensure .kapi/cache/ exists.
	storePath, ok := app.projectBlockStorePath(op)
	require.True(t, ok)
	require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o755))

	s1, err := app.projectBlockStore(op)
	require.NoError(t, err)
	s2, err := app.projectBlockStore(op)
	require.NoError(t, err)
	require.True(t, s1 == s2, "projectBlockStore should return the cached instance")

	// Concurrent sessions on the one shared pool must not deadlock or lock.
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sess, err := s1.Begin(context.Background())
			if err != nil {
				errs <- err
				return
			}
			_ = sess.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e, "concurrent block-store session")
	}

	// CloseProject releases the cached store.
	app.CloseProject(tab.ID)
	op.blockStoreMu.Lock()
	assert.Nil(t, op.blockStore, "CloseProject should clear the cached block store")
	op.blockStoreMu.Unlock()
}
