package resx_test

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/resx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamingPairRoundtripRESX runs the reader and writer against a *concurrent*
// streaming skeleton store (as the file-runner does when both sides are
// streaming): the reader emits skeleton entries + blocks while the writer
// consumes them interleaved. It asserts no deadlock and returns the output.
func streamingPairRoundtripRESX(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()

	reader := resx.NewReader()
	writer := resx.NewWriter()
	store := format.NewStreamingSkeletonStore()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)
	require.True(t, store.IsStreaming())

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	partsCh := make(chan *model.Part, 64)
	go func() {
		defer close(partsCh)
		defer store.CloseWrite()
		defer reader.Close()
		for res := range reader.Read(ctx) {
			assert.NoError(t, res.Error)
			if res.Part != nil {
				partsCh <- res.Part
			}
		}
	}()
	require.NoError(t, writer.Write(ctx, partsCh))
	writer.Close()
	return buf.String()
}

// TestStreamingPair_ByteExactRESX asserts the concurrent streaming reader+writer
// pair round-trips byte-exact across the tricky shapes (matching the buffered
// path, which is the established behavior).
func TestStreamingPair_ByteExactRESX(t *testing.T) {
	const header = `<?xml version="1.0" encoding="utf-8"?>
<root>
  <resheader name="resmimetype">
    <value>text/microsoft-resx</value>
  </resheader>
`
	cases := map[string]string{
		"plain":  header + "  <data name=\"Hello\" xml:space=\"preserve\">\n    <value>Hello world</value>\n  </data>\n</root>\n",
		"multi":  header + "  <data name=\"A\" xml:space=\"preserve\">\n    <value>First</value>\n  </data>\n  <data name=\"B\" xml:space=\"preserve\">\n    <value>Second</value>\n  </data>\n</root>\n",
		"entity": header + "  <data name=\"E\" xml:space=\"preserve\">\n    <value>A &amp; B &lt;x&gt;</value>\n  </data>\n</root>\n",
		"typed":  header + "  <data name=\"Icon\" type=\"System.Drawing.Bitmap\">\n    <value>bin</value>\n  </data>\n  <data name=\"R\" xml:space=\"preserve\">\n    <value>Real</value>\n  </data>\n</root>\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, in, streamingPairRoundtripRESX(t, in), "streaming pair must round-trip byte-exact")
		})
	}
}

// TestStreamingPair_BoundedWriterRESX asserts the writer's peak live heap stays
// flat across a 20x document-size change — it does not buffer the whole block
// map. Reader and writer run concurrently against a streaming store, mirroring
// the file-runner's isStreamingPair path.
func TestStreamingPair_BoundedWriterRESX(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	liveHeap := func() uint64 {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}

	peakWriting := func(n int) (peak uint64, size int) {
		pr, sz := genRESX(n)
		reader := resx.NewReader()
		writer := resx.NewWriter()
		store := format.NewStreamingSkeletonStore()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{
			URI: "big.resx", SourceLocale: model.LocaleEnglish, Reader: io.NopCloser(pr),
		}))
		require.NoError(t, writer.SetOutputWriter(io.Discard))

		partsCh := make(chan *model.Part, 64)
		go func() {
			defer close(partsCh)
			defer store.CloseWrite()
			defer reader.Close()
			for res := range reader.Read(context.Background()) {
				if res.Error != nil || res.Part == nil {
					continue
				}
				partsCh <- res.Part
			}
		}()

		stride := max(n/256, 1)
		base := liveHeap()
		// Sample from the writer side: it pulls blocks + consumes skeleton.
		wrapped := make(chan *model.Part)
		go func() {
			defer close(wrapped)
			count := 0
			for p := range partsCh {
				wrapped <- p
				count++
				if count%stride == 0 {
					if h := liveHeap(); h > base && h-base > peak {
						peak = h - base
					}
				}
			}
		}()
		require.NoError(t, writer.Write(context.Background(), wrapped))
		writer.Close()
		return peak, sz
	}

	ps, smallSize := peakWriting(2_000)
	pl, largeSize := peakWriting(40_000)
	t.Logf("resx streaming writer peakΔ: small(%d KiB)=%d KiB, large(%d KiB)=%d KiB",
		smallSize/1024, ps/1024, largeSize/1024, pl/1024)
	assert.Less(t, pl, uint64(largeSize)/4, "streaming writer peak not bounded well below doc size")
	if ps > 0 {
		assert.Less(t, pl, max(ps, uint64(1<<20))*3, "streaming writer peak scaled with input")
	}
}
