package epub_test

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chapterCodeXHTML has standalone verbatim listings (a bare <pre> and a
// <pre><code> pair) between ordinary translatable paragraphs. The fallback
// XHTML extractor leaves <pre>/<code> bodies in skeleton; the
// ExtractNonTranslatableContent flag surfaces them as RoleCode content blocks.
const chapterCodeXHTML = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Code Chapter</title></head>
<body>
  <h1>Examples</h1>
  <p>Run this:</p>
  <pre>func main() {
    fmt.Println("hi")
}</pre>
  <pre><code>plain &lt;b&gt; code</code></pre>
  <p>After code.</p>
</body>
</html>`

const (
	wantPreBody  = "func main() {\n    fmt.Println(\"hi\")\n}"
	wantCodeBody = "plain <b> code"
)

func makeCodeEPUB(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	header := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	w, err := zw.CreateHeader(header)
	require.NoError(t, err)
	_, err = io.WriteString(w, "application/epub+zip")
	require.NoError(t, err)

	cont := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest><item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	entries := map[string]string{
		"META-INF/container.xml": cont,
		"OEBPS/content.opf":      opf,
		"OEBPS/chapter1.xhtml":   chapterCodeXHTML,
	}
	for name, content := range entries {
		fw, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(fw, content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// reconstructChapter reads the EPUB through the skeleton store (optionally
// disabling the surfacing flag), writes it back, and returns the
// reconstructed OEBPS/chapter1.xhtml bytes.
func reconstructChapter(t *testing.T, data []byte, surface bool) []byte {
	t.Helper()
	ctx := t.Context()

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := epub.NewReader()
	reader.SetSkeletonStore(skelStore)
	if !surface {
		reader.Config().(*epub.Config).SetExtractNonTranslatableContent(false)
	}
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := epub.NewWriter()
	writer.SetOriginalContent(data)
	writer.SetSkeletonStore(skelStore)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name == "OEBPS/chapter1.xhtml" {
			rc, err := f.Open()
			require.NoError(t, err)
			out, err := io.ReadAll(rc)
			rc.Close()
			require.NoError(t, err)
			return out
		}
	}
	t.Fatal("OEBPS/chapter1.xhtml not found in reconstructed EPUB")
	return nil
}

func readParts(t *testing.T, data []byte, surface bool) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := epub.NewReader()
	if !surface {
		reader.Config().(*epub.Config).SetExtractNonTranslatableContent(false)
	}
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish)))
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func codeBlocks(parts []*model.Part) []*model.Block {
	var out []*model.Block
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || b.Translatable {
			continue
		}
		out = append(out, b)
	}
	return out
}

// translatableTexts returns the source text of every translatable block.
func translatableTexts(parts []*model.Part) []string {
	var out []string
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || !b.Translatable {
			continue
		}
		out = append(out, b.SourceText())
	}
	return out
}

// TestVerbatim_DefaultSurfacesContentBlocks verifies the default (flag on)
// surfaces standalone <pre>/<code> listings as non-translatable RoleCode
// content blocks with their verbatim body and PreserveWhitespace, while the
// surrounding translatable payload is untouched.
func TestVerbatim_DefaultSurfacesContentBlocks(t *testing.T) {
	parts := readParts(t, makeCodeEPUB(t), true)

	nt := codeBlocks(parts)
	require.Len(t, nt, 2, "both verbatim listings should surface as content blocks")

	for _, b := range nt {
		assert.False(t, b.Translatable, "content block must be non-translatable")
		assert.True(t, b.PreserveWhitespace, "verbatim block must preserve whitespace")
		assert.Equal(t, model.RoleCode, b.SemanticRole(), "verbatim block role")
		// Single verbatim run, not inline-parsed.
		require.Len(t, b.Source, 1)
		require.NotNil(t, b.Source[0].Text)
	}

	bodies := []string{nt[0].SourceText(), nt[1].SourceText()}
	assert.Contains(t, bodies, wantPreBody)
	assert.Contains(t, bodies, wantCodeBody)

	// The translatable payload is unchanged: same translatable blocks, none
	// of which is the verbatim code body.
	texts := translatableTexts(parts)
	assert.Equal(t, []string{"Code Chapter", "Examples", "Run this:", "After code."}, texts)
	assert.NotContains(t, texts, wantPreBody)
	assert.NotContains(t, texts, wantCodeBody)
}

// TestVerbatim_FlagOffKeepsSkeleton verifies that with the flag off the
// verbatim listings stay in skeleton (no content blocks emitted) and the
// translatable payload is identical to the flag-on case.
func TestVerbatim_FlagOffKeepsSkeleton(t *testing.T) {
	parts := readParts(t, makeCodeEPUB(t), false)

	assert.Empty(t, codeBlocks(parts), "flag off must not surface verbatim content blocks")

	// Translatable payload identical to the flag-on case.
	texts := translatableTexts(parts)
	assert.Equal(t, []string{"Code Chapter", "Examples", "Run this:", "After code."}, texts)
}

// TestVerbatim_RoundTripByteExact verifies the surfacing is transparent to the
// round-trip: the reconstructed chapter is byte-identical whether the flag is
// on (body rides as a Ref) or off (body stays skeleton text), and the verbatim
// listings reappear verbatim (re-escaped) in the output.
func TestVerbatim_RoundTripByteExact(t *testing.T) {
	data := makeCodeEPUB(t)

	on := reconstructChapter(t, data, true)
	off := reconstructChapter(t, data, false)

	assert.Equal(t, string(off), string(on),
		"surfacing the verbatim content must not change the reconstructed bytes")

	// Both verbatim listings round-trip byte-exact (the <pre><code> body is
	// re-escaped back to &lt;b&gt;).
	assert.Contains(t, string(on), "<pre>func main() {\n    fmt.Println(\"hi\")\n}</pre>")
	assert.Contains(t, string(on), "<pre><code>plain &lt;b&gt; code</code></pre>")
}

// TestVerbatim_ApplyMap verifies the config key round-trips through ApplyMap.
func TestVerbatim_ApplyMap(t *testing.T) {
	cfg := &epub.Config{}
	assert.True(t, cfg.ExtractNonTranslatableContent(), "default is on")

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())

	cfg.Reset()
	assert.True(t, cfg.ExtractNonTranslatableContent(), "Reset restores the on default")
}

// TestVerbatim_SchemaExposesFlag verifies the schema advertises the new flag.
func TestVerbatim_SchemaExposesFlag(t *testing.T) {
	cfg := &epub.Config{}
	s := cfg.Schema()
	require.NotNil(t, s)
	prop, ok := s.Properties["extractNonTranslatableContent"]
	require.True(t, ok, "schema must declare extractNonTranslatableContent")
	assert.Equal(t, "boolean", prop.Type)
	assert.Equal(t, true, prop.Default)
}

// TestVerbatim_InlineCodeInsideBlockUntouched verifies that inline <code>
// inside a translatable paragraph is NOT surfaced as a separate content block
// — it stays part of the translatable block's source text.
func TestVerbatim_InlineCodeInsideBlockUntouched(t *testing.T) {
	const inlineChapter = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Inline</title></head>
<body>
  <p>Call <code>main()</code> now.</p>
</body>
</html>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	header := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	w, err := zw.CreateHeader(header)
	require.NoError(t, err)
	_, err = io.WriteString(w, "application/epub+zip")
	require.NoError(t, err)
	cont := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest><item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	for name, content := range map[string]string{
		"META-INF/container.xml": cont,
		"OEBPS/content.opf":      opf,
		"OEBPS/chapter1.xhtml":   inlineChapter,
	} {
		fw, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(fw, content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	parts := readParts(t, buf.Bytes(), true)
	assert.Empty(t, codeBlocks(parts), "inline code inside a block is not a standalone listing")
	// The paragraph remains translatable with its inline text folded in.
	assert.Contains(t, translatableTexts(parts), "Call main() now.")
}
