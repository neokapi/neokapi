package po

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// CommentProvider supplies the comments of a PO file to the comment layer, each
// with the span it occupies. It adds nothing to the reader's part stream.
type CommentProvider struct{}

var _ comment.Provider = CommentProvider{}

// Language implements comment.Provider.
func (CommentProvider) Language() string { return "po" }

// Extensions implements comment.Provider. A format's provider is found by the
// format's name, so it claims no extension.
func (CommentProvider) Extensions() []string { return nil }

// Locate implements comment.Provider.
func (CommentProvider) Locate(_ string, src []byte) (*comment.File, error) {
	return LocateComments(src)
}

// LineText implements comment.Provider: a comment line up to its trailing
// blanks, and what follows its marker, which is `#` or one of `#.`, `#:`, `#,`
// and `#|`.
func (CommentProvider) LineText(line []byte) (n int, text string, ok bool) {
	if !isCommentLine(line) {
		return 0, "", false
	}
	trimmed := bytes.TrimRight(line, " \t\r")
	return len(trimmed), string(trimmed[markerLen(trimmed):]), true
}

// Canary implements comment.Provider: a translator comment with a doubled word,
// above an entry whose msgstr holds a `#` that is content.
func (CommentProvider) Canary() comment.Canary {
	return comment.Canary{
		Name:   "a PO translator comment with a doubled word, above a msgstr holding a #",
		Source: []byte("# Greets the the reader.\n#: src/app.js:1\nmsgid \"Hello\"\nmsgstr \"Bonjour # monde\"\n"),
		Block:  "comment/Hello",
	}
}

// ErrCommentsUnlocated reports a PO file whose comment lines the scan and the
// PO reader do not agree on, or that the reader reads transcoded. It wraps
// comment.ErrUnlocated.
var ErrCommentsUnlocated = fmt.Errorf("po: %w", comment.ErrUnlocated)

// LocateComments returns the comments in a PO file.
//
// A comment is a line that opens with `#`, other than an obsolete entry's `#~`
// line. Translator comments are prose, and consecutive ones form one comment.
// An extracted comment (`#.`) is rewritten by the next extraction, so it is set
// aside as generated. References (`#:`), flags (`#,`) and a previous msgid
// (`#|`) are read by gettext's tools, so they are set aside as directives, with
// the form "reference", "flags" or "previous". The lines must be the comments
// the PO reader parses, or the file is refused with ErrCommentsUnlocated. So is
// a file the reader reads transcoded, where a span in the text it reads is not
// a span in the file.
//
// A comment is named for the entry it sits in, as `comment/<msgctxt>/<msgid>`,
// or `comment/header` in the header entry.
func LocateComments(src []byte) (*comment.File, error) {
	bom, body := format.SplitBOM(src)
	if !utf8.Valid(body) {
		return nil, commentsUnlocated("the file is not UTF-8, and the reader reads it transcoded")
	}
	if cs := detectHeaderCharset(src); cs != "" && !isUTF8Charset(cs) && !isASCII(body) {
		return nil, commentsUnlocated("the header declares the charset %s, and the reader reads the file transcoded", cs)
	}
	lines := scanCommentLines(src, len(bom))
	if err := agreeWithReader(body, lines); err != nil {
		return nil, err
	}

	idx := format.NewLineIndex(src)
	file := &comment.File{Language: "po"}
	exclude := func(l poCommentLine, reason comment.Reason, form string) {
		file.Excluded = append(file.Excluded, comment.Excluded{Start: l.start, End: l.end, Lines: idx.Range(l.start, l.end), Reason: reason, Form: form})
	}
	var run []poCommentLine
	flush := func() {
		first, last := 0, len(run)
		for first < last && run[first].blank() {
			exclude(run[first], comment.ReasonBlank, "")
			first++
		}
		for last > first && run[last-1].blank() {
			last--
		}
		if first < last {
			lines := run[first:last]
			texts := make([]string, len(lines))
			for i, l := range lines {
				texts[i] = l.prose()
			}
			start, end := lines[0].start, lines[len(lines)-1].end
			file.Comments = append(file.Comments, comment.Comment{
				Start:   start,
				End:     end,
				Lines:   idx.Range(start, end),
				Style:   comment.StyleLine,
				Subject: entrySubject(src, end),
				Runs:    []model.Run{model.TextR(strings.Join(texts, "\n"))},
			})
		}
		for _, l := range run[last:] {
			exclude(l, comment.ReasonBlank, "")
		}
		run = nil
	}
	for _, l := range lines {
		switch l.kind {
		case '.':
			flush()
			exclude(l, comment.ReasonGenerated, "")
		case ':':
			flush()
			exclude(l, comment.ReasonDirective, "reference")
		case ',':
			flush()
			exclude(l, comment.ReasonDirective, "flags")
		case '|':
			flush()
			exclude(l, comment.ReasonDirective, "previous")
		default:
			if len(run) > 0 && run[len(run)-1].line+1 != l.line {
				flush()
			}
			run = append(run, l)
		}
	}
	flush()
	return file, nil
}

