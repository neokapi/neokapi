package transtable

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for translation table files.
type Writer struct {
	format.BaseFormatWriter
	firstEntry bool
}

// NewWriter creates a new translation table writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "transtable",
		},
		firstEntry: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed translation table.
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
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("transtable writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	key := block.Properties["key"]

	if !w.firstEntry {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstEntry = false

	_, err := fmt.Fprintf(w.Output, "%s\t%s", key, text)
	return err
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return nil
	}

	if !w.firstEntry {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstEntry = false

	if comment, has := data.Properties["comment"]; has {
		_, err := fmt.Fprint(w.Output, comment)
		return err
	}

	// Empty line
	return nil
}
