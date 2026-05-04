package po

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for PO (gettext) files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	firstEntry    bool
	inPlural      bool
	pluralGroup   []*model.Block
	pendingBlock  bool // true if we've written metadata (comment/ref/flags) for the next block
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new PO writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "po",
		},
		firstEntry: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed PO content.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeleton(ctx, parts)
	}

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

// writeWithSkeleton collects all blocks, then reconstructs output from skeleton entries.
func (w *Writer) writeWithSkeleton(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
				}
			}
		}
	}
done:
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("po writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("po writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			if err := w.writeBlockAsMsgstr(blocks, refID); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeBlockAsMsgstr writes the complete msgstr field for a block reference.
// The refID is the block ID (e.g., "tu1", "tu1-singular", "tu1-plural").
// The skeleton ref replaces the entire msgstr field (keyword + value lines).
//
// If the block has a "raw-msgstr" property and the text to write matches the
// original value, the raw bytes are output verbatim for byte-exact roundtrip.
// Otherwise, the value is re-serialized using writeMultilineField.
func (w *Writer) writeBlockAsMsgstr(blocks map[string]*model.Block, refID string) error {
	// Determine the field name based on the refID
	fieldName := "msgstr"
	if strings.HasSuffix(refID, "-singular") {
		fieldName = "msgstr[0]"
	} else if strings.HasSuffix(refID, "-plural") {
		fieldName = "msgstr[1]"
	}

	block, ok := blocks[refID]
	if !ok {
		// Block not found — write empty msgstr field
		_, err := fmt.Fprintf(w.Output, "%s \"\"\n", fieldName)
		return err
	}

	text := w.blockText(block)

	// Check if we can use the raw msgstr bytes for byte-exact output.
	if raw, ok := block.Properties["raw-msgstr"]; ok && raw != "" {
		// Parse the original msgstr value from the raw field to compare.
		origValue := w.parseRawMsgstrValue(raw)
		if origValue == text {
			// Text unchanged — output raw bytes verbatim.
			_, err := io.WriteString(w.Output, raw)
			return err
		}
	}

	return w.writeMultilineField(fieldName, text)
}

// parseRawMsgstrValue extracts the decoded string value from raw msgstr field text.
// For example, from `msgstr "Bonjour"\n` it returns "Bonjour".
// For multiline: `msgstr ""\n"Hello "\n"World"\n` returns "Hello World".
func (w *Writer) parseRawMsgstrValue(raw string) string {
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}

	var result strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip the keyword prefix
		if strings.HasPrefix(line, "msgstr") {
			// Extract the quoted portion after "msgstr " or "msgstr[N] "
			idx := strings.Index(line, " ")
			if idx < 0 {
				continue
			}
			line = strings.TrimSpace(line[idx+1:])
		}
		// Unquote
		result.WriteString(unquotePO(line))
	}
	return result.String()
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
		return errors.New("po writer: expected Data resource")
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
		for line := range strings.SplitSeq(comment, "\n") {
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
		for line := range strings.SplitSeq(ref, "\n") {
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
		return errors.New("po writer: expected Block resource")
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

	source := renderSource(block)

	// Write msgid
	if err := w.writeMultilineField("msgid", source); err != nil {
		return err
	}

	// Write msgstr - use target text if available
	target := ""
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		target = renderTarget(block, w.Locale)
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
	if err := w.writeMultilineField("msgid", renderSource(singular)); err != nil {
		return err
	}

	// Write msgid_plural
	if err := w.writeMultilineField("msgid_plural", renderSource(plural)); err != nil {
		return err
	}

	// Write msgstr[0] and msgstr[1]
	singularTarget := ""
	if !w.Locale.IsEmpty() && singular.HasTarget(w.Locale) {
		singularTarget = renderTarget(singular, w.Locale)
	}
	if err := w.writeMultilineField("msgstr[0]", singularTarget); err != nil {
		return err
	}

	pluralTarget := ""
	if !w.Locale.IsEmpty() && plural.HasTarget(w.Locale) {
		pluralTarget = renderTarget(plural, w.Locale)
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

// escapePO escapes special characters for PO format. Regions wrapped
// in escapeSkipStart/escapeSkipEnd sentinels are emitted verbatim — the
// caller has already prepared them in PO escape form (Ph run Data such
// as printf specifiers and `\r` / `\t` escape sequences).
func escapePO(s string) string {
	if !strings.ContainsRune(s, escapeSkipStart) {
		return escapePOPlain(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == escapeSkipStart {
			end := strings.IndexByte(s[i+1:], escapeSkipEnd)
			if end < 0 {
				// Malformed sentinel pair — fall back to escaping the
				// remainder so we never emit a literal control byte.
				b.WriteString(escapePOPlain(s[i+1:]))
				return b.String()
			}
			b.WriteString(s[i+1 : i+1+end])
			i = i + 1 + end + 1
			continue
		}
		// Escape the next contiguous non-sentinel run in one batch so
		// ReplaceAll's bulk performance is preserved on long strings.
		next := strings.IndexByte(s[i:], escapeSkipStart)
		if next < 0 {
			b.WriteString(escapePOPlain(s[i:]))
			return b.String()
		}
		b.WriteString(escapePOPlain(s[i : i+next]))
		i += next
	}
	return b.String()
}

func escapePOPlain(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// blockText returns the target text if available for the writer's locale,
// otherwise an empty string. PO is a bilingual format — untranslated
// entries keep an empty msgstr rather than falling back to source text.
//
// Inline-code Ph runs (e.g. printf specifiers split out by codeFinder)
// re-emit their original Data string, so `%s` survives a round-trip
// through the model instead of disappearing as `seg.Text()` would.
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return renderTarget(block, w.Locale)
	}
	return ""
}

// renderSource returns the source text with inline-code Data preserved
// verbatim (e.g. printf specifiers `%s` extracted by the codeFinder
// come back out as `%s`, not as their `{equiv}` placeholder form). Ph
// data is wrapped in escapeSkipStart/escapeSkipEnd sentinels so
// escapePO emits it untouched — without sentinels, escape sequences
// like `\r` (a literal backslash + r carried by the Ph data) would
// have their backslash double-escaped to `\\r` along with TextRun
// backslashes.
func renderSource(block *model.Block) string {
	return renderSegmentsWithSentinels(block.Source)
}

// renderTarget mirrors renderSource for a target locale. Returns the
// empty string if the block has no target for the given locale (PO is
// bilingual — untranslated entries keep an empty msgstr rather than
// falling back to source text).
func renderTarget(block *model.Block, locale model.LocaleID) string {
	if locale == "" {
		return ""
	}
	segs, ok := block.Targets[locale]
	if !ok {
		return ""
	}
	return renderSegmentsWithSentinels(segs)
}

// escapeSkipStart and escapeSkipEnd bracket regions of text that
// escapePO must emit verbatim (Ph run Data is already in PO escape
// form). Use ASCII control bytes that never appear in PO source text.
const (
	escapeSkipStart = '\x01'
	escapeSkipEnd   = '\x02'
)

// renderSegmentsWithSentinels walks a segment slice, emitting TextRun
// content verbatim (newlines preserved so writeMultilineField can split
// on them) and Ph Data wrapped in escape-skip sentinels. Other run
// shapes fall back to RenderRunsWithData semantics on a single-run
// slice so any future run types stay handled.
func renderSegmentsWithSentinels(segs []*model.Segment) string {
	var buf strings.Builder
	for _, seg := range segs {
		if seg == nil {
			continue
		}
		for _, run := range seg.Runs {
			switch {
			case run.Ph != nil:
				buf.WriteByte(escapeSkipStart)
				buf.WriteString(run.Ph.Data)
				buf.WriteByte(escapeSkipEnd)
			case run.Text != nil:
				buf.WriteString(run.Text.Text)
			default:
				buf.WriteString(model.RenderRunsWithData([]model.Run{run}))
			}
		}
	}
	return buf.String()
}
