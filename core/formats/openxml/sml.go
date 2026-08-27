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
	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
)

// smlParser parses SpreadsheetML XML parts (XLSX worksheets, shared strings).
type smlParser struct {
	cfg          *Config
	blockCounter *int
	// ids separates the ids this document repeats. A cell anchor is identified
	// by its `r` reference, which the worksheet supplies: OOXML says a reference
	// is unique within a sheet, and a repaired or machine-generated workbook
	// repeats one anyway. See core/model/blockid.go.
	ids           *model.IDBuilder
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	sharedStrings []string // pre-parsed shared string table
	// sheetNames maps a worksheet part path to its display name, so a grid can
	// be labelled with the tab a reader would see.
	sheetNames map[string]string
	// tableColumnSeq positions a <tableColumn> that carries no `id` attribute,
	// counted within the table part it belongs to.
	tableColumnSeq int
	// emitPart, when set, emits a Part directly to the reader's output channel,
	// which is how a worksheet's table/table-row groups reach it — they are
	// structure, not blocks. Additive and skeleton-free, so the byte-exact
	// round-trip is unaffected. Groups are skipped entirely when unset.
	emitPart func(*model.Part)
	// groupCounter names the emitted groups.
	groupCounter int
}

func (p *smlParser) nextGroupID() string {
	p.groupCounter++
	return fmt.Sprintf("sg%d", p.groupCounter)
}

// openGroup emits a GroupStart and returns its ID; closeGroup ends it. Both are
// no-ops when the reader did not wire emitPart.
func (p *smlParser) openGroup(kind string, props map[string]string) string {
	if p.emitPart == nil {
		return ""
	}
	id := p.nextGroupID()
	p.emitPart(&model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{
		ID: id, Name: kind, Type: kind, Properties: props,
	}})
	return id
}

func (p *smlParser) closeGroup(id string) {
	if p.emitPart == nil || id == "" {
		return
	}
	p.emitPart(&model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: id}})
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
				p.skelText(xmlesc.Text(string(t)))
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

// parseWorksheet parses a buffered worksheet XML part.
func (p *smlParser) parseWorksheet(data []byte, partPath string, emitBlock func(*model.Block)) error {
	merges, width := scanWorksheet(bytes.NewReader(data))
	return p.parseWorksheetFrom(xml.NewDecoder(bytes.NewReader(data)), merges, width, partPath, emitBlock)
}

// parseWorksheetStream parses a worksheet straight from its zip entry, which
// open re-opens for each of the two passes the part needs.
//
// A worksheet is the largest part in a spreadsheet — tens of megabytes
// uncompressed is ordinary — and buffering it was the last thing in the reader
// whose cost scaled with the document. Two passes cost one extra decompression;
// holding it cost the whole part for the length of the parse.
func (p *smlParser) parseWorksheetStream(open func() (io.ReadCloser, error), partPath string, emitBlock func(*model.Block)) error {
	scan, err := open()
	if err != nil {
		return err
	}
	merges, width := scanWorksheet(scan)
	if cerr := scan.Close(); cerr != nil {
		return cerr
	}

	parse, err := open()
	if err != nil {
		return err
	}
	defer parse.Close()
	return p.parseWorksheetFrom(xml.NewDecoder(parse), merges, width, partPath, emitBlock)
}

// scanWorksheet reads a worksheet once for the two facts the parse pass needs
// up front but cannot learn in document order: each merged range's extent
// (<mergeCells> follows <sheetData>) and the grid's width (the last cell settles
// it). Nothing per cell is retained.
func scanWorksheet(r io.Reader) (map[string]mergeSpan, int) {
	merges := map[string]mergeSpan{}
	width := 0
	d := xml.NewDecoder(r)
	for {
		tok, err := d.Token()
		if err != nil {
			return merges, width
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "c":
			if col, _, ok := parseCellRefA1(attrVal(se, "r")); ok && col >= width {
				width = col + 1
			}
		case "mergeCell":
			addMergeSpan(merges, attrVal(se, "ref"))
		}
	}
}

