package model

import "sort"

// Overlay rebasing for content rewrites (AD-002 / AD-006). Overlays anchor to
// the runs of one edition: the edition Source holds (Variant nil) or a derived
// one (Variant set). A rewrite of that edition's runs normally invalidates
// every overlay on it, segmentation, terms, entities, because their ranges no
// longer line up. A rewrite with a known mapping (a redaction's span to
// replacement edits, an edit's own ranges, or the changed region two texts
// differ in) remaps the surviving spans so they follow the rewrite. A rewrite
// with no mapping drops the edition's overlays rather than leave them
// dangling. Overlays on other editions are never touched.

// RunEdit describes one edit applied to a run sequence's flattened text (the
// RunsText coordinate space, in rune offsets): the half-open old span
// [Start, End) was replaced by content contributing NewLen runes to the new
// flattening. A deletion, or a replacement by a zero-width inline run as a
// redaction placeholder is, has NewLen == 0; a pure insertion has Start == End.
// Edits are expressed against the pre-edit text and must be sorted ascending and
// non-overlapping.
type RunEdit struct {
	Start  int
	End    int
	NewLen int
}

// onEdition reports whether o annotates the edition named by edition (nil for
// the edition Source holds).
func (o *Overlay) onEdition(edition *VariantKey) bool {
	if edition == nil || o.Variant == nil {
		return edition == nil && o.Variant == nil
	}
	return o.Variant.Canonical() == edition.Canonical()
}

// RemapOverlays rebases the overlay spans on one edition of b from oldRuns,
// the runs they anchor to, onto newRuns, the edition's rewritten runs, given
// the edits applied to the flattened text. edition nil is the edition Source
// holds. It returns the number of spans dropped.
//
// A range span follows the text it covers (remapRangeSpan): an edit inside it
// grows or shrinks it, an edit that replaces all of its text leaves it over the
// replacement, an edit across one of its boundaries leaves it over the part of
// its text the edit kept, and an edit before it shifts it. It is dropped only
// when the edits delete everything it covered. A quality finding
// (OverlayCheck) states something about the exact text it covers, so an edit
// that overlaps it drops it. A range span inside a plural or select branch (its
// anchor has a Path) is kept when the branch it points into is unchanged and
// dropped otherwise. A block anchor is kept; a run anchor is kept while a run
// with its id is still at its path; a form anchor is kept while its branch
// still exists. A span whose new anchor does not resolve in newRuns is
// dropped. An overlay left with no spans is removed.
//
// A segmentation overlay is a partition of the edition, and the bilingual
// writers read its spans as the complete segment list, so it is kept whole or
// dropped whole (remapPartition): a segment that contains an edit grows or
// shrinks with it, and an edit across a segment boundary drops the layer.
//
// Inline codes have no width in the flattened text, so a text offset alone
// cannot say which side of a code a boundary sits on. Each carried boundary
// keeps its side of every code beside it (boundaries.carry): a code that
// ended a segment still ends it, and a term that started after a code still
// starts after it.
//
// With no edits the call still re-anchors: a structure-only rewrite (runs
// added, removed, or reclassified without changing the text flattening) shifts
// run indices, so every range span is re-projected through its text range onto
// the new runs.
func RemapOverlays(b *Block, edition *VariantKey, oldRuns, newRuns []Run, edits []RunEdit) int {
	if b == nil || len(b.Overlays) == 0 {
		return 0
	}
	dropped := 0
	var bs *boundaries
	out := b.Overlays[:0]
	for oi := range b.Overlays {
		o := b.Overlays[oi]
		if !o.onEdition(edition) {
			out = append(out, o)
			continue
		}
		if bs == nil {
			bs = newBoundaries(oldRuns, newRuns)
			bs.edits = newEditIndex(edits)
		}
		if o.Type == OverlaySegmentation {
			spans, ok := remapPartition(o.Spans, bs, edits)
			if !ok {
				dropped += len(o.Spans)
				continue
			}
			o.Spans = spans
		} else {
			kept := make([]Span, 0, len(o.Spans))
			for _, s := range o.Spans {
				ns, ok := remapSpan(s, bs, edits, o.Type == OverlayCheck)
				if !ok {
					dropped++
					continue
				}
				kept = append(kept, ns)
			}
			o.Spans = kept
		}
		if len(o.Spans) == 0 {
			continue // drop the now-empty overlay
		}
		out = append(out, o)
	}
	b.Overlays = out
	return dropped
}

