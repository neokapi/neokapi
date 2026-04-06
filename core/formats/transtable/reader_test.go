package transtable_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/transtable"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: TransTableFilterTest#testSimplePair
func TestSimplePair(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "greeting\tHello"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "greeting", blocks[0].Properties["key"])
	assert.Equal(t, "greeting", blocks[0].Name)
}

// okapi: TransTableFilterTest#testMultiplePairs
func TestMultiplePairs(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "greeting\tHello\nfarewell\tGoodbye"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
}

// okapi: TransTableFilterTest#testCommentLine
func TestCommentLine(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "# This is a comment\ngreeting\tHello"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())

	// Verify comment is Data
	hasComment := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Properties["comment"] == "# This is a comment" {
				hasComment = true
			}
		}
	}
	assert.True(t, hasComment, "comment should be emitted as Data")
}

// okapi: TransTableFilterTest#testEmptyLine
func TestEmptyLine(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "greeting\tHello\n\nfarewell\tGoodbye"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 2)

	hasEmptyData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if _, has := data.Properties["comment"]; !has {
				hasEmptyData = true
			}
		}
	}
	assert.True(t, hasEmptyData, "empty line should be emitted as Data")
}

// okapi: TransTableFilterTest#testEmptyValue
func TestEmptyValue(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "greeting\t"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "", blocks[0].SourceText())
	assert.Equal(t, "greeting", blocks[0].Properties["key"])
}

// okapi: TransTableFilterTest#testKeyOnly
func TestKeyOnly(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "greeting"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "", blocks[0].SourceText())
	assert.Equal(t, "greeting", blocks[0].Properties["key"])
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

// okapi: TransTableFilterTest#testStartDocument — verifies LayerStart/LayerEnd wraps transtable content.
func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "greeting\tHello"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "transtable", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := transtable.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.Extensions, ".tab")
	assert.Contains(t, sig.Extensions, ".tsv")
}

func TestReaderMetadata(t *testing.T) {
	reader := transtable.NewReader()
	assert.Equal(t, "transtable", reader.Name())
	assert.Equal(t, "Translation Table", reader.DisplayName())
}

func TestConfigFormatName(t *testing.T) {
	cfg := &transtable.Config{}
	assert.Equal(t, "transtable", cfg.FormatName())
}

func TestConfigValidate(t *testing.T) {
	cfg := &transtable.Config{}
	require.NoError(t, cfg.Validate())
}

func TestConfigReset(t *testing.T) {
	cfg := &transtable.Config{}
	cfg.Reset()
	require.NoError(t, cfg.Validate())
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &transtable.Config{}
	err := cfg.ApplyMap(map[string]any{"unknown": true})
	require.Error(t, err)
}

func TestConfigApplyMapEmpty(t *testing.T) {
	cfg := &transtable.Config{}
	err := cfg.ApplyMap(map[string]any{})
	require.NoError(t, err)
}

func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.tab")
	require.NoError(t, err)
	reader := transtable.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.tab", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := transtable.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "greeting\tHello")
	assert.Contains(t, output, "farewell\tGoodbye")
	assert.Contains(t, output, "# This is a comment")
	assert.Contains(t, output, "welcome\tWelcome back")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := "greeting\tHello\nfarewell\tGoodbye"
	reader := transtable.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			} else if block.SourceText() == "Goodbye" {
				block.SetTargetText(model.LocaleFrench, "Au revoir")
			}
		}
	}

	var buf bytes.Buffer
	writer := transtable.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "greeting\tBonjour")
	assert.Contains(t, output, "farewell\tAu revoir")
	assert.NotContains(t, output, "Hello")
	assert.NotContains(t, output, "Goodbye")
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately

	reader := transtable.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("greeting\tHello\nfarewell\tGoodbye", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var count int
	for range ch {
		count++
	}
	// With cancelled context, we may get fewer parts
	assert.LessOrEqual(t, count, 5)
}

// okapi: TransTableFilterTest#testTabInValue
func TestTabInValue(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "greeting\tHello\tWorld"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	// Value includes everything after first tab
	assert.Equal(t, "Hello\tWorld", blocks[0].SourceText())
}

// okapi: TransTableFilterTest#testWhitespaceKey
func TestWhitespaceHandling(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "  \n\t\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks, "whitespace-only lines should be treated as empty/data")
}

func TestLineNumbers(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "a\t1\nb\t2\nc\t3"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "1", blocks[0].Properties["line"])
	assert.Equal(t, "2", blocks[1].Properties["line"])
	assert.Equal(t, "3", blocks[2].Properties["line"])
}

func TestBlockIDs(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "a\t1\nb\t2"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	// Block ID is the key
	assert.Equal(t, "a", blocks[0].ID)
	assert.Equal(t, "b", blocks[1].ID)
}

func TestMultipleComments(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "# Comment 1\n# Comment 2\ngreeting\tHello"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)

	commentCount := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if _, has := data.Properties["comment"]; has {
				commentCount++
			}
		}
	}
	assert.Equal(t, 2, commentCount)
}

func TestWriterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var buf bytes.Buffer
	writer := transtable.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	cancel()

	err = writer.Write(ctx, ch)
	assert.ErrorIs(t, err, context.Canceled)
}
