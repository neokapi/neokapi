package markdown_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helpers ---

func hasInlineCodeRun(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

func runHasCodeType(runs []model.Run, codeType string) bool {
	for _, r := range runs {
		switch {
		case r.PcOpen != nil && r.PcOpen.Type == codeType:
			return true
		case r.PcClose != nil && r.PcClose.Type == codeType:
			return true
		case r.Ph != nil && r.Ph.Type == codeType:
			return true
		}
	}
	return false
}

// readBlocks reads markdown input and returns the extracted blocks.
func readBlocks(t *testing.T, input string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}

// readParts reads markdown input and returns all parts.
func readParts(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// readBlocksWithConfig reads markdown input with a custom config modifier.
func readBlocksWithConfig(t *testing.T, input string, configure func(*markdown.Config)) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := markdown.NewReader()
	configure(reader.MarkdownConfig())
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}

// readPartsWithConfig reads markdown input with a custom config modifier.
func readPartsWithConfig(t *testing.T, input string, configure func(*markdown.Config)) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := markdown.NewReader()
	configure(reader.MarkdownConfig())
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// roundtrip reads then writes markdown, returning the output.
func roundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := markdown.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	return buf.String()
}

// roundtripWithSkeletonConfig reads then writes using a skeleton store with custom config.
func roundtripWithSkeletonConfig(t *testing.T, input string, configure func(*markdown.Config)) string {
	t.Helper()
	ctx := t.Context()

	reader := markdown.NewReader()
	configure(reader.MarkdownConfig())
	writer := markdown.NewWriter()

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

// roundtripWithSkeleton reads then writes using a skeleton store.
func roundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := markdown.NewReader()
	writer := markdown.NewWriter()

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

// --- Basic Tests ---

func TestReadSimpleParagraphs(t *testing.T) {
	blocks := readBlocks(t, "Hello world\n\nSecond paragraph")
	assert.Len(t, blocks, 2)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Second paragraph", blocks[1].SourceText())
}

// okapi: MarkdownFilterTest#testHeadingPrefix
func TestReadHeadings(t *testing.T) {
	blocks := readBlocks(t, "# Title\n\n## Subtitle\n\nText")
	require.Len(t, blocks, 3)
	assert.Equal(t, "Title", blocks[0].SourceText())
	assert.Equal(t, "heading", blocks[0].Type)
	assert.Equal(t, "1", blocks[0].Properties["level"])
	assert.Equal(t, "Subtitle", blocks[1].SourceText())
	assert.Equal(t, "heading", blocks[1].Type)
	assert.Equal(t, "2", blocks[1].Properties["level"])
	assert.Equal(t, "Text", blocks[2].SourceText())
}

// okapi: MarkdownFilterTest#testEmphasisAndStrong
func TestReadBoldItalicInline(t *testing.T) {
	blocks := readBlocks(t, "This has **bold** and *italic* text")
	require.Len(t, blocks, 1)
	assert.Equal(t, "This has bold and italic text", blocks[0].SourceText())

	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
}

// okapi: MarkdownFilterTest#testDontTranslateFencedCodeBlocks
// With non-translatable-content surfacing disabled (the Okapi-faithful config),
// a fenced code block stays opaque Data.
func TestReadCodeBlockAsData(t *testing.T) {
	input := "# Title\n\n```go\nfmt.Println(\"hello\")\n```\n\nText after code"
	parts := readPartsWithConfig(t, input, func(c *markdown.Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	blocks := testutil.FilterBlocks(parts)

	assert.Len(t, blocks, 2)
	assert.Equal(t, "Title", blocks[0].SourceText())
	assert.Equal(t, "Text after code", blocks[1].SourceText())

	hasCodeData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "code-block" {
				hasCodeData = true
				assert.Equal(t, "go", data.Properties["language"])
			}
		}
	}
	assert.True(t, hasCodeData, "expected code block as Data")
}

// By default (ExtractNonTranslatableContent on), a fenced code block is surfaced
// as a non-translatable RoleCode content block — visible to ingestion, skipped
// by MT — instead of opaque skeleton, and the document still round-trips.
func TestReadFencedCodeBlockAsContent(t *testing.T) {
	input := "# Title\n\n```go\nfmt.Println(\"hello\")\n```\n\nText after code"
	parts := readParts(t, input)

	var translatable, content []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.Translatable {
				translatable = append(translatable, b)
			} else {
				content = append(content, b)
			}
		}
	}
	assert.Len(t, translatable, 2, "Title + trailing text stay translatable")
	require.Len(t, content, 1, "code surfaces as one non-translatable content block")
	code := content[0]
	assert.False(t, code.Translatable)
	assert.Equal(t, model.RoleCode, code.SemanticRole())
	assert.True(t, code.PreserveWhitespace)
	assert.Contains(t, code.SourceText(), `fmt.Println("hello")`)
	assert.Equal(t, "go", code.Properties["language"])

	assert.Equal(t, input, roundtripWithSkeleton(t, input), "round-trip stays byte-exact")
}

