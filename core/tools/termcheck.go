package tools

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/terms"
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
	TermRules []profile.TermRule `json:"term_rules,omitempty"    schema:"-"`
	// SourceLocale is the language the source is written in. An English source
	// finds a term with its regular inflections; any other finds a term as a
	// whole word under its text and declared forms.
	SourceLocale  model.LocaleID `json:"sourceLocale,omitempty"  schema:"-"`
	TargetLocale  model.LocaleID `json:"targetLocale,omitempty"  schema:"-"`
	CaseSensitive bool           `json:"caseSensitive,omitempty" schema:"title=Case Sensitive,description=Whether term matching is case-sensitive"`
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
	c.SourceLocale = ""
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

// NewTermCheckTool creates a tool that holds a translation to the renderings
// its term rules require.
//
// A rule is demanded when the source uses its term. Both sides are read as
// terms are matched (check.TermText), so a placeholder's name neither uses a
// term nor satisfies a rule. The source is matched per word: an English source
// finds a term with its regular inflections unless the rule declares forms of
// its own, and any other source finds the term and its declared forms as whole
// words. Where the terms of two rules cover the same words, only the longer is
// demanded. A demanded rule is satisfied when the target contains its
// replacement, an accepted rendering, or a declared form of either. The target
// is matched by containment, so a compound or a derivation that contains a
// rendering satisfies the rule as well.
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

		errs, warns := TermCheckViolations(conf, v.SourceText(), v.TargetText(conf.TargetLocale))

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

// TermCheckViolations is term-check's decision for one source and target
// text: the messages of the demanded rules the target does not satisfy, sorted
// into the violations that fail and those that only warn. A surface that holds
// content to term rules outside a pipeline calls it, so it decides exactly
// what the tool decides.
func TermCheckViolations(cfg *TermCheckConfig, source, target string) (errs, warns []string) {
	sourceText := check.TermText(source)
	targetText := check.TermText(target)
	for _, i := range sourceUses(sourceText, cfg) {
		rule := cfg.TermRules[i]
		renderings := rule.Renderings()
		if rendered(targetText, renderings, cfg.CaseSensitive || rule.CaseSensitive) {
			continue
		}
		msg := violation(rule, renderings, cfg.TargetLocale)
		if failsTheCheck(rule.Severity) {
			errs = append(errs, msg)
		} else {
			warns = append(warns, msg)
		}
	}
	return errs, warns
}

// sourceUses returns, in rule order, the index of every rule that names a
// replacement and whose term the source uses.
//
// Every rule that names a term takes part in the longest-declared-match rule,
// including a do-not-translate name that requires no replacement, because each
// is a declaration about which words belong to which term.
func sourceUses(source string, conf *TermCheckConfig) []int {
	p := check.PrepareText(source)
	english := terms.BaseLanguage(conf.SourceLocale) == "en"
	var spans []check.DeclaredSpan
	for i, rule := range conf.TermRules {
		if strings.TrimSpace(rule.Term) == "" {
			continue
		}
		cased := conf.CaseSensitive || rule.CaseSensitive
		var hits [][2]int
		if english && len(rule.Forms) == 0 {
			hits = check.FindEnglishInflectionsIn(p, rule.Term, cased)
		} else {
			hits = check.FindTermFormsIn(p, rule.AllForms(), cased)
		}
		for _, h := range hits {
			spans = append(spans, check.DeclaredSpan{Start: h[0], End: h[1], Index: i})
		}
	}

	var out []int
	for _, s := range check.KeepLongestDeclared(spans) {
		if conf.TermRules[s.Index].Replacement != "" && !slices.Contains(out, s.Index) {
			out = append(out, s.Index)
		}
	}
	slices.Sort(out)
	return out
}

// rendered reports whether target contains one of the renderings or a declared
// form of one.
func rendered(target string, renderings []profile.Rendering, caseSensitive bool) bool {
	for _, r := range renderings {
		if containsTerm(target, r.Text, caseSensitive) {
			return true
		}
		for _, f := range r.Forms {
			if strings.TrimSpace(f) != "" && containsTerm(target, f, caseSensitive) {
				return true
			}
		}
	}
	return false
}

