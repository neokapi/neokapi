package xliff2

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// XLIFF 2.x XML structures (used for DOM-based parsing without skeleton)

type xliff2Doc struct {
	XMLName xml.Name     `xml:"xliff"`
	Attrs   []xml.Attr   `xml:",any,attr"`
	Version string       `xml:"version,attr"`
	SrcLang string       `xml:"srcLang,attr"`
	TrgLang string       `xml:"trgLang,attr"`
	Files   []xliff2File `xml:"file"`
}

type xliff2File struct {
	ID     string        `xml:"id,attr"`
	Notes  []xliff2Note  `xml:"notes>note"`
	Groups []xliff2Group `xml:"group"`
	Units  []xliff2Unit  `xml:"unit"`
}

type xliff2Group struct {
	ID     string        `xml:"id,attr"`
	Name   string        `xml:"name,attr"`
	Groups []xliff2Group `xml:"group"`
	Units  []xliff2Unit  `xml:"unit"`
}

type xliff2Unit struct {
	ID        string          `xml:"id,attr"`
	Name      string          `xml:"name,attr"`
	Translate string          `xml:"translate,attr"`
	Notes     []xliff2Note    `xml:"notes>note"`
	Segments  []xliff2Segment `xml:"segment"`
	Ignorable []xliff2Segment `xml:"ignorable"`
}

type xliff2Segment struct {
	ID     string        `xml:"id,attr"`
	State  string        `xml:"state,attr"`
	Source xliff2Content `xml:"source"`
	Target xliff2Content `xml:"target"`
}

type xliff2Note struct {
	ID       string `xml:"id,attr,omitempty"`
	Category string `xml:"category,attr,omitempty"`
	Content  string `xml:",chardata"`
}

// FileNotePropertyKey is the layer.Properties key used to surface a
// file-level <note> parsed from an XLIFF 2 <file>. One key is written per
// note, combining category + id so multiple notes coexist:
//
//	file-note:<category>:<id>   (empty category and/or id are kept empty)
//
// The kapi batch id, emitted by kapi extract, is carried as
// "file-note:kapi:batch-id" and read back by kapi merge via
// BatchIDFromLayer.
const FileNotePropertyPrefix = "file-note:"

type xliff2Content struct {
	InnerXML string `xml:",innerxml"`
}

// Reader implements DataFormatReader for XLIFF 2.x files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new XLIFF 2.x reader. The reader accepts the
// OASIS 2.0, 2.1 and 2.2 document namespaces as a compatible family.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xliff2",
			FormatDisplayName: "XLIFF 2.x",
			FormatMimeType:    "application/xliff+xml",
			FormatExtensions:  []string{".xlf", ".xliff"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format. The Sniff function
// accepts any OASIS XLIFF 2.x document (namespace …:2.0/2.1/2.2 or version
// attribute 2.0/2.1/2.2).
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/xliff+xml"},
		Extensions: []string{".xlf", ".xliff"},
		Sniff: func(data []byte) bool {
			s := string(data)
			if !strings.Contains(s, "<xliff") {
				return false
			}
			return strings.Contains(s, "urn:oasis:names:tc:xliff:document:2") ||
				strings.Contains(s, `version="2.0"`) ||
				strings.Contains(s, `version="2.1"`) ||
				strings.Contains(s, `version="2.2"`)
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("xliff2: nil document or reader")
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

// elemPos tracks the byte position of a source or target element's inner content.
type elemPos struct {
	startOffset int    // byte offset after opening tag
	endOffset   int    // byte offset before closing tag
	blockIdx    int    // 0-based block index
	elemType    string // "source" or "target"
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff2: reading: %w", err)}
		return
	}

	if r.skeletonStore != nil {
		r.readContentStreaming(ctx, ch, content)
		return
	}

	// DOM-based parsing (no skeleton)
	var doc xliff2Doc
	if err := xml.Unmarshal(content, &doc); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff2: parsing: %w", err)}
		return
	}

	srcLang := model.LocaleID(doc.SrcLang)
	trgLang := model.LocaleID(doc.TrgLang)
	version := doc.Version

	for _, file := range doc.Files {
		layer := &model.Layer{
			ID:             "file-" + file.ID,
			Name:           file.ID,
			Format:         "xliff2",
			Locale:         srcLang,
			IsMultilingual: true,
			Properties: map[string]string{
				"target-language": string(trgLang),
			},
		}
		if version != "" {
			layer.Properties["xliff-version"] = version
		}
		setExtraXliffAttrs(layer, doc.Attrs)
		setFileNoteProperties(layer, file.Notes)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
			return
		}

		for _, group := range file.Groups {
			r.emitGroup(ctx, ch, group, srcLang, trgLang)
		}
		for _, unit := range file.Units {
			r.emitUnit(ctx, ch, unit, srcLang, trgLang)
		}

		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
	}
}

