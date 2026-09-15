package comment

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

// Layout is how one delimited comment sets its text out between its
// delimiters, read from the comment as written, so that a rewrite keeps it.
//
// Three layouts are common. The whole comment sits on one line, as in
// `/* text */`. Or the opener and the closer sit on lines of their own, with
// each text line between them opening with ` * `, or bare. The text may also
// start on the opener's line or end on the closer's.
//
// A layout holds everything but the text. That is the delimiters as written, the
// space between a delimiter and the text beside it, and the prefix each text
// line below the opener's line opens with. It is also how an empty line between
// two paragraphs is written, the blank lines above the first text line and below
// the last, and the line ending. Rendering a comment's own text in its layout
// reproduces the comment's bytes.
type Layout struct {
	// marker is the comment syntax, and open the opener as written, which may
	// draw the marker's opener longer with asterisks, as `/**` does.
	marker BlockMarker
	open   string
	eol    string
	// single reports a comment on one line.
	single bool
	// textOnOpen reports text on the opener's line, after padOpen. Otherwise
	// openTrail is what follows the opener on its line.
	textOnOpen bool
	padOpen    string
	openTrail  string
	// lead holds the blank lines between an opener alone and the first text
	// line, and trail those between the last text line and a closer alone, each
	// as written.
	lead, trail []string
	// prefix opens each text line below the opener's line, and blank is an empty
	// line between two text lines.
	prefix, blank string
	// closeAlone reports a closer on a line of its own, after closeLead.
	// Otherwise the closer follows the last text line, after padClose.
	closeAlone bool
	closeLead  string
	padClose   string
	text       []string
}

// ParseLayout reads the layout of the delimited comment span holds, written
// with m. indent is what precedes the comment on its line when only spaces and
// tabs do, and empty otherwise. A span that is not one comment written with m,
// or whose empty lines or line endings are written in more than one way, has no
// layout to keep and is refused as RefusedLayout.
func ParseLayout(span []byte, indent string, m BlockMarker) (*Layout, error) {
	return readLayout(span, indent, m, nil)
}

// ParseLayoutWithPrefix reads the layout as ParseLayout does, with the prefix
// of each text line below the opener's line given rather than read. A language
// whose formatter writes a comment's lines with a known prefix reads the
// comment this way, as gofmt writes the lines of a doc comment with none, so
// the indentation of a list or a code block stays in the comment's text.
func ParseLayoutWithPrefix(span []byte, m BlockMarker, prefix string) (*Layout, error) {
	return readLayout(span, "", m, &prefix)
}

func readLayout(span []byte, indent string, m BlockMarker, prefix *string) (*Layout, error) {
	l, err := parseLayout(span, indent, m, true, prefix)
	if errors.Is(err, errNoText) {
		// The asterisks after the opener were the comment's text, as in `/** */`.
		l, err = parseLayout(span, indent, m, false, prefix)
	}
	if errors.Is(err, errNoText) {
		return nil, refuse(RefusedLayout, "the comment holds no text")
	}
	return l, err
}

// errNoText is a comment read as holding no text.
var errNoText = errors.New("the comment holds no text")

// parseLayout reads a layout as ParseLayout does. With drawn set, asterisks
// directly after an opener that ends in one are read as part of the opener.
// A prefix that is not nil is the prefix of each text line.
func parseLayout(span []byte, indent string, m BlockMarker, drawn bool, prefix *string) (*Layout, error) {
	s := string(span)
	if m.Open == "" || m.Close == "" || len(s) < len(m.Open)+len(m.Close) || !strings.HasPrefix(s, m.Open) || !strings.HasSuffix(s, m.Close) ||
		closeAt(s[len(m.Open):], m) != len(s)-len(m.Open)-len(m.Close) {
		return nil, refuse(RefusedLayout, "the comment is not one %s %s comment: a comment made of several comments, or of delimited and line comments together, has no one layout to keep", m.Open, m.Close)
	}
	if !utf8.ValidString(s) {
		return nil, refuse(RefusedLayout, "the comment is not valid UTF-8")
	}
	l := &Layout{marker: m, open: m.Open, eol: "\n"}
	body := s[len(m.Open) : len(s)-len(m.Close)]
	if drawn && strings.HasSuffix(m.Open, "*") {
		if stars := len(body) - len(strings.TrimLeft(body, "*")); stars < len(body) {
			l.open += body[:stars]
			body = body[stars:]
		}
	}
	if strings.Contains(body, "\r") {
		crlf := strings.Count(body, "\r\n")
		if crlf != strings.Count(body, "\r") || crlf != strings.Count(body, "\n") {
			return nil, refuse(RefusedLayout, "the comment's lines do not all end the same way, or it holds a carriage return that ends no line")
		}
		l.eol = "\r\n"
		body = strings.ReplaceAll(body, "\r\n", "\n")
	}

	lines := strings.Split(body, "\n")
	if len(lines) == 1 {
		if strings.Trim(body, " \t") == "" {
			return nil, errNoText
		}
		l.single = true
		l.padOpen = leadingBlanks(body)
		l.padClose = body[len(strings.TrimRight(body, " \t")):]
		l.text = []string{body[len(l.padOpen) : len(body)-len(l.padClose)]}
		return l, nil
	}

	first, last := lines[0], lines[len(lines)-1]
	if strings.Trim(first, " \t") == "" {
		l.openTrail = first
	} else {
		l.textOnOpen = true
		l.padOpen = leadingBlanks(first)
	}
	below := slices.Clone(lines[1 : len(lines)-1])
	if strings.Trim(last, " \t*") == "" {
		l.closeAlone = true
		l.closeLead = last
	} else {
		l.padClose = last[len(strings.TrimRight(last, " \t")):]
		below = append(below, last[:len(last)-len(l.padClose)])
	}

	// A line below the opener's holds text when something other than white
	// space follows its decoration. The prefix is what every such line opens
	// with, and never runs past a line's decoration, so no line's text starts
	// inside it.
	holdsText := func(line string) bool {
		return strings.TrimSpace(line[len(decoration(line)):]) != ""
	}
	prefixed := prefix != nil
	if prefixed {
		l.prefix = *prefix
	} else {
		for _, line := range below {
			if !holdsText(line) {
				continue
			}
			if !prefixed {
				l.prefix, prefixed = decoration(line), true
				continue
			}
			l.prefix = sharedPrefix(l.prefix, line)
		}
	}
	if !prefixed {
		if !l.textOnOpen {
			return nil, errNoText
		}
		// A line added below text that starts on the opener's line is aligned
		// under that text.
		l.prefix = indent + strings.Repeat(" ", len(l.open)) + l.padOpen
	}

	if l.textOnOpen {
		l.text = append(l.text, first[len(l.padOpen):])
	}
	start, end := 0, len(below)
	if !l.textOnOpen {
		for start < end && !holdsText(below[start]) {
			start++
		}
		l.lead = below[:start]
	}
	if l.closeAlone {
		for end > start && !holdsText(below[end-1]) {
			end--
		}
		l.trail = below[end:]
	}
	blankSeen := false
	for _, line := range below[start:end] {
		if holdsText(line) {
			if !strings.HasPrefix(line, l.prefix) {
				return nil, refuse(RefusedLayout, "a line of the comment does not open with the prefix %q its other lines open with", l.prefix)
			}
			l.text = append(l.text, line[len(l.prefix):])
			continue
		}
		if blankSeen && line != l.blank {
			return nil, refuse(RefusedLayout, "the empty lines between the comment's paragraphs are written in more than one way (%q and %q)", l.blank, line)
		}
		l.blank, blankSeen = line, true
		l.text = append(l.text, "")
	}
	if len(l.text) == 0 {
		return nil, errNoText
	}
	if !blankSeen {
		l.blank = strings.TrimRight(l.prefix, " \t")
	}
	return l, nil
}

