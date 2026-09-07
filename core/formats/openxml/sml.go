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
	// styles resolves a cell's style index to the number format its value
	// displays through. Nil resolves every cell to General.
	styles *cellStyles
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
	d := newRawDecoder(data)

	var inSI bool
	var currentRuns []textRun
	var currentProps runProps
	siIndex := 0
	// Where the current <si> element's content starts, so an item that holds
	// no translatable text goes back as the source wrote it.
	var siInnerOff int64
	// The current <si> element's phonetic markup, as the source wrote it.
	var siPhonetic string

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
				siPhonetic = ""
				// A shared string that holds a bare <t> opens no <r>, so
				// without this reset it inherits the run properties of the
				// previous item's last run and comes back wearing them.
				currentProps = runProps{}
				p.skelWriteStartElement(d, t)
				siInnerOff = d.EndOffset()

			case "r":
				if inSI {
					currentProps = runProps{}
				}

			case "rPr":
				if inSI {
					currentProps = p.parseSMLRunProps(d)
					continue
				}
				p.skelWriteStartElement(d, t)

			case "rPh", "phoneticPr":
				if inSI {
					raw, err := capturePhoneticElement(d)
					if err != nil {
						return fmt.Errorf("sml: parsing %s: %w", partPath, err)
					}
					siPhonetic += raw
					continue
				}
				p.skelWriteStartElement(d, t)

			case "t":
				if inSI {
					text, err := readCharData(d)
					if err != nil {
						return err
					}
					currentRuns = append(currentRuns, textRun{text: text, props: currentProps})
					continue
				}
				p.skelWriteStartElement(d, t)

			default:
				if !inSI {
					p.skelWriteStartElement(d, t)
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
					block := p.buildBlock(blockID, merged, partPath, siIndex, siPhonetic, d.UptoString(siInnerOff))
					emitBlock(block)
				} else {
					// Nothing translatable in this item: replay its content.
					p.skelWriteString(d.UptoString(siInnerOff))
				}
				p.skelWriteEndElement(d)
				inSI = false
				siIndex++

			case "r":
				continue

			default:
				if !inSI {
					p.skelWriteEndElement(d)
				}
			}

		case xml.CharData:
			if !inSI {
				p.skelRaw(d)
			}

		case xml.ProcInst:
			p.skelRaw(d)
		}
	}
	return nil
}

// parseSMLRunProps parses a CT_Rst run's <rPr> (ECMA-376 Part 1 §18.4.7).
//
// Five of its children carry formatting the model names, and they become the
// declared inline codes a translator sees. The rest — <color>, <sz>, <rFont>,
// <family>, <charset>, <scheme> and anything else a producer writes — carry
// formatting the model does not name, and they are kept as the source wrote
// them so the writer can put them back.
//
// Every child is recorded in source order, whichever group it falls in, so the
// writer rebuilds the element in the order it was read.
func (p *smlParser) parseSMLRunProps(d *rawDecoder) runProps {
	var props runProps
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return props
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth > 1 {
				// Not a direct child: ECMA-376 defines every <rPr> child as
				// empty, so this is a producer's own nesting and it travels
				// inside the child captured below.
				depth++
				continue
			}
			raw, err := captureRawElement(d, t)
			if err != nil {
				return props
			}
			props.smlRPr = append(props.smlRPr, rPrChild{name: t.Name.Local, xml: raw})
			switch t.Name.Local {
			case "b":
				props.bold = smlToggleOn(t)
			case "i":
				props.italic = smlToggleOn(t)
			case "u":
				// ST_UnderlineValues (ECMA-376 Part 1 §18.18.86) spells the
				// absence of an underline as `none`, and a bare <u/> as
				// single. A run that says `none` is not underlined, so it gets
				// no code and its child travels with the rest of the <rPr>.
				switch v := attrVal(t, "val"); v {
				case "":
					props.underline = "single"
				case "none":
					props.underline = ""
				default:
					props.underline = v
				}
			case "strike":
				props.strike = smlToggleOn(t)
			case "vertAlign":
				props.vertAlign = attrVal(t, "val")
			}
		case xml.EndElement:
			depth--
		}
	}
	return props
}

