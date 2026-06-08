package tmx

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	coreenc "github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for TMX (Translation Memory eXchange) files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new TMX reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "tmx",
			FormatDisplayName: "TMX",
			FormatMimeType:    "application/x-tmx+xml",
			FormatExtensions:  []string{".tmx"},
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
		MIMETypes:  []string{"application/x-tmx+xml"},
		Extensions: []string{".tmx"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("tmx: nil document or reader")
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

// readContent uses streaming XML parsing to handle TMX features including
// inline codes, DTD declarations, and both xml:lang and lang attributes.
func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:             "doc1",
		Name:           r.Doc.URI,
		Format:         "tmx",
		Locale:         locale,
		Encoding:       r.Doc.Encoding,
		MimeType:       "application/x-tmx+xml",
		IsMultilingual: true,
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("tmx: reading: %w", err)}
		return
	}
	// Capture a UTF-8 BOM presence flag before ToUTF8 strips it so the
	// writer can re-emit it via the skeleton; without this, BOM-prefixed
	// fixtures (e.g. ImportTest2C.tmx) lose their BOM on round-trip.
	hadUTF8BOM := len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF
	// Real-world TMX (Trados, Windows-native editors) is often UTF-16
	// LE/BE with a BOM. Transcode upfront so the XML parser and
	// skeleton offsets all see UTF-8 bytes.
	utf8Bytes, _, err := coreenc.ToUTF8(content)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("tmx: transcoding to UTF-8: %w", err)}
		return
	}
	content = utf8Bytes
	rawText := string(content)

	decoder := xml.NewDecoder(strings.NewReader(rawText))
	// Enable tolerance for DTD declarations and entity references.
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var (
		version        string
		headerProps    = map[string]string{}
		srcLang        string
		blockCount     int
		headerNotes    []string
		headerPropList []headerProp // header-level <prop> and <note> elements
	)

	// Skeleton tracking: collect seg positions for byte-exact reconstruction
	type segPos struct {
		startOffset int // byte offset of start of <seg> content (after <seg> tag)
		endOffset   int // byte offset of end of <seg> content (before </seg> tag)
		tuIdx       int // which TU (0-based)
		lang        string
	}
	var segPositions []segPos

	var (
		currentTU      *tuState
		currentTUV     *tuvState
		inSeg          bool
		inHeaderNote   bool
		inHeaderProp   bool
		headerPropType string
		inTUNote       bool
		inTUProp       bool
		tuPropType     string
		inHeader       bool
		segBuilder     *segContentBuilder
		noteBuilder    strings.Builder
		propBuilder    strings.Builder
		segStartOff    int64 // byte offset after <seg> start tag
		tuCount        int   // track TU index for skeleton
	)

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("tmx: parsing: %w", err)}
			return
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "tmx":
				for _, attr := range t.Attr {
					if attr.Name.Local == "version" {
						version = attr.Value
					}
				}

			case "header":
				inHeader = true
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "creationtool":
						headerProps["creationtool"] = attr.Value
					case "creationtoolversion":
						headerProps["creationtoolversion"] = attr.Value
					case "segtype":
						headerProps["segtype"] = attr.Value
					case "o-tmf":
						headerProps["o-tmf"] = attr.Value
					case "adminlang":
						headerProps["adminlang"] = attr.Value
					case "srclang":
						headerProps["srclang"] = attr.Value
						srcLang = attr.Value
					case "datatype":
						headerProps["datatype"] = attr.Value
					}
				}

			case "note":
				if inHeader && currentTU == nil {
					inHeaderNote = true
					noteBuilder.Reset()
				} else if currentTU != nil && !inSeg {
					inTUNote = true
					noteBuilder.Reset()
				}
				// <note> inside a <seg> — not per TMX spec, ignored

			case "prop":
				propType := ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "type" {
						propType = attr.Value
					}
				}
				if inHeader && currentTU == nil {
					inHeaderProp = true
					headerPropType = propType
					propBuilder.Reset()
				} else if currentTU != nil && !inSeg {
					inTUProp = true
					tuPropType = propType
					propBuilder.Reset()
				}

			case "tu":
				tuCount++
				currentTU = &tuState{}
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "tuid":
						currentTU.id = attr.Value
					case "segtype":
						currentTU.segtype = attr.Value
					}
				}

			case "tuv":
				if currentTU != nil {
					lang := extractLang(t.Attr)
					currentTUV = &tuvState{lang: lang}
				}

			case "seg":
				if currentTUV != nil {
					inSeg = true
					segBuilder = newSegContentBuilder()
					segStartOff = decoder.InputOffset()
				}

			case "bpt":
				if inSeg && segBuilder != nil {
					id, spanType, sourceX := extractInlineAttrs(t.Attr)
					segBuilder.startInline("bpt", id, spanType)
					segBuilder.currentInline.sourceX = sourceX
				}

			case "ept":
				if inSeg && segBuilder != nil {
					id, _, sourceX := extractInlineAttrs(t.Attr)
					segBuilder.startInline("ept", id, "")
					segBuilder.currentInline.sourceX = sourceX
				}

			case "ph":
				if inSeg && segBuilder != nil {
					id, spanType, sourceX := extractInlineAttrs(t.Attr)
					segBuilder.startInline("ph", id, spanType)
					segBuilder.currentInline.sourceX = sourceX
					for _, attr := range t.Attr {
						if attr.Name.Local == "assoc" {
							segBuilder.currentInline.assoc = attr.Value
						}
					}
				}

			case "it":
				if inSeg && segBuilder != nil {
					id, spanType, sourceX := extractInlineAttrs(t.Attr)
					pos := ""
					for _, attr := range t.Attr {
						if attr.Name.Local == "pos" {
							pos = attr.Value
						}
					}
					segBuilder.startInline("it", id, spanType)
					segBuilder.currentInline.pos = pos
					segBuilder.currentInline.sourceX = sourceX
				}

			case "hi":
				if inSeg && segBuilder != nil {
					id, spanType, sourceX := extractInlineAttrs(t.Attr)
					segBuilder.startInline("hi", id, spanType)
					segBuilder.currentInline.sourceX = sourceX
				}

			case "sub":
				if inSeg && segBuilder != nil {
					// <sub> inside an inline element — capture its text content
					segBuilder.startSub()
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "header":
				inHeader = false
				// Emit header metadata as Data
				headerData := &model.Data{
					ID:   "d1",
					Name: "tmx-header",
					Properties: map[string]string{
						"version":      version,
						"srclang":      srcLang,
						"adminlang":    headerProps["adminlang"],
						"datatype":     headerProps["datatype"],
						"segtype":      headerProps["segtype"],
						"o-tmf":        headerProps["o-tmf"],
						"creationtool": headerProps["creationtool"],
					},
				}
				if len(headerNotes) > 0 {
					headerData.Properties["notes"] = strings.Join(headerNotes, "\n")
				}
				for _, hp := range headerPropList {
					headerData.Properties["prop:"+hp.propType] = hp.value
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: headerData}) {
					return
				}

			case "note":
				if inHeaderNote {
					inHeaderNote = false
					headerNotes = append(headerNotes, noteBuilder.String())
				} else if inTUNote && currentTU != nil {
					inTUNote = false
					currentTU.notes = append(currentTU.notes, noteBuilder.String())
				}

			case "prop":
				if inHeaderProp {
					inHeaderProp = false
					headerPropList = append(headerPropList, headerProp{
						propType: headerPropType,
						value:    propBuilder.String(),
					})
				} else if inTUProp && currentTU != nil {
					inTUProp = false
					currentTU.props = append(currentTU.props, headerProp{
						propType: tuPropType,
						value:    propBuilder.String(),
					})
				}

			case "seg":
				if inSeg && segBuilder != nil && currentTUV != nil {
					// Record seg position for skeleton before consuming the end tag
					if r.skeletonStore != nil {
						// InputOffset() is now past </seg>, so we need to find the </seg> start
						endOff := decoder.InputOffset()
						segEndTag := "</seg>"
						segEndPos := int(endOff) - len(segEndTag)
						if segEndPos < 0 {
							segEndPos = 0
						}
						segPositions = append(segPositions, segPos{
							startOffset: int(segStartOff),
							endOffset:   segEndPos,
							tuIdx:       tuCount - 1,
							lang:        currentTUV.lang,
						})
					}
					currentTUV.seg = segBuilder.build()
					inSeg = false
					segBuilder = nil
				}

			case "tuv":
				if currentTUV != nil && currentTU != nil {
					currentTU.tuvs = append(currentTU.tuvs, tuvData{
						lang: currentTUV.lang,
						seg:  currentTUV.seg,
					})
					currentTUV = nil
				}

			case "tu":
				if currentTU != nil {
					blockCount++
					tuID := currentTU.id
					if tuID == "" {
						tuID = fmt.Sprintf("tu%d", blockCount)
					}

					block := r.buildBlock(tuID, currentTU, srcLang, locale)
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						currentTU = nil
						return
					}
					currentTU = nil
				}

			case "bpt", "ept", "ph", "it", "hi":
				if inSeg && segBuilder != nil {
					segBuilder.endInline(t.Name.Local)
				}

			case "sub":
				if inSeg && segBuilder != nil {
					segBuilder.endSub()
				}
			}

		case xml.CharData:
			text := string(t)
			if inHeaderNote {
				noteBuilder.WriteString(text)
			} else if inHeaderProp {
				propBuilder.WriteString(text)
			} else if inTUNote {
				noteBuilder.WriteString(text)
			} else if inTUProp {
				propBuilder.WriteString(text)
			} else if inSeg && segBuilder != nil {
				segBuilder.addText(text)
			}
		}
	}

	// Build skeleton from collected seg positions
	if r.skeletonStore != nil && len(segPositions) > 0 {
		// Re-emit the original UTF-8 BOM so the writer's output
		// matches BOM-prefixed source fixtures byte-for-byte.
		if hadUTF8BOM {
			r.skelText("\ufeff")
		}
		skelPos := 0
		for _, sp := range segPositions {
			// Write skeleton text from skelPos to seg content start
			if sp.startOffset > skelPos {
				r.skelText(rawText[skelPos:sp.startOffset])
			}
			// Write skeleton ref for this seg content
			// Use "tuIdx:lang" as the ref ID so the writer can look up the right text
			refID := fmt.Sprintf("%d:%s", sp.tuIdx, sp.lang)
			r.skelRef(refID)
			skelPos = sp.endOffset
		}
		// Write remaining skeleton text
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// tuState holds the state of a TU being parsed.
type tuState struct {
	id      string
	segtype string
	props   []headerProp
	notes   []string
	tuvs    []tuvData
}

// tuvState holds the state of a TUV being parsed.
type tuvState struct {
	lang string
	seg  *segContent
}

// headerProp holds a property from the header or TU level.
type headerProp struct {
	propType string
	value    string
}

// tuvData holds parsed TUV data.
type tuvData struct {
	lang string
	seg  *segContent
}

// segContent holds the parsed content of a <seg> element.
type segContent struct {
	runs []model.Run
}

// inlineState tracks an inline element being built.
type inlineState struct {
	elemType string // bpt, ept, ph, it, hi
	id       string
	spanType string
	pos      string // for <it>: begin/end
	assoc    string // for <ph>: assoc attribute (e.g. "p")
	sourceX  string // raw `x=` attribute as it appeared in source TMX
	data     strings.Builder
}

// segContentBuilder builds a segContent with inline codes.
type segContentBuilder struct {
	runs          []model.Run
	currentInline *inlineState
	inSub         bool
	spanCounter   int
}

func newSegContentBuilder() *segContentBuilder {
	return &segContentBuilder{}
}

func (b *segContentBuilder) addText(text string) {
	if b.currentInline != nil {
		b.currentInline.data.WriteString(text)
		if b.inSub {
			return
		}
		return
	}
	if text == "" {
		return
	}
	if n := len(b.runs); n > 0 && b.runs[n-1].Text != nil {
		b.runs[n-1].Text.Text += text
		return
	}
	b.runs = append(b.runs, model.Run{Text: &model.TextRun{Text: text}})
}

func (b *segContentBuilder) startInline(elemType, id, spanType string) {
	b.currentInline = &inlineState{
		elemType: elemType,
		id:       id,
		spanType: spanType,
	}
}

func (b *segContentBuilder) endInline(elemType string) {
	if b.currentInline == nil {
		return
	}
	inline := b.currentInline
	b.currentInline = nil
	b.spanCounter++

	spanID := inline.id
	if spanID == "" {
		spanID = fmt.Sprintf("c%d", b.spanCounter)
	}

	data := inline.data.String()
	// SubType encodes the original TMX element name so the writer can
	// reconstruct the inline as <ph>, <bpt>, <ept>, <it pos=...>, or
	// <hi>. Without this the runs collapse to PcOpen/PcClose/Ph and the
	// element identity is lost on round-trip.
	// Equiv preserves the original `x=` attribute when present in the
	// source TMX. Without this, the writer falls back to a per-seg counter
	// or uses the (paired) `i` value, both of which lose source-x identity
	// when authors set explicit, non-sequential x values (e.g.
	// <bpt x="2" i="1">).
	switch inline.elemType {
	case "bpt":
		b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
			ID: spanID, SubType: "tmx-bpt", Type: inline.spanType, Data: data, Equiv: inline.sourceX,
		}})
	case "ept":
		b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
			ID: spanID, SubType: "tmx-ept", Data: data, Equiv: inline.sourceX,
		}})
	case "ph":
		// Disp carries the optional `assoc` attribute (TMX-specific —
		// "p"=preceding, "f"=following, "b"=both) so the writer can
		// re-emit it; PlaceholderRun has no dedicated property bag.
		b.runs = append(b.runs, model.Run{Ph: &model.PlaceholderRun{
			ID: spanID, SubType: "tmx-ph", Type: inline.spanType, Data: data, Disp: inline.assoc, Equiv: inline.sourceX,
		}})
	case "it":
		switch inline.pos {
		case "begin":
			b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
				ID: spanID, SubType: "tmx-it-begin", Type: inline.spanType, Data: data, Equiv: inline.sourceX,
			}})
		case "end":
			b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
				ID: spanID, SubType: "tmx-it-end", Type: inline.spanType, Data: data, Equiv: inline.sourceX,
			}})
		default:
			b.runs = append(b.runs, model.Run{Ph: &model.PlaceholderRun{
				ID: spanID, SubType: "tmx-it", Type: inline.spanType, Data: data, Equiv: inline.sourceX,
			}})
		}
	case "hi":
		// <hi> is a paired highlight. TMX captures all text inside
		// <hi> as inline data, so we emit an opening run, a text
		// run for the captured body, and a closing run.
		b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
			ID: spanID, SubType: "tmx-hi", Type: inline.spanType, Equiv: inline.sourceX,
		}})
		if data != "" {
			b.runs = append(b.runs, model.Run{Text: &model.TextRun{Text: data}})
		}
		b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
			ID: spanID, SubType: "tmx-hi",
		}})
	}
}

