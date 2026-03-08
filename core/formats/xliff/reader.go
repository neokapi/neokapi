package xliff

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for XLIFF 1.2 files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new XLIFF 1.2 reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xliff",
			FormatDisplayName: "XLIFF 1.2",
			FormatMimeType:    "application/xliff+xml",
			FormatExtensions:  []string{".xlf", ".xliff"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/xliff+xml", "application/x-xliff+xml"},
		Extensions: []string{".xlf", ".xliff"},
		Sniff: func(data []byte) bool {
			s := string(data)
			return strings.Contains(s, "<xliff") && strings.Contains(s, "urn:oasis:names:tc:xliff:document:1")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("xliff: nil document or reader")
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

// fileInfo holds metadata from a <file> element.
type fileInfo struct {
	original   string
	sourceLang string
	targetLang string
	datatype   string
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff: reading: %w", err)}
		return
	}

	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	decoder.Strict = false

	var currentFile *fileInfo
	var inBody bool
	var groupStack []string // stack of group IDs
	// preserveWSStack tracks xml:space inheritance. When we see xml:space="preserve"
	// on any ancestor, we push true. Default is false.
	var preserveWSStack []bool

	// inheritPreserveWS returns true if any ancestor has xml:space="preserve"
	inheritPreserveWS := func() bool {
		for i := len(preserveWSStack) - 1; i >= 0; i-- {
			if preserveWSStack[i] {
				return true
			}
		}
		return false
	}

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xliff: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local

			switch local {
			case "file":
				fi := &fileInfo{}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "original":
						fi.original = a.Value
					case "source-language":
						fi.sourceLang = a.Value
					case "target-language":
						fi.targetLang = a.Value
					case "datatype":
						fi.datatype = a.Value
					}
				}
				currentFile = fi

				// Check for xml:space on file
				ws := xmlSpaceAttr(t.Attr)
				preserveWSStack = append(preserveWSStack, ws == "preserve")

				sourceLang := model.LocaleID(fi.sourceLang)
				targetLang := model.LocaleID(fi.targetLang)

				layer := &model.Layer{
					ID:             fmt.Sprintf("file-%s", fi.original),
					Name:           fi.original,
					Format:         "xliff",
					Locale:         sourceLang,
					Encoding:       "UTF-8",
					IsMultilingual: true,
					Properties: map[string]string{
						"datatype":        fi.datatype,
						"target-language": string(targetLang),
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
					return
				}

			case "body":
				inBody = true

			case "group":
				if !inBody {
					continue
				}
				groupID := ""
				translateAttr := ""
				ws := xmlSpaceAttr(t.Attr)
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "id":
						groupID = a.Value
					case "translate":
						translateAttr = a.Value
					}
				}
				preserveWSStack = append(preserveWSStack, ws == "preserve")
				groupStack = append(groupStack, translateAttr)

				gs := &model.GroupStart{
					ID:   groupID,
					Name: groupID,
					Properties: map[string]string{
						"translate": translateAttr,
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: gs}) {
					return
				}

			case "trans-unit":
				if !inBody || currentFile == nil {
					continue
				}
				tu := r.parseTransUnit(decoder, t, currentFile)
				if tu == nil {
					continue
				}

				// Check xml:space on this trans-unit
				ws := xmlSpaceAttr(t.Attr)
				preserveWS := ws == "preserve" || inheritPreserveWS()

				sourceLang := model.LocaleID(currentFile.sourceLang)
				targetLang := model.LocaleID(currentFile.targetLang)

				// Check translate attribute on trans-unit and group ancestors
				translatable := tu.translatable
				if translatable {
					// Check group stack for translate="no"
					for _, gt := range groupStack {
						if gt == "no" {
							translatable = false
							break
						}
					}
				}

				block := r.buildBlock(tu, sourceLang, targetLang, translatable, preserveWS)
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}

		case xml.EndElement:
			local := t.Name.Local
			switch local {
			case "file":
				if currentFile != nil {
					if len(preserveWSStack) > 0 {
						preserveWSStack = preserveWSStack[:len(preserveWSStack)-1]
					}
					layer := &model.Layer{
						ID:   fmt.Sprintf("file-%s", currentFile.original),
						Name: currentFile.original,
					}
					r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
					currentFile = nil
				}

			case "body":
				inBody = false

			case "group":
				if len(groupStack) > 0 {
					if len(preserveWSStack) > 0 {
						preserveWSStack = preserveWSStack[:len(preserveWSStack)-1]
					}
					groupStack = groupStack[:len(groupStack)-1]
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{}})
			}
		}
	}
}

