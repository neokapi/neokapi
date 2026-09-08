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
		FormatName:        "openxml",
		FormatDisplayName: "Office Open XML",
		FormatMimeType:    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		FormatExtensions:  []string{".docx", ".docm", ".dotx", ".dotm", ".xlsx", ".xlsm", ".xltx", ".xltm", ".pptx", ".pptm", ".ppsx", ".potx"},
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
	return format.StreamParts(ctx, func(ctx context.Context, ch chan<- model.PartResult) error {
		r.readContent(ctx, ch)
		return nil
	})
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Entries are read straight out of the container — from the file handle
	// when the caller gave us one, otherwise from a single buffered copy. Both
	// the container size and the zip limits (entry count, declared sizes,
	// inflate ratio) are checked before any entry is read; per-entry reads are
	// additionally bounded in readZipFile.
	zr, err := format.OpenZipDocument(r.Doc, "openxml")
	if err != nil {
		ch <- model.PartResult{Error: err}
		return
	}

	// Parse container metadata
	info, err := parseContainer(zr, r.cfg)
	if err != nil {
		ch <- model.PartResult{Error: err}
		return
	}

	// For XLSX, pre-parse the shared string table and the sheet names.
	if info.docType == docTypeXLSX {
		info.sharedStrings, err = parseSharedStrings(zr)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("openxml: parsing shared strings: %w", err)}
			return
		}
		info.sheetNames = parseSheetNames(zr, info.relationships)
		info.cellStyles = parseCellStyles(zr, info.relationships)
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
	dataCounter := 0
	// ids separates the ids the archive's parts repeat; see core/model/blockid.go.
	// One per document, like the counter beside it: an id has to be an identity
	// across the whole archive, because that is the unit the block store keys on.
	var ids model.IDBuilder

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

		// A worksheet is read from its entry rather than buffered: it is the
		// largest part a spreadsheet has, and it is the only one no consumer
		// needs a second look at after the parse.
		streamSheet := info.docType == docTypeXLSX && isWorksheetPartPath(partPath)
		var partData []byte
		if !streamSheet {
			var rerr error
			partData, rerr = readZipFile(zf)
			if rerr != nil {
				ch <- model.PartResult{Error: fmt.Errorf("openxml: reading %s: %w", partPath, rerr)}
				return
			}
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
			// Every block leaves with a digest of the source runs it was
			// read with, so a writer replaying source bytes can tell an
			// in-place edit from the runs it read. See sourceRunsAsRead.
			stampSourceFingerprint(block)
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}

		// emitData emits a non-translatable Data part carrying contextual text
		// (PPTX/XLSX comment bodies, #928). The Data part is purely informational
		// — it carries no skeleton ref, so the writer ignores it and the comment
		// part round-trips verbatim (PPTX) or via verbatim copy (XLSX). Gated by
		// the caller behind ExtractNonTranslatableContent so the flag-off part
		// stream stays byte-identical for parity.
		emitData := func(name, text, ref string) {
			dataCounter++
			props := map[string]string{"partPath": partPath, "text": text}
			if ref != "" {
				props["ref"] = ref
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: &model.Data{
				ID:         fmt.Sprintf("comment%d", dataCounter),
				Name:       name,
				Properties: props,
			}})
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
				// Surface table topology (w:tbl/w:tr → Groups, cells → RoleTableCell)
				// so cross-format writers and core/projection rebuild the grid.
				emitPart: func(part *model.Part) { r.emit(ctx, ch, part) },
			}
			parser.partPlane, parser.partNoteRole = docxPartStructure(partPath)
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
			// Surface PowerPoint comment bodies (<p:text>) as informational
			// Data parts (#928). The comment part itself round-trips verbatim
			// through the skeleton above; this only adds context for ingestion.
			if r.cfg.ExtractNonTranslatableContent() && isCommentPartPath(partPath) {
				emitPPTXCommentData(partData, emitData)
			}

		case docTypeXLSX:
			parser := &smlParser{
				cfg:           r.cfg,
				blockCounter:  &blockCounter,
				ids:           &ids,
				skeletonStore: r.skeletonStore,
				sharedStrings: info.sharedStrings,
				sheetNames:    info.sheetNames,
				styles:        info.cellStyles,
				// Surface worksheet topology (table/table-row Groups) so
				// cross-format writers and core/projection rebuild the grid
				// from the stream instead of buffering it to rediscover.
				emitPart: func(part *model.Part) { r.emit(ctx, ch, part) },
			}
			if streamSheet {
				err = parser.parseWorksheetStream(
					func() (io.ReadCloser, error) { return safeio.DefaultZipLimits.OpenEntry(zf) },
					partPath, emitBlock)
			} else {
				err = parser.parsePart(partData, partPath, emitBlock)
			}
			if err != nil {
				ch <- model.PartResult{Error: err}
				return
			}
			parser.skelFlush()
			// Surface Excel comment bodies (<comment><text>) as informational
			// Data parts (#928). The comment part is not parsed for skeleton (it
			// is copied verbatim by the writer), so this only adds context.
			if r.cfg.ExtractNonTranslatableContent() && isCommentPartPath(partPath) {
				emitXLSXCommentData(partData, emitData)
			}
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
		r.skeletonStore.WriteText(data)
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
		r.skeletonStore.WriteText(data[pos:valStart])
		r.skeletonStore.WriteLang(string(data[valStart:valEnd]))
		pos = valEnd
	}
	r.skeletonStore.WriteText(data[pos:])
}

