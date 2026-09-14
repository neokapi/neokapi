package comments

import (
	"bytes"
	"regexp"
	"slices"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
)

// unitKind is how one comment is written.
type unitKind int

const (
	// kindLine runs to the end of its line, such as `// text`.
	kindLine unitKind = iota
	// kindBlock is delimited, such as `/* text */`.
	kindBlock
	// kindDocBlock is a delimited documentation comment, such as `/** text */`.
	kindDocBlock
	// kindShebang is the interpreter line a script opens with.
	kindShebang
	// kindDocLine documents what follows it and runs to the end of its line,
	// such as Rust's `/// text`.
	kindDocLine
	// kindInnerDocLine documents what it sits in and runs to the end of its
	// line, such as Rust's `//! text`.
	kindInnerDocLine
	// kindInnerDocBlock is a delimited comment that documents what it sits in,
	// such as Rust's `/*! text */`.
	kindInnerDocBlock
)

// line reports whether a comment of this kind runs to the end of its line.
func (k unitKind) line() bool { return k == kindLine || k == kindDocLine || k == kindInnerDocLine }

// doc reports whether a comment of this kind documents the declaration after it.
func (k unitKind) doc() bool { return k == kindDocBlock || k == kindDocLine }

// innerDoc reports whether a comment of this kind documents what encloses it.
func (k unitKind) innerDoc() bool { return k == kindInnerDocLine || k == kindInnerDocBlock }

// unit is one comment the grammar reports.
type unit struct {
	node       *ts.Node
	start, end int
	kind       unitKind
	// open and close are the lengths of the markers at either end of the span.
	open, close int
	// fullLine reports that nothing but whitespace precedes the comment on its
	// line.
	fullLine bool
}

// syntax holds what a family of languages shares: how a comment node is
// written, which comments a tool reads, and which nodes a comment documents.
type syntax struct {
	// unit classifies a node the grammar reports, and reports false for a node
	// that is not a comment.
	unit func(n *ts.Node, src []byte) (unit, bool)
	// directives are the forms a tool reads, tested in order.
	directives []directiveForm
	// decl names the declaration n is, below a container of kind parent. The
	// name is one segment of a comment's subject, such as "func/parse" at the
	// top of a file or "parse" inside a class.
	decl func(n *ts.Node, parent string, src []byte) (string, bool)
	// container reports that the declarations directly inside a node of kind
	// kind, whose parent is of kind parent, extend the subject path.
	container func(kind, parent string) bool
	// wrappers are the node kinds that hold a declaration without being one,
	// such as an export statement. decl unwraps them for the comment above.
	wrappers map[string]bool
	// attached are the node kinds that may sit between a declaration and the
	// comment above it as part of the declaration, such as Rust's attributes.
	attached map[string]bool
	// docTags reports that a documentation comment's tags are structured, so a
	// tag and its type and name are placeholders in the runs.
	docTags bool
	// markup reports that a documentation comment is written in HTML or XML, so
	// its tags and code elements are placeholders in the runs.
	markup bool
	// generated is a marker that sets a file aside as generated when the
	// comments on its first lines hold it, beside the phrases every language
	// shares.
	generated *regexp.Regexp
	// docCommands reports that a documentation comment's tags may open with a
	// backslash as well as `@`, as Doxygen's `\param` does.
	docCommands bool
	// preprocessor, when set, returns where a file's preprocessor directives
	// sit and the comments lexed from them. A comment the tree places inside a
	// directive is read from the directive's bytes instead, since the grammar
	// can fold a comment into a directive's text.
	preprocessor func(src []byte) (directives [][2]int, units []unit)
	// tolerant reports that a file whose tree holds syntax errors is still
	// located.
	tolerant bool
}

// scanner builds one file's comments.
type scanner struct {
	lang  *Language
	src   []byte
	root  *ts.Node
	idx   *format.LineIndex
	units []unit
	out   *comment.File
}

