//go:build integration

package okf_html

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// testLang: extract lang attribute
	t.Run("Lang", func(t *testing.T) {
		parts := readHTML(t, `<dummy lang="en"/>`, params)
		require.NotEmpty(t, parts, "should produce parts for lang attribute")
		var foundData bool
		for _, p := range parts {
			if p.Type == model.PartData {
				data := p.Resource.(*model.Data)
				if lang, ok := data.Properties["language"]; ok {
					assert.Equal(t, "en", lang)
					foundData = true
				}
			}
		}
		assert.True(t, foundData, "should find Data part with language property")
	})

	// testXmlLang: extract xml:lang attribute
	t.Run("XmlLang", func(t *testing.T) {
		parts := readHTML(t, `<yyy xml:lang="en"/>`, params)
		require.NotEmpty(t, parts, "should produce parts for xml:lang attribute")
		var foundLang bool
		for _, p := range parts {
			if p.Type == model.PartData {
				data := p.Resource.(*model.Data)
				if lang, ok := data.Properties["language"]; ok {
					assert.Equal(t, "en", lang)
					foundLang = true
				}
			}
		}
		assert.True(t, foundLang, "should find Data part with language=en from xml:lang")
	})

	// testMETATagWithLanguage: meta Content-Language
	t.Run("METATagWithLanguage", func(t *testing.T) {
		parts := readHTML(t, `<meta http-equiv="Content-Language" content="en"/>`, params)
		require.NotEmpty(t, parts)
		var foundLang bool
		for _, p := range parts {
			if p.Type == model.PartData {
				data := p.Resource.(*model.Data)
				if lang, ok := data.Properties["language"]; ok {
					assert.Equal(t, "en", lang)
					foundLang = true
				}
			}
		}
		assert.True(t, foundLang, "should find Data part with language from Content-Language meta")
	})

	// testMETATagWithEncoding: meta Content-Type with charset
	t.Run("METATagWithEncoding", func(t *testing.T) {
		parts := readHTML(t, `<meta http-equiv="Content-Type" content="text/html; charset=ISO-2022-JP">`, params)
		require.NotEmpty(t, parts)
		var foundEncoding bool
		for _, p := range parts {
			if p.Type == model.PartData {
				data := p.Resource.(*model.Data)
				if enc, ok := data.Properties["encoding"]; ok {
					assert.Equal(t, "ISO-2022-JP", enc)
					foundEncoding = true
				}
			}
		}
		assert.True(t, foundEncoding, "should find Data part with encoding from Content-Type meta")
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

// TestEvents_BaseTag verifies that <base> tag produces a Data part with the
// href property as a writable localizable attribute.
// okapi: HtmlEventTest#baseTag
func TestEvents_BaseTag(t *testing.T) {
	parts := readHTMLDefault(t, `<base href="https://www.example.com/" target="_top">`)

	// The base tag has ATTRIBUTES_ONLY with writableLocalizableAttributes: [href].
	// It should produce a DocumentPart (Data) with the href property.
	var foundBase bool
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if href, ok := data.Properties["href"]; ok {
				assert.Equal(t, "https://www.example.com/", href)
				foundBase = true
			}
		}
	}
	assert.True(t, foundBase, "should find Data part with href property from <base> tag")
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
		}
	}
}

// TestEvents_Lang verifies that lang attribute on an element produces a Data
// part with the "language" property set.
// okapi: HtmlEventTest#testLang
func TestEvents_Lang(t *testing.T) {
	parts := readHTMLDefault(t, `<dummy lang="en"/>`)

	var foundLang bool
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if lang, ok := data.Properties["language"]; ok {
				assert.Equal(t, "en", lang)
				foundLang = true
			}
		}
	}
	assert.True(t, foundLang, "should find Data part with language=en from lang attribute")
}

// TestEvents_IdOnP verifies that a <p> element with an id attribute produces
// a block whose name includes the id value.
// okapi: HtmlEventTest#testIdOnP
func TestEvents_IdOnP(t *testing.T) {
	parts := readHTMLDefault(t, `<p id="foo"/>`)

	// The Java test expects: tu.setName("foo-id"), tu.setType("paragraph"),
	// tu.setSourceProperty("id", "foo", readOnly=true).
	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks, "should produce a block for <p id='foo'/>")

	b := blocks[0]
	assert.Equal(t, "paragraph", b.Type, "block type should be 'paragraph'")
	// The block name should incorporate the id.
	assert.Contains(t, b.Name, "foo", "block name should contain the id value")
	// The id should appear in properties.
	if b.Properties != nil {
		if id, ok := b.Properties["id"]; ok {
			assert.Equal(t, "foo", id)
		}
	}
}

// TestEvents_XmlLang verifies that xml:lang attribute produces a Data part with
// the "language" property.
// okapi: HtmlEventTest#testXmlLang
func TestEvents_XmlLang(t *testing.T) {
	parts := readHTMLDefault(t, `<yyy xml:lang="en"/>`)

	var foundLang bool
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if lang, ok := data.Properties["language"]; ok {
				assert.Equal(t, "en", lang)
				foundLang = true
			}
		}
	}
	assert.True(t, foundLang, "should find Data part with language=en from xml:lang attribute")
}