// xliff2StreamState holds the mutable state for streaming XLIFF 2.x parsing.
type xliff2StreamState struct {
	reader  *Reader
	ctx     context.Context
	ch      chan<- model.PartResult
	rawText string
	decoder *xml.Decoder

	srcLang       string
	trgLang       string
	version       string
	extraAttrs    []xml.Attr
	fileID        string
	inFile        bool
	inUnit        bool
	inSegment     bool
	inSource      bool
	inTarget      bool
	inNotes       bool
	inNote        bool
	unitID        string
	unitName      string
	unitTranslate string
	segID         string
	blockCount    int
	elemPositions []elemPos
	elemStartOff  int64

	// Accumulators
	sourceInnerXML strings.Builder
	targetInnerXML strings.Builder
	noteBuilder    strings.Builder
	sourceDepth    int
	targetDepth    int

	// Current unit data
	sourceSegs []*model.Segment
	targets    map[model.LocaleID][]*model.Segment
	notes      []string
	states     []string
}

// handleStartElement processes an xml.StartElement in the XLIFF 2 stream.
func (s *xliff2StreamState) handleStartElement(t xml.StartElement) {
	switch t.Name.Local {
	case "xliff":
		for _, a := range t.Attr {
			switch a.Name.Local {
			case "srcLang":
				s.srcLang = a.Value
			case "trgLang":
				s.trgLang = a.Value
			case "version":
				s.version = a.Value
			default:
				if isXliffExtraAttr(a) {
					s.extraAttrs = append(s.extraAttrs, a)
				}
			}
		}

	case "file":
		s.inFile = true
		s.fileID = ""
		for _, a := range t.Attr {
			if a.Name.Local == "id" {
				s.fileID = a.Value
			}
		}
		layer := &model.Layer{
			ID:             "file-" + s.fileID,
			Name:           s.fileID,
			Format:         "xliff2",
			Locale:         model.LocaleID(s.srcLang),
			IsMultilingual: true,
			Properties: map[string]string{
				"target-language": s.trgLang,
			},
		}
		if s.version != "" {
			layer.Properties["xliff-version"] = s.version
		}
		for i, a := range s.extraAttrs {
			layer.Properties[extraAttrPropKey(i)] = encodeExtraAttr(a)
		}
		if !s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
			return
		}

	case "group":
		groupID := ""
		groupName := ""
		for _, a := range t.Attr {
			switch a.Name.Local {
			case "id":
				groupID = a.Value
			case "name":
				groupName = a.Value
			}
		}
		gs := &model.GroupStart{ID: groupID, Name: groupName}
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartGroupStart, Resource: gs})

	case "unit":
		s.inUnit = true
		s.unitID = ""
		s.unitName = ""
		s.unitTranslate = ""
		s.sourceSegs = nil
		s.targets = make(map[model.LocaleID][]*model.Segment)
		s.notes = nil
		s.states = nil
		for _, a := range t.Attr {
			switch a.Name.Local {
			case "id":
				s.unitID = a.Value
			case "name":
				s.unitName = a.Value
			case "translate":
				s.unitTranslate = a.Value
			}
		}

	case "notes":
		if s.inUnit {
			s.inNotes = true
		}

	case "note":
		if s.inNotes {
			s.inNote = true
			s.noteBuilder.Reset()
		}

	case "segment":
		if s.inUnit {
			s.inSegment = true
			s.segID = ""
			segState := ""
			for _, a := range t.Attr {
				switch a.Name.Local {
				case "id":
					s.segID = a.Value
				case "state":
					segState = a.Value
				}
			}
			if segState != "" {
				s.states = append(s.states, segState)
			}
		}

	case "source":
		if s.inSegment {
			s.inSource = true
			s.sourceDepth = 0
			s.sourceInnerXML.Reset()
			s.elemStartOff = s.decoder.InputOffset()
		}

	case "target":
		if s.inSegment {
			s.inTarget = true
			s.targetDepth = 0
			s.targetInnerXML.Reset()
			s.elemStartOff = s.decoder.InputOffset()
		}

	default:
		// Track nested elements inside source/target for inner XML reconstruction
		s.writeNestedStartTag(t)
	}
}

