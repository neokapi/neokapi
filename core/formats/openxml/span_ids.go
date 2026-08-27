package openxml

import "strconv"

// spanIDs hands out inline-code ids for one block, and remembers which open a
// close belongs to.
//
// The counter this replaces was incremented on every code, open and close
// alike, so a bold span opened as `c1` closed as `c2` and nested bold+italic
// read `c1 c2 /c3 /c4`. Nothing pairs there: the ids are positions in the
// sequence rather than the identity of a span.
//
// Every other reader in the tree already pairs. Markdown and HTML both emit
// `1 … /1` for a bold span and `2 … /2` for the link after it, which is also
// what XLIFF means by `<pc id>` and what `core/model.pairedCodesBalanced`
// checks: a close matches an earlier open of the same id.
//
// The consequence of not pairing was that `kapi apply` refused every edit to a
// block with paired formatting, including an edit whose codes were byte-
// identical to the source's, because the guard asks whether the result is
// balanced and an OOXML block never was. 14 of 26 coded blocks across this
// repo's own .docx fixtures were uneditable. See issue #2227.
//
// The zero value is ready to use.
type spanIDs struct {
	// next is the number the following freshly-allocated id takes.
	next int
	// open is the ids of spans awaiting their close, innermost last.
	//
	// A stack works because a close is always emitted for the innermost open
	// span: appendClosingRuns walks the formatting flags in reverse of
	// appendOpeningRuns, and a hyperlink close first closes any formatting
	// opened inside it.
	open []string
}

// placeholder allocates an id for a standalone code, which has no other half.
func (s *spanIDs) placeholder() string {
	s.next++
	return "c" + strconv.Itoa(s.next)
}

// openSpan allocates an id and remembers it for the matching close.
func (s *spanIDs) openSpan() string {
	id := s.placeholder()
	s.open = append(s.open, id)
	return id
}

// closeSpan returns the id of the innermost span still open.
//
// A close with nothing open means the source's markup does not nest, which a
// reader cannot repair and should not hide. It gets a fresh id, which leaves
// the block unbalanced and lets the fidelity guard refuse an edit to it rather
// than writing markup that closes something that was never opened.
func (s *spanIDs) closeSpan() string {
	if len(s.open) == 0 {
		return s.placeholder()
	}
	id := s.open[len(s.open)-1]
	s.open = s.open[:len(s.open)-1]
	return id
}
