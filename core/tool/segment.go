package tool

import (
	"iter"

	"github.com/neokapi/neokapi/core/model"
)

// Segment is one processing granularity within a Block: the whole block when
// no segmentation overlay is present, or one segment span of a named
// segmentation layer when it is. It hides whether segmentation is materialised
// as structure or as a stand-off overlay (AD-002), giving per-segment tools a
// uniform, position-correct view instead of re-implementing the "segmented or
// not" branch over Block.SourceSegmentation each time.
//
// Obtain segments from BlockView.SourceSegments (read-only) or
// VariantView.TargetSegments (writable per-segment target production).
type Segment interface {
	// Index is the segment's position in iteration order: 0 for the single
	// whole-block segment, the span index for a segmented layer.
	Index() int

	// Range is the source run range the segment covers, or nil for the
	// whole-block segment. The returned pointer is a copy; mutating it does not
	// affect the block.
	Range() *model.Anchor

	// Ignorable reports whether the segment is a non-translatable structural
	// span (a segmentation span marked model.SpanPropIgnorable). Always false
	// for the whole-block segment.
	Ignorable() bool

	// SourceRuns returns the segment's source runs: the whole source for the
	// whole-block segment, or the span's extracted runs for a segment span.
	SourceRuns() []model.Run

	// TargetRuns returns the segment's target runs for loc, mapped through the
	// matching target segment span when a target-side segmentation of the same
	// layer is present, otherwise the whole target.
	TargetRuns(loc model.LocaleID) []model.Run
}

// WritableSegment adds per-segment target production. Writes are buffered and
// spliced back into the block target in span order when iteration completes;
// see VariantView.TargetSegments for the all-or-nothing commit semantics.
type WritableSegment interface {
	Segment

	// SetTargetRuns records this segment's translated runs for loc. The runs
	// are committed to the block only when iteration finishes and every
	// non-ignorable segment has been written.
	SetTargetRuns(loc model.LocaleID, runs []model.Run)
}

// segment is the single concrete Segment / WritableSegment. The whole-block
// segment has a nil rng; a segment span carries the span's range, layer and
// ignorable flag.
type segment struct {
	b         *model.Block
	idx       int
	rng       *model.Anchor // nil = whole block
	layer     string
	src       []model.Run
	ignorable bool

	// Write buffer (WritableSegment). written records that SetTargetRuns was called
	// so the assembler can distinguish "translated to empty" from "untouched".
	written bool
	outRuns []model.Run
}

func (u *segment) Index() int { return u.idx }

func (u *segment) Range() *model.Anchor {
	if u.rng == nil {
		return nil
	}
	r := *u.rng
	return &r
}

func (u *segment) Ignorable() bool         { return u.ignorable }
func (u *segment) SourceRuns() []model.Run { return u.src }

func (u *segment) TargetRuns(loc model.LocaleID) []model.Run {
	if u.rng == nil {
		return u.b.TargetRuns(loc)
	}
	if tseg := u.b.TargetSegmentationLayer(loc, u.layer); tseg != nil && u.idx < len(tseg.Spans) {
		return tseg.Spans[u.idx].Range.ExtractRuns(u.b.TargetRuns(loc))
	}
	return u.b.TargetRuns(loc)
}

func (u *segment) SetTargetRuns(_ model.LocaleID, runs []model.Run) {
	u.written = true
	u.outRuns = runs
}

// sourceSegments yields the source segments of the given layer ("" = primary): one
// per segmentation span, or a single whole-block segment when the layer carries no
// segmentation overlay. An empty source yields nothing (matching
// Block.SourceSegmentCount).
func sourceSegments(b *model.Block, layer string) iter.Seq[Segment] {
	return func(yield func(Segment) bool) {
		src := authoritative(b).Runs
		seg := b.SegmentationLayerFor(model.EditionKey{}, layer)
		if seg == nil || len(seg.Spans) == 0 {
			if len(src) > 0 {
				yield(&segment{b: b, idx: 0, layer: layer, src: src})
			}
			return
		}
		for i := range seg.Spans {
			span := seg.Spans[i]
			rng := span.Range
			u := &segment{
				b:         b,
				idx:       i,
				rng:       &rng,
				layer:     layer,
				src:       rng.ExtractRuns(src),
				ignorable: span.Ignorable(),
			}
			if !yield(u) {
				return
			}
		}
	}
}

// targetSegments yields writable segments over the source segmentation of the given
// layer and, when iteration completes normally, splices the written runs back
// into the block target for loc in span order. Commit is all-or-nothing: it
// happens only if every non-ignorable segment was written (ignorable segments
// contribute their source runs verbatim). If the loop is stopped early, or any
// non-ignorable segment was left unwritten, nothing is committed — so a tool that
// can only translate some segments leaves the target untouched for a later
// stage, exactly as per-segment content-memory leverage requires.
func targetSegments(v *blockView, loc model.LocaleID, layer string) iter.Seq[WritableSegment] {
	b := v.b
	return func(yield func(WritableSegment) bool) {
		src := authoritative(b).Runs
		seg := b.SegmentationLayerFor(model.EditionKey{}, layer)
		if seg == nil || len(seg.Spans) == 0 {
			if len(src) == 0 {
				return
			}
			u := &segment{b: b, idx: 0, layer: layer, src: src}
			if !yield(u) {
				return
			}
			if u.written {
				v.SetTargetRuns(loc, u.outRuns)
			}
			return
		}
		segments := make([]*segment, len(seg.Spans))
		for i := range seg.Spans {
			span := seg.Spans[i]
			rng := span.Range
			u := &segment{
				b:         b,
				idx:       i,
				rng:       &rng,
				layer:     layer,
				src:       rng.ExtractRuns(src),
				ignorable: span.Ignorable(),
			}
			segments[i] = u
			if !yield(u) {
				return // stopped early: commit nothing
			}
		}
		var assembled []model.Run
		for _, u := range segments {
			switch {
			case u.written:
				assembled = append(assembled, u.outRuns...)
			case u.ignorable:
				assembled = append(assembled, u.src...)
			default:
				return // a non-ignorable segment was not written: commit nothing
			}
		}
		v.SetTargetRuns(loc, assembled)
	}
}

var (
	_ Segment         = (*segment)(nil)
	_ WritableSegment = (*segment)(nil)
)
