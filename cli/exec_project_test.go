package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/project"
)

// kapi exec over a file of the project the command resolves records what it
// changed in the project's history, as kapi apply in that project does, and
// takes the file's lock where kapi apply takes it.
func TestExec_InAProjectRecordsWhatItChanged(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "page.html"),
		[]byte(`<html><body><p>The shop opens at nine.</p></body></html>`+"\n"), 0o644))
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version:     project.CurrentVersion,
		Name:        "exec",
		Defaults:    project.Defaults{SourceLanguage: "en"},
		Collections: []project.Collection{{Name: "docs", Path: "docs/*.html"}},
	}))
	// The project is the one the command resolves, as kapi apply resolves it.
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv(project.ProjectEnvVar, recipe)
	t.Chdir(filepath.Join(root, "docs"))

	app := &App{Quiet: true}
	app.InitRegistries()
	defer app.Shutdown()
	execCmd := NewToolCommands(app)[0]
	execCmd.SetArgs([]string{"case-transform", "page.html", "--mode", "upper", "--apply-source"})
	_, err := captureStdout(t, func() error { return execCmd.Execute() })
	require.NoError(t, err)

	page, err := os.ReadFile(filepath.Join(root, "docs", "page.html"))
	require.NoError(t, err)
	require.Contains(t, string(page), "THE SHOP OPENS AT NINE.")

	ctx := context.Background()
	db, err := app.ProjectDB(ctx, root)
	require.NoError(t, err)
	idx, err := app.DocumentIndex(ctx, root)
	require.NoError(t, err)
	rows, err := db.History().Document(ctx, idx.Key("docs/page.html"))
	require.NoError(t, err)
	require.NotEmpty(t, rows, "the run records what it changed in the project")
	assert.Equal(t, string(change.ActorTool), rows[0].Actor)
	assert.Equal(t, "case-transform", rows[0].ActorName)
	assert.DirExists(t, filepath.Join(project.LayoutAt(root).WorkDir(), "locks"))
}
