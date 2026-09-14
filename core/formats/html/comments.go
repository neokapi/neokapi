package html

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/markup"
	"github.com/neokapi/neokapi/core/format"
)

// CommentProvider supplies the comments of an HTML document to the comment
// layer, each with the span it occupies. It adds nothing to the reader's part
// stream.
type CommentProvider struct{}

var _ comment.Provider = CommentProvider{}

// Language implements comment.Provider.
func (CommentProvider) Language() string { return "html" }

// Extensions implements comment.Provider. A format's provider is found by the
// format's name, so it claims no extension.
func (CommentProvider) Extensions() []string { return nil }

// Locate implements comment.Provider.
func (CommentProvider) Locate(_ string, src []byte) (*comment.File, error) {
	return LocateComments(src)
}

// LineText implements comment.Provider: the comment that opens line and closes
// on it, the bytes it occupies through its closing marker, and what it holds
// between the markers.
func (CommentProvider) LineText(line []byte) (n int, text string, ok bool) {
	if !bytes.HasPrefix(line, []byte(markup.Open)) {
		return 0, "", false
	}
	end, closeLen, closed := commentEnd(line, 0)
	if !closed {
		return 0, "", false
	}
	return end, string(line[len(markup.Open) : end-closeLen]), true
}

// Canary implements comment.Provider: a comment with a doubled word, above an
// element whose attribute value holds a comment marker that is content.
func (CommentProvider) Canary() comment.Canary {
	return comment.Canary{
		Name:   "an HTML comment with a doubled word, above an attribute value holding a comment marker",
		Source: []byte("<!-- Greets the the reader. -->\n<p title=\"<!-- not a comment -->\">Hello</p>\n"),
		Block:  "comment/p",
	}
}

// ErrCommentsUnlocated reports an HTML document whose comments the byte scan,
// the HTML tokenizer and the HTML parser do not agree on. It wraps
// comment.ErrUnlocated.
var ErrCommentsUnlocated = fmt.Errorf("html: %w", comment.ErrUnlocated)

// LocateComments returns the comments in an HTML document.
//
// The bytes are scanned by the HTML tokenizer's rules: `<!--` opens a comment in
// content, and never inside a tag, a doctype, a bogus comment such as `<?xml?>`,
// or the text of a raw text element such as `<script>`, `<style>` or
// `<textarea>`. A comment closes on `-->` or `--!>`, and `<!-->` and `<!--->`
// are empty comments. The tokenizer must place every comment at the bytes the
// scan does, and the HTML parser, which reads SVG and MathML content by rules of
// its own, must find the same comments, or the document is refused with
// ErrCommentsUnlocated. So is a document that ends inside a comment.
//
// Each comment is its own block, named for the element that follows it, or for
// the element that closed earlier on its line. An `id` attribute names the
// element among its siblings.
func LocateComments(src []byte) (*comment.File, error) {
	return locateComments(src, classify)
}

// CommentSpan is where one `<!-- -->` comment sits in HTML bytes, with the
// length of its closing marker.
type CommentSpan struct {
	Start, End int
	Close      int
}

// CommentSpans returns every `<!-- -->` comment in HTML bytes, held to the HTML
// tokenizer and the HTML parser as LocateComments holds them, for a format whose
// documents embed HTML.
func CommentSpans(src []byte) ([]CommentSpan, error) {
	tokens, err := readHTMLComments(src)
	if err != nil {
		return nil, err
	}
	spans := make([]CommentSpan, len(tokens.comments))
	for i, c := range tokens.comments {
		_, closeLen, _ := commentEnd(src, c.start)
		spans[i] = CommentSpan{Start: c.start, End: c.end, Close: closeLen}
	}
	return spans, nil
}

// readHTMLComments reads the comments with the scan and the tokenizer, and
// holds the two to each other and to the parser.
func readHTMLComments(src []byte) (*tokenized, error) {
	scanned, err := scanComments(src)
	if err != nil {
		return nil, err
	}
	tokens, err := tokenizeComments(src)
	if err != nil {
		return nil, err
	}
	if err := agree(scanned, tokens.comments); err != nil {
		return nil, err
	}
	if err := agreeWithParser(src, tokens.data); err != nil {
		return nil, err
	}
	return tokens, nil
}

