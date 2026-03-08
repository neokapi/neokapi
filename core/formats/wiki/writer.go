package wiki

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for Wiki files.
type Writer struct {
	format.BaseFormatWriter
	cfg        *Config
	firstBlock bool
	// lastPartType tracks what we last wrote for whitespace decisions.
	lastPartType model.PartType
}

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

// Config returns the writer's configuration.
func (w *Writer) Config() *Config { return w.cfg }

// Write consumes Parts from a channel and writes reconstructed wiki markup.
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
