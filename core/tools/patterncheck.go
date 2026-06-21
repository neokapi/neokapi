package tools

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
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

	// CheckSource evaluates the source text instead of a target, so forbidden
	// (MustNotMatch) and required (MustMatch) patterns can be validated on a
	// single file with no target-language. Default false keeps the bilingual
	// (source-vs-target) behavior.
	CheckSource bool `json:"checkSource,omitempty" schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *PatternCheckConfig) ToolName() string { return "pattern-check" }

// Reset restores default values.
func (c *PatternCheckConfig) Reset() {
	c.TargetLocale = ""
	c.CheckSource = false
	c.Patterns = nil
}

// Validate checks configuration validity.
func (c *PatternCheckConfig) Validate() error {
	if !c.CheckSource && c.TargetLocale.IsEmpty() {
		return errors.New("pattern-check: TargetLocale is required")
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
		Category:    schema.CategoryQuality,
		DisplayName: "Pattern Check",
		Description: "Validate content against custom regex patterns",
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
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*PatternCheckConfig)

		// Source scope: validate the source text directly (no target). Forbidden
		// patterns must be absent; required patterns must be present.
		if conf.CheckSource {
			text := v.SourceText()
			var findings []check.Finding
			for _, rule := range compiled {
				if rule.re == nil {
					continue
				}
				if rule.MustMatch && !rule.re.MatchString(text) {
					findings = append(findings, check.Finding{
						Category: "pattern-missing",
						Severity: check.SeverityMajor,
						Message: fmt.Sprintf("Pattern %q (%s): required pattern not found in source",
							rule.Name, rule.Pattern),
					})
				}
				if rule.MustNotMatch {
					if loc := rule.re.FindString(text); loc != "" {
						findings = append(findings, check.Finding{
							Category: "forbidden-pattern",
							Severity: check.SeverityMajor,
							Message: fmt.Sprintf("Pattern %q (%s): forbidden pattern found in source",
								rule.Name, rule.Pattern),
							OriginalText: loc,
						})
					}
				}
			}
			check.Annotate(v, "pattern-check", findings)
			return nil
		}

		sourceText := v.SourceText()

		// If no target, nothing to check.
		if !v.HasTarget(conf.TargetLocale) {
			return nil
		}

		targetText := v.TargetText(conf.TargetLocale)

		var findings []check.Finding

		for _, rule := range compiled {
			if rule.re == nil {
				continue
			}

			if rule.MustMatch {
				// Find all matches in source and target; counts must match.
				sourceMatches := rule.re.FindAllString(sourceText, -1)
				targetMatches := rule.re.FindAllString(targetText, -1)
				if len(sourceMatches) != len(targetMatches) {
					findings = append(findings, check.Finding{
						Category: "pattern-mismatch",
						Severity: check.SeverityMajor,
						Message: fmt.Sprintf("Pattern %q (%s): source has %d matches, target has %d",
							rule.Name, rule.Pattern, len(sourceMatches), len(targetMatches)),
					})
				}
			}

			if rule.MustNotMatch {
				// Pattern must not appear in target.
				if loc := rule.re.FindString(targetText); loc != "" {
					findings = append(findings, check.Finding{
						Category: "forbidden-pattern",
						Severity: check.SeverityMajor,
						Message: fmt.Sprintf("Pattern %q (%s): forbidden pattern found in target",
							rule.Name, rule.Pattern),
						OriginalText: loc,
					})
				}
			}
		}

		check.Annotate(v, "pattern-check", findings)

		return nil
	}
	return t
}
