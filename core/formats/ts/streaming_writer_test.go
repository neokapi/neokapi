package ts_test

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func streamingPairRoundtripTS(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()
	reader := ts.NewReader()
	writer := ts.NewWriter()
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

func TestStreamingPair_ByteExactTS(t *testing.T) {
	cases := map[string]string{
		"simple":  "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE TS []>\n<TS version=\"2.1\" language=\"fr\">\n<context>\n    <name>M</name>\n    <message>\n        <source>Hello</source>\n        <translation>Bonjour</translation>\n    </message>\n    <message>\n        <source>Bye</source>\n        <translation type=\"unfinished\"></translation>\n    </message>\n</context>\n</TS>\n",
		"numerus": "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE TS []>\n<TS version=\"2.1\" language=\"fr\">\n<context>\n    <name>P</name>\n    <message numerus=\"yes\">\n        <source>%n file(s)</source>\n        <translation>\n            <numerusform>%n fichier</numerusform>\n            <numerusform>%n fichiers</numerusform>\n        </translation>\n    </message>\n</context>\n</TS>\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, in, streamingPairRoundtripTS(t, in), "streaming pair must round-trip byte-exact")
		})
	}
}

func TestStreamingPair_BoundedWriterTS(t *testing.T) {
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
		pr, sz := genTS(n)
		reader := ts.NewReader()
		writer := ts.NewWriter()
		store := format.NewStreamingSkeletonStore()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{URI: "big.ts", SourceLocale: model.LocaleEnglish, Reader: io.NopCloser(pr)}))
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
	t.Logf("ts streaming writer peakΔ: small(%d KiB)=%d KiB, large(%d KiB)=%d KiB", smallSize/1024, ps/1024, largeSize/1024, pl/1024)
	assert.Less(t, pl, uint64(largeSize)/4, "streaming writer peak not bounded well below doc size")
	if ps > 0 {
		assert.Less(t, pl, ps*3, "streaming writer peak scaled with input")
	}
}
