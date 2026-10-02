package html

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/net/html"

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

// synthesizedTags are the elements the writer writes for a new code, by
// vocabulary type: the semantic forms the cross-format export writes too
// (htmlInlineTag).
var synthesizedTags = map[string]string{
	"fmt:bold":       "strong",
	"fmt:italic":     "em",
	"link:hyperlink": "a",
}

// Synthesizes lists the vocabulary types the writer writes as a new code.
func (w *Writer) Synthesizes() []string {
	return []string{"fmt:bold", "fmt:italic", "link:hyperlink"}
}

// SynthesizeCode writes a new <strong>, <em> or <a href> pair, as the reader
// would read it back. A block that holds text and no markup (an attribute
// value, the document title, a textarea) takes no new code, and a link does
// not go inside another link.
func (w *Writer) SynthesizeCode(site format.CodeSite) (open, closing model.Run, err error) {
	element, ok := synthesizedTags[site.Type]
	if !ok {
		return open, closing, fmt.Errorf("the writer has no element for %s", site.Type)
	}
	if b := site.Block; b != nil && (b.IsReferent || attrBlockTypes[b.Type] || b.Type == "textarea") {
		return open, closing, errors.New("the block's text is an attribute value or a text-only element, which holds no markup")
	}
	data := "<" + element + ">"
	var attrs map[string]string
	switch site.Type {
	case "link:hyperlink":
		for _, enc := range site.Enclosing {
			if enc.PcOpen != nil && enc.PcOpen.Type == "link:hyperlink" {
				return open, closing, errors.New("a link cannot sit inside another link")
			}
		}
		for _, r := range site.Inner {
			if (r.PcOpen != nil && r.PcOpen.Type == "link:hyperlink") || (r.Ph != nil && r.Ph.Type == "link:hyperlink") {
				return open, closing, errors.New("a link cannot hold another link")
			}
		}
		href, ok := site.Attrs[model.AttrHref]
		if !ok {
			return open, closing, errors.New("a new link needs an href")
		}
		if strings.ContainsRune(href, 0) {
			return open, closing, errors.New("an HTML attribute value cannot hold a NUL character")
		}
		data = `<a href="` + html.EscapeString(href) + `">`
		attrs = tagAttrsFor(site.Type, data)
	default:
		if len(site.Attrs) > 0 {
			return open, closing, fmt.Errorf("a new %s code takes no attributes", site.Type)
		}
	}
	info := model.DefaultVocabulary().LookupOrFallback(site.Type)
	open = model.Run{PcOpen: &model.PcOpenRun{
		Type:    site.Type,
		SubType: "html:" + element,
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
	closing = model.Run{PcClose: &model.PcCloseRun{Type: site.Type, SubType: "html:" + element, Data: "</" + element + ">"}}
	return open, closing, nil
}
