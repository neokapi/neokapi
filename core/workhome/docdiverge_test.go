package workhome_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workhome"
)

// TestFoldDocument_SettlesDivergentWrites: the fold lists a write that was
// not staged on the head, a rebase narrows it to the blocks it left contested
// or settles it, a discard drops it, and a person's write settles the blocks
// it decided of a rebased write and leaves a write nobody rebased.
func TestFoldDocument_SettlesDivergentWrites(t *testing.T) {
	greeting := workhome.DocBlock{Block: "greeting"}
	thanks := workhome.DocBlock{Block: "thanks"}
	french := workhome.DocBlock{Block: "greeting", Edition: "fr"}
	open := workhome.DocWrite{Op: "01", Key: "a.json", Before: "", After: "r0", Blob: "b0"}
	head := workhome.DocWrite{Op: "02", Key: "a.json", Base: "01", Before: "r0", After: "rA", Blob: "bA", Writer: true}
	other := workhome.DocWrite{Op: "03", Key: "a.json", Base: "01", Before: "r0", After: "rB", Blob: "bB", Writer: true}

	tests := []struct {
		name      string
		writes    []workhome.DocWrite
		wantOp    string
		wantRev   string
		divergent []workhome.DocDivergence
	}{
		{
			name:      "a write staged on an older head is divergent",
			writes:    []workhome.DocWrite{open, head, other},
			wantOp:    "02",
			wantRev:   "rA",
			divergent: []workhome.DocDivergence{{Op: "03", Before: "r0", After: "rB"}},
		},
		{
			name: "a rebase that leaves blocks contested lists them and keeps the head",
			writes: []workhome.DocWrite{open, head, other,
				{Op: "04", Key: "a.json", Base: "02", Before: "rA", After: "rA", Blob: "bA", Writer: true, Cause: "03",
					Contested: []workhome.DocBlock{greeting, french}}},
			wantOp:    "02",
			wantRev:   "rA",
			divergent: []workhome.DocDivergence{{Op: "03", Before: "r0", After: "rB", Contested: []workhome.DocBlock{greeting, french}}},
		},
		{
			name: "a rebase that leaves nothing contested settles the write",
			writes: []workhome.DocWrite{open, head, other,
				{Op: "04", Key: "a.json", Base: "02", Before: "rA", After: "rA", Blob: "bA", Writer: true, Cause: "03"}},
			wantOp:  "02",
			wantRev: "rA",
		},
		{
			name: "a person's write settles the contested blocks it decided",
			writes: []workhome.DocWrite{open, head, other,
				{Op: "04", Key: "a.json", Base: "02", Before: "rA", After: "rA", Blob: "bA", Writer: true, Cause: "03",
					Contested: []workhome.DocBlock{greeting, french}},
				{Op: "05", Key: "a.json", Base: "02", Before: "rA", After: "rC", Blob: "bC", Writer: true,
					Decided: []workhome.DocBlock{greeting, thanks}}},
			wantOp:    "05",
			wantRev:   "rC",
			divergent: []workhome.DocDivergence{{Op: "03", Before: "r0", After: "rB", Contested: []workhome.DocBlock{french}}},
		},
		{
			name: "deciding the last contested block settles the write",
			writes: []workhome.DocWrite{open, head, other,
				{Op: "04", Key: "a.json", Base: "02", Before: "rA", After: "rA", Blob: "bA", Writer: true, Cause: "03",
					Contested: []workhome.DocBlock{greeting}},
				{Op: "05", Key: "a.json", Base: "02", Before: "rA", After: "rA", Blob: "bA", Writer: true,
					Decided: []workhome.DocBlock{greeting}}},
			wantOp:  "05",
			wantRev: "rA",
		},
		{
			name: "a person's write leaves a write nobody rebased",
			writes: []workhome.DocWrite{open, head, other,
				{Op: "04", Key: "a.json", Base: "02", Before: "rA", After: "rC", Blob: "bC", Writer: true,
					Decided: []workhome.DocBlock{greeting}}},
			wantOp:    "04",
			wantRev:   "rC",
			divergent: []workhome.DocDivergence{{Op: "03", Before: "r0", After: "rB"}},
		},
		{
			name: "a discard drops the write and keeps the head",
			writes: []workhome.DocWrite{open, head, other,
				{Op: "04", Key: "a.json", Base: "02", Before: "rA", After: "rA", Blob: "bA", Writer: true, Cause: "03"},
				{Op: "05", Key: "a.json", Base: "02", Before: "rA", After: "rD", Blob: "bD"}},
			wantOp:  "05",
			wantRev: "rD",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := workhome.FoldDocument(tt.writes)
			assert.Equal(t, tt.wantOp, h.Op)
			assert.Equal(t, tt.wantRev, h.Rev)
			assert.Equal(t, tt.divergent, h.Divergent)
			// Any order of arrival folds to the same head.
			reversed := make([]workhome.DocWrite, len(tt.writes))
			for i, w := range tt.writes {
				reversed[len(tt.writes)-1-i] = w
			}
			assert.Equal(t, h, workhome.FoldDocument(reversed))
		})
	}
}

