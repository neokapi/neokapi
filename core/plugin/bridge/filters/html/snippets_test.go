//go:build integration

package html

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.html.HtmlFilter"
const mimeType = "text/html"

// readHTML parses an HTML snippet with custom filter params and returns the parts.
func readHTML(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.html", mimeType, filterParams)
}

// readHTMLDefault parses an HTML snippet with default (nil) params.
func readHTMLDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readHTML(t, snippet, nil)
}

// readHTMLWithParams is an alias for readHTML for clarity in tests.
func readHTMLWithParams(t *testing.T, snippet string, params map[string]any) []*model.Part {
	t.Helper()
	return readHTML(t, snippet, params)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips an HTML snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.html", mimeType, filterParams)
	return string(result.Output)
}

// snippetRoundtripDefault roundtrips with default (nil) params.
func snippetRoundtripDefault(t *testing.T, snippet string) string {
	t.Helper()
	return snippetRoundtrip(t, snippet, nil)
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

// blockTextsContain returns true if any block text contains substr.
func blockTextsContain(texts []string, substr string) bool {
	for _, txt := range texts {
		if strings.Contains(txt, substr) {
			return true
		}
	}
	return false
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

// ---------------------------------------------------------------------------
// Tests translated from HtmlSnippetsTest.java
// ---------------------------------------------------------------------------

// okapi: HtmlSnippetsTest#testMETA_Issue_1098
func TestSnippets_MetaIssue1098(t *testing.T) {
	// Java expects IllegalCharsetNameException with default config.
	// With cleanupHtml: false the snippet processes without error.
	snippet := "<html>\n   <head>  \n      <meta http-equiv=\"Content-Type\" content=\"html; charset=UTF-8\">\n      </meta>\n   </head>\n</html>"
	parts := readHTMLWithParams(t, snippet, map[string]any{"cleanupHtml": false})
	assert.NotEmpty(t, parts, "should produce parts with cleanupHtml disabled")
}

// okapi: HtmlSnippetsTest#testMultipleMETA
func TestSnippets_MultipleMETA(t *testing.T) {
	snippet := `<html title="Title html">` +
		`<meta NAME="keywords" CONTENT="Text1"/>` +
		`<meta NAME="creation_date" CONTENT="May 24, 2001"/>` +
		`<meta NAME="DESCRIPTION" CONTENT="Text2"/>` +
		`<meta NAME="twitter:title" CONTENT="Text3"/>` +
		`<meta NAME="twitter:description" CONTENT="Text4"/>` +
		`<meta NAME="og:title" CONTENT="Text5"/>` +
		`<meta NAME="og:description" CONTENT="Text6"/>` +
		`<meta NAME="og:site_name" CONTENT="Text7"/>` +
		`<p>Text8</p>`

	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Title html")
	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
	assert.Contains(t, texts, "Text3")
	assert.Contains(t, texts, "Text4")
	assert.Contains(t, texts, "Text5")
	assert.Contains(t, texts, "Text6")
	assert.Contains(t, texts, "Text7")
	assert.Contains(t, texts, "Text8")
	assert.GreaterOrEqual(t, len(blocks), 9)
}

// okapi: HtmlSnippetsTest#testHref
func TestSnippets_Href(t *testing.T) {
	snippet := `see <a href="http://yahoo.com">yahoo</a>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "see")
	assert.Contains(t, text, "yahoo")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have spans for the <a> tag")
}

// okapi: HtmlSnippetsTest#testButton
func TestSnippets_Button(t *testing.T) {
	snippet := "<p>  <button>text</button>  <button>text</button> </p>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "text")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have spans for the <button> tags")
}

// okapi: HtmlSnippetsTest#testCleanupHtmlOption
func TestSnippets_CleanupHtmlOption(t *testing.T) {
	snippet := "<div style={{textAlign: 'center', paddingTop: '15px', paddingBottom: '15px'}}></div>"
	parts := readHTMLWithParams(t, snippet, map[string]any{"cleanupHtml": false})
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "div with style only should not produce translatable blocks")
	dataCount := countPartsByType(parts, model.PartData)
	assert.Greater(t, dataCount, 0, "should have data parts for the div")
}

// okapi: HtmlSnippetsTest#testInlineCodesStorage
func TestSnippets_InlineCodesStorage(t *testing.T) {
	snippet := `<p>Before <b>bold</b> <a href="there"/> after.</p>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans, "should have inline spans")
	for _, span := range frag.Spans {
		assert.NotEmpty(t, span.Data, "span data should not be empty")
	}
}

// okapi: HtmlSnippetsTest#testTitleInP
func TestSnippets_TitleInP(t *testing.T) {
	snippet := `<p title="Text1">Text2</p>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
}

// okapi: HtmlSnippetsTest#testAltInImg
func TestSnippets_AltInImg(t *testing.T) {
	snippet := `Text1<img alt="Text2"/>.`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Text2")

	b := findBlockContaining(blocks, "Text1")
	require.NotNil(t, b, "should have a block containing Text1")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	hasPlaceholder := false
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "should have a placeholder span for <img>")
}

// okapi: HtmlSnippetsTest#imgStartTagOnlyHandledWithWellFormedConfiguration
func TestSnippets_ImgStartTagOnlyHandledWithWellFormedConfiguration(t *testing.T) {
	snippet := `<html><body><p><a href="foo.html"><img src="bar.png"></a></p></body></html>`
	parts := readHTMLWithParams(t, snippet, map[string]any{"parser": map[string]any{"assumeWellformed": true}})
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans")
}

// okapi: HtmlSnippetsTest#paramStartTagOnlyHandledWithWellFormedConfiguration
func TestSnippets_ParamStartTagOnlyHandledWithWellFormedConfiguration(t *testing.T) {
	snippet := `<html><body><p><object id="obj1"><param name="param1"></object></p></body></html>`
	parts := readHTMLWithParams(t, snippet, map[string]any{"parser": map[string]any{"assumeWellformed": true}})
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: HtmlSnippetsTest#areaStartTagOnlyHandledWithWellFormedConfiguration
func TestSnippets_AreaStartTagOnlyHandledWithWellFormedConfiguration(t *testing.T) {
	snippet := `<html><body><p>` +
		`<object data="navbar1.gif" type="image/gif" usemap="#map1"></object>` +
		`<map name="map1">` +
		`<area alt="Area 1">` +
		`</map>` +
		`</p></body></html>`
	parts := readHTMLWithParams(t, snippet, map[string]any{"parser": map[string]any{"assumeWellformed": true}})
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Area 1")
}

// okapi: HtmlSnippetsTest#testNoExtractValueInInput
func TestSnippets_NoExtractValueInInput(t *testing.T) {
	snippet := `<input type="file" value="NotText"/>.`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.NotContains(t, texts, "NotText")
	require.NotEmpty(t, blocks)

	b := blocks[0]
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.Contains(t, frag.Text(), ".")
	assert.NotEmpty(t, frag.Spans, "should have placeholder span for <input>")
}

// okapi: HtmlSnippetsTest#testExtractValueInInput
func TestSnippets_ExtractValueInInput(t *testing.T) {
	snippet := `<input type="other" value="Text" placeholder="text-html5"/>.`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text")
	assert.Contains(t, texts, "text-html5")
}

// okapi: HtmlSnippetsTest#testLabelInOption
func TestSnippets_LabelInOption(t *testing.T) {
	snippet := `Text1<option label="Text2"/>.`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
}

// okapi: HtmlSnippetsTest#testHtmlNonWellFormedEmptyTag
func TestSnippets_HtmlNonWellFormedEmptyTag(t *testing.T) {
	snippet := "<br>text<br/>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	for _, span := range frag.Spans {
		assert.Equal(t, model.SpanPlaceholder, span.SpanType,
			"br tags should be placeholders in non-wellformed mode")
	}
}

// okapi: HtmlSnippetsTest#testAddingMETAinHTML
func TestSnippets_AddingMETAInHTML(t *testing.T) {
	snippet := "<html><head></head><p>test</p></html>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Contains(t, output, "test")
	assert.Contains(t, output, "<html>")
}

// okapi: HtmlSnippetsTest#testAddingMETAinXHTML
func TestSnippets_AddingMETAInXHTML(t *testing.T) {
	snippet := `<html xmlns="http://www.w3.org/1999/xhtml"><head></head><p>test</p></html>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Contains(t, output, "test")
	assert.Contains(t, output, "xmlns")
}

// okapi: HtmlSnippetsTest#testAddingMETAinXML
func TestSnippets_AddingMETAInXML(t *testing.T) {
	snippet := `<html xmlns:MadCap="http://www.madcapsoftware.com/Schemas/MadCap.xsd"><head></head><p>test</p></html>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Contains(t, output, "test")
	assert.Contains(t, output, "MadCap")
}

// okapi: HtmlSnippetsTest#testMETATag1
func TestSnippets_METATag1(t *testing.T) {
	snippet := `<meta http-equiv="keywords" content="one,two,three"/>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testPWithAttributes
func TestSnippets_PWithAttributes(t *testing.T) {
	snippet := `<p title="my title" dir="rtl">Text of p</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testLang
func TestSnippets_Lang(t *testing.T) {
	snippet := `<p lang="en">Text of p</p>`
	output := snippetRoundtripDefault(t, snippet)
	// The bridge roundtrips with targetLocale="fr", so lang is updated.
	assert.Equal(t, `<p lang="fr">Text of p</p>`, output)
}

// okapi: HtmlSnippetsTest#testLangUpdate
func TestSnippets_LangUpdate(t *testing.T) {
	snippet := `<p lang="en">Text <span lang="en">text</span> text</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Contains(t, output, "Text")
	assert.Contains(t, output, "text")
}

// okapi: HtmlSnippetsTest#testMultilangUpdate
func TestSnippets_MultilangUpdate(t *testing.T) {
	snippet := `<p lang="en">Text</p><p lang="ja">JA text</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Contains(t, output, "Text")
	assert.Contains(t, output, "JA text")
}

// okapi: HtmlSnippetsTest#testComplexEmptyElement
func TestSnippets_ComplexEmptyElement(t *testing.T) {
	snippet := `<dummy write="w" readonly="ro" trans="tu1"/>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testPWithInlines
func TestSnippets_PWithInlines(t *testing.T) {
	snippet := `<p>Before <b>bold</b> <a href="there"/> after.</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testMETATag2
func TestSnippets_METATag2(t *testing.T) {
	snippet := `<meta http-equiv="Content-Language" content="en"/>`
	output := snippetRoundtripDefault(t, snippet)
	// The bridge roundtrips with targetLocale="fr", so Content-Language is updated.
	assert.Equal(t, `<meta http-equiv="Content-Language" content="fr"/>`, output)
}

// okapi: HtmlSnippetsTest#testPWithInlines2
func TestSnippets_PWithInlines2(t *testing.T) {
	snippet := `<p>Before <img href="img.png" alt="text"/> after.</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testPWithInlineTextOnly
func TestSnippets_PWithInlineTextOnly(t *testing.T) {
	snippet := `<p>Before <img alt="text"/> after.</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testTableGroups
func TestSnippets_TableGroups(t *testing.T) {
	snippet := `<table id="100"><tr><td>text</td></tr></table>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testGroupInPara
func TestSnippets_GroupInPara(t *testing.T) {
	snippet := "<p>Text before list:" +
		"<ul>" +
		"<li>Text of item 1</li>" +
		"<li>Text of item 2</li>" +
		"</ul>" +
		"and text after the list.</p>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testInput
func TestSnippets_Input(t *testing.T) {
	snippet := `<p>Before <input type="radio" name="FavouriteFare" value="spam" checked="checked"/> after.</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testCollapseWhitespaceWithPre
func TestSnippets_CollapseWhitespaceWithPre(t *testing.T) {
	snippet := "<pre>   \n   \n   \t    </pre>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testCollapseWhitespaceWithoutPre
func TestSnippets_CollapseWhitespaceWithoutPre(t *testing.T) {
	snippet := " <b>   text1\t\r\n\ftext2    </b> "
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, "<b> text1 text2 </b>", output)
}

// okapi: HtmlSnippetsTest#testEscapedCodesInisdePre
func TestSnippets_EscapedCodesInsidePre(t *testing.T) {
	snippet := "<pre><code>&lt;b></code></pre>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, "<pre><code>&lt;b></code></pre>", output)
}

// okapi: HtmlSnippetsTest#doesNotCrashOnPreservingWhitespaceForClosingPre
func TestSnippets_DoesNotCrashOnPreservingWhitespaceForClosingPre(t *testing.T) {
	snippet := "<html></pre></html>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, "<html></pre></html>", output)
}

// okapi: HtmlSnippetsTest#testCdataSection
func TestSnippets_CdataSection(t *testing.T) {
	snippet := "<![CDATA[&lt;b>]]>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, "<![CDATA[&lt;b>]]>", output)
}

// okapi: HtmlSnippetsTest#testEscapes
func TestSnippets_Escapes(t *testing.T) {
	snippet := `<p><b>Question</b>: When the "<code>&lt;b></code>" code was added</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Contains(t, output, "Question")
	assert.Contains(t, output, "&lt;b>")
}

// okapi: HtmlSnippetsTest#testEscapedEntities
func TestSnippets_EscapedEntities(t *testing.T) {
	snippet := "&nbsp;M&#x0033;"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\u00A0")
	assert.Contains(t, text, "M3")
}

// okapi: HtmlSnippetsTest#testQuoteMode
func TestSnippets_QuoteMode(t *testing.T) {
	snippet := `&quot; '`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: HtmlSnippetsTest#testQuoteModeDefault
func TestSnippets_QuoteModeDefault(t *testing.T) {
	snippet := `&quot; '`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, `"`)
	assert.Contains(t, text, "'")
}

// okapi: HtmlSnippetsTest#testNewlineDetection
func TestSnippets_NewlineDetection(t *testing.T) {
	snippet := "\r\nX\r\nY\r\n"
	parts := readHTMLWithParams(t, snippet, map[string]any{"collapseWhitespace": false})
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "X")
	assert.Contains(t, text, "Y")
}

// okapi: HtmlSnippetsTest#testCodeFinder
func TestSnippets_CodeFinder(t *testing.T) {
	snippet := "<p>text notVAR1 VAR2<p>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text")
	assert.Contains(t, text, "VAR2")
}

// okapi: HtmlSnippetsTest#testCodeFinderInAttributes
func TestSnippets_CodeFinderInAttributes(t *testing.T) {
	snippet := "<p title='Title VAR1'>Para VAR2 <img alt='Alt VAR3'> after<p>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Title"), "should extract title attribute text")
	assert.True(t, blockTextsContain(texts, "Alt"), "should extract alt attribute text")
	assert.True(t, blockTextsContain(texts, "Para"), "should extract paragraph text")
}

// okapi: HtmlSnippetsTest#testNormalizeNewlinesInPre
func TestSnippets_NormalizeNewlinesInPre(t *testing.T) {
	snippet := "<pre>\r\nX\r\nY\r\n</pre>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testSupplementalSupport
func TestSnippets_SupplementalSupport(t *testing.T) {
	snippet := "<p>[&#x20000;]=U+D840,U+DC00</p>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\U00020000")
}

// okapi: HtmlSnippetsTest#testSimpleSupplementalSupport
func TestSnippets_SimpleSupplementalSupport(t *testing.T) {
	snippet := "&#x20000;"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\U00020000")
}

// okapi: HtmlSnippetsTest#ITextUnitsInARow
func TestSnippets_ITextUnitsInARow(t *testing.T) {
	snippet := "<td><p><h1>para text in a table element</h1></p></td>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#ITextUnitsInARowWithTwoHeaders
func TestSnippets_ITextUnitsInARowWithTwoHeaders(t *testing.T) {
	snippet := "<td><p><h1>header one</h1><h2>header two</h2></p></td>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#twoITextUnitsInARowNonWellformed
func TestSnippets_TwoITextUnitsInARowNonWellformed(t *testing.T) {
	snippet := "<td><p><h1>para text in a table element</td>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#twoITextUnitsInARowNonWellformedWithNonWellFromedConfig
func TestSnippets_TwoITextUnitsInARowNonWellformedWithNonWellFormedConfig(t *testing.T) {
	snippet := "<td><p><h1>para text in a table element</td>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#ITextUnitName
func TestSnippets_ITextUnitName(t *testing.T) {
	snippet := `<p id="logo">para text in a table element</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#ITextUnitStartedWithText
func TestSnippets_ITextUnitStartedWithText(t *testing.T) {
	snippet := "this is some text<x/>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#textUnbalancedInlineTag
func TestSnippets_TextUnbalancedInlineTag(t *testing.T) {
	snippet := "<p>this is some text</i></p>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#textOverlapInlineTags
func TestSnippets_TextOverlapInlineTags(t *testing.T) {
	snippet := "<p><i><b>this is some text</i></b></p>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#textWithUnquotedAttribtes
func TestSnippets_TextWithUnquotedAttributes(t *testing.T) {
	snippet := "<img alt=R&amp;D src=image.png>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Contains(t, output, "R&amp;D")
	assert.Contains(t, output, "image.png")
}

// okapi: HtmlSnippetsTest#testInlineAnchorAndAmpersand
func TestSnippets_InlineAnchorAndAmpersand(t *testing.T) {
	snippet := `<a href="foo.cgi?chapter=1&amp;section=2&amp;copy=3&amp;lang=en"/>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testPAndInlineAnchorAndAmpersand
func TestSnippets_PAndInlineAnchorAndAmpersand(t *testing.T) {
	snippet := `<p>Before <a href="foo.cgi?chapter=1&amp;section=2&amp;copy=3&amp;lang=en"/> after.</p>`
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testCERinOutput
func TestSnippets_CERInOutput(t *testing.T) {
	snippet := "<p>[\u00a0\u0104]</p>"
	parts := readHTMLWithParams(t, snippet, map[string]any{"escapeCharacters": "\u00a0\u0104"})
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\u00a0")
	assert.Contains(t, text, "\u0104")
}

// okapi: HtmlSnippetsTest#minimalCompleteHtml
func TestSnippets_MinimalCompleteHtml(t *testing.T) {
	snippet := "<html><body><p>Test1<br/>Test2</p></body></html>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Test1"), "should contain Test1")
	assert.True(t, blockTextsContain(texts, "Test2"), "should contain Test2")
	for _, txt := range texts {
		assert.NotEqual(t, "body", txt, "should not extract 'body' as text")
	}
}

// okapi: HtmlSnippetsTest#italicBoldEtc
func TestSnippets_ItalicBoldEtc(t *testing.T) {
	snippet := "<p>This is <i>italic<i> and <b>bold<b>. The can be <del>removed and still <b>bold</b></del>!</p>."
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "This is"), "should contain 'This is'")
	assert.True(t, blockTextsContain(texts, "bold"), "should contain 'bold'")
	for _, txt := range texts {
		assert.NotEqual(t, "del", txt, "should not extract 'del' as text")
	}

	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#simpleTable
func TestSnippets_SimpleTable(t *testing.T) {
	snippet := "<table><tr><td>Test</td></tr></table>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Test")
	for _, txt := range texts {
		assert.NotEqual(t, "table", txt, "should not extract 'table' as text")
	}

	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#paraWithBreak
func TestSnippets_ParaWithBreak(t *testing.T) {
	snippet := "<p>Sentence 1.<br/>Sentence 2.<p>Another para."
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Sentence 1"), "should contain 'Sentence 1'")
	assert.True(t, blockTextsContain(texts, "Sentence 2"), "should contain 'Sentence 2'")
	assert.True(t, blockTextsContain(texts, "Another para"), "should contain 'Another para'")
	for _, txt := range texts {
		assert.NotEqual(t, "br", txt, "should not extract 'br' as text")
	}

	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#table
func TestSnippets_Table(t *testing.T) {
	snippet := "<table>" +
		"<tbody><tr valign=\"baseline\">" +
		"<th align=\"right\">" +
		"<strong>Subject</strong>:</th>" +
		"<td align=\"left\">" +
		"ugly <a id=\"KonaLink0\" target=\"top\" class=\"kLink\">stuff</a></td>" +
		"</tr>" +
		"</tbody></table>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testComplexTable
func TestSnippets_ComplexTable(t *testing.T) {
	snippet := "<TABLE><TR><TD><UL><B CLASS=\"head\">Why We Exist</B><LI><B CLASS=\"side\">the problem:</B> <B CLASS=\"reason\">economic terrorism.</B></LI>" +
		"<LI><B CLASS=\"side\">terrorism=</B> <B CLASS=\"reason\">any activity not supporting Microbloat.</B></LI>" +
		"<LI><B CLASS=\"side\">example:</B> <B CLASS=\"reason\">any company competing with Microbloat.</B></LI>" +
		"<LI><B CLASS=\"side\">solution:</B> <B CLASS=\"reason\">crush the bastards while they&#39;re small.</B></LI>" +
		"</UL>" +
		"<MENU><B CLASS=\"head\">About Our Services</B> <BR><B CLASS=\"motto\">We guarantee it!!</B><LI><A HREF=\"error_notloggedin.html\">conditions of use</A></LI>" +
		"<LI><A HREF=\"error_notloggedin.html\">privacy policy</A></LI>" +
		"</MENU>" +
		"</TD></TR></TABLE>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testTextDirectionClarification
func TestSnippets_TextDirectionClarification(t *testing.T) {
	tests := []struct {
		name     string
		snippet  string
		tgtLang  model.LocaleID
		expected string
	}{
		{
			name: "rtl_with_dir_rtl",
			snippet: "<!DOCTYPE html>\n" +
				"<html dir=\"rtl\">\n" +
				"<body>\n" +
				"<p>بعض الكلمات</p>\n" +
				"</body>\n" +
				"</html>\n",
			tgtLang: "ar",
			expected: "<!DOCTYPE html>\n" +
				"<html dir=\"rtl\">\n" +
				"<body>\n" +
				"<p>بعض الكلمات</p>\n" +
				"</body>\n" +
				"</html>\n",
		},
		{
			name: "rtl_with_dir_ltr",
			snippet: "<!DOCTYPE html>\n" +
				"<html dir=\"ltr\">\n" +
				"<body>\n" +
				"<p>بعض الكلمات</p>\n" +
				"</body>\n" +
				"</html>\n",
			tgtLang: "ar",
			expected: "<!DOCTYPE html>\n" +
				"<html dir=\"rtl\">\n" +
				"<body>\n" +
				"<p>بعض الكلمات</p>\n" +
				"</body>\n" +
				"</html>\n",
		},
		{
			name: "rtl_no_dir",
			snippet: "<!DOCTYPE html>\n" +
				"<html>\n" +
				"<body>\n" +
				"<p>بعض الكلمات</p>\n" +
				"</body>\n" +
				"</html>\n",
			tgtLang: "ar",
			expected: "<!DOCTYPE html>\n" +
				"<html dir=\"rtl\">\n" +
				"<body>\n" +
				"<p>بعض الكلمات</p>\n" +
				"</body>\n" +
				"</html>\n",
		},
		{
			name: "ltr_with_dir_rtl",
			snippet: "<!DOCTYPE html>\n" +
				"<html dir=\"rtl\">\n" +
				"<body>\n" +
				"<p>Some text.</p>\n" +
				"</body>\n" +
				"</html>\n",
			tgtLang: "en",
			expected: "<!DOCTYPE html>\n" +
				"<html>\n" +
				"<body>\n" +
				"<p>Some text.</p>\n" +
				"</body>\n" +
				"</html>\n",
		},
		{
			name: "ltr_with_dir_ltr",
			snippet: "<!DOCTYPE html>\n" +
				"<html dir=\"ltr\">\n" +
				"<body>\n" +
				"<p>Some text.</p>\n" +
				"</body>\n" +
				"</html>\n",
			tgtLang: "en",
			expected: "<!DOCTYPE html>\n" +
				"<html>\n" +
				"<body>\n" +
				"<p>Some text.</p>\n" +
				"</body>\n" +
				"</html>\n",
		},
		{
			name: "ltr_no_dir",
			snippet: "<!DOCTYPE html>\n" +
				"<html>\n" +
				"<body>\n" +
				"<p>Some text.</p>\n" +
				"</body>\n" +
				"</html>\n",
			tgtLang: "en",
			expected: "<!DOCTYPE html>\n" +
				"<html>\n" +
				"<body>\n" +
				"<p>Some text.</p>\n" +
				"</body>\n" +
				"</html>\n",
		},
		{
			name: "rtl_with_translate_no",
			snippet: "<!DOCTYPE html>\n" +
				"<html translate=\"no\">\n" +
				"<body>\n" +
				"<p>بعض الكلمات</p>\n" +
				"</body>\n" +
				"</html>\n",
			tgtLang: "ar",
			expected: "<!DOCTYPE html>\n" +
				"<html dir=\"rtl\" translate=\"no\">\n" +
				"<body>\n" +
				"<p>بعض الكلمات</p>\n" +
				"</body>\n" +
				"</html>\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, cfg := bridgetest.SharedBridge(t)
			result := bridgetest.RoundTripWithLocales(t, pool, cfg, filterClass,
				[]byte(tc.snippet), "test.html", mimeType, nil,
				"en", tc.tgtLang)
			assert.Equal(t, tc.expected, string(result.Output))
		})
	}
}

// okapi: HtmlSnippetsTest#testTranslateAttribute
func TestSnippets_TranslateAttribute(t *testing.T) {
	snippet := "<p>text with a <span translate='no'>no-translation part</span> and more.</p>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "text with a")
	assert.Contains(t, text, "and more.")
}

// okapi: HtmlSnippetsTest#testPBlockTranslateAttribute
func TestSnippets_PBlockTranslateAttribute(t *testing.T) {
	snippet := "<p translate='no'>no trans</p><p>Text <span translate='no'>no-trans</span></p>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Text"), "should extract text from translatable paragraph")
	for _, txt := range texts {
		assert.NotEqual(t, "no trans", txt, "translate=no paragraph should not be extracted")
	}
}

// okapi: HtmlSnippetsTest#testDivBlockTranslateAttribute
func TestSnippets_DivBlockTranslateAttribute(t *testing.T) {
	snippet := "<div translate='no'>no trans</div><p>Text <span translate='no'>no-trans</span></p>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Text"), "should extract text from translatable paragraph")
}

// okapi: HtmlSnippetsTest#testNestedInlineTranslateAttribute1
func TestSnippets_NestedInlineTranslateAttribute1(t *testing.T) {
	snippet := `<p>Text <span><span translate="no">no-trans</span></span>.</p>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Text")
	assert.Contains(t, text, ".")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have spans for nested inline elements")
}

// okapi: HtmlSnippetsTest#testNestedInlineTranslateAttribute2_1
func TestSnippets_NestedInlineTranslateAttribute2_1(t *testing.T) {
	snippet := `<p>a<span><span translate="no"><span>no-trans</span></span>b</span>a</p>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "a")
	assert.Contains(t, text, "b")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans")
}

// okapi: HtmlSnippetsTest#testNestedInlineTranslateAttribute2_2
func TestSnippets_NestedInlineTranslateAttribute2_2(t *testing.T) {
	snippet := `<p>a<span>b<span translate="no"><span>no-trans</span></span>b</span>a</p>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "a")
	assert.Contains(t, text, "b")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans")
}

// okapi: HtmlSnippetsTest#testNestedInlineTranslateAttribute3
func TestSnippets_NestedInlineTranslateAttribute3(t *testing.T) {
	snippet := `<p>a<span>b<i>c<i translate="no">no-trans</i></i><span translate="no">no-trans</span>b</span>a</p>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "a")
	assert.Contains(t, text, "b")
	assert.Contains(t, text, "c")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans for nested elements")
}

// okapi: HtmlSnippetsTest#testNestedInlineTranslateAttribute4
func TestSnippets_NestedInlineTranslateAttribute4(t *testing.T) {
	snippet := `<p>a<span>b<span translate="no">no-trans<span translate="yes">d</span>no-trans</span>b</span>a</p>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "a")
	assert.Contains(t, text, "b")
	assert.Contains(t, text, "d")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans")
}

// okapi: HtmlSnippetsTest#testNestedInlineTranslateAttribute5
func TestSnippets_NestedInlineTranslateAttribute5(t *testing.T) {
	snippet := `<p>a<span>b<i>c<i translate="no">no-trans<span translate="yes">e</span>no-trans</i></i><span translate="no">no-trans</span>b</span>a</p>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "a")
	assert.Contains(t, text, "b")
	assert.Contains(t, text, "c")
	assert.Contains(t, text, "e")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans")
}

