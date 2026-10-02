package xliff2

import (
	"slices"
	"strconv"
	"strings"
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
// A marker the file wrote as <mrk> is written as <mrk> while its two ends
// still sit in one inline list, so no code boundary falls between them;
// otherwise, and for a mark nothing in the file carried, it is written as an
// <sm>/<em> pair. Only the pair can express a span that does not nest, and a
// term crossing a <pc> boundary is exactly the case model.Anchor exists to
// carry: a phrase running through a bold segment is one range rather than
// three. Because the two markers are independent nodes rather than a wrapper,
// each is inserted wherever its own boundary lands, including inside a <pc>
// whose partner sits outside it, so there is no span shape this has to refuse.
//
// A segment the overlays describe as the file wrote it is written from its own
// IR, so every untouched segment keeps the markup it was read with on every
// write path.

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
// A span straddling a segment boundary is drawn as the pair the file wrote it
// as, an <sm> in one segment and its <em> in a later one, when those segments'
// IR holds the two halves. Any other span straddling a boundary is returned
// unplaced: a segment is the unit a translator is handed, and a marker that
// only closes in the following one is a trap for every tool downstream, so the
// writer reports it rather than open a pair the file never had.
func marksForSegments(segs []seg, runs []model.Run, spans []model.Span, attrs func(model.Span) MrkAttrs) (placed [][]markSpan, unplaced []model.Span) {
	placed = make([][]markSpan, len(segs))
	bounds := make([][2]textPos, len(segs))
	var cursor textPos
	for i, s := range segs {
		end := cursor.plus(posOf(s.Runs, model.RunPos{Run: len(s.Runs)}))
		bounds[i] = [2]textPos{cursor, end}
		cursor = end
	}
	local := func(i int, at textPos) (model.RunPos, bool) {
		return runPosAt(segs[i].Runs, at.minus(bounds[i][0]))
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
			ls, okStart := local(i, start)
			le, okEnd := local(i, end)
			if okStart && okEnd {
				placed[i] = append(placed[i], markSpan{Attrs: attrs(sp), Start: ls, End: le})
				seated = true
			}
			break
		}
		if !seated && sp.ID != "" {
			seated = placeHalves(segs, bounds, placed, start, end, attrs(sp), local)
		}
		if !seated {
			unplaced = append(unplaced, sp)
		}
	}
	return placed, unplaced
}

