package openxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// wmlNamespace is the WordprocessingML namespace.
const wmlNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// textRun holds a single run's text and formatting within a paragraph.
type textRun struct {
	text  string
	props runProps
}

// wmlParser parses WordprocessingML XML parts (document.xml, headers, footers, etc.).
type wmlParser struct {
	cfg           *Config
	blockCounter  *int
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	rels          map[string]relationship // hyperlink rels for this part
}

// parsePart streams through a WordprocessingML XML part, emitting Blocks.
func (p *wmlParser) parsePart(data []byte, partPath string, emitBlock func(*model.Block), emitData func()) error {
	d := xml.NewDecoder(bytes.NewReader(data))

	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("wml: parsing %s: %w", partPath, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				if isWML(t) || isWMLNoNS(t) {
					if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
						return err
					}
				} else {
					p.skelWriteStartElement(t)
				}
			case "sdt":
				// Structured document tag — recurse into content
				if err := p.parseSDT(d, partPath, emitBlock, emitData); err != nil {
					return err
				}
			case "tbl":
				// Table — recurse to find paragraphs inside cells
				p.skelWriteStartElement(t)
			case "footnote", "endnote":
				// Skip the auto-generated separator footnotes (id 0 and 1)
				id := attrVal(t, "id")
				if id == "0" || id == "1" || id == "-1" {
					p.skelWriteStartElement(t)
					if err := p.skipAndSkel(d); err != nil {
						return err
					}
					continue
				}
				p.skelWriteStartElement(t)
			case "pPr", "sectPr", "tblPr", "tblGrid", "trPr", "tcPr":
				// Non-translatable properties — skeleton only
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				p.skelText(raw)
			default:
				p.skelWriteStartElement(t)
			}

		case xml.EndElement:
			p.skelWriteEndElement(t)

		case xml.CharData:
			p.skelText(xmlEscape(string(t)))

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")

		case xml.Directive:
			p.skelText("<!" + string(t) + ">")

		case xml.Comment:
			p.skelText("<!--" + string(t) + "-->")
		}
	}
	return nil
}

// parseParagraph parses a <w:p> element and emits a Block if it contains text.
func (p *wmlParser) parseParagraph(d *xml.Decoder, partPath string, emitBlock func(*model.Block)) error {
	var runs []textRun
	var hyperlinkRuns []textRun
	var inHyperlink bool
	var hyperlinkID string
	var paraProps string

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pPr":
				// Capture paragraph properties for skeleton
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				paraProps = raw

			case "r":
				// Text run
				run, err := p.parseRun(d)
				if err != nil {
					return err
				}
				if inHyperlink {
					hyperlinkRuns = append(hyperlinkRuns, run...)
				} else {
					runs = append(runs, run...)
				}

			case "hyperlink":
				inHyperlink = true
				hyperlinkID = attrVal(t, "id")
				hyperlinkRuns = nil

			case "bookmarkStart", "bookmarkEnd",
				"proofErr", "commentRangeStart", "commentRangeEnd",
				"permStart", "permEnd":
				if err := skipElement(d); err != nil {
					return err
				}

			case "sdt":
				// Inline structured document tag — recurse
				sdtRuns, err := p.parseInlineSDT(d)
				if err != nil {
					return err
				}
				runs = append(runs, sdtRuns...)

			default:
				if err := skipElement(d); err != nil {
					return err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "hyperlink" {
				if inHyperlink && len(hyperlinkRuns) > 0 {
					runs = append(runs, p.wrapHyperlinkRuns(hyperlinkRuns, hyperlinkID)...)
				}
				inHyperlink = false
				hyperlinkID = ""
				continue
			}

			if t.Name.Local == "p" {
				// Merge adjacent runs with same formatting
				merged := mergeRuns(runs)

				// Skip empty paragraphs
				if isEmptyRuns(merged) {
					p.skelWriteString("<w:p>")
					if paraProps != "" {
						p.skelText(paraProps)
					}
					p.skelWriteString("</w:p>")
					return nil
				}

				// Skip hidden text unless configured
				if !p.cfg.TranslateHiddenText && allHidden(merged) {
					p.skelWriteString("<w:p>")
					if paraProps != "" {
						p.skelText(paraProps)
					}
					// Write runs as skeleton text
					for _, r := range merged {
						p.skelText(runToXML(r))
					}
					p.skelWriteString("</w:p>")
					return nil
				}

				// Build block
				*p.blockCounter++
				blockID := fmt.Sprintf("tu%d", *p.blockCounter)

				// Skeleton: write paragraph open, props, ref, close
				p.skelWriteString("<w:p>")
				if paraProps != "" {
					p.skelText(paraProps)
				}
				p.skelRef(blockID)
				p.skelWriteString("</w:p>")

				block := p.buildBlock(blockID, merged, partPath)
				emitBlock(block)
				return nil
			}
		}
	}
}