// xmlSpaceAttr returns the value of xml:space attribute from attrs, or "".
func xmlSpaceAttr(attrs []xml.Attr) string {
	for _, a := range attrs {
		if a.Name.Local == "space" && (a.Name.Space == "xml" || a.Name.Space == "http://www.w3.org/XML/1998/namespace") {
			return a.Value
		}
	}
	return ""
}

// parsedTransUnit holds parsed trans-unit data.
type parsedTransUnit struct {
	id           string
	resname      string
	translatable bool
	approved     bool
	state        string
	maxWidth     string
	sizeUnit     string

	source    string    // raw inner XML of <source>
	target    string    // raw inner XML of <target>
	segSource []segment // parsed <seg-source> segments

	notes    []parsedNote
	altTrans []parsedAltTrans

	preserveWS bool // xml:space="preserve" on this TU
}

type segment struct {
	mid  string
	text string // inner XML
}

type parsedNote struct {
	text      string
	from      string
	priority  int
	annotates string
}

type parsedAltTrans struct {
	matchQuality float64
	origin       string
	source       string
	target       string
}

// parseTransUnit parses a <trans-unit> element and all its children.
func (r *Reader) parseTransUnit(decoder *xml.Decoder, start xml.StartElement, fi *fileInfo) *parsedTransUnit {
	tu := &parsedTransUnit{
		translatable: true,
	}

	for _, a := range start.Attr {
		switch a.Name.Local {
		case "id":
			tu.id = a.Value
		case "resname":
			tu.resname = a.Value
		case "translate":
			tu.translatable = a.Value != "no"
		case "approved":
			tu.approved = a.Value == "yes"
		case "maxwidth":
			tu.maxWidth = a.Value
		case "size-unit":
			tu.sizeUnit = a.Value
		}
		if a.Name.Local == "space" && (a.Name.Space == "xml" || a.Name.Space == "http://www.w3.org/XML/1998/namespace") {
			tu.preserveWS = a.Value == "preserve"
		}
	}

	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return nil
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "source":
				tu.source = readInnerXML(decoder)
				depth-- // readInnerXML consumed the end element
			case "target":
				tu.target = readInnerXML(decoder)
				depth--
			case "seg-source":
				tu.segSource = parseSegSource(decoder)
				depth--
			case "note":
				n := parseNote(decoder, t)
				tu.notes = append(tu.notes, n)
				depth--
			case "alt-trans":
				at := parseAltTrans(decoder, t)
				tu.altTrans = append(tu.altTrans, at)
				depth--
			default:
				// Skip unknown elements
			}
		case xml.EndElement:
			depth--
		}
	}

	return tu
}

