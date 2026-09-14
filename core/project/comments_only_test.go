package project_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commentsOnlyRecipe declares a YAML file with `comments:` spelled as given,
// beside a JSON file whose values are content.
func commentsOnlyRecipe(spelled string) string {
	return `version: v1
name: comments-only
defaults:
  source_language: en
profiles:
  source:
    channels: [comments]
collections:
  - name: ui
    source_only: true
    content:
      - path: "src/*.json"
  - name: config
    content:
      - path: "config/*.yaml"
        comments: ` + spelled + `
`
}

// writeCommentsOnlyFiles writes the files commentsOnlyRecipe declares beside
// the recipe.
func writeCommentsOnlyFiles(t *testing.T, recipe string) {
	t.Helper()
	root := filepath.Dir(recipe)
	for name, body := range map[string]string{
		"src/a.json":      `{"greeting":"Hello"}`,
		"config/app.yaml": "# Greets the reader.\ngreeting: Hello\n",
		"code/main.go":    "package main\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(body), 0o644))
	}
}

// resolveCommentsOnly loads a recipe and resolves its content.
func resolveCommentsOnly(t *testing.T, recipe string) (*project.KapiProject, []project.ResolvedFile) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	resolved, err := project.NewProjectContext(proj, recipe).ResolveContent(contentRegistry(t))
	require.NoError(t, err)
	return proj, resolved
}