// writeNestedStartTag writes an opening tag to the source or target inner XML accumulator.
func (s *xliff2StreamState) writeNestedStartTag(t xml.StartElement) {
	if s.inSource {
		s.sourceDepth++
		s.sourceInnerXML.WriteString("<")
		s.sourceInnerXML.WriteString(t.Name.Local)
		for _, a := range t.Attr {
			s.sourceInnerXML.WriteString(" ")
			if a.Name.Space != "" {
				s.sourceInnerXML.WriteString(a.Name.Space)
				s.sourceInnerXML.WriteString(":")
			}
			s.sourceInnerXML.WriteString(a.Name.Local)
			s.sourceInnerXML.WriteString(`="`)
			s.sourceInnerXML.WriteString(xmlEscapeAttr(a.Value))
			s.sourceInnerXML.WriteString(`"`)
		}
		s.sourceInnerXML.WriteString(">")
	} else if s.inTarget {
		s.targetDepth++
		s.targetInnerXML.WriteString("<")
		s.targetInnerXML.WriteString(t.Name.Local)
		for _, a := range t.Attr {
			s.targetInnerXML.WriteString(" ")
			if a.Name.Space != "" {
				s.targetInnerXML.WriteString(a.Name.Space)
				s.targetInnerXML.WriteString(":")
			}
			s.targetInnerXML.WriteString(a.Name.Local)
			s.targetInnerXML.WriteString(`="`)
			s.targetInnerXML.WriteString(xmlEscapeAttr(a.Value))
			s.targetInnerXML.WriteString(`"`)
		}
		s.targetInnerXML.WriteString(">")
	}
}

// handleEndElement processes an xml.EndElement in the XLIFF 2 stream.
func (s *xliff2StreamState) handleEndElement(t xml.EndElement) {
	switch t.Name.Local {
	case "file":
		if s.inFile {
			layer := &model.Layer{
				ID:   "file-" + s.fileID,
				Name: s.fileID,
			}
			s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
			s.inFile = false
		}

	case "group":
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{}})

	case "unit":
		if s.inUnit {
			s.emitUnit()
		}

	case "notes":
		s.inNotes = false

	case "note":
		if s.inNote {
			s.notes = append(s.notes, s.noteBuilder.String())
			s.inNote = false
		}

	case "segment":
		s.inSegment = false

	case "source":
		if s.inSource {
			s.finishSource()
		}

	case "target":
		if s.inTarget {
			s.finishTarget()
		}

	default:
		// Track nested end elements inside source/target
		if s.inSource && s.sourceDepth > 0 {
			s.sourceDepth--
			s.sourceInnerXML.WriteString("</")
			s.sourceInnerXML.WriteString(t.Name.Local)
			s.sourceInnerXML.WriteString(">")
		} else if s.inTarget && s.targetDepth > 0 {
			s.targetDepth--
			s.targetInnerXML.WriteString("</")
			s.targetInnerXML.WriteString(t.Name.Local)
			s.targetInnerXML.WriteString(">")
		}
	}
}

// emitUnit constructs and emits a Block from the accumulated unit data.
func (s *xliff2StreamState) emitUnit() {
	translatable := true
	if strings.EqualFold(s.unitTranslate, "no") {
		translatable = false
	}

	block := &model.Block{
		ID:           s.unitID,
		Name:         s.unitName,
		Translatable: translatable,
		Source:       s.sourceSegs,
		Targets:      s.targets,
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}

	for _, st := range s.states {
		if st != "" {
			block.Properties["state"] = st
		}
	}
	for i, note := range s.notes {
		block.Properties[fmt.Sprintf("note-%d", i)] = note
	}

	s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartBlock, Resource: block})
	s.blockCount++
	s.inUnit = false
}