// subOpenSentinel and subCloseSentinel mark the boundaries of a TMX
// `<sub>` element captured inside an inline element's data. The writer
// scans for these markers and emits the wrapped substring as raw XML
// (`<sub>` / `</sub>`) instead of escaping it. \x01 / \x02 are XML 1.0
// control characters that cannot legally appear in user content.
const (
	subOpenSentinel  = "\x01<sub>\x02"
	subCloseSentinel = "\x01</sub>\x02"
)

func (b *segContentBuilder) startSub() {
	b.inSub = true
	if b.currentInline != nil {
		b.currentInline.data.WriteString(subOpenSentinel)
	}
}

func (b *segContentBuilder) endSub() {
	b.inSub = false
	if b.currentInline != nil {
		b.currentInline.data.WriteString(subCloseSentinel)
	}
}

func (b *segContentBuilder) build() *segContent {
	return &segContent{runs: b.runs}
}

// xmlNamespace is the standard XML namespace URI for xml:lang etc.
const xmlNamespace = "http://www.w3.org/XML/1998/namespace"

// extractLang gets the language from TUV attributes.
// xml:lang takes precedence over lang per the TMX spec.
func extractLang(attrs []xml.Attr) string {
	var xmlLang, lang string
	for _, attr := range attrs {
		if attr.Name.Local == "lang" && attr.Name.Space == xmlNamespace {
			xmlLang = attr.Value
		} else if attr.Name.Local == "lang" && attr.Name.Space == "" {
			lang = attr.Value
		}
	}
	if xmlLang != "" {
		return xmlLang
	}
	return lang
}

