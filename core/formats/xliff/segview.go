package xliff

import (
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// segView is the writer's lightweight view of one xliff segment:
// the segment's span id, its runs, and the xliff-native IR the reader
// captured for it. The Run content model has no structural segment
// type, so the writer reconstitutes these views from a Block's runs +
// segmentation overlay + per-span native annotations. This keeps the
// byte-faithful emission logic (renderBodyWithSegments, wrapSegments-
// AsMrk, concatSegments, …) working over a stable per-segment shape.
type segView struct {
	ID     string
	Runs   []model.Run
	Native *NativeContent
}

// Text returns the plain text of the segment's runs (TextRun content
// only), matching the old model.Segment.Text() behaviour.
func (s segView) Text() string { return model.RunsText(s.Runs) }

// segNativeKey is the block-annotation key under which the reader
// stores a source segment's xliff-native IR. spanID is the segment's
// span id ("s1" when the block is unsegmented, otherwise the mrk mid).
func segNativeKey(spanID string) string { return "xliff:native:" + spanID }

// targetSegNativeKey is the block-annotation key for a target segment's
// xliff-native IR for a given locale.
func targetSegNativeKey(loc model.LocaleID, spanID string) string {
	return "xliff:native@" + string(loc) + ":" + spanID
}

// nativeFromBlock returns the *NativeContent stored under key on the
// block, or nil if absent / wrong type.
func nativeFromBlock(block *model.Block, key string) *NativeContent {
	if block == nil || block.Annotations == nil {
		return nil
	}
	a, ok := block.Annotations[key]
	if !ok {
		return nil
	}
	if na, ok := a.(*SegmentNativeAnnotation); ok && na != nil {
		return na.Content
	}
	return nil
}

// sourceSegViews reconstructs the source segment views of a block. When
// the block carries a source segmentation overlay each span becomes one
// view (id = span id, runs = the span's runs); otherwise the whole
// source is one view with id "s1". Per-span native IR is looked up via
// segNativeKey.
func sourceSegViews(block *model.Block) []segView {
	if block == nil {
		return nil
	}
	seg := block.SourceSegmentation()
	if seg == nil {
		if len(block.Source) == 0 {
			return nil
		}
		return []segView{{
			ID:     "s1",
			Runs:   block.Source,
			Native: nativeFromBlock(block, segNativeKey("s1")),
		}}
	}
	out := make([]segView, len(seg.Spans))
	for i, span := range seg.Spans {
		id := span.ID
		out[i] = segView{
			ID:     id,
			Runs:   span.Range.ExtractRuns(block.Source),
			Native: nativeFromBlock(block, segNativeKey(id)),
		}
	}
	return out
}

// targetSegViews reconstructs the target segment views of a block for a
// locale. Mirrors sourceSegViews but reads the target runs and a
// target-side segmentation overlay; per-span native IR is looked up via
// targetSegNativeKey.
func targetSegViews(block *model.Block, loc model.LocaleID) []segView {
	if block == nil {
		return nil
	}
	runs := block.TargetRuns(loc)
	if runs == nil {
		return nil
	}
	key := model.Variant(loc)
	seg := block.SegmentationFor(&key)
	if seg == nil {
		return []segView{{
			ID:     "s1",
			Runs:   runs,
			Native: nativeFromBlock(block, targetSegNativeKey(loc, "s1")),
		}}
	}
	out := make([]segView, len(seg.Spans))
	for i, span := range seg.Spans {
		id := span.ID
		out[i] = segView{
			ID:     id,
			Runs:   span.Range.ExtractRuns(runs),
			Native: nativeFromBlock(block, targetSegNativeKey(loc, id)),
		}
	}
	return out
}

// anyTargetLocale returns a locale that has a committed target on the
// block, or empty if none. Used by the writer to fall back to any
// existing target's segments when the writer locale has none.
func anyTargetLocale(block *model.Block) model.LocaleID {
	for _, loc := range block.TargetLocales() {
		if len(block.TargetRuns(loc)) > 0 {
			return loc
		}
	}
	return ""
}

// blockIsSegmented reports whether the block carries seg-source-style
// segmentation: a source segmentation overlay with more than one span,
// or a single span whose id looks like a mrk mid (not the default
// "s1"). When true the writer mirrors the segmentation onto <target>.
func blockIsSegmented(block *model.Block) bool {
	seg := block.SourceSegmentation()
	if seg == nil {
		return false
	}
	if len(seg.Spans) > 1 {
		return true
	}
	if len(seg.Spans) == 1 {
		id := seg.Spans[0].ID
		if id != "" && id != "s1" {
			return true
		}
	}
	return false
}

// midForSegView derives the mrk mid for the i-th segment view, matching
// the old wrapSegmentsAsMrk behaviour: use the segment's id unless it is
// empty or the default "s1", in which case fall back to the index.
func midForSegView(s segView, i int) string {
	mid := s.ID
	if mid == "" || mid == "s1" {
		mid = strconv.Itoa(i)
	}
	return mid
}
