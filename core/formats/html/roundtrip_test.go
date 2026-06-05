package html_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertRoundtripPreserved reads HTML, writes it back with original content,
// re-reads the output, and compares the block texts match.
func assertRoundtripPreserved(t *testing.T, input string) {
	t.Helper()
	ctx := t.Context()

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
	ctx := t.Context()
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
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Bonjour le monde"}}})
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
	ctx := t.Context()
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
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Originaltitel"}}})
			}
			if b.SourceText() == "Text" {
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Texte"}}})
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
	ctx := t.Context()
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
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Descripción original"}}})
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
	ctx := t.Context()
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
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "翻訳してください"}}})
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
	ctx := t.Context()

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

// --- Skeleton Roundtrip Tests ---

// roundtripWithSkeleton performs a read/write roundtrip using the skeleton store.
func roundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := htmlfmt.NewReader()
	writer := htmlfmt.NewWriter()

	// Wire skeleton store.
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	return buf.String()
}

// okapi: RoundTripHtmlIT#htmlFiles
func TestSkeletonRoundtrip_ByteExact(t *testing.T) {
	// Mirror okapi's HtmlFilter: when a <head> exists with no
	// <meta charset> / <meta http-equiv="content-type">, the writer
	// injects a UTF-8 Content-Type meta directly after <head>. The
	// `expected` field captures that injection (and any other reader
	// normalizations like inter-element whitespace trimming) so the
	// byte-exact contract describes the actual emitted output rather
	// than the input.
	cases := []struct {
		name string
		// input is the source HTML.
		input string
		// expected, when non-empty, overrides the byte-exact assertion
		// (input==output). Use this when the reader intentionally
		// normalizes the source — e.g. dropping inter-element
		// whitespace inside translatable block content, or injecting a
		// Content-Type meta after <head> — mirroring okapi.
		expected string
	}{
		{name: "simple_p", input: `<html><body><p>Hello</p></body></html>`},
		{name: "doctype", input: "<!DOCTYPE html>\n<html>\n<body><p>Text</p></body>\n</html>"},
		{name: "single_quotes", input: `<p title='Tip'>Text</p>`},
		{name: "self_closing", input: `<p>Line one<br/>Line two</p>`},
		{name: "nested_blocks", input: `<html><body><ul><li>Item 1</li><li>Item 2</li></ul></body></html>`},
		{
			name:     "script_style",
			input:    `<html><head><style>body{color:red}</style></head><body><script>var x=1;</script><p>Text</p></body></html>`,
			expected: `<html><head>` + injectedContentTypeMeta + `<style>body{color:red}</style></head><body><script>var x=1;</script><p>Text</p></body></html>`,
		},
		{name: "comments", input: `<html><body><!-- nav --><p>Content</p><!-- footer --></body></html>`},
		{name: "meta", input: `<html><head><meta charset="utf-8"><meta name="description" content="A test page"></head><body><p>Body</p></body></html>`},
		// Regression: preserve lang/xml:lang attributes unchanged (#147).
		{name: "lang_preserved", input: `<html lang="en"><body><p>Hello</p></body></html>`},
		{name: "xml_lang_preserved", input: `<html xml:lang="en"><body><p>Hello</p></body></html>`},
		// Regression: preserve charset declarations unchanged (#147).
		{name: "charset_iso8859", input: `<html><head><meta http-equiv="Content-Type" content="text/html; charset=ISO-8859-1"></head><body><p>Text</p></body></html>`},
		// Regression: preserve whitespace in attribute values unchanged (#147).
		{
			name:     "attr_double_space",
			input:    `<html><head><meta name="keywords" content="UFO,  Burlington"></head><body><p>Text</p></body></html>`,
			expected: `<html><head>` + injectedContentTypeMeta + `<meta name="keywords" content="UFO,  Burlington"></head><body><p>Text</p></body></html>`,
		},
		// Inter-element whitespace inside translatable block content is
		// dropped by the reader (mirroring okapi's HtmlFilter — leading/
		// trailing newlines get treated as non-significant source
		// formatting, not part of the translatable text). Parity with
		// okapi requires this trim so translated text joins its sibling
		// tags directly; untranslated round-trip loses the source
		// indentation as a result.
		{name: "block_ws_newlines", input: "<html><body><p>\n  Hello world\n</p></body></html>", expected: "<html><body><p>Hello world</p></body></html>"},
		{name: "block_ws_indented", input: "<html><body><li>\n    Item text\n  </li></body></html>", expected: "<html><body><li>Item text</li></body></html>"},
		// Regression: known container elements roundtrip correctly (#151).
		{name: "table_nested", input: `<html><body><table><tbody><tr><td>Cell</td></tr></tbody></table></body></html>`},
		{name: "ul_nested", input: `<html><body><ul><li>One</li><li>Two</li></ul></body></html>`},
		{name: "dl_nested", input: `<html><body><dl><dt>Term</dt><dd>Definition</dd></dl></body></html>`},
		{name: "select_nested", input: `<html><body><select><option>A</option><option>B</option></select></body></html>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.input
			if tc.expected != "" {
				want = tc.expected
			}
			output := roundtripWithSkeleton(t, tc.input)
			assert.Equal(t, want, output, "skeleton roundtrip output mismatch")
		})
	}
}

const injectedContentTypeMeta = `<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">`

func TestSkeletonRoundtrip_WithTranslation(t *testing.T) {
	input := `<html><body><p>Hello world</p></body></html>`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := htmlfmt.NewReader()
	writer := htmlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello world" {
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Bonjour le monde"}}})
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour le monde")
	assert.NotContains(t, output, "Hello world")
	// Should preserve all non-translatable HTML structure.
	assert.Contains(t, output, "<html><body><p>")
	assert.Contains(t, output, "</p></body></html>")
}

func TestSkeletonRoundtrip_TranslatableAttributes(t *testing.T) {
	input := `<html><body><p title="Tooltip">Text</p><img src="pic.png" alt="Photo"></body></html>`
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := htmlfmt.NewReader()
	writer := htmlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Tooltip":
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Hinweis"}}})
			case "Text":
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Texte"}}})
			case "Photo":
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Foto"}}})
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, `title="Hinweis"`)
	assert.Contains(t, output, "Texte")
	assert.Contains(t, output, `alt="Foto"`)
}

// --- Lang Attribute Rewriting Tests (#147) ---

func TestSkeletonRoundtrip_LangRewrittenToTargetLocale(t *testing.T) {
	input := `<html lang="en"><body><p>Hello world</p></body></html>`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := htmlfmt.NewReader()
	writer := htmlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello world" {
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Bonjour le monde"}}})
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour le monde", "should contain translated text")
	assert.Contains(t, output, `lang="fr"`, "lang should be rewritten to target locale")
	assert.NotContains(t, output, `lang="en"`, "source locale should be replaced")
}

func TestSkeletonRoundtrip_LangPreservedWithoutTargetLocale(t *testing.T) {
	// Without a target locale, lang attributes should be preserved as-is.
	input := `<html lang="en"><body><p>Hello</p></body></html>`
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "skeleton roundtrip without target locale should be byte-exact")
}

func TestSkeletonRoundtrip_LangUnrelatedLocalePreserved(t *testing.T) {
	// lang="de" should NOT be rewritten when source is "en" and target is "fr".
	input := `<html lang="en"><body><p lang="de">German</p><p>English</p></body></html>`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := htmlfmt.NewReader()
	writer := htmlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer.SetLocale(locale)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, `lang="fr"`, "source locale 'en' should be rewritten to 'fr'")
	assert.Contains(t, output, `lang="de"`, "unrelated locale 'de' should be preserved")
}

