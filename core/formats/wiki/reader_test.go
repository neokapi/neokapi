package wiki_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/wiki"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: read a string with the supplied reader and return parts
func readString(t *testing.T, reader *wiki.Reader, content string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	err := reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	return parts
}

// helper: read a string with the default (DokuWiki) reader. The default
// matches the okf_wiki bridge contract — see issue #496.
func readDefault(t *testing.T, content string) []*model.Part {
	t.Helper()
	return readString(t, wiki.NewReader(), content)
}

// helper: create a DokuWiki reader (now equivalent to the default).
// Retained for symmetry with newMediaWikiReader.
func newDokuWikiReader(t *testing.T) *wiki.Reader {
	t.Helper()
	reader := wiki.NewReader()
	cfg := reader.Config().(*wiki.Config)
	cfg.Variant = wiki.VariantDokuWiki
	return reader
}

// helper: create a MediaWiki reader. The native package supports the
// MediaWiki dialect but it is no longer the default — okf_wiki targets
// DokuWiki only. Tests covering MediaWiki-specific markup use this
// helper to opt in.
func newMediaWikiReader(t *testing.T) *wiki.Reader {
	t.Helper()
	reader := wiki.NewReader()
	cfg := reader.Config().(*wiki.Config)
	cfg.Variant = wiki.VariantMediaWiki
	return reader
}

// readMediaWiki reads a string with the explicit MediaWiki reader.
func readMediaWiki(t *testing.T, content string) []*model.Part {
	t.Helper()
	return readString(t, newMediaWikiReader(t), content)
}

// roundtripWith roundtrips a snippet using the supplied reader factory.
func roundtripWith(t *testing.T, newReader func(*testing.T) *wiki.Reader, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := newReader(t)
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := wiki.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	return buf.String()
}

// helper: roundtrip a snippet using the default (DokuWiki) reader.
func roundtrip(t *testing.T, input string) string {
	t.Helper()
	return roundtripWith(t, func(*testing.T) *wiki.Reader { return wiki.NewReader() }, input)
}

// helper: roundtrip a snippet using the explicit MediaWiki reader.
func roundtripMediaWiki(t *testing.T, input string) string {
	t.Helper()
	return roundtripWith(t, newMediaWikiReader, input)
}

// ---- WikiFilterTest ----

// okapi: WikiFilterTest#testDefaultInfo
// Okapi's testDefaultInfo asserts the filter exposes a non-null name, a
// non-null display name, and a non-empty configuration list. The native
// analog: the reader reports a non-empty Name and DisplayName, and its
// Signature advertises at least one extension and one MIME type (the
// equivalent of okapi's filter configurations).
func TestDefaultInfo(t *testing.T) {
	reader := wiki.NewReader()
	assert.NotEmpty(t, reader.Name())
	assert.NotEmpty(t, reader.DisplayName())
	sig := reader.Signature()
	assert.NotEmpty(t, sig.Extensions, "wiki should advertise at least one extension")
	assert.NotEmpty(t, sig.MIMETypes, "wiki should advertise at least one MIME type")
}

// okapi: WikiFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readDefault(t, "== Title ==\nSimple text.")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "text/x-wiki", layer.MimeType)
	assert.Equal(t, "UTF-8", layer.Encoding)
	assert.Equal(t, model.LocaleEnglish, layer.Locale)
}

// okapi: WikiFilterTest#testSimpleLine
func TestExtract_SimpleLine(t *testing.T) {
	parts := readDefault(t, "The quick brown fox jumps over the lazy dog.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "simple line should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "The quick brown fox jumps over the lazy dog.")
}

// okapi: WikiFilterTest#testMultipleLines
func TestExtract_MultipleLines(t *testing.T) {
	parts := readDefault(t, "Line one.\nLine two.\nLine three.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "multiple lines should produce translatable blocks")

	// All lines should be part of one paragraph block
	texts := testutil.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Line one") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find 'Line one' in extracted texts")
}

// okapi: WikiFilterTest#testHeader
func TestExtract_Header(t *testing.T) {
	parts := readDefault(t, "== Header Text ==\nBody text here.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "header should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	// The header text should be extracted without == markers
	headerFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Header Text") {
			headerFound = true
			break
		}
	}
	assert.True(t, headerFound, "should extract header text 'Header Text'")

	// Body text should also be extracted
	bodyFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Body text here") {
			bodyFound = true
			break
		}
	}
	assert.True(t, bodyFound, "should extract body text 'Body text here'")
}

