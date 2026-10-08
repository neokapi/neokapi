package contextop_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/stretchr/testify/assert"
)

func TestAvoidedForms(t *testing.T) {
	tests := []struct {
		name      string
		term      string
		insteadOf []string
		want      []string
	}{
		{
			name:      "a closed word split where a given form breaks it",
			term:      "Quickcast",
			insteadOf: []string{"Quick cast"},
			want:      []string{"Quick cast", "Quick-cast", "QuickCast"},
		},
		{
			name: "a closed word with no break to go on derives nothing",
			term: "Quickcast",
		},
		{
			name: "a capitalised ordinary word keeps its lower case",
			term: "Team",
		},
		{
			name:      "a single word names its lower-case misuse itself",
			term:      "Quickcast",
			insteadOf: []string{"quickcast"},
			want:      []string{"quickcast"},
		},
		{
			name: "a camel-case compound splits at its case change",
			term: "KapiDesktop",
			want: []string{"Kapi Desktop", "Kapi-Desktop"},
		},
		{
			name: "a capitalised spaced compound is also written closed",
			term: "Kapi Desktop",
			want: []string{"Kapi-Desktop", "KapiDesktop"},
		},
		{
			name:      "a capitalised label is never avoided in lower case",
			term:      "Sign out",
			insteadOf: []string{"Log out"},
			want:      []string{"Log out", "Sign-out", "Signout", "SignOut"},
		},
		{
			name: "a lower-case phrase derives nothing",
			term: "sign in",
		},
		{
			name:      "a lower-case closed word split like a given form",
			term:      "ripgrep",
			insteadOf: []string{"Ripgrep", "rip grep"},
			want:      []string{"Ripgrep", "rip grep", "rip-grep"},
		},
		{
			name: "a plain lower-case word derives nothing",
			term: "use",
		},
		{
			name:      "the given forms lead, each once, and never the term",
			term:      "use",
			insteadOf: []string{"utilise", "utilize", "utilise", "use"},
			want:      []string{"utilise", "utilize"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, contextop.AvoidedForms(tt.term, tt.insteadOf))
		})
	}
}

func TestObservedRule(t *testing.T) {
	rule := contextop.ObservedRule("Quickcast", []string{"Quick cast"})
	assert.Equal(t, "Quick cast", rule.Term)
	assert.Equal(t, []string{"Quick-cast", "QuickCast"}, rule.Forms)
	assert.Equal(t, "Quickcast", rule.Replacement)
	assert.True(t, rule.MatchesCase(), "a form that differs only in case must not match the term itself")

	plain := contextop.ObservedRule("use", []string{"utilise"})
	assert.Equal(t, "utilise", plain.Term)
	assert.Equal(t, "use", plain.Replacement)
	assert.False(t, plain.MatchesCase())

	bare := contextop.ObservedRule("use", nil)
	assert.Equal(t, "use", bare.Term)
	assert.Empty(t, bare.Replacement, "with nothing to avoid the rule records the term alone")
}

func TestSplitForms(t *testing.T) {
	stored, caseOnly := contextop.SplitForms(contextop.ObservedRule("Quickcast", []string{"Quick cast"}))
	assert.Equal(t, []string{"Quick cast", "Quick-cast"}, stored)
	assert.Equal(t, []string{"QuickCast"}, caseOnly, "the store folds case, so the rule enforces it itself")

	stored, caseOnly = contextop.SplitForms(contextop.ObservedRule("use", []string{"utilise"}))
	assert.Equal(t, []string{"utilise"}, stored)
	assert.Empty(t, caseOnly)
}
