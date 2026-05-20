package mif_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/mif"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: ExtractionTest#testSimpleText — extracts simple text strings from MIF paragraphs.
func TestReadSimpleMIF(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Hello world.'>" + `
  >
 >
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Second paragraph.'>" + `
  >
 >
>
`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello world.", blocks[0].SourceText())
	assert.Equal(t, "Second paragraph.", blocks[1].SourceText())
}

func TestReadMIFWithPgfTag(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Heading'>" + `
  <ParaLine
   <String ` + "`Chapter Title'>" + `
  >
 >
>
`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Chapter Title", blocks[0].SourceText())
	assert.Equal(t, "Heading", blocks[0].Properties["pgf_tag"])
}

// okapi: ExtractionTest#testNoTextEntry — non-translatable catalog entries are skipped.
func TestReadMIFSkipsNonTranslatable(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()
	input := `<MIFFile 2015>
<FontCatalog
 <Font
  <FTag ` + "`Body'>" + `
 >
>
<ColorCatalog
 <Color
  <ColorTag ` + "`Black'>" + `
 >
>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Translatable text.'>" + `
  >
 >
>
`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Translatable text.", blocks[0].SourceText())
}

// okapi: ExtractionTest#testParagraphLinesProcessing — multiple ParaLine elements merge into one block.
func TestReadMIFMultipleParaLines(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`First part '>" + `
  >
  <ParaLine
   <String ` + "`second part.'>" + `
  >
 >
>
`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "First part second part.", blocks[0].SourceText())
}

// okapi: ExtractionTest#testTabs — verifies tab characters are preserved in extracted text.
func TestReadMIFSpecialCharacters(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Before tab'>" + `
   <Char Tab>
   <String ` + "`after tab.'>" + `
  >
 >
>
`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].SourceText(), "Before tab")
	assert.Contains(t, blocks[0].SourceText(), "\t")
	assert.Contains(t, blocks[0].SourceText(), "after tab.")
}

// okapi: ExtractionTest#testStartDocument — verifies LayerStart/LayerEnd structure wraps MIF content.
func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Hello.'>" + `
  >
 >
>
`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "mif", layer.Format)
}

// okapi: ExtractionTest#testDefaultInfo — verifies MIF MIME type and file signature.
func TestReaderSignature(t *testing.T) {
	reader := mif.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-mif")
	assert.Contains(t, sig.Extensions, ".mif")
	assert.NotNil(t, sig.Sniff)
	assert.True(t, sig.Sniff([]byte("<MIFFile 2015>")))
	assert.False(t, sig.Sniff([]byte("not a mif file")))
}

func TestReaderMetadata(t *testing.T) {
	reader := mif.NewReader()
	assert.Equal(t, "mif", reader.Name())
	assert.Equal(t, "Adobe FrameMaker MIF", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestReadMIFVersionData(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()
	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Text.'>" + `
  >
 >
>
`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	hasVersionData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Properties["tag"] == "MIFFile" {
				hasVersionData = true
				assert.Equal(t, "2015", data.Properties["version"])
			}
		}
	}
	assert.True(t, hasVersionData, "MIFFile version should be emitted as Data")
}

// okapi: RoundTripTest#roundTripsWithDifferentParameters — roundtrip read/write preserves MIF content.
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.mif")
	require.NoError(t, err)
	reader := mif.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.mif", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := mif.NewWriter()
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
	assert.Contains(t, output, "A heading paragraph.")
	assert.Contains(t, output, "MIFFile")
}

// TestRoundTripNonSkeletonDeepNesting exercises the non-skeleton writer
// fallback (writer.go writeData -> data.Properties["raw"]) and proves it
// stays byte-exact for deeply nested non-translatable top-level
// statements. Skip tags such as <Document> / <ColorCatalog> are
// TOPSTATEMENTSTOSKIP, so the reader emits each as a single Data part
// whose "raw" carries the full, verbatim source text of the entire
// subtree. This is the path that the parseMIF raw accumulation feeds;
// the perf fix (#608 M1) replaced the O(file × tree-depth) up-the-tree
// copy with a single root-level builder and must keep it byte-identical.
//
// The fixture is entirely non-translatable so the whole file round-trips
// through the raw path (translatable containers like <TextFlow> are
// rebuilt from the model and are deliberately out of scope here — see
// TestRoundTrip, which uses Contains for that reason).
func TestRoundTripNonSkeletonDeepNesting(t *testing.T) {
	ctx := t.Context()

	// A deep tree (5+ levels) under a skipped top-level <Document>, plus a
	// second skipped sibling. The old accumulation copied every leaf's
	// bytes up through every ancestor; the new code records each line
	// once into the root builder.
	input := `<MIFFile 2015>
<Document
 <DStartPage 1>
 <DOutline
  <DOutlineLevel
   <DOLPgfTag ` + "`Level1'>" + `
   <DOLNested
    <DOLDeep ` + "`leaf value'>" + `
   >
  >
 >
 <DPageSize  21.0 cm 29.7 cm>
>
<ColorCatalog
 <Color
  <ColorTag ` + "`Black'>" + `
  <ColorCyan  0.000000>
 >
>
`

	reader := mif.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// The skipped <Document> must capture its full subtree verbatim in
	// the "raw" Data property.
	foundDocumentRaw := false
	for _, p := range parts {
		if p.Type != model.PartData {
			continue
		}
		data := p.Resource.(*model.Data)
		if data.Properties["tag"] == "Document" {
			foundDocumentRaw = true
			raw := data.Properties["raw"]
			assert.Contains(t, raw, "<DOLDeep `leaf value'>",
				"deepest leaf must be present in the accumulated raw")
			assert.Contains(t, raw, "<DPageSize  21.0 cm 29.7 cm>",
				"sibling after the deep subtree must also be present")
			assert.True(t, strings.HasPrefix(raw, "<Document\n"),
				"raw must begin with the <Document> opener")
			assert.True(t, strings.HasSuffix(raw, ">\n"),
				"raw must end with the <Document> closer")
		}
	}
	require.True(t, foundDocumentRaw, "expected a <Document> Data part with raw")

	// Non-skeleton writer path: no SkeletonStore set on reader or writer.
	var buf bytes.Buffer
	writer := mif.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// The non-skeleton fallback rebuilds both skipped statements verbatim
	// from raw, so the whole file is byte-exact.
	assert.Equal(t, input, buf.String(),
		"non-skeleton roundtrip of deeply nested skipped statements must be byte-exact")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Hello.'>" + `
  >
 >
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`World.'>" + `
  >
 >
>
`
	reader := mif.NewReader()
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
	writer := mif.NewWriter()
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
	cfg := &mif.Config{}
	assert.Equal(t, "mif", cfg.FormatName())
	require.NoError(t, cfg.Validate())

	err := cfg.ApplyMap(map[string]any{"unknown": "value"})
	require.Error(t, err)

	err = cfg.ApplyMap(map[string]any{})
	require.NoError(t, err)
}
