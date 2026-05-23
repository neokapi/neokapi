package redaction

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Rule declares one sensitive-content matcher. Exactly one of Term or
// Pattern must be set: Term matches a literal string, Pattern a regular
// expression (RE2 syntax). Both compile to a regexp internally so byte
// offsets are always correct, including under case-insensitive matching.
type Rule struct {
	Term     string   `yaml:"term,omitempty" json:"term,omitempty"`
	Pattern  string   `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Category string   `yaml:"category" json:"category"`
	Flags    []string `yaml:"flags,omitempty" json:"flags,omitempty"`
}

// Supported rule flags.
const (
	FlagIgnoreCase = "ignorecase"
	FlagWholeWord  = "wholeword" // only meaningful for Term rules
)

type compiledRule struct {
	re       *regexp.Regexp
	category string
}

// RuleDetector matches declared literal terms and regular expressions. It is
// fully offline and deterministic — the default detector, and the only one
// that preserves the locality guarantee without qualification.
type RuleDetector struct {
	rules []compiledRule
}

// NewRuleDetector compiles a rule set into a detector. Rules with an empty
// category, or with neither/both of Term and Pattern set, are rejected.
func NewRuleDetector(rules []Rule) (*RuleDetector, error) {
	compiled := make([]compiledRule, 0, len(rules))
	for i, r := range rules {
		if strings.TrimSpace(r.Category) == "" {
			return nil, fmt.Errorf("redaction rule %d: category is required", i)
		}
		hasTerm := r.Term != ""
		hasPattern := r.Pattern != ""
		if hasTerm == hasPattern {
			return nil, fmt.Errorf("redaction rule %d (%q): set exactly one of term or pattern", i, r.Category)
		}

		var expr string
		if hasTerm {
			expr = regexp.QuoteMeta(r.Term)
			if hasFlag(r.Flags, FlagWholeWord) {
				expr = `\b` + expr + `\b`
			}
		} else {
			expr = r.Pattern
		}
		if hasFlag(r.Flags, FlagIgnoreCase) {
			expr = "(?i)" + expr
		}

		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("redaction rule %d (%q): %w", i, r.Category, err)
		}
		compiled = append(compiled, compiledRule{re: re, category: r.Category})
	}
	return &RuleDetector{rules: compiled}, nil
}

// Name identifies this detector.
func (d *RuleDetector) Name() string { return "rules" }

// Detect returns every rule match in text. Results are normalized
// (sorted, non-overlapping). The locale is accepted for interface symmetry
// but unused — rule matching is locale-independent.
func (d *RuleDetector) Detect(_ context.Context, text string, _ model.LocaleID) ([]Match, error) {
	if text == "" {
		return nil, nil
	}
	var matches []Match
	for _, cr := range d.rules {
		for _, loc := range cr.re.FindAllStringIndex(text, -1) {
			start, end := loc[0], loc[1]
			if end <= start {
				continue
			}
			matches = append(matches, Match{
				Start:    start,
				End:      end,
				Category: cr.category,
				Original: text[start:end],
			})
		}
	}
	return NormalizeMatches(matches), nil
}

func hasFlag(flags []string, want string) bool {
	for _, f := range flags {
		if strings.EqualFold(strings.TrimSpace(f), want) {
			return true
		}
	}
	return false
}
