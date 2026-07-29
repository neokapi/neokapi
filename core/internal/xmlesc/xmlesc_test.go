package xmlesc_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyAttr and legacyText are verbatim copies of the four (Attr) and three
// (Text) implementations this package replaced — xliff, xliff2, openxml/wml and
// odf all carried exactly these bodies. They are kept here as the oracle for
// the differential test below: consolidation is only safe if the shared
// function agrees with what each reader used to emit on every input, and the
// cheapest way to keep proving that is to keep the old code where a test can
// call it.
func legacyAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func legacyText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// runeWalkText is the OTHER shape of the same escaper, as epub and tmx write
// it. It escapes the same three characters, so it reads as interchangeable —
// and is not. This test exists to pin the difference that makes folding those
// packages in a behaviour change rather than a cleanup.
func runeWalkText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeCorpus is the shared input set: the characters that matter, the
// orderings that could double-escape, and the byte sequences that separate a
// replacement-based escaper from a rune-walking one.
func escapeCorpus() []string {
	return []string{
		"",
		"plain text",
		"&",
		"<",
		">",
		`"`,
		"'",
		"&<>\"'",
		"a & b",
		"a < b > c",
		`say "hello"`,
		"it's",
		// Already-escaped input: escaping must be idempotent in the sense of
		// producing &amp;lt; not &lt;, and must not re-escape its own output's
		// ampersands into &amp;amp;lt;.
		"&amp;",
		"&lt;tag&gt;",
		"&amp;amp;",
		// The CDATA-end sequence, which core/formats/xliff treats specially.
		"]]>",
		"a]]>b",
		"]]",
		// Markup-shaped payloads, which is what these escapers actually see.
		`<x id="1" ctype="image"/>`,
		`<g id="1">bold</g>`,
		"<!-- comment -->",
		// Non-ASCII: multi-byte runes must pass through untouched.
		"héllo wörld",
		"日本語のテキスト",
		"emoji 🎉 and ZWJ 👨‍👩‍👧",
		" non-breaking space",
		// Control and whitespace characters: these escapers leave them alone
		// (unlike encoding/xml's EscapeText, which encodes \t \n \r).
		"tab\there",
		"line\nbreak",
		"carriage\rreturn",
		// INVALID UTF-8. This is the input that tells the two implementation
		// shapes apart.
		"\xff\xfe",
		"caf\xe9",
		"a\x80b",
		"\xed\xa0\x80",
		"valid \xc3\x28 mixed",
	}
}

// TestAttrMatchesEveryImplementationItReplaced is the safety proof for the
// consolidation. Four packages carried byte-identical copies of this escaper;
// if the shared version diverges from them on any input, round-trip bytes move
// and the parity goldens move with them. Agreement across the corpus — invalid
// UTF-8 included — is what makes the replacement a no-op rather than a change.
func TestAttrMatchesEveryImplementationItReplaced(t *testing.T) {
	t.Parallel()
	for _, in := range escapeCorpus() {
		assert.Equalf(t, legacyAttr(in), xmlesc.Attr(in),
			"Attr(%q) must emit exactly what xliff/xliff2/openxml/odf emitted", in)
	}
}

func TestTextMatchesEveryImplementationItReplaced(t *testing.T) {
	t.Parallel()
	for _, in := range escapeCorpus() {
		assert.Equalf(t, legacyText(in), xmlesc.Text(in),
			"Text(%q) must emit exactly what xliff2/openxml/odf emitted", in)
	}
}

// TestEscapersPreserveInvalidUTF8 pins the property that kept the rune-walking
// escapers out of this package.
//
// A `for _, r := range s` loop rebuilt with WriteRune turns every byte that is
// not valid UTF-8 into U+FFFD, because that is what ranging over a string
// yields. These escapers sit on paths carrying raw document bytes, so that
// substitution is data loss wearing the costume of sanitisation. Migrating
// epub or tmx onto this package would therefore change what they emit for
// undecodable input — which is why they were left alone, and why this test
// asserts the divergence exists rather than assuming it does not.
func TestEscapersPreserveInvalidUTF8(t *testing.T) {
	t.Parallel()

	const invalid = "caf\xe9 \xff\xfe"
	require.False(t, utf8.ValidString(invalid), "the fixture must actually be invalid UTF-8")

	assert.Equal(t, invalid, xmlesc.Text(invalid),
		"nothing in this input needs escaping, so the bytes must come through unchanged")
	assert.Equal(t, invalid, xmlesc.Attr(invalid))

	// The rune-walking shape does not have this property. If this assertion
	// ever starts failing, the two shapes have converged and the epub/tmx
	// escapers become genuine candidates for folding in.
	assert.NotEqual(t, invalid, runeWalkText(invalid),
		"a rune-walking escaper substitutes U+FFFD; that is the difference this package documents")
	assert.Contains(t, runeWalkText(invalid), "�")
}

// TestEscapersDoNotDoubleEscape pins the replacement ORDER. `&` must be
// replaced first, or the ampersands introduced by the `<`/`>`/`"` replacements
// get escaped a second time and `<` comes out as `&amp;lt;`.
func TestEscapersDoNotDoubleEscape(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "&lt;a&gt;", xmlesc.Text("<a>"))
	assert.Equal(t, "&lt;a&gt;", xmlesc.Attr("<a>"))
	assert.Equal(t, "&amp;quot;", xmlesc.Attr("&quot;"),
		"a literal ampersand in the input is escaped exactly once")
	assert.NotContains(t, xmlesc.Text("<&>"), "&amp;lt;")
	assert.NotContains(t, xmlesc.Attr(`"<&>"`), "&amp;quot;")
}

// TestAttrAndTextDifferOnlyInQuoting states the relationship between the two
// exported functions, so a future edit to one has to be a deliberate decision
// about the other.
func TestAttrAndTextDifferOnlyInQuoting(t *testing.T) {
	t.Parallel()
	for _, in := range escapeCorpus() {
		assert.Equalf(t, strings.ReplaceAll(xmlesc.Text(in), `"`, "&quot;"), xmlesc.Attr(in),
			"Attr is Text plus the double-quote delimiter, for input %q", in)
	}
}
