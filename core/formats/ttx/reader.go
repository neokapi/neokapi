package ttx

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Trados TagEditor TTX files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new TTX reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "ttx",
			FormatDisplayName: "Trados TagEditor TTX",
			FormatMimeType:    "application/x-ttx+xml",
			FormatExtensions:  []string{".ttx"},
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
		MIMETypes:  []string{"application/x-ttx+xml"},
		Extensions: []string{".ttx"},
		Sniff: func(data []byte) bool {
			// Trados emits .ttx as UTF-16 LE with BOM by convention,
			// so a raw UTF-8 substring check misses every native
			// Trados file. Transcode via BOM detection before
			// scanning for the root element.
			text, _, err := encoding.ToUTF8(data)
			if err != nil {
				return false
			}
			return strings.Contains(string(text), "<TRADOStag")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("ttx: nil document or reader")
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

// segPosition records the byte position of a translatable content region
// in the source rawText, so the writer can replace it with translated text
// while preserving every surrounding byte (markup, FrontMatter, whitespace)
// verbatim in the skeleton.
//
// Two flavors exist:
//   - <Tuv> segments inside a <Tu>: the region is the <Tuv> content; the
//     writer fills it in place (ref "tuIdx:tuvIdx").
//   - unsegmented text runs (auto/all mode, no <Tu>): the region is a bare
//     CharData run inside <Raw>; the writer WRAPS it in a fresh
//     <Tu MatchPercent="0"><Tuv Lang="SRC">…</Tuv><Tuv Lang="TRG">…</Tuv></Tu>
//     element, mirroring Okapi's TTXSkeletonWriter.processSegment
//     (TTXSkeletonWriter.java:118-158). Ref "u<emitIdx>".
type segPosition struct {
	startOffset int  // byte offset where translatable content begins
	endOffset   int  // byte offset where translatable content ends
	tuIdx       int  // which TU (0-based) — also the writer's block emit index
	tuvIdx      int  // which TUV within TU (0=source, 1=target)
	unsegmented bool // true → writer wraps the filled text in a new <Tu>
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("ttx: reading: %w", err)}
		return
	}
	// Trados writes .ttx as UTF-16 LE with a BOM; UTF-8 with a BOM
	// shows up too. Transcode to BOM-stripped UTF-8 before parsing.
	decoded, detectedEnc, err := encoding.ToUTF8(content)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("ttx: decoding %s: %w", detectedEnc, err)}
		return
	}
	rawText := string(decoded)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Surface the detected on-disk encoding on the Layer so downstream
	// stages (including the writer) can re-emit in the same encoding
	// without losing the Trados convention. Caller-provided Encoding
	// wins when set.
	layerEncoding := r.Doc.Encoding
	if layerEncoding == "" {
		layerEncoding = detectedEnc
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "ttx",
		Locale:   locale,
		Encoding: layerEncoding,
		MimeType: "application/x-ttx+xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Determine effective segment mode
	mode := r.cfg.SegmentMode
	includeUnsegmented := false
	if mode == SegmentModeAll {
		includeUnsegmented = true
	} else if mode == SegmentModeAuto {
		// Auto-detect: scan for Tu elements first
		preDecoder := xml.NewDecoder(strings.NewReader(rawText))
		preDecoder.Strict = false
		hasTu := false
		for {
			ptok, perr := preDecoder.Token()
			if perr != nil {
				break
			}
			if start, ok := ptok.(xml.StartElement); ok && start.Name.Local == "Tu" {
				hasTu = true
				break
			}
		}
		// If no Tu elements found, extract all text
		includeUnsegmented = !hasTu
	}

	decoder := xml.NewDecoder(strings.NewReader(rawText))
	decoder.Strict = false

	// sourceLangCode is the source language code emitted on the synthesized
	// <Tuv Lang="…"> of an unsegmented run. Okapi takes the file's
	// <UserSettings SourceLanguage="…"> when present and overrides the request
	// locale with it (TTXFilter.processUserSettings), uppercasing the result
	// (TTXFilter.open: srcLangCode = srcLoc.toString().toUpperCase()). We
	// default to the request locale uppercased and refine it once UserSettings
	// is seen — UserSettings always precedes <Raw>, so the value is fixed
	// before any unsegmented run is flushed.
	sourceLangCode := strings.ToUpper(string(locale))
	// targetLangCode is the target language code Okapi fills into the
	// (usually empty) <UserSettings TargetLanguage="…"> attribute and uses on
	// synthesized target <Tuv> elements.
	targetLangCode := strings.ToUpper(string(r.Doc.TargetLocale))

	blockCounter := 0
	// emitIndex is the 0-based position of the next emitted Block in the
	// stream. The writer collects every emitted Block (unsegmented runs +
	// <Tu> units) into one slice in this same order, so skeleton refs must
	// key off this emission index — not a <Tu>-only counter — or the writer
	// would fill the wrong segment when unsegmented runs are interleaved.
	emitIndex := 0
	inRaw := false
	// extDepth > 0 means we are inside an external <ut> (Style="external" or
	// Class="procinstr") — Okapi treats those as document-part boundaries, so
	// their content stays in the skeleton and is NOT folded into a text run
	// (TTXFilter.isInline / read()'s non-inline <ut> branch). extName records
	// the element name whose external scope we opened.

	var segPositions []segPosition
	// targetLangSubst records the byte span of the <UserSettings
	// TargetLanguage="…"> attribute value, so the skeleton build can replace
	// the (usually empty) source value with the target language code — Okapi
	// fills this placeholder in buildStartElement (TTXFilter.java:926-933,
	// TARGETLANGUAGE_ATTR handling). startOffset/endOffset bracket the value
	// between its quotes; replacement is targetLangCode.
	targetLangSubst := segPosition{startOffset: -1, endOffset: -1}

	// An unsegmented run accumulates the translatable text between external
	// boundaries inside <Raw>. Inline <ut>/<df> markup is folded: its CharData
	// contributes to runText and the run's byte span extends over it (the
	// inline codes are not modeled as Spans — the documented native
	// behavior). encoding/xml splits CharData around entities/elements, so we
	// coalesce by byte offset: runStart is the run's first byte, runEnd the
	// offset just past the last folded byte, runText the decoded text.
	runStart := -1
	runEnd := -1
	var runText strings.Builder

	// extDepth tracks nesting inside an external <ut>; inlineDepth tracks
	// nesting inside inline <ut> markup being folded into the current run. They
	// are declared before flushUnsegmented so the flush can defensively reset
	// inlineDepth (a run boundary always closes any open inline scope).
	extDepth := 0
	inlineDepth := 0

	// flushUnsegmented emits a Block + skeleton position for a pending
	// unsegmented text run. Leading whitespace is moved to the skeleton (kept
	// verbatim) and the text (with trailing whitespace) becomes the
	// translatable region, mirroring Okapi's read() which moves leading
	// whitespace out of the segment (TTXFilter.java:668-680) before wrapping
	// the run in a fresh <Tu> (TTXSkeletonWriter.processSegment). A run that
	// holds no TTX "text" characters (whitespace / punctuation only — see
	// hasTTXText / TTXFilter.hasText) is left entirely in the skeleton.
	flushUnsegmented := func() bool {
		defer func() {
			runStart, runEnd = -1, -1
			runText.Reset()
			inlineDepth = 0
		}()
		if !includeUnsegmented || runStart < 0 {
			return true
		}
		decoded := runText.String()
		if !hasTTXText(decoded) {
			return true // whitespace/punctuation/markup only: stays in skeleton
		}
		// Move leading whitespace to the skeleton. We split on the decoded
		// text but advance the raw start offset by the same byte count: a TTX
		// run's leading whitespace is plain spaces / CR / LF / tabs with no
		// entities or markup, so the decoded and raw lengths of the
		// leading-whitespace prefix are identical.
		trimmed := strings.TrimLeft(decoded, " \t\r\n")
		lead := len(decoded) - len(trimmed)
		startOff := runStart + lead
		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), trimmed)
		block.Name = fmt.Sprintf("tu%d", blockCounter)
		block.Properties["unsegmented"] = "true"
		block.Properties["source-lang"] = sourceLangCode
		segPositions = append(segPositions, segPosition{
			startOffset: startOff,
			endOffset:   runEnd,
			tuIdx:       emitIndex,
			tuvIdx:      0,
			unsegmented: true,
		})
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
		emitIndex++
		return true
	}

	// extDepth: while >0 we are inside an external <ut>; nested elements
	// increment, matching closes decrement, and CharData stays in the skeleton.
	// inlineDepth: while >0 we are inside an inline <ut> whose markup is folded
	// into the current run. An inline element that participates in a run must be
	// fully contained in the run's byte span (open AND close) so the writer's
	// ref doesn't swallow a dangling close tag and emit malformed XML; we
	// therefore anchor the run at the inline element's OPEN offset.

	// startRunAt begins (or keeps) the unsegmented run at byte offset off.
	startRunAt := func(off int) {
		if includeUnsegmented && inRaw && runStart < 0 {
			runStart = off
		}
	}

	for {
		preOff := int(decoder.InputOffset())
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("ttx: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if extDepth > 0 {
				// Inside an external <ut>: nested elements stay skeleton.
				extDepth++
				break
			}
			switch t.Name.Local {
			case "UserSettings":
				// Okapi's processUserSettings (TTXFilter.java:957-994) adopts
				// the file's SourceLanguage / TargetLanguage when present, and
				// only fills the TargetLanguage placeholder when it is empty.
				if sl := attrVal(t.Attr, "SourceLanguage"); sl != "" {
					sourceLangCode = strings.ToUpper(sl)
				}
				if tl := attrVal(t.Attr, "TargetLanguage"); tl != "" {
					// File already declares a target language: Okapi keeps it
					// (and uses it as the target lang code). Native preserves
					// the source bytes verbatim — no substitution.
					targetLangCode = strings.ToUpper(tl)
				} else {
					// Empty TargetLanguage placeholder: record its byte span so
					// the skeleton can fill it with the requested target code.
					endOff := int(decoder.InputOffset())
					if preOff >= 0 && endOff <= len(rawText) && preOff < endOff {
						tag := rawText[preOff:endOff]
						if rel := strings.Index(tag, `TargetLanguage="`); rel >= 0 {
							valStart := preOff + rel + len(`TargetLanguage="`)
							if q := strings.IndexByte(rawText[valStart:], '"'); q >= 0 {
								targetLangSubst.startOffset = valStart
								targetLangSubst.endOffset = valStart + q
							}
						}
					}
				}
			case "Raw":
				inRaw = true
			case "Tu":
				// A <Tu> ends the current unsegmented run.
				if !flushUnsegmented() {
					return
				}
				blockCounter++
				matchPercent := attrVal(t.Attr, "MatchPercent")
				var segs []segPosition
				block := r.parseTransUnitWithSkeleton(decoder, locale, blockCounter, matchPercent, emitIndex, &segs)
				segPositions = append(segPositions, segs...)
				if block != nil {
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
					emitIndex++
				}
			case "ut":
				if inRaw && isExternalUT(t.Attr) {
					// External <ut> (Style="external" / Class="procinstr") is a
					// document-part boundary: end any pending run and keep its
					// whole content in the skeleton.
					if !flushUnsegmented() {
						return
					}
					extDepth = 1
				} else if inRaw {
					// Inline <ut>: fold it into the current run. Anchor the run
					// at its open tag so the writer's ref consumes the whole
					// element (open + content + close) and the output stays
					// well-formed even when the run begins inside the <ut>.
					startRunAt(preOff)
					inlineDepth++
				}
			case "df":
				// A <df> formatting span is a run boundary: its open/close stay
				// in the skeleton and its inner text forms its own run. Okapi
				// folds df text with placeholder codes; native keeps the df
				// markup verbatim in the skeleton and extracts the inner text as
				// a separate Tu — both are valid; native simply does not model
				// the df as an inline code.
				if inRaw {
					if !flushUnsegmented() {
						return
					}
				}
			}
		case xml.EndElement:
			if extDepth > 0 {
				extDepth--
				break
			}
			switch t.Name.Local {
			case "ut":
				if inlineDepth > 0 {
					inlineDepth--
					runEnd = int(decoder.InputOffset())
				}
			case "df":
				// End of a <df> span is a run boundary (Okapi's read() ends the
				// text unit at an unbalanced </df>).
				if inRaw {
					if !flushUnsegmented() {
						return
					}
				}
			case "Raw", "Body":
				if !flushUnsegmented() {
					return
				}
				if t.Name.Local == "Raw" {
					inRaw = false
				}
			}
		case xml.CharData:
			if includeUnsegmented && inRaw && extDepth == 0 {
				startRunAt(preOff)
				runEnd = int(decoder.InputOffset())
				runText.Write(t)
			}
		}
	}

	// Build the skeleton covering the whole document verbatim, with a ref at
	// each translatable region (Tuv segments and unsegmented runs). Building
	// it unconditionally — not only when refs exist — is what preserves the
	// <FrontMatter>, <ToolSettings>, <UserSettings> and surrounding markup
	// for files with no <Tu> (Okapi keeps every non-extracted byte in the
	// skeleton via read()/DocumentPart events). Without this, a no-segment
	// file produced an empty skeleton and the writer fell back to a bare
	// <TRADOStag> wrapper, dropping FrontMatter entirely.
	if r.skeletonStore != nil {
		skelPos := 0
		// Substitute the <UserSettings TargetLanguage="…"> value first (it
		// lives in the FrontMatter, ahead of every translatable region).
		// Only when a target locale was actually requested — a plain
		// non-retargeting round-trip (no TargetLocale) leaves the source
		// value byte-exact, matching Okapi which only fills the placeholder
		// when given a target.
		if targetLangCode != "" &&
			targetLangSubst.startOffset >= 0 && targetLangSubst.endOffset >= targetLangSubst.startOffset {
			if targetLangSubst.startOffset > skelPos {
				r.skelText(rawText[skelPos:targetLangSubst.startOffset])
			}
			r.skelText(targetLangCode)
			skelPos = targetLangSubst.endOffset
		}
		for _, sp := range segPositions {
			if sp.startOffset > skelPos {
				r.skelText(rawText[skelPos:sp.startOffset])
			}
			refID := fmt.Sprintf("%d:%d", sp.tuIdx, sp.tuvIdx)
			if sp.unsegmented {
				// Unsegmented refs are wrapped in a fresh <Tu> by the writer.
				refID = fmt.Sprintf("u%d", sp.tuIdx)
			}
			r.skelRef(refID)
			skelPos = sp.endOffset
		}
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// parseTransUnitWithSkeleton parses a <Tu> element, collecting seg positions for skeleton.
func (r *Reader) parseTransUnitWithSkeleton(decoder *xml.Decoder, sourceLocale model.LocaleID, counter int, matchPercent string, tuIdx int, segs *[]segPosition) *model.Block {
	var sourceText string
	var targetText string
	var targetLang model.LocaleID
	var sourceLang model.LocaleID
	tuvIdx := 0

	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return nil
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "Tuv" {
				lang := model.LocaleID(attrVal(t.Attr, "Lang"))
				// The decoder offset is now positioned just after the
				// <Tuv ...> start tag, which is where the translatable
				// content begins in real TTX (the text lives directly in
				// <Tuv>, there is no <Seg> wrapper in the TRADOStag format).
				tuvStartOff := decoder.InputOffset()
				segText := r.parseTuvWithSkeleton(decoder, tuIdx, tuvIdx, tuvStartOff, segs)
				depth-- // parseTuv consumed end element

				if sourceLang.IsEmpty() {
					sourceLang = lang
					sourceText = segText
				} else {
					targetLang = lang
					targetText = segText
				}
				tuvIdx++
			}
		case xml.EndElement:
			depth--
		}
	}

	if sourceText == "" {
		return nil
	}

	block := model.NewBlock(fmt.Sprintf("tu%d", counter), sourceText)
	block.Name = fmt.Sprintf("tu%d", counter)
	if matchPercent != "" {
		block.Properties["match-percent"] = matchPercent
	}
	if !sourceLang.IsEmpty() {
		block.Properties["source-lang"] = string(sourceLang)
	}

	if targetText != "" && !targetLang.IsEmpty() {
		block.SetTargetText(targetLang, targetText)
	}

	return block
}

