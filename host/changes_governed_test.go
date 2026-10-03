package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projector"
)

// TestChangeService_GovernsAndRecordsAnEdit drives the host's change service
// with its hooks plugged in: the commit check refuses an edit that introduces
// a failing term rule, a person may land it under gate: report and the
// record names what was overridden, an agent may not ask for report, every
// edit that lands is one content.edit operation with block-history rows, and a
// read shows the basis the history recorded for a translation.
func TestChangeService_GovernsAndRecordsAnEdit(t *testing.T) {
	f := newCommitFixture(t)
	ctx := t.Context()
	svc, err := f.app.ChangeService(ctx, ChangeServiceOptions{Project: f.recipe, Origin: "test"})
	require.NoError(t, err)
	const doc = "docs/guide.md"
	original := readFile(t, f.recipe, doc)
	person := change.Actor{Kind: change.ActorPerson, Name: "asgeir"}
	agent := change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s_01"}

	read := func(t *testing.T, editions ...string) change.BlockRead {
		t.Helper()
		req := change.ReadRequest{Doc: doc}
		for _, e := range editions {
			req.Editions = append(req.Editions, editionKey(t, e))
		}
		page, err := svc.Read(ctx, req)
		require.NoError(t, err)
		require.NotEmpty(t, page.Blocks)
		return page.Blocks[len(page.Blocks)-1]
	}
	utilize := func(gate change.Gate) change.Set {
		p := read(t)
		return change.Set{Gate: gate, Note: "Say utilize", Ops: []change.Op{setTo(p.Ref, p.Rev, "We utilize the widget every day.")}}
	}
	docKey := func(t *testing.T) string {
		t.Helper()
		docs, err := f.app.DocumentIndex(ctx, f.root)
		require.NoError(t, err)
		return docs.Key(doc)
	}

	t.Run("an edit that introduces a failing term rule is refused", func(t *testing.T) {
		res, err := svc.Apply(ctx, utilize(""), person)
		require.NoError(t, err)
		require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
		require.NotNil(t, res.Ops[0].Error)
		assert.Equal(t, change.CodeGateFailed, res.Ops[0].Error.Code)
		assert.Contains(t, findingRules(docFindings(res)), "terms.vocabulary")
		assert.Empty(t, res.Ops[0].Findings, "the findings are the document's")
		for _, f := range docFindings(res) {
			if f.Rule != "terms.vocabulary" {
				continue
			}
			assert.Equal(t, "use", f.Replacement, "a term finding names the wording to use")
			require.NotNil(t, f.Range, "a term finding names the span it found")
			assert.Equal(t, model.RunPos{Run: 0, Offset: 3}, f.Range.Start)
			assert.Equal(t, model.RunPos{Run: 0, Offset: 10}, f.Range.End)
		}
		assert.Nil(t, res.Record)
		assert.Equal(t, original, readFile(t, f.recipe, doc), "a refused edit writes nothing")
		assert.Empty(t, editOps(t, f.app, f.root), "a refused edit records nothing")
	})

	t.Run("an agent may not land it under report", func(t *testing.T) {
		res, err := svc.Apply(ctx, utilize(change.GateReport), agent)
		require.NoError(t, err)
		require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
		require.NotNil(t, res.Ops[0].Error)
		assert.Equal(t, change.CodeNotPermitted, res.Ops[0].Error.Code)
		assert.Equal(t, original, readFile(t, f.recipe, doc))
	})

	t.Run("a person lands it under report and the record lists the overridden finding", func(t *testing.T) {
		res, err := svc.Apply(ctx, utilize(change.GateReport), person)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.Equal(t, "# Guide\n\nWe utilize the widget every day.\n", readFile(t, f.recipe, doc))
		require.NotNil(t, res.Record)

		ops := editOps(t, f.app, f.root)
		require.Len(t, ops, 1)
		assert.Equal(t, *res.Record, ops[0].ID)
		var e projector.Edit
		require.NoError(t, json.Unmarshal(ops[0].Payload, &e))
		assert.Equal(t, person, e.Actor)
		assert.Equal(t, "test", e.Origin.By)
		assert.Equal(t, "Say utilize", e.Note)
		assert.NotEmpty(t, e.Fingerprint, "the record names the governance the edit was checked under")
		assert.Contains(t, findingRules(e.Overridden), "terms.vocabulary")
		for _, o := range e.Overridden {
			assert.True(t, o.Fails, "an overridden finding is one that fails")
		}
	})

	t.Run("an edit that lands is a content.edit operation and a block-history row", func(t *testing.T) {
		p := read(t)
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(p.Ref, p.Rev, "We use the widget each day.")}}, person)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		require.NotNil(t, res.Record)

		ops := editOps(t, f.app, f.root)
		require.Len(t, ops, 2)
		assert.Equal(t, projector.KindEdit, ops[1].Kind)
		assert.Equal(t, *res.Record, ops[1].ID)

		db, err := f.app.ProjectDB(ctx, f.root)
		require.NoError(t, err)
		row, found, err := db.History().LastWrite(ctx, docKey(t), p.Ref.Block, "en")
		require.NoError(t, err)
		require.True(t, found, "the edit is in the block history")
		assert.Equal(t, *res.Record, row.Op)
		assert.Equal(t, string(change.ActorPerson), row.Actor)
		assert.Equal(t, "asgeir", row.ActorName)
		assert.Equal(t, p.Rev, row.Before)
		assert.Equal(t, read(t).Rev, row.After)
		rows, err := db.History().Edition(ctx, docKey(t), p.Ref.Block, "en", 0)
		require.NoError(t, err)
		assert.Len(t, rows, 2, "both edits that landed are in the history")
	})

	t.Run("a read shows the basis the history recorded for a translation", func(t *testing.T) {
		p := read(t)
		at := p.Ref
		at.Edition = editionKey(t, "fr")
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(at, model.AbsentRevision, "Nous utilisons le gadget chaque jour.")}}, person)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

		fr := read(t, "fr").Editions["fr"]
		assert.Equal(t, "Nous utilisons le gadget chaque jour.", fr.Text)
		assert.Equal(t, p.Rev, fr.Basis, "the translation was made from the source as it stands")
		assert.False(t, fr.Stale)

		res, err = svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(p.Ref, p.Rev, "We use the widget daily.")}}, person)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		fr = read(t, "fr").Editions["fr"]
		assert.Equal(t, p.Rev, fr.Basis)
		assert.True(t, fr.Stale, "the source moved past the translation's basis")
	})

	t.Run("a history lists the recorded changes to an edition, most recent first", func(t *testing.T) {
		p := read(t, "fr")
		h, err := svc.History(ctx, change.HistoryRequest{Ref: p.Ref})
		require.NoError(t, err)
		assert.Equal(t, p.Ref, h.Ref)
		assert.Equal(t, p.Rev, h.Rev)
		require.Len(t, h.Entries, 3, "the three edits of the source that landed")
		latest := h.Entries[0]
		assert.Equal(t, p.Rev, latest.After, "the most recent change left the edition as it reads now")
		require.NotNil(t, latest.Actor)
		assert.Equal(t, person, *latest.Actor)
		assert.Equal(t, "test", latest.Origin)
		assert.False(t, latest.At.IsZero())
		for i := 1; i < len(h.Entries); i++ {
			assert.Equal(t, h.Entries[i].After, h.Entries[i-1].Before, "each change starts where the one before it ended")
		}

		at := p.Ref
		at.Edition = editionKey(t, "fr")
		frHist, err := svc.History(ctx, change.HistoryRequest{Ref: at})
		require.NoError(t, err)
		assert.Equal(t, at, frHist.Ref)
		assert.Equal(t, p.Editions["fr"].Rev, frHist.Rev)
		require.Len(t, frHist.Entries, 1)
		assert.Equal(t, model.AbsentRevision, frHist.Entries[0].Before, "the translation was created")
		assert.NotEmpty(t, frHist.Entries[0].Basis)

		one, err := svc.History(ctx, change.HistoryRequest{Ref: p.Ref, Limit: 1})
		require.NoError(t, err)
		require.Len(t, one.Entries, 1)
		assert.Equal(t, latest, one.Entries[0])
	})
}