// parseRun parses a <w:r> element and returns its text runs.
func (p *wmlParser) parseRun(d *xml.Decoder) ([]textRun, error) {
	var props runProps
	var runs []textRun
	hasProps := false

	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "rPr":
				hasProps = true
				props, err = parseRunProps(d, p.cfg.AggressiveCleanup)
				if err != nil {
					return nil, err
				}

			case "t":
				text, err := readCharData(d)
				if err != nil {
					return nil, err
				}
				_ = hasProps
				runs = append(runs, textRun{text: text, props: props})

			case "br":
				runs = append(runs, textRun{
					text:  "\n",
					props: runProps{}, // break has no formatting
				})
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "tab":
				if p.cfg.TabAsCharacter {
					runs = append(runs, textRun{text: "\t", props: props})
				} else {
					// Tab as placeholder — handled as special run
					runs = append(runs, textRun{text: "\uE100", props: props}) // sentinel
				}
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "drawing", "pict", "object":
				// Non-text inline content — placeholder
				runs = append(runs, textRun{text: "\uE101", props: props}) // image sentinel
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "footnoteReference", "endnoteReference":
				noteID := attrVal(t, "id")
				runs = append(runs, textRun{text: "\uE102:" + noteID, props: props}) // footnote sentinel
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "sym":
				// Symbol character
				char := attrVal(t, "char")
				if char != "" {
					runs = append(runs, textRun{text: "[sym:" + char + "]", props: props})
				}
				if err := skipElement(d); err != nil {
					return nil, err
				}

			default:
				if err := skipElement(d); err != nil {
					return nil, err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "r" {
				return runs, nil
			}
		}
	}
}

// parseSDT parses a structured document tag, extracting its content.
func (p *wmlParser) parseSDT(d *xml.Decoder, partPath string, emitBlock func(*model.Block), emitData func()) error {
	depth := 1
	inContent := false

	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "sdtContent":
				inContent = true
			case "sdtPr":
				// Skip SDT properties
				if err := skipElement(d); err != nil {
					return err
				}
				depth--
			case "p":
				if inContent {
					if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
						return err
					}
					depth--
				}
			}
		case xml.EndElement:
			depth--
			if t.Name.Local == "sdtContent" {
				inContent = false
			}
		}
	}
	return nil
}

// parseInlineSDT parses an inline SDT and returns its text runs.
func (p *wmlParser) parseInlineSDT(d *xml.Decoder) ([]textRun, error) {
	var runs []textRun
	depth := 1

	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "sdtPr":
				if err := skipElement(d); err != nil {
					return nil, err
				}
				depth--
			case "r":
				r, err := p.parseRun(d)
				if err != nil {
					return nil, err
				}
				runs = append(runs, r...)
				depth--
			}
		case xml.EndElement:
			depth--
		}
	}
	return runs, nil
}

// wrapHyperlinkRuns wraps runs in hyperlink opening/closing markers.
func (p *wmlParser) wrapHyperlinkRuns(runs []textRun, relID string) []textRun {
	// Resolve the hyperlink URL from relationships
	url := ""
	if rel, ok := p.rels[relID]; ok {
		url = rel.Target
	}

	data := "<w:hyperlink>"
	if url != "" {
		data = fmt.Sprintf(`<w:hyperlink r:id="%s" href="%s">`, xmlEscapeAttr(relID), xmlEscapeAttr(url))
	}

	// Create wrapper with sentinel markers
	var result []textRun
	result = append(result, textRun{text: "\uE103:" + data, props: runProps{}})   // hyperlink open sentinel
	result = append(result, runs...)
	result = append(result, textRun{text: "\uE104:" + data, props: runProps{}})   // hyperlink close sentinel
	return result
}

