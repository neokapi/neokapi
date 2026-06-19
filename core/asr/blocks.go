package asr

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// BlocksFromASR converts recognized speech segments into positioned content
// Blocks: one Block per segment, carrying a TimingAnnotation (the temporal
// anchor facet, AD-002) and a source Origin{Kind: asr, Confidence} — the audio
// counterpart of vision.BlocksFromOCR. IDs are allocated from counter (advanced
// in place) so they stay unique across inputs. Empty segments are skipped. The
// blocks feed the normal block path (TM, AI translate, …) and round-trip into a
// timed-text writer (WebVTT/SubRip/TTML) via the timing anchor.
func BlocksFromASR(res *Result, counter *int) []*model.Block {
	if res == nil {
		return nil
	}
	var out []*model.Block
	for _, seg := range res.Segments {
		if seg.Text == "" {
			continue
		}
		*counter++
		b := model.NewBlock(fmt.Sprintf("tu%d", *counter), seg.Text)
		b.SetTiming(&model.TimingAnnotation{StartMS: seg.StartMS, EndMS: seg.EndMS})
		b.SetSourceOrigin(&model.Origin{Kind: model.OriginASR, Confidence: seg.Confidence})
		out = append(out, b)
	}
	return out
}

// ResultFromBlocks rebuilds a Result from timing-anchored blocks — the inverse of
// BlocksFromASR, reading each block's source text, timing anchor, and (when
// present) source-Origin confidence. Blocks lacking text or a timing anchor are
// skipped.
func ResultFromBlocks(blocks []*model.Block) *Result {
	res := &Result{}
	for _, b := range blocks {
		if b == nil {
			continue
		}
		text := b.SourceText()
		if text == "" {
			continue
		}
		t, ok := b.Timing()
		if !ok || t == nil {
			continue
		}
		conf := 1.0
		if o, ok := b.SourceOrigin(); ok && o.Confidence > 0 {
			conf = o.Confidence
		}
		res.Segments = append(res.Segments, Segment{
			Text:       text,
			StartMS:    t.StartMS,
			EndMS:      t.EndMS,
			Confidence: conf,
		})
	}
	return res
}
