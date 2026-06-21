package srx

import (
	"context"
	_ "embed"
	"fmt"
	"slices"
	"unicode"

	"github.com/dlclark/regexp2"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/segment"
)

// default.srx is a reduced, self-contained pure-Go ruleset (explicit break rules
// + common exceptions) used wherever no UAX-29 base breaker is available
// (nocgo/wasm) — it does not rely on ICU for breaking.
//
//go:embed default.srx
var defaultSRX []byte

// okapi.srx is Okapi's full defaultSegmentation.srx (Apache-2.0,
// net.sf.okapi.lib.segmentation; derived in part from LanguageTool's
// segment.srx). It declares useIcu4jBreakRules="yes": ICU/UAX-29 does the
// breaking and these rules are ~2,800 no-break exceptions across 14 languages.
// It is the default only when a base breaker is linked (cgo/ICU), where it
// reproduces Okapi's segmentation; without a base breaker it would
// under-segment, so default.srx is used instead.
//
//go:embed okapi.srx
var okapiSRX []byte

func init() {
	segment.Register(segment.EngineDescriptor{
		Name:        segment.DefaultEngine, // "srx"
		Label:       "Rule-based (hybrid SRX/UAX-29)",
		Description: "Faithful sentence segmentation: ICU UAX-29 boundaries refined by Okapi SRX exceptions where ICU is available, pure-Go SRX rules otherwise. No configuration required.",
		Order:       0,
		Schema:      schema.FromStruct(&Params{}, schema.ToolMeta{ID: "segment-engine-srx"}),
		New: func(base segment.BaseConfig, params map[string]any) (segment.Segmenter, error) {
			p := &Params{}
			if err := schema.ApplyConfig(params, p); err != nil {
				return nil, fmt.Errorf("srx config: %w", err)
			}
			return New(base, p)
		},
	})
}

// Params is the SRX engine's own configuration. Both fields are optional: with
// neither set the engine uses its built-in ruleset (Okapi's full hybrid set when
// an ICU base breaker is linked, the reduced pure-Go set otherwise).
type Params struct {
	// RulesPath points at a custom SRX 2.0 rules file, overriding the built-in
	// ruleset.
	RulesPath string `json:"rulesPath,omitempty" schema:"title=SRX Rules File,description=Path to a custom SRX 2.0 rules file (overrides the built-in ruleset),widget=file-picker,order=10"`
	// RulesXML is inline SRX 2.0 XML, taking precedence over RulesPath. It is not
	// a form field — it backs programmatic callers (the docs lab, tests) that
	// embed a ruleset directly.
	RulesXML string `json:"-" schema:"-"`
}

// DefaultRuleset returns the embedded reduced, self-contained pure-Go SRX
// ruleset (explicit break rules, no ICU base). Pass it as [Params.RulesXML]
// to force pure-rule segmentation regardless of whether a base breaker is linked
// — used by the docs lab to show the rule-based engine distinctly from the hybrid.
func DefaultRuleset() []byte { return defaultSRX }

// OkapiRuleset returns the embedded full Okapi defaultSegmentation.srx (declares
// useIcu4jBreakRules: a UAX-29 base breaker supplies the breaks and these rules
// apply as exceptions). Pass it as [Params.RulesXML] to run the Okapi hybrid
// where a base breaker is available.
func OkapiRuleset() []byte { return okapiSRX }

// regexp2 options used for SRX rules. SRX 2.0 regexes assume Unicode semantics
// and "." matching anything but a line break; Multiline lets ^/$ anchor to
// embedded line breaks the way ICU's segmenter context does. RE2 disables some
// .NET-only constructs that SRX rulesets do not use, keeping behaviour close to
// the ICU/Java regex engines Okapi targets.
const ruleRegexOpts = regexp2.RE2 | regexp2.Multiline

// compiledRule is a Rule with its before/after patterns compiled into a single
// regexp2 program. The candidate boundary is the end of the before-break group
// (group "bb"); the after-break is a zero-width lookahead so the boundary lands
// exactly between them.
type compiledRule struct {
	doBreak bool
	re      *regexp2.Regexp
}

