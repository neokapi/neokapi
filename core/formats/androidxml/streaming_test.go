package androidxml_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/androidxml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genAndroid streams an Android string-resources document with n <string>
// entries through an io.Pipe, so the document never exists as a whole buffer on
// the producer side.
func genAndroid(n int) (io.Reader, int) {
	head := `<?xml version="1.0" encoding="utf-8"?>` + "\n" + `<resources>` + "\n"
	tail := `</resources>` + "\n"
	entry := func(i int) string {
		return fmt.Sprintf("    <!-- entry %d -->\n    <string name=\"key_%d\">Translatable value number %d with several descriptive words that make each entry longer for the memory benchmark</string>\n", i, i, i)
	}
	size := len(head) + len(tail)
	for i := range n {
		size += len(entry(i))
	}
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, head)
		for i := range n {
			if _, err := io.WriteString(pw, entry(i)); err != nil {
				return
			}
		}
		_, _ = io.WriteString(pw, tail)
		_ = pw.Close()
	}()
	return pr, size
}

// TestStreamingReaderBoundedMemory asserts the androidxml reader's peak heap
// stays small and flat across a 20x document-size change when reading from a
// pure stream with a (file-backed) skeleton store.
func TestStreamingReaderBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	peakReading := func(n int) (peak uint64, size int) {
		pr, sz := genAndroid(n)
		reader := androidxml.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		gate := testutil.NewGate(pr)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{
			URI:          "big.xml",
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

	// The large run is 40x the small one (not the usual 20x): the absolute
	// /4 bound below needs headroom over the peak's environment-dependent
	// floor. The retained-delta floor is size-independent (the ratio check
	// below is what enforces scaling) but varies with runner: ~300 KiB
	// locally vs ~1.8 MiB observed under -race -shuffle on CI. At that floor a
	// 7 MiB doc sits at largeSize/3.9, a 3% margin that flakes. Doubling the doc
	// doubles the margin without weakening the bound.
	ps, smallSize := peakReading(2_000)
	pl, largeSize := peakReading(80_000) // 40x
	testutil.AssertBoundedMemory(t, "androidxml streaming reader", ps, uint64(smallSize), pl, uint64(largeSize))
}

// TestStreamingByteExact_Large round-trips a large generated Android resources
// document byte-for-byte through the streaming path.
func TestStreamingByteExact_Large(t *testing.T) {
	pr, _ := genAndroid(5_000)
	input, err := io.ReadAll(pr)
	require.NoError(t, err)

	ctx := context.Background()
	reader := androidxml.NewReader()
	writer := androidxml.NewWriter()
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

	assert.Equal(t, string(input), buf.String(), "large Android XML must round-trip byte-exact through the streaming path")
}
