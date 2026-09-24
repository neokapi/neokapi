package messageformat

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for ICU MessageFormat files.
type Writer struct {
	format.BaseFormatWriter
	firstLine     bool
	skeletonStore *format.SkeletonStore

	// blocks stores blocks by path for reconstruction.
	blocks map[string]*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer and StreamingWriter.
var (
	_ format.SkeletonStoreConsumer = (*Writer)(nil)
	_ format.StreamingWriter       = (*Writer)(nil)
)

// StreamingWriter marks this writer as able to consume a streaming skeleton
// interleaved with the Part stream (Write → StreamSkeletonWrite), so a
// messageformat round-trip stays bounded-memory when paired with the streaming
// reader. Output is byte-identical to the buffered skeleton path.
func (w *Writer) StreamingWriter() {}

// NewWriter creates a new MessageFormat writer.
func NewWriter() *Writer {
	return &Writer{
		FormatName: "messageformat",
		firstLine:  true,
		blocks:     make(map[string]*model.Block),
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed MessageFormat.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		if w.skeletonStore.IsStreaming() {
			// Interleave: pull each referenced block from the Part stream on
			// demand rather than buffering the whole block map, so the round-trip
			// stays bounded-memory. Byte-identical to writeFromSkeleton.
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
		return fmt.Errorf("messageformat writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	return format.BufferedSkeletonWrite(w.skeletonStore, blocks, w.Output, w.renderRef, nil)
}

// renderRef returns the bytes a SkeletonRef contributes for the given block,
// shared by the buffered and streaming skeleton paths so both produce identical
// output. A nil block contributes nothing, matching the buffered path's map miss.
func (w *Writer) renderRef(block *model.Block) ([]byte, error) {
	if block == nil {
		return nil, nil
	}
	text := w.getBlockText(block)
	if err := checkPattern(block, text); err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// checkPattern refuses a value that is not a MessageFormat pattern.
//
// Inline argument runs are rendered with their original tokens before parsing.
// A malformed target is rejected before its bytes reach the output.
func checkPattern(block *model.Block, text string) error {
	if _, err := parse(text); err != nil {
		name := block.ID
		if block.Name != "" {
			name = fmt.Sprintf("%s (%s)", block.ID, block.Name)
		}
		return fmt.Errorf("messageformat writer: block %s is not a valid pattern: %w", name, err)
	}
	return nil
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		return w.writeBlock(part)
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("messageformat writer: expected Block resource")
	}

	text := w.getBlockText(block)
	if err := checkPattern(block, text); err != nil {
		return err
	}

	if !w.firstLine {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstLine = false

	_, err := fmt.Fprint(w.Output, text)
	return err
}

// getBlockText returns the appropriate text from a block, preferring target
// text when a locale is set.
func (w *Writer) getBlockText(block *model.Block) string {
	runs := block.Source
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		runs = block.Target(w.Locale).Runs
	}
	text := model.RenderRunsWithData(runs)
	if raw, ok := format.VerbatimFor(block, "messageformat.raw", text); ok {
		return raw
	}
	return text
}
