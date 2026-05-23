package applestrings

import (
	"sort"
	"strings"
)

// rewriteStringsdict re-tokenizes the original .stringsdict bytes and rewrites
// the inner text of each translatable <string> whose corresponding block has a
// changed value, preserving every other token (the DOCTYPE, keys, whitespace,
// entity encoding) exactly. Unmodified documents round-trip byte-identically.
func rewriteStringsdict(original string, values map[leafRef]string) ([]byte, error) {
	doc, err := parseStringsdict(original)
	if err != nil {
		return nil, err
	}

	// Map each leaf's <string> start-tag token index → encoded replacement,
	// but only where the encoding differs from the original inner bytes.
	type repl struct {
		strStart int
		strEnd   int
		encoded  string
	}
	var repls []repl
	for i := range doc.leafs {
		leaf := doc.leafs[i]
		ref := leafRef{
			key:      leaf.topKey,
			leaf:     string(leaf.kind),
			variable: leaf.variable,
			category: leaf.category,
		}
		newVal, ok := values[ref]
		if !ok {
			continue
		}
		// Compare against the DECODED leaf value so a re-encoding that differs
		// only in entity form (e.g. &#8230; → …) does not trigger a needless
		// rewrite. Only a genuine value change re-encodes.
		if newVal == leaf.value {
			continue
		}
		repls = append(repls, repl{strStart: leaf.strStart, strEnd: leaf.strEnd, encoded: encodePlistText(newVal)})
	}
	sort.Slice(repls, func(i, j int) bool { return repls[i].strStart < repls[j].strStart })

	var b strings.Builder
	b.Grow(len(original))
	ri := 0
	for i := 0; i < len(doc.toks); i++ {
		if ri < len(repls) && repls[ri].strStart == i {
			r := repls[ri]
			// Emit the <string> start tag, the encoded replacement, then the
			// </string> end tag; skip the original inner tokens.
			b.WriteString(doc.toks[r.strStart].raw)
			b.WriteString(r.encoded)
			b.WriteString(doc.toks[r.strEnd].raw)
			i = r.strEnd
			ri++
			continue
		}
		b.WriteString(doc.toks[i].raw)
	}
	return []byte(b.String()), nil
}

// buildStringsdictFromScratch emits a canonical .stringsdict document when no
// original is available. It groups collected leaves by top-level key, writing
// the NSStringLocalizedFormatKey format string and one variable plural sub-dict
// per variable. This path is best-effort for synthetic pipelines; real
// round-trips always use the original-bytes rewrite path.
func buildStringsdictFromScratch(order []leafRef, values map[leafRef]string) []byte {
	// Group by topKey preserving first-seen order.
	type group struct {
		key       string
		format    string
		hasFormat bool
		variables []string
		plurals   map[string]map[string]string // variable -> category -> value
	}
	var groups []*group
	byKey := make(map[string]*group)
	get := func(key string) *group {
		if g, ok := byKey[key]; ok {
			return g
		}
		g := &group{key: key, plurals: make(map[string]map[string]string)}
		byKey[key] = g
		groups = append(groups, g)
		return g
	}
	for _, ref := range order {
		g := get(ref.key)
		switch ref.leaf {
		case string(leafFormatKey):
			g.format = values[ref]
			g.hasFormat = true
		case string(leafPlural):
			if g.plurals[ref.variable] == nil {
				g.plurals[ref.variable] = make(map[string]string)
				g.variables = append(g.variables, ref.variable)
			}
			g.plurals[ref.variable][ref.category] = values[ref]
		}
	}

	var b strings.Builder
	b.WriteString(stringsdictHeader)
	for _, g := range groups {
		b.WriteString("\t<key>")
		b.WriteString(encodePlistText(g.key))
		b.WriteString("</key>\n\t<dict>\n")
		format := g.format
		if !g.hasFormat && len(g.variables) > 0 {
			format = "%#@" + g.variables[0] + "@"
		}
		b.WriteString("\t\t<key>NSStringLocalizedFormatKey</key>\n\t\t<string>")
		b.WriteString(encodePlistText(format))
		b.WriteString("</string>\n")
		for _, v := range g.variables {
			b.WriteString("\t\t<key>")
			b.WriteString(encodePlistText(v))
			b.WriteString("</key>\n\t\t<dict>\n")
			b.WriteString("\t\t\t<key>NSStringFormatSpecTypeKey</key>\n\t\t\t<string>NSStringPluralRuleType</string>\n")
			b.WriteString("\t\t\t<key>NSStringFormatValueTypeKey</key>\n\t\t\t<string>d</string>\n")
			cats := g.plurals[v]
			for _, cat := range orderedCategories(cats) {
				b.WriteString("\t\t\t<key>")
				b.WriteString(cat)
				b.WriteString("</key>\n\t\t\t<string>")
				b.WriteString(encodePlistText(cats[cat]))
				b.WriteString("</string>\n")
			}
			b.WriteString("\t\t</dict>\n")
		}
		b.WriteString("\t</dict>\n")
	}
	b.WriteString(stringsdictFooter)
	return []byte(b.String())
}

// orderedCategories returns CLDR plural categories in canonical order, omitting
// those absent from the map.
func orderedCategories(m map[string]string) []string {
	canonical := []string{"zero", "one", "two", "few", "many", "other"}
	var out []string
	for _, c := range canonical {
		if _, ok := m[c]; ok {
			out = append(out, c)
		}
	}
	// Any non-canonical keys (shouldn't happen) appended sorted for determinism.
	var extra []string
	for c := range m {
		if !cldrPluralKeys[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

const stringsdictHeader = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`

const stringsdictFooter = `</dict>
</plist>
`
