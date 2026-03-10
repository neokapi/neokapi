package mosestext

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for Moses Text files.
// Each non-empty line becomes a translatable Block (text unit).
// Empty lines become Data parts.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

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

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
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

	if r.skeletonStore != nil {
		r.readLinesSkeleton(ctx, ch)
	} else {
		r.readLinesNormal(ctx, ch)
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// readLinesNormal reads all lines without skeleton tracking.
func (r *Reader) readLinesNormal(ctx context.Context, ch chan<- model.PartResult) {
	lines := r.scanLines()

	blockCounter := 0
	dataCounter := 0

	for _, line := range lines {
		if line == "" {
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
}

// readLinesSkeleton reads lines while recording skeleton entries for byte-exact roundtrip.
func (r *Reader) readLinesSkeleton(ctx context.Context, ch chan<- model.PartResult) {
	br := bufio.NewReader(r.Doc.Reader)
	blockCounter := 0
	dataCounter := 0

	for {
		rawLine, err := br.ReadString('\n')
		if rawLine == "" && err != nil {
			if err != io.EOF {
				ch <- model.PartResult{Error: fmt.Errorf("mosestext: reading: %w", err)}
			}
			break
		}

		// Split into content and line ending.
		content := rawLine
		lineEnding := ""
		if strings.HasSuffix(content, "\r\n") {
			content = content[:len(content)-2]
			lineEnding = "\r\n"
		} else if strings.HasSuffix(content, "\n") {
			content = content[:len(content)-1]
			lineEnding = "\n"
		}

		if content == "" {
			// Empty line is non-translatable data
			r.skelText(lineEnding)
			dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: fmt.Sprintf("empty-line%d", dataCounter),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		} else {
			blockCounter++
			blockIDStr := fmt.Sprintf("tu%d", blockCounter)
			r.skelRef(blockIDStr)
			r.skelText(lineEnding)
			block := model.NewBlock(blockIDStr, content)
			block.Name = fmt.Sprintf("line%d", blockCounter)
			block.PreserveWhitespace = true
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}

		if err == io.EOF {
			break
		}
	}
}

// scanLines reads all lines from the document, handling CR, CRLF, and LF line endings.
func (r *Reader) scanLines() []string {
	scanner := bufio.NewScanner(r.Doc.Reader)
	var lines []string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, "\r")
		if strings.Contains(line, "\r") {
			parts := strings.Split(line, "\r")
			lines = append(lines, parts...)
		} else {
			lines = append(lines, line)
		}
	}

	return lines
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
