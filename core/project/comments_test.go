package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A content item with `comments: true` over files no reader covers is claimed
// for its comments alone; the same item over a file a reader covers is not,
// because that file also has content a run converges.
func TestCommentsOnlyNeedsTheFlagAndNoReader(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: comments
defaults:
  source_language: en
collections:
  - name: code
    channel: acme/engineering
    source_only: true
    content:
      - path: "src/*.go"
        comments: true
      - path: "config/*.yaml"
        comments: true
  - name: undeclared
    content:
      - path: "other/*.go"
profiles:
  acme:
    channels: [engineering]
`), 0o644))
	for _, f := range []string{"src/main.go", "config/app.yaml", "other/x.go"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644))
	}

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	require.True(t, proj.Collections[0].Content[0].Comments.Declared, "the key loads")

	resolved, err := project.NewProjectContext(proj, recipe).ResolveContent(contentRegistry(t))
	require.NoError(t, err)
	only := map[string]bool{}
	for _, rf := range resolved {
		only[filepath.ToSlash(rf.Relative)] = rf.CommentsOnly()
	}
	assert.Equal(t, map[string]bool{
		"src/main.go":     true,
		"config/app.yaml": false,
		"other/x.go":      false,
	}, only)
}

// The key round-trips through Save, so a recipe edited by kapi keeps it.
func TestCommentsKeySurvivesSave(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, project.RecipeFileName)
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "comments",
		Collections: []project.Collection{{
			Name:       "code",
			SourceOnly: true,
			Content:    []project.ContentItem{{Path: "*.go", Comments: project.ContentComments{Declared: true}}},
		}},
	}
	require.NoError(t, project.Save(recipe, proj))
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(data), "comments: true")
	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	assert.True(t, loaded.Collections[0].Content[0].Comments.Declared)
}
