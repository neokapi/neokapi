package html

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Writer implements DataFormatWriter for HTML files.
type Writer struct {
	format.BaseFormatWriter
	sourcePath      string
	originalContent []byte
	cfg             *Config
}

// NewWriter creates a new HTML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "html",
		},
		cfg: &Config{},
	}
}

// SetSourcePath sets the path to the original document for re-parse mode.
func (w *Writer) SetSourcePath(path string) {
	w.sourcePath = path
}

// SetOriginalContent sets the original document bytes for re-parse mode.
func (w *Writer) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// Write consumes Parts from a channel and writes reconstructed HTML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all blocks keyed by ID.
	blocks := make(map[string]*model.Block)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if b, ok := part.Resource.(*model.Block); ok {
					blocks[b.ID] = b
				}
			}
		}
	}
done:

	// If we have original content, use re-parse mode.
	content, err := w.loadOriginalContent()
	if err != nil {
		return err
	}
	if content != nil {
		return w.writeReparse(content, blocks)
	}

	// Fallback: block-only output (existing behavior).
	return w.writeFallback(blocks)
}

// loadOriginalContent returns original content bytes, or nil if unavailable.
func (w *Writer) loadOriginalContent() ([]byte, error) {
	if w.originalContent != nil {
		return w.originalContent, nil
	}
	if w.sourcePath != "" {
		data, err := os.ReadFile(w.sourcePath)
		if err != nil {
			return nil, fmt.Errorf("html writer: read source: %w", err)
		}
		return data, nil
	}
	return nil, nil
}

// writeReparse re-parses the original HTML, patches translations, and renders.
func (w *Writer) writeReparse(content []byte, blocks map[string]*model.Block) error {
	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return fmt.Errorf("html writer: parse original: %w", err)
	}

	state := &writerState{blocks: blocks}
	w.patchNode(doc, state, false)

	return html.Render(w.Output, doc)
}

// writerState tracks mutable counters during tree traversal,
// mirroring readerState for identical ID assignment.
type writerState struct {
	blockCounter int
	dataCounter  int
	blocks       map[string]*model.Block
}

// patchNode mirrors walkNode in reader.go, assigning IDs in the same order.
func (w *Writer) patchNode(n *html.Node, state *writerState, translateNo bool) {
	switch n.Type {
	case html.DocumentNode:
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			w.patchNode(child, state, translateNo)
		}

	case html.DoctypeNode:
		state.dataCounter++

	case html.CommentNode:
		state.dataCounter++

	case html.ElementNode:
		w.patchElement(n, state, translateNo)

	case html.TextNode:
		if translateNo {
			return
		}
		text := n.Data
		if !w.cfg.PreserveWhitespace {
			text = collapseWhitespace(text)
			text = strings.TrimFunc(text, isHTMLWhitespace)
		}
		if text != "" {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			if block, ok := state.blocks[blockID]; ok {
				n.Data = w.getBlockText(block)
			}
		}
	}
}

// patchElement mirrors walkElement in reader.go.
func (w *Writer) patchElement(n *html.Node, state *writerState, translateNo bool) {
	elemTranslateNo := translateNo
	if tv := getAttr(n, "translate"); tv != "" {
		if tv == "no" {
			elemTranslateNo = true
		} else if tv == "yes" {
			elemTranslateNo = false
		}
	}

	if nonTranslatableElements[n.DataAtom] {
		state.dataCounter++
		return
	}

	if n.DataAtom == atom.Meta {
		w.patchMetaTag(n, state)
		return
	}

	// lang/xml:lang → data counter (mirrors extractLangAttribute)
	lang := getAttr(n, "lang")
	if lang == "" {
		lang = getAttrNS(n, "xml", "lang")
	}
	if lang != "" {
		state.dataCounter++
	}

	// Translatable attributes (mirrors extractTranslatableAttributes)
	w.patchTranslatableAttributes(n, state, elemTranslateNo)

	if !isInlineElement(n) {
		if elemTranslateNo {
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				w.patchNode(child, state, elemTranslateNo)
			}
			return
		}

		hasBlockChildren := writerHasBlockLevelChildren(n)

		if hasBlockChildren {
			w.patchBlockWithMixedContent(n, state, elemTranslateNo)
			return
		}

		if writerHasAnyContent(n) || getAttr(n, "id") != "" {
			preserveWS := w.cfg.PreserveWhitespace || preserveWhitespaceElements[n.DataAtom]

			// Mirror the reader's collectInlineContent call: count spans
			// and collect text the same way to determine if a block is emitted.
			spanCounter := 0
			w.countInlineSpansFromNode(n, &spanCounter, elemTranslateNo, state)

			hasID := getAttr(n, "id") != ""
			text := collectPlainText(n, preserveWS)
			fragOK := text != "" || spanCounter > 0
			if fragOK || hasID {
				if text != "" || spanCounter > 0 || hasID {
					state.blockCounter++
					blockID := fmt.Sprintf("tu%d", state.blockCounter)
					if block, ok := state.blocks[blockID]; ok {
						w.replaceElementContent(n, block)
					}
					return
				}
			}
		}
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		w.patchNode(child, state, elemTranslateNo)
	}
}

