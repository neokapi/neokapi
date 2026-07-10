package messageformat_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamingSkeletonByteExact runs the skeleton round-trip oracle through the
// concurrent (streaming) skeleton store + StreamSkeletonWrite path — the same
// inputs the buffered path asserts in skeleton_test.go — and confirms the
// streaming write is byte-identical. This is the streaming half of the acceptance
// criteria: byte-exact through the streaming path, not just the buffered store.
func TestStreamingSkeletonByteExact(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"Hello world",
		"Hello world\nGoodbye world",
		"Hello world\n",
		"",
		"{count, plural, one {# item} other {# items}}",
		"Hello\r\nWorld",
		"Hello\n\nWorld",
		"{gender, select, male {He} female {She} other {They}} left\nPlain line",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			reader := messageformat.NewReader()
			writer := messageformat.NewWriter()
			store := format.NewStreamingSkeletonStore()
			reader.SetSkeletonStore(store)
			writer.SetSkeletonStore(store)

			require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
			var buf bytes.Buffer
			require.NoError(t, writer.SetOutputWriter(&buf))

			partsCh := make(chan *model.Part, 8)
			go func() {
				defer close(partsCh)
				defer store.CloseWrite()
				defer reader.Close()
				for res := range reader.Read(ctx) {
					assert.NoError(t, res.Error) // assert (not require) — runs off the test goroutine
					if res.Part != nil {
						partsCh <- res.Part
					}
				}
			}()

			require.NoError(t, writer.Write(ctx, partsCh))
			require.NoError(t, writer.Close())
			assert.Equal(t, input, buf.String(), "streaming skeleton round-trip must be byte-exact")
		})
	}
}

// TestStreamingPairDeclared guards the capability markers the file runner probes
// to wire the concurrent skeleton store: without both, the streaming round-trip
// never engages and the bounded-memory guarantee below is moot.
func TestStreamingPairDeclared(t *testing.T) {
	t.Parallel()
	if !format.IsStreamingReader(messageformat.NewReader()) {
		t.Fatal("messageformat reader is not a StreamingReader")
	}
	if !format.IsStreamingWriter(messageformat.NewWriter()) {
		t.Fatal("messageformat writer is not a StreamingWriter")
	}
}

// TestStreamingWriterBoundedMemory drives a full read→write round-trip of a
// large MessageFormat document supplied as a pure stream (an io.Pipe generator —
// the document never exists as a whole buffer) through the streaming reader, a
// concurrent skeleton store, and the streaming writer, and asserts peak heap
// stays a small, flat window independent of document size. With the buffered
// writer the block map + skeleton queue would grow O(document); this is the
// proof the pair genuinely streams and the StreamingReader/StreamingWriter
// markers are not a false claim.
func TestStreamingWriterBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	peakWriting := func(n int) (peak uint64, size int) {
		pr, sz := genLargeMessageFormat(n)
		reader := messageformat.NewReader()
		writer := messageformat.NewWriter()
		store := format.NewStreamingSkeletonStore()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)

		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, &model.RawDocument{
			URI:          "big.mf",
			SourceLocale: model.LocaleEnglish,
			Reader:       io.NopCloser(pr),
		}))
		require.NoError(t, writer.SetOutputWriter(io.Discard))

		// liveHeap forces a GC so HeapAlloc reflects the *retained* working set
		// rather than transient per-part garbage that the streaming round-trip
		// legitimately produces. Sampling raw HeapAlloc (garbage included) made
		// the absolute bound below flaky on slower CI runners, where more
		// uncollected garbage accumulates between samples; live heap is the
		// metric this test actually means. A buffered writer would keep the
		// whole block map + skeleton queue *live*, so this still trips the bound.
		liveHeap := func() uint64 {
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			return m.HeapAlloc
		}
		base := liveHeap()

		// Sample ~256 times regardless of document size (stride scales with n)
		// rather than at a fixed stride. The streaming round-trip's live heap
		// fluctuates by a bounded, document-size-independent amount, so a fixed
		// stride samples the 20x-larger run 20x more often and reliably catches
		// that bounded burst while the small run misses it — which would make the
		// ps*3 ratio bound flaky for this low-footprint format. Equal sample
		// density makes ps and pl estimate the same bounded working set, so the
		// ratio reflects real scaling, not sampling luck.
		stride := max(n/256, 1)

		// Small producer→writer buffer keeps the in-flight window tight so the
		// sampled peak reflects the streaming round-trip's bounded working set,
		// not a transient backlog the reader raced ahead to buffer. A wide buffer
		// let the 20x-longer large run occasionally catch a bounded burst the
		// small run never built up, tripping the ps*3 ratio bound.
		partsCh := make(chan *model.Part, 8)
		go func() {
			defer close(partsCh)
			defer store.CloseWrite()
			defer reader.Close()
			count := 0
			for res := range reader.Read(ctx) {
				if res.Error != nil || res.Part == nil {
					continue
				}
				partsCh <- res.Part
				count++
				if count%stride == 0 {
					if h := liveHeap(); h > base && h-base > peak {
						peak = h - base
					}
				}
			}
		}()

		require.NoError(t, writer.Write(ctx, partsCh))
		require.NoError(t, writer.Close())
		return peak, sz
	}

	ps, smallSize := peakWriting(10_000)
	pl, largeSize := peakWriting(200_000) // 20x
	t.Logf("streaming writer peakΔ: small(%d KiB)=%d KiB, large(%d KiB)=%d KiB",
		smallSize/1024, ps/1024, largeSize/1024, pl/1024)

	if pl > uint64(largeSize)/4 {
		t.Errorf("streaming write round-trip peak %d B is not bounded well below doc size %d B", pl, largeSize)
	}
	if ps > 0 && pl > ps*3 {
		t.Errorf("streaming write round-trip peak scaled with input: small=%d B large=%d B (20x doc -> %.1fx)", ps, pl, float64(pl)/float64(ps))
	}
}

// genLargeMessageFormat emits a MessageFormat document of n one-per-line literal
// messages on demand via an io.Pipe, so the document never exists as a whole
// buffer. Each line is a branchless leaf, so the reader emits it as a single
// skeleton ref + block — the simple, per-line streaming path.
func genLargeMessageFormat(n int) (io.Reader, int) {
	line := func(i int) string {
		return fmt.Sprintf("translatable message number %d with several words", i)
	}
	size := 0
	for i := range n {
		size += len(line(i))
		if i < n-1 {
			size++ // '\n'
		}
	}
	pr, pw := io.Pipe()
	go func() {
		for i := range n {
			if i > 0 {
				_, _ = io.WriteString(pw, "\n")
			}
			_, _ = io.WriteString(pw, line(i))
		}
		pw.Close()
	}()
	return pr, size
}
