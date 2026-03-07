package markdown

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// Reader implements DataFormatReader for Markdown files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	vocab         *model.VocabularyRegistry
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalescing buffer for skeleton text
	skelCursor    int          // current position in source for skeleton tracking

	source       []byte
	blockCounter int
	dataCounter  int
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new Markdown reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	vocab := model.NewVocabularyRegistry()
	_ = vocab.LoadDefaults()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "markdown",
			FormatDisplayName: "Markdown",
			FormatMimeType:    "text/markdown",
			FormatExtensions:  []string{".md", ".markdown"},
			Cfg:               cfg,
		},
		cfg:   cfg,
		vocab: vocab,
	}
}

// MarkdownConfig returns the reader's markdown-specific config for customization.
func (r *Reader) MarkdownConfig() *Config {
	return r.cfg
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
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
		if err := r.readContent(ctx, ch); err != nil {
			ch <- model.PartResult{Error: err}
		}
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) error {
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
		return nil
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		return fmt.Errorf("markdown: reading: %w", err)
	}
	r.source = content
	r.skelCursor = 0

	r.blockCounter = 0
	r.dataCounter = 0

	// Handle YAML front matter.
	bodyOffset := r.handleFrontMatter(ctx, ch, content)

	// Parse the markdown body with GFM extensions (tables, strikethrough).
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
	)
	body := content[bodyOffset:]
	doc := md.Parser().Parse(text.NewReader(body))

	r.walkNode(ctx, ch, doc, body, bodyOffset)

	// Flush remaining source bytes as skeleton text.
	if r.skeletonStore != nil && r.skelCursor < len(content) {
		r.skelText(string(content[r.skelCursor:]))
	}
	r.skelFlush()
	if r.skeletonStore != nil {
		if err := r.skeletonStore.Flush(); err != nil {
			return err
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
	return nil
}

// handleFrontMatter detects and processes YAML front matter (--- delimited).
// Returns the byte offset where the markdown body starts.
func (r *Reader) handleFrontMatter(ctx context.Context, ch chan<- model.PartResult, content []byte) int {
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return 0
	}

	// Find closing ---
	var searchStart int
	if len(content) > 4 && content[3] == '\r' {
		searchStart = 5
	} else {
		searchStart = 4
	}
	closingIdx := -1
	for i := searchStart; i < len(content); i++ {
		if content[i] == '-' && i+3 <= len(content) && string(content[i:i+3]) == "---" {
			if i == 0 || content[i-1] == '\n' {
				endIdx := i + 3
				if endIdx >= len(content) || content[endIdx] == '\n' || content[endIdx] == '\r' {
					closingIdx = i
					break
				}
			}
		}
	}
	if closingIdx < 0 {
		return 0
	}

	endOfFrontMatter := closingIdx + 3
	if endOfFrontMatter < len(content) && content[endOfFrontMatter] == '\r' {
		endOfFrontMatter++
	}
	if endOfFrontMatter < len(content) && content[endOfFrontMatter] == '\n' {
		endOfFrontMatter++
	}

	frontMatterRaw := string(content[:endOfFrontMatter])

	if r.cfg.TranslateFrontMatter {
		yamlContent := string(content[searchStart:closingIdx])
		r.skelText(string(content[:searchStart]))
		r.emitFrontMatterBlocks(ctx, ch, yamlContent)
		endMarker := string(content[closingIdx:endOfFrontMatter])
		r.skelText(endMarker)
	} else {
		r.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", r.dataCounter),
			Name: "front-matter",
			Properties: map[string]string{
				"content": frontMatterRaw,
			},
		}
		r.skelText(frontMatterRaw)
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}

	r.skelCursor = endOfFrontMatter
	return endOfFrontMatter
}

