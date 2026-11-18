package tools

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

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
	SourceLocale  model.LocaleID `json:"source_locale,omitempty"  schema:"-"`
	TargetLocale  model.LocaleID `json:"target_locale,omitempty"  schema:"-"`
	CaseSensitive bool           `json:"case_sensitive,omitempty" schema:"title=Case Sensitive,description=Whether term matching is case-sensitive"`
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
// content to term rules outside a pipeline calls it, so it reports exactly
// what the tool reports.
func TermCheckViolations(cfg *TermCheckConfig, source, target string) (errs, warns []string) {
	for _, use := range TermCheckUses(cfg, source, target) {
		if use.Satisfied {
			continue
		}
		var msg string
		if use.Rule.DoNotTranslate {
			msg = doNotTranslateViolation(use.Rule)
		} else {
			msg = violation(use.Rule, use.Rule.Renderings(), cfg.TargetLocale)
		}
		// A rule fails unless it is marked advisory. Rules resolved from the
		// terms store are established terms and fail.
		if !use.Rule.Advisory {
			errs = append(errs, msg)
		} else {
			warns = append(warns, msg)
		}
	}
	return errs, warns
}

// TermUse is one rule term-check holds a translation to, with where the
// decision found it: the byte ranges of the term in the source, and of each
// rendering (or, for a do-not-translate rule, of the kept term) in the target.
// Satisfied is the verdict, and it is true exactly when Target holds a range.
type TermUse struct {
	Rule      profile.TermRule
	Source    [][2]int
	Target    [][2]int
	Satisfied bool
}

// TermCheckUses is term-check's decision for one source and target text,
// rule by rule, with positions. TermCheckViolations reports from it and
// TermCheckOverlays draws from it, so a surface that shows where a term sits
// shows what the check decided.
func TermCheckUses(cfg *TermCheckConfig, source, target string) []TermUse {
	targetText := check.PlaceholderText(target)
	uses := sourceUses(source, cfg)
	out := make([]TermUse, 0, len(uses))
	for _, use := range uses {
		rule := cfg.TermRules[use.index]
		var hits [][2]int
		if rule.DoNotTranslate {
			hits = verbatimSpans(targetText, rule, use.texts)
		} else {
			hits = renderingSpans(targetText, rule.Renderings(), casedFor(cfg, rule))
		}
		out = append(out, TermUse{Rule: rule, Source: use.spans, Target: hits, Satisfied: len(hits) > 0})
	}
	return out
}

// TermCheckOverlays draws term-check's decision as term overlays: one over
// the source runs, a span for each place the source uses a demanded term, and
// one over the translation in cfg.TargetLocale, a span for each rendering the
// target holds. The spans of one rule share an id ("term-check:<n>"), and
// each carries the rule as a TermAnnotation and, in its props, the verdict
// ("satisfied"), the source term ("term"), the rendering it requires
// ("replacement") and the concept ("concept_id"), so
// a reader of the wire form, which keeps props, can pair a source term with
// its rendering, tell a missing one, and open the concept. It returns nil when
// the source uses no demanded term.
func TermCheckOverlays(cfg *TermCheckConfig, sourceRuns, targetRuns []model.Run) []model.Overlay {
	uses := TermCheckUses(cfg, model.RunsText(sourceRuns), model.RunsText(targetRuns))
	if len(uses) == 0 {
		return nil
	}
	src := model.Overlay{Type: model.OverlayTerm}
	tgt := model.Overlay{Type: model.OverlayTerm, Edition: model.Variant(cfg.TargetLocale)}
	for i, use := range uses {
		id := fmt.Sprintf("term-check:%d", i)
		ann := termAnnotation(use.Rule, cfg.TargetLocale)
		props := map[string]string{"satisfied": strconv.FormatBool(use.Satisfied), "term": use.Rule.Term}
		if use.Rule.ConceptID != "" {
			props["concept_id"] = use.Rule.ConceptID
		}
		if use.Rule.Replacement != "" {
			props["replacement"] = use.Rule.Replacement
		}
		if use.Rule.Advisory {
			props["advisory"] = "true"
		}
		if use.Rule.DoNotTranslate {
			props["do_not_translate"] = "true"
		}
		for _, h := range use.Source {
			src.Spans = append(src.Spans, model.Span{ID: id, Range: model.RangeAnchorForBytes(sourceRuns, h[0], h[1]), Props: props, Value: ann})
		}
		for _, h := range use.Target {
			tgt.Spans = append(tgt.Spans, model.Span{ID: id, Range: model.RangeAnchorForBytes(targetRuns, h[0], h[1]), Props: props, Value: ann})
		}
	}
	out := []model.Overlay{src}
	if len(tgt.Spans) > 0 {
		out = append(out, tgt)
	}
	return out
}

