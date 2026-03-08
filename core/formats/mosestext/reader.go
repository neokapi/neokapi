package mosestext

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for Moses Text files.
// Each non-empty line becomes a translatable Block (text unit).
// Empty lines become Data parts.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new Moses Text reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "mosestext",
			FormatDisplayName: "Moses Text",
			FormatMimeType:    "text/x-mosestext",
			FormatExtensions:  []string{".txt"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/x-mosestext"},
		Extensions: []string{}, // Don't auto-detect .txt as mosestext
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("mosestext: nil document or reader")
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

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "mosestext",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-mosestext",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	lines := r.readLines()

	blockCounter := 0
	dataCounter := 0

	for _, line := range lines {
		if line == "" {
			// Empty lines become Data parts
			dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: fmt.Sprintf("empty-line%d", dataCounter),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), line)
		block.Name = fmt.Sprintf("line%d", blockCounter)
		block.PreserveWhitespace = true
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// readLines reads all lines from the document, handling CR, CRLF, and LF line endings.
func (r *Reader) readLines() []string {
	scanner := bufio.NewScanner(r.Doc.Reader)
	var lines []string

	for scanner.Scan() {
		line := scanner.Text()
		// Handle CR within lines (bufio.Scanner splits on LF, so CR-only
		// line endings appear as a single line with embedded \r characters).
		line = strings.TrimRight(line, "\r")
		// If the original text uses CR-only line endings, bufio.Scanner will
		// return the entire content as one line with \r separators. Split on \r.
		if strings.Contains(line, "\r") {
			parts := strings.Split(line, "\r")
			lines = append(lines, parts...)
		} else {
			lines = append(lines, line)
		}
	}

	return lines
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