// emitFrontMatterBlocks emits each YAML value as a translatable block.
func (r *Reader) emitFrontMatterBlocks(ctx context.Context, ch chan<- model.PartResult, yaml string) {
	lines := strings.Split(yaml, "\n")
	for _, line := range lines {
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			r.skelText(line + "\n")
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		if value == "" || key == "" {
			r.skelText(line + "\n")
			continue
		}

		unquoted := value
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			unquoted = value[1 : len(value)-1]
		}

		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, unquoted)
		block.Name = fmt.Sprintf("fm_%s", key)
		block.Type = "front-matter"
		block.Properties["key"] = key

		prefix := line[:colonIdx+1]
		valuePart := line[colonIdx+1:]
		leadingSpace := ""
		for _, c := range valuePart {
			if c == ' ' || c == '\t' {
				leadingSpace += string(c)
			} else {
				break
			}
		}
		r.skelText(prefix + leadingSpace)
		r.skelRef(blockID)
		r.skelText("\n")

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}
}

// blockRange returns the full byte range of a block node including its prefix
// markers (like "# " for headings, "- " for list items) and trailing newlines.
// For nodes with Lines(), this is the range from the line start to line end.
// The returned range is relative to source (body), not absolute.
func blockRange(node ast.Node, source []byte) (int, int) {
	lines := node.Lines()
	if lines.Len() > 0 {
		first := lines.At(0)
		last := lines.At(lines.Len() - 1)
		return first.Start, last.Stop
	}
	// For container nodes (List, ListItem, Blockquote), compute from children.
	start := len(source)
	end := 0
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		cs, ce := blockRange(child, source)
		if cs < start {
			start = cs
		}
		if ce > end {
			end = ce
		}
	}
	if start >= end {
		return 0, 0
	}
	return start, end
}

func (r *Reader) walkNode(ctx context.Context, ch chan<- model.PartResult, node ast.Node, source []byte, baseOffset int) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Heading:
			r.emitHeading(ctx, ch, n, source, baseOffset)

		case *ast.Paragraph:
			r.emitParagraph(ctx, ch, n, source, baseOffset)

		case *ast.ListItem:
			r.emitListItem(ctx, ch, n, source, baseOffset)

		case *ast.FencedCodeBlock:
			r.emitFencedCodeBlock(ctx, ch, n, source, baseOffset)

		case *ast.CodeBlock:
			r.emitIndentedCodeBlock(ctx, ch, n, source, baseOffset)

		case *ast.HTMLBlock:
			r.emitHTMLBlock(ctx, ch, n, source, baseOffset)

		case *ast.ThematicBreak:
			r.emitThematicBreak(ctx, ch, n, source, baseOffset)

		case *ast.List:
			r.walkNode(ctx, ch, child, source, baseOffset)

		case *ast.Blockquote:
			if r.cfg.TranslateBlockQuotes() {
				r.walkNode(ctx, ch, child, source, baseOffset)
			} else {
				r.emitBlockquoteAsData(ctx, ch, n, source, baseOffset)
			}

		default:
			if child.Kind() == east.KindTable {
				r.emitTable(ctx, ch, child, source, baseOffset)
			} else {
				r.walkNode(ctx, ch, child, source, baseOffset)
			}
		}
	}
}

// skelEmitGap emits any source bytes between the current cursor and the given
// absolute position as skeleton text.
func (r *Reader) skelEmitGap(absPos int) {
	if r.skeletonStore == nil {
		return
	}
	if absPos > r.skelCursor {
		r.skelText(string(r.source[r.skelCursor:absPos]))
		r.skelCursor = absPos
	}
}

// nodeAbsRange returns the absolute byte range of a node's lines content,
// accounting for the baseOffset from the body start.
func nodeAbsRange(node ast.Node, source []byte, baseOffset int) (int, int) {
	s, e := blockRange(node, source)
	return s + baseOffset, e + baseOffset
}

// fullNodeAbsRange returns the absolute byte range of a node including
// any prefix characters (like "# " for headings). This scans backward
// from the line start to find the actual start of the markdown line.
func fullNodeAbsRange(node ast.Node, source []byte, baseOffset int) (int, int) {
	lines := node.Lines()
	if lines.Len() == 0 {
		return nodeAbsRange(node, source, baseOffset)
	}
	first := lines.At(0)
	last := lines.At(lines.Len() - 1)

	// Scan backward from line content start to find the beginning of the
	// source line (to capture prefixes like "# ", "- ", "> ", "1. ").
	lineStart := first.Start
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}

	return lineStart + baseOffset, last.Stop + baseOffset
}

