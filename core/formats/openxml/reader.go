package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// wmlThemeFontLangValRE matches a `<w:themeFontLang ... w:val="VALUE" ...>`
// element and captures the prefix up to and including the opening quote in
// group 1 and the VALUE (the bytes between the surrounding quotes) in group
// 2. The reader uses the group-2 byte range to splice the source-locale
// language value out as a typed SkeletonLang entry; the surrounding bytes
// (including the closing quote and any other attributes such as w:eastAsia)
// are preserved verbatim. The value character class excludes both quote
// characters so the match cannot cross an attribute boundary. This is the
// structural successor to the retired write-side rewriteWMLLangVal regex —
// it targets only `<w:themeFontLang>`'s w:val because that is the only
// language declaration that survives into a settings part (run-property
// `<w:lang>` is stripped by stripWMLSkippableElements before it could be
// retargeted).
var wmlThemeFontLangValRE = regexp.MustCompile(
	`(<w:themeFontLang\b[^>]*?\bw:val=["'])([^"']*)`,
)

// Reader implements DataFormatReader for OpenXML files (DOCX, PPTX, XLSX).
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	tmpFile       string // path to temp file if we had to copy from stream
}

var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new OpenXML reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "openxml",
			FormatDisplayName: "Office Open XML",
			FormatMimeType:    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			FormatExtensions:  []string{".docx", ".docm", ".dotx", ".dotm", ".xlsx", ".xlsm", ".xltx", ".xltm", ".pptx", ".pptm", ".ppsx", ".potx"},
			Cfg:               cfg,
		},
		cfg: cfg,
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
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		Extensions: []string{".docx", ".docm", ".dotx", ".dotm", ".xlsx", ".xlsm", ".xltx", ".xltm", ".pptx", ".pptm", ".ppsx", ".potx"},
		MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}}, // PK ZIP header
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("openxml: nil document or reader")
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

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Read all content into memory (ZIP requires random access)
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("openxml: reading: %w", err)}
		return
	}

	// Open as ZIP
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("openxml: not a valid ZIP archive: %w", err)}
		return
	}

	// Validate the archive against the shared safeio budget (entry count +
	// declared per-entry/total uncompressed sizes + inflate-ratio) before
	// reading any part, so a zip bomb or oversized container is rejected up
	// front. Per-entry reads are additionally bounded in readZipFile.
	if err := safeio.DefaultZipLimits.CheckReader(zr); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("openxml: %w", err)}
		return
	}

	// Parse container metadata
	info, err := parseContainer(zr, r.cfg)
	if err != nil {
		ch <- model.PartResult{Error: err}
		return
	}

	// For XLSX, pre-parse the shared string table
	if info.docType == docTypeXLSX {
		info.sharedStrings, err = parseSharedStrings(zr)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("openxml: parsing shared strings: %w", err)}
			return
		}
	}

	// Emit root layer
	rootLayer := &model.Layer{
		ID:         "doc1",
		Name:       r.Doc.URI,
		Format:     "openxml",
		Locale:     locale,
		Encoding:   "UTF-8",
		MimeType:   r.Doc.MimeType,
		Properties: map[string]string{"docType": info.docType.String()},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: rootLayer}) {
		return
	}

	// Extract embedded media files as PartMedia (Bowrain AD-007).
	if r.cfg.ExtractMedia {
		r.emitMediaParts(ctx, ch, zr, info)
	}

	blockCounter := 0

	// Initialize code finder if configured
	var cf *codeFinder
	if r.cfg.UseCodeFinder && len(r.cfg.CodeFinderRules) > 0 {
		var err error
		cf, err = newCodeFinder(r.cfg.CodeFinderRules)
		if err != nil {
			ch <- model.PartResult{Error: err}
			return
		}
	}

	// Native is faithful: source rPr is preserved inline and the writer
	// does no style synthesis (Word Style Optimisation was removed). The
	// reader therefore does NOT resolve the style chain to subtract
	// style-inherited formatting — every run's direct rPr travels through
	// to the writer via the per-run rPr sidecar. styles stays nil; the
	// wmlParser handles a nil styleMap (no subtraction). The styleMap /
	// parseStyles machinery is retained for the parity comparator's
	// effective-rPr resolution and unit tests.
	var styles *styleMap

	// Build the paragraph-style → semantic-role map from word/styles.xml
	// (WS2). This is additive stand-off metadata used by semantic export
	// (e.g. DOCX → clean Markdown) and the visual editor; it is NEVER
	// serialized back, so byte-faithful round-trip is unaffected. It is
	// deliberately kept separate from the rPr `styles` map (which stays nil
	// so the faithful writer does no style subtraction). Absent/unreadable
	// styles.xml leaves the map nil; roleForParaStyle's built-in styleId
	// heuristic still resolves headings.
	var roleStyles styleRoleMap
	if info.docType == docTypeDOCX {
		if zf := zipFileByName(zr, stylesPartPath(info.mainDocumentPart)); zf != nil {
			if data, err := readZipFile(zf); err == nil {
				roleStyles = buildStyleRoleMap(data)
			}
		}
	}

	// Process each translatable part
	for _, partPath := range info.translatableParts {
		zf := zipFileByName(zr, partPath)
		if zf == nil {
			continue
		}

		partData, err := readZipFile(zf)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("openxml: reading %s: %w", partPath, err)}
			return
		}

		// Emit child layer for this XML part
		childLayer := &model.Layer{
			ID:         "layer-" + partPath,
			Name:       partPath,
			Format:     "", // Same format (openxml)
			Locale:     locale,
			ParentID:   rootLayer.ID,
			Properties: map[string]string{},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
			return
		}

		// Emit skeleton part-boundary marker so the writer knows
		// which ZIP entry each skeleton segment belongs to.
		r.skelPartStart(partPath)

		// Build relationship map for this part
		mainDir := ""
		if idx := lastIndex(partPath, '/'); idx >= 0 {
			mainDir = partPath[:idx+1]
		}
		relsPath := mainDir + "_rels/" + partPath[len(mainDir):] + ".rels"
		relsMap := relsByID(info, relsPath)

		emitBlock := func(block *model.Block) {
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}

		// DocProps/core.xml is format-independent
		if partPath == "docProps/core.xml" {
			parseCoreProperties(partData, partPath, &blockCounter, emitBlock, r.skeletonStore)
			r.skelPartEnd(partPath)
			r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
			continue
		}

		// Parse the part based on document type
		switch info.docType {
		case docTypeDOCX:
			// Chart and SmartArt diagram parts are DrawingML, not
			// WordprocessingML. Their text-bearing leaves are <a:p>
			// paragraphs (chart titles inside <c:tx><c:rich>; diagram
			// node text inside <dgm:t>) — the same paragraph shape
			// PPTX slides use, just without the <txBody> wrapper. We
			// route them through the dml parser's chart/diagram
			// dispatch. Mirrors okapi WordDocument.java line 202-203
			// (DIAGRAM_DATA_TYPE / CHART_TYPE → StyledTextPart).
			if isChartPartPath(partPath) || isDiagramDataPartPath(partPath) {
				parser := &dmlParser{
					cfg:                 r.cfg,
					blockCounter:        &blockCounter,
					skeletonStore:       r.skeletonStore,
					rels:                relsMap,
					stripEmptyParaProps: true,
				}
				err = parser.parseChartOrDiagramPart(partData, partPath, emitBlock)
				if err != nil {
					ch <- model.PartResult{Error: err}
					return
				}
				parser.skelFlush()
				break
			}
			parser := &wmlParser{
				cfg:           r.cfg,
				blockCounter:  &blockCounter,
				skeletonStore: r.skeletonStore,
				rels:          relsMap,
				codeFinder:    cf,
				styles:        styles,
				roleStyles:    roleStyles,
				// Detect Strict OOXML conformance by searching the
				// part bytes for the strict WPML namespace URI.
				// Every WPML XML part declares the prefix binding on
				// its root element; a substring scan is sufficient
				// because the URI is unique to OOXML Strict. Mirrors
				// upstream Okapi's namespace classification via
				// Namespaces.WordProcessingML vs
				// Namespaces.StrictWordProcessingML
				// (Namespaces.java:26-27).
				strict: bytes.Contains(partData, []byte(wmlStrictNamespace)),
			}
			err = parser.parsePart(partData, partPath, emitBlock, func() {})
			if err != nil {
				ch <- model.PartResult{Error: err}
				return
			}
			parser.skelFlush()

		case docTypePPTX:
			parser := &dmlParser{
				cfg:           r.cfg,
				blockCounter:  &blockCounter,
				skeletonStore: r.skeletonStore,
				rels:          relsMap,
				slideNum:      pptxSlideNum(partPath),
			}
			err = parser.parsePart(partData, partPath, emitBlock)
			if err != nil {
				ch <- model.PartResult{Error: err}
				return
			}
			parser.skelFlush()

		case docTypeXLSX:
			parser := &smlParser{
				cfg:           r.cfg,
				blockCounter:  &blockCounter,
				skeletonStore: r.skeletonStore,
				sharedStrings: info.sharedStrings,
			}
			err = parser.parsePart(partData, partPath, emitBlock)
			if err != nil {
				ch <- model.PartResult{Error: err}
				return
			}
			parser.skelFlush()
		}

		r.skelPartEnd(partPath)

		// End child layer
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
	}

	// Settings parts (word/settings.xml, word/glossary/settings.xml) are
	// non-translatable, so they don't appear in the loop above — but they
	// carry the document's `<w:themeFontLang w:val="...">` declaration,
	// whose value the writer retargets from the source to the target
	// locale on a translation round-trip (mirroring okapi's Property.LANGUAGE
	// rewrite — see Writer.SetSourceLocale). To make that retarget
	// structural rather than a write-side regex over assembled bytes (#607),
	// the reader splices the `w:val` value out of each settings part as a
	// typed SkeletonLang entry surrounded by the verbatim part bytes, so the
	// writer reconstructs the part from skeleton and consumes the lang value
	// structurally. Only emitted when a skeleton store is wired and the doc
	// is WordprocessingML.
	if r.skeletonStore != nil && info.docType == docTypeDOCX {
		r.emitSettingsLangSkeleton(zr, "word/settings.xml")
		r.emitSettingsLangSkeleton(zr, "word/glossary/settings.xml")
	}

	// End root layer
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
}

