package ttx_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ttx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file ports the upstream Okapi TTXFilterTest (net.sf.okapi.filters.ttx,
// v1.48.0) cases to neokapi's native TTX reader/writer. Each test is annotated
// `// okapi: TTXFilterTest#method` when the native reader/writer reproduces the
// same observable behavior, or `// okapi-skip: …` when the Java case exercises
// an Okapi-only capability that the native reader does not implement.
//
// Snippet constants below mirror the upstream STARTFILE / STARTFILENOLB
// constants. The TRADOStag (.ttx) format places translatable text directly
// inside <Tuv> (there is no <Seg> wrapper — no real Trados file uses one and
// Okapi's TTXFilter reads <Tuv> content directly), so these snippets match the
// genuine format shape exactly as in the Java fixtures.

const startFile = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<TRADOStag Version="2.0"><FrontMatter>` + "\n" +
	`<ToolSettings CreationDate="20070508T094743Z" CreationTool="TRADOS TagEditor" CreationToolVersion="7.0.0.615"></ToolSettings>` + "\n" +
	`<UserSettings DataType="STF" O-Encoding="UTF-8" SettingsName="" SettingsPath="" SourceLanguage="EN-US" TargetLanguage="ES-EM" SourceDocumentPath="abc.rtf" SettingsRelativePath="" PlugInInfo=""></UserSettings>` + "\n" +
	`</FrontMatter><Body><Raw>` + "\n"

const startFileNoLB = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<TRADOStag Version="2.0"><FrontMatter>` + "\n" +
	`<ToolSettings CreationDate="20070508T094743Z" CreationTool="TRADOS TagEditor" CreationToolVersion="7.0.0.615"></ToolSettings>` + "\n" +
	`<UserSettings DataType="STF" O-Encoding="UTF-8" SettingsName="" SettingsPath="" SourceLanguage="EN-US" TargetLanguage="ES-EM" SourceDocumentPath="abc.rtf" SettingsRelativePath="" PlugInInfo=""></UserSettings>` + "\n" +
	`</FrontMatter><Body><Raw>`

// readBlocks runs the native reader over snippet at the given segment mode and
// returns the extracted Blocks. Source locale is EN-US (matching the Java
// FilterTestDriver source locale).
func readBlocks(t *testing.T, snippet string, mode ttx.SegmentMode) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := ttx.NewReader()
	reader.Config().(*ttx.Config).SegmentMode = mode
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(snippet, "EN-US")))
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}

// roundtripOutput reads snippet (with a skeleton store) and writes it back out,
// returning the reconstructed TTX. Mirrors FilterTestDriver.generateOutput with
// source EN-US / target ES-EM.
func roundtripOutput(t *testing.T, snippet string, mode ttx.SegmentMode, escapeGT bool) string {
	t.Helper()
	ctx := t.Context()
	reader := ttx.NewReader()
	reader.Config().(*ttx.Config).SegmentMode = mode
	writer := ttx.NewWriter()
	writer.Config().EscapeGT = escapeGT

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(snippet, "EN-US")))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale("ES-EM")
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// ── Read-side ports ──────────────────────────────────────────────────────

// okapi: TTXFilterTest#testBasicWithEscapes
// The reader resolves the standard XML entities in <Tuv> text: &lt;→<,
// &amp;→&, &gt;→>, &quot;→". Matches Java cont.toString().
func TestReadBasicWithEscapes(t *testing.T) {
	snippet := startFileNoLB +
		`&lt;=lt, &amp;=amp, &gt;=gt, &quot;=quot.` +
		`</Raw></Body></TRADOStag>`
	blocks := readBlocks(t, snippet, ttx.SegmentModeAll)
	require.Len(t, blocks, 1)
	assert.Equal(t, `<=lt, &=amp, >=gt, "=quot.`, blocks[0].SourceText())
}

// okapi: TTXFilterTest#testNoTUContentWithUT
// Inline <ut> placeholder text is folded into the surrounding source text:
// "before <ut>[</ut>in<ut>]</ut> after" → "before [in] after". Matches Java
// cont.toString(). (The native reader does not retain the codes as Spans; the
// Java code-formatted assertion is covered by the skip below.)
func TestReadNoTUContentWithUT(t *testing.T) {
	snippet := startFileNoLB +
		`before <ut Type="start">[</ut>in<ut Type="end">]</ut> after` +
		`</Raw></Body></TRADOStag>`
	blocks := readBlocks(t, snippet, ttx.SegmentModeAll)
	require.Len(t, blocks, 1)
	assert.Equal(t, "before [in] after", blocks[0].SourceText())
}

