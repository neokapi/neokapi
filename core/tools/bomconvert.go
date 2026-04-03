package tools

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// BOMConvertConfig holds configuration for the BOM conversion tool.
type BOMConvertConfig struct {
	AddBOM      bool `schema:"title=Add BOM,description=When true the Unicode BOM is added; when false it is removed"`     // true = ensure BOM is present, false = remove BOM
	AlsoNonUTF8 bool `schema:"title=Remove UTF-16 BOMs Also,description=Also remove the BOM from UTF-16 files not just UTF-8"` // Also handle UTF-16 BOMs
}

// ToolName returns the tool name this config applies to.
func (c *BOMConvertConfig) ToolName() string { return "bom-convert" }

// Reset restores default values.
func (c *BOMConvertConfig) Reset() {
	c.AddBOM = false
	c.AlsoNonUTF8 = false
}

// Validate checks configuration validity.
func (c *BOMConvertConfig) Validate() error {
	return nil
}

// NewBOMConvertTool creates a tool that manages the Unicode BOM on Layer properties.
func NewBOMConvertTool(cfg *BOMConvertConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "bom-convert",
		ToolDescription: "Adds or removes the Unicode BOM marker on document layers",
		Cfg:             cfg,
	}
	t.HandleLayerStartFn = func(part *model.Part) (*model.Part, error) {
		layer, ok := part.Resource.(*model.Layer)
		if !ok {
			return part, nil
		}

		conf := t.Cfg.(*BOMConvertConfig)
		layer.HasBOM = conf.AddBOM

		return part, nil
	}
	return t
}
