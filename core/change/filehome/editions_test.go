package filehome_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// targetLayout is a directory whose documents keep each translation in a
// file of its own, at <locale>/<name>, as a project's target template puts it.
type targetLayout struct {
	filehome.DirLayout
}

func (l targetLayout) Locate(ctx context.Context, doc string) (filehome.Doc, error) {
	d, err := l.DirLayout.Locate(ctx, doc)
	if err != nil {
		return d, err
	}
	ref := d.Ref
	d.EditionFile = func(k model.EditionKey) (filehome.EditionFile, bool) {
		rel := string(k.Locale) + "/" + filepath.Base(ref)
		return filehome.EditionFile{Ref: rel, Path: filepath.Join(l.Root, filepath.FromSlash(rel))}, true
	}
	return d, nil
}

func newTargetFixture(t *testing.T, files map[string]string, opts ...filehome.Options) *fixture {
	t.Helper()
	f := newFixture(t, files)
	reg := f.reg
	o := filehome.Options{}
	if len(opts) > 0 {
		o = opts[0]
	}
	o.LockDir = t.TempDir()
	home := filehome.New(targetLayout{filehome.DirLayout{Root: f.dir, Formats: reg, SourceLocale: "en"}}, o)
	f.svc = change.NewService(filehome.Formats{Registry: reg}, change.OneHome(home))
	return f
}

func setOp(at change.Ref, ifMatch, text string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: at, IfMatch: ifMatch, Body: &change.SetContent{Text: &text}}
}

func TestFileHome_AnEditionInItsOwnFileIsEditedThroughThatFile(t *testing.T) {
	f := newTargetFixture(t, map[string]string{
		"guide.json":    `{"title": "Welcome", "body": "Read this first"}` + "\n",
		"de/guide.json": `{"title": "Willkommen", "body": "Lies das zuerst"}` + "\n",
	})
	ctx := context.Background()
	de := mustEdition(t, "de")
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "guide.json", Editions: []model.EditionKey{de}})
	require.NoError(t, err)
	var title change.BlockRead
	for _, b := range page.Blocks {
		if b.Text == "Welcome" {
			title = b
		}
	}
	require.NotEmpty(t, title.Rev)
	ed, ok := title.Editions["de"]
	require.True(t, ok, "the read joins the German edition: %+v", title.Editions)
	assert.Equal(t, "Willkommen", ed.Text)

	at := title.Ref
	at.Edition = de
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, ed.Rev, "Herzlich willkommen")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, `{"title": "Herzlich willkommen", "body": "Lies das zuerst"}`+"\n", f.read(t, "de/guide.json"))
	assert.Equal(t, `{"title": "Welcome", "body": "Read this first"}`+"\n", f.read(t, "guide.json"), "the document's own file is untouched")
	var file *change.DocResult
	for i := range res.Docs {
		if res.Docs[i].File == "de/guide.json" {
			file = &res.Docs[i]
		}
	}
	require.NotNil(t, file, "the result names the edition's file: %+v", res.Docs)
	assert.True(t, file.Written)
	assert.Equal(t, "de", file.Edition)
	assert.Equal(t, "guide.json", file.Doc)

	// A stale write to the edition is refused with what the file now says.
	res, err = f.svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, ed.Rev, "Hallo")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status)
	assert.Equal(t, "Herzlich willkommen", res.Ops[0].Current.Text)
}

// capturedRecords keeps every record the service hands its recorder.
type capturedRecords struct{ records []change.Record }

func (c *capturedRecords) Record(_ context.Context, rec change.Record) (string, error) {
	c.records = append(c.records, rec)
	return "op-1", nil
}

