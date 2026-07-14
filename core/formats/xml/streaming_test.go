package xml_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xmlStreamingRoundtrip writes input to a temp file and reads it with a skeleton
// store and a URI that points at the file — so readContentStreaming's os.Open
// succeeds and the bounded-memory two-pass path is exercised (not the buffered
// fallback). Returns the writer's reconstruction.
func xmlStreamingRoundtrip(t *testing.T, input string) string {
	return xmlStreamingRoundtripCfg(t, input, nil)
}

func xmlStreamingRoundtripCfg(t *testing.T, input string, cfg *xmlfmt.Config) string {
	t.Helper()
	ctx := context.Background()

	path := filepath.Join(t.TempDir(), "doc.xml")
	require.NoError(t, os.WriteFile(path, []byte(input), 0o644))
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := xmlfmt.NewReader()
	writer := xmlfmt.NewWriter()
	if cfg != nil {
		require.NoError(t, reader.SetConfig(cfg))
	}
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleEnglish,
		Reader:       f,
	}))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// xmlBufferedRoundtrip reads the same input via an in-memory doc (URI is not a
// file), so readContentStreaming declines and the buffered walk runs — the
// established byte-exact behavior to compare the streaming path against.
func xmlBufferedRoundtrip(t *testing.T, input string) string {
	return xmlBufferedRoundtripCfg(t, input, nil)
}

func xmlBufferedRoundtripCfg(t *testing.T, input string, cfg *xmlfmt.Config) string {
	t.Helper()
	ctx := context.Background()
	reader := xmlfmt.NewReader()
	writer := xmlfmt.NewWriter()
	if cfg != nil {
		require.NoError(t, reader.SetConfig(cfg))
	}
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// TestStreaming_ByteExactParity runs a corpus through the streaming two-pass and
// asserts it (a) round-trips the input byte-exact and (b) matches the buffered
// path exactly — across attributes, nesting, comments/PIs, namespaces, CDATA,
// multiple top-level children, a nested-translatable case (parent-removal), and
// ITS (both global <its:rules> and local its:translate — the streamed resolver).
func TestStreaming_ByteExactParity(t *testing.T) {
	cases := map[string]string{
		"simple":     `<?xml version="1.0"?><root><message>Hello World</message></root>`,
		"multiple":   "<?xml version=\"1.0\"?>\n<resources>\n  <string>Title</string>\n  <string>Description</string>\n</resources>",
		"attributes": "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<root xmlns:custom=\"http://example.com\" xml:lang=\"en\">\n  <item id=\"123\" class=\"primary\">First item</item>\n  <item id=\"456\">Second item</item>\n</root>",
		"nested":     "<?xml version=\"1.0\"?>\n<root>\n  <parent>\n    <child>Nested content</child>\n  </parent>\n  <other>Sibling content</other>\n</root>",
		"comments":   "<?xml version=\"1.0\"?>\n<!-- file comment -->\n<root>\n  <!-- elem comment -->\n  <message>Hello</message>\n</root>",
		"pi":         "<?xml version=\"1.0\"?>\n<?xml-stylesheet type=\"text/xsl\" href=\"style.xsl\"?>\n<root>\n  <message>Hello</message>\n</root>",
		"cdata":      `<?xml version="1.0"?><root><message><![CDATA[a & b < c]]></message></root>`,
		"entities":   `<?xml version="1.0"?><root><message>A &amp; B &lt;x&gt;</message></root>`,
		"its_global": "<?xml version=\"1.0\"?>\n<doc xmlns:its=\"http://www.w3.org/2005/11/its\">\n  <its:rules version=\"2.0\"><its:translateRule selector=\"//code\" translate=\"no\"/></its:rules>\n  <p>Translate me</p>\n  <code>do not translate</code>\n</doc>",
		"its_local":  "<?xml version=\"1.0\"?>\n<doc xmlns:its=\"http://www.w3.org/2005/11/its\">\n  <p>Translate me</p>\n  <p its:translate=\"no\">Keep me</p>\n</doc>",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			streamed := xmlStreamingRoundtrip(t, in)
			// The streaming path must match the buffered path exactly (the
			// established behavior) in every case.
			assert.Equal(t, xmlBufferedRoundtrip(t, in), streamed, "streaming must match buffered output")
			// It must also round-trip the input byte-exact — except CDATA in
			// translatable content, which both paths re-encode as escaped text
			// (a documented, pre-existing limitation, not streaming-specific).
			if name != "cdata" {
				assert.Equal(t, in, streamed, "streaming round-trip must be byte-exact")
			}
		})
	}
}

