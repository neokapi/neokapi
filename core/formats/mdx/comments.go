package mdx

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/markup"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/markdown"
)

// CommentProvider supplies the expression comments of an MDX document to the
// comment layer, each with the span it occupies. It adds nothing to the
// reader's part stream.
type CommentProvider struct{}

var _ comment.Provider = CommentProvider{}

// Language implements comment.Provider.
func (CommentProvider) Language() string { return "mdx" }

// Extensions implements comment.Provider. A format's provider is found by the
// format's name, so it claims no extension.
func (CommentProvider) Extensions() []string { return nil }

// Locate implements comment.Provider.
func (CommentProvider) Locate(_ string, src []byte) (*comment.File, error) {
	return LocateComments(src)
}

// LineText implements comment.Provider for both comment syntaxes an MDX
// document carries: the expression comment that opens line and closes on it,
// the bytes it occupies through its closing brace, and what it holds between
// `/*` and `*/`; or an HTML comment, read by the Markdown provider, so the
// empty forms `<!-->` and `<!--->` close where Markdown closes them.
func (CommentProvider) LineText(line []byte) (n int, text string, ok bool) {
	if c, found := expressionComment(line); found {
		return c.end, string(line[c.open : c.end-c.close]), true
	}
	return markdown.CommentProvider{}.LineText(line)
}

// Canary implements comment.Provider: an expression comment with a doubled
// word, above a paragraph whose code span holds a comment marker that is
// content.
func (CommentProvider) Canary() comment.Canary {
	return comment.Canary{
		Name:   "an MDX expression comment with a doubled word, above a code span holding a comment marker",
		Source: []byte("# Greeting\n\n{/* Greets the the reader. */}\n\nHello `{/* not a comment */}` world.\n"),
		Block:  "comment/greeting",
	}
}

// ErrCommentsUnlocated reports an MDX document whose comments the MDX scan and
// a scan of the bytes do not agree on. It wraps comment.ErrUnlocated.
var ErrCommentsUnlocated = fmt.Errorf("mdx: %w", comment.ErrUnlocated)

// LocateComments returns the expression comments in an MDX document.
//
// The document is read with the reader's own scan, which separates Markdown
// from ESM statements, JSX elements and top-level expressions. A comment is a
// top-level expression that holds one `/* */` comment and nothing else, such as
// `{/* A note. */}`. Every `{/*` and `<!--` in the file must open such a
// comment, sit inside one, or sit in content: code or front matter in a Markdown
// span, which the Markdown format reads, code in the Markdown children of a JSX
// element, or an ESM statement. So a document with
// an expression comment inside a JSX element or a paragraph, or an expression
// holding more than one comment, is refused with ErrCommentsUnlocated.
//
// A file whose first comment says `DO NOT EDIT` belongs to its generator, and
// every comment in it is set aside as generated. A comment is named for the
// section it sits in, as a Markdown comment is.
func LocateComments(src []byte) (*comment.File, error) {
	return locateComments(src, classifyComment)
}

func locateComments(src []byte, classify func(inner string) (string, bool)) (*comment.File, error) {
	bom, body := format.SplitBOM(src)
	base := len(bom)
	segs := scanSegments(body)
	if err := checkSegments(segs, body); err != nil {
		return nil, commentsUnlocated("the MDX scan: %v", err)
	}
	var comments []mdxComment
	var content [][2]int
	var headings []markdown.Heading
	for _, seg := range segs {
		start, end := base+seg.start, base+seg.end
		switch seg.kind {
		case segMarkdown:
			span, err := markdown.ReadSpan(src[start:end])
			if err != nil {
				return nil, fmt.Errorf("%w: the Markdown at byte %d: %w", ErrCommentsUnlocated, start, err)
			}
			for _, c := range span.Comments {
				comments = append(comments, mdxComment{start: start + c.Start, end: start + c.End, open: len(markup.Open), close: c.Close, html: true})
			}
			for _, r := range span.Content {
				content = append(content, [2]int{start + r[0], start + r[1]})
			}
			for _, h := range span.Headings {
				h.Offset += start
				headings = append(headings, h)
			}
		case segESM:
			content = append(content, [2]int{start, end})
		case segJSX:
			// A JSX element's children are Markdown, so code in them is
			// content. Anything else in the element holds no marker the scan
			// can account for, and an HTML comment is not MDX.
			span, err := markdown.ReadSpan(src[start:end])
			if err != nil {
				continue
			}
			for _, c := range span.Comments {
				comments = append(comments, mdxComment{start: start + c.Start, end: start + c.End, open: len(markup.Open), close: c.Close, html: true})
			}
			for _, r := range span.Code {
				content = append(content, [2]int{start + r[0], start + r[1]})
			}
			for _, h := range span.Headings {
				h.Offset += start
				headings = append(headings, h)
			}
		case segExpr:
			region := src[start:end]
			if !bytes.HasPrefix(region[skipCommentSpace(region, 1):], []byte("/*")) {
				content = append(content, [2]int{start, end})
				continue
			}
			c, ok := expressionComment(region)
			if !ok || len(bytes.TrimSpace(region[c.end:])) > 0 {
				return nil, commentsUnlocated("the expression at byte %d holds more than one comment", start)
			}
			comments = append(comments, mdxComment{start: start, end: start + c.end, open: c.open, close: c.close})
		}
	}
	if err := agree(src, base, comments, content); err != nil {
		return nil, err
	}

	generated := len(comments) > 0 && generatedRe.Match(src[comments[0].start+comments[0].open:comments[0].end-comments[0].close])
	idx := format.NewLineIndex(src)
	file := &comment.File{Language: "mdx"}
	for _, c := range comments {
		inner := string(src[c.start+c.open : c.end-c.close])
		lines := idx.Range(c.start, c.end)
		exclude := func(reason comment.Reason, form string) {
			file.Excluded = append(file.Excluded, comment.Excluded{Start: c.start, End: c.end, Lines: lines, Reason: reason, Form: form})
		}
		if generated {
			exclude(comment.ReasonGenerated, "")
			continue
		}
		if markup.Blank(inner) {
			exclude(comment.ReasonBlank, "")
			continue
		}
		classifyThis := classify
		if c.html {
			classifyThis = classifyHTMLComment
		}
		if form, ok := classifyThis(inner); ok {
			exclude(comment.ReasonDirective, form)
			continue
		}
		file.Comments = append(file.Comments, comment.Comment{
			Start:   c.start,
			End:     c.end,
			Lines:   lines,
			Style:   comment.StyleBlock,
			Subject: markdown.CommentSubject(headings, c.start),
			Runs:    markup.Runs(inner),
		})
	}
	return file, nil
}

