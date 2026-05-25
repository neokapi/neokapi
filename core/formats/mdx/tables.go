package mdx

import "bytes"

// mdSubSpan is a sub-range of a Markdown span, tagged with whether it is a
// GFM table block. Table blocks are preserved verbatim (opaque) because
// the markdown reader normalises table cell padding for Okapi parity
// rather than preserving the source bytes, which would break the MDX
// byte-faithful round-trip. Non-table sub-spans are delegated to the
// markdown reader for prose extraction.
type mdSubSpan struct {
	isTable bool
	start   int // relative to the Markdown span
	end     int
}

// splitMarkdownTables partitions a Markdown span into an ordered, gap-free
// list of sub-spans, isolating GFM table blocks. Concatenating
// span[s.start:s.end] over the result reproduces span exactly.
//
// A GFM table is recognised as a header row line containing an unescaped
// `|`, immediately followed by a delimiter row (pipes, colons, dashes,
// spaces — with at least one dash), then zero or more body rows that
// contain a `|`. The block ends at the first line that is blank or is not
// a table row. Tables must start at column 0 or with up to three spaces of
// indent (CommonMark leading-indent tolerance); more indent makes it a
// code block, handled by ordinary Markdown delegation.
func splitMarkdownTables(span []byte) []mdSubSpan {
	var subs []mdSubSpan
	nonTableStart := 0
	i := 0
	n := len(span)

	flushNonTable := func(upto int) {
		if upto > nonTableStart {
			subs = append(subs, mdSubSpan{start: nonTableStart, end: upto})
		}
	}

	for i < n {
		lineStart := i
		lineEnd := lineEndAt(span, lineStart)
		nextStart := lineEnd
		if nextStart < n && span[nextStart] == '\n' {
			nextStart++
		}

		// Candidate header: this line has a pipe and the NEXT line is a
		// delimiter row.
		if lineHasUnescapedPipe(span[lineStart:lineEnd]) && nextStart < n {
			delimEnd := lineEndAt(span, nextStart)
			if isTableDelimiterRow(span[nextStart:delimEnd]) {
				// Found a table starting at lineStart. Consume header +
				// delimiter + body rows.
				flushNonTable(lineStart)
				tableEnd := consumeTableBody(span, nextStart)
				subs = append(subs, mdSubSpan{isTable: true, start: lineStart, end: tableEnd})
				i = tableEnd
				nonTableStart = tableEnd
				continue
			}
		}

		// Advance to next line.
		i = nextStart
		if nextStart == lineStart {
			// No progress (empty trailing) — guard against infinite loop.
			i = n
		}
	}

	flushNonTable(n)
	return subs
}

// consumeTableBody returns the end index (exclusive) of a table whose
// delimiter row starts at delimStart. It consumes the delimiter row and
// every following line that looks like a table body row (contains a pipe
// and is not blank), stopping at the first blank/non-row line.
func consumeTableBody(span []byte, delimStart int) int {
	n := len(span)
	// Skip the delimiter row itself.
	i := lineEndAt(span, delimStart)
	if i < n && span[i] == '\n' {
		i++
	}
	for i < n {
		lineEnd := lineEndAt(span, i)
		line := span[i:lineEnd]
		if isBlankBytes(line) || !lineHasUnescapedPipe(line) {
			break
		}
		i = lineEnd
		if i < n && span[i] == '\n' {
			i++
		}
	}
	return i
}

// lineEndAt returns the index of the LF terminating the line at lineStart
// (or len(span) at EOF) — i.e. the line content end, exclusive of the LF.
func lineEndAt(span []byte, lineStart int) int {
	i := lineStart
	for i < len(span) && span[i] != '\n' {
		i++
	}
	return i
}

// isBlankBytes reports whether b is empty or only spaces/tabs/CR.
func isBlankBytes(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}
	return true
}

// lineHasUnescapedPipe reports whether the line contains a `|` that is not
// backslash-escaped. Leading indent of up to three spaces is tolerated
// (more indent is a code block, not a table).
func lineHasUnescapedPipe(line []byte) bool {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return false
	}
	for i := indent; i < len(line); i++ {
		if line[i] == '|' {
			// Count preceding backslashes; an odd count means escaped.
			bs := 0
			for j := i - 1; j >= 0 && line[j] == '\\'; j-- {
				bs++
			}
			if bs%2 == 0 {
				return true
			}
		}
	}
	return false
}

// isTableDelimiterRow reports whether line is a GFM table delimiter row:
// after stripping up to three leading spaces and an optional leading/
// trailing `|`, every cell is a run of dashes optionally bracketed by
// alignment colons, and there is at least one dash overall.
func isTableDelimiterRow(line []byte) bool {
	trimmed := bytes.TrimRight(line, " \t\r")
	// Strip up to three leading spaces.
	indent := 0
	for indent < len(trimmed) && trimmed[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return false
	}
	body := trimmed[indent:]
	if len(body) == 0 {
		return false
	}
	hasDash := false
	hasPipe := false
	for _, c := range body {
		switch c {
		case '|':
			hasPipe = true
		case '-':
			hasDash = true
		case ':', ' ', '\t':
		default:
			return false
		}
	}
	// A valid delimiter row needs at least one dash; a single-column table
	// without outer pipes still needs the dash, and GFM requires a pipe to
	// separate columns — but a one-column table can be `---` with framing
	// pipes. Require either a pipe or that the whole row is dashes/colons.
	return hasDash && (hasPipe || onlyDashColon(body))
}

// onlyDashColon reports whether body consists solely of dashes, colons,
// and inner spaces (a pipe-less single-column delimiter like `:---:`).
func onlyDashColon(body []byte) bool {
	for _, c := range body {
		if c != '-' && c != ':' && c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}
