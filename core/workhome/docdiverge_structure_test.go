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

// divergeWith opens doc, applies onHead to it through the change service as
// the head, and records data as a write made from the version the document
// opened with: a write that did not land, as a replaced KPZ leaves one.
func divergeWith(t *testing.T, doc, data string, onHead func(t *testing.T, f *docFixture)) (*docFixture, string, string) {
	t.Helper()
	ctx := context.Background()
	f := newDocFixture(t, newMachine(t, t.TempDir()), docFiles, nil)
	_, err := f.svc.Read(ctx, change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	key, _ := f.docs.Key(doc)
	opened, _, _, err := f.docs.Head(ctx, key)
	require.NoError(t, err)
	onHead(t, f)

	seq, err := f.m.p.DocumentSubjectHead(ctx, key)
	require.NoError(t, err)
	_, err = f.m.p.CommitDocument(ctx, workhome.DocCommit{Key: key, Path: doc, Expect: seq, Base: opened.Op,
		Format: opened.Format, Data: []byte(data), Before: opened.Rev, After: workhome.DocumentRevision([]byte(data)),
		Actor: change.Actor{Kind: change.ActorTool, Name: "kpz"}, Origin: "kpz"})
	require.NoError(t, err)
	head, _, _, err := f.docs.Head(ctx, key)
	require.NoError(t, err)
	require.Len(t, head.Divergent, 1)
	return f, key, head.Divergent[0].Op
}

// texts lists the block keys and texts of doc in document order.
func orderOf(t *testing.T, f *docFixture, doc string) ([]string, map[string]string) {
	t.Helper()
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	var keys []string
	texts := map[string]string{}
	for _, b := range page.Blocks {
		keys = append(keys, b.Ref.Block)
		texts[b.Ref.Block] = b.Text
	}
	return keys, texts
}

// setOn sets block key of doc to text, as a person.
func setOn(t *testing.T, f *docFixture, doc, key, text string) {
	t.Helper()
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: doc, Blocks: []string{key}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	b := page.Blocks[0]
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev,
		Body: &change.SetContent{Text: &text}}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// deleteOn removes block key of doc, as a person.
func deleteOn(t *testing.T, f *docFixture, doc, key string) {
	t.Helper()
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: doc, Blocks: []string{key}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: []change.Op{{Kind: change.KindDeleteBlock,
		At: change.Ref{Doc: doc, Block: key}, Body: &change.DeleteBlock{IfMatch: map[string]string{"en": page.Blocks[0].Rev}}}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// insertOn adds block key with text after block after, as a person.
func insertOn(t *testing.T, f *docFixture, doc, after, key, text string) {
	t.Helper()
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: []change.Op{{Kind: change.KindInsertBlock, At: change.Ref{Doc: doc},
		Body: &change.InsertBlock{After: after, Name: key, Editions: map[string]change.Content{"en": {Text: &text}}}}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// TestDocuments_RebaseAddsAndRemovesBlocks: a rebase inserts a block the
// divergent write added, anchored on a neighbour the head still holds and in
// the write's order, and removes a block the write removed, through the
// change service. A block is left contested only when the structure around
// it conflicts: the head changed a removed block, removed both neighbours of
// an added one, added the same key with other content, or the format adds no
// block.
func TestDocuments_RebaseAddsAndRemovesBlocks(t *testing.T) {
	ctx := context.Background()
	const doc = "work.kpz!a.json"
	tests := []struct {
		name      string
		doc       string
		write     string
		onHead    func(t *testing.T, f *docFixture)
		wantOrder []string
		want      map[string]string
		contested []workhome.DocBlock
	}{
		{
			name:      "a block added between two the head holds",
			doc:       doc,
			write:     `{"greeting": "Hello there", "welcome": "Welcome aboard", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
			onHead:    func(t *testing.T, f *docFixture) { setOn(t, f, doc, "greeting", "Hello, edited") },
			wantOrder: []string{"greeting", "welcome", "farewell", "thanks"},
			want:      map[string]string{"greeting": "Hello, edited", "welcome": "Welcome aboard"},
		},
		{
			name:      "two blocks added at the end keep their order",
			doc:       doc,
			write:     `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you", "p": "First", "q": "Second"}` + "\n",
			onHead:    func(t *testing.T, f *docFixture) { setOn(t, f, doc, "greeting", "Hello, edited") },
			wantOrder: []string{"greeting", "farewell", "thanks", "p", "q"},
			want:      map[string]string{"p": "First", "q": "Second"},
		},
		{
			name:      "a block removed that the head left alone",
			doc:       doc,
			write:     `{"greeting": "Hello there", "farewell": "Goodbye now"}` + "\n",
			onHead:    func(t *testing.T, f *docFixture) { setOn(t, f, doc, "greeting", "Hello, edited") },
			wantOrder: []string{"greeting", "farewell"},
			want:      map[string]string{"greeting": "Hello, edited"},
		},
		{
			name:      "a removed block the head changed is contested",
			doc:       doc,
			write:     `{"greeting": "Hello there", "farewell": "Goodbye now"}` + "\n",
			onHead:    func(t *testing.T, f *docFixture) { setOn(t, f, doc, "thanks", "Thanks a lot") },
			wantOrder: []string{"greeting", "farewell", "thanks"},
			contested: []workhome.DocBlock{{Block: "thanks"}},
		},
		{
			name:  "an added block whose neighbours the head removed is contested",
			doc:   doc,
			write: `{"greeting": "Hello there", "welcome": "Welcome aboard", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
			onHead: func(t *testing.T, f *docFixture) {
				deleteOn(t, f, doc, "greeting")
				deleteOn(t, f, doc, "farewell")
			},
			wantOrder: []string{"thanks"},
			contested: []workhome.DocBlock{{Block: "welcome"}},
		},
		{
			name:      "a block the head added under the same key with other content is contested",
			doc:       doc,
			write:     `{"greeting": "Hello there", "welcome": "Welcome aboard", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
			onHead:    func(t *testing.T, f *docFixture) { insertOn(t, f, doc, "greeting", "welcome", "Welcome home") },
			wantOrder: []string{"greeting", "welcome", "farewell", "thanks"},
			want:      map[string]string{"welcome": "Welcome home"},
			contested: []workhome.DocBlock{{Block: "welcome"}},
		},
		{
			name:  "a block added to a catalog whose format adds none is contested",
			doc:   "work.kpz!c.po",
			write: docFiles["c.po"][1] + "\nmsgid \"Thanks\"\nmsgstr \"Merci\"\n",
			onHead: func(t *testing.T, f *docFixture) {
				f.set(t, "work.kpz!c.po", model.EditionKey{Locale: "fr"}, map[string]string{"Bonjour": "Salut"})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, key, op := divergeWith(t, tt.doc, tt.write, tt.onHead)
			rb, err := f.docs.Rebase(ctx, f.svc, key, op, person, "desktop")
			require.NoError(t, err)
			if rb.Result != nil {
				require.Equal(t, change.SetApplied, rb.Result.Status, "%+v", rb.Result.Ops)
			}
			keys, texts := orderOf(t, f, tt.doc)
			if tt.wantOrder != nil {
				assert.Equal(t, tt.wantOrder, keys)
			}
			for k, want := range tt.want {
				assert.Equal(t, want, texts[k], k)
			}
			if tt.doc == "work.kpz!c.po" {
				require.Len(t, rb.Contested, 1, "the catalog's format adds no block")
				assert.Empty(t, rb.Contested[0].Edition)
				return
			}
			assert.Equal(t, tt.contested, rb.Contested)
			head, _, _, err := f.docs.Head(ctx, key)
			require.NoError(t, err)
			if len(tt.contested) == 0 {
				assert.Empty(t, head.Divergent, "the rebase settled the write")
			} else {
				require.Len(t, head.Divergent, 1)
				assert.Equal(t, tt.contested, head.Divergent[0].Contested)
			}
		})
	}
}
