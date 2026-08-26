package xliff2_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readOneBlock reads a single-unit XLIFF 2 document and returns its block.
func readOneBlock(t *testing.T, xml string) *model.Block {
	t.Helper()
	blocks := readBlocks(t, xml)
	require.Len(t, blocks, 1)
	return blocks[0]
}

// An <mrk type="term"> records a decision another tool made about these exact
// characters. It has no run analogue — it wraps content rather than being
// content — so it arrives as the stand-off term overlay constructs.yaml maps
// `meta.terminology` to, anchored at the runs it covered.
func TestReadRestoresTermMarkAsOverlay(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Install the <mrk id="m1" type="term" ref="concept:kapi">kapi</mrk> CLI</source>
  </segment></unit></file>
</xliff>`)

	overlay := block.OverlayOf(model.OverlayTerm)
	require.NotNil(t, overlay, "a term mark must survive the downconversion")
	require.Len(t, overlay.Spans, 1)

	span := overlay.Spans[0]
	assert.Equal(t, "m1", span.ID)
	assert.Equal(t, "concept:kapi", span.Props["ref"],
		"ref is how a mark points back at the concept it denotes")

	// The span covers exactly the marked run, and resolving it returns the
	// marked text rather than an approximation of it.
	assert.Equal(t, "kapi", model.RunsText(span.Range.ExtractRuns(block.Source)))
}

// A marker type the framework has no overlay of its own for is still carried:
// discarding it would throw away a decision the file recorded.
func TestReadKeepsUnknownMarkTypes(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Check <mrk id="c1" type="comment" value="ask legal">the wording</mrk> here</source>
  </segment></unit></file>
</xliff>`)

	assert.Nil(t, block.OverlayOf(model.OverlayTerm), "a comment is not a term")

	overlay := block.OverlayOf(xliff2.OverlayMrk)
	require.NotNil(t, overlay)
	require.Len(t, overlay.Spans, 1)
	assert.Equal(t, "comment", overlay.Spans[0].Props["type"])
	assert.Equal(t, "ask legal", overlay.Spans[0].Props["value"])
	assert.Equal(t, "the wording",
		model.RunsText(overlay.Spans[0].Range.ExtractRuns(block.Source)))
}

// A mark inside a <pc> is positioned against the block's runs, not the pc's:
// the pc contributes an opening run of its own that the mark sits after.
func TestReadPositionsMarkInsidePairedCode(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Read <pc id="1">the <mrk id="m1" type="term">kapi</mrk> guide</pc> now</source>
  </segment></unit></file>
</xliff>`)

	overlay := block.OverlayOf(model.OverlayTerm)
	require.NotNil(t, overlay)
	require.Len(t, overlay.Spans, 1)
	assert.Equal(t, "kapi",
		model.RunsText(overlay.Spans[0].Range.ExtractRuns(block.Source)))
}

// <sm>/<em> is the shape XLIFF 2 provides for a span that need not nest, which
// is exactly the case an Anchor is built to express: a phrase running through a
// paired code is one range rather than three.
func TestReadRestoresSpanningMarkers(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Use <sm id="s1" type="term"/>the <pc id="1">kapi</pc> CLI<em startRef="s1"/> today</source>
  </segment></unit></file>
</xliff>`)

	overlay := block.OverlayOf(model.OverlayTerm)
	require.NotNil(t, overlay, "a spanning marker pair must survive too")
	require.Len(t, overlay.Spans, 1)
	assert.Equal(t, "s1", overlay.Spans[0].ID)

	covered := model.RunsText(overlay.Spans[0].Range.ExtractRuns(block.Source))
	assert.True(t, strings.Contains(covered, "kapi"),
		"the span must cover the content between the markers, got %q", covered)
}

// An <sm> whose <em> never arrives marks nothing: there is no end to anchor to,
// and inventing one would put a span somewhere the file did not say.
func TestReadDropsUnterminatedMarker(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Use <sm id="s1" type="term"/>the kapi CLI</source>
  </segment></unit></file>
</xliff>`)

	assert.Nil(t, block.OverlayOf(model.OverlayTerm))
	assert.Equal(t, "Use the kapi CLI", block.SourceText(),
		"the content still reads whole; only the unanchorable span is dropped")
}

// The shape real files arrive in. A CAT tool that decided part of a segment
// must not be translated records it as a vendor-namespaced marker carrying
// translate="no" — the parity corpus's translated_with_mrk.xlf is exactly this,
// with caits:regex-no-translate and caits:terms-no-translate marks.
//
// Nothing here understands that vendor's type, and that is the point: the
// instruction is carried anyway, so a consumer that does understand it still
// can, and a round-trip does not quietly drop another tool's decision.
func TestReadCarriesForeignDoNotTranslateMarks(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Send to <mrk id="1" translate="no" type="caits:terms-no-translate">support@example.com</mrk> now</source>
  </segment></unit></file>
</xliff>`)

	overlay := block.OverlayOf(xliff2.OverlayMrk)
	require.NotNil(t, overlay, "a foreign marker must not be discarded")
	require.Len(t, overlay.Spans, 1)

	span := overlay.Spans[0]
	assert.Equal(t, "caits:terms-no-translate", span.Props["type"])
	assert.Equal(t, "no", span.Props["translate"],
		"translate=no is the instruction; losing it loses the decision")
	assert.Equal(t, "support@example.com",
		model.RunsText(span.Range.ExtractRuns(block.Source)))
}