func commentsUnlocated(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrCommentsUnlocated}, args...)...)
}

// poCommentLine is one comment line.
type poCommentLine struct {
	// start and end are its span, from `#` to its last byte before trailing
	// blanks.
	start, end int
	// line is its index among the file's lines.
	line int
	// kind is the character after `#` for `.`, `:`, `,` and `|`, and a space
	// for a translator comment.
	kind byte
	// text is the line as the reader reads it.
	text string
}

func (l poCommentLine) blank() bool {
	return strings.TrimSpace(strings.TrimPrefix(l.text, "#")) == ""
}

// prose is what a translator comment holds.
func (l poCommentLine) prose() string {
	return strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(l.text, "#"), " "), " \t")
}

func isCommentLine(b []byte) bool {
	return bytes.HasPrefix(b, []byte("#")) && !bytes.HasPrefix(b, []byte("#~"))
}

func commentKind(b []byte) byte {
	if len(b) > 1 {
		switch b[1] {
		case '.', ':', ',', '|':
			return b[1]
		}
	}
	return ' '
}

func markerLen(b []byte) int {
	if commentKind(b) != ' ' {
		return 2
	}
	return 1
}

// scanCommentLines finds every comment line after the byte-order mark.
func scanCommentLines(src []byte, base int) []poCommentLine {
	var out []poCommentLine
	for n, start := 0, base; start < len(src); n++ {
		end, next := len(src), len(src)
		if i := bytes.IndexByte(src[start:], '\n'); i >= 0 {
			end, next = start+i, start+i+1
		}
		raw := bytes.TrimSuffix(src[start:end], []byte("\r"))
		if isCommentLine(raw) {
			trimmed := bytes.TrimRight(raw, " \t")
			out = append(out, poCommentLine{start: start, end: start + len(trimmed), line: n, kind: commentKind(raw), text: string(raw)})
		}
		start = next
	}
	return out
}

// agreeWithReader holds the scan to the PO reader's parse: the translator
// comments, extracted comments, references and flags of its entries are the
// scan's lines of each kind, as many times each. A `#` followed by anything
// else is a comment line the reader parses and keeps nothing of.
func agreeWithReader(body []byte, lines []poCommentLine) error {
	r := &Reader{}
	r.Doc = &model.RawDocument{Reader: io.NopCloser(bytes.NewReader(body))}
	parsed := map[string]int{}
	for _, e := range r.parseEntries() {
		for _, t := range e.translatorComments {
			parsed["# "+t]++
		}
		for _, t := range e.extractedComments {
			parsed["#."+t]++
		}
		for _, t := range e.references {
			parsed["#:"+t]++
		}
		for _, t := range e.flags {
			parsed["#,"+t]++
		}
	}
	scanned := map[string]int{}
	for _, l := range lines {
		switch {
		case l.kind == '.' || l.kind == ':' || l.kind == ',':
			scanned["#"+string(l.kind)+strings.TrimSpace(l.text[2:])]++
		case strings.HasPrefix(l.text, "# "):
			scanned["# "+strings.TrimPrefix(l.text, "# ")]++
		case l.text == "#":
			scanned["# #"]++
		}
	}
	if !maps.Equal(parsed, scanned) {
		return commentsUnlocated("the scan's comment lines are not the comments the PO reader parses")
	}
	return nil
}

// entrySubject names what a comment ending at offset sits on: the entry whose
// msgctxt and msgid follow it before the next blank line.
func entrySubject(src []byte, offset int) string {
	nl := bytes.IndexByte(src[offset:], '\n')
	if nl < 0 {
		return "comment/document"
	}
	var ctx, id strings.Builder
	var cur *strings.Builder
	seenID := false
	for start := offset + nl + 1; start < len(src); {
		end, next := len(src), len(src)
		if i := bytes.IndexByte(src[start:], '\n'); i >= 0 {
			end, next = start+i, start+i+1
		}
		line := strings.TrimSuffix(string(src[start:end]), "\r")
		switch {
		case strings.HasPrefix(line, "\""):
			if cur != nil {
				cur.WriteString(unquotePO(line))
			}
		case seenID:
			return entryPath(ctx.String(), id.String())
		case strings.HasPrefix(line, "msgctxt "):
			ctx.WriteString(unquotePO(line[len("msgctxt "):]))
			cur = &ctx
		case strings.HasPrefix(line, "msgid "):
			id.WriteString(unquotePO(line[len("msgid "):]))
			cur, seenID = &id, true
		case strings.TrimSpace(line) == "":
			return "comment/document"
		default:
			cur = nil
		}
		start = next
	}
	if seenID {
		return entryPath(ctx.String(), id.String())
	}
	return "comment/document"
}

func entryPath(ctx, id string) string {
	if path := model.StructuralPath(ctx, id); path != "" {
		return "comment/" + path
	}
	return "comment/header"
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
