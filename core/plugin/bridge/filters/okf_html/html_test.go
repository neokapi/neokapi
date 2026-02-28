//go:build integration

package okf_html

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.html.HtmlFilter"
const mimeType = "text/html"

// okapi: HtmlSnippetsTest#minimalCompleteHtml
func TestExtract_SimpleHTML(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello world</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract at least one translatable block")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}

// okapi: HtmlSnippetsTest#testInlineCodesStorage
func TestExtract_InlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello <b>bold</b> world</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Find the block containing "bold"
	var found *model.Block
	for _, b := range blocks {
		if b.SourceText() == "Hello bold world" {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find block with 'Hello bold world'")

	// The block should have spans (inline codes) for <b> and </b>
	frag := found.FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 2, "should have at least opening and closing spans for <b>")

	// Check span types
	hasOpening := false
	hasClosing := false
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening {
			hasOpening = true
		}
		if s.SpanType == model.SpanClosing {
			hasClosing = true
		}
	}
	assert.True(t, hasOpening, "should have an opening span")
	assert.True(t, hasClosing, "should have a closing span")
}

func TestExtract_MultipleBlocks(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<h1>Title</h1>
		<p>First paragraph</p>
		<p>Second paragraph</p>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3, "should extract title and two paragraphs")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "First paragraph")
	assert.Contains(t, texts, "Second paragraph")
}

// okapi: HtmlFullFileTest#testSkippedScriptandStyleElements
func TestExtract_NonTranslatable(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<p>Translatable text</p>
		<script>var x = 1;</script>
		<style>.foo { color: red; }</style>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	// Script and style content should NOT appear in translatable blocks.
	assert.Contains(t, texts, "Translatable text")
	for _, text := range texts {
		assert.NotContains(t, text, "var x = 1")
		assert.NotContains(t, text, "color: red")
	}
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello</p></body></html>`,
		"test.html", mimeType, nil)

	// Should have at least one LayerStart and one LayerEnd
	var hasLayerStart, hasLayerEnd bool
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			hasLayerStart = true
		}
		if p.Type == model.PartLayerEnd {
			hasLayerEnd = true
		}
	}
	assert.True(t, hasLayerStart, "should have a LayerStart part")
	assert.True(t, hasLayerEnd, "should have a LayerEnd part")
}

// okapi: HtmlSnippetsTest#testPWithInlines2
func TestExtract_NestedInlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello <b><i>bold italic</i></b> text</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Find the block with nested codes
	var found *model.Block
	for _, b := range blocks {
		if b.SourceText() == "Hello bold italic text" {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find block with nested codes")

	frag := found.FirstFragment()
	require.NotNil(t, frag)
	// Should have 4 spans: <b>, <i>, </i>, </b>
	assert.GreaterOrEqual(t, len(frag.Spans), 4, "should have spans for nested <b><i>...</i></b>")
}

// okapi: HtmlSnippetsTest#paraWithBreak
func TestExtract_SelfClosingTags(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Line one<br/>Line two</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Find block with <br/>
	var found *model.Block
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil {
			for _, s := range frag.Spans {
				if s.SpanType == model.SpanPlaceholder {
					found = b
					break
				}
			}
		}
	}
	assert.NotNil(t, found, "should find a block with a placeholder span for <br/>")
}

// okapi: HtmlSnippetsTest#testEscapes
func TestExtract_Entities(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Price: &lt;$10 &amp; &gt;$5</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	// Entities should be decoded in the extracted text.
	assert.Contains(t, texts, "Price: <$10 & >$5")
}

// okapi: HtmlSnippetsTest#testAltInImg
// okapi: HtmlSnippetsTest#testTitleInP
func TestExtract_Attributes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<img src="photo.jpg" alt="A beautiful sunset" title="Sunset photo"/>
		<p>Some text</p>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	// The alt and title attributes should be extracted as translatable content.
	assert.Contains(t, texts, "A beautiful sunset")
	assert.Contains(t, texts, "Sunset photo")
}

// okapi: HtmlSnippetsTest#testTableGroups
func TestExtract_Table(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<table>
			<tr><th>Header 1</th><th>Header 2</th></tr>
			<tr><td>Cell A</td><td>Cell B</td></tr>
		</table>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Header 1")
	assert.Contains(t, texts, "Header 2")
	assert.Contains(t, texts, "Cell A")
	assert.Contains(t, texts, "Cell B")
}

func TestExtract_BlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<p>First</p>
		<p>Second</p>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	// Each block should have a unique, non-empty ID.
	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestExtract_SegmentIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello world</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotNil(t, seg.Content, "segment should have content")
		}
	}
}

func TestExtract_SpanData(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello <b>bold</b> world</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	var found *model.Block
	for _, b := range blocks {
		if b.SourceText() == "Hello bold world" {
			found = b
			break
		}
	}
	require.NotNil(t, found)

	frag := found.FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 2)

	// Opening span should have data containing the original markup.
	openingSpan := frag.Spans[0]
	assert.Equal(t, model.SpanOpening, openingSpan.SpanType)
	assert.NotEmpty(t, openingSpan.Data, "opening span should have data (original markup)")
	assert.Contains(t, openingSpan.Data, "b", "data should reference the <b> tag")

	// Closing span should also have data.
	var closingSpan *model.Span
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanClosing {
			closingSpan = s
			break
		}
	}
	require.NotNil(t, closingSpan, "should have a closing span")
	assert.NotEmpty(t, closingSpan.Data, "closing span should have data")
}

func TestExtract_SpanDisplayText(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Click <a href="http://example.com">here</a> now</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Find the block with the link.
	var found *model.Block
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find block with inline codes")

	frag := found.FirstFragment()
	require.NotNil(t, frag)

	// Verify spans carry data about the markup.
	for _, s := range frag.Spans {
		assert.NotEmpty(t, s.ID, "span should have an ID")
	}
}

func TestExtract_LayerFormat(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello</p></body></html>`,
		"test.html", mimeType, nil)

	// The LayerStart should carry the filter format ID.
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layer := p.Resource.(*model.Layer)
			assert.NotEmpty(t, layer.Format, "layer should have a format (filter ID)")
			assert.Contains(t, layer.Format, "html", "format should reference HTML filter")
			assert.NotEmpty(t, layer.MimeType, "layer should have a MIME type")
			break
		}
	}
}

