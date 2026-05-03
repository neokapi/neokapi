//go:build parity

package roundtrip

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// LowerHexUnicodeEscape normalizes `\uXXXX` escapes to lowercase hex.
// Used by formats whose writers emit different-case hex escapes that
// are semantically identical (Okapi tikal: lowercase; native Go
// fmt.Sprintf("%04X"): uppercase).
type LowerHexUnicodeEscape struct{}

var lowerHexEscapeRE = regexp.MustCompile(`\\u[0-9A-Fa-f]{4}`)

// Name implements Normalizer.
func (LowerHexUnicodeEscape) Name() string { return "lower-hex-unicode-escape" }

// Normalize implements Normalizer.
func (LowerHexUnicodeEscape) Normalize(in []byte) ([]byte, error) {
	return lowerHexEscapeRE.ReplaceAllFunc(in, func(b []byte) []byte {
		return []byte(strings.ToLower(string(b)))
	}), nil
}

// LFLineEndings collapses CRLF and bare CR to LF. Used by formats
// whose writers emit one line ending and okapi emits another — both
// are valid "newline" but differ byte-for-byte.
type LFLineEndings struct{}

// Name implements Normalizer.
func (LFLineEndings) Name() string { return "lf-line-endings" }

// Normalize implements Normalizer.
func (LFLineEndings) Normalize(in []byte) ([]byte, error) {
	out := bytes.ReplaceAll(in, []byte("\r\n"), []byte("\n"))
	out = bytes.ReplaceAll(out, []byte("\r"), []byte("\n"))
	return out, nil
}

// XMLCanonical re-serializes XML through encoding/xml.Decoder +
// encoding/xml.Encoder so two semantically-equivalent documents that
// differ only in attribute ordering, namespace prefix style, or
// non-significant whitespace come out byte-identical.
//
// Important caveat: encoding/xml mangles namespaces (`xmlns:its` becomes
// `_xmlns:its` plus an extra `xmlns:_xmlns="xmlns"` attribute, prefixed
// elements get re-emitted as default-namespace elements). This makes
// the normalizer's output unsuitable for human reading or downstream
// XML tools — but for parity comparison it's fine: both got and ref
// get mangled in the same deterministic way, so the mangling cancels
// out and we're left comparing the underlying structure.
//
// Use this on formats whose semantic shape matches okapi's but whose
// byte shape never will (xml, xliff, ttml, …) — reaches
// TierCanonicalEqual when the underlying XML structure agrees.
type XMLCanonical struct {
	// SortAttrs reorders each element's attributes alphabetically by
	// (namespace, local name) before re-emitting. Useful when okapi
	// reorders attributes vs. native preserves source order.
	SortAttrs bool
}

// Name implements Normalizer.
func (n XMLCanonical) Name() string {
	if n.SortAttrs {
		return "xml-canonical(sort-attrs)"
	}
	return "xml-canonical"
}

// Normalize implements Normalizer.
func (n XMLCanonical) Normalize(in []byte) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(in))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml-canonical: decode: %w", err)
		}
		if n.SortAttrs {
			if se, ok := tok.(xml.StartElement); ok {
				attrs := make([]xml.Attr, len(se.Attr))
				copy(attrs, se.Attr)
				sort.SliceStable(attrs, func(i, j int) bool {
					if attrs[i].Name.Space != attrs[j].Name.Space {
						return attrs[i].Name.Space < attrs[j].Name.Space
					}
					return attrs[i].Name.Local < attrs[j].Name.Local
				})
				se.Attr = attrs
				tok = se
			}
		}
		if err := enc.EncodeToken(tok); err != nil {
			return nil, fmt.Errorf("xml-canonical: encode: %w", err)
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("xml-canonical: flush: %w", err)
	}
	return buf.Bytes(), nil
}

// JSONCanonical re-serializes JSON through encoding/json so two
// semantically-identical documents that differ in whitespace (e.g.
// `"key" : "value"` vs `"key": "value"`) come out byte-equal. The
// re-serialization uses encoding/json's compact form (no extra
// whitespace) and HTML-escapes `<`, `>`, `&` — both got and ref get
// the same treatment, so the difference cancels.
//
// Use on json fixtures whose source has unusual spacing around colons
// or whose pretty-printer differs from okapi's.
type JSONCanonical struct{}

// Name implements Normalizer.
func (JSONCanonical) Name() string { return "json-canonical" }

// Normalize implements Normalizer.
func (JSONCanonical) Normalize(in []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, fmt.Errorf("json-canonical: parse: %w", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json-canonical: re-serialize: %w", err)
	}
	return out, nil
}

// Chain composes multiple normalizers, applying them in sequence.
// Each normalizer's output is fed to the next.
type Chain struct {
	Steps []Normalizer
}

// Name implements Normalizer.
func (c Chain) Name() string {
	parts := make([]string, 0, len(c.Steps))
	for _, s := range c.Steps {
		parts = append(parts, s.Name())
	}
	return "chain[" + strings.Join(parts, "+") + "]"
}

// Normalize implements Normalizer.
func (c Chain) Normalize(in []byte) ([]byte, error) {
	cur := in
	for _, s := range c.Steps {
		out, err := s.Normalize(cur)
		if err != nil {
			return nil, err
		}
		cur = out
	}
	return cur, nil
}
