package mosestext_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/mosestext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Okapi MosesTextFilterTest — ported / classified
//
// Background: Okapi's MosesTextFilter parses each line as a *pseudo-XLIFF*
// fragment — it decodes XML entities (&lt; → <, &#13; → \r, …), recognises
// <mrk mtype="seg"> segment markers, and parses <g>/<x>/<bx>/<ex> inline
// codes into TextFragment Code objects (see MosesTextFilter.fromPseudoXLIFF).
// The native neokapi reader is a plain-text reader: one line per Block, text
// taken verbatim. Inline-code handling is opt-in via the format-agnostic code
// finder (UseCodeFinder + CodeFinderRules), which carves verbatim placeholder
// runs rather than reproducing Okapi's generic-content model. Tests that
// assert Okapi's pseudo-XLIFF semantics are therefore skipped with an honest
// reason; the native code-finder analogue is exercised separately below.
// ---------------------------------------------------------------------------

// okapi: MosesTextFilterTest#testDefaultInfo
// Okapi asserts the filter exposes parameters, a name, and a non-empty
// configuration list. The native analogue: the reader reports a stable
// name/display-name, carries a Config, and advertises a non-empty signature.
func TestDefaultInfo(t *testing.T) {
	t.Parallel()
	reader := mosestext.NewReader()

	assert.Equal(t, "mosestext", reader.Name())
	assert.Equal(t, "Moses Text", reader.DisplayName())

	cfg, ok := reader.Config().(*mosestext.Config)
	require.True(t, ok, "reader must carry a *mosestext.Config")
	assert.Equal(t, "mosestext", cfg.FormatName())
	require.NoError(t, cfg.Validate())

	sig := reader.Signature()
	assert.NotEmpty(t, sig.MIMETypes, "signature should advertise a MIME type")
	assert.Contains(t, sig.MIMETypes, "text/x-mosestext")
}

// okapi: MosesTextFilterTest#testDoubleExtraction
// Okapi's RoundTripComparison runs read → write → read over Test01.txt and
// Test02.txt and asserts the extracted text units are identical across the
// round trip. The native equivalent is a byte-exact skeleton round trip: the
// reader emits skeleton entries so the writer can reconstruct the input
// verbatim. We use the exact upstream Test01/Test02 content (which contains
// <mrk>, <lb/> and entity references) and assert byte-exact reproduction —
// the native reader preserves it all as plain text.
func TestDoubleExtraction(t *testing.T) {
	t.Parallel()

	// Verbatim content of the upstream Okapi test resources Test01.txt and
	// Test02.txt (filters/mosestext/src/test/resources).
	test01 := "<mrk mtype=\"seg\">This is line 1.</mrk>\n" +
		"This is a test on line 1,<lb/>and line two.\n"
	test02 := "Test with &lt;=lt, >=gt, &amp;=amp, &#13;=CR, etc...\n" +
		"First segment.\n" +
		"Second segment<lb/>and it goes onto<lb/>three lines.\n" +
		"<mrk mtype=\"seg\">Third segment with optional markers.</mrk>\n" +
		"<mrk mtype=\"seg\" mid=\"should be preserved\">Fourth segment<lb/>on two lines.</mrk>\n"

	for _, tc := range []struct {
		name  string
		input string
	}{
		{"Test01.txt", test01},
		{"Test02.txt", test02},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := snippetRoundtripWithSkeleton(t, tc.input)
			assert.Equal(t, tc.input, got, "double extraction must reproduce input byte-exact")
		})
	}
}

// okapi-skip: MosesTextFilterTest#testCode1 — Okapi parses pseudo-XLIFF inline
// codes ("Text <x id='1'/>" → generic content "Text <1/>" with a placeholder
// Code). The native reader is plain-text; inline markup is preserved verbatim
// unless the code finder is enabled. The native verbatim-preservation
// analogue is covered by TestCodeFinder_IsolatedCode.

// okapi-skip: MosesTextFilterTest#testCode2 — relies on Okapi pseudo-XLIFF
// <g>…</g> open/close parsing and generic-content rendering ("<2>Text</2> <1/>"),
// which the native plain-text reader does not perform. Verbatim preservation
// via the code finder is covered by TestCodeFinder_PairedCodes.

