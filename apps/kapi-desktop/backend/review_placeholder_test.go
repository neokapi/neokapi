package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInterpolationProject opens a project whose source is an i18next bundle,
// so `{{name}}` is read as a placeholder run rather than as text.
func newInterpolationProject(t *testing.T, app *App) *TabInfo {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "en.json"),
		[]byte(`{"cordoned":"Cordoned node {{name}}.","cordonFailed":"Failed to cordon node {{name}}","drained":"Drained node {{name}}."}`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "nb.json"),
		[]byte(`{"cordoned":"Sperret node {{name}} for planlegging.","cordonFailed":"Kunne ikke sperre node {{name}} for planlegging.","drained":"Tømte node {{name}}."}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Glossary",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []project.Collection{{
			Name: "Glossary",
			Content: []project.ContentItem{{
				Path:   "locales/en.json",
				Target: "locales/{lang}.json",
				Format: &project.FormatSpec{Name: "i18next"},
			}},
		}},
	}
	path := filepath.Join(root, "project.kapi")
	require.NoError(t, project.Save(path, proj))
	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab
}

// TestReview_KeepsTheSelectedUnitsPlaceholder guards the Review page against a
// unit whose placeholder vanished from every place the selected unit is drawn
// (the queue row, the detail header, its own row among its neighbours) while
// the neighbours kept theirs.
func TestReview_KeepsTheSelectedUnitsPlaceholder(t *testing.T) {
	app := NewApp()
	tab := newInterpolationProject(t, app)
	file := filepath.Join("locales", "nb.json")

	queue, err := app.ReviewQueue(tab.ID, ProjectFilter{})
	require.NoError(t, err)
	var row *string
	for _, it := range queue.Pending {
		if it.Key == "/cordonFailed" && it.Locale == "nb" {
			src := it.Source
			row = &src
			assert.Equal(t, "Kunne ikke sperre node {{name}} for planlegging.", it.Target, "the row's target keeps the placeholder")
		}
	}
	require.NotNil(t, row, "the unit is in the queue")
	assert.Equal(t, "Failed to cordon node {{name}}", *row, "the row's source keeps the placeholder")

	d, err := app.GetReviewUnit(tab.ID, "nb", file, "/cordonFailed")
	require.NoError(t, err)
	assert.Equal(t, "Failed to cordon node {{name}}", d.Source)
	assert.Equal(t, "Kunne ikke sperre node {{name}} for planlegging.", d.Target)

	t.Run("the unit travels as runs, placeholder included", func(t *testing.T) {
		require.Len(t, d.SourceRuns, 2)
		require.NotNil(t, d.SourceRuns[1].Ph, "{{name}} is a placeholder run")
		assert.Equal(t, "{{name}}", d.SourceRuns[1].Ph.Data)
		require.Len(t, d.TargetRuns, 3)
		require.NotNil(t, d.TargetRuns[1].Ph)
		assert.Equal(t, "{{name}}", d.TargetRuns[1].Ph.Data)
	})

	t.Run("the neighbourhood carries the unit between its neighbours", func(t *testing.T) {
		require.NotNil(t, d.Context)
		n := d.Context.Neighbourhood
		require.NotNil(t, n.Unit, "the unit's own row has runs to draw")
		assert.Equal(t, d.SourceRuns, n.Unit.Source)
		assert.Equal(t, d.TargetRuns, n.Unit.Target)
		require.Len(t, n.Before, 1)
		require.Len(t, n.After, 1)
	})
}
