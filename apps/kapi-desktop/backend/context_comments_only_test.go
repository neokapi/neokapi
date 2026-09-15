package backend

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// The context panes answer for a file's content: an item that claims only the
// file's comments governs none of it, so the point is the project's default one.
// That holds for an item declared `comments: {only: true}`, and for one that
// declares the comments of a file no reader parses.
func TestContextPointResolvesContentPastACommentsOnlyItem(t *testing.T) {
	isolateCheckPlugins(t)
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("config/app.yaml", "# The greeting.\ngreeting: Hello\n")
	write("code/parse.go", "package code\n\n// Parse reads the input.\nfunc Parse() {}\n")
	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "Acme",
		Defaults: project.Defaults{SourceLanguage: "en-US"},
		Profiles: map[string]project.Profile{"source": {Channels: []project.Channel{{ID: "comments"}}}},
		Collections: []project.Collection{
			{Name: "config-comments", Channel: "source/comments", Content: []project.ContentItem{
				{Path: "config/*.yaml", Comments: project.ContentComments{Declared: true, Only: true}},
			}},
			{Name: "code-comments", Channel: "source/comments", Content: []project.ContentItem{
				{Path: "code/*.go", Comments: project.ContentComments{Declared: true}},
			}},
		},
	}
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, project.Save(recipe, proj))

	_, content, err := contextPoint(proj, "", "config/app.yaml", false, false, time.Now())
	require.NoError(t, err)
	defaults, err := proj.ResolveGovernanceFor(project.GovernancePoint{})
	require.NoError(t, err)
	assert.Equal(t, defaults.Ref().String(), content.Ref().String())

	app := NewApp()
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	for _, rel := range []string{"config/app.yaml", "code/parse.go"} {
		res, err := app.ContextGoverns(tab.ID, "", rel, 0)
		require.NoError(t, err)
		assert.True(t, res.Point.Default, "%s: no item governs the file's content", rel)
		assert.Empty(t, res.Point.Profile, rel)
	}
}

// The Checks panel hands the files whose items declare their comments to the
// project's check as declared content, so a file declared for its comments alone
// has its comment read and none of its values.
func TestChecksPanelReadsOnlyTheCommentsOfACommentsOnlyFile(t *testing.T) {
	isolateCheckPlugins(t)
	app := NewApp()
	tabID, _ := commentsOnlyTab(t, app, "{only: true}")
	op := app.getOpenProject(tabID)
	capp := app.checksCLI()
	capp.InitRegistries()
	resolved, err := project.NewProjectContext(op.Project, op.Path).ResolveContent(capp.FormatReg)
	require.NoError(t, err)
	files := slices.DeleteFunc(resolved, func(rf project.ResolvedFile) bool { return !rf.CommentsOnly() })
	require.Len(t, files, 1)

	out, err := checkComments(t.Context(), capp, op.Path, files, "en-US")
	require.NoError(t, err)
	assert.Equal(t, 1, out.blocks, "the one comment, and no value")
}
