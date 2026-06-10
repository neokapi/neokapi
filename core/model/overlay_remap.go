package model

// Overlay rebasing for structured source transforms (AD-002 / AD-006). A source
// transform that rewrites the runs normally invalidates every run-anchored
// source overlay — segmentation, terms, entities — because their ranges no
// longer line up. A *structured* transform (e.g. redaction, whose edits are a
// known span→replacement map) can instead remap the surviving overlays so they
// follow the rewrite and still reach the main stage. Arbitrary rewrites (LLM
// simplification) have no derivable mapping and must drop their source overlays
// rather than call RemapOverlays.

// RunEdit describes one edit applied to a run sequence's flattened text (the
// RunsText coordinate space, in rune offsets): the half-open old span
// [Start, End) was replaced by content contributing NewLen runes to the new
// flattening. A deletion — or replacement by a zero-width inline run, as a
// redaction placeholder is — has NewLen == 0; a pure insertion has Start == End.
// Edits are expressed against the pre-edit text and must be sorted ascending and
// non-overlapping.
type RunEdit struct {
	Start  int
	End    int
	NewLen int
}

// RemapOverlays rebases every source-side overlay span of b onto the block's
// current (already-rewritten) source runs, given the structured edits applied to
// the flattened source text. oldRuns are the runs the existing spans anchor to
// (the pre-rewrite source); b.Source holds the new runs. A span that overlaps any
// edit is dropped — its content changed, so it can no longer anchor cleanly; a
// span lying entirely outside every edit is shifted by the cumulative length
// delta of the edits before it and re-anchored to b.Source. An overlay left with
// no spans is removed. Target-side overlays are untouched. It returns the number
// of spans dropped.
//
// With no edits the call still re-anchors: a structure-only rewrite (runs
// added, removed, or reclassified without changing the text flattening) shifts
// run indices, so every span is re-projected through its text range onto the
// new runs.
func RemapOverlays(b *Block, oldRuns []Run, edits []RunEdit) int {
	if b == nil || len(b.Overlays) == 0 {
		return 0
	}
	dropped := 0
	out := b.Overlays[:0]
	for oi := range b.Overlays {
		o := b.Overlays[oi]
		if !o.OnSource() {
			out = append(out, o)
			continue
		}
		kept := o.Spans[:0]
		for _, s := range o.Spans {
			ns, ok := remapSpan(s, oldRuns, b.Source, edits)
			if !ok {
				dropped++
				continue
			}
			kept = append(kept, ns)
		}
		o.Spans = kept
		if len(o.Spans) == 0 {
			continue // drop the now-empty overlay
		}
		out = append(out, o)
	}
	b.Overlays = out
	return dropped
}

// remapSpan projects s from oldRuns to the flattened-text rune span it covers,
// drops it if it overlaps any edit, and otherwise shifts it by the cumulative
// delta of edits before it and re-anchors it to newRuns. Edits are ascending and
// non-overlapping, so a surviving span has every edit entirely before its start
// or entirely after its end — both endpoints carry the same delta.
//
// A shifted span that does not fit the new flattening is dropped rather than
// clamped: the edits then do not describe the rewrite (RunRangeFor would
// silently mis-anchor the span at the end), and a missing span is honest while
// a misplaced one is corrupt.
func remapSpan(s Span, oldRuns, newRuns []Run, edits []RunEdit) (Span, bool) {
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
	if newLen := len([]rune(RunsText(newRuns))); start+delta < 0 || end+delta > newLen {
		return Span{}, false // the edits do not describe the rewrite
	}
	ns := s
	ns.Range = RunRangeFor(newRuns, start+delta, end+delta)
	return ns, true
}

// DropSourceOverlays removes every source-side overlay from b — the opaque
// rewrite path (AD-006): a whole-source replacement with no derivable mapping
// cannot rebase run-anchored spans, so the framework applier drops them rather
// than leave them dangling. Target-side overlays are untouched. It returns the
// number of overlays dropped.
func DropSourceOverlays(b *Block) int {
	if b == nil || len(b.Overlays) == 0 {
		return 0
	}
	dropped := 0
	out := b.Overlays[:0]
	for _, o := range b.Overlays {
		if o.OnSource() {
			dropped++
			continue
		}
		out = append(out, o)
	}
	b.Overlays = out
	return dropped
}

// SourceOverlaysInBounds reports whether every source-side overlay span anchors
// to a valid range within the current source runs. It is the post-rewrite
// backstop for source transforms: after rewriting source a transform must drop
// or rebase (RemapOverlays) its source overlays so no span dangles. When a span
// is out of bounds it returns that overlay's type and false.
func (b *Block) SourceOverlaysInBounds() (OverlayType, bool) {
	for i := range b.Overlays {
		o := &b.Overlays[i]
		if !o.OnSource() {
			continue
		}
		for _, s := range o.Spans {
			if !s.Range.InBounds(b.Source) {
				return o.Type, false
			}
		}
	}
	return "", true
}
