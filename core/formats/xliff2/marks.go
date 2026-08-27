package xliff2

import (
	"github.com/neokapi/neokapi/core/model"
)

// This file draws a block's stand-off annotations into the inline IR on the way
// out, the mirror of what reader.go restores on the way in.
//
// Marks are spliced into the IR at emit time rather than derived from the runs,
// because the writer prefers the native IR and its freshness test is the runs'
// flat text — which a marker does not change. A mark derived from runs would be
// invisible to that test and dropped whenever the IR was reused, which is every
// round-trip.
//
// Every mark is written as an <sm>/<em> pair, never as <mrk>. XLIFF 2 offers
// both, and <mrk> reads better when a span nests cleanly inside one element —
// but only <sm>/<em> can express a span that does not, and a term crossing a
// <pc> boundary is exactly the case model.Anchor exists to carry: a phrase
// running through a bold segment is one range rather than three. One shape for
// both keeps the awkward case from being the least-tested one.
//
// Because the two markers are independent nodes rather than a wrapper, each is
// inserted wherever its own boundary lands — including inside a <pc> whose
// partner sits outside it. There is no span shape this has to refuse.

// InlineAnnotations declares what this writer draws into the document. It is
// the capability the registry records on FormatInfo.InlineAnnotations, and what
// the recipe narrows under defaults.annotations.write.
func (w *Writer) InlineAnnotations() []string {
	return []string{string(model.OverlayTerm)}
}

// marksForSegments distributes a block's term spans across the segments they
// fall in, rebased to each segment's own run coordinates.
//
// A span straddling a segment boundary is returned unplaced. XLIFF can pair an
// <sm> in one <segment> with an <em> in the next, but a segment is the unit a
// translator is handed, and a marker that only closes in the following one is a
// trap for every tool downstream. Saying so is the honest answer; drawing half
// a pair is not.
func marksForSegments(segs []seg, spans []model.Span) (placed [][]markSpan, unplaced []model.Span) {
	placed = make([][]markSpan, len(segs))
	bounds := make([][2]int, len(segs))
	cursor := 0
	for i, s := range segs {
		bounds[i] = [2]int{cursor, cursor + len(s.Runs)}
		cursor += len(s.Runs)
	}

	for _, sp := range spans {
		start, end := sp.Range.Start.Run, sp.Range.End.Run
		if end <= start {
			continue // an empty span marks nothing
		}
		seated := false
		for i, b := range bounds {
			if start >= b[0] && end <= b[1] {
				placed[i] = append(placed[i], markSpan{
					Attrs: MrkAttrs{ID: sp.ID, Type: "term", Ref: sp.Props["concept_id"]},
					Start: start - b[0],
					End:   end - b[0],
				})
				seated = true
				break
			}
		}
		if !seated {
			unplaced = append(unplaced, sp)
		}
	}
	return placed, unplaced
}

// spliceMarks returns inls with an <sm>/<em> pair bounding each mark's run
// span, plus any mark whose boundaries fall outside the sequence.
func spliceMarks(inls []Inline, marks []markSpan) (out []Inline, unplaced []markSpan) {
	if len(marks) == 0 {
		return inls, nil
	}

	width := 0
	for _, in := range inls {
		width += inlineRunWidth(in)
	}

	st := &splicer{opens: map[int][]Inline{}, closes: map[int][]Inline{}}
	for _, m := range marks {
		if m.Start < 0 || m.End > width || m.End <= m.Start {
			unplaced = append(unplaced, m)
			continue
		}
		st.opens[m.Start] = append(st.opens[m.Start], Inline{Sm: &Sm{MrkAttrs: m.Attrs}})
		st.closes[m.End] = append(st.closes[m.End], Inline{Em: &Em{StartRef: m.Attrs.ID}})
	}
	if len(st.opens) == 0 {
		return inls, unplaced
	}

	out = st.walk(inls)
	out = append(out, st.boundary()...) // the boundary past the last inline
	return out, unplaced
}

// splicer carries the run cursor through a nested walk, emitting the markers
// whose boundary the cursor has reached.
type splicer struct {
	run    int
	opens  map[int][]Inline
	closes map[int][]Inline
}

// boundary is what belongs at the cursor's current position: every mark ending
// here closes before any mark starting here opens, so two adjacent spans do not
// interleave into a pair that reads as one.
func (s *splicer) boundary() []Inline {
	var out []Inline
	out = append(out, s.closes[s.run]...)
	out = append(out, s.opens[s.run]...)
	return out
}

func (s *splicer) walk(inls []Inline) []Inline {
	out := make([]Inline, 0, len(inls))
	for _, in := range inls {
		out = append(out, s.boundary()...)
		switch {
		case in.Pc != nil:
			// A pc spends one run on its open, then its children, then one on
			// its close. A boundary between the open and the first child, or
			// after the last child, therefore belongs INSIDE the element.
			s.run++
			pc := *in.Pc
			kids := s.walk(in.Pc.Children)
			kids = append(kids, s.boundary()...)
			s.run++
			pc.Children = kids
			out = append(out, Inline{Pc: &pc})
		case in.Mrk != nil:
			mrk := *in.Mrk
			mrk.Children = s.walk(in.Mrk.Children)
			out = append(out, Inline{Mrk: &mrk})
		case in.Sm != nil, in.Em != nil:
			out = append(out, in) // a marker is not content and spends no run
		default:
			s.run++
			out = append(out, in)
		}
	}
	return out
}

// inlineRunWidth is how many runs one inline contributes to the
// downconversion. It must agree with inlinesToRunsWithMarks, or every position
// computed here lands somewhere else.
func inlineRunWidth(in Inline) int {
	switch {
	case in.Text != nil, in.Ph != nil, in.Sc != nil, in.Ec != nil:
		return 1
	case in.Pc != nil:
		w := 2
		for _, c := range in.Pc.Children {
			w += inlineRunWidth(c)
		}
		return w
	case in.Mrk != nil:
		w := 0
		for _, c := range in.Mrk.Children {
			w += inlineRunWidth(c)
		}
		return w
	}
	return 0 // sm, em
}

// UnplacedTermMark is a term span the writer could not draw, and why.
type UnplacedTermMark struct {
	// ID is the span's own id, empty when it carried none.
	ID string
	// Reason is why it was not drawn, in words a reader can act on.
	Reason string
}

// UnplacedTermMarks is every term span this writer could not draw, in the order
// it met them. Empty after a write that drew everything.
//
// Not silently dropped and not fatal: a term that cannot be marked is content
// that still writes correctly, so failing the write would trade a whole
// document for an annotation. The caller decides what a missing mark is worth.
func (w *Writer) UnplacedTermMarks() []UnplacedTermMark { return w.unplacedTermMarks }

func (w *Writer) noteUnplaced(s *seg, marks []markSpan) {
	for _, sp := range s.UnplacedMarks {
		w.unplacedTermMarks = append(w.unplacedTermMarks, UnplacedTermMark{
			ID:     sp.ID,
			Reason: "the span crosses a segment boundary, and a marker pair cannot span two segments",
		})
	}
	s.UnplacedMarks = nil
	for _, m := range marks {
		w.unplacedTermMarks = append(w.unplacedTermMarks, UnplacedTermMark{
			ID:     m.Attrs.ID,
			Reason: "the span's boundaries fall outside the segment's run sequence",
		})
	}
}