// okapi: TTXFilterTest#testNoTUContentWithSplitStart
// RightEdge/LeftEdge="split" <ut> elements still contribute their placeholder
// text to the plain source: result "before [ulink={text1}]text2[/ulink] after".
// Matches Java cont.toString().
func TestReadNoTUContentWithSplitStart(t *testing.T) {
	snippet := startFileNoLB +
		`before <ut Type="start" RightEdge="split">[ulink={</ut>text1` +
		`<ut Type="start" LeftEdge="split">}]</ut>text2<ut Type="end">[/ulink]</ut> after` +
		`</Raw></Body></TRADOStag>`
	blocks := readBlocks(t, snippet, ttx.SegmentModeAll)
	require.Len(t, blocks, 1)
	assert.Equal(t, "before [ulink={text1}]text2[/ulink] after", blocks[0].SourceText())
}

// okapi: TTXFilterTest#testBasicNoTUWithDF
// Text wrapped only in a <df> formatting element extracts as the bare text
// "text"; the <df> markup is not part of the translatable content. Matches Java
// cont.toString()=="text".
func TestReadBasicNoTUWithDF(t *testing.T) {
	snippet := startFileNoLB +
		`<df Size="16">text</df>` +
		`</Raw></Body></TRADOStag>`
	blocks := readBlocks(t, snippet, ttx.SegmentModeAll)
	require.Len(t, blocks, 1)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: TTXFilterTest#testSegmentedSurroundedByDF
// A bilingual <Tu> surrounded by a <df> and external <ut> markers extracts to
// one Block with source "en1" and target "es1". With existing-segments-only
// mode the surrounding df/ut becomes skeleton, exactly mirroring Java's
// filterIncUnSeg result for the segmented TU (segments.get(0)=="en1"/"es1").
func TestReadSegmentedSurroundedByDF(t *testing.T) {
	snippet := startFileNoLB +
		`<df Size="16">` +
		`<ut Type="start" Style="external">bc</ut>` +
		`<Tu MatchPercent="0"><Tuv Lang="EN-US">en1</Tuv><Tuv Lang="ES-EM">es1</Tuv></Tu>` +
		`</df>` +
		`<ut Type="end" Style="external">ec</ut>` +
		`</Raw></Body></TRADOStag>`
	blocks := readBlocks(t, snippet, ttx.SegmentModeExistingOnly)
	require.Len(t, blocks, 1)
	assert.Equal(t, "en1", blocks[0].SourceText())
	assert.Equal(t, "es1", blocks[0].TargetText("ES-EM"))
}

// okapi: TTXFilterTest#testSegmentedSurroundedByInternalCodes
// A <Tu> bounded by internal (non-external) <ut> markers extracts to one Block
// whose source is "en1" and target is "es1". Matches Java's source
// segments.get(0).toString()=="en1" (the target's surrounding-code formatting
// "<b1/>[es1]<e1/>" is an Okapi code view the native reader does not model;
// the plain target text "es1" is asserted here).
func TestReadSegmentedSurroundedByInternalCodes(t *testing.T) {
	snippet := startFileNoLB +
		`<ut Type="start">bc</ut>` +
		`<Tu MatchPercent="100"><Tuv Lang="EN-US">en1</Tuv><Tuv Lang="ES-EM">es1</Tuv></Tu>` +
		`<ut Type="end">ec</ut>` +
		`</Raw></Body></TRADOStag>`
	blocks := readBlocks(t, snippet, ttx.SegmentModeExistingOnly)
	require.Len(t, blocks, 1)
	assert.Equal(t, "en1", blocks[0].SourceText())
	assert.Equal(t, "es1", blocks[0].TargetText("ES-EM"))
}

// okapi: TTXFilterTest#testSegmentedAndNot
// In existing-segments-only mode (Java filterNoUnSeg) a snippet mixing free
// text and a bilingual <Tu> yields exactly the segmented TU: source "en1",
// target "es1". Matches Java's "Without un-segmented text" branch. (The Java
// include-unsegmented branch expects one TextUnit with two segments — " text "
// then "en1" — which the native reader does not reproduce, as it emits
// separate Blocks per text run rather than one multi-segment TextUnit.)
func TestReadSegmentedAndNotExistingOnly(t *testing.T) {
	snippet := startFileNoLB +
		`<df Size="16">` +
		`<ut Type="start" Style="external">bc</ut>` +
		` text ` +
		`<Tu MatchPercent="0"><Tuv Lang="EN-US">en1</Tuv><Tuv Lang="ES-EM">es1</Tuv></Tu>` +
		`</df>` +
		`<ut Type="end" Style="external">ec</ut>` +
		`</Raw></Body></TRADOStag>`
	blocks := readBlocks(t, snippet, ttx.SegmentModeExistingOnly)
	require.Len(t, blocks, 1)
	assert.Equal(t, "en1", blocks[0].SourceText())
	assert.Equal(t, "es1", blocks[0].TargetText("ES-EM"))
}

// ── Output-side ports (byte-exact round-trip via the skeleton store) ───────

// okapi: TTXFilterTest#testOutputSimple
// A bilingual <Tu> round-trips byte-exact when the content is unchanged and
// EscapeGT is off (the source's literal '>' is preserved). Matches Java's
// expected==input.
func TestOutputSimple(t *testing.T) {
	snippet := startFile +
		`<Tu MatchPercent="0">` +
		`<Tuv Lang="EN-US">text en >=gt</Tuv>` +
		`<Tuv Lang="ES-EM">text es >=gt</Tuv>` +
		`</Tu>` +
		`</Raw></Body></TRADOStag>`
	assert.Equal(t, snippet, roundtripOutput(t, snippet, ttx.SegmentModeAll, false))
}

// okapi: TTXFilterTest#testOutputSimpleGTEscaped
// With EscapeGT enabled, '>' in the round-tripped <Tuv> content is emitted as
// &gt;. Matches Java's expected output after setEscapeGT(true).
func TestOutputSimpleGTEscaped(t *testing.T) {
	snippet := startFile +
		`<Tu MatchPercent="0">` +
		`<Tuv Lang="EN-US">text en >=gt</Tuv>` +
		`<Tuv Lang="ES-EM">text es >=gt</Tuv>` +
		`</Tu>` +
		`</Raw></Body></TRADOStag>`
	expected := startFile +
		`<Tu MatchPercent="0">` +
		`<Tuv Lang="EN-US">text en &gt;=gt</Tuv>` +
		`<Tuv Lang="ES-EM">text es &gt;=gt</Tuv>` +
		`</Tu>` +
		`</Raw></Body></TRADOStag>`
	assert.Equal(t, expected, roundtripOutput(t, snippet, ttx.SegmentModeAll, true))
}

// okapi: TTXFilterTest#testOutputTUInfo
// A <Tu> carrying Origin and MatchPercent attributes round-trips byte-exact:
// the attributes live in the skeleton and are re-emitted verbatim. Matches
// Java's expected==input.
func TestOutputTUInfo(t *testing.T) {
	snippet := startFileNoLB +
		`<Tu Origin="abc" MatchPercent="50"><Tuv Lang="EN-US">en</Tuv><Tuv Lang="ES-EM">es</Tuv></Tu>` +
		`</Raw></Body></TRADOStag>`
	assert.Equal(t, snippet, roundtripOutput(t, snippet, ttx.SegmentModeAll, false))
}

// okapi: TTXFilterTest#testOutputBasicTwoSegInOneTextUnit
// Two bilingual <Tu> elements separated by whitespace round-trip byte-exact.
// Matches Java's expected==input (snippet).
func TestOutputBasicTwoSegInOneTextUnit(t *testing.T) {
	snippet := startFileNoLB +
		`<Tu MatchPercent="0"><Tuv Lang="EN-US">text1 en</Tuv><Tuv Lang="ES-EM">text1 es</Tuv></Tu>` +
		`  <Tu MatchPercent="0"><Tuv Lang="EN-US">text2 en</Tuv><Tuv Lang="ES-EM">text2 es</Tuv></Tu>` +
		`</Raw></Body></TRADOStag>`
	assert.Equal(t, snippet, roundtripOutput(t, snippet, ttx.SegmentModeAll, false))
}

// okapi: TTXFilterTest#testOutputSegmentedSurroundedByDF
// A bilingual <Tu> wrapped in a <df> and external <ut> markers round-trips
// byte-exact: only the <Tuv> content is filled from the Block, the surrounding
// markup stays in the skeleton. Matches Java's expected==input.
func TestOutputSegmentedSurroundedByDF(t *testing.T) {
	snippet := startFileNoLB +
		`<df Size="16">` +
		`<ut Type="start" Style="external">bc</ut>` +
		`<Tu MatchPercent="0"><Tuv Lang="EN-US">en1</Tuv><Tuv Lang="ES-EM">es1</Tuv></Tu>` +
		`</df>` +
		`<ut Type="end" Style="external">ec</ut>` +
		`</Raw></Body></TRADOStag>`
	assert.Equal(t, snippet, roundtripOutput(t, snippet, ttx.SegmentModeAll, false))
}

// okapi: TTXFilterTest#testOutputTwoTU
// Two bilingual <Tu> elements separated by an external <ut> and whitespace
// round-trip byte-exact; the external code run is retained verbatim in the
// skeleton and the correct Block fills each <Tu> (regression test for the
// skeleton-ref/emission-index alignment fix). Matches Java's expected==input.
func TestOutputTwoTU(t *testing.T) {
	snippet := startFile +
		`<Tu MatchPercent="0">` +
		`<Tuv Lang="EN-US">text1 en</Tuv><Tuv Lang="ES-EM">text1 es</Tuv></Tu>` + "\n" +
		`  <ut Style="external">some code</ut>  ` +
		`<Tu MatchPercent="0"><Tuv Lang="EN-US">text2 en</Tuv><Tuv Lang="ES-EM">text2 es</Tuv></Tu>` + "\n" +
		`</Raw></Body></TRADOStag>`
	assert.Equal(t, snippet, roundtripOutput(t, snippet, ttx.SegmentModeAll, false))
}

// ── Skips: Okapi-only capabilities the native TTX reader does not implement ──
//
// The native reader is a single-pass text extractor: it does not model the
// `Style="external"` attribute (so external <ut>/<df> are not skeleton
// boundaries), it does not preserve inline <ut>/<df>/<it> codes as Spans, it
// emits one Block per <Tu> (rather than Okapi's merge-all-<Tu>-within-one-<Raw>
// into a single multi-segment TextUnit), and it has no AltTranslations
// annotation, forced-segmentation, or source→target-copy behavior. The cases
// below depend on one or more of those Okapi-only behaviors.

// okapi-skip: TTXFilterTest#testBasicNoExtractableData — native does not honor Style="external"; it extracts the external <ut> text as a Block instead of producing no TextUnit.
// okapi-skip: TTXFilterTest#testOutputNoExtractableData — no-<Tu> input yields an empty skeleton (the writer's skeleton path emits nothing when no Tu refs exist); cannot reproduce output==input.
// okapi-skip: TTXFilterTest#testBasicNoTU — Okapi keeps the leading newline ("\ntext") and a present-but-empty target; native trims leading whitespace and models no empty target.
// okapi-skip: TTXFilterTest#testForOneTU — depends on Style="external" boundary handling (the external <li> <ut> is dropped from the source); native includes it as "<li>...</li>".
// okapi-skip: TTXFilterTest#testForOneTUWithTextParts — Okapi merges the run into one TextUnit with 4 TextParts; native emits separate Blocks per <Tu>/text run.
// okapi-skip: TTXFilterTest#testForTwoTUs — external <ut> splits into two TextUnits in Okapi; native does not treat external <ut> as a boundary and merges into one Block.
// okapi-skip: TTXFilterTest#testForExternalDF — depends on Style="external" handling; native concatenates the external code text into the source ("codetextcode").
// okapi-skip: TTXFilterTest#testNoTUEndsWithUT — Okapi drops the trailing procinstr <ut>; native folds its text into the source ("textpi").
// okapi-skip: TTXFilterTest#testWithPINoTU — Okapi drops the leading procinstr <ut>; native folds its text into the source ("pitext").
// okapi-skip: TTXFilterTest#testVariousTags — assertions are over preserved inline codes (printSegmentedContent "<1/><2>text<3/></2>"); native strips codes and keeps display text only.
// okapi-skip: TTXFilterTest#testVariousTagsWithSegmentation — asserts inline Code ids/types and per-segment code counts; native does not model inline codes.
// okapi-skip: TTXFilterTest#testNotSegmentedWithDF — asserts code-formatted segmented content ("[src text]" after moving </df> out); depends on df/external code handling native lacks.
// okapi-skip: TTXFilterTest#testNotSegmentedWithDFAndCodes — asserts preserved inline codes ("[<1>src text</1>]"); native strips codes.
// okapi-skip: TTXFilterTest#testStartingExtraDF — asserts inline Code structure and XLIFFContent (<ph>/<bpt>/<ept>); native does not model codes.
// okapi-skip: TTXFilterTest#testTUInfo — asserts an AltTranslationsAnnotation (origin/score/MatchType FUZZY); native surfaces MatchPercent only and has no AltTranslations model.
// okapi-skip: TTXFilterTest#testTUInfoXU — asserts an AltTranslationsAnnotation (xtranslate / EXACT_LOCAL_CONTEXT); native has no AltTranslations model.
// okapi-skip: TTXFilterTest#testWithMixedSegmentation — Okapi builds one TextUnit with two segments and an AltTranslation on the 50% match; native emits two separate Blocks and no annotation.
// okapi-skip: TTXFilterTest#testBasicTwoSegInOneTextUnit — Okapi merges two whitespace-separated <Tu> into one TextUnit with two segments; native emits one Block per <Tu>.
// okapi-skip: TTXFilterTest#testPartiallySegmentedEntry — Okapi produces one TextUnit segmented "[Outside1 ][Inside][ Outside2]"; native emits three separate Blocks (and trims segment whitespace).
// okapi-skip: TTXFilterTest#testPartiallySegmentedEntryAfter — depends on one-TextUnit segmentation plus Style="external" handling; native emits separate Blocks including the external [z] code text.
// okapi-skip: TTXFilterTest#testPartiallySegmentedEntryNothingTranslatable — Okapi yields one segmented TextUnit; native emits three separate Blocks.
// okapi-skip: TTXFilterTest#testLargePartiallySegmentedEntry — Okapi yields one TextUnit with five segments; native emits five separate Blocks.
// okapi-skip: TTXFilterTest#testOutputBasicWithEscapes — requires forced segmentation of free text into a <Tu> wrapper; native does not force-segment.
// okapi-skip: TTXFilterTest#testOutputBasicNoTUWithSegmentation — requires forced segmentation of free text into a <Tu>; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputBasicNoTUWithDFWithSegementation — requires forced segmentation of <df>-wrapped text into a <Tu>; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputEscapesInSkeleton — requires forced segmentation of free text into a <Tu> (with the long DisplayText code preserved); not implemented natively.
// okapi-skip: TTXFilterTest#testOutputForExternalDFwithSegmentation — requires forced segmentation plus Style="external" boundary handling; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputForTwoTUsWithSegmentation — requires forced segmentation of two free-text runs into <Tu> wrappers; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputNoTUContentWithUTWithSegmentation — requires forced segmentation of free text (with inline <ut> codes preserved); not implemented natively.
// okapi-skip: TTXFilterTest#testOutputNotSegmentedWithDF_ForcingOutSeg — requires forced output segmentation with the </df> moved outside the new <Tu>; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputNotSegmentedWithLeadingWS — requires forced segmentation that keeps the leading whitespace in the skeleton and wraps only "text"; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputPartiallySegmentedEntryAfter — requires forced segmentation of the trailing free text into a second <Tu>; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputStartingExtraDFWithSegmentation — requires forced segmentation with df re-balancing; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputVariousTagsWithSegmentation — requires forced segmentation with inline codes preserved; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputWithMixedSegmentation — requires forced segmentation of the trailing free text and target back-fill from the source; not implemented natively.
// okapi-skip: TTXFilterTest#testOutputWithOriginalWithoutTraget — Okapi copies source→target when a <Tu> has only a source <Tuv>; native does not synthesize a target.
// okapi-skip: TTXFilterTest#testOutputWithPINoTUWithSegmentation — requires forced segmentation of free text after a procinstr <ut>; not implemented natively.
// okapi-skip: TTXFilterTest#textDoubleExtractionOriginalAllSegmented — byte-exact round-trip of a real all-segmented TTX file requires preserving inline <ut>/<df> codes inside <Tuv>; native strips them, so the round-trip is not byte-identical.
// okapi-skip: RoundTripTtxIT#ttxSerializedFiles — Okapi serialized-skeleton variant; native uses its own skeleton store
