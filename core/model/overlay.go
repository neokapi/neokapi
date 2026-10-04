package model

// This file defines the stand-off overlay model (AD-002). A Block's content
// is a flat []Run per locale; every interpretation *of* that content —
// sentence segmentation, terminology, entities, check findings, source↔target
// alignment — is a typed, run-anchored Overlay layered over the runs rather
// than baked into the structure. Overlays are produced on demand and never
// rewrite the runs they describe, so segmentation is opt-in, multi-layer, and
// reversible (dropping the overlay restores the unsegmented content).

import "unicode/utf8"

// OverlayType names a kind of positional, run-anchored stand-off
// interpretation. Built-in content overlays have stable constants below;
// formats and plugins may use any string for their own run-anchored state.
// Block-scoped metadata is not an overlay — it rides on Block.Annotations as a
// keyed typed payload (see annotation.go).
type OverlayType string

const (
	// OverlaySegmentation marks sentence / chunk boundaries over the runs.
	OverlaySegmentation OverlayType = "segmentation"
	// OverlayTerm marks matched terminology spans.
	OverlayTerm OverlayType = "term"
	// OverlayEntity marks recognized named-entity spans.
	OverlayEntity OverlayType = "entity"
	// OverlayCheck marks quality-check findings.
	OverlayCheck OverlayType = "qa"
	// OverlayAlignment links source spans to target spans.
	OverlayAlignment OverlayType = "alignment"
	// OverlayTermCandidate marks proposed terminology spans awaiting review.
	OverlayTermCandidate OverlayType = "term-candidate"
)

// SpanPropIgnorable, when set to "true" on a segmentation span's Props, marks
// that span as non-content structural material — an okapi "ignorable"
// TextPart, e.g. an xliff2 <ignorable>, inter-segment whitespace, or an ICU
// plural selector. Translation tools and bilingual round-trips preserve such a
// span's target verbatim instead of translating it; the span still occupies
// its run range so neighbouring segment positions stay aligned. It is the
// format-agnostic marker shared by the native readers and the okapi bridge.
const SpanPropIgnorable = "ignorable"

// Span is one occurrence within an Overlay: a run-anchored Range (its
// position), an optional overlay-local id (e.g. a segment id "s1"),
// type-specific Props, and an optional typed payload Value. Block-scoped
// metadata is not a span — it rides on Block.Annotations (see annotation.go).
type Span struct {
	ID    string            `json:"id,omitempty"`
	Range Anchor            `json:"range"`
	Props map[string]string `json:"props,omitempty"`
	Value Payload           `json:"value,omitempty"` // typed payload (payload registry)
}

// Ignorable reports whether the span is marked as non-content structural
// material (see [SpanPropIgnorable]).
func (s Span) Ignorable() bool { return s.Props[SpanPropIgnorable] == "true" }

// Overlay is a typed, positional (run-anchored) stand-off layer over one
// edition of a Block, which Edition names: the zero key names the edition the
// block was read in, and any other key the edition filed under it. The
// overlays on a translation filed under no language, which has no key, sit
// apart from Block.Overlays (target_overlay.go). An overlay's spans carry real
// ranges into the runs — segmentation, terminology, entities, check findings,
// alignment. Block-scoped metadata that has no position (notes,
// alt-translations, analysis results, format round-trip state) is not an
// overlay; it rides on Block.Annotations (see annotation.go). Spans are ordered
// by position.
//
// Layer names a segmentation granularity so several can coexist over the same
// runs (AD-002): LayerPrimary is the primary sentence segmentation — the one
// bilingual formats (XLIFF 2.0 <segment>, TMX <seg>) project to and from —
// while named layers ("llm-chunk", "clause", …) are additional interpretations
// produced on demand. Layer is meaningful only for segmentation overlays.
type Overlay struct {
	Type OverlayType `json:"type"`
	// Edition names the edition the spans anchor to. Its JSON form is the
	// key's text form under "variant", left out for the zero key.
	Edition EditionKey `json:"variant,omitzero"`
	Layer   string     `json:"layer,omitempty"` // LayerPrimary ("") = primary sentence segmentation
	Spans   []Span     `json:"spans,omitempty"`
}

// LayerPrimary names the primary sentence segmentation layer — the one bilingual
// formats (XLIFF 2.0 <segment>, TMX <seg>) project to and from, and the default
// for the unit iterators. It is the zero value of Overlay.Layer (and of the
// layer parameter on the segmentation/unit APIs); named layers ("llm-chunk",
// "clause", …) are additional, on-demand interpretations.
const LayerPrimary = ""

// OnSource reports whether the overlay annotates the edition the block was
// read in: whether it names the zero key.
func (o *Overlay) OnSource() bool { return o == nil || o.Edition.IsZero() }

func clampInt(v, lo, hi int) int {
	return min(max(v, lo), hi)
}

func offsetInBounds(runs []Run, idx, off int) bool {
	if off < 0 {
		return false
	}
	if idx == len(runs) {
		return off == 0 // end boundary just past the last run
	}
	if r := runs[idx]; r.Text != nil {
		return off <= len([]rune(r.Text.Text))
	}
	return off == 0 // inline-code / non-text run carries offset 0
}

