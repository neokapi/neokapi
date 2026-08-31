package po

import (
	"bytes"
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
	lineEnd       string
}

// nl returns the writer's line separator. CRLF when the skeleton's
// first text entry contained \r\n (Windows / okapi-emitted .po), else
// LF. msgstr value lines use the same separator so the round-trip
// stays byte-stable.
func (w *Writer) nl() string {
	if w.lineEnd == "" {
		return "\n"
	}
	return w.lineEnd
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new PO writer.
func NewWriter() *Writer {
	return &Writer{
		FormatName:  "po",
		Interchange: true,
		firstEntry:  true,
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
			if w.lineEnd == "" && bytes.Contains(entry.Data, []byte("\r\n")) {
				w.lineEnd = "\r\n"
			}
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			if blockID, field, ok := splitMsgidRef(refID); ok {
				if err := w.writeBlockAsMsgid(blocks, blockID, field); err != nil {
					return err
				}
				continue
			}
			if err := w.writeBlockAsMsgstr(blocks, refID); err != nil {
				return err
			}
		}
	}
	return nil
}

// splitMsgidRef recognises a skeleton ref that stands in for a msgid /
// msgid_plural line group, returning the block id it addresses and the field
// name to emit. Msgstr refs carry no separator and are reported as not-a-msgid.
func splitMsgidRef(refID string) (blockID, field string, ok bool) {
	blockID, field, ok = strings.Cut(refID, msgidRefSep)
	if !ok || blockID == "" {
		return "", "", false
	}
	if field != "msgid" && field != "msgid_plural" {
		return "", "", false
	}
	return blockID, field, true
}

// writeBlockAsMsgid writes the complete msgid (or msgid_plural) field for a
// block reference — the source side of the round-trip.
//
// The original line group is re-emitted verbatim while it still spells the
// block's source, which keeps an untouched catalog byte-exact (line wrapping,
// escaping and the leading `msgid ""` continuation style all survive). Once the
// source has been edited the field is re-serialized, so the edit actually
// reaches the file. Before #1473 there was no ref here at all: the msgid was
// opaque skeleton text, so `kapi ksed -i` on a .po applied the edit to the
// content model and then wrote the original bytes back, exit 0.
func (w *Writer) writeBlockAsMsgid(blocks map[string]*model.Block, blockID, field string) error {
	block, ok := blocks[blockID]
	if !ok {
		// No block for this ref (a plural form the header's nplurals did not
		// produce, say). Nothing to substitute — emit an empty field rather
		// than dropping the keyword, which would corrupt the entry.
		_, err := fmt.Fprintf(w.Output, "%s \"\"%s", field, w.nl())
		return err
	}
	// The recorded witness is the msgid as parsed, so compare against the
	// source rebuilt with its inline-code Data spliced back in (a codeFinder
	// `%s` comes back as `%s`) — that reproduces exactly what the reader read.
	if raw, ok := format.VerbatimFor(block, "raw-msgid", model.RenderRunsWithData(block.Source)); ok && raw != "" {
		_, err := io.WriteString(w.Output, raw)
		return err
	}
	return w.writeMultilineField(field, renderSource(block))
}

// writeBlockAsMsgstr writes the complete msgstr field for a block reference.
// The refID is the block ID (e.g., "tu1", "tu1-singular", "tu1-plural").
// The skeleton ref replaces the entire msgstr field (keyword + value lines).
//
// If the block has a "raw-msgstr" property and the text to write matches the
// original value, the raw bytes are output verbatim for byte-exact roundtrip.
// Otherwise, the value is re-serialized using writeMultilineField.
func (w *Writer) writeBlockAsMsgstr(blocks map[string]*model.Block, refID string) error {
	// Determine the field name based on the refID. Plural refs encode
	// their msgstr[N] index: `-singular` is form 0, `-plural` is form 1,
	// and `-pl4N` (e.g. `-plural2`) is form N for languages with more
	// than two plural forms.
	fieldName := "msgstr"
	switch {
	case strings.HasSuffix(refID, "-singular"):
		fieldName = "msgstr[0]"
	case strings.HasSuffix(refID, "-plural"):
		fieldName = "msgstr[1]"
	default:
		if idx := strings.LastIndex(refID, "-plural"); idx >= 0 {
			n := 0
			if _, err := fmt.Sscanf(refID[idx+len("-plural"):], "%d", &n); err == nil && n > 0 {
				fieldName = fmt.Sprintf("msgstr[%d]", n)
			}
		}
	}

	block, ok := blocks[refID]
	if !ok {
		// Block not found — write empty msgstr field
		_, err := fmt.Fprintf(w.Output, "%s \"\"%s", fieldName, w.nl())
		return err
	}

	text, known := w.msgstrText(block)

	// Check if we can use the raw msgstr bytes for byte-exact output. When the
	// writer cannot say what belongs in the slot, the capture is the only
	// statement of it and stands unconditionally: blanking a translation the file
	// arrived with is the worst of the available answers.
	if raw, ok := block.Properties[propRawMsgstr]; ok && raw != "" {
		if !known || w.parseRawMsgstrValue(raw) == text.text {
			_, err := io.WriteString(w.Output, raw)
			return err
		}
	}

	return w.writeMultilineField(fieldName, text)
}

