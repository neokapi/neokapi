package openxml

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
		return fmt.Errorf("openxml: nil document or reader")
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

	blockCounter := 0

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
			parseCoreProperties(partData, partPath, &blockCounter, emitBlock)
			r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
			continue
		}

		// Parse the part based on document type
		switch info.docType {
		case docTypeDOCX:
			parser := &wmlParser{
				cfg:           r.cfg,
				blockCounter:  &blockCounter,
				skeletonStore: r.skeletonStore,
				rels:          relsMap,
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

		// End child layer
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
	}

	// End root layer
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
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

// parseCoreProperties extracts translatable content from docProps/core.xml.
// Dublin Core elements like dc:title, dc:subject, dc:creator, cp:keywords etc.
func parseCoreProperties(data []byte, partPath string, blockCounter *int, emitBlock func(*model.Block)) {
	d := xml.NewDecoder(bytes.NewReader(data))

	// Translatable DC elements
	translatableElements := map[string]bool{
		"title":       true,
		"subject":     true,
		"creator":     true,
		"keywords":    true,
		"description": true,
		"category":    true,
	}

	var inTranslatable bool
	var currentElement string
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
				textBuf.Reset()
			}
		case xml.CharData:
			if inTranslatable {
				textBuf.Write(t)
			}
		case xml.EndElement:
			if inTranslatable && t.Name.Local == currentElement {
				text := strings.TrimSpace(textBuf.String())
				if text != "" {
					*blockCounter++
					blockID := fmt.Sprintf("tu%d", *blockCounter)
					block := &model.Block{
						ID:           blockID,
						Type:         "property",
						Translatable: true,
						Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(text)}},
						Targets:      make(map[model.LocaleID][]*model.Segment),
						Properties: map[string]string{
							"partPath": partPath,
							"element":  currentElement,
						},
						Annotations: make(map[string]model.Annotation),
					}
					emitBlock(block)
				}
				inTranslatable = false
				currentElement = ""
			}
		}
	}
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
