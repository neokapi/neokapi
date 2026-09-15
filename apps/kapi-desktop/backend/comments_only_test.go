package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// commentsOnlyTab opens a project with a JSON catalog and a YAML file at
// site/web, whose voice forbids "utilize". The YAML item's `comments:` is
// spelled as given. It returns the tab ID and the project root.
func commentsOnlyTab(t *testing.T, app *App, spelled string) (string, string) {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("locales/en.json", `{"greeting":"Please utilize the plan."}`)
	write("config/app.yaml", "# Greets the reader.\ngreeting: Please utilize the reader.\n")
	writeVoice(t, filepath.Join(root, project.RelStatePath(project.ProfilesDirName, "site", "voice.yaml")), `name: Site
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: major
`)

	var comments project.ContentComments
	require.NoError(t, yaml.Unmarshal([]byte(spelled), &comments))
	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "CommentsOnly",
		Defaults: project.Defaults{SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"fr-FR"}},
		Profiles: map[string]project.Profile{"site": {Channels: []project.Channel{{ID: "web"}}}},
		Collections: []project.Collection{
			{Name: "app", Channel: "site/web", Content: []project.ContentItem{{Path: "locales/en.json"}}},
			{Name: "config", Channel: "site/web", Content: []project.ContentItem{{Path: "config/app.yaml", Comments: comments}}},
		},
	}
	path := filepath.Join(root, "project.kapi")
	require.NoError(t, project.Save(path, proj))
	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID, root
}

// The Checks panel checks a comments-only item's comments and none of its
// values: the file is listed, and the value's "utilize" is not a finding.
func TestRunChecksReadsNoValueOfACommentsOnlyItem(t *testing.T) {
	checked := func(t *testing.T, spelled string) map[string]int {
		t.Helper()
		isolateCheckPlugins(t)
		app := NewApp()
		tabID, _ := commentsOnlyTab(t, app, spelled)
		res, err := app.RunChecks(tabID, ProjectFilter{})
		require.NoError(t, err)
		findings := map[string]int{}
		for _, f := range res.Files {
			findings[filepath.Base(f.Path)] = len(f.Findings)
		}
		return findings
	}

	assert.Equal(t, map[string]int{"en.json": 1, "app.yaml": 0}, checked(t, "{only: true}"), "the comment is checked and the value is not")
	assert.Equal(t, map[string]int{"en.json": 1, "app.yaml": 1}, checked(t, "true"), "must fail: comments: true checks the value")
}

// A source edit, a source unit's review context and a check fix each address a
// value, so each refuses a file declared for its comments alone.
func TestSourceEditsRefuseACommentsOnlyItem(t *testing.T) {
	for name, act := range map[string]func(app *App, tabID, root string) error{
		"UpdateSourceText": func(app *App, tabID, _ string) error {
			_, err := app.UpdateSourceText(tabID, "config/app.yaml", "greeting", "Please use the reader.")
			return err
		},
		"GetSourceUnitContext": func(app *App, tabID, _ string) error {
			_, err := app.GetSourceUnitContext(tabID, "config/app.yaml", "greeting")
			return err
		},
		"ApplyCheckFix": func(app *App, tabID, root string) error {
			return app.ApplyCheckFix(tabID, filepath.Join(root, "config", "app.yaml"), "greeting", "source", "utilize", "use")
		},
	} {
		t.Run(name, func(t *testing.T) {
			app := NewApp()
			tabID, root := commentsOnlyTab(t, app, "{only: true}")
			err := act(app, tabID, root)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "declared for its comments alone")

			app = NewApp()
			tabID, root = commentsOnlyTab(t, app, "true")
			if err := act(app, tabID, root); err != nil {
				assert.False(t, strings.Contains(err.Error(), "declared for its comments alone"), "must fail: comments: true keeps the values: %v", err)
			}
		})
	}
}