// patchMetaTag mirrors handleMetaTag for ID assignment.
func (w *Writer) patchMetaTag(n *html.Node, state *writerState) {
	httpEquiv := strings.ToLower(getAttr(n, "http-equiv"))
	metaName := strings.ToLower(getAttr(n, "name"))
	content := getAttr(n, "content")
	charset := getAttr(n, "charset")

	if charset != "" {
		state.dataCounter++
		return
	}

	if httpEquiv == "content-type" && content != "" {
		if cs := extractCharset(content); cs != "" {
			state.dataCounter++
			return
		}
	}

	if httpEquiv == "content-language" && content != "" {
		state.dataCounter++
		return
	}

	if content != "" {
		isTranslatable := false
		if httpEquiv == "keywords" || translatableMetaNames[metaName] {
			isTranslatable = true
		}

		if isTranslatable {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			if block, ok := state.blocks[blockID]; ok {
				text := w.getBlockText(block)
				setAttr(n, "content", text)
			}
		}
	}

	state.dataCounter++
}

// patchTranslatableAttributes mirrors extractTranslatableAttributes for ID assignment.
func (w *Writer) patchTranslatableAttributes(n *html.Node, state *writerState, translateNo bool) {
	if translateNo {
		return
	}

	if title := getAttr(n, "title"); title != "" {
		state.blockCounter++
		blockID := fmt.Sprintf("tu%d", state.blockCounter)
		if block, ok := state.blocks[blockID]; ok {
			setAttr(n, "title", w.getBlockText(block))
		}
	}

	if alt := getAttr(n, "alt"); alt != "" {
		if n.DataAtom == atom.Img || n.DataAtom == atom.Input || n.DataAtom == atom.Area {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			if block, ok := state.blocks[blockID]; ok {
				setAttr(n, "alt", w.getBlockText(block))
			}
		}
	}

	if label := getAttr(n, "label"); label != "" {
		if n.DataAtom == atom.Option {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			if block, ok := state.blocks[blockID]; ok {
				setAttr(n, "label", w.getBlockText(block))
			}
		}
	}

	if ph := getAttr(n, "placeholder"); ph != "" {
		if n.DataAtom == atom.Input || n.DataAtom == atom.Textarea {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			if block, ok := state.blocks[blockID]; ok {
				setAttr(n, "placeholder", w.getBlockText(block))
			}
		}
	}

	if val := getAttr(n, "value"); val != "" && n.DataAtom == atom.Input {
		inputType := strings.ToLower(getAttr(n, "type"))
		if isTranslatableInputValue(inputType) {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			if block, ok := state.blocks[blockID]; ok {
				setAttr(n, "value", w.getBlockText(block))
			}
		}
	}
}

