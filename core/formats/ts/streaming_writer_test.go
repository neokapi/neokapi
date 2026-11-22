package ts_test

import (
	"bytes"
	"context"
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
	// The gate stops the reader at 256 points through the document. At each
	// stop the writer catches up and waits on the skeleton store, so every
	// sample is taken with every stage of the pipeline waiting.
	peakWriting := func(n int) (peak uint64, size int) {
		pr, sz := genTS(n)
		gate := testutil.NewGate(pr)
		reader := ts.NewReader()
		writer := ts.NewWriter()
		store := format.NewStreamingSkeletonStore()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{URI: "big.ts", SourceLocale: model.LocaleEnglish, Reader: io.NopCloser(gate)}))
		require.NoError(t, writer.SetOutputWriter(io.Discard))
		parts := make(chan *model.Part, 64)
		go func() {
			defer close(parts)
			defer store.CloseWrite()
			defer reader.Close()
			peak = testutil.SamplePeakPair(gate, int64(sz), 256, reader.Read(context.Background()), func(res model.PartResult) {
				if res.Error == nil && res.Part != nil {
					parts <- res.Part
				}
			}, store)
		}()
		require.NoError(t, writer.Write(context.Background(), parts))
		writer.Close()
		return peak, sz
	}
	ps, smallSize := peakWriting(2_000)
	pl, largeSize := peakWriting(40_000)
	testutil.AssertBoundedMemory(t, "ts streaming writer", ps, uint64(smallSize), pl, uint64(largeSize))
}
