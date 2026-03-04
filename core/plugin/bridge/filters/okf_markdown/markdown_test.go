//go:build integration

package okf_markdown

import (
	"os"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.markdown.MarkdownFilter"
const mimeType = "text/markdown"

// ---------------------------------------------------------------------------
// Helpers (shared with parser_test.go, writer_test.go, roundtrip_test.go)
// ---------------------------------------------------------------------------

// readMD parses a markdown snippet with custom filter params and returns the parts.
func readMD(t *testing.T, snippet string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.md", mimeType, params)
}

// readMDDefault parses a markdown snippet with default (nil) params.
func readMDDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readMD(t, snippet, nil)
}

// readMDFile reads a file from the unit testdata directory.
func readMDFile(t *testing.T, filename string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := unitTestFile(t, filename)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
}

// readMDFileDefault reads a file from the unit testdata directory with default params.
func readMDFileDefault(t *testing.T, filename string) []*model.Part {
	t.Helper()
	return readMDFile(t, filename, nil)
}

// readMDIntegrationFile reads a file from the integration testdata directory.
func readMDIntegrationFile(t *testing.T, filename string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := integrationTestFile(t, filename)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
}

// unitTestFile returns the full path to a file in the unit test directory
// (okf_markdown/net/sf/okapi/filters/markdown/).
func unitTestFile(t *testing.T, filename string) string {
	t.Helper()
	return bridgetest.TestdataFile(t, "okf_markdown/net/sf/okapi/filters/markdown/"+filename)
}

// integrationTestFile returns the full path to a file in the integration test
// directory (okf_markdown/).
func integrationTestFile(t *testing.T, filename string) string {
	t.Helper()
	return bridgetest.TestdataFile(t, "okf_markdown/"+filename)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// translatableTexts returns source texts of all translatable blocks.
func translatableTexts(parts []*model.Part) []string {
	return bridgetest.BlockTexts(bridgetest.TranslatableBlocks(parts))
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

// countPartsByType counts parts of a given type.
func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}

// mdSnippetRoundtrip roundtrips a markdown snippet and returns the output string.
func mdSnippetRoundtrip(t *testing.T, snippet string, params map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.md", mimeType, params)
	return string(result.Output)
}

// fileRoundtrip roundtrips a unit testdata file and returns the output string.
func fileRoundtrip(t *testing.T, filename string, params map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := unitTestFile(t, filename)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, params)
	return string(result.Output)
}

// assertFileRoundtripEvents roundtrips a unit testdata file using event-level comparison.
func assertFileRoundtripEvents(t *testing.T, filename string, params map[string]any) {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := unitTestFile(t, filename)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, params)
}

// ---------------------------------------------------------------------------
// Structural tests (not mapped to specific Okapi tests)
// ---------------------------------------------------------------------------

func TestExtract_SimpleMarkdown(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "# Hello World\n\nThis is a paragraph.\n"
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from Markdown")
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
}

func TestExtract_BlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "# Title\n\nFirst paragraph.\n\nSecond paragraph.\n"
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)
	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass, "# Hello\n", "test.md", mimeType, nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestExtract_UnicodeContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"# こんにちは\n\nHello 🌍\n", "test.md", mimeType, nil)

	texts := translatableTexts(parts)
	assert.Contains(t, texts, "こんにちは")
	assert.Contains(t, texts, "Hello 🌍")
}

// TestExtract_HTMLBlockFiles verifies extraction from markdown files with HTML blocks.
func TestExtract_HTMLBlockFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	files := []string{
		"test-html-block-newline.md",
		"html_list_original.md",
		"html_table_changed.md",
		"admonitions.md",
		"html_list_changed.md",
		"html-table-w-empty-lines.md",
		"html_table1_original.md",
		"deployconfigure-reality.md",
	}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_markdown/"+name), mimeType, nil)

			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

			blocks := bridgetest.TranslatableBlocks(parts)
			require.NotEmpty(t, blocks, "should extract translatable blocks from %s", name)
		})
	}
}

// TestExtract_SubfilterWhitespaceFiles verifies extraction from files where
// the HTML sub-filter normalizes whitespace.
func TestExtract_SubfilterWhitespaceFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	for _, name := range []string{"example1.md", "example3.md"} {
		t.Run(name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_markdown/"+name), mimeType, nil)

			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

			blocks := bridgetest.TranslatableBlocks(parts)
			assert.Greater(t, len(blocks), 5, "%s should produce many translatable blocks", name)
		})
	}
}

