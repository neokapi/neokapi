package properties

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// CommentProvider supplies the comments of a Java properties file to the
// comment layer, each with the span it occupies. It adds nothing to the
// reader's part stream.
type CommentProvider struct{}

var _ comment.Provider = CommentProvider{}

// Language implements comment.Provider.
func (CommentProvider) Language() string { return "properties" }

// Extensions implements comment.Provider. A format's provider is found by the
// format's name, so it claims no extension.
func (CommentProvider) Extensions() []string { return nil }

// Locate implements comment.Provider.
func (CommentProvider) Locate(_ string, src []byte) (*comment.File, error) {
	return LocateComments(src)
}

// LineText implements comment.Provider: a comment line up to its trailing
// blanks, and what follows its `#` or `!`.
func (CommentProvider) LineText(line []byte) (n int, text string, ok bool) {
	if len(line) == 0 || (line[0] != '#' && line[0] != '!') {
		return 0, "", false
	}
	trimmed := bytes.TrimRight(line, " \t\r")
	return len(trimmed), string(trimmed[1:]), true
}

// Canary implements comment.Provider: a comment with a doubled word, above a
// key whose value continues onto a line that opens with `#`, which is content.
func (CommentProvider) Canary() comment.Canary {
	return comment.Canary{
		Name:   "a properties comment with a doubled word, above a value continued onto a line opening with #",
		Source: []byte("# Greets the the reader.\ngreeting.text = Hello \\\n  # not a comment\n"),
		Block:  "comment/greeting.text",
	}
}

// ErrCommentsUnlocated reports a properties file whose comment lines the scan
// and the properties reader do not agree on. It wraps comment.ErrUnlocated.
var ErrCommentsUnlocated = fmt.Errorf("properties: %w", comment.ErrUnlocated)

// LocateComments returns the comments in a properties file.
//
// A comment is a line whose first character other than a space or a tab is `#`
// or `!`, unless the line continues a value, whose line before it ends in an odd
// number of backslashes. A line ends at LF, CR or CRLF, as the reader reads it.
// Consecutive comment lines form one comment. The comment lines must be the
// ones the properties reader reads as comments, or the file is refused with
// ErrCommentsUnlocated. Okapi's extraction directives, such as `#_skip`, and
// IntelliJ's `# suppress inspection` are read by tools, so they are set aside
// as directives.
//
// A comment is named for the key that follows it: `comment/greeting.text`, or
// `comment/document` after the last key.
func LocateComments(src []byte) (*comment.File, error) {
	return locateComments(src, propertiesDirectives)
}

// propertiesDirective is one kind of comment line a tool reads. match receives
// the comment marker and what follows it, trimmed, and returns the form.
type propertiesDirective struct {
	name  string
	match func(marker byte, body string) (form string, ok bool)
}

// propertiesDirectives are the comment lines tools read in properties files.
var propertiesDirectives = []propertiesDirective{
	// Okapi's extraction directives, on `#` lines only, as the reader honours
	// them.
	{name: "okapi", match: func(marker byte, body string) (string, bool) {
		switch body {
		case "_text", "_skip", "_btext", "_etext", "_bskip", "_eskip":
			return body, marker == '#'
		}
		return "", false
	}},
	// IntelliJ's suppression of an inspection for the property below it or
	// the whole file.
	{name: "suppress", match: func(_ byte, body string) (string, bool) {
		return "suppress", strings.HasPrefix(body, "suppress inspection ")
	}},
}

