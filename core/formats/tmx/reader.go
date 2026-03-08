package tmx

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for TMX (Translation Memory eXchange) files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new TMX reader.
func NewReader() *Reader {
	cfg := &Config{}
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
		return fmt.Errorf("tmx: nil document or reader")
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

	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	// Enable tolerance for DTD declarations and entity references.
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var (
		version     string
		headerProps = map[string]string{}
		srcLang     string
		blockCount  int
		headerNotes []string
		headerPropList []headerProp // header-level <prop> and <note> elements
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
	)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
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
				}

			case "bpt":
				if inSeg && segBuilder != nil {
					id, spanType := extractInlineAttrs(t.Attr)
					segBuilder.startInline("bpt", id, spanType)
				}

			case "ept":
				if inSeg && segBuilder != nil {
					id, _ := extractInlineAttrs(t.Attr)
					segBuilder.startInline("ept", id, "")
				}

			case "ph":
				if inSeg && segBuilder != nil {
					id, spanType := extractInlineAttrs(t.Attr)
					segBuilder.startInline("ph", id, spanType)
				}

			case "it":
				if inSeg && segBuilder != nil {
					id, spanType := extractInlineAttrs(t.Attr)
					pos := ""
					for _, attr := range t.Attr {
						if attr.Name.Local == "pos" {
							pos = attr.Value
						}
					}
					segBuilder.startInline("it", id, spanType)
					segBuilder.currentInline.pos = pos
				}

			case "hi":
				if inSeg && segBuilder != nil {
					id, spanType := extractInlineAttrs(t.Attr)
					segBuilder.startInline("hi", id, spanType)
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
	frag *model.Fragment
}

// inlineState tracks an inline element being built.
type inlineState struct {
	elemType string // bpt, ept, ph, it, hi
	id       string
	spanType string
	pos      string // for <it>: begin/end
	data     strings.Builder
}

// segContentBuilder builds a segContent with inline codes.
type segContentBuilder struct {
	frag          *model.Fragment
	currentInline *inlineState
	inSub         bool
	spanCounter   int
}

func newSegContentBuilder() *segContentBuilder {
	return &segContentBuilder{
		frag: &model.Fragment{},
	}
}

func (b *segContentBuilder) addText(text string) {
	if b.currentInline != nil {
		b.currentInline.data.WriteString(text)
		if b.inSub {
			return // sub text goes into inline data only
		}
		return
	}
	b.frag.AppendText(text)
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

	switch inline.elemType {
	case "bpt":
		b.frag.AppendSpan(&model.Span{
			SpanType: model.SpanOpening,
			ID:       spanID,
			Type:     inline.spanType,
			Data:     inline.data.String(),
		})
	case "ept":
		b.frag.AppendSpan(&model.Span{
			SpanType: model.SpanClosing,
			ID:       spanID,
			Data:     inline.data.String(),
		})
	case "ph":
		b.frag.AppendSpan(&model.Span{
			SpanType: model.SpanPlaceholder,
			ID:       spanID,
			Type:     inline.spanType,
			Data:     inline.data.String(),
		})
	case "it":
		st := model.SpanPlaceholder
		if inline.pos == "begin" {
			st = model.SpanOpening
		} else if inline.pos == "end" {
			st = model.SpanClosing
		}
		b.frag.AppendSpan(&model.Span{
			SpanType: st,
			ID:       spanID,
			Type:     inline.spanType,
			Data:     inline.data.String(),
		})
	case "hi":
		// <hi> is a paired highlight — emit opening span, then text is added
		// by addText, then endInline adds closing span. Since we capture all
		// text inside <hi> as inline data, we need to emit the text as
		// regular text and use opening/closing spans.
		hiText := inline.data.String()
		b.frag.AppendSpan(&model.Span{
			SpanType: model.SpanOpening,
			ID:       spanID,
			Type:     inline.spanType,
		})
		b.frag.AppendText(hiText)
		b.frag.AppendSpan(&model.Span{
			SpanType: model.SpanClosing,
			ID:       spanID,
		})
		return // already handled
	}
}

func (b *segContentBuilder) startSub() {
	b.inSub = true
}

func (b *segContentBuilder) endSub() {
	b.inSub = false
}

func (b *segContentBuilder) build() *segContent {
	return &segContent{frag: b.frag}
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
func extractInlineAttrs(attrs []xml.Attr) (id, spanType string) {
	for _, attr := range attrs {
		switch attr.Name.Local {
		case "i", "x":
			if id == "" {
				id = attr.Value
			}
		case "type":
			spanType = attr.Value
		}
	}
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
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
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
				block.Source = []*model.Segment{{ID: "s1", Content: tuv.seg.frag}}
			} else {
				block.Source = []*model.Segment{{ID: "s1", Content: model.NewFragment("")}}
			}
			sourceFound = true
			break
		}
	}

	// If no source found by language, use first TUV
	if !sourceFound && len(tu.tuvs) > 0 {
		tuv := tu.tuvs[0]
		if tuv.seg != nil {
			block.Source = []*model.Segment{{ID: "s1", Content: tuv.seg.frag}}
		} else {
			block.Source = []*model.Segment{{ID: "s1", Content: model.NewFragment("")}}
		}
	}

	// If still no source, set empty
	if block.Source == nil {
		block.Source = []*model.Segment{{ID: "s1", Content: model.NewFragment("")}}
	}

	// Add targets
	for _, tuv := range tu.tuvs {
		tuvLangLower := strings.ToLower(tuv.lang)
		if langMatches(tuvLangLower, srcLangLower) {
			continue
		}
		if tuv.lang == "" {
			continue
		}
		if tuv.seg != nil {
			block.Targets[model.LocaleID(tuv.lang)] = []*model.Segment{{ID: "s1", Content: tuv.seg.frag}}
		} else {
			block.SetTargetText(model.LocaleID(tuv.lang), "")
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
