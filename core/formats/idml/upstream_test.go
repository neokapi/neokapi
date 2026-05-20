package idml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Upstream-fixture-backed tests
//
// These tests run the native IDML reader against the SAME real .idml files
// the upstream Okapi (Java) filter tests use, fetched into okapi-testdata/ via
// scripts/fetch-okapi-testdata.sh (the `okapi:` fixture scheme). They assert
// the same OBSERVABLE behavior the Java tests assert, expressed in neokapi's
// content model (one Block per <Content> element, plain source text).
//
// Important model difference, relevant to which Okapi tests are mappable:
// Okapi groups a whole paragraph into ONE TextUnit and represents inline
// CharacterStyleRange / Br / conditional-text boundaries as coded-text inline
// codes (…). neokapi's native reader instead emits ONE translatable
// Block per <Content> element with plain text and no inline coded-text
// markers. Okapi tests whose assertions hinge on that TextUnit-with-inline-
// codes shape (getCodedText() with \uE1xx markers, paragraph-level merge
// counts, toText() <content-N> placeholder serialization) therefore probe a
// segmentation model the native reader does not implement; those are
// classified // okapi-skip below with the architectural reason. Tests whose
// observable contract survives translation into the per-Content block model
// (presence/absence of extracted text, block deltas, geometry robustness,
// round-trip stability, thread safety) are mapped here.
// ---------------------------------------------------------------------------

// loadUpstreamIDML reads a real upstream IDML fixture from the okapi-testdata
// tree. It skips (rather than fails) when the corpus has not been fetched, so
// the package still builds and tests in environments without the binary
// fixtures (mirrors TestUpstreamHelloWorld14 in reader_test.go).
func loadUpstreamIDML(t *testing.T, name string) []byte {
	t.Helper()
	root, err := spec.FindOkapiTestdataRoot()
	if err != nil {
		t.Skipf("skipping upstream IDML fixture test: %v", err)
	}
	fixture := filepath.Join(root, "okapi", "filters", "idml", "src",
		"test", "resources", name)
	data, err := os.ReadFile(fixture)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("skipping: fixture not present at %s", fixture)
		}
		require.NoError(t, err)
	}
	return data
}

// ---------------------------------------------------------------------------
// Reviewed-not-applicable upstream tests (// okapi-skip)
//
// The native IDML reader segments per <Content> element with plain source
// text. Okapi segments per paragraph into one TextUnit and encodes inline
// CharacterStyleRange / Br / conditional-text / kerning-merge boundaries as
// coded-text inline codes (\uE1xx), or serializes referent/inline placeholders
// via toText() (<content-N>). The native reader also implements only six
// boolean Config flags; it has no Parameters serialization, no character-style
// ignorance thresholds, no code finder, no special-character pattern, no
// font mappings, no style-exclusion configuration, and no inline-break-code or
// hyperlink-URL extraction modes. The tests below assert exactly those Okapi
// TextUnit-with-inline-codes shapes or those unimplemented Parameters
// features, so they probe behavior the native reader/writer does not model.
// Each is classified not-applicable with the specific reason.
// ---------------------------------------------------------------------------