// skeletonRoundtripRetargeted reads input via the skeleton path with the
// given source locale, then writes it back targeting target. No blocks are
// translated; the test focuses on language-declaration retargeting in the
// skeleton (lang/xml:lang/meta-content-language) entries.
func skeletonRoundtripRetargeted(t *testing.T, input string, source, target model.LocaleID) string {
	t.Helper()
	ctx := t.Context()

	reader := htmlfmt.NewReader()
	writer := htmlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, source))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer.SetLocale(target)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

// TestSkeletonRoundtrip_BothLangAndXMLLangRetargeted is the regression guard
// for the W3CHTMHLTest1 parity divergence: an XHTML root element carrying BOTH
// xml:lang and lang (HTML5 §3.2.6.1 allows both with the same value) must have
// BOTH declarations retargeted on translation. Previously only the bare lang
// was spliced/retargeted, leaving a stale xml:lang="en" beside lang="fr".
// Okapi normalizes both to Property.LANGUAGE (HtmlFilter.normalizeAttributeName)
// and GenericSkeletonWriter retargets every language property to the output
// locale.
func TestSkeletonRoundtrip_BothLangAndXMLLangRetargeted(t *testing.T) {
	input := `<html xml:lang="en" lang="en"><body><p>Hello</p></body></html>`
	output := skeletonRoundtripRetargeted(t, input, model.LocaleEnglish, model.LocaleFrench)

	assert.Contains(t, output, `xml:lang="fr"`, "xml:lang must be retargeted")
	assert.Contains(t, output, `lang="fr"`, "lang must be retargeted")
	assert.NotContains(t, output, `lang="en"`,
		"no source-locale language declaration should remain (covers both lang and xml:lang)")
}

