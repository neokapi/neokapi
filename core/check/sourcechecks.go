package check

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// This file hosts the source-side checkers behind `kapi check`. They were
// once registry tools (content-lint, and the source scopes of length-check and
// pattern-check) but are check infrastructure, not user-facing pipeline tools:
// host.ComputeCheck instantiates them directly, so they live here, next to the
// Finding/Report vocabulary they produce.

// The ids of the source-side checkers: the name each records its findings
// under, and the name each is documented by. A finding's stable rule id is
// `<family>.<category>`, and the family is the one the venue attributes the
// checker to (host.collectFileDiagnostics reports the content-lint checker's
// findings under `hygiene`), not the id here.
//
// They are exported because a checker off the registry is still something a user
// meets and something documentation is written for, and this file is the only
// record of which ones exist. [SourceChecks] is the list, with the family and
// the categories each reports; the reference generator holds the authored
// dossiers under `scripts/gen-refs/nativedocs` to it, so retiring a checker
// fails the build until its dossier goes with it, adding one fails until it has
// a dossier, and a dossier names exactly the rule ids the checker reports.
const (
	ContentLintID   = "content-lint"
	SourceLengthID  = "length-check"
	SourcePatternID = "pattern-check"
)

// The families the source-side checkers' findings are reported under: the
// first segment of the rule id a user reads, `hygiene.doubled-word`.
const (
	FamilyHygiene = "hygiene"
	FamilyLength  = "length"
	FamilyPattern = "pattern"
	FamilyComment = "comment"
)

// The categories the content-lint checker reports, under [FamilyHygiene].
const (
	CategoryEmpty              = "empty"
	CategoryLeadingWhitespace  = "leading-whitespace"
	CategoryTrailingWhitespace = "trailing-whitespace"
	CategoryDoubleSpaces       = "double-spaces"
	CategoryDoubledWord        = "doubled-word"
	CategoryControlChar        = "control-char"
)

// The categories the length checker reports, under [FamilyLength].
const (
	CategoryMaxChars = "max-chars-exceeded"
	CategoryMaxWords = "max-words-exceeded"
)

// The categories the pattern checker reports, under [FamilyPattern].
const (
	CategoryForbiddenPattern = "forbidden-pattern"
	CategoryPatternMissing   = "pattern-missing"
)

// SourceCheck is one source-side checker as a reader meets it: the id the code
// registers it under, the family its findings are reported under, and the
// categories it reports. A finding's rule id is `<Family>.<category>`.
type SourceCheck struct {
	ID         string
	Family     string
	Categories []string
}

// RuleIDs returns the rule ids the checker's findings carry, in the order the
// checker reports them.
func (c SourceCheck) RuleIDs() []string {
	out := make([]string, 0, len(c.Categories))
	for _, category := range c.Categories {
		out = append(out, RuleID(c.Family, category))
	}
	return out
}

// SourceChecks returns every source-side checker, in the order `kapi check`
// runs them, with the rule ids each reports.
func SourceChecks() []SourceCheck {
	return []SourceCheck{
		{ID: ContentLintID, Family: FamilyHygiene, Categories: []string{
			CategoryEmpty, CategoryLeadingWhitespace, CategoryTrailingWhitespace,
			CategoryDoubleSpaces, CategoryDoubledWord, CategoryControlChar,
		}},
		{ID: SourceLengthID, Family: FamilyLength, Categories: []string{CategoryMaxChars, CategoryMaxWords}},
		{ID: SourcePatternID, Family: FamilyPattern, Categories: []string{CategoryForbiddenPattern, CategoryPatternMissing}},
		{ID: CommentStyleID, Family: FamilyComment, Categories: []string{CategorySentenceLength, CategoryCommentLength, CategoryCommentDensity}},
	}
}

// SourceCheckIDs returns the ids of every source-side checker, in the order
// `kapi check` runs them.
func SourceCheckIDs() []string {
	checks := SourceChecks()
	ids := make([]string, 0, len(checks))
	for _, c := range checks {
		ids = append(ids, c.ID)
	}
	return ids
}

// NewContentLintTool creates the generic, source-side content-hygiene checker.
// It inspects a single text (the source) with no target or locale comparison
// and records issues as Finding under the unified quality.findings annotation
// (Annotate), where they accumulate alongside other checkers. It is always-on
// in `kapi check`.
func NewContentLintTool() *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        ContentLintID,
		ToolDescription: "Flags text-hygiene issues (empty, double spaces, doubled words, stray whitespace, control chars) in source content",
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}
		isComment := v.Type() == comment.BlockType && v.Property(comment.PropLanguage) != ""
		Annotate(v, ContentLintID, contentLintFindings(HygieneText(v.SourceRuns()), isComment))
		return nil
	}
	return t
}