// msgstrText resolves what belongs in a block's msgstr slot, and whether the
// writer can say.
//
// With an active locale the writer is merging that locale into the catalog, so
// the answer is its target — empty when there is none, which is PO's spelling of
// "untranslated" and the long-standing behaviour.
//
// With NO active locale the writer is reproducing the catalog rather than merging
// into it, and the msgstr slot still belongs to the locale the reader attached it
// under. The model's target for that locale is then the authority — the rule
// #1471 established for .kbf.json, here under a non-write locale (#1482). Before this,
// "no active locale" resolved to the empty string, so every translated msgstr was
// rewritten empty: `kapi apply` and the MCP edit tool pass no write locale at
// all, and `kapi ksed -i` passes one only with --target-lang, so editing a source
// string in a translated catalog blanked every translation in it and exited 0.
//
// When no locale was recorded either — a monolingual catalog, where the msgstr
// carries the source and no target is ever attached — the writer knows nothing
// about the slot and says so, and its caller keeps the captured bytes.
func (w *Writer) msgstrText(block *model.Block) (text poValue, known bool) {
	if !w.Locale.IsEmpty() {
		return renderTarget(block, w.Locale), true
	}
	if loc, ok := format.VerbatimSlotLocale(block, propRawMsgstr); ok {
		return renderTarget(block, loc), true
	}
	return poValue{}, false
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

	// Write msgstr — the target for the writer's locale, or, with no active
	// locale, the target for the locale the slot belongs to (msgstrText).
	target, _ := w.msgstrText(block)
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

	// Write one msgstr[N] per plural form in the group. Languages with
	// more than two forms (e.g. Russian nplurals=3) carry extra blocks.
	for i, block := range w.pluralGroup {
		target, _ := w.msgstrText(block)
		if err := w.writeMultilineField(fmt.Sprintf("msgstr[%d]", i), target); err != nil {
			return err
		}
	}

	return nil
}

// writeMultilineField writes a PO field. Okapi uses the multi-line
// `msgstr ""` + continuation lines form only when (a) every embedded
// `\n` falls at the end of a continuation segment (i.e. the raw value
// genuinely spans multiple physical lines), or (b) the caller forces
// multi-line via writeMultilineFieldForced. A single embedded newline
// in an otherwise short value (e.g. `Cannot find file '%s'\n.`) stays
// on one line as the `\n` escape.
func (w *Writer) writeMultilineField(field string, v poValue) error {
	if v.text == "" || !strings.Contains(v.text, "\n") {
		nl := w.nl()
		_, err := fmt.Fprintf(w.Output, "%s %s%s", field, v.quoted(), nl)
		return err
	}
	// Heuristic: only emit multi-line when the value ends with `\n`
	// (the canonical "this is a multi-line value" marker okapi uses).
	if !strings.HasSuffix(v.text, "\n") {
		nl := w.nl()
		_, err := fmt.Fprintf(w.Output, "%s %s%s", field, v.quoted(), nl)
		return err
	}
	return w.writeMultilineFieldForced(field, v)
}

