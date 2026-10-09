package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
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

// kapi exec -p names the project that governs the run, as -p does on every
// other command, and the file named on the command line is read by the format
// its extension calls for either way. A one-unit XLIFF 2.0 file is one block
// ad hoc and one block under -p; the recipe is never read as a second input.
func TestExec_ProjectFlagGovernsWithoutChangingTheReader(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	// Discovery is off, so the project reaches the run through -p alone.
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	file := filepath.Join(root, "docs", "one.xlf")
	require.NoError(t, os.WriteFile(file, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="nb">
  <file id="f1">
    <unit id="u1">
      <segment>
        <source>Reuse comes from the content memory.</source>
        <target>Gjenbruk kommer fra oversettelsesminnet.</target>
      </segment>
    </unit>
  </file>
</xliff>
`), 0o644))
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version:     project.CurrentVersion,
		Name:        "exec-project",
		Defaults:    project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"nb"}},
		Collections: []project.Collection{{Name: "docs", Path: "docs/*.xlf"}},
	}))
	// The project's terms store is what governs a term-check run in it.
	seedTermsStore(t, root, terms.Concept{
		ID: "c1",
		Terms: []terms.Term{
			{Text: "content memory", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "innholdsminnet", Locale: model.LocaleID("nb"), Status: model.TermPreferred},
		},
	})
	t.Chdir(t.TempDir())

	run := func(args ...string) map[string]any {
		t.Helper()
		app := &App{Quiet: true}
		app.InitRegistries()
		defer app.Shutdown()
		execCmd := NewToolCommands(app)[0]
		execCmd.SetArgs(append([]string{"term-check", "--target-lang", "nb", "--json"}, args...))
		out, _ := captureStdout(t, func() error { return execCmd.Execute() })
		var report map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &report), out)
		return report
	}

	adHoc := run(file)
	assert.EqualValues(t, 1, adHoc["target"].(map[string]any)["blocks"], "ad hoc: the one unit is one block")
	assert.Empty(t, adHoc["findings"], "ad hoc: no project, so no term rules")

	governed := run("-p", recipe, file)
	assert.EqualValues(t, 1, governed["target"].(map[string]any)["blocks"], "-p: the same reader, the same one block")
	assert.NotEmpty(t, governed["findings"], "-p: the recipe's term-check preset governs the run")
}