// okapi: WikiFilterTest#testTable
//
// MediaWiki-specific: `{| ... |}` table syntax is not part of DokuWiki.
// The DokuWiki-syntax counterpart is exercised by TestExtract_DokuWikiTable.
// Pinned to Variant=mediawiki because okf_wiki defaults to DokuWiki (#496).
func TestExtract_Table(t *testing.T) {
	wikiText := "{|\n|-\n| Cell 1 || Cell 2\n|-\n| Cell 3 || Cell 4\n|}"
	parts := readMediaWiki(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "table should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	cell1Found := false
	cell2Found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Cell 1") {
			cell1Found = true
		}
		if strings.Contains(txt, "Cell 2") {
			cell2Found = true
		}
	}
	assert.True(t, cell1Found, "should extract table cell 'Cell 1'")
	assert.True(t, cell2Found, "should extract table cell 'Cell 2'")
}

// okapi: WikiFilterTest#testImageCaption
//
// MediaWiki-specific: `[[File:...|...|caption]]` is the MediaWiki image
// link syntax. DokuWiki uses `{{image.ext|caption}}` which is currently
// unrecognised by the native reader (tracked as a separate divergence
// in spec.yaml under image_caption_extraction). Pinned to
// Variant=mediawiki to keep MediaWiki coverage after the default flipped
// to DokuWiki to match okf_wiki (#496).
func TestExtract_ImageCaption(t *testing.T) {
	wikiText := "[[File:Example.jpg|thumb|A caption for the image]]"
	parts := readMediaWiki(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "image caption should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	captionFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "caption") {
			captionFound = true
			break
		}
	}
	assert.True(t, captionFound, "should extract image caption text")
}

// okapi: WikiFilterTest#testSimilarHtmlTags
func TestExtract_SimilarHtmlTags(t *testing.T) {
	wikiText := "Text with <b>bold</b> and <br/> tags."
	parts := readDefault(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "wiki content with HTML tags should produce blocks")

	texts := testutil.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "bold") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract text containing 'bold'")
}

// okapi: WikiFilterTest#testSimilarHtmlTags
// Mirrors the upstream assertion that a real `<file>` block tag cuts the
// surrounding text unit off at the opener (yielding "This is"), even
// mid-line, while a near-miss `<files>` is not a tag and passes through.
func TestExtract_FileTagTruncatesExtraction(t *testing.T) {
	// `<file>` is a DokuWiki untranslatable block tag: extraction stops
	// at the opener, so only the leading prose is translatable.
	parts := readDefault(t, "This is <file> a test.")
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "This is", blocks[0].SourceText())

	// `<files>` is defeated by the `\b` word boundary in the opener
	// pattern, so it is treated as plain text and flows through whole.
	parts = readDefault(t, "This is <files> a test.")
	blocks = testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "This is <files> a test.", blocks[0].SourceText())

	// A `<code>` opener behaves identically.
	parts = readDefault(t, "Before <code> after.")
	blocks = testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Before", blocks[0].SourceText())
}

// A mid-line `<file>…</file>` pair that closes on the same line leaves
// only the prose before the opener translatable, and — crucially — the
// block does not stay open, so the following line is extracted normally.
// The remainder of the opener line (the bracketed region and any text
// after the closer) is treated as the block's non-translatable content.
func TestExtract_FileTagClosedInline(t *testing.T) {
	parts := readDefault(t, "Lead <file>x = 1;</file>\nNext line.")
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Lead", blocks[0].SourceText())
	assert.Equal(t, "Next line.", blocks[1].SourceText())
}

// okapi: WikiFilterTest#testComplexSeparatingWhitespace
func TestExtract_ComplexSeparatingWhitespace(t *testing.T) {
	wikiText := "First paragraph.\n\n\nSecond paragraph."
	parts := readDefault(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "complex whitespace content should produce blocks")

	texts := testutil.BlockTexts(blocks)
	firstFound := false
	secondFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "First paragraph") {
			firstFound = true
		}
		if strings.Contains(txt, "Second paragraph") {
			secondFound = true
		}
	}
	assert.True(t, firstFound, "should extract 'First paragraph'")
	assert.True(t, secondFound, "should extract 'Second paragraph'")
}

