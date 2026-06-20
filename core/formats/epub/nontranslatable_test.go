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

// richChapterXHTML exercises the four non-translatable surfacing findings that
// live inside the direct-XHTML fallback extractor: bare prose in
// <div>/<section>/<aside> and an image's alt/title/aria-label attributes,
// interleaved with ordinary translatable blocks (title, h1, p).
const richChapterXHTML = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Rich Chapter</title></head>
<body>
  <h1>Intro</h1>
  <p>A paragraph.</p>
  <div>Bare prose in a div.</div>
  <section>Section prose.</section>
  <aside>Aside note.</aside>
  <figure><img src="x.png" alt="An illustration" title="Fig 1" aria-label="figure one"/></figure>
  <p>End.</p>
</body>
</html>`

// richOPF carries the Dublin Core fields the reader surfaces plus dc:identifier
// (which must stay in skeleton) and dc:language (which is not surfaced).
const richOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">urn:uuid:rich-fixture</dc:identifier>
    <dc:title>Rich Title</dc:title>
    <dc:creator>Ada Lovelace</dc:creator>
    <dc:subject>Computing</dc:subject>
    <dc:subject>History</dc:subject>
    <dc:description>A rich description.</dc:description>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="ch1"/>
  </spine>
</package>`

const richNCX = `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="np1" playOrder="1">
      <navLabel><text>NCX Chapter One</text></navLabel>
      <content src="chapter1.xhtml"/>
    </navPoint>
  </navMap>
</ncx>`

const richNavDoc = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Navigation</title></head>
<body>
  <nav epub:type="toc">
    <ol><li><a href="chapter1.xhtml">Nav Chapter One</a></li></ol>
  </nav>
