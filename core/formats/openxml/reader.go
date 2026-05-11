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
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
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

	// Parse styles.xml for style optimization if configured
	var styles *styleMap
	if r.cfg.OptimiseWordStyles && info.docType == docTypeDOCX {
		styles = parseStyles(zr)
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

	// End root layer
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
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

// readZipFile reads the contents of a ZIP file entry.
func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// corePropsParser parses docProps/core.xml with skeleton support.
type corePropsParser struct {
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
}

// parseCoreProperties extracts translatable content from docProps/core.xml.
// Dublin Core elements like dc:title, dc:subject, dc:creator, cp:keywords etc.
func parseCoreProperties(data []byte, partPath string, blockCounter *int, emitBlock func(*model.Block), skelStore *format.SkeletonStore) {
	p := &corePropsParser{skeletonStore: skelStore}
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
	var textBuf strings.Builder

	for {
		tok, err := d.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if translatableElements[t.Name.Local] {
				inTranslatable = true
				currentElement = t.Name.Local
				currentStart = t
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
						Source:       []*model.Segment{model.NewRunsSegment("s1", []model.Run{{Text: &model.TextRun{Text: text}}})},
						Targets:      make(map[model.LocaleID][]*model.Segment),
						Properties: map[string]string{
							"partPath": partPath,
							"element":  currentElement,
						},
						Annotations: make(map[string]model.Annotation),
					}
					emitBlock(block)
				} else {
					// Empty translatable element — pass through to skeleton
					p.skelWriteStartElement(currentStart)
					p.skelText(xmlEscape(textBuf.String()))
					p.skelWriteEndElement(t)
				}
				inTranslatable = false
				currentElement = ""
			} else {
				p.skelWriteEndElement(t)
			}
		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")
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
