package html_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	htmlfmt "github.com/gokapi/gokapi/core/formats/html"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func readHTML(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	ctx := context.Background()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func readHTMLWithConfig(t *testing.T, snippet string, params map[string]any) []*model.Part {
	t.Helper()
	ctx := context.Background()
	reader := htmlfmt.NewReader()
	if params != nil {
		err := reader.Config().ApplyMap(params)
		require.NoError(t, err)
	}
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func translatableBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

func allBlocks(parts []*model.Part) []*model.Block {
	return testutil.FilterBlocks(parts)
}

func blockTexts(blocks []*model.Block) []string {
	return testutil.BlockTexts(blocks)
}

func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

func blockTextsContain(texts []string, substr string) bool {
	for _, txt := range texts {
		if strings.Contains(txt, substr) {
			return true
		}
	}
	return false
}

func dataParts(parts []*model.Part) []*model.Part {
	var result []*model.Part
	for _, p := range parts {
		if p.Type == model.PartData {
			result = append(result, p)
		}
	}
	return result
}

func findDataPartWithProperty(parts []*model.Part, key string) *model.Data {
	for _, p := range parts {
		if p.Type == model.PartData {
			d, ok := p.Resource.(*model.Data)
			if ok && d.Properties != nil {
				if _, exists := d.Properties[key]; exists {
					return d
				}
			}
		}
	}
	return nil
}

func roundtrip(t *testing.T, snippet string) string {
	t.Helper()
	ctx := context.Background()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	return buf.String()
}

// --- Snippet Tests (from HtmlSnippetsTest.java) ---

// okapi: HtmlSnippetsTest#minimalCompleteHtml
func TestSnippets_MinimalCompleteHtml(t *testing.T) {
	parts := readHTML(t, "<html><body><p>Test1<br/>Test2</p></body></html>")
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Test1"), "should contain Test1")
	assert.True(t, blockTextsContain(texts, "Test2"), "should contain Test2")
	for _, txt := range texts {
		assert.NotEqual(t, "body", txt, "should not extract 'body' as text")
	}
}

// okapi: HtmlSnippetsTest#testMultipleMETA
func TestSnippets_MultipleMETA(t *testing.T) {
	snippet := `<html title="Title html">` +
		`<meta name="keywords" content="Text1"/>` +
		`<meta name="creation_date" content="May 24, 2001"/>` +
		`<meta name="description" content="Text2"/>` +
		`<meta name="twitter:title" content="Text3"/>` +
		`<meta name="twitter:description" content="Text4"/>` +
		`<meta name="og:title" content="Text5"/>` +
		`<meta name="og:description" content="Text6"/>` +
		`<meta name="og:site_name" content="Text7"/>` +
		`<p>Text8</p>`

	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

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
	parts := readHTML(t, `see <a href="http://yahoo.com">yahoo</a>`)
	blocks := translatableBlocks(parts)
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
	parts := readHTML(t, "<p>  <button>text</button>  <button>text</button> </p>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "text")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have spans for the <button> tags")
}

// okapi: HtmlSnippetsTest#testInlineCodesStorage
func TestSnippets_InlineCodesStorage(t *testing.T) {
	parts := readHTML(t, `<p>Before <b>bold</b> <a href="there"/> after.</p>`)
	blocks := translatableBlocks(parts)
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
	parts := readHTML(t, `<p title="Text1">Text2</p>`)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
}

// okapi: HtmlSnippetsTest#testAltInImg
func TestSnippets_AltInImg(t *testing.T) {
	parts := readHTML(t, `Text1<img alt="Text2"/>.`)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

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

// okapi: HtmlSnippetsTest#testNoExtractValueInInput
func TestSnippets_NoExtractValueInInput(t *testing.T) {
	parts := readHTML(t, `<input type="file" value="NotText"/>.`)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.NotContains(t, texts, "NotText")
}

// okapi: HtmlSnippetsTest#testExtractValueInInput
func TestSnippets_ExtractValueInInput(t *testing.T) {
	parts := readHTML(t, `<input type="other" value="Text" placeholder="text-html5"/>.`)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text")
	assert.Contains(t, texts, "text-html5")
}

// okapi: HtmlSnippetsTest#testLabelInOption
func TestSnippets_LabelInOption(t *testing.T) {
	parts := readHTML(t, `Text1<option label="Text2"/>.`)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Text1"))
	assert.Contains(t, texts, "Text2")
}

// okapi: HtmlSnippetsTest#testHtmlNonWellFormedEmptyTag
func TestSnippets_HtmlNonWellFormedEmptyTag(t *testing.T) {
	parts := readHTML(t, "<br>text<br/>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	for _, span := range frag.Spans {
		assert.Equal(t, model.SpanPlaceholder, span.SpanType,
			"br tags should be placeholders")
	}
}

// okapi: HtmlSnippetsTest#testEscapedEntities
func TestSnippets_EscapedEntities(t *testing.T) {
	parts := readHTML(t, "&nbsp;M&#x0033;")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\u00A0")
	assert.Contains(t, text, "M3")
}

// okapi: HtmlSnippetsTest#testQuoteModeDefault
func TestSnippets_QuoteModeDefault(t *testing.T) {
	parts := readHTML(t, `&quot; '`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, `"`)
	assert.Contains(t, text, "'")
}

// okapi: HtmlSnippetsTest#testSupplementalSupport
func TestSnippets_SupplementalSupport(t *testing.T) {
	parts := readHTML(t, "<p>[&#x20000;]=U+D840,U+DC00</p>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\U00020000")
}

// okapi: HtmlSnippetsTest#testSimpleSupplementalSupport
func TestSnippets_SimpleSupplementalSupport(t *testing.T) {
	parts := readHTML(t, "&#x20000;")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\U00020000")
}

// okapi: HtmlSnippetsTest#ITextUnitsInARowWithTwoHeaders
func TestSnippets_ITextUnitsInARowWithTwoHeaders(t *testing.T) {
	parts := readHTML(t, "<td><p><h1>header one</h1><h2>header two</h2></p></td>")
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "header one")
	assert.Contains(t, texts, "header two")
}

// okapi: HtmlSnippetsTest#ITextUnitName
func TestSnippets_ITextUnitName(t *testing.T) {
	parts := readHTML(t, `<p id="logo">para text in a table element</p>`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].Name, "logo")
}

// okapi: HtmlSnippetsTest#italicBoldEtc
func TestSnippets_ItalicBoldEtc(t *testing.T) {
	snippet := "<p>This is <i>italic<i> and <b>bold<b>. The can be <del>removed and still <b>bold</b></del>!</p>."
	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "This is"), "should contain 'This is'")
	assert.True(t, blockTextsContain(texts, "bold"), "should contain 'bold'")
}

