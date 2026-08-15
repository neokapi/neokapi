package clip_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/clip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"empty stays empty", "", 10, ""},
		{"shorter than the budget is returned whole", "abc", 10, "abc"},
		{"exactly the budget is not marked", "abcde", 5, "abcde"},
		{"one over the budget is cut and marked", "abcdef", 5, "abcde…"},
		{"zero budget leaves only the mark", "abc", 0, "…"},
		{"zero budget on an empty string stays empty", "", 0, ""},
		{"a negative budget clips to nothing", "abc", -3, "…"},
		{"the budget counts runes, not bytes", "ååååå", 3, "ååå…"},
		{"a multi-byte string within budget is untouched", "ååå", 3, "ååå"},
		{"astral runes count once each", "🙂🙂🙂", 2, "🙂🙂…"},
		{"mixed-width prose cuts on a rune boundary", "en – dash", 4, "en –…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clip.Runes(tt.in, tt.n))
		})
	}
}

// A byte-slicing truncation splits a multi-byte rune and emits U+FFFD into the
// message it was shortening. Every cut offset must stay well-formed.
func TestRunesNeverEmitsInvalidUTF8(t *testing.T) {
	inputs := []string{
		"på tvers av grensesnittet",             // Latin-1 supplement
		"それはドキュメントです",                           // CJK
		"“typographic quotes” and an – en dash", // punctuation the prose readers see
		"🇳🇴 regional indicators 🙂",              // astral plane
		strings.Repeat("é", 40),
	}
	for _, in := range inputs {
		t.Run(clip.Runes(in, 12), func(t *testing.T) {
			for n := range utf8.RuneCountInString(in) + 3 {
				got := clip.Runes(in, n)
				require.True(t, utf8.ValidString(got),
					"clip.Runes(%q, %d) = %q is not valid UTF-8", in, n, got)
				assert.NotContains(t, got, "�",
					"clip.Runes(%q, %d) emitted a replacement character", in, n)
			}
		})
	}
}

func TestRunesKeepsThePrefixIntact(t *testing.T) {
	const in = "kort tekst på norsk med æ, ø og å"
	for n := range utf8.RuneCountInString(in) {
		got := clip.Runes(in, n)
		require.True(t, strings.HasPrefix(in, strings.TrimSuffix(got, "…")),
			"clip.Runes(%q, %d) = %q is not a prefix of the input", in, n, got)
		assert.Equal(t, n, utf8.RuneCountInString(strings.TrimSuffix(got, "…")),
			"clip.Runes(%q, %d) kept the wrong number of runes", in, n)
	}
}
