package xml

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for XML files.
type Writer struct {
	format.BaseFormatWriter
	resolver      format.SubfilterResolver
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SubfilterAware and SkeletonStoreConsumer.
var _ format.SubfilterAware = (*Writer)(nil)
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new XML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xml",
		},
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format writers.
func (w *Writer) SetSubfilterResolver(resolver format.SubfilterResolver) {
	w.resolver = resolver
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed XML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeletonStore(ctx, parts)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if part.Type == model.PartBlock {
				block, ok := part.Resource.(*model.Block)
				if !ok {
					continue
				}
				text := w.blockText(block)
				if _, err := fmt.Fprint(w.Output, text); err != nil {
					return err
				}
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok && layer.IsEmbedded() {
					val, err := w.writeChildLayer(ctx, layer, parts)
					if err != nil {
						return fmt.Errorf("xml: writing child layer %s: %w", layer.Name, err)
					}
					if _, err := fmt.Fprint(w.Output, val); err != nil {
						return err
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
		return fmt.Errorf("xml writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
// This produces byte-exact output — only translated text differs from the original.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("xml writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.renderBlockXML(block)
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// renderBlockXML renders a block's text for XML output. Text parts are XML-escaped
// while inline span markup (from span Data) is written as-is since it's already valid XML.
func (w *Writer) renderBlockXML(block *model.Block) string {
	segs := block.Source
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs = block.Targets[w.Locale]
	}
	var buf strings.Builder
	for _, seg := range segs {
		w.renderFragmentXML(&buf, seg.Content)
	}
	return buf.String()
}

// renderFragmentXML writes a fragment with XML-escaped text parts and raw span Data.
func (w *Writer) renderFragmentXML(buf *strings.Builder, frag *model.Fragment) {
	if frag == nil {
		return
	}
	if !frag.HasSpans() {
		buf.WriteString(xmlEscapeString(frag.CodedText))
		return
	}

	spanIdx := 0
	var textBuf strings.Builder
	for _, r := range frag.CodedText {
		if r == model.MarkerOpening || r == model.MarkerClosing || r == model.MarkerPlaceholder {
			// Flush accumulated text (XML-escaped).
			if textBuf.Len() > 0 {
				buf.WriteString(xmlEscapeString(textBuf.String()))
				textBuf.Reset()
			}
			// Write span data as-is (already valid XML).
			if spanIdx < len(frag.Spans) {
				buf.WriteString(frag.Spans[spanIdx].Data)
				spanIdx++
			}
		} else {
			textBuf.WriteRune(r)
		}
	}
	// Flush remaining text.
	if textBuf.Len() > 0 {
		buf.WriteString(xmlEscapeString(textBuf.String()))
	}
}

// xmlEscapeString escapes the five mandatory XML special characters (&, <, >, ", ')
// but preserves whitespace (newlines, tabs) for byte-exact skeleton roundtrip.
func xmlEscapeString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// writeChildLayer collects parts until the matching PartLayerEnd and writes them
// through the appropriate sub-format writer.
func (w *Writer) writeChildLayer(ctx context.Context, layer *model.Layer, parts <-chan *model.Part) (string, error) {
	var childParts []*model.Part
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return "", fmt.Errorf("unexpected end of parts stream in child layer %s", layer.ID)
			}
			if part.Type == model.PartLayerEnd {
				if endLayer, ok := part.Resource.(*model.Layer); ok && endLayer.ID == layer.ID {
					goto collected
				}
			}
			childParts = append(childParts, part)
		}
	}

collected:
	if w.resolver == nil {
		return w.fallbackChildText(childParts), nil
	}

	subWriter, err := w.resolver.ResolveWriter(layer.Format)
	if err != nil {
		return w.fallbackChildText(childParts), nil
	}

	var buf bytes.Buffer
	if err := subWriter.SetOutputWriter(&buf); err != nil {
		return "", err
	}
	subWriter.SetLocale(w.Locale)

	childCh := make(chan *model.Part, len(childParts))
	for _, p := range childParts {
		childCh <- p
	}
	close(childCh)

	if err := subWriter.Write(ctx, childCh); err != nil {
		return "", err
	}
	if err := subWriter.Close(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// fallbackChildText concatenates block texts when no sub-writer is available.
func (w *Writer) fallbackChildText(parts []*model.Part) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if block, ok := p.Resource.(*model.Block); ok {
				sb.WriteString(w.blockText(block))
			}
		}
	}
	return sb.String()
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return w.renderSegments(block.Targets[w.Locale])
	}
	return w.renderSegments(block.Source)
}

// renderSegments reconstructs text from segments, restoring inline span markup from span Data.
func (w *Writer) renderSegments(segs []*model.Segment) string {
	var buf strings.Builder
	for _, seg := range segs {
		w.renderFragment(&buf, seg.Content)
	}
	return buf.String()
}

// renderFragment writes a fragment's coded text, replacing span markers with their raw Data.
func (w *Writer) renderFragment(buf *strings.Builder, frag *model.Fragment) {
	if frag == nil {
		return
	}
	if !frag.HasSpans() {
		buf.WriteString(frag.CodedText)
		return
	}

	spanIdx := 0
	for _, r := range frag.CodedText {
		if r == model.MarkerOpening || r == model.MarkerClosing || r == model.MarkerPlaceholder {
			if spanIdx < len(frag.Spans) {
				buf.WriteString(frag.Spans[spanIdx].Data)
				spanIdx++
			}
		} else {
			buf.WriteRune(r)
		}
	}
}