// okapi: MarkdownFilterTest#testBulletList
func TestReadLists(t *testing.T) {
	blocks := readBlocks(t, "- First item\n- Second item\n- Third item")
	require.Len(t, blocks, 3)
	assert.Equal(t, "First item", blocks[0].SourceText())
	assert.Equal(t, "Second item", blocks[1].SourceText())
	assert.Equal(t, "Third item", blocks[2].SourceText())
	assert.Equal(t, "list-item", blocks[0].Type)
}

func TestReadLayerStartEnd(t *testing.T) {
	parts := readParts(t, "Hello")
	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "markdown", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := markdown.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/markdown")
	assert.Contains(t, sig.Extensions, ".md")
}

func TestReaderMetadata(t *testing.T) {
	reader := markdown.NewReader()
	assert.Equal(t, "markdown", reader.Name())
	assert.Equal(t, "Markdown", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := markdown.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

// okapi: MarkdownFilterTest#testEventsFromEmptyInput
func TestReadEmpty(t *testing.T) {
	parts := readParts(t, "")
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

// --- Roundtrip Tests ---

func TestRoundTrip(t *testing.T) {
	input := "# Hello\n\nThis is text\n\n- Item one\n- Item two"
	output := roundtrip(t, input)
	assert.Contains(t, output, "# Hello")
	assert.Contains(t, output, "This is text")
	assert.Contains(t, output, "- Item one")
	assert.Contains(t, output, "- Item two")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("# Hello\n\nWorld", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			} else if block.SourceText() == "World" {
				block.SetTargetText(model.LocaleFrench, "Monde")
			}
		}
	}

	var buf bytes.Buffer
	writer := markdown.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "# Bonjour")
	assert.Contains(t, output, "Monde")
	assert.NotContains(t, output, "Hello")
	assert.NotContains(t, output, "World")
}

// okapi: MarkdownFilterTest#testHtmlBlockWithMarkdown
//
// HTML blocks are routed through an in-process HTML subfilter (mirrors
// okapi MarkdownFilter.processByHtmlFilter / okf_html@for_markdown.fprm).
// Tag bytes pass through as skeleton text; text content between tags
// becomes a translatable Block.
func TestReadHTMLBlockSubfilter(t *testing.T) {
	input := "Text before\n\n<div>HTML content</div>\n\nText after"
	parts := readParts(t, input)

	hasHTMLContentBlock := false
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.Type == "html-text" && len(block.Source) > 0 && block.SourceText() == "HTML content" {
				hasHTMLContentBlock = true
			}
		}
	}
	assert.True(t, hasHTMLContentBlock, "expected HTML block text content extracted as a translatable Block")
}

// By default (ExtractNonTranslatableContent on), the MathML body of an HTML-block
// <math> element surfaces as a non-translatable RoleFormula content block — the
// <math>…</math> delimiters stay skeleton — and the document round-trips
// byte-exact. The okapi-faithful config (flag off) keeps the body opaque.
func TestReadHTMLBlockMathFormula(t *testing.T) {
	input := "Text before\n\n<div>\n<math><mi>x</mi><mo>=</mo><mn>1</mn></math>\n</div>\n\nText after\n"

	t.Run("surfaced_by_default", func(t *testing.T) {
		parts := readParts(t, input)

		var math *model.Block
		var translatableTexts []string
		for _, p := range parts {
			if p.Type != model.PartBlock {
				continue
			}
			b := p.Resource.(*model.Block)
			if b.SemanticRole() == model.RoleFormula {
				math = b
			} else if b.Translatable {
				translatableTexts = append(translatableTexts, b.SourceText())
			}
		}
		require.NotNil(t, math, "math body should surface as a content block")
		assert.False(t, math.Translatable)
		assert.Equal(t, model.RoleFormula, math.SemanticRole())
		assert.True(t, math.PreserveWhitespace)
		assert.Equal(t, "<mi>x</mi><mo>=</mo><mn>1</mn>", math.SourceText(),
			"verbatim MathML body, no inline parse")
		require.Len(t, math.SourceRuns(), 1, "single verbatim run")
		// The surrounding prose is still translatable; the math text is not in it.
		assert.Contains(t, translatableTexts, "Text before")
		assert.Contains(t, translatableTexts, "Text after")

		assert.Equal(t, input, roundtripWithSkeleton(t, input), "round-trip stays byte-exact")
	})

	t.Run("opaque_when_flag_off", func(t *testing.T) {
		parts := readPartsWithConfig(t, input, func(c *markdown.Config) {
			c.SetExtractNonTranslatableContent(false)
		})
		for _, p := range parts {
			if p.Type == model.PartBlock {
				b := p.Resource.(*model.Block)
				assert.NotEqual(t, model.RoleFormula, b.SemanticRole(),
					"no formula block when extraction is off")
			}
		}
		output := roundtripWithSkeletonConfig(t, input, func(c *markdown.Config) {
			c.SetExtractNonTranslatableContent(false)
		})
		assert.Equal(t, input, output, "round-trip stays byte-exact with flag off")
	})
}