// placeHalves places a span from start to end that crosses a segment boundary
// as the <sm> and <em> the file wrote it with: the <sm> in a segment holding
// start whose IR has an <sm> with the span's id and no <em> for it, and the
// <em> in a later segment holding end whose IR has an <em> for that id and no
// <sm>. It reports whether it found both.
func placeHalves(segs []seg, bounds [][2]textPos, placed [][]markSpan, start, end textPos, attrs MrkAttrs, local func(int, textPos) (model.RunPos, bool)) bool {
	holds := func(b [2]textPos, p textPos) bool { return !p.before(b[0]) && !b[1].before(p) }
	for i := range segs {
		if !holds(bounds[i], start) || !segs[i].halfInIR(attrs.ID, startHalf) {
			continue
		}
		ls, ok := local(i, start)
		if !ok {
			continue
		}
		for j := i + 1; j < len(segs); j++ {
			if !holds(bounds[j], end) || !segs[j].halfInIR(attrs.ID, endHalf) {
				continue
			}
			le, ok := local(j, end)
			if !ok {
				continue
			}
			placed[i] = append(placed[i], markSpan{Attrs: attrs, Start: ls, End: ls, Half: startHalf})
			placed[j] = append(placed[j], markSpan{Attrs: attrs, Start: le, End: le, Half: endHalf})
			return true
		}
	}
	return false
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

// segmentBody returns the inline IR a segment is written with, and the term
// marks it could not draw.
//
// A segment whose IR still describes its runs (irMatchesRuns) is written from
// that IR: as read, when its overlays hold exactly the markers the file gave
// it, and otherwise with those markers taken out and the overlays' marks drawn
// in. A segment an edit changed is rebuilt from its runs, each code with the
// attributes the document gave it (codes), and its marks drawn from the
// overlays. A segment with no IR, such as a target a tool created, is rebuilt
// the same way when codes come from an XLIFF 2 unit (codes.native). ok is false
// when the segment has no IR to write from, or no XLIFF 2 markup expresses its
// runs.
func segmentBody(s *seg, codes codeIndex) (inls []Inline, unplaced []markSpan, ok bool) {
	marks := s.drawnMarks()
	if s.Content != nil && irMatchesRuns(s.Content, s.Runs) {
		if s.marksAsRead() {
			return s.Content.Inlines, nil, true
		}
		inls, unplaced = drawMarks(stripRecordedMarkers(s.Content.Inlines, s.StripHalves), s.Runs, marks, s.Content.Inlines)
		return inls, termMarks(unplaced), true
	}
	if s.Content == nil && !codes.native {
		return nil, nil, false
	}
	rebuilt, ok := inlinesFromRuns(s.Runs, codes)
	if !ok {
		return nil, nil, false
	}
	var ir []Inline
	if s.Content != nil {
		ir = s.Content.Inlines
	}
	inls, unplaced = drawMarks(rebuilt, s.Runs, marks, ir)
	return inls, termMarks(unplaced), true
}

// drawnMarks is every mark the writer draws in the segment: its term marks and
// its other markers.
func (s *seg) drawnMarks() []markSpan {
	marks := make([]markSpan, 0, len(s.Marks)+len(s.OtherMarks))
	return append(append(marks, s.Marks...), s.OtherMarks...)
}

// termMarks keeps the term marks of marks, the ones UnplacedTermMarks reports.
func termMarks(marks []markSpan) []markSpan {
	var out []markSpan
	for _, m := range marks {
		if m.Attrs.Type == termMarkType {
			out = append(out, m)
		}
	}
	return out
}

// halfInIR reports whether the segment's IR holds one half of a marker pair
// whose other half is not in it: an <sm> with id (half startHalf), or an <em>
// closing id (endHalf).
func (s *seg) halfInIR(id string, half markHalf) bool {
	if s.Content == nil {
		return false
	}
	for _, h := range irHalves(s.Content.Inlines) {
		if h.Attrs.ID == id && h.Half == half {
			return true
		}
	}
	return false
}

// irHalves returns the halves of marker pairs inls holds without their
// partner, as the reader records them (inlinesToRunsWithMarks).
func irHalves(inls []Inline) []markSpan {
	_, marks := inlinesToRunsWithMarks(inls)
	var out []markSpan
	for _, m := range marks {
		if m.Half != wholeMark {
			out = append(out, m)
		}
	}
	return out
}

// marksAsRead reports whether the marks the writer draws in a segment whose IR
// is fresh are the markers that IR holds, each with the same attributes over
// the same content: then the IR is written as read. The halves of a pair split
// across segments that the writer keeps as read (not in StripHalves) are left
// out of the comparison, since the IR carries them either way.
func (s *seg) marksAsRead() bool {
	irRuns, irMarks := inlinesToRunsWithMarks(s.Content.Inlines)
	type key struct {
		half       markHalf
		attrs      MrkAttrs
		start, end textPos
	}
	keyOf := func(runs []model.Run, m markSpan) key {
		k := key{half: m.Half, attrs: m.Attrs, start: posOf(runs, m.Start), end: posOf(runs, m.End)}
		switch m.Half {
		case startHalf:
			k.end = k.start
		case endHalf:
			k.start = k.end
		}
		return k
	}
	want := map[key]int{}
	for _, m := range irMarks {
		k := keyOf(irRuns, m)
		if m.Half == wholeMark && !k.start.before(k.end) {
			continue // an empty marker marks nothing; it is written as read
		}
		if m.Half != wholeMark && !s.StripHalves[m.Attrs.ID] {
			continue
		}
		want[k]++
	}
	for _, m := range s.drawnMarks() {
		k := keyOf(s.Runs, m)
		if want[k] == 0 {
			return false
		}
		want[k]--
	}
	for _, n := range want {
		if n != 0 {
			return false
		}
	}
	return true
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

// stripRecordedMarkers returns inls without the markers the reader recorded as
// overlay spans, which the writer draws from the overlays: each <mrk> around
// content gives way to its content, each <sm>/<em> pair around content is left
// out, and so is each half of a pair split across segments whose id is in
// halves. Every other node is kept, and text the removal leaves side by side is
// joined.
func stripRecordedMarkers(inls []Inline, halves map[string]bool) []Inline {
	// Pair each <sm> with its <em> as the reader does (markWalker), counting
	// the content between them: the code points of text and the codes.
	type open struct {
		node    int
		content int
	}
	var (
		node    int
		content int
		opens   = map[string]open{}
		strip   = map[int]bool{}
	)
	var pair func([]Inline)
	pair = func(inls []Inline) {
		for _, in := range inls {
			at := node
			node++
			switch {
			case in.Text != nil:
				content += utf8.RuneCountInString(in.Text.Content)
			case in.Pc != nil:
				content++
				pair(in.Pc.Children)
				content++
			case in.Mrk != nil:
				pair(in.Mrk.Children)
			case in.Sm != nil:
				opens[in.Sm.ID] = open{node: at, content: content}
			case in.Em != nil:
				if o, ok := opens[in.Em.StartRef]; ok {
					delete(opens, in.Em.StartRef)
					if content > o.content {
						strip[o.node], strip[at] = true, true
					}
				} else if halves[in.Em.StartRef] {
					strip[at] = true
				}
			default:
				content++
			}
		}
	}
	pair(inls)
	for id, o := range opens {
		if halves[id] {
			strip[o.node] = true
		}
	}

	node = 0
	var rebuild func([]Inline) []Inline
	rebuild = func(inls []Inline) []Inline {
		out := make([]Inline, 0, len(inls))
		for _, in := range inls {
			at := node
			node++
			switch {
			case strip[at]:
			case in.Mrk != nil:
				children := rebuild(in.Mrk.Children)
				if hasContent(in.Mrk.Children) {
					out = appendInlines(out, children)
					continue
				}
				m := *in.Mrk
				m.Children = children
				out = append(out, Inline{Mrk: &m})
			case in.Pc != nil:
				pc := *in.Pc
				pc.Children = rebuild(in.Pc.Children)
				out = append(out, Inline{Pc: &pc})
			default:
				out = appendInlines(out, []Inline{in})
			}
		}
		return out
	}
	return rebuild(inls)
}

// hasContent reports whether inls hold text or a code.
func hasContent(inls []Inline) bool {
	found := false
	Walk(inls, func(in *Inline) bool {
		if in.Text != nil && in.Text.Content != "" || in.Ph != nil || in.Pc != nil || in.Sc != nil || in.Ec != nil {
			found = true
		}
		return !found
	})
	return found
}

// appendInlines appends add to out, joining a text node to the text before it.
func appendInlines(out, add []Inline) []Inline {
	for _, in := range add {
		if in.Text != nil && len(out) > 0 && out[len(out)-1].Text != nil {
			out[len(out)-1] = Inline{Text: &Text{Content: out[len(out)-1].Text.Content + in.Text.Content}}
			continue
		}
		out = append(out, in)
	}
	return out
}

// drawMarks returns inls with each mark drawn where its boundaries fall, plus
// any mark whose boundaries the sequence does not pass through. A mark is
// located in runs, the segment's run sequence, and drawn where the inlines
// reach the same textPos; a boundary inside a text node splits the node. ir is
// the IR the document gave the segment, which may be stale, or nil: a mark it
// holds as an <mrk> is written as an <mrk> while its ends are siblings in one
// inline list, and markers at one boundary keep the order the IR gave them.
func drawMarks(inls []Inline, runs []model.Run, marks []markSpan, ir []Inline) (out []Inline, unplaced []markSpan) {
	if len(marks) == 0 {
		return inls, nil
	}
	d := newDrawer(marks, ir)
	reach := irReach(inls)
	type event struct {
		mark  int
		start textPos
		end   textPos
	}
	var opens, closes []event
	for k, m := range marks {
		if !model.SpanAnchor(m.Start, m.End).InBounds(runs) {
			unplaced = append(unplaced, m)
			continue
		}
		start, end := posOf(runs, m.Start), posOf(runs, m.End)
		switch m.Half {
		case startHalf:
			if !reach.has(start) {
				unplaced = append(unplaced, m)
				continue
			}
			opens = append(opens, event{mark: k, start: start, end: textPos{text: 1 << 30}})
		case endHalf:
			if !reach.has(end) {
				unplaced = append(unplaced, m)
				continue
			}
			closes = append(closes, event{mark: k, start: textPos{text: -1}, end: end})
		default:
			if !start.before(end) || !reach.has(start) || !reach.has(end) {
				unplaced = append(unplaced, m)
				continue
			}
			opens = append(opens, event{mark: k, start: start, end: end})
			closes = append(closes, event{mark: k, start: start, end: end})
		}
	}
	if len(opens) == 0 && len(closes) == 0 {
		return inls, unplaced
	}
	// At one boundary the mark that opened last closes first, and the mark that
	// closes last opens first, so marks over one stretch of text nest; marks
	// over the same text keep the order the IR gave them.
	slices.SortStableFunc(opens, func(a, b event) int {
		if a.end != b.end {
			return cmpPos(b.end, a.end)
		}
		return d.rank[a.mark] - d.rank[b.mark]
	})
	slices.SortStableFunc(closes, func(a, b event) int {
		if a.start != b.start {
			return cmpPos(b.start, a.start)
		}
		return d.rank[b.mark] - d.rank[a.mark]
	})
	st := &splicer{opens: map[textPos][]Inline{}, closes: map[textPos][]Inline{}}
	for _, e := range opens {
		st.opens[e.start] = append(st.opens[e.start], Inline{Sm: &Sm{ID: drawnID(e.mark)}})
	}
	for _, e := range closes {
		st.closes[e.end] = append(st.closes[e.end], Inline{Em: &Em{StartRef: drawnID(e.mark)}})
	}
	out = st.walk(inls)
	out = append(out, st.boundary()...) // the boundary past the last inline
	return d.finish(out), unplaced
}

// cmpPos orders two positions in a sequence.
func cmpPos(a, b textPos) int {
	switch {
	case a.before(b):
		return -1
	case b.before(a):
		return 1
	}
	return 0
}

// drawnPrefix marks the id of a marker drawMarks has placed and not yet given
// its attributes. A NUL never appears in an XML attribute value, so it cannot
// be an id the document gave a marker.
const drawnPrefix = "\x00"

func drawnID(k int) string { return drawnPrefix + strconv.Itoa(k) }

// drawnMark returns the index of the mark a placed marker draws.
func drawnMark(id string) (int, bool) {
	if !strings.HasPrefix(id, drawnPrefix) {
		return 0, false
	}
	k, err := strconv.Atoi(id[len(drawnPrefix):])
	return k, err == nil
}

// drawer gives the markers drawMarks placed their final form.
type drawer struct {
	marks []markSpan
	// wrap says which marks the IR wrote as an <mrk>.
	wrap []bool
	// rank orders marks over the same text: the IR's order for a marker it
	// holds, then the rest in the order they came.
	rank []int
}

func newDrawer(marks []markSpan, ir []Inline) *drawer {
	d := &drawer{marks: marks, wrap: make([]bool, len(marks)), rank: make([]int, len(marks))}
	// A marker is known by its id, or by its type when the file gave it none.
	name := func(a MrkAttrs) string {
		if a.ID != "" {
			return a.ID
		}
		return "\x00type:" + a.Type
	}
	order := map[string]int{}
	asMrk := map[string]bool{}
	Walk(ir, func(in *Inline) bool {
		switch {
		case in.Mrk != nil:
			setOnce(order, name(in.Mrk.MrkAttrs), len(order))
			asMrk[name(in.Mrk.MrkAttrs)] = true
		case in.Sm != nil:
			setOnce(order, name(in.Sm.MrkAttrs), len(order))
		}
		return true
	})
	for k, m := range marks {
		d.wrap[k] = m.Half == wholeMark && asMrk[name(m.Attrs)]
		if r, ok := order[name(m.Attrs)]; ok {
			d.rank[k] = r
		} else {
			d.rank[k] = len(order) + k
		}
	}
	return d
}

// finish writes each placed marker in its final form: a mark the IR wrote as
// an <mrk> whose <em> is a later sibling in the same list wraps what lies
// between them, and every other mark is an <sm>/<em> pair with its own
// attributes. Lists are finished from the left, so of two such marks that
// overlap, the one that opens first wraps and the other stays a pair.
func (d *drawer) finish(inls []Inline) []Inline {
	out := make([]Inline, 0, len(inls))
	for i := 0; i < len(inls); i++ {
		in := inls[i]
		switch {
		case in.Sm != nil:
			k, ok := drawnMark(in.Sm.ID)
			if !ok {
				out = append(out, in)
				continue
			}
			if d.wrap[k] {
				if j := closesAt(inls, i, in.Sm.ID); j > 0 {
					out = append(out, Inline{Mrk: &Mrk{MrkAttrs: d.marks[k].Attrs, Children: d.finish(inls[i+1 : j])}})
					i = j
					continue
				}
			}
			out = append(out, Inline{Sm: &Sm{MrkAttrs: d.marks[k].Attrs}})
		case in.Em != nil:
			if k, ok := drawnMark(in.Em.StartRef); ok {
				in = Inline{Em: &Em{StartRef: d.marks[k].Attrs.ID}}
			}
			out = append(out, in)
		case in.Pc != nil:
			pc := *in.Pc
			pc.Children = d.finish(in.Pc.Children)
			out = append(out, Inline{Pc: &pc})
		case in.Mrk != nil:
			m := *in.Mrk
			m.Children = d.finish(in.Mrk.Children)
			out = append(out, Inline{Mrk: &m})
		default:
			out = appendInlines(out, []Inline{in})
		}
	}
	return out
}

// closesAt returns the index of the <em> closing id after i in inls, or -1.
func closesAt(inls []Inline, i int, id string) int {
	for j := i + 1; j < len(inls); j++ {
		if inls[j].Em != nil && inls[j].Em.StartRef == id {
			return j
		}
	}
	return -1
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
