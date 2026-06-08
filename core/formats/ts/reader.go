package ts

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Qt TS (Qt Linguist) files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new Qt TS reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "ts",
			FormatDisplayName: "Qt TS",
			FormatMimeType:    "application/x-ts",
			FormatExtensions:  []string{".ts"},
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
		MIMETypes:  []string{"application/x-ts", "application/x-linguist"},
		Extensions: []string{}, // Don't auto-detect .ts (conflicts with TypeScript)
		Sniff: func(data []byte) bool {
			return bytes.Contains(data, []byte("<TS")) && bytes.Contains(data, []byte("</TS>"))
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("ts: nil document or reader")
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

// readContent uses streaming XML parsing to handle Qt TS features.
func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("ts: reading: %w", err)}
		return
	}
	rawText := string(content)

	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	// numerusFormRange records the raw byte offsets of one
	// `<numerusform …>…</numerusform>` element so the writer can
	// preserve the source's exact inter-form whitespace.
	type numerusFormRange struct {
		openStart int // offset of the `<` in `<numerusform`
		openEnd   int // offset just after the `>` of the opening tag
		closeEnd  int // offset just after the `>` of the closing `</numerusform>`
	}

	// Skeleton tracking: collect source/translation content positions.
	// `prefix` / `suffix` carry the markup the writer must inject around
	// the ref content (used for synthesized translation sections — when
	// the source has `<source>` but no `<translation>`, okapi's
	// `addTargetSection` injects `\n<translation type="unfinished" variants="no">…</translation>`
	// after the last `validBefore` element). For real source/translation
	// elements, prefix/suffix are empty and the surrounding tags come
	// from the verbatim source bytes between elemPositions entries.
	type elemPos struct {
		startOffset int    // byte offset after opening tag (or insertion point for synthesized)
		endOffset   int    // byte offset before closing tag (== startOffset for synthesized)
		blockIdx    int    // 0-based block index
		elemType    string // "source" / "translation" / "numerus_translation" / "synthesized_translation"
		prefix      string // markup to emit before the ref content
		suffix      string // markup to emit after the ref content
	}
	var elemPositions []elemPos
	var elemStartOff int64
	// validBeforeNames mirrors okapi TsFilter's `validBefore` constant —
	// the set of element names after which a synthesized `<translation>`
	// section may be inserted when the message has a `<source>` but no
	// `<translation>` of its own.
	validBeforeNames := map[string]bool{
		"source":            true,
		"oldsource":         true,
		"comment":           true,
		"oldcomment":        true,
		"extracomment":      true,
		"translatorcomment": true,
	}

	var (
		tsVersion               string
		tsLanguage              string
		tsSrcLanguage           string
		blockCount              int
		contextName             string
		contextCount            int
		inContext               bool
		inMessage               bool
		inSource                bool
		inTranslation           bool
		inComment               bool
		inExtraComment          bool
		inTransComment          bool
		inNumerusForm           bool
		inContextName           bool
		messageID               string
		messageNumerus          bool
		transType               string
		sourceBuilder           strings.Builder
		transBuilder            strings.Builder
		commentBuilder          strings.Builder
		extraCommentBuilder     strings.Builder
		transCommentBuilder     strings.Builder
		contextNameBuilder      strings.Builder
		numerusForms            []string
		numerusFormAttrs        []string      // per-form attribute strings (e.g. ` variants="no"`)
		numerusFormAttrsCurrent string        // attribute string of the currently-open <numerusform>
		numerusFormRuns         [][]model.Run // per-form Run slices (text + byte placeholders)
		numerusFormRunsCurrent  []model.Run   // Runs of the currently-open <numerusform>
		numerusByteElemCount    int           // running byte element counter for inline-code IDs across forms in this message
		// Per-numerusform raw byte offsets so the writer can preserve
		// the source's leading whitespace/indentation between forms
		// instead of substituting a single hard-coded line break.
		numerusFormOpenStartOff int // byte offset of the `<` of the currently-open <numerusform>
		numerusFormOpenEndOff   int // byte offset just after the `>` of the currently-open <numerusform>
		numerusFormRanges       []numerusFormRange
		numerusFormTrailingWS   string // raw text between last `</numerusform>` and `</translation>`
		sourceRuns              []model.Run
		sourceByteElems         []byteElem
		transByteElems          []byteElem
		buildingSource          bool
		buildingTrans           bool
		transRuns               []model.Run
		// Per-message tracking for synthesized translation sections.
		hasSource             bool  // current message had a `<source>` element
		hasTranslation        bool  // current message had a `<translation>` element
		lastValidBeforeEndOff int64 // byte offset right after the closing `>` of the last validBefore element seen in the current message (0 if none)
	)

	layer := &model.Layer{
		ID:             "doc1",
		Name:           r.Doc.URI,
		Format:         "ts",
		Locale:         locale,
		Encoding:       r.Doc.Encoding,
		MimeType:       "application/x-ts",
		IsMultilingual: true,
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("ts: parsing: %w", err)}
			return
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "TS":
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "version":
						tsVersion = attr.Value
					case "language":
						tsLanguage = attr.Value
					case "sourcelanguage":
						tsSrcLanguage = attr.Value
					}
				}
				// Emit TS metadata as Data part
				dataProps := map[string]string{
					"version":        tsVersion,
					"language":       tsLanguage,
					"sourcelanguage": tsSrcLanguage,
				}
				// Capture the source prologue (everything up to and
				// including `<TS …>`) so the writer can reproduce the
				// original XML declaration (encoding, standalone) and
				// DOCTYPE (including any internal subset `[]`) — the
				// streaming xml.Decoder discards both.
				if pre, ts := extractTSPrologue(rawText); pre != "" {
					dataProps["xml-prologue"] = pre
					dataProps["ts-tag"] = ts
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: &model.Data{
					ID:         "d1",
					Name:       "ts-header",
					Properties: dataProps,
				}}) {
					return
				}

			case "context":
				inContext = true
				contextCount++
				contextName = ""
				contextNameBuilder.Reset()

			case "name":
				if inContext && !inMessage {
					inContextName = true
					contextNameBuilder.Reset()
				}

			case "message":
				inMessage = true
				messageID = ""
				messageNumerus = false
				transType = ""
				sourceBuilder.Reset()
				transBuilder.Reset()
				commentBuilder.Reset()
				extraCommentBuilder.Reset()
				transCommentBuilder.Reset()
				numerusForms = nil
				numerusFormAttrs = nil
				numerusFormAttrsCurrent = ""
				numerusFormRuns = nil
				numerusFormRunsCurrent = nil
				numerusByteElemCount = 0
				numerusFormOpenStartOff = 0
				numerusFormOpenEndOff = 0
				numerusFormRanges = nil
				numerusFormTrailingWS = ""
				sourceRuns = nil
				transRuns = nil
				sourceByteElems = nil
				transByteElems = nil
				buildingSource = false
				buildingTrans = false
				hasSource = false
				hasTranslation = false
				lastValidBeforeEndOff = 0
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "id":
						messageID = attr.Value
					case "numerus":
						messageNumerus = attr.Value == "yes"
					}
				}

			case "source":
				if inMessage {
					inSource = true
					hasSource = true
					sourceBuilder.Reset()
					sourceRuns = nil
					sourceByteElems = nil
					buildingSource = true
					elemStartOff = decoder.InputOffset()
				}

			case "translation":
				if inMessage {
					inTranslation = true
					hasTranslation = true
					transBuilder.Reset()
					transRuns = nil
					transByteElems = nil
					buildingTrans = true
					for _, attr := range t.Attr {
						if attr.Name.Local == "type" {
							transType = attr.Value
						}
					}
					elemStartOff = decoder.InputOffset()
				}

			case "numerusform":
				if inTranslation && messageNumerus {
					inNumerusForm = true
					transBuilder.Reset()
					numerusFormRunsCurrent = nil
					// Capture this `<numerusform …>` start tag's
					// attribute string so the writer can preserve
					// `<numerusform variants="no">` style markers.
					// We rebuild from the parsed attributes (rather than
					// slicing rawText) so non-canonical source quoting
					// still produces canonical output — okapi's
					// addStartElemToSkel always emits `attr="value"` pairs.
					var sb strings.Builder
					for _, attr := range t.Attr {
						sb.WriteByte(' ')
						sb.WriteString(attr.Name.Local)
						sb.WriteString(`="`)
						sb.WriteString(attr.Value)
						sb.WriteByte('"')
					}
					numerusFormAttrsCurrent = sb.String()
					// Record byte offsets so the writer can later replay
					// the source's exact inter-form whitespace. openEnd
					// = decoder.InputOffset() (right after the `>`).
					// openStart is found by scanning backward to the
					// `<numerusform` token start.
					openEnd := int(decoder.InputOffset())
					openStart := strings.LastIndex(rawText[:openEnd], "<numerusform")
					if openStart < 0 {
						openStart = openEnd
					}
					numerusFormOpenStartOff = openStart
					numerusFormOpenEndOff = openEnd
				}

			case "comment":
				if inMessage && !inSource && !inTranslation {
					inComment = true
					commentBuilder.Reset()
				}

			case "extracomment":
				if inMessage && !inSource && !inTranslation {
					inExtraComment = true
					extraCommentBuilder.Reset()
				}

			case "translatorcomment":
				if inMessage && !inSource && !inTranslation {
					inTransComment = true
					transCommentBuilder.Reset()
				}

			case "byte":
				if inSource || inTranslation {
					var byteVal string
					for _, attr := range t.Attr {
						if attr.Name.Local == "value" {
							byteVal = attr.Value
						}
					}
					be := byteElem{value: byteVal}
					if buildingSource && inSource {
						sourceByteElems = append(sourceByteElems, be)
						sourceRuns = append(sourceRuns, model.Run{Ph: &model.PlaceholderRun{
							ID:   fmt.Sprintf("b%d", len(sourceByteElems)),
							Type: "byte",
							Data: fmt.Sprintf(`<byte value="%s"/>`, byteVal),
						}})
					} else if buildingTrans && inTranslation && inNumerusForm {
						// `<byte value="…"/>` inside a numerusform survives
						// as an inline placeholder so the writer can
						// re-emit it alongside the surrounding text after
						// pseudo-translation. Without this the byte
						// element is dropped on round-trip and the form's
						// content shifts.
						numerusByteElemCount++
						numerusFormRunsCurrent = append(numerusFormRunsCurrent, model.Run{Ph: &model.PlaceholderRun{
							ID:   fmt.Sprintf("b%d", numerusByteElemCount),
							Type: "byte",
							Data: fmt.Sprintf(`<byte value="%s"/>`, byteVal),
						}})
					} else if buildingTrans && inTranslation && !inNumerusForm {
						transByteElems = append(transByteElems, be)
						transRuns = append(transRuns, model.Run{Ph: &model.PlaceholderRun{
							ID:   fmt.Sprintf("b%d", len(transByteElems)),
							Type: "byte",
							Data: fmt.Sprintf(`<byte value="%s"/>`, byteVal),
						}})
					}
				}
			}

		case xml.EndElement:
			// Track the end offset of the last validBefore element seen
			// inside the current message so we can synthesize a
			// `<translation>` section after it when the message has a
			// `<source>` but no explicit `<translation>`. This mirrors
			// okapi TsFilter's `elemBeforeTrg` + `addTargetSection`
			// behaviour. decoder.InputOffset() returns the byte offset
			// just after the closing `>` of this end element — exactly
			// where okapi inserts the synthesized section.
			if inMessage && validBeforeNames[t.Name.Local] {
				lastValidBeforeEndOff = decoder.InputOffset()
			}
			switch t.Name.Local {
			case "name":
				if inContextName {
					inContextName = false
					contextName = contextNameBuilder.String()
					// Emit GroupStart for context
					if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{
						ID:   fmt.Sprintf("ctx%d", contextCount),
						Name: contextName,
						Type: "context",
						Properties: map[string]string{
							"name": contextName,
						},
					}}) {
						return
					}
				}

			case "context":
				if inContext {
					inContext = false
					// Emit GroupEnd
					if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{
						ID: fmt.Sprintf("ctx%d", contextCount),
					}}) {
						return
					}
				}

			case "source":
				if inSource {
					if r.skeletonStore != nil {
						endOff := decoder.InputOffset()
						closeTag := "</source>"
						endPos := int(endOff) - len(closeTag)
						if endPos < 0 {
							endPos = 0
						}
						elemPositions = append(elemPositions, elemPos{
							startOffset: int(elemStartOff),
							endOffset:   endPos,
							blockIdx:    blockCount, // will be incremented when </message> fires
							elemType:    "source",
						})
					}
					inSource = false
					buildingSource = false
				}

			case "translation":
				if inTranslation {
					if r.skeletonStore != nil {
						endOff := decoder.InputOffset()
						closeTag := "</translation>"
						endPos := int(endOff) - len(closeTag)
						if endPos < 0 {
							endPos = 0
						}
						elemType := "translation"
						if messageNumerus {
							// Numerus messages need their own ref so the
							// writer can swap in pseudo-translated
							// numerusforms instead of emitting the source
							// bytes verbatim.
							elemType = "numerus_translation"
							// Stash the trailing-whitespace span between
							// the last `</numerusform>` and this
							// `</translation>` so the writer can emit
							// the same indentation before the closing
							// tag (e.g. `\n        </translation>`).
							if len(numerusFormRanges) > 0 {
								last := numerusFormRanges[len(numerusFormRanges)-1].closeEnd
								if last < endPos {
									numerusFormTrailingWS = rawText[last:endPos]
								}
							}
						}
						elemPositions = append(elemPositions, elemPos{
							startOffset: int(elemStartOff),
							endOffset:   endPos,
							blockIdx:    blockCount,
							elemType:    elemType,
						})
					}
					inTranslation = false
					buildingTrans = false
				}

			case "numerusform":
				if inNumerusForm {
					inNumerusForm = false
					numerusForms = append(numerusForms, transBuilder.String())
					numerusFormAttrs = append(numerusFormAttrs, numerusFormAttrsCurrent)
					numerusFormRuns = append(numerusFormRuns, numerusFormRunsCurrent)
					closeEnd := int(decoder.InputOffset())
					numerusFormRanges = append(numerusFormRanges, numerusFormRange{
						openStart: numerusFormOpenStartOff,
						openEnd:   numerusFormOpenEndOff,
						closeEnd:  closeEnd,
					})
					numerusFormAttrsCurrent = ""
					numerusFormRunsCurrent = nil
					transBuilder.Reset()
				}

			case "comment":
				if inComment {
					inComment = false
				}

			case "extracomment":
				if inExtraComment {
					inExtraComment = false
				}

			case "translatorcomment":
				if inTransComment {
					inTransComment = false
				}

			case "message":
				if inMessage {
					inMessage = false
					// Detect synthesized translation section: source
					// present and non-empty, no translation, not
					// obsolete. Capture before blockCount is incremented
					// so the elemPos blockIdx matches the upcoming block.
					// Mirrors okapi TsFilter's `needTargetSection()`:
					// `!noSource() && !targetExists`, where `noSource()`
					// is true when the message has no `<source>` *or*
					// when the source has no content (no non-whitespace
					// CHARACTERS *and* no inline `<byte>` element). So
					// `<source></source>` does not trigger synthesis but
					// `<source><byte value="79"/></source>` does.
					sourceHasContent := strings.TrimSpace(sourceBuilder.String()) != "" || len(sourceByteElems) > 0
					synthesize := r.skeletonStore != nil &&
						hasSource && !hasTranslation &&
						transType != "obsolete" &&
						lastValidBeforeEndOff > 0 &&
						sourceHasContent
					blockCount++

					// Determine block ID
					blockID := messageID
					if blockID == "" {
						blockID = fmt.Sprintf("tu%d", blockCount)
					}
					if synthesize {
						// Append a synthesized elemPos. Skeleton text up
						// to lastValidBeforeEndOff is replayed verbatim;
						// then prefix + ref + suffix injects okapi's
						// `\n<translation type="unfinished" variants="no">…</translation>`
						// section. endOffset == startOffset so the next
						// skeleton chunk continues from the same point in
						// the source. The newline matches the source's
						// prevailing line-break style — okapi's TsFilter
						// captures lineBreak from the input and reuses it
						// in addTargetSection, so CRLF-encoded files
						// synthesize `\r\n<translation …>`.
						pos := int(lastValidBeforeEndOff)
						lb := detectLineBreak(rawText)
						elemPositions = append(elemPositions, elemPos{
							startOffset: pos,
							endOffset:   pos,
							blockIdx:    blockCount - 1,
							elemType:    "synthesized_translation",
							prefix:      lb + "<translation type=\"unfinished\" variants=\"no\">",
							suffix:      "</translation>",
						})
					}

					// Build source text
					sourceText := sourceBuilder.String()

					// Build block. Empty source marks the message as
					// non-translatable so the pseudo / TextModificationStep
					// pipeline leaves the existing translation alone —
					// matches okapi's behavior on the conventional PO-header
					// entry (`<source></source><translation>Project-Id-
					// Version: …</translation>`) at the top of Qt .ts files
					// imported from gettext catalogs. Without this the
					// pseudo overwrites the literal PO header text and the
					// round-trip diverges from the bridge's output.
					translatable := transType != "obsolete" && sourceText != ""
					// Run okapi's TS-default InlineCodeFinder over the
					// source/target text so printf placeholders (`%s`,
					// `%d`, …), C-style escapes (`\n`, `\t`, …) and
					// MessageFormat positional markers (`{0}`, …) are
					// represented as Ph runs the pseudo step skips
					// instead of plain TextRuns it pseudoes through.
					// Without this `Skakel na %s` round-trips as
					// `Śķàķēĺ ńà %ś` — the printf code's `s` letter
					// gets pseudo'd. Also apply to per-numerusform runs
					// so plural forms with placeholders survive pseudo
					// unmodified.
					sourceRuns = applyCodeFinder(sourceRuns)
					transRuns = applyCodeFinder(transRuns)
					for i := range numerusFormRuns {
						numerusFormRuns[i] = applyCodeFinder(numerusFormRuns[i])
					}
					var block *model.Block
					if hasInlineCodes(sourceRuns) {
						block = &model.Block{
							ID:           blockID,
							Name:         contextName,
							Translatable: translatable,
							Source:       sourceRuns,
							Targets:      make(map[model.VariantKey]*model.Target),
							Properties:   make(map[string]string),
						}
					} else {
						block = model.NewBlock(blockID, sourceText)
						block.Name = contextName
						block.Translatable = translatable
					}

					// Store translation type
					if transType != "" {
						block.Properties["type"] = transType
					}

					// Store context name
					if contextName != "" {
						block.Properties["context"] = contextName
					}

					// Store numerus flag
					if messageNumerus {
						block.Properties["numerus"] = "yes"
					}

					// Set target locale
					targetLocale := model.LocaleID(tsLanguage)
					if targetLocale == "" {
						targetLocale = r.Doc.SourceLocale
					}

					// Track which numerusforms were originally empty. Empty
					// forms are bilingual content the okapi pipeline
					// preserves verbatim — `generateNumerusFormTu`
					// extracts each `<numerusform></numerusform>` as a
					// separate TextUnit whose target placeholder is
					// never primed (no addContentPlaceholder fires
					// without character data), so TextModificationStep
					// has nothing to pseudo-translate. The writer reads
					// this property to fall back to the original empty
					// shape rather than the pseudo-of-source content
					// that would otherwise appear.
					var emptyFormFlags strings.Builder
					if messageNumerus {
						for i, form := range numerusForms {
							if i > 0 {
								emptyFormFlags.WriteByte(',')
							}
							hasText := strings.TrimSpace(form) != ""
							hasInline := i < len(numerusFormRuns) && hasInlineCodes(numerusFormRuns[i])
							if hasText || hasInline {
								emptyFormFlags.WriteByte('1')
							} else {
								emptyFormFlags.WriteByte('0')
							}
						}
					}
					// Set target text
					if messageNumerus && len(numerusForms) > 0 {
						// Represent the plural forms as a single flat target
						// run sequence (the numerusforms concatenated in
						// order) plus a target-side SEGMENTATION OVERLAY: one
						// Span per `<numerusform>`, anchored by run-index
						// boundaries, carrying `numerus-form` = the form's
						// 0-based index in its Props. The pseudo /
						// TextModificationStep pipeline still reaches every
						// form (the runs are contiguous), and the writer
						// re-splits them by extracting each span's runs.
						// Prefer the per-form Run sequence (which preserves
						// inline `<byte value="…"/>` placeholders) when
						// available; fall back to the flat text from
						// transBuilder for forms that contain only character
						// data.
						var targetRuns []model.Run
						spans := make([]model.Span, len(numerusForms))
						for i, form := range numerusForms {
							var runs []model.Run
							if i < len(numerusFormRuns) && len(numerusFormRuns[i]) > 0 {
								runs = numerusFormRuns[i]
							} else {
								runs = []model.Run{{Text: &model.TextRun{Text: form}}}
							}
							startRun := len(targetRuns)
							targetRuns = append(targetRuns, runs...)
							endRun := len(targetRuns)
							spans[i] = model.Span{
								ID: fmt.Sprintf("n%d", i),
								Range: model.RunRange{
									StartRun: startRun,
									EndRun:   endRun,
								},
								Props: map[string]string{"numerus-form": strconv.Itoa(i)},
							}
						}
						if block.Targets == nil {
							block.Targets = make(map[model.VariantKey]*model.Target)
						}
						block.SetTargetRuns(targetLocale, targetRuns)
						key := model.Variant(targetLocale)
						block.SetSegmentation(&key, spans)
						// Snapshot the original numerusforms verbatim so
						// the writer can decide whether downstream steps
						// modified any plural form (mirrors okapi's
						// APPROVED-property → "unfinished" flip on
						// content change).
						block.Properties["_orig_target_text"] = strings.Join(numerusForms, "\x1f")
						// Stash per-form attribute strings (e.g.
						// ` variants="no"`) so the writer can preserve
						// `<numerusform variants="no">` markers — okapi's
						// addStartElemToSkel re-emits every original
						// attribute. Joined with \x1f as a private
						// separator that won't appear in attribute values.
						if joinedAttrs := strings.Join(numerusFormAttrs, "\x1f"); joinedAttrs != "" {
							block.Properties["_numerusform_attrs"] = joinedAttrs
						}
						// Stash the source's prevailing line-break style
						// so the writer can re-emit `\r\n<numerusform …>`
						// pairs separated and surrounded by the same line
						// terminator the source used between adjacent
						// numerusforms.
						block.Properties["_line_break"] = detectLineBreak(rawText)
						// Per-form non-empty flags ('1' = had content,
						// '0' = was empty). Joined with commas to keep
						// the value parseable when the writer reaches it.
						if emptyFormFlags.Len() > 0 {
							block.Properties["_numerusform_nonempty"] = emptyFormFlags.String()
						}
						// Pre-form raw whitespace strings: the bytes
						// between the previous element close (or
						// `<translation>` `>`) and the current
						// `<numerusform`. Joined with \x1f. The writer
						// replays each prefix to reproduce the source's
						// indentation rather than synthesising one.
						// `_numerusform_trailing_ws` carries the bytes
						// between the last `</numerusform>` and the
						// upcoming `</translation>`.
						if len(numerusFormRanges) > 0 {
							var prefixes []string
							prev := int(elemStartOff)
							for _, rng := range numerusFormRanges {
								if rng.openStart >= prev {
									prefixes = append(prefixes, rawText[prev:rng.openStart])
								} else {
									prefixes = append(prefixes, "")
								}
								prev = rng.closeEnd
							}
							block.Properties["_numerusform_prefixes"] = strings.Join(prefixes, "\x1f")
						}
						if numerusFormTrailingWS != "" {
							block.Properties["_numerusform_trailing_ws"] = numerusFormTrailingWS
						}
					} else {
						targetText := transBuilder.String()
						if targetText != "" || transType == "unfinished" {
							if hasInlineCodes(transRuns) {
								block.SetTargetRuns(targetLocale, transRuns)
							} else {
								block.SetTargetText(targetLocale, targetText)
							}
						}
						// Snapshot the original target text (even when
						// empty) so the writer can detect downstream
						// modification and flip the `type="unfinished"`
						// flag the way okapi's APPROVED placeholder does.
						block.Properties["_orig_target_text"] = targetText
					}

					// Store comments as annotations
					var noteText string
					comment := commentBuilder.String()
					extraComment := extraCommentBuilder.String()
					transComment := transCommentBuilder.String()

					var noteParts []string
					if comment != "" {
						noteParts = append(noteParts, comment)
						block.Properties["comment"] = comment
					}
					if extraComment != "" {
						noteParts = append(noteParts, extraComment)
						block.Properties["extracomment"] = extraComment
					}
					if transComment != "" {
						noteParts = append(noteParts, transComment)
						block.Properties["translatorcomment"] = transComment
					}
					if len(noteParts) > 0 {
						noteText = strings.Join(noteParts, "\n")
						block.SetAnno("note", &model.NoteAnnotation{
							Text: noteText,
						})
					}

					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
				}
			}

		case xml.CharData:
			text := string(t)
			if inContextName {
				contextNameBuilder.WriteString(text)
			} else if inNumerusForm {
				transBuilder.WriteString(text)
				numerusFormRunsCurrent = appendTSTextRun(numerusFormRunsCurrent, text)
			} else if inSource {
				sourceBuilder.WriteString(text)
				if buildingSource {
					sourceRuns = appendTSTextRun(sourceRuns, text)
				}
			} else if inTranslation {
				if !inNumerusForm {
					transBuilder.WriteString(text)
					if buildingTrans {
						transRuns = appendTSTextRun(transRuns, text)
					}
				}
			} else if inComment {
				commentBuilder.WriteString(text)
			} else if inExtraComment {
				extraCommentBuilder.WriteString(text)
			} else if inTransComment {
				transCommentBuilder.WriteString(text)
			}
		}
	}

	// Build skeleton from collected element positions
	if r.skeletonStore != nil && len(elemPositions) > 0 {
		skelPos := 0
		for _, ep := range elemPositions {
			// Write skeleton text from skelPos to element content start.
			// CDATA sections in the skeleton are unwrapped to their inner
			// payload — okapi TsFilter's procCDATA writes the raw inner
			// characters into the skeleton without the surrounding
			// `<![CDATA[…]]>` markers. Then rewriteSkelCharData
			// re-encodes character data with okapi's quoteMode=0
			// settings (collapses `&quot;`/`&apos;` into raw `"`/`'`).
			if ep.startOffset > skelPos {
				r.skelText(rewriteSkelCharData(stripCDATA(rawText[skelPos:ep.startOffset])))
			}
			// For synthesized translation sections, append the
			// `\n<translation type="unfinished" variants="no">` opener
			// (and later the matching `</translation>` closer) so the
			// writer's translation-ref handling can drop the target text
			// inside without needing to know the wrapping markup.
			if ep.prefix != "" {
				r.skelText(ep.prefix)
			}
			// Write skeleton ref: "blockIdx:elemType"
			refID := fmt.Sprintf("%d:%s", ep.blockIdx, ep.elemType)
			r.skelRef(refID)
			if ep.suffix != "" {
				r.skelText(ep.suffix)
			}
			skelPos = ep.endOffset
		}
		// Write remaining skeleton text
		if skelPos < len(rawText) {
			r.skelText(rewriteSkelCharData(stripCDATA(rawText[skelPos:])))
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// byteElem holds a <byte value="xx"/> element.
type byteElem struct {
	value string // hex or decimal value
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

// rewriteSkelCharData mirrors okapi TsFilter's procCharacters
// re-encoding pass on skeleton text: in CHARACTER DATA regions
// (everything outside `<…>` markup, comments, processing
// instructions, and CDATA sections), the input is decoded via the
// XML parser and re-emitted with `quoteMode=0` semantics — `&quot;`
// and `&apos;` collapse to literal `"` / `'` while `&lt;`, `&gt;`,
// `&amp;` are preserved. Numeric character references decode in the
// usual way. Markup regions pass through verbatim so attribute
// values keep their original `&quot;` quoting.
//
// Implementation is a minimal hand-rolled scanner — Go's xml.Decoder
// would re-tokenise and lose offset stability, and the skeleton may
// contain partial markup the decoder rejects (the prologue, leading
// whitespace, etc.).
func rewriteSkelCharData(s string) string {
	if !strings.ContainsAny(s, "&") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '<':
			// Find the matching `>` and emit the markup verbatim.
			// Comments (`<!--…-->`), processing instructions
			// (`<?…?>`), and DOCTYPE/CDATA (`<!…>`) all terminate at
			// the next `>`. CDATA sections are already stripped by
			// stripCDATA before this runs.
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				b.WriteString(s[i:])
				return b.String()
			}
			b.WriteString(s[i : i+end+1])
			i += end + 1
		case '&':
			// Decode the entity reference. Recognise the named
			// entities `&quot;`, `&apos;`, `&lt;`, `&gt;`, `&amp;`
			// and numeric `&#…;` references. Anything else
			// passes through verbatim — okapi's procCharacters
			// goes through the StAX decoder which would have
			// already resolved any named entity defined in the
			// DTD, but the skeleton text only ever contains the
			// canonical XML predefined set.
			semi := strings.IndexByte(s[i:], ';')
			if semi < 0 || semi > 16 {
				b.WriteByte(c)
				i++
				continue
			}
			ent := s[i : i+semi+1]
			switch ent {
			case "&quot;":
				b.WriteByte('"')
			case "&apos;":
				b.WriteByte('\'')
			case "&lt;":
				b.WriteString("&lt;")
			case "&gt;":
				b.WriteString("&gt;")
			case "&amp;":
				b.WriteString("&amp;")
			default:
				if strings.HasPrefix(ent, "&#") {
					// Numeric reference. Decode and re-encode
					// with the same okapi rules — the resulting
					// rune is emitted raw unless it would also
					// need re-escaping (none of the
					// okapi-relevant ones do, so emit raw).
					body := ent[2 : len(ent)-1]
					var r rune
					var err error
					if strings.HasPrefix(body, "x") || strings.HasPrefix(body, "X") {
						_, err = fmt.Sscanf(body[1:], "%x", &r)
					} else {
						_, err = fmt.Sscanf(body, "%d", &r)
					}
					if err == nil && r > 0 {
						b.WriteRune(r)
					} else {
						b.WriteString(ent)
					}
				} else {
					b.WriteString(ent)
				}
			}
			i += semi + 1
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// stripCDATA replaces every `<![CDATA[…]]>` section in s with its
// inner payload, mirroring okapi TsFilter's procCDATA which appends
// the raw character data into the skeleton without the wrapping
// markers. Other content (markup, comments, character data) passes
// through unchanged. Unterminated CDATA sections are left as-is.
func stripCDATA(s string) string {
	const open = "<![CDATA["
	const close = "]]>"
	if !strings.Contains(s, open) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for {
		i := strings.Index(s, open)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		rest := s[i+len(open):]
		j := strings.Index(rest, close)
		if j < 0 {
			// No terminator — preserve the original text and stop.
			b.WriteString(s[i:])
			return b.String()
		}
		b.WriteString(rest[:j])
		s = rest[j+len(close):]
	}
}

// detectLineBreak reports the prevailing line-break style of a Qt TS
// source — `"\r\n"` when the first newline in the document is preceded
// by a CR, `"\n"` otherwise (including for documents with no newline
// at all). Mirrors okapi TsFilter's `lineBreak` capture from the
// XMLEventReader, which is reused by addTargetSection / procDTD /
// procCharacters when re-emitting markup.
func detectLineBreak(raw string) string {
	idx := strings.IndexByte(raw, '\n')
	if idx > 0 && raw[idx-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// extractTSPrologue returns (prologue, tsTag) from a Qt TS source
// document.
//
// `prologue` is everything from the start of the file up to (but
// excluding) the `<TS …>` element — typically `<?xml …?>`, `<!DOCTYPE
// TS …>` and any leading comments / whitespace. `tsTag` is the entire
// `<TS …>` opening tag including its angle brackets and any internal
// whitespace.
//
// Returns ("", "") when the document does not contain a recognisable
// `<TS` opening, in which case the writer falls back to its
// hard-coded prologue (xml.Header + bare `<!DOCTYPE TS>`).
func extractTSPrologue(raw string) (string, string) {
	idx := strings.Index(raw, "<TS")
	if idx < 0 {
		return "", ""
	}
	// Verify `<TS` is followed by whitespace or `>` (not e.g. `<TStuff`).
	after := idx + len("<TS")
	if after < len(raw) {
		c := raw[after]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '>' {
			return "", ""
		}
	}
	end := strings.IndexByte(raw[idx:], '>')
	if end < 0 {
		return "", ""
	}
	return raw[:idx], raw[idx : idx+end+1]
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

// hasInlineCodes reports whether a run sequence contains any Ph /
// PcOpen / PcClose / Sub run — i.e. the block needs the structured
// Run path instead of the plain-text path.
func hasInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// appendTSTextRun appends text to the run sequence, coalescing with
// a trailing TextRun.
func appendTSTextRun(runs []model.Run, text string) []model.Run {
	if text == "" {
		return runs
	}
	if n := len(runs); n > 0 && runs[n-1].Text != nil {
		runs[n-1].Text.Text += text
		return runs
	}
	return append(runs, model.Run{Text: &model.TextRun{Text: text}})
}
