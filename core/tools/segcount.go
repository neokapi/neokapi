package tools

import (
	"fmt"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Segment count property keys.
const (
	PropSegCountSource = "segment-count-source"
	PropSegCountTarget = "segment-count-target"
)

// SegCountConfig holds configuration for the segment count tool.
type SegCountConfig struct {
	Locale model.LocaleID `json:"locale,omitempty" schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *SegCountConfig) ToolName() string { return "segment-count" }

// Reset restores default values.
func (c *SegCountConfig) Reset() {
	c.Locale = ""
}

// Validate checks configuration validity.
func (c *SegCountConfig) Validate() error { return nil }

// SegCountSchema returns the auto-generated schema for the segment-count tool.
func SegCountSchema() *schema.ComponentSchema {
	return schema.FromStruct(&SegCountConfig{}, schema.ToolMeta{
		ID:          "segment-count",
		Category:    schema.CategoryAnalysis,
		DisplayName: "Segment Count",
		Description: "Count translatable segments",
		Inputs:      []string{schema.PartTypeBlock},
	})
}

// NewSegCountFromConfig creates a segment-count tool from a config map.
func NewSegCountFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg SegCountConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("segment-count config: %w", err)
	}
	if targetLang != "" {
		cfg.Locale = model.LocaleID(targetLang)
	}
	return NewSegCountTool(&cfg), nil
}

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

		block.Properties[PropSegCountSource] = strconv.Itoa(block.SourceSegmentCount())

		conf := t.Cfg.(*SegCountConfig)
		if !conf.Locale.IsEmpty() && block.HasTarget(conf.Locale) {
			key := model.Variant(conf.Locale)
			count := 1
			if seg := block.SegmentationFor(&key); seg != nil {
				count = len(seg.Spans)
			}
			block.Properties[PropSegCountTarget] = strconv.Itoa(count)
		}

		return part, nil
	}
	return t
}
