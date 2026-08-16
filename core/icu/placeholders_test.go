package icu_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/icu"
)

func TestPlaceholderTokens(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		picker    bool
		wantFrame map[string]int
		wantInner []string
	}{
		{
			name:      "simple arguments are counted in the frame",
			in:        "{vessel} is alongside until {until}.",
			picker:    false,
			wantFrame: map[string]int{"{vessel}": 1, "{until}": 1},
		},
		{
			name:      "a repeated argument is counted twice",
			in:        "{vessel} follows {vessel}.",
			picker:    false,
			wantFrame: map[string]int{"{vessel}": 2},
		},
		{
			name:      "a typed argument keeps its type and style",
			in:        "Berth {n, number, integer} of {total, number}.",
			picker:    false,
			wantFrame: map[string]int{"{n, number, integer}": 1, "{total, number}": 1},
		},
		{
			name:      "a plural head is the frame token and # is inner",
			in:        "{count, plural, one {# berth} other {# berths}} at this terminal.",
			picker:    true,
			wantFrame: map[string]int{"{count, plural}": 1},
			wantInner: []string{"#"},
		},
		{
			name:      "branch keywords are not tokens",
			in:        "{count, plural, one {# kaiplass} other {# kaiplasser}} på denne terminalen.",
			picker:    true,
			wantFrame: map[string]int{"{count, plural}": 1},
			wantInner: []string{"#"},
		},
		{
			name:      "a language with four categories yields the same tokens",
			in:        "{count, plural, one {# miejsce} few {# miejsca} many {# miejsc} other {# miejsca}}",
			picker:    true,
			wantFrame: map[string]int{"{count, plural}": 1},
			wantInner: []string{"#"},
		},
		{
			name:      "a select head carries its keyword",
			in:        "{gender, select, male {He is alongside} female {She is alongside} other {They are alongside}}",
			picker:    true,
			wantFrame: map[string]int{"{gender, select}": 1},
		},
		{
			name:      "selectordinal is a picker",
			in:        "The {n, selectordinal, one {#st} other {#th}} berth",
			picker:    true,
			wantFrame: map[string]int{"{n, selectordinal}": 1},
			wantInner: []string{"#"},
		},
		{
			name:      "an offset is part of the plural head",
			in:        "{count, plural, offset:1 =0 {Nobody} one {# other} other {# others}}",
			picker:    true,
			wantFrame: map[string]int{"{count, plural, offset:1}": 1},
			wantInner: []string{"#"},
		},
		{
			name:      "a nested picker is an inner token",
			in:        "{g, select, male {{n, plural, one {# berth} other {# berths}}} other {none}}",
			picker:    true,
			wantFrame: map[string]int{"{g, select}": 1},
			wantInner: []string{"{n, plural}", "#"},
		},
		{
			name:      "an argument inside a sub-message is an inner token",
			in:        "{count, plural, one {{vessel} has # berth} other {{vessel} has # berths}}",
			picker:    true,
			wantFrame: map[string]int{"{count, plural}": 1},
			wantInner: []string{"{vessel}", "#"},
		},
		{
			name:      "a quoted brace is literal text, not an argument",
			in:        "Write '{'name'}' to interpolate {name}.",
			picker:    false,
			wantFrame: map[string]int{"{name}": 1},
		},
		{
			name:      "an apostrophe in prose does not hide the argument",
			in:        "Don't move {vessel} yet.",
			picker:    false,
			wantFrame: map[string]int{"{vessel}": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := icu.PlaceholderTokens(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.picker, got.Picker)
			want := tt.wantFrame
			if want == nil {
				want = map[string]int{}
			}
			assert.Equal(t, want, got.Frame)
			inner := map[string]bool{}
			for _, tok := range tt.wantInner {
				inner[tok] = true
			}
			assert.Equal(t, inner, got.Inner)
		})
	}
}

func TestPlaceholderTokens_Invalid(t *testing.T) {
	for _, in := range []string{
		"{unterminated, plural,",
		"Hello {{name}}, welcome",
		"a stray } brace",
	} {
		t.Run(in, func(t *testing.T) {
			_, err := icu.PlaceholderTokens(in)
			assert.Error(t, err)
		})
	}
}
