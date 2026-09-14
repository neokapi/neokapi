package golang

import (
	"fmt"
	"go/ast"
	comment "go/doc/comment"
	"path"
	"strconv"
	"strings"

	layer "github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
)

// In this file `comment` is go/doc/comment, the doc comment syntax, and `layer`
// is the comment layer.

// Placeholder types and subtypes for the parts of a comment that are not prose.
const (
	phCode      = "code"
	phLink      = "link:hyperlink"
	subCode     = "go:code"
	subDocLink  = "go:doclink"
	subURL      = "go:url"
	subLink     = "go:link"
	subLinkDef  = "go:linkdef"
	subListItem = "go:item"
	paragraphSp = "\n\n"
)

// docCommentParser is go/doc/comment's parser, configured for one file.
type docCommentParser = comment.Parser

// docParser returns a parser that recognises references the way godoc does for
// this file. A package reference resolves against the file's imports and the
// standard library. A reference to a declaration is accepted whatever it names,
// because the declaration may sit in another file of the package, which a
// single-file scan does not load; treating such a reference as prose would
// hold an identifier to the rules for sentences.
func docParser(f *ast.File) *docCommentParser {
	imports := map[string]string{}
	for _, is := range f.Imports {
		p, err := strconv.Unquote(is.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		if is.Name != nil {
			name = is.Name.Name
		}
		imports[name] = p
	}
	return &comment.Parser{
		LookupPackage: func(name string) (string, bool) {
			if p, ok := imports[name]; ok {
				return p, true
			}
			return comment.DefaultLookupPackage(name)
		},
		LookupSym: func(recv, name string) bool { return true },
	}
}

// docRuns projects a parsed comment into runs.
//
// Every block kind and text kind the parser produces has a case here. Prose is
// text; a code block, a URL, a link's brackets and target, a link definition
// and a reference to a declaration are placeholders, so a check reads none of
// them as a sentence and nothing that rewrites prose can alter them. The
// parser consumes list markers and heading markers. Each list item opens with
// a placeholder of type layer.TypeListItem holding the marker gofmt writes, "-"
// or the item's number and a full stop, so one item's prose stays apart from
// the next, and each item and heading sits on its own line. A kind the parser
// gains in a later Go release is an error until it has a case, rather than text
// that silently disappears.
func docRuns(d *comment.Doc) ([]model.Run, error) {
	var b runBuilder
	for i, blk := range d.Content {
		if i > 0 {
			b.text(paragraphSp)
		}
		if err := b.block(blk); err != nil {
			return nil, err
		}
	}
	for _, def := range d.Links {
		if len(b.runs) > 0 {
			b.text(paragraphSp)
		}
		b.placeholder(phLink, subLinkDef, "["+def.Text+"]: "+def.URL)
	}
	return b.runs, nil
}

// deprecated reports whether a parsed comment has a paragraph opening with
// "Deprecated: ", the marker staticcheck and gopls read.
func deprecated(d *comment.Doc) bool {
	for _, blk := range d.Content {
		p, ok := blk.(*comment.Paragraph)
		if !ok || len(p.Text) == 0 {
			continue
		}
		if plain, ok := p.Text[0].(comment.Plain); ok && strings.HasPrefix(string(plain), "Deprecated: ") {
			return true
		}
	}
	return false
}

type runBuilder struct {
	runs []model.Run
	ids  int
}

func (b *runBuilder) block(blk comment.Block) error {
	switch blk := blk.(type) {
	case *comment.Paragraph:
		return b.texts(blk.Text)
	case *comment.Heading:
		return b.texts(blk.Text)
	case *comment.List:
		for i, item := range blk.Items {
			if i > 0 {
				b.text("\n")
			}
			marker := "-"
			if item.Number != "" {
				marker = item.Number + "."
			}
			b.placeholder(layer.TypeListItem, subListItem, marker)
			for j, content := range item.Content {
				if j > 0 {
					b.text(paragraphSp)
				}
				if err := b.block(content); err != nil {
					return err
				}
			}
		}
		return nil
	case *comment.Code:
		b.placeholder(phCode, subCode, blk.Text)
		return nil
	default:
		return fmt.Errorf("comment block kind %T has no projection", blk)
	}
}

func (b *runBuilder) texts(ts []comment.Text) error {
	for _, t := range ts {
		switch t := t.(type) {
		case comment.Plain:
			b.text(string(t))
		case comment.Italic:
			b.text(string(t))
		case *comment.Link:
			if t.Auto {
				b.placeholder(phLink, subURL, t.URL)
				continue
			}
			b.ids++
			id := "g" + strconv.Itoa(b.ids)
			open := model.PcOpenRun{ID: id, Type: phLink, SubType: subLink, Data: "["}
			open.SetAttr(model.AttrHref, t.URL)
			b.runs = append(b.runs, model.PcOpenR(open))
			if err := b.texts(t.Text); err != nil {
				return err
			}
			b.runs = append(b.runs, model.PcCloseR(model.PcCloseRun{ID: id, Type: phLink, SubType: subLink, Data: "]"}))
		case *comment.DocLink:
			name, err := linkText(t.Text)
			if err != nil {
				return err
			}
			b.placeholder(phCode, subDocLink, "["+name+"]")
		default:
			return fmt.Errorf("comment text kind %T has no projection", t)
		}
	}
	return nil
}

// linkText is the text a reference was written with, which the parser holds as
// plain or italic text.
func linkText(ts []comment.Text) (string, error) {
	var sb strings.Builder
	for _, t := range ts {
		switch t := t.(type) {
		case comment.Plain:
			sb.WriteString(string(t))
		case comment.Italic:
			sb.WriteString(string(t))
		default:
			return "", fmt.Errorf("comment reference text kind %T has no projection", t)
		}
	}
	return sb.String(), nil
}

// text appends prose, joining it to a text run already at the end.
func (b *runBuilder) text(s string) {
	if s == "" {
		return
	}
	if n := len(b.runs); n > 0 && b.runs[n-1].Text != nil {
		b.runs[n-1].Text.Text += s
		return
	}
	b.runs = append(b.runs, model.TextR(s))
}

func (b *runBuilder) placeholder(typ, subType, data string) {
	b.ids++
	b.runs = append(b.runs, model.PhR(model.PlaceholderRun{
		ID:      "g" + strconv.Itoa(b.ids),
		Type:    typ,
		SubType: subType,
		Data:    data,
	}))
}