// TestChangeService_DescribesAndWritesWhatAFormatDeclares pins the capability
// wiring: Describe reports the operations an HTML writer declares, a read
// lists a link's writable attributes, and set_attribute and mark land through
// the file home's writer.
func TestChangeService_DescribesAndWritesWhatAFormatDeclares(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	const page = "<html><body><p>Read the guide before you start.</p><p>See <a href=\"https://a.example/\">the docs</a>.</p></body></html>\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "page.html"), []byte(page), 0o644))
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	t.Cleanup(a.Shutdown)
	ctx := t.Context()
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Root: dir, Origin: "test"})
	require.NoError(t, err)

	d, err := svc.Describe(ctx, change.DescribeRequest{Format: "html"})
	require.NoError(t, err)
	assert.Contains(t, d.Ops.SetAttribute["link:hyperlink"], "href")
	require.NotNil(t, d.Ops.Mark, "html writes new codes")
	assert.Contains(t, d.Ops.Mark.Types, "fmt:bold")
	assert.Contains(t, d.Ops.Mark.Types, "link:hyperlink")
	require.NotNil(t, d.Ops.SetContent)
	assert.Contains(t, d.Ops.SetContent.NewCodes, "fmt:bold")
	assert.Nil(t, d.Ops.InsertBlock)

	byDoc, err := svc.Describe(ctx, change.DescribeRequest{Doc: "page.html"})
	require.NoError(t, err)
	assert.Equal(t, d.Ops, byDoc.Ops, "a document is described by what its writer declares")

	read := func(text string) change.BlockRead {
		t.Helper()
		p, err := svc.Read(ctx, change.ReadRequest{Doc: "page.html"})
		require.NoError(t, err)
		return blockWith(t, p, text)
	}
	link := read(`See <x id="1"/>the docs<x id="/1"/>.`)
	require.Contains(t, link.Codes, "1")
	assert.Equal(t, []string{"href"}, link.Codes["1"].Writable)
	assert.True(t, slices.Contains(link.Ops, change.KindSetAttribute))
	assert.True(t, slices.Contains(link.Ops, change.KindMark))

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetAttribute, At: link.Ref, IfMatch: link.Rev,
		Body: &change.SetAttribute{Code: "1", Name: "href", Value: "https://b.example/?a=1&b=2"}}}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	guide := read("Read the guide before you start.")
	find := "guide"
	res, err = svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindMark, At: guide.Ref, IfMatch: guide.Rev,
		Body: &change.Mark{Range: change.Selection{Find: &find}, Type: "fmt:bold"}}}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	got, err := os.ReadFile(filepath.Join(dir, "page.html"))
	require.NoError(t, err)
	assert.Equal(t, "<html><body><p>Read the <strong>guide</strong> before you start.</p><p>See <a href=\"https://b.example/?a=1&amp;b=2\">the docs</a>.</p></body></html>\n", string(got))
}

// docFindings lists what the commit check found on every document of a
// result.
func docFindings(res *change.Result) []change.Finding {
	var out []change.Finding
	for _, d := range res.Docs {
		out = append(out, d.Findings...)
	}
	return out
}

func findingRules(fs []change.Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return out
}