func TestFileHome_TheRecordReadsAnEditionInItsOwnFileAsTheChangeLeftIt(t *testing.T) {
	// The home joins the German file into each block for the change and takes
	// it out again once the writer has seen the block. The record still reads
	// the edition, with the stamp the change left on it.
	f := newFixture(t, map[string]string{
		"guide.json":    `{"title": "Welcome"}` + "\n",
		"de/guide.json": `{"title": "Willkommen"}` + "\n",
	})
	rec := &capturedRecords{}
	home := filehome.New(targetLayout{filehome.DirLayout{Root: f.dir, Formats: f.reg, SourceLocale: "en"}}, filehome.Options{LockDir: t.TempDir()})
	svc := change.NewService(filehome.Formats{Registry: f.reg}, change.OneHome(home), change.WithRecorder(rec))
	ctx := context.Background()
	de := mustEdition(t, "de")
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "guide.json", Editions: []model.EditionKey{de}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	at := page.Blocks[0].Ref
	at.Edition = de

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, page.Blocks[0].Editions["de"].Rev, "Herzlich willkommen")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	require.Len(t, rec.records, 1)
	require.Len(t, rec.records[0].Transitions, 1)
	b := rec.records[0].Transitions[0].Block
	require.NotNil(t, b)
	ed, ok := b.Edition(de)
	require.True(t, ok, "the recorded block holds the edition the change wrote")
	assert.Equal(t, "Herzlich willkommen", model.RunsText(ed.Runs))
	assert.Equal(t, model.OriginHuman, ed.Origin.Kind, "with the stamp the change left")
}

func TestFileHome_AMissingEditionFileIsMaterializedFromTheDocument(t *testing.T) {
	f := newTargetFixture(t, map[string]string{
		"guide.json": `{"title": "Welcome", "body": "Read this first"}` + "\n",
	})
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "guide.json"})
	require.NoError(t, err)
	var title change.BlockRead
	for _, b := range page.Blocks {
		if b.Text == "Welcome" {
			title = b
		}
	}
	at := title.Ref
	at.Edition = mustEdition(t, "fr")
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, "absent", "Bienvenue")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, `{"title": "Bienvenue", "body": "Read this first"}`+"\n", f.read(t, "fr/guide.json"),
		"the file is written from the document's skeleton, the untranslated message falling back to the document's own text")
	var file *change.DocResult
	for i := range res.Docs {
		if res.Docs[i].File == "fr/guide.json" {
			file = &res.Docs[i]
		}
	}
	require.NotNil(t, file)
	assert.Empty(t, file.Before, "the change created the file")
	assert.True(t, file.Written)
	info, err := os.Stat(filepath.Join(f.dir, "fr", "guide.json"))
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestFileHome_AnEditToTheDocumentNamesTheEditionsItLeavesStale(t *testing.T) {
	f := newTargetFixture(t, map[string]string{
		"guide.json":    `{"title": "Welcome"}` + "\n",
		"de/guide.json": `{"title": "Willkommen"}` + "\n",
	})
	f.svc = change.NewService(filehome.Formats{Registry: f.reg}, change.OneHome(filehome.New(derivedLayout{targetLayout{filehome.DirLayout{Root: f.dir, Formats: f.reg, SourceLocale: "en"}}}, filehome.Options{LockDir: t.TempDir()})))
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "guide.json"})
	require.NoError(t, err)
	b := page.Blocks[0]
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(b.Ref, b.Rev, "Welcome aboard")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, []change.Invalidation{{Edition: "de", Reason: change.ReasonBasisMoved}}, res.Ops[0].Invalidates)
}

// derivedLayout reports the German edition as existing.
type derivedLayout struct{ targetLayout }

func (l derivedLayout) Locate(ctx context.Context, doc string) (filehome.Doc, error) {
	d, err := l.targetLayout.Locate(ctx, doc)
	d.Derived = []model.EditionKey{{Locale: "de"}}
	return d, err
}

