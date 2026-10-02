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
		plan, _ := textPlan(v, conf.ApplySource, targets, caseEdits(conf.Mode))
		return plan, nil
	}
	return t
}

// caseEdits is the case conversion's pass: one edit for each run of
// characters the conversion changes, ending where an inline code sits so the
// code stays between the same two characters. unicode.ToUpper, ToLower and
// ToTitle map one code point to one, so the edits change no position and every
// overlay span keeps the characters it covered.
func caseEdits(mode CaseMode) textPass {
	var convert func(rune) rune
	switch mode {
	case CaseUpper:
		convert = unicode.ToUpper
	case CaseLower:
		convert = unicode.ToLower
	case CaseTitle:
		convert = unicode.ToTitle
	default:
		return textPass{matches: func(textSeq) []textMatch { return nil }}
	}
	return textPass{matches: func(ts textSeq) []textMatch {
		var out []textMatch
		start := -1
		var changed []rune
		flush := func(end int) {
			if start < 0 {
				return
			}
			out = append(out, textMatch{start: start, end: end,
				edits: []model.TextEdit{{Start: start, End: end, Replacement: string(changed)}}})
			start, changed = -1, changed[:0]
		}
		ci := 0
		for i, r := range ts.text {
			for ; ci < len(ts.codes) && ts.codes[ci].at <= i; ci++ {
				if ts.codes[ci].at == i {
					flush(i)
				}
			}
			c := convert(r)
			if c == r {
				flush(i)
				continue
			}
			if start < 0 {
				start = i
			}
			changed = append(changed, c)
		}
		flush(len(ts.text))
		return out
	}}
}