// parseWorksheetFrom emits blocks for a worksheet's cells, given the merged
// spans and grid width its scan pass established.
func (p *smlParser) parseWorksheetFrom(d *xml.Decoder, merges map[string]mergeSpan, width int,
	partPath string, emitBlock func(*model.Block)) error {

	p.emitSheetHeading(partPath, emitBlock)

	// A worksheet is a table, and its rows are its rows. Bracketing them is what
	// lets a consumer render the grid one row at a time; without the brackets
	// the topology is only recoverable by buffering every cell and rebuilding it
	// (core/projection.flushFlatCells says as much). The width travels on the
	// group so a writer that must commit to a column count before its first row
	// — GFM puts the delimiter row second — does not have to see the last cell
	// first.
	tableID := ""
	if p.emitPart != nil {
		tableID = p.openGroup("table", map[string]string{
			"columns": strconv.Itoa(width),
		})
		defer func() { p.closeGroup(tableID) }()
	}
	rowID := ""

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

					// The row bracket opens on the row's first surfaced cell,
					// not on <row>: Excel writes rows carrying only styling, and
					// an empty bracket would put an empty row in every
					// consumer's grid. Both kinds of cell open it — an
					// inline-string cell is translatable and emitted directly,
					// a shared-string or literal one is surfaced as an anchor,
					// and a sheet made of either is still a grid.
					openRow := func() {
						if rowID == "" {
							rowID = p.openGroup("table-row", nil)
						}
					}

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
						openRow()
						*p.blockCounter++
						blockID := fmt.Sprintf("tu%d", *p.blockCounter)
						p.skelRef(blockID)

						block := &model.Block{
							ID: blockID,
							// A cell's reference (`B7`) is its address in the
							// sheet — nothing the reader could compute beats it.
							Name:         model.StructuralPath(append(strings.Split(partPath, "/"), cellRef)...),
							Type:         "cell",
							Translatable: true,
							Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
							Properties:   map[string]string{"partPath": partPath, "cell": cellRef},
						}
						// Intrinsic cell-grid geometry (WS2): a literal/inline-string
						// cell lives at a single (col,row), so its position is the
						// cell address itself; a merged cell additionally spans
						// W columns × H rows. Shared-string cells are deduplicated in
						// sharedStrings.xml — one block backs many cells — so the
						// translatable text has no single position (handled there); we
						// surface their position separately as a grid anchor below.
						if g := cellGeometry(cellRef, partPath, merges); g != nil {
							block.SetGeometry(g)
						}
						markGridCell(block, cellRef, p.emitPart != nil)
						emitBlock(block)
					} else {
						p.skelWriteString("<v>")
						p.skelText(xmlesc.Text(cellText.String()))
						p.skelWriteString("</v>")
						// A shared-string or value cell carries no translatable text of
						// its own (a shared string is deduplicated in sharedStrings.xml;
						// a number/formula result is not translated), but it does occupy
						// a position in the grid. Surface that position as a
						// non-translatable grid anchor so previews and structural
						// exports can reconstruct the worksheet — otherwise invisible as
						// a grid. Additive and skeleton-free, so it never affects
						// extraction, word count, or round-trip.
						if cellRef != "" && p.cfg != nil && p.cfg.ExtractNonTranslatableContent() {
							switch {
							case cellType == "s":
								openRow()
								p.emitSharedCellAnchor(text, cellRef, partPath, merges, emitBlock)
							case strings.TrimSpace(text) != "":
								openRow()
								p.emitLiteralCellAnchor(text, cellRef, partPath, merges, emitBlock)
							}
						}
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
				p.closeGroup(rowID)
				rowID = ""

			default:
				if !inCell {
					p.skelWriteEndElement(t)
				}
			}

		case xml.CharData:
			if inValue && inCell {
				cellText.Write(t)
			} else if !inCell {
				p.skelText(xmlesc.Text(string(t)))
			}

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")
		}
	}
	return nil
}