// contentLintFindings runs the single-text hygiene heuristics over text and
// returns one Finding per issue. Empty/whitespace-only content is the single
// major issue (and short-circuits the rest); the remaining nits are minor.
//
// text must be a [HygieneText] flattening, not a plain SourceText: every
// predicate here is a judgement about the content's shape, which dropped
// inline-code runs distort. isComment selects the double-space rule for a
// comment's text (CommentDoubleSpaces).
func contentLintFindings(text string, isComment bool) []Finding {
	if strings.TrimSpace(text) == "" {
		return []Finding{{
			Category: CategoryEmpty,
			Fails:    true,
			Message:  "Content is empty or whitespace-only",
		}}
	}

	var findings []Finding

	if LeadingWhitespace(text) != "" {
		findings = append(findings, Finding{
			Category: CategoryLeadingWhitespace,
			Message:  "Content has leading whitespace",
		})
	}

	// The stray edge, not the whole one: a single terminating line break belongs
	// to the content the way a line ending does, and a format may require it
	// (see [StrayTrailingWhitespace]). This rule judges one text on its own, so
	// nothing else can tell the two apart for it.
	if StrayTrailingWhitespace(text) != "" {
		findings = append(findings, Finding{
			Category: CategoryTrailingWhitespace,
			Message:  "Content has trailing whitespace",
		})
	}

	// A comment reads a rule that leaves its layout out, so what it reports is a
	// slip the reader sees rather than alignment.
	doubleSpaces := DoubleSpaces
	if isComment {
		doubleSpaces = CommentDoubleSpaces
	}
	if doubleSpaces(text) {
		findings = append(findings, Finding{
			Category: CategoryDoubleSpaces,
			Message:  "Content contains consecutive spaces",
		})
	}

	if word := DoubledWord(text, ""); word != "" {
		findings = append(findings, Finding{
			Category:     CategoryDoubledWord,
			Message:      fmt.Sprintf("Content contains a doubled word: %q", word),
			OriginalText: word,
		})
	}

	if r, ok := firstControlChar(text); ok {
		findings = append(findings, Finding{
			Category: CategoryControlChar,
			Message:  fmt.Sprintf("Content contains a stray control character (U+%04X)", r),
		})
	}

	return findings
}

// NewSourceLengthTool creates the source-scope length checker behind `kapi
// check --max-chars/--max-words`: it validates the source text's absolute
// character and word counts (0 disables a limit). It errors on negative
// limits, mirroring the retired length-check config validation.
func NewSourceLengthTool(maxChars, maxWords int) (*tool.BaseTool, error) {
	if maxChars < 0 {
		return nil, errors.New("length-check: MaxChars must be non-negative")
	}
	if maxWords < 0 {
		return nil, errors.New("length-check: MaxWords must be non-negative")
	}
	t := &tool.BaseTool{
		ToolName:        SourceLengthID,
		ToolDescription: "Verifies source length constraints (chars, words)",
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}
		Annotate(v, SourceLengthID, absoluteLengthFindings(v.SourceText(), "Source", maxChars, maxWords))
		return nil
	}
	return t, nil
}

// absoluteLengthFindings runs the absolute-length checks (max chars, max
// words) over a single text, attributing each finding to subject ("Source").
func absoluteLengthFindings(text, subject string, maxChars, maxWords int) []Finding {
	var findings []Finding
	if maxChars > 0 {
		charCount := len([]rune(text))
		if charCount > maxChars {
			findings = append(findings, Finding{
				Category: CategoryMaxChars,
				Fails:    true,
				Message:  fmt.Sprintf("%s has %d characters, exceeds maximum of %d", subject, charCount, maxChars),
			})
		}
	}
	if maxWords > 0 {
		wordCount := model.CountWords(text)
		if wordCount > maxWords {
			findings = append(findings, Finding{
				Category: CategoryMaxWords,
				Fails:    true,
				Message:  fmt.Sprintf("%s has %d words, exceeds maximum of %d", subject, wordCount, maxWords),
			})
		}
	}
	return findings
}

// PatternRule defines a regex pattern to validate in source content.
type PatternRule struct {
	Name         string // Human-readable name (e.g., "forbidden-1", "required-1")
	Pattern      string // Regex pattern to match (e.g., `(?i)todo`, `&\w+;`)
	MustMatch    bool   // If true, pattern MUST appear in the source
	MustNotMatch bool   // If true, pattern must NOT appear in the source
}

