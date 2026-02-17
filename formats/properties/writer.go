package properties

import (
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
)

// Writer implements DataFormatWriter for Java Properties files.
type Writer struct {
	format.BaseFormatWriter
	firstLine bool
}

// NewWriter creates a new Properties writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "properties",
		},
		firstLine: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed properties content.
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
		return fmt.Errorf("properties writer: expected Block resource")
	}

	// Use target text if available, otherwise source text
	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Encode unicode escapes for non-ASCII characters
	text = encodeUnicodeEscapes(text)

	sep := "="
	if s, ok := block.Properties["separator"]; ok && s != "" {
		sep = s
	}

	w.writeLine()
	_, err := fmt.Fprintf(w.Output, "%s%s%s", block.Name, sep, text)
	return err
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return fmt.Errorf("properties writer: expected Data resource")
	}

	switch data.Name {
	case "comment":
		comment := data.Properties["comment"]
		w.writeLine()
		_, err := fmt.Fprint(w.Output, comment)
		return err
	case "blank":
		w.writeLine()
		return nil
	}

	return nil
}

func (w *Writer) writeLine() {
	if !w.firstLine {
		fmt.Fprintln(w.Output)
	}
	w.firstLine = false
}

// encodeUnicodeEscapes converts non-ASCII characters to \uXXXX escapes.
func encodeUnicodeEscapes(s string) string {
	var buf strings.Builder
	for _, r := range s {
		if r > 127 {
			buf.WriteString(fmt.Sprintf("\\u%04X", r))
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
