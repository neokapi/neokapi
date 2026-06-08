package tools

import (
	"fmt"

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
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		sf := &SegCountAnnotation{Source: v.SourceSegmentCount()}

		conf := t.Cfg.(*SegCountConfig)
		if !conf.Locale.IsEmpty() && v.HasTarget(conf.Locale) {
			key := model.Variant(conf.Locale)
			count := 1
			if seg := v.SegmentationFor(&key); seg != nil {
				count = len(seg.Spans)
			}
			sf.Target = count
		}

		v.Annotate(string(model.AnnoSegCount), sf)

		return nil
	}
	return t
}