// extractInlineAttrs extracts common inline element attributes.
func extractInlineAttrs(attrs []xml.Attr) (id string, spanType string, sourceX string) {
	var idI string
	for _, attr := range attrs {
		switch attr.Name.Local {
		case "i":
			idI = attr.Value
		case "x":
			sourceX = attr.Value
		case "type":
			spanType = attr.Value
		}
	}
	// Prefer `i` (the paired-code id, shared by bpt/ept) over `x` (the
	// per-tuv sequence) so PcOpen/PcClose pair up by their TMX i value
	// when both attributes are present.
	if idI != "" {
		id = idI
		return
	}
	id = sourceX
	return
}

// buildBlock constructs a model.Block from parsed TU data.
func (r *Reader) buildBlock(tuID string, tu *tuState, srcLang string, locale model.LocaleID) *model.Block {
	srcLangLower := strings.ToLower(srcLang)
	if srcLangLower == "" {
		srcLangLower = strings.ToLower(string(locale))
	}

	block := &model.Block{
		ID:           tuID,
		Name:         tuID,
		Translatable: true,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
	}

	// Store TU properties
	for _, prop := range tu.props {
		block.Properties[prop.propType] = prop.value
	}

	// Store notes
	if len(tu.notes) > 0 {
		block.Properties["notes"] = strings.Join(tu.notes, "\n")
	}

	// Store segtype if present at TU level
	if tu.segtype != "" {
		block.Properties["segtype"] = tu.segtype
	}

	// Find source TUV
	var sourceFound bool
	for _, tuv := range tu.tuvs {
		tuvLangLower := strings.ToLower(tuv.lang)
		if langMatches(tuvLangLower, srcLangLower) {
			if tuv.seg != nil {
				block.Source = tuv.seg.runs
			} else {
				block.Source = []model.Run{{Text: &model.TextRun{Text: ""}}}
			}
			sourceFound = true
			break
		}
	}

	// If no source found by language, use first TUV
	if !sourceFound && len(tu.tuvs) > 0 {
		tuv := tu.tuvs[0]
		if tuv.seg != nil {
			block.Source = tuv.seg.runs
		} else {
			block.Source = []model.Run{{Text: &model.TextRun{Text: ""}}}
		}
	}

	// If still no source, set empty
	if block.Source == nil {
		block.Source = []model.Run{{Text: &model.TextRun{Text: ""}}}
	}

	// Add targets
	firstTarget := true
	for _, tuv := range tu.tuvs {
		tuvLangLower := strings.ToLower(tuv.lang)
		if langMatches(tuvLangLower, srcLangLower) {
			continue
		}
		if tuv.lang == "" {
			continue
		}
		if tuv.seg != nil {
			block.SetTargetRuns(model.LocaleID(tuv.lang), tuv.seg.runs)
		} else {
			block.SetTargetText(model.LocaleID(tuv.lang), "")
		}
		// When processAllTargets is false, only read the first target TUV
		if !r.cfg.ProcessAllTargets {
			if firstTarget {
				firstTarget = false
			} else {
				break
			}
		}
	}

	return block
}

// langMatches checks if two language codes match, supporting relaxed matching
// where "en" matches "en-US" and vice versa.
func langMatches(a, b string) bool {
	if a == b {
		return true
	}
	// "en" matches "en-US" but "en-US" should not match "en-GB"
	if !strings.Contains(a, "-") && strings.HasPrefix(b, a+"-") {
		return true
	}
	if !strings.Contains(b, "-") && strings.HasPrefix(a, b+"-") {
		return true
	}
	return false
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