// diverged is one document two machines edited from one head, with both
// logs merged both ways: each machine holds the same head and lists the
// write that sorts later beside it.
type diverged struct {
	a, b *docFixture
	doc  string
	key  string
	op   string
}

// diverge opens doc on machine a, carries it to machine b, has each machine
// set the blocks named in its map (block text as read, to the new text), and
// merges the logs both ways.
func diverge(t *testing.T, doc string, edition model.EditionKey, onA, onB map[string]string) *diverged {
	t.Helper()
	ctx := context.Background()
	a, b := newMachine(t, t.TempDir()), newMachine(t, t.TempDir())
	fa := newDocFixture(t, a, docFiles, nil)
	fb := newDocFixture(t, b, docFiles, nil)
	_, err := fa.svc.Read(ctx, change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	mergeInto(t, b, a)
	fa.set(t, doc, edition, onA)
	fb.set(t, doc, edition, onB)
	mergeInto(t, b, a)
	mergeInto(t, a, b)

	key, _ := fa.docs.Key(doc)
	ha, _, found, err := fa.docs.Head(ctx, key)
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, ha.Divergent, 1, "the write that sorts later did not land")
	hb, _, _, err := fb.docs.Head(ctx, key)
	require.NoError(t, err)
	require.Equal(t, ha, hb, "both machines fold to one head")
	return &diverged{a: fa, b: fb, doc: doc, key: key, op: ha.Divergent[0].Op}
}

// set gives each block whose text in edition is a key of texts the text it
// maps to, as one person's change set.
func (f *docFixture) set(t *testing.T, doc string, edition model.EditionKey, texts map[string]string) {
	t.Helper()
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: doc, Editions: []model.EditionKey{edition}})
	require.NoError(t, err)
	var ops []change.Op
	for _, b := range page.Blocks {
		ref, rev, text := b.Ref, b.Rev, b.Text
		if !edition.IsZero() {
			k, _ := edition.MarshalText()
			ed, ok := b.Editions[string(k)]
			if !ok {
				continue
			}
			ref.Edition, rev, text = edition, ed.Rev, ed.Text
		}
		next, ok := texts[text]
		if !ok {
			continue
		}
		ops = append(ops, change.Op{Kind: change.KindSetContent, At: ref, IfMatch: rev, Body: &change.SetContent{Text: &next}})
	}
	require.Len(t, ops, len(texts), "every block to set was found")
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: ops}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// texts reads the text of every block of doc in edition, by block key.
func (f *docFixture) texts(t *testing.T, doc string, edition model.EditionKey) (map[string]string, *change.Page) {
	t.Helper()
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: doc, Editions: []model.EditionKey{edition}})
	require.NoError(t, err)
	out := map[string]string{}
	for _, b := range page.Blocks {
		if edition.IsZero() {
			out[b.Ref.Block] = b.Text
			continue
		}
		k, _ := edition.MarshalText()
		if ed, ok := b.Editions[string(k)]; ok {
			out[b.Ref.Block] = ed.Text
		}
	}
	return out, page
}