func newScanner(lang *Language, src []byte, root *ts.Node) *scanner {
	s := &scanner{lang: lang, src: src, root: root, idx: format.NewLineIndex(src), out: &comment.File{Language: lang.Name}}
	var directives [][2]int
	var lexed []unit
	if lang.syntax.preprocessor != nil {
		directives, lexed = lang.syntax.preprocessor(src)
	}
	c := root.Walk()
	defer c.Close()
walk:
	for {
		n := c.Node()
		if u, ok := lang.syntax.unit(n, src); ok {
			if !within(directives, u.start) {
				u.node = n
				u.fullLine = s.startsLine(u.start)
				s.units = append(s.units, u)
			}
		} else if c.GotoFirstChild() {
			continue
		}
		for !c.GotoNextSibling() {
			if !c.GotoParent() {
				break walk
			}
		}
	}
	if len(lexed) > 0 {
		for _, u := range lexed {
			u.node = root.NamedDescendantForByteRange(uint(u.start), uint(u.start))
			u.fullLine = s.startsLine(u.start)
			s.units = append(s.units, u)
		}
		slices.SortFunc(s.units, func(a, b unit) int { return a.start - b.start })
	}
	return s
}

// startsLine reports whether only spaces and tabs precede offset on its line.
func (s *scanner) startsLine(offset int) bool {
	for i := offset - 1; i >= 0; i-- {
		switch s.src[i] {
		case '\n':
			return true
		case ' ', '\t', '\r', '\f', '\v':
		default:
			return false
		}
	}
	return true
}

// file groups the units into comments and exclusions.
func (s *scanner) file() *comment.File {
	if generatedHeader(s.src, s.units, s.lang.syntax.generated) {
		for _, u := range s.units {
			s.exclude(u, comment.ReasonGenerated, "")
		}
		return s.out
	}
	for i := 0; i < len(s.units); {
		u := s.units[i]
		if !u.kind.line() {
			s.single(u)
			i++
			continue
		}
		j := i + 1
		for j < len(s.units) && s.sameGroup(s.units[j-1], s.units[j]) {
			j++
		}
		s.lineGroup(s.units[i:j])
		i = j
	}
	return s.out
}

// sameGroup reports whether b continues the line comment group a ends: both
// sit alone on their lines, with nothing but a line break between them.
func (s *scanner) sameGroup(a, b unit) bool {
	if !a.kind.line() || a.kind != b.kind || !a.fullLine || !b.fullLine {
		return false
	}
	gap := s.src[a.end:b.start]
	return bytes.Count(gap, []byte("\n")) == 1 && len(bytes.TrimSpace(gap)) == 0
}

// single records a comment that is its own group: a block, a documentation
// block or a shebang.
func (s *scanner) single(u unit) {
	text := s.text(u)
	if form, ok := s.directive(u, text); ok {
		s.exclude(u, comment.ReasonDirective, form)
		return
	}
	if s.blank(u) {
		s.exclude(u, comment.ReasonBlank, "")
		return
	}
	subject, doc := s.subject(u)
	s.comment([]unit{u}, []commentText{text}, subject, doc && u.kind.doc() || u.kind.innerDoc())
}

// lineGroup splits a group of line comments at its directives. Each maximal
// run of lines between directives is a comment of its own, and a blank line at
// either edge of a run is set aside, so a span starts and ends on a line with
// something written on it.
func (s *scanner) lineGroup(group []unit) {
	final := group[len(group)-1]
	subject, doc := s.subject(final)
	doc = doc && final.kind.doc() || final.kind.innerDoc()
	var run []unit
	var texts []commentText
	flush := func() {
		first, last := 0, len(run)
		for first < last && s.blank(run[first]) {
			s.exclude(run[first], comment.ReasonBlank, "")
			first++
		}
		for last > first && s.blank(run[last-1]) {
			last--
		}
		if first < last {
			s.comment(run[first:last], texts[first:last], subject, doc)
		}
		for _, u := range run[last:] {
			s.exclude(u, comment.ReasonBlank, "")
		}
		run, texts = nil, nil
	}
	for _, u := range group {
		text := s.text(u)
		if form, ok := s.directive(u, text); ok {
			flush()
			s.exclude(u, comment.ReasonDirective, form)
			continue
		}
		run = append(run, u)
		texts = append(texts, text)
	}
	flush()
}

func (s *scanner) directive(u unit, text commentText) (string, bool) {
	for _, f := range s.lang.syntax.directives {
		if form, ok := f.match(text); ok {
			return form, true
		}
	}
	return "", false
}

