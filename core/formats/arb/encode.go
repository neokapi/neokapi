package arb

import (
	"fmt"
	"sort"
	"strings"
)

// encodeJSONString escapes a string the way Dart's JsonEncoder (used by
// Flutter's gen-l10n tooling to write .arb files) does: forward slashes are
// NOT escaped, the short escapes \" \\ \b \f \n \r \t are used where defined,
// other control characters use \uXXXX, and non-ASCII text is emitted as literal
// UTF-8. The result includes the surrounding double quotes.
//
// This is only used when substituting a *changed* value or building a catalog
// from scratch; unchanged values are copied byte-for-byte from the original via
// the rewriter, so their exact escaping is always preserved.
func encodeJSONString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// buildCanonical writes an ARB document from scratch using two-space
// indentation and a space on both sides of the key/value colon — the layout
// produced by Dart's JsonEncoder.withIndent('  '). This path is only used when
// no original document is available (a synthetic pipeline produced blocks
// without first reading an .arb file). Round-trips of real files always go
// through rewriteCatalog and are byte-faithful.
//
// Only the @@locale global, message resources, and their descriptions are
// reconstructed; richer attributes (placeholders, type, …) cannot be
// reconstituted without an original and are out of scope for the from-scratch
// path.
func buildCanonical(repl *replacements, locale string) string {
	var keyOrder []string
	for k := range repl.values {
		keyOrder = append(keyOrder, k)
	}
	sort.Strings(keyOrder)

	var b strings.Builder
	b.WriteString("{\n")
	ind := "  "

	var lines []string
	if locale != "" {
		lines = append(lines, ind+encodeJSONString("@@locale")+": "+encodeJSONString(locale))
	}
	for _, key := range keyOrder {
		rv := repl.values[key]
		lines = append(lines, ind+encodeJSONString(key)+": "+encodeJSONString(rv.value))
		if rv.description != "" {
			attrs := ind + encodeJSONString("@"+key) + ": {\n" +
				ind + ind + encodeJSONString("description") + ": " + encodeJSONString(rv.description) + "\n" +
				ind + "}"
			lines = append(lines, attrs)
		}
	}

	for i, line := range lines {
		b.WriteString(line)
		if i < len(lines)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String()
}