// patchBlockWithMixedContent mirrors processBlockWithMixedContent for ID assignment.
func (w *Writer) patchBlockWithMixedContent(n *html.Node, state *writerState, translateNo bool) {
	preserveWS := w.cfg.PreserveWhitespace || preserveWhitespaceElements[n.DataAtom]

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode || (child.Type == html.ElementNode && isInlineElement(child)) {
			// Collect inline run — mirror the reader's logic.
			var textBuf strings.Builder
			spanCounter := 0
			runStart := child
			for child != nil && (child.Type == html.TextNode ||
				child.Type == html.CommentNode ||
				(child.Type == html.ElementNode && isInlineElement(child))) {
				switch child.Type {
				case html.TextNode:
					textBuf.WriteString(child.Data)
				case html.CommentNode:
					spanCounter++
				case html.ElementNode:
					w.patchTranslatableAttributes(child, state, translateNo)
					w.countInlineSpansFromNode(child, &spanCounter, translateNo, state)
				}
				child = child.NextSibling
			}

			text := textBuf.String()
			if !preserveWS {
				text = collapseWhitespace(text)
				text = strings.TrimFunc(text, isHTMLWhitespace)
			}
			if text != "" || spanCounter > 0 {
				state.blockCounter++
				blockID := fmt.Sprintf("tu%d", state.blockCounter)
				if block, ok := state.blocks[blockID]; ok {
					w.replaceInlineRun(n, runStart, child, block)
				}
			}

			if child == nil {
				break
			}
			// Fall through to process this block-level child below.
		}

		w.patchNode(child, state, translateNo)
	}
}

// countInlineSpansFromNode counts spans inside a node's children,
// mirroring collectFromNode span counting, and advances attribute block counters.
func (w *Writer) countInlineSpansFromNode(n *html.Node, spanCounter *int, translateNo bool, state *writerState) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.CommentNode:
			*spanCounter++

		case html.ElementNode:
			// Mirror: extractTranslatableAttributes on inline elements inside blocks
			w.patchTranslatableAttributes(child, state, translateNo)

			if nonTranslatableElements[child.DataAtom] {
				*spanCounter++
				continue
			}

			childTranslateNo := translateNo
			if tv := getAttr(child, "translate"); tv != "" {
				if tv == "no" {
					childTranslateNo = true
				} else if tv == "yes" {
					childTranslateNo = false
				}
			}

			if isInlineElement(child) {
				if childTranslateNo && !translateNo && !hasDescendantTranslateYes(child) {
					*spanCounter++
					continue
				}

				if selfClosingElements[child.DataAtom] {
					*spanCounter++
				} else {
					*spanCounter++ // opening
					w.countInlineSpansFromNode(child, spanCounter, childTranslateNo, state)
					*spanCounter++ // closing
				}
			}
		}
	}
}

// replaceElementContent replaces a block element's children with translated content.
func (w *Writer) replaceElementContent(n *html.Node, block *model.Block) {
	text := w.getBlockText(block)

	// Remove existing children.
	for n.FirstChild != nil {
		n.RemoveChild(n.FirstChild)
	}

	// Parse the translated text (may contain HTML markup from spans) and add as children.
	nodes, err := html.ParseFragment(strings.NewReader(text), n)
	if err != nil {
		// Fallback: insert as plain text.
		n.AppendChild(&html.Node{Type: html.TextNode, Data: text})
		return
	}
	for _, child := range nodes {
		n.AppendChild(child)
	}
}

// replaceInlineRun replaces a run of inline nodes (from runStart up to but not
// including endNode) with translated content.
func (w *Writer) replaceInlineRun(parent *html.Node, runStart, endNode *html.Node, block *model.Block) {
	text := w.getBlockText(block)

	// Remove the inline run nodes.
	for runStart != nil && runStart != endNode {
		next := runStart.NextSibling
		parent.RemoveChild(runStart)
		runStart = next
	}

	// Parse translated content and insert before endNode.
	nodes, err := html.ParseFragment(strings.NewReader(text), parent)
	if err != nil {
		node := &html.Node{Type: html.TextNode, Data: text}
		parent.InsertBefore(node, endNode)
		return
	}
	for _, child := range nodes {
		parent.InsertBefore(child, endNode)
	}
}

// getBlockText returns the text content to write for a block.
func (w *Writer) getBlockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return w.getCodedText(block, w.Locale)
	}
	return w.getSourceCodedText(block)
}

