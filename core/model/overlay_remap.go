package model

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
// A range span that overlaps an edit is dropped: its content changed, so it
// can no longer anchor cleanly. One lying entirely outside every edit is
// shifted by the cumulative length delta of the edits before it and
// re-anchored to newRuns. A range span inside a plural or select branch (its
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
// With no edits the call still re-anchors: a structure-only rewrite (runs
// added, removed, or reclassified without changing the text flattening) shifts
// run indices, so every range span is re-projected through its text range onto
// the new runs.
func RemapOverlays(b *Block, edition *VariantKey, oldRuns, newRuns []Run, edits []RunEdit) int {
	if b == nil || len(b.Overlays) == 0 {
		return 0
	}
	dropped := 0
	out := b.Overlays[:0]
	for oi := range b.Overlays {
		o := b.Overlays[oi]
		if !o.onEdition(edition) {
			out = append(out, o)
			continue
		}
		if o.Type == OverlaySegmentation {
			spans, ok := remapPartition(o.Spans, oldRuns, newRuns, edits)
			if !ok {
				dropped += len(o.Spans)
				continue
			}
			o.Spans = spans
		} else {
			kept := make([]Span, 0, len(o.Spans))
			for _, s := range o.Spans {
				ns, ok := remapSpan(s, oldRuns, newRuns, edits)
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
func remapPartition(spans []Span, oldRuns, newRuns []Run, edits []RunEdit) ([]Span, bool) {
	type flat struct {
		start, end int
		ok         bool
	}
	pos := make([]flat, len(spans))
	for i, s := range spans {
		if isRangeKind(s.Range.Kind) && len(s.Range.Path) == 0 {
			start, end := s.Range.TextSpan(oldRuns)
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

	newLen := runsFlatLen(newRuns)
	out := make([]Span, 0, len(spans))
	for i, s := range spans {
		if !pos[i].ok {
			ns, ok := remapSpan(s, oldRuns, newRuns, edits)
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
		ns.Range = RangeAnchor(newRuns, start, end)
		if !ns.Range.Resolves(newRuns) {
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

// remapSpan carries one span across a rewrite of the runs it anchors to.
func remapSpan(s Span, oldRuns, newRuns []Run, edits []RunEdit) (Span, bool) {
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
	return remapRangeSpan(s, oldRuns, newRuns, edits)
}

// remapRangeSpan projects a top-level range span from oldRuns to the
// flattened-text rune span it covers, drops it if it overlaps any edit, and
// otherwise shifts it by the cumulative delta of edits before it and
// re-anchors it to newRuns. Edits are ascending and non-overlapping, so a
// surviving span has every edit entirely before its start or entirely after
// its end, so both endpoints carry the same delta.
//
// A shifted span that does not fit the new flattening is dropped rather than
// clamped: the edits then do not describe the rewrite (RangeAnchor would
// silently mis-anchor the span at the end), and a missing span is honest while
// a misplaced one is corrupt.
func remapRangeSpan(s Span, oldRuns, newRuns []Run, edits []RunEdit) (Span, bool) {
	start, end := s.Range.TextSpan(oldRuns)
	delta := 0
	for _, e := range edits {
		if e.Start < end && start < e.End {
			return Span{}, false // overlaps the edit
		}
		if e.End <= start {
			delta += e.NewLen - (e.End - e.Start)
		}
	}
	if newLen := runsFlatLen(newRuns); start+delta < 0 || end+delta > newLen {
		return Span{}, false // the edits do not describe the rewrite
	}
	ns := s
	ns.Range = RangeAnchor(newRuns, start+delta, end+delta)
	if !ns.Range.Resolves(newRuns) {
		return Span{}, false // an end falls inside a plural or select
	}
	return ns, true
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