// readInnerXML reads all content until the matching end element, returning inner XML as a string.
func readInnerXML(decoder *xml.Decoder) string {
	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return buf.String()
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			buf.WriteString("<")
			buf.WriteString(t.Name.Local)
			for _, a := range t.Attr {
				buf.WriteString(" ")
				if a.Name.Space != "" {
					buf.WriteString(a.Name.Space)
					buf.WriteString(":")
				}
				buf.WriteString(a.Name.Local)
				buf.WriteString(`="`)
				buf.WriteString(xmlEscapeAttr(a.Value))
				buf.WriteString(`"`)
			}
			buf.WriteString(">")
		case xml.EndElement:
			depth--
			if depth > 0 {
				buf.WriteString("</")
				buf.WriteString(t.Name.Local)
				buf.WriteString(">")
			}
		case xml.CharData:
			buf.WriteString(xmlEscapeText(string(t)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write(t)
			buf.WriteString("-->")
		}
	}
	return buf.String()
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

// parseSegSource parses <seg-source> and returns mrk segments.
func parseSegSource(decoder *xml.Decoder) []segment {
	var segs []segment
	depth := 1

	var currentSeg *segment
	var buf strings.Builder

	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "mrk" {
				mtype := ""
				mid := ""
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "mtype":
						mtype = a.Value
					case "mid":
						mid = a.Value
					}
				}
				if mtype == "seg" {
					buf.Reset()
					currentSeg = &segment{mid: mid}
				} else {
					// Non-seg mrk — write it as inline content
					if currentSeg != nil {
						buf.WriteString("<mrk")
						for _, a := range t.Attr {
							buf.WriteString(" ")
							buf.WriteString(a.Name.Local)
							buf.WriteString(`="`)
							buf.WriteString(xmlEscapeAttr(a.Value))
							buf.WriteString(`"`)
						}
						buf.WriteString(">")
					}
				}
			} else if currentSeg != nil {
				// Inline element within mrk: preserve as-is
				buf.WriteString("<")
				buf.WriteString(t.Name.Local)
				for _, a := range t.Attr {
					buf.WriteString(" ")
					if a.Name.Space != "" {
						buf.WriteString(a.Name.Space)
						buf.WriteString(":")
					}
					buf.WriteString(a.Name.Local)
					buf.WriteString(`="`)
					buf.WriteString(xmlEscapeAttr(a.Value))
					buf.WriteString(`"`)
				}
				buf.WriteString(">")
			} else {
				// Inline element outside mrk (e.g., bpt/ept spanning segments)
				buf.WriteString("<")
				buf.WriteString(t.Name.Local)
				for _, a := range t.Attr {
					buf.WriteString(" ")
					buf.WriteString(a.Name.Local)
					buf.WriteString(`="`)
					buf.WriteString(xmlEscapeAttr(a.Value))
					buf.WriteString(`"`)
				}
				buf.WriteString(">")
			}
		case xml.EndElement:
			depth--
			if depth > 0 {
				if t.Name.Local == "mrk" && currentSeg != nil {
					currentSeg.text = buf.String()
					segs = append(segs, *currentSeg)
					currentSeg = nil
				} else if currentSeg != nil {
					buf.WriteString("</")
					buf.WriteString(t.Name.Local)
					buf.WriteString(">")
				}
			}
		case xml.CharData:
			if currentSeg != nil {
				// Inside a mrk segment — preserve raw text (don't escape already-decoded text)
				buf.Write(t)
			}
		}
	}
	return segs
}

// parseNote parses a <note> element and returns parsed data.
func parseNote(decoder *xml.Decoder, start xml.StartElement) parsedNote {
	n := parsedNote{}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "from":
			n.from = a.Value
		case "priority":
			if p, err := strconv.Atoi(a.Value); err == nil {
				n.priority = p
			}
		case "annotates":
			n.annotates = a.Value
		}
	}

	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			buf.Write(t)
		}
	}
	n.text = buf.String()
	return n
}

// parseAltTrans parses an <alt-trans> element.
func parseAltTrans(decoder *xml.Decoder, start xml.StartElement) parsedAltTrans {
	at := parsedAltTrans{}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "match-quality":
			if f, err := strconv.ParseFloat(a.Value, 64); err == nil {
				at.matchQuality = f
			}
		case "origin":
			at.origin = a.Value
		}
	}

	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "source":
				at.source = readInnerXMLCharData(decoder)
				depth--
			case "target":
				at.target = readInnerXMLCharData(decoder)
				depth--
			}
		case xml.EndElement:
			depth--
		}
	}
	return at
}