// SpansTile reports whether spans divide runs into consecutive pieces, the
// way a segmentation that still describes its runs divides them: each span is
// a range over the top-level runs that is in bounds, the first starts at the
// beginning, each next one starts where the one before it ended, and the last
// ends past the last run. A boundary may fall inside a text run, as one does
// after a rewrite that joined two segments' text runs and rebased the spans
// (RemapOverlays). The end of a text run and the start of the run after it are
// one boundary however a span spells it. Spans that tile give back every run
// exactly once, in order, when each is extracted with Anchor.ExtractRuns.
//
// An empty list tiles only runs that hold nothing.
func SpansTile(spans []Span, runs []Run) bool {
	at := canonicalRunPos(runs, RunPos{})
	for _, s := range spans {
		r := s.Range
		if !isRangeKind(r.Kind) || len(r.Path) > 0 || !r.InBounds(runs) {
			return false
		}
		start, end := canonicalRunPos(runs, r.Start), canonicalRunPos(runs, r.End)
		if start != at || end.Run < start.Run || (end.Run == start.Run && end.Offset < start.Offset) {
			return false
		}
		at = end
	}
	return at == RunPos{Run: len(runs)}
}

// canonicalRunPos spells an in-bounds boundary one way: a position at the end
// of a text run is the start of the run after it.
func canonicalRunPos(runs []Run, p RunPos) RunPos {
	for p.Run < len(runs) && runs[p.Run].Text != nil && p.Offset == utf8.RuneCountInString(runs[p.Run].Text.Text) {
		p = RunPos{Run: p.Run + 1}
	}
	return p
}

// runFlatLen returns the rune width a single run contributes to the text-only
// flattening produced by [RunsText]: a TextRun contributes its rune count,
// inline-code runs (Ph / PcOpen / PcClose / Sub) contribute nothing, and a
// plural / select run contributes the width of its 'other' branch (or its first
// branch when 'other' is absent) — recursively, since branch forms are
// themselves run sequences. This mirrors runsTextTo's branch selection exactly
// so overlay locators agree with RunsText on plural/select-bearing blocks.
func runFlatLen(r Run) int {
	switch r.Kind() {
	case RunKindText:
		return len([]rune(r.Text.Text))
	case RunKindPlural:
		if form, ok := r.Plural.Forms[PluralOther]; ok {
			return runsFlatLen(form)
		}
		for _, form := range r.Plural.Forms {
			return runsFlatLen(form)
		}
	case RunKindSelect:
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

// SegmentationFor returns the primary (layer "") segmentation overlay on
// edition k (the zero key for the edition the block was read in), or nil if
// none.
func (b *Block) SegmentationFor(k EditionKey) *Overlay {
	return b.SegmentationLayerFor(k, LayerPrimary)
}

// SegmentationLayerFor returns the segmentation overlay on edition k (the zero
// key for the edition the block was read in) and layer ("" = primary sentence
// segmentation), or nil.
func (b *Block) SegmentationLayerFor(k EditionKey, layer string) *Overlay {
	for i := range b.Overlays {
		o := &b.Overlays[i]
		if o.Type != OverlaySegmentation {
			continue
		}
		if o.Edition == k && o.Layer == layer {
			return o
		}
	}
	return nil
}

// SegmentationLayers lists the layer names of every segmentation overlay on
// edition k (the zero key for the edition the block was read in), in overlay
// order. The primary layer reports as "".
func (b *Block) SegmentationLayers(k EditionKey) []string {
	var layers []string
	for i := range b.Overlays {
		o := &b.Overlays[i]
		if o.Type == OverlaySegmentation && o.Edition == k {
			layers = append(layers, o.Layer)
		}
	}
	return layers
}

// SetSegmentation replaces the primary (layer "") segmentation overlay on
// edition k (the zero key for the edition the block was read in) with one
// carrying the supplied spans. Empty spans removes it.
func (b *Block) SetSegmentation(k EditionKey, spans []Span) {
	b.SetSegmentationLayer(k, LayerPrimary, spans)
}

// SetSegmentationLayer replaces the segmentation overlay on edition k (the
// zero key for the edition the block was read in) and layer ("" = primary
// sentence segmentation) with one carrying the supplied spans, leaving other
// layers untouched. Empty spans removes that layer.
func (b *Block) SetSegmentationLayer(k EditionKey, layer string, spans []Span) {
	b.Overlays = replaceSegmentation(b.Overlays, k, layer, spans)
}

// replaceSegmentation returns overlays with the segmentation overlay naming k
// and layer replaced by one carrying spans, or removed when spans is empty.
func replaceSegmentation(overlays []Overlay, k EditionKey, layer string, spans []Span) []Overlay {
	out := overlays[:0]
	for _, o := range overlays {
		if o.Type == OverlaySegmentation && o.Edition == k && o.Layer == layer {
			continue
		}
		out = append(out, o)
	}
	if len(spans) > 0 {
		out = append(out, Overlay{Type: OverlaySegmentation, Edition: k, Layer: layer, Spans: spans})
	}
	return out
}

// HasSourceOverlays reports whether the block carries any source-side overlay
// (segmentation, terms, entities, …). Overlays are positional by construction,
// so source mutation after one is attached would invalidate its run-anchored
// ranges. Block-scoped Annotations carry no range and never count here.
func (b *Block) HasSourceOverlays() bool {
	for i := range b.Overlays {
		if b.Overlays[i].OnSource() {
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
		if o.Type == OverlaySegmentation && o.OnSource() && o.Layer == LayerPrimary {
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
			return b.sourceRuns()
		}
		return nil
	}
	if idx < 0 || idx >= len(seg.Spans) {
		return nil
	}
	return seg.Spans[idx].Range.ExtractRuns(b.sourceRuns())
}

// SourceSegmentCount returns the number of source segment spans — len(spans)
// when a segmentation overlay is present, otherwise 1 for a non-empty block
// (0 when empty).
func (b *Block) SourceSegmentCount() int {
	if seg := b.SourceSegmentation(); seg != nil {
		return len(seg.Spans)
	}
	if len(b.sourceRuns()) > 0 {
		return 1
	}
	return 0
}
