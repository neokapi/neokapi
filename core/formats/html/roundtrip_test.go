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

// assertRoundtripPreserved reads HTML, writes it back with original content,
// re-reads the output, and compares the block texts match.
func assertRoundtripPreserved(t *testing.T, input string) {
	t.Helper()
	ctx := context.Background()

	// First read: extract parts from input.
	reader1 := htmlfmt.NewReader()
	err := reader1.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts1 := testutil.CollectParts(t, reader1.Read(ctx))
	reader1.Close()

	// Write with original content.
	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	writer.SetOriginalContent([]byte(input))
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts1)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()

	// Second read: extract parts from output.
	reader2 := htmlfmt.NewReader()
	err = reader2.Open(ctx, testutil.RawDocFromString(output, model.LocaleEnglish))
	require.NoError(t, err)
	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	reader2.Close()

	// Compare block texts.
	blocks1 := translatableBlocks(parts1)
	blocks2 := translatableBlocks(parts2)
	texts1 := blockTexts(blocks1)
	texts2 := blockTexts(blocks2)

	assert.Equal(t, texts1, texts2, "roundtrip should preserve all block texts")
}

// --- Identity Roundtrip Tests ---

func TestRoundtrip_SimpleP(t *testing.T) {
	assertRoundtripPreserved(t, `<html><body><p>Hello world</p></body></html>`)
}

func TestRoundtrip_WithDoctype(t *testing.T) {
	input := `<!DOCTYPE html><html><head><title>My Page</title></head><body><p>Content</p></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, "<!DOCTYPE html>", "should preserve doctype")
	assert.Contains(t, output, "<title>My Page</title>", "should preserve title")
	assert.Contains(t, output, "Content", "should preserve body content")
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_WithHeadMeta(t *testing.T) {
	input := `<html><head><meta charset="utf-8"><meta name="description" content="A test page"><title>Title</title></head><body><p>Body</p></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, `charset="utf-8"`, "should preserve charset meta")
	assert.Contains(t, output, "A test page", "should preserve meta description")
	assert.Contains(t, output, "Title", "should preserve title")
	assert.Contains(t, output, "Body", "should preserve body")
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_WithScriptAndStyle(t *testing.T) {
	input := `<html><head><style>body{color:red}</style></head><body><script>var x=1;</script><p>Text</p></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, "body{color:red}", "should preserve style content")
	assert.Contains(t, output, "var x=1;", "should preserve script content")
	assert.Contains(t, output, "Text", "should preserve paragraph")
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_WithComments(t *testing.T) {
	input := `<html><body><!-- nav --><p>Content</p><!-- footer --></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, "<!-- nav -->", "should preserve comments")
	assert.Contains(t, output, "<!-- footer -->", "should preserve comments")
	assert.Contains(t, output, "Content", "should preserve content")
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_NestedBlocks(t *testing.T) {
	input := `<html><body><ul><li>Item 1</li><li>Item 2</li></ul><table><tr><td>Cell</td></tr></table></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, "Item 1")
	assert.Contains(t, output, "Item 2")
	assert.Contains(t, output, "Cell")
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_MixedInlineBlock(t *testing.T) {
	input := `<html><body><p>Text before list:<ul><li>Item 1</li><li>Item 2</li></ul>and after.</p></body></html>`
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_TranslatableAttributes(t *testing.T) {
	input := `<html><body><p title="Tooltip">Text</p><img src="pic.png" alt="Photo"><input type="submit" value="Go" placeholder="Enter"></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, `title="Tooltip"`, "should preserve title attribute")
	assert.Contains(t, output, `alt="Photo"`, "should preserve alt attribute")
	assert.Contains(t, output, `value="Go"`, "should preserve value attribute")
	assert.Contains(t, output, `placeholder="Enter"`, "should preserve placeholder attribute")
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_TranslateNo(t *testing.T) {
	input := `<html><body><p>Translate this</p><p translate="no">Do not translate</p><p>Also translate</p></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, "Translate this")
	assert.Contains(t, output, "Do not translate")
	assert.Contains(t, output, "Also translate")
	assert.Contains(t, output, `translate="no"`)
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_TranslateYes(t *testing.T) {
	input := `<div translate="no"><p>Hidden</p><div translate="yes"><p>Visible</p></div></div>`
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_InlineElements(t *testing.T) {
	input := `<html><body><p>Hello <b>bold</b> and <i>italic</i> world</p></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, "<b>")
	assert.Contains(t, output, "</b>")
	assert.Contains(t, output, "bold")
	assert.Contains(t, output, "italic")
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_PreserveWhitespace(t *testing.T) {
	input := `<html><body><pre>  line 1
  line 2  </pre></body></html>`
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_MetaKeywords(t *testing.T) {
	input := `<html><head><meta name="keywords" content="go, localization, i18n"></head><body><p>Body</p></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, "go, localization, i18n", "should preserve meta keywords")
	assertRoundtripPreserved(t, input)
}

func TestRoundtrip_ComplexDocument(t *testing.T) {
	input := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><meta name="description" content="Test page"><title>Complex</title><style>h1{color:blue}</style></head><body><!-- header --><h1>Heading</h1><p>Paragraph with <a href="#">link</a> and <strong>bold</strong>.</p><ul><li>First</li><li>Second</li></ul><script>console.log("hi")</script><!-- footer --></body></html>`
	output := roundtrip(t, input)
	assert.Contains(t, output, "<!DOCTYPE html>")
	assert.Contains(t, output, `lang="en"`)
	assert.Contains(t, output, "h1{color:blue}")
	assert.Contains(t, output, "Heading")
	assert.Contains(t, output, "link")
	assert.Contains(t, output, "bold")
	assert.Contains(t, output, "First")
	assert.Contains(t, output, "Second")
	assert.Contains(t, output, `console.log("hi")`)
	assert.Contains(t, output, "<!-- header -->")
	assert.Contains(t, output, "<!-- footer -->")
	assertRoundtripPreserved(t, input)
}

// --- Translation Roundtrip Tests ---

func TestRoundtrip_WithTranslation(t *testing.T) {
	input := `<html><body><p>Hello world</p></body></html>`
	ctx := context.Background()
	locale := model.LocaleID("fr")

	// Read and set translations.
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello world" {
				b.Targets[locale] = []*model.Segment{
					{ID: "s1", Content: model.NewFragment("Bonjour le monde")},
				}
			}
		}
	}

	// Write with locale.
	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	writer.SetOriginalContent([]byte(input))
	writer.SetLocale(locale)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour le monde", "should contain translated text")
	assert.NotContains(t, output, "Hello world", "should not contain source text")
}

