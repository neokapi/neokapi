package xml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/markup"
	"github.com/neokapi/neokapi/core/format"
)

// CommentProvider supplies the comments of an XML document to the comment
// layer, each with the span it occupies. Every format whose reader reads a
// plain XML file registers it under its own name, which it reports as its
// language, so a comment's analyzer and blocks name the format that read the
// file. It adds nothing to any reader's part stream.
type CommentProvider struct {
	// Format names the format the provider serves, such as "androidxml".
	// Empty is "xml".
	Format string
}

var _ comment.Provider = CommentProvider{}

// Language implements comment.Provider.
func (p CommentProvider) Language() string {
	if p.Format == "" {
		return "xml"
	}
	return p.Format
}

// Extensions implements comment.Provider. A format's provider is found by the
// format's name, so it claims no extension.
func (CommentProvider) Extensions() []string { return nil }

// Locate implements comment.Provider.
func (p CommentProvider) Locate(_ string, src []byte) (*comment.File, error) {
	return LocateComments(p.Language(), src)
}

// LineText reports the comment that opens line and closes on it: n is the
// bytes it occupies, through `-->`, and text is what it holds between the
// markers. A line whose comment runs on to the next line holds no whole
// comment.
func (CommentProvider) LineText(line []byte) (n int, text string, ok bool) {
	if !bytes.HasPrefix(line, []byte(markup.Open)) {
		return 0, "", false
	}
	end, err := commentEnd(line, 0)
	if err != nil {
		return 0, "", false
	}
	return end, string(line[len(markup.Open) : end-len(markup.Close)]), true
}

// Canary implements comment.Provider: a comment with a doubled word, above an
// element whose CDATA section holds a comment marker that is content.
func (CommentProvider) Canary() comment.Canary {
	return comment.Canary{
		Name:   "an XML comment with a doubled word, above a CDATA section holding a comment marker",
		Source: []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n  <!-- Greets the the reader. -->\n  <string name=\"greeting\"><![CDATA[<!-- not a comment -->]]></string>\n</resources>\n"),
		Block:  "comment/resources/string[greeting]",
	}
}

// ErrCommentsUnlocated reports an XML document whose comments the byte scan and
// the XML parser do not agree on, or that the parser rejects. It wraps
// comment.ErrUnlocated.
var ErrCommentsUnlocated = fmt.Errorf("xml: %w", comment.ErrUnlocated)

// directiveForms are the comments tools read in XML files.
var directiveForms = []markup.DirectiveForm{markup.PrettierIgnore, markup.FormatterToggle, markup.Suppress, markup.ReSharper}

// LocateComments returns the comments in an XML document, reporting language
// as the file's language.
//
// The bytes are scanned by XML's lexical rules: `<!--` opens a comment in
// content, and never inside a tag, a CDATA section, a processing instruction
// or a declaration. The XML parser reports each comment with its offsets, and
// the two must find the same comments at the same bytes, or the document is
// refused with ErrCommentsUnlocated. So is a document the parser rejects, such
// as one with `--` inside a comment, which the XML specification forbids, and
// one with a comment inside its DOCTYPE, which the parser reports as part of
// the declaration.
//
// Each comment is its own block, named for the element it sits on: the element
// that follows it, the element that closed earlier on its line, or else the
// element that holds it.
func LocateComments(language string, src []byte) (*comment.File, error) {
	return locateComments(language, src, directiveForms)
}

func locateComments(language string, src []byte, forms []markup.DirectiveForm) (*comment.File, error) {
	scanned, err := scanComments(src)
	if err != nil {
		return nil, err
	}
	parsed, err := parseComments(src)
	if err != nil {
		return nil, err
	}
	if err := agree(scanned, parsed); err != nil {
		return nil, err
	}
	idx := format.NewLineIndex(src)
	file := &comment.File{Language: language}
	for _, c := range parsed {
		inner := string(src[c.start+len(markup.Open) : c.end-len(markup.Close)])
		lines := idx.Range(c.start, c.end)
		if markup.Blank(inner) {
			file.Excluded = append(file.Excluded, comment.Excluded{Start: c.start, End: c.end, Lines: lines, Reason: comment.ReasonBlank})
			continue
		}
		if form, ok := markup.Classify(inner, forms); ok {
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

type span struct{ start, end int }

func unlocated(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrCommentsUnlocated}, args...)...)
}

