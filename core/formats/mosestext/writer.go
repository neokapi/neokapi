package mosestext

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Moses Text files.
// Each Block is written as a single line.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	firstBlock    bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var (
	_ format.SkeletonStoreConsumer = (*Writer)(nil)
	_ format.StreamingWriter       = (*Writer)(nil)
)

// StreamingWriter marks this writer as able to consume a streaming skeleton
// interleaved with the Part stream (Write → StreamSkeletonWrite). See [AD-005].
func (w *Writer) StreamingWriter() bool { return true }

// NewWriter creates a new Moses Text writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName:  "mosestext",
			Interchange: true,
		},
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed Moses Text.
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
		return errors.New("mosestext writer: expected Block resource")
	}

	// Use target text if available, otherwise source text. Goes
	// through blockText so inline-code Ph runs get spliced back in.
	text := w.blockText(block)

	if !w.firstBlock {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstBlock = false

	_, err := fmt.Fprint(w.Output, text)
	return err
}

func (w *Writer) writeData(part *model.Part) error {
	// Data parts represent empty lines
	if !w.firstBlock {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstBlock = false
	return nil
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
		return fmt.Errorf("mosestext writer: flush skeleton: %w", err)
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
			return fmt.Errorf("mosestext writer: read skeleton: %w", err)
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
// shared by the buffered and streaming skeleton paths so both produce identical
// output. A nil block contributes nothing, matching the buffered path's map miss.
func (w *Writer) renderRef(block *model.Block) ([]byte, error) {
	if block == nil {
		return nil, nil
	}
	return []byte(w.blockText(block)), nil
}

func (w *Writer) blockText(block *model.Block) string {
	// Moses InlineText-decoded blocks (the reader's default mode) carry
	// the encode marker: their bodies were decoded from pseudo-XLIFF
	// (entities, <lb/>, <g>/<x> codes) on read, so the writer re-encodes
	// them to pseudo-XLIFF for a byte-exact round trip, exactly as
	// Okapi's MosesTextEncoder does. Code-finder blocks omit the marker
	// and are rendered verbatim via RenderRunsWithData (which splices
	// Ph-run Data back in) — plain SourceText/TargetText drops Ph runs so
	// the inline codes would otherwise vanish on round-trip.
	render := model.RenderRunsWithData
	if block.Properties[propEncode] == encodeInlineTextValue {
		render = encodeInlineText
	}

	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return render(block.TargetRuns(w.Locale))
	}
	return render(block.Source)
}
