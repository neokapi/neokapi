package odf

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// ODF XML namespaces.
const (
	nsText         = "urn:oasis:names:tc:opendocument:xmlns:text:1.0"
	nsTable        = "urn:oasis:names:tc:opendocument:xmlns:table:1.0"
	nsOffice       = "urn:oasis:names:tc:opendocument:xmlns:office:1.0"
	nsPresentation = "urn:oasis:names:tc:opendocument:xmlns:presentation:1.0"
	nsXLink        = "http://www.w3.org/1999/xlink"
)

// Span type constants for inline formatting.
const (
	TypeBold          = "bold"
	TypeItalic        = "italic"
	TypeUnderline     = "underline"
	TypeStrikethrough = "strikethrough"
	TypeHyperlink     = "hyperlink"
)

// Skeleton part-boundary markers. The writer uses these to split the
// single skeleton stream into per-ZIP-entry segments.
const (
	skelPartStartPrefix = "@@ODF_SKEL_PART_START@@"
	skelPartEndPrefix   = "@@ODF_SKEL_PART_END@@"
)

// ODF namespace prefix map for skeleton serialization.
var odfNSPrefixMap = map[string]string{
	nsText:         "text",
	nsTable:        "table",
	nsOffice:       "office",
	nsPresentation: "presentation",
	nsXLink:        "xlink",
	"urn:oasis:names:tc:opendocument:xmlns:style:1.0":            "style",
	"urn:oasis:names:tc:opendocument:xmlns:fo:1.0":               "fo",
	"urn:oasis:names:tc:opendocument:xmlns:drawing:1.0":          "draw",
	"urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0":   "svg",
	"urn:oasis:names:tc:opendocument:xmlns:chart:1.0":            "chart",
	"urn:oasis:names:tc:opendocument:xmlns:form:1.0":             "form",
	"urn:oasis:names:tc:opendocument:xmlns:script:1.0":           "script",
	"urn:oasis:names:tc:opendocument:xmlns:meta:1.0":             "meta",
	"urn:oasis:names:tc:opendocument:xmlns:datastyle:1.0":        "number",
	"urn:oasis:names:tc:opendocument:xmlns:animation:1.0":        "anim",
	"urn:oasis:names:tc:opendocument:xmlns:database:1.0":         "db",
	"urn:oasis:names:tc:opendocument:xmlns:smil-compatible:1.0":  "smil",
	"urn:oasis:names:tc:opendocument:xmlns:dr3d:1.0":             "dr3d",
	"urn:oasis:names:tc:opendocument:xmlns:config:1.0":           "config",
	"urn:oasis:names:tc:opendocument:xmlns:manifest:1.0":         "manifest",
	"http://purl.org/dc/elements/1.1/":                           "dc",
	"http://www.w3.org/XML/1998/namespace":                        "xml",
}

// odfNSRegistry tracks dynamic namespace URI -> prefix mappings from the document.
var odfNSRegistry = struct {
	m map[string]string
}{m: make(map[string]string)}

func odfRegisterNamespaces(attrs []xml.Attr) {
	for _, a := range attrs {
		if a.Name.Space == "xmlns" {
			odfNSRegistry.m[a.Value] = a.Name.Local
		} else if a.Name.Space == "" && a.Name.Local == "xmlns" {
			odfNSRegistry.m[a.Value] = ""
		}
	}
}

func odfResolvePrefix(ns string) string {
	if p, ok := odfNSRegistry.m[ns]; ok {
		return p
	}
	if p, ok := odfNSPrefixMap[ns]; ok {
		return p
	}
	return ""
}

// Reader implements DataFormatReader for ODF files (ODT, ODS, ODP).
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	resolver      format.SubfilterResolver
	skeletonStore *format.SkeletonStore
	tmpFile       string // path to temp file for ZIP access
	layerSeq      int    // counter for generating unique child layer IDs
}

var _ format.SkeletonStoreEmitter = (*Reader)(nil)
var _ format.SubfilterAware = (*Reader)(nil)