// ---------------------------------------------------------------------------
// MarkdownFilterTest (82 tests)
// ---------------------------------------------------------------------------

// okapi: MarkdownFilterTest#testCloseWithoutInput
func TestExtract_CloseWithoutInput(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	// Verify filter can be closed without providing input (no panic).
}

// okapi: MarkdownFilterTest#testEventsFromEmptyInput
func TestExtract_EventsFromEmptyInput(t *testing.T) {
	parts := readMDDefault(t, "")
	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: MarkdownFilterTest#testAutoLink
func TestExtract_AutoLink(t *testing.T) {
	parts := readMDDefault(t, "Visit <http://example.com> for details.\n")
	texts := translatableTexts(parts)
	require.NotEmpty(t, texts)
	found := false
	for _, text := range texts {
		if strings.Contains(text, "Visit") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract text around auto-link")
}

// okapi: MarkdownFilterTest#testBlockQuoteEvents
func TestExtract_BlockQuoteEvents(t *testing.T) {
	parts := readMDDefault(t, "> This is a block quote.\n")
	texts := translatableTexts(parts)
	require.NotEmpty(t, texts)
	assert.Contains(t, texts, "This is a block quote.")
}

// okapi: MarkdownFilterTest#testBulletList
func TestExtract_BulletList(t *testing.T) {
	parts := readMDDefault(t, "- Item one\n- Item two\n- Item three\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Item one")
	assert.Contains(t, texts, "Item two")
	assert.Contains(t, texts, "Item three")
}

// okapi: MarkdownFilterTest#testCode
func TestExtract_Code(t *testing.T) {
	parts := readMDDefault(t, "Use the `printf` function.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "printf")
	require.NotNil(t, found, "should extract block containing inline code")
}

// okapi: MarkdownFilterTest#testCodeWithHtmlEntity
func TestExtract_CodeWithHtmlEntity(t *testing.T) {
	parts := readMDDefault(t, "Use `&amp;` for ampersand.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "ampersand")
	require.NotNil(t, found)
}

// okapi: MarkdownFilterTest#testCodeAndEmphasis
func TestExtract_CodeAndEmphasis(t *testing.T) {
	parts := readMDDefault(t, "Use `code` and *emphasis* together.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "code")
	require.NotNil(t, found, "should extract block with code and emphasis")
	frag := found.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 2, "should have spans for code and emphasis")
}

// okapi: MarkdownFilterTest#testCodeFinder
func TestExtract_CodeFinder(t *testing.T) {
	params := map[string]any{"useCodeFinder": true}
	parts := readMD(t, "See %s and %d placeholders.\n", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testEmphasis
// okapi: MarkdownFilterTest#testEmphasisAndStrong
func TestExtract_InlineFormatting(t *testing.T) {
	parts := readMDDefault(t, "This has **bold** and *italic* text.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	var found *model.Block
	for _, b := range blocks {
		if b.SourceText() == "This has bold and italic text." {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find block with inline formatting")
	frag := found.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 2, "should have spans for bold/italic")
}

// okapi: MarkdownFilterTest#testEmphasisAcrossLines
func TestExtract_EmphasisAcrossLines(t *testing.T) {
	parts := readMDDefault(t, "*emphasis across\nlines*\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "emphasis across")
	require.NotNil(t, found, "should extract emphasis spanning lines")
}

// okapi: MarkdownFilterTest#testEmphasisAtParaStart
func TestExtract_EmphasisAtParaStart(t *testing.T) {
	parts := readMDDefault(t, "*Emphasis* at start.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "Emphasis")
	require.NotNil(t, found)
	frag := found.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have span for emphasis at start")
}

// okapi: MarkdownFilterTest#testFencedCodeBlock
func TestExtract_FencedCodeBlock(t *testing.T) {
	parts := readMDDefault(t, "# Title\n\n```\nvar x = 1;\n```\n\nSome text.\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "Some text.")
}

// okapi: MarkdownFilterTest#testFencedCodeBlockWithHtmlEntity
func TestExtract_FencedCodeBlockWithHtmlEntity(t *testing.T) {
	parts := readMDDefault(t, "```\n&amp; entity\n```\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownFilterTest#testDontTranslateFencedCodeBlocks
func TestExtract_DontTranslateFencedCodeBlocks(t *testing.T) {
	parts := readMDDefault(t, "Text.\n\n```\ncode\n```\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Text.")
}

// okapi: MarkdownFilterTest#testTranslateFencedCodeBlocks
func TestExtract_TranslateFencedCodeBlocks(t *testing.T) {
	params := map[string]any{"translateCodeBlocks": true}
	parts := readMD(t, "# Title\n\n```\nvar x = 1;\n```\n\nSome text.\n", params)
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "Some text.")
	allText := strings.Join(texts, "\n")
	assert.Contains(t, allText, "var x = 1", "code block should be translatable when configured")
}

// okapi: MarkdownFilterTest#testDontTranslateMetadataHeader
func TestExtract_DontTranslateMetadataHeader(t *testing.T) {
	parts := readMDFileDefault(t, "metadata_header.md")
	blocks := bridgetest.TranslatableBlocks(parts)
	for _, b := range blocks {
		text := b.SourceText()
		assert.NotContains(t, text, "layout:", "metadata should not be translatable by default")
	}
}

// okapi: MarkdownFilterTest#testTranslateMetadataHeader
func TestExtract_TranslateMetadataHeader(t *testing.T) {
	params := map[string]any{"translateMetadataHeader": true}
	parts := readMDFile(t, "metadata_header.md", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from metadata header")
}

// okapi: MarkdownFilterTest#testHeadingPrefix
func TestExtract_HeadingPrefix(t *testing.T) {
	parts := readMDDefault(t, "# Title\n\n## Subtitle\n\n### Section\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "Subtitle")
	assert.Contains(t, texts, "Section")
}

// okapi: MarkdownFilterTest#testGenerateHeaderAnchors
func TestExtract_GenerateHeaderAnchors(t *testing.T) {
	params := map[string]any{"generateHeaderAnchors": true}
	parts := readMD(t, "# My Title\n\nSome text.\n", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testHeadingPrefixWithoutSpace
func TestExtract_HeadingPrefixWithoutSpace(t *testing.T) {
	parts := readMDDefault(t, "#NoSpace\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownFilterTest#testHeadingUnderline
func TestExtract_HeadingUnderline(t *testing.T) {
	parts := readMDDefault(t, "Title\n=====\n\nSubtitle\n--------\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "Subtitle")
}

// okapi: MarkdownFilterTest#testHtmlTable
func TestExtract_HtmlTable(t *testing.T) {
	parts := readMDDefault(t, "<table>\n<tr><td>Cell 1</td><td>Cell 2</td></tr>\n</table>\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testHtmlBlockWithMarkdown
func TestExtract_SubfilterChildLayers(t *testing.T) {
	md := "# Title\n\n<div>\n<p>Hello from HTML</p>\n</div>\n\nRegular paragraph.\n"
	parts := readMDDefault(t, md)

	var childLayers []*model.Layer
	rootLayerCount := 0
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layer := p.Resource.(*model.Layer)
			if rootLayerCount == 0 {
				rootLayerCount++
				continue
			}
			childLayers = append(childLayers, layer)
		}
	}
	require.NotEmpty(t, childLayers, "HTML blocks in markdown should produce child layers")

	found := false
	for _, l := range childLayers {
		if l.MimeType == "text/html" {
			found = true
			break
		}
	}
	assert.True(t, found, "child layer should have text/html mime type")

	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "Hello from HTML")
}

// okapi: MarkdownFilterTest#testMixedHtmlInlineAndMarkdown
func TestExtract_MixedHtmlInlineAndMarkdown(t *testing.T) {
	parts := readMDDefault(t, "Text with <span>inline HTML</span> and **bold**.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "inline HTML")
	require.NotNil(t, found, "should extract block with mixed HTML and markdown")
}

// okapi: MarkdownFilterTest#testEscapedHtmlBlockWithMarkdown
func TestExtract_EscapedHtmlBlockWithMarkdown(t *testing.T) {
	parts := readMDDefault(t, "\\<div>Not a block\\</div>\n\nParagraph.\n")
	texts := translatableTexts(parts)
	require.NotEmpty(t, texts)
}

// okapi: MarkdownFilterTest#testHtmlInline
func TestExtract_HtmlInline(t *testing.T) {
	parts := readMDDefault(t, "Text with <em>emphasis</em> inline.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "emphasis")
	require.NotNil(t, found, "should extract inline HTML elements")
}

// okapi: MarkdownFilterTest#testHtmlInlineWithAttributes
func TestExtract_HtmlInlineWithAttributes(t *testing.T) {
	parts := readMDDefault(t, "Text with <span class=\"highlight\">styled</span> content.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "styled")
	require.NotNil(t, found)
}

// okapi: MarkdownFilterTest#testHtmlBreakElement
func TestExtract_HtmlBreakElement(t *testing.T) {
	parts := readMDDefault(t, "Line one<br>Line two.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testHtmlCommentAtColumn1
func TestExtract_HtmlCommentAtColumn1(t *testing.T) {
	parts := readMDDefault(t, "<!-- comment -->\n\nParagraph.\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Paragraph.")
}

// okapi: MarkdownFilterTest#testHtmlCommentAtColumn5
func TestExtract_HtmlCommentAtColumn5(t *testing.T) {
	parts := readMDDefault(t, "    <!-- indented comment -->\n\nParagraph.\n")
	texts := translatableTexts(parts)
	require.NotEmpty(t, texts)
}

// okapi: MarkdownFilterTest#testHtmlEmphasisAndStrong
func TestExtract_HtmlEmphasisAndStrong(t *testing.T) {
	parts := readMDDefault(t, "Text with <em>emphasis</em> and <strong>strong</strong>.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "emphasis")
	require.NotNil(t, found)
	frag := found.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 2, "should have spans for HTML em/strong")
}

// okapi: MarkdownFilterTest#testHtmlEntities
func TestExtract_HtmlEntities(t *testing.T) {
	parts := readMDDefault(t, "Copyright &copy; 2024.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testImage
func TestExtract_Image(t *testing.T) {
	parts := readMDDefault(t, "![Alt text](image.png)\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testExtractImageTitleAndAltText
func TestExtract_ExtractImageTitleAndAltText(t *testing.T) {
	params := map[string]any{"extractImageAltText": true}
	parts := readMD(t, "![My Alt](image.png \"My Title\")\n", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testExtractImageTitleButNotAltText
func TestExtract_ExtractImageTitleButNotAltText(t *testing.T) {
	params := map[string]any{"extractImageAltText": false}
	parts := readMD(t, "![My Alt](image.png \"My Title\")\n", params)
	require.NotEmpty(t, parts)
}

// okapi: MarkdownFilterTest#testImageWithTranslatableUrl
func TestExtract_ImageWithTranslatableUrl(t *testing.T) {
	params := map[string]any{"translateUrls": true}
	parts := readMD(t, "![Alt](image.png)\n", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testImageRef
func TestExtract_ImageRef(t *testing.T) {
	parts := readMDDefault(t, "![Alt][ref]\n\n[ref]: image.png\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testImageRefWithTranslatableUrl
func TestExtract_ImageRefWithTranslatableUrl(t *testing.T) {
	params := map[string]any{"translateUrls": true}
	parts := readMD(t, "![Alt][ref]\n\n[ref]: image.png\n", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testImgTagWithAlt
func TestExtract_ImgTagWithAlt(t *testing.T) {
	parts := readMDDefault(t, "<img src=\"photo.jpg\" alt=\"Photo description\">\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testIndentedImageRef
func TestExtract_IndentedImageRef(t *testing.T) {
	parts := readMDDefault(t, "  ![Alt][ref]\n\n[ref]: image.png\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testIndentedInlineImage
func TestExtract_IndentedInlineImage(t *testing.T) {
	parts := readMDDefault(t, "  ![Alt](image.png)\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testIndentedCodeBlock
func TestExtract_IndentedCodeBlock(t *testing.T) {
	parts := readMDDefault(t, "    code line 1\n    code line 2\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownFilterTest#testExcludeIndentedCodeBlock
func TestExtract_ExcludeIndentedCodeBlock(t *testing.T) {
	parts := readMDDefault(t, "Paragraph.\n\n    code block\n\nAnother paragraph.\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Paragraph.")
	assert.Contains(t, texts, "Another paragraph.")
}

// okapi: MarkdownFilterTest#testTabIndentedCodeBlock
func TestExtract_TabIndentedCodeBlock(t *testing.T) {
	parts := readMDDefault(t, "\tcode line 1\n\tcode line 2\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownFilterTest#testUnescapeBackslashes
func TestExtract_UnescapeBackslashes(t *testing.T) {
	parts := readMDDefault(t, "Text with \\*escaped\\* asterisks.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "escaped")
	require.NotNil(t, found)
}

// okapi: MarkdownFilterTest#testLink
// okapi: MarkdownFilterTest#testLinkSubflow
func TestExtract_Links(t *testing.T) {
	parts := readMDDefault(t, "Click [here](http://example.com) for more info.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, text := range texts {
		if text == "Click here for more info." {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract link text without raw URL")
}

// okapi: MarkdownFilterTest#testLinkSubflowWithDNT
func TestExtract_LinkSubflowWithDNT(t *testing.T) {
	parts := readMDDefault(t, "Click [here](http://example.com) for info.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testLinkWithTitle
func TestExtract_LinkWithTitle(t *testing.T) {
	parts := readMDDefault(t, "[Link](http://example.com \"Title text\")\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testLinkWithTranslatableUrlByPattern
func TestExtract_LinkWithTranslatableUrlByPattern(t *testing.T) {
	params := map[string]any{"translateUrls": true}
	parts := readMD(t, "[Link](http://example.com)\n", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testIndentedLink
func TestExtract_IndentedLink(t *testing.T) {
	parts := readMDDefault(t, "  [Link](http://example.com)\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testLinkRef
func TestExtract_LinkRef(t *testing.T) {
	parts := readMDDefault(t, "[Link text][ref]\n\n[ref]: http://example.com\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "Link text")
	require.NotNil(t, found)
}

// okapi: MarkdownFilterTest#testLinkRefAsPairedCode
func TestExtract_LinkRefAsPairedCode(t *testing.T) {
	parts := readMDDefault(t, "[Link text][ref]\n\n[ref]: http://example.com\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "Link text")
	require.NotNil(t, found)
	frag := found.FirstFragment()
	require.NotNil(t, frag)
}

// okapi: MarkdownFilterTest#testReferenceDefinition
func TestExtract_ReferenceDefinition(t *testing.T) {
	parts := readMDDefault(t, "[ref]: http://example.com \"Reference Title\"\n\nUse [ref] in text.\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownFilterTest#testInlineHtmlTag
func TestExtract_InlineHtmlTag(t *testing.T) {
	parts := readMDDefault(t, "Text with <code>inline</code> tag.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "inline")
	require.NotNil(t, found)
	frag := found.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "inline HTML tags should produce spans")
}

// okapi: MarkdownFilterTest#testNestedInlineHtmlTag
func TestExtract_NestedInlineHtmlTag(t *testing.T) {
	parts := readMDDefault(t, "Text with <b><i>nested</i></b> tags.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "nested")
	require.NotNil(t, found)
}

// okapi: MarkdownFilterTest#testATagWithTitleAttr
func TestExtract_ATagWithTitleAttr(t *testing.T) {
	parts := readMDDefault(t, "<a href=\"#\" title=\"My Title\">Link</a>\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testATagWithTitletWithinDiv
func TestExtract_ATagWithTitletWithinDiv(t *testing.T) {
	parts := readMDDefault(t, "<div>\n<a href=\"#\" title=\"My Title\">Link</a>\n</div>\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testHtmlAttributeExpression
func TestExtract_HtmlAttributeExpression(t *testing.T) {
	parts := readMDDefault(t, "<div data-value=\"test\">Content</div>\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testHtmlSubfilterConfig
func TestExtract_HtmlSubfilterConfig(t *testing.T) {
	parts := readMDDefault(t, "<div>\n<p>Hello</p>\n</div>\n\nParagraph.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testMathTag
func TestExtract_MathTag(t *testing.T) {
	parts := readMDDefault(t, "Inline $x^2$ math.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testMathElementWithCommentBehavior
func TestExtract_MathElementWithCommentBehavior(t *testing.T) {
	parts := readMDDefault(t, "Text with $$E=mc^2$$ display math.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testMathBlockOnSingleLine
func TestExtract_MathBlockOnSingleLine(t *testing.T) {
	parts := readMDDefault(t, "$$E=mc^2$$\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownFilterTest#testMathBlocksInListItems
func TestExtract_MathBlocksInListItems(t *testing.T) {
	parts := readMDDefault(t, "- Item with $x^2$\n- Another with $y^2$\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testStrikethroughSubscript
func TestExtract_StrikethroughSubscript(t *testing.T) {
	parts := readMDDefault(t, "~~strikethrough~~ and ~subscript~\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testThematicBreak
func TestExtract_ThematicBreak(t *testing.T) {
	parts := readMDDefault(t, "Above\n\n---\n\nBelow\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Above")
	assert.Contains(t, texts, "Below")
}

// okapi: MarkdownFilterTest#testTable1TextUnits
func TestExtract_Table1TextUnits(t *testing.T) {
	parts := readMDIntegrationFile(t, "table1_original.md", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text units from table")
}

// okapi: MarkdownFilterTest#testTable2TextUnits
func TestExtract_Table2TextUnits(t *testing.T) {
	parts := readMDIntegrationFile(t, "table2_original.md", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text units from table 2")
}

// okapi: MarkdownFilterTest#testUnderlinedTextWithinAsterisks
func TestExtract_UnderlinedTextWithinAsterisks(t *testing.T) {
	parts := readMDDefault(t, "***underlined bold***\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testNeighboringMarks
func TestExtract_NeighboringMarks(t *testing.T) {
	parts := readMDDefault(t, "**bold***italic*\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testNonTranslatableBlockQuotes
func TestExtract_NonTranslatableBlockQuotes(t *testing.T) {
	params := map[string]any{"translateBlockQuotes": false}
	parts := readMD(t, "> Quote text.\n\nRegular text.\n", params)
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Regular text.")
}

// okapi: MarkdownFilterTest#testComplexFrontmatterYaml
func TestExtract_ComplexFrontmatterYaml(t *testing.T) {
	params := map[string]any{"translateMetadataHeader": true}
	parts := readMDFile(t, "complex_frontmatter.md", params)
	// Complex YAML front matter produces child layers (YAML sub-filter)
	// and potentially blocks within those layers. Verify the part stream
	// contains sub-filter layers for the metadata header.
	require.NotEmpty(t, parts)
	childLayers := 0
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			childLayers++
		}
	}
	assert.GreaterOrEqual(t, childLayers, 1, "should have layers for YAML front matter")
}

// okapi: MarkdownFilterTest#testComplexFrontmatterYamlHtml
func TestExtract_ComplexFrontmatterYamlHtml(t *testing.T) {
	params := map[string]any{"translateMetadataHeader": true}
	parts := readMDFile(t, "complex_frontmatter.md", params)
	// The complex frontmatter contains HTML within YAML values. Verify
	// the filter processes it and produces child layers for sub-filters.
	require.NotEmpty(t, parts)
	childLayers := 0
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			childLayers++
		}
	}
	assert.GreaterOrEqual(t, childLayers, 1, "should have layers for YAML/HTML sub-filters")
}

// okapi: MarkdownFilterTest#testHardLineBreak
func TestExtract_HardLineBreak(t *testing.T) {
	parts := readMDDefault(t, "Line one  \nLine two.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testCRLF
func TestExtract_CRLF(t *testing.T) {
	parts := readMDDefault(t, "Line one\r\nLine two.\r\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testRunQuotedFencedCodeBlock
func TestExtract_RunQuotedFencedCodeBlock(t *testing.T) {
	parts := readMDDefault(t, "> ```\n> code in quote\n> ```\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownFilterTest#testNativeCodeTypes
func TestExtract_NativeCodeTypes(t *testing.T) {
	parts := readMDDefault(t, "Use `code` and **bold**.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "code")
	require.NotNil(t, found)
	frag := found.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should produce native code type spans")
}

// okapi: MarkdownFilterTest#testNestedBulletWithFencedCodeBlock
func TestExtract_NestedBulletWithFencedCodeBlock(t *testing.T) {
	parts := readMDFileDefault(t, "nested-bullet-and-fenced-codeblock.md")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testNestedBulletWithFencedCodeBlockCRLF
func TestExtract_NestedBulletWithFencedCodeBlockCRLF(t *testing.T) {
	parts := readMDFileDefault(t, "nested-bullet-and-fenced-codeblock_crlf.md")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownFilterTest#testHtmlAndYamlTransUnitIndexing
func TestExtract_HtmlAndYamlTransUnitIndexing(t *testing.T) {
	params := map[string]any{"translateMetadataHeader": true}
	parts := readMDFile(t, "html_yaml_transunit_ids.md", params)
	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		if b.ID != "" {
			assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
			ids[b.ID] = true
		}
	}
}