// --- Skeleton Store Roundtrip Tests ---

func TestSkeletonRoundtrip_ByteExact(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"simple_para", "Hello world\n"},
		{"two_paragraphs", "First paragraph\n\nSecond paragraph\n"},
		{"heading_and_text", "# Title\n\nSome text\n"},
		{"multiple_headings", "# H1\n\n## H2\n\n### H3\n"},
		{"bullet_list", "- Item one\n- Item two\n- Item three\n"},
		{"fenced_code", "Text before\n\n```go\nfmt.Println()\n```\n\nText after\n"},
		{"thematic_break", "Above\n\n---\n\nBelow\n"},
		{"bold_italic", "This has **bold** and *italic* text\n"},
		{"inline_code", "Use `fmt.Println()` here\n"},
		{"link", "Click [here](https://example.com) please\n"},
		{"html_block", "Text\n\n<div>HTML</div>\n\nMore text\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := roundtripWithSkeleton(t, tc.input)
			assert.Equal(t, tc.input, output, "skeleton roundtrip should be byte-exact")
		})
	}
}

func TestSkeletonRoundtrip_WithTranslation(t *testing.T) {
	input := "# Hello\n\nWorld\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := markdown.NewReader()
	writer := markdown.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Translate blocks
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			switch block.SourceText() {
			case "Hello":
				block.SetTargetText(locale, "Bonjour")
			case "World":
				block.SetTargetText(locale, "Monde")
			}
		}
	}

	var buf bytes.Buffer
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(locale)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Equal(t, "# Bonjour\n\nMonde\n", output)
}

// --- Emphasis Tests ---

// okapi: MarkdownFilterTest#testEmphasis
func TestRead_Emphasis(t *testing.T) {
	blocks := readBlocks(t, "This is *emphasized* text")
	require.Len(t, blocks, 1)
	assert.Equal(t, "This is emphasized text", blocks[0].SourceText())
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
	// Should have opening and closing inline-code runs for italic
	assert.True(t, runHasCodeType(runs, "fmt:italic"), "should have italic inline code")
}

// okapi: MarkdownFilterTest#testEmphasisAcrossLines
func TestRead_EmphasisAcrossLines(t *testing.T) {
	blocks := readBlocks(t, "This *spans\nacross* lines")
	require.Len(t, blocks, 1)
	// The literal LF between source lines is preserved in the block's
	// text (mirroring okapi MarkdownFilter, which keeps soft line breaks
	// in TextUnit content rather than collapsing them to spaces). The
	// writer re-emits the per-line continuation prefix (blockquote `> `,
	// list/indent) from BlockPropLinePrefix so soft-break bodies round-
	// trip byte-for-byte.
	assert.Equal(t, "This spans\nacross lines", blocks[0].SourceText())
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
}

// okapi: MarkdownFilterTest#testEmphasisAtParaStart
func TestRead_EmphasisAtParaStart(t *testing.T) {
	blocks := readBlocks(t, "*Bold start* then normal")
	require.Len(t, blocks, 1)
	assert.Equal(t, "Bold start then normal", blocks[0].SourceText())
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
}

// --- Code Tests ---

// okapi: MarkdownFilterTest#testCode
func TestRead_Code(t *testing.T) {
	blocks := readBlocks(t, "Use the `code` function")
	require.Len(t, blocks, 1)
	assert.Equal(t, "Use the code function", blocks[0].SourceText())
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
	assert.True(t, runHasCodeType(runs, "fmt:code"), "should have code inline run")
}

