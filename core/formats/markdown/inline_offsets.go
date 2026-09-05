package markdown

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

// Inline nodes record their source position unevenly: a Text node and a
// RawHTML node carry segments, every other inline node carries none and is
// located from its children or its neighbours. inlineNodeStart and
// inlineNodeEnd resolve the byte range an inline node's spelling occupies,
// which is what the reader needs to replay a spelling the parser discarded:
// the angle brackets of an autolink, the delimiters of a link title.
//
// Siblings are contiguous, so a node starts where its previous sibling ends,
// or where its parent's content starts; a node ends where its last child's
// spelling ends plus its own closing markup. ok is false when a node on the
// path records nothing that pins it down, and callers fall back to the
// spelling the parser resolved.

// inlineNodeStart returns the absolute offset at which n's source spelling
// begins.
func inlineNodeStart(n ast.Node, source []byte) (int, bool) {
	if prev := n.PreviousSibling(); prev != nil {
		return inlineNodeEnd(prev, source)
	}
	parent := n.Parent()
	if parent == nil {
		return 0, false
	}
	if parent.Type() == ast.TypeBlock {
		lines := parent.Lines()
		if lines == nil || lines.Len() == 0 {
			return 0, false
		}
		return lines.At(0).Start, true
	}
	start, ok := inlineNodeStart(parent, source)
	if !ok {
		return 0, false
	}
	opener, ok := inlineOpenerLen(parent, source)
	if !ok {
		return 0, false
	}
	return start + opener, true
}

// inlineOpenerLen returns the length of the markup that opens n before its
// first child.
func inlineOpenerLen(n ast.Node, source []byte) (int, bool) {
	switch v := n.(type) {
	case *ast.Emphasis:
		return v.Level, true
	case *ast.Link:
		return 1, true
	case *ast.Image:
		return 2, true
	case *ast.CodeSpan:
		open, _ := codeSpanFences(v, source)
		return len(open), true
	}
	if n.Kind() == east.KindStrikethrough {
		open, _ := strikethroughFences(n, source)
		return len(open), true
	}
	return 0, false
}

// inlineNodeEnd returns the absolute offset just past n's source spelling.
func inlineNodeEnd(n ast.Node, source []byte) (int, bool) {
	switch v := n.(type) {
	case *ast.Text:
		return textNodeEnd(v, source), true
	case *ast.RawHTML:
		if v.Segments == nil || v.Segments.Len() == 0 {
			return 0, false
		}
		return v.Segments.At(v.Segments.Len() - 1).Stop, true
	case *ast.CodeSpan:
		last, ok := v.LastChild().(*ast.Text)
		if !ok {
			return 0, false
		}
		_, closeMarker := codeSpanFences(v, source)
		return last.Segment.Stop + len(closeMarker), true
	case *ast.Emphasis:
		end, ok := lastChildEnd(v, source)
		if !ok {
			return 0, false
		}
		return end + v.Level, true
	case *ast.AutoLink:
		start, ok := inlineNodeStart(v, source)
		if !ok {
			return 0, false
		}
		spelling, _, ok := autoLinkSpellingAt(v, source, start)
		if !ok {
			return 0, false
		}
		return start + len(spelling), true
	case *ast.Link:
		return linkNodeEnd(v, v.Reference, 1, source)
	case *ast.Image:
		return linkNodeEnd(v, v.Reference, 2, source)
	case *east.TaskCheckBox:
		return taskCheckBoxEnd(v, source)
	}
	if n.Kind() == east.KindStrikethrough {
		end, ok := lastChildEnd(n, source)
		if !ok {
			return 0, false
		}
		_, closeMarker := strikethroughFences(n, source)
		return end + len(closeMarker), true
	}
	return 0, false
}

// textNodeEnd returns the offset just past a Text node and the line break it
// carries: the continuation bytes of a soft break, or the spelling of a hard
// break and the prefix of the line after it.
func textNodeEnd(t *ast.Text, source []byte) int {
	stop := t.Segment.Stop
	switch {
	case t.SoftLineBreak():
		return stop + len(softBreakContinuation(source, stop))
	case t.HardLineBreak():
		if _, nl, ok := hardBreakSpelling(source, stop); ok {
			return nl + len(softBreakContinuation(source, nl))
		}
	}
	return stop
}

func lastChildEnd(n ast.Node, source []byte) (int, bool) {
	last := n.LastChild()
	if last == nil {
		return 0, false
	}
	return inlineNodeEnd(last, source)
}

// linkContentEnd returns the offset of the `]` that closes a link's or
// image's text: past its last child, or past the opener when it has none.
func linkContentEnd(n ast.Node, openerLen int, source []byte) (int, bool) {
	if n.FirstChild() != nil {
		end, ok := lastChildEnd(n, source)
		if !ok || end >= len(source) || source[end] != ']' {
			return 0, false
		}
		return end, true
	}
	start, ok := inlineNodeStart(n, source)
	end := start + openerLen
	if !ok || end >= len(source) || source[end] != ']' {
		return 0, false
	}
	return end, true
}

// linkNodeEnd returns the offset just past a link or image, whose text is
// closed by `]` and then by the reference label or the inline destination.
func linkNodeEnd(n ast.Node, ref *ast.ReferenceLink, openerLen int, source []byte) (int, bool) {
	contentEnd, ok := linkContentEnd(n, openerLen, source)
	if !ok {
		return 0, false
	}
	if ref == nil {
		closer, ok := scanInlineLinkCloser(source, contentEnd)
		if !ok {
			return 0, false
		}
		return closer.end, true
	}
	end := contentEnd + 1
	if ref.Type == ast.ReferenceLinkShortcut {
		return end, true
	}
	if end >= len(source) || source[end] != '[' {
		return 0, false
	}
	i := indexUnescaped(source, end+1, ']')
	if i < 0 {
		return 0, false
	}
	return i + 1, true
}

