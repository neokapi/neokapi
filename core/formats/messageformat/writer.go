package messageformat

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for ICU MessageFormat files.
type Writer struct {
	format.BaseFormatWriter
	firstLine bool

	// blocks stores blocks by path for reconstruction.
	blocks map[string]*model.Block
}

// NewWriter creates a new MessageFormat writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "messageformat",
		},
		firstLine: true,
		blocks:    make(map[string]*model.Block),
	}
}

// Write consumes Parts from a channel and writes reconstructed MessageFormat.
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
		return fmt.Errorf("messageformat writer: expected Block resource")
	}

	text := w.getBlockText(block)

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
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}
