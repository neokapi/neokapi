package golang

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
)

// accountFor checks what a provider located in src against an independent
// parse of the same bytes. It is the assertion the P1 rung rests on, so it
// trusts nothing the provider computed:
//
//   - every comment the parser reports sits in exactly one addressable comment
//     or one exclusion, so nothing is lost and nothing is counted twice;
//   - an exclusion covers exactly one comment line;
//   - an addressable span starts where a comment starts and ends where a
//     comment ends, holds whole comments only, and never reaches across two
//     comment groups;
//   - the bytes of every span are the parser's own comment text;
//   - the line range is the file's own lines, inclusive at both ends;
//   - no addressable span contains a directive;
//   - no two spans overlap.
func accountFor(name string, src []byte, got *comment.File) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	tf := fset.File(f.Pos())
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	type claim struct {
		span     byteSpan
		lines    format.LineRange
		excluded bool
		index    int
	}
	claims := make([]claim, 0, len(got.Comments)+len(got.Excluded))
	for i, c := range got.Comments {
		claims = append(claims, claim{span: byteSpan{c.Start, c.End}, lines: c.Lines, index: i})
	}
	for i, e := range got.Excluded {
		claims = append(claims, claim{span: byteSpan{e.Start, e.End}, lines: e.Lines, excluded: true, index: i})
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].span.Start < claims[j].span.Start })
	idx := format.NewLineIndex(src)
	for i, c := range claims {
		// The provider counts lines from token positions; the line index every
		// other extent uses must put the span on the same lines.
		if c.span.Start < c.span.End {
			if byIndex := idx.Range(c.span.Start, c.span.End); byIndex != c.lines {
				fail("span %d-%d reports lines %d-%d, the line index puts it on %d-%d",
					c.span.Start, c.span.End, c.lines.First, c.lines.Last, byIndex.First, byIndex.Last)
			}
		}
		if c.span.Start < 0 || c.span.End > len(src) || c.span.Start >= c.span.End {
			fail("span %d-%d is empty or outside the file", c.span.Start, c.span.End)
		}
		if i > 0 && c.span.Start < claims[i-1].span.End {
			fail("span %d-%d overlaps span %d-%d", c.span.Start, c.span.End, claims[i-1].span.Start, claims[i-1].span.End)
		}
		want := format.LineRange{
			First: tf.PositionFor(tf.Pos(clamp(c.span.Start, len(src))), false).Line,
			Last:  tf.PositionFor(tf.Pos(clamp(c.span.End, len(src))), false).Line,
		}
		if c.lines != want {
			fail("span %d-%d reports lines %d-%d, the file puts it on %d-%d",
				c.span.Start, c.span.End, c.lines.First, c.lines.Last, want.First, want.Last)
		}
	}

	// Each parser comment, in file order, with the group it belongs to.
	type located struct {
		start, end int
		group      int
		text       string
	}
	var all []located
	for gi, g := range f.Comments {
		for _, c := range g.List {
			start := tf.Offset(c.Pos())
			all = append(all, located{start: start, end: endOf(src, start, c.Text), group: gi, text: c.Text})
		}
	}

	covered := make([]int, len(claims)) // parser comments inside each claim
	firstGroup := make([]int, len(claims))
	for _, pc := range all {
		n := 0
		for ci, c := range claims {
			if c.span.End <= pc.start || c.span.Start >= pc.end {
				continue
			}
			n++
			if pc.start < c.span.Start || pc.end > c.span.End {
				fail("comment at %d-%d is cut by span %d-%d", pc.start, pc.end, c.span.Start, c.span.End)
				continue
			}
			if c.excluded && (c.span.Start != pc.start || c.span.End != pc.end) {
				fail("exclusion %d-%d does not cover exactly the comment at %d-%d", c.span.Start, c.span.End, pc.start, pc.end)
			}
			if covered[ci] == 0 {
				firstGroup[ci] = pc.group
			} else if firstGroup[ci] != pc.group {
				fail("span %d-%d merges comment group %d with group %d", c.span.Start, c.span.End, firstGroup[ci], pc.group)
			}
			covered[ci]++
			if !c.excluded {
				if _, ok := classifyDirective(pc.text, directiveForms); ok {
					fail("addressable span %d-%d contains the directive %q", c.span.Start, c.span.End, pc.text)
				}
			}
		}
		switch n {
		case 0:
			fail("comment at %d-%d (%q) is neither addressable nor excluded", pc.start, pc.end, firstLine(pc.text))
		case 1:
		default:
			fail("comment at %d-%d is claimed %d times", pc.start, pc.end, n)
		}
	}

	for ci, c := range claims {
		if covered[ci] == 0 {
			fail("span %d-%d holds no comment the parser reports", c.span.Start, c.span.End)
			continue
		}
		if c.excluded {
			continue
		}
		// The bytes of the span are the parser's text: they open with the first
		// comment it holds and close with the last.
		body := strings.ReplaceAll(string(src[c.span.Start:c.span.End]), "\r", "")
		var first, last located
		for _, pc := range all {
			if pc.start >= c.span.Start && pc.end <= c.span.End {
				if first.text == "" {
					first = pc
				}
				last = pc
			}
		}
		if first.start != c.span.Start || last.end != c.span.End {
			fail("span %d-%d does not run from a comment's start (%d) to a comment's end (%d)",
				c.span.Start, c.span.End, first.start, last.end)
		}
		if !strings.HasPrefix(body, first.text) || !strings.HasSuffix(body, last.text) {
			fail("span %d-%d bytes %q are not the parser's comment text", c.span.Start, c.span.End, firstLine(body))
		}
	}
	return errors.Join(errs...)
}

// endOf finds the offset just past a comment the parser reports as text,
// skipping the carriage returns the parser removed from it.
func endOf(src []byte, start int, text string) int {
	i := start
	for n := 0; n < len(text); i++ {
		if src[i] != '\r' {
			n++
		}
	}
	return i
}

func clamp(n, limit int) int {
	if n < 0 {
		return 0
	}
	if n > limit {
		return limit
	}
	return n
}

func firstLine(s string) string {
	if first, _, cut := strings.Cut(s, "\n"); cut {
		return first + "..."
	}
	return s
}

// groupsOf returns the parser's comment groups, for tests that count them.
func groupsOf(name string, src []byte) ([]*ast.CommentGroup, error) {
	f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	return f.Comments, nil
}

// byteSpan is a half-open byte range [Start, End), as a claim records it.
type byteSpan struct{ Start, End int }
