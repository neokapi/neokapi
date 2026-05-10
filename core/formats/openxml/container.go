package openxml

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// docType identifies the OpenXML document type.
type docType int

const (
	docTypeUnknown docType = iota
	docTypeDOCX
	docTypePPTX
	docTypeXLSX
)

func (dt docType) String() string {
	switch dt {
	case docTypeDOCX:
		return "docx"
	case docTypePPTX:
		return "pptx"
	case docTypeXLSX:
		return "xlsx"
	default:
		return "unknown"
	}
}

// containerInfo holds parsed metadata about an OpenXML ZIP container.
type containerInfo struct {
	docType           docType
	translatableParts []string // ordered list of XML part paths to extract
	mainDocumentPart  string   // e.g., "word/document.xml"
	relationships     map[string][]relationship
	sharedStrings     []string // XLSX: shared string table (populated during parsing)
}

// relationship represents an OpenXML relationship entry.
type relationship struct {
	ID     string
	Type   string
	Target string
}

// contentType represents an entry in [Content_Types].xml.
type contentType struct {
	PartName    string
	ContentType string
}

// Well-known content type prefixes for detection.
const (
	ctWordDoc    = "application/vnd.openxmlformats-officedocument.wordprocessingml"
	ctPresentDoc = "application/vnd.openxmlformats-officedocument.presentationml"
	ctSpreadDoc  = "application/vnd.openxmlformats-officedocument.spreadsheetml"
)

// Well-known relationship types.
const (
	relTypeMainDoc     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	relTypeHeader      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/header"
	relTypeFooter      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer"
	relTypeFootnotes   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footnotes"
	relTypeEndnotes    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/endnotes"
	relTypeComments    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
	relTypeHyperlink   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
	relTypeChart       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart"
	relTypeDiagramData = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/diagramData"
)

// parseContainer analyzes the ZIP archive and returns container metadata.
func parseContainer(zr *zip.Reader, cfg *Config) (*containerInfo, error) {
	info := &containerInfo{
		relationships: make(map[string][]relationship),
	}

	// Parse [Content_Types].xml
	ctypes, err := parseContentTypes(zr)
	if err != nil {
		return nil, fmt.Errorf("openxml: %w", err)
	}

	// Detect document type from content types
	info.docType = detectDocType(ctypes)
	if info.docType == docTypeUnknown {
		return nil, errors.New("openxml: unable to determine document type from [Content_Types].xml")
	}

	// Parse all .rels files
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".rels") {
			rels, err := parseRelationships(f)
			if err != nil {
				return nil, fmt.Errorf("openxml: parsing %s: %w", f.Name, err)
			}
			info.relationships[f.Name] = rels
		}
	}

	// Find main document part
	info.mainDocumentPart = findMainDocumentPart(info.relationships)

	// Build ordered list of translatable parts
	info.translatableParts = buildTranslatableParts(info, cfg)

	return info, nil
}

// parseContentTypes parses [Content_Types].xml from the ZIP.
func parseContentTypes(zr *zip.Reader) ([]contentType, error) {
	var ctFile *zip.File
	for _, f := range zr.File {
		if f.Name == "[Content_Types].xml" {
			ctFile = f
			break
		}
	}
	if ctFile == nil {
		return nil, errors.New("missing [Content_Types].xml")
	}

	rc, err := ctFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var result []contentType
	d := xml.NewDecoder(rc)
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			switch se.Name.Local {
			case "Override":
				ct := contentType{}
				for _, a := range se.Attr {
					switch a.Name.Local {
					case "PartName":
						ct.PartName = strings.TrimPrefix(a.Value, "/")
					case "ContentType":
						ct.ContentType = a.Value
					}
				}
				if ct.PartName != "" {
					result = append(result, ct)
				}
			}
		}
	}
	return result, nil
}

// detectDocType determines the document type from content type entries.
func detectDocType(ctypes []contentType) docType {
	for _, ct := range ctypes {
		switch {
		case strings.HasPrefix(ct.ContentType, ctWordDoc):
			return docTypeDOCX
		case strings.HasPrefix(ct.ContentType, ctPresentDoc):
			return docTypePPTX
		case strings.HasPrefix(ct.ContentType, ctSpreadDoc):
			return docTypeXLSX
		}
	}
	return docTypeUnknown
}

// parseRelationships parses a .rels XML file.
func parseRelationships(f *zip.File) ([]relationship, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var rels []relationship
	d := xml.NewDecoder(rc)
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "Relationship" {
			rel := relationship{}
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "Id":
					rel.ID = a.Value
				case "Type":
					rel.Type = a.Value
				case "Target":
					rel.Target = a.Value
				}
			}
			rels = append(rels, rel)
		}
	}
	return rels, nil
}

