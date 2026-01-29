package tools

import (
	"strconv"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
)

// Segment count property keys.
const (
	PropSegCountSource = "segment-count-source"
	PropSegCountTarget = "segment-count-target"
)

// SegCountConfig holds configuration for the segment count tool.
type SegCountConfig struct {
	Locale model.LocaleID // Target locale for counting target segments
}

// ToolName returns the tool name this config applies to.
func (c *SegCountConfig) ToolName() string { return "segment-count" }

// Reset restores default values.
func (c *SegCountConfig) Reset() {
	c.Locale = ""
}

// Validate checks configuration validity.
func (c *SegCountConfig) Validate() error { return nil }

// NewSegCountTool creates a tool that counts source and target segments
// in blocks and stores the counts in properties.
func NewSegCountTool(cfg *SegCountConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "segment-count",
		ToolDescription: "Counts source and target segments in blocks",
		Cfg:             cfg,
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if !block.Translatable {
			return part, nil
		}

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		block.Properties[PropSegCountSource] = strconv.Itoa(len(block.Source))

		conf := t.Cfg.(*SegCountConfig)
		if !conf.Locale.IsEmpty() && block.HasTarget(conf.Locale) {
			segs := block.Targets[conf.Locale]
			block.Properties[PropSegCountTarget] = strconv.Itoa(len(segs))
		}

		return part, nil
	}
	return t
}