// smlToggleOn reads a CT_BooleanProperty toggle. ECMA-376 Part 1 §18.2.10 makes
// the val attribute default to true, so a bare <b/> is on and only an explicit
// false turns it off.
func smlToggleOn(t xml.StartElement) bool {
	switch attrVal(t, "val") {
	case "0", "false":
		return false
	}
	return true
}

// smlNamedRPrChildren is the set of <rPr> children the model names as inline
// codes. Everything else travels as one opaque paired code.
var smlNamedRPrChildren = map[string]bool{
	"b": true, "i": true, "u": true, "strike": true, "vertAlign": true,
}

// smlOpaqueRPr concatenates, in source order, the <rPr> children no declared
// code carries. Empty when the run carries none.
//
// Being named is not enough to be carried: a declared code is emitted only for
// a child that turns its formatting on, so `<b val="false"/>`,
// `<i val="false"/>` and `<vertAlign val="baseline"/>` state something the
// model has no code for. They belong here, or an explicit "not bold" comes back
// as a run that inherits its weight (948-3.xlsx).
func (rp runProps) smlOpaqueRPr() string {
	var b strings.Builder
	for _, c := range rp.smlRPr {
		if smlNamedRPrChildren[c.name] && rp.smlChildHasCode(c.name) {
			continue
		}
		b.WriteString(c.xml)
	}
	return b.String()
}

// smlChildHasCode reports whether a named <rPr> child produced a declared code,
// which is what decides whether the writer already replays its bytes.
func (rp runProps) smlChildHasCode(name string) bool {
	switch name {
	case "b":
		return rp.bold
	case "i":
		return rp.italic
	case "u":
		return rp.underline != ""
	case "strike":
		return rp.strike
	case "vertAlign":
		return rp.vertAlign == "superscript" || rp.vertAlign == "subscript"
	}
	return false
}

// smlNamedRPrXML returns the source bytes of the named child that declared a
// formatting type, so the code carries the form the run was read with and
// `<u val="double"/>` does not come back as `<u/>`.
func (rp runProps) smlNamedRPrXML(local string) string {
	for _, c := range rp.smlRPr {
		if c.name == local {
			return c.xml
		}
	}
	return ""
}

// smlRPrEqual reports whether two runs carry the same <rPr>, children the model
// does not name included. Two runs that differ only in colour are different
// runs: merging them is how the colour and the run boundary were lost.
func (rp runProps) smlRPrEqual(other runProps) bool {
	if len(rp.smlRPr) != len(other.smlRPr) {
		return false
	}
	for i, c := range rp.smlRPr {
		if c != other.smlRPr[i] {
			return false
		}
	}
	return true
}

// parseWorksheet parses a buffered worksheet XML part.
func (p *smlParser) parseWorksheet(data []byte, partPath string, emitBlock func(*model.Block)) error {
	merges, width := scanWorksheet(bytes.NewReader(data))
	return p.parseWorksheetFrom(newRawDecoder(data), merges, width, partPath, emitBlock)
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
	return p.parseWorksheetFrom(newRawDecoderStream(parse), merges, width, partPath, emitBlock)
}

