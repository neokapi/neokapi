package csv

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for CSV files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new CSV reader.
func NewReader() *Reader {
	cfg := &Config{Separator: ',', HasHeader: true}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "csv",
			FormatDisplayName: "CSV",
			FormatMimeType:    "text/csv",
			FormatExtensions:  []string{".csv"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// NewTSVReader creates a new TSV reader (tab-separated values).
func NewTSVReader() *Reader {
	cfg := &Config{Separator: '\t', HasHeader: true}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "tsv",
			FormatDisplayName: "TSV",
			FormatMimeType:    "text/tab-separated-values",
			FormatExtensions:  []string{".tsv"},
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
	if r.cfg.Separator == '\t' {
		return format.FormatSignature{
			MIMETypes:  []string{"text/tab-separated-values"},
			Extensions: []string{".tsv"},
		}
	}
	return format.FormatSignature{
		MIMETypes:  []string{"text/csv"},
		Extensions: []string{".csv"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("csv: nil document or reader")
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

	mimeType := "text/csv"
	if r.cfg.Separator == '\t' {
		mimeType = "text/tab-separated-values"
	}

	formatName := "csv"
	if r.cfg.Separator == '\t' {
		formatName = "tsv"
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   formatName,
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: mimeType,
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("csv: reading: %w", err)}
		return
	}

	if r.skeletonStore != nil {
		r.readContentSkeleton(ctx, ch, layer, string(content))
		return
	}

	csvReader := csv.NewReader(strings.NewReader(string(content)))
	csvReader.Comma = r.cfg.Separator
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1 // allow variable number of fields per row

	records, err := csvReader.ReadAll()
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("csv: parsing: %w", err)}
		return
	}

	if len(records) == 0 {
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
		return
	}

	var headers []string
	headerRow := -1
	startRow := 0
	blockCounter := 0
	dataCounter := 0

	// Determine header row
	if r.cfg.HasHeader {
		if r.cfg.ColumnNamesRow > 0 {
			headerRow = r.cfg.ColumnNamesRow - 1 // convert 1-based to 0-based
		} else {
			headerRow = 0
		}
		if headerRow < len(records) {
			headers = records[headerRow]
		}
	}

	// Determine start row for data values
	if r.cfg.ValuesStartRow > 0 {
		startRow = r.cfg.ValuesStartRow - 1 // convert 1-based to 0-based
	} else if r.cfg.HasHeader {
		if headerRow >= 0 {
			startRow = headerRow + 1
		} else {
			startRow = 1
		}
	}

	// Emit rows before the data start as Data parts (headers, preamble, etc.)
	for rowIdx := 0; rowIdx < startRow && rowIdx < len(records); rowIdx++ {
		dataCounter++
		row := records[rowIdx]
		name := "header-row"
		if rowIdx != headerRow {
			name = fmt.Sprintf("preamble-row%d", rowIdx+1)
		}
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: name,
			Properties: map[string]string{
				"content": strings.Join(row, string(r.cfg.Separator)),
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
			return
		}
	}

	for rowIdx := startRow; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		rowNum := rowIdx - startRow + 1

		// Build key from key columns if configured
		var rowKey string
		if len(r.cfg.KeyColumns) > 0 {
			var keyParts []string
			for _, kc := range r.cfg.KeyColumns {
				if kc < len(row) {
					keyParts = append(keyParts, row[kc])
				}
			}
			rowKey = strings.Join(keyParts, ".")
		}

		// Build comment from comment columns if configured
		var rowComment string
		if len(r.cfg.CommentColumns) > 0 {
			var commentParts []string
			for _, cc := range r.cfg.CommentColumns {
				if cc < len(row) && strings.TrimSpace(row[cc]) != "" {
					commentParts = append(commentParts, row[cc])
				}
			}
			rowComment = strings.Join(commentParts, "; ")
		}

		for colIdx, cell := range row {
			// Skip key and comment columns (they are metadata, not content)
			if r.isKeyColumn(colIdx) || r.isCommentColumn(colIdx) {
				continue
			}

			if !r.isTranslatable(colIdx) {
				// Non-translatable column -> Data
				dataCounter++
				colName := r.columnName(headers, colIdx)
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: fmt.Sprintf("%s.row%d", colName, rowNum),
					Properties: map[string]string{
						"content": cell,
						"column":  strconv.Itoa(colIdx),
						"row":     strconv.Itoa(rowNum),
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				continue
			}

			cellValue := cell
			if r.cfg.TrimValues {
				cellValue = strings.TrimSpace(cellValue)
			}

			if cellValue == "" {
				continue
			}

			blockCounter++
			colName := r.columnName(headers, colIdx)

			blockID := fmt.Sprintf("tu%d", blockCounter)
			if rowKey != "" {
				blockID = rowKey
				if len(r.cfg.TranslatableColumns) > 1 {
					// Multiple translatable columns with key: add column suffix
					blockID = fmt.Sprintf("%s.%s", rowKey, colName)
				}
			}

			block := model.NewBlock(blockID, cellValue)
			block.Name = fmt.Sprintf("%s.row%d", colName, rowNum)
			block.Properties["column"] = strconv.Itoa(colIdx)
			block.Properties["row"] = strconv.Itoa(rowNum)
			if rowComment != "" {
				block.Properties["comment"] = rowComment
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) columnName(headers []string, colIdx int) string {
	if colIdx < len(headers) {
		return headers[colIdx]
	}
	return fmt.Sprintf("col%d", colIdx)
}

func (r *Reader) isTranslatable(colIdx int) bool {
	if len(r.cfg.TranslatableColumns) == 0 {
		return true // all columns translatable by default
	}
	return slices.Contains(r.cfg.TranslatableColumns, colIdx)
}

func (r *Reader) isKeyColumn(colIdx int) bool {
	return slices.Contains(r.cfg.KeyColumns, colIdx)
}

func (r *Reader) isCommentColumn(colIdx int) bool {
	return slices.Contains(r.cfg.CommentColumns, colIdx)
}

// readContentSkeleton handles reading with skeleton store active.
// It parses the raw CSV content character-by-character to preserve exact
// formatting (quoting styles, delimiters, line endings) while extracting
// translatable cell values as skeleton refs.
func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult, layer *model.Layer, content string) {
	// First, use encoding/csv to get parsed records (for column logic).
	csvReader := csv.NewReader(strings.NewReader(content))
	csvReader.Comma = r.cfg.Separator
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1

	records, err := csvReader.ReadAll()
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("csv: parsing: %w", err)}
		return
	}

	if len(records) == 0 {
		r.skelText(content)
		r.skelFlush()
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
		return
	}

	// Determine header/start rows (same logic as non-skeleton path).
	var headers []string
	headerRow := -1
	startRow := 0

	if r.cfg.HasHeader {
		if r.cfg.ColumnNamesRow > 0 {
			headerRow = r.cfg.ColumnNamesRow - 1
		} else {
			headerRow = 0
		}
		if headerRow < len(records) {
			headers = records[headerRow]
		}
	}

	if r.cfg.ValuesStartRow > 0 {
		startRow = r.cfg.ValuesStartRow - 1
	} else if r.cfg.HasHeader {
		if headerRow >= 0 {
			startRow = headerRow + 1
		} else {
			startRow = 1
		}
	}

	// Split raw content into lines preserving line endings.
	rawLines := splitRawLines(content)

	blockCounter := 0
	dataCounter := 0

	// Process each raw line, matching against parsed records.
	for rowIdx, rawLine := range rawLines {
		if rowIdx >= len(records) {
			// Trailing content beyond parsed records (shouldn't happen normally).
			r.skelText(rawLine.text + rawLine.lineEnding)
			continue
		}

		row := records[rowIdx]

		if rowIdx < startRow {
			// Header/preamble row: emit entirely as skeleton text + Data part.
			r.skelText(rawLine.text + rawLine.lineEnding)
			dataCounter++
			name := "header-row"
			if rowIdx != headerRow {
				name = fmt.Sprintf("preamble-row%d", rowIdx+1)
			}
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: name,
				Properties: map[string]string{
					"content": strings.Join(row, string(r.cfg.Separator)),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		rowNum := rowIdx - startRow + 1

		// Build key from key columns if configured.
		var rowKey string
		if len(r.cfg.KeyColumns) > 0 {
			var keyParts []string
			for _, kc := range r.cfg.KeyColumns {
				if kc < len(row) {
					keyParts = append(keyParts, row[kc])
				}
			}
			rowKey = strings.Join(keyParts, ".")
		}

		// Build comment from comment columns if configured.
		var rowComment string
		if len(r.cfg.CommentColumns) > 0 {
			var commentParts []string
			for _, cc := range r.cfg.CommentColumns {
				if cc < len(row) && strings.TrimSpace(row[cc]) != "" {
					commentParts = append(commentParts, row[cc])
				}
			}
			rowComment = strings.Join(commentParts, "; ")
		}

		// Parse raw cells from this line to preserve exact quoting.
		rawCells := splitRawCells(rawLine.text, r.cfg.Separator)

		for colIdx := range rawCells {
			rc := rawCells[colIdx]

			// Write delimiter before cell (except first column).
			if colIdx > 0 {
				r.skelText(string(r.cfg.Separator))
			}

			parsedValue := ""
			if colIdx < len(row) {
				parsedValue = row[colIdx]
			}

			// Key and comment columns are non-translatable skeleton text.
			if r.isKeyColumn(colIdx) || r.isCommentColumn(colIdx) {
				r.skelText(rc.raw)
				continue
			}

			if !r.isTranslatable(colIdx) {
				// Non-translatable column -> skeleton text + Data part.
				r.skelText(rc.raw)
				dataCounter++
				colName := r.columnName(headers, colIdx)
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: fmt.Sprintf("%s.row%d", colName, rowNum),
					Properties: map[string]string{
						"content": parsedValue,
						"column":  strconv.Itoa(colIdx),
						"row":     strconv.Itoa(rowNum),
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				continue
			}

			cellValue := parsedValue
			if r.cfg.TrimValues {
				cellValue = strings.TrimSpace(cellValue)
			}

			if cellValue == "" {
				// Empty translatable cell: write as skeleton text.
				r.skelText(rc.raw)
				continue
			}

			blockCounter++
			colName := r.columnName(headers, colIdx)

			blockID := fmt.Sprintf("tu%d", blockCounter)
			if rowKey != "" {
				blockID = rowKey
				if len(r.cfg.TranslatableColumns) > 1 {
					blockID = fmt.Sprintf("%s.%s", rowKey, colName)
				}
			}

			// Write quoting prefix as skeleton text, then ref for value, then quoting suffix.
			r.skelText(rc.prefix)
			r.skelRef(blockID)
			r.skelText(rc.suffix)

			block := model.NewBlock(blockID, cellValue)
			block.Name = fmt.Sprintf("%s.row%d", colName, rowNum)
			block.Properties["column"] = strconv.Itoa(colIdx)
			block.Properties["row"] = strconv.Itoa(rowNum)
			if rc.prefix == "\"" {
				block.Properties["quoted"] = "true"
			}
			if rowComment != "" {
				block.Properties["comment"] = rowComment
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}

		// Write line ending as skeleton text.
		r.skelText(rawLine.lineEnding)
	}

	r.skelFlush()
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// rawLine holds a line's content and its original line ending.
type rawLine struct {
	text       string
	lineEnding string
}

// splitRawLines splits content into lines preserving exact line endings.
func splitRawLines(content string) []rawLine {
	var lines []rawLine
	remaining := content
	for len(remaining) > 0 {
		// Find next unquoted newline. We need to track whether we're inside
		// a quoted field to avoid splitting on newlines within quotes.
		inQuotes := false
		i := 0
		for i < len(remaining) {
			ch := remaining[i]
			if ch == '"' {
				inQuotes = !inQuotes
				i++
			} else if !inQuotes && ch == '\n' {
				// Found line boundary.
				lineContent := remaining[:i]
				ending := "\n"
				if strings.HasSuffix(lineContent, "\r") {
					lineContent = lineContent[:len(lineContent)-1]
					ending = "\r\n"
				}
				lines = append(lines, rawLine{text: lineContent, lineEnding: ending})
				remaining = remaining[i+1:]
				goto nextLine
			} else {
				i++
			}
		}
		// No newline found: last line with no ending.
		lines = append(lines, rawLine{text: remaining})
		break
	nextLine:
	}
	return lines
}

// rawCell holds a raw cell's text along with its quote prefix and suffix.
type rawCell struct {
	raw    string // full raw text of the cell
	prefix string // quote character(s) before value (e.g., `"`)
	suffix string // quote character(s) after value (e.g., `"`)
}

// splitRawCells splits a raw CSV line into cells preserving exact formatting.
func splitRawCells(line string, sep rune) []rawCell {
	var cells []rawCell
	sepStr := string(sep)
	remaining := line

	for {
		if len(remaining) == 0 {
			cells = append(cells, rawCell{raw: ""})
			break
		}

		if remaining[0] == '"' {
			// Quoted field: find closing quote.
			// The field ends at a quote not followed by another quote.
			i := 1
			for i < len(remaining) {
				if remaining[i] == '"' {
					if i+1 < len(remaining) && remaining[i+1] == '"' {
						// Escaped quote: skip both.
						i += 2
					} else {
						// End of quoted field.
						i++
						break
					}
				} else {
					i++
				}
			}
			raw := remaining[:i]
			cells = append(cells, rawCell{
				raw:    raw,
				prefix: "\"",
				suffix: "\"",
			})
			remaining = remaining[i:]
			// Skip separator after field.
			if strings.HasPrefix(remaining, sepStr) {
				remaining = remaining[len(sepStr):]
			} else {
				break
			}
		} else {
			// Unquoted field: find next separator.
			idx := strings.Index(remaining, sepStr)
			if idx < 0 {
				cells = append(cells, rawCell{raw: remaining})
				break
			}
			cells = append(cells, rawCell{raw: remaining[:idx]})
			remaining = remaining[idx+len(sepStr):]
		}
	}

	return cells
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
	// Apply inline code finder to blocks if enabled
	if part.Type == model.PartBlock && r.cfg.UseCodeFinder {
		if block, ok := part.Resource.(*model.Block); ok {
			r.applyCodeFinder(block)
		}
	}
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// applyCodeFinder applies code finder patterns to a block's fragments.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.CodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}

	if len(block.Source) == 0 {
		return
	}
	text := model.RunsText(block.Source)

	type matchRange struct {
		start, end int
	}
	var matches []matchRange
	for _, re := range patterns {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			matches = append(matches, matchRange{loc[0], loc[1]})
		}
	}
	if len(matches) == 0 {
		return
	}

	// Sort matches by start position
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	var runs []model.Run
	lastEnd := 0
	spanID := 1
	for _, m := range matches {
		if m.start > lastEnd {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:m.start]}})
		}
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
			ID:   fmt.Sprintf("c%d", spanID),
			Type: "code",
			Data: text[m.start:m.end],
		}})
		lastEnd = m.end
		spanID++
	}
	if lastEnd < len(text) {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
	}
	block.SetSourceRuns(runs)
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
