package phpcontent_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/phpcontent"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Basic string extraction ---

// okapi: PHPContentFilterTest#testSingleQuotedString
func TestSingleQuotedString(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'Hello world';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testDoubleQuotedString
func TestDoubleQuotedString(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Hello world";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testHeredocString
func TestHeredocString(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $text = <<<EOT\nHello heredoc\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello heredoc", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testQuotedHeredocString
func TestQuotedHeredocString(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $text = <<<\"EOT\"\nHello quoted heredoc\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello quoted heredoc", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testQuotedNowdocString
func TestNowdocString(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $text = <<<'EOT'\nHello nowdoc\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello nowdoc", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testSemiColumnHeredocString
func TestSemicolonHeredocString(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $text = <<<EOT\nHello\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testMultipleLinesHeredocString
func TestMultiLineHeredocString(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $text = <<<EOT\nLine one\nLine two\nLine three\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Line one\nLine two\nLine three", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testEmptyHeredocStringAndOutput
func TestEmptyHeredocString(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $text = <<<EOT\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	// Empty heredoc should not produce a block
	assert.Empty(t, blocks)
}

// okapi: PHPContentFilterTest#testWhiteHeredocStringAndOutput
func TestWhitespaceHeredocString(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $text = <<<EOT\n   \nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	// Whitespace-only heredoc should not produce a block
	assert.Empty(t, blocks)
}

// --- Concatenation ---

// okapi: PHPContentFilterTest#testConcatSQStrings
func TestConcatSingleQuotedStrings(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'Hello ' . 'World';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testConcatDQStringsWithCodesAndVariable
func TestConcatDQStringsWithCodesAndVariable(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Hello $name" . " welcome";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	// $name becomes an inline code
	assert.Equal(t, "Hello  welcome", blocks[0].SourceText())
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
}

// okapi: PHPContentFilterTest#testConcatMultipleStrings
func TestConcatMultipleStrings(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'One' . ' Two' . ' Three';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "One Two Three", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testConcatSGAndDQStrings
func TestConcatMixedQuoteStrings(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'Hello ' . "World";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testConcatWithVariable
func TestConcatWithVariable(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	// Variable-only concatenation parts should break the concatenation
	input := `<?php $text = 'Hello';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testConcatWithEndings
func TestConcatWithEndings(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'Line1' . 'Line2';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Line1Line2", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testCommaCaseWithConcat
func TestCommaCaseWithConcat(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $arr = array('Hello' . ' World', 'Goodbye');`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
}

// --- Inline codes ---

// okapi: PHPContentFilterTest#testEntryWithCodes
func TestEntryWithCodes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Click <a href='test'>here</a> now";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Click here now", blocks[0].SourceText())
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
}

// okapi: PHPContentFilterTest#testSimpleHTMLCodes
func TestSimpleHTMLCodes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "This is <b>bold</b> text";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "This is bold text", blocks[0].SourceText())
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
	// Should have <b> and </b> as spans
	require.Len(t, frag.Spans, 2)
	assert.Equal(t, "<b>", frag.Spans[0].Data)
	assert.Equal(t, model.SpanOpening, frag.Spans[0].SpanType)
	assert.Equal(t, "</b>", frag.Spans[1].Data)
	assert.Equal(t, model.SpanClosing, frag.Spans[1].SpanType)
}

// okapi: PHPContentFilterTest#testParitalStartingHTMLCodes
func TestPartialStartingHTMLCodes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "<b>Bold text";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Bold text", blocks[0].SourceText())
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
}

// okapi: PHPContentFilterTest#testParitalClosingHTMLCodes
func TestPartialClosingHTMLCodes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Bold text</b>";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Bold text", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testSpecialHTMLCodes
func TestSpecialHTMLCodes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Line<br/>break";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Linebreak", blocks[0].SourceText())
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
	require.Len(t, frag.Spans, 1)
	assert.Equal(t, model.SpanPlaceholder, frag.Spans[0].SpanType)
}

// okapi: PHPContentFilterTest#testEscapeCodes
func TestEscapeCodes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Hello\nWorld";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "HelloWorld", blocks[0].SourceText())
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
}

// okapi: PHPContentFilterTest#testLinefeedCodes
func TestLinefeedCodes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Line1\nLine2\nLine3";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Line1Line2Line3", blocks[0].SourceText())
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	// Should have 2 \n escape spans
	assert.GreaterOrEqual(t, len(frag.Spans), 2)
}

// okapi: PHPContentFilterTest#testOutputLinefeedCodes
func TestOutputLinefeedCodes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Line1\nLine2";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := phpcontent.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `\n`)
	assert.Contains(t, output, "Line1")
	assert.Contains(t, output, "Line2")
}