// scanWorksheet reads a worksheet once for the two facts the parse pass needs
// up front but cannot learn in document order: each merged range's extent
// (<mergeCells> follows <sheetData>) and the grid's width (the last cell settles
// it). Nothing per cell is retained.
func scanWorksheet(r io.Reader) (map[string]mergeSpan, int) {
	merges := map[string]mergeSpan{}
	width := 0
	d := newRawDecoderStream(r)
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
func (p *smlParser) parseWorksheetFrom(d *rawDecoder, merges map[string]mergeSpan, width int,
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
	var cellType, cellRef, cellStyle string
	var cellText strings.Builder
	var hasFormula bool  // the current cell contains a <f> element
	var inlineRaw string // the current cell's <is> element, verbatim
	var inlineRuns []textRun
	var inlinePhonetic string // the <is> element's phonetic markup, verbatim
	// A cell is held back until its </c>: only then is it known whether its
	// text is translatable. A cell that is not goes back as the source wrote
	// it, from cellOff; one that is keeps its own start tag plus whatever
	// non-value children were captured along the way, with a ref in place of
	// the text.
	var cellOff int64
	var cellStartRaw string
	var cellChildren strings.Builder
	// cellTail holds what the source wrote between the cell's content element
	// and </c>, which is the indentation of a pretty-printed worksheet. It is
	// separate from cellChildren because the content element is replaced by a
	// skeleton ref, and these bytes belong on the far side of it.
	var cellTail strings.Builder
	// cellContentSeen is true once the cell's <is> or <v> has been read, which
	// is what decides which side of the ref a run of whitespace belongs to.
	var cellContentSeen bool

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
				p.skelWriteStartElement(d, t)

			case "c":
				if inRow {
					inCell = true
					cellType = attrVal(t, "t")
					cellRef = attrVal(t, "r")
					cellStyle = attrVal(t, "s")
					cellText.Reset()
					hasFormula = false
					inlineRaw = ""
					inlineRuns = nil
					inlinePhonetic = ""
					cellOff = d.Offset()
					d.Pin(cellOff)
					registerNamespaces(t.Attr)
					cellStartRaw = d.RawString()
					cellChildren.Reset()
					cellTail.Reset()
					cellContentSeen = false
				}

			case "v":
				if inCell {
					inValue = true
				} else {
					p.skelWriteStartElement(d, t)
				}

			case "is":
				if inCell {
					raw, runs, phonetic, err := p.parseInlineString(d, t)
					if err != nil {
						return err
					}
					inlinePhonetic = phonetic
					inlineRaw = raw
					inlineRuns = runs
					cellContentSeen = true
					cellText.WriteString(rstText(runs))
					continue
				}
				p.skelWriteStartElement(d, t)

			case "f":
				if inCell {
					// Capture the formula element and hold it with the cell.
					hasFormula = true
					raw, err := captureRawElement(d, t)
					if err != nil {
						return err
					}
					cellChildren.WriteString(raw)
					continue
				}
				p.skelWriteStartElement(d, t)

			case "sheetData", "worksheet":
				p.skelWriteStartElement(d, t)

			default:
				if !inCell {
					p.skelWriteStartElement(d, t)
				} else {
					// Unknown child of <c>: capture it and hold it with the cell.
					raw, err := captureRawElement(d, t)
					if err != nil {
						return err
					}
					cellChildren.WriteString(raw)
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				if inValue {
					inValue = false
					cellContentSeen = true
					continue
				}
				p.skelWriteEndElement(d)

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
						p.skelWriteString(cellStartRaw)
						p.skelWriteString(cellChildren.String())
						p.skelRef(blockID)
						p.skelWriteString(cellTail.String())

						props := map[string]string{"partPath": partPath, "cell": cellRef}
						source := []model.Run{{Text: &model.TextRun{Text: text}}}
						if inlineRaw != "" {
							// An inline string holds CT_Rst, the content model
							// sharedStrings.xml uses for <si>: the same rich runs,
							// stored in the cell. The property tells the writer to
							// spell the <is> wrapper back around them (ECMA-376
							// Part 1 §18.3.1.4); without it the cell's text would
							// go back as a value element and Excel repairs the file.
							props[cellStorageProp] = cellStorageInline
							if inlinePhonetic != "" {
								props[cellPhoneticProp] = inlinePhonetic
							}
							source = rstModelRuns(inlineRuns)
							if inner := rstInnerContent(inlineRaw, "is"); inner != "" {
								if form := smlSourceForm(inner, source, inlinePhonetic); form != "" {
									props[cellSourceProp] = form
								}
							}
						}
						block := &model.Block{
							ID: blockID,
							// A cell's reference (`B7`) is its address in the
							// sheet — nothing the reader could compute beats it.
							Name:         model.StructuralPath(append(strings.Split(partPath, "/"), cellRef)...),
							Type:         "cell",
							Translatable: true,
							Source:       source,
							Properties:   props,
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
						// Nothing in the cell is translated, so it goes back
						// exactly as it was read: self-closing form, child
						// order, whitespace and escaping included.
						p.skelWriteString(d.FromString(cellOff))
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
								display, code := p.cellDisplay(text, cellType, cellStyle)
								p.emitLiteralCellAnchor(text, display, code, cellRef, partPath, merges, emitBlock)
							}
						}
					}

					if translatable && strings.TrimSpace(text) != "" {
						p.skelWriteEndElement(d)
					}
					d.Unpin()
					inCell = false
					cellType = ""
					cellRef = ""
					cellStyle = ""
					cellStartRaw = ""
					cellChildren.Reset()
					cellTail.Reset()
					cellContentSeen = false
					hasFormula = false
					inlineRaw = ""
					inlineRuns = nil
					inlinePhonetic = ""
				} else {
					p.skelWriteEndElement(d)
				}

			case "row":
				inRow = false
				p.skelWriteEndElement(d)
				p.closeGroup(rowID)
				rowID = ""

			default:
				if !inCell {
					p.skelWriteEndElement(d)
				}
			}

		case xml.CharData:
			switch {
			case inValue && inCell:
				cellText.Write(t)
			case inCell:
				// The indentation of a pretty-printed worksheet, which sits
				// between the cell's children and is replayed around the ref
				// that stands in for the content element.
				if cellContentSeen {
					cellTail.Write(d.Raw())
				} else {
					cellChildren.Write(d.Raw())
				}
			default:
				p.skelRaw(d)
			}

		case xml.ProcInst:
			p.skelRaw(d)
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