func locateComments(src []byte, classify func(inner string) (string, bool)) (*comment.File, error) {
	tokens, err := readHTMLComments(src)
	if err != nil {
		return nil, err
	}
	idx := format.NewLineIndex(src)
	file := &comment.File{Language: "html"}
	for _, c := range tokens.comments {
		_, closeLen, _ := commentEnd(src, c.start)
		inner := string(src[c.start+len(markup.Open) : c.end-closeLen])
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

// directiveForms are the comments tools read in HTML files.
var directiveForms = []markup.DirectiveForm{
	conditionalComment, serverSideInclude, markup.Markdownlint,
	markup.PrettierIgnore, markup.FormatterToggle, markup.Suppress, markup.ReSharper,
}

// conditionalRe opens a conditional comment's condition.
var conditionalRe = regexp.MustCompile(`^\[if\b`)

// conditionalComment is a conditional comment, which Outlook and early versions
// of Internet Explorer read: `<!--[if mso]>…<![endif]-->`, and the
// `<!--[if !mso]><!-->` and `<!--<![endif]-->` that show markup to every other
// client.
var conditionalComment = markup.DirectiveForm{Name: "conditional-comment", Match: func(b string) bool {
	return conditionalRe.MatchString(b) || strings.HasPrefix(b, "<![endif]") || strings.HasSuffix(b, "<![endif]")
}}

// ssiRe is a server-side include's command.
var ssiRe = regexp.MustCompile(`^#(include|echo|config|exec|flastmod|fsize|printenv|set|if|elif|else|endif)\b`)

// serverSideInclude is a server-side include, which a web server replaces
// before it sends the page: `<!--#include virtual="/footer.html" -->`.
var serverSideInclude = markup.DirectiveForm{Name: "ssi", Match: ssiRe.MatchString}

// reactMarkers are the comments React writes into a page it renders on the
// server and reads back when it hydrates the page. React writes each with
// nothing around the marker, so a comment that holds the same word with spaces
// is prose.
var reactMarkers = []string{"$", "/$", "$?", "$!", "&", "/&", "F", "F!", "html", "head", "body"}

func classify(inner string) (string, bool) {
	return classifyWith(inner, directiveForms, true)
}

func classifyWith(inner string, forms []markup.DirectiveForm, react bool) (string, bool) {
	if react && slices.Contains(reactMarkers, inner) {
		return "react", true
	}
	return markup.Classify(inner, forms)
}

type span struct{ start, end int }

func unlocated(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrCommentsUnlocated}, args...)...)
}

// rawTextElements are the elements whose content the tokenizer reads as text
// up to their end tag.
var rawTextElements = []string{"iframe", "noembed", "noframes", "noscript", "plaintext", "script", "style", "textarea", "title", "xmp"}

// scanComments finds every comment in the bytes.
func scanComments(src []byte) ([]span, error) {
	var out []span
	for i := 0; i < len(src); {
		lt := bytes.IndexByte(src[i:], '<')
		if lt < 0 || i+lt+1 >= len(src) {
			break
		}
		i += lt
		switch c := src[i+1]; {
		case isLetter(c):
			end, name := tagEnd(src, i+1)
			i = end
			if slices.Contains(rawTextElements, name) {
				i = rawTextEnd(src, i, name)
			}
		case c == '/':
			switch {
			case i+2 >= len(src):
				i = len(src)
			case src[i+2] == '>':
				i += 3
			case isLetter(src[i+2]):
				i, _ = tagEnd(src, i+2)
			default:
				i = pastCloseAngle(src, i+2)
			}
		case c == '!' && bytes.HasPrefix(src[i+2:], []byte("--")):
			end, _, closed := commentEnd(src, i)
			if !closed {
				return nil, unlocated("the comment at byte %d is not closed", i)
			}
			out = append(out, span{i, end})
			i = end
		case c == '!' || c == '?':
			i = pastCloseAngle(src, i+2)
		default:
			i++
		}
	}
	return out, nil
}

// commentEnd returns where the comment opening at i ends, the length of its
// closing marker, and whether it closes before the end of src.
func commentEnd(src []byte, i int) (end, closeLen int, closed bool) {
	dashes, beginning := 0, true
	for j := i + len(markup.Open); j < len(src); j++ {
		switch src[j] {
		case '-':
			dashes++
			continue
		case '>':
			if dashes >= 2 || beginning {
				return j + 1, min(dashes, 2) + 1, true
			}
		case '!':
			if dashes >= 2 && j+1 < len(src) {
				switch src[j+1] {
				case '>':
					return j + 2, len("--!>"), true
				case '-':
					dashes, beginning = 1, false
					j++
					continue
				}
			}
		}
		dashes, beginning = 0, false
	}
	return len(src), 0, false
}