// findMainDocumentPart finds the main document part path from root relationships.
func findMainDocumentPart(allRels map[string][]relationship) string {
	rootRels := allRels["_rels/.rels"]
	for _, rel := range rootRels {
		if rel.Type == relTypeMainDoc {
			return rel.Target
		}
	}
	return ""
}

// buildTranslatableParts returns an ordered list of ZIP entry paths that contain
// translatable XML content for the given document type.
func buildTranslatableParts(info *containerInfo, cfg *Config) []string {
	switch info.docType {
	case docTypeDOCX:
		return buildDOCXParts(info, cfg)
	case docTypePPTX:
		return buildPPTXParts(info, cfg)
	case docTypeXLSX:
		return buildXLSXParts(info, cfg)
	default:
		if info.mainDocumentPart != "" {
			return []string{info.mainDocumentPart}
		}
		return nil
	}
}

// buildDOCXParts returns the ordered translatable parts for a DOCX document.
func buildDOCXParts(info *containerInfo, cfg *Config) []string {
	var parts []string

	// Main document is always first
	if info.mainDocumentPart != "" {
		parts = append(parts, info.mainDocumentPart)
	}

	// Get document-level relationships
	mainDir := ""
	if idx := strings.LastIndex(info.mainDocumentPart, "/"); idx >= 0 {
		mainDir = info.mainDocumentPart[:idx+1]
	}
	relsPath := mainDir + "_rels/" + info.mainDocumentPart[len(mainDir):] + ".rels"
	docRels := info.relationships[relsPath]

	// Collect parts by type
	var headers, footers []string
	var footnotes, endnotes, comments string

	for _, rel := range docRels {
		target := rel.Target
		if !strings.Contains(target, "/") {
			target = mainDir + target
		}

		switch rel.Type {
		case relTypeHeader:
			if cfg.TranslateHeadersFooters {
				headers = append(headers, target)
			}
		case relTypeFooter:
			if cfg.TranslateHeadersFooters {
				footers = append(footers, target)
			}
		case relTypeFootnotes:
			if cfg.TranslateFootnotes {
				footnotes = target
			}
		case relTypeEndnotes:
			if cfg.TranslateFootnotes {
				endnotes = target
			}
		case relTypeComments:
			if cfg.TranslateComments {
				comments = target
			}
		}
	}

	// Sort headers/footers for deterministic order
	slices.Sort(headers)
	slices.Sort(footers)

	parts = append(parts, headers...)
	parts = append(parts, footers...)
	if footnotes != "" {
		parts = append(parts, footnotes)
	}
	if endnotes != "" {
		parts = append(parts, endnotes)
	}
	if comments != "" {
		parts = append(parts, comments)
	}

	// Chart and diagram parts. These contain DrawingML <a:p> paragraphs
	// with translatable text (chart titles, axis labels, SmartArt node
	// labels). Mirrors okapi WordDocument.java line 202-203 / 369:
	//
	//	type.equals(Drawing.DIAGRAM_DATA_TYPE) ||
	//	type.equals(Drawing.CHART_TYPE) ||
	//
	// Charts and diagrams can be referenced from the main document OR
	// from header/footer parts (a header containing a chart is rare but
	// allowed by ECMA-376). We scan every .rels file for the relevant
	// relationship types and de-duplicate.
	parts = appendChartAndDiagramParts(parts, info)

	// Document properties (core.xml)
	if cfg.TranslateDocProperties {
		parts = append(parts, "docProps/core.xml")
	}

	return parts
}

// appendChartAndDiagramParts scans every .rels file for chart and
// diagramData relationship targets, sorts them deterministically, and
// appends them (de-duplicated against `parts`). The same chart can be
// referenced from multiple parts (e.g. linked across header + body),
// so de-duplication is essential.
func appendChartAndDiagramParts(parts []string, info *containerInfo) []string {
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		seen[p] = struct{}{}
	}
	var charts, diagrams []string
	for relsPath, rels := range info.relationships {
		for _, rel := range rels {
			target := resolveRelTarget(relsPath, rel.Target)
			if _, dup := seen[target]; dup {
				continue
			}
			switch rel.Type {
			case relTypeChart:
				charts = append(charts, target)
				seen[target] = struct{}{}
			case relTypeDiagramData:
				diagrams = append(diagrams, target)
				seen[target] = struct{}{}
			}
		}
	}
	slices.Sort(charts)
	slices.Sort(diagrams)
	parts = append(parts, charts...)
	parts = append(parts, diagrams...)
	return parts
}

