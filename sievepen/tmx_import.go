package sievepen

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"github.com/neokapi/neokapi/core/model"
)

// TMX XML structures for parsing.
type tmxDocument struct {
	XMLName xml.Name  `xml:"tmx"`
	Header  tmxHeader `xml:"header"`
	Body    tmxBody   `xml:"body"`
}

type tmxHeader struct {
	CreationTool        string `xml:"creationtool,attr"`
	CreationToolVersion string `xml:"creationtoolversion,attr"`
	SegType             string `xml:"segtype,attr"`
	AdminLang           string `xml:"adminlang,attr"`
	SrcLang             string `xml:"srclang,attr"`
	DataType            string `xml:"datatype,attr"`
}

type tmxBody struct {
	TUs []tmxTU `xml:"tu"`
}

type tmxTU struct {
	TUID       string    `xml:"tuid,attr"`
	SrcLang    string    `xml:"srclang,attr"`
	CreatedAt  string    `xml:"creationdate,attr"`
	ChangedAt  string    `xml:"changedate,attr"`
	Properties []tmxProp `xml:"prop"`
	TUVs       []tmxTUV  `xml:"tuv"`
}

type tmxProp struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type tmxTUV struct {
	Lang       string    `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Seg        string    `xml:"seg"`
	Properties []tmxProp `xml:"prop"`
}

// propValue returns the first matching property value from a slice, or "".
func propValue(props []tmxProp, key string) string {
	for _, p := range props {
		if p.Type == key {
			return p.Value
		}
	}
	return ""
}

// ImportTMXOptions controls optional behavior of the TMX importer.
type ImportTMXOptions struct {
	// OriginKey, when non-empty, is recorded on each imported entry as an
	// Origin with source="import" — e.g. the TMX filename or a relative path.
	OriginKey string
	// OriginAddedBy identifies the agent performing the import (tool name,
	// user ID, etc.). Defaults to "tmx-import" when OriginKey is set and
	// OriginAddedBy is empty.
	OriginAddedBy string
}

// newTMXDecoder builds an XML decoder that handles non-UTF-8 encodings
// (notably UTF-16, commonly used by Euramis/EUR-Lex TMX exports).
//
// We sniff the BOM up front and wrap the reader with a charset-aware
// transforming reader, because Go's encoding/xml only consults the
// CharsetReader callback after reading the `<?xml encoding=...?>`
// declaration — which fails on UTF-16 streams because the first bytes
// are not valid UTF-8.
func newTMXDecoder(reader io.Reader) (*xml.Decoder, error) {
	// charset.NewReader sniffs a BOM (or XML declaration) and returns a
	// reader that converts to UTF-8 on the fly. Passing an empty
	// contentType triggers full auto-detection.
	utf8Reader, err := charset.NewReader(reader, "")
	if err != nil {
		return nil, fmt.Errorf("detect TMX charset: %w", err)
	}
	dec := xml.NewDecoder(utf8Reader)
	// The stream is already UTF-8, but the XML declaration may still
	// advertise a different charset (e.g. "UTF-16LE"). Return the
	// stream unchanged to satisfy the decoder.
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	return dec, nil
}

// ImportTMX reads a TMX file and imports matching translation units into the TM.
// Plain text TMX segments are stored as plain Fragments (no spans/entities).
// They participate in plain matching only.
func ImportTMX(tm TranslationMemory, reader io.Reader, sourceLocale, targetLocale model.LocaleID) (int, error) {
	return ImportTMXWithOptions(tm, reader, sourceLocale, targetLocale, ImportTMXOptions{})
}

// ImportTMXWithOptions is like ImportTMX but supports recording an Origin
// on every imported entry for provenance tracking. When the source TUV
// has a <prop type="source-document"> child, its value is placed in
// Origin.Reference (useful for web-crawl TMX sets like bitextor output).
func ImportTMXWithOptions(tm TranslationMemory, reader io.Reader, sourceLocale, targetLocale model.LocaleID, opts ImportTMXOptions) (int, error) {
	dec, err := newTMXDecoder(reader)
	if err != nil {
		return 0, err
	}
	var doc tmxDocument
	if err := dec.Decode(&doc); err != nil {
		return 0, fmt.Errorf("failed to parse TMX: %w", err)
	}

	addedBy := opts.OriginAddedBy
	if opts.OriginKey != "" && addedBy == "" {
		addedBy = "tmx-import"
	}
	importedAt := time.Now()

	imported := 0
	for i, tu := range doc.Body.TUs {
		var srcTUV, tgtTUV *tmxTUV
		for idx := range tu.TUVs {
			tuv := &tu.TUVs[idx]
			lang := model.LocaleID(tuv.Lang)
			if lang == sourceLocale && srcTUV == nil {
				srcTUV = tuv
			}
			if lang == targetLocale && tgtTUV == nil {
				tgtTUV = tuv
			}
		}

		if srcTUV == nil || tgtTUV == nil {
			continue
		}

		id := scopedID(opts.OriginKey, tu.TUID, i, string(sourceLocale), string(targetLocale))

		entry := buildEntry(id, srcTUV, tgtTUV, sourceLocale, targetLocale, tu)
		if opts.OriginKey != "" {
			entry.Origins = originsForTUV(opts.OriginKey, addedBy, importedAt, srcTUV)
		}

		if err := tm.Add(entry); err != nil {
			return imported, fmt.Errorf("failed to add entry %s: %w", id, err)
		}
		imported++
	}

	return imported, nil
}

// scopedID builds a TM entry ID that is unique across files sharing the
// same origin key. Without scoping, unrelated files that both use "tu-1"
// as their first TUID would overwrite each other.
func scopedID(originKey, tuid string, index int, srcLocale, tgtLocale string) string {
	base := tuid
	if base == "" {
		base = fmt.Sprintf("tu-%d", index+1)
	}
	if originKey != "" {
		base = originKey + ":" + base
	}
	if srcLocale != "" && tgtLocale != "" {
		base = base + ":" + srcLocale + ">" + tgtLocale
	}
	return base
}

// ImportTMXLocalePairs reads a TMX file and emits an entry for every
// ordered (src, tgt) pair present in each TU. When `locales` is nil or
// empty, all language pairs are emitted (cross-product). When `locales`
// is non-empty, only pairs within that set are emitted.
//
// This is intended for multilingual TMX files (e.g. EUR-Lex Euramis
// exports) where a single <tu> contains many <tuv> entries and the
// caller wants to populate multiple locale pairs in a single pass.
//
// ID collisions are avoided by suffixing with the language pair.
func ImportTMXLocalePairs(tm TranslationMemory, reader io.Reader, locales []model.LocaleID, opts ImportTMXOptions) (int, error) {
	dec, err := newTMXDecoder(reader)
	if err != nil {
		return 0, err
	}
	var doc tmxDocument
	if err := dec.Decode(&doc); err != nil {
		return 0, fmt.Errorf("failed to parse TMX: %w", err)
	}

	localeAllowed := make(map[model.LocaleID]bool)
	for _, l := range locales {
		localeAllowed[l] = true
	}
	allPairs := len(localeAllowed) == 0

	addedBy := opts.OriginAddedBy
	if opts.OriginKey != "" && addedBy == "" {
		addedBy = "tmx-import"
	}
	importedAt := time.Now()

	imported := 0
	for i, tu := range doc.Body.TUs {
		// Collect valid TUVs (non-empty seg, matches locales filter).
		valid := make([]*tmxTUV, 0, len(tu.TUVs))
		for idx := range tu.TUVs {
			tuv := &tu.TUVs[idx]
			if strings.TrimSpace(tuv.Seg) == "" {
				continue
			}
			lang := model.LocaleID(tuv.Lang)
			if !allPairs && !localeAllowed[lang] {
				continue
			}
			valid = append(valid, tuv)
		}

		if len(valid) < 2 {
			continue
		}

		// Emit one entry per ordered (src, tgt) pair — cross product.
		for si, src := range valid {
			for tj, tgt := range valid {
				if si == tj {
					continue
				}
				srcLoc := model.LocaleID(src.Lang)
				tgtLoc := model.LocaleID(tgt.Lang)
				id := scopedID(opts.OriginKey, tu.TUID, i, string(srcLoc), string(tgtLoc))

				entry := buildEntry(id, src, tgt, srcLoc, tgtLoc, tu)
				if opts.OriginKey != "" {
					entry.Origins = originsForTUV(opts.OriginKey, addedBy, importedAt, src)
				}

				if err := tm.Add(entry); err != nil {
					return imported, fmt.Errorf("failed to add entry %s: %w", id, err)
				}
				imported++
			}
		}
	}

	return imported, nil
}

// buildEntry constructs a TMEntry from a parsed TU and a specific src/tgt TUV pair.
func buildEntry(id string, srcTUV, tgtTUV *tmxTUV, sourceLocale, targetLocale model.LocaleID, tu tmxTU) TMEntry {
	createdAt := parseTime(tu.CreatedAt)
	updatedAt := parseTime(tu.ChangedAt)
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	// Merge TU-level props and per-TUV props into a single map.
	// TUV props win over TU props (more specific).
	props := make(map[string]string)
	for _, p := range tu.Properties {
		props[p.Type] = p.Value
	}
	for _, p := range srcTUV.Properties {
		props["src:"+p.Type] = p.Value
	}
	for _, p := range tgtTUV.Properties {
		props["tgt:"+p.Type] = p.Value
	}

	return TMEntry{
		ID:           id,
		Source:       model.NewFragment(srcTUV.Seg),
		Target:       model.NewFragment(tgtTUV.Seg),
		SourceLocale: sourceLocale,
		TargetLocale: targetLocale,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		Properties:   props,
	}
}

// originsForTUV builds the Origins slice for an imported entry, pulling
// the source-document URL (if present) into Origin.Reference.
func originsForTUV(key, addedBy string, addedAt time.Time, srcTUV *tmxTUV) []Origin {
	o := Origin{
		Source:  "import",
		Key:     key,
		AddedAt: addedAt,
		AddedBy: addedBy,
	}
	if ref := propValue(srcTUV.Properties, "source-document"); ref != "" {
		o.Reference = ref
	}
	return []Origin{o}
}

// parseTime attempts to parse a TMX date string (YYYYMMDDTHHmmssZ).
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("20060102T150405Z", s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}