// okapi: HtmlSnippetsTest#simpleTable
func TestSnippets_SimpleTable(t *testing.T) {
	parts := readHTML(t, "<table><tr><td>Test</td></tr></table>")
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "Test")
	for _, txt := range texts {
		assert.NotEqual(t, "table", txt, "should not extract 'table' as text")
	}
}

// okapi: HtmlSnippetsTest#paraWithBreak
func TestSnippets_ParaWithBreak(t *testing.T) {
	parts := readHTML(t, "<p>Sentence 1.<br/>Sentence 2.<p>Another para.")
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Sentence 1"), "should contain 'Sentence 1'")
	assert.True(t, blockTextsContain(texts, "Sentence 2"), "should contain 'Sentence 2'")
	assert.True(t, blockTextsContain(texts, "Another para"), "should contain 'Another para'")
}

// okapi: HtmlSnippetsTest#testTranslateAttribute
func TestSnippets_TranslateAttribute(t *testing.T) {
	parts := readHTML(t, "<p>text with a <span translate='no'>no-translation part</span> and more.</p>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "text with a")
	assert.Contains(t, text, "and more.")
}

// okapi: HtmlSnippetsTest#testPBlockTranslateAttribute
func TestSnippets_PBlockTranslateAttribute(t *testing.T) {
	parts := readHTML(t, "<p translate='no'>no trans</p><p>Text <span translate='no'>no-trans</span></p>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Text"), "should extract text from translatable paragraph")
	for _, txt := range texts {
		assert.NotEqual(t, "no trans", txt, "translate=no paragraph should not be extracted")
	}
}