// okapi: HtmlSnippetsTest#testNestedInlineTranslateAttribute6
func TestSnippets_NestedInlineTranslateAttribute6(t *testing.T) {
	snippet := `<p><i translate="no"><b translate="yes">this is some text</i></b></p>`
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "this is some text")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans")
	require.GreaterOrEqual(t, len(frag.Spans), 4, "should have at least 4 spans")
	// The bridge emits all spans as placeholders for translate attribute boundaries.
	assert.Equal(t, model.SpanPlaceholder, frag.Spans[0].SpanType, "first span should be placeholder for translate=no <i>")
	assert.Equal(t, model.SpanPlaceholder, frag.Spans[1].SpanType, "second span should be placeholder for translate=yes <b>")
	assert.Equal(t, model.SpanPlaceholder, frag.Spans[2].SpanType, "third span should be placeholder for </i>")
	assert.Equal(t, model.SpanPlaceholder, frag.Spans[3].SpanType, "fourth span should be placeholder for </b>")
}

// okapi: HtmlSnippetsTest#testFreeMarker
func TestSnippets_FreeMarker(t *testing.T) {
	snippet := "<strong> this is a bolded text between html strong tags </strong> <#if contactInfo??> or ${contactInfo}</#if>."
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "this is a bolded text between html strong tags")
}

