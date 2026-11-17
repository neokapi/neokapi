package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

func TestPatternWords(t *testing.T) {
	tests := []struct {
		regex string
		want  []string
	}{
		{`\b(powerful|blazing|game-changing)\b`, []string{"powerful", "blazing", "game-changing"}},
		{`(?i)\b(?:simply|easily)\b`, []string{"simply", "easily"}},
		{`\bgame[- ]changing\b`, []string{"game changing", "game-changing"}},
		{`\b[Ll]ocaliz(e|ation)\b`, []string{"localize", "localization"}},
		{`\bpower(ful|-hungry)\b`, []string{"powerful", "power-hungry"}},
		{`\butil\w*`, []string{"util…"}},
		{`\bword(s)?\b`, []string{"word", "words"}},
		{`brochure framing`, []string{"brochure framing"}},
		{`—`, []string{"—"}},
		{`\b\w+ly\b`, nil},
		{`.{200,}`, nil},
		{`(`, nil},
		{``, nil},
	}
	for _, tt := range tests {
		t.Run(tt.regex, func(t *testing.T) {
			assert.Equal(t, tt.want, PatternWords(tt.regex))
		})
	}
}

func TestPatternExcerpts(t *testing.T) {
	runs := []model.Run{{Text: &model.TextRun{Text: "Our engine is a powerful way to ship. It is powerful indeed, and the rest of this sentence runs on well past the window an excerpt keeps."}}}
	got := PatternExcerpts(Pattern{Regex: `\bpowerful\b`, Rate: &PatternRate{Max: 5, Per: 100}}, runs, 1)
	if assert.Len(t, got, 1) {
		assert.Equal(t, PatternExcerpt{Before: "Our engine is a ", Match: "powerful", After: " way to ship. It is powerful indeed, and the rest of this…"}, got[0])
	}
	assert.Len(t, PatternExcerpts(Pattern{Regex: `\bpowerful\b`}, runs, 5), 2)
	assert.Empty(t, PatternExcerpts(Pattern{Regex: `\bblazing\b`}, runs, 5))
}
