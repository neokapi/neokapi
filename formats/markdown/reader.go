package markdown

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Reader implements DataFormatReader for Markdown files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new Markdown reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "markdown",
			FormatDisplayName: "Markdown",
			FormatMimeType:    "text/markdown",
			FormatExtensions:  []string{".md", ".markdown"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/markdown", "text/x-markdown"},
		Extensions: []string{".md", ".markdown"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("markdown: nil document or reader")
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
		Format:   "markdown",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/markdown",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("markdown: reading: %w", err)}
		return
	}

	md := goldmark.New()
	doc := md.Parser().Parse(text.NewReader(content))

	blockCounter := 0
	dataCounter := 0
	r.walkNode(ctx, ch, doc, content, &blockCounter, &dataCounter)

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) walkNode(ctx context.Context, ch chan<- model.PartResult, node ast.Node, source []byte, blockCounter, dataCounter *int) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Heading:
			*blockCounter++
			textContent := r.extractInlineText(n, source)
			block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), textContent)
			block.Name = fmt.Sprintf("heading%d", *blockCounter)
			block.Type = "heading"
			block.Properties["level"] = fmt.Sprintf("%d", n.Level)
			r.addInlineSpans(block, n, source)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}

		case *ast.Paragraph:
			*blockCounter++
			textContent := r.extractInlineText(n, source)
			block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), textContent)
			block.Name = fmt.Sprintf("para%d", *blockCounter)
			r.addInlineSpans(block, n, source)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}

		case *ast.ListItem:
			*blockCounter++
			textContent := r.extractListItemText(n, source)
			block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), textContent)
			block.Name = fmt.Sprintf("item%d", *blockCounter)
			block.Type = "list-item"
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}

		case *ast.FencedCodeBlock:
			*dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", *dataCounter),
				Name: "code-block",
				Properties: map[string]string{
					"content": r.extractRawLines(n, source),
				},
			}
			if lang := n.Language(source); lang != nil {
				data.Properties["language"] = string(lang)
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}

		case *ast.CodeBlock:
			*dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", *dataCounter),
				Name: "code-block",
				Properties: map[string]string{
					"content": r.extractRawLines(n, source),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}

		case *ast.HTMLBlock:
			*dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", *dataCounter),
				Name: "html-block",
				Properties: map[string]string{
					"content": r.extractRawLines(n, source),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}

		case *ast.ThematicBreak:
			*dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", *dataCounter),
				Name: "thematic-break",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}

		case *ast.List:
			// Walk into the list to process its items
			r.walkNode(ctx, ch, child, source, blockCounter, dataCounter)

		case *ast.Blockquote:
			// Walk into the blockquote to process its children
			r.walkNode(ctx, ch, child, source, blockCounter, dataCounter)

		default:
			// For other block types, try to walk children
			r.walkNode(ctx, ch, child, source, blockCounter, dataCounter)
		}
	}
}

// extractInlineText extracts the plain text from all inline children of a block node.
func (r *Reader) extractInlineText(node ast.Node, source []byte) string {
	var buf strings.Builder
	r.collectInlineText(&buf, node, source)
	return buf.String()
}

func (r *Reader) collectInlineText(buf *strings.Builder, node ast.Node, source []byte) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			buf.Write(n.Segment.Value(source))
			if n.SoftLineBreak() {
				buf.WriteByte(' ')
			}
		case *ast.String:
			buf.Write(n.Value)
		case *ast.CodeSpan:
			// Extract code span text
			for gc := n.FirstChild(); gc != nil; gc = gc.NextSibling() {
				if t, ok := gc.(*ast.Text); ok {
					buf.Write(t.Segment.Value(source))
				}
			}
		default:
			// Recurse into other inline elements (emphasis, strong, link, etc.)
			r.collectInlineText(buf, child, source)
		}
	}
}