// buildPPTXParts returns the ordered translatable parts for a PPTX document.
func buildPPTXParts(info *containerInfo, cfg *Config) []string {
	var slides, notes, masters, layouts, comments []string

	// Discover parts from content types stored in relationships
	for relsPath, rels := range info.relationships {
		for _, rel := range rels {
			target := resolveRelTarget(relsPath, rel.Target)

			switch rel.Type {
			case "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide":
				slides = append(slides, target)
			case "http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide":
				if cfg.TranslateSlideNotes {
					notes = append(notes, target)
				}
			case "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster":
				if cfg.TranslateSlideMasters {
					masters = append(masters, target)
				}
			case "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout":
				layouts = append(layouts, target)
			case "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments":
				if cfg.TranslateComments {
					comments = append(comments, target)
				}
			}
		}
	}

	slices.Sort(slides)
	slices.Sort(notes)
	slices.Sort(masters)
	slices.Sort(layouts)
	slices.Sort(comments)

	var parts []string
	parts = append(parts, slides...)
	parts = append(parts, notes...)
	parts = append(parts, masters...)
	parts = append(parts, layouts...)
	parts = append(parts, comments...)

	// Document properties
	if cfg.TranslateDocProperties {
		parts = append(parts, "docProps/core.xml")
	}

	return parts
}

// resolveRelTarget resolves a relationship target relative to a .rels file path.
// E.g., relsPath="ppt/_rels/presentation.xml.rels", target="slides/slide1.xml"
// → "ppt/slides/slide1.xml"
func resolveRelTarget(relsPath, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}
	// The .rels file is in a _rels/ subdirectory. The base dir for resolution
	// is the parent of _rels/. E.g., "ppt/_rels/foo.xml.rels" → base is "ppt/".
	dir := ""
	if idx := strings.LastIndex(relsPath, "/"); idx >= 0 {
		dir = relsPath[:idx+1] // e.g., "ppt/_rels/"
	}
	// Strip _rels/ suffix to get the actual base directory
	dir = strings.Replace(dir, "_rels/", "", 1)
	resolved := dir + target
	// Normalize ".." path segments (e.g., "xl/worksheets/../tables/t.xml" → "xl/tables/t.xml")
	return cleanZipPath(resolved)
}

// cleanZipPath normalizes a ZIP-internal path by resolving ".." segments.
func cleanZipPath(p string) string {
	parts := strings.Split(p, "/")
	var out []string
	for _, seg := range parts {
		if seg == ".." && len(out) > 0 {
			out = out[:len(out)-1]
		} else if seg != "." && seg != ".." {
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/")
}

// buildXLSXParts returns the ordered translatable parts for an XLSX document.
func buildXLSXParts(info *containerInfo, cfg *Config) []string {
	var parts []string

	// Shared strings first (if configured)
	if cfg.TranslateSharedStrings {
		for relsPath, rels := range info.relationships {
			for _, rel := range rels {
				if rel.Type == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" {
					parts = append(parts, resolveRelTarget(relsPath, rel.Target))
				}
			}
		}
	}

	// Worksheets
	var sheets []string
	for relsPath, rels := range info.relationships {
		for _, rel := range rels {
			if rel.Type == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" {
				sheets = append(sheets, resolveRelTarget(relsPath, rel.Target))
			}
		}
	}
	slices.Sort(sheets)
	parts = append(parts, sheets...)

	// Tables (column names must stay in sync with header row cell values)
	var tables []string
	for relsPath, rels := range info.relationships {
		for _, rel := range rels {
			if rel.Type == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/table" {
				tables = append(tables, resolveRelTarget(relsPath, rel.Target))
			}
		}
	}
	slices.Sort(tables)
	parts = append(parts, tables...)

	// Comments
	if cfg.TranslateComments {
		for relsPath, rels := range info.relationships {
			for _, rel := range rels {
				if rel.Type == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments" {
					parts = append(parts, resolveRelTarget(relsPath, rel.Target))
				}
			}
		}
	}

	// Document properties
	if cfg.TranslateDocProperties {
		parts = append(parts, "docProps/core.xml")
	}

	return parts
}

// parseSharedStrings parses xl/sharedStrings.xml and returns the string table.
func parseSharedStrings(zr *zip.Reader) ([]string, error) {
	f := zipFileByName(zr, "xl/sharedStrings.xml")
	if f == nil {
		return nil, nil // No shared strings — not an error
	}

	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var table []string
	d := xml.NewDecoder(rc)
	var inSI, inT bool
	var currentText strings.Builder

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSI = true
				currentText.Reset()
			case "t":
				if inSI {
					inT = true
				}
			case "r":
				// Rich text run inside <si> — the <t> inside <r> contributes text
			}
		case xml.CharData:
			if inSI && inT {
				currentText.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inT = false
			case "si":
				table = append(table, currentText.String())
				inSI = false
			}
		}
	}

	return table, nil
}

// relsByID returns a map of relationship ID → relationship for a given rels path.
func relsByID(info *containerInfo, relsPath string) map[string]relationship {
	m := make(map[string]relationship)
	for _, rel := range info.relationships[relsPath] {
		m[rel.ID] = rel
	}
	return m
}

// zipFileByName returns the zip.File for a given path, or nil.
func zipFileByName(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}
