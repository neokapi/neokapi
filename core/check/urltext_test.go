package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{name: "a www inside a host name starts no URL, and the name is a bare host", in: "foo.www.example.com", want: []string{"foo.www.example.com"}},
		{name: "a www inside a word with no host after it", in: "foo.www.example", want: nil},
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

// A bare host name is an address the way a URL is. The last label tells it
// from a version number, a file name, a sentence boundary with no space and a
// name in code (hostSpans).
func TestURLSpans_BareHosts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		// Hosts.
		{name: "a two-label host", in: "Go to example.com for it.", want: []string{"example.com"}},
		{name: "a host of three labels", in: "Read docs.bowrain.cloud first.", want: []string{"docs.bowrain.cloud"}},
		{name: "a dev host", in: "See kapi.dev.", want: []string{"kapi.dev"}},
		{name: "a host with a capital in an earlier label", in: "Example.com hosts it.", want: []string{"Example.com"}},
		{name: "a host with hyphens and digits", in: "Use my-site2.example.io today.", want: []string{"my-site2.example.io"}},
		{name: "a host with a path", in: "See example.com/utilize/setup for the steps.", want: []string{"example.com/utilize/setup"}},
		{name: "a host with a port and a path", in: "Open api.example.dev:8080/v1/blocks.", want: []string{"api.example.dev:8080/v1/blocks"}},
		{name: "a host with a port", in: "Bind to example.org:443, then wait.", want: []string{"example.org:443"}},
		{name: "a host in parentheses", in: "(see example.com)", want: []string{"example.com"}},
		{name: "a host before a colon", in: "Use example.com: it works.", want: []string{"example.com"}},
		{name: "a host before a sentence with no space", in: "Visit kapi.dev.The rest follows.", want: []string{"kapi.dev"}},
		{name: "a host before a closing quote", in: `Write "example.com" there.`, want: []string{"example.com"}},
		{name: "a host already in a URL is one span", in: "Go to https://example.com/x now.", want: []string{"https://example.com/x"}},
		{name: "a country-code host", in: "Read bowrain.no first.", want: []string{"bowrain.no"}},
		{name: "a second-level ending under uk", in: "See bbc.co.uk today.", want: []string{"bbc.co.uk"}},
		{name: "a second-level ending under au", in: "See example.com.au today.", want: []string{"example.com.au"}},
		{name: "a Swedish host with a path", in: "Open example.se/docs now.", want: []string{"example.se/docs"}},
		{name: "an app host", in: "Open kapi.app in a browser.", want: []string{"kapi.app"}},
		// The last label decides, and a capital in an earlier label does not
		// change it, so a macOS bundle name reads as a host.
		{name: "a macOS bundle name", in: "Open Kapi.app to start.", want: []string{"Kapi.app"}},

		// Not hosts.
		{name: "a version number ends in digits", in: "Install 1.3.0 or v1.3.0.", want: nil},
		{name: "a file name ends in an extension", in: "Edit kapi.yaml and README.md.", want: nil},
		{name: "a file name whose stem is a host", in: "Open example.com.json now.", want: nil},
		{name: "a shell script", in: "Run install.sh first.", want: nil},
		{name: "a Markdown file", in: "Read README.md first.", want: nil},
		{name: "an autoconf input", in: "Edit Makefile.in or requirements.in.", want: nil},
		{name: "source files", in: "Open main.go, app.ts, lib.rs and run.py.", want: nil},
		{name: "a sentence boundary with no space", in: "Run kapi.The next step reads it.", want: nil},
		{name: "a name in code ending in a capital", in: "Call cfg.Name and System.Net here.", want: nil},
		{name: "a name in code ending in another word", in: "Call log.debug here.", want: nil},
		{name: "a top-level label inside a longer word", in: "Open kapi.devtools or kapi.dev_mode.", want: nil},
		{name: "a top-level label before a hyphen", in: "Read example.com-backup now.", want: nil},
		{name: "a top-level label in capitals", in: "Mail EXAMPLE.COM now.", want: nil},
		{name: "a top-level label not on the list", in: "Read example.zz first.", want: nil},
		{name: "an abbreviation", in: "Use a tool, e.g. this one, i.e. kapi.", want: nil},
		{name: "a path before the name", in: "Edit src/example.com here.", want: nil},
		{name: "an email address", in: "Mail admin@example.com today.", want: nil},
		{name: "a single label", in: "Open localhost:8080 now.", want: nil},
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

// The list of endings stays sorted and holds no common file extension.
func TestHostTopLabels(t *testing.T) {
	assert.True(t, slices.IsSorted(hostTopLabels))
	for _, ext := range []string{"md", "sh", "rs", "pl", "so", "ps", "go", "js", "ts", "py", "cs", "rb", "php", "css", "yml", "yaml", "json", "txt", "toml", "mod", "sum", "pb", "in"} {
		assert.NotContains(t, hostTopLabels, ext)
	}
}

// A term written in a bare host name is not a use of the term, and the same
// term in the prose beside it is.
func TestTermText_BareHostsAreNotTerms(t *testing.T) {
	tests := []struct {
		name string
		in   string
		term string
		want []string // the matched text, in order
	}{
		{name: "a term as a label", in: "Read docs.example.dev or the docs page.", term: "docs", want: []string{"docs"}},
		{name: "a term in a host's path", in: "See example.com/utilize, then utilize it.", term: "utilize", want: []string{"utilize"}},
		{name: "a term of two words beside a host", in: "Open the Sign in page on example.com.", term: "Sign in", want: []string{"Sign in"}},
		{name: "a term of two words with no host", in: "Open the Sign in page.", term: "Sign in", want: []string{"Sign in"}},
		{name: "a term in a file name", in: "Edit kapi.yaml first.", term: "kapi", want: []string{"kapi"}},
		{name: "a term in a version line", in: "Release 1.3.0 ships the release notes.", term: "release", want: []string{"Release", "release"}},
		{name: "a term before a sentence with no space", in: "Run kapi.The check reads it.", term: "kapi", want: []string{"kapi"}},
		{name: "a term as the host's name", in: "Install from kapi.dev.The kapi binary follows.", term: "kapi", want: []string{"kapi"}},
	}
	for _, tt := range tests {
		for name, project := range map[string]func(string) string{"PlaceholderText": PlaceholderText, "TermText": TermText} {
			t.Run(tt.name+"/"+name, func(t *testing.T) {
				got := project(tt.in)
				require.Len(t, got, len(tt.in))
				var matched []string
				for _, sp := range FindTerm(got, tt.term) {
					matched = append(matched, tt.in[sp[0]:sp[1]])
				}
				assert.Equal(t, tt.want, matched)
			})
		}
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
