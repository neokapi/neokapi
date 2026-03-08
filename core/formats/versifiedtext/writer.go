package versifiedtext

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for versified text files.
type Writer struct {
	format.BaseFormatWriter
	firstEntry bool
}

// NewWriter creates a new versified text writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "versifiedtext",
		},
		firstEntry: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed versified text.
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
		return w.writeData()
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("versifiedtext writer: expected Block resource")
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

	verseNum := block.Properties["verse"]
	if verseNum != "" {
		_, err := fmt.Fprintf(w.Output, "\\v%s %s", verseNum, text)
		return err
	}

	_, err := fmt.Fprint(w.Output, text)
	return err
}

func (w *Writer) writeData() error {
	// Stanza break (blank line)
	if !w.firstEntry {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstEntry = false
	return nil
}