// NewReader creates a new ODF reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "odf",
			FormatDisplayName: "Open Document Format",
			FormatMimeType:    "application/vnd.oasis.opendocument.text",
			FormatExtensions:  []string{".odt", ".ods", ".odp"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format readers.
func (r *Reader) SetSubfilterResolver(resolver format.SubfilterResolver) {
	r.resolver = resolver
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes: []string{
			"application/vnd.oasis.opendocument.text",
			"application/vnd.oasis.opendocument.spreadsheet",
			"application/vnd.oasis.opendocument.presentation",
		},
		Extensions: []string{".odt", ".ods", ".odp", ".odg", ".odf"},
		MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}}, // PK ZIP header
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("odf: nil document or reader")
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

// odfDocType identifies the ODF document subtype.
type odfDocType int

const (
	odfTypeUnknown odfDocType = iota
	odfTypeText               // ODT
	odfTypeSpreadsheet        // ODS
	odfTypePresentation       // ODP
)

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Write content to a temp file (ZIP requires random access)
	tmpFile, err := os.CreateTemp("", "gokapi-odf-*.zip")
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("odf: creating temp file: %w", err)}
		return
	}
	r.tmpFile = tmpFile.Name()

	if _, err := io.Copy(tmpFile, r.Doc.Reader); err != nil {
		tmpFile.Close()
		ch <- model.PartResult{Error: fmt.Errorf("odf: writing temp file: %w", err)}
		return
	}

	// Get size and rewind
	size, err := tmpFile.Seek(0, io.SeekEnd)
	if err != nil {
		tmpFile.Close()
		ch <- model.PartResult{Error: fmt.Errorf("odf: seeking temp file: %w", err)}
		return
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		tmpFile.Close()
		ch <- model.PartResult{Error: fmt.Errorf("odf: seeking temp file: %w", err)}
		return
	}

	// Open as ZIP from temp file
	zr, err := zip.NewReader(tmpFile, size)
	if err != nil {
		tmpFile.Close()
		ch <- model.PartResult{Error: fmt.Errorf("odf: not a valid ZIP archive: %w", err)}
		return
	}

	// Detect document type from mimetype file
	docType := detectODFType(zr)

	// Emit root layer
	rootLayer := &model.Layer{
		ID:         "doc1",
		Name:       r.Doc.URI,
		Format:     "odf",
		Locale:     locale,
		Encoding:   "UTF-8",
		MimeType:   r.Doc.MimeType,
		Properties: map[string]string{"docType": docTypeString(docType)},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: rootLayer}) {
		tmpFile.Close()
		return
	}

	blockCounter := 0

	// Process content.xml (always present)
	contentXML := zipFileByName(zr, "content.xml")
	if contentXML != nil {
		contentData, err := readZipFile(contentXML)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("odf: reading content.xml: %w", err)}
			tmpFile.Close()
			return
		}

		if r.resolver != nil {
			// Route through XML sub-format reader
			r.layerSeq++
			childLayerID := fmt.Sprintf("sf%d", r.layerSeq)
			childLayer := &model.Layer{
				ID:       childLayerID,
				Name:     "content.xml",
				Format:   "xml",
				Locale:   locale,
				ParentID: rootLayer.ID,
				Properties: map[string]string{
					"subfilter.source": "odf",
					"entry":            "content.xml",
				},
			}
			r.skelPartStart("content.xml")
			if r.skeletonStore != nil {
				_ = r.skeletonStore.WriteRef("layer:content.xml")
			}
			r.skelPartEnd("content.xml")
			r.emitSubfiltered(ctx, ch, contentData, "content.xml", rootLayer.ID, childLayer, &blockCounter)
		} else {
			r.skelPartStart("content.xml")
			r.parseODFContent(ctx, ch, contentData, docType, &blockCounter, "content.xml")
			r.skelPartEnd("content.xml")
		}
	}

	// Process styles.xml (may contain translatable master page content)
	stylesXML := zipFileByName(zr, "styles.xml")
	if stylesXML != nil {
		stylesData, err := readZipFile(stylesXML)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("odf: reading styles.xml: %w", err)}
			tmpFile.Close()
			return
		}

		if r.resolver != nil {
			// Route through XML sub-format reader
			r.layerSeq++
			childLayerID := fmt.Sprintf("sf%d", r.layerSeq)
			childLayer := &model.Layer{
				ID:       childLayerID,
				Name:     "styles.xml",
				Format:   "xml",
				Locale:   locale,
				ParentID: rootLayer.ID,
				Properties: map[string]string{
					"subfilter.source": "odf",
					"entry":            "styles.xml",
				},
			}
			r.skelPartStart("styles.xml")
			if r.skeletonStore != nil {
				_ = r.skeletonStore.WriteRef("layer:styles.xml")
			}
			r.skelPartEnd("styles.xml")
			r.emitSubfiltered(ctx, ch, stylesData, "styles.xml", rootLayer.ID, childLayer, &blockCounter)
		} else {
			r.skelPartStart("styles.xml")
			r.parseODFContent(ctx, ch, stylesData, docType, &blockCounter, "styles.xml")
			r.skelPartEnd("styles.xml")
		}
	}

	// End root layer
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
	tmpFile.Close()
}

