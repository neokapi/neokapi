package markdown

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// What the Markdown writer writes beyond its skeleton: the destination of an
// inline link or image (set_attribute), and new bold, italic and link codes
// (mark, and a new code in a runs payload). A link's or image's title is text
// of the block (the reader surfaces it between codes of its own), so it
// changes as text. A reference link takes its destination from a definition
// elsewhere in the document and an autolink's text is its destination, so
// neither takes set_attribute. The operations matrix proves each declaration
// (core/formats/opsmatrix_capabilities_test.go).
var (
	_ format.AttrWriter      = (*Writer)(nil)
	_ format.CodeSynthesizer = (*Writer)(nil)
)

// WritableAttrs declares the attributes of an inline code the writer writes.
func (w *Writer) WritableAttrs() map[string][]string {
	return map[string][]string{
		"link:hyperlink": {model.AttrHref},
		"media:image":    {model.AttrSrc},
	}
}

// WriteAttr sets the destination of an inline link or image. The destination
// sits in the code's closing markup, `](dest)`, or, when the link has a title,
// in the opening markup of the title, `(dest "`; everything around it keeps
// its bytes. The value is spelled so the reader reads it back as given: bare,
// or in angle brackets when the document's spelling had them or the value
// needs them.
func (w *Writer) WriteAttr(seq []model.Run, at int, name, value string) ([]model.Run, error) {
	if at < 0 || at >= len(seq) {
		return nil, fmt.Errorf("no code at run %d", at)
	}
	r := seq[at]
	if r.Ph != nil && r.Ph.SubType == "md:autolink" {
		return nil, errors.New("an autolink's text is its destination; replace the autolink to change it")
	}
	if r.PcOpen == nil {
		return nil, errors.New("the code is not an inline link or image")
	}
	destKey := model.AttrHref
	switch r.PcOpen.SubType {
	case "md:link":
	case "md:image":
		destKey = model.AttrSrc
	case "md:link-ref", "md:image-ref":
		return nil, errors.New("a reference link takes its destination from its definition elsewhere in the document, which other links may share; change the definition instead")
	default:
		return nil, errors.New("the code is not an inline link or image")
	}
	if name != destKey {
		return nil, fmt.Errorf("the writer writes the %s of this code, not its %s", destKey, name)
	}
	id := r.PcOpen.ID
	j := slices.IndexFunc(seq[at+1:], func(c model.Run) bool { return c.PcClose != nil && c.PcClose.ID == id })
	if j < 0 {
		return nil, errors.New("the code does not close in its run sequence")
	}
	j += at + 1

	// The run whose data holds the destination, and the bytes of a complete
	// closer around that data, so the destination can be located in it.
	holder, prefix, suffix := j, "", ""
	data := seq[j].PcClose.Data
	if data == "]" {
		holder = j + 1
		if holder+2 >= len(seq) || seq[holder].PcOpen == nil || seq[holder+1].Text == nil || seq[holder+2].PcClose == nil ||
			(seq[holder].PcOpen.SubType != subTypeLinkTitle && seq[holder].PcOpen.SubType != subTypeImageTitle) {
			return nil, errors.New("the link's destination is not where the reader puts it")
		}
		data = seq[holder].PcOpen.Data
		prefix, suffix = "]", seq[holder+1].Text.Text+seq[holder+2].PcClose.Data
	}
	closer := prefix + data + suffix
	c, ok := scanInlineLinkCloser([]byte(closer), 0)
	if !ok {
		return nil, errors.New("the link's closing markup does not read as `](destination)`")
	}
	destStart := skipLinkWhitespace([]byte(closer), 2) - len(prefix)
	destEnd := destStart + len(c.dest)
	if destStart < 0 || destEnd > len(data) || data[destStart:destEnd] != c.dest {
		return nil, errors.New("the link's destination is not where the reader puts it")
	}
	spelled, err := spellLinkDestination(value, strings.HasPrefix(c.dest, "<"))
	if err != nil {
		return nil, err
	}

	out := slices.Clone(seq)
	open := *r.PcOpen
	open.Attrs = maps.Clone(open.Attrs)
	if open.Attrs == nil {
		open.Attrs = map[string]string{}
	}
	if value == "" {
		delete(open.Attrs, destKey)
		if len(open.Attrs) == 0 {
			open.Attrs = nil
		}
	} else {
		open.Attrs[destKey] = value
	}
	out[at] = model.Run{PcOpen: &open}
	next := data[:destStart] + spelled + data[destEnd:]
	if holder == j {
		cl := *seq[j].PcClose
		cl.Data = next
		out[j] = model.Run{PcClose: &cl}
	} else {
		t := *seq[holder].PcOpen
		t.Data = next
		out[holder] = model.Run{PcOpen: &t}
	}
	return out, nil
}

