package yaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"sort"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// CommentProvider supplies a YAML document's comments to the comment layer,
// each with the span it occupies. The reader keeps those comments in its
// skeleton and surfaces their prose as notes; this gives a check the same
// comments as blocks of their own, and adds nothing to the reader's part stream.
type CommentProvider struct{}

var _ comment.Provider = CommentProvider{}

// Language implements comment.Provider.
func (CommentProvider) Language() string { return "yaml" }

// Extensions implements comment.Provider.
func (CommentProvider) Extensions() []string { return []string{".yaml", ".yml"} }

// Locate implements comment.Provider.
func (CommentProvider) Locate(_ string, src []byte) (*comment.File, error) {
	return LocateComments(src)
}

// LineText implements comment.Provider. A YAML comment runs to the end of its
// line, and its span leaves out the blanks that trail it.
func (CommentProvider) LineText(line []byte) (int, string, bool) {
	body, ok := bytes.CutPrefix(line, []byte("#"))
	if !ok {
		return 0, "", false
	}
	return len(bytes.TrimRight(line, " \t")), string(body), true
}

// Canary implements comment.Provider: a head comment with a doubled word, above
// a key whose quoted value holds a `#` that is content.
func (CommentProvider) Canary() comment.Canary {
	return comment.Canary{
		Name:   "a YAML head comment with a doubled word",
		Source: []byte("# Greets the the reader.\ngreeting: 'Hello # world'\n"),
		Block:  "comment/greeting",
	}
}

// ErrCommentsUnlocated reports a document whose comments the byte scan and the
// YAML parser disagree about. It wraps comment.ErrUnlocated.
var ErrCommentsUnlocated = fmt.Errorf("yaml: %w", comment.ErrUnlocated)

// LocateComments returns the comments in a YAML document.
//
// The parser reports what each comment says and which node it belongs to, and
// nothing about where it is. So the bytes are scanned: a `#` at the start of a
// line or after whitespace begins a comment, unless it sits inside a quoted
// scalar or a block scalar's body, and those extents come from the parsed
// nodes. The scan must then find exactly the comment lines the parser reports,
// or the document is refused with ErrCommentsUnlocated.
//
// Consecutive full-line comments form one comment, and a comment after content
// on a line is one of its own. A comment is named for the key it sits on:
// `comment/title` for the comment above or beside `title:`.
func LocateComments(src []byte) (*comment.File, error) {
	w := &commentWalk{src: src, offsets: buildLineOffsets(src), keyLines: map[int]string{}}
	dec := yamlv3.NewDecoder(bytes.NewReader(src))
	for {
		var doc yamlv3.Node
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("yaml: parsing: %w", err)
		}
		w.node(&doc, nil)
	}
	found := w.scan()
	if err := w.agree(found); err != nil {
		return nil, err
	}
	return w.build(found), nil
}

type commentWalk struct {
	src     []byte
	offsets []int
	// excluded holds the byte ranges of quoted scalars and block scalar bodies,
	// where a `#` is content.
	excluded [][2]int
	// parsed is every comment line the parser reports.
	parsed []string
	// keyLines maps a line to the key path of the deepest node starting on it.
	keyLines map[int]string
}

type scannedComment struct {
	start, end int
	fullLine   bool
}

func (w *commentWalk) node(n *yamlv3.Node, path []string) {
	w.record(n)
	switch n.Kind {
	case yamlv3.DocumentNode:
		for _, c := range n.Content {
			w.node(c, path)
		}
	case yamlv3.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			p := append(append([]string{}, path...), key.Value)
			w.keyLines[key.Line] = strings.Join(p, ".")
			w.node(key, p)
			w.node(val, p)
		}
	case yamlv3.SequenceNode:
		for i, c := range n.Content {
			p := append(append([]string{}, path...), fmt.Sprintf("[%d]", i))
			w.keyLines[c.Line] = strings.Join(p, ".")
			w.node(c, p)
		}
	case yamlv3.ScalarNode:
		w.scalar(n)
	case yamlv3.AliasNode:
		// The target's comments and extents are its definition's, walked where
		// it is defined.
	}
}

