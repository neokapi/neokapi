//go:build parity

package roundtrip

import (
	"bytes"
	"regexp"
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
