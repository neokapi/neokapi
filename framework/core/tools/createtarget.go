package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// CreateTargetConfig holds configuration for the create-target tool.
type CreateTargetConfig struct {
	TargetLocale model.LocaleID `schema:"description=Target locale to create"` // Target locale to create (required)
	CopySource   bool           `schema:"description=Copy source text to the new target segment"` // Whether to copy source text to target (default: false)
	Overwrite    bool           `schema:"description=Overwrite existing target segments if they already exist"` // Whether to overwrite existing targets (default: false)
}

// ToolName returns the tool name this config applies to.
func (c *CreateTargetConfig) ToolName() string { return "create-target" }

// Reset restores default values.
func (c *CreateTargetConfig) Reset() {
	c.TargetLocale = ""
	c.CopySource = false
	c.Overwrite = false
}

// Validate checks configuration validity.
func (c *CreateTargetConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("create-target: TargetLocale is required")
	}
	return nil
}

// NewCreateTargetTool creates a new create-target tool.
// It creates target segment containers for blocks, optionally copying
// source text to the target.
func NewCreateTargetTool(cfg *CreateTargetConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "create-target",
		ToolDescription: "Creates target segment containers for blocks",
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

		conf := t.Cfg.(*CreateTargetConfig)

		// Skip if target already exists and we're not overwriting.
		if block.HasTarget(conf.TargetLocale) && !conf.Overwrite {
			return part, nil
		}

		if conf.CopySource {
			block.SetTargetText(conf.TargetLocale, block.SourceText())
		} else {
			block.SetTargetText(conf.TargetLocale, "")
		}

		return part, nil
	}
	return t
}
