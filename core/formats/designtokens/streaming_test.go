package designtokens_test

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
	"github.com/neokapi/neokapi/core/formats/designtokens"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamingRoundtrip reads input from a temp file (so the two-pass streaming
// path activates via os.Open(URI)) and reconstructs it.
func streamingRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "doc.tokens.json")
	require.NoError(t, os.WriteFile(path, []byte(input), 0o644))
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := designtokens.NewReader()
	writer := designtokens.NewWriter()
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

func TestStreaming_ByteExactDesignTokens(t *testing.T) {
	cases := map[string]string{
		"simple":     "{\n  \"color\": {\n    \"primary\": { \"$value\": \"#fff\", \"$type\": \"color\", \"$description\": \"Primary colour\" }\n  }\n}\n",
		"deprecated": "{\n  \"space\": {\n    \"sm\": { \"$value\": \"4px\", \"$type\": \"dimension\", \"$deprecated\": \"use space.small\", \"$description\": \"Small spacing\" }\n  }\n}\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, in, streamingRoundtrip(t, in), "streaming round-trip must be byte-exact")
		})
	}
}

func genTokens(t *testing.T, n int) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "big.tokens.json")
	f, err := os.Create(path)
	require.NoError(t, err)
	size := 0
	w := func(s string) {
		_, werr := f.WriteString(s)
		require.NoError(t, werr)
		size += len(s)
	}
	w("{\n  \"tokens\": {\n")
	for i := range n {
		comma := ",\n"
		if i == n-1 {
			comma = "\n"
		}
		w(fmt.Sprintf("    \"t%d\": { \"$value\": \"#ffffff\", \"$type\": \"color\", \"$description\": \"Token number %d with descriptive prose\" }%s", i, i, comma))
	}
	w("  }\n}\n")
	require.NoError(t, f.Close())
	return path, size
}

func TestStreaming_BoundedMemoryDesignTokens(t *testing.T) {
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
		path, sz := genTokens(t, n)
		f, err := os.Open(path)
		require.NoError(t, err)
		reader := designtokens.NewReader()
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
	t.Logf("designtokens streaming reader peakΔ: small(%d KiB)=%d KiB, large(%d KiB)=%d KiB", smallSize/1024, ps/1024, largeSize/1024, pl/1024)
	assert.Less(t, pl, uint64(largeSize)/4, "streaming reader peak not bounded well below doc size")
	if ps > 0 {
		assert.Less(t, pl, ps*3, "streaming reader peak scaled with input")
	}
}