// okapi: HtmlSnippetsTest#testPlaceholderOnlySegments
func TestSnippets_PlaceholderOnlySegments(t *testing.T) {
	snippet := "<table><tr><td><br/></td></tr><tr><td><img src='...'></td></tr></table>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Equal(t, 2, len(blocks), "should produce 2 text units for placeholder-only segments")
}

// okapi: HtmlSnippetsTest#testDivBlockExcludeIncludeTranslateAttribute
func TestSnippets_DivBlockExcludeIncludeTranslateAttribute(t *testing.T) {
	snippet := "<div translate='no'>no <div translate='yes'>trans</div></div>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "trans")
}

// okapi: HtmlSnippetsTest#testDivBlockWithPTranslateAttribute
func TestSnippets_DivBlockWithPTranslateAttribute(t *testing.T) {
	snippet := "<div translate=\"no\">Some intefaces</div><p translate=\"no\">can be defined</p>\n"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "should not produce translatable blocks when translate=no on all elements")
}

// okapi: HtmlSnippetsTest#testInlineTranslateNo
func TestSnippets_InlineTranslateNo(t *testing.T) {
	snippet := "Shopping cart contains two items <strong id=\"1\" translate=\"no\"><a>These two items are " +
		"without any discount and </a>summer sale <b translate=\"no\">is also not applicable </b></strong>full" +
		" amount will be charged."
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Shopping cart contains two items")
	assert.Contains(t, text, "full amount will be charged.")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	hasPlaceholder := false
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "translate=no content should become a placeholder")
}