// parseTuvWithSkeleton reads the translatable content of a <Tuv> element.
//
// In the TRADOStag (.ttx) format the segment text lives directly inside
// <Tuv> — there is no <Seg> wrapper (no real Trados file uses one, and
// Okapi's TTXFilter reads <Tuv> content directly). Inline markup codes
// (<ut>, <df>, <it>) are not preserved as Spans by this reader; their text
// content is concatenated into the plain segment text. A <Seg> element, if
// present in a hand-authored file, is descended through transparently.
//
// tuvStartOff is the byte offset just after the <Tuv ...> start tag, used to
// anchor the skeleton content region for byte-exact round-trips.
func (r *Reader) parseTuvWithSkeleton(decoder *xml.Decoder, tuIdx, tuvIdx int, tuvStartOff int64, segs *[]segPosition) string {
	var buf strings.Builder
	depth := 1
	endOff := tuvStartOff // offset just before the </Tuv> end tag

	for depth > 0 {
		// Capture the offset before reading the next token; when the next
		// token is the </Tuv> end element this records the content end.
		preOff := decoder.InputOffset()
		tok, err := decoder.Token()
		if err != nil {
			return buf.String()
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			// Inline elements (<ut>, <df>, <it>, <Seg>) are descended into;
			// their text content contributes to the segment text but the
			// markup itself is dropped.
		case xml.EndElement:
			depth--
			if depth == 0 {
				endOff = preOff
			}
		case xml.CharData:
			buf.Write(t)
		}
	}

	if r.skeletonStore != nil {
		startOff := int(tuvStartOff)
		end := int(endOff)
		if end < startOff {
			end = startOff
		}
		*segs = append(*segs, segPosition{
			startOffset: startOff,
			endOffset:   end,
			tuIdx:       tuIdx,
			tuvIdx:      tuvIdx,
		})
	}

	return buf.String()
}

