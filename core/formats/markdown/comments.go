package markdown

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/markup"
	"github.com/neokapi/neokapi/core/format"
	htmlformat "github.com/neokapi/neokapi/core/formats/html"
)

// CommentProvider supplies the HTML comments of a Markdown document to the
// comment layer, each with the span it occupies. It adds nothing to the
// reader's part stream.
type CommentProvider struct{}

var _ comment.Provider = CommentProvider{}

// Language implements comment.Provider.
func (CommentProvider) Language() string { return "markdown" }

// Extensions implements comment.Provider. A format's provider is found by the
// format's name, so it claims no extension.
func (CommentProvider) Extensions() []string { return nil }

// Locate implements comment.Provider.
func (CommentProvider) Locate(_ string, src []byte) (*comment.File, error) {
	return LocateComments(src)
}

// LineText implements comment.Provider: the comment that opens line and closes
// on it, by the Markdown parser's rule.
func (CommentProvider) LineText(line []byte) (n int, text string, ok bool) {
	end, closeLen, closed := inlineCommentEnd(line)
	if !closed {
		return 0, "", false
	}
	return end, string(line[len(markup.Open) : end-closeLen]), true
}

// Canary implements comment.Provider: a comment with a doubled word, above a
// paragraph whose code span holds a comment marker that is content.
func (CommentProvider) Canary() comment.Canary {
	return comment.Canary{
		Name:   "a Markdown HTML comment with a doubled word, above a code span holding a comment marker",
		Source: []byte("# Greeting\n\n<!-- Greets the the reader. -->\n\nHello `<!-- not a comment -->` world.\n"),
		Block:  "comment/greeting",
	}
}

// ErrCommentsUnlocated reports a Markdown document whose comments the Markdown
// parser and a scan of the bytes do not agree on. It wraps comment.ErrUnlocated.
var ErrCommentsUnlocated = fmt.Errorf("markdown: %w", comment.ErrUnlocated)

// LocateComments returns the HTML comments in a Markdown document.
//
// The document is read as its reader reads it: front matter set aside, and the
// body parsed by the Markdown parser. A comment is an inline HTML comment, or a
// comment inside an HTML block, which the HTML format's scan locates. A marker
// in a code span, a fenced or indented code block, an HTML tag or a backslash
// escape is content. Every `<!--` in the body must open a comment or sit in
// content, or the document is refused with ErrCommentsUnlocated, so a marker the
// parser reads some other way, as in a link title or a comment never closed, is
// never passed over. A comment that closes on `--!>` is refused as well, since
// the Markdown parser and an HTML parser close it in different places.
//
// A comment is named for the section it sits in, by the trail of heading slugs:
// `comment/install/from-homebrew`, or `comment/document` before any heading.
func LocateComments(src []byte) (*comment.File, error) {
	return locateComments(src, classifyComment)
}

func locateComments(src []byte, classify func(inner string) (string, bool)) (*comment.File, error) {
	_, _, bodyStart, _ := frontMatterBounds(src)
	body := src[bodyStart:]
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(body))
	w := &mdCommentWalk{src: src, body: body, base: bodyStart}
	if err := ast.Walk(doc, w.visit); err != nil {
		return nil, err
	}
	if err := w.agree(); err != nil {
		return nil, err
	}
	slices.SortFunc(w.comments, func(a, b mdComment) int { return a.start - b.start })

	idx := format.NewLineIndex(src)
	file := &comment.File{Language: "markdown"}
	for _, c := range w.comments {
		inner := string(src[c.start+len(markup.Open) : c.end-c.close])
		lines := idx.Range(c.start, c.end)
		if markup.Blank(inner) {
			file.Excluded = append(file.Excluded, comment.Excluded{Start: c.start, End: c.end, Lines: lines, Reason: comment.ReasonBlank})
			continue
		}
		if form, ok := classify(inner); ok {
			file.Excluded = append(file.Excluded, comment.Excluded{Start: c.start, End: c.end, Lines: lines, Reason: comment.ReasonDirective, Form: form})
			continue
		}
		file.Comments = append(file.Comments, comment.Comment{
			Start:   c.start,
			End:     c.end,
			Lines:   lines,
			Style:   comment.StyleBlock,
			Subject: c.subject,
			Runs:    markup.Runs(inner),
		})
	}
	return file, nil
}

// commentDirectiveForms are the comments tools read in Markdown files.
var commentDirectiveForms = []markup.DirectiveForm{
	truncateMarker, voicePointer, generatedRegion,
	markup.Markdownlint, markup.PrettierIgnore, markup.FormatterToggle, markup.Suppress,
}

// truncateMarker ends the excerpt of a Docusaurus blog post.
var truncateMarker = markup.DirectiveForm{Name: "truncate", Match: func(b string) bool {
	return b == "truncate"
}}

// voicePointer bounds the region kapi writes into an agent instructions file,
// from `<!-- kapi:voice -->` to `<!-- /kapi:voice -->`.
var voicePointer = markup.DirectiveForm{Name: "kapi:voice", Match: func(b string) bool {
	return strings.HasPrefix(b, "kapi:voice") || strings.HasPrefix(b, "/kapi:voice")
}}

// regionRe is a generated region's marker.
var regionRe = regexp.MustCompile(`^(BEGIN|END)\s*:\s*\S`)

// generatedRegion bounds content a script replaces, as between
// `<!-- BEGIN:downloads-cli -->` and `<!-- END:downloads-cli -->`.
var generatedRegion = markup.DirectiveForm{Name: "region", Match: regionRe.MatchString}

