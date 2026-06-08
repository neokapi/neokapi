package model

import "unicode/utf8"

// This file defines the stand-off overlay model (AD-002). A Block's content
// is a flat []Run per locale; every interpretation *of* that content —
// sentence segmentation, terminology, entities, QA findings, source↔target
// alignment — is a typed, run-anchored Overlay layered over the runs rather
// than baked into the structure. Overlays are produced on demand and never
// rewrite the runs they describe, so segmentation is opt-in, multi-layer, and
// reversible (dropping the overlay restores the unsegmented content).

// OverlayType is a deprecated alias for FacetType (see facet.go); overlays are
// facets. The Overlay* constants below alias the canonical Facet* constants.
type OverlayType = FacetType

const (
	// OverlaySegmentation marks sentence / chunk boundaries over the runs.
	OverlaySegmentation = FacetSegmentation
	// OverlayTerm marks matched terminology spans.
	OverlayTerm = FacetTerm
	// OverlayEntity marks recognized named-entity spans.
	OverlayEntity = FacetEntity
	// OverlayQA marks quality-check findings.
	OverlayQA = FacetQA
	// OverlayAlignment links source spans to target spans.
	OverlayAlignment = FacetAlignment
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

// SpanPropIgnorable, when set to "true" on a segmentation span's Props, marks
// that span as non-translatable structural content — an okapi "ignorable"
// TextPart, e.g. an xliff2 <ignorable>, inter-segment whitespace, or an ICU
// plural selector. Translation tools and bilingual round-trips preserve such a
// span's target verbatim instead of translating it; the span still occupies
// its run range so neighbouring segment positions stay aligned. It is the
// format-agnostic marker shared by the native readers and the okapi bridge.
const SpanPropIgnorable = "ignorable"

// Span is one entry in a Facet: a run-anchored range with an optional
// facet-local id (e.g. a segment id "s1"), type-specific properties, and an
// optional typed payload Value. A block-scoped facet (the former annotation)
// uses a single span with a zero Range and a Value; a positional facet uses
// one or more spans with real ranges.
type Span struct {
	ID    string            `json:"id,omitempty"`
	Range RunRange          `json:"range"`
	Props map[string]string `json:"props,omitempty"`
	Value any               `json:"value,omitempty"` // typed payload (facet registry)
}

// Ignorable reports whether the span is marked as non-translatable structural
// content (see [SpanPropIgnorable]).
func (s Span) Ignorable() bool { return s.Props[SpanPropIgnorable] == "true" }

// Facet is a typed stand-off layer over one side of a Block: the source
// (Variant nil) or a specific target variant. It is the single carrier for
// stand-off block data — positional interpretations (segmentation, term,
// entity, qa, alignment) carry ranged spans, while block-scoped facets (the
// former annotations: notes, alt-translations, analysis results, format
// round-trip state) carry a single span with a Value and a zero range.
// Overlay is a deprecated alias kept while positional call sites migrate.
type Facet = Overlay

// Overlay is a typed stand-off layer over one side of a Block: the source
// (Variant nil) or a specific target variant. Spans are ordered by position.
//
// Layer names a segmentation granularity so several can coexist over the same
// runs (AD-002): the empty string is the primary sentence segmentation — the
// one bilingual formats (XLIFF 2.0 <segment>, TMX <seg>) project to and from —
// while named layers ("llm-chunk", "clause", …) are additional interpretations
// produced on demand. Layer is meaningful only for segmentation overlays.
type Overlay struct {
	Type    OverlayType `json:"type"`
	Variant *VariantKey `json:"variant,omitempty"` // nil = source side
	Layer   string      `json:"layer,omitempty"`   // "" = primary sentence segmentation
	Spans   []Span      `json:"spans,omitempty"`
}

// OnSource reports whether the overlay annotates the source run sequence.
func (o *Overlay) OnSource() bool { return o == nil || o.Variant == nil }

func clampInt(v, lo, hi int) int {
	return min(max(v, lo), hi)
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

// runFlatLen returns the rune width a single run contributes to the text-only
// flattening produced by [RunsText]: a TextRun contributes its rune count,
// inline-code runs (Ph / PcOpen / PcClose / Sub) contribute nothing, and a
// plural / select run contributes the width of its 'other' branch (or its first
// branch when 'other' is absent) — recursively, since branch forms are
// themselves run sequences. This mirrors runsTextTo's branch selection exactly
// so overlay locators agree with RunsText on plural/select-bearing blocks.
func runFlatLen(r Run) int {
	switch {
	case r.Text != nil:
		return len([]rune(r.Text.Text))
	case r.Plural != nil:
		if form, ok := r.Plural.Forms[PluralOther]; ok {
			return runsFlatLen(form)
		}
		for _, form := range r.Plural.Forms {
			return runsFlatLen(form)
		}
	case r.Select != nil:
		if form, ok := r.Select.Cases["other"]; ok {
			return runsFlatLen(form)
		}
		for _, form := range r.Select.Cases {
			return runsFlatLen(form)
		}
	}
	return 0
}

// runsFlatLen returns the total rune width of a run sequence's text-only
// flattening (the rune length of [RunsText]).
func runsFlatLen(runs []Run) int {
	n := 0
	for _, r := range runs {
		n += runFlatLen(r)
	}
	return n
}

// runPosition locates a rune offset within a run sequence's flattened text,
// returning the run index and rune offset within that run's text. Inline-code
// runs have zero text width; plural / select runs contribute the width of their
// flattened 'other' branch (matching [RunsText]). A boundary at the end of a
// text-bearing run is attributed to the start of the following run, so leading
// codes attach to the next span.
func runPosition(runs []Run, runeOffset int) (int, int) {
	if runeOffset <= 0 {
		return 0, 0
	}
	pos := 0
	for i, r := range runs {
		l := runFlatLen(r)
		if l == 0 {
			continue
		}
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

// TextSpan projects a RunRange back to character offsets [start, end) in the
// text-only flattening of runs (RunsText) — the inverse of RunRangeFor.
// Useful for UI / wire consumers that highlight over the flattened source text.
func (r RunRange) TextSpan(runs []Run) (start, end int) {
	return runTextOffset(runs, r.StartRun, r.StartOffset), runTextOffset(runs, r.EndRun, r.EndOffset)
}

func runTextOffset(runs []Run, runIdx, off int) int {
	pos := 0
	for i := 0; i < runIdx && i < len(runs); i++ {
		pos += runFlatLen(runs[i])
	}
	if runIdx >= 0 && runIdx < len(runs) && runFlatLen(runs[runIdx]) > 0 {
		pos += off
	}
	return pos
}

// ByteSpan projects a RunRange to byte offsets [start, end) in the text-only
// flattening of runs (RunsText), for consumers that byte-index the source text.
func (r RunRange) ByteSpan(runs []Run) (start, end int) {
	text := RunsText(runs)
	rs, re := r.TextSpan(runs)
	return runeToByteOffset(text, rs), runeToByteOffset(text, re)
}

func runeToByteOffset(s string, runeOff int) int {
	if runeOff <= 0 {
		return 0
	}
	n := 0
	for i := range s {
		if n == runeOff {
			return i
		}
		n++
	}
	return len(s)
}

// RunRangeForBytes builds a RunRange for a byte-offset span [byteStart, byteEnd)
// into the runs' text-only flattening (RunsText). Convenience for entity/term
// detectors that report byte offsets into the source text; it converts to rune
// offsets so the resulting range is correct for non-ASCII content too.
func RunRangeForBytes(runs []Run, byteStart, byteEnd int) RunRange {
	text := RunsText(runs)
	toRune := func(b int) int {
		if b <= 0 {
			return 0
		}
		if b > len(text) {
			b = len(text)
		}
		return utf8.RuneCountInString(text[:b])
	}
	return RunRangeFor(runs, toRune(byteStart), toRune(byteEnd))
}

// SegmentationFor returns the primary (layer "") segmentation overlay for the
// given side (nil = source), or nil if none.
func (b *Block) SegmentationFor(variant *VariantKey) *Overlay {
	return b.SegmentationLayerFor(variant, "")
}

// SegmentationLayerFor returns the segmentation overlay for the given side
// (nil = source) and layer ("" = primary sentence segmentation), or nil.
func (b *Block) SegmentationLayerFor(variant *VariantKey, layer string) *Overlay {
	for i := range b.Overlays {
		o := &b.Overlays[i]
		if o.Type != OverlaySegmentation {
			continue
		}
		if sameVariant(o.Variant, variant) && o.Layer == layer {
			return o
		}
	}
	return nil
}

// SegmentationLayers lists the layer names of every segmentation overlay on
// the given side (nil = source), in overlay order. The primary layer reports
// as "".
func (b *Block) SegmentationLayers(variant *VariantKey) []string {
	var layers []string
	for i := range b.Overlays {
		o := &b.Overlays[i]
		if o.Type == OverlaySegmentation && sameVariant(o.Variant, variant) {
			layers = append(layers, o.Layer)
		}
	}
	return layers
}

func sameVariant(a, b *VariantKey) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// SetSegmentation replaces the primary (layer "") segmentation overlay for the
// given side (nil = source) with one carrying the supplied spans. Empty spans
// removes it.
func (b *Block) SetSegmentation(variant *VariantKey, spans []Span) {
	b.SetSegmentationLayer(variant, "", spans)
}

// SetSegmentationLayer replaces the segmentation overlay for the given side
// (nil = source) and layer ("" = primary sentence segmentation) with one
// carrying the supplied spans, leaving other layers untouched. Empty spans
// removes that layer.
func (b *Block) SetSegmentationLayer(variant *VariantKey, layer string, spans []Span) {
	out := b.Overlays[:0]
	for _, o := range b.Overlays {
		if o.Type == OverlaySegmentation && sameVariant(o.Variant, variant) && o.Layer == layer {
			continue
		}
		out = append(out, o)
	}
	b.Overlays = out
	if len(spans) > 0 {
		b.Overlays = append(b.Overlays, Overlay{Type: OverlaySegmentation, Variant: variant, Layer: layer, Spans: spans})
	}
}

// HasSourceOverlays reports whether the block carries any source-side
// positional facet (segmentation, terms, entities, …). Source mutation after
// such a facet is attached would invalidate its run-anchored ranges. Only
// positional facets count: block-scoped facets (the former annotations — notes,
// alt-translations, redaction secrets, format round-trip state) carry no
// run-anchored range, so a source rewrite does not invalidate them.
func (b *Block) HasSourceOverlays() bool {
	for i := range b.Overlays {
		f := &b.Overlays[i]
		if f.OnSource() && f.Type.IsPositional() {
			return true
		}
	}
	return false
}

// SourceSegmentation returns the primary (layer "") source-side segmentation
// overlay, or nil.
func (b *Block) SourceSegmentation() *Overlay {
	for i := range b.Overlays {
		o := &b.Overlays[i]
		if o.Type == OverlaySegmentation && o.OnSource() && o.Layer == "" {
			return o
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
