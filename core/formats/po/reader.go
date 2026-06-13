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
	"github.com/neokapi/neokapi/core/safeio"
)

// defaultNPlurals is the plural-form count assumed when the header
// declares none (or declares an unparseable one). Mirrors Okapi's
// POFilter.DEFAULT_NPLURALS = 2 ("Germanic languages" per the gettext
// docs: a singular form and a plural form).
const defaultNPlurals = 2

// Reader implements DataFormatReader for PO (gettext) files.
type Reader struct {
	format.BaseFormatReader
	cfg             *Config
	skeletonStore   *format.SkeletonStore
	skelBuf         bytes.Buffer // coalesces skeleton text between refs
	hadUTF8BOM      bool         // input started with EF BB BF
	crlfLineEndings bool         // input used \r\n (Windows / okapi-emitted) line endings
	nPlurals        int          // plural-form count from the header's Plural-Forms (default 2)
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
	// Bound the whole-input read with the shared safeio byte budget so an
	// unbounded/oversized stream fails with a typed error (identical limit
	// across CLI/server/WASM — see core/safeio).
	raw, err := io.ReadAll(safeio.DefaultBudget().Reader(doc.Reader))
	if err != nil {
		return fmt.Errorf("po: reading: %w", err)
	}
	r.hadUTF8BOM = len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF
	utf8Bytes, _, err := coreenc.ToUTF8(raw)
	if err != nil {
		return fmt.Errorf("po: transcoding to UTF-8: %w", err)
	}
	// If there was no BOM, peek at the header's `Content-Type:
	// charset=...` declaration and transcode if it isn't UTF-8.
	// The charset line uses ASCII bytes only, so it decodes the
	// same in UTF-8 / windows-1252 / ISO-8859-X.
	if charset := detectHeaderCharset(raw); charset != "" && !isUTF8Charset(charset) {
		em := coreenc.NewEncoderManager()
		decoded, derr := em.Decode(raw, charset)
		if derr == nil {
			utf8Bytes = []byte(decoded)
		}
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
	// Default plural-form count until a header declares otherwise.
	r.nPlurals = defaultNPlurals

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
			// The header's Plural-Forms declaration drives how many
			// plural blocks subsequent plural entries expose.
			r.nPlurals = pluralFormsFromHeader(entry.msgstr)
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
			// Plural forms: emit as a group with one block per plural
			// form declared by the header's nplurals (default 2).
			// Okapi's POFilter surfaces a text unit for every msgstr[N]
			// up to nplurals — form 0 carries the singular msgid, every
			// later form carries the msgid_plural (see testThreePlurals).
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

			for _, pf := range r.pluralForms(entry, blockID) {
				block := r.newPluralBlock(entry, pf, targetLocale)
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}

			ge := &model.GroupEnd{ID: groupID}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: ge}) {
				return
			}
		} else {
			// Regular entry
			blockID++
			source := entry.msgid
			target := entry.msgstr
			// Monolingual mode: msgid is an identifier, msgstr supplies
			// the source text (Okapi's okf_po-monolingual configuration).
			if !r.cfg.BilingualMode {
				source = entry.msgstr
				target = ""
			}
			block := model.NewBlock(fmt.Sprintf("tu%d", blockID), source)
			block.Name = entry.msgid
			if entry.msgctxt != "" {
				block.Properties["context"] = entry.msgctxt
			}
			if target != "" && !targetLocale.IsEmpty() {
				block.SetTargetText(targetLocale, target)
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

// pluralForm describes one plural-form block to emit for a plural entry.
type pluralForm struct {
	index    int    // msgstr[N] index
	id       string // block ID
	source   string // source text (msgid for index 0, msgid_plural otherwise)
	formName string // "singular" or "plural" — the plural-form property value
}

// pluralBlockID returns the deterministic block/skeleton-ref ID for the
// plural form at msgstr index idx within entry blockID. Index 0 is the
// singular form; later indices are plural forms (the second form keeps
// the legacy `-plural` suffix, further forms add their index so IDs stay
// unique for languages declaring more than two forms, e.g. Russian).
func pluralBlockID(blockID, idx int) string {
	if idx == 0 {
		return fmt.Sprintf("tu%d-singular", blockID)
	}
	if idx == 1 {
		return fmt.Sprintf("tu%d-plural", blockID)
	}
	return fmt.Sprintf("tu%d-plural%d", blockID, idx)
}

// pluralForms enumerates the plural-form blocks for a plural entry,
// one per plural form declared by the header's nplurals (default 2).
// Form 0 carries the singular msgid; every later form carries the
// msgid_plural — matching Okapi's POFilter, which sets the source to
// msgID for plural index 0 and to msgIDPlural for index > 0.
func (r *Reader) pluralForms(entry *poEntry, blockID int) []pluralForm {
	n := r.nPlurals
	if n < 1 {
		n = defaultNPlurals
	}
	forms := make([]pluralForm, 0, n)
	for i := range n {
		source := entry.msgidPlural
		formName := "plural"
		if i == 0 {
			source = entry.msgid
			formName = "singular"
		}
		forms = append(forms, pluralForm{
			index:    i,
			id:       pluralBlockID(blockID, i),
			source:   source,
			formName: formName,
		})
	}
	return forms
}

// newPluralBlock builds the Block for one plural form. The target for
// form N is msgstr[N]; the block carries its plural-form name and
// msgctxt context (when present).
func (r *Reader) newPluralBlock(entry *poEntry, pf pluralForm, targetLocale model.LocaleID) *model.Block {
	block := model.NewBlock(pf.id, pf.source)
	block.Name = pf.source
	if entry.msgctxt != "" {
		block.Properties["context"] = entry.msgctxt
	}
	block.Properties["plural-form"] = pf.formName
	if entry.msgstrPlurals != nil {
		if val, ok := entry.msgstrPlurals[pf.index]; ok && val != "" && !targetLocale.IsEmpty() {
			block.SetTargetText(targetLocale, val)
		}
	}
	if r.cfg.UseCodeFinder {
		r.applyCodeFinder(block)
	}
	return block
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

		// Obsolete entries (`#~ msgid` / `#~ msgstr`): pass through as
		// raw lines so the writer emits them verbatim. They aren't
		// translated and don't drive parts emission, but they need to
		// survive the round-trip to match okapi behaviour.
		if strings.HasPrefix(line, "#~") {
			if current == nil {
				current = newEntry()
			}
			addLine(line, false, -1)
			continue
		}

		// `domain` directive: a non-translatable directive that names
		// the message domain. Pass it through as raw skeleton text; it
		// has no msgid/msgstr to extract.
		if strings.HasPrefix(line, "domain ") {
			if current == nil {
				current = newEntry()
			}
			addLine(line, false, -1)
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
		// For header entries (msgid "" + non-empty msgstr): write the
		// `msgid ""` line as skeleton text, then canonicalise the
		// msgstr block to okapi's preferred form (`msgstr ""` start +
		// one continuation per header field) so source files that put
		// the first field on the msgstr line still round-trip to the
		// canonical layout. Entries that just look header-like but have
		// no real msgstr (e.g. obsolete-only fixtures whose entire
		// content is `#~` lines) fall through to the verbatim path.
		if entry.msgid == "" && entry.msgstr != "" {
			// The header's Plural-Forms declaration drives how many
			// plural blocks subsequent plural entries expose.
			r.nPlurals = pluralFormsFromHeader(entry.msgstr)
			for i, line := range re.lines {
				if re.isMsgstr[i] {
					continue
				}
				r.skelText(line + "\n")
			}
			r.skelText("msgstr \"\"\n")
			// okapi rewrites `Content-Type: text/plain; charset=...` to
			// `charset=UTF-8` because the reader transcodes input to
			// UTF-8 unconditionally. Mirror that so windows-1252 / etc.
			// fixtures round-trip with the declared charset matching
			// the actual byte encoding.
			headerContent := rewriteHeaderCharset(entry.msgstr)
			for hdrLine := range strings.SplitSeq(strings.TrimRight(headerContent, "\n"), "\n") {
				escaped := strings.ReplaceAll(hdrLine, `\`, `\\`)
				escaped = strings.ReplaceAll(escaped, `"`, `\"`)
				r.skelText("\"" + escaped + "\\n\"\n")
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
		if entry.msgid == "" {
			// msgid "" but no msgstr content: pass raw lines through.
			for _, line := range re.lines {
				r.skelText(line + "\n")
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
						refID = pluralBlockID(entryBlockID, msIdx)
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

			// One block per plural form declared by the header's
			// nplurals (default 2), matching Okapi's testThreePlurals.
			for _, pf := range r.pluralForms(entry, entryBlockID) {
				block := r.newPluralBlock(entry, pf, targetLocale)
				block.Properties["raw-msgstr"] = buildRawMsgstr(pf.index)
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}

			ge := &model.GroupEnd{ID: groupID}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: ge}) {
				return
			}
		} else {
			source := entry.msgid
			target := entry.msgstr
			// Monolingual mode: msgid is an identifier, msgstr supplies
			// the source text (Okapi's okf_po-monolingual configuration).
			if !r.cfg.BilingualMode {
				source = entry.msgstr
				target = ""
			}
			block := model.NewBlock(fmt.Sprintf("tu%d", entryBlockID), source)
			block.Name = entry.msgid
			if entry.msgctxt != "" {
				block.Properties["context"] = entry.msgctxt
			}
			block.Properties["raw-msgstr"] = buildRawMsgstr(-1)
			if target != "" && !targetLocale.IsEmpty() {
				block.SetTargetText(targetLocale, target)
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

// charsetHeaderRE matches the Content-Type charset declaration in a PO header.
var charsetHeaderRE = regexp.MustCompile(`(?i)(charset=)[^\s\\]+`)

// charsetValueRE captures only the charset value (no `charset=` prefix).
var charsetValueRE = regexp.MustCompile(`(?i)charset=([A-Za-z0-9_\-:.]+)`)

// npluralsRE captures the integer in a `Plural-Forms: nplurals=N; ...`
// header. Mirrors Okapi's npluralsPattern
// (`nplurals(\s*)(=)(\s*)(\d*)(;|\\n|\z)`); we capture the digits only.
var npluralsRE = regexp.MustCompile(`(?i)nplurals\s*=\s*(\d+)`)

// pluralFormsFromHeader extracts the declared nplurals count from a PO
// header msgstr value. Returns defaultNPlurals when the header has no
// (or an unparseable) nplurals field, matching Okapi's fallback.
func pluralFormsFromHeader(header string) int {
	m := npluralsRE.FindStringSubmatch(header)
	if m == nil {
		return defaultNPlurals
	}
	n := 0
	if _, err := fmt.Sscanf(m[1], "%d", &n); err != nil || n < 1 {
		return defaultNPlurals
	}
	return n
}

// detectHeaderCharset peeks at raw PO bytes for a `Content-Type:
// charset=<name>` declaration and returns the charset value, or an
// empty string when none is found. The charset declaration is ASCII,
// so it can be safely scanned before transcoding the rest of the file.
func detectHeaderCharset(raw []byte) string {
	// Limit the scan to the first ~4 KiB — the header always sits at
	// the top of the file and we don't want a charset-shaped substring
	// in a translation to influence decoding.
	limit := min(len(raw), 4096)
	m := charsetValueRE.FindSubmatch(raw[:limit])
	if m == nil {
		return ""
	}
	return string(m[1])
}

// isUTF8Charset reports whether a charset name maps to UTF-8 (the
// default we already produce). Compares case-insensitively and
// tolerates the common aliases (utf8, utf-8, UTF_8).
func isUTF8Charset(name string) bool {
	n := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(name))
	return n == "utf8"
}

// rewriteHeaderCharset rewrites the Content-Type charset declaration in a
// PO header to UTF-8. The reader transcodes all input to UTF-8 via
// coreenc.ToUTF8, so the on-disk byte encoding no longer matches whatever
// the source declared (e.g. windows-1252, ISO-8859-1). okapi makes the
// same rewrite when its filter normalises encodings on read.
func rewriteHeaderCharset(s string) string {
	return charsetHeaderRE.ReplaceAllString(s, "${1}UTF-8")
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
			case 'r':
				buf.WriteByte('\r')
			case 'a':
				buf.WriteByte('\a')
			case 'b':
				buf.WriteByte('\b')
			case 'f':
				buf.WriteByte('\f')
			case 'v':
				buf.WriteByte('\v')
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
	patterns := r.cfg.CodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}
	block.SetSourceRuns(applyCodeFinderToRuns(block.Source, patterns))
	for _, loc := range block.TargetLocales() {
		block.SetTargetRuns(loc, applyCodeFinderToRuns(block.TargetRuns(loc), patterns))
	}
}

// applyCodeFinderToSegments applies the patterns to each segment's
// TextRuns, splitting them at every match into text+placeholder runs.
// Existing non-text runs (placeholders, paired codes) are left in place.
func applyCodeFinderToRuns(in []model.Run, patterns []*regexp.Regexp) []model.Run {
	if len(in) == 0 {
		return in
	}
	text := model.RunsText(in)

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
		return in
	}

	// Sort matches by start position.
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
	return runs
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
