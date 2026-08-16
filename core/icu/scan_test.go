package icu_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/icu"
)

func spanTexts(s string) []string {
	var out []string
	for _, sp := range icu.Spans(s) {
		out = append(out, s[sp.Start:sp.End])
	}
	return out
}

func TestSpans(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "simple argument",
			in:   "The {vessel} is alongside until {until}.",
			want: []string{"{vessel}", "{until}"},
		},
		{
			name: "plural is one span, braces and all",
			in:   "{count, plural, one {# berth} other {# berths}} at this terminal.",
			want: []string{"{count, plural, one {# berth} other {# berths}}"},
		},
		{
			name: "select is one span",
			in:   "{gender, select, male {He} female {She} other {They}} is alongside",
			want: []string{"{gender, select, male {He} female {She} other {They}}"},
		},
		{
			name: "selectordinal is one span",
			in:   "The {n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}} berth",
			want: []string{"{n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}"},
		},
		{
			name: "plural nested in select is one span",
			in:   "{g, select, male {{n, plural, one {# berth} other {# berths}}} other {none}}",
			want: []string{"{g, select, male {{n, plural, one {# berth} other {# berths}}} other {none}}"},
		},
		{
			name: "double braces are one balanced span",
			in:   "Hello {{name}}, welcome",
			want: []string{"{{name}}"},
		},
		{
			name: "an apostrophe in prose does not swallow the placeholder",
			in:   "Don't touch {name}, it isn't yours.",
			want: []string{"{name}"},
		},
		{
			name: "a quoted brace opens no span",
			in:   "Use '{' and '}' around {name}.",
			want: []string{"{name}"},
		},
		{
			name: "an apostrophe inside a sub-message does not end the picker",
			in:   "{count, plural, one {It''s # berth} other {It''s # berths}}",
			want: []string{"{count, plural, one {It''s # berth} other {It''s # berths}}"},
		},
		{
			name: "an unbalanced brace opens no span",
			in:   "A stray { brace and {name}",
			want: []string{"{name}"},
		},
		{
			name: "no braces at all",
			in:   "Just plain prose.",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, spanTexts(tt.in))
		})
	}
}

func TestMatchBrace(t *testing.T) {
	t.Run("rejects a start that is not a brace", func(t *testing.T) {
		_, ok := icu.MatchBrace("no brace here", 3)
		assert.False(t, ok)
	})
	t.Run("rejects an unclosed group", func(t *testing.T) {
		_, ok := icu.MatchBrace("{count, plural, one {# berth}", 0)
		assert.False(t, ok)
	})
	t.Run("finds the matching close across nesting", func(t *testing.T) {
		in := "{a {b} {c}} tail"
		end, ok := icu.MatchBrace(in, 0)
		require.True(t, ok)
		assert.Equal(t, "{a {b} {c}}", in[:end+1])
	})
}

func TestUnquote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"'", "'"},
		{"''", "'"},
		{"'{'", "{"},
		{"'{braces}'", "{braces}"},
		{"'#'", "#"},
		{"'{a''b}'", "{a'b}"},
		{"'{unterminated", "{unterminated"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, icu.Unquote(tt.in))
		})
	}
}
