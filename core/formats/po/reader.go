package po

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	coreenc "github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for PO (gettext) files.
type Reader struct {
	format.BaseFormatReader
	cfg             *Config
	skeletonStore   *format.SkeletonStore
	skelBuf         bytes.Buffer // coalesces skeleton text between refs
	hadUTF8BOM      bool         // input started with EF BB BF
	crlfLineEndings bool         // input used \r\n (Windows / okapi-emitted) line endings
}

// HadUTF8BOM reports whether the input started with a UTF-8 BOM. The
// writer uses this to re-emit the BOM when its OriginalContent comes
// from this reader.
func (r *Reader) HadUTF8BOM() bool { return r.hadUTF8BOM }

// CRLFLineEndings reports whether the input used \r\n line endings.
// Used by the writer to re-emit CRLF on round-trip.
func (r *Reader) CRLFLineEndings() bool { return r.crlfLineEndings }

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new PO reader.
func NewReader() *Reader {
	cfg := &Config{
		PreserveUntranslated: true,
		BilingualMode:        true,
		WrapContent:          true,
	}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "po",
			FormatDisplayName: "PO (Gettext)",
			FormatMimeType:    "text/x-gettext-translation",
			FormatExtensions:  []string{".po", ".pot"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/x-gettext-translation"},
		Extensions: []string{".po", ".pot"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("po: nil document or reader")
	}
	// Buffer the input and transcode UTF-16/UTF-8-BOM up front so the
	// line scanner sees plain UTF-8. Real-world .po files (Windows-
	// authored, exported from Trados-style tools) are routinely UTF-16
	// LE and the scanner would otherwise see the spaces between every
	// other byte and emit zero entries.
	raw, err := io.ReadAll(doc.Reader)
	if err != nil {
		return fmt.Errorf("po: reading: %w", err)
	}
	r.hadUTF8BOM = len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF
	utf8Bytes, _, err := coreenc.ToUTF8(raw)
	if err != nil {
		return fmt.Errorf("po: transcoding to UTF-8: %w", err)
	}
	// Detect CRLF line endings before normalising. Most parsers want LF
	// only; the writer re-emits CRLF when the source had it.
	r.crlfLineEndings = bytes.Contains(utf8Bytes, []byte("\r\n"))
	doc.Reader = io.NopCloser(bytes.NewReader(utf8Bytes))
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

// poEntry represents a single PO entry parsed from the file.
type poEntry struct {
	translatorComments []string // Lines starting with "# "
	extractedComments  []string // Lines starting with "#."
	references         []string // Lines starting with "#:"
	flags              []string // Lines starting with "#,"
	prevMsgid          string   // Lines starting with "#|"
	msgctxt            string
	msgid              string
	msgidPlural        string
	msgstr             string
	msgstrPlurals      map[int]string // msgstr[0], msgstr[1], ...
	isPlural           bool
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	targetLocale := r.Doc.TargetLocale

	// Emit layer start
	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "po",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-gettext-translation",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	if r.skeletonStore != nil {
		r.readContentSkeleton(ctx, ch, targetLocale)
	} else {
		r.readContentNormal(ctx, ch, targetLocale)
	}

	r.skelFlush()

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readContentNormal(ctx context.Context, ch chan<- model.PartResult, targetLocale model.LocaleID) {
	entries := r.parseEntries()

	blockID := 0
	dataID := 0

	for _, entry := range entries {
		// Header entry: empty msgid
		if entry.msgid == "" {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "header",
				Properties: map[string]string{
					"content": entry.msgstr,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		// Emit translator comments as Data
		if len(entry.translatorComments) > 0 {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "comment",
				Properties: map[string]string{
					"comment": strings.Join(entry.translatorComments, "\n"),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}

		// Emit references as Data
		if len(entry.references) > 0 {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "reference",
				Properties: map[string]string{
					"reference": strings.Join(entry.references, "\n"),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}

		// Emit flags as Data
		if len(entry.flags) > 0 {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "flags",
				Properties: map[string]string{
					"flags": strings.Join(entry.flags, ", "),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}

		if entry.isPlural {
			// Plural forms: emit as a group with multiple blocks
			blockID++
			groupID := fmt.Sprintf("g%d", blockID)
			gs := &model.GroupStart{
				ID:   groupID,
				Name: entry.msgid,
				Type: "plural",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: gs}) {
				return
			}

			// Singular block
			singularBlock := model.NewBlock(fmt.Sprintf("tu%d-singular", blockID), entry.msgid)
			singularBlock.Name = entry.msgid
			if entry.msgctxt != "" {
				singularBlock.Properties["context"] = entry.msgctxt
			}
			singularBlock.Properties["plural-form"] = "singular"
			if entry.msgstrPlurals != nil {
				if val, ok := entry.msgstrPlurals[0]; ok && val != "" && !targetLocale.IsEmpty() {
					singularBlock.SetTargetText(targetLocale, val)
				}
			}
			if r.cfg.UseCodeFinder {
				r.applyCodeFinder(singularBlock)
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: singularBlock}) {
				return
			}

			// Plural block
			pluralBlock := model.NewBlock(fmt.Sprintf("tu%d-plural", blockID), entry.msgidPlural)
			pluralBlock.Name = entry.msgidPlural
			if entry.msgctxt != "" {
				pluralBlock.Properties["context"] = entry.msgctxt
			}
			pluralBlock.Properties["plural-form"] = "plural"
			if entry.msgstrPlurals != nil {
				if val, ok := entry.msgstrPlurals[1]; ok && val != "" && !targetLocale.IsEmpty() {
					pluralBlock.SetTargetText(targetLocale, val)
				}
			}
			if r.cfg.UseCodeFinder {
				r.applyCodeFinder(pluralBlock)
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: pluralBlock}) {
				return
			}

			ge := &model.GroupEnd{ID: groupID}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: ge}) {
				return
			}
		} else {
			// Regular entry
			blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockID), entry.msgid)
			block.Name = entry.msgid
			if entry.msgctxt != "" {
				block.Properties["context"] = entry.msgctxt
			}
			if entry.msgstr != "" && !targetLocale.IsEmpty() {
				block.SetTargetText(targetLocale, entry.msgstr)
			}
			if r.cfg.UseCodeFinder {
				r.applyCodeFinder(block)
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}
}

// readContentSkeleton does a single-pass parse of the PO file, simultaneously
// building structured entries, emitting parts, and writing skeleton data.
// Non-translatable lines (comments, msgctxt, msgid, blank lines) become
// skeleton text. Each msgstr field becomes a skeleton ref whose ID is the
// block ID. The writer re-serializes the msgstr value when it encounters
// the ref.
func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult, targetLocale model.LocaleID) {
	scanner := bufio.NewScanner(r.Doc.Reader)

	// Re-emit the original UTF-8 BOM so the writer's output matches
	// BOM-prefixed source fixtures byte-for-byte. Direct skel buffer
	// write so the CRLF rewrite in skelText doesn't mangle the BOM.
	if r.hadUTF8BOM {
		r.skelBuf.WriteString("\ufeff")
	}

	type fieldType int
	const (
		fieldNone fieldType = iota
		fieldMsgctxt
		fieldMsgid
		fieldMsgidPlural
		fieldMsgstr
		fieldMsgstrPlural
	)

	// Collect entries first (with raw line tracking), then emit parts.
	// We need the full entry to know the block ID before we can write refs.
	//
	// Strategy: collect all raw lines grouped by entry. For each entry,
	// record which lines are msgstr lines. Then emit parts and skeleton
	// in a second pass over the collected data.

	type rawEntry struct {
		entry *poEntry
		lines []string // raw lines (without \n)
		// For each line, whether it belongs to a msgstr field.
		isMsgstr []bool
		// Which msgstr field (for plurals): -1 = regular msgstr, N = msgstr[N]
		msgstrIndex []int
		// Blank lines that precede this entry (separator from previous entry)
		leadingBlanks []string
	}

	var entries []*rawEntry
	var current *rawEntry
	currentField := fieldNone
	currentPluralIndex := 0
	var pendingBlanks []string

	newEntry := func() *rawEntry {
		re := &rawEntry{
			entry:         &poEntry{msgstrPlurals: make(map[int]string)},
			leadingBlanks: pendingBlanks,
		}
		pendingBlanks = nil
		return re
	}

	addLine := func(line string, isMsgstr bool, msgstrIdx int) {
		if current == nil {
			current = newEntry()
		}
		current.lines = append(current.lines, line)
		current.isMsgstr = append(current.isMsgstr, isMsgstr)
		current.msgstrIndex = append(current.msgstrIndex, msgstrIdx)
	}

	finishEntry := func() {
		if current != nil {
			entries = append(entries, current)
			current = nil
			currentField = fieldNone
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		if strings.TrimSpace(line) == "" {
			finishEntry()
			pendingBlanks = append(pendingBlanks, line)
			continue
		}

		// Comment lines
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#~") {
			if current == nil {
				current = newEntry()
			}
			addLine(line, false, -1)

			e := current.entry
			if strings.HasPrefix(line, "#:") {
				e.references = append(e.references, strings.TrimSpace(line[2:]))
			} else if strings.HasPrefix(line, "#.") {
				e.extractedComments = append(e.extractedComments, strings.TrimSpace(line[2:]))
			} else if strings.HasPrefix(line, "#,") {
				e.flags = append(e.flags, strings.TrimSpace(line[2:]))
			} else if strings.HasPrefix(line, "#|") {
				e.prevMsgid = strings.TrimSpace(line[2:])
			} else if strings.HasPrefix(line, "# ") || line == "#" {
				e.translatorComments = append(e.translatorComments, strings.TrimPrefix(line, "# "))
			}
			continue
		}

		if strings.HasPrefix(line, "msgctxt ") {
			if current == nil {
				current = newEntry()
			}
			current.entry.msgctxt = unquotePO(line[8:])
			currentField = fieldMsgctxt
			addLine(line, false, -1)
			continue
		}

		if strings.HasPrefix(line, "msgid_plural ") {
			if current == nil {
				current = newEntry()
			}
			current.entry.msgidPlural = unquotePO(line[13:])
			current.entry.isPlural = true
			currentField = fieldMsgidPlural
			addLine(line, false, -1)
			continue
		}

		if strings.HasPrefix(line, "msgid ") {
			if current == nil {
				current = newEntry()
			}
			current.entry.msgid = unquotePO(line[6:])
			currentField = fieldMsgid
			addLine(line, false, -1)
			continue
		}

		if strings.HasPrefix(line, "msgstr[") {
			if current == nil {
				current = newEntry()
			}
			closeBracket := strings.Index(line, "]")
			if closeBracket > 7 {
				n := 0
				_, _ = fmt.Sscanf(line[7:closeBracket], "%d", &n)
				val := unquotePO(strings.TrimSpace(line[closeBracket+1:]))
				current.entry.msgstrPlurals[n] = val
				currentPluralIndex = n
				currentField = fieldMsgstrPlural
				addLine(line, true, n)
			}
			continue
		}

		if strings.HasPrefix(line, "msgstr ") {
			if current == nil {
				current = newEntry()
			}
			current.entry.msgstr = unquotePO(line[7:])
			currentField = fieldMsgstr
			addLine(line, true, -1)
			continue
		}

		// Continuation line
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "\"") && current != nil {
			val := unquotePO(trimmed)
			switch currentField {
			case fieldMsgctxt:
				current.entry.msgctxt += val
				addLine(line, false, -1)
			case fieldMsgid:
				current.entry.msgid += val
				addLine(line, false, -1)
			case fieldMsgidPlural:
				current.entry.msgidPlural += val
				addLine(line, false, -1)
			case fieldMsgstr:
				current.entry.msgstr += val
				addLine(line, true, -1)
			case fieldMsgstrPlural:
				current.entry.msgstrPlurals[currentPluralIndex] += val
				addLine(line, true, currentPluralIndex)
			}
		}
	}
	finishEntry()

	// Second pass: emit parts and skeleton data.
	blockID := 0
	dataID := 0

	for _, re := range entries {
		entry := re.entry

		// Write leading blank lines as skeleton text
		for _, blank := range re.leadingBlanks {
			r.skelText(blank + "\n")
		}

		// Determine block ID for this entry (needed for ref IDs).
		// Header entries don't get a block ID.
		entryBlockID := 0
		if entry.msgid != "" {
			blockID++
			entryBlockID = blockID
		}

		// Write skeleton and emit parts.
		// For header entries, everything is skeleton text.
		if entry.msgid == "" {
			// Header: all lines as skeleton text
			for _, line := range re.lines {
				r.skelText(line + "\n")
			}
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "header",
				Properties: map[string]string{
					"content": entry.msgstr,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		// For regular/plural entries: non-msgstr lines are skeleton text,
		// msgstr lines are skeleton refs.
		//
		// We group consecutive msgstr lines into a single ref.
		// For regular entries: ref ID = "tu{N}"
		// For plural entries: ref ID = "tu{N}-singular" (msgstr[0]) or "tu{N}-plural" (msgstr[1])
		//
		// Also collect raw msgstr lines per field so the writer can
		// reproduce the exact formatting for byte-exact roundtrip.
		inRef := false
		lastMsgstrIdx := -2
		rawMsgstrLines := make(map[int][]string) // msgstrIndex -> raw lines

		for i, line := range re.lines {
			if re.isMsgstr[i] {
				msIdx := re.msgstrIndex[i]
				rawMsgstrLines[msIdx] = append(rawMsgstrLines[msIdx], line)
				if !inRef || msIdx != lastMsgstrIdx {
					var refID string
					if entry.isPlural {
						if msIdx == 0 {
							refID = fmt.Sprintf("tu%d-singular", entryBlockID)
						} else {
							refID = fmt.Sprintf("tu%d-plural", entryBlockID)
						}
					} else {
						refID = fmt.Sprintf("tu%d", entryBlockID)
					}
					r.skelRef(refID)
					inRef = true
					lastMsgstrIdx = msIdx
				}
			} else {
				inRef = false
				r.skelText(line + "\n")
			}
		}

		// Build raw msgstr field text for each field.
		// For regular entries: rawMsgstrLines[-1] has the lines.
		// For plural entries: rawMsgstrLines[0] and rawMsgstrLines[1].
		buildRawMsgstr := func(idx int) string {
			lines := rawMsgstrLines[idx]
			if len(lines) == 0 {
				return ""
			}
			var sb strings.Builder
			for i, l := range lines {
				if i > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(l)
			}
			sb.WriteByte('\n')
			return sb.String()
		}

		// Emit Data parts for metadata
		if len(entry.translatorComments) > 0 {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "comment",
				Properties: map[string]string{
					"comment": strings.Join(entry.translatorComments, "\n"),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
		if len(entry.references) > 0 {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "reference",
				Properties: map[string]string{
					"reference": strings.Join(entry.references, "\n"),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
		if len(entry.flags) > 0 {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "flags",
				Properties: map[string]string{
					"flags": strings.Join(entry.flags, ", "),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}

		// Emit block parts
		if entry.isPlural {
			groupID := fmt.Sprintf("g%d", entryBlockID)
			gs := &model.GroupStart{ID: groupID, Name: entry.msgid, Type: "plural"}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: gs}) {
				return
			}

			singularBlock := model.NewBlock(fmt.Sprintf("tu%d-singular", entryBlockID), entry.msgid)
			singularBlock.Name = entry.msgid
			if entry.msgctxt != "" {
				singularBlock.Properties["context"] = entry.msgctxt
			}
			singularBlock.Properties["plural-form"] = "singular"
			singularBlock.Properties["raw-msgstr"] = buildRawMsgstr(0)
			if entry.msgstrPlurals != nil {
				if val, ok := entry.msgstrPlurals[0]; ok && val != "" && !targetLocale.IsEmpty() {
					singularBlock.SetTargetText(targetLocale, val)
				}
			}
			if r.cfg.UseCodeFinder {
				r.applyCodeFinder(singularBlock)
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: singularBlock}) {
				return
			}

			pluralBlock := model.NewBlock(fmt.Sprintf("tu%d-plural", entryBlockID), entry.msgidPlural)
			pluralBlock.Name = entry.msgidPlural
			if entry.msgctxt != "" {
				pluralBlock.Properties["context"] = entry.msgctxt
			}
			pluralBlock.Properties["plural-form"] = "plural"
			pluralBlock.Properties["raw-msgstr"] = buildRawMsgstr(1)
			if entry.msgstrPlurals != nil {
				if val, ok := entry.msgstrPlurals[1]; ok && val != "" && !targetLocale.IsEmpty() {
					pluralBlock.SetTargetText(targetLocale, val)
				}
			}
			if r.cfg.UseCodeFinder {
				r.applyCodeFinder(pluralBlock)
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: pluralBlock}) {
				return
			}

			ge := &model.GroupEnd{ID: groupID}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: ge}) {
				return
			}
		} else {
			block := model.NewBlock(fmt.Sprintf("tu%d", entryBlockID), entry.msgid)
			block.Name = entry.msgid
			if entry.msgctxt != "" {
				block.Properties["context"] = entry.msgctxt
			}
			block.Properties["raw-msgstr"] = buildRawMsgstr(-1)
			if entry.msgstr != "" && !targetLocale.IsEmpty() {
				block.SetTargetText(targetLocale, entry.msgstr)
			}
			if r.cfg.UseCodeFinder {
				r.applyCodeFinder(block)
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}

	// Write any trailing blank lines
	for _, blank := range pendingBlanks {
		r.skelText(blank + "\n")
	}
}

// parseEntries reads the entire PO file and returns parsed entries.
func (r *Reader) parseEntries() []*poEntry {
	scanner := bufio.NewScanner(r.Doc.Reader)
	var entries []*poEntry
	var current *poEntry

	// State tracking for multiline strings
	type fieldType int
	const (
		fieldNone fieldType = iota
		fieldMsgctxt
		fieldMsgid
		fieldMsgidPlural
		fieldMsgstr
		fieldMsgstrPlural
	)
	currentField := fieldNone
	currentPluralIndex := 0

	finishEntry := func() {
		if current != nil {
			entries = append(entries, current)
			current = nil
			currentField = fieldNone
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Blank line: finish current entry
		if strings.TrimSpace(line) == "" {
			finishEntry()
			continue
		}

		// Comment lines
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#~") {
			if current == nil {
				current = &poEntry{msgstrPlurals: make(map[int]string)}
			}
			if strings.HasPrefix(line, "#:") {
				current.references = append(current.references, strings.TrimSpace(line[2:]))
			} else if strings.HasPrefix(line, "#.") {
				current.extractedComments = append(current.extractedComments, strings.TrimSpace(line[2:]))
			} else if strings.HasPrefix(line, "#,") {
				current.flags = append(current.flags, strings.TrimSpace(line[2:]))
			} else if strings.HasPrefix(line, "#|") {
				current.prevMsgid = strings.TrimSpace(line[2:])
			} else if strings.HasPrefix(line, "# ") || line == "#" {
				current.translatorComments = append(current.translatorComments, strings.TrimPrefix(line, "# "))
			}
			continue
		}

		// Keyword lines
		if strings.HasPrefix(line, "msgctxt ") {
			if current == nil {
				current = &poEntry{msgstrPlurals: make(map[int]string)}
			}
			current.msgctxt = unquotePO(line[8:])
			currentField = fieldMsgctxt
			continue
		}

		if strings.HasPrefix(line, "msgid_plural ") {
			if current == nil {
				current = &poEntry{msgstrPlurals: make(map[int]string)}
			}
			current.msgidPlural = unquotePO(line[13:])
			current.isPlural = true
			currentField = fieldMsgidPlural
			continue
		}

		if strings.HasPrefix(line, "msgid ") {
			if current == nil {
				current = &poEntry{msgstrPlurals: make(map[int]string)}
			}
			current.msgid = unquotePO(line[6:])
			currentField = fieldMsgid
			continue
		}

		if strings.HasPrefix(line, "msgstr[") {
			if current == nil {
				current = &poEntry{msgstrPlurals: make(map[int]string)}
			}
			// Parse msgstr[N]
			closeBracket := strings.Index(line, "]")
			if closeBracket > 7 {
				n := 0
				_, _ = fmt.Sscanf(line[7:closeBracket], "%d", &n)
				val := unquotePO(strings.TrimSpace(line[closeBracket+1:]))
				current.msgstrPlurals[n] = val
				currentPluralIndex = n
				currentField = fieldMsgstrPlural
			}
			continue
		}

		if strings.HasPrefix(line, "msgstr ") {
			if current == nil {
				current = &poEntry{msgstrPlurals: make(map[int]string)}
			}
			current.msgstr = unquotePO(line[7:])
			currentField = fieldMsgstr
			continue
		}

		// Continuation line: starts with a quoted string
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "\"") && current != nil {
			val := unquotePO(trimmed)
			switch currentField {
			case fieldMsgctxt:
				current.msgctxt += val
			case fieldMsgid:
				current.msgid += val
			case fieldMsgidPlural:
				current.msgidPlural += val
			case fieldMsgstr:
				current.msgstr += val
			case fieldMsgstrPlural:
				current.msgstrPlurals[currentPluralIndex] += val
			}
		}
	}

	// Don't forget the last entry
	finishEntry()

	return entries
}

// unquotePO strips surrounding quotes and processes escape sequences.
func unquotePO(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	s = s[1 : len(s)-1]

	var buf strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case '\\':
				buf.WriteByte('\\')
			case '"':
				buf.WriteByte('"')
			default:
				buf.WriteByte(s[i])
				buf.WriteByte(s[i+1])
			}
			i += 2
		} else {
			buf.WriteByte(s[i])
			i++
		}
	}
	return buf.String()
}

