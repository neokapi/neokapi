package sievepen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"github.com/neokapi/neokapi/core/model"
)

// --- Parsed TMX tree ---

type tmxDocument struct {
	Header tmxHeader
	TUs    []tmxTU
}

type tmxHeader struct {
	CreationTool        string
	CreationToolVersion string
	SegType             string
	AdminLang           string
	SrcLang             string
	DataType            string
	OriginalFormat      string // o-tmf
	OriginalEncoding    string // o-encoding
	Properties          map[string]string
}

type tmxTU struct {
	TUID       string
	SrcLang    string
	CreatedAt  string
	ChangedAt  string
	Properties map[string]string
	TUVs       []tmxTUV
}

type tmxTUV struct {
	Lang       string
	Runs       []model.Run
	Properties map[string]string
}

// --- Public import API ---

// ImportTMXOptions controls optional behavior of the TMX importer.
type ImportTMXOptions struct {
	// OriginKey identifies this import in Origin.Key (typically the filename).
	OriginKey string
	// OriginAddedBy is recorded in Origin.AddedBy.
	OriginAddedBy string
	// MappingPath is an optional override file for TMX element → vocabulary.
	MappingPath string
	// Locales restricts the set of variants that are kept from each TU.
	// An empty slice means "keep every TUV present".
	Locales []model.LocaleID
	// WarnFunc is called with a human-readable warning when a re-import is
	// detected (same sha256 as a previous session).
	WarnFunc func(msg string)
	// ImportedBy is recorded on the ImportSession.
	ImportedBy string
}

// ImportTMXWithOptions imports a TMX file into the TM, supports recording an Origin
// and filtering to a specific (src, tgt) bilingual pair when both locales
// are set. Other TUVs are dropped.
func ImportTMXWithOptions(tm TranslationMemory, reader io.Reader, sourceLocale, targetLocale model.LocaleID, opts ImportTMXOptions) (int, error) {
	store, ok := tm.(TMStore)
	if !ok {
		return 0, errors.New("TM does not support import sessions")
	}
	if sourceLocale != "" && targetLocale != "" {
		opts.Locales = []model.LocaleID{sourceLocale, targetLocale}
	}
	_, imported, err := ImportTMXSession(store, reader, opts)
	return imported, err
}

// ImportTMXLocalePairs reads a TMX file and imports TUs into the TM, keeping
// only the specified locales as variants. An empty locales slice keeps every
// TUV present.
//
// This is a multilingual import — each TU becomes one entry with N variants.
// The "locale pairs" in the name is a historical artifact; the old behavior
// of emitting a separate entry per (src, tgt) pair has been removed.
func ImportTMXLocalePairs(tm TranslationMemory, reader io.Reader, locales []model.LocaleID, opts ImportTMXOptions) (int, error) {
	store, ok := tm.(TMStore)
	if !ok {
		return 0, errors.New("TM does not support import sessions")
	}
	opts.Locales = locales
	_, imported, err := ImportTMXSession(store, reader, opts)
	return imported, err
}