// readInnerXMLCharData reads character data until end element, ignoring child elements.
func readInnerXMLCharData(decoder *xml.Decoder) string {
	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			buf.Write(t)
		}
	}
	return buf.String()
}

// buildBlock creates a Block from parsed trans-unit data.
func (r *Reader) buildBlock(tu *parsedTransUnit, sourceLang, targetLang model.LocaleID, translatable, preserveWS bool) *model.Block {
	block := &model.Block{
		ID:                 tu.id,
		Name:               tu.id,
		Translatable:       translatable,
		PreserveWhitespace: preserveWS || tu.preserveWS,
		Properties:         make(map[string]string),
		Annotations:        make(map[string]Annotation),
		Targets:            make(map[model.LocaleID][]*model.Segment),
	}

	if tu.resname != "" {
		block.Name = tu.resname
		block.Properties["resname"] = tu.resname
	}

	if tu.approved {
		block.Properties["approved"] = "yes"
	}

	if tu.state != "" {
		block.Properties["state"] = tu.state
	}

	if tu.maxWidth != "" {
		block.Properties["maxwidth"] = tu.maxWidth
	}
	if tu.sizeUnit != "" {
		block.Properties["size-unit"] = tu.sizeUnit
	}

	// Build source segments
	if len(tu.segSource) > 0 {
		// Use seg-source segments
		block.Source = make([]*model.Segment, len(tu.segSource))
		for i, seg := range tu.segSource {
			block.Source[i] = &model.Segment{
				ID:      seg.mid,
				Content: parseInlineContent(seg.text),
			}
		}
	} else {
		// Use <source> content
		block.Source = []*model.Segment{{
			ID:      "s1",
			Content: parseInlineContent(tu.source),
		}}
	}

	// Build target segments
	targetContent := tu.target
	if targetContent != "" && !targetLang.IsEmpty() {
		// Check if target has mrk segments
		targetSegs := parseMrkSegmentsFromString(targetContent)
		if len(targetSegs) > 0 {
			tgtSegs := make([]*model.Segment, len(targetSegs))
			for i, seg := range targetSegs {
				tgtSegs[i] = &model.Segment{
					ID:      seg.mid,
					Content: parseInlineContent(seg.text),
				}
			}
			block.Targets[targetLang] = tgtSegs
		} else {
			block.Targets[targetLang] = []*model.Segment{{
				ID:      "s1",
				Content: parseInlineContent(targetContent),
			}}
		}
	}

	// Add notes
	for i, note := range tu.notes {
		key := "note"
		if i > 0 {
			key = fmt.Sprintf("note-%d", i)
		}
		block.Annotations[key] = &model.NoteAnnotation{
			Text:      note.text,
			From:      note.from,
			Priority:  note.priority,
			Annotates: note.annotates,
		}
	}

	// Add alt-trans
	for i, at := range tu.altTrans {
		key := "alt-translation"
		if i > 0 {
			key = fmt.Sprintf("alt-translation-%d", i)
		}

		matchType := ""
		if at.matchQuality >= 100 {
			matchType = "EXACT"
		} else if at.matchQuality > 0 {
			matchType = "FUZZY"
		}

		alt := &model.AltTranslation{
			Origin:        at.origin,
			CombinedScore: at.matchQuality,
			MatchType:     matchType,
			FromOriginal:  true,
		}
		if at.source != "" {
			alt.Source = model.NewFragment(at.source)
		}
		if at.target != "" {
			alt.Target = model.NewFragment(at.target)
		}
		block.Annotations[key] = alt
	}

	return block
}

// Annotation is an alias for model.Annotation to make things cleaner.
type Annotation = model.Annotation

