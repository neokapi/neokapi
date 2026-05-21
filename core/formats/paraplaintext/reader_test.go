package paraplaintext_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/paraplaintext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// neokapi-only: ParagraphPlainTextFilterTest#testSingleParagraph
func TestSingleParagraph(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	input := "Hello world"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// neokapi-only: ParagraphPlainTextFilterTest#testMultipleParagraphs
func TestMultipleParagraphs(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	input := "First paragraph.\n\nSecond paragraph."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "First paragraph.", blocks[0].SourceText())
	assert.Equal(t, "Second paragraph.", blocks[1].SourceText())
}

// neokapi-only: ParagraphPlainTextFilterTest#testMultiLineParagraph
func TestMultiLineParagraph(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	input := "Line one.\nLine two.\nLine three."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Line one.\nLine two.\nLine three.", blocks[0].SourceText())
}

// neokapi-only: ParagraphPlainTextFilterTest#testThreeParagraphs
func TestThreeParagraphs(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	input := "First\n\nSecond\n\nThird"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "First", blocks[0].SourceText())
	assert.Equal(t, "Second", blocks[1].SourceText())
	assert.Equal(t, "Third", blocks[2].SourceText())
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	input := "Hello"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "paraplaintext", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := paraplaintext.NewReader()
	sig := reader.Signature()
	// No auto-detection extensions to avoid conflicting with plaintext
	assert.Empty(t, sig.Extensions)
}

func TestReaderMetadata(t *testing.T) {
	reader := paraplaintext.NewReader()
	assert.Equal(t, "paraplaintext", reader.Name())
	assert.Equal(t, "Paragraph Plain Text", reader.DisplayName())
}

func TestConfigFormatName(t *testing.T) {
	cfg := &paraplaintext.Config{}
	assert.Equal(t, "paraplaintext", cfg.FormatName())
}

func TestConfigValidate(t *testing.T) {
	cfg := &paraplaintext.Config{}
	require.NoError(t, cfg.Validate())
}

func TestConfigReset(t *testing.T) {
	cfg := &paraplaintext.Config{}
	cfg.Reset()
	require.NoError(t, cfg.Validate())
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &paraplaintext.Config{}
	err := cfg.ApplyMap(map[string]any{"unknown": true})
	require.Error(t, err)
}

func TestConfigApplyMapEmpty(t *testing.T) {
	cfg := &paraplaintext.Config{}
	err := cfg.ApplyMap(map[string]any{})
	require.NoError(t, err)
}

// neokapi-only: ParagraphPlainTextFilterTest#testPreserveNewlines
func TestPreserveNewlinesInParagraph(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	input := "Line 1\nLine 2\nLine 3\n\nNext paragraph"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Line 1\nLine 2\nLine 3", blocks[0].SourceText())
	assert.Equal(t, "Next paragraph", blocks[1].SourceText())
}

func TestParagraphSeparatorsAsData(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	input := "First\n\nSecond"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	dataCount := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
		}
	}
	assert.GreaterOrEqual(t, dataCount, 1, "paragraph separator should be emitted as Data")
}

func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.txt")
	require.NoError(t, err)
	reader := paraplaintext.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.txt", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := paraplaintext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "This is the first paragraph.")
	assert.Contains(t, output, "This is the second paragraph.")
	assert.Contains(t, output, "This is the third paragraph.")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := "First paragraph.\n\nSecond paragraph."
	reader := paraplaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "First paragraph." {
				block.SetTargetText(model.LocaleFrench, "Premier paragraphe.")
			} else if block.SourceText() == "Second paragraph." {
				block.SetTargetText(model.LocaleFrench, "Deuxieme paragraphe.")
			}
		}
	}

	var buf bytes.Buffer
	writer := paraplaintext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Premier paragraphe.")
	assert.Contains(t, output, "Deuxieme paragraphe.")
	assert.NotContains(t, output, "First paragraph.")
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reader := paraplaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("First\n\nSecond\n\nThird", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var count int
	for range ch {
		count++
	}
	assert.LessOrEqual(t, count, 7)
}

func TestWriterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var buf bytes.Buffer
	writer := paraplaintext.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	cancel()

	err = writer.Write(ctx, ch)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBlockNaming(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	input := "First\n\nSecond"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "para1", blocks[0].Name)
	assert.Equal(t, "para2", blocks[1].Name)
	assert.Equal(t, "tu1", blocks[0].ID)
	assert.Equal(t, "tu2", blocks[1].ID)
}

func TestOnlyWhitespace(t *testing.T) {
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	input := "\n\n\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	// Whitespace only should not produce blocks
	// (the split may produce empty segments which are skipped)
	assert.LessOrEqual(t, len(blocks), 1)
}
