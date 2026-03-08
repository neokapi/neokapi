package versifiedtext

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// versePattern matches verse markers at the start of a line.
// Supports formats: \v1, \v 1, \v12, or plain numbers like "1 " or "1." at start.
var versePattern = regexp.MustCompile(`^(?:\\v\s*(\d+)\s+|(\d+)[.\s]\s*)(.*)$`)

// Reader implements DataFormatReader for versified text (poetry/scripture).
// Lines with verse markers become separate Blocks with verse metadata.
// Blank lines separate stanzas and are emitted as Data parts.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new versified text reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "versifiedtext",
			FormatDisplayName: "Versified Text",
			FormatMimeType:    "text/plain",
			FormatExtensions:  []string{".txt", ".ver"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		Extensions: []string{".ver"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("versifiedtext: nil document or reader")
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
		Format:   "versifiedtext",
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

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, "\r")

		// Blank lines are stanza separators (Data)
		if strings.TrimSpace(line) == "" {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("stanza-break.%d", dataID),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		// Try to match verse marker
		matches := versePattern.FindStringSubmatch(line)
		if matches != nil {
			verseNum := matches[1]
			if verseNum == "" {
				verseNum = matches[2]
			}
			text := matches[3]

			blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockID), text)
			block.Name = fmt.Sprintf("verse.%s", verseNum)
			block.Properties["verse"] = verseNum
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		} else {
			// Non-verse line becomes a plain Block
			blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockID), line)
			block.Name = fmt.Sprintf("line%d", blockID)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("versifiedtext: reading: %w", err)}
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