// finishSource completes source element parsing, recording position and creating a segment.
func (s *xliff2StreamState) finishSource() {
	endOff := s.decoder.InputOffset()
	closeTag := "</source>"
	endPos := int(endOff) - len(closeTag)
	if endPos < 0 {
		endPos = 0
	}
	s.elemPositions = append(s.elemPositions, elemPos{
		startOffset: int(s.elemStartOff),
		endOffset:   endPos,
		blockIdx:    s.blockCount,
		elemType:    "source",
	})

	sid := s.segID
	if sid == "" {
		sid = fmt.Sprintf("s%d", len(s.sourceSegs)+1)
	}
	sourceText := strings.TrimSpace(s.sourceInnerXML.String())
	s.sourceSegs = append(s.sourceSegs, &model.Segment{
		ID:   sid,
		Runs: []model.Run{{Text: &model.TextRun{Text: sourceText}}},
	})
	s.inSource = false
}

// finishTarget completes target element parsing, recording position and creating a segment.
func (s *xliff2StreamState) finishTarget() {
	endOff := s.decoder.InputOffset()
	closeTag := "</target>"
	endPos := int(endOff) - len(closeTag)
	if endPos < 0 {
		endPos = 0
	}
	s.elemPositions = append(s.elemPositions, elemPos{
		startOffset: int(s.elemStartOff),
		endOffset:   endPos,
		blockIdx:    s.blockCount,
		elemType:    "target",
	})

	targetText := strings.TrimSpace(s.targetInnerXML.String())
	tl := model.LocaleID(s.trgLang)
	if targetText != "" && !tl.IsEmpty() {
		sid := s.segID
		if sid == "" {
			sid = fmt.Sprintf("s%d", len(s.sourceSegs))
		}
		s.targets[tl] = append(s.targets[tl], &model.Segment{
			ID:   sid,
			Runs: []model.Run{{Text: &model.TextRun{Text: targetText}}},
		})
	}
	s.inTarget = false
}

// buildSkeleton constructs the skeleton from collected element positions.
func (s *xliff2StreamState) buildSkeleton() {
	if len(s.elemPositions) == 0 {
		return
	}
	skelPos := 0
	for _, ep := range s.elemPositions {
		if ep.startOffset > skelPos {
			s.reader.skelText(s.rawText[skelPos:ep.startOffset])
		}
		refID := fmt.Sprintf("%d:%s", ep.blockIdx, ep.elemType)
		s.reader.skelRef(refID)
		skelPos = ep.endOffset
	}
	if skelPos < len(s.rawText) {
		s.reader.skelText(s.rawText[skelPos:])
	}
	s.reader.skelFlush()
}

// readContentStreaming uses streaming XML parsing for skeleton byte-offset tracking.
func (r *Reader) readContentStreaming(ctx context.Context, ch chan<- model.PartResult, content []byte) {
	rawText := string(content)
	decoder := xml.NewDecoder(strings.NewReader(rawText))
	decoder.Strict = false

	s := &xliff2StreamState{
		reader:  r,
		ctx:     ctx,
		ch:      ch,
		rawText: rawText,
		decoder: decoder,
	}

	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xliff2: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			s.handleStartElement(t)
		case xml.EndElement:
			s.handleEndElement(t)
		case xml.CharData:
			text := string(t)
			if s.inNote {
				s.noteBuilder.WriteString(text)
			} else if s.inSource {
				s.sourceInnerXML.WriteString(text)
			} else if s.inTarget {
				s.targetInnerXML.WriteString(text)
			}
		}
	}

	s.buildSkeleton()
}

func (r *Reader) emitGroup(ctx context.Context, ch chan<- model.PartResult, group xliff2Group, srcLang, trgLang model.LocaleID) {
	gs := &model.GroupStart{
		ID:   group.ID,
		Name: group.Name,
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: gs}) {
		return
	}

	for _, child := range group.Groups {
		r.emitGroup(ctx, ch, child, srcLang, trgLang)
	}
	for _, unit := range group.Units {
		r.emitUnit(ctx, ch, unit, srcLang, trgLang)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: gs})
}