func (s *scanner) exclude(u unit, reason comment.Reason, form string) {
	s.out.Excluded = append(s.out.Excluded, comment.Excluded{
		Start: u.start, End: u.end, Lines: s.idx.Range(u.start, u.end), Reason: reason, Form: form,
	})
}

func (s *scanner) comment(units []unit, texts []commentText, subject string, doc bool) {
	start, end := units[0].start, units[len(units)-1].end
	style := comment.StyleLine
	if !units[0].kind.line() {
		style = comment.StyleBlock
	}
	var lines []string
	for _, t := range texts {
		lines = append(lines, t.lines...)
	}
	docBlock := units[0].kind.doc() || units[0].kind.innerDoc()
	runs, deprecated := buildRuns(trimBlankLines(lines), docBlock && s.lang.syntax.docTags, docBlock && s.lang.syntax.markup, docBlock && s.lang.syntax.docCommands)
	s.out.Comments = append(s.out.Comments, comment.Comment{
		Start:      start,
		End:        end,
		Lines:      s.idx.Range(start, end),
		Style:      style,
		Subject:    subject,
		Doc:        doc,
		Deprecated: deprecated,
		Runs:       runs,
	})
}

// subject returns the structural path of what the comment in u sits on, and
// whether the comment is that declaration's documentation.
//
// A comment sits on a declaration when the next thing after it that is not a
// comment declares something, and no blank line separates the two. It documents
// the declaration when nothing but whitespace separates them. A comment that
// sits on nothing carries the path of the declaration around it with "comment"
// appended, or "comment" at the top of a file. A comment after code on its
// line annotates that code, so it sits on nothing after it.
func (s *scanner) subject(u unit) (string, bool) {
	syn := s.lang.syntax
	parent := u.node.Parent()
	path, structural := s.pathTo(u.node)
	if u.kind.innerDoc() && structural {
		if path == "" {
			return "module", true
		}
		return path, true
	}
	if u.fullLine && structural && parent != nil && syn.container(parent.Kind(), kindOf(parent.Parent())) {
		onlyWhitespace, blankLine := true, false
		prev := u.end
		for n := u.node.NextSibling(); n != nil; n = n.NextSibling() {
			candidate, container := n, parent.Kind()
			// A grammar can open a container before its first declaration, as
			// Python puts a comment above a class's first member ahead of the
			// class body, so the comment sits on that first declaration.
			if _, isDecl := syn.decl(n, container, s.src); !isDecl && syn.container(n.Kind(), container) {
				if first := n.NamedChild(0); first != nil {
					candidate, container = first, n.Kind()
				}
			}
			start := int(candidate.StartByte())
			if bytes.Count(s.src[prev:start], []byte("\n")) > 1 {
				blankLine = true
			}
			if syn.attached[candidate.Kind()] {
				prev = int(candidate.EndByte())
				continue
			}
			if _, isComment := syn.unit(candidate, s.src); isComment {
				onlyWhitespace = false
				prev = int(candidate.EndByte())
				continue
			}
			segment, ok := syn.decl(candidate, container, s.src)
			if ok && (onlyWhitespace && u.kind.doc() || !blankLine) {
				return joinPath(path, segment), onlyWhitespace
			}
			break
		}
	}
	return joinPath(path, "comment"), false
}

// pathTo returns the subject path of the declarations enclosing n, descending
// from the root only through containers, and whether every ancestor of n was
// reached that way. A wrapper such as an export statement adds no segment of
// its own, since the declaration inside it names the path.
func (s *scanner) pathTo(n *ts.Node) (string, bool) {
	syn := s.lang.syntax
	var chain []*ts.Node
	for p := n.Parent(); p != nil; p = p.Parent() {
		chain = append(chain, p)
	}
	path := ""
	for i := len(chain) - 2; i >= 0; i-- {
		a, parent := chain[i], chain[i+1]
		if !syn.container(parent.Kind(), kindOf(parentOf(chain, i+1))) {
			return path, false
		}
		if syn.wrappers[a.Kind()] {
			continue
		}
		if segment, ok := syn.decl(a, parent.Kind(), s.src); ok {
			path = joinPath(path, segment)
		}
	}
	return path, true
}

