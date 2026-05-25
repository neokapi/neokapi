package mdx

// segmentKind classifies a top-level region of an MDX document body.
type segmentKind int

const (
	// segMarkdown is a run of plain CommonMark content. The MDX reader
	// delegates these spans to the markdown reader/writer machinery so
	// prose, headings, lists, links, code fences, etc. extract and
	// round-trip exactly as they do for `.md`.
	segMarkdown segmentKind = iota
	// segESM is an MDX ESM statement region: a top-level `import …` or
	// `export …` block. Opaque — never translated, preserved byte-for-byte.
	segESM
	// segJSX is an MDX block-level JSX region: an element (`<Tag …>…</Tag>`,
	// `<Tag/>`), a fragment (`<>…</>`), or a closing tag at column 0 that
	// opens a balanced JSX tree. Opaque in v1 — the whole element (tags,
	// attributes, children, expressions) is preserved byte-for-byte.
	segJSX
	// segExpr is an MDX top-level expression region: a `{ … }` block at
	// column 0 (e.g. a `{/* comment */}` or `{someJsExpression}`). Opaque.
	segExpr
)

// segment is one contiguous top-level region of the MDX body, expressed
// as a half-open byte range [start,end) into the body slice.
type segment struct {
	kind  segmentKind
	start int
	end   int
}

// scanSegments splits an MDX body (front matter already stripped) into a
// flat, gap-free, ordered list of top-level segments. Every byte of body
// belongs to exactly one segment, so concatenating body[seg.start:seg.end]
// across the result reproduces body exactly — the foundation of the
// faithful byte round-trip.
//
// The scan is line-oriented, mirroring how CommonMark and MDX both decide
// block boundaries from the first non-whitespace character of a line at
// container nesting level zero:
//
//   - A line whose first non-blank content (with no leading indent — MDX
//     ESM/JSX/expression nodes must start at column 0) begins an ESM
//     statement (`import`/`export` keyword), a JSX element/fragment
//     (`<`), or an expression (`{`) opens an opaque region. The region
//     extends across continuation lines until the construct is complete
//     (brace/paren/bracket balanced for ESM and expressions, tag depth
//     back to zero for JSX) and terminated by a blank line or EOF.
//   - Everything else is Markdown and accumulates until the next opaque
//     region opens.
//
// Indented `<`/`{`/`import` lines are NOT treated as MDX constructs:
// per the MDX grammar these nodes are recognised only at the document's
// top level (no indentation), and an indented `<` is ordinary Markdown
// (e.g. inside a list item or an indented code block). Keeping them in
// the Markdown span lets the markdown machinery handle them as it would
// for `.md`.
func scanSegments(body []byte) []segment {
	var segs []segment
	mdStart := 0 // start of the current pending Markdown span
	i := 0
	n := len(body)

	flushMarkdown := func(upto int) {
		if upto > mdStart {
			segs = append(segs, segment{kind: segMarkdown, start: mdStart, end: upto})
		}
	}

	for i < n {
		lineStart := i
		// Identify the byte that begins the line's content and whether the
		// line has any leading indentation (spaces/tabs).
		j := lineStart
		for j < n && (body[j] == ' ' || body[j] == '\t') {
			j++
		}
		indented := j > lineStart
		var kind segmentKind
		opaque := false
		if !indented && j < n {
			switch {
			case isESMStart(body, j):
				kind, opaque = segESM, true
			case body[j] == '<' && isJSXStart(body, j):
				kind, opaque = segJSX, true
			case body[j] == '{':
				kind, opaque = segExpr, true
			}
		}

		if !opaque {
			// Ordinary Markdown line — advance past it and keep
			// accumulating the Markdown span.
			i = nextLine(body, lineStart)
			continue
		}

		// An opaque MDX region starts at lineStart. Flush any pending
		// Markdown before it, then consume the region's lines.
		flushMarkdown(lineStart)
		var regionEnd int
		switch kind {
		case segESM:
			regionEnd = scanESM(body, lineStart)
		case segJSX:
			regionEnd = scanJSX(body, lineStart)
		case segExpr:
			regionEnd = scanExpr(body, lineStart)
		}
		segs = append(segs, segment{kind: kind, start: lineStart, end: regionEnd})
		i = regionEnd
		mdStart = regionEnd
	}

	flushMarkdown(n)
	return segs
}

// nextLine returns the index just past the newline terminating the line
// that starts at lineStart, or len(body) at EOF.
func nextLine(body []byte, lineStart int) int {
	i := lineStart
	for i < len(body) && body[i] != '\n' {
		i++
	}
	if i < len(body) {
		i++ // include the LF
	}
	return i
}

// isESMStart reports whether position p begins an MDX ESM statement —
// the keyword `import` or `export` followed by whitespace, EOL, or a `{`.
// MDX only recognises ESM at column 0, which the caller has already
// verified.
func isESMStart(body []byte, p int) bool {
	for _, kw := range []string{"import", "export"} {
		if hasWordAt(body, p, kw) {
			after := p + len(kw)
			if after >= len(body) {
				return true
			}
			c := body[after]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '{' || c == '*' || c == '(' {
				return true
			}
		}
	}
	return false
}

