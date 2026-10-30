package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
)

// guideMDX holds JSX elements whose children are block-level markdown: a
// paragraph and a list inside a Callout, and prose inside nested tabs.
const guideMDX = `import { Callout, Tabs, Tab } from "@site/components";

# Guide

Read the steps below before you start.

<Callout type="note">

Keep the **recipe** beside the files it reads.

- Open the project folder.
- Check every file.

</Callout>

<Tabs>
  <Tab label="CLI">

    Run the check in a terminal.

  </Tab>
  <Tab label="Desktop">

    Open the project in Kapi Desktop.

  </Tab>
</Tabs>

That is all there is to it.
`

// blockReading is one block as a reading shows it: its id or key, and its
// text with inline codes as placeholders.
type blockReading struct {
	ID   string
	Text string
}

// TestSyncProjectStore_ReadsAsTheCheckAndTheChangeService: for an HTML file
// and an MDX file with JSX children, the block cache the desktop fills holds
// the blocks `kapi check` reads, with the same ids and texts, and the change
// service names the same blocks in the same order.
func TestSyncProjectStore_ReadsAsTheCheckAndTheChangeService(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	for _, tc := range []struct {
		name, rel string
		recipe    func(t *testing.T) string
	}{
		{"kapimart html", "src/en/email-templates.html", func(t *testing.T) string {
			dir := t.TempDir()
			require.NoError(t, sample.Scaffold("kapimart", dir))
			return filepath.Join(dir, "kapi.yaml")
		}},
		{"mdx with jsx children", "docs/guide.mdx", func(t *testing.T) string {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.mdx"), []byte(guideMDX), 0o644))
			recipe := filepath.Join(dir, "kapi.yaml")
			require.NoError(t, project.Save(recipe, &project.KapiProject{
				Version:     project.CurrentVersion,
				Name:        "Guide",
				Defaults:    project.Defaults{SourceLanguage: "en"},
				Collections: []project.Collection{{Name: "Docs", Path: "docs/*.mdx"}},
			}))
			return recipe
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp()
			t.Cleanup(func() { app.shutdownEngine() })
			tab, err := app.OpenProject(tc.recipe(t))
			require.NoError(t, err)
			t.Cleanup(func() { app.CloseProject(tab.ID) })
			synced, err := app.SyncProjectStore(tab.ID)
			require.NoError(t, err)
			require.True(t, synced.Extracted)

			op := app.getOpenProject(tab.ID)
			rf, err := app.readerSource(tab.ID, op, tc.rel)
			require.NoError(t, err)
			pctx := project.NewProjectContext(op.Project, op.Path)

			// The check's read, as `kapi check` reads the file.
			checker := &host.App{}
			checker.InitRegistries()
			t.Cleanup(checker.Shutdown)
			checked, err := checker.ReadBlocksForCheck(t.Context(), rf.Path, rf.Format,
				pctx.FormatConfigFor(rf.Format, rf.Item), string(pctx.SourceLocale))
			require.NoError(t, err)
			require.NotEmpty(t, checked)
			byID := map[string]string{}
			var byKey []blockReading
			for _, b := range checked {
				byID[b.ID] = model.RunsEditText(b.SourceRuns())
				byKey = append(byKey, blockReading{change.BlockKey(b), model.RunsEditText(b.SourceRuns())})
			}

			assert.Equal(t, byID, cachedEditTexts(t, app, tab.ID, tc.rel), "the block cache holds the blocks the check reads")
			assert.Equal(t, byKey, changeServiceReading(t, app, tab.ID, tc.rel), "the change service names the blocks the check reads")
		})
	}
}

// cachedEditTexts are the block cache's translatable blocks for one file, by
// id, each with its text in placeholder form.
func cachedEditTexts(t *testing.T, app *App, tabID, rel string) map[string]string {
	t.Helper()
	db, ok := app.existingProjectStore(app.getOpenProject(tabID))
	require.True(t, ok, "the store sync wrote the block cache")
	sess, err := db.BlocksAutocommit().Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()
	translatable := true
	out := map[string]string{}
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Translatable: &translatable}) {
		require.NoError(t, berr)
		if filepath.ToSlash(b.Properties.File) == rel {
			out[b.ID] = model.RunsEditText(b.SourceRuns())
		}
	}
	return out
}

// changeServiceReading is the file's blocks as the desktop's change service
// reads them, in file order.
func changeServiceReading(t *testing.T, app *App, tabID, rel string) []blockReading {
	t.Helper()
	svc, err := app.changeServiceFor(t.Context(), tabID, nil)
	require.NoError(t, err)
	var out []blockReading
	req := change.ReadRequest{Doc: rel, Limit: change.MaxReadLimit}
	for {
		page, err := svc.Read(t.Context(), req)
		require.NoError(t, err)
		for _, b := range page.Blocks {
			out = append(out, blockReading{b.Ref.Block, b.Text})
		}
		if page.Next == "" {
			return out
		}
		req.Cursor = page.Next
	}
}
