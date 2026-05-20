package mosestext_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/mosestext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// MosesTextFilterTest.java — ported from bridge tests
// ---------------------------------------------------------------------------

// okapi: MosesTextFilterTest#testStartDocument
func TestStartDocument(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello world\n", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart (START_DOCUMENT)")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "text/x-mosestext", layer.MimeType)
	assert.Equal(t, "UTF-8", layer.Encoding)
	assert.Equal(t, model.LocaleEnglish, layer.Locale)
}

// okapi: MosesTextFilterTest#testLineBreaks_CR
func TestLineBreaks_CR(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	snippet := "Line 1\rLine 2\r"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 2, "should produce at least 2 blocks for 2 lines")

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: MosesTextFilterTest#testineBreaks_CRLF
// (the upstream Okapi method name has a typo: "testineBreaks_CRLF").
func TestLineBreaks_CRLF(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	snippet := "Line 1\r\nLine 2\r\n"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 2)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: MosesTextFilterTest#testLineBreaks_LF
func TestLineBreaks_LF(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	snippet := "Line 1\nLine 2\n"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 2)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: MosesTextFilterTest#testEntry
func TestEntry(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	snippet := "Line 1\rLine 2"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 2, "should produce at least 2 blocks (one per line)")
	assert.Equal(t, "Line 2", blocks[1].SourceText())
}

// neokapi-only: multi-line extraction sanity check. Okapi has no
// matching @Test (testEntry covers single-line extraction).
func TestEntries(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	input := "Hello world\nSecond line\nThird line\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 3)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Second line", blocks[1].SourceText())
	assert.Equal(t, "Third line", blocks[2].SourceText())
}

// okapi: MosesTextFilterTest#testSpecialChars
func TestSpecialChars(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	snippet := "Line 1\rLine 2 with tab[\t] and more [<{|&/\\}>]"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 2)

	b := blocks[1]
	text := b.SourceText()
	assert.Contains(t, text, "Line 2 with tab[")
	assert.Contains(t, text, "] and more [")
}

// okapi: MosesTextFilterTest#testLiterals
func TestLiterals(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	// Moses text is a plain-text format; literal text is preserved as-is.
	// (Entity decoding is a feature of the Okapi Java bridge XML parser, not the
	// native format. The native reader treats all text as plain text.)
	snippet := "Simple literal text with symbols: < > & \" '"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "<")
	assert.Contains(t, text, ">")
	assert.Contains(t, text, "&")
	assert.Contains(t, text, "\"")
	assert.Contains(t, text, "'")
}

// okapi: MosesTextFilterTest#testWhiteSpaces
func TestWhiteSpaces(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	snippet := "Text 1   .\rLine with   extra   spaces"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 2)

	// First block: "Text 1   ." with preserved whitespace.
	assert.Equal(t, "Text 1   .", blocks[0].SourceText())
	assert.True(t, blocks[0].PreserveWhitespace,
		"Moses text blocks should preserve whitespace")

	// Second block: whitespace preserved.
	assert.Equal(t, "Line with   extra   spaces", blocks[1].SourceText())
	assert.True(t, blocks[1].PreserveWhitespace,
		"Moses text blocks should preserve whitespace")
}

// okapi: MosesTextFilterTest#testEntry (layer structure)
func TestLayerStructure(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello world", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
		"last part should be LayerEnd")

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "mosestext", layer.Format)
}

// okapi: MosesTextFilterTest#testEntry (single line)
func TestSingleLine(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Simple text", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Simple text", blocks[0].SourceText())
}

// okapi: MosesTextFilterTest#testEntry (block IDs)
func TestBlockIDs(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Line A\nLine B\nLine C", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: MosesTextFilterTest#testEntry (segment structure)
func TestSegmentIDs(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello world", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotEmpty(t, seg.Runs, "segment should have content")
		}
	}
}

// okapi: MosesTextFilterTest#testCode1 (no spans for plain text)
func TestNoSpansForPlainText(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Simple plain text", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		runs := b.SourceRuns()
		for _, r := range runs {
			assert.Nil(t, r.Ph, "plain text should have no placeholder runs")
			assert.Nil(t, r.PcOpen, "plain text should have no opening inline-code runs")
			assert.Nil(t, r.PcClose, "plain text should have no closing inline-code runs")
		}
	}
}

