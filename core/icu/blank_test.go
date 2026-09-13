package icu_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/icu"
)

// literalText is the text of msg outside its syntax ranges, one trimmed piece
// per gap, so a case reads as the words a reader sees.
func literalText(msg string) []string {
	var out []string
	last := 0
	for _, sp := range icu.SyntaxSpans(msg) {
		if piece := strings.TrimSpace(msg[last:sp.Start]); piece != "" {
			out = append(out, piece)
		}
		last = sp.End
	}
	if piece := strings.TrimSpace(msg[last:]); piece != "" {
		out = append(out, piece)
	}
	return out
}

func TestSyntaxSpans(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "simple arguments",
			in:   "{vessel} is alongside until {until}.",
			want: []string{"is alongside until", "."},
		},
		{
			name: "typed argument",
			in:   "Declared draught {draught, number} m.",
			want: []string{"Declared draught", "m."},
		},
		{
			name: "plural keeps the text of each branch",
			in:   "{count, plural, one {# berth} other {# berths}} at this terminal.",
			want: []string{"berth", "berths", "at this terminal."},
		},
		{
			name: "select",
			in:   "{gender, select, male {He} female {She} other {They}} is alongside.",
			want: []string{"He", "She", "They", "is alongside."},
		},
		{
			name: "offset and exact-value branches",
			in:   "{count, plural, offset:1 =0 {Nobody} one {# other} other {# others}} aboard.",
			want: []string{"Nobody", "other", "others", "aboard."},
		},
		{
			name: "plural nested in select",
			in:   "{g, select, male {{n, plural, one {# berth} other {# berths}}} other {none}} today.",
			want: []string{"berth", "berths", "none", "today."},
		},
		{
			name: "a # outside any branch is text",
			in:   "Berth #4 for {vessel}",
			want: []string{"Berth #4 for"},
		},
		{
			name: "quoted braces are text",
			in:   "Write '{' and '}' around {name}.",
			want: []string{"Write '{' and '}' around", "."},
		},
		{
			name: "a brace that never closes is text",
			in:   "Use {unclosed brace for the vessel",
			want: []string{"Use {unclosed brace for the vessel"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, literalText(tt.in))
		})
	}
}

func TestSyntaxSpans_InOrderAndDisjoint(t *testing.T) {
	msg := "{g, select, male {{n, plural, offset:1 one {# berth} other {# berths}}} other {none}} today {when}."
	last := 0
	for _, sp := range icu.SyntaxSpans(msg) {
		assert.GreaterOrEqual(t, sp.Start, last, "ranges are in order and do not overlap")
		assert.Greater(t, sp.End, sp.Start)
		last = sp.End
	}
}
