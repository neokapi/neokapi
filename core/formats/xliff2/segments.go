package xliff2

import (
	"github.com/neokapi/neokapi/core/model"
)

// This file bridges the framework's stand-off segmentation model (AD-002)
// to the xliff2 reader/writer, which is genuinely multi-segment: an XLIFF 2
// <unit> contains an ordered sequence of <segment> and <ignorable> elements.
//
// The framework's Block now carries a single flat []Run per side (source /
// each target) plus segmentation overlays; there is no structural Segment
// type. The xliff2 code internally still reasons in terms of per-segment
// records — id, runs, per-segment attributes, and a per-segment inline IR —
// so we keep a package-private `seg` value type and convert to/from the
// stand-off overlay representation at the Block boundary.
//
// Mapping:
//   - block.Source holds the concatenation of every <segment>/<ignorable>
//     source run sequence, in document order.
//   - The source segmentation overlay (block.SetSegmentation(nil, spans))
//     has one Span per <segment> AND per <ignorable>, anchored by run-index
//     boundaries. Per-element attributes (id via Span.ID; state; the
//     <ignorable> marker) live in Span.Props.
//   - The per-segment inline IR (SegmentInlineAnnotation) is stored on the
//     Block via a UnitSegmentsAnnotation, keyed by span id, because the IR
//     is not representable as flat Run text and must survive round-trip.
//   - Targets mirror the same structure per locale (variant key).

// An <ignorable> element is marked on its segmentation span via the
// framework's format-agnostic [model.SpanPropIgnorable] property (shared with
// the okapi bridge); a translatable <segment> carries no such marker.

// seg is the package-private per-segment record the reader builds and the
// writer consumes — one <segment>/<ignorable> element's id, runs, and inline
// IR, used only within the xliff2 package to bridge the format's multi-segment
// units to the Block's flat runs + segmentation overlay.
type seg struct {
	ID        string
	Runs      []model.Run
	Ignorable bool
	Content   *Content // full inline IR for this segment's body
}

// UnitSegmentsAnnotation carries the per-segment inline IR for a unit's
// source and target sides, keyed by segment span id. It rides on
// Block.Annotations because the IR cannot be reconstructed from the flat
// runs + overlay alone (inline-code attribute fidelity would be lost).
type UnitSegmentsAnnotation struct {
	// Source maps source-segment span id → inline IR.
	Source map[string]*Content
	// Target maps locale → (target-segment span id → inline IR).
	Target map[model.LocaleID]map[string]*Content
}

// AnnotationType identifies the annotation key.
func (a *UnitSegmentsAnnotation) TypeName() string { return "xliff2:unit-segments" }

const unitSegmentsAnnotationKey = "xliff2:unit-segments"

func init() {
	model.RegisterPayload(unitSegmentsAnnotationKey, func() model.Payload {
		return &UnitSegmentsAnnotation{
			Source: map[string]*Content{},
			Target: map[model.LocaleID]map[string]*Content{},
		}
	})
}

// applySegmentsToBlock writes the source and target seg lists onto the block
// as flat runs + segmentation overlays + a UnitSegmentsAnnotation carrying
// the inline IR. trgLang names the target locale (may be empty).
func applySegmentsToBlock(block *model.Block, srcSegs []seg, tgtSegs []seg, trgLang model.LocaleID) {
	ann := &UnitSegmentsAnnotation{
		Source: map[string]*Content{},
		Target: map[model.LocaleID]map[string]*Content{},
	}

	block.Source = layOutSegments(srcSegs, ann.Source)
	block.SetSegmentation(nil, buildSegmentSpans(srcSegs))

	if len(tgtSegs) > 0 && !trgLang.IsEmpty() {
		irByID := map[string]*Content{}
		runs := layOutSegments(tgtSegs, irByID)
		ann.Target[trgLang] = irByID
		block.SetTargetRuns(trgLang, runs)
		key := model.Variant(trgLang)
		block.SetSegmentation(&key, buildSegmentSpans(tgtSegs))
	}

	if len(ann.Source) > 0 || len(ann.Target) > 0 {
		block.SetAnno(unitSegmentsAnnotationKey, ann)
	}
}

// layOutSegments concatenates the runs of every seg into a single sequence,
// populating irByID with the per-segment inline IR keyed by segment id.
func layOutSegments(segs []seg, irByID map[string]*Content) []model.Run {
	var runs []model.Run
	for _, s := range segs {
		runs = append(runs, s.Runs...)
		if s.Content != nil && s.ID != "" {
			irByID[s.ID] = s.Content
		}
	}
	return runs
}

// buildSegmentSpans builds one segmentation Span per seg, anchored by
// run-index boundaries, carrying the segment's id in Span.ID and the
// <ignorable> marker in Span.Props.
func buildSegmentSpans(segs []seg) []model.Span {
	if len(segs) == 0 {
		return nil
	}
	spans := make([]model.Span, 0, len(segs))
	cursor := 0
	for _, s := range segs {
		start := cursor
		end := cursor + len(s.Runs)
		cursor = end
		sp := model.Span{
			ID:    s.ID,
			Range: model.RunRange{StartRun: start, EndRun: end},
		}
		if s.Ignorable {
			sp.Props = map[string]string{model.SpanPropIgnorable: "true"}
		}
		spans = append(spans, sp)
	}
	return spans
}

// sourceSegsFromBlock reconstructs the source seg list from the block's
// flat source runs, source segmentation overlay, and inline-IR annotation.
// When no segmentation overlay is present, the whole source is one segment.
func sourceSegsFromBlock(block *model.Block) []seg {
	overlay := block.SourceSegmentation()
	ir := unitSegmentsIR(block)
	var srcIR map[string]*Content
	if ir != nil {
		srcIR = ir.Source
	}
	return segsFromOverlay(block.Source, overlay, srcIR)
}

// targetSegsFromBlock reconstructs the target seg list for a locale.
func targetSegsFromBlock(block *model.Block, loc model.LocaleID) []seg {
	runs := block.TargetRuns(loc)
	if runs == nil {
		return nil
	}
	key := model.Variant(loc)
	overlay := block.SegmentationFor(&key)
	ir := unitSegmentsIR(block)
	var tgtIR map[string]*Content
	if ir != nil {
		tgtIR = ir.Target[loc]
	}
	return segsFromOverlay(runs, overlay, tgtIR)
}

// segsFromOverlay turns a flat run sequence plus an optional segmentation
// overlay into the per-segment seg list. With no overlay the whole sequence
// is one anonymous segment.
func segsFromOverlay(runs []model.Run, overlay *model.Overlay, irByID map[string]*Content) []seg {
	if overlay == nil || len(overlay.Spans) == 0 {
		if len(runs) == 0 {
			return nil
		}
		return []seg{{Runs: runs}}
	}
	out := make([]seg, 0, len(overlay.Spans))
	for _, sp := range overlay.Spans {
		s := seg{
			ID:        sp.ID,
			Runs:      sp.Range.ExtractRuns(runs),
			Ignorable: sp.Ignorable(),
		}
		if irByID != nil {
			s.Content = irByID[sp.ID]
		}
		out = append(out, s)
	}
	return out
}

// unitSegmentsIR returns the block's UnitSegmentsAnnotation, or nil.
func unitSegmentsIR(block *model.Block) *UnitSegmentsAnnotation {
	if block == nil {
		return nil
	}
	av, _ := block.Anno(unitSegmentsAnnotationKey)
	a, _ := av.(*UnitSegmentsAnnotation)
	return a
}