func TestFileHome_AnArchiveMemberIsEditedInPlace(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"locales/en.json": `{"title": "Welcome"}` + "\n",
		"README.txt":      "not a document anyone edits\n",
	} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.zip"), buf.Bytes(), 0o644))

	reg := newRegistry()
	svc := change.NewService(filehome.Formats{Registry: reg},
		change.OneHome(filehome.New(filehome.DirLayout{Root: dir, Formats: reg, SourceLocale: "en"}, filehome.Options{LockDir: t.TempDir()})))
	ctx := context.Background()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "bundle.zip!locales/en.json"})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	assert.Equal(t, "bundle.zip!locales/en.json", page.Doc)
	b := page.Blocks[0]
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(b.Ref, b.Rev, "Welcome aboard")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	zr, err := zip.OpenReader(filepath.Join(dir, "bundle.zip"))
	require.NoError(t, err)
	defer zr.Close()
	got := map[string]string{}
	for _, zf := range zr.File {
		rc, err := zf.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		got[zf.Name] = string(body)
	}
	assert.Equal(t, `{"title": "Welcome aboard"}`+"\n", got["locales/en.json"])
	assert.Equal(t, "not a document anyone edits\n", got["README.txt"])
}

// movingLayout is a layout whose second read of the document rewrites it
// first, as an editor saving the file while the home applies an edit again
// would.
type movingLayout struct {
	filehome.DirLayout
	reads  *int
	moveOn []int
}

func (l movingLayout) Locate(ctx context.Context, doc string) (filehome.Doc, error) {
	d, err := l.DirLayout.Locate(ctx, doc)
	if err != nil {
		return d, err
	}
	inner := d.Format.NewReader
	path := d.Path
	d.Format.NewReader = func() (format.DataFormatReader, error) {
		*l.reads++
		for _, n := range l.moveOn {
			if *l.reads == n {
				body, _ := os.ReadFile(path)
				_ = os.WriteFile(path, append(body, ' '), 0o644)
			}
		}
		return inner()
	}
	return d, nil
}

func TestFileHome_ADocumentThatKeepsMovingIsDocChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"title": "Hello"}`), 0o644))
	reg := newRegistry()
	reads := 0
	// Read 1 is the caller's read, 2 the stage; the file moves before 3, the
	// read that applies the edit again under the lock, and again during it.
	layout := movingLayout{Root: dir, Formats: reg, SourceLocale: "en", reads: &reads, moveOn: []int{3}}
	svc := change.NewService(filehome.Formats{Registry: reg}, change.OneHome(filehome.New(layout, filehome.Options{
		LockDir: t.TempDir(),
		BeforeSettle: func(string) {
			body, _ := os.ReadFile(path)
			require.NoError(t, os.WriteFile(path, append(body, '\n'), 0o644))
		},
	})))
	ctx := context.Background()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "a.json"})
	require.NoError(t, err)
	b := page.Blocks[0]
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(b.Ref, b.Rev, "Hi")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeDocChanged, res.Ops[0].Error.Code)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(body), `{"title": "Hello"}`), "nothing was written: %q", body)
}

func TestFileHome_ADocumentThatMovedOnceIsAppliedAgain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"title": "Hello", "body": "Text"}`), 0o644))
	reg := newRegistry()
	svc := change.NewService(filehome.Formats{Registry: reg}, change.OneHome(filehome.New(
		filehome.DirLayout{Root: dir, Formats: reg, SourceLocale: "en"},
		filehome.Options{LockDir: t.TempDir(), BeforeSettle: func(string) {
			// A person saves another message between the stage and the commit.
			require.NoError(t, os.WriteFile(path, []byte(`{"title": "Hello", "body": "Saved"}`), 0o644))
		}})))
	ctx := context.Background()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "a.json"})
	require.NoError(t, err)
	b := page.Blocks[0]
	require.Equal(t, "Hello", b.Text)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(b.Ref, b.Rev, "Hi")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, `{"title": "Hi", "body": "Saved"}`, string(body), "the edit lands on what the person saved")
}