// okapi: HtmlSnippetsTest#testDivBlockTranslateAttribute
func TestSnippets_DivBlockTranslateAttribute(t *testing.T) {
	parts := readHTML(t, "<div translate='no'>no trans</div><p>Text <span translate='no'>no-trans</span></p>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Text"), "should extract text from translatable paragraph")
}

// okapi: HtmlSnippetsTest#testDivBlockWithPTranslateAttribute
func TestSnippets_DivBlockWithPTranslateAttribute(t *testing.T) {
	parts := readHTML(t, "<div translate=\"no\">Some intefaces</div><p translate=\"no\">can be defined</p>\n")
	blocks := translatableBlocks(parts)
	assert.Empty(t, blocks, "should not produce translatable blocks when translate=no on all elements")
}

// okapi: HtmlSnippetsTest#testDivBlockExcludeIncludeTranslateAttribute
func TestSnippets_DivBlockExcludeIncludeTranslateAttribute(t *testing.T) {
	parts := readHTML(t, "<div translate='no'>no <div translate='yes'>trans</div></div>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "trans")
}

// okapi: HtmlSnippetsTest#testNestedInlineTranslateAttribute1
func TestSnippets_NestedInlineTranslateAttribute1(t *testing.T) {
	parts := readHTML(t, `<p>Text <span><span translate="no">no-trans</span></span>.</p>`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Text")
	assert.Contains(t, text, ".")
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have spans for nested inline elements")
}

// okapi: HtmlSnippetsTest#testNestedInlineTranslateAttribute4
func TestSnippets_NestedInlineTranslateAttribute4(t *testing.T) {
	parts := readHTML(t, `<p>a<span>b<span translate="no">no-trans<span translate="yes">d</span>no-trans</span>b</span>a</p>`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "a")
	assert.Contains(t, text, "b")
	assert.Contains(t, text, "d")
}

// okapi: HtmlSnippetsTest#testInlineTranslateNo
func TestSnippets_InlineTranslateNo(t *testing.T) {
	snippet := "Shopping cart contains two items <strong id=\"1\" translate=\"no\"><a>These two items are " +
		"without any discount and </a>summer sale <b translate=\"no\">is also not applicable </b></strong>full" +
		" amount will be charged."
	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
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
	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Shopping cart contains two items")
	assert.Contains(t, text, "These two items are without any discount and")
	assert.Contains(t, text, "summer sale")
	assert.Contains(t, text, "full amount will be charged.")
}

// okapi: HtmlSnippetsTest#testTagLowerCaseFix
func TestSnippets_TagLowerCaseFix(t *testing.T) {
	parts := readHTML(t, "<B><FONT SIZE=3>our accomplishments</B></FONT>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "our accomplishments")
}

// okapi: HtmlSnippetsTest#testFreeMarker
func TestSnippets_FreeMarker(t *testing.T) {
	parts := readHTML(t, "<strong> this is a bolded text between html strong tags </strong> <#if contactInfo??> or ${contactInfo}</#if>.")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "this is a bolded text between html strong tags")
}

// okapi: HtmlSnippetsTest#testUlWithScriptTag
func TestSnippets_UlWithScriptTag(t *testing.T) {
	snippet := "<ul>\n" +
		"<li><script type=\"text/x-nonsense\">H</script> is the magnetic field intensity</li>\n" +
		"<li><i>J</i>is the conduction current density</li>\n" +
		"</ul>"
	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "magnetic field intensity"),
		"should contain 'magnetic field intensity'")
	assert.True(t, blockTextsContain(texts, "J"),
		"should contain 'J'")
}

