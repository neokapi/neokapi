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
	// PropTermCheckWarnings carries the violations that do not fail the check:
	// rules marked advisory, which say a reader should look rather than that
	// the content is wrong. Separate from the errors property so a gate can
	// report both while failing on one.
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

// casedFor reports whether a bilingual rule matches in its own casing: when
// the tool or the rule says so. The replacement is written in the target
// language, so its capitalisation says nothing about how the source term is
// written, and TermRule.MatchesCase's default does not apply here.
func casedFor(cfg *TermCheckConfig, rule profile.TermRule) bool {
	return cfg.CaseSensitive || (rule.CaseSensitive != nil && *rule.CaseSensitive)
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
		if rule.Replacement == "" && !rule.DoNotTranslate {
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
// A rule is demanded when the source uses its term. The source is read as terms
// are matched (check.TermText), so a placeholder's name, inline code, a quoted
// kapi command and a flag name do not use a term. A do-not-translate rule reads
// the source with only its placeholders overwritten, so a term inside a command
// still has to be kept. The target is read with its
// placeholders overwritten (check.PlaceholderText), so a placeholder's name
// does not satisfy a rule and a rendering the target writes anywhere else does.
// The source is matched per word: an English source
// finds a term with its regular inflections unless the rule declares forms of
// its own, and any other source finds the term and its declared forms as whole
// words. Where the terms of two rules cover the same words, only the longer is
// demanded. A demanded rule is satisfied when the target contains its
// replacement, an accepted rendering, or a declared form of either. The target
// is matched by containment, so a compound or a derivation that contains a
// rendering satisfies the rule as well. A do-not-translate rule names no
// replacement: it is satisfied when the target keeps the term verbatim, as the
// term, a declared form or the source's own occurrence, in that casing.
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
	targetText := check.PlaceholderText(target)
	for _, use := range sourceUses(source, cfg) {
		rule := cfg.TermRules[use.index]
		var msg string
		if rule.DoNotTranslate {
			if keptVerbatim(targetText, rule, use.texts) {
				continue
			}
			msg = doNotTranslateViolation(rule)
		} else {
			renderings := rule.Renderings()
			if rendered(targetText, renderings, casedFor(cfg, rule)) {
				continue
			}
			msg = violation(rule, renderings, cfg.TargetLocale)
		}
		// A rule fails unless it is marked advisory. Rules resolved from the
		// terms store are established terms and fail.
		if !rule.Advisory {
			errs = append(errs, msg)
		} else {
			warns = append(warns, msg)
		}
	}
	return errs, warns
}

// sourceUse is a rule the source uses, with the text of each place it does.
type sourceUse struct {
	index int
	texts []string
}

// sourceUses returns, in rule order, every rule the target is held to whose
// term the source uses: a rule that names a replacement, and a do-not-translate
// rule.
//
// A rule that names a replacement reads the source as terms are matched
// (check.TermText), so a command the target keeps as written demands no
// rendering. A do-not-translate rule reads it with only its placeholders
// overwritten (check.PlaceholderText): a target that keeps the command keeps
// the term inside it, and one that drops the command has dropped the term.
//
// Every rule that names a term takes part in the longest-declared-match rule,
// because each is a declaration about which words belong to which term. Both
// readings keep every offset, so their spans compare.
func sourceUses(source string, conf *TermCheckConfig) []sourceUse {
	prose := check.PrepareText(check.TermText(source))
	var kept *check.PreparedText
	english := terms.BaseLanguage(conf.SourceLocale) == "en"
	var spans []check.DeclaredSpan
	for i, rule := range conf.TermRules {
		if strings.TrimSpace(rule.Term) == "" {
			continue
		}
		p := prose
		if rule.DoNotTranslate {
			if kept == nil {
				kept = check.PrepareText(check.PlaceholderText(source))
			}
			p = kept
		}
		cased := casedFor(conf, rule)
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

	uses := map[int]*sourceUse{}
	var order []int
	for _, s := range check.KeepLongestDeclared(spans) {
		rule := conf.TermRules[s.Index]
		if rule.Replacement == "" && !rule.DoNotTranslate {
			continue
		}
		use, ok := uses[s.Index]
		if !ok {
			use = &sourceUse{index: s.Index}
			uses[s.Index] = use
			order = append(order, s.Index)
		}
		use.texts = append(use.texts, source[s.Start:s.End])
	}
	slices.Sort(order)
	out := make([]sourceUse, 0, len(order))
	for _, i := range order {
		out = append(out, *uses[i])
	}
	return out
}

// keptVerbatim reports whether target keeps a do-not-translate term as written:
// the term or a declared form in its own casing, or the source's own occurrence
// of it, so a term the source capitalises at the start of a sentence is kept
// when the target capitalises it the same way.
func keptVerbatim(target string, rule profile.TermRule, occurrences []string) bool {
	for _, s := range append(rule.AllForms(), occurrences...) {
		if s != "" && strings.Contains(target, s) {
			return true
		}
	}
	return false
}

// doNotTranslateViolation words a do-not-translate rule the target does not
// keep. It never contains "; ", which separates findings.
func doNotTranslateViolation(rule profile.TermRule) string {
	msg := fmt.Sprintf("do-not-translate term %q found in source but missing verbatim in target", rule.Term)
	if rule.ConceptID != "" {
		msg += " (concept " + rule.ConceptID + ")"
	}
	return msg
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
		if r.Replacement == "" && !r.DoNotTranslate {
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
// cfg. For the first rule that names a required translation: a block whose
// target lacks every rendering of the rule, and a block whose target holds a
// word that opens like the replacement and ends differently. The second is the
// shape of "Kaiplan" for "kaiplass", and fails a checker that matches a
// rendering by its opening characters. For the first do-not-translate rule: a
// block whose target does not keep the term. With neither kind of rule there is
// nothing to catch, and uncheckable says so.
func TermCheckCanaries(cfg *TermCheckConfig) ([]check.Canary, string) {
	var canaries []check.Canary
	replacement, doNotTranslate := false, false
	for _, rule := range cfg.TermRules {
		if strings.TrimSpace(rule.Term) == "" {
			continue
		}
		switch {
		case rule.DoNotTranslate && !doNotTranslate:
			doNotTranslate = true
			canaries = append(canaries, doNotTranslateCanary(cfg, rule))
		case !rule.DoNotTranslate && rule.Replacement != "" && !replacement:
			replacement = true
			canaries = append(canaries, replacementCanaries(cfg, rule)...)
		}
	}
	if len(canaries) == 0 {
		return nil, "no term rule names a required translation or a do-not-translate term"
	}
	return canaries, ""
}

// replacementCanaries is the known-bad input for a rule that names a required
// translation.
func replacementCanaries(cfg *TermCheckConfig, rule profile.TermRule) []check.Canary {
	renderings := rule.Renderings()
	cased := casedFor(cfg, rule)

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
	return canaries
}

// doNotTranslateCanary is the known-bad input for a do-not-translate rule: a
// block whose target does not keep the term.
func doNotTranslateCanary(cfg *TermCheckConfig, rule profile.TermRule) check.Canary {
	absent := "0"
	if keptVerbatim(absent, rule, nil) {
		absent = "1"
	}
	b := check.CanaryBlock(rule.Term)
	b.SetTargetText(cfg.TargetLocale, absent)
	return check.Canary{Name: fmt.Sprintf("do-not-translate term %q not kept", rule.Term), Block: b}
}