// termAnnotation is a rule as a term annotation: the source term, its concept,
// and the renderings that satisfy it in target. A do-not-translate rule's
// rendering is the term itself.
func termAnnotation(rule profile.TermRule, target model.LocaleID) *model.TermAnnotation {
	ann := &model.TermAnnotation{SourceTerm: rule.Term, ConceptID: rule.ConceptID, Score: 1}
	if rule.DoNotTranslate {
		ann.TargetTerms = []model.TermRef{{Text: rule.Term, Locale: target}}
		return ann
	}
	for i, r := range rule.Renderings() {
		ref := model.TermRef{Text: r.Text, Locale: target}
		if i == 0 {
			ref.Status = model.TermPreferred
		}
		ann.TargetTerms = append(ann.TargetTerms, ref)
	}
	return ann
}

// sourceUse is a rule the source uses, with the text of each place it does.
type sourceUse struct {
	index int
	texts []string
	spans [][2]int
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
		use.spans = append(use.spans, [2]int{s.Start, s.End})
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
	return len(verbatimSpans(target, rule, occurrences)) > 0
}

// verbatimSpans returns the byte ranges where target keeps a do-not-translate
// term as keptVerbatim reads it.
func verbatimSpans(target string, rule profile.TermRule, occurrences []string) [][2]int {
	var hits [][2]int
	for _, s := range append(rule.AllForms(), occurrences...) {
		hits = append(hits, occurrencesOf(target, s, true)...)
	}
	return outermostSpans(hits)
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
	return len(renderingSpans(target, renderings, caseSensitive)) > 0
}

// renderingSpans returns the byte ranges where target contains one of the
// renderings or a declared form of one. The match is containment, as rendered
// decides it, so a compound holding a rendering is a range.
func renderingSpans(target string, renderings []profile.Rendering, caseSensitive bool) [][2]int {
	var hits [][2]int
	for _, r := range renderings {
		hits = append(hits, occurrencesOf(target, r.Text, caseSensitive)...)
		for _, f := range r.Forms {
			hits = append(hits, occurrencesOf(target, f, caseSensitive)...)
		}
	}
	return outermostSpans(hits)
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

// occurrencesOf returns the byte ranges of every occurrence of term in text.
// Without caseSensitive a letter matches any of its cases (strings.EqualFold).
// A blank term occurs nowhere.
func occurrencesOf(text, term string, caseSensitive bool) [][2]int {
	if strings.TrimSpace(term) == "" {
		return nil
	}
	var hits [][2]int
	for i := 0; i+len(term) <= len(text); {
		window := text[i : i+len(term)]
		if window == term || (!caseSensitive && strings.EqualFold(window, term)) {
			hits = append(hits, [2]int{i, i + len(term)})
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
	}
	return hits
}

// outermostSpans sorts ranges by position and drops any range that lies
// inside one already kept, so a declared form and its rendering covering the
// same word are one range.
func outermostSpans(hits [][2]int) [][2]int {
	slices.SortFunc(hits, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return b[1] - a[1]
	})
	var out [][2]int
	for _, h := range hits {
		if n := len(out); n > 0 && h[1] <= out[n-1][1] {
			continue
		}
		out = append(out, h)
	}
	return out
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
