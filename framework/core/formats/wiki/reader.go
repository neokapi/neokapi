package wiki

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// MediaWiki header pattern: == Title == through ====== Title ======
var mediaWikiHeaderRe = regexp.MustCompile(`^(={2,6})\s*(.+?)\s*(={2,6})\s*$`)

// DokuWiki header pattern: same syntax as MediaWiki (= delimiters)
var dokuWikiHeaderRe = regexp.MustCompile(`^(={2,6})\s*(.+?)\s*(={2,6})\s*$`)

// MediaWiki table patterns
var mediaWikiTableStartRe = regexp.MustCompile(`^\{\|`)
var mediaWikiTableEndRe = regexp.MustCompile(`^\|\}`)
var mediaWikiTableRowRe = regexp.MustCompile(`^\|-`)
var mediaWikiTableCellRe = regexp.MustCompile(`^\|(.+)`)
var mediaWikiTableHeaderRe = regexp.MustCompile(`^!(.+)`)

// DokuWiki table row: ^ Header ^ or | Cell |
var dokuWikiTableRe = regexp.MustCompile(`^[|^].*[|^]\s*$`)

// MediaWiki image/file link: [[File:...|...|caption]] or [[Image:...|...|caption]]
var mediaWikiImageRe = regexp.MustCompile(`\[\[(?:File|Image):([^]|]+)((?:\|[^]|]*)*)?\]\]`)

