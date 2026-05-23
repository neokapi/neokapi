package xcstrings

import (
	"fmt"
	"sort"
	"strings"
)

// encodeJSONString escapes a string the way Apple's JSONEncoder does for
// String Catalogs: forward slashes are NOT escaped, control characters use
// short escapes where defined and \uXXXX otherwise, and non-ASCII text is
// emitted as literal UTF-8 (Apple does not escape it). The result includes
// the surrounding double quotes.
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

// buildCanonical writes a string catalog from scratch using Apple's canonical
// formatting: a top-level object with sourceLanguage, strings, and version
// keys in alphabetical order, two-space indentation, and a space on both sides
// of the key/value colon (Xcode's JSONEncoder default for this format).
//
// This path is only used when no original document is available — i.e. a
// synthetic pipeline produced blocks without first reading an .xcstrings file.
// It groups replacements back into entries and emits a minimal but valid
// catalog. Round-trips of real files always go through rewriteCatalog and are
// byte-faithful.
func buildCanonical(repl *replacements, srcLang, version string) string {
	// Group replacements by entry key, then by language.
	type leaf struct {
		vr    valueRef
		value string
		state string
	}
	entries := make(map[string][]leaf)
	var keyOrder []string
	for k, v := range repl.values {
		vr := valueRef{Key: k.key, Lang: k.lang, Kind: k.kind, Sub: k.sub, Category: k.category}
		if _, ok := entries[k.key]; !ok {
			keyOrder = append(keyOrder, k.key)
		}
		entries[k.key] = append(entries[k.key], leaf{vr: vr, value: v.value, state: v.state})
	}
	sort.Strings(keyOrder)

	var b strings.Builder
	b.WriteString("{\n")
	ind := "  "
	b.WriteString(ind + "\"sourceLanguage\" : " + encodeJSONString(srcLang) + ",\n")
	b.WriteString(ind + "\"strings\" : {")

	if len(keyOrder) == 0 {
		b.WriteString("\n" + ind + "},\n")
	} else {
		b.WriteString("\n")
		for ei, key := range keyOrder {
			leaves := entries[key]
			b.WriteString(ind + ind + encodeJSONString(key) + " : {\n")
			// Group leaves by language. Only plain stringUnit leaves are
			// emitted from scratch; variation reconstruction without an
			// original is out of scope (real files always have an original).
			byLang := make(map[string]string)
			byLangState := make(map[string]string)
			var langOrder []string
			for _, lf := range leaves {
				if lf.vr.Kind != kindStringUnit {
					continue
				}
				if _, ok := byLang[lf.vr.Lang]; !ok {
					langOrder = append(langOrder, lf.vr.Lang)
				}
				byLang[lf.vr.Lang] = lf.value
				byLangState[lf.vr.Lang] = lf.state
			}
			sort.Strings(langOrder)
			b.WriteString(ind + ind + ind + "\"localizations\" : {\n")
			for li, lang := range langOrder {
				state := byLangState[lang]
				if state == "" {
					state = "translated"
				}
				b.WriteString(ind + ind + ind + ind + encodeJSONString(lang) + " : {\n")
				b.WriteString(ind + ind + ind + ind + ind + "\"stringUnit\" : {\n")
				b.WriteString(ind + ind + ind + ind + ind + ind + "\"state\" : " + encodeJSONString(state) + ",\n")
				b.WriteString(ind + ind + ind + ind + ind + ind + "\"value\" : " + encodeJSONString(byLang[lang]) + "\n")
				b.WriteString(ind + ind + ind + ind + ind + "}\n")
				closing := "}"
				if li < len(langOrder)-1 {
					closing += ","
				}
				b.WriteString(ind + ind + ind + ind + closing + "\n")
			}
			b.WriteString(ind + ind + ind + "}\n")
			closing := "}"
			if ei < len(keyOrder)-1 {
				closing += ","
			}
			b.WriteString(ind + ind + closing + "\n")
		}
		b.WriteString(ind + "},\n")
	}

	b.WriteString(ind + "\"version\" : " + encodeJSONString(version) + "\n")
	b.WriteString("}\n")
	return b.String()
}