// --- ExtractionTest: character-style "ignorance threshold" tag merging ---
// These assert getCodedText() with \uE1xx inline codes (or merged paragraph
// TextUnits) produced by the ignoreCharacterKerning/Tracking/Leading/
// BaselineShift Parameters and their min/max thresholds. The native reader has
// neither the inline-code TextUnit model nor those Parameters, so it splits at
// every CharacterStyleRange boundary regardless of kerning/tracking/etc.
//
// okapi-skip: ExtractionTest#doesNotMergeTagsThatDifferByKerning — native splits per <Content>; no inline-code TextUnit model or ignorance thresholds
// okapi-skip: ExtractionTest#mergesTagsThatDifferByKerningWithEmptyIgnoranceThresholds — requires ignoreCharacterKerning Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByKerningWithMinIgnoranceThreshold — requires kerning min-threshold Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByKerningWithMaxIgnoranceThreshold — requires kerning max-threshold Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByKerningWithMinAndMaxIgnoranceThresholds — requires kerning min/max-threshold Parameters + inline codes; not in native model
// okapi-skip: ExtractionTest#doesNotMergeTagsThatDifferByTracking — native splits per <Content>; no inline-code TextUnit model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByTrackingWithEmptyIgnoranceThresholds — requires ignoreCharacterTracking Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByTrackingWithMinIgnoranceThreshold — requires tracking min-threshold Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByTrackingWithMaxIgnoranceThreshold — requires tracking max-threshold Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByTrackingWithMinAndMaxIgnoranceThresholds — requires tracking min/max-threshold Parameters + inline codes; not in native model
// okapi-skip: ExtractionTest#doesNotMergeTagsThatDifferByLeading — native splits per <Content>; no inline-code TextUnit model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByLeadingWithoutIgnoranceThresholds — requires ignoreCharacterLeading Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByLeadingWithMinIgnoranceThreshold — requires leading min-threshold Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByLeadingWithMaxIgnoranceThreshold — requires leading max-threshold Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByLeadingWithMinAndMaxIgnoranceThresholds — requires leading min/max-threshold Parameters + inline codes; not in native model
// okapi-skip: ExtractionTest#doesNotMergeTagsThatDifferByBaselineShift — native splits per <Content>; no inline-code TextUnit model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByBaselineShiftWithoutIgnoranceThresholds — requires ignoreCharacterBaselineShift Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByBaselineShiftWithMinIgnoranceThreshold — requires baseline-shift min-threshold Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByBaselineShiftWithMaxIgnoranceThreshold — requires baseline-shift max-threshold Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByBaselineShiftWithMinAndMaxIgnoranceThresholds — requires baseline-shift min/max-threshold Parameters + inline codes; not in native model
// okapi-skip: ExtractionTest#doesNotMergeTagsThatDifferByKerningMethod — native splits per <Content>; no inline-code TextUnit model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByKerningMethod — requires ignoreCharacterKerning Parameter + inline codes; not in native model
// okapi-skip: ExtractionTest#doesNotMergeTagsThatDifferByKerningInReferencesAndXmlStructures — asserts getCodedText() inline codes across reference/XML structures; not in native per-<Content> model
// okapi-skip: ExtractionTest#mergesTagsThatDifferByKerningInReferencesAndXmlStructures — requires ignoreCharacterKerning Parameter + inline codes across references/XML structures; not in native model

// --- ExtractionTest: features needing Parameters / inline-code assertions ---
// okapi-skip: ExtractionTest#extractsBreaksInline — requires extractBreaksInline Parameter; native always splits per paragraph and has no inline  break-code model
// okapi-skip: ExtractionTest#customTextVariablesExtracted — requires setExtractCustomTextVariables Parameter; native has no such config flag
// okapi-skip: ExtractionTest#indexTopicsExtracted — requires setExtractIndexTopics Parameter; native has no such config flag
// okapi-skip: ExtractionTest#mathZonesConditionallyExtracted — requires setExtractMathZones Parameter and asserts toText() <content-N> placeholders; not in native model
// okapi-skip: ExtractionTest#stylesExcluded — requires excludedStyleConfigurations Parameter API; not implemented in native Config
// okapi-skip: ExtractionTest#specialCharacterPatternApplied — requires specialCharacterPattern Parameter producing Code objects + toText(); not in native model
// okapi-skip: ExtractionTest#codeFinderApplied — requires setUseCodeFinder/setCodeFinderData Parameters; native has no code finder
// okapi-skip: ExtractionTest#adjacentCodesMerged — requires mergeAdjacentCodes + codeFinder + specialCharacterPattern Parameters and inline codes; not in native model
// okapi-skip: ExtractionTest#externalHyperlinksExtracted — requires setExtractExternalHyperlinks Parameter (extracts hyperlink destination URLs as TextUnits); native does not extract URLs
// okapi-skip: ExtractionTest#hyperlinkTextSourcesExtractedAsReferenceGroups — asserts toText() <content-N> referent-group serialization; native uses per-<Content> blocks, not Okapi's referent TextUnits
// okapi-skip: ExtractionTest#hyperlinkTextSourcesExtractedInline — asserts toText() inline <content-N> placeholder serialization; native uses per-<Content> blocks, not Okapi's inline-code TextUnits
// okapi-skip: ExtractionTest#extractsWithLeastAvailableStyleFormattingBaselined — asserts 9 paragraph TextUnits with inline style codes; native emits per-<Content> blocks (25) with no inline codes
// okapi-skip: ExtractionTest#pasteboardItemsWithoutAnchorPointsPositionedCorrectly — asserts anchor-point geometry reordering of TextUnits; native uses sorted story-file order (story-ordering geometry subsystem not native)

