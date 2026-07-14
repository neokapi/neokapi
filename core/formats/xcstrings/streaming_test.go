package xcstrings_test

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
	"github.com/neokapi/neokapi/core/formats/xcstrings"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xcStreamingRoundtrip reads input from a temp file (so the per-entry streaming
// path activates) with a file-backed skeleton store, and reconstructs it.
func xcStreamingRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "Localizable.xcstrings")
	require.NoError(t, os.WriteFile(path, []byte(input), 0o644))
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := xcstrings.NewReader()
	writer := xcstrings.NewWriter()
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

func TestStreaming_ByteExactXCStrings(t *testing.T) {
	cases := map[string]string{
		"simple":      "{\n  \"sourceLanguage\" : \"en\",\n  \"strings\" : {\n    \"Cancel\" : {\n      \"localizations\" : {\n        \"de\" : {\n          \"stringUnit\" : {\n            \"state\" : \"translated\",\n            \"value\" : \"Abbrechen\"\n          }\n        }\n      }\n    }\n  },\n  \"version\" : \"1.0\"\n}\n",
		"comment":     "{\n  \"sourceLanguage\" : \"en\",\n  \"strings\" : {\n    \"Hello\" : {\n      \"comment\" : \"a greeting\",\n      \"localizations\" : {\n        \"fr\" : {\n          \"stringUnit\" : {\n            \"state\" : \"translated\",\n            \"value\" : \"Bonjour\"\n          }\n        }\n      }\n    }\n  },\n  \"version\" : \"1.0\"\n}\n",
		"plural":      "{\n  \"sourceLanguage\" : \"en\",\n  \"strings\" : {\n    \"%d items\" : {\n      \"localizations\" : {\n        \"en\" : {\n          \"variations\" : {\n            \"plural\" : {\n              \"one\" : {\n                \"stringUnit\" : {\n                  \"state\" : \"translated\",\n                  \"value\" : \"%d item\"\n                }\n              },\n              \"other\" : {\n                \"stringUnit\" : {\n                  \"state\" : \"translated\",\n                  \"value\" : \"%d items\"\n                }\n              }\n            }\n          }\n        }\n      }\n    }\n  },\n  \"version\" : \"1.0\"\n}\n",
		"multi_entry": "{\n  \"sourceLanguage\" : \"en\",\n  \"strings\" : {\n    \"A\" : {\n      \"localizations\" : {\n        \"de\" : {\n          \"stringUnit\" : {\n            \"state\" : \"translated\",\n            \"value\" : \"Ah\"\n          }\n        }\n      }\n    },\n    \"B\" : {\n      \"localizations\" : {\n        \"de\" : {\n          \"stringUnit\" : {\n            \"state\" : \"translated\",\n            \"value\" : \"Beh\"\n          }\n        }\n      }\n    }\n  },\n  \"version\" : \"1.0\"\n}\n",
		"stale":       "{\n  \"sourceLanguage\" : \"en\",\n  \"strings\" : {\n    \"Old\" : {\n      \"extractionState\" : \"stale\",\n      \"localizations\" : {\n        \"de\" : {\n          \"stringUnit\" : {\n            \"state\" : \"translated\",\n            \"value\" : \"Alt\"\n          }\n        }\n      }\n    }\n  },\n  \"version\" : \"1.0\"\n}\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			streamed := xcStreamingRoundtrip(t, in)
			assert.Equal(t, in, streamed, "streaming round-trip must be byte-exact")
		})
	}
}

func genXCStrings(t *testing.T, n int) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "big.xcstrings")
	f, err := os.Create(path)
	require.NoError(t, err)
	size := 0
	w := func(s string) {
		_, werr := f.WriteString(s)
		require.NoError(t, werr)
		size += len(s)
	}
	w("{\n  \"sourceLanguage\" : \"en\",\n  \"strings\" : {\n")
	for i := range n {
		comma := ",\n"
		if i == n-1 {
			comma = "\n"
		}
		w(fmt.Sprintf("    \"key%d\" : {\n      \"comment\" : \"comment number %d here\",\n      \"localizations\" : {\n        \"de\" : {\n          \"stringUnit\" : {\n            \"state\" : \"translated\",\n            \"value\" : \"Wert nummer %d mit text\"\n          }\n        }\n      }\n    }%s", i, i, i, comma))
	}
	w("  },\n  \"version\" : \"1.0\"\n}\n")
	require.NoError(t, f.Close())
	return path, size
}

func TestStreaming_BoundedMemoryXCStrings(t *testing.T) {
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
		path, sz := genXCStrings(t, n)
		f, err := os.Open(path)
		require.NoError(t, err)
		reader := xcstrings.NewReader()
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
	ps, smallSize := peakReading(1_000)
	pl, largeSize := peakReading(20_000)
	testutil.AssertBoundedMemory(t, "xcstrings streaming reader", ps, uint64(smallSize), pl, uint64(largeSize))
}
