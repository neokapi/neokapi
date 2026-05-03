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

	yaml "gopkg.in/yaml.v3"
)

// yamlDecoder is a tiny wrapper for the canonical normalizer's parse
// step — pulled out so future tweaks (e.g. KnownFields, custom tags)
// have a single home.
func yamlDecoder(in []byte) *yaml.Decoder {
	return yaml.NewDecoder(bytes.NewReader(in))
}

// yamlEncoder mirrors yamlDecoder for the re-serialize step. Indent is
// fixed at 2 (YAML's most common default) so re-emitted output is
// deterministic regardless of the source's indentation choice.
func yamlEncoder(w io.Writer) *yaml.Encoder {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc
}

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

// StripBOM removes a leading UTF-8 byte-order mark (`\xef\xbb\xbf`).
// Used by formats whose source has a BOM that one engine preserves
// and another drops — both readings are valid for UTF-8 (the BOM is
// optional) but differ byte-for-byte.
type StripBOM struct{}

// Name implements Normalizer.
func (StripBOM) Name() string { return "strip-bom" }

// Normalize implements Normalizer.
func (StripBOM) Normalize(in []byte) ([]byte, error) {
	if bytes.HasPrefix(in, []byte("\xef\xbb\xbf")) {
		return in[3:], nil
	}
	return in, nil
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
		// Drop the XML declaration entirely. encoding/xml refuses to
		// encode a `<?xml ... ?>` ProcInst (it expects callers to
		// produce it themselves at the top of the stream), and for
		// canonical comparison we'd want to discard any declaration
		// difference anyway — the underlying structure is what matters.
		if pi, ok := tok.(xml.ProcInst); ok && pi.Target == "xml" {
			continue
		}
		// Drop DOCTYPE directives. They're metadata about the document
		// (which DTD applies, internal subset declarations) and a
		// canonical structural comparison shouldn't care whether the
		// source had `<!DOCTYPE TS>` or `<!DOCTYPE TS []>` — both
		// validate against the same external schema. Emitters often
		// disagree on the empty-internal-subset bracket form.
		if d, ok := tok.(xml.Directive); ok {
			trimmed := bytes.TrimLeft(d, " \t\r\n")
			if bytes.HasPrefix(trimmed, []byte("DOCTYPE")) {
				continue
			}
		}
		// Drop CharData runs that consist of nothing but ASCII
		// whitespace — different writers indent differently and the
		// inter-element whitespace isn't significant to XML semantics
		// (excepting xml:space="preserve", which encoding/xml's
		// tokenizer doesn't propagate to us so we conservatively
		// always strip).
		if cd, ok := tok.(xml.CharData); ok {
			trimmed := bytes.TrimRight(bytes.TrimLeft(cd, " \t\r\n"), " \t\r\n")
			if len(trimmed) == 0 {
				continue
			}
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

// YAMLCanonical re-serializes YAML through gopkg.in/yaml.v3 so two
// documents that differ in flow vs block style, indentation,
// quote-character choice, or boolean/null spelling come out identical
// (or close enough that surviving differences are real).
//
// Important caveat: YAML's "what is a string vs keyword" decisions are
// driven by the source spelling — `true` is a boolean, `"true"` is a
// string. If native preserves `true` while okapi pseudo-translates it
// as `ţŕũē` (treating it as a translatable string), those are real
// semantic differences the normalizer can't bridge — both sides will
// re-serialize them differently. The normalizer only collapses
// stylistic noise.
type YAMLCanonical struct{}

// Name implements Normalizer.
func (YAMLCanonical) Name() string { return "yaml-canonical" }

// Normalize implements Normalizer.
func (YAMLCanonical) Normalize(in []byte) ([]byte, error) {
	// Parse via yaml.Decoder so multi-document streams (separated by
	// `---`) round-trip correctly — each doc gets its own re-emit.
	dec := yamlDecoder(in)
	var buf bytes.Buffer
	enc := yamlEncoder(&buf)
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("yaml-canonical: parse: %w", err)
		}
		if err := enc.Encode(v); err != nil {
			return nil, fmt.Errorf("yaml-canonical: re-serialize: %w", err)
		}
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("yaml-canonical: flush: %w", err)
	}
	return buf.Bytes(), nil
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
