package tools

import (
	"errors"

	"github.com/neokapi/neokapi/core/tool"
)

// PropertiesSetConfig holds configuration for the properties set tool.
type PropertiesSetConfig struct {
	Properties       map[string]string `schema:"title=Properties,description=Key-value pairs to set on each block"`                                 // Key-value pairs to set on each block
	Overwrite        bool              `schema:"title=Overwrite Existing,description=Overwrite existing properties with the same key,default=true"` // Overwrite existing properties (default: true)
	OnlyTranslatable bool              `schema:"title=Only Translatable,description=Only set properties on translatable blocks,default=true"`       // Only set on translatable blocks (default: true)
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
		return errors.New("properties-set: properties must not be empty")
	}
	return nil
}

// NewPropertiesSetConfig creates a PropertiesSetConfig at its documented
// defaults.
func NewPropertiesSetConfig() *PropertiesSetConfig {
	cfg := &PropertiesSetConfig{}
	cfg.Reset()
	return cfg
}

// NewPropertiesSetFromConfig creates a properties-set tool from a config map.
//
// The Properties map is the tool's entire behaviour and can only come from the
// step, so without this factory the tool set nothing at all — its Validate()
// rejects an empty map, but a flow never reached Validate: the zero-arg factory
// simply produced a tool with no properties to set (#1476). Monolingual: there
// is no locale to pin.
func NewPropertiesSetFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	cfg := NewPropertiesSetConfig()
	if err := applyStepConfig("properties-set", config, cfg, "", nil); err != nil {
		return nil, err
	}
	return NewPropertiesSetTool(cfg), nil
}

// NewPropertiesSetTool creates a new tool that sets or modifies properties
// on blocks programmatically.
func NewPropertiesSetTool(cfg *PropertiesSetConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "properties-set",
		ToolDescription: "Sets or modifies properties on blocks programmatically",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if cfg.OnlyTranslatable && !v.Translatable() {
			return nil
		}

		props := v.Properties()
		for key, value := range cfg.Properties {
			if !cfg.Overwrite {
				if _, exists := props[key]; exists {
					continue
				}
			}
			v.SetProperty(key, value)
		}

		return nil
	}
	return t
}
