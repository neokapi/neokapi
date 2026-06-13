package paraplaintext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Reader implements DataFormatReader for paragraph-oriented plain text files.
// Text is split into paragraphs by blank lines (double newline).
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new paragraph plain text reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "paraplaintext",
			FormatDisplayName: "Paragraph Plain Text",
			FormatMimeType:    "text/plain",
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
	return format.FormatSignature{}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("paraplaintext: nil document or reader")
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
		Format:   "paraplaintext",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/plain",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("paraplaintext: reading: %w", err)}
		return
	}

	text := string(content)
	if text == "" {
		r.skelFlush()
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
		return
	}

	if r.skeletonStore != nil {
		r.readContentSkeleton(ctx, ch, text)
	} else {
		r.readContentNormal(ctx, ch, text)
	}

	r.skelFlush()
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readContentNormal(ctx context.Context, ch chan<- model.PartResult, text string) {
	// Normalize line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")

	// Split on double newlines to get paragraphs
	segments := strings.Split(text, "\n\n")

	blockID := 0
	dataID := 0

	for i, segment := range segments {
		if segment == "" {
			// Additional blank line separators
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "paragraph-separator",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockID), segment)
		block.Name = fmt.Sprintf("para%d", blockID)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}

		// Emit separator between paragraphs (not after last)
		if i < len(segments)-1 {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "paragraph-separator",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
	}
}

// readContentSkeleton handles reading with skeleton store active, preserving
// exact line endings for byte-exact roundtrips.
func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult, text string) {
	// Parse into raw lines preserving line endings.
	type rawLine struct {
		content    string
		lineEnding string
	}
	var lines []rawLine
	remaining := text
	for len(remaining) > 0 {
		idx := strings.Index(remaining, "\n")
		if idx < 0 {
			lines = append(lines, rawLine{content: remaining})
			break
		}
		lineContent := remaining[:idx]
		ending := "\n"
		if strings.HasSuffix(lineContent, "\r") {
			lineContent = lineContent[:len(lineContent)-1]
			ending = "\r\n"
		}
		lines = append(lines, rawLine{content: lineContent, lineEnding: ending})
		remaining = remaining[idx+1:]
	}

	// Group lines into paragraphs separated by blank lines.
	type paragraph struct {
		text           string // joined content (internal newlines use \n)
		lastLineEnding string // line ending after the last line
	}

	var paragraphs []paragraph
	var separatorsBetween []string // separator text between paragraph i and i+1
	var leadingSep strings.Builder
	var curLines []rawLine

	flushParagraph := func() {
		if len(curLines) == 0 {
			return
		}
		var sb strings.Builder
		for i, l := range curLines {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(l.content)
		}
		lastEnding := curLines[len(curLines)-1].lineEnding
		paragraphs = append(paragraphs, paragraph{text: sb.String(), lastLineEnding: lastEnding})
		curLines = nil
	}

	var sepBuf strings.Builder
	seenContent := false

	for _, l := range lines {
		if l.content == "" {
			if len(curLines) > 0 {
				flushParagraph()
				sepBuf.WriteString(l.lineEnding)
			} else if !seenContent {
				leadingSep.WriteString(l.lineEnding)
			} else {
				sepBuf.WriteString(l.lineEnding)
			}
		} else {
			seenContent = true
			if sepBuf.Len() > 0 {
				separatorsBetween = append(separatorsBetween, sepBuf.String())
				sepBuf.Reset()
			}
			curLines = append(curLines, l)
		}
	}
	flushParagraph()
	trailingSep := sepBuf.String()

	// Emit parts with skeleton entries
	blockID := 0
	dataID := 0

	if leadingSep.Len() > 0 {
		r.skelText(leadingSep.String())
	}

	for i, para := range paragraphs {
		blockID++
		blockIDStr := fmt.Sprintf("tu%d", blockID)

		r.skelRef(blockIDStr)
		r.skelText(para.lastLineEnding)

		block := model.NewBlock(blockIDStr, para.text)
		block.Name = fmt.Sprintf("para%d", blockID)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}

		if i < len(separatorsBetween) {
			r.skelText(separatorsBetween[i])

			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "paragraph-separator",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
	}

	if trailingSep != "" {
		r.skelText(trailingSep)
	}
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
