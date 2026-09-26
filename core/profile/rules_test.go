package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasDeterministicRules(t *testing.T) {
	tone, err := LoadProfileYAML(strings.NewReader("name: Tone\ntone:\n  personality: [plain]\n"))
	require.NoError(t, err)
	guidance := Constraint{ID: "plain", Version: 1, Source: "guide.md", Statement: "Write plainly.", Kind: ConstraintGuidance}
	prohibited := Constraint{ID: "no-todo", Version: 1, Source: "guide.md", Statement: "No TODO.", Kind: ConstraintProhibitedPattern, Regex: "TODO"}
	elsewhere := prohibited
	elsewhere.Scope = ConstraintScope{Channel: "docs"}
	tests := []struct {
		name    string
		profile *VoiceProfile
		want    bool
	}{
		{"no profile", nil, false},
		{"tone alone", tone, false},
		{"comment limits alone", &VoiceProfile{Style: StyleRules{Comments: &CommentRules{}}}, false},
		{"guidance alone", &VoiceProfile{Constraints: []Constraint{guidance}}, false},
		{"a term with no text", carrying(TermRule{Term: "  "}), false},
		{"a preferred form, which rejects nothing", carrying(TermRule{Replacement: "sign in"}), false},
		{"a prohibited-pattern constraint scoped elsewhere", &VoiceProfile{Constraints: []Constraint{elsewhere}}, false},
		{"a forbidden term", carrying(TermRule{Term: "risk-free"}), true},
		{"a competitor term", carrying(TermRule{Term: "Acme", Competitor: true}), true},
		{"a prohibited pattern", &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{Regex: "TODO"}}}}, true},
		{"a required pattern", &VoiceProfile{Style: StyleRules{RequiredPatterns: []Pattern{{Regex: "Hello"}}}}, true},
		{"a prohibited-pattern constraint", &VoiceProfile{Constraints: []Constraint{prohibited}}, true},
		{"an invalid constraint, which reports on every block", &VoiceProfile{Constraints: []Constraint{{Kind: ConstraintGuidance}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasDeterministicRules(tt.profile))
		})
	}
}

// carrying is an empty voice profile carrying the given word rules.
func carrying(rules ...TermRule) *VoiceProfile {
	return (&VoiceProfile{}).Carry("test", rules)
}
