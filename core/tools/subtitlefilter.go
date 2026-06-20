package tools

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// SubtitleFilterConfig configures the subtitle-filter tool. It takes no parameters.
type SubtitleFilterConfig struct{}

// ToolName returns the tool this config applies to.
func (c *SubtitleFilterConfig) ToolName() string { return "subtitle-filter" }

// Reset restores defaults (none).
func (c *SubtitleFilterConfig) Reset() {}

// Validate always succeeds — there is nothing to configure.
func (c *SubtitleFilterConfig) Validate() error { return nil }

// NewSubtitleFilterTool creates a tool that keeps only subtitle/caption cues —
// Block parts with a temporal anchor (model.TimingAnnotation) and no spatial
// anchor (model.GeometryAnnotation) — and drops every other Block. This lets a
// video flow emit a clean subtitle track from the speech layer while discarding
// on-screen text recognized from sampled frames (which is geometry-anchored).
// All non-Block parts (layers, groups, data, media) pass through unchanged.
func NewSubtitleFilterTool(cfg *SubtitleFilterConfig) *SubtitleFilter {
	if cfg == nil {
		cfg = &SubtitleFilterConfig{}
	}
	return &SubtitleFilter{
		BaseTool: tool.BaseTool{
			ToolName:        "subtitle-filter",
			ToolDescription: "Keeps only timing-anchored subtitle cues, dropping on-screen (geometry-anchored) text",
			Cfg:             cfg,
		},
	}
}

// SubtitleFilter drops Block parts that are not subtitle cues.
type SubtitleFilter struct {
	tool.BaseTool
}

// Process forwards every part except Block parts that are not subtitle cues.
func (s *SubtitleFilter) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for part := range in {
		if part.Type == model.PartBlock {
			if b, ok := part.Resource.(*model.Block); ok && !isSubtitleCue(b) {
				continue
			}
		}
		select {
		case out <- part:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// isSubtitleCue reports whether a block is a timed caption: it carries a temporal
// anchor and no spatial (on-screen) anchor.
func isSubtitleCue(b *model.Block) bool {
	if _, hasGeometry := b.Geometry(); hasGeometry {
		return false
	}
	_, hasTiming := b.Timing()
	return hasTiming
}
