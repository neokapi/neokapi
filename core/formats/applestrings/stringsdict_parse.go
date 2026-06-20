package applestrings

import (
	"errors"
	"strings"
)

// This file walks the .stringsdict plist token stream and models its
// translatable <string> leaves. A .stringsdict maps each top-level key to a
// "format dict" containing one NSStringLocalizedFormatKey format string plus
// one sub-dict per variable. Each variable sub-dict carries
// NSStringFormatSpecTypeKey / NSStringFormatValueTypeKey metadata and a set of
// CLDR plural keys (zero/one/two/few/many/other) whose <string> values are the
// translatable plural forms.
//
// Apple stringsdict variable spec keys (not translatable, but recorded):
//
//	NSStringLocalizedFormatKey  – the top-level format string (%#@var@ tokens)
//	NSStringFormatSpecTypeKey   – e.g. "NSStringPluralRuleType"
//	NSStringFormatValueTypeKey  – e.g. "d", "lld", "@"
//
// CLDR plural categories that hold translatable values:
var cldrPluralKeys = map[string]bool{
	"zero": true, "one": true, "two": true,
	"few": true, "many": true, "other": true,
}

// dictLeafKind classifies a translatable <string> leaf inside a stringsdict.
type dictLeafKind string

const (
	// leafFormatKey is the NSStringLocalizedFormatKey format string for a
	// top-level entry. It is translatable (translators may reorder %#@var@
	// tokens and surrounding text) but typically left unchanged.
	leafFormatKey dictLeafKind = "format"
	// leafPlural is a CLDR plural-category value inside a variable sub-dict.
	leafPlural dictLeafKind = "plural"
)

// dictLeaf addresses one translatable <string> value inside the stringsdict and
// records the token span of its value content so the writer can splice a
// changed translation while preserving every other byte.
type dictLeaf struct {
	topKey   string       // the top-level entry key
	variable string       // variable name (sub-dict key); "" for the format key
	category string       // CLDR plural category for leafPlural; "" otherwise
	kind     dictLeafKind // leafFormatKey or leafPlural
	value    string       // decoded <string> value

	// Token span of the <string> element holding this value. The content the
	// writer rewrites is the tokens strictly between strStart and strEnd. When
	// strStart == strEnd the element is empty (<string></string>) with no
	// inner token.
	strStart int // index of the <string> start tag in the token slice
	strEnd   int // index of the matching </string> end tag

	// specType / valueType are the variable's NSStringFormatSpecTypeKey and
	// NSStringFormatValueTypeKey values (recorded for tooling; not edited).
	specType  string
	valueType string
}

// stringsdictDoc is the lossless parse of a whole .stringsdict file.
type stringsdictDoc struct {
	toks  []plistToken
	leafs []dictLeaf
}

// commentTexts returns the trimmed inner text of every XML comment
// (<!-- ... -->) in the document. Such comments are tokenized losslessly (they
// round-trip verbatim in the skeleton) but never belong to a translatable
// <string> leaf, so the reader surfaces them as layer-scoped NoteAnnotations to
// keep the developer/context text reachable as structured metadata. Empty
// comments are skipped.
func (doc *stringsdictDoc) commentTexts() []string {
	var out []string
	for _, t := range doc.toks {
		if t.kind != plTokComment {
			continue
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(t.raw, "<!--"), "-->")
		if inner = strings.TrimSpace(inner); inner != "" {
			out = append(out, inner)
		}
	}
	return out
}

// parseStringsdict tokenizes and walks the .stringsdict source, producing the
// token stream (for byte-faithful rewrite) and the list of translatable
// <string> leaves in document order.
func parseStringsdict(input string) (*stringsdictDoc, error) {
	toks, err := newPlistTokenizer(input).tokenize()
	if err != nil {
		return nil, err
	}
	doc := &stringsdictDoc{toks: toks}

	// Locate the root <plist>'s top-level <dict>.
	rootDict := findFirstElement(toks, 0, "dict")
	if rootDict < 0 {
		return doc, nil // no dict — nothing translatable, but bytes round-trip
	}
	rootEnd := matchPlistEnd(toks, rootDict, "dict")
	if rootEnd < 0 {
		return nil, errors.New("applestrings: unterminated root <dict>")
	}

	// Walk the root dict's key/value pairs. Each value is a "format dict".
	pairs := dictPairs(toks, rootDict, rootEnd)
	for _, p := range pairs {
		if toks[p.valStart].kind != plTokStartTag || toks[p.valStart].name != "dict" {
			continue // value not a dict — skip (still round-trips)
		}
		doc.walkFormatDict(toks, p.key, p.valStart, p.valEnd)
	}

	return doc, nil
}

