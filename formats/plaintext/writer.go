package plaintext

import (
	"context"
	"fmt"

	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/model"
)

// Writer implements DataFormatWriter for plain text files.
type Writer struct {
	format.BaseFormatWriter
	firstBlock bool
}

// NewWriter creates a new plain text writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "plaintext",
		},
		firstBlock: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed plain text.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
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
		// Skip layer start/end and other structural parts
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("plaintext writer: expected Block resource")
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
