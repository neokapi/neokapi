package fixedwidth

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

// Reader implements DataFormatReader for fixed-width column files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

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

func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult) {
	raw, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("fixedwidth: reading: %w", err)}
		return
	}

	// Split into raw lines preserving exact line endings
	var lines []fwRawLine
	remaining := string(raw)
	for len(remaining) > 0 {
		idx := strings.Index(remaining, "\n")
		if idx < 0 {
			lines = append(lines, fwRawLine{content: remaining})
			break
		}
		lineContent := remaining[:idx]
		ending := "\n"
		if strings.HasSuffix(lineContent, "\r") {
			lineContent = lineContent[:len(lineContent)-1]
			ending = "\r\n"
		}
		lines = append(lines, fwRawLine{content: lineContent, lineEnding: ending})
		remaining = remaining[idx+1:]
	}

	if len(lines) == 0 {
		return
	}

	startRow := 0
	dataCounter := 0
	blockCounter := 0

	// Handle header row
	if r.cfg.HasHeader && len(lines) > 0 {
		if r.cfg.ExtractNonTranslatableContent() {
			// Surface the header / column-label line as a non-translatable
			// caption block (visible to ingestion/LLM consumers, skipped by
			// MT). Its body rides a skeleton ref; the line ending stays
			// skeleton so the round-trip is byte-exact.
			blockCounter++
			blockID := fmt.Sprintf("tu%d", blockCounter)
			r.skelRef(blockID)
			r.skelText(lines[0].lineEnding)
			block := model.NewBlock(blockID, lines[0].content)
			block.Name = "header-row"
			block.Translatable = false
			block.PreserveWhitespace = true
			block.SetSemanticRole(model.RoleCaption, 0)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		} else {
			r.skelText(lines[0].content + lines[0].lineEnding)
			dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "header-row",
				Properties: map[string]string{
					"content": lines[0].content,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
		startRow = 1
	}

	for rowIdx := startRow; rowIdx < len(lines); rowIdx++ {
		rl := lines[rowIdx]
		line := rl.content
		runes := []rune(line)
		rowNum := rowIdx - startRow + 1

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
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("fixedwidth: reading: %w", err)}
		return
	}

	if len(lines) == 0 {
		return
	}

	startRow := 0
	dataCounter := 0
	blockCounter := 0

	// Handle header row
	if r.cfg.HasHeader && len(lines) > 0 {
		if r.cfg.ExtractNonTranslatableContent() {
			// Surface the header / column-label line as a non-translatable
			// caption block (visible to ingestion/LLM consumers, skipped by MT).
			blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), lines[0])
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
					"content": lines[0],
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
		startRow = 1
	}

	for rowIdx := startRow; rowIdx < len(lines); rowIdx++ {
		line := lines[rowIdx]
		runes := []rune(line)
		rowNum := rowIdx - startRow + 1

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
