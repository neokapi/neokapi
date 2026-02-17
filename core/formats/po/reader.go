package po

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for PO (gettext) files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new PO reader.
func NewReader() *Reader {
	cfg := &Config{PreserveUntranslated: true}
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
		return fmt.Errorf("po: nil document or reader")
	}
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
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
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

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
