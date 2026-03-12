package tools

import (
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// RemoveTargetConfig holds configuration for the remove-target tool.
type RemoveTargetConfig struct {
	TargetLocale model.LocaleID // Target locale to remove (if empty, removes ALL targets)
}

// ToolName returns the tool name this config applies to.
func (c *RemoveTargetConfig) ToolName() string { return "remove-target" }

// Reset restores default values.
func (c *RemoveTargetConfig) Reset() {
	c.TargetLocale = ""
}

// Validate checks configuration validity.
func (c *RemoveTargetConfig) Validate() error {
	return nil
}

// NewRemoveTargetTool creates a new remove-target tool.
// It removes target segments from blocks for a specified locale,
// or all targets if no locale is specified.
func NewRemoveTargetTool(cfg *RemoveTargetConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "remove-target",
		ToolDescription: "Removes target segments from blocks",
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

		conf := t.Cfg.(*RemoveTargetConfig)

		if conf.TargetLocale.IsEmpty() {
			// Remove all targets.
			block.Targets = make(map[model.LocaleID][]*model.Segment)
		} else {
			delete(block.Targets, conf.TargetLocale)
		}

		return part, nil
	}
	return t
}