// Text is the comment's text: one string per line, without the delimiters, the
// space beside them, or the prefix of each line.
func (l *Layout) Text() string {
	return strings.Join(l.text, "\n")
}

// Single reports whether the comment sits on one line.
func (l *Layout) Single() bool { return l.single }

// Prefix is what each text line below the opener's line opens with, which
// counts toward the width of that line.
func (l *Layout) Prefix() string { return l.prefix }

// Render lays lines out in the layout. Each line is one line of text, and a
// line holding only white space is written as the layout writes an empty line.
// A comment on one line holds one line of text.
//
// Text that closes the comment before its closer is refused as
// RefusedTerminator: a delimited comment ends at the first closer, so what
// followed it would be read as code. So is text that opens a comment inside a
// comment that nests, which would leave it unclosed. There is no escape for the
// closer inside a comment in the languages that use one, and a rewrite that
// changed the text to avoid it would write prose nobody wrote.
func (l *Layout) Render(lines []string) ([]byte, error) {
	for i, line := range lines {
		if !l.marker.Nested && strings.Contains(line, l.marker.Close) {
			return nil, refuse(RefusedTerminator, "line %d of the text holds %q, which ends the comment there and turns the rest of the text into code; write it another way", i+1, l.marker.Close)
		}
	}
	var span string
	if l.single {
		if len(lines) != 1 {
			return nil, refuse(RefusedText, "the comment holds one line of text between its delimiters, and a second line would change its layout")
		}
		span = l.open + l.padOpen + lines[0] + l.padClose + l.marker.Close
	} else {
		out := make([]string, 0, len(lines)+len(l.lead)+len(l.trail)+2)
		rest := lines
		if l.textOnOpen {
			out = append(out, l.open+l.padOpen+lines[0])
			rest = lines[1:]
		} else {
			out = append(out, l.open+l.openTrail)
			out = append(out, l.lead...)
		}
		for i, line := range rest {
			if strings.TrimSpace(line) == "" {
				out = append(out, l.blank)
				continue
			}
			// A line whose text is only what a decoration holds would read back as
			// an empty line, and never as the text.
			full := l.prefix + line
			if strings.TrimSpace(full[len(decoration(full)):]) == "" {
				return nil, refuse(RefusedText, "line %d of the text holds only asterisks and white space, which this comment's layout reads as an empty line", len(lines)-len(rest)+i+1)
			}
			out = append(out, full)
		}
		if l.closeAlone {
			out = append(out, l.trail...)
			out = append(out, l.closeLead+l.marker.Close)
		} else {
			out[len(out)-1] += l.padClose + l.marker.Close
		}
		span = strings.Join(out, l.eol)
	}
	// The text and the layout around it can meet in a closer, as a prefix that
	// ends in an asterisk does with a line that opens with a slash.
	if closeAt(span[len(l.marker.Open):], l.marker) != len(span)-len(l.marker.Open)-len(l.marker.Close) {
		return nil, refuse(RefusedTerminator, "the text closes the comment before its closer, where it meets the comment's delimiters or line prefix; write it another way")
	}
	return []byte(span), nil
}

// leadingBlanks is the run of spaces and tabs s opens with.
func leadingBlanks(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

// decoration is what a line of a delimited comment opens with before its text:
// spaces and tabs, then at most one asterisk, then spaces and tabs.
func decoration(line string) string {
	n := len(leadingBlanks(line))
	if n < len(line) && line[n] == '*' {
		n++
		n += len(leadingBlanks(line[n:]))
	}
	return line[:n]
}

// sharedPrefix is the longest prefix a and b share.
func sharedPrefix(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return a[:i]
}
