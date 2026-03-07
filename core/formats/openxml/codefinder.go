package openxml

import (
	"fmt"
	"regexp"

	"github.com/gokapi/gokapi/core/model"
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

// apply scans a Fragment's plain text for regex matches and wraps them as
// placeholder spans. Only works on fragments without existing spans (code
// finder applies to raw text before formatting spans are added in Okapi's model,
// but here we apply it after since our Fragment is already built).
func (cf *codeFinder) apply(frag *model.Fragment, spanCounter *int) {
	if len(cf.patterns) == 0 || frag == nil {
		return
	}

	// Only apply to plain text fragments (no existing spans)
	if frag.HasSpans() {
		return
	}

	text := frag.CodedText

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
		return
	}

	// Sort by position, remove overlaps (first match wins)
	sortCodeMatches(matches)
	matches = removeCodeOverlaps(matches)

	// Rebuild the fragment with placeholder spans at match positions
	newFrag := &model.Fragment{}
	pos := 0

	for _, m := range matches {
		// Add text before this match
		if pos < m.start {
			newFrag.AppendText(text[pos:m.start])
		}

		// Add placeholder span for the match
		*spanCounter++
		newFrag.AppendSpan(&model.Span{
			SpanType:  model.SpanPlaceholder,
			Type:      "fmt:code",
			SubType:   "openxml:codeFinder",
			ID:        fmt.Sprintf("c%d", *spanCounter),
			Data:      m.text,
			EquivText: m.text,
			Deletable: false,
		})

		pos = m.end
	}

	// Add remaining text
	if pos < len(text) {
		newFrag.AppendText(text[pos:])
	}

	// Replace the original fragment's content
	frag.CodedText = newFrag.CodedText
	frag.Spans = newFrag.Spans
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
