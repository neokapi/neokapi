package html

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// What the HTML writer writes beyond its skeleton: the target of a link and the
// source of an image (set_attribute), and new bold, italic and link codes
// (mark, and a new code in a runs payload). A link's title and an image's alt
// text are blocks of their own (the reader surfaces them as content), so they
// change through those blocks and are not attributes here. The operations
// matrix proves each declaration (core/formats/opsmatrix_capabilities_test.go).
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

// WriteAttr sets one attribute in the start tag a link or image code holds,
// keeping every other byte of the tag. The value is written as an HTML
// character reference wherever HTML or the writer's own encoding pass could
// read it as markup, so it reads back as the value given. The code's
// attributes are read again from the new tag, as the reader reads them.
func (w *Writer) WriteAttr(seq []model.Run, at int, name, value string) ([]model.Run, error) {
	if at < 0 || at >= len(seq) {
		return nil, fmt.Errorf("no code at run %d", at)
	}
	r := seq[at]
	var typ, data string
	switch {
	case r.PcOpen != nil:
		typ, data = r.PcOpen.Type, r.PcOpen.Data
	case r.Ph != nil:
		typ, data = r.Ph.Type, r.Ph.Data
	default:
		return nil, errors.New("the run is not an inline code")
	}
	if strings.ContainsRune(value, 0) {
		return nil, errors.New("an HTML attribute value cannot hold a NUL character")
	}
	tag, ok := format.ParseStartTag(data)
	if !ok || tag.End != len(data) {
		return nil, errors.New("the code's markup is not a single start tag")
	}
	encoded := html.EscapeString(value)
	var next string
	switch a, found := tag.Attr(name, true); {
	case found && strings.Contains(data[a.ValueStart:a.ValueEnd], blockRefSentinelStart):
		return nil, fmt.Errorf("the %s attribute is content of a block of its own; edit that block", name)
	case found && a.HasValue && a.Quote != 0:
		next = data[:a.ValueStart] + encoded + data[a.ValueEnd:]
	case found && a.HasValue:
		next = data[:a.ValueStart] + `"` + encoded + `"` + data[a.ValueEnd:]
	case found:
		next = data[:a.ValueEnd] + `="` + encoded + `"` + data[a.ValueEnd:]
	default:
		next = data[:tag.InsertAt] + " " + name + `="` + encoded + `"` + data[tag.InsertAt:]
	}
	out := slices.Clone(seq)
	switch {
	case r.PcOpen != nil:
		c := *r.PcOpen
		c.Data, c.Attrs = next, tagAttrsFor(typ, next)
		out[at] = model.Run{PcOpen: &c}
	default:
		c := *r.Ph
		c.Data, c.Attrs = next, tagAttrsFor(typ, next)
		out[at] = model.Run{Ph: &c}
	}
	return out, nil
}

// synthesizedTypes are the vocabulary types the writer writes as a new code.
// Bold and italic take the element the format's projection of the vocabulary
// names (htmlInlineTag), which the cross-format export writes too; a link is
// an <a> with its href.
var synthesizedTypes = []string{"fmt:bold", "fmt:italic", "link:hyperlink"}

// Synthesizes lists the vocabulary types the writer writes as a new code.
func (w *Writer) Synthesizes() []string {
	return slices.Clone(synthesizedTypes)
}

// PropInteractiveAncestor is the block property naming the element, a or
// button, that the block sits inside in the document's markup, outside the
// block's own runs. HTML allows no link inside either, and the parser closes an
// <a> at a nested one, so the writer writes no new link in such a block.
const PropInteractiveAncestor = "html.interactive-ancestor"

// markInteractiveAncestor records the interactive element a block sits
// inside, if any.
func markInteractiveAncestor(b *model.Block, tag string) {
	if tag == "" {
		return
	}
	if b.Properties == nil {
		b.Properties = map[string]string{}
	}
	b.Properties[PropInteractiveAncestor] = tag
}

