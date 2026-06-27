package splicedlines

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for line-spliced text files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	firstEntry    bool
}

// Ensure Writer implements SkeletonStoreConsumer and StreamingWriter.
var (
	_ format.SkeletonStoreConsumer = (*Writer)(nil)
	_ format.StreamingWriter       = (*Writer)(nil)
)

// StreamingWriter marks this writer as able to consume a streaming skeleton
// interleaved with the Part stream (see Write → StreamSkeletonWrite). See [AD-005].
func (w *Writer) StreamingWriter() bool { return true }

// NewWriter creates a new spliced lines writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "splicedlines",
		},
		firstEntry: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed spliced lines.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		if w.skeletonStore.IsStreaming() {
			return format.StreamSkeletonWrite(ctx, w.skeletonStore, parts, w.Output, w.renderRef, nil)
		}
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
		return fmt.Errorf("splicedlines writer: flush skeleton: %w", err)
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
			return fmt.Errorf("splicedlines writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			data, err := w.renderRef(blocks[string(entry.Data)])
			if err != nil {
				return err
			}
			if _, err := w.Output.Write(data); err != nil {
				return err
			}
		}
	}
	return nil
}

// renderRef returns the bytes a SkeletonRef contributes for the given block,
// shared by the buffered (writeFromSkeleton) and streaming (StreamSkeletonWrite)
// paths so both produce identical output. It re-adds the backslash continuations
// (using the block's stored continuation endings) and the trailing-splicer byte
// the reader stripped, for a byte-exact round-trip. A nil block contributes
// nothing, matching the buffered path's map miss.
func (w *Writer) renderRef(block *model.Block) ([]byte, error) {
	if block == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	text := w.blockText(block)
	// Re-add backslash continuations for multi-line blocks.
	lines := strings.Split(text, "\n")
	if len(lines) > 1 {
		// Use stored continuation endings if available.
		endings := w.continuationEndings(block, len(lines)-1)
		for i, line := range lines {
			if i < len(lines)-1 {
				ending := "\n"
				if i < len(endings) {
					ending = endings[i]
				}
				fmt.Fprintf(&buf, "%s\\%s", line, ending)
			} else {
				buf.WriteString(line)
			}
		}
	} else {
		buf.WriteString(text)
	}
	// Re-emit the trailing `\` byte for blocks that ended the file
	// mid-continuation; the reader strips it from the block's logical text but
	// tags the block so we can restore byte-exact output here.
	if block.Properties["trailing-splicer"] == "true" {
		buf.WriteString(`\`)
	}
	return buf.Bytes(), nil
}

// continuationEndings extracts stored continuation line endings from block properties.
func (w *Writer) continuationEndings(block *model.Block, count int) []string {
	raw, ok := block.Properties["continuation-endings"]
	if !ok {
		return nil
	}
	return strings.SplitN(raw, "|", count)
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		return w.writeBlock(part)
	case model.PartData:
		return w.writeData()
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("splicedlines writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	if !w.firstEntry {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstEntry = false

	// If text contains newlines, re-add backslash continuations
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i < len(lines)-1 {
			if _, err := fmt.Fprintf(w.Output, "%s\\\n", line); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprint(w.Output, line); err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *Writer) writeData() error {
	if !w.firstEntry {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstEntry = false
	return nil
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}
