package archive

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for ZIP archive files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	resolver      format.SubfilterResolver
	tmpFile       *os.File
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	layerSeq      int // counter for generating unique child layer IDs
}

// Ensure Reader implements SkeletonStoreEmitter and SubfilterAware.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)
var _ format.SubfilterAware = (*Reader)(nil)

// NewReader creates a new archive reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "archive",
			FormatDisplayName: "ZIP Archive",
			FormatMimeType:    "application/zip",
			FormatExtensions:  []string{".zip"},
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
		MIMETypes:  []string{"application/zip", "application/x-zip-compressed"},
		Extensions: []string{".zip"},
		MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}}, // PK\x03\x04
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("archive: nil document or reader")
	}
	r.Doc = doc

	// Write content to a temp file instead of holding the entire ZIP in memory.
	tmpFile, err := os.CreateTemp("", "gokapi-archive-*")
	if err != nil {
		return fmt.Errorf("archive: creating temp file: %w", err)
	}
	if _, err := io.Copy(tmpFile, doc.Reader); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return fmt.Errorf("archive: writing temp file: %w", err)
	}
	r.tmpFile = tmpFile
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

	// Emit root layer
	rootLayer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "archive",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/zip",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: rootLayer}) {
		return
	}

	// Open ZIP from temp file
	fileInfo, err := r.tmpFile.Stat()
	if err != nil {
		r.emitError(ch, fmt.Errorf("archive: stat temp file: %w", err))
		return
	}
	if _, err := r.tmpFile.Seek(0, io.SeekStart); err != nil {
		r.emitError(ch, fmt.Errorf("archive: seek temp file: %w", err))
		return
	}
	zipReader, err := zip.NewReader(r.tmpFile, fileInfo.Size())
	if err != nil {
		r.emitError(ch, fmt.Errorf("archive: opening zip: %w", err))
		return
	}

	patterns := r.cfg.FilePatterns
	if len(patterns) == 0 {
		patterns = defaultTextPatterns()
	}

	blockCounter := 0
	dataCounter := 0
	layerCounter := 1 // doc1 is 1

	for _, file := range zipReader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		if r.isTextFile(file.Name, patterns) {
			// Check if this file should be routed through a sub-format reader
			if mapping := r.matchSubfilter(file.Name); mapping != nil && r.resolver != nil {
				r.layerSeq++
				childLayerID := fmt.Sprintf("sf%d", r.layerSeq)

				childLayer := &model.Layer{
					ID:       childLayerID,
					Name:     file.Name,
					Format:   mapping.Format,
					Locale:   locale,
					ParentID: rootLayer.ID,
					Properties: map[string]string{
						"subfilter.source": "archive",
						"entry":            file.Name,
					},
				}

				rc, err := file.Open()
				if err != nil {
					r.emitError(ch, fmt.Errorf("archive: opening entry %s: %w", file.Name, err))
					return
				}
				content, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					r.emitError(ch, fmt.Errorf("archive: reading entry %s: %w", file.Name, err))
					return
				}

				// Write skeleton marker for subfiltered entry
				r.skelText("<<SUBFILTER:" + file.Name + ">>\n")
				r.skelRef("layer:" + file.Name)
				r.skelText("\n")

				r.emitSubfiltered(ctx, ch, content, file.Name, rootLayer.ID, mapping, childLayer, &blockCounter)
				continue
			}

			// Text file: emit as child layer with blocks (fallback line-by-line)
			layerCounter++
			childLayer := &model.Layer{
				ID:       fmt.Sprintf("layer%d", layerCounter),
				Name:     file.Name,
				Format:   "archive",
				Locale:   locale,
				ParentID: rootLayer.ID,
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
				return
			}

			// Write skeleton marker for entry start
			r.skelText("<<ENTRY:" + file.Name + ">>\n")

			// Stream lines from the ZIP entry
			rc, err := file.Open()
			if err != nil {
				r.emitError(ch, fmt.Errorf("archive: opening entry %s: %w", file.Name, err))
				return
			}

			scanner := bufio.NewScanner(rc)
			for scanner.Scan() {
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					r.skelText(line + "\n")
					continue
				}
				blockCounter++
				blockIDStr := fmt.Sprintf("tu%d", blockCounter)
				r.skelRef(blockIDStr)
				r.skelText("\n")
				block := model.NewBlock(blockIDStr, trimmed)
				block.Name = file.Name
				block.Properties["entry"] = file.Name
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					rc.Close()
					return
				}
			}
			if err := scanner.Err(); err != nil {
				rc.Close()
				r.emitError(ch, fmt.Errorf("archive: reading entry %s: %w", file.Name, err))
				return
			}
			rc.Close()

			if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer}) {
				return
			}
		} else {
			// Binary file: emit as Data, skeleton marker for binary entry
			dataCounter++
			r.skelText("<<BINARY:" + file.Name + ">>\n")
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
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
}

func (r *Reader) isTextFile(name string, patterns []string) bool {
	base := filepath.Base(name)
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
	}
	return false
}

// skelText appends text to the skeleton buffer if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		r.skelBuf.WriteString(s)
	}
}

// skelRef flushes buffered text and writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

// matchSubfilter checks if the given file name matches any configured subfilter mapping.
// Falls back to default mappings when no explicit mappings are configured.
func (r *Reader) matchSubfilter(fileName string) *format.SubfilterMapping {
	mappings := r.cfg.SubfilterMappings
	if len(mappings) == 0 {
		mappings = DefaultSubfilterMappings()
	}
	base := filepath.Base(fileName)
	for i := range mappings {
		sf := &mappings[i]
		if matched, _ := filepath.Match(sf.Pattern, base); matched {
			return sf
		}
	}
	return nil
}

// emitSubfiltered emits a child layer with content parsed by the subfilter format reader.
func (r *Reader) emitSubfiltered(ctx context.Context, ch chan<- model.PartResult,
	content []byte, entryName, parentLayerID string, mapping *format.SubfilterMapping,
	childLayer *model.Layer, blockCounter *int) {

	subReader, err := r.resolver.ResolveReader(mapping.Format)
	if err != nil {
		// Fall back to line-by-line extraction if sub-reader unavailable
		r.emitLineByLine(ctx, ch, content, entryName, parentLayerID, blockCounter)
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
		ch <- model.PartResult{Error: fmt.Errorf("archive: subfilter open for %s: %w", entryName, err)}
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
		return
	}

	// Read sub-reader parts, skipping the sub-reader's own root layer start/end
	for pr := range subReader.Read(ctx) {
		if pr.Error != nil {
			ch <- model.PartResult{Error: fmt.Errorf("archive: subfilter read for %s: %w", entryName, pr.Error)}
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

// emitLineByLine is the fallback when subfilter fails — emits content as plain blocks.
func (r *Reader) emitLineByLine(ctx context.Context, ch chan<- model.PartResult,
	content []byte, entryName, parentLayerID string, blockCounter *int) {

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	childLayer := &model.Layer{
		ID:       fmt.Sprintf("fallback-%s", entryName),
		Name:     entryName,
		Format:   "archive",
		Locale:   locale,
		ParentID: parentLayerID,
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
		return
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		*blockCounter++
		blockIDStr := fmt.Sprintf("tu%d", *blockCounter)
		block := model.NewBlock(blockIDStr, trimmed)
		block.Name = entryName
		block.Properties["entry"] = entryName
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
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
	if r.tmpFile != nil {
		name := r.tmpFile.Name()
		r.tmpFile.Close()
		os.Remove(name)
		r.tmpFile = nil
	}
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
