package source

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// claimOrderConnector is a project whose config/*.yaml item reads the greeting
// alone, and whose comments-only items claim the YAML files under config/ and
// notes/. The comments-only collection is listed first when commentsFirst is
// set.
func claimOrderConnector(t *testing.T, commentsFirst bool) *BowrainSourceConnector {
	t.Helper()
	only := coreproj.ContentComments{Declared: true, Only: true}
	collections := []coreproj.Collection{
		{Name: "config", Content: []coreproj.ContentItem{{
			Path:   "config/*.yaml",
			Format: &coreproj.FormatSpec{Name: "yaml", Config: map[string]any{"keyPathPatterns": []any{"greeting"}}},
		}}},
		{Name: "comments", Content: []coreproj.ContentItem{
			{Path: "config/**/*.yaml", Comments: only},
			{Path: "notes/*.yaml", Comments: only},
		}},
	}
	if commentsFirst {
		slices.Reverse(collections)
	}
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults:    coreproj.Defaults{SourceLanguage: "en"},
		Collections: collections,
	})
	require.NoError(t, err)
	for rel, body := range map[string]string{
		"config/app.yaml": "# Greets the reader.\ngreeting: Hello\nfarewell: Goodbye\n",
		"notes/todo.yaml": "# A note.\nnote: Later\n",
	} {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
	}
	conn := NewLocalConnector(&host.App{}, proj, reg)
	t.Cleanup(func() { conn.Close() })
	return conn
}

// A push sends the values of a file that a comments-only item also claims,
// read under the value item's format configuration, whichever item the recipe
// lists first. Its scope carries the value item's pattern and no comments-only
// pattern, and a named file with values stays in it.
func TestPushSendsTheValuesOfAFileAlsoClaimedForItsComments(t *testing.T) {
	for name, commentsFirst := range map[string]bool{"the comments-only item first": true, "the value item first": false} {
		t.Run(name, func(t *testing.T) {
			conn := claimOrderConnector(t, commentsFirst)
			for scan, paths := range map[string][]string{"the recipe's content": nil, "named paths": {"config/app.yaml", "notes/todo.yaml"}} {
				_, blocks, err := conn.scanLocalBlocks(context.Background(), paths)
				require.NoError(t, err, scan)
				assert.Len(t, blocks["config/app.yaml"], 1, "%s: the greeting, read under the value item's configuration", scan)
				assert.NotContains(t, blocks, "notes/todo.yaml", "%s: a file only a comments-only item claims holds no value", scan)
			}

			scope := []string(conn.pushScope(nil))
			assert.Contains(t, scope, "config/*.yaml")
			assert.NotContains(t, scope, "config/**/*.yaml", "a comments-only item declares no pattern")
			assert.NotContains(t, scope, "notes/*.yaml", "a comments-only item declares no pattern")

			named := []string(conn.pushScope([]string{"config/app.yaml", "notes/todo.yaml"}))
			assert.Contains(t, named, "config/app.yaml", "a named file with values stays in scope")
			assert.NotContains(t, named, "notes/todo.yaml")
		})
	}
}
