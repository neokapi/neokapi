package json

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writerCase configures one reader/writer pair for the parity helpers below.
type writerCase struct {
	params    map[string]any           // reader config (ApplyMap)
	resolver  format.SubfilterResolver // wired on both reader and writer when set
	locale    model.LocaleID           // writer target locale ("" = source)
	mutate    func(*model.Block)       // applied to every block in flight (e.g. set a target)
	escSlash  bool
	setEscape bool // apply escSlash to the writer config
}

func (c writerCase) newReader(t *testing.T) *Reader {
	t.Helper()
	reader := NewReader()
	if c.params != nil {
		require.NoError(t, reader.Config().ApplyMap(c.params))
	}
	if c.resolver != nil {
		reader.SetSubfilterResolver(c.resolver)
	}
	return reader
}

func (c writerCase) newWriter(t *testing.T) *Writer {
	t.Helper()
	writer := NewWriter()
	if c.setEscape {
		writer.Config().EscapeForwardSlashes = c.escSlash
	}
	if c.resolver != nil {
		writer.SetSubfilterResolver(c.resolver)
	}
	if c.locale != "" {
		writer.SetLocale(c.locale)
	}
	return writer
}

// bufferedWriterRoundtrip runs the buffered skeleton path: collect every Part,
// then Write with a file/memory-backed skeleton store (writeFromSkeleton).
func bufferedWriterRoundtrip(t *testing.T, input string, c writerCase) string {
	t.Helper()
	ctx := t.Context()

	reader := c.newReader(t)
	writer := c.newWriter(t)

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	if c.mutate != nil {
		for _, p := range parts {
			if p.Type == model.PartBlock {
				if b, ok := p.Resource.(*model.Block); ok {
					c.mutate(b)
				}
			}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	return buf.String()
}

// streamingWriterRoundtrip runs the concurrent streaming path: the reader and
// writer share a NewStreamingSkeletonStore and run at the same time, with the
// reader's Parts forwarded into the writer as they are produced (streamWrite).
func streamingWriterRoundtrip(t *testing.T, input string, c writerCase) string {
	t.Helper()
	ctx := t.Context()

	reader := c.newReader(t)
	writer := c.newWriter(t)

	store := format.NewStreamingSkeletonStore()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	partsCh := make(chan *model.Part, 4)
	readCh := reader.Read(ctx)
	go func() {
		defer close(partsCh)
		defer store.CloseWrite()
		defer reader.Close()
		for res := range readCh {
			if res.Error != nil || res.Part == nil {
				continue
			}
			if c.mutate != nil && res.Part.Type == model.PartBlock {
				if b, ok := res.Part.Resource.(*model.Block); ok {
					c.mutate(b)
				}
			}
			partsCh <- res.Part
		}
	}()

	require.NoError(t, writer.Write(ctx, partsCh))
	require.NoError(t, writer.Close())
	return buf.String()
}

// TestStreamingWriterMatchesBuffered asserts the streaming write path
// (streamWrite over a concurrent skeleton store) produces byte-identical
// output to the buffered skeleton path (writeFromSkeleton) — and that both
// round-trip the source byte-exact when nothing is translated.
func TestStreamingWriterMatchesBuffered(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		input  string
		params map[string]any
	}{
		{"simple", `{"key": "value"}`, nil},
		{"whitespace", "{\n    \"key\" :   \"value\"\n}", nil},
		{"comments", "{\n  // line\n  /* block */\n  # hash\n  \"key\": \"value\"\n}", nil},
		{"nested", `{"parent": {"child": "value"}, "other": "text"}`, nil},
		{"array_isolated", `{"items": ["a", "b", "c"]}`, map[string]any{"extractIsolatedStrings": true}},
		{"array_non_isolated", `{"items": ["a", "b", "c"]}`, nil},
		{"non_string_values", `{"name": "Test", "count": 42, "rate": 3.14, "active": true, "meta": null}`, nil},
		{"json5_single_quotes", `{'single':'quoted',"dbl":"v"}`, nil},
		{"unicode", `{"esc":"line\nbreak \"q\" \\ é 😀","u":"café"}`, nil},
		{"trailing_newline", "{\"key\": \"value\"}\n", nil},
		{"empty_object", `{"data": {}, "text": "Hello"}`, nil},
		{"excluded_value_surfaced", `{"keep": "Hi", "_internal": "secret"}`, map[string]any{"exceptions": "^_internal$"}},
		{"note_and_id", `{"id": "greet", "text": "Hello", "note": "hint"}`, map[string]any{"idRules": "id", "noteRules": "note"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := writerCase{params: tc.params}
			buffered := bufferedWriterRoundtrip(t, tc.input, c)
			streaming := streamingWriterRoundtrip(t, tc.input, c)
			assert.Equal(t, buffered, streaming, "streaming output must match buffered output")
			assert.Equal(t, tc.input, streaming, "streaming round-trip must be byte-exact")
		})
	}
}

// TestStreamingWriterWithTranslation asserts translated targets flow through
// the streaming path identically to the buffered path.
func TestStreamingWriterWithTranslation(t *testing.T) {
	t.Parallel()
	input := `{"greeting": "Hello World", "farewell": "Goodbye"}`
	locale := model.LocaleID("fr")
	c := writerCase{
		locale: locale,
		mutate: func(b *model.Block) {
			switch b.SourceText() {
			case "Hello World":
				b.SetTargetText(locale, "Bonjour le monde")
			case "Goodbye":
				b.SetTargetText(locale, "Au revoir")
			}
		},
	}
	buffered := bufferedWriterRoundtrip(t, input, c)
	streaming := streamingWriterRoundtrip(t, input, c)
	assert.Equal(t, buffered, streaming)
	assert.Equal(t, `{"greeting": "Bonjour le monde", "farewell": "Au revoir"}`, streaming)
}

// TestStreamingWriterChildLayer asserts the `layer:<path>` skeleton refs —
// embedded subfilter content that arrives as a child layer in the Part stream
// — reconstruct identically on the streaming and buffered paths.
func TestStreamingWriterChildLayer(t *testing.T) {
	t.Parallel()
	// No forward slashes: the writer re-escapes the reconstructed child-layer
	// value, and EscapeForwardSlashes defaults to true (`/` → `\/`).
	input := `{"title": "Hello", "body": "<b>Rich content", "tail": "End"}`
	c := writerCase{
		params:   map[string]any{"subfilter": "html"},
		resolver: &testResolver{},
	}
	buffered := bufferedWriterRoundtrip(t, input, c)
	streaming := streamingWriterRoundtrip(t, input, c)
	assert.Equal(t, buffered, streaming, "streaming output must match buffered output")
	assert.Equal(t, input, streaming, "child-layer streaming round-trip must be byte-exact")
}

// TestStreamingWriterReorderedBlocks asserts the pending-block window absorbs
// a tool that reorders blocks: parts arrive out of skeleton order and the
// output is still byte-identical to the buffered path.
func TestStreamingWriterReorderedBlocks(t *testing.T) {
	t.Parallel()
	input := `{"a": "one", "b": "two", "c": "three"}`
	ctx := t.Context()

	reader := NewReader()
	writer := NewWriter()
	store := format.NewStreamingSkeletonStore()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))

	// Collect everything first (the skeleton queue is unbounded, so the reader
	// never blocks), then feed the writer the parts in REVERSE order.
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	store.CloseWrite()

	partsCh := make(chan *model.Part, len(parts))
	for i := len(parts) - 1; i >= 0; i-- {
		partsCh <- parts[i]
	}
	close(partsCh)

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, partsCh))
	require.NoError(t, writer.Close())
	assert.Equal(t, input, buf.String())
}