// okapi: HtmlSnippetsTest#testWithoutPreserveCharacterEntities
func TestSnippets_WithoutPreserveCharacterEntities(t *testing.T) {
	parts := readHTML(t, "<p>& and &amp;</p>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "& and &")
}

// okapi: HtmlSnippetsTest#testPlaceholderOnlySegments
func TestSnippets_PlaceholderOnlySegments(t *testing.T) {
	parts := readHTML(t, "<table><tr><td><br/></td></tr><tr><td><img src='...'></td></tr></table>")
	blocks := translatableBlocks(parts)
	assert.Equal(t, 2, len(blocks), "should produce 2 text units for placeholder-only segments")
}

// --- Events Tests (from HtmlEventTest.java) ---

// okapi: HtmlEventTest#testMetaTagContent
func TestEvents_MetaTagContent(t *testing.T) {
	parts := readHTML(t, `<meta http-equiv="keywords" content="one,two,three"/>`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable block for meta keywords content")
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "one,two,three")
}

// okapi: HtmlEventTest#testPWithAttributes
func TestEvents_PWithAttributes(t *testing.T) {
	parts := readHTML(t, `<p title="my title" dir="rtl">Text of p</p>`)
	blocks := translatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should have at least 2 translatable blocks")

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "my title", "should extract title attribute value")
	assert.Contains(t, texts, "Text of p", "should extract paragraph text")

	for _, b := range blocks {
		if b.SourceText() == "my title" {
			assert.True(t, b.IsReferent, "title attribute block should be a referent")
			assert.Equal(t, "title", b.Type, "title block type should be 'title'")
		}
		if b.SourceText() == "Text of p" {
			assert.Equal(t, "paragraph", b.Type, "p element block type should be 'paragraph'")
			require.NotNil(t, b.Properties, "paragraph block should have properties")
			assert.Equal(t, "rtl", b.Properties["dir"], "dir property should be 'rtl'")
		}
	}
}

// okapi: HtmlEventTest#testIdOnP
func TestEvents_IdOnP(t *testing.T) {
	parts := readHTML(t, `<p id="foo"/>`)
	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks, "should produce a block for <p id='foo'/>")

	b := blocks[0]
	assert.Equal(t, "paragraph", b.Type, "block type should be 'paragraph'")
	assert.Contains(t, b.Name, "foo", "block name should contain the id value")
	require.NotNil(t, b.Properties, "block should have properties")
	assert.Equal(t, "foo", b.Properties["id"], "id source property should be 'foo'")
}

// okapi: HtmlEventTest#testLang
func TestEvents_Lang(t *testing.T) {
	parts := readHTML(t, `<dummy lang="en"/>`)
	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// okapi: HtmlEventTest#testXmlLang
func TestEvents_XmlLang(t *testing.T) {
	parts := readHTML(t, `<yyy xml:lang="en"/>`)
	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// okapi: HtmlEventTest#testMETATagWithLanguage
func TestEvents_METATagWithLanguage(t *testing.T) {
	parts := readHTML(t, `<meta http-equiv="Content-Language" content="en"/>`)
	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// okapi: HtmlEventTest#testMETATagWithEncoding
func TestEvents_METATagWithEncoding(t *testing.T) {
	parts := readHTML(t, `<meta http-equiv="Content-Type" content="text/html; charset=ISO-2022-JP">`)
	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "encoding")
	require.NotNil(t, dp, "should have Data part with encoding property")
	assert.Equal(t, "ISO-2022-JP", dp.Properties["encoding"])
}

// okapi: HtmlEventTest#testMetaWithCharsetAttribute
func TestEvents_MetaWithCharsetAttribute(t *testing.T) {
	parts := readHTML(t, `<meta charset="ISO-2022-JP">`)
	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "encoding")
	require.NotNil(t, dp, "should have Data part with encoding property")
	assert.Equal(t, "ISO-2022-JP", dp.Properties["encoding"])
}

// okapi: HtmlEventTest#testPWithInlines
func TestEvents_PWithInlines(t *testing.T) {
	parts := readHTML(t, `<p>Before <b>bold</b> <a href="there"/> after.</p>`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	paraBlock := findBlockContaining(blocks, "Before")
	require.NotNil(t, paraBlock, "should have a block containing 'Before'")
	assert.Equal(t, "paragraph", paraBlock.Type)

	text := paraBlock.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "bold")
	assert.Contains(t, text, "after.")

	frag := paraBlock.FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 3,
		"should have at least 3 spans: <b> opening, </b> closing, <a/> placeholder")

	var hasOpening, hasClosing bool
	for _, s := range frag.Spans {
		switch s.SpanType {
		case model.SpanOpening:
			hasOpening = true
		case model.SpanClosing:
			hasClosing = true
		}
	}
	assert.True(t, hasOpening, "should have opening span for <b>")
	assert.True(t, hasClosing, "should have closing span for </b>")
	// Note: <a href="there"/> is not self-closing in HTML5.
	// Go's HTML parser treats it as <a href="there"> with children,
	// so it produces opening/closing spans rather than a placeholder.
}

