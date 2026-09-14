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
	// Splice, when set, carries a line comment whose line ends in it, spaces
	// and tabs aside, onto the next line, as a backslash does in C.
	Splice string
}

// BlockMarker opens and closes a delimited comment. A nested one, as in Rust,
// holds comments of its own and closes only once each comment opened inside it
// has closed.
type BlockMarker struct {
	Open, Close string
	Nested      bool
}

// LineText reads one comment line as Provider.LineText describes. A line
// comment is whole to the end of the line unless a splice ends the line, and a
// delimited comment is whole only when it closes on the line it opens on. Line
// markers are tried first, then block markers, each in order.
func (m Markers) LineText(line []byte) (int, string, bool) {
	s := string(line)
	for _, marker := range m.Line {
		if body, ok := strings.CutPrefix(s, marker); ok && marker != "" {
			if m.Splice != "" && strings.HasSuffix(strings.TrimRight(s, " \t\f\v\r"), m.Splice) {
				return 0, "", false
			}
			return len(s), body, true
		}
	}
	for _, b := range m.Block {
		body, ok := strings.CutPrefix(s, b.Open)
		if !ok || b.Open == "" || b.Close == "" {
			continue
		}
		if i := closeAt(body, b); i >= 0 {
			return len(b.Open) + i + len(b.Close), body[:i], true
		}
		return 0, "", false
	}
	return 0, "", false
}

// closeAt returns where b closes in body, the text after its opener, or -1.
func closeAt(body string, b BlockMarker) int {
	if !b.Nested {
		return strings.Index(body, b.Close)
	}
	depth := 1
	for i := 0; i < len(body); {
		switch {
		case strings.HasPrefix(body[i:], b.Close):
			depth--
			if depth == 0 {
				return i
			}
			i += len(b.Close)
		case strings.HasPrefix(body[i:], b.Open):
			depth++
			i += len(b.Open)
		default:
			i++
		}
	}
	return -1
}
