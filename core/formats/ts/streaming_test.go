package ts_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genTS streams a Qt TS document with n <message> entries through an io.Pipe,
// so the document never exists as a whole buffer on the producer side. Returns
// the reader and the exact byte size it will produce.
func genTS(n int) (io.Reader, int) {
	// Canonical prologue form the ts writer emits (encoding="UTF-8",
	// `<!DOCTYPE TS []>`) so the round-trip is byte-exact. The writer
	// canonicalizes both to mirror Okapi/Woodstox output (see writer.go).
	head := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<!DOCTYPE TS []>` + "\n" +
		`<TS version="2.1" language="fr" sourcelanguage="en">` + "\n" +
		`<context>` + "\n" +
		`    <name>Bench</name>` + "\n"
	tail := `</context>` + "\n" + `</TS>` + "\n"

	msg := func(i int) string {
		return fmt.Sprintf(
			"    <message>\n        <source>Source string number %d with several words</source>\n"+
				"        <translation>Chaîne traduite numéro %d avec plusieurs mots</translation>\n    </message>\n",
			i, i)
	}

	size := len(head) + len(tail)
	for i := range n {
		size += len(msg(i))
	}

	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, head)
		for i := range n {
			if _, err := io.WriteString(pw, msg(i)); err != nil {
				return
			}
		}
		_, _ = io.WriteString(pw, tail)
		_ = pw.Close()
	}()
	return pr, size
}

// TestStreamingReaderBoundedMemory asserts the ts reader's peak heap stays small
// and flat across a 20x document-size change when reading from a pure stream
// with a (file-backed) skeleton store. It does not hold the whole
// document.
func TestStreamingReaderBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	peakReading := func(n int) (peak uint64, size int) {
		pr, sz := genTS(n)
		reader := ts.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		gate := testutil.NewGate(pr)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{
			URI:          "big.ts",
			SourceLocale: model.LocaleEnglish,
			Reader:       io.NopCloser(gate),
		}))

		// The gate stops the reader at 256 points through the document, the
		// same density for both sizes, so the two runs estimate the same
		// bounded working set. Each sample is taken with the reader waiting at
		// the gate and every part it produced received: nothing allocates
		// during the collection. A reader that held the document would keep it
		// live, so the bound below still trips.
		peak = testutil.SamplePeak(gate, int64(sz), 256, reader.Read(context.Background()), func(res model.PartResult) {
			require.NoError(t, res.Error)
		})
		reader.Close()
		return peak, sz
	}

	ps, smallSize := peakReading(2_000)
	pl, largeSize := peakReading(40_000) // 20x
	testutil.AssertBoundedMemory(t, "ts streaming reader", ps, uint64(smallSize), pl, uint64(largeSize))
}

// TestStreamingByteExact_Large round-trips a large generated TS byte-for-byte
// through the streaming path, exercising the capturing reader + incremental
// per-message emission at scale.
func TestStreamingByteExact_Large(t *testing.T) {
	pr, _ := genTS(5_000)
	input, err := io.ReadAll(pr)
	require.NoError(t, err)

	ctx := context.Background()
	reader := ts.NewReader()
	writer := ts.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(string(input), model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, string(input), buf.String(), "large TS must round-trip byte-exact through the streaming path")
}
