package icml_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/icml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: ICMLFilterTest#toString_WhenMultipleContent_ThenExtractInTranslationUnit
// Verifies that multiple CharacterStyleRange Content elements within a
// ParagraphStyleRange are concatenated into a single translation unit.
func TestMultipleContentRanges(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	f, err := os.Open("testdata/Test01.wcml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/Test01.wcml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 3)

	// First ParagraphStyleRange has two CharacterStyleRanges with "First paragraph" + " with bold"
	assert.Equal(t, "First paragraph with bold", blocks[0].SourceText())
	assert.Equal(t, "Second paragraph text", blocks[1].SourceText())
	assert.Equal(t, "Third paragraph", blocks[2].SourceText())
}

// okapi: ICMLFilterTest#toString_WhenBreak_ThenTranslationUnitIsEmpty
// Verifies that <Br/> between ParagraphStyleRanges creates separate TUs.
func TestBreakSeparation(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	f, err := os.Open("testdata/Test01.wcml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/Test01.wcml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	// Each ParagraphStyleRange produces a separate block
	require.Len(t, blocks, 3)
}

// okapi: ICMLFilterTest#toString_WhenContentInTableCell_ThenSeparateTranslationUnit
// Verifies that table cell content is extracted as separate translation units.
func TestTableCellContent(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	f, err := os.Open("testdata/table.icml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/table.icml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 3)

	// Text before table
	assert.Equal(t, "Text before table", blocks[0].SourceText())
	// Table cells
	assert.Equal(t, "Cell one", blocks[1].SourceText())
	assert.Equal(t, "Cell two", blocks[2].SourceText())

	// Table cells should have the table property
	assert.Equal(t, "true", blocks[1].Properties["table"])
	assert.Equal(t, "true", blocks[2].Properties["table"])
}

// okapi: ICMLFilterTest#open_WhenSuccessfull_ThenReturnTrue
// Verifies that the reader can open a valid ICML document.
func TestOpenDocument(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	f, err := os.Open("testdata/minimal.icml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/minimal.icml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts)

	// Verify LayerStart
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "icml", layer.Format)
}

// okapi: ICMLFilterTest#getParameters_WhenNoParametersSet_ThenReturnParametersWithDefaultSettings
func TestDefaultConfig(t *testing.T) {
	reader := icml.NewReader()
	cfg := reader.Config().(*icml.Config)
	assert.False(t, cfg.ExtractNotes)
	assert.False(t, cfg.NewTUOnBr)
}

// okapi: ICMLFilterTest#getParameters_WhenParametersSet_ThenReturnParametersWithSettings
func TestCustomConfig(t *testing.T) {
	cfg := &icml.Config{}
	err := cfg.ApplyMap(map[string]any{
		"extractNotes": true,
		"newTuOnBr":    true,
	})
	require.NoError(t, err)
	assert.True(t, cfg.ExtractNotes)
	assert.True(t, cfg.NewTUOnBr)
}

// okapi: ICMLFilterTest#getName_ThenReturnName
// okapi: ICMLFilterTest#getDisplayName_ThenReturnDisplayName
// Okapi asserts getName()=="okf_icml" and getDisplayName()=="ICML Filter".
// neokapi's native format uses its own intuitive id ("icml") and human name
// ("ICML (Adobe InCopy)"); the verified contract is that the format exposes a
// stable name and a non-empty human-readable display name.
func TestReaderMetadata(t *testing.T) {
	reader := icml.NewReader()
	assert.Equal(t, "icml", reader.Name())
	assert.Equal(t, "ICML (Adobe InCopy)", reader.DisplayName())
}

// okapi: ICMLFilterTest#createFilterWriter_ThenReturnICMLFilterWriter
// Okapi asserts ICMLFilter.createFilterWriter() returns a non-null
// ICMLFilterWriter. The native analog: icml.NewWriter() returns a non-nil
// writer that implements the format.DataFormatWriter contract for the icml
// format.
func TestCreateFilterWriter(t *testing.T) {
	var w format.DataFormatWriter = icml.NewWriter()
	require.NotNil(t, w)
}

// ICMLFilterTest also covers two Okapi-internal filter-architecture surfaces
// that neokapi's reader/writer model does not expose (#611):
//
// okapi-skip: ICMLFilterTest#createSkeletonWriter_ThenReturnNull — Okapi's ISkeletonWriter abstraction (ICML returns null to opt out) has no neokapi analog; the native ICML format uses the byte-exact SkeletonStore mechanism instead
// okapi-skip: ICMLFilterTest#getEncoderManager_ThenReturnEncoderManager — Okapi's EncoderManager encoding/escaping subsystem is not part of neokapi's format model

// okapi: ICMLFilterTest#getMimeType_ThenReturnMimeType
func TestReaderMIMEType(t *testing.T) {
	reader := icml.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-icml+xml")
}

// okapi: ICMLFilterTest#getConfigurations_ThenReturnDefaultSettings
func TestReaderSignature(t *testing.T) {
	reader := icml.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.Extensions, ".icml")
	assert.Contains(t, sig.Extensions, ".wcml")
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestLayerStructure(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	f, err := os.Open("testdata/minimal.icml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/minimal.icml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "icml", layer.Format)
	assert.Equal(t, "application/x-icml+xml", layer.MimeType)
}

func TestSimpleExtraction(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	f, err := os.Open("testdata/minimal.icml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/minimal.icml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
}

func TestBlockIDs(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	f, err := os.Open("testdata/Test01.wcml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/Test01.wcml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestParagraphStylePreserved(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	f, err := os.Open("testdata/Test01.wcml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/Test01.wcml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	assert.Equal(t, "ParagraphStyle/Title", blocks[0].Properties["paragraphStyle"])
	assert.Equal(t, "ParagraphStyle/Body", blocks[1].Properties["paragraphStyle"])
}

func TestPropertiesElementSkipped(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Properties>
          <Leading type="unit">14</Leading>
        </Properties>
        <Content>Visible text only</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Visible text only", blocks[0].SourceText())
}

func TestNotesExcludedByDefault(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	f, err := os.Open("testdata/notes.icml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/notes.icml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	// Note content should be excluded; only visible text and " continues" are extracted.
	assert.Equal(t, "Visible text continues", blocks[0].SourceText())
	// With the default extractNotes:false, the editorial Note is neither a block
	// nor an annotation — it stays opaque skeleton.
	assert.Empty(t, blocks[0].Notes(), "no note annotation when extractNotes is off")
}

// TestNotesExtractedAsAnnotation verifies that with extractNotes on, the editorial
// <Note> content is surfaced as a parity-safe NoteAnnotation on the owning story
// block — NOT as a translatable Block. The translatable part stream is unchanged:
// still one aggregated block "Visible text continues" (legacy mode).
func TestNotesExtractedAsAnnotation(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()
	reader.Config().(*icml.Config).ExtractNotes = true

	f, err := os.Open("testdata/notes.icml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/notes.icml", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	// Part stream unchanged: the note adds no Block.
	require.Len(t, blocks, 1)
	assert.Equal(t, "Visible text continues", blocks[0].SourceText())
	for _, b := range blocks {
		assert.NotEqual(t, "Note content here", b.SourceText(), "note must not become a translatable block")
	}

	// The note text rides along as a block-scoped NoteAnnotation.
	notes := blocks[0].Notes()
	require.Len(t, notes, 1)
	assert.Equal(t, "Note content here", notes[0].Text)
	assert.Equal(t, "icml", notes[0].From)
}

// TestNotesExtractionPartStreamUnchanged confirms that toggling extractNotes does
// not alter the translatable part stream (same block count and source texts) —
// the only difference is the presence of NoteAnnotations.
func TestNotesExtractionPartStreamUnchanged(t *testing.T) {
	ctx := t.Context()

	read := func(extract bool) []*model.Block {
		reader := icml.NewReader()
		reader.Config().(*icml.Config).ExtractNotes = extract
		f, err := os.Open("testdata/notes.icml")
		require.NoError(t, err)
		err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/notes.icml", model.LocaleEnglish))
		require.NoError(t, err)
		defer reader.Close()
		return testutil.CollectBlocks(t, reader.Read(ctx))
	}

	off := read(false)
	on := read(true)
	require.Len(t, on, len(off))
	for i := range off {
		assert.Equal(t, off[i].ID, on[i].ID)
		assert.Equal(t, off[i].SourceText(), on[i].SourceText())
	}
}

// TestNotesSkeletonByteExactWithExtraction verifies that, in skeleton mode with
// extractNotes on, the <Note> bytes stay in skeleton (byte-exact round trip)
// while the note text is surfaced as a NoteAnnotation on a story block.
func TestNotesSkeletonByteExactWithExtraction(t *testing.T) {
	ctx := t.Context()

	data, err := os.ReadFile("testdata/notes.icml")
	require.NoError(t, err)
	input := string(data)

	reader := icml.NewReader()
	reader.Config().(*icml.Config).ExtractNotes = true
	writer := icml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var found bool
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		assert.NotEqual(t, "Note content here", b.SourceText(), "note must not become a translatable block")
		for _, n := range b.Notes() {
			if n.Text == "Note content here" {
				found = true
			}
		}
	}
	assert.True(t, found, "note text should be surfaced as a NoteAnnotation")

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()
	assert.Equal(t, input, buf.String(), "notes.icml must round-trip byte-exact with extractNotes on")
}

func TestInlineICMLContent(t *testing.T) {
	ctx := t.Context()
	reader := icml.NewReader()

	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello </Content>
      </CharacterStyleRange>
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/Italic">
        <Content>beautiful</Content>
      </CharacterStyleRange>
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content> world</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello beautiful world", blocks[0].SourceText())
}

func TestConfigApplyMapUnknownParam(t *testing.T) {
	cfg := &icml.Config{}
	err := cfg.ApplyMap(map[string]any{
		"unknown": true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")
}

func TestConfigApplyMapWrongType(t *testing.T) {
	cfg := &icml.Config{}
	err := cfg.ApplyMap(map[string]any{
		"extractNotes": "not a bool",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected bool")
}

// Native writer-validation roundtrip on a synthetic fixture: read ICML, write
// back, verify content is preserved. The real-corpus roundtrip contract
// (RoundTripIcmlIT / IcmlXliffCompareIT) is mapped on the upstream fixtures in
// TestRoundTrip_UpstreamCorpus (upstream_test.go).
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/minimal.icml")
	require.NoError(t, err)
	reader := icml.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/minimal.icml", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := icml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello World")
	assert.Contains(t, output, "Document")
	assert.Contains(t, output, "Story")
}

// Native writer-validation roundtrip with a target translation on a synthetic
// fixture. The real-corpus roundtrip contract is mapped in
// TestRoundTrip_UpstreamCorpus (upstream_test.go).
func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>World</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`

	reader := icml.NewReader()
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
	writer := icml.NewWriter()
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
}

// TestRoundTripWCML tests roundtrip with the Test01.wcml file.
func TestRoundTripWCML(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/Test01.wcml")
	require.NoError(t, err)
	reader := icml.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/Test01.wcml", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := icml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "First paragraph")
	assert.Contains(t, output, "with bold")
	assert.Contains(t, output, "Second paragraph text")
	assert.Contains(t, output, "Third paragraph")
}

// TestRoundTripWCMLMultiContentTranslation translates a block that aggregates
// multiple <Content> elements ("First paragraph with bold"), forcing the legacy
// (non-skeleton) writer down the replaceSequential path. This exercises the N3
// forward-walk splice rewrite and verifies it preserves untranslated content
// while applying the multi-Content replacement.
func TestRoundTripWCMLMultiContentTranslation(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/Test01.wcml")
	require.NoError(t, err)
	reader := icml.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/Test01.wcml", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Translate ONLY the multi-Content block. With no single-Content block
	// translated, the per-Content pass applies nothing and the writer falls
	// through to replaceSequential, which aggregates the two <Content> elements
	// of "First paragraph with bold" into one replacement span.
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block := p.Resource.(*model.Block)
		if block.SourceText() == "First paragraph with bold" {
			block.SetTargetText(model.LocaleFrench, "Premier paragraphe en gras")
		}
	}

	var buf bytes.Buffer
	writer := icml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	// Multi-Content block translation applied (replaceSequential path).
	assert.Contains(t, output, "Premier paragraphe en gras")
	// Untranslated blocks left intact.
	assert.Contains(t, output, "Second paragraph text")
	assert.Contains(t, output, "Third paragraph")
	// Output must remain well-formed: a single forward pass should leave no
	// duplicated or dropped document scaffolding.
	assert.Equal(t, 1, strings.Count(output, "</Document>"))
}

// TestWriterMinimalICML verifies the writer can generate ICML without a skeleton.
func TestWriterMinimalICML(t *testing.T) {
	ctx := t.Context()

	block1 := model.NewBlock("tu1", "Hello world")
	block2 := model.NewBlock("tu2", "Goodbye")

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block1},
		{Type: model.PartBlock, Resource: block2},
	}

	var buf bytes.Buffer
	writer := icml.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "Goodbye")
	assert.Contains(t, output, "<Document")
	assert.Contains(t, output, "<Story>")
	assert.Contains(t, output, "<Content>")
}
