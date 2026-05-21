package ttml_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/ttml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: TTMLFilterTest#testProcessTextUnit
// Verifies basic <p> element extraction: two subtitles with <br/> line breaks
// produce two text units with <br/> removed (default escapeBR=true).
func TestTextUnitExtraction(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Hello world</p>
    <p begin="00:00:05.000" end="00:00:08.000">Second subtitle</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Second subtitle", blocks[1].SourceText())
}

// okapi: TTMLFilterTest#testProcessTextUnit (timing attributes)
// Verifies that timing attributes are preserved as block properties.
func TestTimingAttributes(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Hello world</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "00:00:01.000", blocks[0].Properties["begin"])
	assert.Equal(t, "00:00:04.000", blocks[0].Properties["end"])
}

// okapi: TTMLFilterTest#testProcessTextUnit (br escape mode)
// Verifies that <br/> elements are replaced with spaces in default escape mode.
func TestBRDefaultEscapeMode(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">Thanks everyone <br/>for joining us today.</p>
    <p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/> to be with you.</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Thanks everyone for joining us today.", blocks[0].SourceText())
	assert.Equal(t, "I am so excited to be with you.", blocks[1].SourceText())
}

// okapi: TTMLFilterTest#testProcessTextUnitNonEscapeBrMode
// When escapeBR is disabled, <br/> tags are preserved as literal text.
func TestNonEscapeBRMode(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	err := reader.Config().ApplyMap(map[string]any{
		"escapeBR": false,
	})
	require.NoError(t, err)

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">Thanks everyone <br/>for joining us today.</p>
    <p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/> to be with you.</p>
  </div></body>
</tt>`
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Thanks everyone <br/>for joining us today.", blocks[0].SourceText())
	assert.Equal(t, "I am so excited<br/> to be with you.", blocks[1].SourceText())
}

// okapi: TTMLFilterTest#testMergeCaptions
// When adjacent captions end with trailing punctuation (comma), the reader
// merges them into a single text unit.
func TestCaptionMerging(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	err := reader.Config().ApplyMap(map[string]any{
		"mergeAdjacentCaptions": true,
	})
	require.NoError(t, err)

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">Thanks everyone<br/>for joining us today,</p>
    <p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/>to be with you.</p>
  </div></body>
</tt>`
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Thanks everyone")
	assert.Contains(t, text, "to be with you.")
	assert.Contains(t, text, "I am so excited")
}

// okapi: TTMLFilterTest#testDontMergeCaptions
// When mergeAdjacentCaptions is disabled, each <p> element produces a separate block.
func TestDontMergeCaptions(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	// Default: mergeAdjacentCaptions = false

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">Thanks everyone <br/>for joining us today,</p>
    <p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/> to be with you.</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Thanks everyone for joining us today,", blocks[0].SourceText())
	assert.Equal(t, "I am so excited to be with you.", blocks[1].SourceText())
}

// okapi: TTMLFilterTest#testQuoteCaptions
// Verifies quote and punctuation handling.
func TestQuoteCaptions(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">'Thanks everyone <br/>for joining us today.'</p>
    <p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">'I am so excited<br/> to be with you.'</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 1)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "'Thanks everyone")
	assert.Contains(t, text, "today.'")
}

// okapi: TTMLFilterTest#testEmptyCaptions
// Empty <p> elements are skipped (no block emitted for empty content).
func TestEmptyCaptions(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center"></p>
    <p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/> to be with you.</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "I am so excited to be with you.", blocks[0].SourceText())
}

// okapi: TTMLFilterTest#testReadMaxCharMaxLine
// Metadata elements are ignored; text extraction still works.
func TestMaxCharMaxLine(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <head>
    <metadata>
      <okp:maximum_character_count>20</okp:maximum_character_count>
      <okp:maximum_line_count>3</okp:maximum_line_count>
    </metadata>
  </head>
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">Thanks everyone <br/>for joining us today.</p>
    <p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/> to be with you.</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Thanks everyone for joining us today.", blocks[0].SourceText())
	assert.Equal(t, "I am so excited to be with you.", blocks[1].SourceText())
}

// okapi: TTMLFilterTest (block IDs)
// Verifies that each extracted block has a unique ID.
func TestBlockIDs(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.430" end="00:00:02.497" region="bottom" tts:textAlign="center">First.</p>
    <p xml:id="subtitle2" begin="00:00:02.497" end="00:00:06.363" region="bottom" tts:textAlign="center">Second.</p>
    <p xml:id="subtitle3" begin="00:00:06.830" end="00:00:09.963" region="bottom" tts:textAlign="center">Third.</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: TTMLFilterTest (layer structure)
// Verifies the part stream starts with LayerStart and ends with LayerEnd.
func TestLayerStructure(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.430" end="00:00:02.497">Hello.</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "ttml", layer.Format)
	assert.Equal(t, "application/ttml+xml", layer.MimeType)
}

// okapi: TTMLFilterTest (empty document)
// Verifies that a TTML document with only empty subtitles does not crash.
func TestEmptyDocument(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.430" end="00:00:02.497"></p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts, "should produce parts even for empty subtitles")

	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "should not produce blocks for empty subtitles")
}

func TestReaderSignature(t *testing.T) {
	reader := ttml.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/ttml+xml")
	assert.Contains(t, sig.Extensions, ".ttml")
	assert.Contains(t, sig.Extensions, ".dfxp")
}