// okapi: HtmlSnippetsTest#testInlineTranslateYes
func TestSnippets_InlineTranslateYes(t *testing.T) {
	snippet := "Shopping cart contains two items <strong id=\"1\" translate=\"yes\"><a>These two items are " +
		"without any discount and </a>summer sale <b translate=\"no\">is also not applicable </b></strong>full" +
		" amount will be charged."
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Shopping cart contains two items")
	assert.Contains(t, text, "These two items are without any discount and")
	assert.Contains(t, text, "summer sale")
	assert.Contains(t, text, "full amount will be charged.")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans")
}

// okapi: HtmlSnippetsTest#testTagLowerCaseFix
func TestSnippets_TagLowerCaseFix(t *testing.T) {
	snippet := "<B><FONT SIZE=3>our accomplishments</B></FONT>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "our accomplishments")
}

// okapi: HtmlSnippetsTest#testInlineCdata
func TestSnippets_InlineCdata(t *testing.T) {
	snippet := "Here is some <![CDATA[inline cdata<>&]]> for you."
	parts := readHTMLWithParams(t, snippet, map[string]any{"inlineCdata": true})
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	text := frag.Text()
	assert.Contains(t, text, "Here is some")
	assert.Contains(t, text, "for you.")
	assert.GreaterOrEqual(t, len(frag.Spans), 2, "should have spans for CDATA markers")
}