// okapi: WikiFilterTest#testDoubleExtraction
// okapi: WikXliffCompareIT#wikiXliffCompareFiles — native extract→write→re-extract verifies extracted content is stable; Okapi's wikiXliffCompareFiles extracts to XLIFF and compares against a gold XLIFF corpus.
func TestExtract_DoubleExtraction(t *testing.T) {
	input := "== Title ==\nSome text.\n\nAnother paragraph."
	ctx := t.Context()

	// First extraction
	reader1 := wiki.NewReader()
	err := reader1.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts1 := testutil.CollectParts(t, reader1.Read(ctx))
	reader1.Close()

	// Write output
	var buf bytes.Buffer
	writer := wiki.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	err = writer.Write(ctx, testutil.PartsToChannel(parts1))
	require.NoError(t, err)
	writer.Close()

	// Second extraction from output
	reader2 := wiki.NewReader()
	err = reader2.Open(ctx, testutil.RawDocFromString(buf.String(), model.LocaleEnglish))
	require.NoError(t, err)
	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	reader2.Close()

	blocks1 := testutil.FilterBlocks(parts1)
	blocks2 := testutil.FilterBlocks(parts2)

	require.NotEmpty(t, blocks1, "first extraction should produce blocks")
	require.NotEmpty(t, blocks2, "second extraction should produce blocks")
	assert.Equal(t, len(blocks1), len(blocks2), "double extraction should produce same block count")
}

// okapi: WikiFilterTest#testOpenTwiceWithString
func TestExtract_OpenTwiceWithString(t *testing.T) {
	wiki1 := "First document content."
	wiki2 := "Second document content."

	parts1 := readDefault(t, wiki1)
	parts2 := readDefault(t, wiki2)

	blocks1 := testutil.FilterBlocks(parts1)
	blocks2 := testutil.FilterBlocks(parts2)

	require.NotEmpty(t, blocks1, "first open should produce blocks")
	require.NotEmpty(t, blocks2, "second open should produce blocks")

	texts1 := testutil.BlockTexts(blocks1)
	texts2 := testutil.BlockTexts(blocks2)
	assert.Contains(t, texts1, "First document content.")
	assert.Contains(t, texts2, "Second document content.")
}

// ---- Additional extraction tests ----

// okapi: WikiFilterTest#testSimpleLine (layer structure variant)
func TestExtract_LayerStructure(t *testing.T) {
	parts := readDefault(t, "Hello wiki world")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// okapi: WikiFilterTest#testSimpleLine (block IDs uniqueness)
func TestExtract_BlockIDs(t *testing.T) {
	parts := readDefault(t, "Line A.\n\nLine B.\n\nLine C.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: WikiFilterTest#testSimpleLine (segment structure)
func TestExtract_SegmentIDs(t *testing.T) {
	parts := readDefault(t, "Hello wiki world.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source content")
		assert.NotEmpty(t, b.SourceText(), "block should have content")
	}
}

// okapi: WikiFilterTest#testHeader (multiple heading levels)
func TestExtract_MultipleHeadingLevels(t *testing.T) {
	wikiText := "== Level 2 ==\n=== Level 3 ===\n==== Level 4 ===="
	parts := readDefault(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "multiple heading levels should produce blocks")

	texts := testutil.BlockTexts(blocks)
	level2 := false
	level3 := false
	level4 := false
	for _, txt := range texts {
		if strings.Contains(txt, "Level 2") {
			level2 = true
		}
		if strings.Contains(txt, "Level 3") {
			level3 = true
		}
		if strings.Contains(txt, "Level 4") {
			level4 = true
		}
	}
	assert.True(t, level2, "should extract Level 2 heading")
	assert.True(t, level3, "should extract Level 3 heading")
	assert.True(t, level4, "should extract Level 4 heading")
}

// okapi: WikiFilterTest#testMultipleLines (full-file extraction: simple.wiki)
//
// The fixture uses MediaWiki list markers (`# ...`). Pinned to MediaWiki
// for stability after the default flipped to DokuWiki under #496.
func TestExtract_SimpleWikiFile(t *testing.T) {
	ctx := t.Context()
	f, err := os.Open("testdata/simple.wiki")
	require.NoError(t, err)

	reader := newMediaWikiReader(t)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.wiki", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "simple.wiki should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	titleFound := false
	foxFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Title") {
			titleFound = true
		}
		if strings.Contains(txt, "quick brown fox") {
			foxFound = true
		}
	}
	assert.True(t, titleFound, "should extract 'Title' from simple.wiki")
	assert.True(t, foxFound, "should extract 'quick brown fox' from simple.wiki")
}