// violation words a demanded rule the target does not satisfy. It opens the way
// every term-check finding has, and adds what a reviewer needs to tell a
// missing form from a different word: the concept, the other renderings the
// rule accepts, and whether the target language needs forms that the rule
// does not declare. It never contains "; ", which separates findings.
func violation(rule profile.TermRule, renderings []profile.Rendering, target model.LocaleID) string {
	msg := fmt.Sprintf("term %q found in source but required translation %q missing in target", rule.Term, rule.Replacement)
	var notes []string
	if rule.ConceptID != "" {
		notes = append(notes, "concept "+rule.ConceptID)
	}
	if len(renderings) > 1 {
		accepted := make([]string, 0, len(renderings)-1)
		for _, r := range renderings[1:] {
			accepted = append(accepted, fmt.Sprintf("%q", r.Text))
		}
		notes = append(notes, "also accepted: "+strings.Join(accepted, ", "))
	}
	if !declaresForms(renderings) && terms.LanguageInflects(target) {
		notes = append(notes, "no forms declared in "+terms.BaseLanguage(target))
	}
	if len(notes) == 0 {
		return msg
	}
	return msg + " (" + strings.Join(notes, ", ") + ")"
}

func declaresForms(renderings []profile.Rendering) bool {
	for _, r := range renderings {
		if len(r.Forms) > 0 {
			return true
		}
	}
	return false
}

func containsTerm(text, term string, caseSensitive bool) bool {
	if caseSensitive {
		return strings.Contains(text, term)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(term))
}

// TermCheckMatching describes how term-check matches under cfg, for the verdict
// to record per target language.
func TermCheckMatching(cfg *TermCheckConfig) check.TermMatching {
	m := check.TermMatching{
		Locale: string(cfg.TargetLocale),
		Source: check.TermSourceWholeWord,
		Target: check.TermTargetContainment,
	}
	if terms.BaseLanguage(cfg.SourceLocale) == "en" {
		m.Source = check.TermSourceEnglishInflection
	}
	for _, r := range cfg.TermRules {
		if r.Replacement == "" {
			continue
		}
		m.Rules++
		if declaresForms(r.Renderings()) {
			m.RulesWithForms++
		}
	}
	if m.RulesWithForms > 0 {
		m.Target = check.TermTargetContainmentForms
	}
	return m
}

// TermCheckCanaries is the known-bad input the term-check tool must flag under
// cfg, built from the first rule that names a required translation: a block
// whose target lacks every rendering of the rule, and a block whose target holds
// a word that opens like the replacement and ends differently. The second is
// the shape of "Kaiplan" for "kaiplass", and fails a checker that matches a
// rendering by its opening characters. With no rule that names a translation
// there is nothing to catch, and uncheckable says so.
func TermCheckCanaries(cfg *TermCheckConfig) ([]check.Canary, string) {
	for _, rule := range cfg.TermRules {
		if strings.TrimSpace(rule.Term) == "" || rule.Replacement == "" {
			continue
		}
		renderings := rule.Renderings()
		cased := cfg.CaseSensitive || rule.CaseSensitive

		absent := "0"
		if rendered(absent, renderings, cased) {
			absent = "1"
		}
		deleted := check.CanaryBlock(rule.Term)
		deleted.SetTargetText(cfg.TargetLocale, absent)
		canaries := []check.Canary{{Name: fmt.Sprintf("term %q without %q", rule.Term, rule.Replacement), Block: deleted}}

		if clipped := check.ClipWord(rule.Replacement); clipped != "" && !rendered(clipped, renderings, cased) {
			b := check.CanaryBlock(rule.Term)
			b.SetTargetText(cfg.TargetLocale, clipped)
			canaries = append(canaries, check.Canary{
				Name:  fmt.Sprintf("term %q with %q in place of %q", rule.Term, clipped, rule.Replacement),
				Block: b,
			})
		}
		return canaries, ""
	}
	return nil, "no term rule names a required translation"
}
