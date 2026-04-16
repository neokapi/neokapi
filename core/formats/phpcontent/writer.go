package phpcontent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for PHP content files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new PHP content writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "phpcontent",
		},
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed PHP content.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeleton(ctx, parts)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if err := w.writePart(part); err != nil {
				return err
			}
		}
	}
}

// writeWithSkeleton collects all blocks, then reconstructs output from skeleton entries.
func (w *Writer) writeWithSkeleton(ctx context.Context, parts <-chan *model.Part) error {
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
		return fmt.Errorf("phpcontent writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("phpcontent writer: read skeleton: %w", err)
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

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		return w.writeBlock(part)
	case model.PartData:
		return w.writeData(part)
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("phpcontent writer: expected Block resource")
	}

	// Get the text to write - use target if available, otherwise source
	text := w.blockText(block)

	// Write the string value
	if _, err := fmt.Fprint(w.Output, text); err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return errors.New("phpcontent writer: expected Data resource")
	}

	// Write code, comments, and structural elements as-is
	if code, ok := data.Properties["code"]; ok {
		if _, err := fmt.Fprint(w.Output, code); err != nil {
			return err
		}
	}
	if comment, ok := data.Properties["comment"]; ok {
		if _, err := fmt.Fprint(w.Output, comment); err != nil {
			return err
		}
	}
	if idx, ok := data.Properties["arrayIndex"]; ok {
		if _, err := fmt.Fprint(w.Output, idx); err != nil {
			return err
		}
	}
	if skipped, ok := data.Properties["skipped"]; ok {
		if _, err := fmt.Fprint(w.Output, skipped); err != nil {
			return err
		}
	}

	return nil
}

// blockText returns the text to write for a block, expanding inline codes.
func (w *Writer) blockText(block *model.Block) string {
	var frag *model.Fragment
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs := block.Targets[w.Locale]
		if len(segs) > 0 {
			frag = segs[0].Fragment()
		}
	}
	if frag == nil && len(block.Source) > 0 {
		frag = block.Source[0].Fragment()
	}
	if frag == nil {
		return ""
	}

	// If no spans, just return the text
	if !frag.HasSpans() {
		return frag.CodedText
	}

	// Expand coded text: replace markers with span data
	var buf strings.Builder
	spanIdx := 0
	for _, r := range frag.CodedText {
		if isMarker(r) && spanIdx < len(frag.Spans) {
			buf.WriteString(frag.Spans[spanIdx].Data)
			spanIdx++
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// isMarker returns true if the rune is a span marker character.
func isMarker(r rune) bool {
	return r >= '\uE001' && r <= '\uE003'
}