// emitSettingsLangSkeleton emits a skeleton segment for a WordprocessingML
// settings part, splicing the `w:val` value of each `<w:themeFontLang>`
// element out as a typed SkeletonLang entry between verbatim text segments.
// Everything else in the part — including the element's other attributes
// (e.g. w:eastAsia) — is preserved byte-for-byte as SkeletonText, so a
// no-retarget round-trip is byte-exact and a retargeting round-trip only
// substitutes the spliced w:val.
//
// Strict OOXML parts are emitted as a single verbatim SkeletonText segment
// (no splice): upstream okapi's Property.LANGUAGE rewrite is QName-keyed to
// the transitional WordProcessingML URI and never fires on strict parts, so
// their themeFontLang must round-trip unchanged. Mirrors the strict-namespace
// guard the retired rewriteWMLLangVal regex applied.
//
// The part is skipped entirely when it is absent from the ZIP — settings
// parts that don't exist need no skeleton segment; the writer leaves them to
// the verbatim ZIP copy path.
func (r *Reader) emitSettingsLangSkeleton(zr *zip.Reader, partPath string) {
	zf := zipFileByName(zr, partPath)
	if zf == nil {
		return
	}
	data, err := readZipFile(zf)
	if err != nil {
		return
	}

	r.skelPartStart(partPath)
	defer r.skelPartEnd(partPath)

	strict := bytes.Contains(data, []byte(wmlStrictNamespace))
	if strict || !bytes.Contains(data, []byte("<w:themeFontLang")) {
		_ = r.skeletonStore.WriteText(data)
		return
	}

	// Splice each <w:themeFontLang ... w:val="VALUE" ...> value range out as
	// a SkeletonLang entry. wmlThemeFontLangValRE captures the prefix up to
	// and including the open quote (sub[1]), the value (sub[2]), so the
	// match end minus one byte is the close quote. We emit verbatim bytes up
	// to the value, the SkeletonLang(value) entry, then continue after it.
	pos := 0
	for _, loc := range wmlThemeFontLangValRE.FindAllSubmatchIndex(data, -1) {
		// loc: [matchStart matchEnd, g1Start g1End, g2Start g2End]
		valStart, valEnd := loc[4], loc[5]
		_ = r.skeletonStore.WriteText(data[pos:valStart])
		_ = r.skeletonStore.WriteLang(string(data[valStart:valEnd]))
		pos = valEnd
	}
	_ = r.skeletonStore.WriteText(data[pos:])
}

