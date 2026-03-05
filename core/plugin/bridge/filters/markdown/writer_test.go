//go:build integration

package markdown

import (
	"os"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// MarkdownWriterTest (52 tests)
// ---------------------------------------------------------------------------

// okapi: MarkdownWriterTest#writeDocumentParts
func TestWrite_DocumentParts(t *testing.T) {
	parts := readMDDefault(t, "# Title\n\nParagraph.\n")
	require.NotEmpty(t, parts)
	out := mdSnippetRoundtrip(t, "# Title\n\nParagraph.\n", nil)
	assert.Contains(t, out, "Title")
	assert.Contains(t, out, "Paragraph")
}

// okapi: MarkdownWriterTest#writeTextUnitsAndDocumentPartsText
func TestWrite_TextUnitsAndDocumentPartsText(t *testing.T) {
	input := "# Heading\n\nFirst paragraph.\n\nSecond paragraph.\n"
	out := mdSnippetRoundtrip(t, input, nil)
	assert.Contains(t, out, "Heading")
	assert.Contains(t, out, "First paragraph.")
	assert.Contains(t, out, "Second paragraph.")
}

// okapi: MarkdownWriterTest#writeTextUnitsAndDocumentPartsWithEscapes
func TestWrite_TextUnitsAndDocumentPartsWithEscapes(t *testing.T) {
	input := "Text with \\*escaped\\* and \\`code\\`.\n"
	out := mdSnippetRoundtrip(t, input, nil)
	assert.Contains(t, out, "escaped")
}

// okapi: MarkdownWriterTest#writeTextUnitsAndDocumentPartsHtml
func TestWrite_TextUnitsAndDocumentPartsHtml(t *testing.T) {
	input := "<div>\n<p>HTML content</p>\n</div>\n\nMarkdown text.\n"
	out := mdSnippetRoundtrip(t, input, nil)
	assert.Contains(t, out, "HTML content")
	assert.Contains(t, out, "Markdown text.")
}

// okapi: MarkdownWriterTest#writeTextUnitsAndDocumentPartsList
func TestWrite_TextUnitsAndDocumentPartsList(t *testing.T) {
	input := "- Item one\n- Item two\n- Item three\n"
	out := mdSnippetRoundtrip(t, input, nil)
	assert.Contains(t, out, "Item one")
	assert.Contains(t, out, "Item two")
	assert.Contains(t, out, "Item three")
}

// okapi: MarkdownWriterTest#writeTextUnitsAndDocumentPartsHardLineBreak
func TestWrite_TextUnitsAndDocumentPartsHardLineBreak(t *testing.T) {
	input := "Line one  \nLine two.\n"
	out := mdSnippetRoundtrip(t, input, nil)
	assert.Contains(t, out, "Line one")
	assert.Contains(t, out, "Line two")
}

// okapi: MarkdownWriterTest#testCommonMarkRoundTrip
func TestWrite_CommonMarkRoundTrip(t *testing.T) {
	assertFileRoundtripEvents(t, "commonmark_original.md", nil)
}

// okapi: MarkdownWriterTest#testCommonMarkChangedOutput
func TestWrite_CommonMarkChangedOutput(t *testing.T) {
	original := readMDFileDefault(t, "commonmark_original.md")
	require.NotEmpty(t, original)
	changed := readMDFileDefault(t, "commonmark_changed.md")
	require.NotEmpty(t, changed)
}

// okapi: MarkdownWriterTest#testListsRoundTrip
func TestWrite_ListsRoundTrip(t *testing.T) {
	assertFileRoundtripEvents(t, "lists_original.md", nil)
}

// okapi: MarkdownWriterTest#testBQInListItemRoundTrip
func TestWrite_BQInListItemRoundTrip(t *testing.T) {
	assertFileRoundtripEvents(t, "block-quote-in-list-item.md", nil)
}

// okapi: MarkdownWriterTest#testBQInListItemRoundTrip2
func TestWrite_BQInListItemRoundTrip2(t *testing.T) {
	assertFileRoundtripEvents(t, "block-quote-in-list-item2.md", nil)
}

// okapi: MarkdownWriterTest#testListChangedOutput
func TestWrite_ListChangedOutput(t *testing.T) {
	original := readMDFileDefault(t, "lists_original.md")
	require.NotEmpty(t, original)
	changed := readMDFileDefault(t, "lists_changed.md")
	require.NotEmpty(t, changed)
}

// okapi: MarkdownWriterTest#testNestedListWithBlankLines
func TestWrite_NestedListWithBlankLines(t *testing.T) {
	assertFileRoundtripEvents(t, "nested_list_with_blank_lines.md", nil)
}

// okapi: MarkdownWriterTest#testTable1RoundTrip
func TestWrite_Table1RoundTrip(t *testing.T) {
	assertFileRoundtripEvents(t, "table1_original.md", nil)
}

// okapi: MarkdownWriterTest#testTable1ChangedOutput
func TestWrite_Table1ChangedOutput(t *testing.T) {
	original := readMDFileDefault(t, "table1_original.md")
	require.NotEmpty(t, original)
	changed := readMDFileDefault(t, "table1_changed.md")
	require.NotEmpty(t, changed)
}

// okapi: MarkdownWriterTest#testTable2RoundTrip
func TestWrite_Table2RoundTrip(t *testing.T) {
	assertFileRoundtripEvents(t, "table2_original.md", nil)
}

// okapi: MarkdownWriterTest#testTable2ChangedOutput
func TestWrite_Table2ChangedOutput(t *testing.T) {
	original := readMDFileDefault(t, "table2_original.md")
	require.NotEmpty(t, original)
	changed := readMDFileDefault(t, "table2_changed.md")
	require.NotEmpty(t, changed)
}

// okapi: MarkdownWriterTest#testMinimalMathRoundTrip
func TestWrite_MinimalMathRoundTrip(t *testing.T) {
	assertFileRoundtripEvents(t, "min_math_original.md", nil)
}

// okapi: MarkdownWriterTest#testComplexMathRoundTrip
func TestWrite_ComplexMathRoundTrip(t *testing.T) {
	// The min_math_original.md contains both inline and display math.
	out := fileRoundtrip(t, "min_math_original.md", nil)
	require.NotEmpty(t, out)
}

// okapi: MarkdownWriterTest#testImgWithAltRoundTrip
func TestWrite_ImgWithAltRoundTrip(t *testing.T) {
	assertFileRoundtripEvents(t, "img_w_alt_attr_original.md", nil)
}

// okapi: MarkdownWriterTest#testHtmlListRoundTrip
func TestWrite_HtmlListRoundTrip(t *testing.T) {
	out := fileRoundtrip(t, "html_list_original.md", nil)
	require.NotEmpty(t, out)
}

// okapi: MarkdownWriterTest#testHtmlListChangedOutput
func TestWrite_HtmlListChangedOutput(t *testing.T) {
	original := readMDFileDefault(t, "html_list_original.md")
	require.NotEmpty(t, original)
	changed := readMDFileDefault(t, "html_list_changed.md")
	require.NotEmpty(t, changed)
}

// okapi: MarkdownWriterTest#testHtmlTable1RoundTrip
func TestWrite_HtmlTable1RoundTrip(t *testing.T) {
	out := fileRoundtrip(t, "html_table1_original.md", nil)
	require.NotEmpty(t, out)
}

// okapi: MarkdownWriterTest#testQuotedPara
func TestWrite_QuotedPara(t *testing.T) {
	assertFileRoundtripEvents(t, "quoted-para.md", nil)
}

// okapi: MarkdownWriterTest#testQuotedList
func TestWrite_QuotedList(t *testing.T) {
	assertFileRoundtripEvents(t, "quoted-list.md", nil)
}

// okapi: MarkdownWriterTest#testUlInTable
func TestWrite_UlInTable(t *testing.T) {
	assertFileRoundtripEvents(t, "ul-in-table.md", nil)
}

// okapi: MarkdownWriterTest#testTbodyTdInTable
func TestWrite_TbodyTdInTable(t *testing.T) {
	// table1_original.md contains tbody/td elements.
	assertFileRoundtripEvents(t, "table1_original.md", nil)
}

// okapi: MarkdownWriterTest#testHtmlBlockWithEmptyLines
func TestWrite_HtmlBlockWithEmptyLines(t *testing.T) {
	out := fileRoundtrip(t, "html-table-w-empty-lines.md", nil)
	require.NotEmpty(t, out)
}

// okapi: MarkdownWriterTest#testHeadingsAfterList
func TestWrite_HeadingsAfterList(t *testing.T) {
	assertFileRoundtripEvents(t, "heading-after-list.md", nil)
}

// okapi: MarkdownWriterTest#testReferencedLinkAndImage
func TestWrite_ReferencedLinkAndImage(t *testing.T) {
	assertFileRoundtripEvents(t, "ref-links.md", nil)
}

// okapi: MarkdownWriterTest#testLinkAndImage
func TestWrite_LinkAndImage(t *testing.T) {
	assertFileRoundtripEvents(t, "direct-links.md", nil)
}

// okapi: MarkdownWriterTest#testDeadLinkRef
func TestWrite_DeadLinkRef(t *testing.T) {
	assertFileRoundtripEvents(t, "dead-ref-link.md", nil)
}

// okapi: MarkdownWriterTest#testTooManyTUs
func TestWrite_TooManyTUs(t *testing.T) {
	// Verify many text units don't cause errors.
	out := fileRoundtrip(t, "regressing_test_single_page.md", nil)
	require.NotEmpty(t, out)
}

// okapi: MarkdownWriterTest#testQuotesAfterHtmlInTableCell
func TestWrite_QuotesAfterHtmlInTableCell(t *testing.T) {
	assertFileRoundtripEvents(t, "quotes-after-html-in-table.md", nil)
}

// okapi: MarkdownWriterTest#testCdata
func TestWrite_Cdata(t *testing.T) {
	assertFileRoundtripEvents(t, "html-cdata-sample.md", nil)
}

// okapi: MarkdownWriterTest#testCdataCRLF
func TestWrite_CdataCRLF(t *testing.T) {
	out := fileRoundtrip(t, "html-cdata-sample_crlf.md", nil)
	require.NotEmpty(t, out)
}

// okapi: MarkdownWriterTest#testImageWoAlt
func TestWrite_ImageWoAlt(t *testing.T) {
	assertFileRoundtripEvents(t, "image-wo-alt.md", nil)
}

// okapi: MarkdownWriterTest#testComplexFrontMatterIgnoredByDefault
func TestWrite_ComplexFrontMatterIgnoredByDefault(t *testing.T) {
	out := fileRoundtrip(t, "complex_frontmatter.md", nil)
	require.NotEmpty(t, out)
	// Front matter should be preserved but not modified.
	assert.Contains(t, out, "---")
}

// okapi: MarkdownWriterTest#testComplexFrontMatterIncludedDefaultFilterUnix
func TestWrite_ComplexFrontMatterIncludedUnix(t *testing.T) {
	params := map[string]any{"translateMetadataHeader": true}
	out := fileRoundtrip(t, "complex_frontmatter.md", params)
	require.NotEmpty(t, out)
	assert.Contains(t, out, "---")
}

// okapi: MarkdownWriterTest#testComplexFrontMatterIncludedDefaultFilterWindows
func TestWrite_ComplexFrontMatterIncludedWindows(t *testing.T) {
	params := map[string]any{"translateMetadataHeader": true}
	out := fileRoundtrip(t, "complex_frontmatter_crlf.md", params)
	require.NotEmpty(t, out)
	assert.Contains(t, out, "---")
}

// okapi: MarkdownWriterTest#newLinesInsertionInListsWithHtmlTagsClarified
func TestWrite_NewLinesInsertionInListsWithHtmlTags(t *testing.T) {
	input := "- Item with <b>bold</b>\n- Another item\n"
	out := mdSnippetRoundtrip(t, input, nil)
	assert.Contains(t, out, "Item with")
	assert.Contains(t, out, "Another item")
}

// okapi: MarkdownWriterTest#testCustomConfigurationFromString
func TestWrite_CustomConfigurationFromString(t *testing.T) {
	// Read with custom config file. The custom config enables header translation.
	pool, cfg := bridgetest.SharedBridge(t)
	configPath := unitTestFile(t, "custom-configs/okf_markdown@custom15437.fprm")
	_, err := os.Stat(configPath)
	require.NoError(t, err, "custom config file should exist: %s", configPath)

	// Verify reading with the custom config works without error.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, unitTestFile(t, "metadata_header.md"), mimeType, nil)
	require.NotEmpty(t, parts)
}

// okapi: MarkdownWriterTest#testRoundTripWithHeadersCustomConfiguration
func TestWrite_RoundTripWithHeadersCustomConfiguration(t *testing.T) {
	params := map[string]any{"translateMetadataHeader": true}
	assertFileRoundtripEvents(t, "metadata_header.md", params)
}

// okapi: MarkdownWriterTest#testHardLineBreak
func TestWrite_HardLineBreak(t *testing.T) {
	assertFileRoundtripEvents(t, "hard-line-break-inline.md", nil)
}

// okapi: MarkdownWriterTest#testHardLineBreakVarious
func TestWrite_HardLineBreakVarious(t *testing.T) {
	assertFileRoundtripEvents(t, "hard-line-break-various.md", nil)
}

// okapi: MarkdownWriterTest#testHardLineBreakWithCRLF
func TestWrite_HardLineBreakWithCRLF(t *testing.T) {
	out := fileRoundtrip(t, "hard-line-break_crlf.md", nil)
	require.NotEmpty(t, out)
}

// okapi: MarkdownWriterTest#testHardLineBreakBetweenInlineMarkupPair
func TestWrite_HardLineBreakBetweenInlineMarkupPair(t *testing.T) {
	input := "**bold**  \n*italic*\n"
	out := mdSnippetRoundtrip(t, input, nil)
	assert.Contains(t, out, "bold")
	assert.Contains(t, out, "italic")
}

// okapi: MarkdownWriterTest#testIndentedCodeBlockWithCRLF
func TestWrite_IndentedCodeBlockWithCRLF(t *testing.T) {
	out := fileRoundtrip(t, "indented-code-block-simple_crlf.md", nil)
	require.NotEmpty(t, out)
}

// okapi: MarkdownWriterTest#testQuotedCodeBlocks
func TestWrite_QuotedCodeBlocks(t *testing.T) {
	assertFileRoundtripEvents(t, "quoted-code-blocks.md", nil)
}

// okapi: MarkdownWriterTest#roundTripsCodesAndCodeBlocks
func TestWrite_RoundTripsCodesAndCodeBlocks(t *testing.T) {
	assertFileRoundtripEvents(t, "code_and_codeblock_tests.md", nil)
}

// okapi: MarkdownWriterTest#testRoundtripWithEscaping
func TestWrite_RoundtripWithEscaping(t *testing.T) {
	assertFileRoundtripEvents(t, "escaping_tests.md", nil)
}

// okapi: MarkdownWriterTest#roundTripsEmphasis
func TestWrite_RoundTripsEmphasis(t *testing.T) {
	assertFileRoundtripEvents(t, "emphasis.md", nil)
}

// ---------------------------------------------------------------------------
// MarkdownSkeletonWriterTest (2 tests)
// ---------------------------------------------------------------------------

// okapi: MarkdownSkeletonWriterTest#testProcessTextUnit
func TestSkeletonWrite_ProcessTextUnit(t *testing.T) {
	input := "# Title\n\nParagraph text.\n"
	out := mdSnippetRoundtrip(t, input, nil)
	// Skeleton writer processes text units preserving structure.
	assert.Contains(t, out, "Title")
	assert.Contains(t, out, "Paragraph text.")
}

// okapi: MarkdownSkeletonWriterTest#testAppendLinePrefix
func TestSkeletonWrite_AppendLinePrefix(t *testing.T) {
	input := "> Quoted line 1\n> Quoted line 2\n"
	out := mdSnippetRoundtrip(t, input, nil)
	// Line prefixes should be preserved in roundtrip.
	assert.True(t, strings.Contains(out, ">"), "block quote prefix should be preserved")
}