func (r *Reader) emitHeading(ctx context.Context, ch chan<- model.PartResult, n *ast.Heading, source []byte, baseOffset int) {
	r.blockCounter++
	blockID := fmt.Sprintf("tu%d", r.blockCounter)
	textContent := r.extractInlineText(n, source)
	block := model.NewBlock(blockID, textContent)
	block.Name = fmt.Sprintf("heading%d", r.blockCounter)
	block.Type = "heading"
	block.Properties["level"] = fmt.Sprintf("%d", n.Level)
	r.addInlineSpans(block, n, source)

	absStart, _ := fullNodeAbsRange(n, source, baseOffset)
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)

	// Emit gap from cursor to the node's full start (includes blank lines before)
	r.skelEmitGap(absStart)
	// Emit prefix (e.g. "# ") as skeleton text
	r.skelText(string(r.source[absStart:lineStart]))
	// Emit block ref for the inline content
	r.skelRef(blockID)
	// Advance cursor past the lines
	r.skelCursor = lineEnd

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

func (r *Reader) emitParagraph(ctx context.Context, ch chan<- model.PartResult, n *ast.Paragraph, source []byte, baseOffset int) {
	r.blockCounter++
	blockID := fmt.Sprintf("tu%d", r.blockCounter)
	textContent := r.extractInlineText(n, source)
	block := model.NewBlock(blockID, textContent)
	block.Name = fmt.Sprintf("para%d", r.blockCounter)
	r.addInlineSpans(block, n, source)

	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)

	r.skelEmitGap(lineStart)
	r.skelRef(blockID)
	r.skelCursor = lineEnd

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

func (r *Reader) emitListItem(ctx context.Context, ch chan<- model.PartResult, n *ast.ListItem, source []byte, baseOffset int) {
	// Check for nested blocks
	hasNestedBlocks := false
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.(type) {
		case *ast.List, *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock:
			hasNestedBlocks = true
		}
	}

	if hasNestedBlocks {
		r.walkNode(ctx, ch, n, source, baseOffset)
		return
	}

	r.blockCounter++
	blockID := fmt.Sprintf("tu%d", r.blockCounter)
	textContent := r.extractListItemText(n, source)
	block := model.NewBlock(blockID, textContent)
	block.Name = fmt.Sprintf("item%d", r.blockCounter)
	block.Type = "list-item"

	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if p, ok := child.(*ast.Paragraph); ok {
			r.addInlineSpans(block, p, source)
			break
		}
		if _, ok := child.(*ast.TextBlock); ok {
			r.addInlineSpans(block, child, source)
			break
		}
	}

	// For list items: find the text block range and include the list marker prefix
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)
	absStart := lineStart
	// Scan backward to find the list marker
	for absStart > 0 && r.source[absStart-1] != '\n' {
		absStart--
	}

	r.skelEmitGap(absStart)
	// The prefix (e.g. "- " or "1. ") goes as skeleton text
	r.skelText(string(r.source[absStart:lineStart]))
	r.skelRef(blockID)
	r.skelCursor = lineEnd

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

