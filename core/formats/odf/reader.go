package odf

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// ODF XML namespaces.
const (
	nsText         = "urn:oasis:names:tc:opendocument:xmlns:text:1.0"
	nsTable        = "urn:oasis:names:tc:opendocument:xmlns:table:1.0"
	nsOffice       = "urn:oasis:names:tc:opendocument:xmlns:office:1.0"
	nsPresentation = "urn:oasis:names:tc:opendocument:xmlns:presentation:1.0"
	nsStyle        = "urn:oasis:names:tc:opendocument:xmlns:style:1.0"
	nsXLink        = "http://www.w3.org/1999/xlink"
	nsMeta         = "urn:oasis:names:tc:opendocument:xmlns:meta:1.0"
	nsDC           = "http://purl.org/dc/elements/1.1/"
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
	nsStyle:        "style",
	nsXLink:        "xlink",
	"urn:oasis:names:tc:opendocument:xmlns:fo:1.0":              "fo",
	"urn:oasis:names:tc:opendocument:xmlns:drawing:1.0":         "draw",
	"urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0":  "svg",
	"urn:oasis:names:tc:opendocument:xmlns:chart:1.0":           "chart",
	"urn:oasis:names:tc:opendocument:xmlns:form:1.0":            "form",
	"urn:oasis:names:tc:opendocument:xmlns:script:1.0":          "script",
	"urn:oasis:names:tc:opendocument:xmlns:meta:1.0":            "meta",
	"urn:oasis:names:tc:opendocument:xmlns:datastyle:1.0":       "number",
	"urn:oasis:names:tc:opendocument:xmlns:animation:1.0":       "anim",
	"urn:oasis:names:tc:opendocument:xmlns:database:1.0":        "db",
	"urn:oasis:names:tc:opendocument:xmlns:smil-compatible:1.0": "smil",
	"urn:oasis:names:tc:opendocument:xmlns:dr3d:1.0":            "dr3d",
	"urn:oasis:names:tc:opendocument:xmlns:config:1.0":          "config",
	"urn:oasis:names:tc:opendocument:xmlns:manifest:1.0":        "manifest",
	"http://purl.org/dc/elements/1.1/":                          "dc",
	"http://www.w3.org/XML/1998/namespace":                      "xml",
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
		return errors.New("odf: nil document or reader")
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
	odfTypeUnknown      odfDocType = iota
	odfTypeText                    // ODT
	odfTypeSpreadsheet             // ODS
	odfTypePresentation            // ODP
)

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Write content to a temp file (ZIP requires random access)
	tmpFile, err := os.CreateTemp("", "neokapi-odf-*.zip")
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

	// Process meta.xml (carries document metadata: dc:title, dc:description,
	// dc:subject, meta:keyword, meta:user-defined — see upstream Okapi
	// ODFFilter.java:127-130). Matches OpenDocument 1.2 §4.3 (document
	// metadata).
	metaXML := zipFileByName(zr, "meta.xml")
	if metaXML != nil {
		metaData, err := readZipFile(metaXML)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("odf: reading meta.xml: %w", err)}
			tmpFile.Close()
			return
		}
		r.skelPartStart("meta.xml")
		r.parseODFContent(ctx, ch, metaData, docType, &blockCounter, "meta.xml")
		r.skelPartEnd("meta.xml")
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
	var b *runBuilder
	var idCounter int
	inTranslatable := false
	var translatableDepth int
	// For skeleton: buffer the start element of a translatable block
	var translatableStart xml.StartElement
	// inlineIDStack records the PcOpen id for each currently-open
	// generic inline element so the matching EndElement can emit a
	// PcClose with the same id. Special-cased elements (text:line-break,
	// text:tab, text:s, and the translatable wrapper itself) push a 0
	// to keep depths aligned without emitting a code.
	var inlineIDStack []int

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
				b = newRunBuilder()
				idCounter = 0
				inlineIDStack = inlineIDStack[:0]
				translatableStart = t.Copy()
			} else if inTranslatable {
				switch {
				case isProtectedInlineElement(t.Name):
					// Capture the entire subtree (open + inner content
					// + close) verbatim as a single PLACEHOLDER code so
					// the inner text is NOT pseudo-translated. Mirrors
					// upstream Okapi's opaque-element handling for
					// metadata + auto-generated reference fields.
					sub, err := odfReadSubtreeMarkup(d, t)
					if err != nil {
						// Fall back to the generic inline path on
						// decode error so we don't drop content.
						idCounter++
						b.AddPcOpen(fmt.Sprintf("s%d", idCounter), inlineSpanTypeFor(t.Name), odfBuildStartTagMarkup(t))
						inlineIDStack = append(inlineIDStack, idCounter)
						break
					}
					idCounter++
					b.AddPh(fmt.Sprintf("s%d", idCounter), "x-"+t.Name.Local, sub)
					// elementStack was pushed for this StartElement;
					// the subtree consume read past the matching
					// EndElement, so pop the entry here.
					elementStack = elementStack[:len(elementStack)-1]
				case t.Name.Space == nsText && t.Name.Local == "line-break":
					// Upstream Okapi emits a PLACEHOLDER code carrying the
					// original `<text:line-break/>` markup (ODFFilter.java:619-622).
					// Preserve the same shape so the writer splices the
					// original self-closing element back into the output.
					idCounter++
					b.AddPh(fmt.Sprintf("s%d", idCounter), "lb", odfBuildEmptyTagMarkup(t))
					inlineIDStack = append(inlineIDStack, 0)
				case t.Name.Space == nsText && t.Name.Local == "tab":
					// Upstream Okapi extracts the tab as a literal "\t"
					// character (ODFFilter.java:615-618), not a code.
					b.AddText("\t")
					inlineIDStack = append(inlineIDStack, 0)
				case t.Name.Space == nsText && t.Name.Local == "s":
					// text:s = space(s); upstream extracts as literal spaces
					// (ODFFilter.java:604-613).
					count := 1
					for _, a := range t.Attr {
						if a.Name.Local == "c" {
							_, _ = fmt.Sscanf(a.Value, "%d", &count)
						}
					}
					b.AddText(strings.Repeat(" ", count))
					inlineIDStack = append(inlineIDStack, 0)
				default:
					// Generic inline element: preserve its full opening
					// markup (element + attributes) as PcOpen.Data so the
					// writer can splice the original tag back around the
					// translated inner runs. Mirrors upstream Okapi
					// ODFFilter.processStartElement falling through to
					// `tf.append(TagType.OPENING, name, buildStartTag(...))`
					// (ODFFilter.java:636-644) for any element that's not
					// in toExtract/toProtect/subFlow.
					idCounter++
					data := odfBuildStartTagMarkup(t)
					spanType := inlineSpanTypeFor(t.Name)
					b.AddPcOpen(fmt.Sprintf("s%d", idCounter), spanType, data)
					inlineIDStack = append(inlineIDStack, idCounter)
				}
			} else {
				r.emitElementWithAttrExtraction(ctx, ch, p, t, docType, blockCounter, partPath)
			}

		case xml.CharData:
			if inTranslatable {
				b.AddText(string(t))
			} else {
				p.skelText(odfXMLEscape(string(t)))
			}

		case xml.EndElement:
			if inTranslatable {
				if len(elementStack) == translatableDepth {
					// End of translatable element. Use the trimmed plain
					// text only for the emptiness check; preserve the
					// untrimmed content in the emitted block so leading
					// or trailing whitespace inside the element round-
					// trips byte-for-byte (upstream Okapi keeps it).
					raw := b.PlainText()
					runs := b.Runs()
					hasContent := strings.TrimSpace(raw) != "" || hasInlineCodeRuns(runs)
					if hasContent {
						*blockCounter++
						blockID := fmt.Sprintf("tu%d", *blockCounter)

						// Skeleton: write element open, ref, element close
						p.skelWriteStartElement(translatableStart)
						p.skelRef(blockID)
						p.skelWriteEndElement(t)

						var block *model.Block
						if hasInlineCodeRuns(runs) {
							block = &model.Block{
								ID:           blockID,
								Translatable: true,
								Source:       []*model.Segment{model.NewRunsSegment("s1", runs)},
								Targets:      make(map[model.LocaleID][]*model.Segment),
								Properties: map[string]string{
									"partPath": partPath,
									"element":  t.Name.Local,
								},
								Annotations: make(map[string]model.Annotation),
							}
						} else {
							block = model.NewBlock(blockID, raw)
							block.Properties["partPath"] = partPath
							block.Properties["element"] = t.Name.Local
						}

						r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
					} else {
						// Empty translatable element — pass through to skeleton
						p.skelWriteStartElement(translatableStart)
						p.skelText(odfXMLEscape(raw))
						p.skelWriteEndElement(t)
					}
					inTranslatable = false
					inlineIDStack = inlineIDStack[:0]
				} else if len(inlineIDStack) > 0 {
					// Pop the matching inline id; emit a PcClose iff the
					// open emitted a PcOpen (id > 0).
					id := inlineIDStack[len(inlineIDStack)-1]
					inlineIDStack = inlineIDStack[:len(inlineIDStack)-1]
					if id > 0 {
						data := odfBuildEndTagMarkup(t)
						spanType := inlineSpanTypeFor(t.Name)
						b.AddPcCloseData(fmt.Sprintf("s%d", id), spanType, data)
					}
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

// odfBuildStartTagMarkup serialises an xml.StartElement back to its
// `<prefix:name attr="val" ...>` form for use as inline-code Data.
// Mirrors upstream Okapi ODFFilter.buildStartTag (ODFFilter.java:431-489)
// — the captured outer markup is what gets spliced back into the
// reconstructed XML around the translated inner text.
func odfBuildStartTagMarkup(t xml.StartElement) string {
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
	return buf.String()
}

// odfBuildEndTagMarkup serialises an xml.EndElement back to `</prefix:name>`.
func odfBuildEndTagMarkup(t xml.EndElement) string {
	var buf strings.Builder
	buf.WriteString("</")
	odfWriteElementName(&buf, t.Name)
	buf.WriteString(">")
	return buf.String()
}

// odfReadSubtreeMarkup consumes XML tokens from dec starting JUST
// AFTER the given xml.StartElement and through its matching
// xml.EndElement, returning the verbatim serialised subtree (including
// the open and close tags). Used for "protected" elements whose entire
// content must round-trip without extraction (annotation metadata,
// auto-generated reference fields). The decoder is left positioned
// after the consumed EndElement so the caller's outer loop continues
// past the subtree cleanly.
func odfReadSubtreeMarkup(dec *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	buf.WriteString(odfBuildStartTagMarkup(start))
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			buf.WriteString(odfBuildStartTagMarkup(tt))
			depth++
		case xml.EndElement:
			buf.WriteString(odfBuildEndTagMarkup(tt))
			depth--
		case xml.CharData:
			buf.WriteString(odfXMLEscape(string(tt)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write([]byte(tt))
			buf.WriteString("-->")
		case xml.ProcInst:
			buf.WriteString("<?")
			buf.WriteString(tt.Target)
			if len(tt.Inst) > 0 {
				buf.WriteString(" ")
				buf.Write(tt.Inst)
			}
			buf.WriteString("?>")
		}
	}
	return buf.String(), nil
}

// odfBuildEmptyTagMarkup serialises an xml.StartElement back to a
// self-closing `<prefix:name attr="val"/>` form for use as a placeholder
// inline-code Data. Mirrors upstream Okapi's `<text:line-break/>`
// emission for self-closing inline elements.
func odfBuildEmptyTagMarkup(t xml.StartElement) string {
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
	buf.WriteString("/>")
	return buf.String()
}

// inlineSpanTypeFor returns the semantic type for an inline element.
// Known formatting elements (text:span, text:a) keep their named types
// for downstream tools that branch on them; all other elements get an
// `x-<localName>` generic type so the original markup round-trips
// without losing identity.
func inlineSpanTypeFor(name xml.Name) string {
	switch {
	case name.Space == nsText && name.Local == "span":
		return TypeBold
	case name.Space == nsText && name.Local == "a":
		return TypeHyperlink
	}
	return "x-" + name.Local
}

// emitElementWithAttrExtraction writes a start element to the skeleton,
// extracting translatable attribute values into Blocks. This mirrors
// upstream Okapi ODFFilter's attrbutesToExtract behaviour (see
// ODFFilter.java:133-136 and ODFFilter.java:454-474): the attributes
// style:num-prefix, style:num-suffix, and (for spreadsheets) table:name
// hold display text wrapping list-level numbering / sheet names and
// should be pseudo-translated. Matches OpenDocument 1.2 §19.711 /
// §19.812 / §19.731.
func (r *Reader) emitElementWithAttrExtraction(ctx context.Context, ch chan<- model.PartResult,
	p *odfParser, t xml.StartElement, docType odfDocType, blockCounter *int, partPath string) {

	odfRegisterNamespaces(t.Attr)

	// Buffer "<elementName" into the skeleton text buffer.
	var head strings.Builder
	head.WriteString("<")
	odfWriteElementName(&head, t.Name)
	p.skelText(head.String())

	for _, a := range t.Attr {
		if isTranslatableAttribute(a.Name, docType) && hasTrueText(a.Value) {
			// Emit a Block whose source text is the attribute value.
			*blockCounter++
			blockID := fmt.Sprintf("tu%d", *blockCounter)
			block := model.NewBlock(blockID, a.Value)
			block.Properties["partPath"] = partPath
			block.Properties["element"] = t.Name.Local
			block.Properties["attribute"] = a.Name.Local
			if a.Name.Space != "" {
				block.Properties["attributeNS"] = a.Name.Space
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})

			// Write ` name="` then a ref to the block, then `"`.
			var pre strings.Builder
			pre.WriteString(" ")
			odfWriteAttrName(&pre, a.Name)
			pre.WriteString(`="`)
			p.skelText(pre.String())
			p.skelRef(blockID)
			p.skelText(`"`)
			continue
		}

		// Non-translatable: write literally.
		var attr strings.Builder
		attr.WriteString(" ")
		odfWriteAttrName(&attr, a.Name)
		attr.WriteString(`="`)
		attr.WriteString(odfXMLEscapeAttr(a.Value))
		attr.WriteString(`"`)
		p.skelText(attr.String())
	}

	p.skelText(">")
}

// isTranslatableAttribute reports whether an attribute should be
// extracted as a translatable string. Mirrors upstream ODFFilter's
// attrbutesToExtract map (initialised in ODFFilter.java:133-136):
//   - style:num-prefix and style:num-suffix on every document type
//     (list-level numbering wrappers, e.g. "Text before>" / "<Text after")
//   - table:name only on spreadsheets (sheet display name)
func isTranslatableAttribute(name xml.Name, docType odfDocType) bool {
	switch name.Space {
	case nsStyle:
		switch name.Local {
		case "num-prefix", "num-suffix":
			return true
		}
	case nsTable:
		if name.Local == "name" {
			return docType == odfTypeSpreadsheet
		}
	}
	return false
}

// hasTrueText returns true if the string contains at least one letter
// character. Mirrors upstream ODFFilter.hasTrueText (ODFFilter.java:491):
// purely punctuation/whitespace/digit values (such as the num-suffix="."
// found on most list levels) aren't worth extracting as translatable.
func hasTrueText(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
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

// isTranslatableElement returns true if the XML element can contain
// translatable text. Mirrors upstream Okapi ODFFilter.toExtract
// (ODFFilter.java:124-131): paragraph/heading shells in content.xml +
// styles.xml, plus the document-metadata elements in meta.xml
// (dc:title, dc:description, dc:subject, meta:keyword, meta:user-defined).
func isTranslatableElement(name xml.Name) bool {
	switch name.Space {
	case nsText:
		switch name.Local {
		case "p", "h", "index-title-template":
			return true
		}
	case nsDC:
		switch name.Local {
		case "title", "description", "subject":
			return true
		}
	case nsMeta:
		switch name.Local {
		case "keyword", "user-defined":
			return true
		}
	}
	return false
}

// isProtectedInlineElement returns true for elements whose ENTIRE
// subtree (open tag + inner content + close tag) should be preserved
// verbatim as opaque inline markup — the inner text is NOT extracted
// for translation. Mirrors upstream Okapi ODFFilter.toProtect / opaque-
// content handling (ODFFilter.java:148-156 + buildOpaqueElement
// fallback). Without this, native walks into the inner CharData and
// pseudo-translates it, while okapi leaves it verbatim. Examples:
//
//   - <office:annotation>/<dc:creator>, <dc:date>: comment author +
//     timestamp metadata, not authored text.
//   - <text:bookmark-ref>, <text:reference-ref>, <text:sequence-ref>,
//     <text:note-ref>, <text:bibliography-mark>: auto-generated
//     reference text that mirrors a target elsewhere in the document.
//   - <text:page-number>, <text:page-count>: presentation/header-footer
//     placeholders whose inner text ("<number>") is a literal sentinel.
func isProtectedInlineElement(name xml.Name) bool {
	switch name.Space {
	case nsDC:
		switch name.Local {
		case "creator", "date":
			return true
		}
	case nsText:
		switch name.Local {
		case "bookmark-ref", "reference-ref", "sequence-ref", "note-ref":
			return true
		case "page-number", "page-count":
			return true
		case
			// text:* informational field elements that render values
			// from document metadata / system state, plus auto-
			// generated reference fields. Inner CharData is the cached
			// preview of the rendered value (read-only on round-trip),
			// not authored text. Mirrors upstream Okapi
			// ODFFilter.toProtect verbatim (ODFFilter.java:143-178).
			"initial-creator", "creation-date", "creation-time",
			"description", "user-defined",
			"print-time", "print-date", "printed-by",
			"editing-cycles", "editing-duration",
			"modification-time", "modification-date",
			"creator",
			"paragraph-count", "word-count", "character-count",
			"table-count", "image-count", "object-count",
			"note-citation",
			"tracked-changes",
			"title", "subject", "keywords":
			return true
		}
	}
	return false
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