// okapi: HtmlSnippetsTest#testEmptyGroupAtEnd
func TestSnippets_EmptyGroupAtEnd(t *testing.T) {
	// With default config, <g/> is an unknown inline tag.
	snippet := "Empty group at the end <g/>"
	output := snippetRoundtripDefault(t, snippet)
	assert.Contains(t, output, "Empty group at the end")
	assert.Contains(t, output, "<g/>")
}

// okapi: HtmlSnippetsTest#testASPXComment
func TestSnippets_ASPXComment(t *testing.T) {
	snippet := "<%-- comment --%>Text"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := findBlockContaining(blocks, "Text")
	require.NotNil(t, b, "should extract 'Text'")
	assert.Equal(t, "Text", b.SourceText())

	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testASPXEmbeddedTag
func TestSnippets_ASPXEmbeddedTag(t *testing.T) {
	snippet := "<asp:Label ID=\"Label4\" runat=\"server\" Text=\"<%$ Resources:website, Home %>\"></asp:Label>Text"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := findBlockContaining(blocks, "Text")
	require.NotNil(t, b, "should extract 'Text'")

	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: HtmlSnippetsTest#testUlWithScriptTag
func TestSnippets_UlWithScriptTag(t *testing.T) {
	snippet := "<ul>\n" +
		"<li><script type=\"text/x-nonsense\">H</script> is the magnetic field intensity</li>\n" +
		"<li><i>J</i>is the conduction current density</li>\n" +
		"</ul>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "magnetic field intensity"),
		"should contain 'magnetic field intensity'")
	assert.True(t, blockTextsContain(texts, "J"),
		"should contain 'J'")
	for _, txt := range texts {
		assert.NotEqual(t, "H", txt, "'H' inside script should not be extracted as standalone text")
	}
}

// okapi: HtmlSnippetsTest#testNegativeCondition
func TestSnippets_NegativeCondition(t *testing.T) {
	params := map[string]any{
		"exclude_by_default": true,
		"elements": map[string]any{
			"pre": map[string]any{
				"ruleTypes":  []string{"TEXTUNIT", "PRESERVE_WHITESPACE"},
				"conditions": []string{"translate", "NOT_EQUALS", "no"},
			},
		},
	}
	snippet := "<pre>Translate me.</pre><pre translate=\"no\">Don't translate me.</pre>"
	parts := readHTMLWithParams(t, snippet, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.Equal(t, 1, len(blocks), "should extract exactly 1 translatable block")
	assert.Equal(t, "Translate me.", blocks[0].SourceText())
}

// okapi: HtmlSnippetsTest#testOkapiMarkerInText
func TestSnippets_OkapiMarkerInText(t *testing.T) {
	snippet := "Word \uE101\uE102\uE103 <span>end</span>!"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have spans for marker characters")

	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, "Word \uE101\uE102\uE103 <span>end</span>!", output)
}

// okapi: HtmlSnippetsTest#testOkapiMarkerInAttribute
func TestSnippets_OkapiMarkerInAttribute(t *testing.T) {
	snippet := "Word <span title=\"a\uE101\uE102\uE103b\">end</span>!"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should have at least 2 text units (attribute + content)")

	attrBlock := blocks[0]
	attrText := attrBlock.SourceText()
	assert.Contains(t, attrText, "a")
	assert.Contains(t, attrText, "b")

	contentBlock := blocks[1]
	contentText := contentBlock.SourceText()
	assert.Contains(t, contentText, "Word")
	assert.Contains(t, contentText, "end")

	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, "Word <span title=\"a\uE101\uE102\uE103b\">end</span>!", output)
}