// skelText appends text to the skeleton buffer if active. When the
// source used CRLF line endings, every \n in the appended text is
// rewritten to \r\n so the writer's output matches Windows-authored or
// okapi-emitted .po files byte-for-byte.
func (r *Reader) skelText(s string) {
	if r.skeletonStore == nil || s == "" {
		return
	}
	if r.crlfLineEndings {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	r.skelBuf.WriteString(s)
}

// skelRef flushes buffered text and writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// applyCodeFinder applies code finder patterns to a block's source AND
// target segments. Splitting the target too matters when a downstream
// pseudo-translate or transform step prefers an existing target as its
// base (Okapi's TextModificationStep with applyToBlankEntries=true does
// exactly this) — without inline-code splits on the target, printf
// specifiers like `%s` in the existing translation would be treated as
// translatable text and corrupted.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.GetCodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}
	applyCodeFinderToSegments(block.Source, patterns)
	for _, segs := range block.Targets {
		applyCodeFinderToSegments(segs, patterns)
	}
}

// applyCodeFinderToSegments applies the patterns to each segment's
// TextRuns, splitting them at every match into text+placeholder runs.
// Existing non-text runs (placeholders, paired codes) are left in place.
func applyCodeFinderToSegments(segs []*model.Segment, patterns []*regexp.Regexp) {
	for _, seg := range segs {
		if seg == nil || len(seg.Runs) == 0 {
			continue
		}
		text := seg.Text()

		type matchRange struct {
			start, end int
		}
		var matches []matchRange
		for _, re := range patterns {
			for _, loc := range re.FindAllStringIndex(text, -1) {
				matches = append(matches, matchRange{loc[0], loc[1]})
			}
		}
		if len(matches) == 0 {
			continue
		}

		// Sort matches by start position
		for i := 1; i < len(matches); i++ {
			for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
				matches[j], matches[j-1] = matches[j-1], matches[j]
			}
		}

		var runs []model.Run
		lastEnd := 0
		spanID := 1
		for _, m := range matches {
			if m.start > lastEnd {
				runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:m.start]}})
			}
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID:   fmt.Sprintf("c%d", spanID),
				Type: "code",
				Data: text[m.start:m.end],
			}})
			lastEnd = m.end
			spanID++
		}
		if lastEnd < len(text) {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
		}
		seg.SetRuns(runs)
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