// remapPartition carries a segmentation layer across a rewrite as one piece.
// Each edit belongs to the segment that contains it: a replacement to the one
// holding both its ends, an insertion to the segment it ends or, failing that,
// the one it starts. That segment's end moves by the edit's length delta and
// every later segment shifts by it, so the edited text stays in the segment it
// was written in. An edit with no owner that overlaps a segment crosses a
// boundary, and then no segment list describes the rewrite; nor does one with
// a span that cannot be carried. Either way it reports false, and the caller
// drops the layer.
func remapPartition(spans []Span, bs *boundaries, edits []RunEdit) ([]Span, bool) {
	type flat struct {
		start, end int
		ok         bool
	}
	pos := make([]flat, len(spans))
	for i, s := range spans {
		if isRangeKind(s.Range.Kind) && len(s.Range.Path) == 0 {
			start, end := s.Range.TextSpan(bs.oldRuns)
			pos[i] = flat{start: start, end: end, ok: true}
		}
	}
	owner := make([]int, len(edits))
	for j, e := range edits {
		owner[j] = -1
		if e.Start == e.End {
			for i, p := range pos {
				if p.ok && p.start < e.Start && e.Start <= p.end {
					owner[j] = i
					break
				}
			}
			if owner[j] < 0 {
				for i, p := range pos {
					if p.ok && p.start == e.Start {
						owner[j] = i
						break
					}
				}
			}
			continue
		}
		for i, p := range pos {
			if !p.ok {
				continue
			}
			if p.start <= e.Start && e.End <= p.end {
				owner[j] = i
				break
			}
			if e.Start < p.end && p.start < e.End {
				return nil, false // the edit crosses a segment boundary
			}
		}
	}

	newLen := runsFlatLen(bs.newRuns)
	out := make([]Span, 0, len(spans))
	for i, s := range spans {
		if !pos[i].ok {
			ns, ok := remapSpan(s, bs, edits, false)
			if !ok {
				return nil, false
			}
			out = append(out, ns)
			continue
		}
		shift, grow := 0, 0
		for j, e := range edits {
			d := e.NewLen - (e.End - e.Start)
			switch {
			case owner[j] == i:
				grow += d
			case e.End <= pos[i].start:
				shift += d
			}
		}
		start, end := pos[i].start+shift, pos[i].end+shift+grow
		if start < 0 || end < start || end > newLen {
			return nil, false
		}
		ns := s
		ns.Range = SpanAnchor(bs.carry(s.Range.Start, start), bs.carry(s.Range.End, end))
		if !ns.Range.Resolves(bs.newRuns) {
			return nil, false
		}
		out = append(out, ns)
	}
	return out, true
}

// isRangeKind reports whether an anchor kind addresses a span of text: the
// range kind, or no kind, which a span made before kinds existed carries.
func isRangeKind(k AnchorKind) bool {
	return k != AnchorBlock && k != AnchorRun && k != AnchorForm
}