// `comments: {only: true}` claims a file a reader parses for its comments
// alone. `comments: true` over the same file keeps its values as content, and
// over a file no reader covers it already claims the comments alone.
func TestCommentsOnlyClaimsAReaderParsedFileForItsComments(t *testing.T) {
	recipe := writeRecipe(t, commentsOnlyRecipe("\n          only: true\n          channel: source/comments")+`  - name: code
    source_only: true
    content:
      - path: "code/*.go"
        comments: true
  - name: docs
    source_only: true
    content:
      - path: "docs/*.yaml"
        comments: true
`)
	writeCommentsOnlyFiles(t, recipe)
	require.NoError(t, os.MkdirAll(filepath.Join(filepath.Dir(recipe), "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(recipe), "docs", "guide.yaml"), []byte("# Guide.\ntitle: Guide\n"), 0o644))

	proj, resolved := resolveCommentsOnly(t, recipe)
	only := map[string]bool{}
	for _, rf := range resolved {
		only[filepath.ToSlash(rf.Relative)] = rf.CommentsOnly()
	}
	assert.Equal(t, map[string]bool{
		"src/a.json":      false,
		"config/app.yaml": true,
		"code/main.go":    true,
		"docs/guide.yaml": false,
	}, only)
	assert.Equal(t, "source/comments", proj.Collections[1].Content[0].Comments.Channel, "the channel loads beside only")
}

// A recipe kapi saves keeps the declaration as written.
func TestCommentsOnlySurvivesSave(t *testing.T) {
	recipe := writeRecipe(t, commentsOnlyRecipe("\n          only: true"))
	writeCommentsOnlyFiles(t, recipe)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Name = "renamed"
	require.NoError(t, project.Save(recipe, proj))

	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(data), "only: true")
	_, resolved := resolveCommentsOnly(t, recipe)
	for _, rf := range resolved {
		assert.Equal(t, filepath.ToSlash(rf.Relative) == "config/app.yaml", rf.CommentsOnly(), rf.Relative)
	}
}

// An item declared for its comments alone has no values, so a key that only
// shapes, redacts or delivers values is a contradiction the recipe cannot load
// with. The format's name stays: it picks the comment provider.
func TestCommentsOnlyRejectsWhatOnlyValuesUse(t *testing.T) {
	const only = "\n          only: true\n"
	for name, tc := range map[string]struct{ item, want string }{
		"a target": {
			item: `        target: "config/{lang}/app.yaml"`,
			want: `collections[1].content[0]: comments.only is set, so the item cannot have a target (found "config/{lang}/app.yaml")`,
		},
		"target languages": {
			item: `        target_languages: [nb]`,
			want: "collections[1].content[0]: comments.only is set, so the item cannot have target_languages (found [nb])",
		},
		"a redaction": {
			item: "        redaction:\n          enabled: true",
			want: "collections[1].content[0]: comments.only is set, so the item has no values to redact",
		},
		"a reader config": {
			item: "        format:\n          name: yaml\n          config:\n            keyPathPatterns: [\"greeting\"]",
			want: "collections[1].content[0]: comments.only is set, so format.config has no values to configure",
		},
		"a reader preset": {
			item: "        format:\n          name: yaml\n          preset: strings",
			want: "collections[1].content[0]: comments.only is set, so format.preset has no values to configure",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := project.Load(writeRecipe(t, commentsOnlyRecipe(only+tc.item)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	t.Run("must fail: the same keys load beside comments: true", func(t *testing.T) {
		for _, item := range []string{`        target: "config/{lang}/app.yaml"`, "        format:\n          name: yaml\n          preset: strings"} {
			_, err := project.Load(writeRecipe(t, commentsOnlyRecipe("true\n"+item)))
			assert.NoError(t, err, item)
		}
	})

	t.Run("a format name loads beside only", func(t *testing.T) {
		recipe := writeRecipe(t, commentsOnlyRecipe(only+"        format: yaml"))
		writeCommentsOnlyFiles(t, recipe)
		_, resolved := resolveCommentsOnly(t, recipe)
		i := slices.IndexFunc(resolved, func(rf project.ResolvedFile) bool { return rf.Format == "yaml" })
		require.GreaterOrEqual(t, i, 0)
		assert.True(t, resolved[i].CommentsOnly())
	})

	t.Run("only: false declares the comments beside the values", func(t *testing.T) {
		recipe := writeRecipe(t, commentsOnlyRecipe("\n          only: false"))
		writeCommentsOnlyFiles(t, recipe)
		proj, resolved := resolveCommentsOnly(t, recipe)
		assert.True(t, proj.Collections[1].Content[0].Comments.Declared)
		for _, rf := range resolved {
			assert.False(t, rf.CommentsOnly(), rf.Relative)
		}
	})
}

// Extraction reads the values of a project's files into the block store. A
// file declared for its comments alone gives it no block and no drift stamp,
// and drift detection never reports that file as changed.
func TestExtractToBlockStoreLeavesCommentsOnlyFilesOut(t *testing.T) {
	extract := func(t *testing.T, spelled string) (project.ExtractStats, *recordingStamper, []project.ResolvedFile) {
		t.Helper()
		real, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		recipe := filepath.Join(real, project.RecipeFileName)
		require.NoError(t, os.WriteFile(recipe, []byte(commentsOnlyRecipe(spelled)), 0o644))
		writeCommentsOnlyFiles(t, recipe)
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		pctx := project.NewProjectContext(proj, recipe)
		reg := contentRegistry(t)
		files, err := pctx.ResolveContent(reg)
		require.NoError(t, err)
		require.Len(t, files, 2)

		store := blockstore.NewMemoryStore()
		t.Cleanup(func() { _ = store.Close() })
		stamper := newStamper()
		stats, err := project.ExtractToBlockStore(context.Background(), reg, pctx, store, stamper, files)
		require.NoError(t, err)
		return stats, stamper, files
	}

	t.Run("a comments-only file is never extracted", func(t *testing.T) {
		stats, stamper, files := extract(t, "{only: true}")
		assert.Equal(t, 1, stats.Files, "the JSON file alone")
		assert.Equal(t, 1, stats.Blocks)
		assert.Empty(t, stats.Skipped, "a file with no values to extract is not a failed extraction")
		assert.NotContains(t, stamper.stamps, "config/app.yaml")
		changed, removed := project.CompareSourceStamps(stamper.stamps, files)
		assert.Empty(t, changed, "a file whose values are no content never drifts")
		assert.Empty(t, removed)
	})

	t.Run("must fail: comments: true extracts the values", func(t *testing.T) {
		stats, stamper, _ := extract(t, "true")
		assert.Equal(t, 2, stats.Files)
		assert.Equal(t, 2, stats.Blocks)
		assert.Contains(t, stamper.stamps, "config/app.yaml")
	})
}
