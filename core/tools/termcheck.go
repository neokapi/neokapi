package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Terminology check property keys.
const (
	PropTermCheckPassed = "term-check-passed"
	PropTermCheckErrors = "term-check-errors"
	// PropTermCheckWarnings carries the violations that did NOT fail the
	// check: rules whose severity says a reader should look, not that the
	// content is wrong. Separate from the errors property so a gate can report
	// both while failing on one.
	PropTermCheckWarnings = "term-check-warnings"
)

// TermCheckConfig holds configuration for the terminology check tool.
//
// TermRules is the same type the voice profile writes its vocabulary with, and
// it reads the same way: when the text contains Term, it should say Replacement
// instead. What differs is only WHICH text — TargetLocale says the replacement
// is required in the translation rather than in the same language, so one rule
// shape serves a monolingual style rule and a cross-language terminology
// requirement.
type TermCheckConfig struct {
	TermRules     []profile.TermRule `json:"term_rules,omitempty"    schema:"-"`
	TargetLocale  model.LocaleID     `json:"targetLocale,omitempty"  schema:"-"`
	CaseSensitive bool               `json:"caseSensitive,omitempty" schema:"title=Case Sensitive,description=Whether term matching is case-sensitive"`
}

// failsTheCheck reports whether a violated rule makes the block fail rather
// than merely warn.
//
// An unset severity fails. Rules resolved from the terms store carry no
// severity — they are the project's terminology, not a graded suggestion — and
// silently downgrading those to warnings would turn the terminology gate off
// for every project that never wrote a severity by hand.
func failsTheCheck(severity string) bool {
	return check.Severity(severity) != check.SeverityMinor &&
		check.Severity(severity) != check.SeverityNeutral
}

// ToolName returns the tool name this config applies to.
func (c *TermCheckConfig) ToolName() string { return "term-check" }

// Reset restores default values.
func (c *TermCheckConfig) Reset() {
	c.TermRules = nil
	c.TargetLocale = ""
	c.CaseSensitive = false
}

// Validate checks configuration validity.
func (c *TermCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("term-check: TargetLocale is required")
	}
	for i, rule := range c.TermRules {
		if rule.Term == "" {
			return fmt.Errorf("term-check: term rule %d has no term", i)
		}
		if rule.Replacement == "" {
			return fmt.Errorf("term-check: term rule %d (%q) has no replacement: a rule here says what to use INSTEAD, so both halves are required", i, rule.Term)
		}
	}
	return nil
}

// NewTermCheckFromConfig creates a term-check tool from a config map.
func NewTermCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg TermCheckConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("term-check config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewTermCheckTool(&cfg), nil
}

// NewTermCheckTool creates a tool that verifies terminology usage in translations.
// For each rule, if the term appears in the source text,
// the tool checks that the required target term appears in the target text.
func NewTermCheckTool(cfg *TermCheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "term-check",
		ToolDescription: "Verifies terminology usage in translations against the project's term rules",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*TermCheckConfig)
		if len(conf.TermRules) == 0 {
			return nil
		}

		if !v.HasTarget(conf.TargetLocale) {
			return nil
		}

		sourceText := v.SourceText()
		targetText := v.TargetText(conf.TargetLocale)

		var errs, warns []string
		for _, rule := range conf.TermRules {
			if rule.Replacement == "" {
				// A rule with nothing to require cannot be violated. In a voice
				// profile a bare term is meaningful — `preferred_terms` lists
				// wording to reach for — but here the rule is "say this
				// instead", and there is no "this".
				continue
			}
			if !containsTerm(sourceText, rule.Term, conf.CaseSensitive) {
				continue
			}
			if containsTerm(targetText, rule.Replacement, conf.CaseSensitive) {
				continue
			}
			msg := fmt.Sprintf("term %q found in source but required translation %q missing in target", rule.Term, rule.Replacement)
			if failsTheCheck(rule.Severity) {
				errs = append(errs, msg)
			} else {
				warns = append(warns, msg)
			}
		}

		if len(errs) == 0 {
			v.SetProperty(PropTermCheckPassed, "true")
		} else {
			v.SetProperty(PropTermCheckPassed, "false")
			v.SetProperty(PropTermCheckErrors, strings.Join(errs, "; "))
		}
		if len(warns) > 0 {
			v.SetProperty(PropTermCheckWarnings, strings.Join(warns, "; "))
		}

		return nil
	}
	return t
}

func containsTerm(text, term string, caseSensitive bool) bool {
	if caseSensitive {
		return strings.Contains(text, term)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(term))
}
