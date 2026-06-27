package fixedwidth

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for fixed-width column files.
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

// StreamingReader marks this reader as bounded-memory streaming: it reads lines
// via bufio and emits each row's cells incrementally, holding only the current
// row. See [AD-005].
func (r *Reader) StreamingReader() bool { return true }

// NewReader creates a new fixed-width reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "fixedwidth",
			FormatDisplayName: "Fixed-Width",
			FormatMimeType:    "text/plain",
			FormatExtensions:  []string{".txt", ".dat", ".fixed"},
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
		Extensions: []string{".dat", ".fixed"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("fixedwidth: nil document or reader")
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
		Format:   "fixedwidth",
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
		r.readContentNormal(ctx, ch)
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// fwRawLine holds a parsed line with its original line ending preserved.
type fwRawLine struct {
	content    string // line content without line ending
	lineEnding string // "\n", "\r\n", or "" for last line
}

// rawLines streams the document's lines one at a time, preserving exact line
// endings ("\n"/"\r\n", "" for an unterminated last line), so the skeleton path
// processes each row as it is read rather than after buffering the whole file.
func (r *Reader) rawLines() iter.Seq[fwRawLine] {
	return func(yield func(fwRawLine) bool) {
		br := bufio.NewReader(r.Doc.Reader)
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
			if !yield(fwRawLine{content: content, lineEnding: ending}) {
				return
			}
			if err == io.EOF {
				break
			}
		}
	}
}

func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult) {
	dataCounter := 0
	blockCounter := 0
	rowNum := 0
	first := true

	for rl := range r.rawLines() {
		// Handle header row (the first line) once.
		if first {
			first = false
			if r.cfg.HasHeader {
				if r.cfg.ExtractNonTranslatableContent() {
					// Surface the header / column-label line as a non-translatable
					// caption block (visible to ingestion/LLM consumers, skipped by
					// MT). Its body rides a skeleton ref; the line ending stays
					// skeleton so the round-trip is byte-exact.
					blockCounter++
					blockID := fmt.Sprintf("tu%d", blockCounter)
					r.skelRef(blockID)
					r.skelText(rl.lineEnding)
					block := model.NewBlock(blockID, rl.content)
					block.Name = "header-row"
					block.Translatable = false
					block.PreserveWhitespace = true
					block.SetSemanticRole(model.RoleCaption, 0)
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
				} else {
					r.skelText(rl.content + rl.lineEnding)
					dataCounter++
					data := &model.Data{
						ID:   fmt.Sprintf("d%d", dataCounter),
						Name: "header-row",
						Properties: map[string]string{
							"content": rl.content,
						},
					}
					if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
						return
					}
				}
				continue
			}
		}

		rowNum++
		line := rl.content
		runes := []rune(line)

		// Track how far we've written in this line for skeleton
		runePos := 0

		for _, col := range r.cfg.Columns {
			// Write any gap between previous position and this column as skeleton text
			if col.Start > runePos {
				gapEnd := min(col.Start, len(runes))
				if runePos < len(runes) {
					r.skelText(string(runes[runePos:gapEnd]))
				}
			}

			value := r.extractColumn(runes, col)
			rawValue := value // preserve raw value before trim for skeleton
			if r.cfg.TrimValues {
				value = strings.TrimSpace(value)
			}

			colEnd := min(col.Start+col.Width, len(runes))

			if !col.Translatable {
				if r.cfg.ExtractNonTranslatableContent() {
					// Surface the non-translatable cell as a content block
					// (visible to ingestion/LLM consumers, skipped by MT). Its
					// body rides a skeleton ref so the cell round-trips
					// byte-exact; inter-column gaps/padding stay skeleton.
					blockCounter++
					blockID := fmt.Sprintf("tu%d", blockCounter)
					r.skelRef(blockID)
					runePos = colEnd
					block := model.NewBlock(blockID, value)
					block.Name = fmt.Sprintf("%s.row%d", col.Name, rowNum)
					block.Translatable = false
					block.PreserveWhitespace = true
					block.Properties["column"] = col.Name
					block.Properties["row"] = strconv.Itoa(rowNum)
					block.Properties["start"] = strconv.Itoa(col.Start)
					block.Properties["width"] = strconv.Itoa(col.Width)
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
					continue
				}
				r.skelText(rawValue)
				runePos = colEnd
				dataCounter++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: fmt.Sprintf("%s.row%d", col.Name, rowNum),
					Properties: map[string]string{
						"content": value,
						"column":  col.Name,
						"row":     strconv.Itoa(rowNum),
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				continue
			}

			if value == "" {
				r.skelText(rawValue)
				runePos = colEnd
				continue
			}

			blockCounter++
			blockID := fmt.Sprintf("tu%d", blockCounter)
			r.skelRef(blockID)
			runePos = colEnd

			block := model.NewBlock(blockID, value)
			block.Name = fmt.Sprintf("%s.row%d", col.Name, rowNum)
			block.Properties["column"] = col.Name
			block.Properties["row"] = strconv.Itoa(rowNum)
			block.Properties["start"] = strconv.Itoa(col.Start)
			block.Properties["width"] = strconv.Itoa(col.Width)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}

		// Write any remaining content after the last column
		if runePos < len(runes) {
			r.skelText(string(runes[runePos:]))
		}
		// Write the line ending
		r.skelText(rl.lineEnding)
	}
}

