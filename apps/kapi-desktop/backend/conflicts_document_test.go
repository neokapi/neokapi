package backend

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/project"
)

// kpzTab opens a project whose directory holds work.kpz, extracted with
// messages.json as its source.
func kpzTab(t *testing.T, app *App) (tab, work string) {
	t.Helper()
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	tab, root := changeProjectOf(t, app, map[string]string{"locales/en.json": `{"title": "Tide window"}` + "\n"},
		[]project.ContentItem{{Path: "locales/en.json", Target: "locales/{lang}.json"}})
	work = filepath.Join(root, "work.kpz")
	extractKpz(t, app, work, `{"greeting": "Hello there", "farewell": "Goodbye now"}`+"\n")
	return tab, work
}

// extractKpz writes a KPZ to work that carries source as messages.json.
func extractKpz(t *testing.T, app *App, work, source string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(source), 0o644))
	out := filepath.Join(dir, "out.kpz")
	require.NoError(t, app.hostEngine().ExtractToKpz(context.Background(), []string{src}, out, "fr", "", true))
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(work, data, 0o644))
}

// readDoc reads doc through the Read binding.
func readDoc(t *testing.T, app *App, tab, doc string) change.Page {
	t.Helper()
	raw, err := app.Read(tab, `{"doc": "`+doc+`"}`)
	require.NoError(t, err)
	var page change.Page
	require.NoError(t, json.Unmarshal([]byte(raw), &page))
	return page
}

// applyJSON sends one set_content through the Apply binding, as the
// conflicts card does.
func applyJSON(t *testing.T, app *App, tab string, at change.Ref, rev, text string) change.Result {
	t.Helper()
	body, err := json.Marshal(map[string]any{"ops": []any{map[string]any{"op": "set_content",
		"at": map[string]any{"doc": at.Doc, "block": at.Block}, "if_match": rev, "text": text}}})
	require.NoError(t, err)
	raw, err := app.Apply(tab, string(body))
	require.NoError(t, err)
	var res change.Result
	require.NoError(t, json.Unmarshal([]byte(raw), &res))
	return res
}

// TestKeptConflicts_TheDesktopRebasesOrDiscardsADivergentDocument: a KPZ
// replaced on disk while a document it carries holds the person's edit is a
// "document" conflict with no blocks. Rebasing carries the new version's
// other changes over and lists the block both changed; deciding it through
// Apply settles the conflict. Discarding keeps the edit and drops the rest.
func TestKeptConflicts_TheDesktopRebasesOrDiscardsADivergentDocument(t *testing.T) {
	const doc = "work.kpz!messages.json"
	for _, discard := range []bool{false, true} {
		name := "rebase"
		if discard {
			name = "discard"
		}
		t.Run(name, func(t *testing.T) {
			app := NewApp()
			tab, work := kpzTab(t, app)
			page := readDoc(t, app, tab, doc)
			require.Len(t, page.Blocks, 2)
			greeting := page.Blocks[0]
			res := applyJSON(t, app, tab, greeting.Ref, greeting.Rev, "Hello, edited on the desktop")
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

			extractKpz(t, app, work, `{"greeting": "Hello from the team", "farewell": "Goodbye from the team"}`+"\n")
			conflicts, err := app.GetKeptConflicts(tab)
			require.NoError(t, err)
			require.Len(t, conflicts, 1)
			c := conflicts[0]
			assert.Equal(t, "document", c.Kind)
			assert.Equal(t, doc, c.Doc)
			assert.False(t, c.Rebased)
			assert.Empty(t, c.Blocks)
			assert.Len(t, readDoc(t, app, tab, doc).Divergent, 1, "a read reports the version that did not land")

			if discard {
				require.NoError(t, app.DiscardKeptDocument(tab, c.Doc, c.Edit))
				page = readDoc(t, app, tab, doc)
				assert.Equal(t, "Hello, edited on the desktop", page.Blocks[0].Text)
				assert.Equal(t, "Goodbye now", page.Blocks[1].Text)
			} else {
				rb, err := app.RebaseKeptDocument(tab, c.Doc, c.Edit)
				require.NoError(t, err)
				assert.Empty(t, rb.Refused)
				assert.Equal(t, 1, rb.Carried)
				assert.Equal(t, 1, rb.Contested)

				conflicts, err = app.GetKeptConflicts(tab)
				require.NoError(t, err)
				require.Len(t, conflicts, 1)
				c = conflicts[0]
				assert.True(t, c.Rebased)
				require.Len(t, c.Blocks, 1)
				b := c.Blocks[0]
				assert.Equal(t, greeting.Ref.Block, b.Block)
				assert.Equal(t, "Hello, edited on the desktop", b.Held.Text)
				assert.Equal(t, "Hello from the team", b.Other.Text)

				res = applyJSON(t, app, tab, change.Ref{Doc: c.Doc, Block: b.Block}, b.Held.Rev, b.Other.Text)
				require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
				page = readDoc(t, app, tab, doc)
				assert.Equal(t, "Hello from the team", page.Blocks[0].Text)
				assert.Equal(t, "Goodbye from the team", page.Blocks[1].Text)
			}
			assert.Empty(t, page.Divergent)
			after, err := app.GetKeptConflicts(tab)
			require.NoError(t, err)
			assert.Empty(t, after)
		})
	}
}

