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
	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
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
	nsForm         = "urn:oasis:names:tc:opendocument:xmlns:form:1.0"
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

// Reader implements DataFormatReader for ODF files (ODT, ODS, ODP).
//
// The package's XML streams — content.xml, styles.xml, meta.xml — are parsed
// here, by parseODFContent, and are never handed to the generic XML reader.
// They are ODF, not arbitrary XML: which elements hold translatable text
// (text:p, text:h, dc:title, …), which are opaque, which carry geometry, and
// what this reader's own Config says about notes, hidden content and
// non-translatable content are all ODF rules that a generic XML reader knows
// nothing about. Upstream Okapi draws the same line — OpenOfficeFilter unpacks
// the ZIP and dispatches every inner stream to its own ODFFilter (okf_odf),
// never to okf_xml.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	tmpFile       string // path to temp file for ZIP access
}

var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new ODF reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		FormatName:        "odf",
		FormatDisplayName: "Open Document Format",
		FormatMimeType:    "application/vnd.oasis.opendocument.text",
		FormatExtensions:  []string{".odt", ".ods", ".odp"},
		Cfg:               cfg,
		cfg:               cfg,
	}
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

// openArchive opens the document as a ZIP, ready for random-access entry
// reads. When the caller gave us random access (a file handle) the container is
// read in place — no copy at all. Otherwise the bytes are spilled to a temp
// file rather than buffered: a ZIP needs random access either way, and disk is
// the cheaper place to hold a package whose bulk is embedded media.
//
// The returned func releases whatever was held and is always safe to call; the
// temp file itself is removed by Close.
func (r *Reader) openArchive() (*zip.Reader, func(), error) {
	noop := func() {}

	if ra, size, ok := r.Doc.RandomAccess(); ok {
		if err := safeio.DefaultBudget().CheckSize(size); err != nil {
			return nil, noop, fmt.Errorf("odf: %w", err)
		}
		zr, err := zip.NewReader(ra, size)
		if err != nil {
			return nil, noop, fmt.Errorf("odf: not a valid ZIP archive: %w", err)
		}
		if err := safeio.DefaultZipLimits.CheckReader(zr); err != nil {
			return nil, noop, fmt.Errorf("odf: %w", err)
		}
		return zr, noop, nil
	}

	tmpFile, err := os.CreateTemp("", "neokapi-odf-*.zip")
	if err != nil {
		return nil, noop, fmt.Errorf("odf: creating temp file: %w", err)
	}
	r.tmpFile = tmpFile.Name()
	closeTmp := func() { tmpFile.Close() }

	if _, err := io.Copy(tmpFile, safeio.DefaultBudget().Reader(r.Doc.Reader)); err != nil {
		closeTmp()
		return nil, noop, fmt.Errorf("odf: writing temp file: %w", err)
	}
	size, err := tmpFile.Seek(0, io.SeekEnd)
	if err != nil {
		closeTmp()
		return nil, noop, fmt.Errorf("odf: seeking temp file: %w", err)
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		closeTmp()
		return nil, noop, fmt.Errorf("odf: seeking temp file: %w", err)
	}
	zr, err := zip.NewReader(tmpFile, size)
	if err != nil {
		closeTmp()
		return nil, noop, fmt.Errorf("odf: not a valid ZIP archive: %w", err)
	}
	if err := safeio.DefaultZipLimits.CheckReader(zr); err != nil {
		closeTmp()
		return nil, noop, fmt.Errorf("odf: %w", err)
	}
	return zr, closeTmp, nil
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	zr, closeArchive, err := r.openArchive()
	if err != nil {
		ch <- model.PartResult{Error: err}
		return
	}
	defer closeArchive()

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

		r.skelPartStart("content.xml")
		r.parseODFContent(ctx, ch, contentData, docType, &blockCounter, "content.xml")
		r.skelPartEnd("content.xml")
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
			return
		}

		r.skelPartStart("styles.xml")
		r.parseODFContent(ctx, ch, stylesData, docType, &blockCounter, "styles.xml")
		r.skelPartEnd("styles.xml")
	}

	// End root layer
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
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
			p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		p.skeletonStore.WriteRef(id)
	}
}

