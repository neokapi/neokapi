package wiki

import (
	"context"
	"fmt"
	"io"
	"regexp"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Wiki files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	firstBlock    bool
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new wiki writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "wiki",
		},
		cfg:        cfg,
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Config returns the writer's configuration.
func (w *Writer) Config() *Config { return w.cfg }

// Write consumes Parts from a channel and writes reconstructed wiki markup.
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
		return fmt.Errorf("wiki writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("wiki writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				// Reconstruct using block name for headers
				if block.Name == "header" {
					raw := block.Properties["raw"]
					if raw != "" {
						// Extract header level from raw line
						if err := w.writeHeaderFromRaw(text, raw); err != nil {
							return err
						}
						continue
					}
				}
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeHeaderFromRaw reconstructs a wiki header line from the original raw line.
func (w *Writer) writeHeaderFromRaw(text, raw string) error {
	// Extract the = delimiters from the raw line
	m := mediaWikiHeaderReWriter.FindStringSubmatch(raw)
	if m == nil {
		_, err := io.WriteString(w.Output, text)
		return err
	}
	_, err := fmt.Fprintf(w.Output, "%s %s %s", m[1], text, m[3])
	return err
}

var mediaWikiHeaderReWriter = regexp.MustCompile(`^(={2,6})\s*(.+?)\s*(={2,6})\s*$`)

// blockText returns target or source text for a block.
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
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
		return fmt.Errorf("wiki writer: expected Block resource")
	}

	// Use target text if available, otherwise source text
	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	if !w.firstBlock {
		if _, err := fmt.Fprintln(w.Output); err != nil {
			return err
		}
	}
	w.firstBlock = false

	// Reconstruct wiki markup based on block name
	switch block.Name {
	case "header":
		_, err := fmt.Fprintf(w.Output, "== %s ==", text)
		return err
	case "table-header":
		_, err := fmt.Fprintf(w.Output, "! %s", text)
		return err
	case "table-cell":
		_, err := fmt.Fprintf(w.Output, "| %s", text)
		return err
	case "image-caption":
		// Captions are complex to reconstruct; write as plain text
		_, err := fmt.Fprint(w.Output, text)
		return err
	default:
		_, err := fmt.Fprint(w.Output, text)
		return err
	}
}

func (w *Writer) writeData(part *model.Part) error {
	// Data parts represent structural separators (blank lines, table markers, etc.)
	if !w.firstBlock {
		if _, err := fmt.Fprintln(w.Output); err != nil {
			return err
		}
	}
	w.firstBlock = false
	return nil
}
