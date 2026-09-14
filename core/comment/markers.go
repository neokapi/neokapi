package comment

import "strings"

// Markers are the delimiters a language writes its comments with. A provider
// whose comments on one line are known from their delimiters alone answers
// LineText from them.
type Markers struct {
	// Line are the markers of comments that run to the end of their line, such
	// as "//". A marker that begins with another, such as "///", needs no entry
	// of its own.
	Line []string
	// Block are the delimiters of comments that close, such as "/*" and "*/".
	Block []BlockMarker
}

// BlockMarker opens and closes a delimited comment.
type BlockMarker struct {
	Open, Close string
}

// LineText reads one comment line as Provider.LineText describes. A line
// comment is whole to the end of the line, and a delimited comment is whole
// only when it closes on the line it opens on. Line markers are tried first,
// then block markers, each in order.
func (m Markers) LineText(line []byte) (int, string, bool) {
	s := string(line)
	for _, marker := range m.Line {
		if body, ok := strings.CutPrefix(s, marker); ok && marker != "" {
			return len(s), body, true
		}
	}
	for _, b := range m.Block {
		body, ok := strings.CutPrefix(s, b.Open)
		if !ok || b.Open == "" || b.Close == "" {
			continue
		}
		if i := strings.Index(body, b.Close); i >= 0 {
			return len(b.Open) + i + len(b.Close), body[:i], true
		}
		return 0, "", false
	}
	return 0, "", false
}
