//go:build integration

package wiki

import (
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.wiki.WikiFilter"
const mimeType = "text/x-wiki-txt"

// readWiki parses a wiki snippet with custom filter params and returns the parts.
func readWiki(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, filterParams)
}

// readWikiDefault parses a wiki snippet with default (nil) params.
func readWikiDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readWiki(t, snippet, nil)
}

// readWikiFile reads a wiki file from testdata and returns parts.
func readWikiFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips a wiki snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.txt", mimeType, filterParams)
	return string(result.Output)
}

// findBlockContaining finds a block whose source text contains the given substring.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// readTestFile reads a file from disk and returns the content bytes.
func readTestFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ---- WikiFilterTest (11 tests) ----

// okapi-unmapped: WikiFilterTest#testDefaultInfo — Java-only API test (IFilter.getDisplayName/getMimeType)

// okapi: WikiFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readWikiDefault(t, "== Title ==\nSimple text.")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, mimeType, layer.MimeType)
	assert.Equal(t, "UTF-8", layer.Encoding)
	assert.Equal(t, model.LocaleID("en"), layer.Locale)
}

// okapi: WikiFilterTest#testSimpleLine
func TestExtract_SimpleLine(t *testing.T) {
	parts := readWikiDefault(t, "The quick brown fox jumps over the lazy dog.")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "simple line should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "The quick brown fox jumps over the lazy dog.")
}

// okapi: WikiFilterTest#testMultipleLines
func TestExtract_MultipleLines(t *testing.T) {
	parts := readWikiDefault(t, "Line one.\nLine two.\nLine three.")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "multiple lines should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	// All lines should be extracted.
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
	parts := readWikiDefault(t, "== Header Text ==\nBody text here.")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "header should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	// The header text should be extracted (without the == markers).
	headerFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Header Text") {
			headerFound = true
			break
		}
	}
	assert.True(t, headerFound, "should extract header text 'Header Text'")

	// Body text should also be extracted.
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
func TestExtract_Table(t *testing.T) {
	wiki := "{|\n|-\n| Cell 1 || Cell 2\n|-\n| Cell 3 || Cell 4\n|}"
	parts := readWikiDefault(t, wiki)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "table should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	// Table cells should be extracted.
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
func TestExtract_ImageCaption(t *testing.T) {
	wiki := "[[File:Example.jpg|thumb|A caption for the image]]"
	parts := readWikiDefault(t, wiki)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "image caption should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
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
	wiki := "Text with <b>bold</b> and <br/> tags."
	parts := readWikiDefault(t, wiki)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "wiki content with HTML tags should produce blocks")

	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "bold") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract text containing 'bold'")
}

// okapi: WikiFilterTest#testComplexSeparatingWhitespace
func TestExtract_ComplexSeparatingWhitespace(t *testing.T) {
	wiki := "First paragraph.\n\n\nSecond paragraph."
	parts := readWikiDefault(t, wiki)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "complex whitespace content should produce blocks")

	texts := bridgetest.BlockTexts(blocks)
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
func TestExtract_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// The Java testDoubleExtraction does a roundtrip for dokuwiki.txt.
	path := tdDir + "/okapi/filters/wiki/src/test/resources/dokuwiki.txt"
	content, err := readTestFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, nil)

	// Re-read the output.
	parts2 := bridgetest.ReadBytes(t, pool, cfg, filterClass, result.Output, "test.txt", mimeType, nil)
	blocks1 := bridgetest.TranslatableBlocks(result.Parts)
	blocks2 := bridgetest.TranslatableBlocks(parts2)

	require.NotEmpty(t, blocks1, "first extraction should produce blocks")
	require.NotEmpty(t, blocks2, "second extraction should produce blocks")
	assert.Equal(t, len(blocks1), len(blocks2), "double extraction should produce same block count")
}