// --- ParametersTest: Okapi Parameters Java API internals ---
// The native reader exposes a six-flag Config (no Okapi Parameters string
// serialization, no threshold setters/getters, no StyleIgnorances/
// fontMappings/excludedStyleConfigurations). Every test here exercises that
// Java API surface, which has no native counterpart.
//
// okapi-skip: ParametersTest#initialisesDefaultParameters — asserts Okapi Parameters.toString() serialization format; native Config has no such serialization
// okapi-skip: ParametersTest#initialisesStyleIgnorances — exercises Okapi StyleIgnorances/Namespaces API; no native equivalent
// okapi-skip: ParametersTest#setsCharacterKerningMinIgnoranceThreshold — Okapi Parameters threshold setter/getter API; not in native Config
// okapi-skip: ParametersTest#failsToSetCharacterKerningMinIgnoranceThreshold — Okapi Parameters NumberFormatException validation; not in native Config
// okapi-skip: ParametersTest#setsCharacterKerningMaxIgnoranceThreshold — Okapi Parameters threshold setter/getter API; not in native Config
// okapi-skip: ParametersTest#failsToSetCharacterKerningMaxIgnoranceThreshold — Okapi Parameters NumberFormatException validation; not in native Config
// okapi-skip: ParametersTest#setsCharacterTrackingMinIgnoranceThreshold — Okapi Parameters threshold setter/getter API; not in native Config
// okapi-skip: ParametersTest#failsToSetCharacterTrackingMinIgnoranceThreshold — Okapi Parameters NumberFormatException validation; not in native Config
// okapi-skip: ParametersTest#setsCharacterTrackingMaxIgnoranceThreshold — Okapi Parameters threshold setter/getter API; not in native Config
// okapi-skip: ParametersTest#failsToSetCharacterTrackingMaxIgnoranceThreshold — Okapi Parameters NumberFormatException validation; not in native Config
// okapi-skip: ParametersTest#setsCharacterLeadingMinIgnoranceThreshold — Okapi Parameters threshold setter/getter API; not in native Config
// okapi-skip: ParametersTest#failsToSetCharacterLeadingMinIgnoranceThreshold — Okapi Parameters NumberFormatException validation; not in native Config
// okapi-skip: ParametersTest#setsCharacterLeadingMaxIgnoranceThreshold — Okapi Parameters threshold setter/getter API; not in native Config
// okapi-skip: ParametersTest#failsToSetCharacterLeadingMaxIgnoranceThreshold — Okapi Parameters NumberFormatException validation; not in native Config
// okapi-skip: ParametersTest#setsCharacterBaselineShiftMinIgnoranceThreshold — Okapi Parameters threshold setter/getter API; not in native Config
// okapi-skip: ParametersTest#failsToSetCharacterBaselineShiftMinIgnoranceThreshold — Okapi Parameters NumberFormatException validation; not in native Config
// okapi-skip: ParametersTest#setsCharacterBaselineShiftMaxIgnoranceThreshold — Okapi Parameters threshold setter/getter API; not in native Config
// okapi-skip: ParametersTest#failsToSetCharacterBaselineShiftMaxIgnoranceThreshold — Okapi Parameters NumberFormatException validation; not in native Config
// okapi-skip: ParametersTest#excludedStyleConfigurationsInitialised — Okapi excludedStyleConfigurations Parameters API; not in native Config
// okapi-skip: ParametersTest#fontMappingsAreInitialised — Okapi fontMappings Parameters API; not in native Config