// TestDocuments_RebaseCarriesADivergentWriteOver: a read reports a write that
// did not land, and a rebase applies its changes to the head, block by block.
// A block the head changed too stays contested until a person decides it,
// whichever wording they keep; a rebase of edits to different blocks settles
// the write at once. The other machine reaches the same head once the logs
// meet again.
func TestDocuments_RebaseCarriesADivergentWriteOver(t *testing.T) {
	ctx := context.Background()
	fr := model.EditionKey{Locale: "fr"}
	tests := []struct {
		name    string
		doc     string
		edition model.EditionKey
		onA     map[string]string
		onB     map[string]string
		// contested is how many blocks the rebase leaves contested, and
		// decide, for a contested block, which wording the person keeps:
		// "held", "other" or a new one.
		contested int
		decide    string
		// want are the texts the document ends with, by the text it opened
		// with, where the outcome does not depend on which write sorts later.
		want map[string]string
	}{
		{
			name: "edits to different blocks", doc: "work.kpz!a.json",
			onA:  map[string]string{"Hello there": "Hello from A"},
			onB:  map[string]string{"Goodbye now": "Goodbye from B"},
			want: map[string]string{"Hello there": "Hello from A", "Goodbye now": "Goodbye from B", "Thank you": "Thank you"},
		},
		{
			name: "one block both changed, the held wording kept", doc: "work.kpz!a.json",
			onA:       map[string]string{"Hello there": "Hello from A", "Thank you": "Thanks from A"},
			onB:       map[string]string{"Hello there": "Hello from B", "Goodbye now": "Goodbye from B"},
			contested: 1, decide: "held",
		},
		{
			name: "one block both changed, the other wording used", doc: "work.kpz!a.json",
			onA:       map[string]string{"Hello there": "Hello from A"},
			onB:       map[string]string{"Hello there": "Hello from B"},
			contested: 1, decide: "other",
		},
		{
			name: "a translation in a bilingual catalog, a new wording written", doc: "work.kpz!c.po", edition: fr,
			onA:       map[string]string{"Bonjour": "Bonjour de A"},
			onB:       map[string]string{"Bonjour": "Bonjour de B", "Au revoir": "Au revoir de B"},
			contested: 1, decide: "Salut",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diverge(t, tt.doc, tt.edition, tt.onA, tt.onB)
			_, page := d.a.texts(t, tt.doc, tt.edition)
			require.Len(t, page.Divergent, 1, "a read reports the write that did not land")
			assert.Equal(t, d.op, page.Divergent[0].Op)
			assert.Empty(t, page.Divergent[0].Contested, "nothing is contested before a rebase")

			rb, err := d.a.docs.Rebase(ctx, d.a.svc, d.key, d.op, person, "desktop")
			require.NoError(t, err)
			require.Len(t, rb.Contested, tt.contested)
			if rb.Result != nil {
				require.Equal(t, change.SetApplied, rb.Result.Status, "%+v", rb.Result.Ops)
			}

			texts, page := d.a.texts(t, tt.doc, tt.edition)
			if tt.contested == 0 {
				assert.Empty(t, page.Divergent, "the rebase settled the write")
				for from, want := range tt.want {
					assert.Containsf(t, texts, keyOf(t, d, tt.edition, from), "block %q", from)
					assert.Equal(t, want, texts[keyOf(t, d, tt.edition, from)])
				}
			} else {
				require.Len(t, page.Divergent, 1, "a rebase that leaves a block contested keeps the write listed")
				require.Len(t, page.Divergent[0].Contested, 1)
				at := page.Divergent[0].Contested[0]
				assert.Equal(t, tt.edition, at.Edition)
				// Every change the write made to a block the head left alone
				// was carried over.
				for from, to := range tt.onB {
					k := keyOf(t, d, tt.edition, from)
					if k != at.Block && tt.onA[from] == "" {
						assert.Equal(t, to, texts[k], "carried over: %q", from)
					}
				}
				for from, to := range tt.onA {
					k := keyOf(t, d, tt.edition, from)
					if k != at.Block && tt.onB[from] == "" {
						assert.Equal(t, to, texts[k], "kept: %q", from)
					}
				}

				other, err := d.a.docs.DivergentWording(ctx, d.key, d.op, []workhome.DocBlock{{Block: at.Block, Edition: editionName(tt.edition)}})
				require.NoError(t, err)
				otherText := model.RunsEditText(other[workhome.DocBlock{Block: at.Block, Edition: editionName(tt.edition)}].Runs)
				held := texts[at.Block]
				assert.NotEqual(t, held, otherText)

				choice := tt.decide
				switch tt.decide {
				case "held":
					choice = held
				case "other":
					choice = otherText
				}
				var rev string
				for _, b := range page.Blocks {
					if b.Ref.Block != at.Block {
						continue
					}
					rev = b.Rev
					if !tt.edition.IsZero() {
						k, _ := tt.edition.MarshalText()
						rev = b.Editions[string(k)].Rev
					}
				}
				res, err := d.a.svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: at, IfMatch: rev,
					Body: &change.SetContent{Text: &choice}}}}, person)
				require.NoError(t, err)
				require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

				texts, page = d.a.texts(t, tt.doc, tt.edition)
				assert.Empty(t, page.Divergent, "deciding the last contested block settles the write")
				assert.Equal(t, choice, texts[at.Block])
			}

			// The rebase and the decision travel: the other machine reaches the
			// same head.
			mergeInto(t, d.b.m, d.a.m)
			ha, da, _, err := d.a.docs.Head(ctx, d.key)
			require.NoError(t, err)
			hb, db, _, err := d.b.docs.Head(ctx, d.key)
			require.NoError(t, err)
			assert.Equal(t, ha, hb)
			assert.Equal(t, string(da), string(db))

			// A rebuild from the log reaches the head the fold reached.
			_, err = d.a.m.p.Rebuild(ctx)
			require.NoError(t, err)
			again, _, _, err := d.a.docs.Head(ctx, d.key)
			require.NoError(t, err)
			assert.Equal(t, ha, again)
		})
	}
}