// odfParser handles skeleton state during ODF XML parsing.
type odfParser struct {
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
}

func (p *odfParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *odfParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		_ = p.skeletonStore.WriteRef(id)
	}
}

func (p *odfParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

func (p *odfParser) skelWriteStartElement(t xml.StartElement) {
	if p.skeletonStore == nil {
		return
	}
	odfRegisterNamespaces(t.Attr)
	var buf strings.Builder
	buf.WriteString("<")
	odfWriteElementName(&buf, t.Name)
	for _, a := range t.Attr {
		buf.WriteString(" ")
		odfWriteAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(odfXMLEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *odfParser) skelWriteEndElement(t xml.EndElement) {
	if p.skeletonStore == nil {
		return
	}
	var buf strings.Builder
	buf.WriteString("</")
	odfWriteElementName(&buf, t.Name)
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

// parseODFContent parses an ODF XML file (content.xml or styles.xml) and emits blocks.
func (r *Reader) parseODFContent(ctx context.Context, ch chan<- model.PartResult,
	data []byte, docType odfDocType, blockCounter *int, partPath string) {

	p := &odfParser{skeletonStore: r.skeletonStore}
	d := xml.NewDecoder(bytes.NewReader(data))

	// Track nesting for context
	var elementStack []xml.Name
	var textBuf strings.Builder
	var spans []*model.Span
	inTranslatable := false
	var translatableDepth int
	// For skeleton: buffer the start element of a translatable block
	var translatableStart xml.StartElement

	for {
		tok, err := d.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			elementStack = append(elementStack, t.Name)

			if isTranslatableElement(t.Name) && !inTranslatable {
				inTranslatable = true
				translatableDepth = len(elementStack)
				textBuf.Reset()
				spans = nil
				translatableStart = t.Copy()
			} else if inTranslatable {
				// Handle inline formatting elements
				if isInlineFormattingElement(t.Name) {
					spanType := inlineSpanType(t.Name)
					if spanType != "" {
						spans = append(spans, &model.Span{
							ID:   fmt.Sprintf("s%d", len(spans)+1),
							Type: spanType,
						})
						textBuf.WriteRune(model.MarkerOpening)
					}
				} else if t.Name.Space == nsText && t.Name.Local == "a" {
					// Hyperlink — the text inside is still translatable
					spans = append(spans, &model.Span{
						ID:   fmt.Sprintf("s%d", len(spans)+1),
						Type: TypeHyperlink,
						Data: getAttr(t, nsXLink, "href"),
					})
					textBuf.WriteRune(model.MarkerOpening)
				} else if t.Name.Space == nsText && t.Name.Local == "line-break" {
					textBuf.WriteString("\n")
				} else if t.Name.Space == nsText && t.Name.Local == "tab" {
					textBuf.WriteString("\t")
				} else if t.Name.Space == nsText && t.Name.Local == "s" {
					// text:s = space(s)
					count := 1
					for _, a := range t.Attr {
						if a.Name.Local == "c" {
							_, _ = fmt.Sscanf(a.Value, "%d", &count)
						}
					}
					for i := 0; i < count; i++ {
						textBuf.WriteRune(' ')
					}
				}
			} else {
				p.skelWriteStartElement(t)
			}

		case xml.CharData:
			if inTranslatable {
				textBuf.Write(t)
			} else {
				p.skelText(odfXMLEscape(string(t)))
			}

		case xml.EndElement:
			if inTranslatable {
				if isInlineFormattingElement(t.Name) {
					spanType := inlineSpanType(t.Name)
					if spanType != "" {
						spans = append(spans, &model.Span{
							ID:   fmt.Sprintf("s%d", len(spans)+1),
							Type: spanType,
						})
						textBuf.WriteRune(model.MarkerClosing)
					}
				} else if t.Name.Space == nsText && t.Name.Local == "a" {
					spans = append(spans, &model.Span{
						ID:   fmt.Sprintf("s%d", len(spans)+1),
						Type: TypeHyperlink,
					})
					textBuf.WriteRune(model.MarkerClosing)
				}

				if len(elementStack) == translatableDepth {
					// End of translatable element
					text := strings.TrimSpace(textBuf.String())
					if text != "" {
						*blockCounter++
						blockID := fmt.Sprintf("tu%d", *blockCounter)

						// Skeleton: write element open, ref, element close
						p.skelWriteStartElement(translatableStart)
						p.skelRef(blockID)
						p.skelWriteEndElement(t)

						var block *model.Block
						if len(spans) > 0 {
							frag := &model.Fragment{
								CodedText: textBuf.String(),
								Spans:     spans,
							}
							block = &model.Block{
								ID:           blockID,
								Translatable: true,
								Source:       []*model.Segment{{ID: "s1", Content: frag}},
								Targets:      make(map[model.LocaleID][]*model.Segment),
								Properties: map[string]string{
									"partPath": partPath,
									"element":  t.Name.Local,
								},
								Annotations: make(map[string]model.Annotation),
							}
						} else {
							block = model.NewBlock(blockID, text)
							block.Properties["partPath"] = partPath
							block.Properties["element"] = t.Name.Local
						}

						r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
					} else {
						// Empty translatable element — pass through to skeleton
						p.skelWriteStartElement(translatableStart)
						p.skelText(odfXMLEscape(textBuf.String()))
						p.skelWriteEndElement(t)
					}
					inTranslatable = false
				}
			} else {
				p.skelWriteEndElement(t)
			}

			if len(elementStack) > 0 {
				elementStack = elementStack[:len(elementStack)-1]
			}

		case xml.ProcInst:
			if !inTranslatable {
				p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")
			}

		case xml.Comment:
			if !inTranslatable {
				p.skelText("<!--" + string(t) + "-->")
			}

		case xml.Directive:
			if !inTranslatable {
				p.skelText("<!" + string(t) + ">")
			}
		}
	}

	p.skelFlush()
}

// emitSubfiltered emits a child layer with content parsed by the XML sub-format reader.
func (r *Reader) emitSubfiltered(ctx context.Context, ch chan<- model.PartResult,
	content []byte, entryName, parentLayerID string,
	childLayer *model.Layer, blockCounter *int) {

	subReader, err := r.resolver.ResolveReader("xml")
	if err != nil {
		// Fall back to direct ODF parsing if XML reader is unavailable
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
			return
		}
		r.parseODFContent(ctx, ch, content, odfTypeUnknown, blockCounter, entryName)
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
		return
	}

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Emit child layer start
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
		return
	}

	// Open sub-reader and emit its parts
	subDoc := &model.RawDocument{
		URI:          entryName,
		SourceLocale: locale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := subReader.Open(ctx, subDoc); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("odf: subfilter open for %s: %w", entryName, err)}
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
		return
	}

	// Read sub-reader parts, skipping the sub-reader's own root layer start/end
	for pr := range subReader.Read(ctx) {
		if pr.Error != nil {
			ch <- model.PartResult{Error: fmt.Errorf("odf: subfilter read for %s: %w", entryName, pr.Error)}
			break
		}
		if pr.Part.Type == model.PartLayerStart || pr.Part.Type == model.PartLayerEnd {
			if layer, ok := pr.Part.Resource.(*model.Layer); ok && layer.IsRoot() {
				continue
			}
		}
		r.emit(ctx, ch, pr.Part)
	}
	subReader.Close()

	// Emit child layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
}

