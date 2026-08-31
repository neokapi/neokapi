package tmx

import (
	"bufio"
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
	"github.com/neokapi/neokapi/core/safeio"
)

// docBudget is the safeio budget applied to the whole-document read. It is a
// package variable (not inlined at the call site) only so tests can shrink the
// byte cap and exercise the budget-exceeded path without streaming 1 GiB.
var docBudget = safeio.DefaultBudget()

// Reader implements DataFormatReader for TMX (Translation Memory eXchange) files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter and StreamingReader.
var (
	_ format.SkeletonStoreEmitter = (*Reader)(nil)
	_ format.StreamingReader      = (*Reader)(nil)
)

// StreamingReader marks that the reader can read incrementally from doc.Reader
// (via a bounded CaptureReader window) without materialising the whole document
// — see readStreaming. The file-runner uses this to concurrent-feed the reader.
func (r *Reader) StreamingReader() {}

// NewReader creates a new TMX reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		FormatName:        "tmx",
		FormatDisplayName: "TMX",
		FormatMimeType:    "application/x-tmx+xml",
		FormatExtensions:  []string{".tmx"},
		Cfg:               cfg,
		cfg:               cfg,
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

// readContent builds the document layer, then dispatches to the streaming or
// buffered walk. Both share walkTokens; they differ only in where raw skeleton
// bytes come from (a bounded CaptureReader window vs. the whole transcoded
// buffer) and whether the reader holds the document in memory.
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

	// Bound the whole-input read with the shared safeio byte budget so an
	// unbounded/oversized stream fails with a typed error (identical limit
	// across CLI/server/WASM — see core/safeio). Peek (non-consuming) resolves
	// the BOM-detected encoding without reading the whole document.
	br := bufio.NewReader(docBudget.Reader(r.Doc.Reader))
	head, _ := br.Peek(3)
	enc, _ := coreenc.Detect(head)

	// Streaming path: same-format skeleton round-trip, validation off, UTF-8
	// input. The UTF-16 transcode (coreenc.ToUTF8) is inherently whole-document,
	// so non-UTF-8 inputs fall back to the buffered walk. StreamingReader is
	// declared so the file-runner concurrent-feeds and the reader stays bounded.
	if r.skeletonStore != nil && r.ValidationMode() == format.ValidationOff && enc == "utf-8" {
		r.readStreaming(ctx, ch, layer, locale, br)
		return
	}
	r.readBuffered(ctx, ch, layer, locale, br)
}

// readBuffered is the whole-document fallback: it reads and (when needed)
// transcodes the entire input, then walks a decoder over the buffer. Raw
// skeleton bytes come from the buffer; nothing is discarded.
func (r *Reader) readBuffered(ctx context.Context, ch chan<- model.PartResult, layer *model.Layer, locale model.LocaleID, br io.Reader) {
	content, err := io.ReadAll(br)
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
	rawText := string(utf8Bytes)

	rawAt := func(start, end int64) string { return rawText[start:end] }
	r.walkTokens(ctx, ch, newTMXDecoder(strings.NewReader(rawText)), layer, locale, rawAt, func(int64) {}, hadUTF8BOM)
}

// readStreaming walks the decoder over a bounded CaptureReader window: raw
// skeleton bytes are sliced from the window and the window is discarded as each
// TU completes, so peak reader memory is O(read-ahead + current TU), not the
// document size. Reached only for UTF-8 input (no transcode required).
func (r *Reader) readStreaming(ctx context.Context, ch chan<- model.PartResult, layer *model.Layer, locale model.LocaleID, br *bufio.Reader) {
	// Strip a leading UTF-8 BOM so CaptureReader offsets (and the decoder) see
	// post-BOM bytes, matching the buffered path; the BOM is re-emitted as the
	// first skeleton text below.
	hadUTF8BOM := false
	if head, _ := br.Peek(3); len(head) >= 3 && head[0] == 0xEF && head[1] == 0xBB && head[2] == 0xBF {
		_, _ = br.Discard(3)
		hadUTF8BOM = true
	}
	cr := format.NewCaptureReader(br)
	var sliceErr error
	rawAt := func(start, end int64) string {
		b, ok := cr.Slice(start, end)
		if !ok {
			if sliceErr == nil {
				sliceErr = fmt.Errorf("tmx: skeleton range [%d,%d) outside capture window", start, end)
			}
			return ""
		}
		return string(b)
	}
	r.walkTokens(ctx, ch, newTMXDecoder(cr), layer, locale, rawAt, cr.DiscardTo, hadUTF8BOM)
	if sliceErr != nil {
		ch <- model.PartResult{Error: sliceErr}
	}
}

// newTMXDecoder builds an xml.Decoder configured for TMX's lenient dialect
// (DTD declarations, HTML entity/auto-close tolerance).
func newTMXDecoder(r io.Reader) *xml.Decoder {
	decoder := xml.NewDecoder(r)
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity
	return decoder
}