func (r *Reader) readContentNormal(ctx context.Context, ch chan<- model.PartResult) {
	scanner := bufio.NewScanner(r.Doc.Reader)
	dataCounter := 0
	blockCounter := 0
	rowNum := 0
	first := true

	for scanner.Scan() {
		line := scanner.Text()

		// Handle header row (the first line) once.
		if first {
			first = false
			if r.cfg.HasHeader {
				if r.cfg.ExtractNonTranslatableContent() {
					// Surface the header / column-label line as a non-translatable
					// caption block (visible to ingestion/LLM consumers, skipped by MT).
					blockCounter++
					block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), line)
					block.Name = "header-row"
					block.Translatable = false
					block.PreserveWhitespace = true
					block.SetSemanticRole(model.RoleCaption, 0)
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
				} else {
					dataCounter++
					data := &model.Data{
						ID:   fmt.Sprintf("d%d", dataCounter),
						Name: "header-row",
						Properties: map[string]string{
							"content": line,
						},
					}
					if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
						return
					}
				}
				continue
			}
		}

		rowNum++
		runes := []rune(line)

		for _, col := range r.cfg.Columns {
			value := r.extractColumn(runes, col)
			if r.cfg.TrimValues {
				value = strings.TrimSpace(value)
			}

			if !col.Translatable {
				if r.cfg.ExtractNonTranslatableContent() {
					// Surface the non-translatable cell as a content block
					// (visible to ingestion/LLM consumers, skipped by MT).
					blockCounter++
					block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), value)
					block.Name = fmt.Sprintf("%s.row%d", col.Name, rowNum)
					block.Translatable = false
					block.PreserveWhitespace = true
					block.Properties["column"] = col.Name
					block.Properties["row"] = strconv.Itoa(rowNum)
					block.Properties["start"] = strconv.Itoa(col.Start)
					block.Properties["width"] = strconv.Itoa(col.Width)
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
					continue
				}
				// Non-translatable -> Data
				dataCounter++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: fmt.Sprintf("%s.row%d", col.Name, rowNum),
					Properties: map[string]string{
						"content": value,
						"column":  col.Name,
						"row":     strconv.Itoa(rowNum),
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				continue
			}

			if value == "" {
				continue
			}

			blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), value)
			block.Name = fmt.Sprintf("%s.row%d", col.Name, rowNum)
			block.Properties["column"] = col.Name
			block.Properties["row"] = strconv.Itoa(rowNum)
			block.Properties["start"] = strconv.Itoa(col.Start)
			block.Properties["width"] = strconv.Itoa(col.Width)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("fixedwidth: reading: %w", err)}
	}
}

// extractColumn extracts a column value from a line of runes.
func (r *Reader) extractColumn(runes []rune, col ColumnDef) string {
	if col.Start >= len(runes) {
		return ""
	}
	end := min(col.Start+col.Width, len(runes))
	return string(runes[col.Start:end])
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
