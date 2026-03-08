package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for EPUB e-book files.
type Reader struct {
	format.BaseFormatReader
	cfg     *Config
	content []byte
}

// NewReader creates a new EPUB reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "epub",
			FormatDisplayName: "EPUB E-Book",
			FormatMimeType:    "application/epub+zip",
			FormatExtensions:  []string{".epub"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/epub+zip"},
		Extensions: []string{".epub"},
		MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}},
		Sniff: func(data []byte) bool {
			// Check for "mimetype" entry with "application/epub+zip"
			return bytes.Contains(data, []byte("application/epub+zip"))
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("epub: nil document or reader")
	}
	r.Doc = doc

	data, err := io.ReadAll(doc.Reader)
	if err != nil {
		return fmt.Errorf("epub: reading document: %w", err)
	}
	r.content = data
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

// container.xml structure
type container struct {
	Rootfiles []rootfile `xml:"rootfiles>rootfile"`
}

type rootfile struct {
	FullPath  string `xml:"full-path,attr"`
	MediaType string `xml:"media-type,attr"`
}

// OPF package document structure
type opfPackage struct {
	Manifest opfManifest `xml:"manifest"`
	Spine    opfSpine    `xml:"spine"`
}

type opfManifest struct {
	Items []opfItem `xml:"item"`
}

type opfItem struct {
	ID        string `xml:"id,attr"`
	Href      string `xml:"href,attr"`
	MediaType string `xml:"media-type,attr"`
}

type opfSpine struct {
	ItemRefs []opfItemRef `xml:"itemref"`
}

type opfItemRef struct {
	IDRef string `xml:"idref,attr"`
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	rootLayer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "epub",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/epub+zip",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: rootLayer}) {
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(r.content), int64(len(r.content)))
	if err != nil {
		r.emitError(ch, fmt.Errorf("epub: opening zip: %w", err))
		return
	}

	// Build file map
	fileMap := make(map[string]*zip.File)
	for _, f := range zr.File {
		fileMap[f.Name] = f
	}

	// Parse container.xml to find OPF
	opfPath, err := r.findOPF(fileMap)
	if err != nil {
		r.emitError(ch, err)
		return
	}

	// Parse OPF to find spine items
	spineItems, err := r.parseOPF(fileMap, opfPath)
	if err != nil {
		r.emitError(ch, err)
		return
	}

	blockCounter := 0
	layerCounter := 1
	dataCounter := 0

	// Process each spine item (content document)
	for _, itemPath := range spineItems {
		file, ok := fileMap[itemPath]
		if !ok {
			continue
		}

		layerCounter++
		childLayer := &model.Layer{
			ID:       fmt.Sprintf("layer%d", layerCounter),
			Name:     itemPath,
			Format:   "epub",
			Locale:   locale,
			ParentID: rootLayer.ID,
			MimeType: "application/xhtml+xml",
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
			return
		}

		content, err := r.readEntry(file)
		if err != nil {
			r.emitError(ch, fmt.Errorf("epub: reading %s: %w", itemPath, err))
			return
		}

		texts := extractXHTMLText(content)
		for _, text := range texts {
			blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
			block.Name = itemPath
			block.Properties["entry"] = itemPath
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer}) {
			return
		}
	}

	// Emit non-content files as Data
	spineSet := make(map[string]bool)
	for _, item := range spineItems {
		spineSet[item] = true
	}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() || spineSet[file.Name] {
			continue
		}
		// Skip structural EPUB files (container.xml, OPF, NCX)
		if file.Name == "META-INF/container.xml" || file.Name == opfPath {
			continue
		}
		if file.Name == "mimetype" {
			continue
		}
		dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: file.Name,
			Properties: map[string]string{
				"entry": file.Name,
				"size":  fmt.Sprintf("%d", file.UncompressedSize64),
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
}

func (r *Reader) findOPF(fileMap map[string]*zip.File) (string, error) {
	containerFile, ok := fileMap["META-INF/container.xml"]
	if !ok {
		return "", fmt.Errorf("epub: missing META-INF/container.xml")
	}

	data, err := r.readEntry(containerFile)
	if err != nil {
		return "", fmt.Errorf("epub: reading container.xml: %w", err)
	}

	var cont container
	if err := xml.Unmarshal(data, &cont); err != nil {
		return "", fmt.Errorf("epub: parsing container.xml: %w", err)
	}

	if len(cont.Rootfiles) == 0 {
		return "", fmt.Errorf("epub: no rootfile in container.xml")
	}

	return cont.Rootfiles[0].FullPath, nil
}

func (r *Reader) parseOPF(fileMap map[string]*zip.File, opfPath string) ([]string, error) {
	opfFile, ok := fileMap[opfPath]
	if !ok {
		return nil, fmt.Errorf("epub: OPF file not found: %s", opfPath)
	}

	data, err := r.readEntry(opfFile)
	if err != nil {
		return nil, fmt.Errorf("epub: reading OPF: %w", err)
	}

	var pkg opfPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("epub: parsing OPF: %w", err)
	}

	// Build manifest ID -> href map
	idToHref := make(map[string]string)
	for _, item := range pkg.Manifest.Items {
		idToHref[item.ID] = item.Href
	}

	// Resolve spine items to file paths
	opfDir := path.Dir(opfPath)
	var items []string
	for _, ref := range pkg.Spine.ItemRefs {
		href, ok := idToHref[ref.IDRef]
		if !ok {
			continue
		}
		// Resolve relative to OPF directory
		fullPath := href
		if opfDir != "." && opfDir != "" {
			fullPath = opfDir + "/" + href
		}
		items = append(items, fullPath)
	}

	return items, nil
}

func (r *Reader) readEntry(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// extractXHTMLText parses XHTML content and extracts translatable text.
func extractXHTMLText(content []byte) []string {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var texts []string
	var textBuf strings.Builder
	inBlock := false
	depth := 0

	blockElements := map[string]bool{
		"p": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "li": true,
		"dt": true, "dd": true, "th": true, "td": true,
		"figcaption": true, "caption": true, "summary": true,
		"blockquote": true, "title": true,
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			localName := t.Name.Local
			if blockElements[localName] {
				if inBlock && textBuf.Len() > 0 {
					text := strings.TrimSpace(textBuf.String())
					if text != "" {
						texts = append(texts, text)
					}
					textBuf.Reset()
				}
				inBlock = true
				depth++
			} else if inBlock {
				depth++
			}
		case xml.EndElement:
			localName := t.Name.Local
			if blockElements[localName] {
				if textBuf.Len() > 0 {
					text := strings.TrimSpace(textBuf.String())
					if text != "" {
						texts = append(texts, text)
					}
					textBuf.Reset()
				}
				depth--
				if depth <= 0 {
					inBlock = false
					depth = 0
				}
			} else if inBlock {
				depth--
			}
		case xml.CharData:
			if inBlock {
				textBuf.Write(t)
			}
		}
	}

	// Flush remaining
	if textBuf.Len() > 0 {
		text := strings.TrimSpace(textBuf.String())
		if text != "" {
			texts = append(texts, text)
		}
	}

	return texts
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *Reader) emitError(ch chan<- model.PartResult, err error) {
	ch <- model.PartResult{Error: err}
}

// Close releases resources.
func (r *Reader) Close() error {
	r.content = nil
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
