package plaintext

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for plain text files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	firstBlock    bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new plain text writer.
func NewWriter() *Writer {
	return &Writer{
		FormatName: "plaintext",
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed plain text.
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
		return fmt.Errorf("plaintext writer: flush skeleton: %w", err)
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
			return fmt.Errorf("plaintext writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.renderText(block)
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
		// Skip layer start/end and other structural parts
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("plaintext writer: expected Block resource")
	}

	text := w.renderText(block)

	if !w.firstBlock {
		if _, err := fmt.Fprintln(w.Output); err != nil {
			return err
		}
	}
	w.firstBlock = false

	_, err := fmt.Fprint(w.Output, text)
	return err
}

func (w *Writer) writeData(part *model.Part) error {
	// Data parts in plaintext represent empty lines
	if !w.firstBlock {
		if _, err := fmt.Fprintln(w.Output); err != nil {
			return err
		}
	}
	w.firstBlock = false
	return nil
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}

// renderText returns the bytes a block contributes to the output. For
// blocks produced by splice mode (marked by the reader with splice
// properties) it re-adds the continuation markers — using the stored
// per-line endings — and the trailing-splicer marker the reader
// stripped, so the round-trip is byte-exact. Other blocks render their
// text verbatim.
func (w *Writer) renderText(block *model.Block) string {
	text := w.blockText(block)
	marker := block.Properties["splice-marker"]
	if marker == "" {
		// Paragraph-mode blocks may carry non-LF internal line endings
		// (e.g. CRLF) recorded by the reader; restore them byte-exact.
		if raw := block.Properties["line-endings"]; raw != "" {
			lines := strings.Split(text, "\n")
			endings := strings.SplitN(raw, "|", len(lines)-1)
			var sb strings.Builder
			for i, line := range lines {
				sb.WriteString(line)
				if i < len(lines)-1 {
					if i < len(endings) && endings[i] != "" {
						sb.WriteString(endings[i])
					} else {
						sb.WriteString("\n")
					}
				}
			}
			return sb.String()
		}
		return text
	}

	var sb strings.Builder
	lines := strings.Split(text, "\n")
	var endings []string
	if raw := block.Properties["continuation-endings"]; raw != "" {
		endings = strings.SplitN(raw, "|", len(lines)-1)
	}
	for i, line := range lines {
		if i < len(lines)-1 {
			ending := "\n"
			if i < len(endings) && endings[i] != "" {
				ending = endings[i]
			}
			sb.WriteString(line)
			sb.WriteString(marker)
			sb.WriteString(ending)
		} else {
			sb.WriteString(line)
		}
	}
	// Re-emit the trailing marker for blocks that ended the input
	// mid-continuation; the reader strips it from the block's logical
	// text but tags the block so output stays byte-exact.
	if block.Properties["trailing-splicer"] == "true" {
		sb.WriteString(marker)
	}
	return sb.String()
}