// TestStreamingWriterBoundedMemory drives a full read→write round-trip of a
// large JSON document supplied as a pure stream (an io.Pipe generator — the
// document never exists as a whole buffer) through the streaming reader, a
// concurrent skeleton store, and the streaming writer, and asserts peak heap
// stays a small, flat window — independent of document size. This is the write
// half of TestStreamingReaderBoundedMemory: with the buffered writer the block
// map + skeleton queue would grow O(document).
func TestStreamingWriterBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	peakWriting := func(n int) (peak uint64, size int) {
		pr, sz := genLargeJSON(n)
		reader := NewReader()
		writer := NewWriter()
		store := format.NewStreamingSkeletonStore()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)

		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, &model.RawDocument{
			URI:          "big.json",
			SourceLocale: model.LocaleEnglish,
			Reader:       io.NopCloser(pr),
		}))
		require.NoError(t, writer.SetOutputWriter(io.Discard))

		runtime.GC()
		var base runtime.MemStats
		runtime.ReadMemStats(&base)

		partsCh := make(chan *model.Part, 64)
		go func() {
			defer close(partsCh)
			defer store.CloseWrite()
			defer reader.Close()
			var m runtime.MemStats
			count := 0
			for res := range reader.Read(ctx) {
				if res.Error != nil || res.Part == nil {
					continue
				}
				partsCh <- res.Part
				count++
				if count%256 == 0 {
					runtime.ReadMemStats(&m)
					if m.HeapAlloc > base.HeapAlloc && m.HeapAlloc-base.HeapAlloc > peak {
						peak = m.HeapAlloc - base.HeapAlloc
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

// genLargeJSON emits a JSON object of n translatable entries on demand via an
// io.Pipe, so the document never exists as a whole buffer.
func genLargeJSON(n int) (io.Reader, int) {
	size := len("{\n") + len("\n}\n")
	for i := range n {
		if i > 0 {
			size += len(",\n")
		}
		size += len(fmt.Sprintf("  \"key_%d\": \"translatable value number %d with words\"", i, i))
	}
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, "{\n")
		for i := range n {
			if i > 0 {
				_, _ = io.WriteString(pw, ",\n")
			}
			fmt.Fprintf(pw, "  \"key_%d\": \"translatable value number %d with words\"", i, i)
		}
		_, _ = io.WriteString(pw, "\n}\n")
		pw.Close()
	}()
	return pr, size
}
