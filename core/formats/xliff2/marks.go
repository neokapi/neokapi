package xliff2

import (
	"slices"
	"unicode/utf8"

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

// marksForSegments distributes an edition's marker spans across the segments
// they fall in, rebased to each segment's own run positions, each written with
// the attributes attrs gives it. runs is the sequence the spans anchor to,
// which segs divide in order.
//
// Segments and spans are matched by position (textPos), never by run index: an
// edit that joins the text on either side of a segment boundary into one run
// leaves the segments' runs and the block's numbered differently, and a span
// the edit carried can start partway into a run.
//
// A span straddling a segment boundary is returned unplaced. XLIFF can pair an
// <sm> in one <segment> with an <em> in the next, but a segment is the unit a
// translator is handed, and a marker that only closes in the following one is a
// trap for every tool downstream. Saying so is the honest answer; drawing half
// a pair is not.
func marksForSegments(segs []seg, runs []model.Run, spans []model.Span, attrs func(model.Span) MrkAttrs) (placed [][]markSpan, unplaced []model.Span) {
	placed = make([][]markSpan, len(segs))
	bounds := make([][2]textPos, len(segs))
	var cursor textPos
	for i, s := range segs {
		end := cursor.plus(posOf(s.Runs, model.RunPos{Run: len(s.Runs)}))
		bounds[i] = [2]textPos{cursor, end}
		cursor = end
	}

	for _, sp := range spans {
		a := sp.Range
		if (a.Kind != model.AnchorRange && a.Kind != "") || len(a.Path) > 0 {
			continue // a term in a plural or select, or one naming no characters
		}
		if !a.InBounds(runs) {
			unplaced = append(unplaced, sp)
			continue
		}
		start, end := posOf(runs, a.Start), posOf(runs, a.End)
		if !start.before(end) {
			continue // an empty span marks nothing
		}
		seated := false
		for i, b := range bounds {
			if start.before(b[0]) || b[1].before(end) {
				continue
			}
			local := segs[i].Runs
			ls, okStart := runPosAt(local, start.minus(b[0]))
			le, okEnd := runPosAt(local, end.minus(b[0]))
			if okStart && okEnd {
				placed[i] = append(placed[i], markSpan{Attrs: attrs(sp), Start: ls, End: le})
				seated = true
			}
			break
		}
		if !seated {
			unplaced = append(unplaced, sp)
		}
	}
	return placed, unplaced
}

// termAttrs is how a term span is written: as a term marker with the span's
// id, and the attributes the file gave it when it came from one.
func termAttrs(sp model.Span) MrkAttrs {
	return MrkAttrs{ID: sp.ID, Type: termMarkType, Ref: termRef(sp), Value: sp.Props["value"], Translate: sp.Props["translate"]}
}

// termRef is the concept a term span points back at: the ref the file gave its
// marker, which the reader keeps in the span's ref prop, else the concept a
// term annotator recorded on it.
func termRef(sp model.Span) string {
	if ref := sp.Props["ref"]; ref != "" {
		return ref
	}
	if id := sp.Props["concept_id"]; id != "" {
		return id
	}
	if ta, ok := sp.Value.(*model.TermAnnotation); ok && ta != nil {
		return ta.ConceptID
	}
	return ""
}

// markerAttrs is how a span of the OverlayMrk overlay is written: as the
// marker the reader read it from, with the type and attributes it declared.
func markerAttrs(sp model.Span) MrkAttrs {
	return MrkAttrs{ID: sp.ID, Type: sp.Props["type"], Ref: sp.Props["ref"], Value: sp.Props["value"], Translate: sp.Props["translate"]}
}

// editedInlines rebuilds the inline IR of a segment an edit changed from its
// runs, each code with the attributes the document gave it (codes), and draws
// the segment's marks into it: its term marks, and the other markers the
// document carried, which the stale IR held and the runs do not. unplaced are
// the term marks it could not draw. ok is false when no XLIFF 2 markup
// expresses the runs.
func editedInlines(s *seg, codes codeIndex) (inls []Inline, unplaced []markSpan, ok bool) {
	rebuilt, ok := inlinesFromRuns(s.Runs, codes)
	if !ok {
		return nil, nil, false
	}
	marks := make([]markSpan, 0, len(s.Marks)+len(s.OtherMarks))
	marks = append(append(marks, s.Marks...), s.OtherMarks...)
	inls, missed := spliceMarks(rebuilt, s.Runs, marks)
	for _, m := range missed {
		if m.Attrs.Type == termMarkType {
			unplaced = append(unplaced, m)
		}
	}
	return inls, unplaced, true
}

// segmentBody returns the inline IR a write without a skeleton gives a segment
// read from XLIFF 2: its own IR while that still holds the runs' text, with the
// term marks drawn from the overlay, and the IR rebuilt from the runs once an
// edit made it stale (editedInlines). ok is false when the segment has no IR,
// or no XLIFF 2 markup expresses its edited runs.
func segmentBody(s *seg, codes codeIndex) (inls []Inline, unplaced []markSpan, ok bool) {
	if ir := freshInlineIR(s); ir != nil {
		inls, unplaced = spliceMarks(withoutTermMarkers(ir.Inlines), s.Runs, s.Marks)
		return inls, unplaced, true
	}
	if s.Content == nil {
		return nil, nil, false
	}
	return editedInlines(s, codes)
}

// textPos is a boundary in a segment's content, measured in what an edit's run
// boundaries cannot shift: the code points of text and the inline codes before
// it. Two run sequences with the same text and codes agree on every textPos
// however their text is split into runs, and so does the inline IR that
// describes them.
type textPos struct {
	text  int // code points of text before the boundary
	codes int // inline codes before the boundary: each ph, sc and ec, and each end of a pc
}

// before reports whether p comes earlier in the content than q. Along a
// sequence both counts only grow, so comparing text first and codes second is
// the content's order.
func (p textPos) before(q textPos) bool {
	return p.text < q.text || p.text == q.text && p.codes < q.codes
}

func (p textPos) plus(q textPos) textPos  { return textPos{p.text + q.text, p.codes + q.codes} }
func (p textPos) minus(q textPos) textPos { return textPos{p.text - q.text, p.codes - q.codes} }

// posOf measures the boundary at p in runs. Every run that is not text counts
// as one code, as the reader downconverts each code element to one run.
func posOf(runs []model.Run, p model.RunPos) textPos {
	var tp textPos
	for i := 0; i < p.Run && i < len(runs); i++ {
		if t := runs[i].Text; t != nil {
			tp.text += utf8.RuneCountInString(t.Text)
		} else {
			tp.codes++
		}
	}
	if p.Run < len(runs) && runs[p.Run].Text != nil {
		tp.text += p.Offset
	}
	return tp
}

// runPosAt finds the run position in runs that lies at tp: the first one, when
// several runs carry no text between codes. ok is false when no position of
// runs lies there.
func runPosAt(runs []model.Run, tp textPos) (model.RunPos, bool) {
	var at textPos
	for i, r := range runs {
		if at == tp {
			return model.RunPos{Run: i}, true
		}
		if r.Text == nil {
			at.codes++
			continue
		}
		n := utf8.RuneCountInString(r.Text.Text)
		if at.codes == tp.codes && at.text < tp.text && tp.text < at.text+n {
			return model.RunPos{Run: i, Offset: tp.text - at.text}, true
		}
		at.text += n
	}
	if at == tp {
		return model.RunPos{Run: len(runs)}, true
	}
	return model.RunPos{}, false
}

// termMarkType is the XLIFF 2 marker type a term carries.
const termMarkType = "term"

// withoutTermMarkers returns inls without the term markers the document
// carried: each <mrk type="term"> gives way to its content, and each
// <sm type="term"> is left out with the <em> that closes it. The reader records
// every one of them as a term overlay span, and the writer draws term marks
// from the overlay alone (spliceMarks), so a mark the file carried is written
// once, and a span an edit removed or moved is written as the model holds it.
// Every other node is kept.
func withoutTermMarkers(inls []Inline) []Inline {
	opened := map[string]bool{}
	Walk(inls, func(in *Inline) bool {
		if in.Sm != nil && in.Sm.Type == termMarkType {
			opened[in.Sm.ID] = true
		}
		return true
	})
	var strip func([]Inline) ([]Inline, bool)
	strip = func(inls []Inline) ([]Inline, bool) {
		out := make([]Inline, 0, len(inls))
		changed := false
		for _, in := range inls {
			switch {
			case in.Mrk != nil && in.Mrk.Type == termMarkType:
				children, _ := strip(in.Mrk.Children)
				out = append(out, children...)
				changed = true
			case in.Sm != nil && in.Sm.Type == termMarkType,
				in.Em != nil && opened[in.Em.StartRef]:
				changed = true
			case in.Mrk != nil:
				children, c := strip(in.Mrk.Children)
				if c {
					m := *in.Mrk
					m.Children = children
					in = Inline{Mrk: &m}
					changed = true
				}
				out = append(out, in)
			case in.Pc != nil:
				children, c := strip(in.Pc.Children)
				if c {
					pc := *in.Pc
					pc.Children = children
					in = Inline{Pc: &pc}
					changed = true
				}
				out = append(out, in)
			default:
				out = append(out, in)
			}
		}
		return out, changed
	}
	out, changed := strip(inls)
	if !changed {
		return inls
	}
	return out
}

// spliceMarks returns inls with an <sm>/<em> pair bounding each mark, plus any
// mark whose boundaries fall outside the sequence. A mark is located in runs,
// the segment's run sequence, and drawn where the IR reaches the same textPos.
// The IR need not split its text where the runs do, so a boundary that falls
// inside one of its text nodes splits the node.
func spliceMarks(inls []Inline, runs []model.Run, marks []markSpan) (out []Inline, unplaced []markSpan) {
	if len(marks) == 0 {
		return inls, nil
	}

	reach := irReach(inls)
	st := &splicer{opens: map[textPos][]Inline{}, closes: map[textPos][]Inline{}}
	for _, m := range marks {
		if !model.SpanAnchor(m.Start, m.End).InBounds(runs) {
			unplaced = append(unplaced, m)
			continue
		}
		start, end := posOf(runs, m.Start), posOf(runs, m.End)
		if !start.before(end) || !reach.has(start) || !reach.has(end) {
			unplaced = append(unplaced, m)
			continue
		}
		st.opens[start] = append(st.opens[start], Inline{Sm: &Sm{MrkAttrs: m.Attrs}})
		st.closes[end] = append(st.closes[end], Inline{Em: &Em{StartRef: m.Attrs.ID}})
	}
	if len(st.opens) == 0 {
		return inls, unplaced
	}

	out = st.walk(inls)
	out = append(out, st.boundary()...) // the boundary past the last inline
	return out, unplaced
}

// splicer carries the position through a nested walk, emitting the markers
// whose boundary the walk has reached.
type splicer struct {
	at     textPos
	opens  map[textPos][]Inline
	closes map[textPos][]Inline
}

// boundary is what belongs at the walk's current position: every mark ending
// here closes before any mark starting here opens, so two adjacent spans do not
// interleave into a pair that reads as one. A position the walk passes more
// than once, around a node that carries neither text nor a code, draws its
// markers the first time.
func (s *splicer) boundary() []Inline {
	out := make([]Inline, 0, len(s.closes[s.at])+len(s.opens[s.at]))
	out = append(out, s.closes[s.at]...)
	out = append(out, s.opens[s.at]...)
	delete(s.closes, s.at)
	delete(s.opens, s.at)
	return out
}

// cuts lists the offsets inside a text node of n code points, starting at the
// walk's position, where a marker boundary falls.
func (s *splicer) cuts(n int) []int {
	var out []int
	for _, m := range []map[textPos][]Inline{s.opens, s.closes} {
		for p := range m {
			if p.codes == s.at.codes && s.at.text < p.text && p.text < s.at.text+n {
				out = append(out, p.text-s.at.text)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (s *splicer) walk(inls []Inline) []Inline {
	out := make([]Inline, 0, len(inls))
	for _, in := range inls {
		out = append(out, s.boundary()...)
		switch {
		case in.Text != nil:
			text := []rune(in.Text.Content)
			from := 0
			for _, cut := range s.cuts(len(text)) {
				out = append(out, Inline{Text: &Text{Content: string(text[from:cut])}})
				s.at.text += cut - from
				from = cut
				out = append(out, s.boundary()...)
			}
			if from == 0 {
				out = append(out, in)
			} else {
				out = append(out, Inline{Text: &Text{Content: string(text[from:])}})
			}
			s.at.text += len(text) - from
		case in.Pc != nil:
			// A pc is a code on its open, then its children, then a code on
			// its close. A boundary between the open and the first child, or
			// after the last child, therefore belongs INSIDE the element.
			s.at.codes++
			pc := *in.Pc
			kids := s.walk(in.Pc.Children)
			kids = append(kids, s.boundary()...)
			s.at.codes++
			pc.Children = kids
			out = append(out, Inline{Pc: &pc})
		case in.Mrk != nil:
			mrk := *in.Mrk
			mrk.Children = s.walk(in.Mrk.Children)
			out = append(out, Inline{Mrk: &mrk})
		case in.Sm != nil, in.Em != nil:
			out = append(out, in) // a marker is not content and has no width
		default:
			s.at.codes++
			out = append(out, in)
		}
	}
	return out
}

// reach records the positions an inline sequence passes through. The points
// with c codes behind them hold the text from start[c] up to the next code, or
// to the end of the sequence after the last one. It counts codes the way
// inlinesToRunsWithMarks makes runs of them, one per element and two per pc,
// or a position measured on the runs would name a different point in the IR.
type reach struct {
	start []int // text before the first point with c codes behind it, by c
	text  int   // the sequence's whole text
}

// irReach measures the positions inls passes through.
func irReach(inls []Inline) reach {
	r := reach{start: []int{0}}
	var walk func([]Inline)
	code := func() { r.start = append(r.start, r.text) }
	walk = func(inls []Inline) {
		for _, in := range inls {
			switch {
			case in.Text != nil:
				r.text += utf8.RuneCountInString(in.Text.Content)
			case in.Pc != nil:
				code()
				walk(in.Pc.Children)
				code()
			case in.Mrk != nil:
				walk(in.Mrk.Children)
			case in.Ph != nil, in.Sc != nil, in.Ec != nil:
				code()
			}
		}
	}
	walk(inls)
	return r
}

// has reports whether the sequence passes through p.
func (r reach) has(p textPos) bool {
	if p.codes < 0 || p.codes >= len(r.start) {
		return false
	}
	limit := r.text
	if p.codes+1 < len(r.start) {
		limit = r.start[p.codes+1]
	}
	return r.start[p.codes] <= p.text && p.text <= limit
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
