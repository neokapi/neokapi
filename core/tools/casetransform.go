package tools

import (
	"errors"
	"fmt"
	"unicode"

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
	// Transform producer: returns the case rewrite as an edit plan; the
	// framework applier rewrites the block (AD-006).
	t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		if !v.Translatable() {
			return tool.EditPlan{}, nil
		}
		conf := t.Cfg.(*CaseTransformConfig)
		var targets []model.LocaleID
		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() {
			targets = []model.LocaleID{conf.TargetLocale}
		}
		return textPlan(v, conf.ApplySource, targets, caseEdits(conf.Mode)), nil
	}
	return t
}

// caseEdits is the case conversion's pass: one edit for each character the
// conversion changes. strings.ToUpper, ToLower and ToTitle map character by
// character, so the edits change no position, and an inline code between two
// characters stays between them.
func caseEdits(mode CaseMode) textRewrite {
	var convert func(rune) rune
	switch mode {
	case CaseUpper:
		convert = unicode.ToUpper
	case CaseLower:
		convert = unicode.ToLower
	case CaseTitle:
		convert = unicode.ToTitle
	default:
		return func(string) []model.TextEdit { return nil }
	}
	return func(text string) []model.TextEdit {
		var edits []model.TextEdit
		i := 0
		for _, r := range text {
			if c := convert(r); c != r {
				edits = append(edits, model.TextEdit{Start: i, End: i + 1, Replacement: string(c)})
			}
			i++
		}
		return edits
	}
}
