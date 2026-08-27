package xliff2_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeBlocks serializes blocks the caller has already modified, which is the
// point: the marks under test are added to the model between read and write.
func writeBlocks(t *testing.T, blocks ...*model.Block) string {
	t.Helper()
	parts := make([]*model.Part, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
	}
	var buf bytes.Buffer
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(&buf))
	require.NoError(t, w.Write(t.Context(), testutil.PartsToChannel(parts)))
	require.NoError(t, w.Close())
	return buf.String()
}

// markTerm puts a term overlay span over the half-open run range [from, to).
func markTerm(t *testing.T, b *model.Block, id, conceptID string, from, to int) {
	t.Helper()
	span := model.Span{
		ID:    id,
		Range: model.SpanAnchor(model.RunPos{Run: from}, model.RunPos{Run: to}),
	}
	if conceptID != "" {
		span.Props = map[string]string{"concept_id": conceptID}
	}
	b.AddOverlaySpan(model.OverlayTerm, span)
}

// The capability is what #2217's registry probe records, and what the recipe
// narrows. A writer that draws marks has to say so, or nothing asks it to.
func TestWriterDeclaresTermAnnotations(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"term"}, xliff2.NewWriter().InlineAnnotations())
}

// The whole point, end to end: a term located in the model comes out as a
// marker in the file, and reading that file back gives the span again.
func TestTermMarkRoundTripsThroughTheFile(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Install the kapi CLI</source>
  </segment></unit></file>
</xliff>`)
	require.Nil(t, block.OverlayOf(model.OverlayTerm), "the file carried no marks")

	// One text run, so the whole segment is the span.
	markTerm(t, block, "t0", "concept:kapi", 0, 1)

	out := writeBlocks(t, block)
	assert.Contains(t, out, "<sm", "the mark must reach the file")
	assert.Contains(t, out, `type="term"`)
	assert.Contains(t, out, `ref="concept:kapi"`)
	assert.Contains(t, out, "<em")

	// And the file means the same thing to the reader that wrote it.
	back := readOneBlock(t, out)
	overlay := back.OverlayOf(model.OverlayTerm)
	require.NotNil(t, overlay, "a written mark must read back as a term span")
	require.Len(t, overlay.Spans, 1)
	assert.Equal(t, "t0", overlay.Spans[0].ID)
	assert.Equal(t, "concept:kapi", overlay.Spans[0].Props["ref"])
	assert.Equal(t, block.SourceText(), back.SourceText(), "content is unchanged by marking")
}

// The case the pair shape exists for. A span running from before a paired code
// to after it cannot be one <mrk>, and a writer that only knew <mrk> would have
// to refuse it — which is exactly the span an Anchor is built to carry.
func TestMarkSpanningAPairedCodeIsDrawn(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Use <pc id="1">the kapi</pc> CLI</source>
  </segment></unit></file>
</xliff>`)

	// Runs: 0 "Use ", 1 pcOpen, 2 "the kapi", 3 pcClose, 4 " CLI".
	// The span opens outside the pc and closes outside it, crossing both ends.
	markTerm(t, block, "t0", "", 0, 5)

	out := writeBlocks(t, block)
	assert.Contains(t, out, "<sm")
	assert.Contains(t, out, "<em")

	back := readOneBlock(t, out)
	overlay := back.OverlayOf(model.OverlayTerm)
	require.NotNil(t, overlay, "a span crossing a paired code must survive")
	require.Len(t, overlay.Spans, 1)
	covered := model.RunsText(overlay.Spans[0].Range.ExtractRuns(back.Source))
	assert.Contains(t, covered, "kapi")
}

// A boundary INSIDE a paired code is the shape that has no <mrk> at all: the
// marker opens within the element and closes outside it. Independent nodes make
// that ordinary rather than a special case.
func TestMarkOpeningInsideAPairedCodeIsDrawn(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Use <pc id="1">the kapi</pc> CLI</source>
  </segment></unit></file>
</xliff>`)

	// Open at run 2 (inside the pc) and close at run 5 (past its close).
	markTerm(t, block, "t0", "", 2, 5)

	out := writeBlocks(t, block)
	// The <sm> has to sit inside the <pc> element, not before it.
	pcAt := strings.Index(out, "<pc")
	smAt := strings.Index(out, "<sm")
	require.Positive(t, pcAt)
	require.Positive(t, smAt)
	assert.Greater(t, smAt, pcAt, "the marker opens inside the paired code")

	back := readOneBlock(t, out)
	overlay := back.OverlayOf(model.OverlayTerm)
	require.NotNil(t, overlay)
	require.Len(t, overlay.Spans, 1)
}

// Marking must not disturb the content it marks. This is the origin split in
// F-02 made testable: a conclusion about the runs never becomes one.
func TestMarkingLeavesTheContentAlone(t *testing.T) {
	t.Parallel()
	source := `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Read <pc id="1">the <ph id="2"/> guide</pc> now</source>
  </segment></unit></file>
</xliff>`
	plain := readOneBlock(t, source)
	before := writeBlocks(t, plain)

	marked := readOneBlock(t, source)
	markTerm(t, marked, "t0", "", 0, 1)
	after := writeBlocks(t, marked)

	assert.NotEqual(t, before, after, "the mark is in the output")
	// Strip the markers and the rest must be byte-identical.
	stripped := stripMarkers(after)
	assert.Equal(t, before, stripped,
		"marking changed something other than the markers")
}

// Two adjacent spans must not interleave into one: the first closes before the
// second opens, or a reader pairs the wrong ends.
func TestAdjacentMarksCloseBeforeTheNextOpens(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>alpha<ph id="1"/>beta</source>
  </segment></unit></file>
</xliff>`)
	markTerm(t, block, "a", "", 0, 1)
	markTerm(t, block, "b", "", 1, 3)

	out := writeBlocks(t, block)
	firstEm := strings.Index(out, "<em")
	secondSm := strings.LastIndex(out, "<sm")
	require.Positive(t, firstEm)
	require.Positive(t, secondSm)
	assert.Less(t, firstEm, secondSm, "the first span closes before the second opens")

	back := readOneBlock(t, out)
	overlay := back.OverlayOf(model.OverlayTerm)
	require.NotNil(t, overlay)
	assert.Len(t, overlay.Spans, 2, "two spans stay two")
}

// A block with no term overlay writes exactly what it wrote before the feature
// existed. Nothing is drawn that nobody asked for.
func TestUnmarkedBlockIsUnchanged(t *testing.T) {
	t.Parallel()
	source := `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Install the kapi CLI</source>
  </segment></unit></file>
</xliff>`
	out := writeBlocks(t, readOneBlock(t, source))
	assert.NotContains(t, out, "<sm")
	assert.NotContains(t, out, "<em")
}

// stripMarkers removes the sm/em elements so the rest of the output can be
// compared byte for byte.
func stripMarkers(s string) string {
	for _, tag := range []string{"sm", "em"} {
		for {
			i := strings.Index(s, "<"+tag)
			if i < 0 {
				break
			}
			j := strings.Index(s[i:], "/>")
			if j < 0 {
				break
			}
			s = s[:i] + s[i+j+2:]
		}
	}
	return s
}
