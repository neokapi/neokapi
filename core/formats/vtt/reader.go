package vtt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for WebVTT subtitle files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new VTT reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "vtt",
			FormatDisplayName: "WebVTT",
			FormatMimeType:    "text/vtt",
			FormatExtensions:  []string{".vtt"},
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
		MIMETypes:  []string{"text/vtt"},
		Extensions: []string{".vtt"},
		MagicBytes: [][]byte{[]byte("WEBVTT")},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("vtt: nil document or reader")
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

// vttCue represents a single VTT cue (subtitle entry).
type vttCue struct {
	identifier string
	timecode   string
	text       string
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "vtt",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/vtt",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	if r.skeletonStore != nil {
		r.readContentSkeleton(ctx, ch)
	} else {
		r.readContentSimple(ctx, ch)
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readContentSimple(ctx context.Context, ch chan<- model.PartResult) {
	cues, header := r.parseCues()

	dataCounter := 0

	// Emit WEBVTT header as Data
	dataCounter++
	headerData := &model.Data{
		ID:   fmt.Sprintf("d%d", dataCounter),
		Name: "vtt-header",
		Properties: map[string]string{
			"content": header,
		},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: headerData}) {
		return
	}

	blockCounter := 0

	for i, cue := range cues {
		// Emit cue identifier as Data if present
		if cue.identifier != "" {
			dataCounter++
			idData := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: fmt.Sprintf("cue-id.%d", i+1),
				Properties: map[string]string{
					"identifier": cue.identifier,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: idData}) {
				return
			}
		}

		// Emit cue text as Block
		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), cue.text)
		block.Name = fmt.Sprintf("subtitle.%d", i+1)
		block.Properties["timecode"] = cue.timecode
		setBlockTiming(block, cue.timecode)
		if cue.identifier != "" {
			block.Properties["cue-id"] = cue.identifier
		}
		block.Properties["index"] = strconv.Itoa(i + 1)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// readContentSkeleton reads the VTT with skeleton tracking, preserving exact bytes.
func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult) {
	// Read the full content to preserve exact bytes
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("vtt: reading: %w", err)}
		return
	}

	lines := splitRawLines(data)
	lineIdx := 0
	dataCounter := 0
	blockCounter := 0
	cueIndex := 0

	// Read the WEBVTT header line
	header := ""
	if lineIdx < len(lines) {
		header = lines[lineIdx].content
		r.skelText(lines[lineIdx].raw)
		lineIdx++
	}

	// Emit WEBVTT header as Data
	dataCounter++
	headerData := &model.Data{
		ID:   fmt.Sprintf("d%d", dataCounter),
		Name: "vtt-header",
		Properties: map[string]string{
			"content": header,
		},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: headerData}) {
		return
	}

	// Skip blank lines after header
	for lineIdx < len(lines) && strings.TrimSpace(lines[lineIdx].content) == "" {
		r.skelText(lines[lineIdx].raw)
		lineIdx++
	}

	// Parse cues
	for lineIdx < len(lines) {
		// Skip blank lines between cues
		if strings.TrimSpace(lines[lineIdx].content) == "" {
			r.skelText(lines[lineIdx].raw)
			lineIdx++
			continue
		}

		cueIndex++
		cue := &vttCue{}

		// Determine if this line is a timecode or a cue identifier
		if isTimecode(lines[lineIdx].content) {
			cue.timecode = lines[lineIdx].content
			r.skelText(lines[lineIdx].raw)
			lineIdx++
		} else {
			// It's a cue identifier
			cue.identifier = lines[lineIdx].content
			r.skelText(lines[lineIdx].raw)
			lineIdx++

			// Next line should be the timecode
			if lineIdx < len(lines) {
				cue.timecode = lines[lineIdx].content
				r.skelText(lines[lineIdx].raw)
				lineIdx++
			}

			// Emit cue identifier as Data
			dataCounter++
			idData := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: fmt.Sprintf("cue-id.%d", cueIndex),
				Properties: map[string]string{
					"identifier": cue.identifier,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: idData}) {
				return
			}
		}

		// Read text lines until blank line or EOF
		var textLines []string
		textStartIdx := lineIdx
		for lineIdx < len(lines) && strings.TrimSpace(lines[lineIdx].content) != "" {
			textLines = append(textLines, lines[lineIdx].content)
			lineIdx++
		}

		// Join text lines using the original line endings between them
		// so that CRLF intermediate separators are preserved in the block text.
		var textBuilder strings.Builder
		for i, tl := range textLines {
			textBuilder.WriteString(tl)
			if i < len(textLines)-1 {
				// Use the actual line ending from this line as separator
				ending := lines[textStartIdx+i].lineEnding
				if ending == "" {
					ending = "\n"
				}
				textBuilder.WriteString(ending)
			}
		}
		cue.text = textBuilder.String()

		// Write skeleton ref for the block
		blockCounter++
		blockIDStr := fmt.Sprintf("tu%d", blockCounter)
		r.skelRef(blockIDStr)

		// Only the last text line's ending is skeleton text
		lastTextIdx := textStartIdx + len(textLines) - 1
		if lastTextIdx >= textStartIdx {
			r.skelText(lines[lastTextIdx].lineEnding)
		}

		// Emit cue text as Block
		block := model.NewBlock(blockIDStr, cue.text)
		block.Name = fmt.Sprintf("subtitle.%d", cueIndex)
		block.Properties["timecode"] = cue.timecode
		setBlockTiming(block, cue.timecode)
		if cue.identifier != "" {
			block.Properties["cue-id"] = cue.identifier
		}
		block.Properties["index"] = strconv.Itoa(cueIndex)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// rawLine holds a line with its original line ending preserved.
