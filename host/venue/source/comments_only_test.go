package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// A push sends the values of a project's files to the server. A file declared
// for its comments alone has none, whether the push scans the recipe's content
// or a path named on its own.
func TestScanLocalBlocksLeavesCommentsOnlyItemsOut(t *testing.T) {
	scan := func(t *testing.T, spelled string, paths []string) map[string][]*model.Block {
		t.Helper()
		root := t.TempDir()
		reg := registry.NewFormatRegistry()
		formats.RegisterAll(reg)

		var comments coreproj.ContentComments
		require.NoError(t, yaml.Unmarshal([]byte(spelled), &comments))
		recipe := &bproject.Recipe{
			Defaults: coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
			Collections: []coreproj.Collection{
				{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}},
				{Name: "config", Content: []coreproj.ContentItem{{Path: "config/app.yaml", Comments: comments}}},
			},
		}
		proj, err := bproject.InitProject(root, recipe)
		require.NoError(t, err)
		for rel, body := range map[string]string{
			"locales/en.json": `{"greeting":"Hello"}`,
			"config/app.yaml": "# Greets the reader.\ngreeting: Hello\n",
		} {
			abs := filepath.Join(root, filepath.FromSlash(rel))
			require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
			require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
		}

		conn := NewLocalConnector(&host.App{}, proj, reg)
		t.Cleanup(func() { conn.Close() })
		_, blockMap, err := conn.scanLocalBlocks(context.Background(), paths)
		require.NoError(t, err)
		return blockMap
	}

	for name, paths := range map[string][]string{"the recipe's content": nil, "named paths": {"locales/en.json", "config/app.yaml"}} {
		t.Run(name, func(t *testing.T) {
			only := scan(t, "{only: true}", paths)
			assert.Contains(t, only, "locales/en.json")
			assert.NotContains(t, only, "config/app.yaml")

			beside := scan(t, "true", paths)
			assert.Contains(t, beside, "config/app.yaml", "must fail: comments: true pushes the values")
		})
	}
}