// okapi: MarkdownFilterTest#testCodeAndEmphasis
func TestRead_CodeAndEmphasis(t *testing.T) {
	blocks := readBlocks(t, "Use `code` and *emphasis* here")
	require.Len(t, blocks, 1)
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
	assert.True(t, runHasCodeType(runs, "fmt:code"), "should have code inline run")
	assert.True(t, runHasCodeType(runs, "fmt:italic"), "should have italic inline run")
}

// --- Fenced Code Block Tests ---

// okapi: MarkdownFilterTest#testFencedCodeBlock
func TestRead_FencedCodeBlock(t *testing.T) {
	input := "```\ncode here\n```\n"
	// Okapi-faithful config: code stays opaque Data.
	parts := readPartsWithConfig(t, input, func(c *markdown.Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	hasCode := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "code-block" {
				hasCode = true
				assert.Contains(t, data.Properties["content"], "code here")
			}
		}
	}
	assert.True(t, hasCode, "expected code block as Data")
}

// okapi: MarkdownFilterTest#testTranslateFencedCodeBlocks
func TestRead_TranslateFencedCodeBlocks(t *testing.T) {
	input := "```\ntranslatable code\n```\n"
	blocks := readBlocksWithConfig(t, input, func(c *markdown.Config) {
		c.TranslateCodeBlocks = true
	})
	require.Len(t, blocks, 1)
	assert.Equal(t, "code-block", blocks[0].Type)
	assert.Contains(t, blocks[0].SourceText(), "translatable code")
}

// okapi: MarkdownFilterTest#testExcludeIndentedCodeBlock
// Okapi-faithful config (surfacing off): indented code stays Data, not a block.
func TestRead_ExcludeIndentedCodeBlock(t *testing.T) {
	input := "Text\n\n    indented code\n\nMore text\n"
	parts := readPartsWithConfig(t, input, func(c *markdown.Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	blocks := testutil.FilterBlocks(parts)
	assert.Len(t, blocks, 2)
	assert.Equal(t, "Text", blocks[0].SourceText())
	assert.Equal(t, "More text", blocks[1].SourceText())
}

// okapi: MarkdownFilterTest#testIndentedCodeBlock
func TestRead_IndentedCodeBlock(t *testing.T) {
	input := "    indented code line 1\n    indented code line 2\n"
	// Okapi-faithful config: indented code stays Data.
	parts := readPartsWithConfig(t, input, func(c *markdown.Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	hasCode := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "code-block" {
				hasCode = true
			}
		}
	}
	assert.True(t, hasCode, "indented code should be Data")
}

// By default, indented code surfaces as a non-translatable RoleCode content
// block and the document still round-trips byte-exact.
func TestRead_IndentedCodeBlockAsContent(t *testing.T) {
	input := "Text\n\n    indented code\n\nMore text\n"
	parts := readParts(t, input)
	var content []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b := p.Resource.(*model.Block); !b.Translatable {
				content = append(content, b)
			}
		}
	}
	require.Len(t, content, 1, "indented code surfaces as one non-translatable content block")
	assert.Equal(t, model.RoleCode, content[0].SemanticRole())
	assert.Contains(t, content[0].SourceText(), "indented code")
	assert.Equal(t, input, roundtripWithSkeleton(t, input), "round-trip stays byte-exact")
}

// --- Link Tests ---

// okapi: MarkdownFilterTest#testLink
func TestRead_Link(t *testing.T) {
	blocks := readBlocks(t, "Click [here](https://example.com) please")
	require.Len(t, blocks, 1)
	assert.Equal(t, "Click here please", blocks[0].SourceText())
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
	assert.True(t, runHasCodeType(runs, "link:hyperlink"), "should have hyperlink inline run")
}

// okapi: MarkdownFilterTest#testLinkWithTitle
func TestRead_LinkWithTitle(t *testing.T) {
	blocks := readBlocks(t, `Click [here](https://example.com "Example") please`)
	require.Len(t, blocks, 1)
	// Title is extracted as a translatable text run between two paired
	// codes (md:link wrapping the link text, md:link-title wrapping the
	// title). SourceText concatenates only TextRuns, so the title text
	// "Example" appears adjacent to the link text "here".
	assert.Equal(t, "Click hereExample please", blocks[0].SourceText())
	runs := blocks[0].SourceRuns()
	// Verify the title is emitted as a translatable text run inside an
	// md:link-title paired code (not baked into the closing skeleton).
	var sawTitle bool
	for i, r := range runs {
		if r.PcOpen != nil && r.PcOpen.SubType == "md:link-title" {
			require.Less(t, i+1, len(runs), "md:link-title pc-open must be followed by content")
			require.NotNil(t, runs[i+1].Text, "md:link-title content must be a TextRun")
			assert.Equal(t, "Example", runs[i+1].Text.Text)
			sawTitle = true
		}
	}
	assert.True(t, sawTitle, "expected an md:link-title paired code wrapping the link title")
}

