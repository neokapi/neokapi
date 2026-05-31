package xliff_test

import (
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errAfterWriter is an io.Writer that succeeds for the first
// failAfter writes and then returns errWriteFailed on every
// subsequent write. failAfter == 0 fails immediately. It models a
// terminal write failure such as a full disk or a broken pipe.
type errAfterWriter struct {
	failAfter int
	calls     int
}

var errWriteFailed = errors.New("simulated write failure")

func (w *errAfterWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > w.failAfter {
		return 0, errWriteFailed
	}
	return len(p), nil
}

// TestFlush_SurfacesWriteError drives the non-skeleton flush() path
// (synthetic block, no skeleton store) against a writer that fails on
// its first Write and asserts Write returns the error instead of
// swallowing it and reporting success on a truncated file.
func TestFlush_SurfacesWriteError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	block := &model.Block{
		ID:           "1",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: "Hello"}}},
	}
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	for _, failAfter := range []int{0, 1, 3} {
		failAfter := failAfter
		t.Run("failAfter="+string(rune('0'+failAfter)), func(t *testing.T) {
			t.Parallel()
			writer := xliff.NewWriter()
			fw := &errAfterWriter{failAfter: failAfter}
			require.NoError(t, writer.SetOutputWriter(fw))

			ch := testutil.PartsToChannel(parts)
			err := writer.Write(ctx, ch)
			require.Error(t, err, "flush() must surface the terminal write error")
			assert.ErrorIs(t, err, errWriteFailed)
		})
	}
}

// TestWriteFromSkeleton_CompatPath_SurfacesWriteError drives the
// okapi-compat post-process path (a compat flag is on, so the whole
// document is buffered and flushed via the deferred closure) against a
// failing writer and asserts the failed terminal write surfaces rather
// than being dropped by the deferred `_, _ = finalOut.Write(...)`.
func TestWriteFromSkeleton_CompatPath_SurfacesWriteError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="test" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
      </trans-unit>
    </body>
  </file>
</xliff>`

	reader := xliff.NewReader()
	writer := xliff.NewWriter()

	// Enable an okapi-compat post-process flag so writeFromSkeleton
	// buffers the whole document and flushes it through the deferred
	// closure (the path that swallowed the terminal write error).
	cfg := &xliff.Config{}
	cfg.Reset()
	cfg.OkapiCompat.HoistAltTransNotes = true
	writer.SetConfig(cfg)

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Fail the very first Write. In the compat path that first Write is
	// the deferred flush of the whole buffered document, so the error
	// must propagate out of writer.Write.
	fw := &errAfterWriter{failAfter: 0}
	require.NoError(t, writer.SetOutputWriter(fw))

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.Error(t, err, "compat path must surface the deferred terminal write error")
	assert.ErrorIs(t, err, errWriteFailed)
}