// buildBlock builds a model.Block from a list of merged text runs.
func (p *wmlParser) buildBlock(id string, runs []textRun, partPath string) *model.Block {
	frag := &model.Fragment{}
	spanCounter := 0

	var activeProps *runProps

	for _, run := range runs {
		// Handle sentinel markers for special content
		if strings.HasPrefix(run.text, "\uE100") {
			// Tab placeholder
			spanCounter++
			frag.AppendSpan(&model.Span{
				SpanType:  model.SpanPlaceholder,
				Type:      TypeTab,
				SubType:   SubTypeTab,
				ID:        fmt.Sprintf("c%d", spanCounter),
				Data:      "<w:tab/>",
				Deletable: false,
				EquivText: "\t",
			})
			continue
		}
		if strings.HasPrefix(run.text, "\uE101") {
			// Image/drawing placeholder
			spanCounter++
			frag.AppendSpan(&model.Span{
				SpanType:  model.SpanPlaceholder,
				Type:      TypeImage,
				SubType:   SubTypeImage,
				ID:        fmt.Sprintf("c%d", spanCounter),
				Data:      "<w:drawing/>",
				Deletable: false,
			})
			continue
		}
		if strings.HasPrefix(run.text, "\uE102:") {
			// Footnote/endnote reference
			noteID := strings.TrimPrefix(run.text, "\uE102:")
			spanCounter++
			frag.AppendSpan(&model.Span{
				SpanType:    model.SpanPlaceholder,
				Type:        TypeFootnoteRef,
				SubType:     SubTypeFootnoteRef,
				ID:          fmt.Sprintf("c%d", spanCounter),
				Data:        fmt.Sprintf(`<w:footnoteReference w:id="%s"/>`, noteID),
				DisplayText: fmt.Sprintf("[%s]", noteID),
				Deletable:   false,
			})
			continue
		}
		if strings.HasPrefix(run.text, "\uE103:") {
			// Hyperlink open
			data := strings.TrimPrefix(run.text, "\uE103:")
			spanCounter++
			frag.AppendSpan(&model.Span{
				SpanType:   model.SpanOpening,
				Type:       TypeHyperlink,
				SubType:    SubTypeHyperlink,
				ID:         fmt.Sprintf("c%d", spanCounter),
				Data:       data,
				Deletable:  true,
				Cloneable:  true,
				CanReorder: true,
			})
			continue
		}
		if strings.HasPrefix(run.text, "\uE104:") {
			// Hyperlink close
			if activeProps != nil && !activeProps.isEmpty() {
				// Close formatting before hyperlink close
				for _, s := range activeProps.closingSpans(&spanCounter) {
					frag.AppendSpan(s)
				}
				activeProps = nil
			}
			spanCounter++
			frag.AppendSpan(&model.Span{
				SpanType:   model.SpanClosing,
				Type:       TypeHyperlink,
				SubType:    SubTypeHyperlink,
				ID:         fmt.Sprintf("c%d", spanCounter),
				Data:       "</w:hyperlink>",
				Deletable:  true,
				Cloneable:  true,
				CanReorder: true,
			})
			continue
		}

		// Handle line break
		if run.text == "\n" {
			spanCounter++
			frag.AppendSpan(&model.Span{
				SpanType:  model.SpanPlaceholder,
				Type:      TypeBreak,
				SubType:   SubTypeBreak,
				ID:        fmt.Sprintf("c%d", spanCounter),
				Data:      "<w:br/>",
				EquivText: "\n",
				Deletable: false,
			})
			continue
		}

		// Handle formatting changes
		if activeProps == nil || !activeProps.equal(run.props) {
			// Close previous formatting
			if activeProps != nil && !activeProps.isEmpty() {
				for _, s := range activeProps.closingSpans(&spanCounter) {
					frag.AppendSpan(s)
				}
			}
			// Open new formatting
			if !run.props.isEmpty() {
				for _, s := range run.props.openingSpans(&spanCounter) {
					frag.AppendSpan(s)
				}
			}
			propsCopy := run.props
			activeProps = &propsCopy
		}

		frag.AppendText(run.text)
	}

	// Close any remaining open formatting
	if activeProps != nil && !activeProps.isEmpty() {
		for _, s := range activeProps.closingSpans(&spanCounter) {
			frag.AppendSpan(s)
		}
	}

	block := &model.Block{
		ID:           id,
		Type:         "paragraph",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: frag}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   map[string]string{"partPath": partPath},
		Annotations:  make(map[string]model.Annotation),
	}
	return block
}

