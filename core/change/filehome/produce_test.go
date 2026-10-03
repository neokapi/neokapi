package filehome_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
)

func produceBytes(data string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, data)
		return err
	}
}

func TestProduce_CommitsWhatTheRunWrote(t *testing.T) {
	cases := []struct {
		name string
		// existing is the file before the run, nil for none.
		existing *string
		// between runs after the stage and before the commit.
		between func(t *testing.T, path string)
		wantErr error
		want    string
		written bool
	}{
		{name: "a file the run read lands", existing: new("old\n"), want: "new\n", written: true},
		{name: "a new file lands with its directory", want: "new\n", written: true},
		{name: "a file that already holds the bytes is left as it is", existing: new("new\n"), want: "new\n"},
		{
			name: "a file a person saved meanwhile is kept", existing: new("old\n"),
			between: func(t *testing.T, path string) { require.NoError(t, os.WriteFile(path, []byte("saved\n"), 0o640)) },
			wantErr: filehome.ErrMoved, want: "saved\n",
		},
		{
			name: "a file another writer created meanwhile is kept",
			between: func(t *testing.T, path string) {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte("theirs\n"), 0o640))
			},
			wantErr: filehome.ErrMoved, want: "theirs\n",
		},
		{
			name: "a writer that produced the same bytes meanwhile is no conflict", existing: new("old\n"),
			between: func(t *testing.T, path string) { require.NoError(t, os.WriteFile(path, []byte("new\n"), 0o640)) },
			want:    "new\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "fr", "messages.txt")
			if tc.existing != nil {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(*tc.existing), 0o640))
			}
			before, err := filehome.Digest(path)
			require.NoError(t, err)
			home := filehome.New(nil, filehome.Options{LockDir: filepath.Join(t.TempDir(), "locks")})

			p, err := home.Produce(t.Context(), path, before, produceBytes("new\n"))
			require.NoError(t, err)
			defer func() { _ = p.Release() }()
			if tc.existing == nil {
				_, serr := os.Stat(filepath.Dir(path))
				require.ErrorIs(t, serr, os.ErrNotExist, "a stage creates no directory")
			}
			if tc.between != nil {
				tc.between(t, path)
			}
			err = p.Commit(t.Context())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				var moved *filehome.MovedError
				require.ErrorAs(t, err, &moved)
				assert.Equal(t, path, moved.Path)
			} else {
				require.NoError(t, err)
			}
			got, rerr := os.ReadFile(path)
			require.NoError(t, rerr)
			assert.Equal(t, tc.want, string(got))
			assert.Equal(t, tc.written, p.Written())
			assert.Equal(t, before, p.Before())

			entries, _ := os.ReadDir(filepath.Dir(path))
			assert.Len(t, entries, 1, "no staged file is left beside the document")
		})
	}
}

func TestProduce_KeepsTheModeOfTheFileItReplaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not kept on Windows")
	}
	path := filepath.Join(t.TempDir(), "run.sh")
	require.NoError(t, os.WriteFile(path, []byte("old\n"), 0o750))
	require.NoError(t, os.Chmod(path, 0o750))
	before, err := filehome.Digest(path)
	require.NoError(t, err)
	home := filehome.New(nil, filehome.Options{LockDir: filepath.Join(t.TempDir(), "locks")})
	p, err := home.Produce(t.Context(), path, before, produceBytes("new\n"))
	require.NoError(t, err)
	require.NoError(t, p.Commit(t.Context()))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o750), info.Mode().Perm())
}

func TestProduce_AHomeThatWritesNothingStagesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out", "messages.txt")
	home := filehome.New(nil, filehome.Options{LockDir: filepath.Join(t.TempDir(), "locks"), WriteNothing: true})
	p, err := home.Produce(t.Context(), path, "", produceBytes("new\n"))
	require.NoError(t, err)
	want, err := home.Produce(t.Context(), filepath.Join(dir, "elsewhere"), "", produceBytes("new\n"))
	require.NoError(t, err)
	assert.Equal(t, want.After(), p.After(), "the digest is the digest of the bytes")
	require.NoError(t, p.Commit(t.Context()))
	assert.False(t, p.Written())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is staged or written")
}

