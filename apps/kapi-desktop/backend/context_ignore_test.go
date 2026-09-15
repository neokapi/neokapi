package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// The context panes answer for a file the project's ignore rules match the way
// `kapi context` does: no collection's point governs it, so the point is the
// project's default one. A sibling the rules leave alone keeps its collection's
// point.
func TestContextPanesGiveAnIgnoredFileTheDefaultPoint(t *testing.T) {
	isolateCheckPlugins(t)
	root := t.TempDir()
	for rel, body := range map[string]string{
		"config/app.yaml":   "# The greeting.\ngreeting: Hello\n",
		"config/other.yaml": "# The farewell.\nfarewell: Goodbye\n",
		".kapiignore":       "config/app.yaml\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "Acme",
		Defaults: project.Defaults{SourceLanguage: "en-US"},
		Profiles: map[string]project.Profile{"site": {Channels: []project.Channel{{ID: "web"}}}},
		Collections: []project.Collection{
			{Name: "config", Channel: "site/web", Content: []project.ContentItem{{Path: "config/*.yaml"}}},
		},
	}))

	app := NewApp()
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	for rel, wantDefault := range map[string]bool{"config/app.yaml": true, "config/other.yaml": false} {
		governs, err := app.ContextGoverns(tab.ID, "", rel, 0)
		require.NoError(t, err)
		assert.Equal(t, wantDefault, governs.Point.Default, "ContextGoverns %s", rel)
		lives, err := app.ContextLives(tab.ID, "", rel, 0)
		require.NoError(t, err)
		assert.Equal(t, wantDefault, lives.Point.Default, "ContextLives %s", rel)
	}
}