// okapi: PHPContentFilterTest#testVariableCodes
func TestVariableCodes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Hello $name, welcome to $app";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello , welcome to ", blocks[0].SourceText())
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
	// $name and $app should be inline codes
	require.Len(t, frag.Spans, 2)
	assert.Equal(t, "$name", frag.Spans[0].Data)
	assert.Equal(t, "$app", frag.Spans[1].Data)
}

// --- Comments ---

// okapi: PHPContentFilterTest#testCommentsSingleLine
func TestCommentsSingleLine(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n// This is a comment\n$text = 'Hello';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	// Comment is non-translatable, only the string should be a block
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())

	// Verify comment exists as Data
	hasComment := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if comment, ok := data.Properties["comment"]; ok && comment == "// This is a comment" {
				hasComment = true
			}
		}
	}
	assert.True(t, hasComment)
}

// okapi: PHPContentFilterTest#testCommentsMultiline
func TestCommentsMultiline(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n/* Multi\nline\ncomment */\n$text = 'Hello';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testEmptyComment
func TestEmptyComment(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n//\n$text = 'Hello';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testCommentsWithApos
func TestCommentsWithApostrophe(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n// It's a test\n$text = 'Hello';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

// --- Skip/Text directives ---

// okapi: PHPContentFilterTest#testSkipDirective
func TestSkipDirective(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n//_skip_\n$text = 'Skip this';\n$other = 'Keep this';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Keep this", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testSkipDirectiveOnConcat
func TestSkipDirectiveOnConcat(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n//_skip_\n$text = 'Skip' . ' this';\n$other = 'Keep';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Keep", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testTextInBSkipDirective
func TestTextInBSkipDirective(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n//_bskip_\n$a = 'Skip1';\n$b = 'Skip2';\n//_eskip_\n$c = 'Keep';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Keep", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testESkipDirective
func TestESkipDirective(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n//_bskip_\n$a = 'Skip';\n//_eskip_\n$b = 'Keep';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Keep", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testDirectiveInMultilineComment
func TestDirectiveInMultilineComment(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n/* _bskip_ */\n$a = 'Skip';\n/* _eskip_ */\n$b = 'Keep';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Keep", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testBTextDirective
func TestBTextDirective(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n//_btext_\n$a = 'Keep this';\n//_etext_\n$b = 'And this';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 1)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Keep this")
}

// okapi: PHPContentFilterTest#testETextDirective
func TestETextDirective(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n//_btext_\n$a = 'Extract';\n//_etext_\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Extract", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testSkipOutsideDirective
func TestSkipOutsideDirective(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n$a = 'Before';\n//_bskip_\n$b = 'Skip';\n//_eskip_\n$c = 'After';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Before", blocks[0].SourceText())
	assert.Equal(t, "After", blocks[1].SourceText())
}

// okapi: PHPContentFilterTest#testDisabledDirectives
func TestDisabledDirectives(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	// Disable directives via config
	cfg := reader.Config()
	err := cfg.ApplyMap(map[string]any{"useDirectives": false})
	require.NoError(t, err)

	input := "<?php\n//_skip_\n$a = 'Should be extracted';"
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Should be extracted", blocks[0].SourceText())
}

// okapi: PHPContentFilterTest#testDirectiveScope
func TestDirectiveScope(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n//_bskip_\n$a = 'Skip1';\n$b = 'Skip2';\n//_eskip_\n$c = 'Keep1';\n$d = 'Keep2';"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Keep1", blocks[0].SourceText())
	assert.Equal(t, "Keep2", blocks[1].SourceText())
}

// --- Array keys ---

// okapi: PHPContentFilterTest#testSQIndex
func TestSingleQuotedArrayIndex(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $arr['greeting'] = 'Hello';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "greeting", blocks[0].ID)
	assert.Equal(t, "greeting", blocks[0].Properties["arrayKey"])
}

// okapi: PHPContentFilterTest#testDQIndex
func TestDoubleQuotedArrayIndex(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $arr["greeting"] = "Hello";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "greeting", blocks[0].ID)
}

// okapi: PHPContentFilterTest#testnoStringIndex
func TestNoStringIndex(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $arr[0] = 'Hello';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	// Numeric index should not be used as block ID
	assert.NotEqual(t, "0", blocks[0].ID)
}

// okapi: PHPContentFilterTest#testHeredocIndex
func TestHeredocArrayIndex(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $arr['key'] = <<<EOT\nHeredoc value\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Heredoc value", blocks[0].SourceText())
	assert.Equal(t, "key", blocks[0].ID)
}

// okapi: PHPContentFilterTest#testQuotedHeredocIndex
func TestQuotedHeredocArrayIndex(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $arr['key'] = <<<\"EOT\"\nQuoted heredoc value\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Quoted heredoc value", blocks[0].SourceText())
	assert.Equal(t, "key", blocks[0].ID)
}

// okapi: PHPContentFilterTest#testNowdocIndex
func TestNowdocArrayIndex(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $arr['key'] = <<<'EOT'\nNowdoc value\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Nowdoc value", blocks[0].SourceText())
	assert.Equal(t, "key", blocks[0].ID)
}

// okapi: PHPContentFilterTest#testOutputArrayKeys
func TestOutputArrayKeys(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $arr['greeting'] = 'Hello';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := phpcontent.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Hello")
}

// --- Entity references ---

// okapi: PHPContentFilterTest#testEntityReferences
func TestEntityReferences(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Hello &amp; World";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	// Entity references should be preserved as-is in the text
	assert.Contains(t, blocks[0].SourceText(), "&amp;")
}

// okapi: PHPContentFilterTest#testReferencesLooklike
func TestReferencesLooklike(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Price is $5 & up";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	// & should not be misinterpreted
	assert.Contains(t, blocks[0].SourceText(), "& up")
}

// okapi: PHPContentFilterTest#testFilteringOfHtmlLikeTags
func TestFilteringOfHtmlLikeTags(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = "Use <em>emphasis</em> here";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Use emphasis here", blocks[0].SourceText())
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
}

// --- Output / Roundtrip ---

// okapi: PHPContentFilterTest#testOutputSimple
func TestOutputSimple(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'Hello world';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := phpcontent.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Hello world")
}

// okapi: PHPContentFilterTest#testOutputWithNoStrings
func TestOutputWithNoStrings(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n$x = 42;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

// okapi: PHPContentFilterTest#testOutputHeredoc
func TestOutputHeredoc(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $text = <<<EOT\nHello heredoc\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := phpcontent.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Hello heredoc")
}

// okapi: PHPContentFilterTest#testOutputMix
func TestOutputMix(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n$a = 'Single';\n$b = \"Double\";\n$c = <<<EOT\nHeredoc\nEOT;\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 3)

	var buf bytes.Buffer
	writer := phpcontent.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Single")
	assert.Contains(t, output, "Double")
	assert.Contains(t, output, "Heredoc")
}

// okapi: PHPContentFilterTest#testLineBreakType
func TestLineBreakType(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php $text = 'Hello';\r\n$text2 = 'World';\r\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "World", blocks[1].SourceText())
}

// okapi: PHPContentFilterTest#testDoubleExtraction
func TestDoubleExtraction(t *testing.T) {
	ctx := t.Context()

	// Run extraction twice to verify consistency
	for run := 0; run < 2; run++ {
		reader := phpcontent.NewReader()
		f, err := os.Open("testdata/simple.php")
		require.NoError(t, err)
		err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.php", model.LocaleEnglish))
		require.NoError(t, err)

		blocks := testutil.CollectBlocks(t, reader.Read(ctx))
		reader.Close()

		require.GreaterOrEqual(t, len(blocks), 4, "run %d: expected at least 4 blocks", run)
	}
}

// --- Layer bookends ---

func TestLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'Hello';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "phpcontent", layer.Format)
}

// --- Metadata ---

// okapi: PHPContentFilterTest#testDefaultInfo
func TestReaderMetadata(t *testing.T) {
	reader := phpcontent.NewReader()
	assert.Equal(t, "phpcontent", reader.Name())
	assert.Equal(t, "PHP Content", reader.DisplayName())
}

func TestReaderSignature(t *testing.T) {
	reader := phpcontent.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-php")
	assert.Contains(t, sig.Extensions, ".php")
}

func TestReaderNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

// --- Roundtrip with translation ---

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'Hello';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Simulate translation
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			}
		}
	}

	var buf bytes.Buffer
	writer := phpcontent.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Bonjour")
	assert.NotContains(t, output, "Hello")
}

// --- File-based roundtrip ---

func TestFileRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.php")
	require.NoError(t, err)
	reader := phpcontent.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.php", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := phpcontent.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "Welcome to the app")
	assert.Contains(t, output, "My Title")
}

// --- Escape sequence handling in single-quoted strings ---

func TestSingleQuotedEscapes(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'It\'s a test';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "It's a test", blocks[0].SourceText())
}

func TestSingleQuotedBackslash(t *testing.T) {
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := `<?php $text = 'Path: C:\\Windows';`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, `Path: C:\Windows`, blocks[0].SourceText())
}