// extractListItemText extracts text from a list item's children.
func (r *Reader) extractListItemText(item *ast.ListItem, source []byte) string {
	var buf strings.Builder
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch t := child.(type) {
		case *ast.Paragraph:
			r.collectInlineText(&buf, child, source)
		case *ast.TextBlock:
			r.collectInlineText(&buf, child, source)
		case *ast.Text:
			buf.Write(t.Segment.Value(source))
		default:
			r.collectInlineText(&buf, child, source)
		}
	}
	return buf.String()
}

// extractRawLines extracts text from a block node's lines.
func (r *Reader) extractRawLines(node ast.Node, source []byte) string {
	var buf strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}
	return buf.String()
}

// addInlineSpans adds span information to a block for inline formatting.
func (r *Reader) addInlineSpans(block *model.Block, node ast.Node, source []byte) {
	var spans []*inlineSpan
	r.collectInlineSpans(&spans, node, source)
	if len(spans) == 0 {
		return
	}

	// Build coded text with span markers
	frag := &model.Fragment{}
	r.buildCodedFragment(frag, node, source)
	if frag.HasSpans() {
		block.Source = []*model.Segment{{ID: "s1", Content: frag}}
	}
}

type inlineSpan struct {
	spanType model.SpanType
	tag      string
	semType  string
}

func (r *Reader) collectInlineSpans(spans *[]*inlineSpan, node ast.Node, source []byte) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch em := child.(type) {
		case *ast.Emphasis:
			var tag, semType string
			if em.Level == 2 {
				tag = "**"
				semType = "bold"
			} else {
				tag = "*"
				semType = "italic"
			}
			*spans = append(*spans, &inlineSpan{spanType: model.SpanOpening, tag: tag, semType: semType})
			r.collectInlineSpans(spans, child, source)
			*spans = append(*spans, &inlineSpan{spanType: model.SpanClosing, tag: tag, semType: semType})
		case *ast.CodeSpan:
			*spans = append(*spans, &inlineSpan{spanType: model.SpanOpening, tag: "`", semType: "code"})
			*spans = append(*spans, &inlineSpan{spanType: model.SpanClosing, tag: "`", semType: "code"})
		default:
			r.collectInlineSpans(spans, child, source)
		}
	}
}

func (r *Reader) buildCodedFragment(frag *model.Fragment, node ast.Node, source []byte) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			frag.AppendText(string(n.Segment.Value(source)))
			if n.SoftLineBreak() {
				frag.AppendText(" ")
			}
		case *ast.String:
			frag.AppendText(string(n.Value))
		case *ast.Emphasis:
			var tag, semType string
			if n.Level == 2 {
				tag = "**"
				semType = "bold"
			} else {
				tag = "*"
				semType = "italic"
			}
			frag.AppendSpan(&model.Span{
				SpanType: model.SpanOpening,
				Type:     semType,
				Data:     tag,
			})
			r.buildCodedFragment(frag, child, source)
			frag.AppendSpan(&model.Span{
				SpanType: model.SpanClosing,
				Type:     semType,
				Data:     tag,
			})
		case *ast.CodeSpan:
			frag.AppendSpan(&model.Span{
				SpanType: model.SpanOpening,
				Type:     "code",
				Data:     "`",
			})
			for gc := n.FirstChild(); gc != nil; gc = gc.NextSibling() {
				if t, ok := gc.(*ast.Text); ok {
					frag.AppendText(string(t.Segment.Value(source)))
				}
			}
			frag.AppendSpan(&model.Span{
				SpanType: model.SpanClosing,
				Type:     "code",
				Data:     "`",
			})
		case *ast.Link:
			frag.AppendSpan(&model.Span{
				SpanType: model.SpanOpening,
				Type:     "link",
				Data:     "[",
			})
			r.buildCodedFragment(frag, child, source)
			frag.AppendSpan(&model.Span{
				SpanType: model.SpanClosing,
				Type:     "link",
				Data:     fmt.Sprintf("](%s)", string(n.Destination)),
			})
		default:
			r.buildCodedFragment(frag, child, source)
		}
	}
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