// okapi: HtmlEventTest#testPWithInlines2
func TestEvents_PWithInlines2(t *testing.T) {
	parts := readHTML(t, `<p>Before <b>bold</b> <img href="there" alt="text"/> after.</p>`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "text", "should extract alt attribute as translatable text")

	paraBlock := findBlockContaining(blocks, "Before")
	require.NotNil(t, paraBlock, "should have a paragraph block")

	paraText := paraBlock.SourceText()
	assert.Contains(t, paraText, "Before")
	assert.Contains(t, paraText, "bold")
	assert.Contains(t, paraText, "after.")

	for _, b := range blocks {
		if b.SourceText() == "text" {
			assert.True(t, b.IsReferent, "alt attribute block should be a referent")
			assert.Equal(t, "alt", b.Type, "alt block type should be 'alt'")
		}
	}
}

// okapi: HtmlEventTest#testPWithComment
func TestEvents_PWithComment(t *testing.T) {
	parts := readHTML(t, `<p>Before <!--comment--> after.</p>`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "paragraph", b.Type)

	text := b.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "after.")

	frag := b.FirstFragment()
	require.NotNil(t, frag)
	var hasPlaceholder bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "HTML comment should produce a placeholder span")
}

// okapi: HtmlEventTest#testTableGroups
func TestEvents_TableGroups(t *testing.T) {
	parts := readHTML(t, `<table id="100"><tr><td>text</td></tr></table>`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "text")

	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: HtmlEventTest#testGroupInPara
func TestEvents_GroupInPara(t *testing.T) {
	snippet := "<p>Text before list:" +
		"<ul>" +
		"<li>Text of item 1</li>" +
		"<li>Text of item 2</li>" +
		"</ul>" +
		"and text after the list.</p>"
	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "Text of item 1")
	assert.Contains(t, texts, "Text of item 2")

	var hasBeforeText, hasAfterText bool
	for _, b := range blocks {
		src := b.SourceText()
		if strings.Contains(src, "Text before list:") {
			hasBeforeText = true
		}
		if strings.Contains(src, "and text after the list.") {
			hasAfterText = true
		}
	}
	assert.True(t, hasBeforeText, "should contain 'Text before list:' in some block")
	assert.True(t, hasAfterText, "should contain 'and text after the list.' in some block")
}

// okapi: HtmlEventTest#testPropertyInEmptyParagraph
func TestEvents_PropertyInEmptyParagraph(t *testing.T) {
	parts := readHTML(t, "<p dir=\"test\"> </p>\n")
	require.NotEmpty(t, parts, "should produce parts without errors")
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: HtmlEventTest#testPreserveWhitespace
func TestEvents_PreserveWhitespace(t *testing.T) {
	parts := readHTML(t, "<pre>\twhitespace is preserved</pre>")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "pre", b.Type, "block type should be 'pre'")
	assert.True(t, b.PreserveWhitespace, "block should have PreserveWhitespace=true")

	text := b.SourceText()
	assert.Contains(t, text, "\t", "should preserve tab character")
	assert.Contains(t, text, "whitespace is preserved")
}

