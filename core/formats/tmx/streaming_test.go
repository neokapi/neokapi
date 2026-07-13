package tmx_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"testing"
	"unicode/utf16"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/tmx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genTMX streams a bilingual TMX with n <tu> entries through an io.Pipe, so the
// document never exists as a whole buffer on the producer side. Returns the
// reader and the exact byte size it will produce.
func genTMX(n int) (io.Reader, int) {
	head := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<tmx version="1.4">` + "\n" +
		`<header creationtool="bench" srclang="en" datatype="plaintext"/>` + "\n" +
		`<body>` + "\n"
	tail := `</body>` + "\n" + `</tmx>` + "\n"

	tu := func(i int) string {
		return fmt.Sprintf(
			"<tu tuid=\"u%d\"><tuv xml:lang=\"en\"><seg>Source string number %d with several words</seg></tuv>"+
				"<tuv xml:lang=\"fr\"><seg>Chaîne source numéro %d avec plusieurs mots</seg></tuv></tu>\n",
			i, i, i)
	}

	size := len(head) + len(tail)
	for i := range n {
		size += len(tu(i))
	}

	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, head)
		for i := range n {
			if _, err := io.WriteString(pw, tu(i)); err != nil {
				return
			}
		}
		_, _ = io.WriteString(pw, tail)
		_ = pw.Close()
	}()
	return pr, size
}

// TestStreamingReaderBoundedMemory asserts the TMX reader's peak heap stays
// small and flat across a 20x document-size change when reading from a pure
// stream with a (file-backed) skeleton store — i.e. it does not hold the whole
// document. This is the load-bearing guarantee of the capturing-reader path.
func TestStreamingReaderBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	peakReading := func(n int) (peak uint64, size int) {
		pr, sz := genTMX(n)
		reader := tmx.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{
			URI:          "big.tmx",
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
			count++ // drain without retaining parts, so the test itself is bounded
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
	t.Logf("tmx streaming reader peakΔ: small(%d KiB doc)=%d KiB, large(%d KiB doc)=%d KiB",
		smallSize/1024, ps/1024, largeSize/1024, pl/1024)

	// Peak must be well below the document size and must not scale with it.
	assert.Less(t, pl, uint64(largeSize)/4,
		"streaming reader peak not bounded well below doc size")
	if ps > 0 {
		assert.Less(t, pl, max(ps, uint64(1<<20))*3,
			"streaming reader peak scaled with input (20x doc should not ~20x memory)")
	}
}

// TestStreamingByteExact_Large round-trips a large generated TMX byte-for-byte
// through the streaming path (skeleton store set, UTF-8 input), exercising the
// capturing reader + incremental emission at scale.
func TestStreamingByteExact_Large(t *testing.T) {
	pr, _ := genTMX(5_000)
	input, err := io.ReadAll(pr)
	require.NoError(t, err)

	ctx := context.Background()
	reader := tmx.NewReader()
	writer := tmx.NewWriter()
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

	assert.Equal(t, string(input), buf.String(), "large TMX must round-trip byte-exact through the streaming path")
}

// TestBufferedFallback_UTF16 verifies that a UTF-16 (BOM) input falls back to
// the buffered walk (the transcode is inherently whole-document) and yields the
// same parts/skeleton output as the equivalent UTF-8 input. The gate must route
// non-UTF-8 away from the streaming path.
func TestBufferedFallback_UTF16(t *testing.T) {
	utf8Doc := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<tmx version="1.4"><header srclang="en" datatype="plaintext"/><body>` + "\n" +
		`<tu tuid="u1"><tuv xml:lang="en"><seg>Hello</seg></tuv><tuv xml:lang="fr"><seg>Bonjour</seg></tuv></tu>` + "\n" +
		`</body></tmx>` + "\n"

	roundtrip := func(t *testing.T, doc *model.RawDocument) string {
		t.Helper()
		ctx := context.Background()
		reader := tmx.NewReader()
		writer := tmx.NewWriter()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)
		require.NoError(t, reader.Open(ctx, doc))
		parts := testutil.CollectParts(t, reader.Read(ctx))
		reader.Close()
		var buf bytes.Buffer
		require.NoError(t, writer.SetOutputWriter(&buf))
		require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
		writer.Close()
		return buf.String()
	}

	// UTF-8 input goes through the streaming path.
	utf8Out := roundtrip(t, testutil.RawDocFromString(utf8Doc, model.LocaleEnglish))
	assert.Equal(t, utf8Doc, utf8Out, "UTF-8 input should round-trip byte-exact")

	// UTF-16LE (with BOM) input hits the buffered fallback; after transcode it
	// produces the same UTF-8 output as the UTF-8 input.
	u16 := utf16.Encode([]rune(utf8Doc))
	le := make([]byte, 0, 2+len(u16)*2)
	le = append(le, 0xFF, 0xFE) // UTF-16LE BOM
	for _, u := range u16 {
		le = append(le, byte(u), byte(u>>8))
	}
	utf16Out := roundtrip(t, &model.RawDocument{
		URI:          "u16.tmx",
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(le)),
	})
	assert.Equal(t, utf8Out, utf16Out, "UTF-16 input (buffered fallback) should transcode to the same UTF-8 output")
}