// segmenter is a compiled SRX engine for a parsed Document. Rule selection per
// locale is cached so repeated Segment calls for the same locale skip the
// language-map walk and recompilation.
type segmenter struct {
	doc  *Document
	lang string              // Config.Language override; "" = use per-call locale
	mask segment.MaskOptions // how to flatten codes and trim segment edges

	cache rulesCache
}

// New builds the SRX engine from the shared base options and SRX [Params]
// (nil = defaults). It resolves the ruleset in precedence order: inline
// RulesXML, then RulesPath, then the embedded default ruleset.
func New(base segment.BaseConfig, p *Params) (segment.Segmenter, error) {
	if p == nil {
		p = &Params{}
	}
	var (
		doc *Document
		err error
	)
	switch {
	case p.RulesXML != "":
		doc, err = Parse([]byte(p.RulesXML))
	case p.RulesPath != "":
		doc, err = ParseFile(p.RulesPath)
	case segment.HasBaseBreaker():
		// ICU is linked: default to Okapi's full hybrid ruleset (ICU base +
		// SRX exceptions) for Okapi-grade segmentation.
		doc, err = Parse(okapiSRX)
	default:
		// No base breaker (nocgo/wasm): use the self-contained pure-rule set.
		doc, err = Parse(defaultSRX)
	}
	if err != nil {
		return nil, err
	}
	return &segmenter{doc: doc, lang: base.Language, mask: base.Mask}, nil
}

// Layer reports that this engine produces the primary sentence segmentation.
func (s *segmenter) Layer() string { return segment.LayerSentence }

// Segment computes sentence boundaries over runs in the given locale. The
// Config.Language override (if set at construction) takes precedence over the
// per-call locale for language-map selection.
func (s *segmenter) Segment(ctx context.Context, runs []model.Run, loc model.LocaleID) ([]model.Span, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	locale := s.lang
	if locale == "" {
		locale = string(loc)
	}

	// Flatten the runs under the configured mask options. The engine works
	// purely over the masked text and computes rune-offset breaks; Flattened.
	// Spans then projects those offsets back to run-anchored spans, applying
	// the same mask options' edge trimming. SRX places a boundary AFTER the
	// sentence-final punctuation, so inter-segment whitespace leads the next
	// segment unless TrimLeading/TrailingWS moves it out as an ignorable.
	// Honor the SRX header's trim options (Okapi's okpsrx:options) on top of any
	// caller mask, so a ruleset that asks to trim (okapi.srx does) produces clean
	// sentence spans with inter-sentence whitespace left uncovered.
	mask := s.mask
	if s.doc.TrimLeadingWS {
		mask.TrimLeadingWS = true
	}
	if s.doc.TrimTrailingWS {
		mask.TrimTrailingWS = true
	}
	fl := segment.Flatten(runs, mask)
	text := fl.Runes()
	if len(text) == 0 {
		return nil, nil
	}

	rules, err := s.rulesFor(locale)
	if err != nil {
		return nil, err
	}

	breaks, err := s.computeBreaks(ctx, rules, text, locale)
	if err != nil {
		return nil, err
	}

	return fl.Spans(breaks), nil
}

// computeBreaks returns the interior break offsets, taking the Okapi
// `useIcu4jBreakRules` hybrid path when the ruleset requests it and a UAX-29
// base breaker is linked: ICU supplies the candidate breaks (adjusted back over
// trailing whitespace, as Okapi does) and the SRX rules are applied on top as
// exceptions. Without a base breaker it falls back to pure-rule breaking.
func (s *segmenter) computeBreaks(ctx context.Context, rules []compiledRule, text []rune, locale string) ([]int, error) {
	var (
		breaks []int
		err    error
	)
	if s.doc.UseICUBreakRules {
		// Hybrid path: ICU/UAX-29 base + SRX exceptions. If no base breaker is
		// linked (ok=false) OR the base breaker fails (e.g. the browser ICU4X
		// bridge isn't loaded yet, berr != nil), gracefully fall back to pure-rule
		// breaking rather than erroring — the ruleset's own rules still segment.
		if base, ok, berr := segment.BaseBreaks(ctx, text, locale); ok && berr == nil {
			breaks, err = applyRulesWithBase(ctx, rules, text, adjustICUBreaks(text, base))
		} else {
			breaks, err = applyRules(ctx, rules, text)
		}
	} else {
		breaks, err = applyRules(ctx, rules, text)
	}
	if err != nil {
		return nil, err
	}
	return collapseWhitespaceOnly(text, breaks), nil
}

