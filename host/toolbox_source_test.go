package host

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	neokapiconfig "github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocSourceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello world"), 0o644))

	src, err := openDocSource(context.Background(), path)
	require.NoError(t, err)
	defer src.Close()

	// Detection reads a prefix and seeks back; the document read that follows
	// must still start at byte zero.
	sniff, err := src.seeker()
	require.NoError(t, err)
	head := make([]byte, 5)
	_, err = io.ReadFull(sniff, head)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(head))

	var doc model.RawDocument
	require.NoError(t, src.rawDocument(&doc))

	ra, size, ok := doc.RandomAccess()
	require.True(t, ok, "a file source must offer random access")
	assert.Equal(t, int64(11), size)
	at := make([]byte, 5)
	_, err = ra.ReadAt(at, 6)
	require.NoError(t, err)
	assert.Equal(t, "world", string(at))

	streamed, err := io.ReadAll(doc.Reader)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(streamed))

	// The stream's closer must not take the handle with it — the random-access
	// view and any deferred media accessor are still using it.
	require.NoError(t, doc.Reader.Close())
	full, err := src.bytes()
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(full))
}

func TestDocSourceStdin(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	go func() {
		_, _ = w.Write([]byte("piped content"))
		w.Close()
	}()

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig; r.Close() })

	src, err := openDocSource(context.Background(), StdinName)
	require.NoError(t, err)
	defer src.Close()

	// Standard input is buffered, so it too can be replayed — the bytes are
	// already resident, which is exactly why they double as the random view.
	sniff, err := src.seeker()
	require.NoError(t, err)
	got, err := io.ReadAll(sniff)
	require.NoError(t, err)
	assert.Equal(t, "piped content", string(got))

	var doc model.RawDocument
	require.NoError(t, src.rawDocument(&doc))
	streamed, err := io.ReadAll(doc.Reader)
	require.NoError(t, err)
	assert.Equal(t, "piped content", string(streamed))

	full, err := src.bytes()
	require.NoError(t, err)
	assert.Equal(t, "piped content", string(full))
}

// recordingWriter implements both writer bindings so a test can see which one a
// call site chose.
type recordingWriter struct {
	format.BaseFormatWriter
	sourcePath string
	content    []byte
}

func (w *recordingWriter) SetSourcePath(path string)      { w.sourcePath = path }
func (w *recordingWriter) SetOriginalContent(data []byte) { w.content = data }
func (w *recordingWriter) Write(context.Context, <-chan *model.Part) error {
	return nil
}

func TestBindWriterSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.docx")
	require.NoError(t, os.WriteFile(path, []byte("package bytes"), 0o644))

	t.Run("file binds by path", func(t *testing.T) {
		src, err := openDocSource(context.Background(), path)
		require.NoError(t, err)
		defer src.Close()

		w := &recordingWriter{}
		require.NoError(t, bindWriterSource(w, path, filepath.Join(dir, "out.docx"), src))
		assert.Equal(t, path, w.sourcePath)
		assert.Nil(t, w.content, "a path-bound writer must not also be handed the bytes")
	})

	t.Run("self-overwrite falls back to bytes", func(t *testing.T) {
		// SetOutput truncates the output before Write runs, so a writer told to
		// re-read its own output would rebuild from an empty document.
		src, err := openDocSource(context.Background(), path)
		require.NoError(t, err)
		defer src.Close()

		w := &recordingWriter{}
		require.NoError(t, bindWriterSource(w, path, path, src))
		assert.Empty(t, w.sourcePath)
		assert.Equal(t, "package bytes", string(w.content))
	})

	t.Run("stdin falls back to bytes", func(t *testing.T) {
		src := &docSource{buf: []byte("piped"), size: 5}
		w := &recordingWriter{}
		require.NoError(t, bindWriterSource(w, StdinName, "out.docx", src))
		assert.Empty(t, w.sourcePath)
		assert.Equal(t, "piped", string(w.content))
	})
}

func TestSameFile(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "doc.docx")

	assert.False(t, sameFile(abs, ""), "a stream output can never be the input")
	assert.True(t, sameFile(abs, abs))
	assert.False(t, sameFile(abs, filepath.Join(dir, "other.docx")))

	cwd, err := os.Getwd()
	require.NoError(t, err)
	assert.True(t, sameFile(filepath.Join(cwd, "rel.docx"), "rel.docx"),
		"a relative output naming the input must still count as a self-overwrite")
}

// TestResolveFormatFromSeeker pins that detection over a stream agrees with
// detection over a buffer — the equivalence that lets the toolbox stop reading
// every input whole just to name its format.
func TestResolveFormatFromSeeker(t *testing.T) {
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg, formats.RegisterOptions{
		SchemaReg: schema.NewSchemaRegistry(),
		ConfigReg: neokapiconfig.DefaultRegistry,
	})
	app := &App{FormatReg: formatReg}
	content := []byte(`{"greeting":"hello"}`)

	assert.Equal(t,
		app.ResolveFormatName("catalog.json", content),
		app.resolveFormatFrom("catalog.json", bytes.NewReader(content)))
}
