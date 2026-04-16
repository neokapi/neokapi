package openxml

import (
	"fmt"
	"regexp"

	"github.com/neokapi/neokapi/core/model"
)

// codeFinder wraps text matching regex patterns as placeholder spans.
// This provides parity with Okapi's GenericContent.codeFinder mechanism.
type codeFinder struct {
	patterns []*regexp.Regexp
}

// codeMatch represents a regex match position in text.
type codeMatch struct {
	start, end int
	text       string
}

// newCodeFinder compiles regex patterns for inline code detection.
func newCodeFinder(rules []string) (*codeFinder, error) {
	cf := &codeFinder{}
	for _, rule := range rules {
		re, err := regexp.Compile(rule)
		if err != nil {
			return nil, fmt.Errorf("openxml: invalid code finder rule %q: %w", rule, err)
		}
		cf.patterns = append(cf.patterns, re)
	}
	return cf, nil
}

// applyToRuns scans a plain-text run sequence for regex matches and
// wraps them as placeholder runs. Only applies when the sequence is a
// single TextRun (no inline codes) — mirroring the original Fragment
// behaviour that skipped fragments with spans.
//
// Returns the (possibly rewritten) run slice. If no patterns match, or
// the sequence contains anything other than a single TextRun, the
// input is returned unchanged.
func (cf *codeFinder) applyToRuns(runs []model.Run, spanCounter *int) []model.Run {
	if len(cf.patterns) == 0 {
		return runs
	}

	// Only apply when the run sequence is a single TextRun. Matches
	// the original Fragment check (no existing spans).
	if len(runs) != 1 || runs[0].Text == nil {
		return runs
	}

	text := runs[0].Text.Text

	// Find all matches across all patterns
	var matches []codeMatch
	for _, re := range cf.patterns {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			matches = append(matches, codeMatch{
				start: loc[0],
				end:   loc[1],
				text:  text[loc[0]:loc[1]],
			})
		}
	}

	if len(matches) == 0 {
		return runs
	}

	// Sort by position, remove overlaps (first match wins)
	sortCodeMatches(matches)
	matches = removeCodeOverlaps(matches)

	// Rebuild the run sequence with placeholder runs at match
	// positions, using a runBuilder so adjacent text chunks coalesce.
	b := &runBuilder{}
	pos := 0

	for _, m := range matches {
		if pos < m.start {
			b.AddText(text[pos:m.start])
		}

		*spanCounter++
		b.AddPh(fmt.Sprintf("c%d", *spanCounter),
			"fmt:code", "openxml:codeFinder",
			m.text, m.text, "",
			false, false, false)

		pos = m.end
	}

	if pos < len(text) {
		b.AddText(text[pos:])
	}

	return b.Runs()
}

// sortCodeMatches sorts matches by start position using insertion sort.
func sortCodeMatches(matches []codeMatch) {
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}
}

// removeCodeOverlaps removes matches that overlap with earlier ones.
func removeCodeOverlaps(matches []codeMatch) []codeMatch {
	if len(matches) <= 1 {
		return matches
	}
	result := []codeMatch{matches[0]}
	for i := 1; i < len(matches); i++ {
		if matches[i].start >= result[len(result)-1].end {
			result = append(result, matches[i])
		}
	}
	return result
}
