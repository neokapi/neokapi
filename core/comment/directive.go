package comment

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/format"
)

// Directives are the literal markers a project's own tools read at the start of
// a comment line, such as "okapi-skip:". A provider sets aside the directives
// its language's toolchain reads; a project's own markers reach the layer only
// when a recipe declares them.
type Directives []string

// Match reports the declared marker that text opens with once its leading
// whitespace is removed. text is a comment line with its comment marker
// removed. The match is exact and case-sensitive, and the longest marker wins.
func (d Directives) Match(text string) (string, bool) {
	text = strings.TrimLeft(text, " \t")
	form := ""
	for _, m := range d {
		if len(m) > len(form) && strings.HasPrefix(text, m) {
			form = m
		}
	}
	return form, form != ""
}

// Locate locates the comments in src through p and sets aside every comment
// line that holds one whole comment opening with a declared directive, as
// ReasonDirective with the marker as its Form.
//
// A marker inside a comment splits it: the lines on each side are comments of
// their own, on the subject the whole comment sat on. Their spans and runs come
// from a second reading of the file by p, with the marker lines blanked. That
// reading must line up with the first. Away from the markers it locates the
// same comments and exclusions, with the same spans and text, and inside each
// split comment its pieces and the markers claim every byte that is not
// whitespace, each once. A reading that does not line up leaves the file's
// comments unlocated. What a comment is attached to may differ between the
// readings, because blanking a line changes what sits between a doc comment and
// its declaration, so the first reading's attachment is kept.
func Locate(p Provider, name string, src []byte, declared Directives) (*File, error) {
	first, err := p.Locate(name, src)
	if err != nil || len(declared) == 0 {
		return first, err
	}
	marks, err := markDirectives(p, name, src, first, declared)
	if err != nil || len(marks) == 0 {
		return first, err
	}
	blanked := bytes.Clone(src)
	for _, lines := range marks {
		for _, m := range lines {
			for i := m.Start; i < m.End; i++ {
				blanked[i] = ' '
			}
		}
	}
	second, err := p.Locate(name, blanked)
	if err != nil {
		return nil, unlocated("%s with its declared directives blanked: %v", name, err)
	}
	return splitAtDirectives(name, src, first, second, marks)
}

func unlocated(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrUnlocated}, args...)...)
}

// markDirectives finds the declared directives in each comment, keyed by the
// comment's index.
func markDirectives(p Provider, name string, src []byte, f *File, declared Directives) (map[int][]Excluded, error) {
	idx := format.NewLineIndex(src)
	marks := map[int][]Excluded{}
	for ci, c := range f.Comments {
		for start := c.Start; start < c.End; {
			lineEnd := len(src)
			if i := bytes.IndexByte(src[start:], '\n'); i >= 0 {
				lineEnd = start + i
			}
			line := bytes.TrimSuffix(src[start:lineEnd], []byte("\r"))
			at := start + len(line) - len(bytes.TrimLeft(line, " \t"))
			n, text, ok := p.LineText(src[at : start+len(line)])
			if ok {
				if n <= 0 || at+n > c.End {
					return nil, unlocated("%s: line %d holds a comment of %d bytes, outside the comment located on lines %d-%d",
						name, idx.Line(at), n, c.Lines.First, c.Lines.Last)
				}
				if form, match := declared.Match(text); match {
					marks[ci] = append(marks[ci], Excluded{
						Start: at, End: at + n, Lines: idx.Range(at, at+n), Reason: ReasonDirective, Form: form,
					})
				}
			}
			start = lineEnd + 1
		}
	}
	return marks, nil
}

// splitAtDirectives replaces each comment holding a marker with the pieces the
// second reading located inside it, once that reading is shown to line up.
func splitAtDirectives(name string, src []byte, first, second *File, marks map[int][]Excluded) (*File, error) {
	// splitIn returns the split comment a span lies in, or -1 for none. ok is
	// false for a span that crosses the edge of one.
	splitIn := func(start, end int) (ci int, ok bool) {
		for ci := range marks {
			c := first.Comments[ci]
			if start >= c.Start && end <= c.End {
				return ci, true
			}
			if start < c.End && end > c.Start {
				return 0, false
			}
		}
		return -1, true
	}

	out := &File{Language: first.Language}
	var keptFirst, keptSecond []Comment
	for ci, c := range first.Comments {
		if _, split := marks[ci]; !split {
			keptFirst = append(keptFirst, c)
		}
	}
	claims := map[int][][2]int{}
	for _, c := range second.Comments {
		ci, ok := splitIn(c.Start, c.End)
		switch {
		case !ok:
			return nil, unlocated("%s: with its directives blanked, a comment on lines %d-%d crosses a comment they split", name, c.Lines.First, c.Lines.Last)
		case ci < 0:
			keptSecond = append(keptSecond, c)
		default:
			c.Subject, c.Doc = first.Comments[ci].Subject, first.Comments[ci].Doc
			out.Comments = append(out.Comments, c)
			claims[ci] = append(claims[ci], [2]int{c.Start, c.End})
		}
	}
	var excludedSecond []Excluded
	for _, e := range second.Excluded {
		ci, ok := splitIn(e.Start, e.End)
		switch {
		case !ok:
			return nil, unlocated("%s: with its directives blanked, an exclusion on lines %d-%d crosses a comment they split", name, e.Lines.First, e.Lines.Last)
		case ci < 0:
			excludedSecond = append(excludedSecond, e)
		case e.Reason != ReasonBlank:
			return nil, unlocated("%s: with its directives blanked, line %d of a comment they split is set aside as %s", name, e.Lines.First, e.Reason)
		default:
			out.Excluded = append(out.Excluded, e)
			claims[ci] = append(claims[ci], [2]int{e.Start, e.End})
		}
	}
	// keptFirst is what the file holds, so the second reading is held to its
	// spans and text and not to what each comment is attached to.
	sameComment := func(a, b Comment) bool {
		b.Subject, b.Doc = a.Subject, a.Doc
		return reflect.DeepEqual(a, b)
	}
	sameExclusion := func(a, b Excluded) bool { return reflect.DeepEqual(a, b) }
	if !slices.EqualFunc(keptFirst, keptSecond, sameComment) || !slices.EqualFunc(first.Excluded, excludedSecond, sameExclusion) {
		return nil, unlocated("%s: blanking its declared directives changed the comments around them", name)
	}

	for ci, lines := range marks {
		c := first.Comments[ci]
		claimed := make([]int, c.End-c.Start)
		for _, m := range lines {
			claims[ci] = append(claims[ci], [2]int{m.Start, m.End})
		}
		for _, span := range claims[ci] {
			for i := span[0]; i < span[1]; i++ {
				claimed[i-c.Start]++
			}
		}
		for i, n := range claimed {
			if b := src[c.Start+i]; n > 1 || (n == 0 && !isSpace(b)) {
				return nil, unlocated("%s: the comment on lines %d-%d does not split into pieces that hold each of its bytes once",
					name, c.Lines.First, c.Lines.Last)
			}
		}
		out.Excluded = append(out.Excluded, lines...)
	}

	out.Comments = append(out.Comments, keptFirst...)
	out.Excluded = append(out.Excluded, first.Excluded...)
	slices.SortStableFunc(out.Comments, func(a, b Comment) int { return a.Start - b.Start })
	slices.SortStableFunc(out.Excluded, func(a, b Excluded) int { return a.Start - b.Start })
	return out, nil
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\f' || b == '\v'
}
