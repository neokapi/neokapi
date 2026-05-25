package srx

import (
	"context"
	_ "embed"
	"fmt"
	"sort"

	"github.com/dlclark/regexp2"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
)

//go:embed default.srx
var defaultSRX []byte

func init() {
	segment.RegisterEngine(segment.DefaultEngine, New)
}

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

// New builds the SRX engine. It resolves the ruleset in precedence order:
// inline SrxRules, then SrxPath, then the embedded default ruleset.
func New(cfg segment.Config) (segment.Segmenter, error) {
	var (
		doc *Document
		err error
	)
	switch {
	case cfg.SrxRules != "":
		doc, err = Parse([]byte(cfg.SrxRules))
	case cfg.SrxPath != "":
		doc, err = ParseFile(cfg.SrxPath)
	default:
		doc, err = Parse(defaultSRX)
	}
	if err != nil {
		return nil, err
	}
	return &segmenter{doc: doc, lang: cfg.Language, mask: cfg.Mask}, nil
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
	fl := segment.Flatten(runs, s.mask)
	text := fl.Runes()
	if len(text) == 0 {
		return nil, nil
	}

	rules, err := s.rulesFor(locale)
	if err != nil {
		return nil, err
	}

	breaks, err := applyRules(ctx, rules, text)
	if err != nil {
		return nil, err
	}

	return fl.Spans(breaks), nil
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

// applyRules runs the ordered rules over text and returns the sorted rune
// offsets at which a new segment begins.
//
// For each rule in order it finds every match; the candidate boundary for a
// match is the end of the before-break group (where the after-break lookahead
// begins). The first rule to decide a position wins: a no-break rule placed
// before a later break rule suppresses the split at that position, which is how
// SRX exceptions (abbreviations, decimals, initials) and the cascade resolve
// conflicts. Final breaks are the positions decided true.
func applyRules(ctx context.Context, rules []compiledRule, text []rune) ([]int, error) {
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

	breaks := make([]int, 0, len(decided))
	for pos, isBreak := range decided {
		if isBreak {
			breaks = append(breaks, pos)
		}
	}
	sort.Ints(breaks)
	return breaks, nil
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
