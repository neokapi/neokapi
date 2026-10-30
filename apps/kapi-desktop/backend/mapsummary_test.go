package backend

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
)

// TestMapSummary_KapiMart: the Map of the bundled sample counts what the other
// surfaces count. Its files are the recipe's content files, each on disk. Its
// blocks are the block cache's, as the status counts them. Its findings are
// the last whole-project checks run's, file by file. The suggestions an
// assistant recorded are the rows awaiting a decision, with their evidence
// counted.
func TestMapSummary_KapiMart(t *testing.T) {
	// The sample reads its context into the workspace the data directory
	// holds, so the app and the scaffold share one of their own.
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	app := NewApp()
	t.Cleanup(func() { app.shutdownEngine() })
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))
	recipe := filepath.Join(dir, "kapi.yaml")
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	// Before the store sync and any check: the files are known, nothing has
	// been read or checked in this session.
	before, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	assert.Empty(t, before.ReadAt)
	assert.Nil(t, before.Findings)
	assert.Empty(t, before.CheckedAt)
	assert.Equal(t, []string{"de", "fr", "ja", "nb", "ar"}, before.TargetLanguages)

	_, err = app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	res, err := app.RunChecks(tab.ID, ProjectFilter{})
	require.NoError(t, err)

	engine := agentProcess(t, app)
	quickcast := agentSuggest(t, engine, recipe, "sess-1", "Quick cast", "Quickcast", "site/index.html", "Try Quick cast today.")
	agentSuggest(t, engine, recipe, "sess-1", "business", "studio", "site/about.html", "Our business plan.")

	got, err := app.MapSummary(tab.ID)
	require.NoError(t, err)

	// Files: one row per resolved content file, each one a file the project
	// directory lists.
	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)
	declared := map[string]bool{}
	for _, m := range matches {
		declared[filepath.ToSlash(m.Relative)] = true
	}
	assert.Equal(t, len(declared), got.Files)
	require.Len(t, got.FileRows, got.Files)
	listed, err := app.ListProjectFiles(tab.ID)
	require.NoError(t, err)
	onDisk := map[string]bool{}
	for _, f := range listed {
		onDisk[f.Path] = true
	}
	for _, row := range got.FileRows {
		assert.Truef(t, declared[row.Relative], "%s is a declared content file", row.Relative)
		assert.Truef(t, onDisk[row.Path], "%s is listed in the project directory", row.Relative)
		assert.NotEmpty(t, row.Format, row.Relative)
	}

	// Blocks: the block cache's, as the status counts them collection by
	// collection.
	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	statusBlocks := 0
	for _, c := range status.Collections {
		statusBlocks += c.BlockCount
	}
	assert.Positive(t, got.Blocks)
	assert.Equal(t, statusBlocks, got.Blocks)
	rowBlocks := 0
	for _, row := range got.FileRows {
		rowBlocks += row.Blocks
	}
	assert.Equal(t, got.Blocks, rowBlocks)
	assert.Positive(t, got.ReadFiles)
	assert.NotEmpty(t, got.ReadAt, "the store sync's read is recorded")

	// Findings: the checks run's, in total and per file.
	wantByFile := map[string]int{}
	wantTotal := 0
	for _, f := range res.Files {
		wantByFile[f.Path] += len(f.Findings)
		wantTotal += len(f.Findings)
	}
	require.NotNil(t, got.Findings)
	assert.Equal(t, wantTotal, *got.Findings)
	assert.NotEmpty(t, got.CheckedAt)
	for _, row := range got.FileRows {
		assert.Equalf(t, wantByFile[row.Path], row.Findings, "findings in %s", row.Relative)
	}
	assert.True(t, sort.SliceIsSorted(got.FileRows, func(i, j int) bool {
		a, b := got.FileRows[i], got.FileRows[j]
		if a.Findings != b.Findings {
			return a.Findings > b.Findings
		}
		return a.Relative < b.Relative
	}), "most findings first, then by path")

	// Rules: the sample's context is imported into its stores, so it holds
	// rules before anybody keeps one.
	assert.Positive(t, got.Rules)

	// Suggestions: both of the assistant's, with the evidence each names.
	assert.Equal(t, 2, got.Suggested)
	assert.Equal(t, 2, got.Awaiting)
	require.Len(t, got.Suggestions, 2)
	ids := map[string]MapSuggestionDTO{}
	for _, s := range got.Suggestions {
		assert.Equal(t, "suggested", s.Group)
		ids[s.Item.ID] = s
	}
	require.Contains(t, ids, quickcast.ID)
	assert.Equal(t, 1, ids[quickcast.ID].Blocks)
	assert.Equal(t, 1, ids[quickcast.ID].Files)
	assert.NotEmpty(t, got.ProjectKey)

	// A narrowed checks run leaves the Map's counts where the whole-project
	// run put them.
	_, err = app.RunChecks(tab.ID, ProjectFilter{Glob: "nothing/**"})
	require.NoError(t, err)
	again, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, again.Findings)
	assert.Equal(t, wantTotal, *again.Findings)
}

// TestMapSummary_EmptyProject: a project with no content files answers with
// zeros and no error, and creates no project store by being read.
func TestMapSummary_EmptyProject(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := t.TempDir()
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version:  project.CurrentVersion,
		ID:       project.NewID(),
		Name:     "Empty",
		Defaults: project.Defaults{SourceLanguage: "en-US"},
	}))
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	_, statErr := os.Stat(project.LayoutAt(dir).StorePath())
	require.True(t, os.IsNotExist(statErr), "precondition: opening the project creates no store")

	got, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	assert.Zero(t, got.Files)
	assert.Zero(t, got.Blocks)
	assert.Zero(t, got.Rules)
	assert.Nil(t, got.Findings)
	assert.Zero(t, got.Awaiting)
	assert.Empty(t, got.FileRows)
	assert.Empty(t, got.Suggestions)
	assert.Empty(t, got.TargetLanguages, "a source-only project declares no target")
	_, statErr = os.Stat(project.LayoutAt(dir).StorePath())
	assert.True(t, os.IsNotExist(statErr), "reading the Map creates no store")
}

// TestMapSummary_SourceOnlyAndTargeted: the Map reports the recipe's target
// languages, canonical, and none for a source-only project; the counts do not
// depend on them.
func TestMapSummary_SourceOnlyAndTargeted(t *testing.T) {
	app := NewApp()
	tab, _ := newCoverageProject(t, app)
	_, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)

	targeted, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"fr-FR", "de-DE"}, targeted.TargetLanguages)
	assert.Equal(t, 1, targeted.Files)
	assert.Equal(t, 3, targeted.Blocks)
	require.Len(t, targeted.FileRows, 1)
	assert.Equal(t, "locales/en.json", targeted.FileRows[0].Relative)
	assert.Equal(t, 3, targeted.FileRows[0].Blocks)

	op := app.getOpenProject(tab.ID)
	app.mu.Lock()
	op.Project.Defaults.TargetLanguages = nil
	app.mu.Unlock()
	sourceOnly, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	assert.Empty(t, sourceOnly.TargetLanguages)
	assert.Equal(t, targeted.Files, sourceOnly.Files)
	assert.Equal(t, targeted.Blocks, sourceOnly.Blocks)
}

func TestMapSummary_UnknownTab(t *testing.T) {
	_, err := NewApp().MapSummary("nope")
	require.Error(t, err)
}