// walkTokens is the shared TMX token walk. rawAt returns the raw source bytes
// for an absolute byte range (from the buffer or the CaptureReader window);
// discard releases window bytes below an offset (a no-op in buffered mode).
// Skeleton is emitted incrementally — segments are non-overlapping leaves in
// document order, so a monotonic emit frontier reproduces the byte-exact
// skeleton without a whole-buffer post-pass. On success it emits PartLayerEnd.
func (r *Reader) walkTokens(ctx context.Context, ch chan<- model.PartResult, decoder *xml.Decoder, layer *model.Layer, locale model.LocaleID, rawAt func(start, end int64) string, discard func(int64), hadUTF8BOM bool) {
	srcLang := ""

	var (
		version        string
		headerProps    = map[string]string{}
		blockCount     int
		headerNotes    []string
		headerPropList []headerProp // header-level <prop> and <note> elements
	)

	// Incremental skeleton frontier: emitPos is the absolute byte offset up to
	// which skeleton has been emitted; the BOM (if any) is emitted lazily before
	// the first segment; segSeen gates trailing emission so a seg-less document
	// emits no skeleton (matching the prior whole-buffer behavior).
	var (
		emitPos    int64
		bomEmitted bool
		segSeen    bool
	)

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

	// One builder per document read: the ordinals it hands out on a repeated key
	// are scoped to this file. See unitName, and nameOrdinals for why this is not
	// model.NameBuilder.
	var names nameOrdinals
	// ids separates the `tuid`s this document repeats. TMX declares `tuid` as
	// CDATA #IMPLIED rather than an XML ID and states that its value is not
	// defined by the standard, so a memory whose units all carry `tuid="1"` — or
	// none at all, where the positional fallback below can meet a literal `tu3` —
	// is a conforming file, and the block store still needs to tell its units
	// apart. See core/model/blockid.go.
	var ids model.IDBuilder

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
					// Also surface each header-level <note> as a layer-scoped
					// NoteAnnotation so the comment text is reachable as
					// structured metadata (model.NoteAnnotation) rather than
					// only as a joined string on the header Data part. Per the
					// maintainer's guidance, comment/context text stays semantic
					// DATA/ANNOTATION, never translatable content. Annotations
					// are parity-safe (not in the canonical part stream) and the
					// part stream is unchanged, so this needs no extraction flag.
					notes := &model.Notes{}
					for _, n := range headerNotes {
						notes.Items = append(notes.Items, &model.NoteAnnotation{
							Text:      n,
							From:      "tmx",
							Annotates: "general",
						})
					}
					layer.SetAnno(model.AnnoNote, notes)
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
					// Emit skeleton incrementally: the bytes from the last emit
					// frontier up to this seg's content are skeleton text; the
					// seg content itself becomes a ref keyed "tuIdx:lang" (which
					// the writer resolves to the right variant). InputOffset() is
					// past </seg>, so back up by its length to find the content end.
					if r.skeletonStore != nil {
						endOff := decoder.InputOffset()
						segEndPos := max(endOff-int64(len("</seg>")), 0)
						if hadUTF8BOM && !bomEmitted {
							r.skelText("\ufeff")
							bomEmitted = true
						}
						if segStartOff > emitPos {
							r.skelText(rawAt(emitPos, segStartOff))
						}
						r.skelRef(fmt.Sprintf("%d:%s", tuCount-1, currentTUV.lang))
						emitPos = segEndPos
						segSeen = true
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
					ids.Assign(block)
					block.Name = unitName(&names, currentTU.id, block.SourceText())
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						currentTU = nil
						return
					}
					currentTU = nil
					// The TU is fully processed and its skeleton emitted up to
					// emitPos; release the capture window below it (no-op when
					// buffered). Bytes between here and the next seg are retained
					// until that seg's skeleton text is emitted.
					discard(emitPos)
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

	// Trailing skeleton: emit the tail bytes after the last seg. Gated on
	// segSeen so a document with no translatable segments emits no skeleton at
	// all (matching the prior whole-buffer behavior). InputOffset() at EOF is
	// the total post-BOM byte length.
	if r.skeletonStore != nil && segSeen {
		total := decoder.InputOffset()
		if total > emitPos {
			r.skelText(rawAt(emitPos, total))
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

// unitName is the structural name for a TMX translation unit.
//
// A TMX file has no structure above `<tu>` and no meaning in the order of its
// units: it is a set, and reordering it changes nothing about what it says. So
// there is no structural path to compose — only the unit's own key.
//
// `tuid` is that key where a producer wrote one. Where none was written, the
// name is the source segment, which is what a translation memory matches on
// anyway and is stable under every edit to the rest of the file. The name it
// replaces, `tu7`, said only how many units had gone past: deleting one unit
// renamed every unit after it, and in a memory file — where units are added and
// dropped constantly — that renamed nearly everything on nearly every write.
//
// Two units with the same source and no tuid are indistinguishable by key, and
// the builder's ordinal separates them. That ordinal counts occurrences of that
// one key, not units in the document, so an unrelated insertion does not move it.
func unitName(nb *nameOrdinals, tuid, sourceText string) string {
	if tuid != "" {
		return nb.name(model.StructuralPath(tuid))
	}
	return nb.name(model.StructuralPath(sourceText))
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
			r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		r.skeletonStore.WriteRef(id)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		r.skeletonStore.WriteText(r.skelBuf.Bytes())
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
