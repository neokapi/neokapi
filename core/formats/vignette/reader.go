// Package vignette implements the Vignette CMS export/import XML format
// (the output of Vignette's `vgnexport` tool). It mirrors the Okapi
// `okf_vignette` filter contract: walk every `<importContentInstance>`
// block, extract the `<attribute name="…">` payloads listed in
// PartsNames, and pair source / target instances via SOURCE_ID +
// LOCALE_ID (or extract every instance independently in monolingual
// mode).
package vignette

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Vignette CMS export/import XML.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new Vignette CMS XML reader with default config.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "vignette",
			FormatDisplayName: "Vignette CMS Export",
			FormatMimeType:    "text/xml",
			// Extensions intentionally empty — vignette files use the
			// generic .xml extension. See Signature() Sniff hook for
			// per-document detection.
			Cfg: cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format. Vignette CMS
// XML files use the generic .xml extension + text/xml MIME, so
// detection is sniff-only: we claim ownership only when the document
// carries the Vignette importexport namespace or an
// importContentInstance element. This keeps generic XML detection
// routed to the xml reader.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		Sniff: func(data []byte) bool {
			s := string(data)
			return strings.Contains(s, "vignette.com/xmlschemas/importexport") ||
				strings.Contains(s, "<importContentInstance")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("vignette: nil document or reader")
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

// extractedAttribute holds one extracted attribute payload.
type extractedAttribute struct {
	name        string // attribute @name
	subFilter   string // sub-filter id from PartsConfigurations (e.g. "okf_html", "default")
	rawPayload  string // raw character data inside <valueString>/<valueCLOB> (entity-decoded)
	rawValue    string // literal source bytes of the payload region [startOffset:endOffset], used verbatim for non-translatable surfacing
	startOffset int    // byte offset of the inner payload region (just after the opening valueString/valueCLOB tag)
	endOffset   int    // byte offset of the start of the closing tag
	valueElem   string // "valueString" / "valueCLOB" / etc.
}