// writeFallback writes blocks without original content (existing behavior).
func (w *Writer) writeFallback(blocks map[string]*model.Block) error {
	// We need ordered output, but map iteration is random.
	// Collect block IDs and sort them.
	type indexedBlock struct {
		idx   int
		block *model.Block
	}
	var ordered []indexedBlock
	for _, b := range blocks {
		var idx int
		if _, err := fmt.Sscanf(b.ID, "tu%d", &idx); err == nil {
			ordered = append(ordered, indexedBlock{idx: idx, block: b})
		}
	}
	// Sort by index.
	for i := range ordered {
		for j := i + 1; j < len(ordered); j++ {
			if ordered[j].idx < ordered[i].idx {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}

	for _, ob := range ordered {
		block := ob.block
		text := w.getBlockText(block)

		if block.Skeleton != nil && block.Skeleton.Strategy == model.SkeletonFragmentBased {
			for _, sp := range block.Skeleton.Parts {
				switch p := sp.(type) {
				case *model.SkeletonText:
					if _, err := fmt.Fprint(w.Output, p.Text); err != nil {
						return err
					}
				case *model.SkeletonRef:
					if _, err := fmt.Fprint(w.Output, text); err != nil {
						return err
					}
				}
			}
		} else {
			if _, err := fmt.Fprint(w.Output, text); err != nil {
				return err
			}
		}
	}
	return nil
}

// getCodedText reconstructs the full text from a block's target including span markup.
func (w *Writer) getCodedText(block *model.Block, locale model.LocaleID) string {
	segs := block.Targets[locale]
	if len(segs) == 0 {
		return w.getSourceCodedText(block)
	}
	var buf strings.Builder
	for _, seg := range segs {
		w.renderFragment(&buf, seg.Content)
	}
	return buf.String()
}

func (w *Writer) getSourceCodedText(block *model.Block) string {
	var buf strings.Builder
	for _, seg := range block.Source {
		w.renderFragment(&buf, seg.Content)
	}
	return buf.String()
}

func (w *Writer) renderFragment(buf *strings.Builder, frag *model.Fragment) {
	if !frag.HasSpans() {
		buf.WriteString(frag.CodedText)
		return
	}

	spanIdx := 0
	for _, r := range frag.CodedText {
		if model.MarkerOpening == r || model.MarkerClosing == r || model.MarkerPlaceholder == r {
			if spanIdx < len(frag.Spans) {
				buf.WriteString(frag.Spans[spanIdx].Data)
				spanIdx++
			}
		} else {
			buf.WriteRune(r)
		}
	}
}

// setAttr sets an attribute value on an HTML node, adding it if not present.
func setAttr(n *html.Node, key, val string) {
	for i, attr := range n.Attr {
		if attr.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

// writerHasBlockLevelChildren mirrors Reader.hasBlockLevelChildren.
func writerHasBlockLevelChildren(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && !isInlineElement(child) && !nonTranslatableElements[child.DataAtom] {
			return true
		}
	}
	return false
}

// writerHasAnyContent mirrors Reader.hasAnyContent.
func writerHasAnyContent(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			if strings.TrimFunc(child.Data, isHTMLWhitespace) != "" {
				return true
			}
		}
		if child.Type == html.ElementNode {
			if isInlineElement(child) {
				if selfClosingElements[child.DataAtom] {
					return true
				}
				if writerHasAnyContent(child) {
					return true
				}
			}
		}
	}
	return false
}

// collectPlainText collects plain text from a node's children (for the writer's
// block-emission check), without building spans.
func collectPlainText(n *html.Node, preserveWS bool) string {
	var buf strings.Builder
	collectPlainTextRecur(n, &buf)
	text := buf.String()
	if !preserveWS {
		text = collapseWhitespace(text)
		text = strings.TrimFunc(text, isHTMLWhitespace)
	}
	return text
}

func collectPlainTextRecur(n *html.Node, buf *strings.Builder) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			buf.WriteString(child.Data)
		} else if child.Type == html.ElementNode && isInlineElement(child) {
			collectPlainTextRecur(child, buf)
		}
	}
}