func (w *Writer) writeMultilineFieldForced(field string, v poValue) error {
	nl := w.nl()
	if _, err := fmt.Fprintf(w.Output, "%s \"\"%s", field, nl); err != nil {
		return err
	}
	lines := strings.Split(v.text, "\n")
	// Track each line's offset in v.text so the verbatim ranges follow the
	// split rather than being lost at it.
	offset := 0
	for i, line := range lines {
		lineValue := v.slice(offset, offset+len(line))
		offset += len(line) + len("\n")
		if i == len(lines)-1 && line == "" {
			continue
		}
		suffix := ""
		if i < len(lines)-1 {
			suffix = "\\n"
		}
		if _, err := fmt.Fprintf(w.Output, "\"%s%s\"%s", lineValue.escaped(), suffix, nl); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) writeEntryGap() {
	if !w.firstEntry {
		fmt.Fprintln(w.Output)
	}
	w.firstEntry = false
}

// quotePO wraps a plain string in double quotes with proper escaping.
// Callers holding a rendered field value use poValue.quoted instead, which
// honours the value's verbatim regions.
func quotePO(s string) string {
	return "\"" + escapePOPlain(s) + "\""
}

func escapePOPlain(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\a", "\\a")
	s = strings.ReplaceAll(s, "\b", "\\b")
	s = strings.ReplaceAll(s, "\f", "\\f")
	s = strings.ReplaceAll(s, "\v", "\\v")
	return s
}

// poValue is a rendered PO field value: the text exactly as it belongs in the
// document, plus the byte ranges within that text which are already in PO
// escape form and must reach the output untouched (Ph run Data such as printf
// specifiers, or a `\r` carried as a literal backslash + r whose backslash
// must not be doubled along with the surrounding TextRun backslashes).
//
// The verbatim ranges are carried BESIDE the text rather than marked inside
// it. An earlier design bracketed them with '\x01' / '\x02' on the stated
// assumption that those are "control bytes that never appear in PO source
// text" — but the reader accepts any byte inside a quoted msgid, so a literal
// U+0001 in the content was indistinguishable from a marker and was silently
// deleted on write (#1600). PO reserves no byte value, so neither may the
// writer: in-band signalling has no safe alphabet here.
type poValue struct {
	text string
	// verbatim holds sorted, non-overlapping, half-open [start, end) ranges
	// into text.
	verbatim []poSpan
}

// poSpan is a half-open [start, end) byte range of a poValue's text.
type poSpan struct{ start, end int }

// quoted renders the value as a quoted PO string literal.
func (v poValue) quoted() string { return "\"" + v.escaped() + "\"" }

// escaped applies PO escaping to everything outside the verbatim ranges.
func (v poValue) escaped() string {
	if len(v.verbatim) == 0 {
		return escapePOPlain(v.text)
	}
	var b strings.Builder
	b.Grow(len(v.text))
	pos := 0
	for _, sp := range v.verbatim {
		b.WriteString(escapePOPlain(v.text[pos:sp.start]))
		b.WriteString(v.text[sp.start:sp.end])
		pos = sp.end
	}
	b.WriteString(escapePOPlain(v.text[pos:]))
	return b.String()
}

// slice returns the sub-value covering text[from:to], with the verbatim ranges
// clipped to that window and rebased onto it. A verbatim range that straddles
// the boundary stays verbatim on both sides.
func (v poValue) slice(from, to int) poValue {
	out := poValue{text: v.text[from:to]}
	for _, sp := range v.verbatim {
		start, end := max(sp.start, from), min(sp.end, to)
		if start < end {
			out.verbatim = append(out.verbatim, poSpan{start - from, end - from})
		}
	}
	return out
}

// renderSource returns the source text with inline-code Data preserved
// verbatim (e.g. printf specifiers `%s` extracted by the codeFinder come back
// out as `%s`, not as their `{equiv}` placeholder form).
func renderSource(block *model.Block) poValue {
	return renderRuns(block.Source)
}

// renderTarget mirrors renderSource for a target locale. Returns the zero
// value if the block has no target for the given locale (PO is bilingual —
// untranslated entries keep an empty msgstr rather than falling back to source
// text).
func renderTarget(block *model.Block, locale model.LocaleID) poValue {
	if locale == "" {
		return poValue{}
	}
	t := block.Target(locale)
	if t == nil {
		return poValue{}
	}
	return renderRuns(t.Runs)
}

// renderRuns walks a run slice, emitting TextRun content verbatim (newlines
// preserved so writeMultilineField can split on them) and recording each Ph
// Data range as verbatim. Other run shapes fall back to RenderRunsWithData
// semantics on a single-run slice so any future run types stay handled.
func renderRuns(runs []model.Run) poValue {
	var v poValue
	var buf strings.Builder
	for _, run := range runs {
		switch {
		case run.Ph != nil:
			start := buf.Len()
			buf.WriteString(run.Ph.Data)
			if buf.Len() > start {
				v.verbatim = append(v.verbatim, poSpan{start, buf.Len()})
			}
		case run.Text != nil:
			buf.WriteString(run.Text.Text)
		case run.PcOpen != nil || run.PcClose != nil:
			// PO has no inline-markup vocabulary; drop paired-code markers so a
			// foreign format's markup (markdown **…**, OOXML <w:b>…</w:b>) never
			// leaks into a msgid on cross-format conversion. The wrapped text is
			// carried by separate TextRuns and survives. Same-format PO has no
			// such runs, so round-trip is unaffected.
			continue
		default:
			buf.WriteString(model.RenderRunsWithData([]model.Run{run}))
		}
	}
	v.text = buf.String()
	return v
}