// --- RoundTripTest: round-trips gated on unimplemented Parameters ---
// These compare a translated package byte-for-byte against a gold file that was
// produced WITH a specific Parameter applied. The native reader cannot apply
// those Parameters (code finder, custom text variables, index topics, math-zone
// suppression, style exclusion, external hyperlink extraction, font mappings),
// so the gated round-trip has no native counterpart.
//
// okapi-skip: RoundTripTest#adjacentCodesMergeSupported — gold produced with mergeAdjacentCodes + codeFinder + specialCharacterPattern Parameters; not in native model
// okapi-skip: RoundTripTest#customTextVariablesExtractedAndMerged — gold produced with extractCustomTextVariables Parameter; not in native Config
// okapi-skip: RoundTripTest#indexTopicsExtractedAndMerged — gold produced with extractIndexTopics Parameter; not in native Config
// okapi-skip: RoundTripTest#mathZonesConditionalExtractionSupported — gold produced with extractMathZones=false Parameter; not in native Config
// okapi-skip: RoundTripTest#stylesExclusionSupported — gold produced with excludedStyleConfigurations Parameter; not in native Config
// okapi-skip: RoundTripTest#externalHyperlinksExtractedAndMerged — gold produced with extractExternalHyperlinks Parameter; not in native Config
// okapi-skip: RoundTripTest#documentWithChainedFontMappings — gold produced with fontMappings Parameters; not in native Config
// okapi-skip: RoundTripTest#fontMappingForNamesWithProcessingInstructionsSupported — gold produced with fontMappings Parameters; not in native Config

// ---------------------------------------------------------------------------
// ExtractionTest equivalents (mapped on real fixtures)
// ---------------------------------------------------------------------------

// okapi: ExtractionTest#testObjectsWithoutPathPointsAndText
//
// 618-objects-without-path-points-and-text.idml contains spread items whose
// geometry lacks <PathPointArray>/<PathPointType> and which carry no
// translatable text. Upstream asserts 0 TextUnits; the native reader must
// likewise parse the missing-geometry document without error and extract no
// blocks. Guards the geometry parser against a nil-path-points crash.
func TestExtraction_ObjectsWithoutPathPointsAndText(t *testing.T) {
	data := loadUpstreamIDML(t, "618-objects-without-path-points-and-text.idml")
	blocks := testutil.FilterBlocks(readIDMLBytes(t, data))
	assert.Empty(t, blocks,
		"objects without path points and no text should yield no translatable blocks")
}

// okapi: ExtractionTest#testAnchoredFrameWithoutPathPoints
//
// 618-anchored-frame-without-path-points.idml has an anchored TextFrame whose
// geometry has no path points. Upstream asserts the anchored frame's text
// ("Anchored") is still extracted (its TextUnit at the Okapi-ordered index 4).
// neokapi's per-Content block model and story-file ordering place it at a
// different index, so we assert the observable contract that matters: the
// anchored-frame text survives the missing-geometry parse and is extracted.
func TestExtraction_AnchoredFrameWithoutPathPoints(t *testing.T) {
	data := loadUpstreamIDML(t, "618-anchored-frame-without-path-points.idml")
	blocks := testutil.FilterBlocks(readIDMLBytes(t, data))
	texts := testutil.BlockTexts(blocks)
	require.NotEmpty(t, blocks)
	assert.Contains(t, texts, "Anchored",
		"anchored frame text must be extracted despite missing path points")
}