// emitSheetHeading surfaces a worksheet's display name as a heading ahead of
// its cells.
//
// A workbook is a sequence of sheets, and that boundary is real: without it
// every sheet's cells arrive as one undifferentiated run and a multi-sheet
// workbook exports as a single merged table whose rows come from different
// grids. The heading both names the sheet and, being a non-cell block, ends the
// preceding sheet's run of cells.
//
// The name lives in xl/workbook.xml behind a relationship id, so it is resolved
// once at open time (parseSheetNames). Emitted as a Translatable:false block
// gated behind ExtractNonTranslatableContent and carrying no skeleton ref, like
// the grid anchors it precedes: additive, so extraction, word count and the
// round-trip are untouched, and the parity runner (which forces the flag off)
// sees an unchanged part stream. Sheet names have their own translatability
// switch — TranslateSheetNames, default false, mirroring upstream Okapi's
// translateExcelSheetNames — and this is deliberately not that.
func (p *smlParser) emitSheetHeading(partPath string, emitBlock func(*model.Block)) {
	if p.cfg == nil || !p.cfg.ExtractNonTranslatableContent() {
		return
	}
	name := p.sheetNames[partPath]
	if name == "" {
		return
	}
	*p.blockCounter++
	block := &model.Block{
		ID:           fmt.Sprintf("tu%d", *p.blockCounter),
		Name:         model.StructuralPath(append(strings.Split(partPath, "/"), "@name")...),
		Type:         "sheet-name",
		Translatable: false,
		Source:       []model.Run{{Text: &model.TextRun{Text: name}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   map[string]string{"partPath": partPath},
	}
	block.SetSemanticRole(model.RoleHeading, 2)
	emitBlock(block)
}

// emitSharedCellAnchor surfaces a shared-string worksheet cell as a
// non-translatable grid anchor. idxText is the cell's <v> body (the index into
// the shared string table); it is resolved to the actual string so a preview
// can place the text at its (col,row). The block carries no skeleton ref, so
// the writer ignores it and the round-trip is unaffected; its siIndex property
// links back to the translatable shared-string block (under sharedStrings.xml)
// so a consumer can join to that block's targets/overlays. Gated by the caller
// behind ExtractNonTranslatableContent, matching the comment-surfacing path.
func (p *smlParser) emitSharedCellAnchor(idxText, cellRef, partPath string, merges map[string]mergeSpan, emitBlock func(*model.Block)) {
	idx, err := strconv.Atoi(strings.TrimSpace(idxText))
	if err != nil || idx < 0 || idx >= len(p.sharedStrings) {
		return
	}
	text := p.sharedStrings[idx]
	if strings.TrimSpace(text) == "" {
		return
	}

	sheetTag := strings.TrimSuffix(strings.TrimPrefix(partPath, "xl/worksheets/"), ".xml")
	block := &model.Block{
		ID: fmt.Sprintf("cell-%s-%s", sheetTag, cellRef),
		// The cell reference is the address; nothing computed beats it.
		Name:         model.StructuralPath(append(strings.Split(partPath, "/"), cellRef)...),
		Type:         "cell",
		Translatable: false,
		Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
		Properties: map[string]string{
			"partPath": partPath,
			"cell":     cellRef,
			"siIndex":  strconv.Itoa(idx),
		},
	}
	p.ids.Assign(block)
	if g := cellGeometry(cellRef, partPath, merges); g != nil {
		block.SetGeometry(g)
	}
	markGridCell(block, cellRef, p.emitPart != nil)
	emitBlock(block)
}

// emitLiteralCellAnchor surfaces a non-string worksheet cell (a number, boolean,
// or cached formula result) as a non-translatable grid anchor carrying its
// displayed value, so a structural export or preview can place it in the grid.
// Like the shared-string anchor it is additive and skeleton-free.
func (p *smlParser) emitLiteralCellAnchor(text, cellRef, partPath string, merges map[string]mergeSpan, emitBlock func(*model.Block)) {
	sheetTag := strings.TrimSuffix(strings.TrimPrefix(partPath, "xl/worksheets/"), ".xml")
	block := &model.Block{
		ID: fmt.Sprintf("cell-%s-%s", sheetTag, cellRef),
		// The cell reference is the address; nothing computed beats it.
		Name:         model.StructuralPath(append(strings.Split(partPath, "/"), cellRef)...),
		Type:         "cell",
		Translatable: false,
		Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
		Properties:   map[string]string{"partPath": partPath, "cell": cellRef},
	}
	p.ids.Assign(block)
	if g := cellGeometry(cellRef, partPath, merges); g != nil {
		block.SetGeometry(g)
	}
	markGridCell(block, cellRef, p.emitPart != nil)
	emitBlock(block)
}

// markGridCell gives a worksheet cell anchor the canonical table-cell role and
// the row hint the flat-cell projection path groups on.
//
// A worksheet has no row *container* to bracket — a row is an addressing fact,
// not a markup element, and the cells arrive as a flat stream. The geometry was
// always recorded (address plus merge span), but without a role nothing
// downstream could see a grid: core/projection assembles tables from cell roles,
// so every cell fell through as a standalone block and a spreadsheet exported as
// a run of loose paragraphs. The role plus projection.PropFlatRow is exactly
// what the flat-cell fallback was built to consume.
func markGridCell(block *model.Block, cellRef string, bracketed bool) {
	col, row, ok := parseCellRefA1(cellRef)
	if !ok {
		return
	}
	block.SetSemanticRole(model.RoleTableCell, 0)
	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}
	// A worksheet omits empty cells rather than writing blanks, so a row that
	// skips columns arrives as fewer cells than the grid is wide. Without the
	// address, consumers place those cells in sequence and the values land
	// under the wrong headings — the column is what keeps a sparse row aligned.
	block.Properties["column"] = strconv.Itoa(col)
	if bracketed {
		// The row bracket carries the topology. A per-cell row hint would be
		// the same fact stored a second time, on every cell.
		return
	}
	block.Properties[projection.PropFlatRow] = strconv.Itoa(row)
}

// mergeSpan is a merged-cell range's extent in cells (cols × rows), ≥1 each.
type mergeSpan struct{ cols, rows int }

// cellGeometry builds the cell-grid geometry for a worksheet cell: BBox X/Y are
// the zero-based column/row, W/H the merged-cell span (1×1 when unmerged). Nil
// for a malformed reference or a non-worksheet part.
func cellGeometry(cellRef, partPath string, merges map[string]mergeSpan) *model.GeometryAnnotation {
	col, row, ok := parseCellRefA1(cellRef)
	if !ok {
		return nil
	}
	sheet := sheetNumFromPath(partPath)
	if sheet <= 0 {
		return nil
	}
	w, h := 1, 1
	if m, ok := merges[cellRef]; ok {
		w, h = m.cols, m.rows
	}
	return &model.GeometryAnnotation{
		Page:   sheet,
		BBox:   model.Rect{X: float64(col), Y: float64(row), W: float64(w), H: float64(h)},
		Origin: "cell-grid",
	}
}

// parseMergeCells scans a worksheet part for <mergeCell ref="A1:B2"/> ranges and
// returns each range's extent keyed by its top-left cell reference. Malformed
// refs are skipped.
func addMergeSpan(out map[string]mergeSpan, ref string) {
	lo, hi, ok := strings.Cut(ref, ":")
	if !ok {
		return
	}
	c0, r0, ok0 := parseCellRefA1(lo)
	c1, r1, ok1 := parseCellRefA1(hi)
	if !ok0 || !ok1 {
		return
	}
	cols, rows := c1-c0+1, r1-r0+1
	if cols < 1 || rows < 1 {
		return
	}
	out[lo] = mergeSpan{cols: cols, rows: rows}
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
	// The remainder must be bare digits (no sign): the TS parseCellRef regex
	// is /^([A-Za-z]+)(\d+)$/, but strconv.Atoi would accept a leading +/-.
	for _, c := range ref[i:] {
		if c < '0' || c > '9' {
			return 0, 0, false
		}
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
		buf.WriteString(xmlesc.Text(r.text))
		buf.WriteString("</t>")
	}
	return buf.String()
}

// buildBlock creates a model.Block from shared string text runs.
func (p *smlParser) buildBlock(id string, runs []textRun, partPath string, siIndex int) *model.Block {
	b := &runBuilder{}
	ids := &spanIDs{}
	var activeProps *runProps

	for _, run := range runs {
		if activeProps == nil || !activeProps.equal(run.props) {
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
			}
			if !run.props.isEmpty() {
				run.props.appendOpeningRuns(b, ids)
			}
			propsCopy := run.props
			activeProps = &propsCopy
		}

		b.AddText(run.text)
	}

	if activeProps != nil && !activeProps.isEmpty() {
		activeProps.appendClosingRuns(b, ids)
	}

	return &model.Block{
		ID: id,
		// A shared string is addressed by the index every cell references it
		// by — the format's own key, and a better name than any count.
		Name:         model.StructuralPath(append(strings.Split(partPath, "/"), "si["+strconv.Itoa(siIndex)+"]")...),
		Type:         "shared-string",
		Translatable: true,
		Source:       b.Runs(),
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
			p.skelText(xmlesc.Text(string(t)))

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
	var colID string
	nameIdx := -1
	for i, a := range t.Attr {
		if a.Name.Local == "id" && a.Name.Space == "" {
			colID = a.Value
		}
		if a.Name.Local == "name" && (a.Name.Space == "" || a.Name.Space == t.Name.Space) {
			nameVal = a.Value
			nameIdx = i
		}
	}

	if nameIdx < 0 || strings.TrimSpace(nameVal) == "" {
		// No translatable name — write element unchanged
		p.skelWriteStartElement(t)
		return
	}

	*p.blockCounter++
	blockID := fmt.Sprintf("tu%d", *p.blockCounter)
	// A table column carries its own `id` — the key the sheet references it by.
	// Without one, fall back to its position among the columns of this part.
	step := "tableColumn"
	if colID != "" {
		step += "[" + colID + "]"
	} else {
		p.tableColumnSeq++
		step = ordinalStep(step, p.tableColumnSeq)
	}
	blockName := model.StructuralPath(append(strings.Split(partPath, "/"), step, "@name")...)

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
					p.skelWriteString(xmlesc.Attr(a2.Value))
					p.skelWriteString(`"`)
				}
				p.skelWriteString(">")

				block := &model.Block{
					ID:           blockID,
					Name:         blockName,
					Type:         "table-column",
					Translatable: true,
					Source:       []model.Run{{Text: &model.TextRun{Text: nameVal}}},
					Targets:      make(map[model.VariantKey]*model.Target),
					Properties:   map[string]string{"partPath": partPath},
				}
				emitBlock(block)
				return
			}
			buf.WriteString(xmlesc.Attr(a.Value))
			buf.WriteString(`"`)
		}
	}

	// Fallback when no skeleton store: just write the element normally
	p.skelWriteStartElement(t)

	block := &model.Block{
		ID:           blockID,
		Name:         blockName,
		Type:         "table-column",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: nameVal}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   map[string]string{"partPath": partPath},
	}
	emitBlock(block)
}