func (r *Reader) emitUnit(ctx context.Context, ch chan<- model.PartResult, unit xliff2Unit, srcLang, trgLang model.LocaleID) {
	// Respect translate="no" attribute
	translatable := true
	if strings.EqualFold(unit.Translate, "no") {
		translatable = false
	}

	// Build source and target segments from the unit's segments
	var sourceSegs []*model.Segment
	targets := make(map[model.LocaleID][]*model.Segment)

	for _, seg := range unit.Segments {
		segID := seg.ID
		if segID == "" {
			segID = fmt.Sprintf("s%d", len(sourceSegs)+1)
		}

		sourceText := strings.TrimSpace(seg.Source.InnerXML)
		sourceSegs = append(sourceSegs, &model.Segment{
			ID:   segID,
			Runs: []model.Run{{Text: &model.TextRun{Text: sourceText}}},
		})

		targetText := strings.TrimSpace(seg.Target.InnerXML)
		if targetText != "" && !trgLang.IsEmpty() {
			targets[trgLang] = append(targets[trgLang], &model.Segment{
				ID:   segID,
				Runs: []model.Run{{Text: &model.TextRun{Text: targetText}}},
			})
		}
	}

	block := &model.Block{
		ID:           unit.ID,
		Name:         unit.Name,
		Translatable: translatable,
		Source:       sourceSegs,
		Targets:      targets,
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}

	// Store segment state if present
	for _, seg := range unit.Segments {
		if seg.State != "" {
			block.Properties["state"] = seg.State
		}
	}

	// Add notes as properties
	for i, note := range unit.Notes {
		block.Properties[fmt.Sprintf("note-%d", i)] = note.Content
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// extraAttrPropKeyPrefix prefixes Layer.Properties keys that carry XLIFF
// root attributes the reader captured but didn't interpret. Kept short so
// it doesn't visually dominate property dumps.
const extraAttrPropKeyPrefix = "xliff-xattr-"

// extraAttrPropKey returns the property key for the i-th captured extra attr.
// Indices keep the original source order so the writer can re-emit attrs
// in the order they appeared on the input document.
func extraAttrPropKey(i int) string {
	return fmt.Sprintf("%s%d", extraAttrPropKeyPrefix, i)
}

// encodeExtraAttr serializes an xml.Attr as "space|local=value". Space may
// be empty. The delimiters ('|' and '=') are not valid in XML attribute
// names, so round-tripping is unambiguous.
func encodeExtraAttr(a xml.Attr) string {
	return a.Name.Space + "|" + a.Name.Local + "=" + a.Value
}

// decodeExtraAttr inverts encodeExtraAttr.
func decodeExtraAttr(s string) (xml.Attr, bool) {
	bar := strings.IndexByte(s, '|')
	if bar < 0 {
		return xml.Attr{}, false
	}
	eq := strings.IndexByte(s[bar+1:], '=')
	if eq < 0 {
		return xml.Attr{}, false
	}
	space := s[:bar]
	local := s[bar+1 : bar+1+eq]
	value := s[bar+1+eq+1:]
	return xml.Attr{Name: xml.Name{Space: space, Local: local}, Value: value}, true
}

// isXliffExtraAttr reports whether an attribute on the <xliff> root should
// be preserved for round-trip. We skip attrs we interpret explicitly
// (version, srcLang, trgLang) and the default-namespace xmlns declaration
// (handled by the writer via the chosen version's namespace URI).
func isXliffExtraAttr(a xml.Attr) bool {
	if a.Name.Space == "" {
		switch a.Name.Local {
		case "version", "srcLang", "trgLang", "xmlns":
			return false
		}
	}
	// xmlns:xyz (namespace prefix declarations) use Space="xmlns" in Go's
	// encoding/xml. Preserve those so custom namespace bindings survive
	// roundtrip.
	return true
}

// setExtraXliffAttrs copies reader-captured extra root-element attrs onto a
// Layer's Properties map using the extraAttrPropKey() scheme.
func setExtraXliffAttrs(layer *model.Layer, attrs []xml.Attr) {
	n := 0
	for _, a := range attrs {
		if !isXliffExtraAttr(a) {
			continue
		}
		layer.Properties[extraAttrPropKey(n)] = encodeExtraAttr(a)
		n++
	}
}

// extraAttrsFromLayer reconstructs captured extra attrs from a Layer's
// Properties, preserving source order via the numeric index.
func extraAttrsFromLayer(layer *model.Layer) []xml.Attr {
	if layer == nil {
		return nil
	}
	var out []xml.Attr
	for i := 0; ; i++ {
		v, ok := layer.Properties[extraAttrPropKey(i)]
		if !ok {
			break
		}
		if a, ok := decodeExtraAttr(v); ok {
			out = append(out, a)
		}
	}
	return out
}

// xmlEscapeText escapes XML special characters in text content.
func xmlEscapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// xmlEscapeAttr escapes XML special characters in attribute values.
func xmlEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
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