// Skeleton part-boundary markers. The writer uses these to split the
// single skeleton stream into per-ZIP-entry segments.
const (
	skelPartStartPrefix = "@@SKEL_PART_START@@"
	skelPartEndPrefix   = "@@SKEL_PART_END@@"
)

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
//
// The bytes stay in the container: each part carries a deferred accessor that
// re-opens its zip entry, not a materialized blob. Embedded media is what makes
// a real .docx large — images and fonts are already compressed and store close
// to 1:1 — and most consumers never want it, so holding every asset for the
// duration of the run to serve the few that do was the single largest resident
// copy in a package read. The content-addressed key still travels with the
// part, computed by streaming the entry through the digest and discarding the
// bytes, so asset dedup does not have to materialize anything either.
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

		blobKey, size, err := digestZipFile(f)
		if err != nil {
			continue // best-effort: skip unreadable media
		}

		filename := f.Name[strings.LastIndex(f.Name, "/")+1:]

		media := &model.Media{
			ID:       "media:" + f.Name,
			MimeType: detectMediaMIME(filename),
			BlobKey:  blobKey,
			Filename: filename,
			Size:     size,
			Open:     func() (io.ReadCloser, error) { return safeio.DefaultZipLimits.OpenEntry(f) },
			Properties: map[string]string{
				"zipPath": f.Name,
			},
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartMedia, Resource: media}) {
			return
		}
	}
}