func classifyComment(inner string) (string, bool) {
	return markup.Classify(inner, commentDirectiveForms)
}

// inlineCommentEnd returns where the comment that opens b ends by the Markdown
// parser's rule: `<!-->` and `<!--->` are empty comments, and any other comment
// closes at its first `-->`.
func inlineCommentEnd(b []byte) (end, closeLen int, closed bool) {
	switch {
	case !bytes.HasPrefix(b, []byte(markup.Open)):
		return 0, 0, false
	case bytes.HasPrefix(b, []byte("<!-->")):
		return len("<!-->"), 1, true
	case bytes.HasPrefix(b, []byte("<!--->")):
		return len("<!--->"), 2, true
	}
	at := bytes.Index(b[len(markup.Open):], []byte(markup.Close))
	if at < 0 {
		return 0, 0, false
	}
	return len(markup.Open) + at + len(markup.Close), len(markup.Close), true
}

func commentsUnlocated(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrCommentsUnlocated}, args...)...)
}

type mdComment struct {
	start, end, close int
	subject           string
}

type commentRange struct{ start, end int }

type mdSection struct {
	level int
	slug  string
}

// mdCommentWalk collects a document's comments and the ranges where a comment
// marker is content, in document order, with offsets into the whole file.
type mdCommentWalk struct {
	src, body []byte
	base      int
	comments  []mdComment
	content   []commentRange
	sections  []mdSection
}

func (w *mdCommentWalk) visit(n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	switch n := n.(type) {
	case *ast.Heading:
		for len(w.sections) > 0 && w.sections[len(w.sections)-1].level >= n.Level {
			w.sections = w.sections[:len(w.sections)-1]
		}
		slug := headingSlug(headingPlainText(n, w.body))
		if slug == "" {
			slug = "heading"
		}
		w.sections = append(w.sections, mdSection{level: n.Level, slug: slug})
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		s, e := blockRange(n, w.body)
		w.content = append(w.content, commentRange{w.base + s, w.base + e})
		return ast.WalkSkipChildren, nil
	case *ast.CodeSpan:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				w.content = append(w.content, commentRange{w.base + t.Segment.Start, w.base + t.Segment.Stop})
			}
		}
		return ast.WalkSkipChildren, nil
	case *ast.HTMLBlock:
		return ast.WalkSkipChildren, w.htmlBlock(n)
	case *ast.RawHTML:
		return ast.WalkSkipChildren, w.rawHTML(n)
	}
	return ast.WalkContinue, nil
}

// htmlBlock locates the comments in an HTML block with the HTML format's scan.
func (w *mdCommentWalk) htmlBlock(n *ast.HTMLBlock) error {
	s, e := blockRange(n, w.body)
	if n.HasClosure() && n.ClosureLine.Stop > e {
		e = n.ClosureLine.Stop
	}
	start, end := w.base+s, w.base+e
	w.content = append(w.content, commentRange{start, end})
	spans, err := htmlformat.CommentSpans(w.src[start:end])
	if err != nil {
		return fmt.Errorf("%w: the HTML block at byte %d: %w", ErrCommentsUnlocated, start, err)
	}
	for _, sp := range spans {
		if err := w.add(start+sp.Start, start+sp.End, sp.Close); err != nil {
			return err
		}
	}
	return nil
}

// rawHTML takes an inline HTML comment, and records any other inline HTML as
// content.
func (w *mdCommentWalk) rawHTML(n *ast.RawHTML) error {
	if n.Segments.Len() == 0 {
		return nil
	}
	start := w.base + n.Segments.At(0).Start
	end := w.base + n.Segments.At(n.Segments.Len()-1).Stop
	if !bytes.HasPrefix(w.src[start:end], []byte(markup.Open)) {
		w.content = append(w.content, commentRange{start, end})
		return nil
	}
	e, closeLen, closed := inlineCommentEnd(w.src[start:end])
	if !closed || start+e != end {
		return commentsUnlocated("the Markdown parser places a comment at bytes %d-%d, which its closing marker does not end", start, end)
	}
	return w.add(start, end, closeLen)
}

func (w *mdCommentWalk) add(start, end, closeLen int) error {
	if bytes.Contains(w.src[start:end], []byte("--!>")) {
		return commentsUnlocated("the comment at byte %d holds `--!>`, which the Markdown parser and an HTML parser close in different places", start)
	}
	subject := "comment/document"
	if len(w.sections) > 0 {
		slugs := make([]string, len(w.sections))
		for i, s := range w.sections {
			slugs[i] = s.slug
		}
		subject = "comment/" + strings.Join(slugs, "/")
	}
	w.comments = append(w.comments, mdComment{start: start, end: end, close: closeLen, subject: subject})
	return nil
}

// agree holds the parser to the bytes: every `<!--` in the body opens a comment
// the parser reports, sits in content, or follows a backslash.
func (w *mdCommentWalk) agree() error {
	starts := make(map[int]bool, len(w.comments))
	for _, c := range w.comments {
		starts[c.start] = true
	}
	for i := w.base; ; {
		at := bytes.Index(w.src[i:], []byte(markup.Open))
		if at < 0 {
			return nil
		}
		at += i
		i = at + 1
		switch {
		case starts[at]:
		case slices.ContainsFunc(w.content, func(r commentRange) bool { return r.start <= at && at < r.end }):
		case at > 0 && w.src[at-1] == '\\':
		default:
			return commentsUnlocated("the scan finds `<!--` at byte %d, which the Markdown parser reads as neither a comment nor content", at)
		}
	}
}

// headingPlainText is the text of a heading's inline content.
func headingPlainText(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			b.Write(t.Segment.Value(source))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}
