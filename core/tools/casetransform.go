package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// CaseMode controls the case transformation applied.
type CaseMode string

const (
	CaseUpper CaseMode = "upper"
	CaseLower CaseMode = "lower"
	CaseTitle CaseMode = "title"
)

// CaseTransformConfig holds configuration for the case transform tool.
type CaseTransformConfig struct {
	Mode         CaseMode       `json:"mode,omitempty"         schema:"title=Transformation Mode,description=Case transformation mode,enum=upper|lower|title,default=upper"`
	ApplySource  bool           `json:"applySource,omitempty"  schema:"title=Apply to Source,description=Apply to source text"`
	ApplyTarget  bool           `json:"applyTarget,omitempty"  schema:"title=Apply to Target,description=Apply to target text"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *CaseTransformConfig) ToolName() string { return "case-transform" }

// Reset restores default values.
func (c *CaseTransformConfig) Reset() {
	c.Mode = CaseUpper
	c.ApplySource = true
	c.ApplyTarget = false
	c.TargetLocale = ""
}

// Validate checks configuration validity.
func (c *CaseTransformConfig) Validate() error {
	switch c.Mode {
	case CaseUpper, CaseLower, CaseTitle:
	default:
		return fmt.Errorf("case-transform: invalid Mode %q (use upper, lower, or title)", c.Mode)
	}
	if c.ApplyTarget && c.TargetLocale.IsEmpty() {
		return errors.New("case-transform: TargetLocale required when ApplyTarget is true")
	}
	return nil
}

// CaseTransformSchema returns the auto-generated schema for the case-transform tool.
func CaseTransformSchema() *schema.ComponentSchema {
	return schema.FromStruct(&CaseTransformConfig{}, schema.ToolMeta{
		ID:          "case-transform",
		Category:    schema.CategoryTextProcessing,
		DisplayName: "Case Transform",
		Description: "Transform text case (upper, lower, title)",
	})
}

// NewCaseTransformFromConfig creates a case-transform tool from a config map.
func NewCaseTransformFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg CaseTransformConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("case-transform config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewCaseTransformTool(&cfg), nil
}

// NewCaseTransformTool creates a tool that transforms the case of text in blocks.
func NewCaseTransformTool(cfg *CaseTransformConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "case-transform",
		ToolDescription: "Transforms the case of source and/or target text",
		Cfg:             cfg,
	}
	// Transform: case-transform may rewrite source (and/or target). As a
	// source-transform it runs early, before any overlay is attached.
	t.Transform = func(v tool.SourceView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*CaseTransformConfig)

		if conf.ApplySource {
			v.SetSourceText(transformCase(v.SourceText(), conf.Mode))
		}

		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() && v.HasTarget(conf.TargetLocale) {
			v.SetTargetText(conf.TargetLocale, transformCase(v.TargetText(conf.TargetLocale), conf.Mode))
		}

		return nil
	}
	return t
}

func transformCase(text string, mode CaseMode) string {
	switch mode {
	case CaseUpper:
		return strings.ToUpper(text)
	case CaseLower:
		return strings.ToLower(text)
	case CaseTitle:
		return strings.ToTitle(text)
	default:
		return text
	}
}
