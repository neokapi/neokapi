package rtf_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/rtf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: RTFFilterTest#testSimpleTU — extracts translatable text from RTF paragraphs.
func TestReadSimpleRTF(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	input := `{\rtf1\ansi\deff0
\pard Hello world.\par
\pard Second paragraph.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello world.", blocks[0].SourceText())
	assert.Equal(t, "Second paragraph.", blocks[1].SourceText())
}

// okapi: RtfSnippetsTest#testBold — bold formatting is processed as inline content.
func TestReadRTFWithFormatting(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	input := `{\rtf1\ansi\deff0
\pard Normal and \b bold\b0  text.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Normal and bold text.", blocks[0].SourceText())
}

func TestReadRTFSkipsFontTable(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	input := `{\rtf1\ansi\deff0
{\fonttbl{\f0 Times New Roman;}{\f1 Arial;}}
\pard Translatable text.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Translatable text.", blocks[0].SourceText())
	// Font names should not appear in blocks.
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "Times New Roman")
		assert.NotContains(t, b.SourceText(), "Arial")
	}
}

func TestReadRTFSkipsColorTable(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	input := `{\rtf1\ansi\deff0
{\colortbl;\red255\green0\blue0;}
\pard Visible text.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Visible text.", blocks[0].SourceText())
}

func TestReadRTFSkipsStylesheet(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	input := `{\rtf1\ansi\deff0
{\stylesheet{\s0 Normal;}}
\pard Body text here.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Body text here.", blocks[0].SourceText())
}

func TestReadRTFUnicode(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	// \u233 is e with acute (U+00E9)
	input := `{\rtf1\ansi\deff0
\pard Caf\u233?.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].SourceText(), "Caf")
	assert.Contains(t, blocks[0].SourceText(), "\u00e9")
}

func TestReadRTFHexCharacter(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	// \'e9 is e with acute in Windows-1252
	input := `{\rtf1\ansi\deff0
\pard Caf\'e9.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].SourceText(), "Caf")
}

func TestReadRTFSpecialCharacters(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	input := `{\rtf1\ansi\deff0
\pard Left\emdash Right.\par
\pard Tab\tab here.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Contains(t, blocks[0].SourceText(), "\u2014") // em dash
	assert.Contains(t, blocks[1].SourceText(), "\t")     // tab
}

func TestReadRTFEscapedCharacters(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	input := `{\rtf1\ansi\deff0
\pard Curly \{ braces \} and backslash \\.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].SourceText(), "{")
	assert.Contains(t, blocks[0].SourceText(), "}")
	assert.Contains(t, blocks[0].SourceText(), "\\")
}

// okapi: RtfEventTest#testStartDoc — verifies LayerStart/LayerEnd structure wraps RTF content.
func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	input := `{\rtf1\ansi\deff0
\pard Hello.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "rtf", layer.Format)
}

// okapi: RTFFilterTest#testBasicProcessing — verifies RTF MIME type and signature.
func TestReaderSignature(t *testing.T) {
	reader := rtf.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/rtf")
	assert.Contains(t, sig.Extensions, ".rtf")
	assert.NotEmpty(t, sig.MagicBytes)
}

func TestReaderMetadata(t *testing.T) {
	reader := rtf.NewReader()
	assert.Equal(t, "rtf", reader.Name())
	assert.Equal(t, "Rich Text Format", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestReadMultipleParagraphs(t *testing.T) {
	ctx := t.Context()
	reader := rtf.NewReader()
	input := `{\rtf1\ansi\deff0
\pard First paragraph.\par
\pard Second paragraph.\par
\pard Third paragraph.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 3)
	assert.Equal(t, "First paragraph.", blocks[0].SourceText())
	assert.Equal(t, "Second paragraph.", blocks[1].SourceText())
	assert.Equal(t, "Third paragraph.", blocks[2].SourceText())
}

// okapi: ExtractionComparisionTest#testDoubleExtraction — roundtrip read/write preserves RTF content.
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.rtf")
	require.NoError(t, err)
	reader := rtf.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.rtf", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := rtf.NewWriter()
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
	assert.Contains(t, output, "\\rtf1")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := `{\rtf1\ansi\deff0
\pard Hello.\par
\pard World.\par
}`
	reader := rtf.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello." {
				block.SetTargetText(model.LocaleFrench, "Bonjour.")
			} else if block.SourceText() == "World." {
				block.SetTargetText(model.LocaleFrench, "Monde.")
			}
		}
	}

	var buf bytes.Buffer
	writer := rtf.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour.")
	assert.Contains(t, output, "Monde.")
	assert.NotContains(t, output, "Hello.")
	assert.NotContains(t, output, "World.")
}

func TestConfigApplyMap(t *testing.T) {
	cfg := &rtf.Config{}
	assert.Equal(t, "rtf", cfg.FormatName())
	require.NoError(t, cfg.Validate())

	err := cfg.ApplyMap(map[string]any{"unknown": "value"})
	require.Error(t, err)

	err = cfg.ApplyMap(map[string]any{})
	require.NoError(t, err)
}
