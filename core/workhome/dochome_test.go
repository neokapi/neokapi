package workhome_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changetest"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/workhome"
)

// docFixture is a workspace home that keeps whole documents, as a KPZ opened
// for editing keeps its sources, and a change service over it.
type docFixture struct {
	m    *machine
	docs *workhome.Documents
	svc  *change.Service
}

func newDocFixture(t *testing.T, m *machine, files map[string][2]string, beforeSettle func(string)) *docFixture {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	docs := &workhome.Documents{Store: m.st.Heads, Log: m.p, Formats: reg, SourceLocale: "en", TargetLocale: "fr",
		Prefix: "work.kpz!", WorkDir: filepath.Join(t.TempDir(), "work"), BeforeSettle: beforeSettle,
		Seed: func(_ context.Context, key string) (workhome.Seeded, bool, error) {
			f, ok := files[key]
			return workhome.Seeded{Format: f[0], Data: []byte(f[1])}, ok, nil
		}}
	return &docFixture{m: m, docs: docs, svc: change.NewService(filehome.Formats{Registry: reg}, change.OneHome(docs))}
}

func (f *docFixture) bytes(t *testing.T, doc string) []byte {
	t.Helper()
	key, ok := f.docs.Key(doc)
	require.True(t, ok, doc)
	_, data, found, err := f.docs.Head(context.Background(), key)
	require.NoError(t, err)
	if !found {
		return nil
	}
	return data
}

var docFiles = map[string][2]string{
	"a.json": {"json", `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n"},
	"b.json": {"json", `{"title": "Welcome"}` + "\n"},
	"c.po":   {"po", "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\"Language: fr\\n\"\n\nmsgid \"Hello there\"\nmsgstr \"Bonjour\"\n\nmsgid \"Goodbye now\"\nmsgstr \"Au revoir\"\n"},
}

// TestDocuments_Conformance runs the change service's conformance suite on
// documents the workspace home keeps whole: two JSON files and a bilingual
// catalog, each opened for editing from what the host seeds.
func TestDocuments_Conformance(t *testing.T) {
	changetest.Run(t, func(t *testing.T) changetest.Env {
		var hook func(string)
		m := newMachine(t, t.TempDir())
		f := newDocFixture(t, m, docFiles, func(doc string) {
			if hook != nil {
				hook(doc)
			}
		})
		return changetest.Env{
			SetBeforeSettle: func(fn func(string)) { hook = fn },
			Service:         f.svc,
			DocA:            "work.kpz!a.json",
			DocB:            "work.kpz!b.json",
			Translated:      "work.kpz!c.po",
			Snapshot: func(t *testing.T, doc string) []byte {
				return f.bytes(t, doc)
			},
		}
	})
}

// TestDocuments_RecordsTheDocumentAndReadsItBackAfterARebuild: an edit is one
// content.edit operation naming the document as its subject and the blob of
// its bytes; the projection holds the head, and a rebuild from the log
// reaches the same head.
func TestDocuments_RecordsTheDocumentAndReadsItBackAfterARebuild(t *testing.T) {
	ctx := context.Background()
	m := newMachine(t, t.TempDir())
	f := newDocFixture(t, m, docFiles, nil)

	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "work.kpz!a.json"})
	require.NoError(t, err)
	require.NotEmpty(t, page.Blocks)
	b := page.Blocks[0]
	text := "Hello, edited"
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev,
		Body: &change.SetContent{Text: &text}}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Record, "the commit is the record")
	require.Len(t, res.Docs, 1)
	assert.Equal(t, workhome.Name, res.Docs[0].Home)
	assert.True(t, res.Docs[0].Written)

	head, data, found, err := f.docs.Head(ctx, "a.json")
	require.NoError(t, err)
	require.True(t, found)
	assert.Contains(t, string(data), "Hello, edited")
	assert.Equal(t, workhome.DocumentRevision(data), head.Rev)
	assert.Equal(t, *res.Record, head.Op)
	assert.Equal(t, "json", head.Format)

	_, err = m.p.Rebuild(ctx)
	require.NoError(t, err)
	again, _, found, err := f.docs.Head(ctx, "a.json")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, head.Rev, again.Rev)
	assert.Equal(t, head.Op, again.Op)
	assert.Equal(t, head.Blob, again.Blob)
}

// TestDocuments_TwoMachinesConvergeOnOneHead: two machines that edit one
// document from one head reach the same head once their logs meet, and the
// write that sorts later is divergent on both.
func TestDocuments_TwoMachinesConvergeOnOneHead(t *testing.T) {
	ctx := context.Background()
	a, b := newMachine(t, t.TempDir()), newMachine(t, t.TempDir())
	fa := newDocFixture(t, a, docFiles, nil)
	fb := newDocFixture(t, b, docFiles, nil)
	_, err := fa.docs.Put(ctx, "b.json", "json", []byte(docFiles["b.json"][1]), person, "open")
	require.NoError(t, err)
	mergeInto(t, b, a)

	_, err = fa.docs.Put(ctx, "b.json", "json", []byte(`{"title": "Welcome from A"}`+"\n"), person, "apply")
	require.NoError(t, err)
	_, err = fb.docs.Put(ctx, "b.json", "json", []byte(`{"title": "Welcome from B"}`+"\n"), person, "apply")
	require.NoError(t, err)
	mergeInto(t, b, a)
	mergeInto(t, a, b)

	ha, da, _, err := fa.docs.Head(ctx, "b.json")
	require.NoError(t, err)
	hb, db, _, err := fb.docs.Head(ctx, "b.json")
	require.NoError(t, err)
	assert.Equal(t, ha.Rev, hb.Rev)
	assert.Equal(t, string(da), string(db))
	require.Len(t, ha.Divergent, 1, "the write that sorts later did not advance the head")
	assert.Equal(t, ha.Divergent, hb.Divergent)
	assert.True(t, strings.Contains(string(da), "from A") || strings.Contains(string(da), "from B"))
}
