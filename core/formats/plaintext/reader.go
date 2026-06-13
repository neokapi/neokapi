package plaintext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	coreenc "github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Reader implements DataFormatReader for plain text files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new plain text reader.
func NewReader() *Reader {
	cfg := &Config{SegmentByLine: true}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "plaintext",
			FormatDisplayName: "Plain Text",
			FormatMimeType:    "text/plain",
			FormatExtensions:  []string{".txt", ".text"},
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
		MIMETypes:  []string{"text/plain"},
		Extensions: []string{".txt", ".text"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("plaintext: nil document or reader")
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

	// Emit layer start
	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "plaintext",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/plain",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Buffer + transcode upfront so UTF-16-with-BOM fixtures (e.g.
	// BOM_MacUTF16withBOM2.txt) get split on '\n' as UTF-8 instead of
	// snagging the high byte of each UTF-16 codepoint.
	raw, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("plaintext: reading: %w", err)}
		return
	}
	utf8Bytes, _, err := coreenc.ToUTF8(raw)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("plaintext: transcoding to UTF-8: %w", err)}
		return
	}

	if r.cfg.SegmentByLine {
		r.readByLine(ctx, ch, string(utf8Bytes))
	} else {
		r.readByParagraph(ctx, ch, string(utf8Bytes))
	}

	r.skelFlush()

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readByLine(ctx context.Context, ch chan<- model.PartResult, text string) {
	blockID := 0
	lineNum := 0
	remaining := text

	for len(remaining) > 0 {
		lineNum++

		content, lineEnding, rest := nextPlainLine(remaining)
		remaining = rest

		if content == "" {
			r.skelText(lineEnding)
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", lineNum),
				Name: fmt.Sprintf("line%d", lineNum),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		blockID++
		blockIDStr := fmt.Sprintf("tu%d", blockID)
		r.skelRef(blockIDStr)
		r.skelText(lineEnding)
		block := model.NewBlock(blockIDStr, content)
		block.Name = fmt.Sprintf("line%d", lineNum)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// nextPlainLine peels one line off the front of text, returning the
// content, the line-ending bytes (\n, \r\n, \r, or "" at EOF), and the
// remaining unread text. Recognises bare \r as a line terminator so
// Mac-classic / UTF-16-derived fixtures don't get mashed into one
// gigantic block.
func nextPlainLine(text string) (content, ending, rest string) {
	for i := range len(text) {
		switch text[i] {
		case '\n':
			return text[:i], "\n", text[i+1:]
		case '\r':
			if i+1 < len(text) && text[i+1] == '\n' {
				return text[:i], "\r\n", text[i+2:]
			}
			return text[:i], "\r", text[i+1:]
		}
	}
	return text, "", ""
}

func (r *Reader) readByParagraph(ctx context.Context, ch chan<- model.PartResult, text string) {
	if r.skeletonStore != nil {
		r.readByParagraphSkeleton(ctx, ch, text)
		return
	}

	paragraphs := strings.Split(text, "\n\n")
	blockID := 0

	for i, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockID), para)
		block.Name = fmt.Sprintf("para%d", blockID)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}

		// Emit separator data between paragraphs (not after last)
		if i < len(paragraphs)-1 {
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", i+1),
				Name: "paragraph-separator",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
	}
}

// readByParagraphSkeleton handles paragraph mode with skeleton store active.
// It scans line by line to preserve exact line endings, grouping non-empty
// lines into paragraphs separated by blank lines.
func (r *Reader) readByParagraphSkeleton(ctx context.Context, ch chan<- model.PartResult, text string) {
	// Split into raw lines preserving line endings.
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

	// Group lines into paragraphs. A paragraph is a sequence of non-empty lines.
	// Between paragraphs we track the exact separator bytes (line endings of
	// the last content line + empty line endings).
	type paragraph struct {
		text           string // joined content (internal newlines use \n)
		lastLineEnding string // line ending after the last line of the paragraph
	}

	var paragraphs []paragraph
	// separatorsBetween[i] = separator text between paragraph i and i+1
	var separatorsBetween []string
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
				// End of a paragraph: flush it, start accumulating separator
				flushParagraph()
				// The last line ending of the paragraph was captured in flushParagraph.
				// The empty line's ending is part of the separator.
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

	// Leading empty lines
	if leadingSep.Len() > 0 {
		r.skelText(leadingSep.String())
	}

	for i, para := range paragraphs {
		blockID++
		blockIDStr := fmt.Sprintf("tu%d", blockID)

		r.skelRef(blockIDStr)
		// Write the line ending after this paragraph's last line
		r.skelText(para.lastLineEnding)

		block := model.NewBlock(blockIDStr, para.text)
		block.Name = fmt.Sprintf("para%d", blockID)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}

		// Emit separator between paragraphs
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
