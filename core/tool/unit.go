package tool

import (
	"iter"

	"github.com/neokapi/neokapi/core/model"
)

// Unit is one processing granularity within a Block: the whole block when no
// segmentation overlay is present, or one segment span of a named segmentation
// layer when it is. It hides whether segmentation is materialised as structure
// or as a stand-off overlay (AD-002), giving per-segment tools a uniform,
// position-correct view instead of re-implementing the "segmented or not"
// branch over Block.SourceSegmentation each time.
//
// Obtain units from BlockView.SourceUnits (read-only) or
// VariantView.TargetUnits (writable per-unit target production).
type Unit interface {
	// Index is the unit's position in iteration order — 0 for the single
	// whole-block unit, the span index for a segmented layer.
	Index() int

	// Range is the source run range the unit covers, or nil for the whole-block
	// unit. The returned pointer is a copy; mutating it does not affect the block.
	Range() *model.RunRange

	// Ignorable reports whether the unit is a non-translatable structural span
	// (a segmentation span marked model.SpanPropIgnorable). Always false for the
	// whole-block unit.
	Ignorable() bool

	// SourceRuns returns the unit's source runs: the whole source for the
	// whole-block unit, or the span's extracted runs for a segment.
	SourceRuns() []model.Run

	// TargetRuns returns the unit's target runs for loc — mapped through the
	// matching target segment span when a target-side segmentation of the same
	// layer is present, otherwise the whole target.
	TargetRuns(loc model.LocaleID) []model.Run
}

// WritableUnit adds per-unit target production. Writes are buffered and spliced
// back into the block target in span order when iteration completes; see
// VariantView.TargetUnits for the all-or-nothing commit semantics.
type WritableUnit interface {
	Unit

	// SetTargetRuns records this unit's translated runs for loc. The runs are
	// committed to the block only when iteration finishes and every
	// non-ignorable unit has been written.
	SetTargetRuns(loc model.LocaleID, runs []model.Run)
}

// unit is the single concrete Unit / WritableUnit. The whole-block unit has a
// nil rng; segment units carry the span's range, layer and ignorable flag.
type unit struct {
	b         *model.Block
	idx       int
	rng       *model.RunRange // nil = whole block
	layer     string
	src       []model.Run
	ignorable bool

	// Write buffer (WritableUnit). written records that SetTargetRuns was called
	// so the assembler can distinguish "translated to empty" from "untouched".
	written bool
	outRuns []model.Run
}

func (u *unit) Index() int { return u.idx }

func (u *unit) Range() *model.RunRange {
	if u.rng == nil {
		return nil
	}
	r := *u.rng
	return &r
}

func (u *unit) Ignorable() bool         { return u.ignorable }
func (u *unit) SourceRuns() []model.Run { return u.src }

func (u *unit) TargetRuns(loc model.LocaleID) []model.Run {
	if u.rng == nil {
		return u.b.TargetRuns(loc)
	}
	key := model.Variant(loc)
	if tseg := u.b.SegmentationLayerFor(&key, u.layer); tseg != nil && u.idx < len(tseg.Spans) {
		return tseg.Spans[u.idx].Range.ExtractRuns(u.b.TargetRuns(loc))
	}
	return u.b.TargetRuns(loc)
}

func (u *unit) SetTargetRuns(_ model.LocaleID, runs []model.Run) {
	u.written = true
	u.outRuns = runs
}

// sourceUnits yields the source units of the given layer ("" = primary): one
// per segmentation span, or a single whole-block unit when the layer carries no
// segmentation overlay. An empty source yields nothing (matching
// Block.SourceSegmentCount).
func sourceUnits(b *model.Block, layer string) iter.Seq[Unit] {
	return func(yield func(Unit) bool) {
		seg := b.SegmentationLayerFor(nil, layer)
		if seg == nil || len(seg.Spans) == 0 {
			if len(b.Source) > 0 {
				yield(&unit{b: b, idx: 0, layer: layer, src: b.Source})
			}
			return
		}
		for i := range seg.Spans {
			span := seg.Spans[i]
			rng := span.Range
			u := &unit{
				b:         b,
				idx:       i,
				rng:       &rng,
				layer:     layer,
				src:       rng.ExtractRuns(b.Source),
				ignorable: span.Ignorable(),
			}
			if !yield(u) {
				return
			}
		}
	}
}

// targetUnits yields writable units over the source segmentation of the given
// layer and, when iteration completes normally, splices the written runs back
// into the block target for loc in span order. Commit is all-or-nothing: it
// happens only if every non-ignorable unit was written (ignorable units
// contribute their source runs verbatim). If the loop is stopped early, or any
// non-ignorable unit was left unwritten, nothing is committed — so a tool that
// can only translate some segments leaves the target untouched for a later
// stage, exactly as per-segment TM leverage requires.
func targetUnits(b *model.Block, loc model.LocaleID, layer string) iter.Seq[WritableUnit] {
	return func(yield func(WritableUnit) bool) {
		seg := b.SegmentationLayerFor(nil, layer)
		if seg == nil || len(seg.Spans) == 0 {
			if len(b.Source) == 0 {
				return
			}
			u := &unit{b: b, idx: 0, layer: layer, src: b.Source}
			if !yield(u) {
				return
			}
			if u.written {
				b.SetTargetRuns(loc, u.outRuns)
			}
			return
		}
		units := make([]*unit, len(seg.Spans))
		for i := range seg.Spans {
			span := seg.Spans[i]
			rng := span.Range
			u := &unit{
				b:         b,
				idx:       i,
				rng:       &rng,
				layer:     layer,
				src:       rng.ExtractRuns(b.Source),
				ignorable: span.Ignorable(),
			}
			units[i] = u
			if !yield(u) {
				return // stopped early: commit nothing
			}
		}
		var assembled []model.Run
		for _, u := range units {
			switch {
			case u.written:
				assembled = append(assembled, u.outRuns...)
			case u.ignorable:
				assembled = append(assembled, u.src...)
			default:
				return // a non-ignorable unit was not written: commit nothing
			}
		}
		b.SetTargetRuns(loc, assembled)
	}
}

var (
	_ Unit         = (*unit)(nil)
	_ WritableUnit = (*unit)(nil)
)