// remapSpan carries one span across a rewrite of the runs it anchors to. A
// range span marked exact is dropped when an edit overlaps it (see
// RemapOverlays).
func remapSpan(s Span, bs *boundaries, edits []RunEdit, exact bool) (Span, bool) {
	oldRuns, newRuns := bs.oldRuns, bs.newRuns
	a := s.Range
	switch a.Kind {
	case AnchorBlock:
		return s, true
	case AnchorRun:
		seq, ok := ResolveRunPath(newRuns, a.Path)
		if !ok {
			return Span{}, false
		}
		for _, r := range seq {
			if r.RunID() == a.RunID && a.RunID != "" {
				return s, true
			}
		}
		return Span{}, false
	case AnchorForm:
		seq, ok := ResolveRunPath(newRuns, a.Path)
		if !ok {
			return Span{}, false
		}
		for _, r := range seq {
			if r.Plural != nil {
				if _, ok := r.Plural.Forms[PluralForm(a.Key)]; ok {
					return s, true
				}
			}
			if r.Select != nil {
				if _, ok := r.Select.Cases[a.Key]; ok {
					return s, true
				}
			}
		}
		return Span{}, false
	}
	if len(a.Path) > 0 {
		before, okBefore := ResolveRunPath(oldRuns, a.Path)
		after, okAfter := ResolveRunPath(newRuns, a.Path)
		if okBefore && okAfter && string(CanonicalRunsJSON(before)) == string(CanonicalRunsJSON(after)) {
			return s, true
		}
		return Span{}, false
	}
	if exact {
		return remapExactSpan(s, bs, edits)
	}
	return remapRangeSpan(s, bs)
}

// remapRangeSpan carries a top-level range span across the edits by moving
// each of its boundaries with the text beside it (editIndex.boundary): an edit
// inside the span grows or shrinks it, an edit that replaces all of its text
// leaves it over the replacement, and an edit across one of its boundaries
// leaves it over the part of its text the edit kept. This is the rule
// model.ApplyTextEdits keeps a paired code by, so a marker and a code over the
// same text stay together. The span is dropped when nothing it covered is left.
//
// A span that does not fit the new flattening is dropped rather than clamped:
// the edits then do not describe the rewrite (the position lookup would
// silently pin the span to the end), and a missing span is honest while a
// misplaced one is corrupt.
func remapRangeSpan(s Span, bs *boundaries) (Span, bool) {
	start, end := s.Range.TextSpan(bs.oldRuns)
	var ns, ne int
	if start == end {
		// An empty span marks a point: it moves with the text before it, and
		// goes with an edit that replaces the text around it.
		if bs.edits.inside(start) {
			return Span{}, false
		}
		ns = bs.edits.boundary(start, true)
		ne = ns
	} else {
		ns, ne = bs.edits.boundary(start, true), bs.edits.boundary(end, false)
		if ne <= ns {
			return Span{}, false // the edits deleted everything it covered
		}
	}
	if newLen := runsFlatLen(bs.newRuns); ns < 0 || ne > newLen {
		return Span{}, false // the edits do not describe the rewrite
	}
	ns2 := s
	ns2.Range = SpanAnchor(bs.carry(s.Range.Start, ns), bs.carry(s.Range.End, ne))
	if !ns2.Range.Resolves(bs.newRuns) {
		return Span{}, false // an end falls inside a plural or select
	}
	return ns2, true
}

// remapExactSpan carries a span that states something about the exact text it
// covers: it is dropped when an edit overlaps that text, and otherwise shifted
// by the edits before it.
func remapExactSpan(s Span, bs *boundaries, edits []RunEdit) (Span, bool) {
	start, end := s.Range.TextSpan(bs.oldRuns)
	for _, e := range edits {
		if e.Start < end && start < e.End {
			return Span{}, false // overlaps the edit
		}
	}
	return remapRangeSpan(s, bs)
}

// editIndex answers where a boundary moves across a set of edits in
// logarithmic time, so carrying every span of a long block with many edits
// stays linear in the block.
type editIndex struct {
	edits []RunEdit
	// delta[i] is the length change of edits[:i].
	delta []int
}

func newEditIndex(edits []RunEdit) editIndex {
	ix := editIndex{edits: edits, delta: make([]int, len(edits)+1)}
	for i, e := range edits {
		ix.delta[i+1] = ix.delta[i] + e.NewLen - (e.End - e.Start)
	}
	return ix
}