// TestStreaming_ConfigDrivenParity covers the config-driven cases — inline
// mixed content (an inline <b> folded into its parent block) and a translatable
// attribute (@label) — through the streaming path, asserting byte-exact + parity.
func TestStreaming_ConfigDrivenParity(t *testing.T) {
	inlineCfg := &xmlfmt.Config{}
	inlineCfg.ElementRules = append(inlineCfg.ElementRules, &xmlfmt.ElementRule{
		Name:      "b",
		RuleTypes: []xmlfmt.RuleType{xmlfmt.RuleInline},
	})
	inlineDoc := "<?xml version=\"1.0\"?>\n<root>\n  <message>Hello <b>World</b> today</message>\n  <message>Second <b>bold</b> line</message>\n</root>"

	attrCfg := &xmlfmt.Config{}
	attrCfg.ElementRules = append(attrCfg.ElementRules, &xmlfmt.ElementRule{
		Name:                   "item",
		RuleTypes:              []xmlfmt.RuleType{xmlfmt.RuleAttributeTrans},
		TranslatableAttributes: map[string]*xmlfmt.TranslatableAttrCondition{"label": {}},
	})
	attrDoc := "<?xml version=\"1.0\"?>\n<root>\n  <item label=\"First label\">body one</item>\n  <item label=\"Second label\">body two</item>\n</root>"

	cases := []struct {
		name string
		doc  string
		cfg  *xmlfmt.Config
	}{
		{"inline_mixed", inlineDoc, inlineCfg},
		{"translatable_attr", attrDoc, attrCfg},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			streamed := xmlStreamingRoundtripCfg(t, c.doc, c.cfg)
			assert.Equal(t, xmlBufferedRoundtripCfg(t, c.doc, c.cfg), streamed, "streaming must match buffered output")
			assert.Equal(t, c.doc, streamed, "streaming round-trip must be byte-exact")
		})
	}
}

// TestStreaming_MalformedSurfacedByPass1 asserts a malformed document fails the
// streaming path (via the pass-1 rules scan, the well-formedness gate) rather
// than producing partial output.
func TestStreaming_MalformedSurfacedByPass1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "bad.xml")
	require.NoError(t, os.WriteFile(path, []byte(`<root><unclosed></root>`), 0o644))
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := xmlfmt.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, &model.RawDocument{URI: path, SourceLocale: model.LocaleEnglish, Reader: f}))

	var gotErr bool
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			gotErr = true
		}
	}
	reader.Close()
	assert.True(t, gotErr, "malformed input must surface an error on the streaming path")
}

// TestStreaming_NonFileFallback confirms an in-memory (non-file) input still
// works — readContentStreaming declines and the buffered walk handles it.
func TestStreaming_NonFileFallback(t *testing.T) {
	in := `<?xml version="1.0"?><root><message>Hello</message></root>`
	assert.Equal(t, in, xmlBufferedRoundtrip(t, in))
}

// genXMLFile writes a resources document with n translatable elements to a temp
// file and returns its path + byte size. Streaming needs a re-openable file, so
// (unlike the other formats' io.Pipe generators) the document is on disk.
func genXMLFile(t *testing.T, n int) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "big.xml")
	f, err := os.Create(path)
	require.NoError(t, err)
	size := 0
	w := func(s string) {
		_, werr := f.WriteString(s)
		require.NoError(t, werr)
		size += len(s)
	}
	w("<?xml version=\"1.0\"?>\n<resources>\n")
	for i := range n {
		w("  <!-- entry " + itoa(i) + " -->\n  <string>Translatable value number " + itoa(i) + " with several descriptive words for the memory benchmark</string>\n")
	}
	w("</resources>\n")
	require.NoError(t, f.Close())
	return path, size
}

func itoa(i int) string { return strconv.Itoa(i) }

// TestStreaming_BoundedMemory asserts the xml reader's peak live heap stays
// small and flat across a 20x document-size change on the streaming (file) path
// — i.e. it never holds the whole document or token slice.
func TestStreaming_BoundedMemory(t *testing.T) {
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
		path, sz := genXMLFile(t, n)
		f, err := os.Open(path)
		require.NoError(t, err)
		reader := xmlfmt.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{
			URI:          path,
			SourceLocale: model.LocaleEnglish,
			Reader:       f,
		}))

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
	pl, largeSize := peakReading(40_000) // 20x

	testutil.AssertBoundedMemory(t, "xml streaming reader", ps, uint64(smallSize), pl, uint64(largeSize))
}