// mergeRuns merges adjacent runs with identical formatting.
func mergeRuns(runs []textRun) []textRun {
	if len(runs) <= 1 {
		return runs
	}

	var merged []textRun
	current := runs[0]

	for i := 1; i < len(runs); i++ {
		r := runs[i]
		// Don't merge sentinel markers or line breaks
		if isSentinel(current.text) || isSentinel(r.text) ||
			current.text == "\n" || r.text == "\n" {
			merged = append(merged, current)
			current = r
			continue
		}
		if current.props.equal(r.props) {
			current.text += r.text
		} else {
			merged = append(merged, current)
			current = r
		}
	}
	merged = append(merged, current)
	return merged
}

// isSentinel returns true if the text is a special marker.
func isSentinel(s string) bool {
	r := []rune(s)
	if len(r) == 0 {
		return false
	}
	if r[0] < '\uE100' || r[0] > '\uE104' {
		return false
	}
	// Single-char sentinels (tab \uE100, image \uE101)
	if len(r) == 1 {
		return true
	}
	// Multi-char sentinels must have ':' separator (\uE102:id, \uE103:data, \uE104:data)
	return len(r) >= 2 && r[1] == ':'
}

// isEmptyRuns returns true if all runs have no visible text content.
func isEmptyRuns(runs []textRun) bool {
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if strings.TrimSpace(r.text) != "" {
			return false
		}
	}
	return true
}

// allHidden returns true if all runs have the vanish property.
func allHidden(runs []textRun) bool {
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if !r.props.vanish && strings.TrimSpace(r.text) != "" {
			return false
		}
	}
	return true
}

// runToXML converts a text run back to XML for skeleton output.
func runToXML(r textRun) string {
	var buf strings.Builder
	buf.WriteString("<w:r>")
	if !r.props.isEmpty() {
		buf.WriteString("<w:rPr>")
		if r.props.bold {
			buf.WriteString("<w:b/>")
		}
		if r.props.italic {
			buf.WriteString("<w:i/>")
		}
		if r.props.underline != "" {
			buf.WriteString(`<w:u w:val="` + r.props.underline + `"/>`)
		}
		if r.props.strike {
			buf.WriteString("<w:strike/>")
		}
		if r.props.vertAlign != "" {
			buf.WriteString(`<w:vertAlign w:val="` + r.props.vertAlign + `"/>`)
		}
		if r.props.vanish {
			buf.WriteString("<w:vanish/>")
		}
		buf.WriteString("</w:rPr>")
	}
	buf.WriteString(`<w:t xml:space="preserve">`)
	buf.WriteString(xmlEscape(r.text))
	buf.WriteString("</w:t></w:r>")
	return buf.String()
}

// Skeleton helpers

func (p *wmlParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *wmlParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		_ = p.skeletonStore.WriteRef(id)
	}
}

func (p *wmlParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

func (p *wmlParser) skelWriteStartElement(t xml.StartElement) {
	if p.skeletonStore == nil {
		return
	}
	registerNamespaces(t.Attr)
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, t.Name)
	for _, a := range t.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *wmlParser) skelWriteEndElement(t xml.EndElement) {
	if p.skeletonStore == nil {
		return
	}
	var buf strings.Builder
	buf.WriteString("</")
	writeElementName(&buf, t.Name)
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *wmlParser) skelWriteString(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *wmlParser) skipAndSkel(d *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			p.skelWriteStartElement(t)
		case xml.EndElement:
			depth--
			p.skelWriteEndElement(t)
		case xml.CharData:
			p.skelText(xmlEscape(string(t)))
		}
	}
	return nil
}

