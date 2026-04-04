package html

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/net/html"
)

// Writer implements DataFormatWriter for HTML files.
type Writer struct {
	format.BaseFormatWriter
	sourcePath      string
	originalContent []byte
	skeletonStore   *format.SkeletonStore
	cfg             *Config
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
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
	// Collect all blocks keyed by ID and capture source locale.
	blocks := make(map[string]*model.Block)
	var sourceLocale model.LocaleID
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			switch part.Type {
			case model.PartBlock:
				if b, ok := part.Resource.(*model.Block); ok {
					blocks[b.ID] = b
				}
			case model.PartLayerStart:
				if l, ok := part.Resource.(*model.Layer); ok && !l.Locale.IsEmpty() {
					sourceLocale = l.Locale
				}
			}
		}
	}
done:

	// If a target locale is set and differs from source, buffer output
	// so we can rewrite lang/xml:lang attributes to the target locale.
	needsLangRewrite := !w.Locale.IsEmpty() && !sourceLocale.IsEmpty() && w.Locale != sourceLocale
	var langBuf bytes.Buffer
	origOutput := w.Output
	if needsLangRewrite {
		w.Output = &langBuf
	}

	// Mode 1: Skeleton store (optimal, byte-exact).
	var writeErr error
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("html writer: flush skeleton: %w", err)
		}
		writeErr = w.writeFromSkeleton(w.skeletonStore, blocks)
	} else if content, err := w.loadOriginalContent(); err != nil {
		return err
	} else if content != nil {
		// Mode 2: Re-parse original content.
		writeErr = w.writeReparse(content, blocks)
	} else {
		// Mode 3: Block-only output (minimal fallback).
		writeErr = w.writeFallback(blocks)
	}

	if writeErr != nil {
		if needsLangRewrite {
			w.Output = origOutput
		}
		return writeErr
	}

	// Post-process: rewrite lang attributes from source to target locale.
	if needsLangRewrite {
		w.Output = origOutput
		result := rewriteLangAttrs(langBuf.Bytes(), sourceLocale, w.Locale)
		_, err := w.Output.Write(result)
		return err
	}
	return nil
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

// writeFromSkeleton reads skeleton entries and fills in block content.
// This produces byte-exact output — only translated text differs from the original.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("html writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.getBlockText(block)
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeReparse re-parses the original HTML, patches translations, and renders.
func (w *Writer) writeReparse(content []byte, blocks map[string]*model.Block) error {
	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return fmt.Errorf("html writer: parse original: %w", err)
	}

	visitor := &writerVisitor{writer: w, blocks: blocks}
	walker := newDOMWalker(w.cfg, visitor)
	walker.walk(doc)

	return html.Render(w.Output, doc)
}

// writerVisitor implements walkVisitor for the writer, patching DOM nodes
// with translated content.
type writerVisitor struct {
	writer *Writer
	blocks map[string]*model.Block
}

func (v *writerVisitor) onData(dataID string, n *html.Node, dataName string, props map[string]string) {
	// No-op: structural elements are preserved as-is in the DOM.
}

func (v *writerVisitor) onTextBlock(blockID string, n *html.Node) {
	if block, ok := v.blocks[blockID]; ok {
		n.Data = v.writer.getBlockText(block)
	}
}

func (v *writerVisitor) onAttributeBlock(blockID string, n *html.Node, attrKey string) {
	if block, ok := v.blocks[blockID]; ok {
		setAttr(n, attrKey, v.writer.getBlockText(block))
	}
}

func (v *writerVisitor) onMetaBlock(blockID string, n *html.Node) {
	if block, ok := v.blocks[blockID]; ok {
		setAttr(n, "content", v.writer.getBlockText(block))
	}
}

func (v *writerVisitor) onBlockElement(blockID string, n *html.Node, preserveWS bool) {
	if block, ok := v.blocks[blockID]; ok {
		v.replaceElementContent(n, block)
	}
}

func (v *writerVisitor) onMixedContentBlock(blockID string, parent *html.Node, runStart, runEnd *html.Node, preserveWS bool) {
	if block, ok := v.blocks[blockID]; ok {
		v.replaceInlineRun(parent, runStart, runEnd, block)
	}
}

// replaceElementContent replaces a block element's children with translated content.
func (v *writerVisitor) replaceElementContent(n *html.Node, block *model.Block) {
	text := v.writer.getBlockText(block)

	for n.FirstChild != nil {
		n.RemoveChild(n.FirstChild)
	}

	nodes, err := html.ParseFragment(strings.NewReader(text), n)
	if err != nil {
		n.AppendChild(&html.Node{Type: html.TextNode, Data: text})
		return
	}
	for _, child := range nodes {
		n.AppendChild(child)
	}
}

// replaceInlineRun replaces a run of inline nodes with translated content.
func (v *writerVisitor) replaceInlineRun(parent *html.Node, runStart, runEnd *html.Node, block *model.Block) {
	text := v.writer.getBlockText(block)

	for runStart != nil && runStart != runEnd {
		next := runStart.NextSibling
		parent.RemoveChild(runStart)
		runStart = next
	}

	nodes, err := html.ParseFragment(strings.NewReader(text), parent)
	if err != nil {
		node := &html.Node{Type: html.TextNode, Data: text}
		parent.InsertBefore(node, runEnd)
		return
	}
	for _, child := range nodes {
		parent.InsertBefore(child, runEnd)
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

// collectPlainText collects plain text from a node's children (for the walker's
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

// rewriteLangAttrs replaces lang/xml:lang attribute values that match the
// source locale with the target locale. This mirrors Okapi's behavior: when
// producing a translated document, the language declaration should reflect
// the output language.
func rewriteLangAttrs(data []byte, srcLocale, tgtLocale model.LocaleID) []byte {
	src := string(srcLocale)
	tgt := string(tgtLocale)

	// Build a regex that matches lang="<srcLocale>" or xml:lang="<srcLocale>"
	// with either double or single quotes, case-insensitive on the attribute name.
	// The locale value is matched case-insensitively too.
	pattern := `(?i)((?:xml:)?lang\s*=\s*)(["'])` + regexp.QuoteMeta(src) + `(["'])`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return data
	}

	return re.ReplaceAll(data, []byte(`${1}${2}`+tgt+`${3}`))
}
