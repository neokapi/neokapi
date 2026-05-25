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