func (r *Reader) emitFencedCodeBlock(ctx context.Context, ch chan<- model.PartResult, n *ast.FencedCodeBlock, source []byte, baseOffset int) {
	content := r.extractRawLines(n, source)
	lang := ""
	if l := n.Language(source); l != nil {
		lang = string(l)
	}

	// For fenced code blocks, we need the full range including the ``` fences.
	// The Lines() only contain the code content, not the fence lines.
	// We need to find the fence lines in the source.
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)

	// Scan backward from the code content to find the opening fence
	fenceStart := lineStart
	for fenceStart > 0 && r.source[fenceStart-1] != '\n' {
		fenceStart--
	}
	// Go one more line back to find the opening fence line itself
	if fenceStart > 0 {
		// fenceStart is at the start of the first code content line,
		// the opening ``` is the line before
		openFenceStart := fenceStart
		if openFenceStart > 0 {
			openFenceStart--
			for openFenceStart > 0 && r.source[openFenceStart-1] != '\n' {
				openFenceStart--
			}
		}
		fenceStart = openFenceStart
	}

	// Find the closing fence after the content
	fenceEnd := lineEnd
	if fenceEnd < len(r.source) && r.source[fenceEnd] == '\n' {
		fenceEnd++
	}
	// The closing ``` line
	closeFenceEnd := fenceEnd
	for closeFenceEnd < len(r.source) && r.source[closeFenceEnd] != '\n' {
		closeFenceEnd++
	}
	if closeFenceEnd < len(r.source) && r.source[closeFenceEnd] == '\n' {
		closeFenceEnd++
	}

	if r.cfg.TranslateCodeBlocks {
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, content)
		block.Name = fmt.Sprintf("code%d", r.blockCounter)
		block.Type = "code-block"
		if lang != "" {
			block.Properties["language"] = lang
		}

		r.skelEmitGap(fenceStart)
		// Opening fence as skeleton text
		r.skelText(string(r.source[fenceStart:lineStart]))
		r.skelRef(blockID)
		// Closing fence as skeleton text
		r.skelText(string(r.source[lineEnd:closeFenceEnd]))
		r.skelCursor = closeFenceEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	} else {
		r.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", r.dataCounter),
			Name: "code-block",
			Properties: map[string]string{
				"content": content,
			},
		}
		if lang != "" {
			data.Properties["language"] = lang
		}

		r.skelEmitGap(fenceStart)
		r.skelText(string(r.source[fenceStart:closeFenceEnd]))
		r.skelCursor = closeFenceEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}
}

func (r *Reader) emitIndentedCodeBlock(ctx context.Context, ch chan<- model.PartResult, n *ast.CodeBlock, source []byte, baseOffset int) {
	content := r.extractRawLines(n, source)

	// For indented code blocks, the Lines() include the indented content.
	// Scan backward to find the real start including indentation.
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)
	absStart := lineStart
	for absStart > 0 && r.source[absStart-1] != '\n' {
		absStart--
	}

	if r.cfg.TranslateCodeBlocks {
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, content)
		block.Name = fmt.Sprintf("code%d", r.blockCounter)
		block.Type = "code-block"

		r.skelEmitGap(absStart)
		r.skelText(string(r.source[absStart:lineStart]))
		r.skelRef(blockID)
		r.skelCursor = lineEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	} else {
		r.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", r.dataCounter),
			Name: "code-block",
			Properties: map[string]string{
				"content": content,
			},
		}

		r.skelEmitGap(absStart)
		r.skelText(string(r.source[absStart:lineEnd]))
		r.skelCursor = lineEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}
}

func (r *Reader) emitHTMLBlock(ctx context.Context, ch chan<- model.PartResult, n *ast.HTMLBlock, source []byte, baseOffset int) {
	content := r.extractRawLines(n, source)
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)

	// HTML blocks also have a ClosureLine that may contain the closing tag
	if n.HasClosure() {
		cl := n.ClosureLine
		closureEnd := cl.Stop + baseOffset
		if closureEnd > lineEnd {
			content += string(cl.Value(source))
			lineEnd = closureEnd
		}
	}

	if r.cfg.TranslateHTMLBlocks {
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, content)
		block.Name = fmt.Sprintf("html%d", r.blockCounter)
		block.Type = "html-block"

		r.skelEmitGap(lineStart)
		r.skelRef(blockID)
		r.skelCursor = lineEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	} else {
		r.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", r.dataCounter),
			Name: "html-block",
			Properties: map[string]string{
				"content": content,
			},
		}

		r.skelEmitGap(lineStart)
		r.skelText(string(r.source[lineStart:lineEnd]))
		r.skelCursor = lineEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}
}

