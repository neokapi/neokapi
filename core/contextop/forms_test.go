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
			want:      []string{"Quick cast", "Quick-cast", "QuickCast", "quickcast"},
		},
		{
			name: "a closed word with no break to go on varies only its case",
			term: "Quickcast",
			want: []string{"quickcast"},
		},
		{
			name: "a camel-case compound splits at its case change",
			term: "KapiDesktop",
			want: []string{"Kapi Desktop", "Kapi-Desktop", "kapidesktop"},
		},
		{
			name: "a capitalised spaced compound is also written closed",
			term: "Kapi Desktop",
			want: []string{"Kapi-Desktop", "KapiDesktop", "kapi desktop"},
		},
		{
			name: "a lower-case phrase is only hyphenated",
			term: "content memory",
			want: []string{"content-memory"},
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
	assert.Equal(t, []string{"Quick-cast", "QuickCast", "quickcast"}, rule.Forms)
	assert.Equal(t, "Quickcast", rule.Replacement)
	assert.True(t, rule.CaseSensitive, "a form that differs only in case must not match the term itself")

	plain := contextop.ObservedRule("use", []string{"utilise"})
	assert.Equal(t, "utilise", plain.Term)
	assert.Equal(t, "use", plain.Replacement)
	assert.False(t, plain.CaseSensitive)

	bare := contextop.ObservedRule("use", nil)
	assert.Equal(t, "use", bare.Term)
	assert.Empty(t, bare.Replacement, "with nothing to avoid the rule records the term alone")
}