// ImportTMXSession reads a TMX file, creates an ImportSession row for it,
// and inserts one multilingual entry per TU. Returns the session ID and the
// number of entries imported.
//
// When the file SHA-256 matches a previous session's hash, opts.WarnFunc is
// invoked but the import proceeds — a new session is created so that the
// caller can distinguish re-imports.
func ImportTMXSession(store TMStore, reader io.Reader, opts ImportTMXOptions) (string, int, error) {
	// 1. Read everything so we can hash and rewind.
	buf, err := io.ReadAll(reader)
	if err != nil {
		return "", 0, fmt.Errorf("read TMX: %w", err)
	}
	hashBytes := sha256.Sum256(buf)
	hash := hex.EncodeToString(hashBytes[:])

	// 2. Warn on duplicate hash.
	if opts.WarnFunc != nil {
		if prev, ok := store.FindImportSessionByHash(hash); ok {
			opts.WarnFunc(fmt.Sprintf(
				"file hash %s was previously imported as session %s (%d entries, imported %s)",
				hash[:16], prev.ID, prev.EntryCount, prev.ImportedAt.Format(time.RFC3339)))
		}
	}

	// 3. Load mapping (embedded default + user override).
	mapping, err := LoadTMXMapping(opts.MappingPath)
	if err != nil {
		return "", 0, err
	}

	// 4. Parse TMX document.
	doc, err := parseTMXDocument(bytes.NewReader(buf), mapping)
	if err != nil {
		return "", 0, err
	}

	// 5. Build and persist the ImportSession.
	addedBy := opts.OriginAddedBy
	if addedBy == "" {
		addedBy = "tmx-import"
	}
	importedAt := time.Now()
	session := ImportSession{
		ID:               newSessionID(hash, importedAt),
		FileKey:          opts.OriginKey,
		FileHash:         hash,
		FileSizeBytes:    int64(len(buf)),
		ImportedAt:       importedAt,
		ImportedBy:       opts.ImportedBy,
		ToolName:         doc.Header.CreationTool,
		ToolVersion:      doc.Header.CreationToolVersion,
		SegType:          doc.Header.SegType,
		AdminLang:        doc.Header.AdminLang,
		SrcLang:          doc.Header.SrcLang,
		DataType:         doc.Header.DataType,
		OriginalFormat:   doc.Header.OriginalFormat,
		OriginalEncoding: doc.Header.OriginalEncoding,
		Properties:       doc.Header.Properties,
	}
	if session.FileKey == "" {
		session.FileKey = "tmx-import"
	}
	if err := store.CreateImportSession(session); err != nil {
		return "", 0, err
	}

	// 6. Build locale filter (empty = keep all).
	localeAllowed := make(map[model.LocaleID]bool, len(opts.Locales))
	for _, l := range opts.Locales {
		localeAllowed[l] = true
	}
	filterAll := len(localeAllowed) == 0

	// 7. Walk TUs, build one multilingual entry per TU.
	entries := make([]TMEntry, 0, len(doc.TUs))
	for i, tu := range doc.TUs {
		variants := make(map[model.LocaleID][]model.Run)
		var hintLang model.LocaleID
		if tu.SrcLang != "" {
			hintLang = model.LocaleID(tu.SrcLang)
		} else if doc.Header.SrcLang != "" {
			hintLang = model.LocaleID(doc.Header.SrcLang)
		}

		var firstSrcRef string
		for _, tuv := range tu.TUVs {
			if tuv.Runs == nil {
				continue
			}
			loc := model.LocaleID(tuv.Lang)
			if !filterAll && !localeAllowed[loc] {
				continue
			}
			if _, exists := variants[loc]; exists {
				continue
			}
			variants[loc] = tuv.Runs
			if firstSrcRef == "" {
				if ref := tuv.Properties["source-document"]; ref != "" && loc == hintLang {
					firstSrcRef = ref
				}
			}
		}
		if len(variants) == 0 {
			continue
		}

		// Merge TU-level props into entry-level props.
		props := make(map[string]string)
		for k, v := range tu.Properties {
			props[k] = v
		}

		entryID := scopedTUID(opts.OriginKey, tu.TUID, i, session.ID)

		origin := Origin{
			Source:    "import",
			Key:       opts.OriginKey,
			AddedAt:   importedAt,
			AddedBy:   addedBy,
			SessionID: session.ID,
		}
		if firstSrcRef != "" {
			origin.Reference = firstSrcRef
		}

		createdAt := parseTMXTime(tu.CreatedAt)
		updatedAt := parseTMXTime(tu.ChangedAt)
		if updatedAt.IsZero() {
			updatedAt = createdAt
		}
		if createdAt.IsZero() {
			createdAt = importedAt
		}
		if updatedAt.IsZero() {
			updatedAt = importedAt
		}

		entries = append(entries, TMEntry{
			ID:          entryID,
			Variants:    variants,
			HintSrcLang: hintLang,
			Properties:  props,
			Origins:     []Origin{origin},
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		})
	}

	// 8. Persist. Use BulkAddWithStream when available — this collapses
	//    thousands of per-entry commits into a single transaction and is
	//    the only way TMX imports of large corpora finish in a reasonable
	//    amount of time.
	imported := 0
	if bulk, ok := store.(BulkAdder); ok {
		if err := bulk.BulkAddWithStream(entries, ""); err != nil {
			return session.ID, 0, err
		}
		imported = len(entries)
	} else {
		for _, entry := range entries {
			if err := store.Add(entry); err != nil {
				return session.ID, imported, fmt.Errorf("add entry %s: %w", entry.ID, err)
			}
			imported++
		}
	}

	if err := store.UpdateImportSessionCount(session.ID, imported); err != nil {
		return session.ID, imported, err
	}
	return session.ID, imported, nil
}