// tagEnd returns the offset just past the tag whose name starts at j, and the
// name in lower case. An attribute value may hold `>` in quotes.
func tagEnd(src []byte, j int) (int, string) {
	k := j + 1
	for k < len(src) && !isSpace(src[k]) && src[k] != '/' && src[k] != '>' {
		k++
	}
	name := strings.ToLower(string(src[j:k]))
	for k = skipSpace(src, k); k < len(src); k = skipSpace(src, k) {
		if src[k] == '>' {
			return k + 1, name
		}
		k = attrValueEnd(src, attrKeyEnd(src, k))
	}
	return len(src), name
}

func attrKeyEnd(src []byte, k int) int {
	for start := k; k < len(src); k++ {
		switch src[k] {
		case '=':
			if k == start {
				continue
			}
			return k
		case ' ', '\n', '\r', '\t', '\f', '/', '>':
			return k
		}
	}
	return k
}

func attrValueEnd(src []byte, k int) int {
	k = skipSpace(src, k)
	if k >= len(src) {
		return k
	}
	switch src[k] {
	case '/':
		return k + 1
	case '=':
	default:
		return k
	}
	k = skipSpace(src, k+1)
	if k >= len(src) {
		return k
	}
	switch q := src[k]; q {
	case '>':
		return k
	case '"', '\'':
		if end := bytes.IndexByte(src[k+1:], q); end >= 0 {
			return k + 1 + end + 1
		}
		return len(src)
	}
	for k < len(src) && !isSpace(src[k]) && src[k] != '>' {
		k++
	}
	return k
}

// rawTextEnd returns where the text of a raw text element that starts at i
// ends: at its end tag, or at the end of the file.
func rawTextEnd(src []byte, i int, name string) int {
	switch name {
	case "plaintext":
		return len(src)
	case "script":
		return scriptEnd(src, i)
	}
	for j := i; j+1 < len(src); j++ {
		if src[j] == '<' && src[j+1] == '/' {
			if _, ok := rawEndTag(src, j+2, name); ok {
				return j
			}
		}
	}
	return len(src)
}

// rawEndTag reports whether the end tag of name, less its `</`, starts at k.
// When it does, end is just past the character that follows the name; when it
// does not, end is where the name stopped matching.
func rawEndTag(src []byte, k int, name string) (end int, ok bool) {
	for m := range len(name) {
		if k >= len(src) || (src[k] != name[m] && src[k] != name[m]-('a'-'A')) {
			return k, false
		}
		k++
	}
	if k < len(src) && (isSpace(src[k]) || src[k] == '/' || src[k] == '>') {
		return k + 1, true
	}
	return k, false
}

// scriptEnd returns where a script element's text ends, following the
// tokenizer's rules for an end tag hidden behind `<!--` in the script.
func scriptEnd(src []byte, i int) int {
	const (
		data = iota
		lessThan
		escapeStart
		escapeStartDash
		escaped
		escapedDash
		escapedDashDash
		escapedLessThan
		doubleEscaped
		doubleEscapedDash
		doubleEscapedDashDash
		doubleEscapedLessThan
	)
	state := data
	for j := i; j < len(src); {
		c := src[j]
		j++
		switch state {
		case data:
			if c == '<' {
				state = lessThan
			}
		case lessThan:
			switch c {
			case '/':
				end, ok := rawEndTag(src, j, "script")
				if ok {
					return j - 2
				}
				j, state = end, data
			case '!':
				state = escapeStart
			default:
				j, state = j-1, data
			}
		case escapeStart, escapeStartDash:
			switch {
			case c == '-' && state == escapeStart:
				state = escapeStartDash
			case c == '-':
				state = escapedDashDash
			default:
				j, state = j-1, data
			}
		case escaped, escapedDash, escapedDashDash:
			switch {
			case c == '-' && state == escaped:
				state = escapedDash
			case c == '-':
				state = escapedDashDash
			case c == '<':
				state = escapedLessThan
			case c == '>' && state == escapedDashDash:
				state = data
			default:
				state = escaped
			}
		case escapedLessThan:
			switch {
			case c == '/':
				end, ok := rawEndTag(src, j, "script")
				if ok {
					return j - 2
				}
				j, state = end, escaped
			case isLetter(c):
				end, ok := rawEndTag(src, j-1, "script")
				switch {
				case ok:
					state = doubleEscaped
				default:
					state = escaped
				}
				j = end
			default:
				j, state = j-1, data
			}
		case doubleEscaped, doubleEscapedDash, doubleEscapedDashDash:
			switch {
			case c == '-' && state == doubleEscaped:
				state = doubleEscapedDash
			case c == '-':
				state = doubleEscapedDashDash
			case c == '<':
				state = doubleEscapedLessThan
			case c == '>' && state == doubleEscapedDashDash:
				state = data
			default:
				state = doubleEscaped
			}
		case doubleEscapedLessThan:
			if c != '/' {
				j, state = j-1, doubleEscaped
				continue
			}
			end, ok := rawEndTag(src, j, "script")
			j = end
			if ok {
				state = escaped
			} else {
				state = doubleEscaped
			}
		}
	}
	return len(src)
}