// cellDisplay renders a value cell's stored text the way the sheet shows it,
// through the number format its style names, and returns the code it used.
// A boolean shows as TRUE or FALSE; an error value and an ISO date string
// show as stored; a formula's string result shows through the code's text
// section when it has one; a number the renderer cannot format under its
// code shows as stored.
func (p *smlParser) cellDisplay(text, cellType, styleIdx string) (display, code string) {
	code, known := p.styles.formatCode(styleIdx)
	text = strings.TrimSpace(text)
	switch cellType {
	case "b":
		if text == "1" || strings.EqualFold(text, "true") {
			return "TRUE", code
		}
		return "FALSE", code
	case "str":
		if known {
			return formatCellText(text, code), code
		}
		return text, code
	case "e", "d":
		return text, code
	}
	if !known {
		return text, code
	}
	display, _ = formatCellValue(text, code, p.styles.epoch1904())
	return display, code
}

// emitLiteralCellAnchor surfaces a non-string worksheet cell (a number, boolean,
// or cached formula result) as a non-translatable grid anchor, so a structural
// export or preview can place it in the grid. The block's text is the value
// as stored, which is what the round-trip and the block's identity rest on;
// the value as the sheet displays it travels beside it as PropCellDisplay,
// with the number-format code that produced it as PropCellFormat. Like the
// shared-string anchor it is additive and skeleton-free.
func (p *smlParser) emitLiteralCellAnchor(text, display, code, cellRef, partPath string, merges map[string]mergeSpan, emitBlock func(*model.Block)) {
	sheetTag := strings.TrimSuffix(strings.TrimPrefix(partPath, "xl/worksheets/"), ".xml")
	block := &model.Block{
		ID: fmt.Sprintf("cell-%s-%s", sheetTag, cellRef),
		// The cell reference is the address; nothing computed beats it.
		Name:         model.StructuralPath(append(strings.Split(partPath, "/"), cellRef)...),
		Type:         "cell",
		Translatable: false,
		Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
		Properties: map[string]string{
			"partPath":            partPath,
			"cell":                cellRef,
			model.PropCellDisplay: display,
			model.PropCellFormat:  code,
		},
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

// Property a worksheet cell block carries when its text lives in the cell as an
// inline string (`<c t="inlineStr"><is>…</is></c>`) rather than in a value
// element or the shared-string table. The writer reads it to spell the wrapper
// back; see renderSMLBlock.
const (
	cellStorageProp   = "openxml:cell-storage"
	cellStorageInline = "inlineStr"
)

// Property a CT_Rst block carries when its source element held phonetic markup:
// the `<rPh>` phonetic runs (ECMA-376 Part 1 §18.4.6) and the `<phoneticPr>`
// phonetic properties (§18.4.3), as the source wrote them, in source order.
//
// A phonetic run spells the reading of the base text, which makes it guidance
// about the string rather than a stretch of it: a translator sees the base text
// alone, and the guide travels beside it. Upstream Okapi reads the two elements
// the same way, as SkippableElement.PhoneticInline consumed by
// StringItemParser, and drops them on write; the writer here replays their
// bytes. CT_Rst orders them after the text and the runs, so appending them to
// the rendered content restores their place.
//
// The `sb` and `eb` attributes index the base text a phonetic run reads, so a
// translated cell keeps a guide written against the source reading.
const cellPhoneticProp = "openxml:sml-phonetic"

// Property a CT_Rst block carries when the writer would spell its content
// differently from the source: the element's content as the source wrote it,
// between the <si> or <is> tags.
//
// Four forms live in those bytes and in nothing else. Excel writes
// xml:space="preserve" on text that has no leading or trailing whitespace,
// where the writer emits it only where the text needs it. A character
// reference (`&#8217;`) is the character by the time the decoder reports it. A
// carriage return inside <t> is normalised to a line feed (XML 1.0 §2.11). The
// whitespace a producer indented an <si>, an <r>, an <rPr> and a <t> with
// belongs to none of them.
//
// The reader stores this only where it differs from what the writer would
// produce, so an ordinary workbook carries nothing extra, and the writer
// replays it only for content nothing has changed. See smlSourceContent.
const cellSourceProp = "openxml:sml-source"

// smlSourceForm returns the CT_Rst content to keep on a block, or "" when the
// writer would rebuild it byte for byte from the runs alone.
func smlSourceForm(content string, runs []model.Run, phonetic string) string {
	if content == renderSMLRichText(runs)+phonetic {
		return ""
	}
	if !smlTextIsSpaceSafe(content) {
		return ""
	}
	return content
}

// smlTextIsSpaceSafe reports whether every <t> in a CT_Rst declares
// xml:space="preserve" where its text needs it.
//
// XML preserves whitespace in element content, but a spreadsheet reads a <t>
// without the attribute as text it may trim, and Excel writes the attribute
// under exactly this condition (ECMA-376 Part 1 §18.4.12). A workbook that
// arrives without it is asking for whitespace to be lost, so the writer adds
// it, and the source form is not replayed over the top of that repair.
func smlTextIsSpaceSafe(content string) bool {
	d := newRawDecoderString(content)
	for {
		tok, err := d.Token()
		if err != nil {
			return true
		}
		t, ok := tok.(xml.StartElement)
		if !ok || t.Name.Local != "t" {
			continue
		}
		preserve := attrVal(t, "space") == "preserve"
		text, err := readCharData(d)
		if err != nil {
			return true
		}
		if !preserve && strings.Trim(text, xmlWhitespace) != text {
			return false
		}
	}
}

// capturePhoneticElement consumes a phonetic element the decoder has just
// reported the start of, returning its source bytes.
func capturePhoneticElement(d *rawDecoder) (string, error) {
	off := d.Offset()
	d.Pin(off)
	defer d.Unpin()
	if err := skipElement(d); err != nil {
		return "", err
	}
	return d.FromString(off), nil
}

// parseInlineString reads an inline string element <is>, returning the element
// verbatim and the rich text runs it holds. <is> carries CT_Rst, the content
// model sharedStrings.xml uses for <si>, so its runs parse the same way.
func (p *smlParser) parseInlineString(d *rawDecoder, start xml.StartElement) (raw string, runs []textRun, phonetic string, err error) {
	raw, err = captureRawElement(d, start)
	if err != nil {
		return "", nil, "", err
	}
	runs, phonetic = p.parseRst(raw)
	return raw, runs, phonetic, nil
}

// rstInnerContent returns what a captured CT_Rst element holds between its
// tags, which is the half the writer rebuilds. It reports "" for an element
// whose tags are not the plain pair the writer writes, so a producer that
// spelled the wrapper some other way falls through to the rebuild rather than
// having the wrapper rewritten around its own content.
func rstInnerContent(raw, name string) string {
	d := newRawDecoderString(raw)
	tok, err := d.Token()
	if err != nil {
		return ""
	}
	start, ok := tok.(xml.StartElement)
	if !ok || start.Name.Local != name {
		return ""
	}
	inner := d.EndOffset()
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return ""
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	content := d.UptoString(inner)
	if "<"+name+">"+content+"</"+name+">" != raw {
		return ""
	}
	return content
}

// parseRst reads the text runs of a CT_Rst element (<si> or <is>): either a
// single <t>, or a sequence of <r> elements each with its own <rPr>. It also
// returns the element's phonetic markup, which is a reading guide rather than
// text; see cellPhoneticProp.
func (p *smlParser) parseRst(raw string) (runs []textRun, phonetic string) {
	d := newRawDecoderString(raw)
	var props runProps

	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		t, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch t.Name.Local {
		case "r":
			props = runProps{}
		case "rPr":
			props = p.parseSMLRunProps(d)
		case "rPh", "phoneticPr":
			captured, err := capturePhoneticElement(d)
			if err != nil {
				return mergeRuns(runs), phonetic
			}
			phonetic += captured
		case "t":
			text, err := readCharData(d)
			if err != nil {
				return mergeRuns(runs), phonetic
			}
			runs = append(runs, textRun{text: text, props: props})
		}
	}
	return mergeRuns(runs), phonetic
}

// rstText concatenates the text of a CT_Rst element's runs.
func rstText(runs []textRun) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.text)
	}
	return b.String()
}

