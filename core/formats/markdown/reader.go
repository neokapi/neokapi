package markdown

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
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
		return errors.New("markdown: nil document or reader")
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
		block.Name = "fm_" + key
		block.Type = "front-matter"
		block.Properties["key"] = key

		prefix := line[:colonIdx+1]
		valuePart := line[colonIdx+1:]
		leadingSpace := ""
		var leadingSpaceSb245 strings.Builder
		for _, c := range valuePart {
			if c == ' ' || c == '\t' {
				leadingSpaceSb245.WriteString(string(c))
			} else {
				break
			}
		}
		leadingSpace += leadingSpaceSb245.String()
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
	block.Properties["level"] = strconv.Itoa(n.Level)
	r.addInlineRuns(block, n, source)

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
	r.addInlineRuns(block, n, source)

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
			r.addInlineRuns(block, p, source)
			break
		}
		if _, ok := child.(*ast.TextBlock); ok {
			r.addInlineRuns(block, child, source)
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
					r.addInlineRuns(block, cell, source)
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
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
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
		switch n := child.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			r.collectInlineText(&buf, child, source)
		case *ast.Text:
			buf.Write(n.Segment.Value(source))
		default:
			r.collectInlineText(&buf, child, source)
		}
	}
	return buf.String()
}

func (r *Reader) extractRawLines(node ast.Node, source []byte) string {
	var buf strings.Builder
	lines := node.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}
	return buf.String()
}

// --- Inline run building ---

func (r *Reader) addInlineRuns(block *model.Block, node ast.Node, source []byte) {
	b := newRunBuilder()
	idCounter := 0
	r.buildCodedRuns(b, node, source, &idCounter)
	if b.HasInlineCodes() {
		block.Source = []*model.Segment{model.NewRunsSegment("s1", b.Runs())}
	}
}

func (r *Reader) buildCodedRuns(b *runBuilder, node ast.Node, source []byte, idCounter *int) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			b.AddText(string(n.Segment.Value(source)))
			if n.SoftLineBreak() {
				b.AddText(" ")
			}
			if n.HardLineBreak() {
				b.AddText("\n")
			}
		case *ast.String:
			b.AddText(string(n.Value))

		case *ast.Emphasis:
			r.buildEmphasisRuns(b, n, source, idCounter)

		case *ast.CodeSpan:
			r.buildCodeSpanRuns(b, n, source, idCounter)

		case *ast.Link:
			r.buildLinkRuns(b, n, source, idCounter)

		case *ast.Image:
			r.buildImageRuns(b, n, source, idCounter)

		case *ast.AutoLink:
			r.buildAutoLinkRuns(b, n, source, idCounter)

		case *ast.RawHTML:
			r.buildRawHTMLRuns(b, n, source, idCounter)

		default:
			if child.Kind() == east.KindStrikethrough {
				r.buildStrikethroughRuns(b, child, source, idCounter)
			} else {
				r.buildCodedRuns(b, child, source, idCounter)
			}
		}
	}
}