// skelOriginal records the source bytes the next ref stands in for, paired
// with what that ref renders to while nothing has edited its block. Extraction
// resolves `<text:s text:c="4"/>` to four spaces and `<text:tab/>` to a tab —
// upstream Okapi's own reading (ODFFilter.java:604-618), and the reading the
// content model is worth having — and an XML processor folds a source CRLF
// inside the text to LF (XML 1.0 §2.11). Each rewrites bytes the document
// held, so the writer replays the original whenever the block is untouched:
// the extraction stays what it is, and an untranslated document still comes
// back byte for byte.
func (p *odfParser) skelOriginal(rendered, original string) {
	if p.skeletonStore == nil || rendered == original {
		return
	}
	p.skelFlush()
	p.skeletonStore.WriteOriginal([]byte(rendered), []byte(original))
}

func (p *odfParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

// skelRaw buffers a token's own source bytes into the skeleton.
//
// The skeleton carries what the file said rather than what a re-serialization
// of the parsed token would say, and the difference is the whole round-trip: a
// decoder reports `<office:scripts/>` and `<office:scripts></office:scripts>`
// identically, and hands back `'` for a source `&apos;`, so rebuilding a tag
// rewrites every document that passes through — attribute order, quote
// character, entity choice, empty-element form and intra-tag whitespace all
// settle on the rebuilder's preference instead of the author's.
func (p *odfParser) skelRaw(raw []byte) {
	if p.skeletonStore != nil {
		p.skelBuf.Write(raw)
	}
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
	// For skeleton: the source bytes of the start tag of a translatable block,
	// so the tag comes back as the file spelled it, and the offset its content
	// begins at, so the content can too.
	var translatableStartRaw []byte
	var translatableContentStart int
	// inlineIDStack records the PcOpen id for each currently-open
	// generic inline element so the matching EndElement can emit a
	// PcClose with the same id. Special-cased elements (text:line-break,
	// text:tab, text:s, and the translatable wrapper itself) push a 0
	// to keep depths aligned without emitting a code.
	var inlineIDStack []int
	// Intrinsic geometry (A3): the stack of enclosing draw:frame boxes and the
	// 1-based draw:page index (presentations). A block emitted inside a frame
	// inherits the innermost frame's box.
	var frameStack []frameBox
	pageNum := 0

	for {
		// InputOffset gives the end of the token just returned and the start of
		// the next, so [tokStart, tokEnd) is this token's own bytes. A
		// self-closing element yields an EndElement whose range is empty, which
		// is exactly what has to go back into the file after `<tag/>`.
		tokStart := int(d.InputOffset())
		tok, err := d.Token()
		if err != nil {
			break
		}
		tokRaw := data[tokStart:int(d.InputOffset())]

		switch t := tok.(type) {
		case xml.StartElement:
			elementStack = append(elementStack, t.Name)

			if t.Name.Space == nsDraw && t.Name.Local == "frame" {
				frameStack = append(frameStack, parseFrameBox(t))
			} else if t.Name.Space == nsDraw && t.Name.Local == "page" {
				pageNum++
			}

			if isTranslatableElement(t.Name) && !inTranslatable {
				inTranslatable = true
				translatableDepth = len(elementStack)
				b = newRunBuilder()
				idCounter = 0
				inlineIDStack = inlineIDStack[:0]
				translatableStartRaw = tokRaw
				translatableContentStart = tokStart + len(tokRaw)
			} else if inTranslatable {
				switch {
				case isProtectedInlineElement(t.Name):
					// Capture the entire subtree (open + inner content
					// + close) verbatim as a single PLACEHOLDER code so
					// the inner text is NOT pseudo-translated. Mirrors
					// upstream Okapi's opaque-element handling for
					// metadata + auto-generated reference fields.
					sub, err := odfReadSubtreeMarkup(d, data, tokStart)
					if err != nil {
						// Fall back to the generic inline path on
						// decode error so we don't drop content.
						idCounter++
						b.AddPcOpen(fmt.Sprintf("s%d", idCounter), inlineSpanTypeFor(t.Name), string(tokRaw))
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
					b.AddPh(fmt.Sprintf("s%d", idCounter), "lb", string(tokRaw))
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
					spanType := inlineSpanTypeFor(t.Name)
					b.AddPcOpen(fmt.Sprintf("s%d", idCounter), spanType, string(tokRaw))
					inlineIDStack = append(inlineIDStack, idCounter)
				}
			} else {
				r.emitElementWithAttrExtraction(ctx, ch, p, t, tokRaw, docType, blockCounter, partPath)
			}

		case xml.CharData:
			switch {
			case inTranslatable:
				b.AddText(string(t))
			case r.cfg.ExtractNonTranslatableContent() &&
				isAccessibilityTextParent(elementStack) &&
				strings.TrimSpace(string(t)) != "":
				// Image accessibility text (<svg:title>/<svg:desc> alt-text
				// and long descriptions on drawing frames, e.g. page-anchored
				// ODP/ODG frames). Surface it as a non-translatable RoleCaption
				// content block and ride the body via a skeleton ref so the
				// round-trip stays byte-exact. The enclosing <svg:title>/
				// <svg:desc> start tag was already written to skeleton by
				// emitElementWithAttrExtraction; the end tag follows it.
				elem := elementStack[len(elementStack)-1].Local
				r.emitAccessibilityContent(ctx, ch, p, string(t), elem, partPath, frameStack, pageNum, blockCounter)
			default:
				p.skelRaw(tokRaw)
			}

		case xml.EndElement:
			if inTranslatable {
				if len(elementStack) == translatableDepth {
					// End of translatable element. Use the trimmed plain
					// text only for the emptiness check; preserve the
					// untrimmed content in the emitted block so leading
					// or trailing whitespace inside the element round-
					// trips byte-for-byte (upstream Okapi keeps it).
					plain := b.PlainText()
					runs := b.Runs()
					hasContent := strings.TrimSpace(plain) != "" || hasInlineCodeRuns(runs)
					if hasContent {
						*blockCounter++
						blockID := fmt.Sprintf("tu%d", *blockCounter)

						// Skeleton: write element open, ref, element close
						p.skelRaw(translatableStartRaw)
						p.skelOriginal(renderSourceRunsForODF(runs, plain),
							string(data[translatableContentStart:tokStart]))
						p.skelRef(blockID)
						p.skelRaw(tokRaw)

						var block *model.Block
						if hasInlineCodeRuns(runs) {
							block = &model.Block{
								ID:           blockID,
								Translatable: true,
								Source:       runs,
								Targets:      make(map[model.VariantKey]*model.Target),
								Properties: map[string]string{
									"partPath": partPath,
									"element":  t.Name.Local,
								},
							}
						} else {
							block = model.NewBlock(blockID, plain)
							block.Properties["partPath"] = partPath
							block.Properties["element"] = t.Name.Local
						}

						applyFrameGeometry(block, frameStack, pageNum)
						r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
					} else {
						// Empty translatable element — pass through to skeleton
						p.skelRaw(translatableStartRaw)
						p.skelText(xmlesc.Text(plain))
						p.skelRaw(tokRaw)
					}
					inTranslatable = false
					inlineIDStack = inlineIDStack[:0]
				} else if len(inlineIDStack) > 0 {
					// Pop the matching inline id; emit a PcClose iff the
					// open emitted a PcOpen (id > 0).
					id := inlineIDStack[len(inlineIDStack)-1]
					inlineIDStack = inlineIDStack[:len(inlineIDStack)-1]
					if id > 0 {
						spanType := inlineSpanTypeFor(t.Name)
						b.AddPcCloseData(fmt.Sprintf("s%d", id), spanType, string(tokRaw))
					}
				}
			} else {
				p.skelRaw(tokRaw)
			}

			if t.Name.Space == nsDraw && t.Name.Local == "frame" && len(frameStack) > 0 {
				frameStack = frameStack[:len(frameStack)-1]
			}

			if len(elementStack) > 0 {
				elementStack = elementStack[:len(elementStack)-1]
			}

		case xml.ProcInst, xml.Comment, xml.Directive:
			if !inTranslatable {
				p.skelRaw(tokRaw)
			}
		}
	}

	p.skelFlush()
}

// odfReadSubtreeMarkup consumes XML tokens from dec starting JUST
// AFTER the given xml.StartElement and through its matching
// xml.EndElement, returning the verbatim serialised subtree (including
// the open and close tags). Used for "protected" elements whose entire
// content must round-trip without extraction (annotation metadata,
// auto-generated reference fields). The decoder is left positioned
// after the consumed EndElement so the caller's outer loop continues
// past the subtree cleanly.
func odfReadSubtreeMarkup(dec *xml.Decoder, src []byte, startOffset int) (string, error) {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return string(src[startOffset:int(dec.InputOffset())]), nil
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
// The tag's own bytes go to the skeleton and only the value of an extracted
// attribute is replaced by a ref, so every other attribute — its order, its
// quoting, its entities — comes back as the file wrote it.
func (r *Reader) emitElementWithAttrExtraction(ctx context.Context, ch chan<- model.PartResult,
	p *odfParser, t xml.StartElement, tagRaw []byte, docType odfDocType, blockCounter *int, partPath string) {

	values := odfTagAttrValueRanges(tagRaw)

	pos := 0
	for i, a := range t.Attr {
		translatable := isTranslatableAttribute(a.Name, docType) && hasTrueText(a.Value)
		// Form-control display strings (form:label / form:title /
		// form:help-text) are UI text that upstream Okapi's ODFFilter does NOT
		// extract. They are surfaced as NON-translatable content blocks
		// (visible to ingestion, skipped by MT) — gated behind
		// ExtractNonTranslatableContent — riding the attribute value via a
		// skeleton ref so the round-trip stays byte-exact. Parity forces the
		// flag off, leaving the value in the tag's own bytes.
		display := r.cfg.ExtractNonTranslatableContent() && isFormDisplayAttribute(a.Name) && hasTrueText(a.Value)
		if !translatable && !display {
			continue
		}
		// The scanner and the decoder disagree about how many attributes this
		// tag has only for a tag neither should have accepted; leaving the
		// value in skeleton keeps the document intact and costs the
		// substitution, which is the safe way round.
		if i >= len(values) {
			continue
		}

		*blockCounter++
		blockID := fmt.Sprintf("tu%d", *blockCounter)
		block := model.NewBlock(blockID, a.Value)
		block.Translatable = translatable
		block.Properties["partPath"] = partPath
		block.Properties["element"] = t.Name.Local
		block.Properties["attribute"] = a.Name.Local
		if a.Name.Space != "" {
			block.Properties["attributeNS"] = a.Name.Space
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})

		p.skelRaw(tagRaw[pos:values[i][0]])
		p.skelRef(blockID)
		pos = values[i][1]
	}
	p.skelRaw(tagRaw[pos:])
}

// odfTagAttrValueRanges returns the byte range of every attribute value inside
// a start tag's own bytes, in document order and excluding the delimiting
// quotes. The decoder reports attributes in that same order, so the i-th range
// belongs to the i-th reported attribute — which is what lets a skeleton
// substitute one value and keep the rest of the tag verbatim.
func odfTagAttrValueRanges(tag []byte) [][2]int {
	var out [][2]int
	i := 0
	// Skip `<` and the element name.
	for i < len(tag) && tag[i] != '<' {
		i++
	}
	i++
	for i < len(tag) && !odfIsTagSpace(tag[i]) && tag[i] != '/' && tag[i] != '>' {
		i++
	}
	for i < len(tag) {
		for i < len(tag) && odfIsTagSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] == '/' || tag[i] == '>' {
			return out
		}
		// Attribute name, then `=`, then the quoted value.
		for i < len(tag) && !odfIsTagSpace(tag[i]) && tag[i] != '=' && tag[i] != '>' {
			i++
		}
		for i < len(tag) && odfIsTagSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] != '=' {
			return out
		}
		i++
		for i < len(tag) && odfIsTagSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || (tag[i] != '"' && tag[i] != '\'') {
			return out
		}
		quote := tag[i]
		i++
		start := i
		for i < len(tag) && tag[i] != quote {
			i++
		}
		if i >= len(tag) {
			return out
		}
		out = append(out, [2]int{start, i})
		i++
	}
	return out
}