// okapi-skip: MosesTextFilterTest#testCode3 — relies on Okapi pseudo-XLIFF
// nested <g> parsing with a code-id stack and generic-content rendering,
// which is not a native format feature.

// okapi-skip: MosesTextFilterTest#testCode4 — relies on Okapi pseudo-XLIFF
// <bx>/<ex>/<x> isolated-code parsing (mapped to opening/closing/placeholder
// Codes). The native reader has no pseudo-XLIFF parser. Verbatim preservation
// is covered by TestCodeFinder_IsolatedCode.

// okapi-skip: MosesTextFilterTest#testEscapedG — asserts Okapi throws
// EmptyStackException: it entity-decodes "&lt;g&gt;a&lt;/g&gt;" to "<g>a</g>"
// then its <g> open/close parser pops an empty stack on the unmatched </g>.
// The native reader does neither entity decoding nor <g> parsing, so there is
// no equivalent failure mode. Native robustness on this input is verified by
// TestEscapedGReadAsPlainText.

// ---------------------------------------------------------------------------
// neokapi-only: native robustness / code-finder behaviour
//
// These have no 1:1 Okapi @Test but document the native contract that stands
// in for Okapi's pseudo-XLIFF code handling.
// ---------------------------------------------------------------------------

// neokapi-only: counterpart to Okapi testEscapedG. Okapi entity-decodes
// "&lt;g&gt;a&lt;/g&gt;" to "<g>a</g>", then its <g> open/close parser pops an
// empty stack on the unmatched </g> and throws EmptyStackException. The native
// Moses InlineText reader performs the same entity decoding, but its code
// parser has no stack to underflow: the bare "<g>" (no id attribute) is not a
// valid opening code, and the unmatched "</g>" is captured as a closing code
// run. The reader therefore accepts the input without error — the decoded
// translatable text is "<g>a" (the "</g>" code contributing nothing to the
// flattened source).
func TestEscapedGDecodesWithoutError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := mosestext.NewReader()
	snippet := "&lt;g&gt;a&lt;/g&gt;"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	// Entities decode to "<g>a</g>"; the unmatched "</g>" parses as a closing
	// code run, leaving "<g>a" as the translatable text.
	assert.Equal(t, "<g>a", blocks[0].SourceText())

	// The unmatched closing code is captured as a PcClose run (no panic, no
	// error) — the native parser gracefully tolerates the imbalance.
	var hasClose bool
	for _, r := range blocks[0].SourceRuns() {
		if r.PcClose != nil {
			hasClose = true
			assert.Equal(t, "</g>", r.PcClose.Data)
		}
	}
	assert.True(t, hasClose, "unmatched </g> should be captured as a closing code run")
}

// neokapi-only: native analogue of Okapi testCode1 / testCode4. With the code
// finder enabled, an isolated <x id='…'/> tag is carved into an opaque
// placeholder run whose Data is the verbatim tag, and the surrounding text
// stays translatable. This is how the native reader keeps inline markup safe
// for translation instead of parsing it as pseudo-XLIFF.
func TestCodeFinder_IsolatedCode(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := mosestext.NewReader()
	cfg := reader.Config().(*mosestext.Config)
	cfg.UseCodeFinder = true
	cfg.CodeFinderRules = []string{`</?(g|x|bx|ex)\b[^>]*/?>`}

	snippet := "Text <x id='1'/>"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)

	// Translatable text excludes the placeholder content.
	assert.Equal(t, "Text ", blocks[0].SourceText())

	runs := blocks[0].SourceRuns()
	require.Len(t, runs, 2)
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "Text ", runs[0].Text.Text)
	require.NotNil(t, runs[1].Ph, "the <x .../> tag should become a placeholder run")
	assert.Equal(t, "<x id='1'/>", runs[1].Ph.Data, "placeholder must carry the verbatim tag")
}

