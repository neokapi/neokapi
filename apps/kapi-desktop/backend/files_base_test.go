package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBasedProject scaffolds KapiMart's shape: every collection declares a
// `base:`, so every effective pattern differs from the path the recipe writes.
func newBasedProject(t *testing.T, app *App) *TabInfo {
	t.Helper()
	root := t.TempDir()

	for _, rel := range []string{
		"web/en/index.md",
		"web/en/about.md",
		"legal/en/terms.txt",
		"legal/en/privacy.json",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(`{"a":"b"}`), 0o644))
	}

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "KapiMart",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"nb-NO"},
		},
		Collections: []project.Collection{
			{
				Name:    "Website",
				Base:    "web",
				Content: []project.ContentItem{{Path: "en/**/*.md"}},
			},
			{
				Name: "Contracts",
				Base: "legal",
				Content: []project.ContentItem{
					{Path: "en/*.txt"},
					{Path: "en/*.json"},
				},
			},
		},
	}
	path := filepath.Join(root, "kapi.yaml")
	require.NoError(t, project.Save(path, proj))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab
}

func TestMatchContentAddressesTheRecipeEntry(t *testing.T) {
	app := NewApp()
	tab := newBasedProject(t, app)

	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)
	require.Len(t, matches, 4)

	byRel := map[string]FileMatch{}
	for _, m := range matches {
		byRel[filepath.ToSlash(m.Relative)] = m
	}

	// The pattern carries the base, which is exactly why a surface cannot join
	// on it against what the recipe declares.
	index := byRel["web/en/index.md"]
	assert.Equal(t, "web/en/**/*.md", filepath.ToSlash(index.Pattern))
	assert.Equal(t, "Website", index.Collection)
	assert.Equal(t, 0, index.CollectionIndex)
	assert.Equal(t, 0, index.ItemIndex)

	// A second collection, and two items within it, are addressed apart.
	assert.Equal(t, 1, byRel["legal/en/terms.txt"].CollectionIndex)
	assert.Equal(t, 0, byRel["legal/en/terms.txt"].ItemIndex)
	assert.Equal(t, 1, byRel["legal/en/privacy.json"].CollectionIndex)
	assert.Equal(t, 1, byRel["legal/en/privacy.json"].ItemIndex)
}

func TestMatchContentIndexAddressesEveryCollection(t *testing.T) {
	app := NewApp()
	tab := newBasedProject(t, app)

	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)

	perCollection := map[int]int{}
	for _, m := range matches {
		perCollection[m.CollectionIndex]++
	}
	// Every declared collection claims files. The join that read the declared
	// path against Pattern found none of these.
	assert.Equal(t, map[int]int{0: 2, 1: 2}, perCollection)
}

func TestResolvedFileIndexTracksTheRecipeOrder(t *testing.T) {
	app := NewApp()
	tab := newBasedProject(t, app)

	op := app.getOpenProject(tab.ID)
	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)

	// The index addresses the row a person edits: the item at that position in
	// the recipe declares the un-based path the file's pattern was built from.
	for _, m := range matches {
		coll := op.Project.Collections[m.CollectionIndex]
		require.Less(t, m.ItemIndex, len(coll.Content))
		declared := coll.Content[m.ItemIndex].Path
		assert.Equal(t, project.JoinBase(coll.Base, declared), filepath.ToSlash(m.Pattern))
	}
}