// digestZipFile streams a zip entry through SHA-256 and reports its
// content-addressed key and uncompressed size without retaining the bytes. The
// declared header size is not trusted for the size (a lying header is the whole
// point of the safeio entry guard), so both come from the actual stream.
func digestZipFile(f *zip.File) (blobKey string, size int64, err error) {
	rc, err := safeio.DefaultZipLimits.OpenEntry(f)
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()
	h := sha256.New()
	n, err := io.Copy(h, rc)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
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
// A property holding no text is replayed from the bytes its author wrote,
// whatever form they took and wherever it sits: `<cp:category/>` goes back
// self-closing and `<cp:category></cp:category>` goes back with both tags.
// Upstream Okapi omits a self-closing one that is the last translatable child
// of `<cp:coreProperties>`. It reads the part with Jericho through
// OpenXMLContentFilter, which holds a start tag as pending and commits it when
// the next start tag arrives (ContentFilter.java, handleStartTag case
// TEXT_UNIT_ELEMENT calling startDelayedTextUnit), and Jericho reports a
// self-closing element as a StartTag with no EndTag, so in that one position
// nothing flushes the pending tag. ECMA-376-1 §15.2.12 makes every Dublin
// Core and cp:* element of the part optional, so both packages are valid; the
// parity canonicaliser drops an empty core property on both sides
// (XMLCanonical.StripEmptyCoreProperties).
func parseCoreProperties(data []byte, partPath string, blockCounter *int, emitBlock func(*model.Block), skelStore *format.SkeletonStore) {
	p := &corePropsParser{skeletonStore: skelStore}
	// Go's encoding/xml reports a leading UTF-8 BOM as character data in the
	// prolog rather than taking it as encoding metadata, so it is held back
	// from the decoder and written to the skeleton as it was: the part goes
	// back with the BOM it came with. ECMA-376-1 §A.2 allows a UTF-8 part
	// either way. The parity canonicaliser drops a BOM on both sides
	// (StripXMLDeclaration), since upstream Okapi writes none.
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		p.skelText(string(data[:3]))
		data = data[3:]
	}
	d := newRawDecoder(data)

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
	var currentStartRaw string // the translatable element's own source bytes
	var startOffsetAfter int64 // d.InputOffset() right after the StartElement token
	var textBuf strings.Builder

	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		curOffset := d.InputOffset()

		switch t := tok.(type) {
		case xml.StartElement:
			if translatableElements[t.Name.Local] {
				inTranslatable = true
				currentElement = t.Name.Local
				registerNamespaces(t.Attr)
				currentStartRaw = d.RawString()
				startOffsetAfter = curOffset
				d.Pin(curOffset)
				textBuf.Reset()
			} else {
				p.skelWriteStartElement(d, t)
			}
		case xml.CharData:
			if inTranslatable {
				textBuf.Write(t)
			} else {
				p.skelRaw(d)
			}
		case xml.EndElement:
			if inTranslatable && t.Name.Local == currentElement {
				// The property's text as the source wrote it, and separately
				// the question of whether it holds anything at all. A title
				// authored with a trailing space is that string: trimming it
				// changes what a translator sees and what goes back. A
				// property holding only whitespace produces no block, and the
				// empty-element branch below replays its source form.
				text := textBuf.String()
				if strings.TrimSpace(text) != "" {
					*blockCounter++
					blockID := fmt.Sprintf("tu%d", *blockCounter)
					// Skeleton: write element open, ref, element close
					p.skelText(currentStartRaw)
					p.skelRef(blockID)
					p.skelWriteEndElement(d)

					block := &model.Block{
						ID: blockID,
						// A core property is named by its element — dc:title,
						// cp:keywords — which is the key the format gives it.
						Name:         model.StructuralPath(append(strings.Split(partPath, "/"), currentElement)...),
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
					// Nothing was extracted, so the element goes back as
					// the source wrote it: the start tag, the bytes
					// between the tags and the end tag. A self-closing
					// element carries its `/>` in the start tag, spans no
					// content and has a synthetic end covering no source
					// bytes, so the same three writes replay it whole.
					p.skelText(currentStartRaw)
					p.skelText(d.UptoString(startOffsetAfter))
					p.skelWriteEndElement(d)
				}
				d.Unpin()
				inTranslatable = false
				currentElement = ""
			} else {
				p.skelWriteEndElement(d)
			}
		default:
			p.skelRaw(d)
		}
	}
	p.skelFlush()
}

func (p *corePropsParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *corePropsParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		p.skeletonStore.WriteRef(id)
	}
}

func (p *corePropsParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

// skelRaw appends the source bytes of the token the decoder last returned.
func (p *corePropsParser) skelRaw(d *rawDecoder) {
	if p.skeletonStore == nil {
		return
	}
	p.skelBuf.Write(d.Raw())
}

func (p *corePropsParser) skelWriteStartElement(d *rawDecoder, t xml.StartElement) {
	if p.skeletonStore == nil {
		return
	}
	registerNamespaces(t.Attr)
	p.skelBuf.Write(d.Raw())
}

// skelWriteEndElement appends an end tag. A self-closing element's synthetic end
// carries no source bytes, so its `/>` came through with the start element.
func (p *corePropsParser) skelWriteEndElement(d *rawDecoder) {
	if p.skeletonStore == nil {
		return
	}
	p.skelBuf.Write(d.Raw())
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

// isWorksheetPartPath reports whether a part is a spreadsheet worksheet, the one
// part large enough to be worth reading from its entry rather than buffering.
func isWorksheetPartPath(partPath string) bool {
	return strings.Contains(partPath, "worksheets/")
}