// okapi: MosesTextFilterTest#testWhiteSpaces (preserve whitespace flag)
func TestPreserveWhitespace(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Text with   spaces", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)
	assert.True(t, blocks[0].PreserveWhitespace,
		"Moses text should always preserve whitespace")
}

// ---------------------------------------------------------------------------
// Additional reader tests
// ---------------------------------------------------------------------------

func TestReaderSignature(t *testing.T) {
	reader := mosestext.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-mosestext")
	// Should NOT auto-detect .txt
	assert.Empty(t, sig.Extensions, "should not auto-detect .txt as mosestext")
}

func TestReaderMetadata(t *testing.T) {
	reader := mosestext.NewReader()
	assert.Equal(t, "mosestext", reader.Name())
	assert.Equal(t, "Moses Text", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestEmptyLinesAsData(t *testing.T) {
	ctx := t.Context()
	reader := mosestext.NewReader()
	input := "Line 1\n\nLine 3\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Should have LayerStart, Block(Line 1), Data(empty), Block(Line 3), LayerEnd
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Line 1", blocks[0].SourceText())
	assert.Equal(t, "Line 3", blocks[1].SourceText())

	hasData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			hasData = true
		}
	}
	assert.True(t, hasData, "empty lines should be emitted as Data parts")
}

// okapi: MosesTextFilterTest#testFromFile
func TestFromFile(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.txt")
	require.NoError(t, err)
	reader := mosestext.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.txt", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 3)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Second line", blocks[1].SourceText())
	assert.Equal(t, "Third line", blocks[2].SourceText())
}

// okapi: MosesTextFilterTest#testFromFile (multiline)
func TestFromFileMultiline(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/multiline.txt")
	require.NoError(t, err)
	reader := mosestext.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/multiline.txt", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 4)
	assert.Equal(t, "First line of text", blocks[0].SourceText())
	assert.Contains(t, blocks[1].SourceText(), "special chars")
	assert.Equal(t, "Fourth line after empty line", blocks[2].SourceText())
	assert.Equal(t, "Fifth line", blocks[3].SourceText())

	// Should have a Data part for the empty line
	hasData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			hasData = true
		}
	}
	assert.True(t, hasData, "empty line in multiline.txt should produce a Data part")
}

// ---------------------------------------------------------------------------
// MosesTextFilterWriterTest.java — ported from bridge tests
// ---------------------------------------------------------------------------

// okapi: MosesTextFilterWriterTest#testSimpleOutputFromMosesText
func TestWriterSimpleOutput(t *testing.T) {
	ctx := t.Context()

	input := "Line 1\nLine 2\n"
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := mosestext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Line 1")
	assert.Contains(t, output, "Line 2")
}

// okapi: MosesTextFilterWriterTest#testMultilineOutputFromMosesText
func TestWriterMultilineOutput(t *testing.T) {
	ctx := t.Context()

	input := "Text 1.\nText 2\nText 3.\nText 4\n"
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := mosestext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Text 1.")
	assert.Contains(t, output, "Text 2")
	assert.Contains(t, output, "Text 3.")
	assert.Contains(t, output, "Text 4")
}

// okapi: MosesTextFilterTest#testDoubleExtraction (roundtrip)
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.txt")
	require.NoError(t, err)
	reader := mosestext.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.txt", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := mosestext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "Second line")
	assert.Contains(t, output, "Third line")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := "Hello\nWorld\n"
	reader := mosestext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set translations
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
	writer := mosestext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour")
	assert.Contains(t, output, "Monde")
	assert.NotContains(t, output, "Hello")
	assert.NotContains(t, output, "World")
}

func TestConfig(t *testing.T) {
	cfg := &mosestext.Config{}
	assert.Equal(t, "mosestext", cfg.FormatName())
	require.NoError(t, cfg.Validate())

	cfg.Reset()
	require.NoError(t, cfg.Validate())

	err := cfg.ApplyMap(map[string]any{"unknown": "value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")

	err = cfg.ApplyMap(map[string]any{})
	require.NoError(t, err)
}