// okapi: WikiFilterTest#testOpenTwiceWithString
func TestExtract_OpenTwiceWithString(t *testing.T) {
	// Verifies the filter can be opened, used, and then opened again with different content.
	wiki1 := "First document content."
	wiki2 := "Second document content."

	parts1 := readWikiDefault(t, wiki1)
	parts2 := readWikiDefault(t, wiki2)

	blocks1 := bridgetest.TranslatableBlocks(parts1)
	blocks2 := bridgetest.TranslatableBlocks(parts2)

	require.NotEmpty(t, blocks1, "first open should produce blocks")
	require.NotEmpty(t, blocks2, "second open should produce blocks")

	texts1 := bridgetest.BlockTexts(blocks1)
	texts2 := bridgetest.BlockTexts(blocks2)
	assert.Contains(t, texts1, "First document content.")
	assert.Contains(t, texts2, "Second document content.")
}

// ---- Additional extraction tests ----

// okapi: WikiFilterTest#testSimpleLine (layer structure variant)
func TestExtract_LayerStructure(t *testing.T) {
	parts := readWikiDefault(t, "Hello wiki world")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// okapi: WikiFilterTest#testSimpleLine (block IDs uniqueness)
func TestExtract_BlockIDs(t *testing.T) {
	parts := readWikiDefault(t, "Line A.\nLine B.\nLine C.")

	blocks := bridgetest.TranslatableBlocks(parts)
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
	parts := readWikiDefault(t, "Hello wiki world.")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotNil(t, seg, "segment should not be nil")
		}
	}
}

// okapi: WikiFilterTest#testHeader (multiple heading levels)
func TestExtract_MultipleHeadingLevels(t *testing.T) {
	wiki := "== Level 2 ==\n=== Level 3 ===\n==== Level 4 ===="
	parts := readWikiDefault(t, wiki)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "multiple heading levels should produce blocks")

	texts := bridgetest.BlockTexts(blocks)
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
func TestExtract_SimpleWikiFile(t *testing.T) {
	parts := readWikiFile(t, "integration-tests/okapi/src/test/resources/wikitext/simple.wiki", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "simple.wiki should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	// simple.wiki contains: "== Title ==" and two numbered list items about foxes.
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
func TestExtract_MediawikiFile(t *testing.T) {
	parts := readWikiFile(t, "integration-tests/okapi/src/test/resources/wikitext/mediawiki.wiki", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "mediawiki.wiki should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	// mediawiki.wiki has a Settlement infobox and body text about Landgrove, Vermont.
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
	parts := readWikiFile(t, "okapi/filters/wiki/src/test/resources/dokuwiki.txt", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "dokuwiki.txt should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	// dokuwiki.txt is a comprehensive formatting syntax reference.
	formattingFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Formatting Syntax") {
			formattingFound = true
			break
		}
	}
	assert.True(t, formattingFound, "should extract 'Formatting Syntax' from dokuwiki.txt")
}

// ---- WikiWriterTest (3 tests) ----

// okapi: WikiWriterTest#testOutput
func TestWrite_Output(t *testing.T) {
	wiki := "== Title ==\nSimple text."
	output := snippetRoundtrip(t, wiki, nil)
	assert.Contains(t, output, "Title", "title should survive roundtrip")
	assert.Contains(t, output, "Simple text", "body text should survive roundtrip")
}

// okapi: WikiWriterTest#testOutputTable
func TestWrite_OutputTable(t *testing.T) {
	wiki := "{|\n|-\n| Cell 1 || Cell 2\n|-\n| Cell 3 || Cell 4\n|}"
	output := snippetRoundtrip(t, wiki, nil)
	assert.Contains(t, output, "Cell 1", "table cell 1 should survive roundtrip")
	assert.Contains(t, output, "Cell 2", "table cell 2 should survive roundtrip")
}

// okapi: WikiWriterTest#testWhitespaces
func TestWrite_Whitespaces(t *testing.T) {
	wiki := "First paragraph.\n\nSecond paragraph."
	output := snippetRoundtrip(t, wiki, nil)
	assert.Contains(t, output, "First paragraph", "first paragraph should survive roundtrip")
	assert.Contains(t, output, "Second paragraph", "second paragraph should survive roundtrip")
}
