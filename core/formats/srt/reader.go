package srt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for SRT subtitle files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new SRT reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "srt",
			FormatDisplayName: "SRT Subtitles",
			FormatMimeType:    "application/x-subrip",
			FormatExtensions:  []string{".srt"},
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
		MIMETypes:  []string{"application/x-subrip", "text/srt"},
		Extensions: []string{".srt"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("srt: nil document or reader")
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

// srtEntry represents a single subtitle entry.
type srtEntry struct {
	sequence string
	timecode string
	text     string
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "srt",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-subrip",
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
	entries := r.parseEntries()

	blockCounter := 0
	dataCounter := 0

	for _, entry := range entries {
		// Emit sequence number as Data
		dataCounter++
		seqData := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: "sequence." + entry.sequence,
			Properties: map[string]string{
				"sequence": entry.sequence,
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: seqData}) {
			return
		}

		// Emit subtitle text as Block
		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), entry.text)
		block.Name = "subtitle." + entry.sequence
		block.Properties["timecode"] = entry.timecode
		block.Properties["sequence"] = entry.sequence
		setBlockTiming(block, entry.timecode)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// readContentSkeleton parses SRT entries while preserving exact bytes in the skeleton store.
// Sequence numbers, timecodes, and blank-line separators go into the skeleton as text.
// Subtitle text goes into blocks referenced by skeleton refs.
// For multi-line subtitles, each text line's line ending is tracked so the block text
// preserves the original line ending style (LF vs CRLF).
func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult) {
	lines := r.readRawLines()

	type state int
	const (
		stateSequence state = iota
		stateTimecode
		stateText
	)

	type textLine struct {
		content    string
		lineEnding string
	}

	st := stateSequence
	var sequence, timecode string
	var textLines []textLine
	blockCounter := 0
	dataCounter := 0

	finishEntry := func() bool {
		if len(textLines) == 0 {
			return true
		}
		// Emit Data for sequence number
		dataCounter++
		seqData := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: "sequence." + sequence,
			Properties: map[string]string{
				"sequence": sequence,
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: seqData}) {
			return false
		}

		// Build block text joining with original line endings between text lines.
		// The line ending after the last text line is NOT part of the block text;
		// it goes into skeleton.
		var sb strings.Builder
		for i, tl := range textLines {
			if i > 0 {
				sb.WriteString(textLines[i-1].lineEnding)
			}
			sb.WriteString(tl.content)
		}

		blockCounter++
		blockIDStr := fmt.Sprintf("tu%d", blockCounter)
		r.skelRef(blockIDStr)
		// Write the line ending after the last text line as skeleton text
		lastEnding := textLines[len(textLines)-1].lineEnding
		r.skelText(lastEnding)

		block := model.NewBlock(blockIDStr, sb.String())
		block.Name = "subtitle." + sequence
		block.Properties["timecode"] = timecode
		block.Properties["sequence"] = sequence
		setBlockTiming(block, timecode)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
		textLines = nil
		return true
	}

	for _, l := range lines {
		switch st {
		case stateSequence:
			if l.content == "" {
				// Blank line between entries or leading blank lines — skeleton only
				r.skelText(l.lineEnding)
				continue
			}
			sequence = l.content
			// Sequence number + line ending go to skeleton
			r.skelText(l.content + l.lineEnding)
			st = stateTimecode

		case stateTimecode:
			timecode = strings.TrimSpace(l.content)
			// Timecode line goes to skeleton
			r.skelText(l.content + l.lineEnding)
			st = stateText
			textLines = nil

		case stateText:
			if strings.TrimSpace(l.content) == "" {
				// End of entry — finish block, then write blank-line ending as skeleton
				if !finishEntry() {
					return
				}
				r.skelText(l.lineEnding)
				st = stateSequence
			} else {
				textLines = append(textLines, textLine(l))
			}
		}
	}

	// Handle last entry if file doesn't end with blank line
	if st == stateText {
		finishEntry()
	}
}

// rawLine holds a parsed line with its original line ending preserved.
type rawLine struct {
	content    string
	lineEnding string
}

// readRawLines reads all lines from the document, preserving exact line endings.
func (r *Reader) readRawLines() []rawLine {
	br := bufio.NewReader(r.Doc.Reader)
	var lines []rawLine

	for {
		raw, err := br.ReadString('\n')
		if raw == "" && err != nil {
			break
		}

		content := raw
		lineEnding := ""
		if strings.HasSuffix(content, "\r\n") {
			content = content[:len(content)-2]
			lineEnding = "\r\n"
		} else if strings.HasSuffix(content, "\n") {
			content = content[:len(content)-1]
			lineEnding = "\n"
		}

		lines = append(lines, rawLine{content: content, lineEnding: lineEnding})

		if err == io.EOF {
			break
		}
	}

	return lines
}

func (r *Reader) parseEntries() []*srtEntry {
	scanner := bufio.NewScanner(r.Doc.Reader)
	var entries []*srtEntry

	type state int
	const (
		stateSequence state = iota
		stateTimecode
		stateText
	)

	current := &srtEntry{}
	st := stateSequence
	var textLines []string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, "\r")

		switch st {
		case stateSequence:
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			current.sequence = trimmed
			st = stateTimecode

		case stateTimecode:
			current.timecode = strings.TrimSpace(line)
			st = stateText
			textLines = nil

		case stateText:
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				// End of entry
				current.text = strings.Join(textLines, "\n")
				entries = append(entries, current)
				current = &srtEntry{}
				st = stateSequence
			} else {
				textLines = append(textLines, line)
			}
		}
	}

	// Don't forget the last entry if file doesn't end with blank line
	if st == stateText && len(textLines) > 0 {
		current.text = strings.Join(textLines, "\n")
		entries = append(entries, current)
	}

	return entries
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
