//go:build integration

package html

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findDataPartWithProperty finds the first Data part that has the given property key.
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

// TestEvents_WithDefaultConfig verifies that several key tests also pass when
// using the non-wellformed (default HTML) configuration. The Java test runs
// testMetaTagContent, testLang, testXmlLang, testMETATagWithLanguage, and
// testMETATagWithEncoding with nonwellformedConfiguration.yml. Here we verify
// the same snippets parse correctly with assumeWellformed=false.
// okapi: HtmlEventTest#testWithDefaultConfig
func TestEvents_WithDefaultConfig(t *testing.T) {
	params := map[string]any{
		"assumeWellformed": false,
	}

	// testMetaTagContent: extract meta keywords content
	t.Run("MetaTagContent", func(t *testing.T) {
		parts := readHTML(t, `<meta http-equiv="keywords" content="one,two,three"/>`, params)
		blocks := bridgetest.TranslatableBlocks(parts)
		require.NotEmpty(t, blocks, "should extract translatable block for meta content")
		texts := bridgetest.BlockTexts(blocks)
		assert.Contains(t, texts, "one,two,three")
	})

	// testLang: extract lang attribute as a source property on the Data part.
	t.Run("Lang", func(t *testing.T) {
		parts := readHTML(t, `<dummy lang="en"/>`, params)
		require.NotEmpty(t, parts)
		dp := findDataPartWithProperty(parts, "language")
		require.NotNil(t, dp, "should have Data part with language property")
		assert.Equal(t, "en", dp.Properties["language"])
	})

	// testXmlLang: extract xml:lang attribute as a source property.
	t.Run("XmlLang", func(t *testing.T) {
		parts := readHTML(t, `<yyy xml:lang="en"/>`, params)
		require.NotEmpty(t, parts)
		dp := findDataPartWithProperty(parts, "language")
		require.NotNil(t, dp, "should have Data part with language property")
		assert.Equal(t, "en", dp.Properties["language"])
	})

	// testMETATagWithLanguage: meta Content-Language stored as language property.
	t.Run("METATagWithLanguage", func(t *testing.T) {
		parts := readHTML(t, `<meta http-equiv="Content-Language" content="en"/>`, params)
		require.NotEmpty(t, parts)
		dp := findDataPartWithProperty(parts, "language")
		require.NotNil(t, dp, "should have Data part with language property")
		assert.Equal(t, "en", dp.Properties["language"])
	})

	// testMETATagWithEncoding: meta Content-Type charset stored as encoding property.
	t.Run("METATagWithEncoding", func(t *testing.T) {
		parts := readHTML(t, `<meta http-equiv="Content-Type" content="text/html; charset=ISO-2022-JP">`, params)
		require.NotEmpty(t, parts)
		dp := findDataPartWithProperty(parts, "encoding")
		require.NotNil(t, dp, "should have Data part with encoding property")
		assert.Equal(t, "ISO-2022-JP", dp.Properties["encoding"])
	})
}

// TestEvents_HtmlKeywordsNotExtracted verifies that meta keywords content is
// extracted as a translatable text unit with nonwellformed configuration.
// The Java test builds expected events manually and compares with laxCompareEvents.
// okapi: HtmlEventTest#testHtmlKeywordsNotExtracted
func TestEvents_HtmlKeywordsNotExtracted(t *testing.T) {
	params := map[string]any{
		"assumeWellformed": false,
	}

	parts := readHTML(t, `<meta http-equiv="keywords" content="keyword text"/>`, params)

	// The HTML filter should extract the "content" attribute value as a
	// translatable text unit when http-equiv="keywords".
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "keywords meta content should produce a translatable block")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "keyword text")

	// Should also have Data parts for the meta tag skeleton.
	assert.Greater(t, countPartsByType(parts, model.PartData), 0,
		"should have Data part for meta tag structure")
}

// TestEvents_BaseTag verifies that <base> tag's href attribute is extracted
// as a source property on the Data part.
// okapi: HtmlEventTest#baseTag
func TestEvents_BaseTag(t *testing.T) {
	parts := readHTMLDefault(t, `<base href="https://www.example.com/" target="_top">`)

	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "href")
	require.NotNil(t, dp, "should have Data part with href property")
	assert.Equal(t, "https://www.example.com/", dp.Properties["href"])
}

