package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// PropertiesSetConfig holds configuration for the properties set tool.
type PropertiesSetConfig struct {
	Properties       map[string]string // Key-value pairs to set on each block
	Overwrite        bool              // Overwrite existing properties (default: true)
	OnlyTranslatable bool             // Only set on translatable blocks (default: true)
}

// ToolName returns the tool name this config applies to.
func (c *PropertiesSetConfig) ToolName() string { return "properties-set" }

// Reset restores default values.
func (c *PropertiesSetConfig) Reset() {
	c.Properties = nil
	c.Overwrite = true
	c.OnlyTranslatable = true
}

// Validate checks configuration validity.
func (c *PropertiesSetConfig) Validate() error {
	if len(c.Properties) == 0 {
		return fmt.Errorf("PropertiesSetConfig: Properties must not be empty")
	}
	return nil
}

// NewPropertiesSetTool creates a new tool that sets or modifies properties
// on blocks programmatically.
func NewPropertiesSetTool(cfg *PropertiesSetConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "properties-set",
		ToolDescription: "Sets or modifies properties on blocks programmatically",
		Cfg:             cfg,
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if cfg.OnlyTranslatable && !block.Translatable {
			return part, nil
		}

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		for key, value := range cfg.Properties {
			if !cfg.Overwrite {
				if _, exists := block.Properties[key]; exists {
					continue
				}
			}
			block.Properties[key] = value
		}

		return part, nil
	}
	return t
}