// okapi: MarkdownFilterTest#testAutoLink
func TestRead_AutoLink(t *testing.T) {
	blocks := readBlocks(t, "Visit <https://example.com> now")
	require.Len(t, blocks, 1)
	// The autolink itself is an opaque placeholder (not translatable
	// text), but its source data must carry the full `<url>` so the
	// writer can re-emit it verbatim. Check the placeholder data.
	runs := blocks[0].SourceRuns()
	var sawAutolink bool
	for _, r := range runs {
		if r.Ph != nil && r.Ph.SubType == "md:autolink" {
			assert.Contains(t, r.Ph.Data, "https://example.com",
				"autolink placeholder data must carry the full <url>")
			sawAutolink = true
		}
	}
	assert.True(t, sawAutolink, "expected an md:autolink placeholder run")
}

// --- Image Tests ---

// okapi: MarkdownFilterTest#testImage
func TestRead_Image(t *testing.T) {
	blocks := readBlocks(t, "See ![alt text](image.png) here")
	require.Len(t, blocks, 1)
	assert.Equal(t, "See alt text here", blocks[0].SourceText())
	runs := blocks[0].SourceRuns()
	assert.True(t, runHasCodeType(runs, "link:image"), "should have image inline run")
}

// okapi: MarkdownFilterTest#testExtractImageTitleAndAltText
func TestRead_ExtractImageTitleAndAltText(t *testing.T) {
	blocks := readBlocks(t, `![alt](image.png "title")`)
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].SourceText(), "alt")
	// Title is now extracted as a translatable text run inside an
	// md:image-title paired code, not part of the closing skeleton —
	// matches okapi MarkdownFilter behaviour.
	assert.Contains(t, blocks[0].SourceText(), "title")
	runs := blocks[0].SourceRuns()
	var sawTitle bool
	for i, r := range runs {
		if r.PcOpen != nil && r.PcOpen.SubType == "md:image-title" {
			require.Less(t, i+1, len(runs), "md:image-title pc-open must be followed by content")
			require.NotNil(t, runs[i+1].Text, "md:image-title content must be a TextRun")
			assert.Equal(t, "title", runs[i+1].Text.Text)
			sawTitle = true
		}
	}
	assert.True(t, sawTitle, "expected an md:image-title paired code wrapping the image title")
}

// --- Heading Variations ---

// okapi: MarkdownFilterTest#testHeadingUnderline
func TestRead_HeadingUnderline(t *testing.T) {
	blocks := readBlocks(t, "Title\n=====\n\nSubtitle\n--------\n")
	require.Len(t, blocks, 2)
	assert.Equal(t, "heading", blocks[0].Type)
	assert.Equal(t, "1", blocks[0].Properties["level"])
	assert.Equal(t, "Title", blocks[0].SourceText())
	assert.Equal(t, "heading", blocks[1].Type)
	assert.Equal(t, "2", blocks[1].Properties["level"])
	assert.Equal(t, "Subtitle", blocks[1].SourceText())
}

// --- Thematic Break Tests ---

// okapi: MarkdownFilterTest#testThematicBreak
func TestRead_ThematicBreak(t *testing.T) {
	input := "Above\n\n---\n\nBelow\n"
	parts := readParts(t, input)
	blocks := testutil.FilterBlocks(parts)
	assert.Len(t, blocks, 2)

	hasBreak := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "thematic-break" {
				hasBreak = true
			}
		}
	}
	assert.True(t, hasBreak, "expected thematic break as Data")
}

// --- Blockquote Tests ---

// okapi: MarkdownFilterTest#testBlockQuoteEvents
func TestRead_BlockQuoteEvents(t *testing.T) {
	blocks := readBlocks(t, "> Quoted text\n>\n> More quoted\n")
	assert.GreaterOrEqual(t, len(blocks), 1)
	// Default: blockquotes are translatable
	foundQuoted := false
	for _, b := range blocks {
		if b.SourceText() == "Quoted text" || b.SourceText() == "More quoted" {
			foundQuoted = true
		}
	}
	assert.True(t, foundQuoted, "should extract blockquote text")
}

