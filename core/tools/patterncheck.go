package tools

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Pattern check property keys stored on Block.Properties.
const (
	PropPatternCheckPassed = "pattern-check-passed" // "true" or "false"
	PropPatternCheckIssues = "pattern-check-issues" // JSON array of issues
)

// PatternRule defines a regex pattern to validate in translations.
type PatternRule struct {
	Name         string // Human-readable name (e.g., "printf-placeholder", "html-entity")
	Pattern      string // Regex pattern to match (e.g., `%[sdfu]`, `&\w+;`)
	MustMatch    bool   // If true, pattern MUST appear in target if it appears in source (preserved patterns)
	MustNotMatch bool   // If true, pattern must NOT appear in target (forbidden patterns)
}

// PatternCheckConfig holds configuration for the pattern check tool.
type PatternCheckConfig struct {
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
	Patterns     []PatternRule  `json:"patterns,omitempty"     schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *PatternCheckConfig) ToolName() string { return "pattern-check" }

// Reset restores default values.
func (c *PatternCheckConfig) Reset() {
	c.TargetLocale = ""
	c.Patterns = nil
}

// Validate checks configuration validity.
func (c *PatternCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("pattern-check: TargetLocale is required")
	}
	for i, rule := range c.Patterns {
		if rule.Pattern == "" {
			return fmt.Errorf("pattern-check: Patterns[%d].Pattern is empty", i)
		}
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("pattern-check: Patterns[%d].Pattern is invalid: %w", i, err)
		}
		if rule.MustMatch && rule.MustNotMatch {
			return fmt.Errorf("pattern-check: Patterns[%d] cannot have both MustMatch and MustNotMatch", i)
		}
	}
	return nil
}

// PatternCheckSchema returns the auto-generated schema for the pattern-check tool.
func PatternCheckSchema() *schema.ComponentSchema {
	return schema.FromStruct(&PatternCheckConfig{}, schema.ToolMeta{
		ID:          "pattern-check",
		Category:    schema.CategoryValidate,
		DisplayName: "Pattern Check",
		Description: "Validate content against custom regex patterns",
		Inputs:      []string{schema.PartTypeBlock},
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewPatternCheckFromConfig creates a pattern-check tool from a config map.
func NewPatternCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg PatternCheckConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("pattern-check config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewPatternCheckTool(&cfg), nil
}

// NewPatternCheckTool creates a pattern validation tool for translations.
// It checks that regex patterns (e.g., placeholders, variables) are correctly
// preserved or absent in target text, storing findings in Block.Properties.
func NewPatternCheckTool(cfg *PatternCheckConfig) *tool.BaseTool {
	// Pre-compile patterns.
	type compiledRule struct {
		PatternRule
		re *regexp.Regexp
	}
	compiled := make([]compiledRule, len(cfg.Patterns))
	for i, rule := range cfg.Patterns {
		re, _ := regexp.Compile(rule.Pattern)
		compiled[i] = compiledRule{PatternRule: rule, re: re}
	}

	t := &tool.BaseTool{
		ToolName:        "pattern-check",
		ToolDescription: "Validates regex patterns in translations (e.g., placeholders, variables)",
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

		conf := t.Cfg.(*PatternCheckConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		sourceText := block.SourceText()

		// If no target, nothing to check.
		if !block.HasTarget(conf.TargetLocale) {
			block.Properties[PropPatternCheckPassed] = "true"
			block.Properties[PropPatternCheckIssues] = "[]"
			return part, nil
		}

		targetText := block.TargetText(conf.TargetLocale)

		var issues []QAIssue

		for _, rule := range compiled {
			if rule.re == nil {
				continue
			}

			if rule.MustMatch {
				// Find all matches in source and target; counts must match.
				sourceMatches := rule.re.FindAllString(sourceText, -1)
				targetMatches := rule.re.FindAllString(targetText, -1)
				if len(sourceMatches) != len(targetMatches) {
					issues = append(issues, QAIssue{
						Type:     "pattern-mismatch",
						Severity: QASeverityError,
						Message: fmt.Sprintf("Pattern %q (%s): source has %d matches, target has %d",
							rule.Name, rule.Pattern, len(sourceMatches), len(targetMatches)),
					})
				}
			}

			if rule.MustNotMatch {
				// Pattern must not appear in target.
				if rule.re.MatchString(targetText) {
					issues = append(issues, QAIssue{
						Type:     "forbidden-pattern",
						Severity: QASeverityError,
						Message: fmt.Sprintf("Pattern %q (%s): forbidden pattern found in target",
							rule.Name, rule.Pattern),
					})
				}
			}
		}

		storePatternCheckIssues(block, issues)

		return part, nil
	}
	return t
}

// storePatternCheckIssues writes pattern check findings to Block.Properties.
func storePatternCheckIssues(block *model.Block, issues []QAIssue) {
	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	if len(issues) == 0 {
		block.Properties[PropPatternCheckPassed] = "true"
		block.Properties[PropPatternCheckIssues] = "[]"
		return
	}

	block.Properties[PropPatternCheckPassed] = "false"
	data, err := json.Marshal(issues)
	if err != nil {
		block.Properties[PropPatternCheckIssues] = "[]"
		return
	}
	block.Properties[PropPatternCheckIssues] = string(data)
}
