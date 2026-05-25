package model

// This file defines the stand-off overlay model (AD-002). A Block's content
// is a flat []Run per locale; every interpretation *of* that content —
// sentence segmentation, terminology, entities, QA findings, source↔target
// alignment — is a typed, run-anchored Overlay layered over the runs rather
// than baked into the structure. Overlays are produced on demand and never
// rewrite the runs they describe, so segmentation is opt-in, multi-layer, and
// reversible (dropping the overlay restores the unsegmented content).

// OverlayType names a stand-off interpretation layered over a run sequence.
type OverlayType string

const (
	// OverlaySegmentation marks sentence / chunk boundaries over the runs.
	OverlaySegmentation OverlayType = "segmentation"
	// OverlayTerm marks matched terminology spans.
	OverlayTerm OverlayType = "term"
	// OverlayEntity marks recognized named-entity spans.
	OverlayEntity OverlayType = "entity"
	// OverlayQA marks quality-check findings.
	OverlayQA OverlayType = "qa"
	// OverlayAlignment links source spans to target spans.
	OverlayAlignment OverlayType = "alignment"
)

// RunRange anchors a span on a []Run sequence: a start and end run position
// plus an intra-text-run character offset, so boundaries stay stable across
// inline-code runs and survive run-preserving edits. Offsets count runes into
// the TextRun at StartRun / EndRun; a range that begins or ends on an
// inline-code run uses offset 0. The range is half-open: [start, end).
type RunRange struct {
	StartRun    int `json:"startRun"`
	StartOffset int `json:"startOffset"`
	EndRun      int `json:"endRun"`
	EndOffset   int `json:"endOffset"`
}

// IsZero reports whether the range is the zero value.
func (r RunRange) IsZero() bool { return r == RunRange{} }

// Span is one entry in an Overlay: a run-anchored range with an optional
// overlay-local id (e.g. a segment id "s1") and type-specific properties.
type Span struct {
	ID    string            `json:"id,omitempty"`
	Range RunRange          `json:"range"`
	Props map[string]string `json:"props,omitempty"`
}

// Overlay is a typed stand-off layer over one side of a Block: the source
// (Variant nil) or a specific target variant. Spans are ordered by position.
type Overlay struct {
	Type    OverlayType `json:"type"`
	Variant *VariantKey `json:"variant,omitempty"` // nil = source side
	Spans   []Span      `json:"spans,omitempty"`
}

// OnSource reports whether the overlay annotates the source run sequence.
func (o *Overlay) OnSource() bool { return o == nil || o.Variant == nil }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ExtractRuns returns the sub-sequence of runs covered by this half-open
// range [start, end). A position is (run index, rune offset into that run's
// text); inline-code runs are atomic and included whole unless they fall on
// the exclusive end boundary. Boundary text runs are split at their offsets.
func (r RunRange) ExtractRuns(runs []Run) []Run {
	n := len(runs)
	if r.StartRun < 0 || r.StartRun > n || r.EndRun < r.StartRun {
		return nil
	}
	out := make([]Run, 0, r.EndRun-r.StartRun+1)
	for i := r.StartRun; i <= r.EndRun && i < n; i++ {
		run := runs[i]
		if run.Text != nil {
			rs := []rune(run.Text.Text)
			s, e := 0, len(rs)
			if i == r.StartRun {
				s = clampInt(r.StartOffset, 0, len(rs))
			}
			if i == r.EndRun {
				e = clampInt(r.EndOffset, 0, len(rs))
			}
			if s < e {
				out = append(out, Run{Text: &TextRun{Text: string(rs[s:e])}})
			}
			continue
		}
		// Inline-code run: include whole unless it is the exclusive end.
		if i == r.EndRun && r.EndOffset == 0 {
			continue
		}
		out = append(out, run)
	}
	return out
}

// runPosition locates a rune offset within a run sequence's flattened text,
// returning the run index and rune offset within that run's text. Inline-code
// runs have zero text width; a boundary at the end of a text run is attributed
// to the start of the following run, so leading codes attach to the next span.
func runPosition(runs []Run, runeOffset int) (int, int) {
	if runeOffset <= 0 {
		return 0, 0
	}
	pos := 0
	for i, r := range runs {
		if r.Text == nil {
			continue
		}
		l := len([]rune(r.Text.Text))
		if runeOffset < pos+l {
			return i, runeOffset - pos
		}
		if runeOffset == pos+l {
			return i + 1, 0
		}
		pos += l
	}
	return len(runs), 0
}

// RunRangeFor builds a RunRange covering the half-open rune span
// [startRune, endRune) of a run sequence's flattened text.
func RunRangeFor(runs []Run, startRune, endRune int) RunRange {
	sr, so := runPosition(runs, startRune)
	er, eo := runPosition(runs, endRune)
	return RunRange{StartRun: sr, StartOffset: so, EndRun: er, EndOffset: eo}
}

// SegmentationFor returns the segmentation overlay for the given side (nil =
// source), or nil if none.
func (b *Block) SegmentationFor(variant *VariantKey) *Overlay {
	for i := range b.Overlays {
		o := &b.Overlays[i]
		if o.Type != OverlaySegmentation {
			continue
		}
		if sameVariant(o.Variant, variant) {
			return o
		}
	}
	return nil
}

func sameVariant(a, b *VariantKey) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// SetSegmentation replaces the segmentation overlay for the given side
// (nil = source) with one carrying the supplied spans. Empty spans removes it.
func (b *Block) SetSegmentation(variant *VariantKey, spans []Span) {
	out := b.Overlays[:0]
	for _, o := range b.Overlays {
		if o.Type == OverlaySegmentation && sameVariant(o.Variant, variant) {
			continue
		}
		out = append(out, o)
	}
	b.Overlays = out
	if len(spans) > 0 {
		b.Overlays = append(b.Overlays, Overlay{Type: OverlaySegmentation, Variant: variant, Spans: spans})
	}
}

// SourceSegmentation returns the source-side segmentation overlay, or nil.
func (b *Block) SourceSegmentation() *Overlay {
	for i := range b.Overlays {
		if b.Overlays[i].Type == OverlaySegmentation && b.Overlays[i].OnSource() {
			return &b.Overlays[i]
		}
	}
	return nil
}

// SourceSegmentRuns returns the runs of the idx-th source segment span. With
// no source segmentation overlay, idx 0 returns the whole source and any
// other index returns nil.
func (b *Block) SourceSegmentRuns(idx int) []Run {
	seg := b.SourceSegmentation()
	if seg == nil {
		if idx == 0 {
			return b.Source
		}
		return nil
	}
	if idx < 0 || idx >= len(seg.Spans) {
		return nil
	}
	return seg.Spans[idx].Range.ExtractRuns(b.Source)
}

// SourceSegmentCount returns the number of source segment spans — len(spans)
// when a segmentation overlay is present, otherwise 1 for a non-empty block
// (0 when empty).
func (b *Block) SourceSegmentCount() int {
	if seg := b.SourceSegmentation(); seg != nil {
		return len(seg.Spans)
	}
	if len(b.Source) > 0 {
		return 1
	}
	return 0
}
