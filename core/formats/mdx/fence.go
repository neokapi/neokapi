package mdx

// fence.go recognises CommonMark fenced code blocks for the MDX scanners.
//
// A fence is OPAQUE to document structure: no byte between an opening fence
// and its closing fence may open an MDX construct, close a JSX element, or
// begin a GFM table. Every scanner in this package that decides structure
// line by line therefore skips a fenced block whole, without looking inside.
//
// Without that rule a shell transcript such as
//
//	```
//	<binary> version
//	```
//
// reads as a top-level JSX element whose closing tag never arrives, and the
// scan swallows the rest of the document into one opaque region.

// fenceInfo describes an opening code fence: its delimiter character and the
// length of the opening run. A closing fence must use the same character and
// be at least as long (CommonMark 4.5).
type fenceInfo struct {
	char   byte // '`' or '~'
	length int
}

// openFenceAt reports the code fence opened by the line beginning at
// lineStart, if any.
//
// Per CommonMark an opening fence is up to three spaces of indent followed by
// at least three backticks or tildes; a backtick fence's info string may not
// contain a backtick (which is what keeps an inline code span on its own line
// from reading as a fence). Four or more columns of indent make the line
// indented code, not a fence — and a tab counts as four columns, so a
// tab-indented line is never a fence opener.
func openFenceAt(body []byte, lineStart int) (fenceInfo, bool) {
	i := lineStart
	for indent := 0; i < len(body) && body[i] == ' '; indent++ {
		if indent == 3 {
			return fenceInfo{}, false
		}
		i++
	}
	if i >= len(body) {
		return fenceInfo{}, false
	}
	char := body[i]
	if char != '`' && char != '~' {
		return fenceInfo{}, false
	}
	run := 0
	for i < len(body) && body[i] == char {
		run++
		i++
	}
	if run < 3 {
		return fenceInfo{}, false
	}
	if char == '`' {
		for ; i < len(body) && body[i] != '\n'; i++ {
			if body[i] == '`' {
				return fenceInfo{}, false
			}
		}
	}
	return fenceInfo{char: char, length: run}, true
}

// closesFence reports whether the line beginning at lineStart is a closing
// fence for f: up to three spaces of indent, a run of at least f.length
// delimiter characters, then nothing but spaces and tabs.
func closesFence(body []byte, lineStart int, f fenceInfo) bool {
	i := lineStart
	for indent := 0; i < len(body) && body[i] == ' '; indent++ {
		if indent == 3 {
			return false
		}
		i++
	}
	run := 0
	for i < len(body) && body[i] == f.char {
		run++
		i++
	}
	if run < f.length {
		return false
	}
	for ; i < len(body) && body[i] != '\n'; i++ {
		if body[i] != ' ' && body[i] != '\t' && body[i] != '\r' {
			return false
		}
	}
	return true
}

// fencedBlockEnd returns the index just past the fenced code block opening at
// lineStart, and whether a fence opens there at all. An unclosed fence runs to
// end of document, which CommonMark allows — it is not a parse failure.
func fencedBlockEnd(body []byte, lineStart int) (int, bool) {
	f, ok := openFenceAt(body, lineStart)
	if !ok {
		return 0, false
	}
	for i := nextLine(body, lineStart); i < len(body); i = nextLine(body, i) {
		if closesFence(body, i, f) {
			return nextLine(body, i), true
		}
	}
	return len(body), true
}

// atLineStart reports whether pos begins a physical line — the only position
// at which a fence may open.
func atLineStart(body []byte, pos int) bool {
	return pos == 0 || (pos <= len(body) && body[pos-1] == '\n')
}

// fenceRegionAt returns the end of a fenced code block opening exactly at
// pos, and whether one opens there. It is the form the character-oriented
// scanners want: positions that are not line starts never open a fence.
func fenceRegionAt(body []byte, pos int) (int, bool) {
	if !atLineStart(body, pos) {
		return 0, false
	}
	return fencedBlockEnd(body, pos)
}

// codeSpanEnd returns the index just past the Markdown inline code span
// opening at pos, and whether one opens there.
//
// A code span is a run of backticks closed by a run of exactly the same
// length (CommonMark 6.1). It may cross a line break but not a blank line,
// and an unclosed run is ordinary text. Like a fence, a code span is opaque:
// `--target <file>` in an element's Markdown children is prose about a
// placeholder, not a tag that leaves the element unbalanced.
func codeSpanEnd(body []byte, pos int) (int, bool) {
	if pos >= len(body) || body[pos] != '`' {
		return 0, false
	}
	open := 0
	i := pos
	for i < len(body) && body[i] == '`' {
		open++
		i++
	}
	for i < len(body) {
		switch body[i] {
		case '`':
			run := 0
			j := i
			for j < len(body) && body[j] == '`' {
				run++
				j++
			}
			if run == open {
				return j, true
			}
			i = j
		case '\n':
			if blankLineAt(body, i+1) {
				return 0, false
			}
			i++
		default:
			i++
		}
	}
	return 0, false
}

// blankLineAt reports whether the line beginning at pos is blank (only
// spaces and tabs before its line ending), end of document included.
func blankLineAt(body []byte, pos int) bool {
	for i := pos; i < len(body); i++ {
		switch body[i] {
		case ' ', '\t', '\r':
		case '\n':
			return true
		default:
			return false
		}
	}
	return true
}
