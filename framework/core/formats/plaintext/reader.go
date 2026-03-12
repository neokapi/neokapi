package plaintext

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
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
		return fmt.Errorf("plaintext: nil document or reader")
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

	if r.cfg.SegmentByLine {
		r.readByLine(ctx, ch)
	} else {
		r.readByParagraph(ctx, ch)
	}

	r.skelFlush()

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readByLine(ctx context.Context, ch chan<- model.PartResult) {
	br := bufio.NewReader(r.Doc.Reader)
	blockID := 0
	lineNum := 0

	for {
		rawLine, err := br.ReadString('\n')
		if rawLine == "" && err != nil {
			if err != io.EOF {
				ch <- model.PartResult{Error: fmt.Errorf("plaintext: reading: %w", err)}
			}
			break
		}

		lineNum++

		// Split into content and line ending.
		// ReadString('\n') includes the delimiter; trim it to get content.
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
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", lineNum),
				Name: fmt.Sprintf("line%d", lineNum),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		} else {
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

		if err == io.EOF {
			break
		}
	}
}

func (r *Reader) readByParagraph(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("plaintext: reading: %w", err)}
		return
	}

	text := string(content)

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
