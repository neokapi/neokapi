package golang

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"

	"github.com/neokapi/neokapi/core/comment"
)

var _ comment.Formatter = Provider{}

// ErrUnlocated reports that the formatter's output could not be matched back to
// the comments of the source, so its disagreements have no location.
var ErrUnlocated = errors.New("formatter output does not line up with the source's comments")

// FormatterName implements comment.Formatter.
func (Provider) FormatterName() string { return "gofmt" }

// Format implements comment.Formatter with go/format, the library gofmt is
// built on.
func (Provider) Format(name string, src []byte) ([]byte, error) {
	formatted, err := format.Source(src)
	if err != nil {
		return nil, fmt.Errorf("gofmt %s: %w", name, err)
	}
	return formatted, nil
}

// Disagreements implements comment.Formatter with go/format, the library gofmt
// is built on. The repository runs `gofmt -s`; the simplification it adds
// rewrites expressions and never touches a comment.
//
// gofmt keeps every comment group, in order: it reindents comment lines,
// reformats doc comments (list markers, code block indentation, directives
// moved to the end) and aligns trailing comments, but never merges, splits or
// drops a group. So the source's groups and the formatted file's groups pair
// by position, and within a pair the lines are compared by a longest common
// subsequence. A comment disagrees when one of its own lines changes, or when a
// line is inserted between two of them.
//
// A directive or blank line has no comment of its own, so a change there is
// attributed to the prose on either side of it in the same group. Moving
// `//go:noinline` from between two paragraphs to the end of the doc comment
// changes only the directive's line, but it is the doc comment gofmt rewrites,
// and a comparison that attributed it to nothing would let that comment pass.
//
// A line is its indentation and its text, so a comment gofmt only reindents
// disagrees. A trailing comment is its text alone: the spacing between code and
// `//` is alignment, and belongs to the code. Line endings are not compared.
func (Provider) Disagreements(name string, src []byte, f *comment.File) ([]comment.Disagreement, error) {
	formatted, err := format.Source(src)
	if err != nil {
		return nil, fmt.Errorf("gofmt %s: %w", name, err)
	}
	if bytes.Equal(formatted, src) || len(f.Comments) == 0 {
		return nil, nil
	}
	before, err := commentLines(name, src)
	if err != nil {
		return nil, err
	}
	after, err := commentLines(name, formatted)
	if err != nil {
		return nil, fmt.Errorf("gofmt output for %s: %w", name, err)
	}
	if len(before) != len(after) {
		return nil, fmt.Errorf("%w: %d comment groups before, %d after", ErrUnlocated, len(before), len(after))
	}

	var out []comment.Disagreement
	for gi, group := range before {
		changed, inserted := comment.DiffLines(group.keys(), after[gi].keys())
		if !anyTrue(changed) && !anyTrue(inserted) {
			continue
		}
		owners := lineOwners(group, f.Comments)
		hit := map[int]bool{}
		flag := func(li int) {
			if li < 0 || li >= len(owners) {
				return
			}
			for _, ci := range owners[li] {
				hit[ci] = true
			}
		}
		for li := range group {
			if changed[li] {
				flag(li)
			}
		}
		for li, ins := range inserted {
			if ins {
				flag(li - 1)
				flag(li)
			}
		}
		for ci := range f.Comments {
			if hit[ci] {
				out = append(out, comment.Disagreement{Comment: ci, Formatted: after[gi].text()})
			}
		}
	}
	return out, nil
}

// lineOwners maps each line of a group to the comments a change there belongs
// to: a prose line to its own comment, and a directive or blank line to the
// nearest comment before it and after it in the group.
func lineOwners(group commentGroupLines, comments []comment.Comment) [][]int {
	own := make([]int, len(group))
	for li, l := range group {
		own[li] = -1
		for ci, c := range comments {
			if l.start >= c.Start && l.start < c.End {
				own[li] = ci
				break
			}
		}
	}
	owners := make([][]int, len(group))
	for li := range group {
		if own[li] >= 0 {
			owners[li] = []int{own[li]}
			continue
		}
		for b := li - 1; b >= 0; b-- {
			if own[b] >= 0 {
				owners[li] = append(owners[li], own[b])
				break
			}
		}
		for a := li + 1; a < len(group); a++ {
			if own[a] >= 0 {
				owners[li] = append(owners[li], own[a])
				break
			}
		}
	}
	return owners
}

// commentLine is one comment as the formatter comparison sees it.
type commentLine struct {
	start  int
	indent string
	text   string
}

type commentGroupLines []commentLine

func (g commentGroupLines) keys() []string {
	keys := make([]string, len(g))
	for i, l := range g {
		keys[i] = l.indent + l.text
	}
	return keys
}

func (g commentGroupLines) text() string {
	lines := make([]string, len(g))
	for i, l := range g {
		lines[i] = l.text
	}
	return strings.Join(lines, "\n")
}

// commentLines parses src and returns each comment group's lines, with the
// indentation of any comment that starts its own line.
func commentLines(name string, src []byte) ([]commentGroupLines, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	tf := fset.File(f.Pos())
	groups := make([]commentGroupLines, len(f.Comments))
	for gi, g := range f.Comments {
		groups[gi] = make(commentGroupLines, len(g.List))
		for li, c := range g.List {
			groups[gi][li] = lineOf(tf, src, c)
		}
	}
	return groups, nil
}

func lineOf(tf *token.File, src []byte, c *ast.Comment) commentLine {
	start := tf.Offset(c.Pos())
	lineStart := bytes.LastIndexByte(src[:start], '\n') + 1
	indent := string(src[lineStart:start])
	if strings.TrimLeft(indent, " \t") != "" {
		indent = "" // a trailing comment: what precedes it is code
	}
	return commentLine{start: start, indent: indent, text: c.Text}
}

func anyTrue(bs []bool) bool {
	for _, b := range bs {
		if b {
			return true
		}
	}
	return false
}