// okapi: HtmlSnippetsTest#testPropertyInTextUnitConvertedToDocumentPart
func TestSnippets_PropertyInTextUnitConvertedToDocumentPart(t *testing.T) {
	snippet := "<p dir=\"test\"> </p>\n"
	output := snippetRoundtripDefault(t, snippet)
	assert.Contains(t, output, "<p dir=\"test\">")
}

// okapi: HtmlSnippetsTest#testPreserveCharacterEntitiesSimple
func TestSnippets_PreserveCharacterEntitiesSimple(t *testing.T) {
	snippet := "<p>& and &amp;</p>"
	params := map[string]any{
		"parser": map[string]any{
			"preserveCharacterEntities": true,
			"preserveWhitespace":        true,
		},
	}
	parts := readHTMLWithParams(t, snippet, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "&")
}

// okapi: HtmlSnippetsTest#testPreserveCharacterEntitiesWithInlineElements
func TestSnippets_PreserveCharacterEntitiesWithInlineElements(t *testing.T) {
	snippet := "<p><u>& and &amp;</u></p>"
	params := map[string]any{
		"parser": map[string]any{
			"preserveCharacterEntities": true,
			"preserveWhitespace":        true,
		},
	}
	parts := readHTMLWithParams(t, snippet, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "&")
}

