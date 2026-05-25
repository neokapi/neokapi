package tools

import (
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// CreateTargetConfig holds configuration for the create-target tool.
type CreateTargetConfig struct {
	TargetLocale            model.LocaleID `schema:"title=Target Locale,description=Target locale to create"`                                                                  // Target locale to create (required)
	CopySource              bool           `schema:"title=Copy Source,description=Copy source text to the new target segment"`                                                 // Whether to copy source text to target (default: false)
	Overwrite               bool           `schema:"title=Overwrite Existing,description=Overwrite existing target segments if they already exist"`                            // Whether to overwrite existing targets (default: false)
	CreateOnNonTranslatable bool           `schema:"title=Create on Non-Translatable,description=Create a target container even for non-translatable text units,default=true"` // Create targets on non-translatable blocks
}

// ToolName returns the tool name this config applies to.
func (c *CreateTargetConfig) ToolName() string { return "create-target" }

// Reset restores default values.
func (c *CreateTargetConfig) Reset() {
	c.TargetLocale = ""
	c.CopySource = false
	c.Overwrite = false
	c.CreateOnNonTranslatable = true
}

// Validate checks configuration validity.
func (c *CreateTargetConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("create-target: TargetLocale is required")
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
	// Translate: create-target writes a target container (optionally seeded
	// with the source text); it never touches source.
	t.Translate = func(v tool.TargetView) error {
		conf := t.Cfg.(*CreateTargetConfig)

		if !v.Translatable() && !conf.CreateOnNonTranslatable {
			return nil
		}

		// Skip if target already exists and we're not overwriting.
		if v.HasTarget(conf.TargetLocale) && !conf.Overwrite {
			return nil
		}

		if conf.CopySource {
			v.SetTargetText(conf.TargetLocale, v.SourceText())
		} else {
			v.SetTargetText(conf.TargetLocale, "")
		}

		return nil
	}
	return t
}
