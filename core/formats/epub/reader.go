package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Reader implements DataFormatReader for EPUB e-book files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	resolver      format.SubfilterResolver
	tmpFile       string // path to temp file backing the ZIP
	skeletonStore *format.SkeletonStore
	layerSeq      int // counter for generating unique child layer IDs
}

var _ format.SkeletonStoreEmitter = (*Reader)(nil)
var _ format.SubfilterAware = (*Reader)(nil)

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
// Content is written to a temp file instead of holding the entire ZIP in memory.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("epub: nil document or reader")
	}
	r.Doc = doc

	// Write content to a temp file so zip.OpenReader can use it
	f, err := os.CreateTemp("", "neokapi-epub-*.zip")
	if err != nil {
		return fmt.Errorf("epub: creating temp file: %w", err)
	}
	r.tmpFile = f.Name()

	if _, err := io.Copy(f, doc.Reader); err != nil {
		f.Close()
		os.Remove(r.tmpFile)
		r.tmpFile = ""
		return fmt.Errorf("epub: writing temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(r.tmpFile)
		r.tmpFile = ""
		return fmt.Errorf("epub: closing temp file: %w", err)
	}

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

// Skeleton part-boundary markers. The writer uses these to split the
// single skeleton stream into per-ZIP-entry segments.
const (
	skelPartStartPrefix = "@@SKEL_PART_START@@"
	skelPartEndPrefix   = "@@SKEL_PART_END@@"
)

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

	zr, err := zip.OpenReader(r.tmpFile)
	if err != nil {
		r.emitError(ch, fmt.Errorf("epub: opening zip: %w", err))
		return
	}
	defer zr.Close()

	// Validate the archive against the shared safeio budget before reading any
	// entry; per-entry reads are additionally bounded in readEntry.
	if err := safeio.DefaultZipLimits.CheckReader(&zr.Reader); err != nil {
		r.emitError(ch, fmt.Errorf("epub: %w", err))
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

		content, err := r.readEntry(file)
		if err != nil {
			r.emitError(ch, fmt.Errorf("epub: reading %s: %w", itemPath, err))
			return
		}

		// When a subfilter resolver is available, route XHTML through the HTML reader
		if r.resolver != nil {
			r.layerSeq++
			childLayerID := fmt.Sprintf("sf%d", r.layerSeq)

			childLayer := &model.Layer{
				ID:       childLayerID,
				Name:     itemPath,
				Format:   "html",
				Locale:   locale,
				ParentID: rootLayer.ID,
				MimeType: "application/xhtml+xml",
				Properties: map[string]string{
					"subfilter.source": "epub",
					"entry":            itemPath,
				},
			}

			// Emit skeleton part-boundary marker for subfiltered content
			r.skelPartStart(itemPath)
			if r.skeletonStore != nil {
				_ = r.skeletonStore.WriteRef("layer:" + itemPath)
			}
			r.skelPartEnd(itemPath)

			r.emitSubfiltered(ctx, ch, content, itemPath, rootLayer.ID, childLayer, &blockCounter)
			continue
		}

		// Fallback: extract XHTML text directly
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

		// Emit skeleton part-boundary marker
		r.skelPartStart(itemPath)

		r.extractAndEmitXHTML(ctx, ch, content, itemPath, &blockCounter)

		// Flush and close skeleton part
		r.skelFlush()
		r.skelPartEnd(itemPath)

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
				"size":  strconv.FormatUint(file.UncompressedSize64, 10),
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
		return "", errors.New("epub: missing META-INF/container.xml")
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
		return "", errors.New("epub: no rootfile in container.xml")
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
	// Bounded by the shared safeio zip limits (per-entry uncompressed size +
	// inflate-ratio zip-bomb guard on the actual decompressed stream).
	return safeio.DefaultZipLimits.ReadEntry(file)
}

// extractAndEmitXHTML parses XHTML content and extracts translatable text,
// writing skeleton data (structure) and emitting blocks (translatable text).
func (r *Reader) extractAndEmitXHTML(ctx context.Context, ch chan<- model.PartResult, content []byte, itemPath string, blockCounter *int) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var skelBuf bytes.Buffer // accumulates skeleton text between block refs
	var textBuf strings.Builder
	inBlock := false
	depth := 0

	// Pending tokens for skeleton reconstruction within a block
	var pendingTokens []xml.Token

	blockElements := map[string]bool{
		"p": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "li": true,
		"dt": true, "dd": true, "th": true, "td": true,
		"figcaption": true, "caption": true, "summary": true,
		"blockquote": true, "title": true,
	}

	// writeSkelToken serializes a token to skeleton-format XML text.
	writeSkelToken := func(tok xml.Token) {
		if r.skeletonStore == nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			skelBuf.WriteString("<")
			writeXMLName(&skelBuf, t.Name)
			for _, a := range t.Attr {
				skelBuf.WriteString(" ")
				writeXMLName(&skelBuf, a.Name)
				skelBuf.WriteString(`="`)
				skelBuf.WriteString(xmlEscapeAttr(a.Value))
				skelBuf.WriteString(`"`)
			}
			skelBuf.WriteString(">")
		case xml.EndElement:
			skelBuf.WriteString("</")
			writeXMLName(&skelBuf, t.Name)
			skelBuf.WriteString(">")
		case xml.CharData:
			skelBuf.WriteString(xmlEscape(string(t)))
		case xml.ProcInst:
			skelBuf.WriteString("<?" + t.Target)
			if len(t.Inst) > 0 {
				skelBuf.WriteString(" " + string(t.Inst))
			}
			skelBuf.WriteString("?>")
		case xml.Comment:
			skelBuf.WriteString("<!--" + string(t) + "-->")
		case xml.Directive:
			skelBuf.WriteString("<!" + string(t) + ">")
		}
	}

	flushBlock := func() {
		if textBuf.Len() > 0 {
			text := strings.TrimSpace(textBuf.String())
			if text != "" {
				*blockCounter++
				blockID := fmt.Sprintf("tu%d", *blockCounter)
				block := model.NewBlock(blockID, text)
				block.Name = itemPath
				block.Properties["entry"] = itemPath

				if r.skeletonStore != nil {
					// Write skeleton: open tags from pending tokens, ref, close tags
					// We reconstruct the element wrappers around the ref.
					var openTokens []xml.Token
					var closeTokens []xml.Token
					for _, tok := range pendingTokens {
						switch tok.(type) {
						case xml.StartElement:
							openTokens = append(openTokens, tok)
						case xml.EndElement:
							closeTokens = append(closeTokens, tok)
						}
					}
					for _, tok := range openTokens {
						writeSkelToken(tok)
					}
					// Flush accumulated skeleton text, then write ref
					if skelBuf.Len() > 0 {
						_ = r.skeletonStore.WriteText(skelBuf.Bytes())
						skelBuf.Reset()
					}
					_ = r.skeletonStore.WriteRef(blockID)
					for _, tok := range closeTokens {
						writeSkelToken(tok)
					}
				}

				r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
			} else {
				// Empty block — write all pending tokens as skeleton text
				for _, tok := range pendingTokens {
					writeSkelToken(tok)
				}
			}
			textBuf.Reset()
			pendingTokens = nil
		} else {
			// No text accumulated — write any pending tokens as skeleton text
			for _, tok := range pendingTokens {
				writeSkelToken(tok)
			}
			pendingTokens = nil
		}
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
					flushBlock()
				}
				inBlock = true
				depth++
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else if inBlock {
				depth++
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else {
				writeSkelToken(t)
			}
		case xml.EndElement:
			localName := t.Name.Local
			if blockElements[localName] {
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
				flushBlock()
				depth--
				if depth <= 0 {
					inBlock = false
					depth = 0
				}
			} else if inBlock {
				depth--
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else {
				writeSkelToken(t)
			}
		case xml.CharData:
			if inBlock {
				textBuf.Write(t)
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else {
				writeSkelToken(t)
			}
		case xml.ProcInst:
			writeSkelToken(t)
		case xml.Comment:
			writeSkelToken(t)
		case xml.Directive:
			writeSkelToken(t)
		}
	}

	// Flush remaining
	flushBlock()
}