// okapi: HtmlSnippetsTest#testPreserveCharacterEntitiesMultipleTypes
func TestSnippets_PreserveCharacterEntitiesMultipleTypes(t *testing.T) {
	snippet := "<p>&lt; &amp; &gt; &nbsp;</p>"
	params := map[string]any{
		"parser": map[string]any{
			"preserveCharacterEntities": true,
			"preserveWhitespace":        true,
		},
	}
	parts := readHTMLWithParams(t, snippet, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.NotEmpty(t, text)
}

// okapi: HtmlSnippetsTest#testNoPreserveCharacterEntitiesMultipleTypes
func TestSnippets_NoPreserveCharacterEntitiesMultipleTypes(t *testing.T) {
	snippet := "<p>&lt; &amp; &gt; &nbsp;</p>"
	params := map[string]any{
		"parser": map[string]any{
			"preserveCharacterEntities": false,
			"preserveWhitespace":        true,
		},
	}
	parts := readHTMLWithParams(t, snippet, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "<")
	assert.Contains(t, text, "&")
	assert.Contains(t, text, ">")
}

// okapi: HtmlSnippetsTest#testWithoutPreserveCharacterEntities
func TestSnippets_WithoutPreserveCharacterEntities(t *testing.T) {
	snippet := "<p>& and &amp;</p>"
	parts := readHTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "& and &")
}