func TestRoundtrip_TranslateAttribute(t *testing.T) {
	input := `<html><body><p title="Original title">Text</p></body></html>`
	ctx := context.Background()
	locale := model.LocaleID("de")

	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Original title" {
				b.Targets[locale] = []*model.Segment{
					{ID: "s1", Content: model.NewFragment("Originaltitel")},
				}
			}
			if b.SourceText() == "Text" {
				b.Targets[locale] = []*model.Segment{
					{ID: "s1", Content: model.NewFragment("Texte")},
				}
			}
		}
	}

	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	writer.SetOriginalContent([]byte(input))
	writer.SetLocale(locale)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, `title="Originaltitel"`, "should contain translated title")
	assert.Contains(t, output, "Texte", "should contain translated text")
}

func TestRoundtrip_TranslateMetaContent(t *testing.T) {
	input := `<html><head><meta name="description" content="Original description"></head><body><p>Body</p></body></html>`
	ctx := context.Background()
	locale := model.LocaleID("es")

	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Original description" {
				b.Targets[locale] = []*model.Segment{
					{ID: "s1", Content: model.NewFragment("Descripción original")},
				}
			}
		}
	}

	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	writer.SetOriginalContent([]byte(input))
	writer.SetLocale(locale)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, `content="Descripción original"`, "should contain translated meta description")
}

func TestRoundtrip_NonTranslatableUnchanged(t *testing.T) {
	input := `<html><body><script>var x=1;</script><p translate="no">Keep me</p><p>Translate me</p></body></html>`
	ctx := context.Background()
	locale := model.LocaleID("ja")

	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Translate me" {
				b.Targets[locale] = []*model.Segment{
					{ID: "s1", Content: model.NewFragment("翻訳してください")},
				}
			}
		}
	}

	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	writer.SetOriginalContent([]byte(input))
	writer.SetLocale(locale)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "var x=1;", "script content should be unchanged")
	assert.Contains(t, output, "Keep me", "translate=no content should be unchanged")
	assert.Contains(t, output, "翻訳してください", "translatable content should be translated")
}

// --- Fallback Test ---

func TestRoundtrip_FallbackWithoutOriginal(t *testing.T) {
	input := `<html><body><p>Hello world</p></body></html>`
	ctx := context.Background()

	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	// Deliberately NOT setting original content.
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello world", "fallback should output block content")
}

// --- Output Completeness Test ---

func TestRoundtrip_OutputNotEmpty(t *testing.T) {
	input := `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Test Page</title>
<style>body { margin: 0; }</style>
</head>
<body>
<h1>Welcome</h1>
<p>This is a <strong>test</strong> page.</p>
<div>
  <ul>
    <li>Item one</li>
    <li>Item two</li>
  </ul>
</div>
<script>console.log("loaded");</script>
</body>
</html>`

	output := roundtrip(t, input)

	// The output should be a complete HTML document, not just block text.
	assert.True(t, len(output) > 200,
		"output should be a complete HTML document, got %d bytes", len(output))
	assert.True(t, strings.Contains(output, "<html") || strings.Contains(output, "<!DOCTYPE"),
		"output should contain HTML structure")
	assert.Contains(t, output, "Welcome")
	assert.Contains(t, output, "test")
	assert.Contains(t, output, "Item one")
	assert.Contains(t, output, "Item two")
	assert.Contains(t, output, "body { margin: 0; }")
	assert.Contains(t, output, `console.log("loaded")`)
}