// TestEvents_MetaTagContent verifies that meta keywords content is extracted as
// a translatable text unit with the wellformed configuration.
// okapi: HtmlEventTest#testMetaTagContent
func TestEvents_MetaTagContent(t *testing.T) {
	parts := readHTMLDefault(t, `<meta http-equiv="keywords" content="one,two,three"/>`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable block for meta keywords content")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "one,two,three")

	// The block should be marked as a referent (embedded in skeleton).
	for _, b := range blocks {
		if b.SourceText() == "one,two,three" {
			assert.True(t, b.IsReferent, "meta content block should be a referent")
			assert.Equal(t, "content", b.Type, "block type should be 'content'")
			break
		}
	}
}

// TestEvents_PWithAttributes verifies that a <p> with title and dir attributes
// produces the correct blocks: one for the title attribute value and one for
// the paragraph text.
// okapi: HtmlEventTest#testPWithAttributes
func TestEvents_PWithAttributes(t *testing.T) {
	parts := readHTMLDefault(t, `<p title='my title' dir='rtl'>Text of p</p>`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should have at least 2 translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "my title", "should extract title attribute value")
	assert.Contains(t, texts, "Text of p", "should extract paragraph text")

	// The title block should be a referent and typed as "title".
	for _, b := range blocks {
		if b.SourceText() == "my title" {
			assert.True(t, b.IsReferent, "title attribute block should be a referent")
			assert.Equal(t, "title", b.Type, "title block type should be 'title'")
		}
		if b.SourceText() == "Text of p" {
			assert.Equal(t, "paragraph", b.Type, "p element block type should be 'paragraph'")
			// The dir attribute should appear as a source property on the block.
			require.NotNil(t, b.Properties, "paragraph block should have properties")
			assert.Equal(t, "rtl", b.Properties["dir"], "dir property should be 'rtl'")
		}
	}
}

// TestEvents_Lang verifies that the lang attribute is extracted as a
// "language" source property on a Data part.
// okapi: HtmlEventTest#testLang
func TestEvents_Lang(t *testing.T) {
	parts := readHTMLDefault(t, `<dummy lang="en"/>`)

	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// TestEvents_IdOnP verifies that a <p> element with an id attribute produces
// a block with the id as a source property and name derived from the id.
// okapi: HtmlEventTest#testIdOnP
func TestEvents_IdOnP(t *testing.T) {
	parts := readHTMLDefault(t, `<p id="foo"/>`)

	// The Java test expects: tu.setName("foo-id"), tu.setType("paragraph"),
	// tu.setSourceProperty("id", "foo", readOnly=true).
	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks, "should produce a block for <p id='foo'/>")

	b := blocks[0]
	assert.Equal(t, "paragraph", b.Type, "block type should be 'paragraph'")
	assert.Contains(t, b.Name, "foo", "block name should contain the id value")
	require.NotNil(t, b.Properties, "block should have properties")
	assert.Equal(t, "foo", b.Properties["id"], "id source property should be 'foo'")
}

// TestEvents_XmlLang verifies that xml:lang attribute is extracted as a
// "language" source property on a Data part.
// okapi: HtmlEventTest#testXmlLang
func TestEvents_XmlLang(t *testing.T) {
	parts := readHTMLDefault(t, `<yyy xml:lang="en"/>`)

	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// TestEvents_ComplexEmptyElement verifies extraction of an element with mixed
// attributes: translatable (trans), writable localizable (write), and
// read-only localizable (readonly). Uses dummyConfiguration.yml equivalent.
// okapi: HtmlEventTest#testComplexEmptyElement
func TestEvents_ComplexEmptyElement(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"elements": map[string]any{
			"dummy": map[string]any{
				"ruleTypes":                      []string{"ATTRIBUTES_ONLY"},
				"translatableAttributes":         []string{"trans"},
				"writableLocalizableAttributes":  []string{"write"},
				"readOnlyLocalizableAttributes":  []string{"readonly"},
			},
		},
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<dummy write="w" readonly="ro" trans="tu1"/>`,
		"test.html", mimeType, params)

	// The Java test expects:
	// - A referent TextUnit with id content "tu1" (the trans attribute value)
	// - A DocumentPart with source properties: write="w", readonly="ro"
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	// Should have a translatable block for the trans attribute.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should have a translatable block for the trans attribute")
	assert.Equal(t, "tu1", blocks[0].SourceText())
	assert.True(t, blocks[0].IsReferent, "trans attribute block should be referent")

	// Should have a Data part with the writable attribute as a source property.
	dataParts := bridgetest.DataParts(parts)
	require.NotEmpty(t, dataParts, "should have a Data part for the element")
	dp := dataParts[0].Resource.(*model.Data)
	assert.Equal(t, "w", dp.Properties["write"], "write attribute should be extracted as source property")
}

// TestEvents_PWithInlines verifies that a paragraph with inline bold and anchor
// elements produces a single translatable block with inline spans (codes).
// okapi: HtmlEventTest#testPWithInlines
func TestEvents_PWithInlines(t *testing.T) {
	parts := readHTMLDefault(t, `<p>Before <b>bold</b> <a href="there"/> after.</p>`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Find the main paragraph block.
	paraBlock := findBlockContaining(blocks, "Before")
	require.NotNil(t, paraBlock, "should have a block containing 'Before'")
	assert.Equal(t, "paragraph", paraBlock.Type)

	// The paragraph text should contain the inline text.
	text := paraBlock.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "bold")
	assert.Contains(t, text, "after.")

	// The fragment should have inline spans for <b>, </b>, and <a/>.
	frag := paraBlock.FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 3,
		"should have at least 3 spans: <b> opening, </b> closing, <a/> placeholder")

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
	assert.True(t, hasOpening, "should have opening span for <b>")
	assert.True(t, hasClosing, "should have closing span for </b>")
	assert.True(t, hasPlaceholder, "should have placeholder span for <a/>")
}

// TestEvents_PWithInlineAnchorAndAmpersand verifies that an anchor with
// ampersand-encoded URL parameters is correctly extracted.
// okapi: HtmlEventTest#testPWithInlineAnchorAndAmpersand
func TestEvents_PWithInlineAnchorAndAmpersand(t *testing.T) {
	parts := readHTMLDefault(t,
		`<p>Before <a href="foo.cgi?chapter=1&amp;section=2&amp;copy=3&amp;lang=en"/> after.</p>`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Find the main paragraph block.
	paraBlock := findBlockContaining(blocks, "Before")
	require.NotNil(t, paraBlock, "should have a block containing 'Before'")

	text := paraBlock.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "after.")

	// The anchor should appear as a placeholder span in the fragment.
	frag := paraBlock.FirstFragment()
	require.NotNil(t, frag)
	var hasPlaceholder bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "anchor should produce a placeholder span")
}

// TestEvents_PWithComment verifies that an HTML comment inside a paragraph
// becomes a placeholder code in the text unit's fragment.
// okapi: HtmlEventTest#testPWithComment
func TestEvents_PWithComment(t *testing.T) {
	parts := readHTMLDefault(t, `<p>Before <!--comment--> after.</p>`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "paragraph", b.Type)

	text := b.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "after.")

	// The comment should be represented as a placeholder span in the fragment.
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

// TestEvents_PWithProcessingInstruction verifies that a processing instruction
// inside a paragraph becomes a placeholder code in the text unit's fragment.
// okapi: HtmlEventTest#testPWithProcessingInstruction
func TestEvents_PWithProcessingInstruction(t *testing.T) {
	parts := readHTMLDefault(t, `<p>Before <?PI?> after.</p>`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "paragraph", b.Type)

	text := b.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "after.")

	// The PI should be represented as a placeholder span.
	frag := b.FirstFragment()
	require.NotNil(t, frag)

	var hasPlaceholder bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "processing instruction should produce a placeholder span")
}

// TestEvents_METATagWithLanguage verifies that a meta Content-Language tag
// stores the language as a source property on a Data part.
// okapi: HtmlEventTest#testMETATagWithLanguage
func TestEvents_METATagWithLanguage(t *testing.T) {
	parts := readHTMLDefault(t, `<meta http-equiv="Content-Language" content="en"/>`)

	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// TestEvents_METATagWithEncoding verifies that a meta Content-Type tag with a
// charset declaration stores the encoding as a source property on a Data part.
// okapi: HtmlEventTest#testMETATagWithEncoding
func TestEvents_METATagWithEncoding(t *testing.T) {
	parts := readHTMLDefault(t,
		`<meta http-equiv="Content-Type" content="text/html; charset=ISO-2022-JP">`)

	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "encoding")
	require.NotNil(t, dp, "should have Data part with encoding property")
	assert.Equal(t, "ISO-2022-JP", dp.Properties["encoding"])
}

// TestEvents_MetaWithCharsetAttribute verifies that a meta tag with a direct
// charset attribute stores the encoding as a source property on a Data part.
// okapi: HtmlEventTest#testMetaWithCharsetAttribute
func TestEvents_MetaWithCharsetAttribute(t *testing.T) {
	parts := readHTMLDefault(t, `<meta charset="ISO-2022-JP">`)

	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "encoding")
	require.NotNil(t, dp, "should have Data part with encoding property")
	assert.Equal(t, "ISO-2022-JP", dp.Properties["encoding"])
}

// TestEvents_PWithInlines2 verifies extraction of a paragraph with bold inline
// and an img element (which has a translatable alt attribute).
// okapi: HtmlEventTest#testPWithInlines2
func TestEvents_PWithInlines2(t *testing.T) {
	parts := readHTMLDefault(t,
		`<p>Before <b>bold</b> <img href="there" alt="text"/> after.</p>`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)

	// The alt attribute value should be extracted as a separate translatable block.
	assert.Contains(t, texts, "text", "should extract alt attribute as translatable text")

	// Find the paragraph block.
	paraBlock := findBlockContaining(blocks, "Before")
	require.NotNil(t, paraBlock, "should have a paragraph block")

	paraText := paraBlock.SourceText()
	assert.Contains(t, paraText, "Before")
	assert.Contains(t, paraText, "bold")
	assert.Contains(t, paraText, "after.")

	// The paragraph fragment should have spans for <b>, </b>, and <img/>.
	frag := paraBlock.FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 3,
		"should have spans for <b>, </b>, and <img/>")

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
	assert.True(t, hasOpening, "should have opening span for <b>")
	assert.True(t, hasClosing, "should have closing span for </b>")
	assert.True(t, hasPlaceholder, "should have placeholder span for <img/>")

	// The alt block should be a referent with type "alt".
	for _, b := range blocks {
		if b.SourceText() == "text" {
			assert.True(t, b.IsReferent, "alt attribute block should be a referent")
			assert.Equal(t, "alt", b.Type, "alt block type should be 'alt'")
		}
	}
}

// TestEvents_TableGroups verifies that table content is extracted as text units.
// Group events for table/tr require element rule YAML config (GROUP rule type),
// which is not yet supported through filter params. We verify content extraction.
// okapi: HtmlEventTest#testTableGroups
func TestEvents_TableGroups(t *testing.T) {
	parts := readHTMLDefault(t, `<table id="100"><tr><td>text</td></tr></table>`)

	// The <td> content should be a translatable text unit.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "text")

	// Verify event framing: starts with LayerStart, ends with LayerEnd.
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// TestEvents_GroupInPara verifies that an embedded list inside a paragraph
// extracts list items and surrounding paragraph text. Group events for ul/li
// require element rule YAML config (GROUP rule type).
// okapi: HtmlEventTest#testGroupInPara
func TestEvents_GroupInPara(t *testing.T) {
	snippet := "<p>Text before list:" +
		"<ul>" +
		"<li>Text of item 1</li>" +
		"<li>Text of item 2</li>" +
		"</ul>" +
		"and text after the list.</p>"
	parts := readHTMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	// Should extract both list items.
	assert.Contains(t, texts, "Text of item 1")
	assert.Contains(t, texts, "Text of item 2")

	// The paragraph text should include text before and after the list.
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

// TestEvents_PropertyInEmptyParagraph verifies that an empty paragraph with a
// dir property does not produce a null reference error. The Java test checks
// that the skeleton's property parent is not null. The empty <p> with only
// whitespace content does not produce a Block (it becomes Data parts).
// okapi: HtmlEventTest#testPropertyInEmptyParagraph
func TestEvents_PropertyInEmptyParagraph(t *testing.T) {
	parts := readHTMLDefault(t, "<p dir=\"test\"> </p>\n")

	// The main verification is that parsing does not panic or error.
	require.NotEmpty(t, parts, "should produce parts without errors")

	// Should have the standard LayerStart/LayerEnd framing.
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// TestEvents_PreserveWhitespace verifies that a <pre> element produces a block
// with PreserveWhitespace=true and preserves the tab character.
// okapi: HtmlEventTest#testPreserveWhitespace
func TestEvents_PreserveWhitespace(t *testing.T) {
	parts := readHTMLDefault(t, "<pre>\twhitespace is preserved</pre>")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "pre", b.Type, "block type should be 'pre'")
	assert.True(t, b.PreserveWhitespace, "block should have PreserveWhitespace=true")

	// The text should preserve the tab character.
	text := b.SourceText()
	assert.Contains(t, text, "\t", "should preserve tab character")
	assert.Contains(t, text, "whitespace is preserved")
}
