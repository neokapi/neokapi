package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// TestChangesJSON_ReadsTheBasisFromAStoreItDidNotOpen pins that a read takes
// a translation's basis from the project's block history when another App
// recorded it: the store is found through the database driver, which in the
// browser holds it in SQLite's memory where the file system cannot see it.
func TestChangesJSON_ReadsTheBasisFromAStoreItDidNotOpen(t *testing.T) {
	a, recipe := changeProject(t, project.ContentItem{Path: "docs/*.md", Target: "i18n/{lang}/{path}.md"}, map[string]string{
		"docs/guide.md": "# Install\n\nRun the installer.\n",
	})
	t.Cleanup(a.Shutdown)
	ctx := t.Context()
	page, _ := readPageJSON(t, a, recipe, map[string]any{"doc": "docs/guide.md"})
	p := blockWith(t, page, "Run the installer.")
	at := p.Ref
	fr, err := model.ParseEditionKey("fr")
	require.NoError(t, err)
	at.Edition = fr
	set := asJSON(t, map[string]any{"ops": []any{map[string]any{"op": "set_content", "at": at, "if_match": model.AbsentRevision, "text": "Lancez le programme d'installation."}}})
	out, err := a.ApplyChangesJSON(ctx, "browser", set, callOptions(t, recipe, nil))
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, resultJSON(t, out).Status, "%s", out)

	other := &App{}
	other.InitRegistries()
	t.Cleanup(other.Shutdown)
	page, _ = readPageJSON(t, other, recipe, map[string]any{"doc": "docs/guide.md", "editions": []string{"fr"}})
	got := blockWith(t, page, "Run the installer.").Editions["fr"]
	assert.Equal(t, "Lancez le programme d'installation.", got.Text)
	assert.Equal(t, p.Rev, got.Basis, "the basis the history recorded")
}