// rstModelRuns converts a CT_Rst element's text runs into model runs, opening
// and closing an inline code around each stretch that carries run properties.
//
// A stretch ends where any part of the <rPr> changes, the children the model
// does not name included, so a run of red text keeps its own boundary.
func rstModelRuns(runs []textRun) []model.Run {
	b := &runBuilder{}
	ids := &spanIDs{}
	var activeProps *runProps

	for _, run := range runs {
		if activeProps == nil || !smlPropsEqual(*activeProps, run.props) {
			if activeProps != nil {
				smlRunPropsProjection.appendClosing(*activeProps, b, ids)
			}
			smlRunPropsProjection.appendOpening(run.props, b, ids)
			propsCopy := run.props
			activeProps = &propsCopy
		}

		b.AddText(run.text)
	}

	if activeProps != nil {
		smlRunPropsProjection.appendClosing(*activeProps, b, ids)
	}

	return b.Runs()
}

// smlPropsEqual reports whether two CT_Rst runs would produce the same <rPr>.
func smlPropsEqual(a, b runProps) bool {
	return a.equal(b) && a.smlRPrEqual(b)
}

// buildBlock creates a model.Block from shared string text runs.
func (p *smlParser) buildBlock(id string, runs []textRun, partPath string, siIndex int, phonetic, content string) *model.Block {
	props := map[string]string{
		"partPath": partPath,
		"siIndex":  strconv.Itoa(siIndex),
	}
	if phonetic != "" {
		props[cellPhoneticProp] = phonetic
	}
	source := rstModelRuns(runs)
	if form := smlSourceForm(content, source, phonetic); form != "" {
		props[cellSourceProp] = form
	}
	return &model.Block{
		ID: id,
		// A shared string is addressed by the index every cell references it
		// by — the format's own key, and a better name than any count.
		Name:         model.StructuralPath(append(strings.Split(partPath, "/"), "si["+strconv.Itoa(siIndex)+"]")...),
		Type:         "shared-string",
		Translatable: true,
		Source:       source,
		Properties:   props,
	}
}

