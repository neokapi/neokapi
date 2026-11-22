//go:build unix

package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

const convDestInput = `{"a": "Hello"}`

// A conversion to /dev/null is written directly: no file can be created
// beside it.
func TestConvToDevNull(t *testing.T) {
	app := newToolboxApp(t)
	in := writeToolboxFile(t, t.TempDir(), "en.json", convDestInput)
	require.NoError(t, app.convertDocument(context.Background(), in, "json", "", os.DevNull))
}

// fifo creates a named pipe and returns its path and a function that returns
// everything written to it once the writer has closed it. A conversion that
// never writes into the pipe fails the test after a timeout rather than
// hanging it.
func fifo(t *testing.T) (string, func() string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pipe")
	require.NoError(t, syscall.Mkfifo(path, 0o600))
	got := make(chan string, 1)
	go func() {
		r, err := os.Open(path)
		if err != nil {
			got <- "open: " + err.Error()
			return
		}
		defer r.Close()
		b, _ := io.ReadAll(r)
		got <- string(b)
	}()
	return path, func() string {
		t.Helper()
		select {
		case b := <-got:
			return b
		case <-time.After(10 * time.Second):
			// Unblock the reader, which still waits for a writer.
			if w, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
				_ = w.Close()
			}
			t.Fatal("nothing was written into the pipe")
			return ""
		}
	}
}

// A conversion to a named pipe, as a shell's process substitution
// (`-o >(cat)`) can pass one, is written directly into the pipe.
func TestConvToAProcessSubstitution(t *testing.T) {
	app := newToolboxApp(t)
	in := writeToolboxFile(t, t.TempDir(), "en.json", convDestInput)
	path, got := fifo(t)
	require.NoError(t, app.convertDocument(context.Background(), in, "json", "", path))
	assert.JSONEq(t, convDestInput, got())
}

// A conversion to /dev/stdout is written to the process's standard output,
// here the write end of a pipe, which only takes writes.
func TestConvToDevStdout(t *testing.T) {
	app := newToolboxApp(t)
	in := writeToolboxFile(t, t.TempDir(), "en.json", convDestInput)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	got := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		got <- string(b)
	}()
	saved, err := syscall.Dup(1)
	require.NoError(t, err)
	require.NoError(t, syscall.Dup2(int(w.Fd()), 1))
	convErr := app.convertDocument(context.Background(), in, "json", "", "/dev/stdout")
	require.NoError(t, syscall.Dup2(saved, 1))
	require.NoError(t, syscall.Close(saved))
	require.NoError(t, w.Close())
	require.NoError(t, convErr)
	assert.JSONEq(t, convDestInput, <-got)
}

// A writable file in a directory where no new file can be created is written
// in place.
func TestConvToAFileInAReadOnlyDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root creates files in a read-only directory")
	}
	app := newToolboxApp(t)
	in := writeToolboxFile(t, t.TempDir(), "en.json", convDestInput)
	dir := t.TempDir()
	out := writeToolboxFile(t, dir, "out.json", "OLD")
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	require.NoError(t, app.convertDocument(context.Background(), in, "json", "", out))
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.JSONEq(t, convDestInput, string(got))
}

// panickingWriter writes part of a document and panics.
type panickingWriter struct{ format.BaseFormatWriter }

func (w *panickingWriter) Write(_ context.Context, parts <-chan *model.Part) error {
	for range parts {
	}
	_, _ = io.WriteString(w.Output, "PARTIAL")
	panic("writer failed mid-write")
}

// A writer that panics mid-write leaves an existing output as it was, and no
// temporary file behind.
func TestConvPanicKeepsAnExistingOutput(t *testing.T) {
	app := newToolboxApp(t)
	app.FormatReg.RegisterWriter("panicky", func() format.DataFormatWriter {
		return &panickingWriter{format.BaseFormatWriter{FormatName: "panicky"}}
	})
	dir := t.TempDir()
	in := writeToolboxFile(t, dir, "en.json", convDestInput)
	out := writeToolboxFile(t, dir, "keep.txt", "USER DATA")

	func() {
		defer func() { require.NotNil(t, recover(), "the writer panicked") }()
		_ = app.convertDocument(context.Background(), in, registry.FormatID("panicky"), "", out)
	}()
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "USER DATA", string(got))
	assertNoStagedOutput(t, dir)
}
