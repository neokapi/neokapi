package paraplaintext

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
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

// Ensure Reader implements SkeletonStoreEmitter and StreamingReader.
var (
	_ format.SkeletonStoreEmitter = (*Reader)(nil)
	_ format.StreamingReader      = (*Reader)(nil)
)

// StreamingReader marks this reader as bounded-memory streaming for its
// byte-exact (skeleton) round-trip: it reads lines via bufio and groups
// paragraphs incrementally, holding only the current paragraph. See [AD-005].
func (r *Reader) StreamingReader() bool { return true }

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

	if r.skeletonStore != nil {
		// Byte-exact path: stream lines and group paragraphs incrementally, so
		// only the current paragraph is held (StreamingReader).
		r.readContentSkeleton(ctx, ch)
	} else {
		// Generative path: the paragraph split is on the exact "\n\n" delimiter,
		// whose semantics differ from line-grouping for odd newline runs, so it
		// reads the whole (normalized) text up front.
		content, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("paraplaintext: reading: %w", err)}
			return
		}
		if text := string(content); text != "" {
			r.readContentNormal(ctx, ch, text)
		}
	}

	r.skelFlush()
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// paraRawLine holds a parsed line with its original line ending preserved.
type paraRawLine struct {
	content    string
	lineEnding string
}

// rawLines streams the document's lines one at a time, preserving exact line
// endings, so the skeleton path groups paragraphs without buffering the file.
func (r *Reader) rawLines() iter.Seq[paraRawLine] {
	return func(yield func(paraRawLine) bool) {
		br := bufio.NewReader(safeio.DefaultBudget().Reader(r.Doc.Reader))
		for {
			raw, err := br.ReadString('\n')
			if raw == "" && err != nil {
				break
			}
			content := raw
			ending := ""
			if strings.HasSuffix(content, "\r\n") {
				content = content[:len(content)-2]
				ending = "\r\n"
			} else if strings.HasSuffix(content, "\n") {
				content = content[:len(content)-1]
				ending = "\n"
			}
			if !yield(paraRawLine{content: content, lineEnding: ending}) {
				return
			}
			if err == io.EOF {
				break
			}
		}
	}
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

// readContentSkeleton streams lines and groups paragraphs incrementally,
// preserving exact line endings for byte-exact roundtrips. A completed paragraph
// is held only until the next paragraph begins (so its inter-paragraph separator
// is known) — the whole file is never buffered.
func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult) {
	blockID := 0
	dataID := 0
	var leadingSep, sepBuf strings.Builder
	var curLines []paraRawLine
	seenContent := false
	leadingEmitted := false

	emitLeading := func() {
		if !leadingEmitted {
			if leadingSep.Len() > 0 {
				r.skelText(leadingSep.String())
			}
			leadingEmitted = true
		}
	}

	// pending holds a completed paragraph awaiting emit, so its trailing
	// separator (emitted only when another paragraph follows) is known first.
	var pendingText, pendingLastEnding string
	havePending := false

	buildPara := func() {
		var sb strings.Builder
		for i, l := range curLines {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(l.content)
		}
		pendingText = sb.String()
		pendingLastEnding = curLines[len(curLines)-1].lineEnding
		havePending = true
		curLines = nil
	}

	// flushPending emits the pending paragraph's block; when withSep it also
	// emits the inter-paragraph separator (skeleton text + a separator Data).
	flushPending := func(withSep bool, sep string) bool {
		emitLeading()
		blockID++
		blockIDStr := fmt.Sprintf("tu%d", blockID)
		r.skelRef(blockIDStr)
		r.skelText(pendingLastEnding)
		block := model.NewBlock(blockIDStr, pendingText)
		block.Name = fmt.Sprintf("para%d", blockID)
		havePending = false
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
		if withSep {
			r.skelText(sep)
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "paragraph-separator",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return false
			}
		}
		return true
	}

	for l := range r.rawLines() {
		if l.content == "" {
			if len(curLines) > 0 {
				buildPara()
				sepBuf.Reset()
				sepBuf.WriteString(l.lineEnding)
			} else if !seenContent {
				leadingSep.WriteString(l.lineEnding)
			} else {
				sepBuf.WriteString(l.lineEnding)
			}
		} else {
			seenContent = true
			if havePending {
				if !flushPending(true, sepBuf.String()) {
					return
				}
				sepBuf.Reset()
			}
			curLines = append(curLines, l)
		}
	}

	// EOF: the last paragraph (if any) emits without a trailing separator.
	if len(curLines) > 0 {
		buildPara()
	}
	if havePending {
		if !flushPending(false, "") {
			return
		}
	}
	emitLeading() // flush leading separator even when there were no paragraphs
	if sepBuf.Len() > 0 {
		r.skelText(sepBuf.String())
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