// ttxNoTextChars mirrors Okapi's TTXFilter.TTXNOTEXTCHARS: characters that,
// together with whitespace, are NOT considered "text" when deciding whether
// an unsegmented run is translatable. A run made up only of these (plus
// whitespace) stays in the skeleton rather than becoming a <Tu> segment.
const ttxNoTextChars = " ~`!@#$%^&*()_+=-{[}]|\\:;\"'<,>.?/•–"

// hasTTXText reports whether s contains at least one "text" character per
// Okapi's TTXFilter.hasText: a non-whitespace rune that is not in
// ttxNoTextChars. Used to decide if an unsegmented run is translatable.
func hasTTXText(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v' {
			continue
		}
		if strings.ContainsRune(ttxNoTextChars, r) {
			continue
		}
		return true
	}
	return false
}

// isExternalUT reports whether a <ut> element is "external" — a structural
// boundary whose content is skeleton, not translatable. Mirrors Okapi's
// TTXFilter.isInline (negated for <ut>): a <ut> is internal/inline by default;
// it is external when Style="external", or (when Style is absent) when
// Class="procinstr".
func isExternalUT(attrs []xml.Attr) bool {
	style := attrVal(attrs, "Style")
	if style != "" {
		return style == "external"
	}
	class := attrVal(attrs, "Class")
	if class != "" {
		return class == "procinstr"
	}
	return false // default is internal (inline)
}

// attrVal returns the value of named attribute, or "".
func attrVal(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// skelText appends text to the skeleton buffer if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		r.skelBuf.WriteString(s)
	}
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

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