func newSessionID(hash string, t time.Time) string {
	return fmt.Sprintf("tmx-%s-%d", hash[:16], t.UnixNano())
}

func scopedTUID(originKey, tuid string, index int, sessionID string) string {
	base := tuid
	if base == "" {
		base = fmt.Sprintf("tu-%d", index+1)
	}
	if originKey != "" {
		base = originKey + ":" + base
	}
	if sessionID != "" {
		base = base + "@" + sessionID[:min(12, len(sessionID))]
	}
	return base
}

// --- TMX parser ---

// newTMXDecoder builds an XML decoder that handles non-UTF-8 encodings
// (notably UTF-16, commonly used by Euramis/EUR-Lex TMX exports).
func newTMXDecoder(reader io.Reader) (*xml.Decoder, error) {
	utf8Reader, err := charset.NewReader(reader, "")
	if err != nil {
		return nil, fmt.Errorf("detect TMX charset: %w", err)
	}
	dec := xml.NewDecoder(utf8Reader)
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	return dec, nil
}

func parseTMXDocument(r io.Reader, mapping *TMXMapping) (*tmxDocument, error) {
	dec, err := newTMXDecoder(r)
	if err != nil {
		return nil, err
	}
	doc := &tmxDocument{}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return doc, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse TMX: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "header":
			if err := parseTMXHeader(dec, start, doc); err != nil {
				return nil, err
			}
		case "tu":
			tu, err := parseTMXTU(dec, start, mapping)
			if err != nil {
				return nil, err
			}
			doc.TUs = append(doc.TUs, tu)
		}
	}
}

func parseTMXHeader(dec *xml.Decoder, start xml.StartElement, doc *tmxDocument) error {
	h := &doc.Header
	h.Properties = make(map[string]string)
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "creationtool":
			h.CreationTool = a.Value
		case "creationtoolversion":
			h.CreationToolVersion = a.Value
		case "segtype":
			h.SegType = a.Value
		case "adminlang":
			h.AdminLang = a.Value
		case "srclang":
			h.SrcLang = a.Value
		case "datatype":
			h.DataType = a.Value
		case "o-tmf":
			h.OriginalFormat = a.Value
		case "o-encoding":
			h.OriginalEncoding = a.Value
		}
	}
	// Walk header children until </header>.
	for {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("parse header: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "prop" {
				propType := findAttr(t.Attr, "type")
				val, err := decodeText(dec, t.Name.Local)
				if err != nil {
					return err
				}
				if propType != "" {
					h.Properties[propType] = val
				}
			} else {
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "header" {
				return nil
			}
		}
	}
}

func parseTMXTU(dec *xml.Decoder, start xml.StartElement, mapping *TMXMapping) (tmxTU, error) {
	tu := tmxTU{Properties: make(map[string]string)}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "tuid":
			tu.TUID = a.Value
		case "srclang":
			tu.SrcLang = a.Value
		case "creationdate":
			tu.CreatedAt = a.Value
		case "changedate":
			tu.ChangedAt = a.Value
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return tu, fmt.Errorf("parse tu: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "prop":
				propType := findAttr(t.Attr, "type")
				val, err := decodeText(dec, "prop")
				if err != nil {
					return tu, err
				}
				if propType != "" {
					tu.Properties[propType] = val
				}
			case "tuv":
				tuv, err := parseTMXTUV(dec, t, mapping)
				if err != nil {
					return tu, err
				}
				tu.TUVs = append(tu.TUVs, tuv)
			default:
				if err := dec.Skip(); err != nil {
					return tu, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "tu" {
				return tu, nil
			}
		}
	}
}