// okapi: ExtractionTest#testDocumentWithoutPathPoints
//
// 618-MBE3.idml is a whole document whose spreads omit path points. Upstream
// asserts the first TextUnit is "Fashion Industry In Colombia". The native
// reader emits this as the first block (the document's first paragraph is a
// single <Content>), so the index aligns here.
func TestExtraction_DocumentWithoutPathPoints(t *testing.T) {
	data := loadUpstreamIDML(t, "618-MBE3.idml")
	blocks := testutil.FilterBlocks(readIDMLBytes(t, data))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Fashion Industry In Colombia", blocks[0].SourceText())
}

// okapi: ExtractionTest#endNotesExtracted
//
// 856-1.idml carries an endnote. Upstream asserts the endnote body text
// "Endonote for sentence 2" appears in the extracted content (within a
// TextUnit that also carries inline break/anchor codes at Okapi index 5).
// The native reader extracts endnote <Content> as ordinary translatable
// blocks by default, so we assert the endnote body text is present (the
// surrounding \uE1xx break codes are part of Okapi's inline-code model, not
// the native per-Content block model).
func TestExtraction_EndNotesExtracted(t *testing.T) {
	data := loadUpstreamIDML(t, "856-1.idml")
	texts := testutil.BlockTexts(testutil.FilterBlocks(readIDMLBytes(t, data)))
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Endonote for sentence 2") {
			found = true
			break
		}
	}
	assert.True(t, found, "endnote body text must be extracted by default")
}

// okapi: ExtractionTest#hiddenPasteboardItemsExtracted
//
// 1016.idml contains stories on hidden pasteboard items. Upstream asserts the
// default extraction yields 533 TextUnits and that enabling
// extractHiddenPasteboardItems yields 545 (delta +12), with the specific
// hidden texts becoming visible. neokapi's per-Content block model produces
// different absolute counts (576 / 588), but the OBSERVABLE contract that the
// flag governs is identical: enabling extractHiddenPasteboardItems reveals
// exactly 12 additional blocks, including the hidden-pasteboard texts upstream
// names. We assert that delta and the presence of those texts.
func TestExtraction_HiddenPasteboardItemsExtracted(t *testing.T) {
	data := loadUpstreamIDML(t, "1016.idml")

	def := testutil.FilterBlocks(readIDMLBytes(t, data))

	cfg := &Config{}
	cfg.Reset()
	cfg.ExtractHiddenPasteboardItems = true
	withHidden := testutil.FilterBlocks(readIDMLBytesWithConfig(t, data, cfg))

	require.Greater(t, len(withHidden), len(def),
		"enabling extractHiddenPasteboardItems must reveal more blocks")
	assert.Equal(t, 12, len(withHidden)-len(def),
		"extractHiddenPasteboardItems must reveal exactly the hidden pasteboard stories (delta matches upstream 545-533)")

	texts := testutil.BlockTexts(withHidden)
	for _, want := range []string{
		"Lighting your grill",
		"Is it the very first time? Perform a Burn-off ",
		"Side Burner Lighting (if equipped) ",
	} {
		assert.Contains(t, texts, want,
			"hidden pasteboard text must appear when extractHiddenPasteboardItems=true")
	}
}

// ---------------------------------------------------------------------------
// RoundTripTest equivalents (mapped on real fixtures)
//
// Okapi's RoundTripTest uses RoundTripComparison / ZipXMLFileCompare: it
// extracts, writes, and re-extracts (or byte-compares the package), asserting
// the translatable surface is preserved across a read→write→read cycle. The
// native analog is double-extraction stability: extract a fixture, write it
// back through the skeleton-based writer, re-extract the produced package, and
// assert the block sequence is identical. This exercises the real reader and
// writer end-to-end on the same upstream binary fixtures.
// ---------------------------------------------------------------------------