func TestProduce_AHomeThatWritesNothingStillWritesUnderItsTree(t *testing.T) {
	dir := t.TempDir()
	drafts := filepath.Join(dir, ".kapi", "work", "drafts")
	home := filehome.New(nil, filehome.Options{LockDir: filepath.Join(t.TempDir(), "locks"), WriteNothing: true, WriteUnder: drafts})

	draft := filepath.Join(drafts, "qps", "messages.txt")
	p, err := home.Produce(t.Context(), draft, "", produceBytes("draft\n"))
	require.NoError(t, err)
	require.NoError(t, p.Commit(t.Context()))
	assert.True(t, p.Written(), "a draft is the run's own tree")
	got, err := os.ReadFile(draft)
	require.NoError(t, err)
	assert.Equal(t, "draft\n", string(got))

	content := filepath.Join(dir, "qps", "messages.txt")
	q, err := home.Produce(t.Context(), content, "", produceBytes("draft\n"))
	require.NoError(t, err)
	require.NoError(t, q.Commit(t.Context()))
	assert.False(t, q.Written())
	assert.NoFileExists(t, content, "a file outside the tree is never written")
	assert.NoDirExists(t, filepath.Join(dir, "qps"))
}

func TestProduce_AWriterThatFailsStagesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "messages.txt")
	home := filehome.New(nil, filehome.Options{LockDir: filepath.Join(t.TempDir(), "locks")})
	boom := errors.New("boom")
	_, err := home.Produce(t.Context(), path, "", func(w io.Writer) error {
		_, _ = io.WriteString(w, "half")
		return boom
	})
	require.ErrorIs(t, err, boom)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// A produced document and a change set on one file take the same lock, so a
// commit of one waits for the other's, and the produced document then finds
// the file the change set left.
func TestProduce_TakesTheLockAChangeSetCommitsUnder(t *testing.T) {
	locks := filepath.Join(t.TempDir(), "locks")
	f := newFixture(t, map[string]string{"a.json": `{"greeting": "Hello there"}` + "\n"}, filehome.Options{LockDir: locks})
	path := filepath.Join(f.dir, "a.json")
	before, err := filehome.Digest(path)
	require.NoError(t, err)

	producer := filehome.New(nil, filehome.Options{LockDir: locks})
	p, err := producer.Produce(t.Context(), path, before, produceBytes(`{"greeting": "Bonjour"}`+"\n"))
	require.NoError(t, err)
	defer func() { _ = p.Release() }()

	// A change set staged and settled holds the file's commit lock until it
	// commits.
	home := filehome.New(filehome.DirLayout{Root: f.dir, Formats: f.reg, SourceLocale: "en"}, filehome.Options{LockDir: locks})
	sess, err := home.Open(t.Context(), "a.json")
	require.NoError(t, err)
	defer sess.Close()
	staged, err := sess.Stage(t.Context(), change.Want{Own: true}, &replaceEditor{text: "Hello again"})
	require.NoError(t, err)
	defer func() { _ = staged.Release() }()
	require.NoError(t, staged.Settle(t.Context()))

	committed := make(chan error, 1)
	go func() { committed <- p.Commit(context.Background()) }()
	select {
	case err := <-committed:
		t.Fatalf("the produced document committed while the change set held the file's lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, staged.Commit(t.Context()))
	require.NoError(t, staged.Release())
	select {
	case err = <-committed:
	case <-time.After(10 * time.Second):
		t.Fatal("the produced document never took the lock")
	}
	require.ErrorIs(t, err, filehome.ErrMoved, "the change set landed first, so the run's bytes are refused")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, bytes.Contains(got, []byte("Hello again")), "the change set's edit stands: %s", got)
}

// replaceEditor sets the own edition of the first block to text.
type replaceEditor struct {
	text string
	done bool
}

func (e *replaceEditor) Begin() { e.done = false }
func (e *replaceEditor) Edit(b *model.Block) ([]model.EditionKey, error) {
	if e.done {
		return nil, nil
	}
	e.done = true
	ed, _ := b.Edition(model.EditionKey{})
	ed.Runs = []model.Run{{Text: &model.TextRun{Text: e.text}}}
	b.SetEdition(model.EditionKey{}, ed)
	return []model.EditionKey{{}}, nil
}
func (e *replaceEditor) End() error { return nil }
