package pdf

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for PDF files.
// Since PDF roundtrip is not feasible, this is a degraded writer that outputs
// extracted text as plain text.
type Writer struct {
	format.BaseFormatWriter
	firstBlock bool
}

// NewWriter creates a new PDF writer (outputs plain text).
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "pdf",
		},
		firstBlock: true,
	}
}

// Write consumes Parts from a channel and writes extracted text as plain text.
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
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("pdf writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	if !w.firstBlock {
		if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
			return err
		}
	}
	w.firstBlock = false

	if _, err := fmt.Fprint(w.Output, text); err != nil {
		return err
	}

	return nil
}