func (w *commentWalk) record(n *yamlv3.Node) {
	for _, field := range []string{n.HeadComment, n.LineComment, n.FootComment} {
		for line := range strings.SplitSeq(field, "\n") {
			if t := strings.TrimSpace(line); t != "" {
				w.parsed = append(w.parsed, t)
			}
		}
	}
}

// scalar records the extent of a scalar in which a `#` is content.
func (w *commentWalk) scalar(n *yamlv3.Node) {
	start := lineColToOffset(w.offsets, n.Line, n.Column)
	if start < 0 || start >= len(w.src) {
		return
	}
	start = skipNodeProperties(w.src, start)
	if start >= len(w.src) {
		return
	}
	switch {
	case n.Style&yamlv3.DoubleQuotedStyle != 0 && w.src[start] == '"':
		w.excluded = append(w.excluded, [2]int{start, scanQuotedEnd(w.src, start, '"')})
	case n.Style&yamlv3.SingleQuotedStyle != 0 && w.src[start] == '\'':
		w.excluded = append(w.excluded, [2]int{start, scanQuotedEnd(w.src, start, '\'')})
	case n.Style&(yamlv3.LiteralStyle|yamlv3.FoldedStyle) != 0 && (w.src[start] == '|' || w.src[start] == '>'):
		// A comment on the indicator line is a comment; the body starts on the
		// next line.
		body := start
		for body < len(w.src) && w.src[body] != '\n' {
			body++
		}
		if body < len(w.src) {
			body++
		}
		if end := scanBlockScalarEnd(w.src, start); end > body {
			w.excluded = append(w.excluded, [2]int{body, end})
		}
	}
}

// skipNodeProperties steps past a tag (`!tag`) and an anchor (`&name`) in front
// of a scalar's own bytes.
func skipNodeProperties(src []byte, i int) int {
	for i < len(src) {
		switch src[i] {
		case '!':
			n := scanTagPrefix(src, i)
			if n == 0 {
				return i
			}
			i += n
		case '&':
			for i < len(src) && src[i] != ' ' && src[i] != '\t' && src[i] != '\n' {
				i++
			}
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
		default:
			return i
		}
	}
	return i
}

func (w *commentWalk) inExcluded(offset int) bool {
	i := sort.Search(len(w.excluded), func(i int) bool { return w.excluded[i][0] > offset })
	for j := i - 1; j >= 0; j-- {
		if offset < w.excluded[j][1] {
			return true
		}
		if w.excluded[j][1] <= w.excluded[j][0] {
			continue
		}
		// Ranges do not nest, so the nearest one starting at or before offset
		// decides.
		return false
	}
	return false
}

// scan finds every comment in the bytes, in order.
func (w *commentWalk) scan() []scannedComment {
	sort.Slice(w.excluded, func(i, j int) bool { return w.excluded[i][0] < w.excluded[j][0] })
	var out []scannedComment
	for lineStart := 0; lineStart < len(w.src); {
		next := len(w.src)
		lineEnd := len(w.src)
		if i := bytes.IndexByte(w.src[lineStart:], '\n'); i >= 0 {
			lineEnd = lineStart + i
			next = lineEnd + 1
		}
		end := lineEnd
		if end > lineStart && w.src[end-1] == '\r' {
			end--
		}
		for j := lineStart; j < end; j++ {
			if w.src[j] != '#' || (j > lineStart && w.src[j-1] != ' ' && w.src[j-1] != '\t') || w.inExcluded(j) {
				continue
			}
			textEnd := end
			for textEnd > j && (w.src[textEnd-1] == ' ' || w.src[textEnd-1] == '\t') {
				textEnd--
			}
			out = append(out, scannedComment{
				start:    j,
				end:      textEnd,
				fullLine: len(bytes.TrimLeft(w.src[lineStart:j], " \t")) == 0,
			})
			break
		}
		lineStart = next
	}
	return out
}

