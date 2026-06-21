package tools

import (
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// CreateTargetConfig holds configuration for the create-target tool.
type CreateTargetConfig struct {
	TargetLocale            model.LocaleID `schema:"title=Target Locale,description=Target locale to create"`                                                        // Target locale to create (required)
	CopySource              bool           `schema:"title=Copy Source,description=Copy the source runs into the new target"`                                         // Whether to copy source text to target (default: false)
	Overwrite               bool           `schema:"title=Overwrite Existing,description=Overwrite the existing target if one already exists"`                       // Whether to overwrite existing targets (default: false)
	CreateOnNonTranslatable bool           `schema:"title=Create on Non-Translatable,description=Create a target even for non-translatable text units,default=true"` // Create targets on non-translatable blocks
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
// It creates a target for blocks, optionally copying the source runs.
func NewCreateTargetTool(cfg *CreateTargetConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "create-target",
		ToolDescription: "Creates a target for blocks, optionally copying the source",
		Cfg:             cfg,
	}
	// Translate: create-target writes a target container (optionally seeded
	// with the source text); it never touches source.
	t.Produce = func(v tool.VariantView) error {
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