// okapi: WikiFilterTest#testMultipleLines (full-file extraction: mediawiki.wiki)
//
// Pinned to Variant=mediawiki — the fixture is MediaWiki markup (`”'bold”'`,
// `[[File:...|...]]`, `{| ... |}` tables). The native reader's MediaWiki
// support remains available via explicit Variant config after #496 flipped
// the default to DokuWiki.
func TestExtract_MediawikiFile(t *testing.T) {
	ctx := t.Context()
	f, err := os.Open("testdata/mediawiki.wiki")
	require.NoError(t, err)

	reader := newMediaWikiReader(t)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/mediawiki.wiki", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "mediawiki.wiki should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	landgroveFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Landgrove") {
			landgroveFound = true
			break
		}
	}
	assert.True(t, landgroveFound, "should extract 'Landgrove' from mediawiki.wiki")
}

// okapi: WikiFilterTest#testDoubleExtraction (full-file: dokuwiki.txt)
func TestExtract_DokuWikiFile(t *testing.T) {
	ctx := t.Context()
	f, err := os.Open("testdata/dokuwiki.txt")
	require.NoError(t, err)

	reader := newDokuWikiReader(t)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/dokuwiki.txt", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "dokuwiki.txt should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	formattingFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Formatting Syntax") {
			formattingFound = true
			break
		}
	}
	assert.True(t, formattingFound, "should extract 'Formatting Syntax' from dokuwiki.txt")
}

// ---- WikiWriterTest ----

// okapi: WikiWriterTest#testOutput
func TestWrite_Output(t *testing.T) {
	wikiText := "== Title ==\nSimple text."
	output := roundtrip(t, wikiText)
	assert.Contains(t, output, "Title", "title should survive roundtrip")
	assert.Contains(t, output, "Simple text", "body text should survive roundtrip")
}

// okapi: WikiWriterTest#testOutputTable
//
// MediaWiki-specific table syntax (`{| ... |}`). Pinned to MediaWiki
// because the default switched to DokuWiki under #496.
func TestWrite_OutputTable(t *testing.T) {
	wikiText := "{|\n|-\n| Cell 1 || Cell 2\n|-\n| Cell 3 || Cell 4\n|}"
	output := roundtripMediaWiki(t, wikiText)
	assert.Contains(t, output, "Cell 1", "table cell 1 should survive roundtrip")
	assert.Contains(t, output, "Cell 2", "table cell 2 should survive roundtrip")
}

// okapi: WikiWriterTest#testWhitespaces
func TestWrite_Whitespaces(t *testing.T) {
	wikiText := "First paragraph.\n\nSecond paragraph."
	output := roundtrip(t, wikiText)
	assert.Contains(t, output, "First paragraph", "first paragraph should survive roundtrip")
	assert.Contains(t, output, "Second paragraph", "second paragraph should survive roundtrip")
}

// ---- RoundTrip tests ----

// okapi: RoundTripWikiIT (simple inline snippet)
func TestRoundTrip_Simple(t *testing.T) {
	output := roundtrip(t, "== Title ==\nSimple text here.\n")
	assert.Contains(t, output, "Title")
	assert.Contains(t, output, "Simple text here")
}

// okapi: RoundTripWikiIT (multi-line snippet)
func TestRoundTrip_MultiLine(t *testing.T) {
	output := roundtrip(t, "== Title ==\nFirst line.\nSecond line.\n")
	assert.Contains(t, output, "Title")
	assert.Contains(t, output, "First line")
	assert.Contains(t, output, "Second line")
}

// okapi: RoundTripWikiIT (table snippet)
//
// MediaWiki-specific table syntax. Pinned to MediaWiki under #496.
func TestRoundTrip_Table(t *testing.T) {
	output := roundtripMediaWiki(t, "{|\n|-\n| Cell 1 || Cell 2\n|-\n| Cell 3 || Cell 4\n|}")
	require.NotEmpty(t, output, "roundtrip should produce output")
	assert.Contains(t, output, "Cell 1")
}

