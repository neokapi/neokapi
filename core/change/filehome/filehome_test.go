package filehome_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changetest"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// newRegistry is the built-in format registry every test reads with.
func newRegistry() *registry.FormatRegistry {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return reg
}

// fixture is a directory of documents and a service over it.
type fixture struct {
	dir string
	reg *registry.FormatRegistry
	svc *change.Service
}

func newFixture(t *testing.T, files map[string]string, opts ...filehome.Options) *fixture {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o640))
	}
	reg := newRegistry()
	o := filehome.Options{LockDir: filepath.Join(t.TempDir(), "locks")}
	if len(opts) > 0 {
		o = opts[0]
		if o.LockDir == "" {
			o.LockDir = filepath.Join(t.TempDir(), "locks")
		}
	}
	home := filehome.New(filehome.DirLayout{Root: dir, Formats: reg, SourceLocale: "en"}, o)
	svc := change.NewService(filehome.Formats{Registry: reg}, change.OneHome(home))
	return &fixture{dir: dir, reg: reg, svc: svc}
}

func (f *fixture) read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(name)))
	require.NoError(t, err)
	return string(b)
}

var person = change.Actor{Kind: change.ActorPerson, Name: "tester"}

func TestFileHome_Conformance(t *testing.T) {
	changetest.Run(t, func(t *testing.T) changetest.Env {
		f := newFixture(t, map[string]string{
			"a.json": `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
			"b.json": `{"title": "Welcome"}` + "\n",
		})
		return changetest.Env{
			Service: f.svc,
			DocA:    "a.json",
			DocB:    "b.json",
			Snapshot: func(t *testing.T, doc string) []byte {
				return []byte(f.read(t, doc))
			},
			Mode: func(t *testing.T, doc string) os.FileMode {
				info, err := os.Stat(filepath.Join(f.dir, doc))
				require.NoError(t, err)
				return info.Mode().Perm()
			},
		}
	})
}

func TestFileHome_EditsAnHTMLParagraphByKeyAndKeepsItsLink(t *testing.T) {
	html := `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Guide</title></head>
<body>
<p>Read the <a href="https://old.example/guide">shop guide</a> before you <b>order</b>.</p>
<p>Second paragraph.</p>
</body></html>
`
	f := newFixture(t, map[string]string{"docs/guide.html": html})
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.html"})
	require.NoError(t, err)
	var para *change.BlockRead
	for i, b := range page.Blocks {
		if strings.Contains(b.Text, "shop guide") {
			para = &page.Blocks[i]
		}
	}
	require.NotNil(t, para, "the read lists the paragraph: %+v", page.Blocks)
	assert.Equal(t, "docs/guide.html", para.Ref.Doc)
	assert.NotEmpty(t, para.Ref.Block)
	require.NotEmpty(t, para.Codes, "the read lists the paragraph's codes")
	var link *change.CodeRead
	for id, c := range para.Codes {
		if c.Attrs["href"] != "" {
			link = &c
			assert.NotContains(t, id, "/", "a paired code is named by the id its opening half shows")
		}
	}
	require.NotNil(t, link, "the link code carries its href: %+v", para.Codes)
	assert.Equal(t, "https://old.example/guide", link.Attrs["href"])
	assert.Contains(t, para.Ops, change.KindReplaceText)

	find, repl := "shop guide", "handbook"
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindReplaceText, At: para.Ref, IfMatch: para.Rev,
		Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: &find, Text: repl}}},
	}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	got := f.read(t, "docs/guide.html")
	assert.Contains(t, got, `<a href="https://old.example/guide">handbook</a>`)
	assert.Equal(t, strings.Replace(html, "shop guide", "handbook", 1), got, "every other byte is kept")
}

func TestFileHome_PreviewRendersADiff(t *testing.T) {
	f := newFixture(t, map[string]string{"a.json": `{"greeting": "Hello there"}` + "\n"})
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "a.json"})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	b := page.Blocks[0]
	text := "Hello, world"
	res, err := f.svc.Apply(ctx, change.Set{Mode: change.ModePreview, Ops: []change.Op{{
		Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev, Body: &change.SetContent{Text: &text},
	}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetPreviewed, res.Status)
	require.Len(t, res.Docs, 1)
	assert.Contains(t, res.Docs[0].Diff, `-{"greeting": "Hello there"}`)
	assert.Contains(t, res.Docs[0].Diff, `+{"greeting": "Hello, world"}`)
	assert.Equal(t, `{"greeting": "Hello there"}`+"\n", f.read(t, "a.json"))
	entries, err := os.ReadDir(f.dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "a preview leaves no temporary file behind")
}

func TestFileHome_AMonolingualDocumentOutsideAProjectHasNoOtherEdition(t *testing.T) {
	f := newFixture(t, map[string]string{"a.json": `{"greeting": "Hello there"}` + "\n"})
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "a.json"})
	require.NoError(t, err)
	b := page.Blocks[0]
	text := "Hei"
	at := b.Ref
	at.Edition = mustEdition(t, "nb")
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: at, IfMatch: "absent", Body: &change.SetContent{Text: &text},
	}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Equal(t, "edition", res.Ops[0].Error.Capability)
}

func TestFileHome_ADocumentOutsideTheRootIsRefused(t *testing.T) {
	f := newFixture(t, map[string]string{"a.json": `{"k": "v"}`})
	_, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: "../outside.json"})
	var ce *change.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, change.CodeInvalid, ce.Code)

	_, err = f.svc.Read(context.Background(), change.ReadRequest{Doc: "missing.json"})
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, change.CodeNotFound, ce.Code)
}

func TestFileHome_PagesAReadWithACursor(t *testing.T) {
	f := newFixture(t, map[string]string{"a.json": `{"a": "One", "b": "Two", "c": "Three"}`})
	ctx := context.Background()
	first, err := f.svc.Read(ctx, change.ReadRequest{Doc: "a.json", Limit: 2})
	require.NoError(t, err)
	require.Len(t, first.Blocks, 2)
	require.NotEmpty(t, first.Next)
	second, err := f.svc.Read(ctx, change.ReadRequest{Doc: "a.json", Limit: 2, Cursor: first.Next})
	require.NoError(t, err)
	require.Len(t, second.Blocks, 1)
	assert.Empty(t, second.Next)
	assert.Equal(t, "Three", second.Blocks[0].Text)

	require.NoError(t, os.WriteFile(filepath.Join(f.dir, "a.json"), []byte(`{"a": "Uno", "b": "Two", "c": "Three"}`), 0o644))
	_, err = f.svc.Read(ctx, change.ReadRequest{Doc: "a.json", Limit: 2, Cursor: first.Next})
	var ce *change.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, change.CodeStale, ce.Code, "a cursor into a document that changed is refused")
}

func mustEdition(t *testing.T, s string) model.EditionKey {
	t.Helper()
	k, err := model.ParseEditionKey(s)
	require.NoError(t, err)
	return k
}