func TestReaderMetadata(t *testing.T) {
	reader := ttml.NewReader()
	assert.Equal(t, "ttml", reader.Name())
	assert.Equal(t, "TTML Subtitles", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

// okapi: TTMLFilterTest (inline span handling)
// Verifies that text inside <span> elements is extracted correctly.
func TestInlineSpanPassthrough(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <body><div>
    <p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263">I am so <span tts:fontStyle="italic">excited</span> to be here.</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "I am so excited to be here.", blocks[0].SourceText())
}

// okapi: TTMLFilterTest (multiple divs)
// Verifies extraction from multiple <div> elements.
func TestMultipleDivs(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:00:01.000" end="00:00:04.000">First div subtitle.</p>
    </div>
    <div>
      <p begin="00:00:05.000" end="00:00:08.000">Second div subtitle.</p>
    </div>
  </body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "First div subtitle.", blocks[0].SourceText())
	assert.Equal(t, "Second div subtitle.", blocks[1].SourceText())
}

// okapi: TTMLFilterTest (xml:id as block name)
// Verifies that the xml:id attribute is used as the block name.
func TestXMLIDAsBlockName(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p xml:id="myCaption" begin="00:00:01.000" end="00:00:04.000">Hello</p>
  </div></body>
</tt>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "myCaption", blocks[0].Name)
}

// neokapi-only: RoundTripTtmlIT#ttmlFiles — no such Okapi IT class in v1.48.0 and no roundtrip @Test in TTMLFilterTest (extraction-only); native roundtrip is neokapi's own coverage.
// Roundtrip test for a simple TTML file.
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.ttml")
	require.NoError(t, err)
	reader := ttml.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.ttml", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := ttml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "Second subtitle")
	assert.Contains(t, output, "Third subtitle")
	assert.Contains(t, output, "00:00:01.000")
	assert.Contains(t, output, "00:00:05.000")
}

// okapi: RoundTripTtmlIT (with target locale)
// Roundtrip test that verifies target text is written when locale is set.
func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Hello</p>
    <p begin="00:00:05.000" end="00:00:08.000">World</p>
  </div></body>
</tt>`

	reader := ttml.NewReader()
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
	writer := ttml.NewWriter()
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
	assert.NotContains(t, output, ">Hello<")
	assert.NotContains(t, output, ">World<")
}

// okapi: RoundTripTtmlIT (with_br roundtrip)
// Roundtrip test for TTML with <br/> elements.
func TestRoundTripWithBR(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/with_br.ttml")
	require.NoError(t, err)
	reader := ttml.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/with_br.ttml", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := ttml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Thanks everyone")
	assert.Contains(t, output, "joining us today")
}

// okapi: RoundTripTtmlIT (complex roundtrip)
// Roundtrip test for TTML with multiple divs and styling.
func TestRoundTripComplex(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/complex.ttml")
	require.NoError(t, err)
	reader := ttml.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/complex.ttml", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 4)

	var buf bytes.Buffer
	writer := ttml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "First subtitle in first div.")
	assert.Contains(t, output, "Subtitle in second div.")
	assert.Contains(t, output, "<head>")
	assert.Contains(t, output, "<styling>")
}

func TestConfigApplyMap(t *testing.T) {
	cfg := &ttml.Config{}
	cfg.Reset()

	assert.False(t, cfg.MergeAdjacentCaptions)
	assert.True(t, cfg.EscapeBR)

	err := cfg.ApplyMap(map[string]any{
		"mergeAdjacentCaptions": true,
		"escapeBR":              false,
	})
	require.NoError(t, err)
	assert.True(t, cfg.MergeAdjacentCaptions)
	assert.False(t, cfg.EscapeBR)
}

func TestConfigApplyMapUnknownParam(t *testing.T) {
	cfg := &ttml.Config{}
	err := cfg.ApplyMap(map[string]any{
		"unknown": true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

func TestConfigApplyMapWrongType(t *testing.T) {
	cfg := &ttml.Config{}
	err := cfg.ApplyMap(map[string]any{
		"escapeBR": "not a bool",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bool")
}

// TestWriterMinimalTTML verifies that the writer can generate TTML from blocks
// without a skeleton document.
// TestReadSurvivesMalformedHead guards against the okapi reference
// fixture (example1.ttml) shipping `<okp:foo>...</lilt:foo>` in the
// head — a real namespace-prefix mismatch that fails Go's xml.Decoder.
// Translatable content lives only in <body>, so the reader must skip
// past head garbage rather than aborting before it ever sees a <p>.
func TestReadSurvivesMalformedHead(t *testing.T) {
	ctx := t.Context()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml">
  <head>
    <metadata>
      <okp:badprefix>val</lilt:badprefix>
    </metadata>
  </head>
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Hello world</p>
  </div></body>
</tt>`
	reader := ttml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

func TestWriterMinimalTTML(t *testing.T) {
	ctx := t.Context()

	block1 := model.NewBlock("tu1", "Hello world")
	block1.Properties["begin"] = "00:00:01.000"
	block1.Properties["end"] = "00:00:04.000"

	block2 := model.NewBlock("tu2", "Goodbye")
	block2.Properties["begin"] = "00:00:05.000"
	block2.Properties["end"] = "00:00:08.000"

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block1},
		{Type: model.PartBlock, Resource: block2},
	}

	var buf bytes.Buffer
	writer := ttml.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "Goodbye")
	assert.Contains(t, output, "<tt")
	assert.Contains(t, output, "</tt>")
}
