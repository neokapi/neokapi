package vtt

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for WebVTT subtitle files.
type Writer struct {
	format.BaseFormatWriter
	wroteHeader bool
	firstCue    bool
}

// NewWriter creates a new VTT writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "vtt",
		},
		firstCue: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed VTT.
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

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return fmt.Errorf("vtt writer: expected Data resource")
	}

	if data.Name == "vtt-header" {
		header := data.Properties["content"]
		if header == "" {
			header = "WEBVTT"
		}
		if _, err := fmt.Fprint(w.Output, header); err != nil {
			return err
		}
		w.wroteHeader = true
	}
	// Cue identifiers are handled inline with blocks
	return nil
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("vtt writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	timecode := block.Properties["timecode"]
	cueID := block.Properties["cue-id"]

	// Blank line separator before cue
	if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
		return err
	}

	// Write cue identifier if present
	if cueID != "" {
		if _, err := fmt.Fprintf(w.Output, "%s\n", cueID); err != nil {
			return err
		}
	}

	// Write timecode
	if _, err := fmt.Fprintf(w.Output, "%s\n", timecode); err != nil {
		return err
	}

	// Write subtitle text
	if _, err := fmt.Fprint(w.Output, text); err != nil {
		return err
	}

	w.firstCue = false
	return nil
}