func TestExtract_DataParts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// HTML with structure creates DocumentPart events (Data parts) for tags
	// that are not translatable.
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello</p></body></html>`,
		"test.html", mimeType, nil)

	var dataCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
			data := p.Resource.(*model.Data)
			assert.NotEmpty(t, data.ID, "data part should have an ID")
		}
	}
	assert.Greater(t, dataCount, 0, "should have at least one Data part from HTML structure")
}

func TestExtract_BlockSkeleton(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello world</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// HTML blocks should have skeleton data that preserves the surrounding markup.
	var found *model.Block
	for _, b := range blocks {
		if b.Skeleton != nil && len(b.Skeleton.Parts) > 0 {
			found = b
			break
		}
	}
	require.NotNil(t, found, "at least one block should have a skeleton")
	assert.Greater(t, len(found.Skeleton.Parts), 0, "skeleton should have parts")
}

func TestExtract_DataSkeleton(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello</p><hr/><p>World</p></body></html>`,
		"test.html", mimeType, nil)

	// Data parts (from non-translatable structure like <hr/>) should also
	// carry skeleton data.
	var dataWithSkeleton int
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Skeleton != nil && len(data.Skeleton.Parts) > 0 {
				dataWithSkeleton++
			}
		}
	}
	assert.Greater(t, dataWithSkeleton, 0, "some Data parts should have skeletons")
}

// okapi: HtmlSnippetsTest#ITextUnitsInARowWithTwoHeaders
func TestExtract_Headings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<h1>Main Title</h1>
		<h2>Subtitle</h2>
		<h3>Section</h3>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Main Title")
	assert.Contains(t, texts, "Subtitle")
	assert.Contains(t, texts, "Section")
}

// okapi: HtmlSnippetsTest#testGroupInPara
func TestExtract_List(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<ul>
			<li>Item one</li>
			<li>Item two</li>
			<li>Item three</li>
		</ul>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Item one")
	assert.Contains(t, texts, "Item two")
	assert.Contains(t, texts, "Item three")
}

