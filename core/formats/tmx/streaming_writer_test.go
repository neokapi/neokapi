package tmx_test

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/tmx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func streamingPairRoundtripTMX(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()
	reader := tmx.NewReader()
	writer := tmx.NewWriter()
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

func TestStreamingPair_ByteExactTMX(t *testing.T) {
	cases := map[string]string{
		"bilingual": "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tmx version=\"1.4\">\n<header srclang=\"en\" datatype=\"plaintext\"/>\n<body>\n<tu tuid=\"a\"><tuv xml:lang=\"en\"><seg>Hello</seg></tuv><tuv xml:lang=\"fr\"><seg>Bonjour</seg></tuv></tu>\n<tu tuid=\"b\"><tuv xml:lang=\"en\"><seg>World</seg></tuv><tuv xml:lang=\"fr\"><seg>Monde</seg></tuv></tu>\n</body>\n</tmx>\n",
		"inline":    "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tmx version=\"1.4\">\n<header srclang=\"en\" datatype=\"plaintext\"/>\n<body>\n<tu tuid=\"a\"><tuv xml:lang=\"en\"><seg>Tap <ph x=\"1\">%s</ph> now</seg></tuv></tu>\n</body>\n</tmx>\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, in, streamingPairRoundtripTMX(t, in), "streaming pair must round-trip byte-exact")
		})
	}
}

func TestStreamingPair_BoundedWriterTMX(t *testing.T) {
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
		pr, sz := genTMX(n)
		reader := tmx.NewReader()
		writer := tmx.NewWriter()
		store := format.NewStreamingSkeletonStore()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{URI: "big.tmx", SourceLocale: model.LocaleEnglish, Reader: io.NopCloser(pr)}))
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
	testutil.AssertBoundedMemory(t, "tmx streaming writer", ps, uint64(smallSize), pl, uint64(largeSize))
}