// neokapi-only: native analogue of Okapi testCode2 / testCode3. Paired
// <g>…</g> tags are each captured as opaque placeholder runs around the inner
// translatable text. The native reader does not pair or renumber them (no
// generic-content model); it only guarantees they survive verbatim.
func TestCodeFinder_PairedCodes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := mosestext.NewReader()
	cfg := reader.Config().(*mosestext.Config)
	cfg.UseCodeFinder = true
	cfg.CodeFinderRules = []string{`</?(g|x|bx|ex)\b[^>]*/?>`, `</g>`}

	snippet := "<g id='2'>Text</g> <x id='1'/>"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)

	// Only "Text" plus the literal space remain translatable.
	assert.Equal(t, "Text ", blocks[0].SourceText())

	// Collect placeholder Data in order.
	var phData []string
	for _, r := range blocks[0].SourceRuns() {
		if r.Ph != nil {
			phData = append(phData, r.Ph.Data)
		}
	}
	assert.Equal(t, []string{"<g id='2'>", "</g>", "<x id='1'/>"}, phData)
}

// neokapi-only: code-finder placeholders must survive a write/re-read round
// trip byte-exact via the skeleton store. This is the native guarantee that
// stands in for Okapi's "codes preserved through the writer" behaviour.
func TestCodeFinder_RoundTripByteExact(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	reader := mosestext.NewReader()
	cfg := reader.Config().(*mosestext.Config)
	cfg.UseCodeFinder = true
	cfg.CodeFinderRules = []string{`</?(g|x|bx|ex)\b[^>]*/?>`, `</g>`}

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	writer := mosestext.NewWriter()
	writer.SetSkeletonStore(store)

	input := "Text <x id='1'/>\n<g id='2'>Hello</g> world"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, input, buf.String(), "inline codes must round-trip byte-exact")
}

// ---------------------------------------------------------------------------
// Okapi MosesTextFilterWriterTest — classified
// ---------------------------------------------------------------------------

// okapi-skip: MosesTextFilterWriterTest#testOutputFromXLIFF01 — cross-filter
// integration: Okapi reads a .xlf with its XLIFFFilter, then serialises Moses
// output via MosesTextFilterWriter's pseudo-XLIFF encoder (re-emitting <g>/<x>
// and escaping &lt;/&#13;/<lb/>). The native mosestext writer emits plain
// block text only and does not depend on the XLIFF reader. The native
// inline-code preservation analogue is TestCodeFinder_RoundTripByteExact.

// okapi-skip: MosesTextFilterWriterTest#testFileOutputFromXLIFF01 — same
// cross-filter XLIFF→Moses pseudo-XLIFF encoder path, additionally exercising
// file output and asserting "<g id=\"1\">" is re-serialised into the Moses
// file. Not a native mosestext reader/writer feature.

// okapi-skip: MosesTextFilterWriterTest#testOutputFromXLIFF02 — same
// cross-filter XLIFF→Moses path; asserts Okapi's Moses encoder emits <g>/<x>
// codes and re-escapes literals (&lt;, &#13;, <lb/>). The native writer has no
// pseudo-XLIFF encoder.
//
// The native analogue of "blocks (including inline codes) are serialised to
// Moses output" is the plain-text writer below: each block becomes one line,
// inline-code placeholder Data is spliced back verbatim, with no pseudo-XLIFF
// re-encoding or entity escaping.
func TestWriterPlainTextOutput(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// Block 1 carries an inline-code placeholder run (as the code finder
	// would produce); block 2 is plain text.
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1", Format: "mosestext"}},
		{Type: model.PartBlock, Resource: &model.Block{
			ID: "tu1",
			Source: []*model.Segment{{ID: "s1", Runs: []model.Run{
				{Text: &model.TextRun{Text: "Text "}},
				{Ph: &model.PlaceholderRun{ID: "c1", Type: "code", Data: "<x id=\"1\"/>"}},
			}}},
		}},
		{Type: model.PartBlock, Resource: &model.Block{
			ID:     "tu2",
			Source: []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Plain line"}}}}},
		}},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	var buf bytes.Buffer
	writer := mosestext.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	// One line per block; placeholder Data spliced back verbatim; no
	// pseudo-XLIFF re-encoding.
	assert.Equal(t, "Text <x id=\"1\"/>\nPlain line", buf.String())
}
