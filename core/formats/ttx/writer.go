package ttx

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for Trados TagEditor TTX files.
type Writer struct {
	format.BaseFormatWriter
}

// NewWriter creates a new TTX writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "ttx",
		},
	}
}

// Write consumes Parts from a channel and writes reconstructed TTX XML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	enc := xml.NewEncoder(w.Output)
	enc.Indent("", "  ")

	// Write XML declaration
	if _, err := io.WriteString(w.Output, `<?xml version="1.0" encoding="utf-8"?>`+"\n"); err != nil {
		return err
	}

	// Open TRADOStag
	if _, err := io.WriteString(w.Output, `<TRADOStag Version="2.0">`+"\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w.Output, "<Body>\n<Raw>\n"); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				// Close tags
				if _, err := io.WriteString(w.Output, "</Raw>\n</Body>\n</TRADOStag>\n"); err != nil {
					return err
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
	case model.PartBlock:
		return w.writeBlock(part)
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("ttx writer: expected Block resource")
	}

	sourceText := block.SourceText()
	targetText := ""
	targetLang := ""

	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		targetText = block.TargetText(w.Locale)
		targetLang = string(w.Locale)
	}

	sourceLang := block.Properties["source-lang"]
	if sourceLang == "" {
		sourceLang = "EN-US"
	}

	matchPercent := block.Properties["match-percent"]
	if matchPercent == "" {
		matchPercent = "0"
	}

	if _, err := fmt.Fprintf(w.Output, `<Tu MatchPercent="%s">`+"\n", xmlEscape(matchPercent)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, `<Tuv Lang="%s"><Seg>%s</Seg></Tuv>`+"\n", xmlEscape(sourceLang), xmlEscape(sourceText)); err != nil {
		return err
	}

	if targetText != "" && targetLang != "" {
		if _, err := fmt.Fprintf(w.Output, `<Tuv Lang="%s"><Seg>%s</Seg></Tuv>`+"\n", xmlEscape(targetLang), xmlEscape(targetText)); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w.Output, "</Tu>\n"); err != nil {
		return err
	}

	return nil
}

// xmlEscape escapes XML special characters.
func xmlEscape(s string) string {
	var buf []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			buf = append(buf, []byte("&amp;")...)
		case '<':
			buf = append(buf, []byte("&lt;")...)
		case '>':
			buf = append(buf, []byte("&gt;")...)
		case '"':
			buf = append(buf, []byte("&quot;")...)
		default:
			buf = append(buf, s[i])
		}
	}
	return string(buf)
}
