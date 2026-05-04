package tex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for TeX/LaTeX files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	firstPart     bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new TeX/LaTeX writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "tex",
		},
		firstPart: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed TeX.
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
		return errors.New("tex writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Reconstruct TeX structure based on block type
	switch block.Type {
	case "section", "subsection", "subsubsection", "chapter", "part",
		"paragraph", "subparagraph", "caption":
		if !w.firstPart {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintf(w.Output, "\\%s{%s}", block.Type, text)
		w.firstPart = false
		return err
	case "title", "author", "date":
		_, err := fmt.Fprintf(w.Output, "\\%s{%s}", block.Type, text)
		w.firstPart = false
		return err
	default:
		// Regular paragraph
		if !w.firstPart {
			if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
				return err
			}
		}
		w.firstPart = false
		_, err := io.WriteString(w.Output, text)
		return err
	}
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return errors.New("tex writer: expected Data resource")
	}

	content := ""
	if data.Properties != nil {
		content = data.Properties["content"]
	}

	if content != "" {
		if !w.firstPart {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		w.firstPart = false
		_, err := io.WriteString(w.Output, content)
		return err
	}
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
		return fmt.Errorf("tex writer: flush skeleton: %w", err)
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
			return fmt.Errorf("tex writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockTextForSkeleton(block)
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *Writer) blockTextForSkeleton(block *model.Block) string {
	raw := block.Properties["tex.rawSource"]

	// If no translation, use the raw source bytes for byte-exact output
	if w.Locale.IsEmpty() || !block.HasTarget(w.Locale) {
		if raw != "" {
			return raw
		}
		return block.SourceText()
	}

	// Render via RenderRunsWithData so embedded inline-code Ph runs
	// (e.g. \LaTeX inside \title{Installing \LaTeX}) emit their captured
	// raw TeX bytes verbatim instead of being lost via the plain-text
	// flatten path.
	text := model.RenderRunsWithData(block.TargetRuns(w.Locale))

	// For typed blocks (section, title, etc.), reconstruct the TeX command.
	// Preserve any leading whitespace from the raw source.
	switch block.Type {
	case "section", "subsection", "subsubsection", "chapter", "part",
		"paragraph", "subparagraph", "caption", "title", "author", "date":
		prefix := extractLeadingWhitespace(raw)
		return prefix + fmt.Sprintf("\\%s{%s}", block.Type, text)
	default:
		// For regular paragraph blocks, preserve leading whitespace from raw source
		prefix := extractLeadingWhitespace(raw)
		return prefix + text
	}
}

// extractLeadingWhitespace returns the leading whitespace from a string.
func extractLeadingWhitespace(s string) string {
	trimmed := strings.TrimLeft(s, " \t\n\r")
	return s[:len(s)-len(trimmed)]
}