// TestSkeletonRoundtrip_LangRegionInsensitiveMatch guards the language-only
// (region/script-insensitive) comparison that mirrors Okapi's
// LocaleId.sameLanguageAs: a document-language declaration of "en-US" in an
// en→fr roundtrip is the document's own language and is retargeted, while a
// foreign-language inline declaration ("ja") is left untouched.
func TestSkeletonRoundtrip_LangRegionInsensitiveMatch(t *testing.T) {
	input := `<html xml:lang="en-US" lang="en-US"><body>` +
		`<p>English</p><span lang="ja">日本語</span></body></html>`
	output := skeletonRoundtripRetargeted(t, input, model.LocaleEnglish, model.LocaleFrench)

	assert.Contains(t, output, `xml:lang="fr"`,
		"xml:lang=en-US is the document language and must be retargeted to fr")
	assert.Contains(t, output, `lang="fr"`,
		"lang=en-US is the document language and must be retargeted to fr")
	assert.NotContains(t, output, `en-US`, "no source-language declaration should remain")
	assert.Contains(t, output, `lang="ja"`,
		"foreign-language inline declaration must be preserved")
}

// TestSkeletonRoundtrip_MetaContentLanguageRetargeted guards retargeting of the
// <meta http-equiv="Content-Language" content="…"> declaration, which Okapi
// also normalizes to Property.LANGUAGE and retargets to the output locale.
func TestSkeletonRoundtrip_MetaContentLanguageRetargeted(t *testing.T) {
	input := `<html><head>` +
		`<meta http-equiv="Content-Language" content="en"/>` +
		`</head><body><p>Hello</p></body></html>`
	output := skeletonRoundtripRetargeted(t, input, model.LocaleEnglish, model.LocaleFrench)

	assert.Contains(t, output, `content="fr"`,
		"meta Content-Language must be retargeted to the output locale")
	assert.NotContains(t, output, `content="en"`, "source locale should be replaced")
}

// TestReparseRoundtrip_LangRewrittenStructurally guards the #604 change: the
// re-parse (DOM) writer path rewrites lang/xml:lang to the target locale by
// setting the attribute on the html.Node tree before html.Render, not via a
// post-serialization regex. Covers the root <html> element, a nested element,
// and the xml:lang variant, plus an unrelated locale that must be preserved.
func TestReparseRoundtrip_LangRewrittenStructurally(t *testing.T) {
	input := `<html lang="en-US"><body>` +
		`<p lang="en-US">Hello world</p>` +
		`<div xml:lang="en-US">Nested</div>` +
		`<p lang="de">German</p>` +
		`</body></html>`
	ctx := t.Context()
	source := model.LocaleID("en-US")
	target := model.LocaleID("fr-FR")

	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, source))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Re-parse (DOM) mode: original content set, no skeleton store.
	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	writer.SetOriginalContent([]byte(input))
	writer.SetLocale(target)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, `lang="fr-FR"`,
		"root and nested lang should be retargeted on the DOM")
	assert.Contains(t, output, `xml:lang="fr-FR"`,
		"xml:lang should be retargeted on the DOM")
	assert.NotContains(t, output, `lang="en-US"`,
		"source locale should be fully replaced (root + nested)")
	assert.NotContains(t, output, `xml:lang="en-US"`,
		"source locale should be fully replaced on xml:lang")
	assert.Contains(t, output, `lang="de"`,
		"unrelated locale should be preserved")
}

// --- Buffer Exhaustion Regression Test (#151) ---

func TestSkeletonRoundtrip_LargeElementBeforeContainer(t *testing.T) {
	// Regression test for #151: a large <td> exhausts the tokenizer buffer,
	// causing the subsequent <table> to be misclassified as a leaf block.
	// The fix: known container elements (table, ul, etc.) skip forward scan.
	largeContent := strings.Repeat("x", 32*1024) // 32KB to exhaust tokenizer buffer
	input := `<html><body><table><tr><td>` + largeContent + `</td></tr></table>` +
		`<div><table><tbody><tr><td>After</td></tr></tbody></table></div></body></html>`

	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "skeleton roundtrip should be byte-exact after large element")
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
	assert.Greater(t, len(output), 200,
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
