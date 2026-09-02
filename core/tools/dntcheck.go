package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
)

// DNTCheckConfig configures the do-not-translate checker.
type DNTCheckConfig struct {
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
	// Terms are the do-not-translate strings (product names, trademarks, code
	// identifiers) that must survive verbatim into the target. Sourced from the
	// recipe or a checkset; the terms store can supply more.
	//
	// Schema-visible, because it is the only input that makes the check do
	// anything: hidden from the schema it had no CLI flag, so `kapi exec
	// dnt-check` could only ever run with an empty list — a check that cannot
	// fail. `kapi check --dnt` is the same list on the porcelain verb.
	Terms []string `json:"terms,omitempty" schema:"title=Do-Not-Translate Terms,description=Strings that must survive verbatim into the target (product names, trademarks, identifiers)"`
	// CaseInsensitive accepts a case-folded match in the target. Off by default:
	// do-not-translate is usually case-sensitive ("iPhone", not "Iphone").
	CaseInsensitive bool `json:"caseInsensitive,omitempty" schema:"title=Case-insensitive preservation,description=Accept a case-folded match in the target instead of requiring exact case"`
	// TermRules is the project's terminology, the same key and shape every
	// governed step takes. Its do-not-translate rules join Terms above, which
	// is what the comment on that field has always promised: a store that
	// already says "never translate" about a product name should not need the
	// claim repeated in the recipe before anything enforces it.
	TermRules []profile.TermRule `json:"term_rules,omitempty" schema:"-"`
}

// EffectiveTerms is every string this check must see survive: the recipe's
// explicit list plus the concepts the terms store marks do-not-translate.
// Declared in both places, a term is checked once.
func (c *DNTCheckConfig) EffectiveTerms() []string {
	seen := make(map[string]bool, len(c.Terms)+len(c.TermRules))
	out := make([]string, 0, len(c.Terms)+len(c.TermRules))
	add := func(term string) {
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		out = append(out, term)
	}
	for _, term := range c.Terms {
		add(term)
	}
	for _, rule := range c.TermRules {
		if rule.DoNotTranslate {
			add(rule.Term)
		}
	}
	return out
}

// ToolName returns the tool name this config applies to.
func (c *DNTCheckConfig) ToolName() string { return "dnt-check" }

// Reset restores default values.
func (c *DNTCheckConfig) Reset() {
	c.TargetLocale = ""
	c.Terms = nil
	c.TermRules = nil
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
//
// The Terms list is the tool's entire behaviour, and it can only arrive here.
// Written but never registered as the registry's ConfigFactory, dnt-check ran
// from every flow with no terms at all and an `en` target locale — a
// do-not-translate guardrail that could not fail, which is worse than none
// because it reports reassurance (#1476).
func NewDNTCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewDNTCheckConfig(model.LocaleID(targetLang))
	if err := applyStepConfig("dnt-check", config, cfg, targetLang, setDNTCheckLocale); err != nil {
		return nil, err
	}
	return NewDNTCheckTool(cfg), nil
}

func setDNTCheckLocale(c *DNTCheckConfig, loc model.LocaleID) { c.TargetLocale = loc }

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
		for _, term := range conf.EffectiveTerms() {
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
				Message:      fmt.Sprintf("Do-not-translate term %q is missing from the %s target: it appears to have been translated or altered", term, conf.TargetLocale),
				Suggestion:   fmt.Sprintf("Keep %q verbatim in the target", term),
				Position:     model.RangeAnchorForBytes(sourceRuns, hits[0][0], hits[0][1]),
				OriginalText: source[hits[0][0]:hits[0][1]],
				// Record the verbatim term so a host can show what must be
				// preserved. We deliberately do NOT set Metadata["replacement"]:
				// the finding fires because the term is *absent* from the target,
				// so there is no corrupted span we can safely substitute — a
				// do-not-translate miss needs a human (or re-translation), not a
				// blind substring replace.
				Metadata: map[string]string{"preserve": term},
			})
		}

		check.Annotate(v, "dnt-check", findings)
		return nil
	}
	return t
}