// emitXLSXCommentData scans an Excel comment part (xl/comments*.xml) for
// <commentList><comment ref="A1"><text>…</text> bodies and surfaces each as an
// informational Data part carrying the concatenated comment text and the cell
// reference (#928). The comment part is not parsed for skeleton (the writer
// copies it verbatim), so this is purely additive and never affects the
// round-trip. Best-effort: a malformed part yields no Data rather than failing
// the read.
func emitXLSXCommentData(data []byte, emitData func(name, text, ref string)) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var inComment bool
	var ref string
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "comment":
				inComment = true
				ref = attrVal(t, "ref")
			case "text":
				if inComment {
					text := collectXLSXTextBody(d)
					if strings.TrimSpace(text) != "" {
						emitData("comment", text, ref)
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "comment" {
				inComment = false
				ref = ""
			}
		}
	}
}

// collectXLSXTextBody reads from just after a <text> start element to its
// matching close, concatenating the character data of every nested <t> run.
// Mirrors parseInlineString's <t>-gathering for rich-text cell strings.
func collectXLSXTextBody(d *xml.Decoder) string {
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

// Skeleton helpers

func (p *smlParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *smlParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		p.skeletonStore.WriteRef(id)
	}
}

func (p *smlParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		p.skeletonStore.WriteText(p.skelBuf.Bytes())
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
		buf.WriteString(xmlesc.Attr(a.Value))
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