// XML helpers

// nsRegistry tracks namespace URI → prefix mappings discovered during parsing.
// It supplements the static nsPrefixMap with dynamic mappings from xmlns: attributes.
var nsRegistry = struct {
	m map[string]string
}{m: make(map[string]string)}

// registerNamespaces scans an element's attributes for xmlns declarations
// and records the prefix → URI mapping.
func registerNamespaces(attrs []xml.Attr) {
	for _, a := range attrs {
		if a.Name.Space == "xmlns" {
			// xmlns:prefix="URI"
			nsRegistry.m[a.Value] = a.Name.Local
		}
	}
}

// resolvePrefix returns the namespace prefix for a URI, checking the dynamic
// registry first, then the static map.
func resolvePrefix(ns string) string {
	if p, ok := nsPrefixMap[ns]; ok {
		return p
	}
	if p, ok := nsRegistry.m[ns]; ok {
		return p
	}
	return ""
}

// writeElementName writes an element name with its namespace prefix.
func writeElementName(buf *strings.Builder, name xml.Name) {
	if name.Space != "" {
		prefix := resolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
		// If no known prefix, write local name only — the namespace is
		// already declared on a parent element via xmlns.
	}
	buf.WriteString(name.Local)
}

// writeAttrName writes an attribute name, handling xmlns declarations.
func writeAttrName(buf *strings.Builder, name xml.Name) {
	if name.Space == "xmlns" {
		// Namespace declaration: xmlns:prefix
		buf.WriteString("xmlns:")
		buf.WriteString(name.Local)
		return
	}
	if name.Space == "" && name.Local == "xmlns" {
		// Default namespace declaration
		buf.WriteString("xmlns")
		return
	}
	if name.Space != "" {
		prefix := resolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
		// Unknown namespace — omit the prefix. The namespace is
		// already declared on a parent element and the attribute
		// name alone is sufficient for well-formed output.
	}
	buf.WriteString(name.Local)
}