// interactiveTag reports whether a link may not go inside an element.
func interactiveTag(a atom.Atom) bool { return a == atom.A || a == atom.Button }

// interactiveAncestor returns the nearest open element a link may not go
// inside, or "" when none is open.
func (s *tokenReaderState) interactiveAncestor() string {
	for _, f := range slices.Backward(s.pathStack) {
		if interactiveTag(f.a) {
			return f.a.String()
		}
	}
	return ""
}

// domInteractiveAncestor returns the nearest element from n up that a link
// may not go inside, or "".
func domInteractiveAncestor(n *html.Node) string {
	for ; n != nil; n = n.Parent {
		if n.Type == html.ElementNode && interactiveTag(n.DataAtom) {
			return n.DataAtom.String()
		}
	}
	return ""
}

// interactiveCode reports whether a run opens or is a link or a button.
func interactiveCode(r model.Run) bool {
	switch {
	case r.PcOpen != nil:
		return r.PcOpen.Type == "link:hyperlink" || r.PcOpen.SubType == "html:a" || r.PcOpen.SubType == "html:button"
	case r.Ph != nil:
		return r.Ph.Type == "link:hyperlink"
	}
	return false
}

// SynthesizeCode writes a new <strong>, <em> or <a href> pair, as the reader
// would read it back. A block that holds text and no markup (an attribute
// value, the document title, a textarea) takes no new code, and a link goes
// neither inside nor around a link or a button, whether that element is a
// code of the block or markup the block sits inside.
func (w *Writer) SynthesizeCode(site format.CodeSite) (open, closing model.Run, err error) {
	if !slices.Contains(synthesizedTypes, site.Type) {
		return open, closing, fmt.Errorf("the writer has no element for %s", site.Type)
	}
	if b := site.Block; b != nil && (b.IsReferent || attrBlockTypes[b.Type] || b.Type == "textarea") {
		return open, closing, errors.New("the block's text is an attribute value or a text-only element, which holds no markup")
	}
	var data, end string
	var attrs map[string]string
	switch site.Type {
	case "link:hyperlink":
		if slices.ContainsFunc(site.Enclosing, interactiveCode) {
			return open, closing, errors.New("a link cannot sit inside another link or a button")
		}
		if b := site.Block; b != nil && b.Properties[PropInteractiveAncestor] != "" {
			return open, closing, fmt.Errorf("a link cannot sit inside another link or a button, and this block sits inside an <%s> element", b.Properties[PropInteractiveAncestor])
		}
		if slices.ContainsFunc(site.Inner, interactiveCode) {
			return open, closing, errors.New("a link cannot hold another link or a button")
		}
		href, ok := site.Attrs[model.AttrHref]
		if !ok {
			return open, closing, errors.New("a new link needs an href")
		}
		if strings.ContainsRune(href, 0) {
			return open, closing, errors.New("an HTML attribute value cannot hold a NUL character")
		}
		data, end = `<a href="`+html.EscapeString(href)+`">`, "</a>"
		attrs = tagAttrsFor(site.Type, data)
	default:
		if len(site.Attrs) > 0 {
			return open, closing, fmt.Errorf("a new %s code takes no attributes", site.Type)
		}
		tags := htmlInlineTag[site.Type]
		data, end = tags[0], tags[1]
	}
	subType := "html:" + scanTagName(data, 1)
	info := model.DefaultVocabulary().LookupOrFallback(site.Type)
	open = model.Run{PcOpen: &model.PcOpenRun{
		Type:    site.Type,
		SubType: subType,
		Data:    data,
		Equiv:   info.Equiv,
		Disp:    info.Display.Open,
		Attrs:   attrs,
		Constraints: &model.RunConstraints{
			Deletable:   info.Constraints.Deletable,
			Cloneable:   info.Constraints.Cloneable,
			Reorderable: info.Constraints.Reorderable,
		},
	}}
	closing = model.Run{PcClose: &model.PcCloseRun{Type: site.Type, SubType: subType, Data: end}}
	return open, closing, nil
}
