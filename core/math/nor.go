package math

import (
	"bytes"
	"encoding/xml"
	"io"
)

// norText is one <m:nor/>-flagged <m:t> span's text plus the byte range of its
// CharData within the namespace-wrapped OMML (see scanNorTexts).
type norText struct {
	text       string
	start, end int
}

// scanNorTexts streams the wrapped OMML and invokes cb for each <m:t> CharData
// that sits inside an <m:r> whose <m:rPr> carries <m:nor/> — i.e. the
// natural-language (non-math) text spans. Offsets are into wrapped.
func scanNorTexts(wrapped []byte, cb func(norText)) {
	dec := xml.NewDecoder(bytes.NewReader(wrapped))
	inRun, isNor, inMT := false, false, false
	mtStart := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF { //nolint:errorlint // decoder returns io.EOF sentinel directly
				return
			}
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != mathNS {
				continue
			}
			switch t.Name.Local {
			case "r":
				inRun, isNor = true, false
			case "nor":
				if inRun {
					isNor = true
				}
			case "t":
				if inRun {
					inMT = true
					mtStart = int(dec.InputOffset()) // just after <m:t>
				}
			}
		case xml.CharData:
			if inMT {
				if isNor {
					cb(norText{text: string(t), start: mtStart, end: int(dec.InputOffset())})
				}
				inMT = false
			}
		case xml.EndElement:
			if t.Name.Space != mathNS {
				continue
			}
			switch t.Name.Local {
			case "r":
				inRun, isNor = false, false
			case "t":
				inMT = false
			}
		}
	}
}

// wrapOMML wraps a namespace-less OMML fragment in a root binding the m: and w:
// prefixes, returning the wrapped bytes and the prefix length.
func wrapOMML(raw []byte) (wrapped []byte, prefixLen int) {
	prefix := []byte(`<ommlRoot xmlns:m="` + mathNS + `" xmlns:w="` + wprNS + `">`)
	wrapped = make([]byte, 0, len(prefix)+len(raw)+len(ommlSuffix))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, raw...)
	wrapped = append(wrapped, ommlSuffix...)
	return wrapped, len(prefix)
}

var ommlSuffix = []byte(`</ommlRoot>`)

// NorTexts returns the literal text of each <m:nor/> run's <m:t> in document
// order — the natural-language prose embedded in an equation. Empty when the
// equation is pure math typography.
func NorTexts(raw []byte) []string {
	wrapped, _ := wrapOMML(raw)
	var out []string
	scanNorTexts(wrapped, func(n norText) { out = append(out, n.text) })
	return out
}

// SpliceNorText returns raw OMML with each <m:nor/> <m:t> CharData replaced by
// the corresponding entry of translations (by document order). An empty entry —
// or a translations slice shorter than the number of spans — leaves that span's
// text verbatim. Every other byte is preserved exactly, so a nil/all-empty
// translations slice returns raw unchanged (byte-exact).
func SpliceNorText(raw []byte, translations []string) []byte {
	wrapped, prefixLen := wrapOMML(raw)
	type repl struct {
		start, end int
		text       string
	}
	var repls []repl
	idx := 0
	scanNorTexts(wrapped, func(n norText) {
		if idx < len(translations) && translations[idx] != "" && translations[idx] != n.text {
			repls = append(repls, repl{n.start, n.end, esc(translations[idx])})
		}
		idx++
	})
	if len(repls) == 0 {
		return raw
	}
	out := make([]byte, 0, len(wrapped))
	cursor := 0
	for _, r := range repls {
		out = append(out, wrapped[cursor:r.start]...)
		out = append(out, r.text...)
		cursor = r.end
	}
	out = append(out, wrapped[cursor:]...)
	// Strip the synthetic wrapper to return the modified fragment.
	return out[prefixLen : len(out)-len(ommlSuffix)]
}