// odfIsTagSpace reports whether c is XML white space (XML 1.0 §2.3 S).
func odfIsTagSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
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

// emitAccessibilityContent surfaces image accessibility text (the inner
// CharData of an <svg:title> or <svg:desc> on a drawing frame/shape) as a
// NON-translatable, RoleCaption content block: a single verbatim run (no
// inline parse), visible to ingestion/LLM consumers but skipped by MT
// (Translatable=false). The body rides via a skeleton ref so the byte-exact
// round-trip is preserved; the enclosing frame's geometry (and presentation
// page) is attached when available. Caller gates this behind
// ExtractNonTranslatableContent.
func (r *Reader) emitAccessibilityContent(ctx context.Context, ch chan<- model.PartResult,
	p *odfParser, text, element, partPath string, frameStack []frameBox, pageNum int, blockCounter *int) bool {

	*blockCounter++
	blockID := fmt.Sprintf("tu%d", *blockCounter)
	block := model.NewBlock(blockID, text) // single verbatim source run
	block.Translatable = false
	block.SetSemanticRole(model.RoleCaption, 0)
	block.Properties["partPath"] = partPath
	block.Properties["element"] = element
	applyFrameGeometry(block, frameStack, pageNum)

	p.skelRef(blockID)
	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// isAccessibilityTextParent reports whether the immediate enclosing element is
// an <svg:title> or <svg:desc> — the SVG accessibility text (alt-text /
// long description) carried on draw:frame and other ODF drawing shapes.
func isAccessibilityTextParent(stack []xml.Name) bool {
	if len(stack) == 0 {
		return false
	}
	top := stack[len(stack)-1]
	return top.Space == nsSvg && (top.Local == "title" || top.Local == "desc")
}

// isFormDisplayAttribute reports whether an attribute holds a form control's
// human-facing display string — form:label (visible label), form:title
// (tooltip), or form:help-text (status/help text). Upstream Okapi's ODFFilter
// does not extract these, so the native reader surfaces them only as
// non-translatable content when ExtractNonTranslatableContent is on. Other
// form:* attributes (e.g. form:apply-design-mode, form:automatic-focus) are
// non-display and excluded.
func isFormDisplayAttribute(name xml.Name) bool {
	if name.Space != nsForm {
		return false
	}
	switch name.Local {
	case "label", "title", "help-text":
		return true
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

func (r *Reader) skelPartStart(partPath string) {
	if r.skeletonStore != nil {
		r.skeletonStore.WriteRef(skelPartStartPrefix + partPath)
	}
}

func (r *Reader) skelPartEnd(partPath string) {
	if r.skeletonStore != nil {
		r.skeletonStore.WriteRef(skelPartEndPrefix + partPath)
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

// readZipFile reads the contents of a ZIP file entry, bounded by the shared
// safeio zip limits (per-entry uncompressed size + inflate-ratio zip-bomb
// guard on the actual decompressed stream).
func readZipFile(f *zip.File) ([]byte, error) {
	return safeio.DefaultZipLimits.ReadEntry(f)
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

// The odf reader's two XML escapers were byte-identical to xliff, xliff2 and
// openxml's; they now live in core/internal/xmlesc.
