package check

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURLSpans(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "a bare URL in a sentence", in: "See https://example.com/utilize/setup for the steps.", want: []string{"https://example.com/utilize/setup"}},
		{name: "a full stop ends the sentence, not the address", in: "Read https://example.com/guide.", want: []string{"https://example.com/guide"}},
		{name: "a parenthesis the address does not open", in: "(see https://example.com/a)", want: []string{"https://example.com/a"}},
		{name: "a parenthesis the address opens", in: "https://en.wikipedia.org/wiki/Okapi_(disambiguation) lists them.", want: []string{"https://en.wikipedia.org/wiki/Okapi_(disambiguation)"}},
		{name: "a www host", in: "Visit www.example.com/use today.", want: []string{"www.example.com/use"}},
		{name: "another scheme", in: "Clone ssh://git@example.com/repo.git, then build.", want: []string{"ssh://git@example.com/repo.git"}},
		{name: "a quote ends the address", in: `href="https://example.com/x" target`, want: []string{"https://example.com/x"}},
		{name: "a URL after a non-ASCII character", in: "→https://example.com/x", want: []string{"https://example.com/x"}},
		{name: "two URLs", in: "https://a.example and http://b.example/p", want: []string{"https://a.example", "http://b.example/p"}},
		{name: "prose with no URL", in: "We utilize the tool: then www and http are words.", want: nil},
		{name: "a www inside a word is not a host", in: "foo.www.example.com", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, sp := range URLSpans(tt.in) {
				got = append(got, tt.in[sp[0]:sp[1]])
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// A URL is an address: a term written in it is not a use of the term, for a
// rule with a scope or without one, and the prose beside it still is.
func TestPlaceholderText_URLsAreNotTerms(t *testing.T) {
	in := "Utilize https://example.com/utilize/setup to utilize it."
	for name, project := range map[string]func(string) string{"PlaceholderText": PlaceholderText, "TermText": TermText} {
		t.Run(name, func(t *testing.T) {
			got := project(in)
			assert.Len(t, got, len(in))
			last := strings.LastIndex(in, "utilize")
			assert.Equal(t, [][2]int{{0, 7}, {last, last + 7}}, FindTerm(got, "utilize"))
			assert.False(t, ContainsTerm(got, "example"))
			assert.False(t, ContainsTerm(got, "setup"))
		})
	}
}

func TestInURL(t *testing.T) {
	text := "See https://example.com/utilize for it."
	assert.True(t, InURL(text, 24, 31), "inside the address")
	assert.True(t, InURL(text, 0, 10), "a range reaching into the address")
	assert.False(t, InURL(text, 0, 3))
	assert.False(t, InURL(text, 32, 35))
}

func TestKeepInitialCapital(t *testing.T) {
	tests := []struct {
		words, replacement string
		atSentence         bool
		want               string
	}{
		{"E-mail", "email", true, "Email"},
		{"E-mail", "email", false, "Email"},
		{"e-mail", "email", true, "email"},
		{"Utilize", "use", false, "Use"},
		{"Éditer", "modifier", true, "Modifier"},
		// A replacement the rule writes with a capital lands as written.
		{"login", "Sign in", false, "Sign in"},
		{"Login", "Sign in", true, "Sign in"},
		// A capital after the first letter belongs to the replacement.
		{"Iphone", "iPhone", true, "iPhone"},
		{"Mac OS", "macOS", true, "macOS"},
		// Capitals of a name stay with the name: an acronym, or a capital in
		// a later word.
		{"URL", "address", false, "address"},
		{"URL", "address", true, "address"},
		{"Kapi CLI", "kapi", false, "kapi"},
		{"Kapi CLI", "kapi", true, "kapi"},
		// A rule about case.
		{"Kapi", "kapi", true, "kapi"},
		// A first word that is not Title-case takes a capital only at a
		// sentence start.
		{"E-Mail", "email", true, "Email"},
		{"E-Mail", "email", false, "email"},
		{"utilize", "", true, ""},
		{"", "use", true, "use"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, KeepInitialCapital(tt.words, tt.replacement, tt.atSentence),
			"%q → %q at a sentence start: %v", tt.words, tt.replacement, tt.atSentence)
	}
}

func TestSentenceStart(t *testing.T) {
	text := "E-mail us. Then e-mail again: E-mail\nE-mail"
	for _, tt := range []struct {
		pos  int
		want bool
	}{
		{0, true},
		{strings.Index(text, "Then"), true},
		{strings.Index(text, "e-mail again"), false},
		{strings.Index(text, ": E") + 2, true},
		{strings.LastIndex(text, "E-mail"), true},
	} {
		assert.Equal(t, tt.want, sentenceStart(text, tt.pos), "at %d", tt.pos)
	}
}