// okapi: MarkdownFilterTest#testNonTranslatableBlockQuotes
// Okapi-faithful config: with extract-non-translatable-content disabled the
// blockquote collapses to opaque Data and no blocks surface at all.
func TestRead_NonTranslatableBlockQuotes(t *testing.T) {
	input := "> Quoted text\n"
	parts := readPartsWithConfig(t, input, func(c *markdown.Config) {
		_ = c.ApplyMap(map[string]any{"translateBlockQuotes": false})
		c.SetExtractNonTranslatableContent(false)
	})
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "blockquote content should not be translatable")
	assert.True(t, hasDataNamed(parts, "blockquote"), "blockquote should be opaque Data")
}

// With translateBlockQuotes off but ExtractNonTranslatableContent on (the
// default), the blockquote body surfaces as non-translatable content blocks —
// visible to ingestion, skipped by MT — instead of opaque Data, and still
// round-trips byte-exact.
func TestRead_NonTranslatableBlockQuotesAsContent(t *testing.T) {
	input := "> Quoted text\n>\n> More quoted\n"
	parts := readPartsWithConfig(t, input, func(c *markdown.Config) {
		_ = c.ApplyMap(map[string]any{"translateBlockQuotes": false})
	})

	var translatable, content []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.Translatable {
				translatable = append(translatable, b)
			} else {
				content = append(content, b)
			}
		}
	}
	assert.Empty(t, translatable, "blockquote content is never translatable when off")
	require.Len(t, content, 2, "one non-translatable block per quoted paragraph")
	assert.Equal(t, "Quoted text", content[0].SourceText())
	assert.Equal(t, "More quoted", content[1].SourceText())
	assert.False(t, hasDataNamed(parts, "blockquote"), "no opaque blockquote Data on the content path")

	output := roundtripWithSkeletonConfig(t, input, func(c *markdown.Config) {
		_ = c.ApplyMap(map[string]any{"translateBlockQuotes": false})
	})
	assert.Equal(t, input, output, "round-trip stays byte-exact")
}

// --- Front Matter Tests ---

// okapi: MarkdownFilterTest#testDontTranslateMetadataHeader
// Okapi-faithful config: with extract-non-translatable-content disabled the
// whole front matter stays opaque Data and only the body yields blocks.
func TestRead_DontTranslateMetadataHeader(t *testing.T) {
	input := "---\ntitle: My Title\nauthor: John\n---\n\n# Heading\n"
	parts := readPartsWithConfig(t, input, func(c *markdown.Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	blocks := testutil.FilterBlocks(parts)

	// Front matter should be Data by default
	hasFrontMatter := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "front-matter" {
				hasFrontMatter = true
			}
		}
	}
	assert.True(t, hasFrontMatter, "front matter should be Data by default")
	assert.Len(t, blocks, 1, "only heading should be a block")
	assert.Equal(t, "Heading", blocks[0].SourceText())
}

// By default (ExtractNonTranslatableContent on, TranslateFrontMatter off), the
// known prose scalars (title/description/summary) surface as non-translatable
// content blocks while non-prose keys (author, date, slug) stay skeleton. The
// translatable payload is unchanged (only the heading) and the round-trip is
// byte-exact.
func TestRead_FrontMatterProseAsContent(t *testing.T) {
	input := "---\ntitle: My Title\ndescription: A summary line\nauthor: John\nslug: my-post\n---\n\n# Heading\n"
	parts := readParts(t, input)

	var translatable, content []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.Translatable {
				translatable = append(translatable, b)
			} else {
				content = append(content, b)
			}
		}
	}
	require.Len(t, translatable, 1, "only the heading stays translatable")
	assert.Equal(t, "Heading", translatable[0].SourceText())

	require.Len(t, content, 2, "title + description surface as non-translatable content")
	assert.Equal(t, "My Title", content[0].SourceText())
	assert.Equal(t, "front-matter", content[0].Type)
	assert.Equal(t, "title", content[0].Properties["key"])
	assert.Equal(t, "A summary line", content[1].SourceText())
	assert.Equal(t, "description", content[1].Properties["key"])

	// author/slug never become blocks; they ride the skeleton.
	for _, b := range content {
		assert.NotContains(t, []string{"John", "my-post"}, b.SourceText())
	}
	// No opaque front-matter Data part on the content path.
	assert.False(t, hasDataNamed(parts, "front-matter"))

	assert.Equal(t, input, roundtripWithSkeleton(t, input), "round-trip stays byte-exact")
}

// okapi: MarkdownFilterTest#testTranslateMetadataHeader
func TestRead_TranslateMetadataHeader(t *testing.T) {
	input := "---\ntitle: My Title\nauthor: John\n---\n\n# Heading\n"
	blocks := readBlocksWithConfig(t, input, func(c *markdown.Config) {
		c.TranslateFrontMatter = true
	})

	// Should have front matter values as blocks + heading
	assert.GreaterOrEqual(t, len(blocks), 3)
	found := false
	for _, b := range blocks {
		if b.SourceText() == "My Title" {
			found = true
			assert.Equal(t, "front-matter", b.Type)
		}
	}
	assert.True(t, found, "front matter title should be translatable")
}

