package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	comment "go/doc/comment"
	"go/parser"
	goscanner "go/scanner"
	"go/token"
	"strings"
	"unicode/utf8"

	layer "github.com/neokapi/neokapi/core/comment"
)

// In this file `comment` is go/doc/comment, the doc comment syntax, and `layer`
// is the comment layer.

var _ layer.Rewriter = Provider{}

// tabColumns is how many columns a tab counts for when a line's width is
// measured.
const tabColumns = 4

// Prose implements layer.Rewriter. Each line of a line comment gives one line
// of text: what follows its `//`, less one space. A delimited comment gives
// its text as layer.Layout reads it, without the delimiters, the space beside
// them or the prefix of each line.
func (Provider) Prose(src []byte, c layer.Comment) (string, error) {
	if c.Style != layer.StyleLine {
		l, err := blockLayout(src, c)
		if err != nil {
			return "", err
		}
		return l.Text(), nil
	}
	lines := strings.Split(string(src[c.Start:c.End]), "\n")
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if i > 0 {
			line = strings.TrimLeft(line, " \t")
		}
		body, ok := strings.CutPrefix(line, "//")
		if !ok {
			return "", fmt.Errorf("line %d of the comment does not open with //", c.Lines.First+i)
		}
		lines[i] = strings.TrimPrefix(body, " ")
	}
	return strings.Join(lines, "\n"), nil
}

// Render implements layer.Rewriter.
//
// Each line of text becomes a `//` line at the comment's indentation, with the
// marker gofmt writes for it, and the file's line ending separates the lines.
// A paragraph or list item holding a line wider than the width is reflowed,
// and every other line stays as the text holds it, so a comment's own text
// renders to its own bytes. A comment in the position gofmt reformats as a doc
// comment goes through go/doc/comment's printer, as gofmt does.
//
// A comment after code on its line holds one line of text, since a second line
// would be a comment of its own. Text a comment cannot hold is refused. A
// delimited comment is rendered by renderBlock.
func (Provider) Render(name string, src []byte, c layer.Comment, text string, opts layer.RenderOptions) ([]byte, error) {
	if c.Style != layer.StyleLine {
		return renderBlock(name, src, c, text, opts)
	}
	lines, err := textLines(text)
	if err != nil {
		return nil, err
	}
	lineStart := bytes.LastIndexByte(src[:c.Start], '\n') + 1
	indent := string(src[lineStart:c.Start])
	if strings.TrimLeft(indent, " \t") != "" {
		if len(lines) > 1 {
			return nil, &layer.Refusal{Reason: layer.RefusedText,
				Detail: "the comment follows code on its line and holds one line of text; a second line would be a comment of its own"}
		}
		return []byte(marked(lines[0])), nil
	}

	width := opts.Width
	if width <= 0 {
		width = max(layer.DefaultWidth, widest(src, c))
	}
	avail := width - columns(indent) - len("// ")
	lines = wrap(lines, avail)
	if gofmtDocGroup(name, src, c) != nil {
		lines = wrap(printed(lines), avail)
		lines = printed(lines)
	}

	eol := "\n"
	if bytes.HasPrefix(src[c.End:], []byte("\r\n")) || bytes.Contains(src[c.Start:c.End], []byte("\r\n")) {
		eol = "\r\n"
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString(eol)
			b.WriteString(indent)
		}
		b.WriteString(marked(line))
	}
	return []byte(b.String()), nil
}

// goBlock is the syntax of a Go delimited comment.
var goBlock = layer.BlockMarker{Open: "/*", Close: "*/"}

// blockLayout reads the layout of a delimited comment. A comment that opens
// with a line comment, or holds more than one comment, has none. gofmt writes
// the lines of a comment it formats as a doc comment with no prefix, so such a
// comment's text keeps the indentation of its lists and code blocks.
func blockLayout(src []byte, c layer.Comment) (*layer.Layout, error) {
	span := src[c.Start:c.End]
	lineStart := bytes.LastIndexByte(src[:c.Start], '\n') + 1
	indent := string(src[lineStart:c.Start])
	if strings.TrimLeft(indent, " \t") != "" {
		indent = ""
	}
	if lineStart == c.Start && bytes.Contains(span, []byte("\n")) && !allStars(string(span)) {
		if g := gofmtDocGroup("", src, c); g != nil && len(g.List) == 1 {
			return layer.ParseLayoutWithPrefix(span, goBlock, "")
		}
	}
	return layer.ParseLayout(span, indent, goBlock)
}

