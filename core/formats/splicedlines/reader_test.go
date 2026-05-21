package splicedlines_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/splicedlines"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// neokapi-only: SplicedLinesFilterTest#testSingleLine — no upstream @Test (SplicedLinesFilterTest exposes only testCombinedLines for splicing); single-line passthrough is neokapi coverage
func TestSingleLine(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	input := "Hello world"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// neokapi-only: SplicedLinesFilterTest#testContinuationLine — upstream covers backslash continuation in testCombinedLines (already mapped in plaintext); this granular case is neokapi coverage
func TestContinuationLine(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	input := "Hello \\\nworld"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello \nworld", blocks[0].SourceText())
}

// neokapi-only: SplicedLinesFilterTest#testMultipleContinuations — upstream covers multi-line splicing in testCombinedLines (already mapped in plaintext); neokapi coverage
func TestMultipleContinuations(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	input := "Line1 \\\nLine2 \\\nLine3"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Line1 \nLine2 \nLine3", blocks[0].SourceText())
}

// neokapi-only: SplicedLinesFilterTest#testMixedLines — upstream covers mixed plain/continued lines in testCombinedLines (already mapped in plaintext); neokapi coverage
func TestMixedLines(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	input := "Single line\nContinued \\\nline\nAnother single"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "Single line", blocks[0].SourceText())
	assert.Equal(t, "Continued \nline", blocks[1].SourceText())
	assert.Equal(t, "Another single", blocks[2].SourceText())
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	input := "Hello"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "splicedlines", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := splicedlines.NewReader()
	sig := reader.Signature()
	// No auto-detection extensions to avoid conflicting with plaintext
	assert.Empty(t, sig.Extensions)
}

func TestReaderMetadata(t *testing.T) {
	reader := splicedlines.NewReader()
	assert.Equal(t, "splicedlines", reader.Name())
	assert.Equal(t, "Spliced Lines", reader.DisplayName())
}

func TestConfigFormatName(t *testing.T) {
	cfg := &splicedlines.Config{}
	assert.Equal(t, "splicedlines", cfg.FormatName())
}

func TestConfigValidate(t *testing.T) {
	cfg := &splicedlines.Config{}
	require.NoError(t, cfg.Validate())
}

func TestConfigReset(t *testing.T) {
	cfg := &splicedlines.Config{}
	cfg.Reset()
	require.NoError(t, cfg.Validate())
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &splicedlines.Config{}
	err := cfg.ApplyMap(map[string]any{"unknown": true})
	require.Error(t, err)
}

func TestConfigApplyMapEmpty(t *testing.T) {
	cfg := &splicedlines.Config{}
	err := cfg.ApplyMap(map[string]any{})
	require.NoError(t, err)
}

// neokapi-only: SplicedLinesFilterTest#testTrailingBackslash — no upstream @Test for trailing-backslash-at-EOF; testCombinedLines covers splicing; neokapi coverage
func TestTrailingBackslashAtEnd(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	// File ends with a continuation line (no following line)
	input := "Hello \\"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	// The backslash is stripped, trailing content is just "Hello "
	assert.Equal(t, "Hello ", blocks[0].SourceText())
}

// neokapi-only: SplicedLinesFilterTest#testBackslashNotAtEnd — no upstream @Test for non-terminal backslash; neokapi coverage
func TestBackslashNotAtEnd(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	input := "Hello \\world"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	// Backslash not at end of line is not a continuation marker
	assert.Equal(t, "Hello \\world", blocks[0].SourceText())
}

func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.txt")
	require.NoError(t, err)
	reader := splicedlines.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.txt", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := splicedlines.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "This is a single line.")
	assert.Contains(t, output, "Another single line.")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := "Hello\nWorld \\\ncontinued"
	reader := splicedlines.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			} else {
				block.SetTargetText(model.LocaleFrench, "Monde suite")
			}
		}
	}

	var buf bytes.Buffer
	writer := splicedlines.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour")
	assert.Contains(t, output, "Monde suite")
	assert.NotContains(t, output, "Hello")
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reader := splicedlines.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello\nWorld", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var count int
	for range ch {
		count++
	}
	assert.LessOrEqual(t, count, 5)
}

func TestWriterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var buf bytes.Buffer
	writer := splicedlines.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	cancel()

	err = writer.Write(ctx, ch)
	assert.ErrorIs(t, err, context.Canceled)
}

// neokapi-only: SplicedLinesFilterTest#testEmptyLines — no upstream @Test for blank-line-as-data; neokapi coverage
func TestEmptyLinesAsData(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	input := "Hello\n\nWorld"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "World", blocks[1].SourceText())

	dataCount := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
		}
	}
	assert.GreaterOrEqual(t, dataCount, 1)
}

func TestMultipleBlocks(t *testing.T) {
	ctx := t.Context()
	reader := splicedlines.NewReader()
	input := "Line1\nLine2\nLine3"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "Line1", blocks[0].SourceText())
	assert.Equal(t, "Line2", blocks[1].SourceText())
	assert.Equal(t, "Line3", blocks[2].SourceText())
}

func TestWriterContinuationOutput(t *testing.T) {
	ctx := t.Context()

	// Create a block with newlines to test writer output
	block := model.NewBlock("tu1", "Hello \nworld")
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1", Format: "splicedlines"}},
		{Type: model.PartBlock, Resource: block},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1", Format: "splicedlines"}},
	}

	var buf bytes.Buffer
	writer := splicedlines.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	// Writer should re-add backslash continuations for multi-line blocks
	assert.Contains(t, output, "Hello \\\n")
	assert.Contains(t, output, "world")
}