// --- Configuration Tests (from HtmlConfigurationTest.java) ---

// okapi: HtmlConfigurationTest#defaultConfiguration
func TestConfig_DefaultConfiguration(t *testing.T) {
	parts := readHTML(t, `<html><head><title>Page Title</title></head><body><p>Body text</p></body></html>`)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "Page Title")
	assert.Contains(t, texts, "Body text")
}

// okapi: HtmlConfigurationTest#preserveWhiteSpace
func TestConfig_PreserveWhiteSpace(t *testing.T) {
	parts := readHTML(t, `<html><body><pre>  preserved  whitespace  </pre></body></html>`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from pre element")

	var preBlock *model.Block
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "preserved") {
			preBlock = b
			break
		}
	}
	require.NotNil(t, preBlock, "should find a block with preserved whitespace from <pre>")
	assert.True(t, preBlock.PreserveWhitespace, "pre element block should have PreserveWhitespace=true")
}

// okapi: HtmlConfigurationTest#genericCodeTypes
func TestConfig_GenericCodeTypes(t *testing.T) {
	snippet := `<html><body><p>` +
		`<b>bold</b> ` +
		`<i>italic</i> ` +
		`<u>underlined</u> ` +
		`<a href="#">link</a> ` +
		`<img src="test.png" alt="image"> ` +
		`text</p></body></html>`

	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks with inline elements")

	var blockWithSpans *model.Block
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			blockWithSpans = b
			break
		}
	}
	require.NotNil(t, blockWithSpans, "should have at least one block with inline spans")

	frag := blockWithSpans.FirstFragment()
	spanTypes := make(map[string]bool)
	for _, s := range frag.Spans {
		if s.Type != "" {
			spanTypes[s.Type] = true
		}
	}
	assert.Greater(t, len(spanTypes), 1,
		"should have multiple distinct span types for b, i, u, a, img elements")

	var hasOpening, hasClosing, hasPlaceholder bool
	for _, s := range frag.Spans {
		switch s.SpanType {
		case model.SpanOpening:
			hasOpening = true
		case model.SpanClosing:
			hasClosing = true
		case model.SpanPlaceholder:
			hasPlaceholder = true
		}
	}
	assert.True(t, hasOpening, "should have opening spans")
	assert.True(t, hasClosing, "should have closing spans")
	assert.True(t, hasPlaceholder, "should have placeholder span for <img>")
}

// okapi: HtmlConfigurationTest#collapseWhitespace
func TestConfig_CollapseWhitespace(t *testing.T) {
	snippet := "<html><body><p> t1  \nt2  </p></body></html>"

	// Default: whitespace should be collapsed.
	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "t1 t2", blocks[0].SourceText(),
		"default config should collapse whitespace")

	// With preserve_whitespace=true.
	parts2 := readHTMLWithConfig(t, snippet, map[string]any{"preserveWhitespace": true})
	blocks2 := translatableBlocks(parts2)
	require.NotEmpty(t, blocks2)
	assert.Equal(t, " t1  \nt2  ", blocks2[0].SourceText(),
		"preserve_whitespace=true should preserve all whitespace")
}

// okapi: HtmlConfigurationSupportTest#test_PRESERVE_WHITESPACE
func TestConfig_PreserveWhitespacePerElement(t *testing.T) {
	snippet := "<p> t1  \nt2  </p><pre> t3  \nt4  </pre>"
	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "t1 t2", blocks[0].SourceText())
	assert.Equal(t, " t3  \nt4  ", blocks[1].SourceText())
}

// okapi: HtmlConfigurationSupportTest#test_GLOBAL_PRESERVE_WHITESPACE
func TestConfig_GlobalPreserveWhitespace(t *testing.T) {
	snippet := "<p> t1  \nt2  </p><pre> t3  \nt4  </pre>"
	parts := readHTMLWithConfig(t, snippet, map[string]any{"preserveWhitespace": true})
	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, " t1  \nt2  ", blocks[0].SourceText())
	assert.Equal(t, " t3  \nt4  ", blocks[1].SourceText())
}

