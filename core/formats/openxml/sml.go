package openxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// smlParser parses SpreadsheetML XML parts (XLSX worksheets, shared strings).
type smlParser struct {
	cfg           *Config
	blockCounter  *int
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	sharedStrings []string // pre-parsed shared string table
}

// parsePart routes to the appropriate sub-parser based on the part path.
func (p *smlParser) parsePart(data []byte, partPath string, emitBlock func(*model.Block)) error {
	if strings.Contains(partPath, "sharedStrings") {
		return p.parseSharedStringsPart(data, partPath, emitBlock)
	}
	if strings.Contains(partPath, "worksheet") || strings.Contains(partPath, "sheet") {
		return p.parseWorksheet(data, partPath, emitBlock)
	}
	return nil
}

// parseSharedStringsPart parses xl/sharedStrings.xml and emits blocks for each string.
func (p *smlParser) parseSharedStringsPart(data []byte, partPath string, emitBlock func(*model.Block)) error {
	d := xml.NewDecoder(bytes.NewReader(data))

	var inSI bool
	var currentRuns []textRun
	var currentProps runProps
	siIndex := 0

	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("sml: parsing %s: %w", partPath, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSI = true
				currentRuns = nil
				p.skelWriteStartElement(t)

			case "r":
				if inSI {
					currentProps = runProps{}
				}

			case "rPr":
				if inSI {
					currentProps = p.parseSMLRunProps(d)
					continue
				}
				p.skelWriteStartElement(t)

			case "t":
				if inSI {
					text, err := readCharData(d)
					if err != nil {
						return err
					}
					currentRuns = append(currentRuns, textRun{text: text, props: currentProps})
					continue
				}
				p.skelWriteStartElement(t)

			default:
				if !inSI {
					p.skelWriteStartElement(t)
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "si":
				merged := mergeRuns(currentRuns)
				if !isEmptyRuns(merged) {
					*p.blockCounter++
					blockID := fmt.Sprintf("tu%d", *p.blockCounter)
					p.skelRef(blockID)
					block := p.buildBlock(blockID, merged, partPath, siIndex)
					emitBlock(block)
				} else {
					p.skelWriteString(p.renderSI(currentRuns))
				}
				p.skelWriteEndElement(t)
				inSI = false
				siIndex++

			case "r":
				continue

			default:
				if !inSI {
					p.skelWriteEndElement(t)
				}
			}

		case xml.CharData:
			if !inSI {
				p.skelText(xmlEscape(string(t)))
			}

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")
		}
	}
	return nil
}

// parseSMLRunProps parses run properties inside shared string rich text <rPr>.
func (p *smlParser) parseSMLRunProps(d *xml.Decoder) runProps {
	var props runProps
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return props
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "b":
				props.bold = true
			case "i":
				props.italic = true
			case "u":
				props.underline = "single"
			case "strike":
				props.strike = true
			case "vertAlign":
				props.vertAlign = attrVal(t, "val")
			}
		case xml.EndElement:
			depth--
		}
	}
	return props
}