// TestFileHome_AnExistingEditionFileIsNeverWrittenAfresh pins that an edition
// is written into a file that exists through that file's own skeleton, and
// that an edit needing a block the file does not hold is refused where the
// format's writer adds no block, as Markdown's does: writing it would mean
// rewriting the file from the document's skeleton, and whatever only that
// file holds (an editor's comment, a section of its own) would be gone.
func TestFileHome_AnExistingEditionFileIsNeverWrittenAfresh(t *testing.T) {
	german := "<!-- Redaktion: bitte nicht entfernen -->\n\n# Installieren\n\nA auf Deutsch.\n\nB auf Deutsch.\n\n## Nur Deutsch\n\nX auf Deutsch.\n"
	f := newTargetFixture(t, map[string]string{
		"guide.md":    "# Install\n\nA.\n\nB.\n\nC.\n",
		"de/guide.md": german,
	})
	ctx := context.Background()
	de := mustEdition(t, "de")
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "guide.md", Editions: []model.EditionKey{de}})
	require.NoError(t, err)
	var unpaired, paired change.BlockRead
	for _, b := range page.Blocks {
		switch b.Text {
		case "C.":
			unpaired = b
		case "A.":
			paired = b
		}
	}
	require.NotContains(t, unpaired.Editions, "de", "the German file holds no block for C.")
	require.Contains(t, paired.Editions, "de")
	at := unpaired.Ref
	at.Edition = de

	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, model.AbsentRevision, "C auf Deutsch.")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Contains(t, res.Ops[0].Error.Message, unpaired.Ref.Block)
	assert.Empty(t, res.Ops[0].After, "a refused operation reports no revision it would have made")
	assert.Equal(t, german, f.read(t, "de/guide.md"), "nothing is written")

	// Beside an edit the file can take, the refusal is the creation's alone:
	// the edit is not applied, blocked by it.
	pairedAt := paired.Ref
	pairedAt.Edition = de
	res, err = f.svc.Apply(ctx, change.Set{Ops: []change.Op{
		setOp(pairedAt, paired.Editions["de"].Rev, "A, neu auf Deutsch."),
		setOp(at, model.AbsentRevision, "C auf Deutsch."),
	}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	assert.Equal(t, change.OpRefused, res.Ops[1].Status)
	assert.Equal(t, change.OpNotApplied, res.Ops[0].Status, "%+v", res.Ops[0])
	require.NotNil(t, res.Ops[0].BlockedBy)
	assert.Equal(t, 1, *res.Ops[0].BlockedBy)
	assert.Equal(t, german, f.read(t, "de/guide.md"), "nothing is written")

	at = pairedAt
	res, err = f.svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, paired.Editions["de"].Rev, "A, neu auf Deutsch.")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, strings.Replace(german, "A auf Deutsch.", "A, neu auf Deutsch.", 1), f.read(t, "de/guide.md"),
		"an edit of a block the file holds keeps every other byte, the comment and the German-only section included")
}

// TestFileHome_MaterializeWritesAnEditionFileFromTheDocument pins
// Options.Materialize: an edition file the change writes is written from the
// document's skeleton, so a block the file does not hold yet takes its
// edition, every block the file held keeps its own, a block neither holds
// falls back to the document's text, and what only the file held is gone.
func TestFileHome_MaterializeWritesAnEditionFileFromTheDocument(t *testing.T) {
	files := map[string]string{
		"guide.json":    `{"title": "Welcome", "body": "Read this first", "new": "Fresh", "late": "Later"}` + "\n",
		"de/guide.json": `{"body":"Lies das zuerst","title":"Willkommen","extra":"Nur Deutsch"}` + "\n",
	}
	ctx := context.Background()
	de := mustEdition(t, "de")
	newOp := func(t *testing.T, f *fixture) change.Op {
		t.Helper()
		page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "guide.json", Editions: []model.EditionKey{de}})
		require.NoError(t, err)
		for _, b := range page.Blocks {
			if b.Text == "Fresh" {
				require.NotContains(t, b.Editions, "de", "the German file holds no block for the new message")
				at := b.Ref
				at.Edition = de
				return setOp(at, model.AbsentRevision, "Frisch")
			}
		}
		t.Fatal("no block reads Fresh")
		return change.Op{}
	}

	t.Run("without it the file gains the block and keeps the rest", func(t *testing.T) {
		f := newTargetFixture(t, files)
		res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{newOp(t, f)}}, person)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.Equal(t, `{"body":"Lies das zuerst","new":"Frisch","title":"Willkommen","extra":"Nur Deutsch"}`+"\n", f.read(t, "de/guide.json"),
			"the new key goes after the block before it in the document, and what only the file holds stays")
	})

	t.Run("with it the file follows the document", func(t *testing.T) {
		f := newTargetFixture(t, files, filehome.Options{Materialize: true})
		res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{newOp(t, f)}}, person)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.Equal(t, `{"title": "Willkommen", "body": "Lies das zuerst", "new": "Frisch", "late": "Later"}`+"\n", f.read(t, "de/guide.json"))
		assert.Equal(t, files["guide.json"], f.read(t, "guide.json"), "the document's own file is untouched")
	})
}

