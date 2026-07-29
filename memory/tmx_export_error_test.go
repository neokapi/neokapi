package memory_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errAfterWriter accepts the first n bytes and then fails every subsequent
// write, modelling the disk filling up (or a broken pipe / closed socket)
// partway through an export.
type errAfterWriter struct {
	remaining int
	err       error
	written   int
}

func (w *errAfterWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, w.err
	}
	if len(p) > w.remaining {
		n := w.remaining
		w.remaining = 0
		w.written += n
		return n, w.err
	}
	w.remaining -= len(p)
	w.written += len(p)
	return len(p), nil
}

// bigStore builds a store with enough entries that the export spills well past
// bufio's 4 KiB buffer, so a write failure lands in the middle of the <body>
// rather than on the header.
func bigStore(t *testing.T, entries int) memory.ContentMemory {
	t.Helper()
	tm := memory.NewInMemoryStore()
	for i := range entries {
		id := "entry-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		require.NoError(t, tm.Add(context.Background(), memory.Entry{
			ID:          id,
			HintSrcLang: "en",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "The quick brown fox jumps over the lazy dog, repeatedly."}}},
				"fr": {{Text: &model.TextRun{Text: "Le vif renard brun saute par-dessus le chien paresseux."}}},
			},
			Properties: map[string]string{"origin": "test-fixture"},
			Note:       "a note long enough to push the export past the buffer boundary",
		}))
	}
	return tm
}

// TestExportTMXReportsWriteFailure is the regression test for a silent
// data-loss path: the writes inside the <body> loop were unchecked and the
// deferred bufio Flush discarded its error, so a disk-full partway through an
// export produced a TRUNCATED TMX file — no closing </body></tmx>, entries
// missing — and still returned nil. The caller had no way to know the export
// it just wrote was incomplete.
//
// Two of these cases already passed before the fix, by accident rather than by
// design: the header writes were checked, and one write deep inside
// runsToTMXSeg was too, so a failure that happened to land on either was
// caught. Nothing caught a failure on the trailing bytes, and nothing caught it
// at all for entries whose runs never reach that one checked write (see
// TestExportTMXReportsWriteFailureCodeOnlyEntries).
//
// The contract asserted here: if any byte of the export could not be written,
// ExportTMX returns an error.
func TestExportTMXReportsWriteFailure(t *testing.T) {
	t.Parallel()

	// A full export of this store, for the byte-count reference below.
	tm := bigStore(t, 200)

	full := &errAfterWriter{remaining: 1 << 24, err: errors.New("unused")}
	require.NoError(t, memory.ExportTMX(context.Background(), tm, full, nil))
	total := full.written
	require.Greater(t, total, 8192, "fixture must exceed bufio's buffer so failures land mid-body")

	tests := []struct {
		name  string
		allow int
	}{
		// Fails on the very first flush — before any TMX structure lands.
		{name: "fails immediately", allow: 0},
		// Fails deep inside the <body>, after several buffer flushes have
		// already succeeded. This is the case that used to return nil.
		{name: "fails mid-body", allow: total / 2},
		// Fails on the final Flush, with only the trailing bytes outstanding.
		{name: "fails on final flush", allow: total - 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sentinel := errors.New("no space left on device")
			w := &errAfterWriter{remaining: tt.allow, err: sentinel}

			err := memory.ExportTMX(context.Background(), bigStore(t, 200), w, nil)

			require.Error(t, err, "a truncated export must not report success")
			require.ErrorIs(t, err, sentinel, "the underlying write error must be wrapped, not replaced")
			assert.Less(t, w.written, total, "the failing writer must have received fewer bytes than a full export")
		})
	}
}

// TestExportTMXReportsWriteFailureCodeOnlyEntries covers the body path with no
// checked write on it at all. runsToTMXSeg checks the write it makes for a text
// run, which incidentally caught mid-body failures for ordinary entries — but
// an entry whose variants are pure inline code goes through writePhAsTMX, whose
// fmt.Fprintf was unchecked and which always returned nil. For those entries a
// disk-full anywhere in the <body> was completely silent.
func TestExportTMXReportsWriteFailureCodeOnlyEntries(t *testing.T) {
	t.Parallel()

	codeStore := func() memory.ContentMemory {
		tm := memory.NewInMemoryStore()
		for i := range 400 {
			ph := func() []model.Run {
				return []model.Run{{Ph: &model.PlaceholderRun{
					SubType: "tmx:ph",
					ID:      "ph-1",
					Data:    "<x id='1' ctype='image' equiv-text='a placeholder wide enough to fill the buffer'/>",
				}}}
			}
			require.NoError(t, tm.Add(context.Background(), memory.Entry{
				ID:          "code-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)),
				HintSrcLang: "en",
				Variants:    map[model.LocaleID][]model.Run{"en": ph(), "fr": ph()},
			}))
		}
		return tm
	}

	full := &errAfterWriter{remaining: 1 << 24, err: errors.New("unused")}
	require.NoError(t, memory.ExportTMX(context.Background(), codeStore(), full, nil))
	require.Greater(t, full.written, 8192, "fixture must exceed bufio's buffer")

	sentinel := errors.New("no space left on device")
	w := &errAfterWriter{remaining: full.written / 2, err: sentinel}

	err := memory.ExportTMX(context.Background(), codeStore(), w, nil)

	require.Error(t, err, "a truncated code-only export must not report success")
	require.ErrorIs(t, err, sentinel)
}

// TestExportTMXReportsFlushFailure pins the narrow case the deferred
// `bw.Flush()` hid: everything the export writes fits in bufio's buffer, so
// the ONLY write to the underlying io.Writer is the final flush. Discarding
// its error meant a small export could fail entirely — producing a zero-byte
// file — and still return nil.
func TestExportTMXReportsFlushFailure(t *testing.T) {
	t.Parallel()

	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), memory.Entry{
		ID:          "e1",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello"}}},
			"fr": {{Text: &model.TextRun{Text: "Bonjour"}}},
		},
	}))

	sentinel := errors.New("no space left on device")
	w := &errAfterWriter{remaining: 0, err: sentinel}

	err := memory.ExportTMX(context.Background(), tm, w, nil)

	require.Error(t, err, "a flush that wrote nothing must not report success")
	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, w.written, "nothing reached the writer, so the file would be empty")
}

// TestExportTMXWritesEverythingOnSuccess guards the happy path against the
// error-latching writer introduced alongside the checks above: a latch that
// engaged spuriously would silently drop content, which is the very failure
// this change exists to prevent. io.Discard never fails, so a complete,
// well-formed document must come out.
func TestExportTMXWritesEverythingOnSuccess(t *testing.T) {
	t.Parallel()

	tm := bigStore(t, 50)
	counting := &errAfterWriter{remaining: 1 << 24, err: errors.New("unused")}
	require.NoError(t, memory.ExportTMX(context.Background(), tm, counting, nil))
	assert.Positive(t, counting.written)

	require.NoError(t, memory.ExportTMX(context.Background(), tm, io.Discard, nil))
}