</body>
</html>`

// makeRichEPUB assembles an EPUB exercising every non-translatable finding:
// OPF Dublin Core metadata, a non-spine NCX + EPUB3 nav document, and a spine
// chapter with image attributes and bare prose. nav.xhtml and toc.ncx are NOT
// in the spine, so they reach the Data-emission path.
func makeRichEPUB(t *testing.T) []byte {
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

	// Fixed insertion order keeps the ZIP entry order deterministic.
	entries := []struct{ name, content string }{
		{"META-INF/container.xml", cont},
		{"OEBPS/content.opf", richOPF},
		{"OEBPS/chapter1.xhtml", richChapterXHTML},
		{"OEBPS/nav.xhtml", richNavDoc},
		{"OEBPS/toc.ncx", richNCX},
	}
	for _, e := range entries {
		fw, err := zw.Create(e.name)
		require.NoError(t, err)
		_, err = io.WriteString(fw, e.content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// contextBlocks returns the source text of every non-translatable block whose
// epub.context property equals kind.
func contextBlocks(parts []*model.Part, kind string) []string {
	var out []string
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || b.Translatable {
			continue
		}
		if b.Properties["epub.context"] == kind {
			out = append(out, b.SourceText())
		}
	}
	return out
}

// findContextBlock returns the first non-translatable block with the given
// epub.context value and source text.
func findContextBlock(parts []*model.Part, kind, text string) *model.Block {
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || b.Translatable {
			continue
		}
		if b.Properties["epub.context"] == kind && b.SourceText() == text {
			return b
		}
	}
	return nil
}

// reconstructEntry round-trips the EPUB through the skeleton store (optionally
// with the surfacing flag off) and returns the reconstructed bytes of the named
// ZIP entry.
func reconstructEntry(t *testing.T, data []byte, surface bool, entry string) []byte {
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
		if f.Name == entry {
			rc, err := f.Open()
			require.NoError(t, err)
			out, err := io.ReadAll(rc)
			rc.Close()
			require.NoError(t, err)
			return out
		}
	}
	t.Fatalf("entry %q not found in reconstructed EPUB", entry)
	return nil
}

// originalEntry returns the original bytes of a named ZIP entry.
func originalEntry(t *testing.T, data []byte, entry string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name == entry {
			rc, err := f.Open()
			require.NoError(t, err)
			out, err := io.ReadAll(rc)
			rc.Close()
			require.NoError(t, err)
			return out
		}
	}
	t.Fatalf("entry %q not found in original EPUB", entry)
	return nil
}

// TestNonTranslatable_TranslatablePayloadUnchanged guards the headline rule:
// the surfacing flag adds ONLY non-translatable blocks. The translatable
// (MT) payload is identical whether the flag is on or off.
func TestNonTranslatable_TranslatablePayloadUnchanged(t *testing.T) {
	data := makeRichEPUB(t)
	want := []string{"Rich Chapter", "Intro", "A paragraph.", "End."}

	on := translatableTexts(readParts(t, data, true))
	off := translatableTexts(readParts(t, data, false))

	assert.Equal(t, want, on, "translatable payload (flag on)")
	assert.Equal(t, want, off, "translatable payload (flag off)")
}

// TestNonTranslatable_OPFMetadata verifies the OPF Dublin Core fields surface
// as non-translatable context blocks (dc:title carrying RoleTitle), while
// dc:identifier and dc:language are NOT surfaced.
func TestNonTranslatable_OPFMetadata(t *testing.T) {
	parts := readParts(t, makeRichEPUB(t), true)

	assert.Equal(t, []string{"Rich Title"}, contextBlocks(parts, "dc:title"))
	assert.Equal(t, []string{"A rich description."}, contextBlocks(parts, "dc:description"))
	assert.Equal(t, []string{"Ada Lovelace"}, contextBlocks(parts, "dc:creator"))
	assert.Equal(t, []string{"Computing", "History"}, contextBlocks(parts, "dc:subject"))

	title := findContextBlock(parts, "dc:title", "Rich Title")
	require.NotNil(t, title)
	assert.False(t, title.Translatable)
	assert.Equal(t, model.RoleTitle, title.SemanticRole())
	require.Len(t, title.Source, 1, "single verbatim run, not inline-parsed")
	require.NotNil(t, title.Source[0].Text)

	// dc:identifier / dc:language never surface as blocks.
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		assert.NotContains(t, b.SourceText(), "urn:uuid", "dc:identifier must stay in skeleton")
		assert.NotEqual(t, "en", b.SourceText(), "dc:language must not surface")
	}
}

// TestNonTranslatable_NavLabels verifies NCX <navLabel><text> and EPUB3 nav-doc
// <a> link labels surface as non-translatable nav-label context blocks.
func TestNonTranslatable_NavLabels(t *testing.T) {
	parts := readParts(t, makeRichEPUB(t), true)

	labels := contextBlocks(parts, "nav-label")
	assert.Contains(t, labels, "NCX Chapter One")
	assert.Contains(t, labels, "Nav Chapter One")

	b := findContextBlock(parts, "nav-label", "NCX Chapter One")
	require.NotNil(t, b)
	assert.False(t, b.Translatable)
	assert.Equal(t, model.RoleTitle, b.SemanticRole())

	// The nav documents still ride through as Data parts so the writer can
	// copy them verbatim.
	dataEntries := map[string]bool{}
	for _, p := range parts {
		if p.Type == model.PartData {
			dataEntries[p.Resource.(*model.Data).Properties["entry"]] = true
		}
	}
	assert.True(t, dataEntries["OEBPS/toc.ncx"], "NCX still emitted as Data")
	assert.True(t, dataEntries["OEBPS/nav.xhtml"], "nav doc still emitted as Data")
}

// TestNonTranslatable_ImageAttrs verifies an <img>'s alt/title/aria-label
// surface as non-translatable context blocks.
func TestNonTranslatable_ImageAttrs(t *testing.T) {
	parts := readParts(t, makeRichEPUB(t), true)

	assert.Equal(t, []string{"An illustration"}, contextBlocks(parts, "img-alt"))
	assert.Equal(t, []string{"Fig 1"}, contextBlocks(parts, "img-title"))
	assert.Equal(t, []string{"figure one"}, contextBlocks(parts, "img-aria-label"))

	alt := findContextBlock(parts, "img-alt", "An illustration")
	require.NotNil(t, alt)
	assert.False(t, alt.Translatable)
}

// TestNonTranslatable_BareProse verifies bare text directly inside
// <div>/<section>/<aside> surfaces as non-translatable context blocks.
func TestNonTranslatable_BareProse(t *testing.T) {
	parts := readParts(t, makeRichEPUB(t), true)

	prose := contextBlocks(parts, "bare-prose")
	assert.ElementsMatch(t,
		[]string{"Bare prose in a div.", "Section prose.", "Aside note."}, prose)

	for _, txt := range prose {
		b := findContextBlock(parts, "bare-prose", txt)
		require.NotNil(t, b)
		assert.False(t, b.Translatable)
	}
}

// TestNonTranslatable_FlagOffSurfacesNothing verifies that with the flag off
// none of the four findings surface — the only blocks are the translatable
// payload. This is the parity baseline.
func TestNonTranslatable_FlagOffSurfacesNothing(t *testing.T) {
	parts := readParts(t, makeRichEPUB(t), false)

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		assert.True(t, b.Translatable,
			"flag off must surface no non-translatable block, got %q", b.SourceText())
	}
}

// TestNonTranslatable_RoundTripChapterByteExact verifies the chapter
// reconstruction is byte-identical with the flag on (bare prose rides as a Ref,
// image attrs ride no skeleton) vs off (bare prose stays skeleton text).
func TestNonTranslatable_RoundTripChapterByteExact(t *testing.T) {
	data := makeRichEPUB(t)

	on := reconstructEntry(t, data, true, "OEBPS/chapter1.xhtml")
	off := reconstructEntry(t, data, false, "OEBPS/chapter1.xhtml")

	assert.Equal(t, string(off), string(on),
		"surfacing must not change the reconstructed chapter bytes")

	// The trailing structure (after the last block) survives reconstruction.
	assert.Contains(t, string(on), "</body>")
	assert.Contains(t, string(on), "</html>")
	// Bare prose reappears verbatim in its container.
	assert.Contains(t, string(on), "<div>Bare prose in a div.</div>")
	assert.Contains(t, string(on), "<section>Section prose.</section>")
}

// TestNonTranslatable_RoundTripStructureVerbatim verifies the OPF, NCX, and nav
// documents are copied through byte-for-byte: surfacing their metadata/labels
// as context blocks never rewrites the source files.
func TestNonTranslatable_RoundTripStructureVerbatim(t *testing.T) {
	data := makeRichEPUB(t)

	for _, entry := range []string{"OEBPS/content.opf", "OEBPS/toc.ncx", "OEBPS/nav.xhtml"} {
		want := originalEntry(t, data, entry)
		on := reconstructEntry(t, data, true, entry)
		off := reconstructEntry(t, data, false, entry)
		assert.Equal(t, string(want), string(on), "%s: flag-on reconstruction must be byte-exact", entry)
		assert.Equal(t, string(want), string(off), "%s: flag-off reconstruction must be byte-exact", entry)
	}
}

// TestNonTranslatable_OPFNoMetadataNoBlocks verifies an OPF without a
// <metadata> block surfaces no metadata context blocks (and does not error).
func TestNonTranslatable_OPFNoMetadataNoBlocks(t *testing.T) {
	// makeEPUB (reader_test.go) uses an OPF with no <metadata> element.
	parts := readParts(t, makeEPUB(t), true)
	for _, kind := range []string{"dc:title", "dc:creator", "dc:subject", "dc:description"} {
		assert.Empty(t, contextBlocks(parts, kind), "no %s blocks expected", kind)
	}
}