// TestFileHome_MaterializeBringsADriftedEditionFileBackToTheDocument pins
// that under Options.Materialize a change set addressing an edition writes its
// file whenever the file no longer holds the document's blocks, even when no
// content changes: the block the document gained lands with the document's
// own text and the block only the file held is gone. A file that still holds
// the document's blocks and gains no content is left as it is.
func TestFileHome_MaterializeBringsADriftedEditionFileBackToTheDocument(t *testing.T) {
	ctx := context.Background()
	de := mustEdition(t, "de")
	for _, tc := range []struct {
		name, doc, edition, want string
		written                  bool
	}{
		{name: "the document gained a block",
			doc:     `{"title": "Welcome", "added": "Brand new"}` + "\n",
			edition: `{"title":"Willkommen"}` + "\n",
			want:    `{"title": "Willkommen", "added": "Brand new"}` + "\n", written: true},
		{name: "the document lost a block",
			doc:     `{"title": "Welcome"}` + "\n",
			edition: `{"title":"Willkommen","gone":"Weg"}` + "\n",
			want:    `{"title": "Willkommen"}` + "\n", written: true},
		{name: "the file holds every block of the document",
			doc:     `{"title": "Welcome"}` + "\n",
			edition: `{"title":"Willkommen"}` + "\n",
			want:    `{"title":"Willkommen"}` + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTargetFixture(t, map[string]string{"guide.json": tc.doc, "de/guide.json": tc.edition}, filehome.Options{Materialize: true})
			page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "guide.json", Editions: []model.EditionKey{de}})
			require.NoError(t, err)
			var title change.BlockRead
			for _, b := range page.Blocks {
				if b.Text == "Welcome" {
					title = b
				}
			}
			at := title.Ref
			at.Edition = de
			res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, title.Editions["de"].Rev, "Willkommen")}}, person)
			require.NoError(t, err)
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
			assert.Equal(t, tc.want, f.read(t, "de/guide.json"))
			var written bool
			for _, d := range res.Docs {
				if d.File == "de/guide.json" {
					written = d.Written
				}
			}
			assert.Equal(t, tc.written, written)
		})
	}
}

// TestFileHome_ThePreviewOfANewTranslationCreatesNoDirectory pins that the
// directory a translation's first file needs is created when the file is
// committed, so a preview, and a change set refused once staged, leave the
// tree as it was.
func TestFileHome_ThePreviewOfANewTranslationCreatesNoDirectory(t *testing.T) {
	f := newTargetFixture(t, map[string]string{
		"guide.json": `{"title": "Welcome", "body": "Read this first"}` + "\n",
	})
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "guide.json"})
	require.NoError(t, err)
	title := page.Blocks[0]
	at := title.Ref
	at.Edition = mustEdition(t, "fr")
	create := setOp(at, model.AbsentRevision, "Bienvenue")

	res, err := f.svc.Apply(ctx, change.Set{Mode: change.ModePreview, Ops: []change.Op{create}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetPreviewed, res.Status, "%+v", res.Ops)
	assert.NoDirExists(t, filepath.Join(f.dir, "fr"), "a preview writes nothing")

	// The term is refused after the French file was staged: this service
	// keeps no terms.
	res, err = f.svc.Apply(ctx, change.Set{Ops: []change.Op{create, {Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "Welcome"}}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	assert.NoDirExists(t, filepath.Join(f.dir, "fr"), "a refusal writes nothing")
	entries, err := os.ReadDir(f.dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no staged file is left behind")

	res, err = f.svc.Apply(ctx, change.Set{Ops: []change.Op{create}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.FileExists(t, filepath.Join(f.dir, "fr", "guide.json"))
}
