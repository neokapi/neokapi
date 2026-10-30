package profile

import (
	"github.com/neokapi/neokapi/core/model"
)

// ScopedText answers where a word rule's scope (TermRule.Scope) applies in a
// text: whether a match at a position sits in prose, in code or in a heading.
//
// Code is what the text spells as code, fenced blocks and backtick spans
// (check.CodeSpans), and, for the text of a block, the inline code spans its
// runs declare. A reader holds a markdown code span as a [model.TypeInlineCode]
// pair around the code's text, so the text of the runs carries no backticks
// and a scope read off the characters alone would take the code for prose.
//
// The code spans are found on the first scoped question, so a caller whose
// rules name no scope pays nothing for them.
type ScopedText struct {
	text  string
	runs  []model.Run
	code  []span
	found bool
}

// NewScopedText reads text for scopes. runs are the runs text flattens from,
// text being model.RunsText(runs), or nil for plain text.
func NewScopedText(text string, runs []model.Run) *ScopedText {
	return &ScopedText{text: text, runs: runs}
}

// Applies reports whether a match at byte offset pos of the text counts for a
// rule with this scope. An empty scope applies everywhere.
func (s *ScopedText) Applies(scope string, pos int) bool {
	if scope == "" {
		return true
	}
	if !s.found {
		s.found = true
		s.code = codeSpans(s.text)
		if len(s.runs) > 0 {
			s.code = append(s.code, runsCodeSpans(s.runs)...)
		}
	}
	return inScope(scope, s.text, s.code, pos)
}

// runsCodeSpans returns the inline code spans the runs declare, as byte
// ranges of model.RunsText(runs). It reads them off the projection the
// pattern rules read (RunsPatternText), so a word rule and a pattern rule
// agree about where code sits in a block.
func runsCodeSpans(runs []model.Run) []span {
	t := RunsPatternText(runs)
	if len(t.code) == 0 {
		return nil
	}
	out := make([]span, 0, len(t.code))
	for _, c := range t.code {
		out = append(out, span{t.RealOffset(c.start), t.RealOffset(c.end)})
	}
	return out
}