// Skeleton part-boundary markers. The writer uses these to split the
// single skeleton stream into per-ZIP-entry segments.
const (
	skelPartStartPrefix = "@@SKEL_PART_START@@"
	skelPartEndPrefix   = "@@SKEL_PART_END@@"
)

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

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// emitMediaParts scans the ZIP for embedded media files (word/media/*, ppt/media/*)
// and emits PartMedia parts with content-addressed blob keys.
func (r *Reader) emitMediaParts(ctx context.Context, ch chan<- model.PartResult, zr *zip.Reader, info *containerInfo) {
	// Determine media directory based on document type.
	var mediaPrefixes []string
	switch info.docType {
	case docTypeDOCX:
		mediaPrefixes = []string{"word/media/"}
	case docTypePPTX:
		mediaPrefixes = []string{"ppt/media/"}
	default:
		return // XLSX typically has no embedded media
	}

	for _, f := range zr.File {
		isMedia := false
		for _, prefix := range mediaPrefixes {
			if strings.HasPrefix(f.Name, prefix) {
				isMedia = true
				break
			}
		}
		if !isMedia {
			continue
		}

		data, err := readZipFile(f)
		if err != nil {
			continue // best-effort: skip unreadable media
		}

		blobKey := computeBlobKey(data)
		filename := f.Name[strings.LastIndex(f.Name, "/")+1:]
		mimeType := detectMediaMIME(filename)

		media := &model.Media{
			ID:       "media:" + f.Name,
			MimeType: mimeType,
			Data:     data,
			BlobKey:  blobKey,
			Filename: filename,
			Size:     int64(len(data)),
			Properties: map[string]string{
				"zipPath": f.Name,
			},
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartMedia, Resource: media}) {
			return
		}
	}
}

