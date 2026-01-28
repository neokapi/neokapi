package po

import (
	"context"
	"fmt"
	"strings"

	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/model"
)

// Writer implements DataFormatWriter for PO (gettext) files.
type Writer struct {
	format.BaseFormatWriter
	firstEntry   bool
	inPlural     bool
	pluralGroup  []*model.Block
	pendingBlock bool // true if we've written metadata (comment/ref/flags) for the next block
}

// NewWriter creates a new PO writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "po",
		},
		firstEntry: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed PO content.
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
	case model.PartData:
		return w.writeData(part)
	case model.PartBlock:
		return w.writeBlock(part)
	case model.PartGroupStart:
		w.inPlural = true
		w.pluralGroup = nil
		return nil
	case model.PartGroupEnd:
		err := w.writePluralGroup()
		w.inPlural = false
		w.pluralGroup = nil
		return err
	default:
		return nil
	}
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return fmt.Errorf("po writer: expected Data resource")
	}

	switch data.Name {
	case "header":
		content := data.Properties["content"]
		w.writeEntryGap()
		if _, err := fmt.Fprint(w.Output, "msgid \"\"\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprint(w.Output, "msgstr \"\"\n"); err != nil {
			return err
		}
		// Write header lines as continuation strings
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			escaped := quotePO(line + "\n")
			if _, err := fmt.Fprintf(w.Output, "%s\n", escaped); err != nil {
				return err
			}
		}
		return nil

	case "comment":
		comment := data.Properties["comment"]
		// Write entry gap before the comment (which is metadata for the next block)
		w.writeEntryGap()
		for _, line := range strings.Split(comment, "\n") {
			if _, err := fmt.Fprintf(w.Output, "# %s\n", line); err != nil {
				return err
			}
		}
		// Mark that we've started writing metadata for the next block,
		// so the block should not emit another entry gap.
		w.pendingBlock = true
		return nil

	case "reference":
		ref := data.Properties["reference"]
		// Only emit entry gap if we haven't already written a comment for this entry
		if !w.pendingBlock {
			w.writeEntryGap()
			w.pendingBlock = true
		}
		for _, line := range strings.Split(ref, "\n") {
			if _, err := fmt.Fprintf(w.Output, "#: %s\n", line); err != nil {
				return err
			}
		}
		return nil

	case "flags":
		flags := data.Properties["flags"]
		if !w.pendingBlock {
			w.writeEntryGap()
			w.pendingBlock = true
		}
		if _, err := fmt.Fprintf(w.Output, "#, %s\n", flags); err != nil {
			return err
		}
		return nil
	}

	return nil
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("po writer: expected Block resource")
	}

	if w.inPlural {
		w.pluralGroup = append(w.pluralGroup, block)
		return nil
	}

	// Only write entry gap if there was no preceding metadata for this entry
	if !w.pendingBlock {
		w.writeEntryGap()
	}
	w.pendingBlock = false

	// Write msgctxt if present
	if ctxt, ok := block.Properties["context"]; ok && ctxt != "" {
		if _, err := fmt.Fprintf(w.Output, "msgctxt %s\n", quotePO(ctxt)); err != nil {
			return err
		}
	}

	source := block.SourceText()

	// Write msgid
	if err := w.writeMultilineField("msgid", source); err != nil {
		return err
	}

	// Write msgstr - use target text if available
	target := ""
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		target = block.TargetText(w.Locale)
	}
	if err := w.writeMultilineField("msgstr", target); err != nil {
		return err
	}

	return nil
}

func (w *Writer) writePluralGroup() error {
	if len(w.pluralGroup) < 2 {
		return nil
	}

	if !w.pendingBlock {
		w.writeEntryGap()
	}
	w.pendingBlock = false

	singular := w.pluralGroup[0]
	plural := w.pluralGroup[1]

	// Write msgctxt if present
	if ctxt, ok := singular.Properties["context"]; ok && ctxt != "" {
		if _, err := fmt.Fprintf(w.Output, "msgctxt %s\n", quotePO(ctxt)); err != nil {
			return err
		}
	}

	// Write msgid (singular)
	if err := w.writeMultilineField("msgid", singular.SourceText()); err != nil {
		return err
	}

	// Write msgid_plural
	if err := w.writeMultilineField("msgid_plural", plural.SourceText()); err != nil {
		return err
	}

	// Write msgstr[0] and msgstr[1]
	singularTarget := ""
	if !w.Locale.IsEmpty() && singular.HasTarget(w.Locale) {
		singularTarget = singular.TargetText(w.Locale)
	}
	if err := w.writeMultilineField("msgstr[0]", singularTarget); err != nil {
		return err
	}

	pluralTarget := ""
	if !w.Locale.IsEmpty() && plural.HasTarget(w.Locale) {
		pluralTarget = plural.TargetText(w.Locale)
	}
	if err := w.writeMultilineField("msgstr[1]", pluralTarget); err != nil {
		return err
	}

	return nil
}

// writeMultilineField writes a PO field, using multiline format if the value
// contains newlines (other than a trailing one).
func (w *Writer) writeMultilineField(field, value string) error {
	// Check if value needs multiline: contains embedded newlines
	if strings.Contains(value, "\n") && value != "" {
		// Multiline: start with empty string, then continuation lines
		if _, err := fmt.Fprintf(w.Output, "%s \"\"\n", field); err != nil {
			return err
		}
		lines := strings.Split(value, "\n")
		for i, line := range lines {
			if i == len(lines)-1 && line == "" {
				// Last empty element from trailing newline - skip
				continue
			}
			suffix := ""
			if i < len(lines)-1 {
				suffix = "\\n"
			}
			if _, err := fmt.Fprintf(w.Output, "\"%s%s\"\n", escapePO(line), suffix); err != nil {
				return err
			}
		}
		return nil
	}

	_, err := fmt.Fprintf(w.Output, "%s %s\n", field, quotePO(value))
	return err
}

func (w *Writer) writeEntryGap() {
	if !w.firstEntry {
		fmt.Fprintln(w.Output)
	}
	w.firstEntry = false
}

// quotePO wraps a string in double quotes with proper escaping.
func quotePO(s string) string {
	return "\"" + escapePO(s) + "\""
}

// escapePO escapes special characters for PO format.
func escapePO(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