// generatedRe marks a file its generator owns, as the reference page generator
// writes in its first comment: `GENERATED FILE. DO NOT EDIT.`
var generatedRe = regexp.MustCompile(`(?i)\bdo not edit\b`)

// commentDirectiveForms are the comments tools read in MDX files.
var commentDirectiveForms = []markup.DirectiveForm{markup.Truncate, markup.PrettierIgnore, markup.Markdownlint, markup.ESLint}

// htmlCommentDirectiveForms are what a tool reads in the HTML comments of a
// document read as MDX: the shared markup forms, and Docusaurus's truncate
// marker, which a Markdown page writes as `<!-- truncate -->`.
var htmlCommentDirectiveForms = append([]markup.DirectiveForm{markup.Truncate}, markup.HTMLComment...)

func classifyComment(inner string) (string, bool) {
	return markup.Classify(inner, commentDirectiveForms)
}

func classifyHTMLComment(inner string) (string, bool) {
	return markup.Classify(inner, htmlCommentDirectiveForms)
}

type mdxComment struct {
	start, end int
	// html marks a comment written `<!-- -->` in a Markdown span rather than
	// an MDX expression comment. The two carry different markers and read
	// different directive forms.
	html bool
	// open and close are the lengths of the markers: `{` and `/*` with any
	// whitespace between them, and `*/` and `}` likewise.
	open, close int
}

// expressionComment reads the expression comment that opens b: `{`, a `/* */`
// comment, and `}`, with whitespace allowed around the comment.
func expressionComment(b []byte) (mdxComment, bool) {
	if len(b) == 0 || b[0] != '{' {
		return mdxComment{}, false
	}
	j := skipCommentSpace(b, 1)
	if !bytes.HasPrefix(b[j:], []byte("/*")) {
		return mdxComment{}, false
	}
	shut := bytes.Index(b[j+2:], []byte("*/"))
	if shut < 0 {
		return mdxComment{}, false
	}
	k := skipCommentSpace(b, j+2+shut+2)
	if k >= len(b) || b[k] != '}' {
		return mdxComment{}, false
	}
	return mdxComment{end: k + 1, open: j + 2, close: k + 1 - (j + 2 + shut)}, true
}

func skipCommentSpace(b []byte, k int) int {
	for k < len(b) && (b[k] == ' ' || b[k] == '\t' || b[k] == '\n' || b[k] == '\r') {
		k++
	}
	return k
}

// agree holds the MDX scan to the bytes: every `{/*` and `<!--` opens a
// comment, sits inside one, or sits in content.
func agree(src []byte, base int, comments []mdxComment, content [][2]int) error {
	for _, marker := range []string{"{/*", markup.Open} {
		for i := base; ; {
			at := bytes.Index(src[i:], []byte(marker))
			if at < 0 {
				break
			}
			at += i
			i = at + 1
			inComment := slices.ContainsFunc(comments, func(c mdxComment) bool { return c.start <= at && at < c.end })
			inContent := slices.ContainsFunc(content, func(r [2]int) bool { return r[0] <= at && at < r[1] })
			if !inComment && !inContent {
				return commentsUnlocated("the scan finds %q at byte %d, which the MDX scan reads as neither a comment nor content", marker, at)
			}
		}
	}
	return nil
}

func commentsUnlocated(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrCommentsUnlocated}, args...)...)
}
