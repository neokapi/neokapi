package xliff2

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// XLIFF 2.0 XML structures (used for DOM-based parsing without skeleton)

type xliff2Doc struct {
	XMLName xml.Name     `xml:"xliff"`
	Version string       `xml:"version,attr"`
	SrcLang string       `xml:"srcLang,attr"`
	TrgLang string       `xml:"trgLang,attr"`
	Files   []xliff2File `xml:"file"`
}

type xliff2File struct {
	ID     string        `xml:"id,attr"`
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
	Content string `xml:",chardata"`
}

type xliff2Content struct {
	InnerXML string `xml:",innerxml"`
}

// Reader implements DataFormatReader for XLIFF 2.0 files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new XLIFF 2.0 reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xliff2",
			FormatDisplayName: "XLIFF 2.0",
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

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/xliff+xml"},
		Extensions: []string{".xlf", ".xliff"},
		Sniff: func(data []byte) bool {
			s := string(data)
			return strings.Contains(s, "<xliff") && strings.Contains(s, "version=\"2")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("xliff2: nil document or reader")
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

	for _, file := range doc.Files {
		layer := &model.Layer{
			ID:             fmt.Sprintf("file-%s", file.ID),
			Name:           file.ID,
			Format:         "xliff2",
			Locale:         srcLang,
			IsMultilingual: true,
			Properties: map[string]string{
				"target-language": string(trgLang),
			},
		}
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

// readContentStreaming uses streaming XML parsing for skeleton byte-offset tracking.
func (r *Reader) readContentStreaming(ctx context.Context, ch chan<- model.PartResult, content []byte) {
	rawText := string(content)
	decoder := xml.NewDecoder(strings.NewReader(rawText))
	decoder.Strict = false

	var (
		srcLang       string
		trgLang       string
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
	)

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xliff2: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "xliff":
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "srcLang":
						srcLang = a.Value
					case "trgLang":
						trgLang = a.Value
					}
				}

			case "file":
				inFile = true
				fileID = ""
				for _, a := range t.Attr {
					if a.Name.Local == "id" {
						fileID = a.Value
					}
				}
				layer := &model.Layer{
					ID:             fmt.Sprintf("file-%s", fileID),
					Name:           fileID,
					Format:         "xliff2",
					Locale:         model.LocaleID(srcLang),
					IsMultilingual: true,
					Properties: map[string]string{
						"target-language": trgLang,
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
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
				if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: gs}) {
					return
				}

			case "unit":
				inUnit = true
				unitID = ""
				unitName = ""
				unitTranslate = ""
				sourceSegs = nil
				targets = make(map[model.LocaleID][]*model.Segment)
				notes = nil
				states = nil
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "id":
						unitID = a.Value
					case "name":
						unitName = a.Value
					case "translate":
						unitTranslate = a.Value
					}
				}

			case "notes":
				if inUnit {
					inNotes = true
				}

			case "note":
				if inNotes {
					inNote = true
					noteBuilder.Reset()
				}

			case "segment":
				if inUnit {
					inSegment = true
					segID = ""
					segState := ""
					for _, a := range t.Attr {
						switch a.Name.Local {
						case "id":
							segID = a.Value
						case "state":
							segState = a.Value
						}
					}
					if segState != "" {
						states = append(states, segState)
					}
				}

			case "source":
				if inSegment {
					inSource = true
					sourceDepth = 0
					sourceInnerXML.Reset()
					elemStartOff = decoder.InputOffset()
				}

			case "target":
				if inSegment {
					inTarget = true
					targetDepth = 0
					targetInnerXML.Reset()
					elemStartOff = decoder.InputOffset()
				}

			default:
				// Track nested elements inside source/target for inner XML reconstruction
				if inSource {
					sourceDepth++
					sourceInnerXML.WriteString("<")
					sourceInnerXML.WriteString(t.Name.Local)
					for _, a := range t.Attr {
						sourceInnerXML.WriteString(" ")
						if a.Name.Space != "" {
							sourceInnerXML.WriteString(a.Name.Space)
							sourceInnerXML.WriteString(":")
						}
						sourceInnerXML.WriteString(a.Name.Local)
						sourceInnerXML.WriteString(`="`)
						sourceInnerXML.WriteString(xmlEscapeAttr(a.Value))
						sourceInnerXML.WriteString(`"`)
					}
					sourceInnerXML.WriteString(">")
				} else if inTarget {
					targetDepth++
					targetInnerXML.WriteString("<")
					targetInnerXML.WriteString(t.Name.Local)
					for _, a := range t.Attr {
						targetInnerXML.WriteString(" ")
						if a.Name.Space != "" {
							targetInnerXML.WriteString(a.Name.Space)
							targetInnerXML.WriteString(":")
						}
						targetInnerXML.WriteString(a.Name.Local)
						targetInnerXML.WriteString(`="`)
						targetInnerXML.WriteString(xmlEscapeAttr(a.Value))
						targetInnerXML.WriteString(`"`)
					}
					targetInnerXML.WriteString(">")
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "file":
				if inFile {
					layer := &model.Layer{
						ID:   fmt.Sprintf("file-%s", fileID),
						Name: fileID,
					}
					r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
					inFile = false
				}

			case "group":
				r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{}})

			case "unit":
				if inUnit {
					translatable := true
					if strings.EqualFold(unitTranslate, "no") {
						translatable = false
					}

					block := &model.Block{
						ID:           unitID,
						Name:         unitName,
						Translatable: translatable,
						Source:       sourceSegs,
						Targets:      targets,
						Properties:   make(map[string]string),
						Annotations:  make(map[string]model.Annotation),
					}

					for _, st := range states {
						if st != "" {
							block.Properties["state"] = st
						}
					}
					for i, note := range notes {
						block.Properties[fmt.Sprintf("note-%d", i)] = note
					}

					r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
					blockCount++
					inUnit = false
				}

			case "notes":
				inNotes = false

			case "note":
				if inNote {
					notes = append(notes, noteBuilder.String())
					inNote = false
				}

			case "segment":
				inSegment = false

			case "source":
				if inSource {
					endOff := decoder.InputOffset()
					closeTag := "</source>"
					endPos := int(endOff) - len(closeTag)
					if endPos < 0 {
						endPos = 0
					}
					elemPositions = append(elemPositions, elemPos{
						startOffset: int(elemStartOff),
						endOffset:   endPos,
						blockIdx:    blockCount,
						elemType:    "source",
					})

					sid := segID
					if sid == "" {
						sid = fmt.Sprintf("s%d", len(sourceSegs)+1)
					}
					sourceText := strings.TrimSpace(sourceInnerXML.String())
					sourceSegs = append(sourceSegs, &model.Segment{
						ID:      sid,
						Content: model.NewFragment(sourceText),
					})
					inSource = false
				}

			case "target":
				if inTarget {
					endOff := decoder.InputOffset()
					closeTag := "</target>"
					endPos := int(endOff) - len(closeTag)
					if endPos < 0 {
						endPos = 0
					}
					elemPositions = append(elemPositions, elemPos{
						startOffset: int(elemStartOff),
						endOffset:   endPos,
						blockIdx:    blockCount,
						elemType:    "target",
					})

					targetText := strings.TrimSpace(targetInnerXML.String())
					tl := model.LocaleID(trgLang)
					if targetText != "" && !tl.IsEmpty() {
						sid := segID
						if sid == "" {
							sid = fmt.Sprintf("s%d", len(sourceSegs))
						}
						targets[tl] = append(targets[tl], &model.Segment{
							ID:      sid,
							Content: model.NewFragment(targetText),
						})
					}
					inTarget = false
				}

			default:
				// Track nested end elements inside source/target
				if inSource && sourceDepth > 0 {
					sourceDepth--
					sourceInnerXML.WriteString("</")
					sourceInnerXML.WriteString(t.Name.Local)
					sourceInnerXML.WriteString(">")
				} else if inTarget && targetDepth > 0 {
					targetDepth--
					targetInnerXML.WriteString("</")
					targetInnerXML.WriteString(t.Name.Local)
					targetInnerXML.WriteString(">")
				}
			}

		case xml.CharData:
			text := string(t)
			if inNote {
				noteBuilder.WriteString(text)
			} else if inSource {
				sourceInnerXML.WriteString(text)
			} else if inTarget {
				targetInnerXML.WriteString(text)
			}
		}
	}

	// Build skeleton from collected element positions
	if len(elemPositions) > 0 {
		skelPos := 0
		for _, ep := range elemPositions {
			if ep.startOffset > skelPos {
				r.skelText(rawText[skelPos:ep.startOffset])
			}
			refID := fmt.Sprintf("%d:%s", ep.blockIdx, ep.elemType)
			r.skelRef(refID)
			skelPos = ep.endOffset
		}
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}
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
			ID:      segID,
			Content: model.NewFragment(sourceText),
		})

		targetText := strings.TrimSpace(seg.Target.InnerXML)
		if targetText != "" && !trgLang.IsEmpty() {
			targets[trgLang] = append(targets[trgLang], &model.Segment{
				ID:      segID,
				Content: model.NewFragment(targetText),
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
