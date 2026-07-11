package androidxml_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
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
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{
			URI:          "big.xml",
			SourceLocale: model.LocaleEnglish,
			Reader:       io.NopCloser(pr),
		}))

		// liveHeap forces a GC so HeapAlloc reflects the *retained* working set
		// rather than transient per-part garbage that a streaming reader
		// legitimately produces. Sampling raw HeapAlloc (garbage included) made
		// the absolute bound below flaky on slower CI runners, where more
		// uncollected garbage accumulates between samples; live heap is the
		// metric this test actually means. A reader that buffered the whole
		// document would keep it *live*, so this still trips the bound below.
		liveHeap := func() uint64 {
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			return m.HeapAlloc
		}

		// Sample ~256 times regardless of document size (stride scales with n)
		// rather than at a fixed stride. A streaming reader's live heap fluctuates
		// by a bounded, document-size-independent amount (up to ~64 in-flight parts
		// queued in its channel), so a fixed stride samples the 20x-larger run 20x
		// more often and reliably catches that bounded burst while the small run
		// misses it — which made the ps*3 ratio bound flaky for this low-footprint
		// format. Equal sample density makes ps and pl estimate the same bounded
		// working set, so the ratio reflects real scaling, not sampling luck.
		stride := max(n/256, 1)
		base := liveHeap()
		count := 0
		for res := range reader.Read(context.Background()) {
			require.NoError(t, res.Error)
			count++
			if count%stride == 0 {
				if h := liveHeap(); h > base && h-base > peak {
					peak = h - base
				}
			}
		}
		reader.Close()
		return peak, sz
	}

	ps, smallSize := peakReading(2_000)
	pl, largeSize := peakReading(40_000) // 20x
	t.Logf("androidxml streaming reader peakΔ: small(%d KiB doc)=%d KiB, large(%d KiB doc)=%d KiB",
		smallSize/1024, ps/1024, largeSize/1024, pl/1024)

	assert.Less(t, pl, uint64(largeSize)/4, "streaming reader peak not bounded well below doc size")
	if ps > 0 {
		assert.Less(t, pl, ps*3, "streaming reader peak scaled with input (20x doc should not ~20x memory)")
	}
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