// roundTripExtractStable reads fixtureName with the given config, writes it
// back out through the skeleton writer (no translation), re-reads the output,
// and returns (firstExtractionTexts, secondExtractionTexts).
func roundTripExtractStable(t *testing.T, data []byte, cfg *Config) ([]string, []string) {
	t.Helper()
	ctx := t.Context()

	skel, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skel.Close()

	reader := NewReader()
	if cfg != nil {
		reader.cfg = cfg
		reader.Cfg = cfg
	}
	reader.SetSkeletonStore(skel)
	require.NoError(t, reader.Open(ctx, &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		MimeType:     "application/vnd.adobe.indesign-idml-package",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	first := testutil.BlockTexts(testutil.FilterBlocks(parts))

	var buf bytes.Buffer
	writer := NewWriter()
	if cfg != nil {
		writer.cfg = cfg
	}
	writer.SetSkeletonStore(skel)
	writer.SetOriginalContent(data)
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleEnglish)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))

	second := testutil.BlockTexts(testutil.FilterBlocks(readIDMLBytesWithConfig(t, buf.Bytes(), defaultableConfig(cfg))))
	return first, second
}

func defaultableConfig(cfg *Config) *Config {
	if cfg != nil {
		return cfg
	}
	c := &Config{}
	c.Reset()
	return c
}

// assertRoundTripStable runs the read→write→read cycle and asserts the block
// sequence is byte-identical across both extractions.
func assertRoundTripStable(t *testing.T, fixtureName string, cfg *Config) {
	t.Helper()
	data := loadUpstreamIDML(t, fixtureName)
	first, second := roundTripExtractStable(t, data, cfg)
	require.NotEmpty(t, first, "fixture %s should extract at least one block", fixtureName)
	assert.Equal(t, first, second,
		"round-trip (read→write→read) must preserve the translatable block sequence for %s", fixtureName)
}

// okapi: RoundTripTest#documentsWithDefaultParameters
func TestRoundTrip_DocumentsWithDefaultParameters(t *testing.T) {
	assertRoundTripStable(t, "926.idml", nil)
}

// okapi: RoundTripTest#specialCharactersExtractedAndMerged
func TestRoundTrip_SpecialCharactersExtractedAndMerged(t *testing.T) {
	assertRoundTripStable(t, "175-special-characters.idml", nil)
}

// okapi: RoundTripTest#endNotesExtractedAndMerged
func TestRoundTrip_EndNotesExtractedAndMerged(t *testing.T) {
	assertRoundTripStable(t, "856-1.idml", nil)
	assertRoundTripStable(t, "856-2.idml", nil)
}

// okapi: RoundTripTest#emptyContentStylesPreserved
//
// Upstream verifies an empty-paragraph document (1369-*) round-trips with its
// empty content/styles intact. The native analog: extract → write → re-extract
// is stable, including the zero-block and single-block empty-paragraph cases.
func TestRoundTrip_EmptyContentStylesPreserved(t *testing.T) {
	// 1369-empty-paragraph-styles.idml has only empty paragraphs (0 blocks);
	// assert the write path copies it through and re-extraction is still 0.
	data := loadUpstreamIDML(t, "1369-empty-paragraph-styles.idml")
	first, second := roundTripExtractStable(t, data, nil)
	assert.Empty(t, first, "empty-paragraph document should extract no blocks")
	assert.Equal(t, first, second, "empty-paragraph round-trip must stay empty")

	assertRoundTripStable(t, "1369-empty-paragraph-in-table-cell-styles.idml", nil)
}

// okapi: RoundTripTest#hyperlinkTextSourcesExtractedAndMerged
//
// Upstream round-trips the hyperlink-text-source documents (1179-*,
// 03-hyperlink-and-table-content) with extractHyperlinkTextSourcesInline both
// false and true, asserting byte-level package equality each way. The native
// reader supports both modes via Config.ExtractHyperlinkTextSourcesInline; we
// assert read→write→read stability for both flag values across the fixtures.
func TestRoundTrip_HyperlinkTextSourcesExtractedAndMerged(t *testing.T) {
	fixtures := []string{
		"1179-0.idml",
		"1179-1.idml",
		"1179-2.idml",
		"1179-3.idml",
		"1179-4.idml",
		"03-hyperlink-and-table-content.idml",
	}
	for _, inline := range []bool{false, true} {
		for _, fx := range fixtures {
			cfg := &Config{}
			cfg.Reset()
			cfg.ExtractHyperlinkTextSourcesInline = inline
			assertRoundTripStable(t, fx, cfg)
		}
	}
}

