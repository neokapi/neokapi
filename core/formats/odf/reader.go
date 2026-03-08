package odf

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
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

// Reader implements DataFormatReader for ODF files (ODT, ODS, ODP).
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

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

	// Read all content into memory (ZIP requires random access)
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("odf: reading: %w", err)}
		return
	}

	// Open as ZIP
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
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
		return
	}

	blockCounter := 0

	// Process content.xml (always present)
	contentXML := zipFileByName(zr, "content.xml")
	if contentXML != nil {
		contentData, err := readZipFile(contentXML)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("odf: reading content.xml: %w", err)}
			return
		}
		r.parseODFContent(ctx, ch, contentData, docType, &blockCounter, "content.xml")
	}

	// Process styles.xml (may contain translatable master page content)
	stylesXML := zipFileByName(zr, "styles.xml")
	if stylesXML != nil {
		stylesData, err := readZipFile(stylesXML)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("odf: reading styles.xml: %w", err)}
			return
		}
		r.parseODFContent(ctx, ch, stylesData, docType, &blockCounter, "styles.xml")
	}

	// End root layer
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
}

// parseODFContent parses an ODF XML file (content.xml or styles.xml) and emits blocks.
func (r *Reader) parseODFContent(ctx context.Context, ch chan<- model.PartResult,
	data []byte, docType odfDocType, blockCounter *int, partPath string) {

	d := xml.NewDecoder(bytes.NewReader(data))

	// Track nesting for context
	var elementStack []xml.Name
	var textBuf strings.Builder
	var spans []*model.Span
	inTranslatable := false
	var translatableDepth int

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
							fmt.Sscanf(a.Value, "%d", &count)
						}
					}
					for i := 0; i < count; i++ {
						textBuf.WriteRune(' ')
					}
				}
			}

		case xml.CharData:
			if inTranslatable {
				textBuf.Write(t)
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
					}
					inTranslatable = false
				}
			}

			if len(elementStack) > 0 {
				elementStack = elementStack[:len(elementStack)-1]
			}
		}
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