func (r *Reader) emitThematicBreak(ctx context.Context, ch chan<- model.PartResult, n *ast.ThematicBreak, source []byte, baseOffset int) {
	r.dataCounter++
	data := &model.Data{
		ID:   fmt.Sprintf("d%d", r.dataCounter),
		Name: "thematic-break",
	}
	// ThematicBreak has no Lines(). The break text (e.g. "---\n") is in the
	// gap between the previous and next nodes. Since thematic break is Data
	// (non-translatable), we don't need a ref — skelEmitGap from the next
	// node will capture this gap as skeleton text.
	r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
}

func (r *Reader) emitBlockquoteAsData(ctx context.Context, ch chan<- model.PartResult, n *ast.Blockquote, source []byte, baseOffset int) {
	r.dataCounter++
	absStart, absEnd := nodeAbsRange(n, source, baseOffset)
	// Scan backward to include the > prefix
	for absStart > 0 && r.source[absStart-1] != '\n' {
		absStart--
	}
	rawContent := string(r.source[absStart:absEnd])
	data := &model.Data{
		ID:   fmt.Sprintf("d%d", r.dataCounter),
		Name: "blockquote",
		Properties: map[string]string{
			"content": rawContent,
		},
	}
	r.skelEmitGap(absStart)
	r.skelText(rawContent)
	r.skelCursor = absEnd
	r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
}

func (r *Reader) emitTable(ctx context.Context, ch chan<- model.PartResult, node ast.Node, source []byte, baseOffset int) {
	// For skeleton: emit the entire table as text with refs for cell content.
	// This requires tracking positions within the table.
	absStart, absEnd := nodeAbsRange(node, source, baseOffset)
	r.skelEmitGap(absStart)

	// Walk through the table raw source line by line, emitting cell content as refs.
	// This is complex; for now, emit the whole table as skeleton text and blocks.
	tableSource := string(r.source[absStart:absEnd])

	// Simple approach: emit table structure as skeleton, with cell content as refs.
	// We'll track position within the table source.
	var cellBlocks []*model.Block
	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		if row.Kind() == east.KindTableHeader || row.Kind() == east.KindTableRow {
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if cell.Kind() == east.KindTableCell {
					cellText := r.extractInlineText(cell, source)
					if strings.TrimSpace(cellText) == "" {
						continue
					}
					r.blockCounter++
					blockID := fmt.Sprintf("tu%d", r.blockCounter)
					block := model.NewBlock(blockID, cellText)
					block.Name = fmt.Sprintf("cell%d", r.blockCounter)
					block.Type = "table-cell"
					r.addInlineSpans(block, cell, source)
					cellBlocks = append(cellBlocks, block)
				}
			}
		}
	}

	// For skeleton: emit the whole table as text (can't do byte-exact cell-level yet)
	r.skelText(tableSource)
	r.skelCursor = absEnd

	for _, block := range cellBlocks {
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}
}

// --- Skeleton store helpers ---

func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil {
		r.skelBuf.WriteString(s)
	}
}

func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		r.skeletonStore.WriteRef(id)
	}
}

func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

// --- Inline text extraction ---

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
			if n.HardLineBreak() {
				buf.WriteByte('\n')
			}
		case *ast.String:
			buf.Write(n.Value)
		case *ast.CodeSpan:
			for gc := n.FirstChild(); gc != nil; gc = gc.NextSibling() {
				if t, ok := gc.(*ast.Text); ok {
					buf.Write(t.Segment.Value(source))
				}
			}
		case *ast.Image:
			if r.cfg.TranslateImageAlt() {
				r.collectInlineText(buf, child, source)
			}
		case *ast.AutoLink:
			buf.Write(n.URL(source))
		default:
			r.collectInlineText(buf, child, source)
		}
	}
}