// renderBlock renders text into the layout of the delimited comment c.
//
// The comment keeps its delimiters, the space beside them, the prefix of each
// line and its line ending, so a comment's own text renders to its own bytes.
// A comment on one line holds one line of text. Over several lines, a paragraph
// or list item holding a line wider than the width is reflowed as in a line
// comment. A comment gofmt reformats as a doc comment goes through
// go/doc/comment's printer, as gofmt does: the whole group in doc position, over
// several lines, without an asterisk opening each line. Text holding `*/` is
// refused, because the comment would end there.
func renderBlock(name string, src []byte, c layer.Comment, text string, opts layer.RenderOptions) ([]byte, error) {
	layout, err := blockLayout(src, c)
	if err != nil {
		return nil, err
	}
	lines, err := textLines(text)
	if err != nil {
		return nil, err
	}
	if layout.Single() {
		return layout.Render(lines)
	}
	width := opts.Width
	if width <= 0 {
		width = max(layer.DefaultWidth, widest(src, c))
	}
	avail := width - columns(layout.Prefix())
	lines = wrap(lines, avail)
	span, err := layout.Render(lines)
	if err != nil {
		return nil, err
	}
	if g := gofmtDocGroup(name, src, c); g != nil && len(g.List) == 1 && !allStars(string(span)) {
		lines = wrap(printed(lines), avail)
		return layout.Render(printed(lines))
	}
	return span, nil
}

// allStars is go/printer's test for an old-style delimited comment, one in
// which every line after the first opens with an asterisk once its spaces and
// tabs are skipped. gofmt leaves the text of such a doc comment as it is.
func allStars(text string) bool {
	for i := range len(text) {
		if text[i] == '\n' {
			j := i + 1
			for j < len(text) && (text[j] == ' ' || text[j] == '\t') {
				j++
			}
			if j < len(text) && text[j] != '*' {
				return false
			}
		}
	}
	return true
}

// marked is one line of text with the comment marker gofmt writes for it.
func marked(line string) string {
	switch {
	case line == "":
		return "//"
	case strings.HasPrefix(line, "\t"):
		return "//" + line
	default:
		return "// " + line
	}
}

// textLines splits text into lines, refusing what a Go comment must not hold
// and dropping blank lines at either edge.
func textLines(text string) ([]string, error) {
	if !utf8.ValidString(text) {
		return nil, &layer.Refusal{Reason: layer.RefusedText, Detail: "the text is not valid UTF-8"}
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, r := range text {
		if reason := unwritable(r); reason != "" {
			return nil, &layer.Refusal{Reason: layer.RefusedText, Detail: fmt.Sprintf("the text holds %s (U+%04X)", reason, r)}
		}
	}
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil, &layer.Refusal{Reason: layer.RefusedText, Detail: "the text is empty, and a comment is rewritten here, never removed"}
	}
	return lines, nil
}

// unwritable names what r is when a comment must not hold it, and returns ""
// for a character it may hold.
func unwritable(r rune) string {
	switch {
	case r == '\n' || r == '\t':
		return ""
	case r == '\r':
		return "a carriage return that ends no line"
	case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
		return "a control character"
	case r == '\uFEFF':
		return "a byte order mark"
	case r == '\u2028' || r == '\u2029':
		return "a line or paragraph separator, which an editor may show as a line break"
	case (r >= '\u202A' && r <= '\u202E') || (r >= '\u2066' && r <= '\u2069'):
		return "a bidirectional control, which can make the code around the comment display out of order"
	}
	return ""
}

// widest is the width of the comment's widest line, from the start of its
// line.
func widest(src []byte, c layer.Comment) int {
	lineStart := bytes.LastIndexByte(src[:c.Start], '\n') + 1
	w := 0
	for line := range strings.SplitSeq(string(src[lineStart:c.End]), "\n") {
		w = max(w, columns(strings.TrimSuffix(line, "\r")))
	}
	return w
}