type rawLine struct {
	content    string // line content without line ending
	lineEnding string // "\n", "\r\n", or ""
	raw        string // content + lineEnding (full original bytes)
}

// splitRawLines splits data into lines preserving exact line endings.
func splitRawLines(data []byte) []rawLine {
	var lines []rawLine
	remaining := string(data)
	for len(remaining) > 0 {
		idx := strings.Index(remaining, "\n")
		if idx < 0 {
			lines = append(lines, rawLine{content: remaining, raw: remaining})
			break
		}
		content := remaining[:idx]
		ending := "\n"
		if strings.HasSuffix(content, "\r") {
			content = content[:len(content)-1]
			ending = "\r\n"
		}
		raw := remaining[:idx+1]
		lines = append(lines, rawLine{content: content, lineEnding: ending, raw: raw})
		remaining = remaining[idx+1:]
	}
	return lines
}

func (r *Reader) parseCues() ([]*vttCue, string) {
	scanner := bufio.NewScanner(r.Doc.Reader)
	var cues []*vttCue
	header := ""

	// Read the WEBVTT header line
	if scanner.Scan() {
		header = strings.TrimRight(scanner.Text(), "\r")
	}

	// Skip blank lines after header
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) != "" {
			// This is the start of the first cue
			cue := r.parseCue(scanner, line)
			if cue != nil {
				cues = append(cues, cue)
			}
			break
		}
	}

	// Parse remaining cues
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		cue := r.parseCue(scanner, line)
		if cue != nil {
			cues = append(cues, cue)
		}
	}

	return cues, header
}

// parseCue parses a single VTT cue starting from the given first non-empty line.
func (r *Reader) parseCue(scanner *bufio.Scanner, firstLine string) *vttCue {
	cue := &vttCue{}

	// Determine if the first line is a timecode or a cue identifier
	if isTimecode(firstLine) {
		cue.timecode = firstLine
	} else {
		// It's a cue identifier
		cue.identifier = firstLine
		// Next line should be the timecode
		if scanner.Scan() {
			cue.timecode = strings.TrimRight(scanner.Text(), "\r")
		}
	}

	// Read text lines until blank line or EOF
	var textLines []string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			break
		}
		textLines = append(textLines, line)
	}

	cue.text = strings.Join(textLines, "\n")
	return cue
}

// isTimecode returns true if the line looks like a VTT timecode line.
func isTimecode(line string) bool {
	return strings.Contains(line, "-->")
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
