package icu_test

import (
	"strings"
	"testing"
	"unicode/utf8"

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

func TestQuoteLiteral(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		inBranch bool
		want     string
	}{
		{name: "plain text", in: "Hello", want: "Hello"},
		{name: "apostrophe in prose", in: "C'est ici", want: "C'est ici"},
		{name: "apostrophe ending text", in: "élèves'", want: "élèves''"},
		{name: "two apostrophes", in: "a''b", want: "a'''b"},
		{name: "braces", in: "{un}", want: "'{'un'}'"},
		{name: "a run of braces is one span", in: "{}", want: "'{}'"},
		{name: "apostrophe beside a brace", in: "{'s", want: "'{'''s"},
		{name: "apostrophe before a brace", in: "l'{", want: "l'''{'"},
		{name: "apostrophe before a pipe", in: "a'|b", want: "a''|b"},
		{name: "number sign in a branch", in: "lot #1", inBranch: true, want: "lot '#'1"},
		{name: "number sign outside a branch", in: "lot #1", want: "lot #1"},
		{name: "apostrophe before a number sign outside a branch", in: "a'#", want: "a''#"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, icu.QuoteLiteral(tt.in, tt.inBranch))
		})
	}
}

// FuzzQuoteLiteral checks that the parser reads QuoteLiteral's spelling back as
// the text it was given, at the top of a message and inside a branch, with an
// argument written straight after it.
func FuzzQuoteLiteral(f *testing.F) {
	for _, seed := range []string{
		"", "Hello", "L'", "L'{", "it's", "a''b", "'", "''", "'''", "{", "}", "{}", "#",
		"'#'", "'{'", "{'s} '{", "a'|b", "lot #1", "ä'{ö}'#",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if !utf8.ValidString(text) {
			t.Skip("the parser reads invalid UTF-8 as U+FFFD; text runs are always valid")
		}
		readsBack := func(t *testing.T, nodes []icu.Node) {
			t.Helper()
			require.NotEmpty(t, nodes)
			var got strings.Builder
			for i, n := range nodes {
				if i == len(nodes)-1 {
					require.Equal(t, icu.NodeArg, n.Type, "the argument after the text survives")
					continue
				}
				require.Equal(t, icu.NodeText, n.Type)
				got.WriteString(n.Text)
			}
			assert.Equal(t, text, got.String())
		}

		top, err := icu.Parse(icu.QuoteLiteral(text, false) + "{x}")
		require.NoError(t, err)
		readsBack(t, top)

		branch, err := icu.Parse("{n, plural, other {" + icu.QuoteLiteral(text, true) + "{x}}}")
		require.NoError(t, err)
		require.Len(t, branch, 1)
		require.Len(t, branch[0].Branches, 1)
		readsBack(t, branch[0].Branches[0].Body)
	})
}