// taskCheckBoxEnd returns the offset just past a task-list checkbox and the
// whitespace after it. The node records no segment, but it is always the
// first thing in its item's text.
func taskCheckBoxEnd(n *east.TaskCheckBox, source []byte) (int, bool) {
	parent := n.Parent()
	if parent == nil || parent.Type() != ast.TypeBlock || parent.Lines() == nil || parent.Lines().Len() == 0 {
		return 0, false
	}
	i := parent.Lines().At(0).Start
	if i+3 > len(source) || source[i] != '[' || source[i+2] != ']' {
		return 0, false
	}
	switch source[i+1] {
	case ' ', 'x', 'X':
	default:
		return 0, false
	}
	i += 3
	for i < len(source) && (source[i] == ' ' || source[i] == '\t') {
		i++
	}
	return i, true
}

// autoLinkSourceSpelling returns an autolink as the source spells it, and
// whether that spelling is the bracketed form of CommonMark 6.5 rather than a
// bare URL the linkify extension recognised. ok is false when the node cannot
// be located.
func autoLinkSourceSpelling(n *ast.AutoLink, source []byte) (spelling string, bracketed, ok bool) {
	start, ok := inlineNodeStart(n, source)
	if !ok {
		return "", false, false
	}
	return autoLinkSpellingAt(n, source, start)
}

// autoLinkSpellingAt returns the bytes that spell the autolink at start: its
// label between angle brackets, or the bare label.
func autoLinkSpellingAt(n *ast.AutoLink, source []byte, start int) (spelling string, bracketed, ok bool) {
	if start < 0 || start > len(source) {
		return "", false, false
	}
	label := n.Label(source)
	rest := source[start:]
	if len(rest) >= len(label)+2 && rest[0] == '<' && bytes.Equal(rest[1:1+len(label)], label) && rest[1+len(label)] == '>' {
		return string(rest[:len(label)+2]), true, true
	}
	if bytes.HasPrefix(rest, label) {
		return string(label), false, true
	}
	return "", false, false
}

// inlineLinkCloser is the spelling of an inline link's or image's closing
// markup, from the `]` that ends its text to the `)` that ends the link.
type inlineLinkCloser struct {
	end        int    // offset just past the closing parenthesis
	dest       string // the destination as spelled, angle brackets included
	titleOpen  int    // offset of the title's opening delimiter, or -1
	titleClose int    // offset of the title's closing delimiter, or -1
	title      string // the raw bytes between the title's delimiters
}

// scanInlineLinkCloser reads `](destination "title")` at pos, following
// CommonMark 6.6: optional whitespace, with up to one line ending, around the
// destination and the title; a destination in angle brackets or bare with
// balanced parentheses; a title in double quotes, single quotes or
// parentheses, separated from the destination by whitespace; and backslash
// escapes throughout. ok is false when the bytes at pos are not such a closer.
func scanInlineLinkCloser(source []byte, pos int) (inlineLinkCloser, bool) {
	c := inlineLinkCloser{titleOpen: -1, titleClose: -1}
	if pos < 0 || pos+1 >= len(source) || source[pos] != ']' || source[pos+1] != '(' {
		return c, false
	}
	i := skipLinkWhitespace(source, pos+2)
	destStart := i
	if i < len(source) && source[i] == '<' {
		i++
		for i < len(source) && source[i] != '>' {
			if source[i] == '\n' || source[i] == '<' {
				return c, false
			}
			if source[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(source) {
			return c, false
		}
		i++
	} else {
		depth := 0
		for i < len(source) {
			ch := source[i]
			if ch == '\\' && i+1 < len(source) {
				i += 2
				continue
			}
			if ch <= ' ' {
				break
			}
			if ch == '(' {
				depth++
			} else if ch == ')' {
				if depth == 0 {
					break
				}
				depth--
			}
			i++
		}
		if depth != 0 {
			return c, false
		}
	}
	c.dest = string(source[destStart:i])
	destEnd := i
	i = skipLinkWhitespace(source, i)
	if i > destEnd && i < len(source) {
		var closer byte
		switch source[i] {
		case '"':
			closer = '"'
		case '\'':
			closer = '\''
		case '(':
			closer = ')'
		}
		if closer != 0 {
			j := indexUnescaped(source, i+1, closer)
			if j < 0 {
				return c, false
			}
			c.titleOpen, c.titleClose = i, j
			c.title = string(source[i+1 : j])
			i = skipLinkWhitespace(source, j+1)
		}
	}
	if i >= len(source) || source[i] != ')' {
		return c, false
	}
	c.end = i + 1
	return c, true
}

// skipLinkWhitespace returns the first offset at or after i that is not a
// space, tab or line ending.
func skipLinkWhitespace(source []byte, i int) int {
	for i < len(source) {
		switch source[i] {
		case ' ', '\t', '\r', '\n':
			i++
		default:
			return i
		}
	}
	return i
}

// indexUnescaped returns the offset of the first ch at or after from that is
// not preceded by a backslash escape, or -1.
func indexUnescaped(source []byte, from int, ch byte) int {
	for i := from; i < len(source); i++ {
		switch source[i] {
		case '\\':
			i++
		case ch:
			return i
		}
	}
	return -1
}
