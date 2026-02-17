package srt

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
)

// Writer implements DataFormatWriter for SRT subtitle files.
type Writer struct {
	format.BaseFormatWriter
	firstEntry bool
}

// NewWriter creates a new SRT writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "srt",
		},
		firstEntry: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed SRT.
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
		return fmt.Errorf("srt writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	sequence := block.Properties["sequence"]
	timecode := block.Properties["timecode"]

	// Blank line separator between entries
	if !w.firstEntry {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstEntry = false

	// Write sequence number
	if _, err := fmt.Fprintf(w.Output, "%s\n", sequence); err != nil {
		return err
	}

	// Write timecode
	if _, err := fmt.Fprintf(w.Output, "%s\n", timecode); err != nil {
		return err
	}

	// Write subtitle text
	if _, err := fmt.Fprintf(w.Output, "%s\n", text); err != nil {
		return err
	}

	return nil
}
