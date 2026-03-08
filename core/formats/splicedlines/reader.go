package splicedlines

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for line-spliced text files.
// Lines ending with backslash (\) are continued on the next line.
// Continuation lines are joined into a single Block.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new spliced lines reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "splicedlines",
			FormatDisplayName: "Spliced Lines",
			FormatMimeType:    "text/plain",
			FormatExtensions:  []string{".txt"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("splicedlines: nil document or reader")
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
		Format:   "splicedlines",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/plain",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	scanner := bufio.NewScanner(r.Doc.Reader)
	blockID := 0
	dataID := 0

	var accumulated []string

	flushBlock := func() bool {
		if len(accumulated) == 0 {
			return true
		}
		joined := strings.Join(accumulated, "\n")
		accumulated = nil

		if strings.TrimSpace(joined) == "" {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("empty.%d", dataID),
			}
			return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		}

		blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockID), joined)
		block.Name = fmt.Sprintf("block%d", blockID)
		block.Properties["continued"] = fmt.Sprintf("%d", len(accumulated))
		return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, "\r")

		if strings.HasSuffix(line, `\`) {
			// Continuation line: strip trailing backslash and accumulate
			accumulated = append(accumulated, strings.TrimSuffix(line, `\`))
		} else {
			// Non-continuation: add to accumulator and flush
			accumulated = append(accumulated, line)
			if !flushBlock() {
				return
			}
		}
	}

	// Flush any remaining accumulated lines
	if len(accumulated) > 0 {
		if !flushBlock() {
			return
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("splicedlines: reading: %w", err)}
		return
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
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