// spellLinkDestination spells value as a link destination the reader reads
// back as value. The reader keeps a destination as the document spells it,
// without its angle brackets (goldmark's ast.Link.Destination), so a value is
// written as itself: bare where a bare destination ends where the value does,
// and in angle brackets when angle asks for them or the value holds a space,
// a control character or a parenthesis that does not balance. A value that
// reads back as given in neither form, such as one holding a line break or an
// angle bracket a bare destination cannot carry, has no spelling.
func spellLinkDestination(value string, angle bool) (string, error) {
	if strings.ContainsAny(value, "\n\r") {
		return "", errors.New("a link destination cannot hold a line break")
	}
	var forms []string
	if !angle && value != "" {
		forms = append(forms, value)
	}
	forms = append(forms, "<"+value+">")
	for _, spelled := range forms {
		if dest, ok := parsedDestination(spelled); ok && dest == value {
			return spelled, nil
		}
	}
	return "", fmt.Errorf("the destination %q has no spelling that reads back as given", value)
}

// parsedDestination is the destination the reader's parser, with the
// reader's extensions, reads from `[x](dest)`.
func parsedDestination(dest string) (string, bool) {
	src := []byte("[x](" + dest + ")")
	doc := parseMarkdown(src)
	var got string
	found := false
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if l, ok := n.(*ast.Link); ok && entering && !found {
			got, found = string(l.Destination), true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return got, found
}

// synthesizedEmphasis maps the emphasis types the writer writes as a new code
// to the subtype the reader gives them. The delimiters are the format's own
// projection of the vocabulary (mdInlineTag), which the cross-format export
// writes too.
var synthesizedEmphasis = map[string]string{
	"fmt:bold":   "md:strong",
	"fmt:italic": "md:emphasis",
}

// Synthesizes lists the vocabulary types the writer writes as a new code.
func (w *Writer) Synthesizes() []string {
	return []string{"fmt:bold", "fmt:italic", "link:hyperlink"}
}

// SynthesizeCode writes a new `**…**`, `*…*` or `[…](href)` pair as the reader
// would read it back. A block whose text is literal (a code block, front
// matter, math, raw HTML) and text inside a code span take no new code; a link
// goes neither inside nor around another link or image, nor after a '!' that
// would make it an image; no markup goes where a backslash in the text would
// escape it; and the markup is parsed in place, with the text around it, and
// written only where CommonMark reads it back as the code over exactly the
// range (readsBack).
func (w *Writer) SynthesizeCode(site format.CodeSite) (open, closing model.Run, err error) {
	if b := site.Block; b != nil {
		switch b.Type {
		case "front-matter", "code-block", "math", "html-block", "html-text", "html-attr":
			return open, closing, errors.New("the block's text is literal and holds no Markdown markup")
		}
		if b.SemanticRole() == model.RoleCode {
			return open, closing, errors.New("the block's text is literal and holds no Markdown markup")
		}
	}
	for _, enc := range site.Enclosing {
		if enc.PcOpen != nil && enc.PcOpen.Type == "fmt:code" {
			return open, closing, errors.New("text inside a code span is literal")
		}
	}
	info := model.DefaultVocabulary().LookupOrFallback(site.Type)
	constraints := &model.RunConstraints{
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		Reorderable: info.Constraints.Reorderable,
	}
	if subType, ok := synthesizedEmphasis[site.Type]; ok {
		if len(site.Attrs) > 0 {
			return open, closing, fmt.Errorf("a new %s code takes no attributes", site.Type)
		}
		if err := emphasisFlanks(site); err != nil {
			return open, closing, err
		}
		delims := mdInlineTag[site.Type]
		level := len(delims[0])
		if err := readsBack(site, delims[0], delims[1], func(n ast.Node) bool {
			em, ok := n.(*ast.Emphasis)
			return ok && em.Level == level
		}); err != nil {
			return open, closing, err
		}
		open = model.Run{PcOpen: &model.PcOpenRun{Type: site.Type, SubType: subType, Data: delims[0],
			Disp: info.Display.Open, Equiv: info.Equiv, Constraints: constraints}}
		closing = model.Run{PcClose: &model.PcCloseRun{Type: site.Type, SubType: subType, Data: delims[1], Equiv: info.Equiv}}
		return open, closing, nil
	}
	if site.Type != "link:hyperlink" {
		return open, closing, fmt.Errorf("the writer has no markup for %s", site.Type)
	}
	isLink := func(r model.Run) bool {
		switch {
		case r.PcOpen != nil:
			return r.PcOpen.Type == "link:hyperlink" || r.PcOpen.Type == "media:image"
		case r.Ph != nil:
			return r.Ph.Type == "link:hyperlink"
		}
		return false
	}
	if slices.ContainsFunc(site.Enclosing, isLink) || slices.ContainsFunc(site.Inner, isLink) {
		return open, closing, errors.New("a link goes neither inside nor around another link or image")
	}
	if !bracketsBalance(model.RenderRunsWithData(site.Inner)) {
		return open, closing, errors.New("the text holds a bracket that does not balance, which would end the link text early")
	}
	for name := range site.Attrs {
		if name != model.AttrHref {
			return open, closing, fmt.Errorf("a new link takes an href and no %s", name)
		}
	}
	href, ok := site.Attrs[model.AttrHref]
	if !ok {
		return open, closing, errors.New("a new link needs an href")
	}
	dest, err := spellLinkDestination(href, false)
	if err != nil {
		return open, closing, err
	}
	// In a table cell a pipe is spelled `\|`, which the reader reads back as
	// the pipe (cellDestination).
	readDest := func(d []byte) string { return string(d) }
	if b := site.Block; b != nil && (b.SemanticRole() == model.RoleTableCell || b.SemanticRole() == model.RoleTableHeader) {
		dest = escapeCellPipes(dest)
		readDest = func(d []byte) string { return strings.ReplaceAll(string(d), `\|`, "|") }
	}
	before := model.RenderRunsWithData(site.Before)
	if strings.HasSuffix(before, "!") && !backslashEscaped(before, len(before)-1) {
		return open, closing, errors.New("a '!' right before a link makes it an image; mark the text from a later character or include the '!'")
	}
	if err := backslashBeside(site); err != nil {
		return open, closing, err
	}
	if err := readsBack(site, "[", "]("+dest+")", func(n ast.Node) bool {
		l, ok := n.(*ast.Link)
		return ok && readDest(l.Destination) == href
	}); err != nil {
		return open, closing, err
	}
	var attrs map[string]string
	if href != "" {
		attrs = map[string]string{model.AttrHref: href}
	}
	open = model.Run{PcOpen: &model.PcOpenRun{Type: site.Type, SubType: "md:link", Data: "[",
		Disp: info.Display.Open, Equiv: info.Equiv, Attrs: attrs, Constraints: constraints}}
	closing = model.Run{PcClose: &model.PcCloseRun{Type: site.Type, SubType: "md:link", Data: "](" + dest + ")", Equiv: info.Equiv}}
	return open, closing, nil
}

// emphasisFlanks reports, as an error with the reason, a place where new
// emphasis delimiters would not read back as emphasis: CommonMark opens it
// only with a left-flanking delimiter run and closes it only with a
// right-flanking one, and a delimiter beside another '*' joins that run.
func emphasisFlanks(site format.CodeSite) error {
	inner := model.RenderRunsWithData(site.Inner)
	before := model.RenderRunsWithData(site.Before)
	after := model.RenderRunsWithData(site.After)
	first, _ := utf8.DecodeRuneInString(inner)
	last, _ := utf8.DecodeLastRuneInString(inner)
	prev, _ := utf8.DecodeLastRuneInString(before)
	next, _ := utf8.DecodeRuneInString(after)
	if before == "" {
		prev = ' '
	}
	if after == "" {
		next = ' '
	}
	if err := backslashBeside(site); err != nil {
		return err
	}
	switch {
	case inner == "":
		return errors.New("emphasis needs text to wrap")
	case unicode.IsSpace(first) || unicode.IsSpace(last):
		return errors.New("emphasis in Markdown cannot begin or end with a space; mark the words alone")
	case prev == '*' || next == '*' || first == '*' || last == '*':
		return errors.New("the delimiters would join a '*' beside them; mark the text without it")
	case mdPunct(first) && !unicode.IsSpace(prev) && !mdPunct(prev):
		return errors.New("emphasis in Markdown does not open between a word character and punctuation; widen the mark to the whole word")
	case mdPunct(last) && !unicode.IsSpace(next) && !mdPunct(next):
		return errors.New("emphasis in Markdown does not close between punctuation and a word character; widen the mark to the whole word")
	}
	return nil
}

// backslashBeside reports, as an error with the reason, a backslash that
// would escape the new code's markup. The reader keeps the document's own
// spelling in text runs, so a backslash the text ends in is written just
// before the opening or the closing markup, and CommonMark reads the
// character after an odd run of backslashes as text.
func backslashBeside(site format.CodeSite) error {
	if before := model.RenderRunsWithData(site.Before); backslashEscaped(before, len(before)) {
		return errors.New("the text before the range ends in a backslash, which would escape the new code's opening markup; mark the text from a later character")
	}
	if inner := model.RenderRunsWithData(site.Inner); backslashEscaped(inner, len(inner)) {
		return errors.New("the range ends in a backslash, which would escape the new code's closing markup; mark the text without it")
	}
	return nil
}

// readsBack parses the block's text with the new code's markup in place, as
// the reader parses it, and reports, as an error with the reason, a place
// where the markup would not read back as the new code: where its delimiters
// would read as text or pair with others, where the code would hold more or
// less than the range, or where the markup would change how anything else in
// the block reads. isCode picks the node the new code reads as. The checks
// before it name the common causes; this one is the proof that the markup
// reads back as the code.
func readsBack(site format.CodeSite, open, closing string, isCode func(ast.Node) bool) error {
	// The text is parsed after a word and a space, which open no block
	// construct and read like the start of a line to the inline parser, so
	// text that would start a list or a heading on a line of its own still
	// parses as the paragraph it is in the document.
	const lead = "x "
	before := lead + model.RenderRunsWithData(site.Before)
	inner := model.RenderRunsWithData(site.Inner)
	after := model.RenderRunsWithData(site.After)
	src := []byte(before + open + inner + closing + after)
	was := mdNodeCounts(parseMarkdown([]byte(before + inner + after)))
	doc := parseMarkdown(src)
	now := mdNodeCounts(doc)

	innerStart := len(before) + len(open)
	innerEnd := innerStart + len(inner)
	markupStart, markupEnd := len(before), innerEnd+len(closing)
	var code ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || code != nil || !isCode(n) {
			return ast.WalkContinue, nil
		}
		inside := true
		_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
			if t, ok := c.(*ast.Text); ok && entering && (t.Segment.Start < innerStart || t.Segment.Stop > innerEnd) {
				inside = false
				return ast.WalkStop, nil
			}
			return ast.WalkContinue, nil
		})
		if inside {
			code = n
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if code == nil {
		return errors.New("CommonMark would not read the new markup as the code here; it would pair with other markup or read as text")
	}
	// Every piece of text from the opening markup to the end of the closing
	// markup is the range's own text, inside the new code: a delimiter left
	// over as text, or range text outside the code, reads back differently.
	var stray error
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		t, ok := n.(*ast.Text)
		if !entering || !ok || t.Segment.Stop <= markupStart || t.Segment.Start >= markupEnd {
			return ast.WalkContinue, nil
		}
		if t.Segment.Start < innerStart || t.Segment.Stop > innerEnd || !hasAncestor(t, code) {
			stray = errors.New("CommonMark would read part of the new markup as text, or leave text of the range outside the code")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if stray != nil {
		return stray
	}
	was[mdNodeKey(code)]++
	if !maps.Equal(was, now) {
		return errors.New("the new markup would change how other markup in the block reads")
	}
	return nil
}

// parseMarkdown parses src with the reader's parser and extensions.
func parseMarkdown(src []byte) ast.Node {
	return goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(src))
}

// mdNodeCounts counts the nodes of a parsed document by kind, emphasis by its
// level, leaving out text, which delimiters split differently wherever they
// sit.
func mdNodeCounts(doc ast.Node) map[string]int {
	out := map[string]int{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() != ast.KindText && n.Kind() != ast.KindString {
			out[mdNodeKey(n)]++
		}
		return ast.WalkContinue, nil
	})
	return out
}

func mdNodeKey(n ast.Node) string {
	if em, ok := n.(*ast.Emphasis); ok {
		return n.Kind().String() + strconv.Itoa(em.Level)
	}
	return n.Kind().String()
}

func hasAncestor(n, ancestor ast.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p == ancestor {
			return true
		}
	}
	return false
}

// mdPunct is CommonMark's Unicode punctuation character: punctuation or a
// symbol.
func mdPunct(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) }

// bracketsBalance reports whether the unescaped square brackets of s balance.
func bracketsBalance(s string) bool {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return false
			}
			depth--
		}
	}
	return depth == 0
}