func (r *Reader) skelPartStart(partPath string) {
	if r.skeletonStore != nil {
		_ = r.skeletonStore.WriteRef(skelPartStartPrefix + partPath)
	}
}

func (r *Reader) skelPartEnd(partPath string) {
	if r.skeletonStore != nil {
		_ = r.skeletonStore.WriteRef(skelPartEndPrefix + partPath)
	}
}

// isTranslatableElement returns true if the XML element can contain translatable text.
func isTranslatableElement(name xml.Name) bool {
	switch name.Space {
	case nsText:
		switch name.Local {
		case "p", "h":
			return true
		}
	}
	return false
}

// isInlineFormattingElement returns true if the element represents inline formatting.
func isInlineFormattingElement(name xml.Name) bool {
	return name.Space == nsText && name.Local == "span"
}

// inlineSpanType returns the span type for an inline formatting element.
// For text:span, we return a generic type since ODF uses style references.
func inlineSpanType(name xml.Name) string {
	if name.Space == nsText && name.Local == "span" {
		return TypeBold // Default — actual style resolution would require parsing styles.xml
	}
	return ""
}

// detectODFType detects the ODF document type from the mimetype file.
func detectODFType(zr *zip.Reader) odfDocType {
	mf := zipFileByName(zr, "mimetype")
	if mf == nil {
		return odfTypeUnknown
	}
	data, err := readZipFile(mf)
	if err != nil {
		return odfTypeUnknown
	}
	mime := strings.TrimSpace(string(data))
	switch {
	case strings.Contains(mime, "text"):
		return odfTypeText
	case strings.Contains(mime, "spreadsheet"):
		return odfTypeSpreadsheet
	case strings.Contains(mime, "presentation"):
		return odfTypePresentation
	default:
		return odfTypeUnknown
	}
}

