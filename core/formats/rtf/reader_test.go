package rtf_test

import (
	"bytes"
	"os"
	"path/filepath"
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
	// \u233 is e with acute (U+00E9). The trailing '?' is the ANSI fallback
	// character that conformant readers must consume; it must NOT leak
	// into the extracted text.
	input := `{\rtf1\ansi\deff0
\pard Caf\u233?.\par
}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Caf\u00e9.", blocks[0].SourceText())
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

// TestRoundTripWrappedTarget round-trips simple.rtf with a known wrap on each
// block target ("«source»") and re-extracts to assert the merged output reads
// back as the wrapped form — catching the regression where the writer's
// `\u<n>?` escape (with `?` as the ANSI fallback) produced "«?text»?" because
// the reader wasn't consuming the spec-mandated fallback character.
func TestRoundTripWrappedTarget(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.rtf")
	require.NoError(t, err)
	reader := rtf.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.rtf", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var sources []string
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block := p.Resource.(*model.Block)
		src := block.SourceText()
		sources = append(sources, src)
		block.SetTargetText(model.LocaleFrench, "«"+src+"»")
	}
	require.NotEmpty(t, sources, "fixture should yield at least one block")

	var buf bytes.Buffer
	writer := rtf.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	merged := buf.Bytes()

	// Re-extract the merged document and verify each block reads back as the
	// wrapped target — no stray '?' fallback bytes around the guillemets.
	reader2 := rtf.NewReader()
	err = reader2.Open(ctx, testutil.RawDocFromString(string(merged), model.LocaleFrench))
	require.NoError(t, err)
	defer reader2.Close()

	got := testutil.CollectBlocks(t, reader2.Read(ctx))
	require.Len(t, got, len(sources))
	for i, b := range got {
		assert.Equal(t, "«"+sources[i]+"»", b.SourceText(),
			"block %d: extracted text must match the wrapped target exactly", i)
	}
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

// okapi: RtfFullFileTest#testAllExternalFiles
// Okapi's testAllExternalFiles opens every external .rtf fixture in the test
// resources directory and drains all events, asserting the filter parses real
// documents without throwing. The native analog opens every .rtf fixture
// under testdata/ (the upstream Test01/Test02/AddComments fixtures plus
// simple.rtf) and reads every Part, asserting no read error surfaces and that
// at least one translatable block is produced from the real-world documents.
func TestAllExternalFiles(t *testing.T) {
	files, err := filepath.Glob("testdata/*.rtf")
	require.NoError(t, err)
	require.NotEmpty(t, files, "expected at least one .rtf fixture under testdata/")

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			ctx := t.Context()
			fh, err := os.Open(f)
			require.NoError(t, err)
			reader := rtf.NewReader()
			doc := testutil.RawDocFromReader(fh, f, model.LocaleEnglish)
			doc.Encoding = "windows-1252"
			doc.TargetLocale = model.LocaleFrench
			require.NoError(t, reader.Open(ctx, doc))
			defer reader.Close()

			blocks := testutil.CollectBlocks(t, reader.Read(ctx))
			assert.NotEmpty(t, blocks, "%s should yield at least one translatable block", f)
		})
	}
}
