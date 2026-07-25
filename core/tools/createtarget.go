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

// NewCreateTargetConfig creates a CreateTargetConfig at its documented defaults
// for the given target locale.
func NewCreateTargetConfig(targetLocale model.LocaleID) *CreateTargetConfig {
	cfg := &CreateTargetConfig{}
	cfg.Reset()
	cfg.TargetLocale = targetLocale
	return cfg
}

// NewCreateTargetFromConfig creates a create-target tool from a config map.
// Without it the tool's target locale came from a zero-arg factory that left it
// empty, so `create-target` from a flow wrote its container under the empty
// locale, and `copySource` / `overwrite` were discarded (#1476).
func NewCreateTargetFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewCreateTargetConfig(model.LocaleID(targetLang))
	if err := applyStepConfig("create-target", config, cfg, targetLang, setCreateTargetLocale); err != nil {
		return nil, err
	}
	return NewCreateTargetTool(cfg), nil
}

func setCreateTargetLocale(c *CreateTargetConfig, loc model.LocaleID) { c.TargetLocale = loc }

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