// --- Table Tests (GFM) ---

// okapi: MarkdownFilterTest#testTable1TextUnits
func TestRead_Table1TextUnits(t *testing.T) {
	input := "| Header 1 | Header 2 |\n| --- | --- |\n| Cell 1 | Cell 2 |\n"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 4)
	texts := make([]string, len(blocks))
	for i, b := range blocks {
		texts[i] = b.SourceText()
	}
	assert.Contains(t, texts, "Header 1")
	assert.Contains(t, texts, "Header 2")
	assert.Contains(t, texts, "Cell 1")
	assert.Contains(t, texts, "Cell 2")
}

// okapi: MarkdownFilterTest#testTable2TextUnits
func TestRead_Table2TextUnits(t *testing.T) {
	input := "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |\n"
	blocks := readBlocks(t, input)
	assert.Len(t, blocks, 9, "3 headers + 6 cells")
}

// --- Strikethrough Tests (GFM) ---

// okapi: MarkdownFilterTest#testStrikethroughSubscript
func TestRead_StrikethroughSubscript(t *testing.T) {
	blocks := readBlocks(t, "This is ~~deleted~~ text")
	require.Len(t, blocks, 1)
	assert.Equal(t, "This is deleted text", blocks[0].SourceText())
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
	assert.True(t, runHasCodeType(runs, "fmt:strike"), "should have strikethrough inline run")
}

// --- Hard Line Break Tests ---

// okapi: MarkdownFilterTest#testHardLineBreak
func TestRead_HardLineBreak(t *testing.T) {
	// Two trailing spaces create a hard line break
	input := "Line one  \nLine two\n"
	blocks := readBlocks(t, input)
	require.Len(t, blocks, 1)
	// Hard line break should be preserved as \n in text
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line one")
	assert.Contains(t, text, "Line two")
}

// --- CRLF Tests ---

// okapi: MarkdownFilterTest#testCRLF
func TestRead_CRLF(t *testing.T) {
	blocks := readBlocks(t, "# Title\r\n\r\nParagraph\r\n")
	require.Len(t, blocks, 2)
	assert.Equal(t, "Title", blocks[0].SourceText())
	assert.Equal(t, "Paragraph", blocks[1].SourceText())
}

// --- Close Without Input ---

// okapi: MarkdownFilterTest#testCloseWithoutInput
func TestRead_CloseWithoutInput(t *testing.T) {
	reader := markdown.NewReader()
	err := reader.Close()
	require.NoError(t, err)
}

// --- HTML Inline ---

// okapi: MarkdownFilterTest#testHtmlInline
func TestRead_HtmlInline(t *testing.T) {
	blocks := readBlocks(t, "Text with <b>bold</b> HTML")
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].SourceText(), "bold")
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs), "inline HTML should produce inline-code runs")
}

// --- HTML Entities ---

// okapi: MarkdownFilterTest#testHtmlEntities
func TestRead_HtmlEntities(t *testing.T) {
	blocks := readBlocks(t, "This &amp; that")
	require.Len(t, blocks, 1)
	// goldmark may decode entities or preserve them
	assert.Contains(t, blocks[0].SourceText(), "that")
}

// --- Nested Lists ---

// okapi: MarkdownFilterTest#testNestedBulletWithFencedCodeBlock
func TestRead_NestedBulletWithFencedCodeBlock(t *testing.T) {
	input := "- Item 1\n- Item 2\n\n  ```\n  code\n  ```\n\n- Item 3\n"
	parts := readParts(t, input)
	blocks := testutil.FilterBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "should have at least two text blocks")
}

// --- Backslash Escapes ---

// okapi: MarkdownFilterTest#testUnescapeBackslashes
func TestRead_UnescapeBackslashes(t *testing.T) {
	blocks := readBlocks(t, `This is \*not emphasis\*`)
	require.Len(t, blocks, 1)
	// Backslash-escaped characters should not be emphasis
	runs := blocks[0].SourceRuns()
	assert.False(t, hasInlineCodeRun(runs), "escaped asterisks should not be emphasis")
}

// --- Mixed HTML and Markdown ---

// okapi: MarkdownFilterTest#testMixedHtmlInlineAndMarkdown
func TestRead_MixedHtmlInlineAndMarkdown(t *testing.T) {
	blocks := readBlocks(t, "Text **bold** and <em>italic</em> together")
	require.Len(t, blocks, 1)
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
}

