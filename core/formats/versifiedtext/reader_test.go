package versifiedtext_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/versifiedtext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// neokapi-only: VersifiedTextFilterTest#testSingleVerse
func TestSingleVerse(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "\\v1 In the beginning was the Word."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "In the beginning was the Word.", blocks[0].SourceText())
	assert.Equal(t, "1", blocks[0].Properties["verse"])
	assert.Equal(t, "verse.1", blocks[0].Name)
}

// neokapi-only: VersifiedTextFilterTest#testMultipleVerses
func TestMultipleVerses(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "\\v1 First verse.\n\\v2 Second verse."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "First verse.", blocks[0].SourceText())
	assert.Equal(t, "1", blocks[0].Properties["verse"])
	assert.Equal(t, "Second verse.", blocks[1].SourceText())
	assert.Equal(t, "2", blocks[1].Properties["verse"])
}

// neokapi-only: VersifiedTextFilterTest#testVerseWithSpace
func TestVerseWithSpace(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "\\v 1 Text after spaced marker."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Text after spaced marker.", blocks[0].SourceText())
	assert.Equal(t, "1", blocks[0].Properties["verse"])
}

// neokapi-only: VersifiedTextFilterTest#testNumericVerseMarker
func TestNumericVerseMarker(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "1. In the beginning."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "In the beginning.", blocks[0].SourceText())
	assert.Equal(t, "1", blocks[0].Properties["verse"])
}

// neokapi-only: VersifiedTextFilterTest#testNumericSpaceMarker
func TestNumericSpaceMarker(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "3 Some text here."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Some text here.", blocks[0].SourceText())
	assert.Equal(t, "3", blocks[0].Properties["verse"])
}

// neokapi-only: VersifiedTextFilterTest#testStanzaBreak
func TestStanzaBreak(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "\\v1 First verse.\n\n\\v2 Second verse."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 2)

	dataCount := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
		}
	}
	assert.GreaterOrEqual(t, dataCount, 1, "stanza break should emit Data")
}

// neokapi-only: VersifiedTextFilterTest#testNonVerseLine
func TestNonVerseLine(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "A line without verse marker."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "A line without verse marker.", blocks[0].SourceText())
	assert.Empty(t, blocks[0].Properties["verse"])
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "\\v1 Hello"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "versifiedtext", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := versifiedtext.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.Extensions, ".ver")
}

func TestReaderMetadata(t *testing.T) {
	reader := versifiedtext.NewReader()
	assert.Equal(t, "versifiedtext", reader.Name())
	assert.Equal(t, "Versified Text", reader.DisplayName())
}

func TestConfigFormatName(t *testing.T) {
	cfg := &versifiedtext.Config{}
	assert.Equal(t, "versifiedtext", cfg.FormatName())
}

func TestConfigValidate(t *testing.T) {
	cfg := &versifiedtext.Config{}
	require.NoError(t, cfg.Validate())
}

func TestConfigReset(t *testing.T) {
	cfg := &versifiedtext.Config{}
	cfg.Reset()
	require.NoError(t, cfg.Validate())
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &versifiedtext.Config{}
	err := cfg.ApplyMap(map[string]any{"unknown": true})
	require.Error(t, err)
}

func TestConfigApplyMapEmpty(t *testing.T) {
	cfg := &versifiedtext.Config{}
	err := cfg.ApplyMap(map[string]any{})
	require.NoError(t, err)
}

func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.ver")
	require.NoError(t, err)
	reader := versifiedtext.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.ver", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := versifiedtext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "In the beginning was the Word.")
	assert.Contains(t, output, "And the Word was with God.")
	assert.Contains(t, output, "In him was life.")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := "\\v1 Hello\n\\v2 World"
	reader := versifiedtext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
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
	writer := versifiedtext.NewWriter()
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

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reader := versifiedtext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("\\v1 Hello\n\\v2 World", model.LocaleEnglish))
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
	writer := versifiedtext.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	cancel()

	err = writer.Write(ctx, ch)
	assert.ErrorIs(t, err, context.Canceled)
}

// neokapi-only: VersifiedTextFilterTest#testMixedContent
func TestMixedContent(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "Title of the poem\n\\v1 First verse.\n\\v2 Second verse.\n\nPlain line in stanza two."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 4)
	assert.Equal(t, "Title of the poem", blocks[0].SourceText())
	assert.Empty(t, blocks[0].Properties["verse"])
	assert.Equal(t, "First verse.", blocks[1].SourceText())
	assert.Equal(t, "1", blocks[1].Properties["verse"])
}

// neokapi-only: VersifiedTextFilterTest#testMultiDigitVerse
func TestMultiDigitVerse(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "\\v12 Verse twelve."
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Verse twelve.", blocks[0].SourceText())
	assert.Equal(t, "12", blocks[0].Properties["verse"])
}

// neokapi-only: VersifiedTextFilterTest#testMultipleStanzas
func TestMultipleStanzas(t *testing.T) {
	ctx := t.Context()
	reader := versifiedtext.NewReader()
	input := "\\v1 First\n\\v2 Second\n\n\\v3 Third\n\\v4 Fourth"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 4)

	dataCount := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
		}
	}
	assert.Equal(t, 1, dataCount, "should have one stanza break")
}
