package splicedlines

import (
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for line-spliced text files.
type Writer struct {
	format.BaseFormatWriter
	firstEntry bool
}

// NewWriter creates a new spliced lines writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "splicedlines",
		},
		firstEntry: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed spliced lines.
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
		return fmt.Errorf("splicedlines writer: expected Block resource")
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

	// If text contains newlines, re-add backslash continuations
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i < len(lines)-1 {
			if _, err := fmt.Fprintf(w.Output, "%s\\\n", line); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprint(w.Output, line); err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *Writer) writeData() error {
	if !w.firstEntry {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstEntry = false
	return nil
}