// parseTable parses an Excel table definition (xl/tables/tableN.xml) and emits
// blocks for translatable tableColumn name attributes. Excel requires these
// names to match the header row cell values; without updating them after
// translating shared strings, the file is reported as corrupted.
func (p *smlParser) parseTable(data []byte, partPath string, emitBlock func(*model.Block)) error {
	d := newRawDecoder(data)

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
				p.skelWriteTableColumn(d, t, partPath, emitBlock)
				continue
			}
			p.skelWriteStartElement(d, t)

		case xml.EndElement:
			p.skelWriteEndElement(d)

		case xml.CharData:
			p.skelRaw(d)

		case xml.ProcInst:
			p.skelRaw(d)
		}
	}
	return nil
}

// skelWriteTableColumn writes a <tableColumn> element to the skeleton,
// extracting the "name" attribute as a translatable block. Every byte of the
// element other than the name value is replayed from the source, so the
// element keeps its self-closing form, its attribute order and its quoting.
func (p *smlParser) skelWriteTableColumn(d *rawDecoder, t xml.StartElement, partPath string, emitBlock func(*model.Block)) {
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

	raw := d.Raw()
	valStart, valEnd, spanOK := 0, 0, false
	if nameIdx >= 0 {
		valStart, valEnd, spanOK = rawAttrValueSpan(raw, nameIdx)
	}

	if nameIdx < 0 || strings.TrimSpace(nameVal) == "" {
		// No translatable name — write the element unchanged.
		p.skelWriteStartElement(d, t)
		return
	}
	if p.skeletonStore != nil && !spanOK {
		// A start tag whose attribute list will not parse byte for byte
		// leaves nowhere to put the ref, and a block whose ref never
		// reaches the skeleton cannot be written back. Pass it through.
		p.skelWriteStartElement(d, t)
		return
	}
	if p.skeletonStore == nil {
		p.skelWriteStartElement(d, t)
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

	if spanOK && p.skeletonStore != nil {
		p.skelBuf.Write(raw[:valStart])
		p.skelRef(blockID)
		p.skelBuf.Write(raw[valEnd:])
	}

	emitBlock(&model.Block{
		ID:           blockID,
		Name:         blockName,
		Type:         "table-column",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: nameVal}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   map[string]string{"partPath": partPath},
	})
}