func parseTMXTUV(dec *xml.Decoder, start xml.StartElement, mapping *TMXMapping) (tmxTUV, error) {
	tuv := tmxTUV{Properties: make(map[string]string)}
	for _, a := range start.Attr {
		if (a.Name.Space == "http://www.w3.org/XML/1998/namespace" || a.Name.Space == "xml") && a.Name.Local == "lang" {
			tuv.Lang = a.Value
		} else if a.Name.Local == "lang" && tuv.Lang == "" {
			tuv.Lang = a.Value
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return tuv, fmt.Errorf("parse tuv: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "prop":
				propType := findAttr(t.Attr, "type")
				val, err := decodeText(dec, "prop")
				if err != nil {
					return tuv, err
				}
				if propType != "" {
					tuv.Properties[propType] = val
				}
			case "seg":
				runs, err := parseTMXSeg(dec, mapping)
				if err != nil {
					return tuv, err
				}
				tuv.Runs = runs
			default:
				if err := dec.Skip(); err != nil {
					return tuv, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "tuv" {
				return tuv, nil
			}
		}
	}
}

// parseTMXSeg walks the token stream for the current <seg> element and
// builds a Run sequence. Handles ph, bpt, ept, it, hi, sub, ut. Any
// unknown child element is round-tripped as a raw placeholder so that no
// content is silently lost.
//
// Always returns a non-nil slice (possibly empty) so callers can tell
// "seg seen but empty" apart from "seg not seen at all".
func parseTMXSeg(dec *xml.Decoder, mapping *TMXMapping) ([]model.Run, error) {
	b := &runBuilder{}
	// pairedIDs maps a bpt `i` attr value to the resolved semantic type so
	// that the matching ept picks up the same type.
	pairedIDs := make(map[string]string)
	var spanCounter int
	nextID := func() string {
		spanCounter++
		return "s" + strconv.Itoa(spanCounter)
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parse seg: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.AddText(string(t))
		case xml.EndElement:
			if t.Name.Local == "seg" {
				return b.Runs(), nil
			}
		case xml.StartElement:
			switch t.Name.Local {
			case "ph":
				if err := handlePH(dec, t, b, mapping, nextID); err != nil {
					return nil, err
				}
			case "bpt":
				if err := handleBPT(dec, t, b, mapping, pairedIDs, nextID); err != nil {
					return nil, err
				}
			case "ept":
				if err := handleEPT(dec, t, b, mapping, pairedIDs, nextID); err != nil {
					return nil, err
				}
			case "it":
				if err := handleIT(dec, t, b, mapping, nextID); err != nil {
					return nil, err
				}
			case "hi":
				if err := handleHI(dec, t, b, mapping, nextID); err != nil {
					return nil, err
				}
			case "sub":
				if err := handleSUB(dec, t, b, mapping, nextID); err != nil {
					return nil, err
				}
			case "ut":
				if err := handleUT(dec, t, b, mapping, nextID); err != nil {
					return nil, err
				}
			default:
				// Unknown element — round-trip as raw placeholder.
				raw, err := captureRawXML(dec, t)
				if err != nil {
					return nil, err
				}
				b.AddPh(nextID(), mapping.Fallback, "tmx:raw", raw)
			}
		}
	}
}

func handlePH(dec *xml.Decoder, start xml.StartElement, b *runBuilder, mapping *TMXMapping, nextID func() string) error {
	typeAttr := findAttr(start.Attr, "type")
	xid := findAttr(start.Attr, "x")
	data, err := decodeText(dec, "ph")
	if err != nil {
		return err
	}
	id := xid
	if id == "" {
		id = nextID()
	}
	b.AddPh(id, mapping.Resolve("ph", typeAttr), "tmx:ph", data)
	return nil
}

func handleBPT(dec *xml.Decoder, start xml.StartElement, b *runBuilder, mapping *TMXMapping, pairedIDs map[string]string, nextID func() string) error {
	typeAttr := findAttr(start.Attr, "type")
	iAttr := findAttr(start.Attr, "i")
	data, err := decodeText(dec, "bpt")
	if err != nil {
		return err
	}
	semType := mapping.Resolve("bpt", typeAttr)
	if iAttr != "" {
		pairedIDs[iAttr] = semType
	}
	id := iAttr
	if id == "" {
		id = nextID()
	}
	b.AddPcOpen(id, semType, "tmx:bpt", data)
	return nil
}

func handleEPT(dec *xml.Decoder, start xml.StartElement, b *runBuilder, mapping *TMXMapping, pairedIDs map[string]string, nextID func() string) error {
	iAttr := findAttr(start.Attr, "i")
	data, err := decodeText(dec, "ept")
	if err != nil {
		return err
	}
	semType := ""
	if iAttr != "" {
		if t, ok := pairedIDs[iAttr]; ok {
			semType = t
			delete(pairedIDs, iAttr)
		}
	}
	if semType == "" {
		semType = mapping.Resolve("ept", "")
	}
	id := iAttr
	if id == "" {
		id = nextID()
	}
	b.AddPcClose(id, semType, "tmx:ept", data)
	return nil
}

func handleIT(dec *xml.Decoder, start xml.StartElement, b *runBuilder, mapping *TMXMapping, nextID func() string) error {
	typeAttr := findAttr(start.Attr, "type")
	pos := findAttr(start.Attr, "pos")
	data, err := decodeText(dec, "it")
	if err != nil {
		return err
	}
	semType := mapping.Resolve("it", typeAttr)
	id := nextID()
	if pos == "end" {
		b.AddPcClose(id, semType, "tmx:it", data)
	} else {
		b.AddPcOpen(id, semType, "tmx:it", data)
	}
	return nil
}

func handleHI(dec *xml.Decoder, start xml.StartElement, b *runBuilder, mapping *TMXMapping, nextID func() string) error {
	typeAttr := findAttr(start.Attr, "type")
	semType := mapping.Resolve("hi", typeAttr)
	id := nextID()
	b.AddPcOpen(id, semType, "tmx:hi", "")
	// Recursively parse children (text + inline elements).
	for {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("parse hi: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.AddText(string(t))
		case xml.EndElement:
			if t.Name.Local == "hi" {
				b.AddPcClose(id, semType, "tmx:hi", "")
				return nil
			}
		case xml.StartElement:
			switch t.Name.Local {
			case "ph":
				if err := handlePH(dec, t, b, mapping, nextID); err != nil {
					return err
				}
			case "bpt":
				if err := handleBPT(dec, t, b, mapping, map[string]string{}, nextID); err != nil {
					return err
				}
			case "ept":
				if err := handleEPT(dec, t, b, mapping, map[string]string{}, nextID); err != nil {
					return err
				}
			case "it":
				if err := handleIT(dec, t, b, mapping, nextID); err != nil {
					return err
				}
			case "hi":
				if err := handleHI(dec, t, b, mapping, nextID); err != nil {
					return err
				}
			case "sub":
				if err := handleSUB(dec, t, b, mapping, nextID); err != nil {
					return err
				}
			case "ut":
				if err := handleUT(dec, t, b, mapping, nextID); err != nil {
					return err
				}
			default:
				raw, err := captureRawXML(dec, t)
				if err != nil {
					return err
				}
				b.AddPh(nextID(), mapping.Fallback, "tmx:raw", raw)
			}
		}
	}
}

func handleSUB(dec *xml.Decoder, start xml.StartElement, b *runBuilder, mapping *TMXMapping, nextID func() string) error {
	raw, err := captureRawXML(dec, start)
	if err != nil {
		return err
	}
	typeAttr := findAttr(start.Attr, "type")
	b.AddPh(nextID(), mapping.Resolve("sub", typeAttr), "tmx:sub", raw)
	return nil
}

func handleUT(dec *xml.Decoder, start xml.StartElement, b *runBuilder, mapping *TMXMapping, nextID func() string) error {
	typeAttr := findAttr(start.Attr, "type")
	data, err := decodeText(dec, "ut")
	if err != nil {
		return err
	}
	b.AddPh(nextID(), mapping.Resolve("ut", typeAttr), "tmx:ut", data)
	return nil
}

// decodeText collects text content (including nested element text) until
// the matching end element is consumed. Nested elements are expanded to
// their text only — sufficient for leaf elements like ph/bpt/ept/ut.
func decodeText(dec *xml.Decoder, endName string) (string, error) {
	var b strings.Builder
	depth := 1
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("decode text for %s: %w", endName, err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 && t.Name.Local == endName {
				return b.String(), nil
			}
		}
	}
}

// captureRawXML re-serializes the current element (already seen as StartElement)
// and all its children up to the matching EndElement back into an XML string.
// Used for sub and unknown element round-tripping.
func captureRawXML(dec *xml.Decoder, start xml.StartElement) (string, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	if err := enc.EncodeToken(start); err != nil {
		return "", err
	}
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("capture raw: %w", err)
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
		if err := enc.EncodeToken(tok); err != nil {
			return "", err
		}
	}
	if err := enc.Flush(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func findAttr(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// parseTMXTime attempts to parse a TMX date string (YYYYMMDDTHHmmssZ).
func parseTMXTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse("20060102T150405Z", s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
