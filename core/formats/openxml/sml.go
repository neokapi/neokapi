package openxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
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
	if strings.Contains(partPath, "table") {
		return p.parseTable(data, partPath, emitBlock)
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
		if errors.Is(err, io.EOF) {
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
	var hasFormula bool // tracks whether the current cell contains a <f> element

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
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
					hasFormula = false
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

			case "f":
				if inCell {
					// Capture the formula element and write it to skeleton verbatim.
					hasFormula = true
					raw, err := captureRawElement(d, t)
					if err != nil {
						return err
					}
					p.skelWriteString(raw)
					continue
				}
				p.skelWriteStartElement(t)

			case "sheetData", "worksheet":
				p.skelWriteStartElement(t)

			default:
				if !inCell {
					p.skelWriteStartElement(t)
				} else {
					// Unknown child of <c>: capture and write to skeleton unchanged.
					raw, err := captureRawElement(d, t)
					if err != nil {
						return err
					}
					p.skelWriteString(raw)
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
						// Formula string results — not translatable (recalculated).
						translatable = false
					case "inlineStr":
						translatable = text != "" && !hasFormula
					case "":
						if text != "" && !hasFormula {
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
							Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
							Targets:      make(map[model.VariantKey]*model.Target),
							Properties:   map[string]string{"partPath": partPath, "cell": cellRef},
						}
						// Intrinsic cell-grid geometry (WS2): a literal/inline-string
						// cell lives at a single (col,row), so its position is the
						// cell address itself. Shared-string cells are deduplicated in
						// sharedStrings.xml — one block backs many cells — so they have
						// no single position and get no geometry (handled there). The
						// BBox is in cell units (W=H=1 = one cell), flagged by the
						// "cell-grid" origin; X/Y are the zero-based column/row.
						if col, row, ok := parseCellRefA1(cellRef); ok {
							if sheet := sheetNumFromPath(partPath); sheet > 0 {
								block.SetGeometry(&model.GeometryAnnotation{
									Page:   sheet,
									BBox:   model.Rect{X: float64(col), Y: float64(row), W: 1, H: 1},
									Origin: "cell-grid",
								})
							}
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
					hasFormula = false
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

// parseCellRefA1 parses an A1-style cell reference ("A1", "AB12") into a
// zero-based (col, row). It mirrors parseCellRef in the editor's renderDoc.ts
// so the Go-derived geometry and the JS layout view agree on the grid origin.
// ok is false for any malformed or empty ref.
func parseCellRefA1(ref string) (col, row int, ok bool) {
	ref = strings.TrimSpace(ref)
	i := 0
	for i < len(ref) {
		c := ref[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c < 'A' || c > 'Z' {
			break
		}
		col = col*26 + int(c-'A'+1) // 'A' → 1
		i++
	}
	if i == 0 || i == len(ref) {
		return 0, 0, false // no letters, or no digits
	}
	n, err := strconv.Atoi(ref[i:])
	if err != nil || col < 1 || n < 1 {
		return 0, 0, false
	}
	return col - 1, n - 1, true
}

// sheetNumFromPath returns the 1-based worksheet number for an
// `xl/worksheets/sheetN.xml` part, or 0 for any other part. The match is exact
// (the prefix is the full worksheets dir), so xl/tables/* and the workbook part
// get no page.
func sheetNumFromPath(partPath string) int {
	const prefix = "xl/worksheets/sheet"
	if !strings.HasPrefix(partPath, prefix) || !strings.HasSuffix(partPath, ".xml") {
		return 0
	}
	n, err := strconv.Atoi(partPath[len(prefix) : len(partPath)-len(".xml")])
	if err != nil || n <= 0 {
		return 0
	}
	return n
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
	b := &runBuilder{}
	spanCounter := 0
	var activeProps *runProps

	for _, run := range runs {
		if activeProps == nil || !activeProps.equal(run.props) {
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, &spanCounter)
			}
			if !run.props.isEmpty() {
				run.props.appendOpeningRuns(b, &spanCounter)
			}
			propsCopy := run.props
			activeProps = &propsCopy
		}

		b.AddText(run.text)
	}

	if activeProps != nil && !activeProps.isEmpty() {
		activeProps.appendClosingRuns(b, &spanCounter)
	}

	return &model.Block{
		ID:           id,
		Type:         "shared-string",
		Translatable: true,
		Source:       b.Runs(),
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties: map[string]string{
			"partPath": partPath,
			"siIndex":  strconv.Itoa(siIndex),
		},
	}
}

// parseTable parses an Excel table definition (xl/tables/tableN.xml) and emits
// blocks for translatable tableColumn name attributes. Excel requires these
// names to match the header row cell values; without updating them after
// translating shared strings, the file is reported as corrupted.
func (p *smlParser) parseTable(data []byte, partPath string, emitBlock func(*model.Block)) error {
	d := xml.NewDecoder(bytes.NewReader(data))

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("sml: parsing %s: %w", partPath, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "tableColumn" {
				p.skelWriteTableColumn(t, partPath, emitBlock)
				continue
			}
			p.skelWriteStartElement(t)

		case xml.EndElement:
			p.skelWriteEndElement(t)

		case xml.CharData:
			p.skelText(xmlEscape(string(t)))

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")
		}
	}
	return nil
}

// skelWriteTableColumn writes a <tableColumn> element to the skeleton,
// extracting the "name" attribute as a translatable block.
func (p *smlParser) skelWriteTableColumn(t xml.StartElement, partPath string, emitBlock func(*model.Block)) {
	registerNamespaces(t.Attr)

	var nameVal string
	nameIdx := -1
	for i, a := range t.Attr {
		if a.Name.Local == "name" && (a.Name.Space == "" || a.Name.Space == t.Name.Space) {
			nameVal = a.Value
			nameIdx = i
			break
		}
	}

	if nameIdx < 0 || strings.TrimSpace(nameVal) == "" {
		// No translatable name — write element unchanged
		p.skelWriteStartElement(t)
		return
	}

	*p.blockCounter++
	blockID := fmt.Sprintf("tu%d", *p.blockCounter)

	// Write the element with a skeleton ref in place of the name value
	if p.skeletonStore != nil {
		var buf strings.Builder
		buf.WriteString("<")
		writeElementName(&buf, t.Name)
		for i, a := range t.Attr {
			buf.WriteString(" ")
			writeAttrName(&buf, a.Name)
			buf.WriteString(`="`)
			if i == nameIdx {
				// Flush text before the ref, write ref, then continue
				buf2 := buf.String()
				p.skelBuf.WriteString(buf2)
				p.skelRef(blockID)
				p.skelWriteString(`"`)
				// Write remaining attributes
				for _, a2 := range t.Attr[i+1:] {
					p.skelWriteString(" ")
					var ab strings.Builder
					writeAttrName(&ab, a2.Name)
					p.skelWriteString(ab.String())
					p.skelWriteString(`="`)
					p.skelWriteString(xmlEscapeAttr(a2.Value))
					p.skelWriteString(`"`)
				}
				p.skelWriteString(">")

				block := &model.Block{
					ID:           blockID,
					Type:         "table-column",
					Translatable: true,
					Source:       []model.Run{{Text: &model.TextRun{Text: nameVal}}},
					Targets:      make(map[model.VariantKey]*model.Target),
					Properties:   map[string]string{"partPath": partPath},
				}
				emitBlock(block)
				return
			}
			buf.WriteString(xmlEscapeAttr(a.Value))
			buf.WriteString(`"`)
		}
	}

	// Fallback when no skeleton store: just write the element normally
	p.skelWriteStartElement(t)

	block := &model.Block{
		ID:           blockID,
		Type:         "table-column",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: nameVal}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   map[string]string{"partPath": partPath},
	}
	emitBlock(block)
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
