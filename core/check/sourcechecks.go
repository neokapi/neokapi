package check

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/tool"
)

// This file hosts the source-side checkers behind `kapi check`. They were
// once registry tools (content-lint, and the source scopes of length-check and
// pattern-check) but are check infrastructure, not user-facing pipeline tools:
// host.ComputeCheck instantiates them directly, so they live here, next to the
// Finding/Report vocabulary they produce.

// NewContentLintTool creates the generic, source-side content-hygiene checker.
// It inspects a single text (the source) with no target or locale comparison
// and records issues as Finding under the unified quality.findings annotation
// (Annotate), where they accumulate alongside other checkers. It is always-on
// in `kapi check`.
func NewContentLintTool() *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "content-lint",
		ToolDescription: "Flags text-hygiene issues (empty, double spaces, doubled words, stray whitespace, control chars) in source content",
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}
		Annotate(v, "content-lint", contentLintFindings(HygieneText(v.SourceRuns())))
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
// inline-code runs distort.
func contentLintFindings(text string) []Finding {
	if strings.TrimSpace(text) == "" {
		return []Finding{{
			Category: "empty",
			Severity: SeverityMajor,
			Message:  "Content is empty or whitespace-only",
		}}
	}

	var findings []Finding

	if LeadingWhitespace(text) != "" {
		findings = append(findings, Finding{
			Category: "leading-whitespace",
			Severity: SeverityMinor,
			Message:  "Content has leading whitespace",
		})
	}

	if TrailingWhitespace(text) != "" {
		findings = append(findings, Finding{
			Category: "trailing-whitespace",
			Severity: SeverityMinor,
			Message:  "Content has trailing whitespace",
		})
	}

	if DoubleSpaces(text) {
		findings = append(findings, Finding{
			Category: "double-spaces",
			Severity: SeverityMinor,
			Message:  "Content contains consecutive spaces",
		})
	}

	if word := DoubledWord(text, ""); word != "" {
		findings = append(findings, Finding{
			Category:     "doubled-word",
			Severity:     SeverityMinor,
			Message:      fmt.Sprintf("Content contains a doubled word: %q", word),
			OriginalText: word,
		})
	}

	if r, ok := firstControlChar(text); ok {
		findings = append(findings, Finding{
			Category: "control-char",
			Severity: SeverityMinor,
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
		ToolName:        "length-check",
		ToolDescription: "Verifies source length constraints (chars, words)",
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}
		Annotate(v, "length-check", absoluteLengthFindings(v.SourceText(), "Source", maxChars, maxWords))
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
				Category: "max-chars-exceeded",
				Severity: SeverityMajor,
				Message:  fmt.Sprintf("%s has %d characters, exceeds maximum of %d", subject, charCount, maxChars),
			})
		}
	}
	if maxWords > 0 {
		wordCount := countWords(text)
		if wordCount > maxWords {
			findings = append(findings, Finding{
				Category: "max-words-exceeded",
				Severity: SeverityMajor,
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
		ToolName:        "pattern-check",
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
					Category: "pattern-missing",
					Severity: SeverityMajor,
					Message: fmt.Sprintf("Pattern %q (%s): required pattern not found in source",
						rule.Name, rule.Pattern),
				})
			}
			if rule.MustNotMatch {
				if loc := rule.re.FindString(text); loc != "" {
					findings = append(findings, Finding{
						Category: "forbidden-pattern",
						Severity: SeverityMajor,
						Message: fmt.Sprintf("Pattern %q (%s): forbidden pattern found in source",
							rule.Name, rule.Pattern),
						OriginalText: loc,
					})
				}
			}
		}
		Annotate(v, "pattern-check", findings)
		return nil
	}
	return t, nil
}

// countWords counts the number of words in a text string. Words are sequences
// of non-whitespace characters separated by whitespace.
func countWords(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
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