func (r *Reader) buildEmphasisRuns(b *runBuilder, n *ast.Emphasis, source []byte, idCounter *int) {
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
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback(semType)
	b.AddPcOpen(id, semType, subType, data, info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	r.buildCodedRuns(b, n, source, idCounter)
	b.AddPcClose(id, semType, subType, data, info.Equiv)
}

func (r *Reader) buildCodeSpanRuns(b *runBuilder, n *ast.CodeSpan, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback("fmt:code")
	b.AddPcOpen(id, "fmt:code", "md:code", "`", info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	for gc := n.FirstChild(); gc != nil; gc = gc.NextSibling() {
		if t, ok := gc.(*ast.Text); ok {
			b.AddText(string(t.Segment.Value(source)))
		}
	}
	b.AddPcClose(id, "fmt:code", "md:code", "`", info.Equiv)
}

func (r *Reader) buildLinkRuns(b *runBuilder, n *ast.Link, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback("link:hyperlink")

	closingData := "](" + string(n.Destination)
	if len(n.Title) > 0 {
		closingData += fmt.Sprintf(` "%s"`, string(n.Title))
	}
	closingData += ")"

	b.AddPcOpen(id, "link:hyperlink", "md:link", "[", info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	r.buildCodedRuns(b, n, source, idCounter)
	b.AddPcClose(id, "link:hyperlink", "md:link", closingData, info.Equiv)
}

func (r *Reader) buildImageRuns(b *runBuilder, n *ast.Image, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback("link:image")

	closingData := "](" + string(n.Destination)
	if len(n.Title) > 0 {
		closingData += fmt.Sprintf(` "%s"`, string(n.Title))
	}
	closingData += ")"

	b.AddPcOpen(id, "link:image", "md:image", "![", info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	if r.cfg.TranslateImageAlt() {
		r.buildCodedRuns(b, n, source, idCounter)
	}
	b.AddPcClose(id, "link:image", "md:image", closingData, info.Equiv)
}

func (r *Reader) buildAutoLinkRuns(b *runBuilder, n *ast.AutoLink, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback("link:hyperlink")
	url := string(n.URL(source))
	b.AddPcOpen(id, "link:hyperlink", "md:autolink", "<", info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	b.AddText(url)
	b.AddPcClose(id, "link:hyperlink", "md:autolink", ">", info.Equiv)
}

func (r *Reader) buildRawHTMLRuns(b *runBuilder, n *ast.RawHTML, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)

	var htmlContent strings.Builder
	for i := range n.Segments.Len() {
		seg := n.Segments.At(i)
		htmlContent.Write(seg.Value(source))
	}
	tag := htmlContent.String()

	// Raw inline HTML has no vocabulary entry in the original Fragment
	// path, so emit with empty display/equiv and zero-valued constraints
	// (mirrors the default all-false RunConstraints that MarshalRuns
	// produces for a Span with unset Deletable/Cloneable/CanReorder).
	b.AddPcOpen(id, "fmt:html", "md:html-inline", tag, "", "", false, false, false)
	b.AddPcClose(id, "fmt:html", "md:html-inline", "", "")
}

func (r *Reader) buildStrikethroughRuns(b *runBuilder, node ast.Node, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback("fmt:strike")
	b.AddPcOpen(id, "fmt:strike", "md:strikethrough", "~~", info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	r.buildCodedRuns(b, node, source, idCounter)
	b.AddPcClose(id, "fmt:strike", "md:strikethrough", "~~", info.Equiv)
}

// --- Emit helper ---

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	// Apply inline code finder to blocks if enabled
	if part.Type == model.PartBlock && r.cfg.UseCodeFinder {
		if block, ok := part.Resource.(*model.Block); ok {
			r.applyCodeFinder(block)
		}
	}
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// applyCodeFinder applies code finder patterns to a block's fragments.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.GetCodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}

	for _, seg := range block.Source {
		if len(seg.Runs) == 0 {
			continue
		}
		text := seg.Text()

		type matchRange struct {
			start, end int
		}
		var matches []matchRange
		for _, re := range patterns {
			for _, loc := range re.FindAllStringIndex(text, -1) {
				matches = append(matches, matchRange{loc[0], loc[1]})
			}
		}
		if len(matches) == 0 {
			continue
		}

		// Sort matches by start position
		for i := 1; i < len(matches); i++ {
			for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
				matches[j], matches[j-1] = matches[j-1], matches[j]
			}
		}

		var newRuns []model.Run
		lastEnd := 0
		spanID := 1
		for _, m := range matches {
			if m.start > lastEnd {
				newRuns = append(newRuns, model.Run{Text: &model.TextRun{Text: text[lastEnd:m.start]}})
			}
			newRuns = append(newRuns, model.Run{Ph: &model.PlaceholderRun{
				ID:   fmt.Sprintf("c%d", spanID),
				Type: "code",
				Data: text[m.start:m.end],
			}})
			lastEnd = m.end
			spanID++
		}
		if lastEnd < len(text) {
			newRuns = append(newRuns, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
		}
		seg.SetRuns(newRuns)
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
