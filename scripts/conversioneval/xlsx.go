package main

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"path"
	"strconv"
	"strings"
)

// A spreadsheet's text is in its cells, not in its string table.
//
// The first version read `xl/sharedStrings.xml` and called that the ground
// truth. That table holds each distinct string once; a sheet refers to it by
// index, so a string used in five hundred cells appears once in the truth and
// five hundred times in any converter's output. Recall is
// min(output, truth)/truth, so undercounting the truth makes every score easier
// — and both converters came back at exactly 100.0% on .xlsx, which is what a
// metric that cannot fail looks like.
//
// Across this corpus the table holds 158,757 strings and the sheets refer to
// them 405,222 times, so the undercount was a factor of 2.55.
//
// Reading the cells is the fix: resolve every shared-string reference through
// the table, take inline strings where a sheet carries them, and keep formula
// results, which are text a reader sees.

// xlsxCellText returns the text of every cell in every sheet, with the
// multiplicity a reader would see.
func xlsxCellText(file string) ([]string, error) {
	z, err := zip.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer z.Close()

	var shared []string
	for _, f := range z.File {
		if f.Name != "xl/sharedStrings.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		shared, err = textNodes(rc, "t")
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		break
	}

	var out []string
	for _, f := range z.File {
		if !strings.HasPrefix(f.Name, "xl/worksheets/sheet") || path.Ext(f.Name) != ".xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		cells, err := sheetCells(rc, shared)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, cells...)
	}
	return out, nil
}

// sheetCells walks one sheet and resolves each cell to the text it shows.
//
// The cell's `t` attribute decides where the text is:
//
//	s          the <v> is an index into the shared-string table
//	inlineStr  the text is in <is><t> on the cell itself
//	str        the <v> is a formula's string result
//	(absent)   a number, which is not text a converter is asked to preserve as
//	           words and is skipped
func sheetCells(r io.Reader, shared []string) ([]string, error) {
	dec := xml.NewDecoder(r)
	var out []string
	var cellType string
	var inCell, inValue, inInline bool
	var buf strings.Builder

	flush := func() {
		text := buf.String()
		buf.Reset()
		switch cellType {
		case "s":
			i, err := strconv.Atoi(strings.TrimSpace(text))
			if err == nil && i >= 0 && i < len(shared) {
				out = append(out, shared[i])
			}
		case "str", "inlineStr":
			if strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "c":
				inCell, cellType = true, ""
				for _, a := range t.Attr {
					if a.Name.Local == "t" {
						cellType = a.Value
					}
				}
			case "v":
				if inCell {
					inValue = true
					buf.Reset()
				}
			case "is":
				if inCell {
					inInline = true
					buf.Reset()
				}
			}
		case xml.CharData:
			if inValue || inInline {
				buf.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				if inValue {
					inValue = false
					flush()
				}
			case "is":
				if inInline {
					inInline = false
					// An inline string's text sits in <t> children, which the
					// CharData branch has already accumulated.
					cellType = "inlineStr"
					flush()
				}
			case "c":
				inCell, cellType = false, ""
			}
		}
	}
}