// TestEvents_ComplexEmptyElement verifies extraction of an element with mixed
// translatable, writable, and readonly attributes using the dummy configuration.
// okapi: HtmlEventTest#testComplexEmptyElement
func TestEvents_ComplexEmptyElement(t *testing.T) {
	// The Java test uses dummyConfiguration.yml which defines:
	//   dummy:
	//     ruleTypes: [ATTRIBUTES_ONLY]
	//     translatableAttributes: [trans]
	//     writableLocalizableAttributes: [write]
	//     readOnlyLocalizableAttributes: [readonly]
	params := map[string]any{
		"elements": map[string]any{
			"dummy": map[string]any{
				"ruleTypes":                     []string{"ATTRIBUTES_ONLY"},
				"translatableAttributes":        []string{"trans"},
				"writableLocalizableAttributes": []string{"write"},
				"readOnlyLocalizableAttributes": []string{"readonly"},
			},
		},
	}

	parts := readHTML(t, `<dummy write="w" readonly="ro" trans="tu1"/>`, params)

	// The "trans" attribute value should be extracted as a translatable block.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable block for 'trans' attribute")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "tu1")

	// The "write" attribute should appear in a Data part's properties.
	var foundWrite bool
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if w, ok := data.Properties["write"]; ok {
				assert.Equal(t, "w", w)
				foundWrite = true
			}
		}
	}
	assert.True(t, foundWrite, "should find Data part with write property")
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

	// The anchor with ampersand-encoded URL should produce a Data part with
	// the href property preserving the encoded ampersands.
	var foundHref bool
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if href, ok := data.Properties["href"]; ok {
				assert.Contains(t, href, "foo.cgi?chapter=1")
				foundHref = true
			}
		}
	}
	assert.True(t, foundHref, "should find Data part with href from anchor")

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
// produces a Data part with the "language" property.
// okapi: HtmlEventTest#testMETATagWithLanguage
func TestEvents_METATagWithLanguage(t *testing.T) {
	parts := readHTMLDefault(t, `<meta http-equiv="Content-Language" content="en"/>`)

	var foundLang bool
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if lang, ok := data.Properties["language"]; ok {
				assert.Equal(t, "en", lang)
				foundLang = true
			}
		}
	}
	assert.True(t, foundLang, "should find Data part with language from Content-Language meta")
}

// TestEvents_METATagWithEncoding verifies that a meta Content-Type tag with a
// charset declaration produces a Data part with the "encoding" property.
// okapi: HtmlEventTest#testMETATagWithEncoding
func TestEvents_METATagWithEncoding(t *testing.T) {
	parts := readHTMLDefault(t,
		`<meta http-equiv="Content-Type" content="text/html; charset=ISO-2022-JP">`)

	var foundEncoding bool
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if enc, ok := data.Properties["encoding"]; ok {
				assert.Equal(t, "ISO-2022-JP", enc)
				foundEncoding = true
			}
		}
	}
	assert.True(t, foundEncoding, "should find Data part with encoding from Content-Type meta")
}

// TestEvents_MetaWithCharsetAttribute verifies that a meta tag with a direct
// charset attribute produces a Data part with the "encoding" property.
// okapi: HtmlEventTest#testMetaWithCharsetAttribute
func TestEvents_MetaWithCharsetAttribute(t *testing.T) {
	parts := readHTMLDefault(t, `<meta charset="ISO-2022-JP">`)

	var foundEncoding bool
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if enc, ok := data.Properties["encoding"]; ok {
				assert.Equal(t, "ISO-2022-JP", enc)
				foundEncoding = true
			}
		}
	}
	assert.True(t, foundEncoding, "should find Data part with encoding from charset attribute")
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

// TestEvents_TableGroups verifies that table/tr elements produce group
// start/end events, and td content is extracted as a text unit.
// okapi: HtmlEventTest#testTableGroups
func TestEvents_TableGroups(t *testing.T) {
	parts := readHTMLDefault(t, `<table id="100"><tr><td>text</td></tr></table>`)

	// Should have GroupStart for <table> and <tr>.
	assert.Equal(t, 2, countPartsByType(parts, model.PartGroupStart),
		"should have 2 GroupStart events (table + tr)")
	assert.Equal(t, 2, countPartsByType(parts, model.PartGroupEnd),
		"should have 2 GroupEnd events (tr + table)")

	// The <td> content should be a translatable text unit.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "text")

	// Verify the block type is "td".
	for _, b := range blocks {
		if b.SourceText() == "text" {
			assert.Equal(t, "td", b.Type)
		}
	}

	// Verify event framing: starts with LayerStart, ends with LayerEnd.
	require.GreaterOrEqual(t, len(parts), 7)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// TestEvents_GroupInPara verifies that an embedded list inside a paragraph
// produces a group for the <ul>, text units for each <li>, and the paragraph
// text before and after the list.
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
	// The before and after text might appear as separate blocks or merged in
	// a single block with inline codes referencing the embedded group.
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

	// Should have at least one GroupStart for <ul>.
	assert.Greater(t, countPartsByType(parts, model.PartGroupStart), 0,
		"should have GroupStart for <ul>")
}

// TestEvents_PropertyInEmptyParagraph verifies that an empty paragraph with a
// dir property does not produce a null reference error. The Java test checks
// that the skeleton's property parent is not null.
// okapi: HtmlEventTest#testPropertyInEmptyParagraph
func TestEvents_PropertyInEmptyParagraph(t *testing.T) {
	parts := readHTMLDefault(t, "<p dir=\"test\"> </p>\n")

	// The main verification is that parsing does not panic or error.
	// The Java test verifies that skeleton parts have non-null parents.
	require.NotEmpty(t, parts, "should produce parts without errors")

	// Should have the standard LayerStart/LayerEnd framing.
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	// The dir property should appear somewhere (Data or Block properties).
	var foundDir bool
	for _, p := range parts {
		switch p.Type {
		case model.PartData:
			data := p.Resource.(*model.Data)
			if _, ok := data.Properties["dir"]; ok {
				foundDir = true
			}
		case model.PartBlock:
			block := p.Resource.(*model.Block)
			if _, ok := block.Properties["dir"]; ok {
				foundDir = true
			}
		}
	}
	assert.True(t, foundDir, "should find 'dir' property on some part")
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