// Reader implements DataFormatReader for Wiki files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new wiki reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "wiki",
			FormatDisplayName: "Wiki",
			FormatMimeType:    "text/x-wiki",
			FormatExtensions:  []string{".wiki", ".mediawiki"},
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
		MIMETypes:  []string{"text/x-wiki"},
		Extensions: []string{".wiki", ".mediawiki"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("wiki: nil document or reader")
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

// rawLine holds a line's content and its original line ending.
type rawLine struct {
	content    string
	lineEnding string
}

// splitRawLines splits raw bytes into lines preserving line endings.
func splitRawLines(data []byte) []rawLine {
	remaining := string(data)
	var lines []rawLine
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
	return lines
}

// parseState holds mutable state during parsing.
type parseState struct {
	blockID       int
	dataID        int
	paraLines     []string
	paraLineIdxes []int // indices into rLines for skeleton tracking
}

func (ps *parseState) flushParagraph(ctx context.Context, r *Reader, ch chan<- model.PartResult, rLines []rawLine) bool {
	if len(ps.paraLines) == 0 {
		return true
	}
	text := strings.Join(ps.paraLines, "\n")
	paraIdxes := ps.paraLineIdxes
	ps.paraLines = nil
	ps.paraLineIdxes = nil
	if strings.TrimSpace(text) == "" {
		return true
	}
	ps.blockID++
	blockID := fmt.Sprintf("tu%d", ps.blockID)

	// Skeleton: ref for the paragraph block, but include inter-line endings
	if r.skeletonStore != nil && len(rLines) > 0 {
		// Write the inter-line endings from raw lines. The block text has \n
		// between lines; the skeleton ref covers the paragraph text.
		// We need to emit: for each paragraph line except the last, add the raw line ending.
		// But since block text already joins with \n, we just emit a single ref
		// and the last line's ending as skeleton text.
		r.skelRef(blockID)
		if len(paraIdxes) > 0 {
			lastIdx := paraIdxes[len(paraIdxes)-1]
			if lastIdx < len(rLines) {
				r.skelText(rLines[lastIdx].lineEnding)
			}
		}
	}

	block := model.NewBlock(blockID, text)
	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

func (ps *parseState) emitData(ctx context.Context, r *Reader, ch chan<- model.PartResult) bool {
	ps.dataID++
	data := &model.Data{ID: fmt.Sprintf("d%d", ps.dataID)}
	return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
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
		Format:   "wiki",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-wiki",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	var rLines []rawLine

	if r.skeletonStore != nil {
		data, err := io.ReadAll(r.Doc.Reader)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("wiki: reading: %w", err)}
			return
		}
		rLines = splitRawLines(data)
		ps := &parseState{}
		if r.cfg.Variant == VariantDokuWiki {
			r.readDokuWikiLines(ctx, ch, rLines, ps)
		} else {
			r.readMediaWikiLines(ctx, ch, rLines, ps)
		}
		r.skelFlush()
	} else {
		scanner := bufio.NewScanner(r.Doc.Reader)
		ps := &parseState{}
		if r.cfg.Variant == VariantDokuWiki {
			r.readDokuWiki(ctx, ch, scanner, ps)
		} else {
			r.readMediaWiki(ctx, ch, scanner, ps)
		}
		if err := scanner.Err(); err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("wiki: reading: %w", err)}
		}
	}

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readMediaWiki(ctx context.Context, ch chan<- model.PartResult,
	scanner *bufio.Scanner, ps *parseState) {

	inTable := false

	for scanner.Scan() {
		line := scanner.Text()

		// Check for header
		if m := mediaWikiHeaderRe.FindStringSubmatch(line); m != nil {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			ps.blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), strings.TrimSpace(m[2]))
			block.Name = "header"
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// Table start
		if mediaWikiTableStartRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			inTable = true
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table end
		if mediaWikiTableEndRe.MatchString(line) {
			inTable = false
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table row separator
		if inTable && mediaWikiTableRowRe.MatchString(line) {
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table header cells
		if inTable && mediaWikiTableHeaderRe.MatchString(line) {
			m := mediaWikiTableHeaderRe.FindStringSubmatch(line)
			cells := splitTableCells(m[1], "!!")
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				if cell == "" {
					continue
				}
				ps.blockID++
				block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
				block.Name = "table-header"
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
			continue
		}

		// Table data cells
		if inTable && mediaWikiTableCellRe.MatchString(line) {
			m := mediaWikiTableCellRe.FindStringSubmatch(line)
			cells := splitTableCells(m[1], "||")
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				if cell == "" {
					continue
				}
				ps.blockID++
				block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
				block.Name = "table-cell"
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
			continue
		}

		// Image/file links with captions
		if mediaWikiImageRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			r.extractImageCaptions(ctx, ch, line, ps)
			continue
		}

		// Blank line separates paragraphs
		if strings.TrimSpace(line) == "" {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Regular text line -- accumulate into paragraph
		ps.paraLines = append(ps.paraLines, line)
	}

	// Flush remaining paragraph
	ps.flushParagraph(ctx, r, ch, nil)
}

func (r *Reader) readMediaWikiLines(ctx context.Context, ch chan<- model.PartResult,
	rLines []rawLine, ps *parseState) {

	inTable := false

	for i, rl := range rLines {
		line := rl.content

		// Check for header
		if m := mediaWikiHeaderRe.FindStringSubmatch(line); m != nil {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			ps.blockID++
			blockID := fmt.Sprintf("tu%d", ps.blockID)
			r.skelRef(blockID)
			r.skelText(rl.lineEnding)
			block := model.NewBlock(blockID, strings.TrimSpace(m[2]))
			block.Name = "header"
			block.Properties["raw"] = line
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// Table start
		if mediaWikiTableStartRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			inTable = true
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table end
		if mediaWikiTableEndRe.MatchString(line) {
			inTable = false
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table row separator
		if inTable && mediaWikiTableRowRe.MatchString(line) {
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table header cells
		if inTable && mediaWikiTableHeaderRe.MatchString(line) {
			// Store entire line as skeleton text (table reconstruction is complex)
			r.skelText(rl.content + rl.lineEnding)
			m := mediaWikiTableHeaderRe.FindStringSubmatch(line)
			cells := splitTableCells(m[1], "!!")
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				if cell == "" {
					continue
				}
				ps.blockID++
				block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
				block.Name = "table-header"
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
			continue
		}

		// Table data cells
		if inTable && mediaWikiTableCellRe.MatchString(line) {
			r.skelText(rl.content + rl.lineEnding)
			m := mediaWikiTableCellRe.FindStringSubmatch(line)
			cells := splitTableCells(m[1], "||")
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				if cell == "" {
					continue
				}
				ps.blockID++
				block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
				block.Name = "table-cell"
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
			continue
		}

		// Image/file links with captions
		if mediaWikiImageRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			r.extractImageCaptions(ctx, ch, line, ps)
			continue
		}

		// Blank line separates paragraphs
		if strings.TrimSpace(line) == "" {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Regular text line -- accumulate into paragraph
		ps.paraLines = append(ps.paraLines, line)
		ps.paraLineIdxes = append(ps.paraLineIdxes, i)
	}

	// Flush remaining paragraph
	ps.flushParagraph(ctx, r, ch, rLines)
}

func (r *Reader) readDokuWiki(ctx context.Context, ch chan<- model.PartResult,
	scanner *bufio.Scanner, ps *parseState) {

	for scanner.Scan() {
		line := scanner.Text()

		// Check for header
		if m := dokuWikiHeaderRe.FindStringSubmatch(line); m != nil {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			ps.blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), strings.TrimSpace(m[2]))
			block.Name = "header"
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// DokuWiki table row
		if dokuWikiTableRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			r.extractDokuWikiTableCells(ctx, ch, line, ps)
			continue
		}

		// Blank line separates paragraphs
		if strings.TrimSpace(line) == "" {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Regular text -- accumulate into paragraph
		ps.paraLines = append(ps.paraLines, line)
	}

	// Flush remaining paragraph
	ps.flushParagraph(ctx, r, ch, nil)
}

func (r *Reader) readDokuWikiLines(ctx context.Context, ch chan<- model.PartResult,
	rLines []rawLine, ps *parseState) {

	for i, rl := range rLines {
		line := rl.content

		// Check for header
		if m := dokuWikiHeaderRe.FindStringSubmatch(line); m != nil {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			ps.blockID++
			blockID := fmt.Sprintf("tu%d", ps.blockID)
			r.skelRef(blockID)
			r.skelText(rl.lineEnding)
			block := model.NewBlock(blockID, strings.TrimSpace(m[2]))
			block.Name = "header"
			block.Properties["raw"] = line
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// DokuWiki table row
		if dokuWikiTableRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			r.extractDokuWikiTableCells(ctx, ch, line, ps)
			continue
		}

		// Blank line separates paragraphs
		if strings.TrimSpace(line) == "" {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Regular text -- accumulate into paragraph
		ps.paraLines = append(ps.paraLines, line)
		ps.paraLineIdxes = append(ps.paraLineIdxes, i)
	}

	// Flush remaining paragraph
	ps.flushParagraph(ctx, r, ch, rLines)
}

func (r *Reader) extractImageCaptions(ctx context.Context, ch chan<- model.PartResult,
	line string, ps *parseState) {

	matches := mediaWikiImageRe.FindAllStringSubmatch(line, -1)
	for _, m := range matches {
		if len(m) < 3 || m[2] == "" {
			continue
		}
		// m[2] contains |param1|param2|...|caption
		// The last pipe-separated segment is typically the caption.
		parts := strings.Split(m[2], "|")
		// Skip the first empty element (leading |)
		var caption string
		for i := len(parts) - 1; i >= 0; i-- {
			seg := strings.TrimSpace(parts[i])
			if seg == "" {
				continue
			}
			// Skip known MediaWiki image parameters
			lower := strings.ToLower(seg)
			if isMediaWikiImageParam(lower) {
				continue
			}
			caption = seg
			break
		}
		if caption != "" {
			ps.blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), caption)
			block.Name = "image-caption"
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
	}

	// Also emit any text outside the image link
	remainder := mediaWikiImageRe.ReplaceAllString(line, "")
	remainder = strings.TrimSpace(remainder)
	if remainder != "" {
		ps.blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), remainder)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}
}

func isMediaWikiImageParam(s string) bool {
	params := []string{
		"thumb", "thumbnail", "frame", "frameless", "border",
		"left", "right", "center", "none",
		"baseline", "sub", "super", "top", "text-top", "middle", "bottom", "text-bottom",
		"upright",
	}
	for _, p := range params {
		if s == p {
			return true
		}
	}
	// Prefixed params like "link=..."
	if strings.HasPrefix(s, "link=") || strings.HasPrefix(s, "alt=") || strings.HasPrefix(s, "page=") {
		return true
	}
	// Size spec like "200px" or "200x300px"
	if strings.HasSuffix(s, "px") {
		return true
	}
	return false
}

func (r *Reader) extractDokuWikiTableCells(ctx context.Context, ch chan<- model.PartResult,
	line string, ps *parseState) {

	// Remove leading/trailing | or ^
	trimmed := line
	if len(trimmed) > 0 && (trimmed[0] == '|' || trimmed[0] == '^') {
		trimmed = trimmed[1:]
	}
	if len(trimmed) > 0 && (trimmed[len(trimmed)-1] == '|' || trimmed[len(trimmed)-1] == '^') {
		trimmed = trimmed[:len(trimmed)-1]
	}

	// Split on | and ^
	var cells []string
	var current strings.Builder
	for _, c := range trimmed {
		if c == '|' || c == '^' {
			cells = append(cells, current.String())
			current.Reset()
		} else {
			current.WriteRune(c)
		}
	}
	cells = append(cells, current.String())

	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		ps.blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
		block.Name = "table-cell"
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}
}

func splitTableCells(content, separator string) []string {
	return strings.Split(content, separator)
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