// boundary maps a span boundary at offset p of the old text to the new text.
// start says the boundary opens the span; otherwise it closes it.
//
// An edit before p shifts it. Text inserted at p goes outside the span: before
// a start, after an end. A boundary strictly inside an edit stays at the same
// offset into the edit's text when the edit keeps its length (a case
// conversion), and otherwise moves to the side of the replacement that keeps
// the replacement out of the span: a start to its end, an end to its start.
// An edit starting at a start, or ending at an end, therefore lies inside the
// span.
func (ix editIndex) boundary(p int, start bool) int {
	edits := ix.edits
	i := sort.Search(len(edits), func(i int) bool { return edits[i].End >= p })
	d := ix.delta[i]
	for ; i < len(edits) && edits[i].End == p; i++ {
		if edits[i].Start == p && !start {
			break // an insertion at an end lies after it
		}
		d += edits[i].NewLen - (edits[i].End - edits[i].Start)
	}
	if i < len(edits) && edits[i].Start < p && p < edits[i].End {
		e := edits[i]
		switch {
		case e.NewLen == e.End-e.Start:
			return p + d
		case start:
			return e.Start + d + e.NewLen
		default:
			return e.Start + d
		}
	}
	return p + d
}

// inside reports whether p lies strictly inside an edit that changes the
// length of its text.
func (ix editIndex) inside(p int) bool {
	edits := ix.edits
	i := sort.Search(len(edits), func(i int) bool { return edits[i].End > p })
	return i < len(edits) && edits[i].Start < p && edits[i].NewLen != edits[i].End-edits[i].Start
}

// boundaries carries range boundaries from a run sequence to its rewrite. The
// edits say where a boundary's text offset moves; the inline codes, which have
// no width there, are paired across the rewrite so a boundary also keeps its
// side of every code beside it.
type boundaries struct {
	oldRuns, newRuns []Run
	// newCode holds, for each run of oldRuns, the index in newRuns of the code
	// it became, or -1 for a text run or a code the rewrite removed.
	newCode []int
	// edits says where a boundary's text offset moves.
	edits editIndex
}

// newBoundaries pairs the codes of oldRuns with those of newRuns in order:
// each old code takes the first equal code after the previous pairing, so a
// code the rewrite removed is skipped and one it added stays unpaired.
func newBoundaries(oldRuns, newRuns []Run) *boundaries {
	keys := make([]string, len(newRuns))
	for j, r := range newRuns {
		if isWidthlessCode(r) {
			keys[j] = codeIdentity(r)
		}
	}
	bs := &boundaries{oldRuns: oldRuns, newRuns: newRuns, newCode: make([]int, len(oldRuns))}
	next := 0
	for i, r := range oldRuns {
		bs.newCode[i] = -1
		if !isWidthlessCode(r) {
			continue
		}
		k := codeIdentity(r)
		for j := next; j < len(newRuns); j++ {
			if keys[j] == k {
				bs.newCode[i], next = j, j+1
				break
			}
		}
	}
	return bs
}

// carry places the boundary at p in the old runs at text offset flat of the
// new runs. Where codes sit at that offset, the boundary goes after each one
// that came from a code before p; otherwise it takes the first position at the
// offset, so codes nothing places lead the span that follows.
func (bs *boundaries) carry(p RunPos, flat int) RunPos {
	r, off := runPosition(bs.newRuns, flat)
	if off > 0 {
		return RunPos{Run: r, Offset: off}
	}
	end := r
	for end < len(bs.newRuns) && runFlatLen(bs.newRuns[end]) == 0 {
		end++
	}
	at := r
	for i := 0; i < p.Run && i < len(bs.newCode); i++ {
		if j := bs.newCode[i]; j >= at && j < end {
			at = j + 1
		}
	}
	return RunPos{Run: at}
}

// isWidthlessCode reports whether a run is a code the text flattening gives no
// width: a placeholder, either half of a paired code, a subblock reference, or
// a plural or select whose branch is empty.
func isWidthlessCode(r Run) bool {
	return r.Text == nil && runFlatLen(r) == 0
}