func (r *Reader) extractListItemText(item *ast.ListItem, source []byte) string {
	var buf strings.Builder
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			r.collectInlineText(&buf, child, source)
		case *ast.Text:
			t := child.(*ast.Text)
			buf.Write(t.Segment.Value(source))
		default:
			r.collectInlineText(&buf, child, source)
		}
	}
	return buf.String()
}

func (r *Reader) extractRawLines(node ast.Node, source []byte) string {
	var buf strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}
	return buf.String()
}

// --- Inline span building ---

func (r *Reader) addInlineSpans(block *model.Block, node ast.Node, source []byte) {
	frag := &model.Fragment{}
	spanCounter := 0
	r.buildCodedFragment(frag, node, source, &spanCounter)
	if frag.HasSpans() {
		block.Source = []*model.Segment{{ID: "s1", Content: frag}}
	}
}

func (r *Reader) buildCodedFragment(frag *model.Fragment, node ast.Node, source []byte, spanCounter *int) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			frag.AppendText(string(n.Segment.Value(source)))
			if n.SoftLineBreak() {
				frag.AppendText(" ")
			}
			if n.HardLineBreak() {
				frag.AppendText("\n")
			}
		case *ast.String:
			frag.AppendText(string(n.Value))

		case *ast.Emphasis:
			r.buildEmphasisSpan(frag, n, source, spanCounter)

		case *ast.CodeSpan:
			r.buildCodeSpan(frag, n, source, spanCounter)

		case *ast.Link:
			r.buildLinkSpan(frag, n, source, spanCounter)

		case *ast.Image:
			r.buildImageSpan(frag, n, source, spanCounter)

		case *ast.AutoLink:
			r.buildAutoLinkSpan(frag, n, source, spanCounter)

		case *ast.RawHTML:
			r.buildRawHTMLSpan(frag, n, source, spanCounter)

		default:
			if child.Kind() == east.KindStrikethrough {
				r.buildStrikethroughSpan(frag, child, source, spanCounter)
			} else {
				r.buildCodedFragment(frag, child, source, spanCounter)
			}
		}
	}
}

func (r *Reader) buildEmphasisSpan(frag *model.Fragment, n *ast.Emphasis, source []byte, spanCounter *int) {
	var semType, subType, data string
	if n.Level == 2 {
		semType = "fmt:bold"
		subType = "md:strong"
		data = "**"
	} else {
		semType = "fmt:italic"
		subType = "md:emphasis"
		data = "*"
	}
	*spanCounter++
	id := strconv.Itoa(*spanCounter)
	info := r.vocab.LookupOrFallback(semType)
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanOpening,
		Type:        semType,
		SubType:     subType,
		ID:          id,
		Data:        data,
		DisplayText: info.Display.Open,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
	r.buildCodedFragment(frag, n, source, spanCounter)
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanClosing,
		Type:        semType,
		SubType:     subType,
		ID:          id,
		Data:        data,
		DisplayText: info.Display.Close,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
}

func (r *Reader) buildCodeSpan(frag *model.Fragment, n *ast.CodeSpan, source []byte, spanCounter *int) {
	*spanCounter++
	id := strconv.Itoa(*spanCounter)
	info := r.vocab.LookupOrFallback("fmt:code")
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanOpening,
		Type:        "fmt:code",
		SubType:     "md:code",
		ID:          id,
		Data:        "`",
		DisplayText: info.Display.Open,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
	for gc := n.FirstChild(); gc != nil; gc = gc.NextSibling() {
		if t, ok := gc.(*ast.Text); ok {
			frag.AppendText(string(t.Segment.Value(source)))
		}
	}
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanClosing,
		Type:        "fmt:code",
		SubType:     "md:code",
		ID:          id,
		Data:        "`",
		DisplayText: info.Display.Close,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
}

