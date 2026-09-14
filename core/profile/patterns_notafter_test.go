package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// usedToNotAfter keeps the phrase a violation after a noun or a pronoun, and
// not after an auxiliary, opening punctuation, an opening quote or the start of
// the text.
const usedToNotAfter = `(?i)(?:(?:\b(?:is|are|was|were|be|been|being|get|gets|got|isn't|aren't|wasn't|weren't)|['’]s)\s+(?:\w+\s+)?|[^\w\s)\]"'\x60’”]\s*|(?:^|\s)["“‘'\x60]|^\s*)$`

func TestPatternNotAfter(t *testing.T) {
	p := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{
		{Regex: `(?i)\bused to\b`, NotAfter: usedToNotAfter, Severity: "major", Scope: ScopeProse},
	}}}
	for _, tc := range []struct {
		name, text string
		hits       int
	}{
		{"TSX-TE112 flag", "The source used to go through the document-preview kit.", 1},
		{"TSX-TE113 flag", "the editing the deleted CLI relation commands used to do.", 1},
		{"TSX-TE114 fine: after a dash", "The active project's id — used to flag same-project vs cross-project content memory.", 0},
		{"after an auxiliary", "The hash is used to bind a decision to its text.", 0},
		{"after a comma", "a translation, used to bind a review decision", 0},
		{"at the start", "Used to locate the file.", 0},
		{"after a word on the line above", "The warning\nused to fire for any governance.", 1},
		{"in a code span", "Call `x used to` here.", 0},
		{"after an auxiliary and an adverb", "The slug is never used to authorize a secret.", 0},
		{"after a contracted is", "It's used to bind a decision.", 0},
		{"after a code span", "`kcat some.dmg` used to fall back to plaintext.", 1},
		{"after a closing parenthesis", "A source edit (the desktop's fix) used to be written back.", 1},
		{"after an opening parenthesis", "the launcher (used to derive the plugins dir)", 0},
		{"quoted as a mention", "The phrase \"used to\" reads as history.", 0},
		{"in single quotes as a mention", "The phrase 'used to' reads as history.", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits := MatchPatterns(p, tc.text)
			require.Len(t, hits, tc.hits)
			for _, h := range hits {
				assert.Equal(t, "used to", strings.ToLower(tc.text[h.Start:h.End]))
			}
		})
	}

	t.Run("must fail: without not_after every use is a violation", func(t *testing.T) {
		bare := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{Regex: `(?i)\bused to\b`}}}}
		assert.Len(t, MatchPatterns(bare, "The hash is used to bind a decision."), 1)
	})

	t.Run("a not_after that does not compile is refused, and matches nothing", func(t *testing.T) {
		broken := &VoiceProfile{Name: "S", Style: StyleRules{ProhibitedPatterns: []Pattern{{Regex: `(?i)\bused to\b`, NotAfter: `(`}}}}
		var fields []string
		for _, prob := range Blocking(ValidateProfile(broken)) {
			fields = append(fields, prob.Field)
		}
		assert.Contains(t, fields, "style.prohibited_patterns[0].not_after")
		assert.Empty(t, MatchPatterns(broken, "The source used to go."))
	})

	t.Run("the key loads", func(t *testing.T) {
		loaded, err := LoadProfileYAML(strings.NewReader("name: S\nstyle:\n  prohibited_patterns:\n    - regex: '(?i)\\bused to\\b'\n      not_after: '(?i)\\bis\\s+$'\n"))
		require.NoError(t, err)
		assert.Equal(t, `(?i)\bis\s+$`, loaded.Style.ProhibitedPatterns[0].NotAfter)
		probs, err := UnknownKeys([]byte("name: S\nstyle:\n  prohibited_patterns:\n    - regex: x\n      not_after: y\n"))
		require.NoError(t, err)
		assert.Empty(t, probs)
	})
}
