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
	ID   string
	Runs []model.Run
	// Marks are the annotation markers this segment carried, in run
	// coordinates local to Runs.
	Marks []markSpan
	// UnplacedMarks are term spans this segmentation cannot carry — one
	// straddling two segments. The writer reports them rather than drawing
	// half of a pair.
	UnplacedMarks []model.Span
	Ignorable     bool
	Content       *Content // full inline IR for this segment's body
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
// targetStatusFromXLIFF2State maps an XLIFF 2.0 segment `state` to a target
// lifecycle status. The XLIFF 2 vocabulary is initial → translated → reviewed →
// final; "initial" / unknown leave the status unset (untranslated / new on our
// ladder, where presence then implies translated).
func targetStatusFromXLIFF2State(state string) model.TargetStatus {
	switch state {
	case "final":
		return model.TargetStatusSignedOff
	case "reviewed":
		return model.TargetStatusReviewed
	case "translated":
		return model.TargetStatusTranslated
	default:
		return ""
	}
}

// xliff2StateFromTargetStatus maps a target lifecycle status to an XLIFF 2.0
// segment `state` (the inverse of targetStatusFromXLIFF2State). A draft is a
// translation awaiting review, so it surfaces as "translated" — a vendor reading
// the file sees there is something to review. An unset/new status emits no state
// (XLIFF defaults to "initial"). Used only on the scratch-build write path so a
// produced XLIFF (e.g. `kapi extract`) reports where each unit stands.
func xliff2StateFromTargetStatus(s model.TargetStatus) string {
	switch s {
	case model.TargetStatusSignedOff:
		return "final"
	case model.TargetStatusReviewed:
		return "reviewed"
	case model.TargetStatusTranslated, model.TargetStatusDraft:
		return "translated"
	default:
		return ""
	}
}

func applySegmentsToBlock(block *model.Block, srcSegs []seg, tgtSegs []seg, trgLang model.LocaleID) {
	ann := &UnitSegmentsAnnotation{
		Source: map[string]*Content{},
		Target: map[model.LocaleID]map[string]*Content{},
	}

	block.Source = layOutSegments(srcSegs, ann.Source)
	block.SetSegmentation(nil, buildSegmentSpans(srcSegs))
	applyMarkOverlays(block, nil, srcSegs)

	if len(tgtSegs) > 0 && !trgLang.IsEmpty() {
		irByID := map[string]*Content{}
		runs := layOutSegments(tgtSegs, irByID)
		ann.Target[trgLang] = irByID
		block.SetTargetRuns(trgLang, runs)
		key := model.Variant(trgLang)
		block.SetSegmentation(&key, buildSegmentSpans(tgtSegs))
		applyMarkOverlays(block, &key, tgtSegs)
		// Map the XLIFF 2 segment state (captured as a block property by both
		// reader paths) onto the target's lifecycle status, so coverage and ship
		// gates see review progress that came in over the interchange. This is
		// read-only enrichment: the raw `state` attribute still round-trips via
		// the property, so the writer is unaffected and byte-exact output holds.
		if st := targetStatusFromXLIFF2State(block.Properties["state"]); st != "" {
			if t := block.Target(trgLang); t != nil {
				t.Status = st
			}
		}
	}

	if len(ann.Source) > 0 || len(ann.Target) > 0 {
		block.SetAnno(unitSegmentsAnnotationKey, ann)
	}
}

// OverlayMrk carries the annotation markers XLIFF 2 spells with a type this
// framework has no overlay of its own for: a comment, a custom annotation from
// whatever tool wrote the file. The marker's declared type rides in the span's
// `type` prop.
//
// OverlayType is deliberately an open string for exactly this — "formats and
// plugins may use any string for their own run-anchored state" — so a marker
// nobody here understands is still carried rather than discarded.
const OverlayMrk model.OverlayType = "xliff2:mrk"

// applyMarkOverlays turns the annotation markers the segments carried into the
// stand-off overlays the model records them as.
//
// A marker is not a run and never becomes one: constructs.yaml maps
// `meta.terminology` to `overlay:term` and says so in as many words. An
// <mrk type="term"> therefore arrives as a term overlay span, and every other
// marker type as an OverlayMrk span keeping the type it declared. Dropping the
// ones we do not recognize would silently discard a decision another tool
// recorded in the file.
//
// Positions are rebased from segment-local runs onto the block's, the same
// cursor walk buildSegmentSpans does over the same segments.
func applyMarkOverlays(block *model.Block, variant *model.VariantKey, segs []seg) {
	var term, other []model.Span
	cursor := 0
	for _, sg := range segs {
		for _, m := range sg.Marks {
			if m.End < m.Start {
				continue
			}
			span := markToSpan(m, cursor)
			if m.Attrs.Type == "term" {
				term = append(term, span)
				continue
			}
			if span.Props == nil {
				span.Props = map[string]string{}
			}
			span.Props["type"] = m.Attrs.Type
			other = append(other, span)
		}
		cursor += len(sg.Runs)
	}
	if len(term) > 0 {
		block.Overlays = append(block.Overlays,
			model.Overlay{Type: model.OverlayTerm, Variant: variant, Spans: term})
	}
	if len(other) > 0 {
		block.Overlays = append(block.Overlays,
			model.Overlay{Type: OverlayMrk, Variant: variant, Spans: other})
	}
}

// markToSpan positions one marker on the block's runs, carrying the marker
// attributes that mean something to a consumer. `ref` is where a term mark
// points back at the concept it denotes.
func markToSpan(m markSpan, offset int) model.Span {
	span := model.Span{
		ID: m.Attrs.ID,
		Range: model.SpanAnchor(
			model.RunPos{Run: m.Start + offset},
			model.RunPos{Run: m.End + offset},
		),
	}
	props := map[string]string{}
	if m.Attrs.Ref != "" {
		props["ref"] = m.Attrs.Ref
	}
	if m.Attrs.Value != "" {
		props["value"] = m.Attrs.Value
	}
	if m.Attrs.Translate != "" {
		props["translate"] = m.Attrs.Translate
	}
	if len(props) > 0 {
		span.Props = props
	}
	return span
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
			Range: model.SpanAnchor(model.RunPos{Run: start}, model.RunPos{Run: end}),
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
	segs := segsFromOverlay(block.Source, overlay, srcIR)
	return withTermMarks(segs, block.OverlayOf(model.OverlayTerm))
}

// withTermMarks hands each segment the term spans that fall inside it, so the
// writer can draw them without needing the block in scope.
func withTermMarks(segs []seg, overlay *model.Overlay) []seg {
	if overlay == nil || len(overlay.Spans) == 0 {
		return segs
	}
	placed, unplaced := marksForSegments(segs, overlay.Spans)
	for i := range segs {
		segs[i].Marks = placed[i]
	}
	if len(unplaced) > 0 && len(segs) > 0 {
		segs[0].UnplacedMarks = unplaced
	}
	return segs
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