// emitSubfiltered emits a child layer with content parsed by the HTML sub-format reader.
func (r *Reader) emitSubfiltered(ctx context.Context, ch chan<- model.PartResult,
	content []byte, entryName, parentLayerID string,
	childLayer *model.Layer, blockCounter *int) {

	subReader, err := r.resolver.ResolveReader("html")
	if err != nil {
		// Fall back to direct XHTML extraction if HTML reader is unavailable
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
			return
		}
		r.extractAndEmitXHTML(ctx, ch, content, entryName, blockCounter)
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
		ch <- model.PartResult{Error: fmt.Errorf("epub: subfilter open for %s: %w", entryName, err)}
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
		return
	}

	// Read sub-reader parts, skipping the sub-reader's own root layer start/end
	for pr := range subReader.Read(ctx) {
		if pr.Error != nil {
			ch <- model.PartResult{Error: fmt.Errorf("epub: subfilter read for %s: %w", entryName, pr.Error)}
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

func (r *Reader) skelFlush() {
	// No-op: skeleton buffer is flushed inline during extractAndEmitXHTML
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
	if r.tmpFile != "" {
		os.Remove(r.tmpFile)
		r.tmpFile = ""
	}
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

// writeXMLName writes an xml.Name as a prefixed string (e.g. "ns:local").
func writeXMLName(buf *bytes.Buffer, name xml.Name) {
	if name.Space != "" {
		// For well-known XHTML namespace, omit prefix
		if name.Space == "http://www.w3.org/1999/xhtml" {
			buf.WriteString(name.Local)
			return
		}
		buf.WriteString(name.Space)
		buf.WriteString(":")
	}
	buf.WriteString(name.Local)
}

// xmlEscape escapes text for XML content.
func xmlEscape(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// xmlEscapeAttr escapes text for XML attribute values.
func xmlEscapeAttr(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&quot;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
