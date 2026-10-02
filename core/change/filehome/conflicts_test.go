package filehome_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// zipOf is an archive holding files.
func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// applyWithin applies set, failing the test when Apply does not return in
// time: two locks on one file in one process wait for each other in the
// kernel, where nothing can cancel them.
func applyWithin(t *testing.T, svc *change.Service, set change.Set) *change.Result {
	t.Helper()
	type out struct {
		res *change.Result
		err error
	}
	done := make(chan out, 1)
	go func() {
		res, err := svc.Apply(context.Background(), set, person)
		done <- out{res, err}
	}()
	select {
	case o := <-done:
		require.NoError(t, o.err)
		return o.res
	case <-time.After(10 * time.Second):
		t.Fatal("Apply did not return: the change set waits for a lock it holds")
		return nil
	}
}

// TestFileHome_OneFileNamedTwiceIsRefused pins that a change set naming one
// file through two document references (two members of one archive, or a file
// and a link to it) is refused whole: each document would stage the file from
// what it read, so one edit would replace the other, and the second lock on
// the file would wait forever for the first.
func TestFileHome_OneFileNamedTwiceIsRefused(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		docs  [2]string
		file  string
	}{
		{
			name: "two members of one archive",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.zip"), zipOf(t, map[string]string{
					"en.json": `{"title": "Welcome"}` + "\n",
					"fr.json": `{"title": "Hello"}` + "\n",
				}), 0o644))
			},
			docs: [2]string{"bundle.zip!en.json", "bundle.zip!fr.json"},
			file: "bundle.zip",
		},
		{
			name: "a file and a link to it",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"one": "One", "two": "Two"}`+"\n"), 0o644))
				if err := os.Symlink("a.json", filepath.Join(dir, "link.json")); err != nil {
					t.Skipf("symlinks: %v", err)
				}
			},
			docs: [2]string{"a.json", "link.json"},
			file: "a.json",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, nil)
			tc.setup(t, f.dir)
			before, err := os.ReadFile(filepath.Join(f.dir, tc.file))
			require.NoError(t, err)
			ctx := context.Background()
			var ops []change.Op
			for i, doc := range tc.docs {
				page, err := f.svc.Read(ctx, change.ReadRequest{Doc: doc})
				require.NoError(t, err)
				b := page.Blocks[i%len(page.Blocks)]
				ops = append(ops, setOp(b.Ref, b.Rev, b.Text+" edited"))
			}
			for _, mode := range []change.Mode{change.ModePreview, change.ModeApply} {
				res := applyWithin(t, f.svc, change.Set{Mode: mode, Ops: ops})
				require.Equal(t, change.SetRefused, res.Status, "%s: %+v", mode, res.Ops)
				assert.Equal(t, change.OpNotApplied, res.Ops[0].Status)
				require.NotNil(t, res.Ops[1].Error)
				assert.Equal(t, change.CodeInvalid, res.Ops[1].Error.Code)
				assert.Contains(t, res.Ops[1].Error.Message, "one file")
				after, err := os.ReadFile(filepath.Join(f.dir, tc.file))
				require.NoError(t, err)
				assert.Equal(t, before, after, "nothing is written")
			}

			// Each document in a change set of its own lands.
			for _, op := range ops {
				res := applyWithin(t, f.svc, change.Set{Ops: []change.Op{op}})
				require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			}
		})
	}
}

// writeAfterRead is a layout whose edition file's reader, on the read that
// number fire makes, hands the file to another writer once it has read it, as
// a commit that lands between the stage's read of the file and its hash
// would.
type writeAfterRead struct {
	targetLayout
	reads *int
	fire  int
	path  string
	body  string
}

type closeHook struct {
	format.DataFormatReader
	onClose func()
}

func (r closeHook) Close() error {
	err := r.DataFormatReader.Close()
	r.onClose()
	return err
}

func (l writeAfterRead) Locate(ctx context.Context, doc string) (filehome.Doc, error) {
	d, err := l.targetLayout.Locate(ctx, doc)
	if err != nil {
		return d, err
	}
	inner, base := d.EditionFile, d.Format
	d.EditionFile = func(k model.EditionKey) (filehome.EditionFile, bool) {
		f, ok := inner(k)
		if !ok {
			return f, ok
		}
		f.Format = base
		f.Format.NewReader = func() (format.DataFormatReader, error) {
			r, err := base.NewReader()
			if err != nil {
				return nil, err
			}
			*l.reads++
			if *l.reads != l.fire {
				return r, nil
			}
			return closeHook{r, func() { _ = os.WriteFile(l.path, []byte(l.body), 0o644) }}, nil
		}
		return f, true
	}
	return d, nil
}

// TestFileHome_AnEditionFileThatMovesDuringTheStageIsReadAgain pins that the
// file of an edition is hashed before the stage reads it, as the document's
// own file is: another writer's commit between the read and the commit moves
// it away from the recorded digest, the edit is applied again to what that
// writer wrote, and an edit of the same block is refused stale instead of
// overwriting it.
func TestFileHome_AnEditionFileThatMovesDuringTheStageIsReadAgain(t *testing.T) {
	f := newFixture(t, map[string]string{
		"guide.json":    `{"title": "Welcome", "body": "Read this first"}` + "\n",
		"de/guide.json": `{"title": "Willkommen", "body": "Lies das zuerst"}` + "\n",
	})
	reads := 0
	other := `{"title": "Hallo vom anderen Prozess", "body": "Lies das zuerst"}` + "\n"
	// Read 1 is the caller's read; read 2 is the stage's join.
	layout := writeAfterRead{targetLayout: targetLayout{filehome.DirLayout{Root: f.dir, Formats: f.reg, SourceLocale: "en"}},
		reads: &reads, fire: 2, path: filepath.Join(f.dir, "de", "guide.json"), body: other}
	svc := change.NewService(filehome.Formats{Registry: f.reg}, change.OneHome(filehome.New(layout, filehome.Options{LockDir: t.TempDir()})))
	ctx := context.Background()
	de := mustEdition(t, "de")
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "guide.json", Editions: []model.EditionKey{de}})
	require.NoError(t, err)
	var title change.BlockRead
	for _, b := range page.Blocks {
		if b.Text == "Welcome" {
			title = b
		}
	}
	ed := title.Editions["de"]
	require.Equal(t, "Willkommen", ed.Text)
	at := title.Ref
	at.Edition = de

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, ed.Rev, "Herzlich willkommen")}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
	require.NotNil(t, res.Ops[0].Current)
	assert.Equal(t, "Hallo vom anderen Prozess", res.Ops[0].Current.Text)
	assert.Equal(t, other, f.read(t, "de/guide.json"), "the other writer's edit is kept")
}

// TestFileHome_ACommitInterruptedAfterOneFileReportsWhatLanded pins that a
// rename that fails after another file of the change set landed is a partial
// result naming the files written, with the operations on the file that was
// not written not_applied.
func TestFileHome_ACommitInterruptedAfterOneFileReportsWhatLanded(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a directory without write permission stops no rename by root")
	}
	f := newTargetFixture(t, map[string]string{
		"guide.json":    `{"title": "Welcome"}` + "\n",
		"de/guide.json": `{"title": "Willkommen"}` + "\n",
	})
	deDir := filepath.Join(f.dir, "de")
	t.Cleanup(func() { _ = os.Chmod(deDir, 0o755) })
	f.svc = change.NewService(filehome.Formats{Registry: f.reg}, change.OneHome(filehome.New(
		targetLayout{filehome.DirLayout{Root: f.dir, Formats: f.reg, SourceLocale: "en"}},
		filehome.Options{LockDir: t.TempDir(), BeforeSettle: func(string) {
			// The German file's staged copy is in place beside it; its
			// directory stops taking renames before the commit.
			require.NoError(t, os.Chmod(deDir, 0o555))
		}})))
	ctx := context.Background()
	de := mustEdition(t, "de")
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "guide.json", Editions: []model.EditionKey{de}})
	require.NoError(t, err)
	title := page.Blocks[0]
	at := title.Ref
	at.Edition = de

	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{
		setOp(title.Ref, title.Rev, "Welcome aboard"),
		setOp(at, title.Editions["de"].Rev, "Herzlich willkommen"),
	}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetPartial, res.Status, "%+v", res.Ops)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status, "the document's own file landed")
	assert.Equal(t, change.OpNotApplied, res.Ops[1].Status, "the German file did not")
	written := map[string]bool{}
	for _, d := range res.Docs {
		key := d.Doc
		if d.File != "" {
			key = d.File
		}
		written[key] = d.Written
	}
	assert.Equal(t, map[string]bool{"guide.json": true, "de/guide.json": false}, written)
	assert.Equal(t, `{"title": "Welcome aboard"}`+"\n", f.read(t, "guide.json"))
	assert.Equal(t, `{"title": "Willkommen"}`+"\n", f.read(t, "de/guide.json"))
}