// hasWordAt reports whether body has the exact bytes of word at p.
func hasWordAt(body []byte, p int, word string) bool {
	if p+len(word) > len(body) {
		return false
	}
	return string(body[p:p+len(word)]) == word
}

// scanESM returns the end index (exclusive) of an ESM region beginning at
// lineStart. The region spans every continuation line until the JS becomes
// brace/paren/bracket balanced (then through that physical line's end), so
// multi-line imports/exports such as
//
//	import {
//	  A,
//	  B,
//	} from "x";
//
// are captured as one opaque region. A bracket-free statement (e.g.
// `import "./side-effect";`) completes at its first line end. String and
// template literals and line/block comments are respected so brackets
// inside them don't affect the balance.
func scanESM(body []byte, lineStart int) int {
	return scanBalancedBlock(body, lineStart)
}

// scanExpr returns the end index of a top-level `{ … }` expression region
// beginning at lineStart, balancing braces (and respecting strings and
// comments) and ending through the physical line on which the depth first
// returns to zero.
func scanExpr(body []byte, lineStart int) int {
	return scanBalancedBlock(body, lineStart)
}

// scanBalancedBlock consumes lines from lineStart, tracking nesting of
// (){}[ ] across the JS-flavoured content (skipping strings, template
// literals, and comments), and returns the index just past the line on
// which the running depth first returns to zero. If depth never returns
// to zero (malformed input), it consumes through EOF. After the depth
// hits zero mid-line, the remainder of that physical line is included so
// trailing `;` and inline comments stay with the region.
func scanBalancedBlock(body []byte, lineStart int) int {
	s := newJSScanner(body, lineStart)
	depth := 0
	started := false
	for s.pos < len(body) {
		tok := s.next()
		switch tok {
		case tokOpen:
			depth++
			started = true
		case tokClose:
			depth--
			if started && depth <= 0 {
				// Finish the current physical line so any trailing
				// `;`, whitespace, or `//` comment is captured.
				return nextLine(body, s.pos)
			}
		case tokNewline:
			// A statement with no brackets at all (e.g. a bare
			// `import "./side-effect";`) completes at its first line end.
			if depth <= 0 {
				return s.pos
			}
		case tokEOF:
			return len(body)
		}
	}
	return len(body)
}

// scanJSX returns the end index of a block-level JSX region beginning at
// lineStart. It tracks JSX tag depth (start tags increment, end tags
// decrement, self-closing tags are neutral) while skipping over `{ … }`
// expression containers, strings, and comments so braces and angle
// brackets inside attributes/expressions don't confuse the depth. The
// region ends just past the line on which tag depth first returns to zero
// (for a balanced element/fragment) — or, for a single self-closing tag,
// just past its `/>`-terminating line. Malformed/unterminated input is
// consumed through EOF.
func scanJSX(body []byte, lineStart int) int {
	s := newJSXScanner(body, lineStart)
	depth := 0
	opened := false
	for s.pos < len(body) {
		tok, selfClosing := s.nextTag()
		switch tok {
		case jsxStartTag:
			opened = true
			if !selfClosing {
				depth++
			} else if depth == 0 {
				// A top-level self-closing element (`<Tag … />`) is the
				// whole region. Finish its physical line.
				return endOfJSXLine(body, s.pos)
			}
		case jsxEndTag:
			depth--
			if opened && depth <= 0 {
				return endOfJSXLine(body, s.pos)
			}
		case jsxFragmentOpen:
			opened = true
			depth++
		case jsxFragmentClose:
			depth--
			if opened && depth <= 0 {
				return endOfJSXLine(body, s.pos)
			}
		case jsxEOF:
			return len(body)
		case jsxOther:
			// Stray content between tags (whitespace, text, expressions).
			// If we have not opened any tag yet this is not valid JSX —
			// bail to keep the byte stream gap-free (treat just this line
			// as the region). This should not happen because the caller
			// only enters scanJSX on a `<tag` line.
			if !opened {
				return nextLine(body, lineStart)
			}
		}
	}
	return len(body)
}

// endOfJSXLine returns the end of the physical line containing pos, but if
// the remainder of that line (after pos) is blank, it returns pos at the
// line break so trailing Markdown on the same line is not swallowed. JSX
// elements in MDX conventionally occupy whole lines, so we extend to the
// line end to capture a trailing `;`-free newline; any non-blank trailing
// content keeps the region tight at pos.
func endOfJSXLine(body []byte, pos int) int {
	// Look at the bytes between pos and the next LF.
	i := pos
	for i < len(body) && body[i] != '\n' {
		if body[i] != ' ' && body[i] != '\t' {
			// Non-blank trailing content on the same line — end the region
			// right after the JSX so the rest stays Markdown/next region.
			return pos
		}
		i++
	}
	// Trailing content is blank — include it plus the LF.
	if i < len(body) {
		i++
	}
	return i
}
