package xliff2_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// An edit to a segment's runs leaves the segment's inline IR stale, so the
// writer rebuilds the segment from its runs, which carry no markers. The edit
// keeps the overlays the reader recorded the markers as, and the writer draws
// them from those: the term with the ref the file gave it, and every other
// marker with its type and value. Without that, `Grind the <mrk
// type="term">coffee</mrk> <mrk type="comment">daily</mrk> now` edited to
// "Brew" came out as `Brew the coffee daily now` on every write path.

const markedDoc = `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Grind the <mrk id="m1" type="term" ref="#c1">coffee</mrk> <mrk id="m2" type="comment" value="x">daily</mrk> now</source>
  </segment></unit></file>
</xliff>`

// brew replaces "Grind" with "Brew" in the source, as an edit through
// core/change does: the runs change and the overlays follow.
func brew(t *testing.T, b *model.Block) {
	t.Helper()
	find := "Grind"
	res := change.ApplyBlock(b, []change.Op{{
		Kind: change.KindReplaceText, At: change.Ref{Block: b.ID}, IfMatch: change.AnyRevision,
		Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: &find, Text: "Brew"}}},
	}}, change.BlockEnv{Actor: change.Actor{Kind: change.ActorTool, Name: "test"}})
	require.Equal(t, change.OpApplied, res[0].Status)
}

// domWrite reads input, edits every block, and writes through the path that
// patches the document the reader parsed, with no skeleton.
func domWrite(t *testing.T, input string, edit func(*model.Block)) string {
	t.Helper()
	ctx := t.Context()
	reader := xliff2.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok {
			edit(b)
		}
	}
	var buf bytes.Buffer
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(&buf))
	require.NoError(t, w.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, w.Close())
	return buf.String()
}

func TestMarkersSurviveAnEditToTheirSegment(t *testing.T) {
	t.Parallel()
	paths := map[string]func(t *testing.T) string{
		"without a skeleton": func(t *testing.T) string {
			b := readOneBlock(t, markedDoc)
			brew(t, b)
			return writeBlocks(t, b)
		},
		"through the skeleton": func(t *testing.T) string {
			return skeletonEdit(t, markedDoc, func(b *model.Block) { brew(t, b) })
		},
		"patching the parsed document": func(t *testing.T) string {
			return domWrite(t, markedDoc, func(b *model.Block) { brew(t, b) })
		},
	}
	for name, write := range paths {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out := write(t)
			assert.Contains(t, out, "Brew the ")
			back := readOneBlock(t, out)
			assert.Equal(t, "Brew the coffee daily now", back.SourceText())

			term := back.OverlayOf(model.OverlayTerm)
			require.NotNil(t, term, "the term mark is kept: %s", out)
			require.Len(t, term.Spans, 1, out)
			assert.Equal(t, "coffee", model.RunsText(term.Spans[0].Range.ExtractRuns(back.SourceRuns())))
			assert.Equal(t, "#c1", term.Spans[0].Props["ref"], "the ref the file gave the term is written back: %s", out)

			other := back.OverlayOf(xliff2.OverlayMrk)
			require.NotNil(t, other, "the comment marker is kept: %s", out)
			require.Len(t, other.Spans, 1, out)
			assert.Equal(t, "daily", model.RunsText(other.Spans[0].Range.ExtractRuns(back.SourceRuns())))
			assert.Equal(t, "comment", other.Spans[0].Props["type"])
			assert.Equal(t, "x", other.Spans[0].Props["value"])
		})
	}
}

// The ref a term carries is the one the file gave it, read into the span's
// ref prop, and a term an annotator located carries its concept instead.
func TestTermMarkRefComesFromTheSpan(t *testing.T) {
	t.Parallel()
	block := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Install the <mrk id="m1" type="term" ref="#c1">kapi</mrk> CLI</source>
  </segment></unit></file>
</xliff>`)
	out := writeBlocks(t, block)
	assert.Contains(t, out, `<sm id="m1" type="term" ref="#c1"/>kapi<em startRef="m1"/>`)

	annotated := readOneBlock(t, `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment>
    <source>Install the kapi CLI</source>
  </segment></unit></file>
</xliff>`)
	annotated.AddOverlaySpan(model.OverlayTerm, model.Span{
		ID:    "term:0",
		Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 12}, model.RunPos{Run: 0, Offset: 16}),
		Value: &model.TermAnnotation{SourceTerm: "kapi", ConceptID: "c-42"},
	})
	assert.Contains(t, writeBlocks(t, annotated), `<sm id="term:0" type="term" ref="c-42"/>kapi<em startRef="term:0"/>`)
}
