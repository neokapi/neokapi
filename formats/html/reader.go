package html

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// inlineElements are HTML elements treated as inline (Spans, not block boundaries).
var inlineElements = map[atom.Atom]bool{
	atom.A: true, atom.Abbr: true, atom.Acronym: true, atom.B: true,
	atom.Bdo: true, atom.Big: true, atom.Br: true, atom.Cite: true,
	atom.Code: true, atom.Del: true, atom.Dfn: true, atom.Em: true,
	atom.Font: true, atom.I: true, atom.Img: true, atom.Ins: true,
	atom.Kbd: true, atom.Label: true, atom.Q: true, atom.S: true,
	atom.Samp: true, atom.Small: true, atom.Span: true, atom.Strike: true,
	atom.Strong: true, atom.Sub: true, atom.Sup: true, atom.Tt: true,
	atom.U: true, atom.Var: true, atom.Wbr: true,
}

// selfClosingElements are void HTML elements.
var selfClosingElements = map[atom.Atom]bool{
	atom.Br: true, atom.Hr: true, atom.Img: true, atom.Input: true,
	atom.Meta: true, atom.Link: true, atom.Wbr: true,
}

// nonTranslatableElements contain content that is not translatable.
var nonTranslatableElements = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true,
}

// Reader implements DataFormatReader for HTML files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new HTML reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "html",
			FormatDisplayName: "HTML",
			FormatMimeType:    "text/html",
			FormatExtensions:  []string{".html", ".htm", ".xhtml"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/html", "application/xhtml+xml"},
		Extensions: []string{".html", ".htm", ".xhtml"},
		MagicBytes: [][]byte{[]byte("<!DOCTYPE"), []byte("<!doctype"), []byte("<html"), []byte("<HTML")},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("html: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "html",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/html",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("html: reading: %w", err)}
		return
	}

	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("html: parsing: %w", err)}
		return
	}

	blockCounter := 0
	dataCounter := 0

	r.walkNode(ctx, ch, doc, &blockCounter, &dataCounter)

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// walkNode traverses the HTML tree. Block-level elements with text content
// have their inline content collected into a single Block. Non-translatable
// elements become Data parts. The function recurses through container elements.
func (r *Reader) walkNode(ctx context.Context, ch chan<- model.PartResult, n *html.Node, blockCounter, dataCounter *int) {
	switch n.Type {
	case html.DocumentNode:
		// Recurse into document's children
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			r.walkNode(ctx, ch, child, blockCounter, dataCounter)
		}

	case html.DoctypeNode:
		*dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", *dataCounter),
			Name: "doctype",
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})

	case html.ElementNode:
		// Non-translatable elements become Data parts
		if nonTranslatableElements[n.DataAtom] {
			*dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", *dataCounter),
				Name: n.Data,
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
			return
		}

		// Block-level element with text content: collect all inline content into one Block
		if !isInlineElement(n) && r.hasTextContent(n) {
			frag := r.collectInlineContent(n)
			if frag != nil && !frag.IsEmpty() && strings.TrimSpace(frag.Text()) != "" {
				*blockCounter++
				block := &model.Block{
					ID:           fmt.Sprintf("tu%d", *blockCounter),
					Name:         n.Data,
					Translatable: true,
					Source:       []*model.Segment{{ID: "s1", Content: frag}},
					Targets:      make(map[model.LocaleID][]*model.Segment),
					Properties:   make(map[string]string),
					Annotations:  make(map[string]model.Annotation),
					Skeleton: &model.Skeleton{
						Strategy: model.SkeletonFragmentBased,
						Parts: []model.SkeletonPart{
							&model.SkeletonText{Text: r.renderOpenTag(n)},
							&model.SkeletonRef{ResourceID: fmt.Sprintf("tu%d", *blockCounter), Property: "target"},
							&model.SkeletonText{Text: fmt.Sprintf("</%s>", n.Data)},
						},
					},
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
				return
			}
		}

		// Container element without direct text: recurse into children
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			r.walkNode(ctx, ch, child, blockCounter, dataCounter)
		}

	case html.TextNode:
		// Standalone text nodes not inside a block element — shouldn't normally happen
		// with well-formed HTML, but handle defensively
		text := strings.TrimSpace(n.Data)
		if text != "" {
			*blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), text)
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
	}
}

// collectInlineContent collects all text and inline elements from a block element
// into a single Fragment with Spans.
func (r *Reader) collectInlineContent(n *html.Node) *model.Fragment {
	frag := &model.Fragment{}
	r.collectFromNode(n, frag)
	return frag
}

func (r *Reader) collectFromNode(n *html.Node, frag *model.Fragment) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			frag.AppendText(child.Data)
		case html.ElementNode:
			if isInlineElement(child) {
				if selfClosingElements[child.DataAtom] {
					frag.AppendSpan(&model.Span{
						SpanType: model.SpanPlaceholder,
						Type:     child.Data,
						ID:       child.Data,
						Data:     r.renderTag(child),
					})
				} else {
					frag.AppendSpan(&model.Span{
						SpanType: model.SpanOpening,
						Type:     child.Data,
						ID:       child.Data,
						Data:     r.renderOpenTag(child),
					})
					r.collectFromNode(child, frag)
					frag.AppendSpan(&model.Span{
						SpanType: model.SpanClosing,
						Type:     child.Data,
						ID:       child.Data,
						Data:     fmt.Sprintf("</%s>", child.Data),
					})
				}
			}
		}
	}
}

// hasTextContent returns true if the node contains text content (directly or via inline children).
func (r *Reader) hasTextContent(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode && strings.TrimSpace(child.Data) != "" {
			return true
		}
		if child.Type == html.ElementNode && isInlineElement(child) {
			if r.hasTextContent(child) {
				return true
			}
		}
	}
	return false
}

func (r *Reader) renderOpenTag(n *html.Node) string {
	var buf strings.Builder
	buf.WriteString("<")
	buf.WriteString(n.Data)
	for _, attr := range n.Attr {
		buf.WriteString(" ")
		if attr.Namespace != "" {
			buf.WriteString(attr.Namespace)
			buf.WriteString(":")
		}
		buf.WriteString(attr.Key)
		buf.WriteString(`="`)
		buf.WriteString(html.EscapeString(attr.Val))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	return buf.String()
}

func (r *Reader) renderTag(n *html.Node) string {
	var buf strings.Builder
	buf.WriteString("<")
	buf.WriteString(n.Data)
	for _, attr := range n.Attr {
		buf.WriteString(" ")
		if attr.Namespace != "" {
			buf.WriteString(attr.Namespace)
			buf.WriteString(":")
		}
		buf.WriteString(attr.Key)
		buf.WriteString(`="`)
		buf.WriteString(html.EscapeString(attr.Val))
		buf.WriteString(`"`)
	}
	if selfClosingElements[n.DataAtom] {
		buf.WriteString("/>")
	} else {
		buf.WriteString(">")
	}
	return buf.String()
}

func isInlineElement(n *html.Node) bool {
	return n.Type == html.ElementNode && inlineElements[n.DataAtom]
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
