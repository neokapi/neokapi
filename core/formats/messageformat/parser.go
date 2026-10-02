package messageformat

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/icu"
	"github.com/neokapi/neokapi/core/model"
)

// The ICU MessageFormat syntax itself is read by core/icu; this file turns the
// nodes it returns into the translatable segments the reader extracts.

// node names the shared parse tree locally, so the extraction below reads as
// one vocabulary with the reader that consumes it.
type node = icu.Node

// errPrefix is the error prefix used to match the Okapi bridge's error messages.
// The capitalization matches Okapi Framework's convention.
const errPrefix = "Error reading Message Format String"

// parse parses an ICU MessageFormat pattern string into a list of nodes.
func parse(pattern string) ([]node, error) {
	nodes, err := icu.Parse(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errPrefix, err)
	}
	return nodes, nil
}

// segment represents a translatable segment extracted from the pattern.
type segment struct {
	// start and end are the byte offsets in the original pattern of the slot
	// the segment's block is written into: a leaf branch body, or the whole
	// line when the line has no picker. trim narrows them past the whitespace
	// at the slot's edges, and lead and trail record how much it removed.
	start, end  int
	lead, trail int
	path        string // Dot-delimited path (e.g., "count.one", "gender.male.count.other")
	text        string // The plain text content of this segment, edges included
	hash        bool   // Whether the segment contains # references
}

// trim narrows the slot past the whitespace at its edges, which stays in the
// file around the block. The block's content is trimmed by the same amounts
// (trimmedText, trimRuns), so an edited value is written between that
// whitespace instead of replacing it.
func (s *segment) trim(raw string) {
	body := raw[s.start:s.end]
	s.lead = len(body) - len(strings.TrimLeftFunc(body, unicode.IsSpace))
	if s.lead == len(body) {
		return
	}
	s.trail = len(body) - len(strings.TrimRightFunc(body, unicode.IsSpace))
	s.start += s.lead
	s.end -= s.trail
}

// trimmedText returns the segment's text without the whitespace trim moved
// into the skeleton.
func (s segment) trimmedText() string {
	return trimEdges(s.text, s.lead, s.trail)
}

// trimRuns removes the whitespace trim moved into the skeleton from the first
// and last runs, dropping a run it empties.
func (s segment) trimRuns(runs []model.Run) []model.Run {
	if len(runs) > 0 && runs[0].Text != nil && s.lead > 0 {
		runs[0].Text = &model.TextRun{Text: trimEdges(runs[0].Text.Text, s.lead, 0)}
		if runs[0].Text.Text == "" {
			runs = runs[1:]
		}
	}
	if n := len(runs); n > 0 && runs[n-1].Text != nil && s.trail > 0 {
		runs[n-1].Text = &model.TextRun{Text: trimEdges(runs[n-1].Text.Text, 0, s.trail)}
		if runs[n-1].Text.Text == "" {
			runs = runs[:n-1]
		}
	}
	return runs
}

// trimEdges removes up to lead bytes of leading and trail bytes of trailing
// whitespace from s, and never anything else.
func trimEdges(s string, lead, trail int) string {
	lead = min(lead, len(s)-len(strings.TrimLeftFunc(s, unicode.IsSpace)))
	s = s[lead:]
	trail = min(trail, len(s)-len(strings.TrimRightFunc(s, unicode.IsSpace)))
	return s[:len(s)-trail]
}

// extractSegments recursively extracts translatable segments from parsed nodes.
func extractSegments(nodes []node, pathPrefix string) []segment {
	var segments []segment

	// Check if this node list contains any plural/select patterns
	hasBranching := false
	for _, n := range nodes {
		if n.Type.Picker() {
			hasBranching = true
			break
		}
	}

	if !hasBranching {
		// This is a leaf: extract as a single translatable segment
		text, hasHash := nodesToText(nodes)
		if strings.TrimSpace(text) != "" {
			segments = append(segments, segment{
				path: pathPrefix,
				text: text,
				hash: hasHash,
			})
		}
		return segments
	}

	// We have branching: process mixed content
	// Collect text before/after/between branching nodes and recurse into branches
	for _, n := range nodes {
		if !n.Type.Picker() {
			continue
		}
		for _, br := range n.Branches {
			branchPath := pathPrefix
			if branchPath != "" {
				branchPath += "."
			}
			branchPath += n.ArgName + "." + br.Keyword
			subSegments := extractSegments(br.Body, branchPath)
			if !icu.HasPicker(br.Body) {
				for i := range subSegments {
					subSegments[i].start = br.Start
					subSegments[i].end = br.End
				}
			}
			segments = append(segments, subSegments...)
		}
	}

	return segments
}

// extractLiteralSiblings returns the literal text nodes that sit beside a
// plural/select node — the sentence frame around a branch (e.g. "You have " and
// " in your cart." in "You have {count, plural, …} in your cart."). These are
// dropped by extractSegments (which only recurses into branches), so they are
// surfaced separately as non-translatable content. Whitespace-only siblings are
// skipped: they carry no prose and are preserved verbatim in the skeleton. The
// walk descends into branch bodies so framing text inside nested pickers (e.g.
// "He has " in a branch that wraps another plural) is also collected.
func extractLiteralSiblings(nodes []node) []string {
	hasBranching := false
	for _, n := range nodes {
		if n.Type.Picker() {
			hasBranching = true
			break
		}
	}
	if !hasBranching {
		// Leaf node list: its text is captured as a translatable segment by
		// extractSegments, not as a framing sibling.
		return nil
	}

	var out []string
	for _, n := range nodes {
		switch {
		case n.Type == icu.NodeText:
			if strings.TrimSpace(n.Text) != "" {
				out = append(out, n.Text)
			}
		case n.Type.Picker():
			for _, br := range n.Branches {
				out = append(out, extractLiteralSiblings(br.Body)...)
			}
		}
	}
	return out
}

// nodesToText converts a list of nodes to plain text for extraction.
// Returns the text and whether any # references were found.
func nodesToText(nodes []node) (string, bool) {
	var buf strings.Builder
	hasHash := false
	for _, n := range nodes {
		switch n.Type {
		case icu.NodeText:
			buf.WriteString(n.Text)
		case icu.NodeHash:
			buf.WriteString("#")
			hasHash = true
		case icu.NodeArg:
			// Placeholder - will be represented as inline span
			buf.WriteString(n.Text)
		}
	}
	return buf.String(), hasHash
}

// nodesHavePlaceholders checks if a node list contains argument references.
func nodesHavePlaceholders(nodes []node) bool {
	for _, n := range nodes {
		if n.Type == icu.NodeArg || n.Type == icu.NodeHash {
			return true
		}
	}
	return false
}
