package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
)

// accountFor checks what a provider located in src against an independent
// parse of the same bytes, through the conformance suite's accounting. It is
// the assertion the P1 rung rests on.
func accountFor(name string, src []byte, got *comment.File) error {
	units, err := goUnits(name, src)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	return commenttest.Err(commenttest.Account(src, got, units))
}

// goUnits is the conformance suite's scan for Go: every comment go/parser
// reports, with the group it belongs to and whether it is a directive.
func goUnits(name string, src []byte) ([]commenttest.Unit, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	tf := fset.File(f.Pos())
	var units []commenttest.Unit
	for gi, g := range f.Comments {
		for _, c := range g.List {
			start := tf.Offset(c.Pos())
			u := commenttest.Unit{Start: start, End: endOf(src, start, c.Text), Open: 2, Group: gi}
			if strings.HasPrefix(c.Text, "/*") {
				u.Close = 2
			}
			_, u.Directive = classifyDirective(c.Text, directiveForms)
			units = append(units, u)
		}
	}
	return units, nil
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

// groupsOf returns the parser's comment groups, for tests that count them.
func groupsOf(name string, src []byte) ([]*ast.CommentGroup, error) {
	f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	return f.Comments, nil
}
