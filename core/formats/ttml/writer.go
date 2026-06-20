package ttml

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for TTML subtitle files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	docContent    string // original document content for skeleton-based reconstruction
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new TTML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "ttml",
		},
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed TTML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeletonStore(ctx, parts)
	}

	// Collect all parts first so we can do skeleton-based reconstruction.
	var blocks []*model.Block
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.writeOutput(blocks)
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					blocks = append(blocks, block)
				}
			case model.PartData:
				if data, ok := part.Resource.(*model.Data); ok {
					if data.Name == "ttml-document" {
						w.docContent = data.Properties["content"]
					}
				}
			}
		}
	}
}

// writeWithSkeletonStore collects all blocks, then reconstructs output from skeleton entries.
func (w *Writer) writeWithSkeletonStore(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
				}
			}
		}
	}
done:
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("ttml writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeletonStore(blocksByID)
}

// writeFromSkeletonStore reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeletonStore(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("ttml writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}

// writeOutput reconstructs the TTML document, replacing <p> element text
// with translated content where available.
func (w *Writer) writeOutput(blocks []*model.Block) error {
	// Non-translatable head-metadata blocks (ttm:copyright, ttm:agent) are
	// surfaced for ingestion only; their bytes are preserved verbatim in the
	// original document Data. This content-replacement writer only swaps <p>
	// caption text and matches blocks to <p> elements by index, so restrict the
	// matched set to the translatable captions.
	captions := make([]*model.Block, 0, len(blocks))
	for _, b := range blocks {
		if b.Translatable {
			captions = append(captions, b)
		}
	}

	if w.docContent == "" {
		// No skeleton: generate minimal TTML from blocks
		return w.writeMinimalTTML(captions)
	}

	// Parse the original document and replace <p> text content with
	// translated text from the blocks.
	return w.writeFromSkeleton(captions)
}

// writeFromSkeleton reconstructs the TTML from the original XML structure,
// replacing <p> element content with translated text.
func (w *Writer) writeFromSkeleton(blocks []*model.Block) error {
	blockIndex := 0

	// We need to track whether we are inside a <p> element so we can
	// replace its content. Rather than trying to manipulate the XML token
	// stream precisely (which is fragile with namespaces), we use a
	// string-replacement approach: find each <p>...</p> in the original
	// and replace the text content.
	output := w.docContent

	// Walk through the XML to find <p> elements and build replacements.
	type pElement struct {
		startOffset int
		endOffset   int
		text        string
	}
	var pElements []pElement

	// Track position using a simplified approach: re-read to find <p> tags.
	reader := strings.NewReader(w.docContent)
	decoder := xml.NewDecoder(reader)

	inP := false
	pDepth := 0
	var currentTextParts []string
	pStartCharOffset := int64(0)

	for {
		offset := decoder.InputOffset()
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" {
				inP = true
				pDepth = 1
				currentTextParts = nil
				pStartCharOffset = offset
			} else if inP {
				pDepth++
			}
		case xml.EndElement:
			if inP {
				pDepth--
				if pDepth == 0 {
					inP = false
					endOffset := decoder.InputOffset()
					pElements = append(pElements, pElement{
						startOffset: int(pStartCharOffset),
						endOffset:   int(endOffset),
						text:        strings.Join(currentTextParts, ""),
					})
				}
			}
		case xml.CharData:
			if inP {
				currentTextParts = append(currentTextParts, string(t))
			}
		}
	}

	// Now replace each <p> element's content with the block text.
	// Work backwards to preserve offsets.
	for i := len(pElements) - 1; i >= 0; i-- {
		pe := pElements[i]
		bi := i
		if bi >= len(blocks) {
			continue // no block for this <p>
		}
		// Skip empty blocks that were filtered out during reading
		for bi < len(blocks) && blockIndex <= bi {
			break
		}
		if bi >= len(blocks) {
			continue
		}

		block := blocks[bi]
		text := block.SourceText()
		if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
			text = block.TargetText(w.Locale)
		}

		// Find the content between the <p ...> opening tag end and the </p> closing tag start.
		segment := output[pe.startOffset:pe.endOffset]
		openEnd := strings.Index(segment, ">")
		closeStart := strings.LastIndex(segment, "</p>")
		if openEnd >= 0 && closeStart > openEnd {
			before := output[:pe.startOffset+openEnd+1]
			after := output[pe.startOffset+closeStart:]
			output = before + text + after
		}
	}

	_, err := io.WriteString(w.Output, output)
	return err
}

// writeMinimalTTML generates a minimal TTML document from blocks only.
func (w *Writer) writeMinimalTTML(blocks []*model.Block) error {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">` + "\n")
	sb.WriteString("  <body>\n    <div>\n")

	for _, block := range blocks {
		text := block.SourceText()
		if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
			text = block.TargetText(w.Locale)
		}

		begin := block.Properties["begin"]
		end := block.Properties["end"]

		sb.WriteString(fmt.Sprintf(`      <p begin="%s" end="%s">%s</p>`,
			xmlEscape(begin), xmlEscape(end), xmlEscape(text)))
		sb.WriteString("\n")
	}

	sb.WriteString("    </div>\n  </body>\n</tt>\n")

	_, err := io.WriteString(w.Output, sb.String())
	return err
}

// xmlEscape escapes special XML characters.
func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