func parentOf(chain []*ts.Node, i int) *ts.Node {
	if i+1 < len(chain) {
		return chain[i+1]
	}
	return nil
}

func kindOf(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Kind()
}

func joinPath(path, segment string) string {
	if path == "" {
		return segment
	}
	return path + "/" + segment
}

// commentText is a comment's content once its markers are removed.
type commentText struct {
	kind unitKind
	// raw is the comment as written, markers included.
	raw string
	// line is the line the comment starts on.
	line int
	// before is the code ahead of the comment on its line, trimmed.
	before string
	// lines is the content, one string per line.
	lines []string
}

// blank reports whether a comment holds nothing but whitespace after the
// markers its language declares, as a reader of the file through those markers
// sees it. A comment whose text is more marker, such as `////` in TypeScript,
// is not blank.
func (s *scanner) blank(u unit) bool {
	raw := s.src[u.start:u.end]
	if n, body, whole := s.lang.Markers.LineText(raw); whole && n == len(raw) {
		return strings.TrimSpace(body) == ""
	}
	for _, b := range s.lang.Markers.Block {
		if len(raw) >= len(b.Open)+len(b.Close) && bytes.HasPrefix(raw, []byte(b.Open)) && bytes.HasSuffix(raw, []byte(b.Close)) {
			return len(bytes.TrimSpace(raw[len(b.Open):len(raw)-len(b.Close)])) == 0
		}
	}
	return false
}

// first returns the comment's first line with something on it, trimmed.
func (t commentText) first() string {
	for _, l := range t.lines {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// text removes a unit's markers. A line comment loses its slashes and one space
// after them. A block comment loses its delimiters and, when every line after
// the first opens with an asterisk, that asterisk and one space after it; the
// lines are then dedented to their shared indentation.
func (s *scanner) text(u unit) commentText {
	raw := string(s.src[u.start:u.end])
	lineStart := bytes.LastIndexByte(s.src[:u.start], '\n') + 1
	t := commentText{kind: u.kind, raw: raw, line: s.idx.Range(u.start, u.end).First, before: strings.TrimSpace(string(s.src[lineStart:u.start]))}
	body := raw[u.open : len(raw)-u.close]
	switch u.kind {
	case kindLine, kindDocLine, kindInnerDocLine, kindShebang:
		// A marker written longer, such as `///` or `##`, is still a marker.
		body = strings.TrimLeft(body, raw[u.open-1:u.open])
		body = strings.TrimPrefix(body, " ")
		t.lines = []string{strings.TrimRight(body, " \t\r")}
	default:
		t.lines = blockLines(trimStars(body))
	}
	return t
}

// trimStars removes the asterisks a delimited comment's opener or closer is
// drawn longer with, such as `/*** text ***/`, when text remains on the line
// beside them.
func trimStars(body string) string {
	if rest := strings.TrimLeft(body, "*"); len(rest) < len(body) {
		if first, _, _ := strings.Cut(rest, "\n"); strings.TrimSpace(first) != "" {
			body = rest
		}
	}
	if rest := strings.TrimRight(body, "*"); len(rest) < len(body) {
		if last := rest[strings.LastIndexByte(rest, '\n')+1:]; strings.TrimSpace(last) != "" {
			body = rest
		}
	}
	return body
}

// blockLines splits a block comment's body into its content lines.
func blockLines(body string) []string {
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	starred := len(lines) > 1
	for _, l := range lines[1:] {
		trimmed := strings.TrimLeft(l, " \t")
		if trimmed != "" && !strings.HasPrefix(trimmed, "*") {
			starred = false
		}
	}
	lines[0] = strings.TrimLeft(lines[0], " \t")
	rest := lines[1:]
	if starred {
		for i, l := range rest {
			l = strings.TrimLeft(l, " \t")
			l = strings.TrimPrefix(l, "*")
			rest[i] = strings.TrimPrefix(l, " ")
		}
	}
	indent := -1
	for _, l := range rest {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	for i, l := range rest {
		if len(l) >= indent && indent > 0 {
			rest[i] = l[indent:]
		}
	}
	return lines
}

// trimBlankLines drops empty lines at either end.
func trimBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
