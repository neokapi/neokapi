package versifiedtext

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// versePattern matches verse markers at the start of a line.
// Supports formats: \v1, \v 1, \v12, or plain numbers like "1 " or "1." at start.
var versePattern = regexp.MustCompile(`^(?:\\v\s*(\d+)\s+|(\d+)[.\s]\s*)(.*)$`)

// Reader implements DataFormatReader for versified text (poetry/scripture).
// Lines with verse markers become separate Blocks with verse metadata.
// Blank lines separate stanzas and are emitted as Data parts.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter and StreamingReader.
var (
	_ format.SkeletonStoreEmitter = (*Reader)(nil)
	_ format.StreamingReader      = (*Reader)(nil)
)

// StreamingReader marks this reader as bounded-memory streaming: it reads its
// input line-by-line via bufio and emits each block/stanza incrementally, never
// buffering the whole document. See [AD-005].
func (r *Reader) StreamingReader() bool { return true }

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

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
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
		return errors.New("versifiedtext: nil document or reader")
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

	if r.skeletonStore != nil {
		r.readContentSkeleton(ctx, ch)
	} else {
		r.readContentSimple(ctx, ch)
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readContentSimple(ctx context.Context, ch chan<- model.PartResult) {
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
			block.Name = "verse." + verseNum
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
	}
}

func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult) {
	br := bufio.NewReader(r.Doc.Reader)
	blockID := 0
	dataID := 0

	for {
		rawLine, err := br.ReadString('\n')
		if rawLine == "" && err != nil {
			if err != io.EOF {
				ch <- model.PartResult{Error: fmt.Errorf("versifiedtext: reading: %w", err)}
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

		// Blank lines are stanza separators (Data)
		if strings.TrimSpace(content) == "" {
			r.skelText(lineEnding)
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("stanza-break.%d", dataID),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		} else {
			// Try to match verse marker
			matches := versePattern.FindStringSubmatch(content)
			if matches != nil {
				verseNum := matches[1]
				if verseNum == "" {
					verseNum = matches[2]
				}
				text := matches[3]
				// The prefix (verse marker) is skeleton text
				prefix := content[:len(content)-len(text)]
				r.skelText(prefix)

				blockID++
				blockIDStr := fmt.Sprintf("tu%d", blockID)
				r.skelRef(blockIDStr)
				r.skelText(lineEnding)

				block := model.NewBlock(blockIDStr, text)
				block.Name = "verse." + verseNum
				block.Properties["verse"] = verseNum
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			} else {
				// Non-verse line: entire content is translatable
				blockID++
				blockIDStr := fmt.Sprintf("tu%d", blockID)
				r.skelRef(blockIDStr)
				r.skelText(lineEnding)

				block := model.NewBlock(blockIDStr, content)
				block.Name = fmt.Sprintf("line%d", blockID)
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
		}

		if err == io.EOF {
			break
		}
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