// collapseWhitespaceOnly drops a boundary when the segment it would open is
// preceded by whitespace-only content since the last kept boundary — i.e. it
// removes degenerate segments that consist solely of inter-sentence whitespace
// (the whitespace instead leads the following segment, per the overlay model).
// In practice this only fires when two boundaries land a whitespace run apart,
// which an isolated code masked as a space, or an ICU/SRX break disagreeing by a
// space, can produce. It never merges two real (non-whitespace) sentences.
func collapseWhitespaceOnly(text []rune, breaks []int) []int {
	if len(breaks) == 0 {
		return breaks
	}
	out := breaks[:0]
	last := 0
	for _, b := range breaks {
		if allWhitespace(text[last:b]) {
			continue // segment [last,b) is whitespace-only — drop this boundary
		}
		out = append(out, b)
		last = b
	}
	return out
}

func allWhitespace(rs []rune) bool {
	for _, r := range rs {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// Breaks computes the sentence-break rune offsets over masked text in a locale,
// without run projection. It is the pure boundary kernel the segment tool can
// call when it has already built its own Flattened view, and it is what the
// engine tests assert on directly.
func (s *segmenter) Breaks(ctx context.Context, text []rune, locale string) ([]int, error) {
	if len(text) == 0 {
		return nil, nil
	}
	if s.lang != "" {
		locale = s.lang
	}
	rules, err := s.rulesFor(locale)
	if err != nil {
		return nil, err
	}
	return applyRules(ctx, rules, text)
}

// decide runs the ordered rules over text and records, per interior position,
// whether the first rule to match there voted to break. The first rule to
// decide a position wins: a no-break rule placed before a later break rule
// suppresses the split at that position, which is how SRX exceptions
// (abbreviations, decimals, initials) and the cascade resolve conflicts.
func decide(ctx context.Context, rules []compiledRule, text []rune) (map[int]bool, error) {
	s := string(text)
	decided := make(map[int]bool) // position -> isBreak; first writer wins

	for _, r := range rules {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m, err := r.re.FindStringMatch(s)
		if err != nil {
			return nil, fmt.Errorf("srx: match: %w", err)
		}
		for m != nil {
			pos := boundaryPos(m)
			if pos > 0 && pos < len(text) {
				if _, seen := decided[pos]; !seen {
					decided[pos] = r.doBreak
				}
			}
			m, err = r.re.FindNextMatch(m)
			if err != nil {
				return nil, fmt.Errorf("srx: next match: %w", err)
			}
		}
	}
	return decided, nil
}

// collectBreaks returns the sorted positions decided true.
func collectBreaks(decided map[int]bool) []int {
	breaks := make([]int, 0, len(decided))
	for pos, isBreak := range decided {
		if isBreak {
			breaks = append(breaks, pos)
		}
	}
	slices.Sort(breaks)
	return breaks
}

// applyRules runs the ordered SRX rules and returns the break offsets
// (pure-rule mode: the ruleset's own break rules produce the breaks).
func applyRules(ctx context.Context, rules []compiledRule, text []rune) ([]int, error) {
	decided, err := decide(ctx, rules, text)
	if err != nil {
		return nil, err
	}
	return collectBreaks(decided), nil
}

// applyRulesWithBase reproduces Okapi's useIcu4jBreakRules hybrid: the SRX rules
// are applied first (first-writer-wins), then the base (ICU/UAX-29) breaks are
// added last — a base break takes effect only where no earlier SRX rule already
// decided that position, so SRX no-break exceptions override ICU and SRX break
// rules add splits. (Okapi appends the ICU breaks as a synthetic rule last.)
func applyRulesWithBase(ctx context.Context, rules []compiledRule, text []rune, base []int) ([]int, error) {
	decided, err := decide(ctx, rules, text)
	if err != nil {
		return nil, err
	}
	n := len(text)
	for _, pos := range base {
		if pos > 0 && pos < n {
			if _, seen := decided[pos]; !seen {
				decided[pos] = true
			}
		}
	}
	return collectBreaks(decided), nil
}

// adjustICUBreaks reproduces Okapi's boundary fix-up for ICU breaks: ICU places
// a sentence boundary after the inter-sentence whitespace (just before the next
// sentence's first character), but SRX semantics put the break right after the
// sentence-final punctuation, so each base boundary is moved back over any
// preceding whitespace. Out-of-range and duplicate results are dropped.
func adjustICUBreaks(text []rune, base []int) []int {
	out := make([]int, 0, len(base))
	seen := make(map[int]bool, len(base))
	for _, b := range base {
		for b-1 > 0 && unicode.IsSpace(text[b-1]) {
			b--
		}
		if b > 0 && b < len(text) && !seen[b] {
			seen[b] = true
			out = append(out, b)
		}
	}
	return out
}

// boundaryPos returns the rune offset of the candidate boundary for a match:
// the end of the before-break group "bb" when present, else the end of the
// whole match (an empty before-break means "break right here"). regexp2 reports
// group Index/Length in rune units, so these are already rune offsets into the
// masked text.
func boundaryPos(m *regexp2.Match) int {
	if g := m.GroupByName("bb"); g != nil && g.Length >= 0 && len(g.Captures) > 0 {
		c := g.Captures[len(g.Captures)-1]
		return c.Index + c.Length
	}
	return m.Index + m.Length
}

// rulesFor selects and compiles the rule list for a locale, cached per locale.
func (s *segmenter) rulesFor(locale string) ([]compiledRule, error) {
	return s.cache.get(locale, func() ([]compiledRule, error) {
		return s.compileRulesFor(locale)
	})
}

// compileRulesFor walks the language maps to select the language rules for a
// locale and compiles their rules in order.
//
// With cascade (the SRX 2.0 default), the rules of EVERY language map whose
// pattern matches the locale are applied, in map order. Without cascade, only
// the first matching map's language rule is used.
func (s *segmenter) compileRulesFor(locale string) ([]compiledRule, error) {
	var selected []*LanguageRule
	for i := range s.doc.LanguageMaps {
		lm := s.doc.LanguageMaps[i]
		matched, err := localePatternMatches(lm.LanguagePattern, locale)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		lr := s.doc.languageRule(lm.LanguageRule)
		if lr == nil {
			return nil, fmt.Errorf("srx: language map references unknown rule %q", lm.LanguageRule)
		}
		selected = append(selected, lr)
		if !s.doc.Cascade {
			break
		}
	}

	var compiled []compiledRule
	for _, lr := range selected {
		for _, r := range lr.Rules {
			cr, err := compileRule(r)
			if err != nil {
				return nil, err
			}
			compiled = append(compiled, cr)
		}
	}
	return compiled, nil
}

// compileRule builds a single regexp2 program for a rule. The before-break is
// captured in a named group "bb" so the boundary can be read at its end; the
// after-break is a zero-width lookahead so the boundary sits exactly between
// the two contexts and matches do not consume the after-break text (which lets
// overlapping candidates at adjacent positions all be considered).
func compileRule(r Rule) (compiledRule, error) {
	pattern := "(?<bb>" + r.BeforeBreak + ")"
	if r.AfterBreak != "" {
		pattern += "(?=" + r.AfterBreak + ")"
	}
	re, err := regexp2.Compile(pattern, ruleRegexOpts)
	if err != nil {
		return compiledRule{}, fmt.Errorf("srx: compile rule (before=%q after=%q): %w", r.BeforeBreak, r.AfterBreak, err)
	}
	return compiledRule{doBreak: r.Break, re: re}, nil
}

// localePatternMatches reports whether an SRX language-map pattern (a regex
// over the BCP-47 tag) matches the locale. SRX patterns are case-insensitive in
// practice (e.g. "EN.*" should match "en-US"), matching Okapi's behaviour.
func localePatternMatches(pattern, locale string) (bool, error) {
	re, err := regexp2.Compile(pattern, regexp2.IgnoreCase)
	if err != nil {
		return false, fmt.Errorf("srx: compile language pattern %q: %w", pattern, err)
	}
	ok, err := re.MatchString(locale)
	if err != nil {
		return false, fmt.Errorf("srx: match language pattern %q: %w", pattern, err)
	}
	return ok, nil
}