func (r *Reader) buildLinkSpan(frag *model.Fragment, n *ast.Link, source []byte, spanCounter *int) {
	*spanCounter++
	id := strconv.Itoa(*spanCounter)
	info := r.vocab.LookupOrFallback("link:hyperlink")

	closingData := fmt.Sprintf("](%s", string(n.Destination))
	if n.Title != nil && len(n.Title) > 0 {
		closingData += fmt.Sprintf(` "%s"`, string(n.Title))
	}
	closingData += ")"

	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanOpening,
		Type:        "link:hyperlink",
		SubType:     "md:link",
		ID:          id,
		Data:        "[",
		DisplayText: info.Display.Open,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
	r.buildCodedFragment(frag, n, source, spanCounter)
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanClosing,
		Type:        "link:hyperlink",
		SubType:     "md:link",
		ID:          id,
		Data:        closingData,
		DisplayText: info.Display.Close,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
}

func (r *Reader) buildImageSpan(frag *model.Fragment, n *ast.Image, source []byte, spanCounter *int) {
	*spanCounter++
	id := strconv.Itoa(*spanCounter)
	info := r.vocab.LookupOrFallback("link:image")

	closingData := fmt.Sprintf("](%s", string(n.Destination))
	if n.Title != nil && len(n.Title) > 0 {
		closingData += fmt.Sprintf(` "%s"`, string(n.Title))
	}
	closingData += ")"

	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanOpening,
		Type:        "link:image",
		SubType:     "md:image",
		ID:          id,
		Data:        "![",
		DisplayText: info.Display.Open,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
	if r.cfg.TranslateImageAlt() {
		r.buildCodedFragment(frag, n, source, spanCounter)
	}
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanClosing,
		Type:        "link:image",
		SubType:     "md:image",
		ID:          id,
		Data:        closingData,
		DisplayText: info.Display.Close,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
}

func (r *Reader) buildAutoLinkSpan(frag *model.Fragment, n *ast.AutoLink, source []byte, spanCounter *int) {
	*spanCounter++
	id := strconv.Itoa(*spanCounter)
	info := r.vocab.LookupOrFallback("link:hyperlink")
	url := string(n.URL(source))
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanOpening,
		Type:        "link:hyperlink",
		SubType:     "md:autolink",
		ID:          id,
		Data:        "<",
		DisplayText: info.Display.Open,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
	frag.AppendText(url)
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanClosing,
		Type:        "link:hyperlink",
		SubType:     "md:autolink",
		ID:          id,
		Data:        ">",
		DisplayText: info.Display.Close,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
}

func (r *Reader) buildRawHTMLSpan(frag *model.Fragment, n *ast.RawHTML, source []byte, spanCounter *int) {
	*spanCounter++
	id := strconv.Itoa(*spanCounter)

	var htmlContent strings.Builder
	for i := 0; i < n.Segments.Len(); i++ {
		seg := n.Segments.At(i)
		htmlContent.Write(seg.Value(source))
	}
	tag := htmlContent.String()

	frag.AppendSpan(&model.Span{
		SpanType: model.SpanOpening,
		Type:     "fmt:html",
		SubType:  "md:html-inline",
		ID:       id,
		Data:     tag,
	})
	frag.AppendSpan(&model.Span{
		SpanType: model.SpanClosing,
		Type:     "fmt:html",
		SubType:  "md:html-inline",
		ID:       id,
		Data:     "",
	})
}

func (r *Reader) buildStrikethroughSpan(frag *model.Fragment, node ast.Node, source []byte, spanCounter *int) {
	*spanCounter++
	id := strconv.Itoa(*spanCounter)
	info := r.vocab.LookupOrFallback("fmt:strike")
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanOpening,
		Type:        "fmt:strike",
		SubType:     "md:strikethrough",
		ID:          id,
		Data:        "~~",
		DisplayText: info.Display.Open,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
	r.buildCodedFragment(frag, node, source, spanCounter)
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanClosing,
		Type:        "fmt:strike",
		SubType:     "md:strikethrough",
		ID:          id,
		Data:        "~~",
		DisplayText: info.Display.Close,
		EquivText:   info.Equiv,
		Deletable:   info.Constraints.Deletable,
		Cloneable:   info.Constraints.Cloneable,
		CanReorder:  info.Constraints.Reorderable,
	})
}

// --- Emit helper ---

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