// okapi: HtmlSnippetsTest#testMETATag1
func TestExtract_MetaNotTranslatable(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html>
		<head>
			<meta charset="UTF-8">
			<meta name="viewport" content="width=device-width">
		</head>
		<body><p>Body text</p></body>
	</html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Body text")
	for _, text := range texts {
		assert.NotContains(t, text, "width=device-width",
			"meta viewport content should not be extracted")
	}
}

func TestExtract_UnicodeContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>こんにちは世界</p><p>مرحبا بالعالم</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "こんにちは世界")
	assert.Contains(t, texts, "مرحبا بالعالم")
}

func TestExtract_PartSequenceIntegrity(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello</p></body></html>`,
		"test.html", mimeType, nil)

	require.NotEmpty(t, parts)

	// First part should be LayerStart, last should be LayerEnd.
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

	// Layer balance: starts == ends.
	var starts, ends int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			starts++
		}
		if p.Type == model.PartLayerEnd {
			ends++
		}
	}
	assert.Equal(t, starts, ends, "layer starts and ends should be balanced")
}

// okapi: HtmlSnippetsTest#italicBoldEtc
func TestExtract_MultipleInlineTags(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<html><body><p>Hello <em>emphasized</em> and <strong>strong</strong> text</p></body></html>`,
		"test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	var found *model.Block
	for _, b := range blocks {
		if b.SourceText() == "Hello emphasized and strong text" {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find block with multiple inline tags")

	frag := found.FirstFragment()
	require.NotNil(t, frag)
	// Should have spans for <em></em> and <strong></strong> (4 spans minimum).
	assert.GreaterOrEqual(t, len(frag.Spans), 4,
		"should have spans for <em></em> and <strong></strong>")
}

// --- Testdata file tests ---

func TestExtract_FormElements(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_html/form.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "form.html should produce translatable blocks")
}

// okapi: HtmlDetectBomTest#testDetectBom
func TestExtract_UTF8WithBOM(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_html/UTF8WithBOM.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "UTF-8 with BOM should produce translatable blocks")
}

func TestExtract_RubyAnnotation(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_html/ruby.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "ruby annotation HTML should produce translatable blocks")
}

// okapi: HtmlSnippetsTest#testSupplementalSupport
func TestExtract_Emoji(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_html/emoji1.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "emoji HTML should produce translatable blocks")
}

func TestExtract_SimpleLink(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_html/simple_link.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Links should produce blocks with inline code spans.
	var hasSpans bool
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			hasSpans = true
			break
		}
	}
	assert.True(t, hasSpans, "simple_link.html should have blocks with inline code spans")
}

// okapi: HtmlSnippetsTest#testCollapseWhitespaceWithPre
func TestExtract_PreformattedText(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<pre>  Preformatted  text  </pre>
		<p>Normal text</p>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Normal text")

	// Pre-formatted text should be extracted with whitespace preserved.
	var hasPreText bool
	for _, text := range texts {
		if text == "  Preformatted  text  " {
			hasPreText = true
			break
		}
	}
	assert.True(t, hasPreText, "pre-formatted text should preserve whitespace")
}

// okapi: HtmlSnippetsTest#testAltInImg
func TestExtract_ImageAltAndTitle(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<img src="photo.jpg" alt="Mountain landscape" title="Beautiful mountains"/>
		<img src="icon.svg" alt=""/>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Mountain landscape")
	assert.Contains(t, texts, "Beautiful mountains")
}

func TestExtract_DefinitionList(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<dl>
			<dt>Term One</dt>
			<dd>Definition one</dd>
			<dt>Term Two</dt>
			<dd>Definition two</dd>
		</dl>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Term One")
	assert.Contains(t, texts, "Definition one")
	assert.Contains(t, texts, "Term Two")
	assert.Contains(t, texts, "Definition two")
}

// okapi: HtmlSnippetsTest#testCollapseWhitespaceWithoutPre
func TestExtract_WhitespaceInMixedContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body><p>Hello   <b>  bold  </b>   world</p></body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The extracted text should combine the inline content.
	var found *model.Block
	for _, b := range blocks {
		text := b.SourceText()
		if text == "Hello bold world" || text == "Hello   bold   world" || text == "Hello  bold  world" {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find block with mixed content text")
}

func TestExtract_NestedDivs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	html := `<html><body>
		<div>
			<div>
				<p>Nested paragraph</p>
			</div>
		</div>
	</body></html>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		html, "test.html", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Nested paragraph")
}
