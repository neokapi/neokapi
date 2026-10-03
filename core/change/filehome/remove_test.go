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

// modes are the two ways a home writes a translation's file: through the
// file's own skeleton, and from the document's (Options.Materialize, as kapi
// merge and kapi pull write it).
var modes = []struct {
	name string
	opts filehome.Options
}{
	{name: "in place"},
	{name: "materialized", opts: filehome.Options{Materialize: true}},
}

// A removal finds the block it takes out of the translation's file by key. An
// edit in the same change set can make the reader skip an earlier block (a
// YAML value written empty, or as spaces alone, reads as no block), which
// moves every block after it; the removal still takes out the block it names
// and no other.
func TestFileHome_ARemovalBesideAnEditTakesOutTheBlockItNames(t *testing.T) {
	de := mustEdition(t, "de")
	cases := []struct {
		mode  int
		value string
		// written is the line the edit leaves for a.
		written string
	}{
		{mode: 0, value: "", written: "a: \"\"\n"},
		{mode: 0, value: "   ", written: "a: \"   \"\n"},
		{mode: 1, value: "   ", written: "a: \"   \"\n"},
	}
	for _, tc := range cases {
		mode := modes[tc.mode]
		t.Run(mode.name+"/"+tc.value, func(t *testing.T) {
			f := newTargetFixture(t, map[string]string{
				"g.yaml":    "a: Alpha\nb: Beta\nc: Gamma\n",
				"de/g.yaml": "a: Alfa\nb: Bet\nc: Gam\n",
			}, mode.opts)
			a, ok := editionOf(t, f.svc, "g.yaml", "a", de)
			require.True(t, ok)
			b, ok := editionOf(t, f.svc, "g.yaml", "b", de)
			require.True(t, ok)
			res, err := f.svc.Apply(context.Background(), change.Set{Ops: []change.Op{
				setOp(change.Ref{Doc: "g.yaml", Block: "a", Edition: de}, a.Rev, tc.value),
				removeOp("g.yaml", "b", de, b.Rev),
			}}, person)
			require.NoError(t, err)
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			assert.Equal(t, tc.written+"c: Gam\n", f.read(t, "de/g.yaml"), "b leaves the file, and c stays")
			_, ok = editionOf(t, f.svc, "g.yaml", "b", de)
			assert.False(t, ok, "the removed translation reads back absent")
			c, ok := editionOf(t, f.svc, "g.yaml", "c", de)
			require.True(t, ok, "the translation nobody removed stays")
			assert.Equal(t, "Gam", c.Text)
		})
	}
}

// A removal the format's writer cannot make (a YAML sequence item, named by
// its position) refuses that operation alone: an edit beside it in the change
// set is not applied, blocked by the refusal, and the file stays as it was.
func TestFileHome_AWriterRefusalRefusesTheRemovalAlone(t *testing.T) {
	de := mustEdition(t, "de")
	const german = "list:\n  - eins\n  - zwei\nt: Titel\n"
	for _, mode := range modes {
		for _, beside := range []bool{false, true} {
			name := mode.name + "/alone"
			if beside {
				name = mode.name + "/beside an edit"
			}
			t.Run(name, func(t *testing.T) {
				f := newTargetFixture(t, map[string]string{"g.yaml": "list:\n  - one\n  - two\nt: Title\n", "de/g.yaml": german}, mode.opts)
				item, ok := editionOf(t, f.svc, "g.yaml", "list.[0]", de)
				require.True(t, ok)
				var ops []change.Op
				if beside {
					title, ok := editionOf(t, f.svc, "g.yaml", "t", de)
					require.True(t, ok)
					ops = append(ops, setOp(change.Ref{Doc: "g.yaml", Block: "t", Edition: de}, title.Rev, "Neu"))
				}
				removal := len(ops)
				ops = append(ops, removeOp("g.yaml", "list.[0]", de, item.Rev))
				res, err := f.svc.Apply(context.Background(), change.Set{Ops: ops}, person)
				require.NoError(t, err)
				require.Equal(t, change.SetRefused, res.Status)
				got := res.Ops[removal]
				assert.Equal(t, change.OpRefused, got.Status)
				require.NotNil(t, got.Error)
				assert.Equal(t, change.CodeUnsupported, got.Error.Code)
				assert.Equal(t, string(change.KindRemoveEdition), got.Error.Capability)
				assert.Contains(t, got.Error.Message, "list.[0]")
				if beside {
					assert.Equal(t, change.OpNotApplied, res.Ops[0].Status, "the edit beside the removal is held, not refused: %+v", res.Ops[0])
					assert.Nil(t, res.Ops[0].Error)
					require.NotNil(t, res.Ops[0].BlockedBy)
					assert.Equal(t, removal, *res.Ops[0].BlockedBy)
				}
				assert.Equal(t, german, f.read(t, "de/g.yaml"), "nothing is written")
			})
		}
	}
}

