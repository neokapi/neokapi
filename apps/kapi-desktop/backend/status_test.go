package backend

import (
	"context"
	"os"
	"path/filepath"
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