// parseInlineContent parses XLIFF 1.2 inline elements and returns a Fragment.
func parseInlineContent(innerXML string) *model.Fragment {
	if innerXML == "" {
		return model.NewFragment("")
	}

	// Try parsing as XML to handle inline codes
	frag := model.NewFragment("")
	frag.Spans = nil // Clear default

	// Wrap in a root element for parsing
	wrapped := "<root>" + innerXML + "</root>"
	decoder := xml.NewDecoder(strings.NewReader(wrapped))
	decoder.Strict = false

	var textBuf strings.Builder
	var spans []*model.Span
	depth := 0

	flushText := func() {
		// Text is accumulated in textBuf and set at the end — nothing to flush mid-parse
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				// Root element
				continue
			}

			switch t.Name.Local {
			case "bpt":
				// Beginning paired tag — opening code
				id := attrVal(t.Attr, "id")
				data := readElementText(decoder)
				depth-- // readElementText consumed end
				flushText()
				spans = append(spans, &model.Span{
					SpanType:  model.SpanOpening,
					Type:      ctypeToSpanType(attrVal(t.Attr, "ctype")),
					ID:        id,
					Data:      data,
					EquivText: attrVal(t.Attr, "equiv-text"),
				})

			case "ept":
				// Ending paired tag — closing code
				id := attrVal(t.Attr, "id")
				data := readElementText(decoder)
				depth--
				flushText()
				spans = append(spans, &model.Span{
					SpanType:  model.SpanClosing,
					Type:      ctypeToSpanType(attrVal(t.Attr, "ctype")),
					ID:        id,
					Data:      data,
					EquivText: attrVal(t.Attr, "equiv-text"),
				})

			case "ph":
				// Placeholder — standalone code
				id := attrVal(t.Attr, "id")
				data := readElementText(decoder)
				depth--
				flushText()
				spans = append(spans, &model.Span{
					SpanType:  model.SpanPlaceholder,
					Type:      ctypeToSpanType(attrVal(t.Attr, "ctype")),
					ID:        id,
					Data:      data,
					EquivText: attrVal(t.Attr, "equiv-text"),
				})

			case "x":
				// Standalone code (self-closing)
				id := attrVal(t.Attr, "id")
				flushText()
				spans = append(spans, &model.Span{
					SpanType:  model.SpanPlaceholder,
					Type:      ctypeToSpanType(attrVal(t.Attr, "ctype")),
					ID:        id,
					EquivText: attrVal(t.Attr, "equiv-text"),
				})

			case "bx":
				// Beginning of paired code (self-closing form)
				id := attrVal(t.Attr, "id")
				flushText()
				spans = append(spans, &model.Span{
					SpanType:  model.SpanOpening,
					Type:      ctypeToSpanType(attrVal(t.Attr, "ctype")),
					ID:        id,
					EquivText: attrVal(t.Attr, "equiv-text"),
				})

			case "ex":
				// End of paired code (self-closing form)
				id := attrVal(t.Attr, "id")
				flushText()
				spans = append(spans, &model.Span{
					SpanType:  model.SpanClosing,
					Type:      ctypeToSpanType(attrVal(t.Attr, "ctype")),
					ID:        id,
					EquivText: attrVal(t.Attr, "equiv-text"),
				})

			case "g":
				// Generic group inline element — treated as opening
				id := attrVal(t.Attr, "id")
				flushText()
				spans = append(spans, &model.Span{
					SpanType:  model.SpanOpening,
					Type:      ctypeToSpanType(attrVal(t.Attr, "ctype")),
					ID:        id,
					EquivText: attrVal(t.Attr, "equiv-text"),
				})

			case "it":
				// Isolated code (opening or closing depending on pos)
				id := attrVal(t.Attr, "id")
				pos := attrVal(t.Attr, "pos")
				data := readElementText(decoder)
				depth--
				flushText()
				spanType := model.SpanPlaceholder
				if pos == "open" {
					spanType = model.SpanOpening
				} else if pos == "close" {
					spanType = model.SpanClosing
				}
				spans = append(spans, &model.Span{
					SpanType:  spanType,
					Type:      ctypeToSpanType(attrVal(t.Attr, "ctype")),
					ID:        id,
					Data:      data,
					EquivText: attrVal(t.Attr, "equiv-text"),
				})

			case "mrk":
				// mrk elements in source/target (non-seg mrk like mtype="term", "protected", etc.)
				// Treat as an annotation span — opening
				id := attrVal(t.Attr, "mid")
				mtype := attrVal(t.Attr, "mtype")
				flushText()
				spans = append(spans, &model.Span{
					SpanType: model.SpanOpening,
					Type:     "xliff:mrk:" + mtype,
					ID:       id,
				})

			case "sub":
				// Translatable sub-flow inside inline codes. Read content.
				readElementText(decoder)
				depth--

			default:
				// Unknown inline element — skip content
			}

		case xml.EndElement:
			depth--
			if depth == 0 {
				// Root end
				continue
			}
			// Check for end of <g> — emit closing span
			if t.Name.Local == "g" {
				flushText()
				// Find the matching opening span to get its ID
				var gID string
				for i := len(spans) - 1; i >= 0; i-- {
					if spans[i].SpanType == model.SpanOpening && spans[i].ID != "" {
						gID = spans[i].ID
						break
					}
				}
				spans = append(spans, &model.Span{
					SpanType: model.SpanClosing,
					ID:       gID,
				})
			} else if t.Name.Local == "mrk" {
				flushText()
				spans = append(spans, &model.Span{
					SpanType: model.SpanClosing,
					Type:     "xliff:mrk",
				})
			}

		case xml.CharData:
			if depth >= 1 {
				textBuf.Write(t)
			}
		}
	}

	result := model.NewFragment(textBuf.String())
	result.Spans = spans
	return result
}

