package check_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/check"
)

// TestDoubledWordNeedsAWord pins what the doubled-word rule counts as a word.
// Tokenization is by whitespace, so a repeated glyph tokenizes exactly like a
// repeated word — and prose that documents a notation repeats glyphs on purpose.
// The rule reports a redundant *word*, so a token with no letter and no digit
// in it is not one.
func TestDoubledWordNeedsAWord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "a repeated word is the redundancy the rule is about",
			text: "the the quick brown fox",
			want: "the",
		},
		{
			name: "case does not hide a repeated word",
			text: "The the quick brown fox",
			want: "the",
		},
		{
			name: "a repeated word with an attached digit still counts",
			text: "run pass1 pass1 again",
			want: "pass1",
		},
		{
			name: "a repeated non-Latin word still counts",
			text: "проверка проверка текста",
			want: "проверка",
		},
		{
			name: "the pseudo-translation wrapper repeats by construction",
			text: "showing ▒ ▒ Utility ▒ ▒ in pseudo",
			want: "",
		},
		{
			name: "a rule of dashes is not a doubled word",
			text: "the divider — — closes the section",
			want: "",
		},
		{
			name: "repeated punctuation is not a doubled word",
			text: "the ellipsis … … marks the elision",
			want: "",
		},
		{
			name: "clean prose reports nothing",
			text: "Detection reads the leading bytes of the file.",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, check.DoubledWord(tt.text, ""))
		})
	}
}

// TestDoubledWordExceptionsStillApply keeps the word-shape guard from
// swallowing the exception list: a language that legitimately doubles a pronoun
// still opts out by naming it, and a word not on the list still reports.
func TestDoubledWordExceptionsStillApply(t *testing.T) {
	t.Parallel()

	assert.Empty(t, check.DoubledWord("vous vous souvenez", "sie;vous;nous"))
	assert.Equal(t, "nous", check.DoubledWord("nous nous souvenons", ""))
}