// extractedInstance holds one parsed `<importContentInstance>` block.
type extractedInstance struct {
	sourceID   string               // value of the <attribute name="SOURCE_ID">
	localeID   string               // value of the <attribute name="LOCALE_ID">
	attributes []extractedAttribute // attributes whose name appears in PartsNames
	startOff   int                  // byte offset of <importContentInstance>
	endOff     int                  // byte offset just after </importContentInstance>
	allAttrs   map[string]string    // all extracted attribute string values (for SOURCE_ID/LOCALE_ID lookup)
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	raw, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("vignette: reading: %w", err)}
		return
	}
	rawText := string(raw)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "vignette",
		Locale:   locale,
		Encoding: "UTF-8",
		MimeType: "text/xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	instances, parseErr := r.parseInstances(rawText)
	if parseErr != nil {
		ch <- model.PartResult{Error: fmt.Errorf("vignette: parsing: %w", parseErr)}
		return
	}

	// Decide which instances become Blocks based on monolingual / bilingual mode.
	emitted := r.emitBlocks(ctx, ch, instances)

	// Build skeleton: cover every byte of the original document, replacing
	// each emitted attribute's payload region with a SkeletonRef.
	if r.skeletonStore != nil {
		r.writeSkeleton(rawText, emitted)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// emittedRegion records one byte-region in the source that was emitted
// as a translatable Block, so the writer can fill it back in from the
// (possibly translated) Block.
type emittedRegion struct {
	startOffset int
	endOffset   int
	blockID     string
}

// emitBlocks decides which extractedInstances participate in the part
// stream and emits one Block per extracted attribute. Returns the
// emittedRegion slice so the skeleton writer can interleave skeleton
// text with refs.
func (r *Reader) emitBlocks(ctx context.Context, ch chan<- model.PartResult, instances []extractedInstance) []emittedRegion {
	parts := r.cfg.PartsMap()
	var regions []emittedRegion
	blockCounter := 0

	emit := func(inst extractedInstance, attr extractedAttribute, suffix string) bool {
		blockCounter++
		blockID := fmt.Sprintf("tu%d%s", blockCounter, suffix)
		text, wrappedP := r.decodePayload(attr.rawPayload, attr.subFilter)
		block := model.NewBlock(blockID, text)
		block.Name = attr.name
		block.Type = "vignette-attribute"
		block.Properties["attribute"] = attr.name
		block.Properties["valueElement"] = attr.valueElem
		block.Properties["subfilter"] = attr.subFilter
		if wrappedP {
			block.Properties["wrappedP"] = "true"
		}
		if inst.sourceID != "" {
			block.Properties["sourceId"] = inst.sourceID
		}
		if inst.localeID != "" {
			block.Properties["localeId"] = inst.localeID
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
		regions = append(regions, emittedRegion{
			startOffset: attr.startOffset,
			endOffset:   attr.endOffset,
			blockID:     blockID,
		})
		return true
	}

	// emitNT surfaces a skipped (non-source-locale / unpaired) instance's
	// attribute payload as a NON-translatable content Block: the literal
	// source bytes ride a single verbatim run (whitespace-significant, NOT
	// inline-parsed) so an ingestion/LLM consumer sees the contextual content
	// while MT skips it (Translatable=false). The payload bytes are written
	// back verbatim by the writer (Properties["rawVerbatim"]), so the
	// skeleton ref round-trips byte-for-byte exactly as the prior skeleton
	// text did.
	emitNT := func(inst extractedInstance, attr extractedAttribute) bool {
		blockCounter++
		blockID := fmt.Sprintf("tu%d", blockCounter)
		block := model.NewBlock(blockID, attr.rawValue)
		block.Name = attr.name
		block.Type = "vignette-attribute"
		block.Translatable = false
		block.PreserveWhitespace = true
		block.Properties["attribute"] = attr.name
		block.Properties["valueElement"] = attr.valueElem
		block.Properties["subfilter"] = attr.subFilter
		block.Properties["rawVerbatim"] = "true"
		block.Properties["nonSourceLocale"] = "true"
		if inst.sourceID != "" {
			block.Properties["sourceId"] = inst.sourceID
		}
		if inst.localeID != "" {
			block.Properties["localeId"] = inst.localeID
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
		regions = append(regions, emittedRegion{
			startOffset: attr.startOffset,
			endOffset:   attr.endOffset,
			blockID:     blockID,
		})
		return true
	}

	// emitSkipped runs the non-translatable second pass: every extractable
	// attribute whose payload region was NOT already emitted as a translatable
	// Block is surfaced verbatim (gated on ExtractNonTranslatableContent). It
	// must be called on the normal completion path only.
	emitSkipped := func() {
		if !r.cfg.ExtractNonTranslatableContent() {
			return
		}
		emitted := make(map[int]bool, len(regions))
		for _, reg := range regions {
			emitted[reg.startOffset] = true
		}
		for _, inst := range instances {
			for _, attr := range inst.attributes {
				if emitted[attr.startOffset] {
					continue
				}
				if !emitNT(inst, attr) {
					return
				}
			}
		}
	}

	if r.cfg.Monolingual {
		// Every importContentInstance contributes its extracted
		// attributes to the stream regardless of pairing.
		for _, inst := range instances {
			for _, attr := range inst.attributes {
				if _, ok := parts[attr.name]; !ok {
					continue
				}
				if !emit(inst, attr, "") {
					return regions
				}
			}
		}
		return regions
	}

	// Bilingual mode: pair source and target instances by SOURCE_ID.
	bySourceLink := make(map[string][]int) // SOURCE_ID -> index list
	for i, inst := range instances {
		if inst.sourceID == "" {
			continue
		}
		bySourceLink[inst.sourceID] = append(bySourceLink[inst.sourceID], i)
	}

	// Determine the requested source / target locales in Vignette's
	// POSIX LOCALE_ID shape (language lowercase + "_" + region UPPERCASE,
	// e.g. "es-es" → "es_ES"). This mirrors Okapi VignetteFilter, whose
	// processList() (filters/vignette VignetteFilter.java:663-694) extracts
	// a block ONLY when its LOCALE_ID equals trgLoc.toPOSIXLocaleId() AND
	// a matching source-locale partner exists; the extracted source text
	// is drawn from that source-locale partner. Source-locale (and any
	// other-locale) blocks are emitted as opaque skeleton, untranslated.
	//
	// When no target locale is supplied we fall back to the locale-blind
	// pairing (extract the source-side instance keyed by CONTENT-ID ==
	// SOURCE_ID), which keeps standalone callers working when they don't
	// declare a target. Okapi itself requires a target locale.
	srcPOSIX := posixLocaleID(string(r.Doc.SourceLocale))
	trgPOSIX := posixLocaleID(string(r.Doc.TargetLocale))

	processedGroups := make(map[string]bool)
	// Walk in document order so the spec's "target-driven" ordering is
	// preserved (one extraction per SOURCE_ID group, taken from the
	// source-side instance, on first encounter).
	for _, inst := range instances {
		if inst.sourceID == "" {
			continue
		}
		if processedGroups[inst.sourceID] {
			continue
		}

		if trgPOSIX != "" {
			// Okapi-faithful path: only target-locale instances drive
			// extraction. Skip non-target instances entirely (they become
			// skeleton).
			if inst.localeID != trgPOSIX {
				continue
			}
			group := bySourceLink[inst.sourceID]
			// Find the source-locale partner in the same SOURCE_ID group.
			sourceInst := -1
			for _, gi := range group {
				if instances[gi].localeID == srcPOSIX {
					sourceInst = gi
					break
				}
			}
			if sourceInst < 0 {
				// Target with no corresponding source: skipped with no
				// extraction (Okapi treats it as a document part).
				continue
			}
			processedGroups[inst.sourceID] = true

			// Emit one Block per extracted attribute, taking the
			// source-locale payload as the Block source text. The byte
			// region replaced on write is the source-locale instance's
			// value region (where the translated content is filled in).
			src := instances[sourceInst]
			for _, attr := range src.attributes {
				if _, ok := parts[attr.name]; !ok {
					continue
				}
				if !emit(src, attr, "") {
					return regions
				}
			}
			continue
		}

		group := bySourceLink[inst.sourceID]
		if len(group) < 2 {
			// No partner — skip with no extraction in bilingual mode.
			continue
		}
		// Locale-blind fallback (no target locale supplied): identify the
		// "source-side" instance, preferring the one whose
		// SMCCONTENT-CONTENT-ID attribute matches the SOURCE_ID value
		// (the upstream pairing rule). Fall back to "the other instance
		// in the group" otherwise.
		sourceInst := -1
		for _, gi := range group {
			gInst := instances[gi]
			if cid := gInst.allAttrs["SMCCONTENT-CONTENT-ID"]; cid != "" && cid == inst.sourceID {
				sourceInst = gi
				break
			}
		}
		if sourceInst < 0 {
			// Fall back: pick any partner with a different localeID.
			for _, gi := range group {
				if instances[gi].localeID != inst.localeID {
					sourceInst = gi
					break
				}
			}
		}
		if sourceInst < 0 {
			// Pair couldn't be identified — skip.
			continue
		}
		processedGroups[inst.sourceID] = true

		// Emit one Block per extracted attribute, taking the source-side
		// payload as the Block source text. Walk the source instance's
		// attributes in document order.
		src := instances[sourceInst]
		for _, attr := range src.attributes {
			if _, ok := parts[attr.name]; !ok {
				continue
			}
			if !emit(src, attr, "") {
				return regions
			}
		}
	}

	// Surface the skipped (non-source-locale / unpaired) instances as
	// non-translatable content (gated; default ON). On the flag-off path the
	// part stream and skeleton stay byte-identical to before.
	emitSkipped()

	return regions
}

// posixLocaleID renders a BCP-47 locale tag in the Vignette LOCALE_ID
// POSIX shape: language lowercase, region UPPERCASE, joined by "_"
// (e.g. "es-es" → "es_ES", "en-US" → "en_US", "fr" → "fr"). This
// mirrors Okapi LocaleId.toPOSIXLocaleId() (common LocaleId.java:457),
// which the VignetteFilter uses to compare a requested locale against
// each block's LOCALE_ID attribute value. An empty input yields "".
func posixLocaleID(tag string) string {
	if tag == "" {
		return ""
	}
	tag = strings.ReplaceAll(tag, "-", "_")
	parts := strings.SplitN(tag, "_", 2)
	lang := strings.ToLower(parts[0])
	if len(parts) == 1 || parts[1] == "" {
		return lang
	}
	return lang + "_" + strings.ToUpper(parts[1])
}

// writeSkeleton writes byte-exact skeleton: copies everything between
// emitted attribute payload regions verbatim, replaces the payload
// regions with SkeletonRef entries.
func (r *Reader) writeSkeleton(rawText string, regions []emittedRegion) {
	sortRegionsByOffset(regions)

	pos := 0
	for _, reg := range regions {
		if reg.startOffset > pos {
			r.skelText(rawText[pos:reg.startOffset])
		}
		r.skelRef(reg.blockID)
		pos = reg.endOffset
	}
	if pos < len(rawText) {
		r.skelText(rawText[pos:])
	}
}

// sortRegionsByOffset sorts the slice in place by startOffset.
func sortRegionsByOffset(regions []emittedRegion) {
	// Simple insertion sort — region count is small.
	for i := 1; i < len(regions); i++ {
		for j := i; j > 0 && regions[j].startOffset < regions[j-1].startOffset; j-- {
			regions[j], regions[j-1] = regions[j-1], regions[j]
		}
	}
}

// parseInstances streams through the XML and collects every
// importContentInstance with its extracted attributes.
func (r *Reader) parseInstances(rawText string) ([]extractedInstance, error) {
	parts := r.cfg.PartsMap()
	sourceIDName := r.cfg.SourceID
	if sourceIDName == "" {
		sourceIDName = DefaultSourceID
	}
	localeIDName := r.cfg.LocaleID
	if localeIDName == "" {
		localeIDName = DefaultLocaleID
	}

	dec := xml.NewDecoder(strings.NewReader(rawText))
	dec.Strict = false

	var instances []extractedInstance
	var current *extractedInstance
	var currentAttrName string
	var currentAttrSubFilter string

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "importContentInstance":
				endTagPos := int(dec.InputOffset())
				startTagPos := findReverse(rawText, endTagPos, "<importContentInstance")
				if startTagPos < 0 {
					startTagPos = endTagPos
				}
				current = &extractedInstance{
					startOff: startTagPos,
					allAttrs: make(map[string]string),
				}
			case "attribute":
				if current == nil {
					continue
				}
				name := attrVal(t.Attr, "name")
				if name == "" {
					continue
				}
				currentAttrName = name
				currentAttrSubFilter = parts[name]
			case "valueString", "valueCLOB", "valueDate", "valueInt":
				if current == nil || currentAttrName == "" {
					continue
				}
				payloadStart := int(dec.InputOffset())
				rawPayload, payloadEnd, perr := readValueElementContent(dec, t.Name.Local)
				if perr != nil {
					return nil, perr
				}
				decoded := rawPayload
				_, isExtractable := parts[currentAttrName]
				// Always store the value in allAttrs so SOURCE_ID /
				// LOCALE_ID lookups work.
				if t.Name.Local == "valueString" {
					current.allAttrs[currentAttrName] = decoded
				}
				switch currentAttrName {
				case sourceIDName:
					if t.Name.Local == "valueString" {
						current.sourceID = decoded
					}
				case localeIDName:
					if t.Name.Local == "valueString" {
						current.localeID = decoded
					}
				}
				if isExtractable && (t.Name.Local == "valueString" || t.Name.Local == "valueCLOB") {
					rawValue := ""
					if payloadStart <= payloadEnd && payloadEnd <= len(rawText) {
						rawValue = rawText[payloadStart:payloadEnd]
					}
					current.attributes = append(current.attributes, extractedAttribute{
						name:        currentAttrName,
						subFilter:   currentAttrSubFilter,
						rawPayload:  decoded,
						rawValue:    rawValue,
						startOffset: payloadStart,
						endOffset:   payloadEnd,
						valueElem:   t.Name.Local,
					})
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "importContentInstance":
				if current != nil {
					current.endOff = int(dec.InputOffset())
					instances = append(instances, *current)
					current = nil
				}
			case "attribute":
				currentAttrName = ""
				currentAttrSubFilter = ""
			}
		}
	}
	return instances, nil
}

// readValueElementContent reads character data between the current
// position (just after the start tag of `tagName`) and the matching
// end tag, returning the decoded text content and the byte offset of
// the start of the closing tag.
func readValueElementContent(dec *xml.Decoder, tagName string) (string, int, error) {
	var buf bytes.Buffer
	depth := 1
	for depth > 0 {
		offsetBefore := int(dec.InputOffset())
		tok, err := dec.Token()
		if err != nil {
			return "", offsetBefore, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			// Real Vignette CLOBs contain entity-escaped HTML, not nested
			// XML, so we just record the local name as a passthrough.
			buf.WriteString("<" + t.Name.Local)
			for _, a := range t.Attr {
				buf.WriteString(" " + a.Name.Local + "=\"" + a.Value + "\"")
			}
			buf.WriteString(">")
		case xml.EndElement:
			depth--
			if depth == 0 && t.Name.Local == tagName {
				return buf.String(), offsetBefore, nil
			}
			buf.WriteString("</" + t.Name.Local + ">")
		case xml.CharData:
			buf.Write(t)
		case xml.Comment:
			// drop comments
		case xml.ProcInst:
			// drop PIs
		}
	}
	return buf.String(), int(dec.InputOffset()), nil
}

func attrVal(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// findReverse returns the highest index in s[:end] at which substr
// begins, or -1 if not found.
func findReverse(s string, end int, substr string) int {
	if end > len(s) {
		end = len(s)
	}
	return strings.LastIndex(s[:end], substr)
}

// pTagWrapRE matches a single outer `<p>...</p>` (or `<P>…</P>`) wrap.
var pTagWrapRE = regexp.MustCompile(`(?is)\A\s*<p\b[^>]*>(.*)</p>\s*\z`)

// decodePayload decodes an attribute payload according to its sub-filter.
// Returns the decoded text and a boolean indicating whether the reader
// stripped an outer `<p>` wrap (the writer needs that signal to round-trip).
func (r *Reader) decodePayload(payload string, subFilter string) (string, bool) {
	switch subFilter {
	case "okf_html":
		decoded := html.UnescapeString(payload)
		if m := pTagWrapRE.FindStringSubmatch(decoded); m != nil {
			return strings.TrimSpace(m[1]), true
		}
		return decoded, false
	default:
		return payload, false
	}
}

// skelText writes text to the skeleton store if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		_ = r.skeletonStore.WriteText([]byte(s))
	}
}

// skelRef writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		_ = r.skeletonStore.WriteRef(id)
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

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
