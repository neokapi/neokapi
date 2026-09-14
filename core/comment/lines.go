package comment

import "github.com/neokapi/neokapi/core/format"

// LineKind is what one line of a file holds, as its comment layer reads it.
type LineKind int

const (
	// LineBlank holds nothing but whitespace.
	LineBlank LineKind = iota
	// LineCode holds text outside every comment, whatever else it holds.
	LineCode
	// LineComment holds a comment's prose, or a blank line of a comment, and
	// nothing outside comments.
	LineComment
	// LinePackageDoc holds the doc comment of the package or module the file
	// belongs to (PackageDoc), and nothing outside comments.
	LinePackageDoc
	// LineSetAside holds only comments the provider set aside for a reason
	// other than being blank, such as a directive.
	LineSetAside
)

// LineKinds classifies each line of src by the comments f located in it. The
// result is indexed by line number, so index 0 is unused and the last index is
// the file's last line. A line holding any byte that is not whitespace outside
// every comment and exclusion is code, so a comment after code on its line
// leaves the line code.
func (f *File) LineKinds(src []byte) []LineKind {
	idx := format.NewLineIndex(src)
	last := idx.Line(len(src))
	kinds := make([]LineKind, last+1)
	covered := make([]LineKind, len(src))
	mark := func(start, end int, k LineKind) {
		for i := max(start, 0); i < min(end, len(src)); i++ {
			if covered[i] < k || covered[i] == LineCode {
				covered[i] = k
			}
		}
	}
	for i := range covered {
		covered[i] = LineCode
	}
	for _, e := range f.Excluded {
		k := LineSetAside
		if e.Reason == ReasonBlank {
			k = LineComment
		}
		mark(e.Start, e.End, k)
	}
	for _, c := range f.Comments {
		k := LineComment
		if c.Doc && PackageDoc(c.Subject) {
			k = LinePackageDoc
		}
		mark(c.Start, c.End, k)
	}
	for i, b := range src {
		if isSpace(b) {
			continue
		}
		line := idx.Line(i)
		switch k := covered[i]; {
		case k == LineCode:
			kinds[line] = LineCode
		case kinds[line] == LineCode:
		case k > kinds[line]:
			kinds[line] = k
		}
	}
	return kinds
}