// readElementText reads text content of an element until its end tag.
// It handles nested elements like <sub>.
func readElementText(decoder *xml.Decoder) string {
	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			buf.Write(t)
		}
	}
	return buf.String()
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

// ctypeToSpanType converts a ctype attribute to a semantic span type.
func ctypeToSpanType(ctype string) string {
	switch ctype {
	case "bold", "x-bold":
		return "fmt:bold"
	case "italic", "x-italic":
		return "fmt:italic"
	case "underlined", "x-underlined":
		return "fmt:underline"
	case "link", "x-link":
		return "link:hyperlink"
	case "lb", "x-lb":
		return "struct:break"
	case "image", "x-image":
		return "media:image"
	case "":
		return ""
	default:
		return "xliff:" + ctype
	}
}

// parseMrkSegmentsFromString parses mrk mtype="seg" elements from a target string.
func parseMrkSegmentsFromString(targetXML string) []segment {
	var segs []segment
	wrapped := "<root>" + targetXML + "</root>"
	decoder := xml.NewDecoder(strings.NewReader(wrapped))
	decoder.Strict = false

	depth := 0
	var currentSeg *segment
	var buf strings.Builder

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "mrk" && depth == 2 {
				mtype := attrVal(t.Attr, "mtype")
				mid := attrVal(t.Attr, "mid")
				if mtype == "seg" {
					buf.Reset()
					currentSeg = &segment{mid: mid}
					continue
				}
			}
			if currentSeg != nil {
				// Inline element inside mrk
				buf.WriteString("<")
				buf.WriteString(t.Name.Local)
				for _, a := range t.Attr {
					buf.WriteString(" ")
					buf.WriteString(a.Name.Local)
					buf.WriteString(`="`)
					buf.WriteString(xmlEscapeAttr(a.Value))
					buf.WriteString(`"`)
				}
				buf.WriteString(">")
			}
		case xml.EndElement:
			depth--
			if t.Name.Local == "mrk" && currentSeg != nil {
				currentSeg.text = buf.String()
				segs = append(segs, *currentSeg)
				currentSeg = nil
			} else if currentSeg != nil {
				buf.WriteString("</")
				buf.WriteString(t.Name.Local)
				buf.WriteString(">")
			}
		case xml.CharData:
			if currentSeg != nil {
				buf.Write(t)
			}
		}
	}
	return segs
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