// parseWorksheet parses a worksheet XML file and emits blocks for string cells.
func (p *smlParser) parseWorksheet(data []byte, partPath string, emitBlock func(*model.Block)) error {
	d := xml.NewDecoder(bytes.NewReader(data))

	var inRow, inCell, inValue bool
	var cellType, cellRef string
	var cellText strings.Builder

	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("sml: parsing %s: %w", partPath, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				inRow = true
				p.skelWriteStartElement(t)

			case "c":
				if inRow {
					inCell = true
					cellType = attrVal(t, "t")
					cellRef = attrVal(t, "r")
					cellText.Reset()
					p.skelWriteStartElement(t)
				}

			case "v":
				if inCell {
					inValue = true
				} else {
					p.skelWriteStartElement(t)
				}

			case "is":
				if inCell {
					text := p.parseInlineString(d)
					cellText.WriteString(text)
					continue
				}
				p.skelWriteStartElement(t)

			case "sheetData", "worksheet":
				p.skelWriteStartElement(t)

			default:
				if !inCell {
					p.skelWriteStartElement(t)
				} else {
					if err := skipElement(d); err != nil {
						return err
					}
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				if inValue {
					inValue = false
					continue
				}
				p.skelWriteEndElement(t)

			case "c":
				if inCell {
					text := cellText.String()
					translatable := false

					switch cellType {
					case "s":
						// Shared string references are handled in sharedStrings.xml.
						// Pass the <v>index</v> through to skeleton unchanged.
						translatable = false
					case "str":
						translatable = text != ""
					case "inlineStr":
						translatable = text != ""
					case "":
						if text != "" {
							_, err := strconv.ParseFloat(text, 64)
							translatable = err != nil
						}
					}

					if translatable && strings.TrimSpace(text) != "" {
						*p.blockCounter++
						blockID := fmt.Sprintf("tu%d", *p.blockCounter)
						p.skelRef(blockID)

						block := &model.Block{
							ID:           blockID,
							Type:         "cell",
							Translatable: true,
							Source: []*model.Segment{{
								ID:      "s1",
								Content: model.NewFragment(text),
							}},
							Targets:     make(map[model.LocaleID][]*model.Segment),
							Properties:  map[string]string{"partPath": partPath, "cell": cellRef},
							Annotations: make(map[string]model.Annotation),
						}
						emitBlock(block)
					} else {
						p.skelWriteString("<v>")
						p.skelText(xmlEscape(cellText.String()))
						p.skelWriteString("</v>")
					}

					p.skelWriteEndElement(t)
					inCell = false
					cellType = ""
					cellRef = ""
				} else {
					p.skelWriteEndElement(t)
				}

			case "row":
				inRow = false
				p.skelWriteEndElement(t)

			default:
				if !inCell {
					p.skelWriteEndElement(t)
				}
			}

		case xml.CharData:
			if inValue && inCell {
				cellText.Write(t)
			} else if !inCell {
				p.skelText(xmlEscape(string(t)))
			}

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")
		}
	}
	return nil
}

// parseInlineString reads an inline string element <is> and returns its text.
func (p *smlParser) parseInlineString(d *xml.Decoder) string {
	var text strings.Builder
	depth := 1
	var inT bool

	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return text.String()
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "t" {
				inT = true
			}
		case xml.EndElement:
			depth--
			if t.Name.Local == "t" {
				inT = false
			}
		case xml.CharData:
			if inT {
				text.Write(t)
			}
		}
	}
	return text.String()
}

// renderSI renders an empty shared string item for skeleton output.
func (p *smlParser) renderSI(runs []textRun) string {
	var buf strings.Builder
	for _, r := range runs {
		buf.WriteString("<t>")
		buf.WriteString(xmlEscape(r.text))
		buf.WriteString("</t>")
	}
	return buf.String()
}

// buildBlock creates a model.Block from shared string text runs.
func (p *smlParser) buildBlock(id string, runs []textRun, partPath string, siIndex int) *model.Block {
	frag := &model.Fragment{}
	spanCounter := 0
	var activeProps *runProps

	for _, run := range runs {
		if activeProps == nil || !activeProps.equal(run.props) {
			if activeProps != nil && !activeProps.isEmpty() {
				for _, s := range activeProps.closingSpans(&spanCounter) {
					frag.AppendSpan(s)
				}
			}
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

	if activeProps != nil && !activeProps.isEmpty() {
		for _, s := range activeProps.closingSpans(&spanCounter) {
			frag.AppendSpan(s)
		}
	}

	return &model.Block{
		ID:           id,
		Type:         "shared-string",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: frag}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties: map[string]string{
			"partPath": partPath,
			"siIndex":  fmt.Sprintf("%d", siIndex),
		},
		Annotations: make(map[string]model.Annotation),
	}
}

// Skeleton helpers

func (p *smlParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *smlParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		_ = p.skeletonStore.WriteRef(id)
	}
}

func (p *smlParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

func (p *smlParser) skelWriteStartElement(t xml.StartElement) {
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

func (p *smlParser) skelWriteEndElement(t xml.EndElement) {
	if p.skeletonStore == nil {
		return
	}
	var buf strings.Builder
	buf.WriteString("</")
	writeElementName(&buf, t.Name)
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *smlParser) skelWriteString(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}
