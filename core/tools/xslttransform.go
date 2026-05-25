package tools

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// XSLTTransformConfig holds configuration for the XSLT-like transform tool.
// Since Go has no built-in XSLT engine, this provides a lightweight
// tag-transformation approach using regex-based rules.
type XSLTTransformConfig struct {
	Rules []TransformRule `json:"rules,omitempty" schema:"title=Transformation Rules,description=Regex-based tag and text transformation rules"`

	ApplySource  bool `json:"applySource,omitempty"   schema:"title=Apply to Source,description=Apply transformation to source text,default=true"`
	ApplyTarget  bool `json:"applyTarget,omitempty"   schema:"title=Apply to Target,description=Apply transformation to target text"`
	PassOnOutput bool `json:"passOnOutput,omitempty"  schema:"title=Pass Output to Next Step,description=Pass the transformation output to subsequent pipeline steps,default=true"`
}

// TransformRule defines a single tag or text transformation rule.
type TransformRule struct {
	Pattern string // Regex pattern to match
	Replace string // Replacement string (supports $1, $2 backreferences)
}

// ToolName returns the tool name this config applies to.
func (c *XSLTTransformConfig) ToolName() string { return "xslt-transform" }

// Reset restores default values.
func (c *XSLTTransformConfig) Reset() {
	c.Rules = nil
	c.ApplySource = true
	c.ApplyTarget = false
	c.PassOnOutput = true
}

// Validate checks configuration validity.
func (c *XSLTTransformConfig) Validate() error {
	for i, rule := range c.Rules {
		if rule.Pattern == "" {
			return fmt.Errorf("xslt-transform: rule %d has empty pattern", i)
		}
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("xslt-transform: rule %d has invalid pattern %q: %w", i, rule.Pattern, err)
		}
	}
	return nil
}

// NewXSLTTransformTool creates a lightweight tag-transformation tool.
// It applies regex-based transformation rules to source text in blocks.
func NewXSLTTransformTool(cfg *XSLTTransformConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "xslt-transform",
		ToolDescription: "Applies regex-based tag/text transformations to block text",
		Cfg:             cfg,
		WritesSource:    true,
		WritesTarget:    true,
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if !block.Translatable {
			return part, nil
		}

		conf := t.Cfg.(*XSLTTransformConfig)
		if len(conf.Rules) == 0 {
			return part, nil
		}

		// Default to source if neither scope is explicitly set.
		applySource := conf.ApplySource || (!conf.ApplySource && !conf.ApplyTarget)

		if applySource {
			sourceText := block.SourceText()
			newText, err := applyTransformRules(sourceText, conf.Rules)
			if err != nil {
				return nil, fmt.Errorf("xslt-transform: source: %w", err)
			}
			if newText != sourceText {
				block.SetSourceText(newText)
			}
		}

		if conf.ApplyTarget {
			for _, locale := range block.TargetLocales() {
				targetText := block.TargetText(locale)
				newText, err := applyTransformRules(targetText, conf.Rules)
				if err != nil {
					return nil, fmt.Errorf("xslt-transform: target: %w", err)
				}
				if newText != targetText {
					block.SetTargetText(locale, newText)
				}
			}
		}

		return part, nil
	}
	return t
}

// applyTransformRules applies all transformation rules to the text sequentially.
func applyTransformRules(text string, rules []TransformRule) (string, error) {
	result := text
	for _, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return "", fmt.Errorf("invalid pattern %q: %w", rule.Pattern, err)
		}
		result = re.ReplaceAllString(result, rule.Replace)
	}
	return strings.TrimRight(result, ""), nil
}
