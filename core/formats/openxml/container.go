package openxml

import (
	"archive/zip"
	"cmp"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/safeio"
	"github.com/neokapi/neokapi/core/xmlesc"
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
	// sheetNames maps an XLSX worksheet part path to the sheet's display name.
	// The name is in xl/workbook.xml and the part path is behind a relationship
	// id, so only joining the two yields "which sheet is this part".
	sheetNames map[string]string
	// cellStyles resolves an XLSX cell's style index to its number-format
	// code, and carries the workbook's date epoch.
	cellStyles *cellStyles
	// slideOrder is a presentation's slide parts in show order, read from
	// presentation.xml's sldIdLst. Part names follow creation order (a slide
	// moved to the front keeps its number), so the list is the only source
	// of the order the deck plays in. Nil when it could not be read.
	slideOrder []string
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

// Well-known relationship types. Both the Transitional (ECMA-376
// Part 1 §A.1, schemas.openxmlformats.org URIs) and Strict
// (ISO/IEC 29500-1 §A.1, purl.oclc.org URIs) variants are accepted —
// .docx files saved as "Strict Open XML" by Word use the purl form
// for the officeDocument relationship type even though the inner
// [Content_Types].xml still declares Transitional content types
// (859.docx is the canonical fixture). Upstream Okapi accepts both
// via the Namespaces enum (StrictDocumentRelationships +
// DocumentRelationships) — see Namespaces.class in
// okapi-filter-openxml-1.48.0.
const (
	relTypeMainDoc           = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	relTypeMainDocStrict     = "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument"
	relTypeHeader            = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/header"
	relTypeHeaderStrict      = "http://purl.oclc.org/ooxml/officeDocument/relationships/header"
	relTypeFooter            = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer"
	relTypeFooterStrict      = "http://purl.oclc.org/ooxml/officeDocument/relationships/footer"
	relTypeFootnotes         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footnotes"
	relTypeFootnotesStrict   = "http://purl.oclc.org/ooxml/officeDocument/relationships/footnotes"
	relTypeEndnotes          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/endnotes"
	relTypeEndnotesStrict    = "http://purl.oclc.org/ooxml/officeDocument/relationships/endnotes"
	relTypeComments          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
	relTypeCommentsStrict    = "http://purl.oclc.org/ooxml/officeDocument/relationships/comments"
	relTypeHyperlink         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
	relTypeHyperlinkStrict   = "http://purl.oclc.org/ooxml/officeDocument/relationships/hyperlink"
	relTypeChart             = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart"
	relTypeChartStrict       = "http://purl.oclc.org/ooxml/officeDocument/relationships/chart"
	relTypeDiagramData       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/diagramData"
	relTypeDiagramDataStrict = "http://purl.oclc.org/ooxml/officeDocument/relationships/diagramData"
	// Glossary document — a parallel WordprocessingML package containing
	// AutoText/building-block entries (ECMA-376 Part 1 §17.12.7). The
	// translatable content is the building-block run text inside
	// <w:docParts><w:docPart><w:docPartBody>. Okapi's WordDocument.java
	// adds /glossaryDocument to its styled-text-part set, so the same
	// reader path handles it — see GLOSSARY_DOCUMENT relationship in
	// WordDocument.java line 100-107 and isGlossaryStyledTextPart at
	// line 208-211.
	relTypeGlossaryDocument       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/glossaryDocument"
	relTypeGlossaryDocumentStrict = "http://purl.oclc.org/ooxml/officeDocument/relationships/glossaryDocument"
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
	if info.docType == docTypePPTX {
		info.slideOrder = parseSlideOrder(zr, info)
	}

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

	rc, err := safeio.DefaultZipLimits.OpenEntry(ctFile)
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
	rc, err := safeio.DefaultZipLimits.OpenEntry(f)
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
		if rel.Type == relTypeMainDoc || rel.Type == relTypeMainDocStrict {
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
	var footnotes, endnotes, comments, glossary string

	for _, rel := range docRels {
		target := rel.Target
		if !strings.Contains(target, "/") {
			target = mainDir + target
		}

		switch rel.Type {
		case relTypeHeader, relTypeHeaderStrict:
			if cfg.TranslateHeadersFooters {
				headers = append(headers, target)
			}
		case relTypeFooter, relTypeFooterStrict:
			if cfg.TranslateHeadersFooters {
				footers = append(footers, target)
			}
		case relTypeFootnotes, relTypeFootnotesStrict:
			if cfg.TranslateFootnotes {
				footnotes = target
			}
		case relTypeEndnotes, relTypeEndnotesStrict:
			if cfg.TranslateFootnotes {
				endnotes = target
			}
		case relTypeComments, relTypeCommentsStrict:
			if cfg.TranslateComments {
				comments = target
			}
		case relTypeGlossaryDocument, relTypeGlossaryDocumentStrict:
			// Glossary part contains AutoText/building-block runs
			// (ECMA-376-1 §17.12.7); Okapi's WordDocument treats it
			// as just another styled-text part — see Practice2.docx
			// glossary/document.xml `[Type text]` placeholder. The
			// glossary target is a *subdirectory*-relative path
			// (e.g. `glossary/document.xml`), so the naive
			// `!strings.Contains(target, "/")` prefix branch above
			// won't apply mainDir. Use resolveRelTarget which honors
			// the rels file path for sibling-of-_rels resolution.
			glossary = resolveRelTarget(relsPath, rel.Target)
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
	if glossary != "" {
		parts = append(parts, glossary)
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
// isCommentPartPath reports whether the ZIP entry path is a reviewer-comment
// part whose body text is surfaced as informational Data (#928): PowerPoint
// ppt/comments/comment*.xml (legacy <p:cm><p:text>) or Excel xl/comments*.xml
// (<comment><text>). Threaded comments (xl/threadedComments/, modern PPTX
// comments) use a different schema and are not handled. Excludes /_rels/.
func isCommentPartPath(name string) bool {
	if !strings.HasSuffix(name, ".xml") || strings.Contains(name, "/_rels/") {
		return false
	}
	switch {
	case strings.HasPrefix(name, "ppt/comments/"):
		return true
	case strings.HasPrefix(name, "xl/comments"):
		return true
	}
	return false
}

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
			case relTypeChart, relTypeChartStrict:
				charts = append(charts, target)
				seen[target] = struct{}{}
			case relTypeDiagramData, relTypeDiagramDataStrict:
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

// PresentationML relationship types (ECMA-376-1 §13.3): the parts a deck
// reaches from presentation.xml and from each slide.
const (
	relTypeSlide       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide"
	relTypeNotesSlide  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide"
	relTypeSlideMaster = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster"
	relTypeSlideLayout = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout"
)

// buildPPTXParts returns the ordered translatable parts for a PPTX document:
// the slides in show order, each followed by its speaker notes, then (with the
// masters option on) the masters and layouts, the comments, and the document
// properties.
//
// Every relationship list in the package is scanned, so a part reachable from
// two places is seen twice: a slide is the target of presentation.xml and of
// its own notes slide. Each part is collected once.
//
// A layout is template furniture like the master it derives from: its text is
// the editor's prompt ("Click to edit Master title style"), drawn on no slide.
// Upstream Okapi extracts layouts and masters under the one masters option
// (PowerpointDocument.slideFragmentsFor), and so does this.
func buildPPTXParts(info *containerInfo, cfg *Config) []string {
	seen := make(map[string]bool)
	var slides, notes, masters, layouts, comments []string
	collect := func(list *[]string, target string) {
		if seen[target] {
			return
		}
		seen[target] = true
		*list = append(*list, target)
	}

	for relsPath, rels := range info.relationships {
		for _, rel := range rels {
			target := resolveRelTarget(relsPath, rel.Target)
			switch rel.Type {
			case relTypeSlide:
				collect(&slides, target)
			case relTypeNotesSlide:
				if cfg.TranslateSlideNotes {
					collect(&notes, target)
				}
			case relTypeSlideMaster:
				if cfg.TranslateSlideMasters {
					collect(&masters, target)
				}
			case relTypeSlideLayout:
				if cfg.TranslateSlideMasters {
					collect(&layouts, target)
				}
			case relTypeComments:
				if cfg.TranslateComments {
					collect(&comments, target)
				}
			}
		}
	}

	slides = orderSlides(slides, info.slideOrder)
	slices.SortFunc(masters, naturalCompare)
	slices.SortFunc(layouts, naturalCompare)
	slices.SortFunc(comments, naturalCompare)

	// Each slide is followed by its speaker notes, so the notes sit under
	// the slide they are about in any export; a notes part no slide claims
	// follows the deck.
	var parts []string
	for _, slide := range slides {
		parts = append(parts, slide)
		parts = append(parts, notesOf(slide, notes, info.relationships)...)
	}
	parts = append(parts, orderNotes(notes, slides, info.relationships)...)
	parts = append(parts, masters...)
	parts = append(parts, layouts...)
	parts = append(parts, comments...)

	// Document properties
	if cfg.TranslateDocProperties {
		parts = append(parts, "docProps/core.xml")
	}

	return parts
}

// orderSlides puts the slide parts in show order. A slide the sldIdLst does not
// name follows the named ones, and with no list at all the parts sort by their
// number.
func orderSlides(slides, show []string) []string {
	present := make(map[string]bool, len(slides))
	for _, s := range slides {
		present[s] = true
	}
	placed := make(map[string]bool, len(slides))
	var out []string
	for _, s := range show {
		if present[s] && !placed[s] {
			placed[s] = true
			out = append(out, s)
		}
	}
	var rest []string
	for _, s := range slides {
		if !placed[s] {
			rest = append(rest, s)
		}
	}
	slices.SortFunc(rest, naturalCompare)
	return append(out, rest...)
}

// notesOf returns the notes parts a slide's own relationships name, among
// those collected for the deck.
func notesOf(slide string, notes []string, rels map[string][]relationship) []string {
	var out []string
	relsPath := partRelsPath(slide)
	for _, rel := range rels[relsPath] {
		if rel.Type != relTypeNotesSlide {
			continue
		}
		target := resolveRelTarget(relsPath, rel.Target)
		if slices.Contains(notes, target) {
			out = append(out, target)
		}
	}
	return out
}

// orderNotes returns the notes parts no slide claims, by number.
func orderNotes(notes, slides []string, rels map[string][]relationship) []string {
	claimed := make(map[string]bool, len(notes))
	for _, slide := range slides {
		for _, n := range notesOf(slide, notes, rels) {
			claimed[n] = true
		}
	}
	var rest []string
	for _, n := range notes {
		if !claimed[n] {
			rest = append(rest, n)
		}
	}
	slices.SortFunc(rest, naturalCompare)
	return rest
}

// partRelsPath is the relationships part of a package part:
// ppt/slides/slide1.xml → ppt/slides/_rels/slide1.xml.rels.
func partRelsPath(part string) string {
	dir, base := "", part
	if idx := strings.LastIndex(part, "/"); idx >= 0 {
		dir, base = part[:idx+1], part[idx+1:]
	}
	return dir + "_rels/" + base + ".rels"
}

// parseSlideOrder reads the presentation part's <p:sldIdLst> and resolves each
// <p:sldId r:id> through the part's relationships to a slide part, in show
// order. Nil when the part or the list cannot be read; buildPPTXParts then
// falls back to the parts' numbering.
func parseSlideOrder(zr *zip.Reader, info *containerInfo) []string {
	main := info.mainDocumentPart
	if main == "" {
		return nil
	}
	f := zipFileByName(zr, main)
	if f == nil {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()

	relsPath := partRelsPath(main)
	byID := make(map[string]string)
	for _, rel := range info.relationships[relsPath] {
		byID[rel.ID] = resolveRelTarget(relsPath, rel.Target)
	}

	dec := xml.NewDecoder(rc)
	var order []string
	inList := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return order
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "sldIdLst":
				inList = true
			case "sldId":
				if !inList {
					continue
				}
				// <p:sldId id="256" r:id="rId2"/>: the bare id is the slide's
				// own number; the relationship id names the part.
				for _, a := range t.Attr {
					if a.Name.Local == "id" && a.Name.Space != "" {
						if target, ok := byID[a.Value]; ok {
							order = append(order, target)
						}
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "sldIdLst" {
				return order
			}
		}
	}
}

// naturalCompare orders part names with their digit runs compared as numbers,
// so slide2.xml sorts before slide10.xml.
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		da, db := leadingDigits(a), leadingDigits(b)
		if da > 0 && db > 0 {
			na, _ := strconv.Atoi(a[:da])
			nb, _ := strconv.Atoi(b[:db])
			if na != nb {
				return cmp.Compare(na, nb)
			}
			a, b = a[da:], b[db:]
			continue
		}
		if a[0] != b[0] {
			return cmp.Compare(a[0], b[0])
		}
		a, b = a[1:], b[1:]
	}
	return cmp.Compare(len(a), len(b))
}

// leadingDigits is the length of the digit run s starts with.
func leadingDigits(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}

// resolveRelTarget resolves a relationship target relative to a .rels file path.
// E.g., relsPath="ppt/_rels/presentation.xml.rels", target="slides/slide1.xml"
// → "ppt/slides/slide1.xml"
func resolveRelTarget(relsPath, target string) string {
	if after, ok := strings.CutPrefix(target, "/"); ok {
		return after
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

// workbookSheet is one <sheet> entry in xl/workbook.xml: the display name a
// user sees on the tab, and the relationship id that leads to its part.
var workbookSheetRE = regexp.MustCompile(`<sheet\b[^>]*>`)

// parseSheetNames maps each worksheet part path to its display name by joining
// xl/workbook.xml's <sheet name= r:id=> entries against the workbook's
// relationships. Returns nil when the workbook part is absent or unreadable —
// a sheet with no recoverable name simply gets none.
func parseSheetNames(zr *zip.Reader, rels map[string][]relationship) map[string]string {
	f := zipFileByName(zr, "xl/workbook.xml")
	if f == nil {
		return nil
	}
	data, err := safeio.DefaultZipLimits.ReadEntry(f)
	if err != nil {
		return nil
	}
	byID := map[string]string{}
	for _, rel := range rels["xl/_rels/workbook.xml.rels"] {
		byID[rel.ID] = resolveRelTarget("xl/_rels/workbook.xml.rels", rel.Target)
	}
	out := map[string]string{}
	for _, tag := range workbookSheetRE.FindAllString(string(data), -1) {
		name := attrFromTag(tag, "name")
		relID := attrFromTag(tag, "r:id")
		if name == "" || relID == "" {
			continue
		}
		if part, ok := byID[relID]; ok {
			out[part] = name
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// attrFromTag reads one attribute value out of a serialized start tag.
func attrFromTag(tag, name string) string {
	i := strings.Index(tag, " "+name+`="`)
	if i < 0 {
		return ""
	}
	rest := tag[i+len(name)+3:]
	before, _, ok := strings.Cut(rest, "\"")
	if !ok {
		return ""
	}
	return xmlesc.UnescapeAttr(before)
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

	rc, err := safeio.DefaultZipLimits.OpenEntry(f)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var table []string
	d := xml.NewDecoder(rc)
	var inSI, inT bool
	var currentText strings.Builder
	// Depth inside a phonetic element, whose own <t> reads the base text
	// rather than adding to it. See cellPhoneticProp.
	var phoneticDepth int

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
			if phoneticDepth > 0 {
				phoneticDepth++
				continue
			}
			switch t.Name.Local {
			case "si":
				inSI = true
				currentText.Reset()
			case "rPh", "phoneticPr":
				if inSI {
					phoneticDepth = 1
				}
			case "t":
				if inSI {
					inT = true
				}
			case "r":
				// Rich text run inside <si> — the <t> inside <r> contributes text
			}
		case xml.CharData:
			if inSI && inT && phoneticDepth == 0 {
				currentText.Write(t)
			}
		case xml.EndElement:
			if phoneticDepth > 0 {
				phoneticDepth--
				continue
			}
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
