package arb_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/arb"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// arbStreamingRoundtrip reads input from a temp file (so the two-pass streaming
// path activates) with a file-backed skeleton store, and reconstructs it.
func arbStreamingRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "app.arb")
	require.NoError(t, os.WriteFile(path, []byte(input), 0o644))
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := arb.NewReader()
	writer := arb.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, &model.RawDocument{URI: path, SourceLocale: model.LocaleEnglish, Reader: f}))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

func TestStreaming_ByteExactARB(t *testing.T) {
	cases := map[string]string{
		"simple":   "{\n  \"@@locale\": \"en\",\n  \"appTitle\": \"Hello World\"\n}\n",
		"metadata": "{\n  \"@@locale\": \"en\",\n  \"greeting\": \"Hi \\\"there\\\"\",\n  \"@greeting\": { \"description\": \"a greeting\" }\n}\n",
		"icu":      "{\n  \"itemCount\": \"{count, plural, one {# item} other {# items}}\",\n  \"@itemCount\": { \"placeholders\": { \"count\": { \"type\": \"int\" } } }\n}\n",
		"unicode":  "{\n  \"emoji\": \"café 😀 end\"\n}\n",
		"multi":    "{\n  \"a\": \"one\",\n  \"b\": \"two\",\n  \"@b\": { \"description\": \"the second\" },\n  \"c\": \"three\"\n}\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			streamed := arbStreamingRoundtrip(t, in)
			// The streaming path must round-trip byte-exact, matching the
			// buffered path (the established behavior).
			assert.Equal(t, arbRoundtripWithSkeleton(t, in), streamed, "streaming must match buffered output")
			assert.Equal(t, in, streamed, "streaming round-trip must be byte-exact")
		})
	}
}

func genARB(t *testing.T, n int) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "big.arb")
	f, err := os.Create(path)
	require.NoError(t, err)
	size := 0
	w := func(s string) {
		_, werr := f.WriteString(s)
		require.NoError(t, werr)
		size += len(s)
	}
	// No @<id> descriptions: arb's partial bound keeps the O(messages) metadata
	// (descriptions/placeholder hints) but not the content bytes/values, so a
	// description-free document is fully bounded. Description-heavy documents are
	// bounded only by their metadata (a documented limitation).
	w("{\n  \"@@locale\": \"en\"")
	for i := range n {
		w(fmt.Sprintf(",\n  \"msg%d\": \"Message number %d with several words of content for the bench\"", i, i))
	}
	w("\n}\n")
	require.NoError(t, f.Close())
	return path, size
}

func TestStreaming_BoundedMemoryARB(t *testing.T) {
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
	peakReading := func(n int) (peak uint64, size int) {
		path, sz := genARB(t, n)
		f, err := os.Open(path)
		require.NoError(t, err)
		reader := arb.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{URI: path, SourceLocale: model.LocaleEnglish, Reader: f}))
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
	pl, largeSize := peakReading(40_000)
	testutil.AssertBoundedMemory(t, "arb streaming reader", ps, uint64(smallSize), pl, uint64(largeSize))
}
