package comment

import (
	"bytes"
	"strings"
)

// LineLayout is how a comment made of line comments sets its text out, read
// from the comment as written so that a rewrite keeps it. That is the marker
// each line opens with, the indentation before the marker on each line after
// the first, what sits between the marker and the text, how an empty line is
// written, and the line ending. Rendering a comment's own text in its layout
// reproduces the comment's bytes.
type LineLayout struct {
	markers Markers
	marker  string
	eol     string
	// indent opens each line after the first. after reports a comment that
	// follows code on its line, which holds one line of text.
	indent string
	after  bool
	// prefix follows the marker on each line that holds text, and blank follows
	// it on an empty line.
	prefix, blank string
	text          []string
}

// ParseLineLayout reads the layout of the line comment c, located in src and
// written with the line markers of m. A comment whose lines open with
// different markers or at different indentation, or whose empty lines or line
// endings are written in more than one way, has no layout to keep and is
// refused as RefusedLayout.
func ParseLineLayout(src []byte, c Comment, m Markers) (*LineLayout, error) {
	span := string(src[c.Start:c.End])
	l := &LineLayout{markers: m, eol: "\n"}
	for _, marker := range m.Line {
		if marker != "" && strings.HasPrefix(span, marker) && len(marker) > len(l.marker) {
			l.marker = marker
		}
	}
	if l.marker == "" {
		return nil, refuse(RefusedLayout, "the comment does not open with a line comment marker")
	}
	lineStart := bytes.LastIndexByte(src[:c.Start], '\n') + 1
	l.indent = string(src[lineStart:c.Start])
	if strings.TrimLeft(l.indent, " \t") != "" {
		l.indent, l.after = "", true
	}
	if strings.Contains(span, "\r") {
		crlf := strings.Count(span, "\r\n")
		if crlf != strings.Count(span, "\r") || crlf != strings.Count(span, "\n") {
			return nil, refuse(RefusedLayout, "the comment's lines do not all end the same way, or it holds a carriage return that ends no line")
		}
		l.eol = "\r\n"
		span = strings.ReplaceAll(span, "\r\n", "\n")
	}
	lines := strings.Split(span, "\n")
	if l.after && len(lines) > 1 {
		return nil, refuse(RefusedLayout, "the comment follows code on its line and runs onto the lines after it")
	}
	bodies := make([]string, len(lines))
	for i, line := range lines {
		if i > 0 {
			rest, ok := strings.CutPrefix(line, l.indent)
			if !ok || strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t") {
				return nil, refuse(RefusedLayout, "the comment's lines are indented in more than one way")
			}
			line = rest
		}
		body, ok := strings.CutPrefix(line, l.marker)
		if !ok {
			return nil, refuse(RefusedLayout, "the comment's lines open with more than one marker")
		}
		bodies[i] = body
	}

	prefixed := false
	for _, body := range bodies {
		if !l.holdsText(body) {
			continue
		}
		if !prefixed {
			l.prefix, prefixed = l.decoration(body), true
			continue
		}
		l.prefix = sharedPrefix(l.prefix, body)
	}
	if !prefixed {
		return nil, refuse(RefusedLayout, "the comment holds no text")
	}
	blankSeen := false
	for _, body := range bodies {
		if l.holdsText(body) {
			l.text = append(l.text, body[len(l.prefix):])
			continue
		}
		if blankSeen && body != l.blank {
			return nil, refuse(RefusedLayout, "the empty lines between the comment's paragraphs are written in more than one way (%q and %q)", l.marker+l.blank, l.marker+body)
		}
		l.blank, blankSeen = body, true
		l.text = append(l.text, "")
	}
	if !blankSeen {
		l.blank = strings.TrimRight(l.prefix, " \t")
	}
	return l, nil
}

// decoration is what a line holds between its marker and its text: the
// marker's last character written again, as `///` writes `//` longer, then
// spaces and tabs.
func (l *LineLayout) decoration(body string) string {
	last := l.marker[len(l.marker)-1:]
	n := len(body) - len(strings.TrimLeft(body, last))
	return body[:n+len(leadingBlanks(body[n:]))]
}

// holdsText reports whether something other than white space follows a line's
// decoration.
func (l *LineLayout) holdsText(body string) bool {
	return strings.TrimSpace(body[len(l.decoration(body)):]) != ""
}

// Text is the comment's text: one string per line, without the markers or what
// sits between each marker and its text.
func (l *LineLayout) Text() string {
	return strings.Join(l.text, "\n")
}

// Render lays lines out in the layout, one comment line for each. A line
// holding only white space is written as the layout writes an empty line. A
// comment after code holds one line of text, since a second line would be a
// comment of its own, and a line of text that would read back as an empty line
// or run onto the next line of the file is refused.
func (l *LineLayout) Render(lines []string) ([]byte, error) {
	if l.after && len(lines) != 1 {
		return nil, refuse(RefusedText, "the comment follows code on its line and holds one line of text; a second line would be a comment of its own")
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			out[i] = l.marker + l.blank
			continue
		}
		body := l.prefix + line
		if !l.holdsText(body) {
			return nil, refuse(RefusedText, "line %d of the text holds only what the comment's markers are written with, which this comment's layout reads as an empty line", i+1)
		}
		if s := l.markers.Splice; s != "" && strings.HasSuffix(strings.TrimRight(line, " \t\f\v"), s) {
			return nil, refuse(RefusedText, "line %d of the text ends in %q, which carries the comment onto the next line of the file", i+1, s)
		}
		out[i] = l.marker + body
	}
	return []byte(strings.Join(out, l.eol+l.indent)), nil
}