// A translation removed from a file of its own can be created again: a
// set_content with if_match absent gives the file the block back, through the
// format's writer, under the key the file gives its keys and beside the block
// it sat by, and every other translation stays as it was.
func TestFileHome_ARemovedTranslationIsCreatedAgain(t *testing.T) {
	de := mustEdition(t, "de")
	cases := []struct {
		name, doc, source, german string
		block, text               string
		// recreated is the German file once the translation is back.
		recreated string
	}{
		{name: "a nested JSON key", doc: "g.json",
			source:    "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\"\n  },\n  \"title\": \"Welcome\"\n}\n",
			german:    "{\n  \"nav\": {\n    \"home\": \"Startseite\",\n    \"cart\": \"Warenkorb\"\n  },\n  \"title\": \"Willkommen\"\n}\n",
			block:     "nav.home",
			text:      "Start",
			recreated: "{\n  \"nav\": {\n    \"home\": \"Start\",\n    \"cart\": \"Warenkorb\"\n  },\n  \"title\": \"Willkommen\"\n}\n"},
		{name: "a YAML key between two others", doc: "g.yaml",
			source: "a: Alpha\nb: Beta\nc: Gamma\n", german: "a: Alfa\nb: Bet\nc: Gam\n",
			block: "b", text: "10", recreated: "a: Alfa\nb: \"10\"\nc: Gam\n"},
		{name: "a YAML key under the file's language", doc: "g.yml",
			source: "en:\n  a: Alpha\n  b: Beta\n  c: Gamma\n", german: "de:\n  a: Alfa\n  b: Bet\n  c: Gam\n",
			block: "en.a", text: "Alfa neu", recreated: "de:\n  a: Alfa neu\n  b: Bet\n  c: Gam\n"},
		{name: "an ARB message", doc: "g.arb",
			source:    "{\n  \"@@locale\": \"en\",\n  \"a\": \"Alpha\",\n  \"b\": \"Beta\"\n}\n",
			german:    "{\n  \"@@locale\": \"de\",\n  \"a\": \"Alfa\",\n  \"b\": \"Bet\"\n}\n",
			block:     "b",
			text:      "Beta neu",
			recreated: "{\n  \"@@locale\": \"de\",\n  \"a\": \"Alfa\",\n  \"b\": \"Beta neu\"\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newTargetFixture(t, map[string]string{tc.doc: tc.source, "de/" + tc.doc: tc.german})
			ctx := context.Background()
			before := map[string]string{}
			page, err := f.svc.Read(ctx, change.ReadRequest{Doc: tc.doc, Editions: []model.EditionKey{de}})
			require.NoError(t, err)
			for _, b := range page.Blocks {
				ed, ok := b.Editions["de"]
				require.True(t, ok, "the German file pairs with block %s", b.Ref.Block)
				before[b.Ref.Block] = ed.Rev
			}

			res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{removeOp(tc.doc, tc.block, de, before[tc.block])}}, person)
			require.NoError(t, err)
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			for block, rev := range before {
				ed, ok := editionOf(t, f.svc, tc.doc, block, de)
				if block == tc.block {
					assert.False(t, ok, "the removed translation reads back absent")
					continue
				}
				require.True(t, ok, "the translation of %s still pairs with its block", block)
				assert.Equal(t, rev, ed.Rev)
			}

			res, err = f.svc.Apply(ctx, change.Set{Ops: []change.Op{
				setOp(change.Ref{Doc: tc.doc, Block: tc.block, Edition: de}, model.AbsentRevision, tc.text),
			}}, person)
			require.NoError(t, err)
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			assert.Equal(t, tc.recreated, f.read(t, "de/"+tc.doc))
			assert.Equal(t, tc.source, f.read(t, tc.doc), "the document keeps its bytes")
			ed, ok := editionOf(t, f.svc, tc.doc, tc.block, de)
			require.True(t, ok, "the translation reads back")
			assert.Equal(t, tc.text, ed.Text)
			assert.Equal(t, res.Ops[0].After, ed.Rev)
			for block, rev := range before {
				if block != tc.block {
					ed, ok := editionOf(t, f.svc, tc.doc, block, de)
					require.True(t, ok)
					assert.Equal(t, rev, ed.Rev, "the translation of %s stays as it was", block)
				}
			}
		})
	}
}

// A file whose last translation was removed holds no block to put a new one
// beside: the translation created there goes last, under the document's key.
func TestFileHome_ATranslationIsCreatedInAFileWithNoBlockLeft(t *testing.T) {
	f := newTargetFixture(t, map[string]string{
		"g.json":    `{"a": "Alpha", "b": "Beta"}` + "\n",
		"de/g.json": "{}\n",
	})
	de := mustEdition(t, "de")
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		setOp(change.Ref{Doc: "g.json", Block: "b", Edition: de}, model.AbsentRevision, "Bet"),
	}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, "{\n  \"b\": \"Bet\"\n}\n", f.read(t, "de/g.json"))
}

// A translation the file's writer cannot add as a block, here because the
// file already holds a value under its key that reads as no block, is
// refused, and the file stays as it was.
func TestFileHome_ACreationTheWriterCannotMakeIsRefused(t *testing.T) {
	const german = "a: 10\nb: Bet\n"
	f := newTargetFixture(t, map[string]string{"g.yaml": "a: Alpha\nb: Beta\n", "de/g.yaml": german})
	de := mustEdition(t, "de")
	_, ok := editionOf(t, f.svc, "g.yaml", "a", de)
	require.False(t, ok, "a number reads as no German for a")
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		setOp(change.Ref{Doc: "g.yaml", Block: "a", Edition: de}, model.AbsentRevision, "Alfa"),
	}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Contains(t, res.Ops[0].Error.Message, "give that entry of de/g.yaml a text value")
	assert.Equal(t, german, f.read(t, "de/g.yaml"), "nothing is written")
}
