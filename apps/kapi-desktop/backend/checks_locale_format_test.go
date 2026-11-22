package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// posixLocaleTab opens a project whose recipe writes locales in POSIX style, so
// its Norwegian translation is app/nb_NO.json, beside a source whose
// placeholder that translation drops.
func posixLocaleTab(t *testing.T, app *App) (tabID, root string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app", "en.json"), []byte(`{"hi":"Hello {name}"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app", "nb_NO.json"), []byte(`{"hi":"Hei"}`), 0o644))
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb_NO"},
			LocaleFormat:    "posix",
		},
		Collections: []project.Collection{{Path: "app/en.json", Target: "app/{lang}.json"}},
	}))
	readContextAt(t, recipe)
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID, root
}

// The Checks panel and the Outputs list find a translation where the project's
// locale_format writes it: app/nb_NO.json, not app/nb-NO.json.
func TestChecksAndOutputsFollowTheLocaleFormat(t *testing.T) {
	app := NewApp()
	tabID, root := posixLocaleTab(t, app)

	res, err := app.RunChecks(tabID, ProjectFilter{Languages: []string{"nb-NO"}})
	require.NoError(t, err)
	require.Len(t, res.Files, 1)
	var target []DesktopFinding
	for _, f := range res.Files[0].Findings {
		if f.Field == "target" {
			target = append(target, f)
		}
	}
	require.NotEmpty(t, target, "the translation in app/nb_NO.json was read and checked")
	assert.Equal(t, "placeholder", target[0].Check)
	assert.Equal(t, "nb-NO", target[0].Locale)
	assert.Equal(t, "failed", res.Verdict)

	outputs, err := app.ListOutputs(tabID)
	require.NoError(t, err)
	var out OutputFileInfo
	for _, o := range outputs["app/en.json"] {
		if o.Lang == "nb-NO" {
			out = o
		}
	}
	assert.Equal(t, "app/nb_NO.json", out.Relative, "the declared language's output: %+v", outputs)
	assert.Equal(t, filepath.Join(root, "app", "nb_NO.json"), out.Path)
	assert.True(t, out.Exists)
}