// computeBlobKey returns the SHA-256 hex digest of data.
func computeBlobKey(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// detectMediaMIME infers MIME type from filename extension.
func detectMediaMIME(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	case ".svg":
		return "image/svg+xml"
	case ".emf":
		return "image/x-emf"
	case ".wmf":
		return "image/x-wmf"
	default:
		return "application/octet-stream"
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

// stylesPartPath returns the conventional styles.xml part path for a package,
// derived from the main document part's directory (e.g. "word/document.xml" →
// "word/styles.xml"). Word always co-locates styles.xml with the main
// document; an absent entry simply yields no role map.
func stylesPartPath(mainDocumentPart string) string {
	dir := ""
	if idx := strings.LastIndex(mainDocumentPart, "/"); idx >= 0 {
		dir = mainDocumentPart[:idx+1]
	}
	return dir + "styles.xml"
}

// readZipFile reads the contents of a ZIP file entry, bounded by the shared
// safeio zip limits (per-entry uncompressed size + inflate-ratio zip-bomb
// guard, enforced on the actual decompressed stream).
func readZipFile(f *zip.File) ([]byte, error) {
	return safeio.DefaultZipLimits.ReadEntry(f)
}

// corePropsParser parses docProps/core.xml with skeleton support.
type corePropsParser struct {
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
}

// parseCoreProperties extracts translatable content from docProps/core.xml.
// Dublin Core elements like dc:title, dc:subject, dc:creator, cp:keywords etc.
//
// One okapi-bridge quirk is faithfully reproduced for byte-equal parity:
// a SELF-CLOSING (e.g. `<cp:category/>`) trailing translatable element is
// dropped from the skeleton when it has no text content. Upstream Okapi
// parses core.xml with Jericho through OpenXMLContentFilter
// (okapi/filters/openxml/src/main/java/.../ContentFilter.java
// handleStartTag case TEXT_UNIT_ELEMENT, lines 312-319): start tags are
// stored as a "pending" tag and only committed to the document part when
// the NEXT start tag arrives (line 269 calls startDelayedTextUnit). For
// self-closing TEXTUNIT elements, Jericho emits only a StartTag (no
// EndTag); when such an element is the last translatable inside
// `<cp:coreProperties>` nothing flushes the pending tag, so it is
// effectively dropped. An empty element written with explicit open/close
// form (`<cp:category></cp:category>`) is preserved because Jericho
// emits an EndTag which fires handleEndTag's TEXT_UNIT_ELEMENT case and
// flushes pendingTagText + the end tag (ContentFilter.java lines 386-394).
// We mirror that distinction by inspecting the byte slice Go's
// xml.Decoder advanced over for the EndElement token: zero-length means
// self-closing (decoder synthesizes EndElement without consuming input);
// non-zero means an explicit `</name>` was consumed.
// ECMA-376-1 §15.2.12 makes every Dublin Core / cp:* element in the
// core properties part optional, so omitting an empty cp:category is
// spec-valid.
func parseCoreProperties(data []byte, partPath string, blockCounter *int, emitBlock func(*model.Block), skelStore *format.SkeletonStore) {
	p := &corePropsParser{skeletonStore: skelStore}
	// Strip a leading UTF-8 BOM (EF BB BF) before handing to xml.Decoder.
	// Go's encoding/xml does NOT consume the BOM as encoding metadata —
	// it is surfaced as a CharData token in the document prolog (i.e.
	// before the XML declaration). Without the strip the BOM bytes flow
	// straight into the skeleton via the CharData branch below and end
	// up echoed at offset 0 of the reconstructed part.
	//
	// Upstream Okapi's OpenXMLContentFilter reads docProps/core.xml
	// through StAX's XMLEventReader, which decodes the source bytes
	// into UTF-16 internally and treats the BOM as encoding metadata
	// (not a character event). The corresponding XMLEventWriter does
	// not emit a BOM either, so the okapi reference output for any
	// docProps/core.xml that ships with a source BOM ends up BOM-less.
	// We strip here so the native skeleton matches that emit shape.
	//
	// Per ECMA-376-1 §A.2 (parts encoding) a UTF-8 XML part may be
	// authored with or without a BOM; consumers must accept both forms
	// but byte-equal parity tracks okapi's no-BOM emit shape. 948-1
	// .docx is the only docx fixture in the corpus whose core.xml
	// ships with a BOM.
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	d := xml.NewDecoder(bytes.NewReader(data))

	// Translatable Dublin Core / OPC core-properties elements. Mirrors
	// okapi's wordDocPropertiesConfiguration.yml (lines 41-60 of okapi/
	// filters/openxml/src/main/resources/net/sf/okapi/filters/openxml/
	// wordDocPropertiesConfiguration.yml) which lists every TEXTUNIT
	// element the OpenXMLContentFilter extracts in MSWORDDOCPROPERTIES
	// mode. The match is on local name (namespace-agnostic) — okapi
	// scopes by qualified name (`cp:contentstatus`, `dc:title`, etc.)
	// but Word never emits competing local names for these in core.xml.
	translatableElements := map[string]bool{
		"title":         true,
		"subject":       true,
		"creator":       true,
		"keywords":      true,
		"description":   true,
		"category":      true,
		"contentStatus": true,
	}

	var inTranslatable bool
	var currentElement string
	var currentStart xml.StartElement
	var startOffsetAfter int64 // d.InputOffset() right after the StartElement token
	var textBuf strings.Builder

	// pendingSelfClosing holds the skeleton bytes for an empty
	// self-closing translatable element. The bytes are only committed
	// to the real skeleton when a subsequent xml.StartElement arrives
	// (mirroring okapi's startDelayedTextUnit at the start of
	// handleStartTag). If the next event is the root EndElement (or
	// EOF), the bytes stay buffered and are dropped — matching okapi's
	// drop of the trailing self-closing TEXTUNIT element. An empty
	// element written with explicit open/close form bypasses this
	// buffer entirely and is committed straight to the skeleton.
	var pendingSelfClosing strings.Builder
	hasPending := false
	flushPending := func() {
		if hasPending {
			p.skelText(pendingSelfClosing.String())
			pendingSelfClosing.Reset()
			hasPending = false
		}
	}

	prevOffset := d.InputOffset()
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		curOffset := d.InputOffset()

		switch t := tok.(type) {
		case xml.StartElement:
			// A new element start always flushes any pending empty
			// translatable, matching okapi's startDelayedTextUnit().
			flushPending()
			if translatableElements[t.Name.Local] {
				inTranslatable = true
				currentElement = t.Name.Local
				currentStart = t
				startOffsetAfter = curOffset
				textBuf.Reset()
			} else {
				p.skelWriteStartElement(t)
			}
		case xml.CharData:
			if inTranslatable {
				textBuf.Write(t)
			} else {
				p.skelText(xmlEscape(string(t)))
			}
		case xml.EndElement:
			if inTranslatable && t.Name.Local == currentElement {
				text := strings.TrimSpace(textBuf.String())
				if text != "" {
					*blockCounter++
					blockID := fmt.Sprintf("tu%d", *blockCounter)
					// Skeleton: write element open, ref, element close
					p.skelWriteStartElement(currentStart)
					p.skelRef(blockID)
					p.skelWriteEndElement(t)

					block := &model.Block{
						ID:           blockID,
						Type:         "property",
						Translatable: true,
						Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
						Targets:      make(map[model.VariantKey]*model.Target),
						Properties: map[string]string{
							"partPath": partPath,
							"element":  currentElement,
						},
					}
					emitBlock(block)
				} else {
					// Empty translatable element. Detect the source
					// form by inspecting the byte slice the decoder
					// consumed for THIS EndElement event. Go's
					// xml.Decoder synthesizes an EndElement for
					// `<x/>` without consuming any input (curOffset
					// == prevOffset), and consumes `</x>` for an
					// explicit close form (curOffset > prevOffset).
					// We additionally require startOffsetAfter ==
					// prevOffset — i.e. no CharData or other token
					// arrived between the StartElement and the
					// EndElement; a `<x>   </x>` form would have a
					// CharData event and is treated as explicit
					// open/close even though textBuf trims to "".
					selfClosing := curOffset == prevOffset && startOffsetAfter == prevOffset
					if selfClosing {
						pendingSelfClosing.Reset()
						writeStartElementToBuilder(&pendingSelfClosing, currentStart)
						writeEndElementToBuilder(&pendingSelfClosing, t)
						hasPending = true
					} else {
						// Explicit open/close form — upstream Okapi
						// preserves it via handleEndTag's
						// TEXT_UNIT_ELEMENT case (ContentFilter.java
						// lines 386-394).
						p.skelWriteStartElement(currentStart)
						p.skelWriteEndElement(t)
					}
				}
				inTranslatable = false
				currentElement = ""
			} else {
				// Non-translatable end tag. For the closing of the
				// root (`</cp:coreProperties>`) the pending self-
				// closing element must NOT be flushed: upstream Okapi
				// drops it because no further start tag arrives to
				// trigger startDelayedTextUnit. For any other
				// non-translatable end tag (future-proofing — none
				// expected today in core.xml) flush so element order
				// is preserved.
				if t.Name.Local != "coreProperties" {
					flushPending()
				}
				p.skelWriteEndElement(t)
			}
		case xml.ProcInst:
			flushPending()
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")
		}
		prevOffset = curOffset
	}
	// Note: any still-pending self-closing translatable is
	// intentionally discarded here, matching okapi's drop of the
	// trailing self-closing TEXTUNIT element.
	p.skelFlush()
}

// writeStartElementToBuilder serializes an xml.StartElement to buf,
// mirroring corePropsParser.skelWriteStartElement but writing to an
// arbitrary strings.Builder. Used to hold "pending self-closing empty
// translatable" skeleton bytes that may be discarded later.
func writeStartElementToBuilder(buf *strings.Builder, t xml.StartElement) {
	registerNamespaces(t.Attr)
	buf.WriteString("<")
	writeElementName(buf, t.Name)
	for _, a := range t.Attr {
		buf.WriteString(" ")
		writeAttrName(buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
}

// writeEndElementToBuilder serializes an xml.EndElement to buf,
// mirroring corePropsParser.skelWriteEndElement.
func writeEndElementToBuilder(buf *strings.Builder, t xml.EndElement) {
	buf.WriteString("</")
	writeElementName(buf, t.Name)
	buf.WriteString(">")
}

func (p *corePropsParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *corePropsParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		_ = p.skeletonStore.WriteRef(id)
	}
}

func (p *corePropsParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

func (p *corePropsParser) skelWriteStartElement(t xml.StartElement) {
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
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *corePropsParser) skelWriteEndElement(t xml.EndElement) {
	if p.skeletonStore == nil {
		return
	}
	var buf strings.Builder
	buf.WriteString("</")
	writeElementName(&buf, t.Name)
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

// lastIndex returns the index of the last occurrence of sep in s, or -1.
func lastIndex(s string, sep byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep {
			return i
		}
	}
	return -1
}