// okapi: HtmlConfigurationTest#metaTag
func TestConfig_MetaTag(t *testing.T) {
	snippet := `<html><head>` +
		`<meta name="keywords" content="localization, translation, i18n">` +
		`<meta name="description" content="A tool for localization">` +
		`<meta name="generator" content="Hugo 0.92">` +
		`<meta http-equiv="content-language" content="en">` +
		`</head><body><p>Body</p></body></html>`

	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "localization, translation, i18n")
	assert.Contains(t, texts, "A tool for localization")
	assert.Contains(t, texts, "Body")
	assert.NotContains(t, texts, "Hugo 0.92",
		"META generator content should not be translatable")
}

// okapi: HtmlConfigurationTest#langAndXmlLang
func TestConfig_LangAndXmlLang(t *testing.T) {
	snippet := `<html lang="en"><body>` +
		`<p lang="en">English paragraph</p>` +
		`<div xml:lang="en">XML lang div</div>` +
		`</body></html>`

	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "English paragraph")
	assert.Contains(t, texts, "XML lang div")
	assert.NotContains(t, texts, "en",
		"lang attribute value should not be extracted as translatable text")
}

// okapi: HtmlConfigurationTest#inputAttributes
func TestConfig_InputAttributes(t *testing.T) {
	// type="hidden" — value should NOT be extracted.
	htmlHidden := `<html><body>` +
		`<input type="hidden" value="hidden-value" title="hidden-title">` +
		`<p>Other text</p>` +
		`</body></html>`

	parts := readHTML(t, htmlHidden)
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.NotContains(t, texts, "hidden-value")
	assert.Contains(t, texts, "hidden-title")

	// type="submit" — value SHOULD be extracted.
	htmlSubmit := `<html><body>` +
		`<input type="submit" value="Submit Form" title="submit-title" alt="submit-alt">` +
		`<p>Other text</p>` +
		`</body></html>`

	parts2 := readHTML(t, htmlSubmit)
	blocks2 := translatableBlocks(parts2)
	texts2 := blockTexts(blocks2)

	assert.Contains(t, texts2, "Submit Form")
	assert.Contains(t, texts2, "submit-title")

	// type="button" — value SHOULD be extracted.
	htmlButton := `<html><body>` +
		`<input type="button" value="Click Me" title="button-title">` +
		`<p>Other text</p>` +
		`</body></html>`

	parts3 := readHTML(t, htmlButton)
	blocks3 := translatableBlocks(parts3)
	texts3 := blockTexts(blocks3)

	assert.Contains(t, texts3, "Click Me")
	assert.Contains(t, texts3, "button-title")
}

// --- Full File Tests (from HtmlFullFileTest.java) ---

// okapi: HtmlFullFileTest#testOpenTwiceWithString
func TestFullFile_OpenTwiceWithString(t *testing.T) {
	htmlContent := "<b>bolded html</b>"

	blocks1 := translatableBlocks(readHTML(t, htmlContent))
	blocks2 := translatableBlocks(readHTML(t, htmlContent))

	require.NotEmpty(t, blocks1)
	require.Equal(t, len(blocks1), len(blocks2))
	assert.Equal(t, blockTexts(blocks1), blockTexts(blocks2))
}

// okapi: HtmlFullFileTest#testSkippedScriptandStyleElements
func TestFullFile_SkippedScriptAndStyleElements(t *testing.T) {
	snippet := `<html><body><p>First Text</p><script>var x=1;</script><style>h1{color:red}</style><p>Second Text</p></body></html>`
	parts := readHTML(t, snippet)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	assert.Equal(t, "First Text", blocks[0].SourceText())
	texts := blockTexts(blocks)
	for _, text := range texts {
		assert.NotContains(t, text, "h1 {color:red}")
	}
}

// --- Roundtrip Tests ---

func TestRoundtrip_Simple(t *testing.T) {
	output := roundtrip(t, `<html><body><p>Hello world</p></body></html>`)
	assert.Contains(t, output, "Hello world")
}
