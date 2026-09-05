package openxml

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/safeio"
)

// cellStyles resolves a worksheet cell's style index to the number-format
// code it displays through (ECMA-376 Part 1 §18.8.10 cellXfs, §18.8.31
// numFmts), together with the workbook's date epoch.
//
// A cell carries `s`, an index into the stylesheet's cellXfs; the xf at that
// index names a numFmtId; the id is either a custom code declared under
// numFmts or one of the built-in ids of §18.8.30. Fonts, fills, borders and
// alignment live in the same xf and are not read: this is about what a value
// shows, not how the cell is painted.
type cellStyles struct {
	// numFmts maps a custom numFmtId to its format code.
	numFmts map[int]string
	// xfNumFmt holds each cellXfs entry's numFmtId, by style index.
	xfNumFmt []int
	// date1904 is the workbook's epoch switch (<workbookPr date1904="1"/>).
	date1904 bool
}

// formatCode returns the number-format code for a cell style index: the
// custom code the xf names, the built-in code for a built-in id, and General
// for an index or id the stylesheet does not carry. ok is false when the id
// is a built-in the renderer has no invariant code for (the East Asian and
// Thai calendar ids), so the caller shows the stored value.
func (s *cellStyles) formatCode(styleIdx string) (code string, ok bool) {
	if s == nil || styleIdx == "" {
		return numFmtGeneral, true
	}
	idx, err := strconv.Atoi(strings.TrimSpace(styleIdx))
	if err != nil || idx < 0 || idx >= len(s.xfNumFmt) {
		return numFmtGeneral, true
	}
	return s.numFmtCode(s.xfNumFmt[idx])
}

// numFmtCode resolves a numFmtId: custom declarations win over the built-in
// table, as the stylesheet may redefine a built-in id.
func (s *cellStyles) numFmtCode(id int) (string, bool) {
	if code, ok := s.numFmts[id]; ok {
		return code, true
	}
	if code, ok := builtInNumFmt(id); ok {
		return code, true
	}
	return numFmtGeneral, false
}

// parseCellStyles reads the workbook's stylesheet and epoch. A workbook with
// no stylesheet yields a resolver that answers General for every cell.
func parseCellStyles(zr *zip.Reader, rels map[string][]relationship) *cellStyles {
	s := &cellStyles{numFmts: map[int]string{}}
	s.date1904 = parseDate1904(zr)

	path := "xl/styles.xml"
	for _, rel := range rels["xl/_rels/workbook.xml.rels"] {
		if rel.Type == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" {
			path = resolveRelTarget("xl/_rels/workbook.xml.rels", rel.Target)
			break
		}
	}
	f := zipFileByName(zr, path)
	if f == nil {
		return s
	}
	rc, err := safeio.DefaultZipLimits.OpenEntry(f)
	if err != nil {
		return s
	}
	defer rc.Close()
	s.parseStylesheet(rc)
	return s
}

// parseStylesheet collects numFmts and cellXfs from a stylesheet part. The
// cellStyleXfs list also holds xf elements and is skipped: a cell's `s` indexes
// cellXfs only. A malformed part keeps whatever was read before the error.
func (s *cellStyles) parseStylesheet(r io.Reader) {
	d := xml.NewDecoder(r)
	var inNumFmts, inCellXfs bool
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) || err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "numFmts":
				inNumFmts = true
			case "cellXfs":
				inCellXfs = true
			case "numFmt":
				if !inNumFmts {
					continue
				}
				id, err := strconv.Atoi(attrVal(t, "numFmtId"))
				if err != nil {
					continue
				}
				s.numFmts[id] = attrVal(t, "formatCode")
			case "xf":
				if !inCellXfs {
					continue
				}
				id, err := strconv.Atoi(attrVal(t, "numFmtId"))
				if err != nil {
					id = 0
				}
				s.xfNumFmt = append(s.xfNumFmt, id)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "numFmts":
				inNumFmts = false
			case "cellXfs":
				inCellXfs = false
			}
		}
	}
}

// workbookPrRE finds the workbook properties element; date1904 is one of its
// attributes.
var workbookPrRE = regexp.MustCompile(`<workbookPr\b[^>]*>`)

// parseDate1904 reads the epoch switch from xl/workbook.xml. Absent element,
// absent attribute, or an unreadable part all mean the 1900 system.
func parseDate1904(zr *zip.Reader) bool {
	f := zipFileByName(zr, "xl/workbook.xml")
	if f == nil {
		return false
	}
	data, err := safeio.DefaultZipLimits.ReadEntry(f)
	if err != nil {
		return false
	}
	tag := workbookPrRE.Find(data)
	if tag == nil {
		return false
	}
	return xsdBoolean(attrFromTag(string(tag), "date1904"))
}

// xsdBoolean reads an XML Schema boolean: "1" and "true" are true.
func xsdBoolean(v string) bool {
	switch strings.TrimSpace(v) {
	case "1", "true":
		return true
	}
	return false
}

// epoch1904 reports the workbook's date system; a workbook without a
// stylesheet resolver counts from 1900.
func (s *cellStyles) epoch1904() bool {
	return s != nil && s.date1904
}