// TestDocuments_DiscardKeepsTheHead: a discard drops the write that did not
// land, the document keeps the head's bytes, and a second discard or rebase
// of the same write finds nothing to settle.
func TestDocuments_DiscardKeepsTheHead(t *testing.T) {
	ctx := context.Background()
	d := diverge(t, "work.kpz!a.json", model.EditionKey{},
		map[string]string{"Hello there": "Hello from A"}, map[string]string{"Goodbye now": "Goodbye from B"})
	before, data, _, err := d.a.docs.Head(ctx, d.key)
	require.NoError(t, err)

	require.NoError(t, d.a.docs.Discard(ctx, d.key, d.op, person, "desktop"))
	after, again, _, err := d.a.docs.Head(ctx, d.key)
	require.NoError(t, err)
	assert.Empty(t, after.Divergent)
	assert.Equal(t, before.Rev, after.Rev)
	assert.Equal(t, before.Op, after.Op, "a discard leaves the head where it is")
	assert.Equal(t, string(data), string(again))

	require.ErrorIs(t, d.a.docs.Discard(ctx, d.key, d.op, person, "desktop"), workhome.ErrNotDivergent)
	_, err = d.a.docs.Rebase(ctx, d.a.svc, d.key, d.op, person, "desktop")
	require.ErrorIs(t, err, workhome.ErrNotDivergent)

	divs, err := d.a.docs.Divergences(ctx)
	require.NoError(t, err)
	assert.Empty(t, divs)
}

// keyOf is the key of the block whose text in edition was from when the
// document opened.
func keyOf(t *testing.T, d *diverged, edition model.EditionKey, from string) string {
	t.Helper()
	fx := newDocFixture(t, newMachine(t, t.TempDir()), docFiles, nil)
	texts, _ := fx.texts(t, d.doc, edition)
	for k, text := range texts {
		if text == from {
			return k
		}
	}
	t.Fatalf("no block of %s reads %q", d.doc, from)
	return ""
}

// editionName is how a DocBlock names an edition.
func editionName(k model.EditionKey) string {
	if k.IsZero() {
		return ""
	}
	text, _ := k.MarshalText()
	return string(text)
}
