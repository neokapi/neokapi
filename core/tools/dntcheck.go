package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// DNTCheckConfig configures the do-not-translate checker.
type DNTCheckConfig struct {
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
	// Terms are the do-not-translate strings (product names, trademarks, code
	// identifiers) that must survive verbatim into the target. Sourced from the
	// recipe or a checkset; the termbase can supply more.
	Terms []string `json:"terms,omitempty" schema:"-"`
	// CaseInsensitive accepts a case-folded match in the target. Off by default:
	// do-not-translate is usually case-sensitive ("iPhone", not "Iphone").
	CaseInsensitive bool `json:"caseInsensitive,omitempty" schema:"title=Case-insensitive preservation,description=Accept a case-folded match in the target instead of requiring exact case"`
}

// ToolName returns the tool name this config applies to.
func (c *DNTCheckConfig) ToolName() string { return "dnt-check" }

// Reset restores default values.
func (c *DNTCheckConfig) Reset() {
	c.TargetLocale = ""
	c.Terms = nil
	c.CaseInsensitive = false
}

// Validate checks configuration validity.
func (c *DNTCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("dnt-check: TargetLocale is required")
	}
	return nil
}

// NewDNTCheckConfig creates a DNTCheckConfig for the given target locale.
func NewDNTCheckConfig(targetLocale model.LocaleID) *DNTCheckConfig {
	return &DNTCheckConfig{TargetLocale: targetLocale}
}

// NewDNTCheckFromConfig creates a dnt-check tool from a config map.
func NewDNTCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewDNTCheckConfig(model.LocaleID(targetLang))
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("dnt-check config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewDNTCheckTool(cfg), nil
}

// NewDNTCheckTool creates a do-not-translate checker: for every configured term
// that appears in the source as a whole word, it verifies the term survives
// verbatim into the target and emits a critical finding when it does not — the
// "the AI translated your product name" case. It is read-only (Annotate).
func NewDNTCheckTool(cfg *DNTCheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "dnt-check",
		ToolDescription: "Verifies do-not-translate terms survive verbatim into the target",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}
		conf := t.Cfg.(*DNTCheckConfig)
		if !v.HasTarget(conf.TargetLocale) {
			return nil
		}

		source := v.SourceText()
		target := v.TargetText(conf.TargetLocale)
		sourceRuns := v.SourceRuns()

		var findings []check.Finding
		for _, term := range conf.Terms {
			term = strings.TrimSpace(term)
			if term == "" {
				continue
			}
			hits := check.FindTerm(source, term)
			if len(hits) == 0 {
				continue // term not present in source — nothing to preserve
			}
			preserved := strings.Contains(target, term)
			if !preserved && conf.CaseInsensitive {
				preserved = check.ContainsTerm(target, term)
			}
			if preserved {
				continue
			}
			findings = append(findings, check.Finding{
				Category:     "do-not-translate",
				Severity:     check.SeverityCritical,
				Message:      fmt.Sprintf("Do-not-translate term %q is missing from the %s target — it appears to have been translated or altered", term, conf.TargetLocale),
				Suggestion:   fmt.Sprintf("Keep %q verbatim in the target", term),
				Position:     model.RunRangeForBytes(sourceRuns, hits[0][0], hits[0][1]),
				OriginalText: source[hits[0][0]:hits[0][1]],
			})
		}

		check.Annotate(v, "dnt-check", findings)
		return nil
	}
	return t
}
