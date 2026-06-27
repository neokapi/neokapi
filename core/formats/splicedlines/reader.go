package splicedlines

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

// Reader implements DataFormatReader for line-spliced text files.
// Lines ending with backslash (\) are continued on the next line.
// Continuation lines are joined into a single Block.
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
// input line-by-line via bufio and emits each block incrementally (holding only
// the in-progress continuation), never buffering the whole document. See [AD-005].
func (r *Reader) StreamingReader() bool { return true }

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
		return errors.New("splicedlines: nil document or reader")
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

	br := bufio.NewReader(r.Doc.Reader)
	blockID := 0
	dataID := 0

	type rawLine struct {
		content    string
		lineEnding string
	}

	var accumulated []rawLine
	// hadTrailingSplicer is set true when the EOF-flush is dealing with
	// an accumulator whose final raw line ended in `\` (and thus had it
	// stripped). The writer reads the resulting block property to add
	// the byte back on emit.
	var hadTrailingSplicer bool

	flushBlock := func() bool {
		if len(accumulated) == 0 {
			return true
		}
		// Build joined content (without backslashes or line endings)
		var parts []string
		for _, rl := range accumulated {
			parts = append(parts, rl.content)
		}
		joined := strings.Join(parts, "\n")

		if strings.TrimSpace(joined) == "" {
			// Write the original raw text (with line endings) to skeleton
			for _, rl := range accumulated {
				r.skelText(rl.lineEnding)
			}
			accumulated = nil

			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("empty.%d", dataID),
			}
			return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		}

		blockID++
		blockIDStr := fmt.Sprintf("tu%d", blockID)
		numLines := len(accumulated)

		// Write skeleton: for continuation lines, backslash+lineEnding are skeleton text.
		// The block ref captures only the content (without backslashes).
		r.skelRef(blockIDStr)
		// After the ref, write the line ending of the last line
		lastEnding := accumulated[numLines-1].lineEnding
		r.skelText(lastEnding)

		block := model.NewBlock(blockIDStr, joined)
		block.Name = fmt.Sprintf("block%d", blockID)
		block.Properties["continued"] = strconv.Itoa(numLines)
		if hadTrailingSplicer {
			block.Properties["trailing-splicer"] = "true"
		}

		// Store the continuation line endings so the writer can reconstruct
		if numLines > 1 {
			var endings []string
			for i := range numLines - 1 {
				endings = append(endings, accumulated[i].lineEnding)
			}
			block.Properties["continuation-endings"] = strings.Join(endings, "|")
		}

		accumulated = nil
		return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	for {
		rawLine, err := br.ReadString('\n')
		if rawLine == "" && err != nil {
			if err != io.EOF {
				ch <- model.PartResult{Error: fmt.Errorf("splicedlines: reading: %w", err)}
			}
			break
		}

		// Split into content and line ending
		content := rawLine
		lineEnding := ""
		if strings.HasSuffix(content, "\r\n") {
			content = content[:len(content)-2]
			lineEnding = "\r\n"
		} else if strings.HasSuffix(content, "\n") {
			content = content[:len(content)-1]
			lineEnding = "\n"
		}

		if before, ok := strings.CutSuffix(content, `\`); ok {
			// Continuation line: strip trailing backslash and accumulate
			stripped := before
			accumulated = append(accumulated, struct {
				content    string
				lineEnding string
			}{content: stripped, lineEnding: lineEnding})
		} else {
			// Non-continuation: add to accumulator and flush
			accumulated = append(accumulated, struct {
				content    string
				lineEnding string
			}{content: content, lineEnding: lineEnding})
			if !flushBlock() {
				return
			}
		}

		if err == io.EOF {
			break
		}
	}

	// Flush any remaining accumulated lines. The accumulator can only
	// be non-empty here if the loop exited via EOF mid-continuation
	// (every non-continuation flushes immediately). Mark the resulting
	// block so the writer can re-emit the trailing `\` byte that the
	// reader stripped — Okapi's okf_splicedlines preserves it on
	// round-trip even though the block's logical text doesn't include
	// it (matches SplicedLinesFilterTest#testTrailingBackslash).
	if len(accumulated) > 0 {
		hadTrailingSplicer = true
		if !flushBlock() {
			return
		}
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
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
