package tools

import (
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/tool"
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
	Mode         CaseMode       // upper, lower, or title
	ApplySource  bool           // Apply to source text
	ApplyTarget  bool           // Apply to target text
	TargetLocale model.LocaleID // Required if ApplyTarget is true
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
		return fmt.Errorf("case-transform: TargetLocale required when ApplyTarget is true")
	}
	return nil
}

// NewCaseTransformTool creates a tool that transforms the case of text in blocks.
func NewCaseTransformTool(cfg *CaseTransformConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "case-transform",
		ToolDescription: "Transforms the case of source and/or target text",
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

		conf := t.Cfg.(*CaseTransformConfig)

		if conf.ApplySource {
			sourceText := block.SourceText()
			block.SetSourceText(transformCase(sourceText, conf.Mode))
		}

		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() && block.HasTarget(conf.TargetLocale) {
			targetText := block.TargetText(conf.TargetLocale)
			block.SetTargetText(conf.TargetLocale, transformCase(targetText, conf.Mode))
		}

		return part, nil
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
