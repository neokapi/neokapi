// Package xmlesc holds the XML escapers that several format readers had
// independently written identically.
//
// # Why this package is small on purpose
//
// A repository-wide survey found 19 escaping helpers across 9 packages. Most
// of them look like duplicates and are not: the differences encode what a
// particular writer must emit to stay byte-stable against okapi's reference
// output, which the parity suite asserts. Folding them together would silently
// change round-trip bytes — the exact failure this package must not cause.
//
// So only the provably identical ones live here. [Attr] and [Text] were
// byte-for-byte the same source in xliff, xliff2, openxml/wml and odf, which
// makes replacing them a no-op by construction. Everything else stays where it
// is, each carrying a comment naming what makes it different, so the next
// reader does not have to repeat the analysis.
//
// # The escapers that deliberately did NOT move here
//
//   - core/formats/xliff xmlEscapeText — escapes `>` only inside `]]>`, to
//     stay byte-identical to okapi's XLIFFWriter.
//   - core/formats/xml xmlEscapeAttrValue — omits `>`, matching okapi-bridge's
//     reference round-trip for ITS test01.xml.
//   - core/formats/xml xmlEscapeString — escapes `"` in TEXT position, because
//     okapi's reference writer emits &quot; in extracted content.
//   - core/formats/ts xmlEscapeRune / xmlEscapeRuneEntity — `'` stays raw in
//     <source> but becomes &apos; in <numerusform>, per the okapi reference.
//   - core/formats/odf odfEscapeText — escapes all five predefined entities,
//     `'` included.
//   - core/formats/epub, core/formats/tmx — walk runes, so invalid UTF-8
//     becomes U+FFFD; see [Text].
//   - core/formats/openxml xmlEscapeRune — streams one rune into a Builder
//     rather than returning a string.
//   - memory/tmx_export xmlEscape — html.EscapeString, which uses numeric
//     entities (&#39;, &#34;) and is guarded by a ContainsAny check, so quotes
//     escape only when an &, < or > is also present.
//   - memory/tmx_export xmlAttr — output-identical to [Attr], but memory/ sits
//     outside core/ and so cannot import this package.
//   - scripts/mkappcast xmlEscape — encoding/xml's EscapeText, which also
//     encodes tab, newline and carriage return.
//
// # The escaping rules
//
// Both functions escape by string replacement rather than by walking runes,
// and that is load-bearing rather than incidental — see [Text] for why.
package xmlesc

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Attr escapes a string for use inside a double-quoted XML attribute value:
// `&`, `<`, `>` and `"`.
//
// `>` is not required inside an attribute value by XML 1.0 §2.4, and `'` is
// not required inside a double-quoted one, but this is the set the readers
// listed above have always emitted and the set okapi's reference output
// carries — so it is the set that keeps round-trips byte-stable.
//
// A writer with a different obligation must NOT be migrated onto this
// function. core/formats/xml's xmlEscapeAttrValue deliberately leaves `>`
// literal because okapi-bridge's reference round-trip for ITS test01.xml shows
// a literal `>` inside attribute values; that is a real behavioural
// difference, not an oversight.
func Attr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// UnescapeAttr reverses [Attr] for an attribute value read back out of already-
// serialized markup: the five predefined entities of XML 1.0 §4.6, plus the
// numeric character references a producer may emit instead.
//
// It exists for the narrow case of recovering a value from markup this package
// (or a document) already wrote, when re-running the XML decoder over the
// fragment would cost more than it is worth. Anything parsed by encoding/xml is
// already unescaped and must not be passed through here — doing so would decode
// a literal `&amp;` in the document down to `&`, which is data loss.
//
// `&amp;` is replaced LAST, mirroring Attr replacing it first: otherwise
// `&amp;lt;` — a document that really contains the text `&lt;` — would decode
// all the way to `<`.
func UnescapeAttr(s string) string {
	if !strings.ContainsRune(s, '&') {
		return s
	}
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&apos;", "'")
	s = numericCharRefRE.ReplaceAllStringFunc(s, func(ref string) string {
		body := ref[2 : len(ref)-1]
		base, digits := 10, body
		if body[0] == 'x' || body[0] == 'X' {
			base, digits = 16, body[1:]
		}
		n, err := strconv.ParseInt(digits, base, 32)
		if err != nil || !utf8.ValidRune(rune(n)) {
			return ref
		}
		return string(rune(n))
	})
	return strings.ReplaceAll(s, "&amp;", "&")
}

// numericCharRefRE matches a decimal or hexadecimal character reference.
var numericCharRefRE = regexp.MustCompile(`&#[xX]?[0-9A-Fa-f]+;`)

// Text escapes a string for XML character-data position: `&`, `<` and `>`.
//
// Escaping `>` unconditionally is more than XML 1.0 §2.4 requires — a bare `>`
// is legal in character data except after `]]` — but it is what these readers
// have always emitted. core/formats/xliff's own xmlEscapeText takes the
// narrower reading, escaping `>` only in `]]>`, to stay byte-identical to
// okapi's XLIFFWriter; it is therefore NOT this function and must not be
// merged into it.
//
// # Replacement order, and why it is not rune-by-rune
//
// `&` is replaced first so the ampersands introduced by the later
// replacements are not escaped a second time into `&amp;lt;`.
//
// The loop is over the string, not over its runes, and that distinction has
// teeth. A `for _, r := range s` loop that rebuilds the string with
// strings.Builder.WriteRune substitutes U+FFFD for every byte that is not
// valid UTF-8, because that is what ranging over a string yields for invalid
// input. These escapers sit on paths that carry raw document bytes, where
// silently rewriting undecodable input to replacement characters would be data
// loss dressed up as sanitisation. strings.ReplaceAll copies unmatched bytes
// through untouched, valid UTF-8 or not.
//
// That is also why the rune-walking escapers elsewhere in the tree — epub,
// tmx, ts, xml — were left alone rather than pointed here: they escape the
// same characters but do not have the same behaviour on invalid UTF-8, so
// swapping them would be a real change wearing the costume of a cleanup.
func Text(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
