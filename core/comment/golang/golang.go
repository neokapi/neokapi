// Package golang locates the comments in Go source.
//
// It uses go/parser rather than a grammar: the parser reports every comment
// with its exact position, and a comment group, a run of `//` lines or one
// `/* */`, is already the multi-line unit a reader would call one comment. The
// interior of a comment is parsed with go/doc/comment, so code blocks and
// references to declarations are recognised the way godoc recognises them.
//
// Standard library only, so every kapi binary can read Go comments with nothing
// installed.
package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
)

// Language is the name the provider reports.
const Language = "go"

// Provider locates the comments in Go source files.
type Provider struct{}

var _ comment.Provider = Provider{}

// Language implements comment.Provider.
func (Provider) Language() string { return Language }

// Extensions implements comment.Provider.
func (Provider) Extensions() []string { return []string{".go"} }

// Locate implements comment.Provider. A file that does not parse is an error:
// a comment's position is only exact in a file the parser accepted.
func (Provider) Locate(name string, src []byte) (*comment.File, error) {
	return locate(name, src, directiveForms)
}

// LineText implements comment.Provider. A line comment runs to the end of its
// line; a delimited comment is whole on a line only when it closes there.
func (Provider) LineText(line []byte) (int, string, bool) {
	s := string(line)
	if body, ok := strings.CutPrefix(s, "//"); ok {
		return len(s), body, true
	}
	if body, ok := strings.CutPrefix(s, "/*"); ok {
		if i := strings.Index(body, "*/"); i >= 0 {
			return len("/*") + i + len("*/"), body[:i], true
		}
	}
	return 0, "", false
}