// columns is the width of s, counting a tab as tabColumns.
func columns(s string) int {
	n := 0
	for _, r := range s {
		if r == '\t' {
			n += tabColumns
		} else {
			n++
		}
	}
	return n
}

// printed is lines as go/doc/comment's printer writes them, which is what
// gofmt writes for a doc comment.
func printed(lines []string) []string {
	var p comment.Parser
	var pr comment.Printer
	out := strings.TrimSuffix(string(pr.Comment(p.Parse(strings.Join(lines, "\n")))), "\n")
	return strings.Split(out, "\n")
}

// gofmtDocGroup returns the comment group c sits in when gofmt reformats that
// group as a doc comment, and nil otherwise. It is go/printer's own test: a
// group that starts a line, ends directly above the next token, follows no
// import keyword, and precedes a token that is not an identifier.
func gofmtDocGroup(name string, src []byte, c layer.Comment) *ast.CommentGroup {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	tf := fset.File(f.Pos())
	var group *ast.CommentGroup
	for _, g := range f.Comments {
		if tf.Offset(g.Pos()) <= c.Start && c.Start < tf.Offset(g.End()) {
			group = g
			break
		}
	}
	if group == nil || tf.PositionFor(group.Pos(), false).Column != 1 {
		return nil
	}
	var s goscanner.Scanner
	s.Init(tf, src, nil, 0)
	prev := token.ILLEGAL
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return nil
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}
		if pos > group.End() {
			if tok != token.IDENT && prev != token.IMPORT && pos == group.End()+1 {
				return group
			}
			return nil
		}
		prev = tok
	}
}

// wrap reflows the paragraphs and list items in lines that hold a line wider
// than avail columns, and keeps every other line as it is.
//
// The spans are go/doc/comment's: a paragraph is a run of lines that are not
// indented, and an indented span is a list when its first line opens with a
// list marker and a code block otherwise. A code block is never reflowed. A
// paragraph is reflowed only when a blank line, a list or the end of the text
// follows it, and a list only when no line opening with a brace does, which
// keeps both out of reach of the parser's heuristics for misindented code.
func wrap(lines []string, avail int) []string {
	prefix := commonIndent(lines)
	body := make([]string, len(lines))
	for i, line := range lines {
		body[i] = strings.TrimPrefix(line, prefix)
		if strings.TrimSpace(body[i]) == "" {
			body[i] = ""
		}
	}
	avail -= columns(prefix)

	var out []string
	for i := 0; i < len(body); {
		if body[i] == "" {
			out = append(out, lines[i])
			i++
			continue
		}
		start := i
		indentedSpan := indented(body[i])
		if indentedSpan {
			// An indented span runs across blank lines to the next line that is
			// not indented, and ends on its last line that is not blank.
			for i < len(body) && (body[i] == "" || indented(body[i])) {
				i++
			}
			for body[i-1] == "" {
				i--
			}
		} else {
			for i < len(body) && body[i] != "" && !indented(body[i]) {
				i++
			}
		}
		span := body[start:i]
		fits := true
		for _, line := range span {
			if columns(line) > avail {
				fits = false
			}
		}
		next := ""
		if i < len(body) {
			next = body[i]
		}
		switch {
		case fits:
			out = append(out, lines[start:i]...)
		case !indentedSpan && (next == "" || isListLine(next)) && !(start > 0 && strings.HasPrefix(span[0], "}")):
			out = append(out, prefixed(prefix, reflowParagraph(span, avail))...)
		case indentedSpan && isListLine(span[0]) && !strings.HasPrefix(next, "}"):
			out = append(out, prefixed(prefix, reflowList(span, avail))...)
		default:
			out = append(out, lines[start:i]...)
		}
	}
	return out
}

func prefixed(prefix string, lines []string) []string {
	for i := range lines {
		if lines[i] != "" {
			lines[i] = prefix + lines[i]
		}
	}
	return lines
}