// codeIdentity names a code for pairing across a rewrite: its kind and id, or,
// for a run that carries no id, its canonical JSON.
func codeIdentity(r Run) string {
	if id := r.RunID(); id != "" {
		return string(r.Kind()) + ":" + id
	}
	return string(CanonicalRunsJSON([]Run{r}))
}

// DropOverlays removes every overlay on one edition of b (edition nil is the
// edition Source holds): the opaque rewrite path (AD-006), where a whole
// replacement with no derivable mapping cannot rebase run-anchored spans.
// Overlays on other editions are untouched. It returns the number of overlays
// dropped.
func DropOverlays(b *Block, edition *VariantKey) int {
	if b == nil || len(b.Overlays) == 0 {
		return 0
	}
	dropped := 0
	out := b.Overlays[:0]
	for _, o := range b.Overlays {
		if o.onEdition(edition) {
			dropped++
			continue
		}
		out = append(out, o)
	}
	b.Overlays = out
	return dropped
}

// OverlaysInBounds reports whether every span of every overlay on one edition
// (edition nil is the edition Source holds) anchors to a valid position in
// runs, the edition's content. It is the backstop after a rewrite: the rewrite
// must drop or rebase (RemapOverlays) the edition's overlays so no span
// dangles. When a span is out of bounds it returns that overlay's type and
// false.
func (b *Block) OverlaysInBounds(edition *VariantKey, runs []Run) (OverlayType, bool) {
	for i := range b.Overlays {
		o := &b.Overlays[i]
		if !o.onEdition(edition) {
			continue
		}
		for _, s := range o.Spans {
			if !s.Range.Resolves(runs) {
				return o.Type, false
			}
		}
	}
	return "", true
}

// Resolves reports whether the anchor addresses something that exists in runs:
// any block anchor; a run anchor whose path resolves and holds a run with its
// id; a form anchor whose path resolves and holds a plural or select with that
// branch; a range anchor whose path resolves and whose positions are in bounds
// of the sequence it reaches (InBounds).
func (a Anchor) Resolves(runs []Run) bool {
	seq, ok := ResolveRunPath(runs, a.Path)
	if !ok {
		return false
	}
	switch a.Kind {
	case AnchorBlock:
		return true
	case AnchorRun:
		for _, r := range seq {
			if a.RunID != "" && r.RunID() == a.RunID {
				return true
			}
		}
		return false
	case AnchorForm:
		for _, r := range seq {
			if r.Plural != nil {
				if _, ok := r.Plural.Forms[PluralForm(a.Key)]; ok {
					return true
				}
			}
			if r.Select != nil {
				if _, ok := r.Select.Cases[a.Key]; ok {
					return true
				}
			}
		}
		return false
	}
	return a.InBounds(seq)
}

// ResolveRunPath returns the run sequence path addresses inside runs. An empty
// path is runs itself. An index step picks a run of the current sequence, which
// must be a plural or a select; the step after it names the form or case whose
// sequence the walk continues in. A path that ends on an index, indexes past a
// sequence, or names a branch the run does not have resolves to nothing.
func ResolveRunPath(runs []Run, path RunPath) ([]Run, bool) {
	seq := runs
	var at *Run
	for _, step := range path {
		switch step.Kind {
		case StepIndex:
			if at != nil || step.Index < 0 || step.Index >= len(seq) {
				return nil, false
			}
			at = &seq[step.Index]
		case StepPlural:
			if at == nil || at.Plural == nil {
				return nil, false
			}
			form, ok := at.Plural.Forms[step.PluralForm]
			if !ok {
				return nil, false
			}
			seq, at = form, nil
		case StepSelect:
			if at == nil || at.Select == nil {
				return nil, false
			}
			c, ok := at.Select.Cases[step.SelectValue]
			if !ok {
				return nil, false
			}
			seq, at = c, nil
		default:
			return nil, false
		}
	}
	if at != nil {
		return nil, false
	}
	return seq, true
}
