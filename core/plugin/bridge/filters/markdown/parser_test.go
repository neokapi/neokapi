//go:build integration

package markdown

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// MarkdownParserTest (73 tests)
// ---------------------------------------------------------------------------

// okapi: MarkdownParserTest#testParagraph
func TestParse_Paragraph(t *testing.T) {
	parts := readMDDefault(t, "Hello world.\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Hello world.")
}

// okapi: MarkdownParserTest#testAutoLink
func TestParse_AutoLink(t *testing.T) {
	parts := readMDDefault(t, "<http://example.com>\n")
	// Auto-links are parsed as links; the URL itself may not produce a
	// translatable block (it becomes a non-translatable inline code).
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: MarkdownParserTest#testBlockQuote1
func TestParse_BlockQuote1(t *testing.T) {
	parts := readMDDefault(t, "> Quote line.\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Quote line.")
}

// okapi: MarkdownParserTest#testBlockQuote2
func TestParse_BlockQuote2(t *testing.T) {
	parts := readMDDefault(t, "> Line 1\n> Line 2\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testBulletList1
func TestParse_BulletList1(t *testing.T) {
	parts := readMDDefault(t, "- Item 1\n- Item 2\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Item 1")
	assert.Contains(t, texts, "Item 2")
}

// okapi: MarkdownParserTest#testBulletList2
func TestParse_BulletList2(t *testing.T) {
	parts := readMDDefault(t, "* Item A\n* Item B\n* Item C\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Item A")
	assert.Contains(t, texts, "Item B")
	assert.Contains(t, texts, "Item C")
}

// okapi: MarkdownParserTest#testBulletListWithWhitespace
func TestParse_BulletListWithWhitespace(t *testing.T) {
	parts := readMDDefault(t, "- Item 1\n\n- Item 2\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Item 1")
	assert.Contains(t, texts, "Item 2")
}

// okapi: MarkdownParserTest#testCode1
func TestParse_Code1(t *testing.T) {
	parts := readMDDefault(t, "Use `code` here.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "code")
	require.NotNil(t, found)
}

// okapi: MarkdownParserTest#testCode2
func TestParse_Code2(t *testing.T) {
	parts := readMDDefault(t, "``double backtick`` code.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "double backtick")
	require.NotNil(t, found)
}

// okapi: MarkdownParserTest#testEmphasis1
func TestParse_Emphasis1(t *testing.T) {
	parts := readMDDefault(t, "*italic text*\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "italic text")
	require.NotNil(t, found)
	assert.True(t, bridgetest.HasInlineCode(found.SourceRuns()),
		"emphasis should produce inline-code runs")
}

// okapi: MarkdownParserTest#testEmphasis2
func TestParse_Emphasis2(t *testing.T) {
	parts := readMDDefault(t, "_italic underline_\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "italic underline")
	require.NotNil(t, found)
}

// okapi: MarkdownParserTest#testStrongEmphasis1
func TestParse_StrongEmphasis1(t *testing.T) {
	parts := readMDDefault(t, "**bold text**\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "bold text")
	require.NotNil(t, found)
	assert.True(t, bridgetest.HasInlineCode(found.SourceRuns()),
		"strong emphasis should produce inline-code runs")
}

// okapi: MarkdownParserTest#testStrongEmphasis2
func TestParse_StrongEmphasis2(t *testing.T) {
	parts := readMDDefault(t, "__bold underline__\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "bold underline")
	require.NotNil(t, found)
}

// okapi: MarkdownParserTest#testFencedCodeBlock
func TestParse_FencedCodeBlock(t *testing.T) {
	parts := readMDDefault(t, "```\ncode line\n```\n")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
}

// okapi: MarkdownParserTest#testFencedCodeBlockWithInfo
func TestParse_FencedCodeBlockWithInfo(t *testing.T) {
	parts := readMDDefault(t, "```javascript\nvar x = 1;\n```\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testFencedCodeBlockWithInfoWithSpace
func TestParse_FencedCodeBlockWithInfoWithSpace(t *testing.T) {
	parts := readMDDefault(t, "```java script\nvar x = 1;\n```\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testNestedFencedCodeBlock
func TestParse_NestedFencedCodeBlock(t *testing.T) {
	parts := readMDDefault(t, "````\n```\nnested\n```\n````\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testHardLineBreak
func TestParse_HardLineBreak(t *testing.T) {
	parts := readMDDefault(t, "Line one  \nLine two.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testSoftLineBreak
func TestParse_SoftLineBreak(t *testing.T) {
	parts := readMDDefault(t, "Line one\nLine two.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testHeading1A
func TestParse_Heading1A(t *testing.T) {
	parts := readMDDefault(t, "# Heading 1\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Heading 1")
}

// okapi: MarkdownParserTest#testHeading1AX
func TestParse_Heading1AX(t *testing.T) {
	parts := readMDDefault(t, "# Heading 1 #\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Heading 1")
}

// okapi: MarkdownParserTest#testHeading1B
func TestParse_Heading1B(t *testing.T) {
	parts := readMDDefault(t, "Heading 1\n=========\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Heading 1")
}

// okapi: MarkdownParserTest#testHeading2A
func TestParse_Heading2A(t *testing.T) {
	parts := readMDDefault(t, "## Heading 2\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Heading 2")
}

// okapi: MarkdownParserTest#testHeading2B
func TestParse_Heading2B(t *testing.T) {
	parts := readMDDefault(t, "Heading 2\n---------\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Heading 2")
}

// okapi: MarkdownParserTest#testHtmlBlock1
func TestParse_HtmlBlock1(t *testing.T) {
	parts := readMDDefault(t, "<div>\nContent\n</div>\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testHtmlBlock2
func TestParse_HtmlBlock2(t *testing.T) {
	parts := readMDDefault(t, "<table>\n<tr><td>Cell</td></tr>\n</table>\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testHtmlBlockWithMarkdown
func TestParse_HtmlBlockWithMarkdown(t *testing.T) {
	parts := readMDDefault(t, "<div>\n\n**bold in div**\n\n</div>\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testHtmlBlockMisparsedDocusaurusAdmonition
func TestParse_HtmlBlockMisparsedDocusaurusAdmonition(t *testing.T) {
	parts := readMDDefault(t, ":::note\nThis is an admonition.\n:::\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testHtmlCommentBlock
func TestParse_HtmlCommentBlock(t *testing.T) {
	parts := readMDDefault(t, "<!-- comment -->\n\nParagraph.\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "Paragraph.")
}

// okapi: MarkdownParserTest#testHtmlEntity
func TestParse_HtmlEntity(t *testing.T) {
	parts := readMDDefault(t, "&amp; &lt; &gt;\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testHtmlInline
func TestParse_HtmlInline(t *testing.T) {
	parts := readMDDefault(t, "Text <em>inline</em> here.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "inline")
	require.NotNil(t, found)
}

// okapi: MarkdownParserTest#testImage
func TestParse_Image(t *testing.T) {
	parts := readMDDefault(t, "![Alt text](image.png \"Title\")\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testImageRef
func TestParse_ImageRef(t *testing.T) {
	parts := readMDDefault(t, "![Alt][ref]\n\n[ref]: image.png\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testImageRefBug
func TestParse_ImageRefBug(t *testing.T) {
	parts := readMDDefault(t, "![Alt][]\n\n[Alt]: image.png\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testIndentedAutoLink
func TestParse_IndentedAutoLink(t *testing.T) {
	parts := readMDDefault(t, "  <http://example.com>\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testIndentedCodeBlock
func TestParse_IndentedCodeBlock(t *testing.T) {
	parts := readMDDefault(t, "    code line\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testIndentedHtml
func TestParse_IndentedHtml(t *testing.T) {
	parts := readMDDefault(t, "  <em>indented html</em>\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testIndentedHtmlBlock
func TestParse_IndentedHtmlBlock(t *testing.T) {
	parts := readMDDefault(t, "   <div>\n   Content\n   </div>\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testIndentedInlineLink
func TestParse_IndentedInlineLink(t *testing.T) {
	parts := readMDDefault(t, "  [Link](http://example.com)\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testIndentedText
func TestParse_IndentedText(t *testing.T) {
	parts := readMDDefault(t, "  Indented text.\n")
	texts := translatableTexts(parts)
	require.NotEmpty(t, texts)
}

// okapi: MarkdownParserTest#testIndentedTextInBulletList
func TestParse_IndentedTextInBulletList(t *testing.T) {
	parts := readMDDefault(t, "- Item 1\n  Continued text.\n- Item 2\n")
	texts := translatableTexts(parts)
	require.NotEmpty(t, texts)
}

// okapi: MarkdownParserTest#testLink1
func TestParse_Link1(t *testing.T) {
	parts := readMDDefault(t, "[Link](http://example.com)\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "Link")
	require.NotNil(t, found)
}

// okapi: MarkdownParserTest#testLink2
func TestParse_Link2(t *testing.T) {
	parts := readMDDefault(t, "[Link](http://example.com \"Title\")\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testLink3
func TestParse_Link3(t *testing.T) {
	parts := readMDDefault(t, "[Link](http://example.com 'Title')\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testLink4
func TestParse_Link4(t *testing.T) {
	parts := readMDDefault(t, "Text [Link](url) more text.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testLink5
func TestParse_Link5(t *testing.T) {
	parts := readMDDefault(t, "[Link1](url1) and [Link2](url2).\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testLink6
func TestParse_Link6(t *testing.T) {
	parts := readMDDefault(t, "[**Bold Link**](url)\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testLinkRef
func TestParse_LinkRef(t *testing.T) {
	parts := readMDDefault(t, "[Link text][ref]\n\n[ref]: http://example.com\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MarkdownParserTest#testLinkWithText
func TestParse_LinkWithText(t *testing.T) {
	parts := readMDDefault(t, "Before [link text](url) after.\n")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := findBlockContaining(blocks, "link text")
	require.NotNil(t, found)
}

// okapi: MarkdownParserTest#testOrderedList
func TestParse_OrderedList(t *testing.T) {
	parts := readMDDefault(t, "1. First\n2. Second\n3. Third\n")
	texts := translatableTexts(parts)
	assert.Contains(t, texts, "First")
	assert.Contains(t, texts, "Second")
	assert.Contains(t, texts, "Third")
}

// okapi: MarkdownParserTest#testOrderedListWithNestedIndents
func TestParse_OrderedListWithNestedIndents(t *testing.T) {
	parts := readMDDefault(t, "1. Item 1\n   - Sub item\n2. Item 2\n")
	texts := translatableTexts(parts)
	require.NotEmpty(t, texts)
}

// okapi: MarkdownParserTest#testReferenceDefinition1
func TestParse_ReferenceDefinition1(t *testing.T) {
	parts := readMDDefault(t, "[ref]: http://example.com\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testReferenceDefinition1plus
func TestParse_ReferenceDefinition1plus(t *testing.T) {
	parts := readMDDefault(t, "[ref]: http://example.com \"Title\"\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testReferenceDefinition2
func TestParse_ReferenceDefinition2(t *testing.T) {
	parts := readMDDefault(t, "[ref]: http://example.com\n[ref2]: http://example2.com\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testThematicBreak
func TestParse_ThematicBreak(t *testing.T) {
	parts := readMDDefault(t, "---\n")
	require.NotEmpty(t, parts)
	dataParts := bridgetest.DataParts(parts)
	assert.NotEmpty(t, dataParts, "thematic break should produce data parts")
}

// okapi: MarkdownParserTest#testCommonMarkTokens
func TestParse_CommonMarkTokens(t *testing.T) {
	parts := readMDFile(t, "parser/commonmark.md", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "CommonMark file should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, text := range texts {
		if strings.Contains(text, "CommonMark") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract CommonMark content")
}

// okapi: MarkdownParserTest#testTable1Tokens
func TestParse_Table1Tokens(t *testing.T) {
	parts := readMDFile(t, "parser/table1.md", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "table should produce translatable blocks")
}

// okapi: MarkdownParserTest#testMathlm
func TestParse_Mathlm(t *testing.T) {
	parts := readMDDefault(t, "$$\nx = y\n$$\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testMathlmInListItem
func TestParse_MathlmInListItem(t *testing.T) {
	parts := readMDDefault(t, "- Item with $$x^2$$ math\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testMathlmSingleLine
func TestParse_MathlmSingleLine(t *testing.T) {
	parts := readMDDefault(t, "$$E = mc^2$$\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testMdxExport
func TestParse_MdxExport(t *testing.T) {
	parts := readMDDefault(t, "export const meta = { title: 'My Page' };\n\n# Title\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testTitleAdmonitions
func TestParse_TitleAdmonitions(t *testing.T) {
	parts := readMDDefault(t, "!!! note \"My Note Title\"\n    Content here.\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testNoTitleAdmonitions
func TestParse_NoTitleAdmonitions(t *testing.T) {
	parts := readMDDefault(t, "!!! note\n    Content here.\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testNoHeaderAdmonitions
func TestParse_NoHeaderAdmonitions(t *testing.T) {
	parts := readMDDefault(t, "!!! \"\"\n    Content here.\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testNestedCollapsibleAdmonitions
func TestParse_NestedCollapsibleAdmonitions(t *testing.T) {
	parts := readMDDefault(t, "??? note \"Collapsible\"\n    Content.\n\n    ??? warning\n        Nested.\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testAdmonitionWhitespaceBeforeHeading
func TestParse_AdmonitionWhitespaceBeforeHeading(t *testing.T) {
	parts := readMDDefault(t, "\n!!! note \"Title\"\n    Content.\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testAdmonitionWithIndent
func TestParse_AdmonitionWithIndent(t *testing.T) {
	parts := readMDDefault(t, "!!! note\n    Indented content.\n    More content.\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testAdmonitionWithNestedBulletList
func TestParse_AdmonitionWithNestedBulletList(t *testing.T) {
	parts := readMDDefault(t, "!!! note\n    - Item 1\n    - Item 2\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testAdmonitionsWithNewline
func TestParse_AdmonitionsWithNewline(t *testing.T) {
	parts := readMDDefault(t, "!!! note\n\n    Content after blank line.\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testDocusaurusAdmonition
func TestParse_DocusaurusAdmonition(t *testing.T) {
	parts := readMDDefault(t, ":::note\nDocusaurus admonition content.\n:::\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testDocusaurusAdmonitionWithTitle
func TestParse_DocusaurusAdmonitionWithTitle(t *testing.T) {
	parts := readMDDefault(t, ":::note[My Title]\nContent.\n:::\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testDocusaurusAdmonitionWithHtmlBlock
func TestParse_DocusaurusAdmonitionWithHtmlBlock(t *testing.T) {
	parts := readMDDefault(t, ":::note\n<div>HTML inside admonition</div>\n:::\n")
	require.NotEmpty(t, parts)
}

// okapi: MarkdownParserTest#testIndentedOrderedListAfterDocusaurusAdmonition
func TestParse_IndentedOrderedListAfterDocusaurusAdmonition(t *testing.T) {
	parts := readMDDefault(t, ":::note\nContent.\n:::\n\n1. Item 1\n2. Item 2\n")
	texts := translatableTexts(parts)
	require.NotEmpty(t, texts)
}