func pastCloseAngle(src []byte, k int) int {
	if end := bytes.IndexByte(src[min(k, len(src)):], '>'); end >= 0 {
		return k + end + 1
	}
	return len(src)
}

func skipSpace(src []byte, k int) int {
	for k < len(src) && isSpace(src[k]) {
		k++
	}
	return k
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f'
}

func isLetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// tokenComment is a comment the tokenizer reports, with what it sits on.
type tokenComment struct {
	start, end int
	subject    string
}

type tokenized struct {
	// comments are the `<!-- -->` comments, in file order.
	comments []tokenComment
	// data holds what every comment token carries, bogus comments included,
	// as the parser receives it.
	data []string
}

// tokenizeComments reads the document with the HTML tokenizer, whose tokens'
// raw bytes partition the file, so a comment's span is the bytes of its token.
func tokenizeComments(src []byte) (*tokenized, error) {
	z := html.NewTokenizer(bytes.NewReader(src))
	z.SetMaxBuf(0)
	lines := format.NewLineIndex(src)
	out := &tokenized{}
	var pending []int
	closedName, closedLine := "", 0
	off := 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				break
			}
			return nil, unlocated("the HTML tokenizer stops at byte %d: %v", off, z.Err())
		}
		n := len(z.Raw())
		tok := z.Token()
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			for _, i := range pending {
				out.comments[i].subject = "comment/" + segment(tok)
			}
			pending = pending[:0]
			closedLine = 0
		case html.EndTagToken:
			closedName, closedLine = tok.Data, lines.Line(off+n-1)
		case html.CommentToken:
			out.data = append(out.data, tok.Data)
			if !bytes.HasPrefix(src[off:off+n], []byte(markup.Open)) {
				break
			}
			out.comments = append(out.comments, tokenComment{start: off, end: off + n})
			lineStart := bytes.LastIndexByte(src[:off], '\n') + 1
			if closedLine == lines.Line(off) && len(bytes.TrimSpace(src[lineStart:off])) > 0 {
				out.comments[len(out.comments)-1].subject = "comment/" + closedName
			} else {
				pending = append(pending, len(out.comments)-1)
			}
		}
		off += n
	}
	for _, i := range pending {
		out.comments[i].subject = "comment/document"
	}
	return out, nil
}

// segment is an element's path segment: its name, and its id in brackets.
func segment(t html.Token) string {
	for _, a := range t.Attr {
		if a.Key == "id" && a.Val != "" {
			return t.Data + "[" + strings.ReplaceAll(a.Val, "/", "-") + "]"
		}
	}
	return t.Data
}

// agree holds the scan to the tokenizer: the same comments at the same bytes.
func agree(scanned []span, tokens []tokenComment) error {
	for i := range max(len(scanned), len(tokens)) {
		switch {
		case i >= len(tokens):
			return unlocated("the scan finds a comment at byte %d that the HTML tokenizer does not report", scanned[i].start)
		case i >= len(scanned):
			return unlocated("the HTML tokenizer reports a comment at byte %d that the scan does not find", tokens[i].start)
		case scanned[i] != span{tokens[i].start, tokens[i].end}:
			return unlocated("the scan places a comment at bytes %d-%d and the HTML tokenizer at %d-%d",
				scanned[i].start, scanned[i].end, tokens[i].start, tokens[i].end)
		}
	}
	return nil
}

// agreeWithParser holds the tokenizer's comments to the comments the HTML
// parser finds, which differ where the parser reads foreign content, such as
// a CDATA section in SVG.
func agreeWithParser(src []byte, tokenized []string) error {
	doc, err := html.Parse(bytes.NewReader(src))
	if err != nil {
		return unlocated("the HTML parser: %v", err)
	}
	var parsed []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.CommentNode {
			parsed = append(parsed, n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if !slices.Equal(slices.Sorted(slices.Values(tokenized)), slices.Sorted(slices.Values(parsed))) {
		return unlocated("the HTML parser finds %d comment(s) where the tokenizer reads %d, as it does in SVG or MathML content", len(parsed), len(tokenized))
	}
	return nil
}