// commonIndent is the indentation every line that is not blank opens with,
// which go/doc/comment removes before it reads the text.
func commonIndent(lines []string) string {
	prefix, first := "", true
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if first {
			prefix, first = lead, false
			continue
		}
		for !strings.HasPrefix(lead, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

func indented(line string) bool {
	return line != "" && (line[0] == ' ' || line[0] == '\t')
}

// reflowParagraph lays a paragraph's words out in lines of at most avail
// columns. A heading and a paragraph of link definitions are kept as they are.
func reflowParagraph(span []string, avail int) []string {
	if len(span) == 1 && strings.HasPrefix(span[0], "# ") {
		return span
	}
	allLinks := true
	for _, line := range span {
		if !isLinkDef(line) {
			allLinks = false
		}
	}
	if allLinks {
		return span
	}
	return fill(words(strings.Join(span, " ")), "", "", avail)
}

// reflowList lays out each item of a list whose lines are wider than avail,
// with its continuation lines indented under the item's text. Blank lines and
// the lines of an item after a blank line are kept as they are.
func reflowList(span []string, avail int) []string {
	var out []string
	for i := 0; i < len(span); {
		if !isListLine(span[i]) {
			out = append(out, span[i])
			i++
			continue
		}
		start := i
		i++
		for i < len(span) && span[i] != "" && !isListLine(span[i]) {
			i++
		}
		item := span[start:i]
		fits := true
		for _, line := range item {
			if columns(line) > avail {
				fits = false
			}
		}
		if fits {
			out = append(out, item...)
			continue
		}
		first := item[0]
		lead := first[:len(first)-len(strings.TrimLeft(first, " \t"))]
		rest := strings.TrimLeft(first, " \t")
		marker, text, _ := strings.Cut(rest, " ")
		open := lead + marker + " "
		cont := lead + strings.Repeat(" ", utf8.RuneCountInString(marker)+1)
		all := []string{text}
		for _, line := range item[1:] {
			all = append(all, strings.TrimSpace(line))
		}
		out = append(out, fill(words(strings.Join(all, " ")), open, cont, avail)...)
	}
	return out
}

// fill lays words out greedily in lines of at most avail columns, the first
// opening with open and the rest with cont. A word wider than the line sits on
// a line of its own. A word that would read as a list marker at the start of a
// line stays on the line before it.
func fill(ws []string, open, cont string, avail int) []string {
	var out []string
	line := open
	empty := true
	for _, w := range ws {
		switch {
		case empty:
			line += w
			empty = false
		case columns(line)+1+columns(w) <= avail || isListLine(w+" x"):
			line += " " + w
		default:
			out = append(out, line)
			line = cont + w
		}
	}
	return append(out, line)
}

// words splits text at spaces and tabs. The words of a bracketed link text
// stay together, so a link never breaks across lines.
func words(text string) []string {
	var out []string
	open := false
	for w := range strings.FieldsFuncSeq(text, func(r rune) bool { return r == ' ' || r == '\t' }) {
		if open {
			out[len(out)-1] += " " + w
		} else {
			out = append(out, w)
		}
		switch {
		case strings.Contains(w, "]"):
			open = false
		case strings.Contains(w, "["):
			open = true
		}
	}
	return out
}

// isListLine is go/doc/comment's test for a line that opens a list item: a
// bullet, or a number and a full stop or parenthesis, then a space or tab, then
// text.
func isListLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	var rest string
	if r, n := utf8.DecodeRuneInString(line); r == '•' || r == '*' || r == '+' || r == '-' {
		rest = line[n:]
	} else if line[0] >= '0' && line[0] <= '9' {
		n := 1
		for n < len(line) && line[n] >= '0' && line[n] <= '9' {
			n++
		}
		if n >= len(line) || (line[n] != '.' && line[n] != ')') {
			return false
		}
		rest = line[n+1:]
	} else {
		return false
	}
	return indented(rest) && strings.TrimSpace(rest) != ""
}

// isLinkDef reports a line that defines a link: `[text]: url`.
func isLinkDef(line string) bool {
	if !strings.HasPrefix(line, "[") {
		return false
	}
	i := strings.Index(line, "]:")
	return i > 1 && strings.TrimSpace(line[i+2:]) != ""
}
