package project_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/project"
)

// claimOrderRecipe declares config/*.yaml for its values at site/web, and the
// YAML files under config/ and notes/ for their comments alone at
// source/comments. The comments-only collection is listed first when
// commentsFirst is set.
func claimOrderRecipe(commentsFirst bool) string {
	const head = `version: v1
name: claim-order
defaults:
  source_language: en
profiles:
  site:
    channels: [web]
  source:
    channels: [comments]
collections:
`
	const comments = `  - name: source-comments
    channel: source/comments
    source_only: true
    content:
      - path: "{config,notes}/**/*.yaml"
        comments:
          only: true
`
	const values = `  - name: site
    channel: site/web
    source_only: true
    content:
      - path: "config/*.yaml"
`
	if commentsFirst {
		return head + comments + values
	}
	return head + values + comments
}

// writeClaimOrderProject writes claimOrderRecipe and its two files into a
// directory with no symlink in its path, and returns the recipe's path.
func writeClaimOrderProject(t *testing.T, recipe string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for name, body := range map[string]string{
		project.RecipeFileName: recipe,
		"config/app.yaml":      "# Greets the reader.\ngreeting: Hello\n",
		"notes/todo.yaml":      "# A note.\nnote: Later\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(body), 0o644))
	}
	return filepath.Join(root, project.RecipeFileName)
}

// resolvedClaim is a resolved file reduced to the collection that claims it and
// whether the claim is on its comments alone.
type resolvedClaim struct {
	collection   string
	commentsOnly bool
}

func resolvedClaims(files []project.ResolvedFile) map[string]resolvedClaim {
	out := make(map[string]resolvedClaim, len(files))
	for _, rf := range files {
		out[filepath.ToSlash(rf.Relative)] = resolvedClaim{rf.Collection, rf.CommentsOnly()}
	}
	return out
}

// An item declared for a file's comments alone never claims the file's values.
// The first item that claims more than the comments extracts them, wherever
// the comments-only item sits in the recipe, and the comments resolve at the
// comments-only item's point. A file only a comments-only item claims still has
// no value to extract.
func TestAValueItemExtractsTheValuesOfAFileACommentsOnlyItemClaims(t *testing.T) {
	want := map[string]resolvedClaim{
		"config/app.yaml": {"site", false},
		"notes/todo.yaml": {"source-comments", true},
	}
	for name, commentsFirst := range map[string]bool{"the comments-only item first": true, "the value item first": false} {
		t.Run(name, func(t *testing.T) {
			recipe := writeClaimOrderProject(t, claimOrderRecipe(commentsFirst))
			proj, err := project.Load(recipe)
			require.NoError(t, err)
			pctx := project.NewProjectContext(proj, recipe)
			reg := contentRegistry(t)

			files, err := pctx.ResolveContent(reg)
			require.NoError(t, err)
			assert.Equal(t, want, resolvedClaims(files), "ResolveContent")
			assert.Equal(t, want, resolvedClaims(pctx.ResolvePaths(reg, []string{"config/app.yaml", "notes/todo.yaml"}, nil)), "ResolvePaths")
			assert.Equal(t, "site", proj.CollectionForPath("config/app.yaml"))
			assert.Equal(t, "source-comments", proj.CollectionForPath("notes/todo.yaml"))

			point := func(path string, comments bool) string {
				t.Helper()
				rc, err := proj.ResolveGovernanceFor(project.GovernancePoint{Path: path, Comments: comments})
				require.NoError(t, err)
				return rc.Ref().String()
			}
			defaults, err := proj.ResolveGovernanceFor(project.GovernancePoint{})
			require.NoError(t, err)
			assert.Equal(t, "site/web", point("config/app.yaml", false), "the values sit at the value item's point")
			assert.Equal(t, "source/comments", point("config/app.yaml", true), "the comments sit at the comments-only item's point")
			assert.Equal(t, defaults.Ref().String(), point("notes/todo.yaml", false))
			assert.Equal(t, "source/comments", point("notes/todo.yaml", true))
			assert.False(t, proj.ClaimsOnlyComments("config/app.yaml", false), "an item claims the file's values")
			assert.True(t, proj.ClaimsOnlyComments("notes/todo.yaml", false))

			store := blockstore.NewMemoryStore()
			t.Cleanup(func() { _ = store.Close() })
			stamper := newStamper()
			stats, err := project.ExtractToBlockStore(context.Background(), reg, pctx, store, stamper, files)
			require.NoError(t, err)
			assert.Equal(t, 1, stats.Files, "config/app.yaml alone holds values")
			assert.Equal(t, 1, stats.Blocks, "the greeting")
			assert.Contains(t, stamper.stamps, "config/app.yaml")
			assert.NotContains(t, stamper.stamps, "notes/todo.yaml", "a file only a comments-only item claims has no value unit")
		})
	}
}
