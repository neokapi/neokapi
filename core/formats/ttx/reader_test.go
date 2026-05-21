package ttx_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/formats/ttx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real TTX (TRADOStag) places translatable text directly inside <Tuv> —
// there is no <Seg> wrapper element in the format. All snippets below match
// the shape of genuine Trados TagEditor output and the Okapi test fixtures.
const simpleTTX = `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Hello world</Tuv>
<Tuv Lang="FR-FR">Bonjour le monde</Tuv>
</Tu>
<Tu MatchPercent="100">
<Tuv Lang="EN-US">Goodbye</Tuv>
<Tuv Lang="FR-FR">Au revoir</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`

const sourceOnlyTTX = `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Source only text</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`

const inlineTagsTTX = `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Text with <ut>tag</ut> inside</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`

// okapi: TTXFilterTest#testBasicNoUT — extracts source and target from TTX translation units.
func TestReadSimpleTTX(t *testing.T) {
	ctx := t.Context()
	reader := ttx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTTX, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.True(t, blocks[0].HasTarget("FR-FR"))
	assert.Equal(t, "Bonjour le monde", blocks[0].TargetText("FR-FR"))
	assert.Equal(t, "Au revoir", blocks[1].TargetText("FR-FR"))
}

// neokapi-only: the <Tu MatchPercent="N"> attribute is surfaced as a Block
// property. Okapi reads MatchPercent into an AltTranslationsAnnotation
// (TTXFilterTest#testTUInfo), which the native reader does not model; this test
// covers the property mapping the native reader provides instead.
func TestReadMatchPercent(t *testing.T) {
	ctx := t.Context()
	reader := ttx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTTX, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "0", blocks[0].Properties["match-percent"])
	assert.Equal(t, "100", blocks[1].Properties["match-percent"])
}

// neokapi-only: a <Tu> with only a source <Tuv> (no target) extracts to a
// Block with source text and no target entry. Okapi's nearest case
// (TTXFilterTest#testOutputWithOriginalWithoutTraget) is output-side and copies
// the source into a synthesized target — behavior the native writer does not
// reproduce — so this exercises the native read mapping only.
func TestReadSourceOnly(t *testing.T) {
	ctx := t.Context()
	reader := ttx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sourceOnlyTTX, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Source only text", blocks[0].SourceText())
}

// okapi: TTXFilterTest#testBasicWithUT — inline <ut> tags are processed within text content.
func TestReadInlineTags(t *testing.T) {
	ctx := t.Context()
	reader := ttx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(inlineTagsTTX, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Text with tag inside", blocks[0].SourceText())
}

// neokapi-only: the streaming reader wraps content in PartLayerStart /
// PartLayerEnd parts. This is a neokapi content-model concern with no Okapi
// TTXFilterTest equivalent.
func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := ttx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTTX, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "ttx", layer.Format)
}

// neokapi-only: verifies the native format signature (MIME type, extension,
// and the <TRADOStag sniff). Format detection is a neokapi registry concern
// with no Okapi TTXFilterTest equivalent.
func TestReaderSignature(t *testing.T) {
	reader := ttx.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-ttx+xml")
	assert.Contains(t, sig.Extensions, ".ttx")
	assert.NotNil(t, sig.Sniff)
	assert.True(t, sig.Sniff([]byte(`<TRADOStag Version="2.0">`)))
	assert.False(t, sig.Sniff([]byte(`<html>`)))
}

func TestReaderMetadata(t *testing.T) {
	reader := ttx.NewReader()
	assert.Equal(t, "ttx", reader.Name())
	assert.Equal(t, "Trados TagEditor TTX", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := ttx.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := ttx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

// okapi: TTXFilterTest#testDoubleExtraction — roundtrip read/write preserves TTX content.
// okapi: TtxXliffCompareIT#ttxXliffCompareFiles
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	reader := ttx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTTX, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := ttx.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale("FR-FR")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "Bonjour le monde")
	assert.Contains(t, output, "TRADOStag")
}

func TestRoundTripWithNewTarget(t *testing.T) {
	ctx := t.Context()

	reader := ttx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sourceOnlyTTX, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Add target translations
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText(model.LocaleFrench, "Texte source uniquement")
		}
	}

	var buf bytes.Buffer
	writer := ttx.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Source only text")
	assert.Contains(t, output, "Texte source uniquement")
}