// --- Ordered Lists ---

func TestRead_OrderedList(t *testing.T) {
	blocks := readBlocks(t, "1. First\n2. Second\n3. Third\n")
	require.Len(t, blocks, 3)
	assert.Equal(t, "First", blocks[0].SourceText())
	assert.Equal(t, "Second", blocks[1].SourceText())
	assert.Equal(t, "Third", blocks[2].SourceText())
	assert.Equal(t, "list-item", blocks[0].Type)
}

// --- HTML Comment Tests ---

// okapi: MarkdownFilterTest#testHtmlCommentAtColumn1
func TestRead_HtmlCommentAtColumn1(t *testing.T) {
	input := "<!-- comment -->\n\nParagraph\n"
	parts := readParts(t, input)
	blocks := testutil.FilterBlocks(parts)
	assert.Len(t, blocks, 1)
	assert.Equal(t, "Paragraph", blocks[0].SourceText())
}

// --- Inline HTML with Attributes ---

// okapi: MarkdownFilterTest#testHtmlInlineWithAttributes
func TestRead_HtmlInlineWithAttributes(t *testing.T) {
	blocks := readBlocks(t, `Text <span class="highlight">highlighted</span> end`)
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].SourceText(), "highlighted")
}

// --- Config Tests ---

func TestConfig_FormatName(t *testing.T) {
	cfg := &markdown.Config{}
	assert.Equal(t, "markdown", cfg.FormatName())
}

func TestConfig_ApplyMap(t *testing.T) {
	cfg := &markdown.Config{}
	err := cfg.ApplyMap(map[string]any{
		"translateCodeBlocks":  true,
		"translateFrontMatter": true,
	})
	require.NoError(t, err)
	assert.True(t, cfg.TranslateCodeBlocks)
	assert.True(t, cfg.TranslateFrontMatter)
}

func TestConfig_ApplyMapUnknown(t *testing.T) {
	cfg := &markdown.Config{}
	err := cfg.ApplyMap(map[string]any{"unknownKey": true})
	require.Error(t, err)
}

func TestConfig_Reset(t *testing.T) {
	cfg := &markdown.Config{TranslateCodeBlocks: true}
	cfg.Reset()
	assert.False(t, cfg.TranslateCodeBlocks)
}

// --- Neighboring Marks ---

// okapi: MarkdownFilterTest#testNeighboringMarks
func TestRead_NeighboringMarks(t *testing.T) {
	blocks := readBlocks(t, "**bold***italic*")
	require.Len(t, blocks, 1)
	assert.Equal(t, "bolditalic", blocks[0].SourceText())
	runs := blocks[0].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
}

// --- HTML Emphasis with HTML tags ---

// okapi: MarkdownFilterTest#testHtmlEmphasisAndStrong
func TestRead_HtmlEmphasisAndStrong(t *testing.T) {
	blocks := readBlocks(t, "Text with <em>italic</em> and <strong>bold</strong>")
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].SourceText(), "italic")
	assert.Contains(t, blocks[0].SourceText(), "bold")
}

// --- Skeleton roundtrip with emphasis ---

func TestSkeletonRoundtrip_Emphasis(t *testing.T) {
	input := "This has **bold** and *italic* text\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_Link(t *testing.T) {
	input := "Click [here](https://example.com) please\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_MultipleElements(t *testing.T) {
	input := "# Heading\n\nParagraph with **bold**\n\n- Item 1\n- Item 2\n\n```go\ncode\n```\n\n---\n\nFinal text\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_HeadingLevels(t *testing.T) {
	input := "# H1\n\n## H2\n\n### H3\n\n#### H4\n\n##### H5\n\n###### H6\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_SetextHeadings(t *testing.T) {
	input := "Title\n=====\n\nSubtitle\n--------\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_FrontMatter(t *testing.T) {
	input := "---\ntitle: Hello\nauthor: World\n---\n\n# Content\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_OrderedList(t *testing.T) {
	input := "1. First\n2. Second\n3. Third\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_Blockquote(t *testing.T) {
	input := "> Quoted text\n>\n> More quoted\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_Image(t *testing.T) {
	input := "See ![alt text](image.png) here\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_Table(t *testing.T) {
	input := "| A | B |\n| --- | --- |\n| 1 | 2 |\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_HTMLBlock(t *testing.T) {
	input := "Text\n\n<div>HTML content</div>\n\nMore text\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}

func TestSkeletonRoundtrip_CRLF(t *testing.T) {
	input := "# Title\r\n\r\nParagraph\r\n"
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output)
}
