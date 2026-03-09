package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for ZIP archive files.
type Reader struct {
	format.BaseFormatReader
	cfg     *Config
	content []byte
}

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

	// Read all content into memory since zip.NewReader needs a ReaderAt.
	data, err := io.ReadAll(doc.Reader)
	if err != nil {
		return fmt.Errorf("archive: reading document: %w", err)
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

	zipReader, err := zip.NewReader(bytes.NewReader(r.content), int64(len(r.content)))
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
			// Text file: emit as child layer with blocks
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

			content, err := r.readEntry(file)
			if err != nil {
				r.emitError(ch, fmt.Errorf("archive: reading entry %s: %w", file.Name, err))
				return
			}

			// Emit each non-empty line as a block
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				blockCounter++
				block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), trimmed)
				block.Name = file.Name
				block.Properties["entry"] = file.Name
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}

			if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer}) {
				return
			}
		} else {
			// Binary file: emit as Data
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
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
}

func (r *Reader) readEntry(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
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
