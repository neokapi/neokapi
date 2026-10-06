package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// parkedTab opens a project that keeps a translation in the workspace home
// until a delivery writes its file, and gives the French title a person's
// wording there.
func parkedTab(t *testing.T, app *App) (string, string) {
	t.Helper()
	tab, root := changeProjectOf(t, app, map[string]string{
		"locales/en.json": `{"title": "Tide window", "cta": "Plan a crossing"}` + "\n",
	}, []project.ContentItem{{Path: "locales/en.json", Target: "locales/{lang}.json"}})
	op := app.getOpenProject(tab)
	op.Project.Defaults.Materialize = project.MaterializeOnConverge
	require.NoError(t, project.Save(op.Path, op.Project))
	app.CloseProject(tab)
	reopened, err := app.OpenProject(op.Path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(reopened.ID) })

	svc := bindingService{app: app, tab: reopened.ID}
	text := "Fenêtre de marée"
	res, err := svc.Apply(t.Context(), change.Set{Ops: []change.Op{{Kind: change.KindSetContent, IfMatch: model.AbsentRevision,
		At: change.Ref{Doc: "locales/en.json", Block: "title", Edition: model.EditionKey{Locale: "fr"}}, Body: &change.SetContent{Text: &text}}}}, change.Actor{})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	_, err = os.Stat(filepath.Join(root, "locales", "fr.json"))
	require.True(t, os.IsNotExist(err), "the translation is kept in the workspace, not written")
	return reopened.ID, root
}

// TestKeptConflicts_TheDesktopDecidesWordingTheFileDoesNotHold: a French
// file that appears without the person's kept wording is a conflict the
// desktop lists with both wordings. Taking the kept wording writes it into
// the file through Apply, and the release drops the workspace's copy.
func TestKeptConflicts_TheDesktopDecidesWordingTheFileDoesNotHold(t *testing.T) {
	app := NewApp()
	tab, root := parkedTab(t, app)
	none, err := app.GetKeptConflicts(tab)
	require.NoError(t, err)
	assert.Empty(t, none, "a kept translation with no file is no conflict")

	fr := filepath.Join(root, "locales", "fr.json")
	require.NoError(t, os.WriteFile(fr, []byte(`{"title": "Autre chose", "cta": "Planifier"}`+"\n"), 0o644))
	conflicts, err := app.GetKeptConflicts(tab)
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	c := conflicts[0]
	assert.Equal(t, "file", c.Kind)
	assert.Equal(t, "locales/en.json", c.Doc)
	assert.Equal(t, "fr", c.Locale)
	assert.Equal(t, "locales/fr.json", c.File)
	require.Len(t, c.Blocks, 1)
	b := c.Blocks[0]
	assert.Equal(t, "title", b.Block)
	assert.Equal(t, "Tide window", b.Source)
	assert.Equal(t, "Autre chose", b.Held.Text)
	assert.Equal(t, "Fenêtre de marée", b.Other.Text)

	set := map[string]any{"ops": []any{map[string]any{"op": "set_content",
		"at": map[string]any{"doc": c.Doc, "block": b.Block, "edition": c.Locale}, "if_match": b.Held.Rev, "text": b.Other.Text}}}
	body, err := json.Marshal(set)
	require.NoError(t, err)
	raw, err := app.Apply(tab, string(body))
	require.NoError(t, err)
	var res change.Result
	require.NoError(t, json.Unmarshal([]byte(raw), &res))
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	require.NoError(t, app.ReleaseKeptWording(tab, c.Doc, c.Locale, []string{b.Block}))

	written, err := os.ReadFile(fr)
	require.NoError(t, err)
	assert.Contains(t, string(written), "Fenêtre de marée")
	after, err := app.GetKeptConflicts(tab)
	require.NoError(t, err)
	assert.Empty(t, after)
}