// agree holds the scan to the parser: the same comment lines, as many times
// each.
func (w *commentWalk) agree(found []scannedComment) error {
	got := map[string]int{}
	for _, f := range found {
		got[string(w.src[f.start:f.end])]++
	}
	want := map[string]int{}
	for _, p := range w.parsed {
		want[p]++
	}
	if maps.Equal(got, want) {
		return nil
	}
	for text, n := range got {
		if want[text] != n {
			return fmt.Errorf("%w: the scan finds %q %d time(s) and the parser %d", ErrCommentsUnlocated, text, n, want[text])
		}
	}
	for text, n := range want {
		if got[text] != n {
			return fmt.Errorf("%w: the parser reports %q %d time(s) and the scan finds it %d", ErrCommentsUnlocated, text, n, got[text])
		}
	}
	return ErrCommentsUnlocated
}

// build groups the scanned comments into comments and exclusions.
func (w *commentWalk) build(found []scannedComment) *comment.File {
	idx := format.NewLineIndex(w.src)
	file := &comment.File{Language: "yaml"}
	keyLines := make([]int, 0, len(w.keyLines))
	for line := range w.keyLines {
		keyLines = append(keyLines, line)
	}
	sort.Ints(keyLines)

	exclude := func(f scannedComment, reason comment.Reason, form string) {
		file.Excluded = append(file.Excluded, comment.Excluded{
			Start: f.start, End: f.end, Lines: idx.Range(f.start, f.end), Reason: reason, Form: form,
		})
	}
	var run []scannedComment
	flush := func() {
		first, last := 0, len(run)
		for first < last && blankComment(w.src, run[first]) {
			exclude(run[first], comment.ReasonBlank, "")
			first++
		}
		for last > first && blankComment(w.src, run[last-1]) {
			last--
		}
		if first < last {
			lines := run[first:last]
			start, end := lines[0].start, lines[len(lines)-1].end
			span := idx.Range(start, end)
			prose := make([]string, len(lines))
			for i, l := range lines {
				t := strings.TrimPrefix(string(w.src[l.start:l.end]), "#")
				prose[i] = strings.TrimPrefix(t, " ")
			}
			file.Comments = append(file.Comments, comment.Comment{
				Start:   start,
				End:     end,
				Lines:   span,
				Style:   comment.StyleLine,
				Subject: w.subject(keyLines, span, lines[0].fullLine),
				Runs:    []model.Run{model.TextR(strings.Join(prose, "\n"))},
			})
		}
		for _, l := range run[last:] {
			exclude(l, comment.ReasonBlank, "")
		}
		run = nil
	}
	for _, f := range found {
		if form, ok := yamlDirective(string(w.src[f.start:f.end])); ok {
			flush()
			exclude(f, comment.ReasonDirective, form)
			continue
		}
		if !f.fullLine {
			flush()
			run = []scannedComment{f}
			flush()
			continue
		}
		if len(run) > 0 && idx.Line(run[len(run)-1].start)+1 != idx.Line(f.start) {
			flush()
		}
		run = append(run, f)
	}
	flush()
	return file
}

// subject names what a comment sits on: for a comment beside content, the key on
// its line; for a comment on lines of its own, the next key below it.
func (w *commentWalk) subject(keyLines []int, span format.LineRange, fullLine bool) string {
	path := ""
	if fullLine {
		i := sort.SearchInts(keyLines, span.Last+1)
		if i < len(keyLines) {
			path = w.keyLines[keyLines[i]]
		}
	} else {
		i := sort.SearchInts(keyLines, span.First+1)
		if i > 0 {
			path = w.keyLines[keyLines[i-1]]
		}
	}
	if path == "" {
		path = "document"
	}
	return "comment/" + path
}

func blankComment(src []byte, f scannedComment) bool {
	return strings.TrimSpace(strings.TrimPrefix(string(src[f.start:f.end]), "#")) == ""
}

// yamlDirective reports a comment a tool reads as an instruction: the YAML
// language server's schema binding, a yamllint switch, or a Prettier ignore.
func yamlDirective(text string) (string, bool) {
	body := strings.TrimSpace(strings.TrimPrefix(text, "#"))
	switch {
	case strings.HasPrefix(body, "yaml-language-server:"):
		return "yaml-language-server", true
	case strings.HasPrefix(body, "yamllint "):
		return "yamllint", true
	case body == "prettier-ignore":
		return "prettier-ignore", true
	}
	return "", false
}