func docTypeString(dt odfDocType) string {
	switch dt {
	case odfTypeText:
		return "odt"
	case odfTypeSpreadsheet:
		return "ods"
	case odfTypePresentation:
		return "odp"
	default:
		return "unknown"
	}
}

// getAttr returns the value of an attribute with the given namespace and local name.
func getAttr(el xml.StartElement, space, local string) string {
	for _, a := range el.Attr {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value
		}
	}
	return ""
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
	if r.tmpFile != "" {
		os.Remove(r.tmpFile)
		r.tmpFile = ""
	}
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

// readZipFile reads the contents of a ZIP file entry.
func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
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

// XML serialization helpers for ODF skeleton.

func odfWriteElementName(buf *strings.Builder, name xml.Name) {
	if name.Space != "" {
		prefix := odfResolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
	}
	buf.WriteString(name.Local)
}

func odfWriteAttrName(buf *strings.Builder, name xml.Name) {
	if name.Space == "xmlns" {
		buf.WriteString("xmlns:")
		buf.WriteString(name.Local)
		return
	}
	if name.Space == "" && name.Local == "xmlns" {
		buf.WriteString("xmlns")
		return
	}
	if name.Space != "" {
		prefix := odfResolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
	}
	buf.WriteString(name.Local)
}

func odfXMLEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func odfXMLEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