func locateComments(src []byte, directives []propertiesDirective) (*comment.File, error) {
	bom, body := format.SplitBOM(src)
	lines := scanCommentLines(src, len(bom))
	if err := agreeWithReader(body, lines); err != nil {
		return nil, err
	}

	idx := format.NewLineIndex(src)
	file := &comment.File{Language: "properties"}
	exclude := func(l propertiesCommentLine, reason comment.Reason, form string) {
		file.Excluded = append(file.Excluded, comment.Excluded{Start: l.start, End: l.end, Lines: idx.Range(l.start, l.end), Reason: reason, Form: form})
	}
	var run []propertiesCommentLine
	flush := func() {
		first, last := 0, len(run)
		for first < last && run[first].prose() == "" {
			exclude(run[first], comment.ReasonBlank, "")
			first++
		}
		for last > first && run[last-1].prose() == "" {
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
				Subject: lines[len(lines)-1].subject,
				Runs:    []model.Run{model.TextR(strings.Join(texts, "\n"))},
			})
		}
		for _, l := range run[last:] {
			exclude(l, comment.ReasonBlank, "")
		}
		run = nil
	}
	for _, l := range lines {
		if form, ok := l.directive(directives); ok {
			flush()
			exclude(l, comment.ReasonDirective, form)
			continue
		}
		if len(run) > 0 && run[len(run)-1].line+1 != l.line {
			flush()
		}
		run = append(run, l)
	}
	flush()
	return file, nil
}

// propertiesCommentLine is one comment line.
type propertiesCommentLine struct {
	// start and end are its span, from its marker to its last byte before
	// trailing blanks.
	start, end int
	// line is its index among the file's physical lines.
	line int
	// content is the line as the reader reads it, leading blanks included.
	content string
	// subject names the key that follows it.
	subject string
}

func (l propertiesCommentLine) prose() string {
	return strings.TrimRight(extractCommentText(l.content), " \t")
}

func (l propertiesCommentLine) directive(directives []propertiesDirective) (string, bool) {
	trimmed := strings.TrimSpace(l.content)
	for _, d := range directives {
		if form, ok := d.match(trimmed[0], strings.TrimSpace(trimmed[1:])); ok {
			return form, true
		}
	}
	return "", false
}

// scanCommentLines finds every comment line after the byte-order mark, and
// names each for the key that follows it.
func scanCommentLines(src []byte, base int) []propertiesCommentLine {
	var out []propertiesCommentLine
	pending := 0
	inContinuation := false
	for n, start := 0, base; start < len(src); n++ {
		end, next := physicalLineEnd(src, start)
		content := string(src[start:end])
		trimmed := strings.TrimLeft(content, " \t")
		switch {
		case inContinuation:
			inContinuation = hasContinuation(trimmed)
		case trimmed == "" || strings.TrimSpace(content) == "":
		case trimmed[0] == '#' || trimmed[0] == '!':
			at := start + len(content) - len(trimmed)
			out = append(out, propertiesCommentLine{
				start: at, end: at + len(strings.TrimRight(trimmed, " \t")), line: n, content: content,
			})
		default:
			key, _, _ := parseProperty(content)
			for ; pending < len(out); pending++ {
				out[pending].subject = "comment/" + strings.ReplaceAll(key, "/", "-")
			}
			inContinuation = hasContinuation(content)
		}
		start = next
	}
	for ; pending < len(out); pending++ {
		out[pending].subject = "comment/document"
	}
	return out
}

// physicalLineEnd returns where the line starting at start ends and where the
// next begins, with LF, CR and CRLF each ending a line.
func physicalLineEnd(src []byte, start int) (end, next int) {
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '\n':
			return i, i + 1
		case '\r':
			if i+1 < len(src) && src[i+1] == '\n' {
				return i, i + 2
			}
			return i, i + 1
		}
	}
	return len(src), len(src)
}

// agreeWithReader holds the scan to the properties reader: the lines it reads
// as comments are the scan's comment lines, in order.
func agreeWithReader(body []byte, lines []propertiesCommentLine) error {
	r := NewReader()
	r.body = bytes.NewReader(body)
	var parsed []string
	for l := range r.logicalLines() {
		if l.isComment {
			parsed = append(parsed, l.content)
		}
	}
	scanned := make([]string, len(lines))
	for i, l := range lines {
		scanned[i] = l.content
	}
	if !slices.Equal(parsed, scanned) {
		return fmt.Errorf("%w: the scan finds %d comment lines where the properties reader reads %d, or reads them differently", ErrCommentsUnlocated, len(scanned), len(parsed))
	}
	return nil
}
