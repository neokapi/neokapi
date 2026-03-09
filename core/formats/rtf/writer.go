package rtf

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for RTF files.
type Writer struct {
	format.BaseFormatWriter
	firstBlock bool
}

// NewWriter creates a new RTF writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "rtf",
		},
		firstBlock: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed RTF.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				// End of stream - close the RTF document.
				if !w.firstBlock {
					if _, err := fmt.Fprint(w.Output, "}\n"); err != nil {
						return err
					}
				}
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
	case model.PartLayerStart:
		return w.writeHeader()
	case model.PartBlock:
		return w.writeBlock(part)
	case model.PartData:
		return w.writeData(part)
	default:
		return nil
	}
}

func (w *Writer) writeHeader() error {
	if _, err := fmt.Fprint(w.Output, "{\\rtf1\\ansi\\deff0\n"); err != nil {
		return err
	}
	w.firstBlock = false
	return nil
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("rtf writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Escape special RTF characters in the text.
	escaped := escapeRTF(text)

	if _, err := fmt.Fprintf(w.Output, "\\pard %s\\par\n", escaped); err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return fmt.Errorf("rtf writer: expected Data resource")
	}
	raw := data.Properties["raw"]
	if raw != "" {
		if _, err := fmt.Fprint(w.Output, raw); err != nil {
			return err
		}
	}
	return nil
}

// escapeRTF escapes special characters for RTF output.
func escapeRTF(s string) string {
	var out []byte
	for _, r := range s {
		switch {
		case r == '\\':
			out = append(out, '\\', '\\')
		case r == '{':
			out = append(out, '\\', '{')
		case r == '}':
			out = append(out, '\\', '}')
		case r == '\t':
			out = append(out, '\\', 't', 'a', 'b', ' ')
		case r > 127:
			out = append(out, []byte(fmt.Sprintf("\\u%d?", r))...)
		default:
			out = append(out, byte(r))
		}
	}
	return string(out)
}
