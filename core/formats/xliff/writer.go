package xliff

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for XLIFF 1.2 files.
type Writer struct {
	format.BaseFormatWriter
	parts      []*model.Part
	sourceLang model.LocaleID
	targetLang model.LocaleID
	fileName   string
}

// NewWriter creates a new XLIFF 1.2 writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xliff",
		},
	}
}

// Write consumes Parts from a channel and writes XLIFF 1.2 output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.flush()
			}
			w.parts = append(w.parts, part)
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok {
					w.sourceLang = layer.Locale
					w.fileName = layer.Name
					if tl, ok := layer.Properties["target-language"]; ok {
						w.targetLang = model.LocaleID(tl)
					}
				}
			}
		}
	}
}

func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	fmt.Fprint(w.Output, xml.Header)
	fmt.Fprintf(w.Output, `<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">`)
	fmt.Fprintf(w.Output, "\n")

	// Write file envelope
	fmt.Fprintf(w.Output, `  <file original="%s" source-language="%s"`,
		xmlEscapeAttr(w.fileName), xmlEscapeAttr(string(w.sourceLang)))
	if !targetLang.IsEmpty() {
		fmt.Fprintf(w.Output, ` target-language="%s"`, xmlEscapeAttr(string(targetLang)))
	}
	fmt.Fprintf(w.Output, ` datatype="plaintext">`)
	fmt.Fprintf(w.Output, "\n    <body>\n")

	// Write trans-units from collected blocks
	for _, part := range w.parts {
		if part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}

		fmt.Fprintf(w.Output, `      <trans-unit id="%s"`, xmlEscapeAttr(block.ID))
		if !block.Translatable {
			fmt.Fprintf(w.Output, ` translate="no"`)
		}
		if block.PreserveWhitespace {
			fmt.Fprintf(w.Output, ` xml:space="preserve"`)
		}
		if v, ok := block.Properties["approved"]; ok && v == "yes" {
			fmt.Fprintf(w.Output, ` approved="yes"`)
		}
		fmt.Fprintf(w.Output, ">\n")

		// Source
		sourceText := fragmentToXLIFF(block.Source)
		fmt.Fprintf(w.Output, "        <source>%s</source>\n", sourceText)

		// Target
		if block.HasTarget(targetLang) {
			targetText := fragmentToXLIFF(block.Targets[targetLang])
			fmt.Fprintf(w.Output, "        <target>%s</target>\n", targetText)
		}

		// Notes
		for key, ann := range block.Annotations {
			if strings.HasPrefix(key, "note") {
				if note, ok := ann.(*model.NoteAnnotation); ok {
					fmt.Fprintf(w.Output, "        <note")
					if note.From != "" {
						fmt.Fprintf(w.Output, ` from="%s"`, xmlEscapeAttr(note.From))
					}
					if note.Priority > 0 {
						fmt.Fprintf(w.Output, ` priority="%d"`, note.Priority)
					}
					if note.Annotates != "" {
						fmt.Fprintf(w.Output, ` annotates="%s"`, xmlEscapeAttr(note.Annotates))
					}
					fmt.Fprintf(w.Output, ">%s</note>\n", xmlEscapeText(note.Text))
				}
			}
		}

		// Alt-trans
		for key, ann := range block.Annotations {
			if strings.HasPrefix(key, "alt-translation") {
				if alt, ok := ann.(*model.AltTranslation); ok {
					fmt.Fprintf(w.Output, "        <alt-trans")
					if alt.CombinedScore > 0 {
						fmt.Fprintf(w.Output, ` match-quality="%.0f"`, alt.CombinedScore)
					}
					if alt.Origin != "" {
						fmt.Fprintf(w.Output, ` origin="%s"`, xmlEscapeAttr(alt.Origin))
					}
					fmt.Fprintf(w.Output, ">\n")
					if alt.Source != nil {
						fmt.Fprintf(w.Output, "          <source>%s</source>\n", xmlEscapeText(alt.Source.Text()))
					}
					if alt.Target != nil {
						fmt.Fprintf(w.Output, `          <target xml:lang="%s">%s</target>`+"\n",
							xmlEscapeAttr(string(targetLang)), xmlEscapeText(alt.Target.Text()))
					}
					fmt.Fprintf(w.Output, "        </alt-trans>\n")
				}
			}
		}

		fmt.Fprintf(w.Output, "      </trans-unit>\n")
	}

	fmt.Fprintf(w.Output, "    </body>\n  </file>\n</xliff>")
	return nil
}

// fragmentToXLIFF converts segments to XLIFF inline content.
func fragmentToXLIFF(segs []*model.Segment) string {
	var buf strings.Builder
	for _, seg := range segs {
		if seg.Content != nil {
			// Simple case: just use the text
			buf.WriteString(xmlEscapeText(seg.Content.Text()))
		}
	}
	return buf.String()
}