// scanComments finds every comment in the bytes.
func scanComments(src []byte) ([]span, error) {
	var out []span
	for i := 0; i < len(src); {
		lt := bytes.IndexByte(src[i:], '<')
		if lt < 0 {
			break
		}
		i += lt
		rest := src[i:]
		var err error
		switch {
		case bytes.HasPrefix(rest, []byte(markup.Open)):
			start := i
			if i, err = commentEnd(src, i); err == nil {
				out = append(out, span{start, i})
			}
		case bytes.HasPrefix(rest, []byte("<![CDATA[")):
			i, err = past(src, i, "]]>", "CDATA section")
		case bytes.HasPrefix(rest, []byte("<?")):
			i, err = past(src, i, "?>", "processing instruction")
		case bytes.HasPrefix(rest, []byte("<!")):
			i, err = declarationEnd(src, i)
		default:
			i, err = tagEnd(src, i)
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// commentEnd returns the offset just past the comment opening at i. The XML
// specification allows `--` in a comment only as part of the closing `-->`.
func commentEnd(src []byte, i int) (int, error) {
	from := i + len(markup.Open)
	at := bytes.Index(src[from:], []byte("--"))
	if at < 0 {
		return 0, unlocated("the comment at byte %d is not closed", i)
	}
	at += from
	if at+2 >= len(src) || src[at+2] != '>' {
		return 0, unlocated("the comment at byte %d holds `--`, which the XML specification allows only in its closing `-->`", i)
	}
	return at + 3, nil
}

// past returns the offset just past the terminator of the construct opening at
// i.
func past(src []byte, i int, terminator, what string) (int, error) {
	at := bytes.Index(src[i:], []byte(terminator))
	if at < 0 {
		return 0, unlocated("the %s at byte %d is not closed", what, i)
	}
	return i + at + len(terminator), nil
}

// tagEnd returns the offset just past the tag opening at i, whose attribute
// values may hold `>`.
func tagEnd(src []byte, i int) (int, error) {
	var quote byte
	for j := i + 1; j < len(src); j++ {
		switch c := src[j]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return j + 1, nil
		}
	}
	return 0, unlocated("the tag at byte %d is not closed", i)
}

// declarationEnd returns the offset just past a declaration such as
// `<!DOCTYPE>`. A comment inside its internal subset is refused.
func declarationEnd(src []byte, i int) (int, error) {
	var quote byte
	subset := false
	for j := i + 2; j < len(src); j++ {
		switch c := src[j]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case subset && bytes.HasPrefix(src[j:], []byte(markup.Open)):
			return 0, unlocated("the declaration at byte %d holds a comment, which the XML parser reports as part of the declaration", i)
		case c == '[':
			subset = true
		case c == ']':
			subset = false
		case c == '>' && !subset:
			return j + 1, nil
		}
	}
	return 0, unlocated("the declaration at byte %d is not closed", i)
}

// parsedComment is a comment the XML parser reports, with what it sits on.
type parsedComment struct {
	start, end int
	subject    string
}

// parseComments reads the document with the XML parser, which reports each
// token's end offset, so a comment's span is the bytes between the end of the
// token before it and its own end.
func parseComments(src []byte) ([]parsedComment, error) {
	d := xml.NewDecoder(bytes.NewReader(src))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	w := &subjectWalk{src: src, lines: format.NewLineIndex(src)}
	prev := 0
	for {
		tok, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, unlocated("the XML parser stops at byte %d: %v", d.InputOffset(), err)
		}
		end := int(d.InputOffset())
		switch t := tok.(type) {
		case xml.StartElement:
			w.startElement(segment(t))
		case xml.EndElement:
			w.endElement(end)
		case xml.CharData:
			if len(bytes.TrimSpace(t)) > 0 {
				w.settle(w.path())
			}
		case xml.Comment:
			w.comment(prev, end)
		}
		prev = end
	}
	w.settle(w.path())
	return w.comments, nil
}

// subjectWalk names what each comment sits on as the parser's tokens go by.
type subjectWalk struct {
	src   []byte
	lines *format.LineIndex
	// stack holds the path segment of each open element.
	stack []string
	// pending indexes the comments waiting for what follows them.
	pending []int
	// closedPath and closedLine are the element that last closed at the
	// current depth and the line its end tag ends on; closedLine is 0 when
	// nothing has.
	closedPath string
	closedLine int
	comments   []parsedComment
}

func (w *subjectWalk) path(segments ...string) string {
	p := strings.Join(append(append([]string{}, w.stack...), segments...), "/")
	if p == "" {
		p = "document"
	}
	return "comment/" + p
}

// settle names every pending comment subject.
func (w *subjectWalk) settle(subject string) {
	for _, i := range w.pending {
		w.comments[i].subject = subject
	}
	w.pending = w.pending[:0]
}

func (w *subjectWalk) startElement(seg string) {
	w.settle(w.path(seg))
	w.stack = append(w.stack, seg)
	w.closedLine = 0
}

func (w *subjectWalk) endElement(end int) {
	w.settle(w.path())
	closed := w.path()
	if len(w.stack) > 0 {
		w.stack = w.stack[:len(w.stack)-1]
	}
	w.closedPath, w.closedLine = closed, w.lines.Line(max(end-1, 0))
}

func (w *subjectWalk) comment(start, end int) {
	w.comments = append(w.comments, parsedComment{start: start, end: end})
	line := w.lines.Line(start)
	lineStart := bytes.LastIndexByte(w.src[:start], '\n') + 1
	if w.closedLine == line && len(bytes.TrimSpace(w.src[lineStart:start])) > 0 {
		w.comments[len(w.comments)-1].subject = w.closedPath
		return
	}
	w.pending = append(w.pending, len(w.comments)-1)
}

// keyAttributes name an element among its siblings, in order of preference.
var keyAttributes = []string{"name", "id", "key", "resname"}

// segment is an element's path segment: its name, and the value of its first
// key attribute in brackets.
func segment(t xml.StartElement) string {
	name := t.Name.Local
	if t.Name.Space != "" {
		name = t.Name.Space + ":" + name
	}
	for _, key := range keyAttributes {
		for _, a := range t.Attr {
			if a.Name.Space == "" && a.Name.Local == key && a.Value != "" {
				return name + "[" + strings.ReplaceAll(a.Value, "/", "-") + "]"
			}
		}
	}
	return name
}

// agree holds the scan to the parser: the same comments at the same bytes.
func agree(scanned []span, parsed []parsedComment) error {
	for i := range max(len(scanned), len(parsed)) {
		switch {
		case i >= len(parsed):
			return unlocated("the scan finds a comment at byte %d that the XML parser does not report", scanned[i].start)
		case i >= len(scanned):
			return unlocated("the XML parser reports a comment at byte %d that the scan does not find", parsed[i].start)
		case scanned[i] != span{parsed[i].start, parsed[i].end}:
			return unlocated("the scan places a comment at bytes %d-%d and the XML parser at %d-%d",
				scanned[i].start, scanned[i].end, parsed[i].start, parsed[i].end)
		}
	}
	return nil
}