// okapi: RoundTripWikiIT (header roundtrip)
func TestRoundTrip_Header(t *testing.T) {
	output := roundtrip(t, "== Header ==\nBody text.\n")
	require.NotEmpty(t, output, "roundtrip should produce output")
	assert.Contains(t, output, "Header")
	assert.Contains(t, output, "Body text")
}

// okapi: RoundTripWikiIT (image caption roundtrip)
//
// MediaWiki-specific `[[File:...|...|caption]]` image syntax. Pinned to
// MediaWiki under #496.
func TestRoundTrip_ImageCaption(t *testing.T) {
	output := roundtripMediaWiki(t, "[[File:Example.jpg|thumb|A caption for the image]]")
	require.NotEmpty(t, output, "roundtrip should produce output")
}

// okapi: RoundTripWikiIT (wiki markup formatting roundtrip)
//
// Mixes MediaWiki-specific (`”'bold”'`, `”italic”`) and shared
// (`==` headers, `[[link|text]]`) constructs. Pinned to MediaWiki because
// the bold/italic patterns rely on MediaWiki conventions; the default
// flipped to DokuWiki under #496.
func TestRoundTrip_WikiFormatting(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"bold", "'''Bold text''' here."},
		{"italic", "''Italic text'' here."},
		{"link", "A [[link|link text]] in text."},
		{"header_l2", "== Header 2 =="},
		{"header_l3", "=== Header 3 ==="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := roundtripMediaWiki(t, tt.input)
			require.NotEmpty(t, output, "roundtrip should produce output for %s", tt.name)
		})
	}
}

// okapi: RoundTripWikiIT#wikiFiles — native extract→write over the .wiki fixture corpus; Okapi's wikiFiles does extract→merge→compare-events over a wiki corpus.
// okapi-skip: RoundTripWikiIT#wikiSerializedFiles — Okapi serialized-skeleton variant; native uses its own skeleton store, not Okapi's serialized event/skeleton format.
//
// `.wiki` files are MediaWiki markup. Pinned to MediaWiki under #496.
func TestRoundTrip_WikiFiles(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"simple", "testdata/simple.wiki"},
		{"mediawiki", "testdata/mediawiki.wiki"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			f, err := os.Open(tt.file)
			require.NoError(t, err)

			reader := newMediaWikiReader(t)
			err = reader.Open(ctx, testutil.RawDocFromReader(f, tt.file, model.LocaleEnglish))
			require.NoError(t, err)
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			var buf bytes.Buffer
			writer := wiki.NewWriter()
			err = writer.SetOutputWriter(&buf)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)
			err = writer.Write(ctx, testutil.PartsToChannel(parts))
			require.NoError(t, err)
			writer.Close()

			require.NotEmpty(t, buf.String(), "roundtrip should produce output for %s", tt.file)
		})
	}
}

// neokapi note: DokuWiki .txt roundtrip companion to TestRoundTrip_WikiFiles
// (which carries the RoundTripWikiIT#wikiFiles mapping for the .wiki corpus).
func TestRoundTrip_DokuWikiTxtFiles(t *testing.T) {
	ctx := t.Context()
	f, err := os.Open("testdata/dokuwiki.txt")
	require.NoError(t, err)

	reader := newDokuWikiReader(t)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/dokuwiki.txt", model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := wiki.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	require.NotEmpty(t, buf.String(), "roundtrip should produce output for dokuwiki.txt")
}

// ---- Additional tests ----

func TestRead_NameAndMimeType(t *testing.T) {
	reader := wiki.NewReader()
	assert.Equal(t, "wiki", reader.Name())
	assert.Equal(t, "Wiki", reader.DisplayName())

	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-wiki")
	assert.Contains(t, sig.Extensions, ".wiki")
}

func TestRead_Configurations(t *testing.T) {
	reader := wiki.NewReader()
	cfg := reader.Config()
	assert.Equal(t, "wiki", cfg.FormatName())

	wikiCfg, ok := cfg.(*wiki.Config)
	require.True(t, ok)
	// Default Variant flipped to DokuWiki under #496 to match the
	// okf_wiki bridge contract (the upstream WikiFilter is DokuWiki-only).
	assert.Equal(t, wiki.VariantDokuWiki, wikiCfg.Variant)

	// Reset restores defaults
	wikiCfg.Variant = wiki.VariantMediaWiki
	wikiCfg.Reset()
	assert.Equal(t, wiki.VariantDokuWiki, wikiCfg.Variant)
}