// okapi: RoundTripTest#emptyTargetsMerged
//
// Upstream sets EMPTY targets on the "Second paragraph." and "Last paragraph."
// TextUnits of 629.idml, writes the package, and compares it byte-for-byte to a
// gold file in which those paragraphs are emptied. The observable contract: a
// block written with an empty target produces empty content (the source text is
// NOT carried through). We reproduce this with the native reader/writer — set
// empty French targets on those two paragraphs and a non-empty target on the
// first, write, then re-extract: only the translated first paragraph remains.
func TestRoundTrip_EmptyTargetsMerged(t *testing.T) {
	data := loadUpstreamIDML(t, "629.idml")
	ctx := t.Context()

	skel, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skel.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skel)
	require.NoError(t, reader.Open(ctx, &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// 629.idml extracts three paragraphs.
	srcTexts := testutil.BlockTexts(testutil.FilterBlocks(parts))
	require.ElementsMatch(t,
		[]string{"Fist paragraph.", "Second paragraph.", "Last paragraph."},
		srcTexts)

	const translatedFirst = "Première."
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block, ok := p.Resource.(*model.Block)
		if !ok {
			continue
		}
		switch block.SourceText() {
		case "Second paragraph.", "Last paragraph.":
			block.SetTargetText(model.LocaleFrench, "") // empty target
		case "Fist paragraph.":
			block.SetTargetText(model.LocaleFrench, translatedFirst)
		}
	}

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetSkeletonStore(skel)
	writer.SetOriginalContent(data)
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleFrench)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))

	// Re-extract: the empty-target paragraphs produced empty content (no
	// block survives); only the translated first paragraph remains.
	out := testutil.BlockTexts(testutil.FilterBlocks(readIDMLBytes(t, buf.Bytes())))
	assert.Equal(t, []string{translatedFirst}, out,
		"empty targets must write empty content; only the non-empty target survives re-extraction")
}

// ---------------------------------------------------------------------------
// IDMLFilterInParallelTest equivalent
// ---------------------------------------------------------------------------

// okapi: IDMLFilterInParallelTest#testInMultipleThreads
//
// Upstream runs the filter over TextPathTest04.idml from 10 threads × 2 rounds
// to surface shared-state races in the filter. The native reader holds no
// cross-Open shared state, but we exercise the same fixture under the same
// concurrency to assert thread safety: every concurrent extraction yields the
// identical block sequence and the race detector stays quiet (run via
// `go test -race`).
func TestParallel_InMultipleThreads(t *testing.T) {
	data := loadUpstreamIDML(t, "TextPathTest04.idml")

	want := testutil.BlockTexts(testutil.FilterBlocks(readIDMLBytes(t, data)))
	require.NotEmpty(t, want, "TextPathTest04.idml should extract blocks")

	const (
		threads = 10
		rounds  = 2
	)
	var wg sync.WaitGroup
	errs := make(chan error, threads*rounds)
	for range threads {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				got := runOnce(t, data)
				if len(got) != len(want) {
					errs <- fmt.Errorf("block count %d != %d", len(got), len(want))
					continue
				}
				for i := range want {
					if got[i] != want[i] {
						errs <- fmt.Errorf("block %d: %q != %q", i, got[i], want[i])
						break
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// runOnce performs a single concurrent-safe extraction. It uses a fresh
// Reader per call (the documented usage) and returns the block texts.
func runOnce(t *testing.T, data []byte) []string {
	ctx := t.Context()
	reader := NewReader()
	if err := reader.Open(ctx, &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}); err != nil {
		return nil
	}
	defer reader.Close()
	return testutil.BlockTexts(testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx))))
}