// xmlEscapeAttr escapes a string for use as an XML attribute value.
func xmlEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// nsPrefix maps namespace URI → prefix for known OpenXML namespaces.
var nsPrefixMap = map[string]string{
	wmlNamespace: "w",
	dmlNamespace: "a",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships":                     "r",
	"http://schemas.openxmlformats.org/markup-compatibility/2006":                              "mc",
	"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing":                   "wp",
	"http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing":                      "xdr",
	"http://schemas.openxmlformats.org/drawingml/2006/chart":                                   "c",
	"http://schemas.openxmlformats.org/drawingml/2006/diagram":                                 "dgm",
	"http://schemas.openxmlformats.org/drawingml/2006/picture":                                 "pic",
	"http://schemas.openxmlformats.org/officeDocument/2006/math":                               "m",
	"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties":                 "ep",
	"http://schemas.openxmlformats.org/officeDocument/2006/custom-properties":                   "cp",
	"http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes":                      "vt",
	"http://schemas.openxmlformats.org/spreadsheetml/2006/main":                                "x",
	"http://schemas.openxmlformats.org/presentationml/2006/main":                               "p",
	"http://schemas.openxmlformats.org/package/2006/relationships":                             "pr",
	"http://schemas.openxmlformats.org/package/2006/content-types":                             "ct",
	"http://schemas.openxmlformats.org/package/2006/metadata/core-properties":                  "coreProperties",
	"http://schemas.microsoft.com/office/word/2010/wordml":                                     "w14",
	"http://schemas.microsoft.com/office/word/2012/wordml":                                     "w15",
	"http://schemas.microsoft.com/office/word/2015/wordml/symex":                               "w16se",
	"http://schemas.microsoft.com/office/spreadsheetml/2009/9/main":                            "x14",
	"http://schemas.microsoft.com/office/spreadsheetml/2010/11/main":                           "x15",
	"http://schemas.microsoft.com/office/powerpoint/2010/main":                                 "p14",
	"http://schemas.microsoft.com/office/powerpoint/2012/main":                                 "p15",
	"http://schemas.microsoft.com/office/drawing/2010/main":                                    "a14",
	"http://schemas.microsoft.com/office/drawing/2014/main":                                    "a16",
	"http://purl.org/dc/elements/1.1/":                                                         "dc",
	"http://purl.org/dc/terms/":                                                                "dcterms",
	"http://schemas.openxmlformats.org/officeDocument/2006/customXml":                          "ds",
	"urn:schemas-microsoft-com:vml":                                                            "v",
	"urn:schemas-microsoft-com:office:office":                                                  "o",
	"urn:schemas-microsoft-com:office:word":                                                    "w10",
	"http://www.w3.org/2001/XMLSchema-instance":                                                "xsi",
	"http://www.w3.org/2001/XMLSchema":                                                         "xsd",
	"http://www.w3.org/XML/1998/namespace":                                                     "xml",
	// Microsoft Office extension namespaces
	"http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas":                        "wpc",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing":                       "wp14",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingGroup":                         "wpg",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingInk":                           "wpi",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingShape":                         "wps",
	"http://schemas.microsoft.com/office/word/2006/wordml":                                     "wne",
	"http://schemas.microsoft.com/office/mac/office/2008/main":                                  "mo",
	"urn:schemas-microsoft-com:mac:vml":                                                        "mv",
	"http://schemas.microsoft.com/office/drawing/2012/chart":                                   "c15",
	"http://schemas.microsoft.com/office/drawing/2014/chartex":                                 "cx",
	"http://schemas.openxmlformats.org/drawingml/2006/lockedCanvas":                            "lc",
	"http://schemas.microsoft.com/office/drawing/2008/diagram":                                 "dsp",
	"http://schemas.microsoft.com/office/drawing/2010/diagram":                                 "dgm14",
	"http://schemas.microsoft.com/office/thememl/2012/main":                                    "thm15",
	"http://schemas.microsoft.com/office/drawing/2017/decorative":                              "adec",
	"http://schemas.microsoft.com/office/drawing/2018/hyperlinkcolor":                          "ahlc",
	"http://schemas.microsoft.com/office/word/2016/wordml/cid":                                 "w16cid",
	"http://schemas.microsoft.com/office/word/2018/wordml":                                     "w16",
	"http://schemas.microsoft.com/office/word/2018/wordml/cex":                                 "w16cex",
	"http://schemas.microsoft.com/office/word/2020/wordml/sdtdatahash":                         "w16sdtdh",
}

func isWML(el xml.StartElement) bool {
	return el.Name.Space == wmlNamespace
}

func isWMLNoNS(el xml.StartElement) bool {
	return el.Name.Space == ""
}

// readCharData reads character data content of a simple element and consumes its end tag.
func readCharData(d *xml.Decoder) (string, error) {
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			return text.String(), nil
		case xml.StartElement:
			// Unexpected nested element — skip it
			if err := skipElement(d); err != nil {
				return "", err
			}
		}
	}
}

// captureRawElement captures an entire element (start to end) as raw XML.
func captureRawElement(d *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, start.Name)
	for _, a := range start.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")

	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			buf.WriteString("<")
			writeElementName(&buf, t.Name)
			for _, a := range t.Attr {
				buf.WriteString(" ")
				writeAttrName(&buf, a.Name)
				buf.WriteString(`="`)
				buf.WriteString(xmlEscapeAttr(a.Value))
				buf.WriteString(`"`)
			}
			buf.WriteString(">")
		case xml.EndElement:
			depth--
			buf.WriteString("</")
			writeElementName(&buf, t.Name)
			buf.WriteString(">")
		case xml.CharData:
			buf.WriteString(xmlEscape(string(t)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write(t)
			buf.WriteString("-->")
		}
	}
	return buf.String(), nil
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// xmlEscapeRune writes a single rune to a string builder, XML-escaping if needed.
func xmlEscapeRune(buf *strings.Builder, r rune) {
	switch r {
	case '&':
		buf.WriteString("&amp;")
	case '<':
		buf.WriteString("&lt;")
	case '>':
		buf.WriteString("&gt;")
	case '"':
		buf.WriteString("&quot;")
	default:
		buf.WriteRune(r)
	}
}