func TestRead_LoadParams(t *testing.T) {
	cfg := &wiki.Config{}
	err := cfg.ApplyMap(map[string]any{"variant": "dokuwiki"})
	require.NoError(t, err)
	assert.Equal(t, wiki.VariantDokuWiki, cfg.Variant)

	// Unknown parameter should error
	err = cfg.ApplyMap(map[string]any{"unknownParam": "value"})
	require.Error(t, err)

	// Wrong type should error
	err = cfg.ApplyMap(map[string]any{"variant": 123})
	require.Error(t, err)
}

func TestRead_EmptyInput(t *testing.T) {
	parts := readDefault(t, "")
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestRead_NilDocument(t *testing.T) {
	ctx := t.Context()
	reader := wiki.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestRead_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	reader := wiki.NewReader()
	// Create a large input
	lines := make([]string, 10000)
	for i := range lines {
		lines[i] = "This is a test line for cancellation"
	}
	input := strings.Join(lines, "\n\n")

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)

	count := 0
	for range ch {
		count++
		if count >= 5 {
			cancel()
			break
		}
	}
	// Drain remaining
	for range ch {
	}

	assert.GreaterOrEqual(t, count, 5, "should have read at least 5 parts before cancel")
}

func TestRead_Synchronization(t *testing.T) {
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			ctx := t.Context()
			reader := wiki.NewReader()
			err := reader.Open(ctx, testutil.RawDocFromString("== Title ==\nHello", model.LocaleEnglish))
			if err != nil {
				t.Errorf("Open failed: %v", err)
				return
			}
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()
			blocks := testutil.FilterBlocks(parts)
			if len(blocks) < 1 {
				t.Errorf("expected at least 1 block, got %d", len(blocks))
			}
		})
	}
	wg.Wait()
}

func TestRoundTrip_WithTargetLocale(t *testing.T) {
	ctx := t.Context()

	reader := wiki.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("== Title ==\nHello world.", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set French targets
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Title" {
				block.SetTargetText(model.LocaleFrench, "Titre")
			} else if block.SourceText() == "Hello world." {
				block.SetTargetText(model.LocaleFrench, "Bonjour le monde.")
			}
		}
	}

	// Write with French locale
	var buf bytes.Buffer
	writer := wiki.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	assert.Contains(t, buf.String(), "Titre")
	assert.Contains(t, buf.String(), "Bonjour le monde.")
}

func TestRead_Schema(t *testing.T) {
	cfg := &wiki.Config{}
	schema := cfg.Schema()
	assert.Equal(t, "Wiki Filter", schema.Title)
	assert.Equal(t, "wiki", schema.FormatMeta.ID)
	assert.Contains(t, schema.FormatMeta.MimeTypes, "text/x-wiki")
	assert.Contains(t, schema.Properties, "variant")
}

// okapi: WikiFilterTest#testTable (DokuWiki variant)
func TestExtract_DokuWikiTable(t *testing.T) {
	wikiText := "^ Header 1 ^ Header 2 ^\n| Cell 1 | Cell 2 |"
	reader := newDokuWikiReader(t)
	parts := readString(t, reader, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "DokuWiki table should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	header1Found := false
	cell1Found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Header 1") {
			header1Found = true
		}
		if strings.Contains(txt, "Cell 1") {
			cell1Found = true
		}
	}
	assert.True(t, header1Found, "should extract DokuWiki table header 'Header 1'")
	assert.True(t, cell1Found, "should extract DokuWiki table cell 'Cell 1'")
}

// okapi: WikiFilterTest#testHeader (DokuWiki variant)
func TestExtract_DokuWikiHeader(t *testing.T) {
	wikiText := "====== Title ======\nBody text."
	reader := newDokuWikiReader(t)
	parts := readString(t, reader, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := testutil.BlockTexts(blocks)
	titleFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Title") {
			titleFound = true
			break
		}
	}
	assert.True(t, titleFound, "should extract DokuWiki title")
}