func locate(name string, src []byte, forms []directiveForm) (*comment.File, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	s := &scanner{
		src:   src,
		file:  fset.File(f.Pos()),
		out:   &comment.File{Language: Language},
		forms: forms,
	}

	if ast.IsGenerated(f) {
		for _, g := range f.Comments {
			s.exclude(g.List, comment.ReasonGenerated, "")
		}
		return s.out, nil
	}

	subjects := subjectsOf(f)
	special := setAside(f, name)
	docs := docParser(f)
	for _, g := range f.Comments {
		if reason, ok := special[g]; ok {
			s.exclude(g.List, reason, "")
			continue
		}
		if err := s.group(g, subjects.of(g), docs); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return s.out, nil
}

// scanner accumulates one file's comments and exclusions.
type scanner struct {
	src   []byte
	file  *token.File
	out   *comment.File
	forms []directiveForm
}

// span returns the bytes from the start of first to the end of last.
func (s *scanner) span(first, last *ast.Comment) (start, end int) {
	return s.file.Offset(first.Pos()), s.end(last)
}

// end returns the offset just past a comment's last byte.
//
// It is counted in the source rather than taken from the comment's End. The
// parser drops every carriage return from a comment's text and derives End
// from the shortened text, so in a file with CRLF line endings End falls short
// of a block comment's closing `*/` by one byte per line. A line comment's own
// terminating carriage return belongs to the line ending, and stays outside
// the span either way.
func (s *scanner) end(c *ast.Comment) int {
	i := s.file.Offset(c.Pos())
	for n := 0; n < len(c.Text); i++ {
		if s.src[i] != '\r' {
			n++
		}
	}
	return i
}

// lines reports the lines a span covers as the file numbers them. A `//line`
// directive changes the position the compiler reports for everything after it,
// but a diff and an editor count the lines of the file itself.
func (s *scanner) lines(start, end int) format.LineRange {
	return format.LineRange{
		First: s.file.PositionFor(s.file.Pos(start), false).Line,
		Last:  s.file.PositionFor(s.file.Pos(end), false).Line,
	}
}

func (s *scanner) exclude(list []*ast.Comment, reason comment.Reason, form string) {
	for _, c := range list {
		start, end := s.span(c, c)
		s.out.Excluded = append(s.out.Excluded, comment.Excluded{
			Start:  start,
			End:    end,
			Lines:  s.lines(start, end),
			Reason: reason,
			Form:   form,
		})
	}
}

// group splits one comment group at its directives. Each maximal run of lines
// between directives is a comment of its own, so the prose in a doc comment
// that also carries `//go:embed` or `//nolint` stays addressable and the
// directive never falls inside an addressable span.
func (s *scanner) group(g *ast.CommentGroup, subj subject, docs *docCommentParser) error {
	var run []*ast.Comment
	for _, c := range g.List {
		if form, ok := classifyDirective(c.Text, s.forms); ok {
			if err := s.prose(run, subj, docs); err != nil {
				return err
			}
			run = nil
			s.exclude([]*ast.Comment{c}, comment.ReasonDirective, form)
			continue
		}
		run = append(run, c)
	}
	return s.prose(run, subj, docs)
}

// prose records a run of non-directive lines. Empty lines at either edge carry
// nothing and are set aside, so a comment's span starts and ends on a line with
// something written on it.
func (s *scanner) prose(run []*ast.Comment, subj subject, docs *docCommentParser) error {
	first, last := 0, len(run)
	for first < last && isBlank(run[first].Text) {
		first++
	}
	for last > first && isBlank(run[last-1].Text) {
		last--
	}
	s.exclude(run[:first], comment.ReasonBlank, "")
	lines := run[first:last]
	if len(lines) > 0 {
		style := comment.StyleLine
		for _, c := range lines {
			if strings.HasPrefix(c.Text, "/*") {
				style = comment.StyleBlock
			}
		}
		doc := docs.Parse(s.text(lines))
		runs, err := docRuns(doc)
		if err != nil {
			return err
		}
		start, end := s.span(lines[0], lines[len(lines)-1])
		s.out.Comments = append(s.out.Comments, comment.Comment{
			Start:      start,
			End:        end,
			Lines:      s.lines(start, end),
			Style:      style,
			Subject:    subj.path,
			Doc:        subj.doc,
			Deprecated: deprecated(doc),
			Runs:       runs,
		})
	}
	s.exclude(run[last:], comment.ReasonBlank, "")
	return nil
}

// text is what a run of comment lines says once its markers are removed, as
// go/doc/comment reads it. A delimited comment alone in the run is read through
// its layout, as a rewrite reads it, so the space beside its delimiters and a
// prefix such as a line of asterisks stay out of its text. A run of several
// comments is read by go/ast, which removes each comment's markers. The run
// holds no directive, so go/ast has nothing else to drop.
func (s *scanner) text(lines []*ast.Comment) string {
	if len(lines) == 1 && strings.HasPrefix(lines[0].Text, "/*") {
		start, end := s.span(lines[0], lines[0])
		if l, err := blockLayout(s.src, comment.Comment{Start: start, End: end}); err == nil {
			out := strings.Split(l.Text(), "\n")
			for i, line := range out {
				out[i] = strings.TrimRight(line, " \t")
			}
			return strings.Join(out, "\n") + "\n"
		}
	}
	return (&ast.CommentGroup{List: lines}).Text()
}

// isBlank reports whether a comment holds nothing once its markers are removed.
func isBlank(text string) bool {
	switch {
	case strings.HasPrefix(text, "//"):
		text = text[2:]
	case strings.HasPrefix(text, "/*"):
		text = strings.TrimSuffix(text[2:], "*/")
	}
	return strings.TrimSpace(text) == ""
}

// outputPrefix is go/doc's test for the comment that closes an example
// function.
var outputPrefix = regexp.MustCompile(`(?i)^[[:space:]]*(unordered )?output:`)

// setAside finds the whole groups a tool reads: the cgo preamble, and the
// output comment of each example function in a test file.
func setAside(f *ast.File, name string) map[*ast.CommentGroup]comment.Reason {
	out := map[*ast.CommentGroup]comment.Reason{}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			// cgo's own rule: the import spec's doc, or the declaration's when
			// the declaration imports "C" alone.
			for _, spec := range d.Specs {
				is, ok := spec.(*ast.ImportSpec)
				if !ok || is.Path.Value != `"C"` {
					continue
				}
				cg := is.Doc
				if cg == nil && len(d.Specs) == 1 {
					cg = d.Doc
				}
				if cg != nil {
					out[cg] = comment.ReasonCgoPreamble
				}
			}
		case *ast.FuncDecl:
			if !strings.HasSuffix(name, "_test.go") || d.Recv != nil || d.Body == nil || !isExampleName(d.Name.Name) {
				continue
			}
			if last := lastCommentIn(d.Body, f.Comments); last != nil && outputPrefix.MatchString(last.Text()) {
				out[last] = comment.ReasonExampleOutput
			}
		}
	}
	return out
}

// isExampleName is go/doc's test for an example function's name: "Example",
// or "Example" followed by anything that does not start with a lower-case
// letter.
func isExampleName(name string) bool {
	rest, ok := strings.CutPrefix(name, "Example")
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

// lastCommentIn returns the last comment group inside a function body, the one
// go/doc reads an example's expected output from.
func lastCommentIn(body *ast.BlockStmt, groups []*ast.CommentGroup) *ast.CommentGroup {
	var last *ast.CommentGroup
	for _, g := range groups {
		if g.Pos() < body.Pos() {
			continue
		}
		if g.End() > body.End() {
			break
		}
		last = g
	}
	return last
}