// TestWorkspaceDocumentConflicts_ListsAKpzOutsideAnyProject: the workspace
// home lists a .kpz's divergent version with no project open, the .kpz named
// by its absolute path, and settles it the way a project's card does:
// rebase, then decide the block left through ApplyWorkspaceDocument.
func TestWorkspaceDocumentConflicts_ListsAKpzOutsideAnyProject(t *testing.T) {
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	app := NewApp()
	ctx := context.Background()
	work := filepath.Join(t.TempDir(), "loose.kpz")
	extractKpz(t, app, work, `{"greeting": "Hello there", "farewell": "Goodbye now"}`+"\n")
	doc := work + "!messages.json"

	svc, err := app.hostEngine().KpzDocumentService(ctx, doc, "desktop")
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	text := "Hello, edited loose"
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: page.Blocks[0].Ref,
		IfMatch: page.Blocks[0].Rev, Body: &change.SetContent{Text: &text}}}}, desktopActor)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	none, err := app.GetWorkspaceDocumentConflicts()
	require.NoError(t, err)
	assert.Empty(t, none)

	extractKpz(t, app, work, `{"greeting": "Hello from the team", "farewell": "Goodbye from the team"}`+"\n")
	conflicts, err := app.GetWorkspaceDocumentConflicts()
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	c := conflicts[0]
	assert.Equal(t, "document", c.Kind)
	assert.Equal(t, doc, c.Doc, "named by the .kpz's absolute path")

	rb, err := app.RebaseWorkspaceDocument(c.Doc, c.Edit)
	require.NoError(t, err)
	assert.Empty(t, rb.Refused)
	assert.Equal(t, 1, rb.Carried)
	assert.Equal(t, 1, rb.Contested)

	conflicts, err = app.GetWorkspaceDocumentConflicts()
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	require.True(t, conflicts[0].Rebased)
	require.Len(t, conflicts[0].Blocks, 1)
	b := conflicts[0].Blocks[0]
	assert.Equal(t, "Hello, edited loose", b.Held.Text)

	body, err := json.Marshal(map[string]any{"ops": []any{map[string]any{"op": "set_content",
		"at": map[string]any{"doc": c.Doc, "block": b.Block}, "if_match": b.Held.Rev, "text": b.Held.Text}}})
	require.NoError(t, err)
	raw, err := app.ApplyWorkspaceDocument(string(body))
	require.NoError(t, err)
	var applied change.Result
	require.NoError(t, json.Unmarshal([]byte(raw), &applied))
	require.Equal(t, change.SetApplied, applied.Status, "%+v", applied.Ops)

	after, err := app.GetWorkspaceDocumentConflicts()
	require.NoError(t, err)
	assert.Empty(t, after, "keeping the held wording decides the last block")

	extractKpz(t, app, work, `{"greeting": "Hello again", "farewell": "Goodbye now"}`+"\n")
	conflicts, err = app.GetWorkspaceDocumentConflicts()
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	require.NoError(t, app.DiscardWorkspaceDocument(conflicts[0].Doc, conflicts[0].Edit))
	after, err = app.GetWorkspaceDocumentConflicts()
	require.NoError(t, err)
	assert.Empty(t, after)

	raw, err = app.ApplyWorkspaceDocument(`{"ops": [{"op": "set_content", "at": {"doc": "relative.kpz!x.json", "block": "a"}, "if_match": "absent", "text": "x"}]}`)
	require.NoError(t, err)
	assert.Contains(t, raw, "not_found", "a .kpz the workspace home edits is named by its absolute path")
}