// emitXLSXCommentData scans an Excel comment part (xl/comments*.xml) for
// <commentList><comment ref="A1"><text>…</text> bodies and surfaces each as an
// informational Data part carrying the concatenated comment text and the cell
// reference (#928). The comment part is not parsed for skeleton (the writer
// copies it verbatim), so this is purely additive and never affects the
// round-trip. Best-effort: a malformed part yields no Data rather than failing
// the read.
func emitXLSXCommentData(data []byte, emitData func(name, text, ref string)) {
	d := newRawDecoder(data)
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
func collectXLSXTextBody(d *rawDecoder) string {
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
			if t.Name.Local == "rPh" || t.Name.Local == "phoneticPr" {
				// A phonetic guide reads the base text; it is not text of its
				// own. skipElement consumes the whole element, so depth is
				// still balanced. See cellPhoneticProp.
				if err := skipElement(d); err != nil {
					return text.String()
				}
				continue
			}
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

// skelRaw appends the source bytes of the token the decoder last returned.
func (p *smlParser) skelRaw(d *rawDecoder) {
	if p.skeletonStore != nil {
		p.skelBuf.Write(d.Raw())
	}
}

func (p *smlParser) skelWriteStartElement(d *rawDecoder, t xml.StartElement) {
	if p.skeletonStore == nil {
		return
	}
	registerNamespaces(t.Attr)
	p.skelBuf.Write(d.Raw())
}

// skelWriteEndElement appends an end tag. A self-closing element's synthetic end
// carries no source bytes, so its `/>` came through with the start element.
func (p *smlParser) skelWriteEndElement(d *rawDecoder) {
	if p.skeletonStore == nil {
		return
	}
	p.skelBuf.Write(d.Raw())
}

func (p *smlParser) skelWriteString(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}