// TestExtract_UpstreamDokuWikiFixture exercises the default reader
// against the upstream Okapi WikiFilter test resource
// `okapi/filters/wiki/src/test/resources/dokuwiki.txt`. Regression for
// #496: with the wrong default Variant (MediaWiki), the reader produced
// systematically divergent output from the bridge on real DokuWiki
// documents. With the default flipped to DokuWiki, the canonical
// landmark texts in the upstream fixture are extracted as Blocks.
//
// Skips cleanly when okapi-testdata is not present (developers can
// fetch via scripts/fetch-okapi-testdata.sh; CI fetches it as part of
// the parity job).
func TestExtract_UpstreamDokuWikiFixture(t *testing.T) {
	root, err := spec.FindOkapiTestdataRoot()
	if err != nil {
		t.Skipf("okapi-testdata not available: %v", err)
	}
	path := filepath.Join(root, "okapi/filters/wiki/src/test/resources/dokuwiki.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("upstream fixture not available at %s: %v", path, err)
	}
	defer f.Close()

	ctx := t.Context()
	reader := wiki.NewReader() // default Variant = DokuWiki under #496
	err = reader.Open(ctx, testutil.RawDocFromReader(f, path, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "upstream dokuwiki.txt should produce translatable blocks")

	// Lower-bound block count — exact counts diverge from the bridge on
	// known per-feature gaps tracked in spec.yaml, but the document is
	// rich enough that any reader producing fewer than a handful of
	// blocks is broken.
	assert.GreaterOrEqual(t, len(blocks), 10,
		"upstream dokuwiki.txt should produce at least 10 blocks")

	// Key landmark texts the upstream document is known to contain.
	texts := testutil.BlockTexts(blocks)
	wantSubstrings := []string{
		"Formatting Syntax",     // top-level H1 headline
		"Basic Text Formatting", // H2 headline
		"Links",                 // H2 headline
		"DokuWiki",              // body text
	}
	for _, want := range wantSubstrings {
		found := false
		for _, txt := range texts {
			if strings.Contains(txt, want) {
				found = true
				break
			}
		}
		assert.True(t, found, "upstream dokuwiki.txt should surface %q in some Block", want)
	}
}

// TestExtract_DokuWikiInlineCodes asserts the reader tokenises DokuWiki
// inline link / image / formatting markup as inline-code runs (Ph,
// PcOpen, PcClose) rather than translatable text. Without this the
// pseudo round-trip mangles `[[doku>DokuWiki]]` into
// `[[ďōķũ>ĎōķũŴĩķĩ]]`, driving the wiki parity harness away from byte
// parity with the okapi reference (#522 follow-up).
func TestExtract_DokuWikiInlineCodes(t *testing.T) {
	type kindCheck struct {
		kind string // "text", "ph", "pcopen", "pcclose"
		text string // for text runs: substring; for codes: opaque Data substring
	}
	tests := []struct {
		name           string
		input          string
		blockSelectIdx int // which Block index to inspect (defaults to 0).
		want           []kindCheck
	}{
		{
			name:  "bare_link_is_placeholder",
			input: "[[doku>DokuWiki]] supports markup.",
			want: []kindCheck{
				{kind: "ph", text: "[[doku>DokuWiki]]"},
				{kind: "text", text: " supports markup."},
			},
		},
		{
			name:  "named_link_emits_paired_code",
			input: "see [[playground:playground|playground]] page.",
			want: []kindCheck{
				{kind: "text", text: "see "},
				{kind: "pcopen", text: "[[playground:playground|"},
				{kind: "text", text: "playground"},
				{kind: "pcclose", text: "]]"},
				{kind: "text", text: " page."},
			},
		},
		{
			name:  "image_is_placeholder",
			input: "Real size: {{wiki:dokuwiki-128.png}} ok.",
			want: []kindCheck{
				{kind: "text", text: "Real size: "},
				{kind: "ph", text: "{{wiki:dokuwiki-128.png}}"},
				{kind: "text", text: " ok."},
			},
		},
		{
			name:  "bold_marker_is_paired_code",
			input: "DokuWiki supports **bold** texts.",
			want: []kindCheck{
				{kind: "text", text: "DokuWiki supports "},
				{kind: "pcopen", text: "**"},
				{kind: "text", text: "bold"},
				{kind: "pcclose", text: "**"},
				{kind: "text", text: " texts."},
			},
		},
		{
			// Verifies the italic guard: `http://` does NOT open an
			// italic run (`(?<!:)//` lookbehind in WikiPatterns) and
			// `//really// ` opens at the space-flanked second `//` then
			// closes at `// ` (the closing `//` must be followed by
			// whitespace or end-of-string per `//(?=\s|$)`).
			name:  "italic_skips_url_colon",
			input: "see http://www.google.com or //really// stop.",
			want: []kindCheck{
				{kind: "text", text: "see http://www.google.com or "},
				{kind: "pcopen", text: "//"},
				{kind: "text", text: "really"},
				{kind: "pcclose", text: "//"},
				{kind: "text", text: " stop."},
			},
		},
		{
			name:  "html_sub_tag_is_paired_code",
			input: "use <sub>subscript</sub> here.",
			want: []kindCheck{
				{kind: "text", text: "use "},
				{kind: "pcopen", text: "<sub>"},
				{kind: "text", text: "subscript"},
				{kind: "pcclose", text: "</sub>"},
				{kind: "text", text: " here."},
			},
		},
		{
			name:  "unmatched_opener_stays_text",
			input: "this text has {{ unmatched braces.",
			want: []kindCheck{
				{kind: "text", text: "this text has {{ unmatched braces."},
			},
		},
		{
			// emitDokuWikiParagraphWithImages routes paragraphs that
			// contain `{{…}}` images through a path that emits a
			// dedicated caption Block (Pass 1) before the surrounding
			// paragraph Block (Pass 2). Skip the caption Block here
			// and check the paragraph Block, which is where the
			// image's captioned tokenisation lives.
			name:           "captioned_image_splits_around_caption",
			input:          "see {{ wiki:dokuwiki-128.png |This is the caption}} below.",
			blockSelectIdx: 1,
			want: []kindCheck{
				{kind: "text", text: "see "},
				{kind: "pcopen", text: "{{ wiki:dokuwiki-128.png |"},
				{kind: "text", text: "This is the caption"},
				{kind: "pcclose", text: "}}"},
				{kind: "text", text: " below."},
			},
		},
		{
			name:  "macro_is_placeholder",
			input: "use ~~NOTOC~~ to disable.",
			want: []kindCheck{
				{kind: "text", text: "use "},
				{kind: "ph", text: "~~NOTOC~~"},
				{kind: "text", text: " to disable."},
			},
		},
		{
			name:  "info_macro_with_word_is_placeholder",
			input: "see ~~INFO:syntaxplugins~~ for plugins.",
			want: []kindCheck{
				{kind: "text", text: "see "},
				{kind: "ph", text: "~~INFO:syntaxplugins~~"},
				{kind: "text", text: " for plugins."},
			},
		},
		{
			name:  "nowiki_percent_suppresses_inner_tokenization",
			input: "raw %%~~NOTOC~~%% kept.",
			want: []kindCheck{
				{kind: "text", text: "raw "},
				{kind: "pcopen", text: "%%"},
				{kind: "text", text: "~~NOTOC~~"},
				{kind: "pcclose", text: "%%"},
				{kind: "text", text: " kept."},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := readDefault(t, tt.input)
			blocks := testutil.FilterBlocks(parts)
			require.Greater(t, len(blocks), tt.blockSelectIdx, "expected at least %d block(s)", tt.blockSelectIdx+1)
			runs := blocks[tt.blockSelectIdx].Source
			require.Len(t, runs, len(tt.want), "run count mismatch: got %d runs, want %d (runs=%+v)", len(runs), len(tt.want), runs)
			for i, w := range tt.want {
				switch w.kind {
				case "text":
					require.NotNil(t, runs[i].Text, "run %d: expected TextRun", i)
					assert.Equal(t, w.text, runs[i].Text.Text, "run %d text", i)
				case "ph":
					require.NotNil(t, runs[i].Ph, "run %d: expected Ph", i)
					assert.Equal(t, w.text, runs[i].Ph.Data, "run %d Ph data", i)
				case "pcopen":
					require.NotNil(t, runs[i].PcOpen, "run %d: expected PcOpen", i)
					assert.Equal(t, w.text, runs[i].PcOpen.Data, "run %d PcOpen data", i)
				case "pcclose":
					require.NotNil(t, runs[i].PcClose, "run %d: expected PcClose", i)
					assert.Equal(t, w.text, runs[i].PcClose.Data, "run %d PcClose data", i)
				default:
					t.Fatalf("unknown kind: %s", w.kind)
				}
			}
		})
	}
}