// NewSourcePatternTool creates the source-scope pattern checker behind `kapi
// check --forbid/--require`: forbidden (MustNotMatch) patterns must be absent
// from the source, required (MustMatch) patterns must be present. It errors on
// empty, invalid, or contradictory rules, mirroring the retired pattern-check
// config validation.
func NewSourcePatternTool(rules []PatternRule) (*tool.BaseTool, error) {
	type compiledRule struct {
		PatternRule
		re *regexp.Regexp
	}
	compiled := make([]compiledRule, len(rules))
	for i, rule := range rules {
		if rule.Pattern == "" {
			return nil, fmt.Errorf("pattern-check: Patterns[%d].Pattern is empty", i)
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, fmt.Errorf("pattern-check: Patterns[%d].Pattern is invalid: %w", i, err)
		}
		if rule.MustMatch && rule.MustNotMatch {
			return nil, fmt.Errorf("pattern-check: Patterns[%d] cannot have both MustMatch and MustNotMatch", i)
		}
		compiled[i] = compiledRule{PatternRule: rule, re: re}
	}

	t := &tool.BaseTool{
		ToolName:        SourcePatternID,
		ToolDescription: "Validates forbidden and required regex patterns in source content",
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}
		text := v.SourceText()
		var findings []Finding
		for _, rule := range compiled {
			if rule.MustMatch && !rule.re.MatchString(text) {
				findings = append(findings, Finding{
					Category: CategoryPatternMissing,
					Fails:    true,
					Message: fmt.Sprintf("Pattern %q (%s): required pattern not found in source",
						rule.Name, rule.Pattern),
				})
			}
			if rule.MustNotMatch {
				if loc := rule.re.FindString(text); loc != "" {
					findings = append(findings, Finding{
						Category: CategoryForbiddenPattern,
						Fails:    true,
						Message: fmt.Sprintf("Pattern %q (%s): forbidden pattern found in source",
							rule.Name, rule.Pattern),
						OriginalText: loc,
					})
				}
			}
		}
		Annotate(v, SourcePatternID, findings)
		return nil
	}
	return t, nil
}

// HygieneCanaries is the known-bad input the content-hygiene checker must flag.
func HygieneCanaries() []Canary {
	return []Canary{
		{Name: "doubled word", Block: CanaryBlock("A canary with a doubled doubled word."), Expect: CategoryDoubledWord},
		{
			Name:   "double space in a comment",
			Block:  commentCanary("func/Canary", true, "A canary comment with two  spaces.\nIts next line holds no column."),
			Expect: CategoryDoubleSpaces,
		},
	}
}

// LengthCanaries returns, for each limit that is set, text one past it.
func LengthCanaries(maxChars, maxWords int) []Canary {
	var out []Canary
	if maxChars > 0 {
		out = append(out, Canary{
			Name:   fmt.Sprintf("%d characters", maxChars+1),
			Block:  CanaryBlock(strings.Repeat("x", maxChars+1)),
			Expect: CategoryMaxChars,
		})
	}
	if maxWords > 0 {
		out = append(out, Canary{
			Name:   fmt.Sprintf("%d words", maxWords+1),
			Block:  CanaryBlock(strings.TrimSpace(strings.Repeat("w ", maxWords+1))),
			Expect: CategoryMaxWords,
		})
	}
	return out
}

// PatternCanaries returns a canary for each rule: text a forbidden pattern
// matches, or text a required pattern does not. A rule that no text can violate
// (a forbidden pattern that only matches the empty string, a required one such
// as `.*`) checks nothing, and because each rule was asked for explicitly, the
// whole set is then reported as uncheckable rather than partly probed.
func PatternCanaries(rules []PatternRule) ([]Canary, string) {
	var out []Canary
	for _, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, fmt.Sprintf("pattern %q does not compile", rule.Pattern)
		}
		switch {
		case rule.MustNotMatch:
			text, ok := TextMatching(re)
			if !ok {
				return nil, fmt.Sprintf("no text can contain the forbidden pattern %q", rule.Pattern)
			}
			out = append(out, Canary{Name: "forbidden " + rule.Name, Block: CanaryBlock(text), Expect: CategoryForbiddenPattern})
		case rule.MustMatch:
			text, ok := TextNotMatching(re)
			if !ok {
				return nil, fmt.Sprintf("every text satisfies the required pattern %q", rule.Pattern)
			}
			out = append(out, Canary{Name: "required " + rule.Name, Block: CanaryBlock(text), Expect: CategoryPatternMissing})
		}
	}
	return out, ""
}

// firstControlChar returns the first non-whitespace control character in s.
// Tab, newline, and carriage return are treated as ordinary whitespace, so only
// stray control codes (e.g. NUL, BEL, ESC) are reported.
func firstControlChar(s string) (rune, bool) {
	for _, r := range s {
		switch r {
		case '\t', '\n', '\r':
			continue
		}
		if unicode.IsControl(r) {
			return r, true
		}
	}
	return 0, false
}