// walkFormatDict processes one top-level entry's format dict, collecting the
// NSStringLocalizedFormatKey leaf and recursing into variable sub-dicts for
// plural leaves.
func (doc *stringsdictDoc) walkFormatDict(toks []plistToken, topKey string, dictStart, dictEnd int) {
	pairs := dictPairs(toks, dictStart, dictEnd)
	for _, p := range pairs {
		switch p.key {
		case "NSStringLocalizedFormatKey":
			if toks[p.valStart].kind == plTokStartTag && toks[p.valStart].name == "string" {
				end := matchPlistEnd(toks, p.valStart, "string")
				doc.leafs = append(doc.leafs, dictLeaf{
					topKey:   topKey,
					kind:     leafFormatKey,
					value:    elementText(toks, p.valStart, end),
					strStart: p.valStart,
					strEnd:   end,
				})
			}
		default:
			// A variable sub-dict.
			if toks[p.valStart].kind == plTokStartTag && toks[p.valStart].name == "dict" {
				doc.walkVariableDict(toks, topKey, p.key, p.valStart, p.valEnd)
			}
		}
	}
}

// walkVariableDict processes one variable sub-dict: it records the spec/value
// type and emits a leaf for each CLDR plural-category <string>.
func (doc *stringsdictDoc) walkVariableDict(toks []plistToken, topKey, variable string, dictStart, dictEnd int) {
	pairs := dictPairs(toks, dictStart, dictEnd)
	var specType, valueType string
	for _, p := range pairs {
		if toks[p.valStart].kind != plTokStartTag || toks[p.valStart].name != "string" {
			continue
		}
		end := matchPlistEnd(toks, p.valStart, "string")
		text := elementText(toks, p.valStart, end)
		switch p.key {
		case "NSStringFormatSpecTypeKey":
			specType = text
		case "NSStringFormatValueTypeKey":
			valueType = text
		}
	}
	for _, p := range pairs {
		if !cldrPluralKeys[p.key] {
			continue
		}
		if toks[p.valStart].kind != plTokStartTag || toks[p.valStart].name != "string" {
			continue
		}
		end := matchPlistEnd(toks, p.valStart, "string")
		doc.leafs = append(doc.leafs, dictLeaf{
			topKey:    topKey,
			variable:  variable,
			category:  p.key,
			kind:      leafPlural,
			value:     elementText(toks, p.valStart, end),
			strStart:  p.valStart,
			strEnd:    end,
			specType:  specType,
			valueType: valueType,
		})
	}
}

// kvPair is a <key>…</key> followed by its value element span inside a dict.
type kvPair struct {
	key      string
	valStart int // index of the value element's start tag
	valEnd   int // index of the value element's matching end tag (== valStart for self-close)
}

// dictPairs returns the direct child key/value pairs of the <dict> whose start
// tag is toks[dictStart] (dictEnd is its matching </dict>). It pairs each
// depth-1 <key> with the next depth-1 element. Self-closing values (e.g.
// <true/>) map valStart==valEnd.
func dictPairs(toks []plistToken, dictStart, dictEnd int) []kvPair {
	var pairs []kvPair
	depth := 0
	i := dictStart
	var pendingKey string
	havePendingKey := false
	for i <= dictEnd {
		t := toks[i]
		switch t.kind {
		case plTokStartTag:
			depth++
			if depth == 2 {
				if t.name == "key" {
					end := matchPlistEnd(toks, i, "key")
					pendingKey = elementText(toks, i, end)
					havePendingKey = true
					i = end
					depth-- // we consumed through the end tag
					i++
					continue
				}
				if havePendingKey {
					end := matchPlistEnd(toks, i, t.name)
					pairs = append(pairs, kvPair{key: pendingKey, valStart: i, valEnd: end})
					havePendingKey = false
					i = end
					depth--
					i++
					continue
				}
			}
		case plTokSelfClose:
			if depth == 1 && havePendingKey {
				pairs = append(pairs, kvPair{key: pendingKey, valStart: i, valEnd: i})
				havePendingKey = false
			}
		case plTokEndTag:
			depth--
		}
		i++
	}
	return pairs
}

// findFirstElement returns the index of the first start tag with the given name
// at or after startIdx, or -1.
func findFirstElement(toks []plistToken, startIdx int, name string) int {
	for i := startIdx; i < len(toks); i++ {
		if (toks[i].kind == plTokStartTag || toks[i].kind == plTokSelfClose) && toks[i].name == name {
			return i
		}
	}
	return -1
}

// matchPlistEnd returns the index of the end tag matching the start tag at
// toks[startIdx], honouring nesting. A self-closing start tag matches itself.
func matchPlistEnd(toks []plistToken, startIdx int, name string) int {
	if toks[startIdx].kind == plTokSelfClose {
		return startIdx
	}
	depth := 0
	for i := startIdx; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.kind == plTokStartTag && t.name == name:
			depth++
		case t.kind == plTokEndTag && t.name == name:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// elementText returns the decoded character data of the element spanning
// toks[startIdx]..toks[endIdx] (its own start and end tags). Text tokens are
// entity-decoded; CDATA inner bytes are included verbatim.
func elementText(toks []plistToken, startIdx, endIdx int) string {
	if endIdx <= startIdx {
		return ""
	}
	var b strings.Builder
	for i := startIdx + 1; i < endIdx; i++ {
		t := toks[i]
		switch t.kind {
		case plTokText:
			b.WriteString(decodePlistText(t.raw))
		case plTokCDATA:
			inner := strings.TrimSuffix(strings.TrimPrefix(t.raw, "<![CDATA["), "]]>")
			b.WriteString(inner)
		}
	}
	return b.String()
}
