package filehome_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
)

// A translation kept in a file of its own, such as de/guide.json beside
// guide.json, is that file's block. remove_edition takes the block out of the
// file through the format's writer, as delete_block takes a block out of an
// edition's file, so the translation reads back absent. The file itself stays,
// with every other block. A format whose writer removes no block refuses the
// removal and writes nothing.

// removeOp is a remove_edition of edition k of block, guarded by rev.
func removeOp(doc, block string, k model.EditionKey, rev string) change.Op {
	return change.Op{Kind: change.KindRemoveEdition, At: change.Ref{Doc: doc, Block: block, Edition: k}, IfMatch: rev, Body: &change.RemoveEdition{}}
}

// editionOf reads edition k of the block keyed key in doc.
func editionOf(t *testing.T, svc *change.Service, doc, key string, k model.EditionKey) (change.EditionRead, bool) {
	t.Helper()
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: doc, Blocks: []string{key}, Editions: []model.EditionKey{k}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	text, _ := k.MarshalText()
	ed, ok := page.Blocks[0].Editions[string(text)]
	return ed, ok
}

func TestFileHome_RemovesATranslationFromItsOwnFile(t *testing.T) {
	de := mustEdition(t, "de")
	cases := []struct {
		name string
		opts filehome.Options
	}{
		{name: "the file is edited through its own skeleton"},
		// kapi merge and kapi pull write a translation's file from the
		// document's skeleton; the block still leaves it.
		{name: "the file is written from the document's skeleton", opts: filehome.Options{Materialize: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newTargetFixture(t, map[string]string{
				"guide.json":    "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\"\n  },\n  \"title\": \"Welcome\"\n}\n",
				"de/guide.json": "{\n  \"nav\": {\n    \"home\": \"Startseite\",\n    \"cart\": \"Warenkorb\"\n  },\n  \"title\": \"Willkommen\"\n}\n",
			}, tc.opts)
			ctx := context.Background()
			ed, ok := editionOf(t, f.svc, "guide.json", "nav.home", de)
			require.True(t, ok)

			res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{removeOp("guide.json", "nav.home", de, ed.Rev)}}, person)
			require.NoError(t, err)
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			assert.Equal(t, model.AbsentRevision, res.Ops[0].After)
			assert.Equal(t, "{\n  \"nav\": {\n    \"cart\": \"Warenkorb\"\n  },\n  \"title\": \"Willkommen\"\n}\n", f.read(t, "de/guide.json"),
				"the translation's member leaves the file, and every other byte stays")
			assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\"\n  },\n  \"title\": \"Welcome\"\n}\n", f.read(t, "guide.json"),
				"the document keeps its bytes")
			_, ok = editionOf(t, f.svc, "guide.json", "nav.home", de)
			assert.False(t, ok, "the translation reads back absent")
			cart, ok := editionOf(t, f.svc, "guide.json", "nav.cart", de)
			require.True(t, ok)
			assert.Equal(t, "Warenkorb", cart.Text, "the other translations stay")
		})
	}
}

func TestFileHome_RemovingEveryTranslationKeepsTheFile(t *testing.T) {
	f := newTargetFixture(t, map[string]string{
		"guide.json":    `{"title": "Welcome", "body": "Read this first"}` + "\n",
		"de/guide.json": `{"title": "Willkommen", "body": "Lies das zuerst"}` + "\n",
	})
	de := mustEdition(t, "de")
	var ops []change.Op
	for _, key := range []string{"title", "body"} {
		ed, ok := editionOf(t, f.svc, "guide.json", key, de)
		require.True(t, ok)
		ops = append(ops, removeOp("guide.json", key, de, ed.Rev))
	}
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: ops}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, "{}\n", f.read(t, "de/guide.json"), "a home never deletes a file: it keeps what the format writes when no block is left")
}

func TestFileHome_RefusesARemovalTheFormatCannotWrite(t *testing.T) {
	f := newTargetFixture(t, map[string]string{
		"guide.md":    "# Guide\n\nRead this first.\n",
		"de/guide.md": "# Anleitung\n\nLies das zuerst.\n",
	})
	de := mustEdition(t, "de")
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: "guide.md", Editions: []model.EditionKey{de}})
	require.NoError(t, err)
	require.NotEmpty(t, page.Blocks)
	b := page.Blocks[len(page.Blocks)-1]
	ed, ok := b.Editions["de"]
	require.True(t, ok, "the read joins the German file: %+v", b.Editions)
	assert.NotContains(t, b.Ops, change.KindRemoveEdition, "a Markdown block offers no removal")

	res, err := f.svc.Apply(context.Background(), change.Set{Ops: []change.Op{removeOp("guide.md", b.Ref.Block, de, ed.Rev)}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Equal(t, string(change.KindRemoveEdition), res.Ops[0].Error.Capability)
	assert.Contains(t, res.Ops[0].Error.Message, "the markdown writer takes no block out of a file")
	assert.Contains(t, res.Ops[0].Error.Message, "set_content")
	assert.Equal(t, "# Anleitung\n\nLies das zuerst.\n", f.read(t, "de/guide.md"), "the refusal writes nothing")
}
