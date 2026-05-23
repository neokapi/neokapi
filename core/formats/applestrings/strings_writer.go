package applestrings

import (
	"strings"
)

// rewriteStrings re-parses the original .strings bytes and splices in the
// resolved value for each entry whose key has a collected block, when the
// re-encoded value differs from the original inner bytes. Everything else
// (comments, key tokens, '=' / ';' punctuation, whitespace, BOM) is copied
// byte-for-byte, so an unmodified document round-trips identically.
func rewriteStrings(original string, values map[leafRef]string) ([]byte, error) {
	doc, err := parseStringsFile(original)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.Grow(len(original))
	cursor := 0

	for i := range doc.entries {
		e := doc.entries[i]
		ref := leafRef{key: e.key, leaf: leafValue}
		newVal, ok := values[ref]
		if !ok {
			continue // no block for this entry — leave bytes untouched
		}
		// Compare the resolved value against the entry's DECODED value, not the
		// re-encoded bytes: this keeps round-trips byte-faithful even when the
		// original used escapes that re-encode differently (e.g. \U2026 → …,
		// \/ → /). Only when the value semantically changed do we re-encode.
		if newVal == e.value {
			continue
		}
		// Copy everything up to the value's inner content, emit the encoded
		// replacement, then resume after the original inner content.
		b.WriteString(original[cursor:e.valStart])
		b.WriteString(encodeStringsValue(newVal))
		cursor = e.valEnd
	}
	b.WriteString(original[cursor:])
	return []byte(b.String()), nil
}

// buildStringsFromScratch emits a canonical .strings document when no original
// is available (a synthetic pipeline produced blocks without first reading a
// file). Only plain "value" leaves are emitted; .stringsdict leaves cannot be
// represented in a .strings file.
func buildStringsFromScratch(order []leafRef, values map[leafRef]string) []byte {
	var b strings.Builder
	for _, ref := range order {
		if ref.leaf != "" && ref.leaf != leafValue {
			continue
		}
		val := values[ref]
		b.WriteString("\"")
		b.WriteString(encodeStringsValue(ref.key))
		b.WriteString("\" = \"")
		b.WriteString(encodeStringsValue(val))
		b.WriteString("\";\n")
	}
	return []byte(b.String())
}
