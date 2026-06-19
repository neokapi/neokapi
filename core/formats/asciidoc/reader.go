package asciidoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Reader implements DataFormatReader for AsciiDoc (.adoc) documents.
//
// Round-trip strategy: SkeletonStore. The reader walks the source byte stream
// once, emitting the non-translatable bytes between extracted spans as skeleton
// text and each extracted span as a skeleton Ref keyed by the block ID. The
// writer replays that stream, splicing each Ref's rendered (target-else-source)
// runs back in, so an untouched document round-trips byte-for-byte. This mirrors
// the markdown and properties readers.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs

	source       []byte
	cursor       int // skeleton position in source
	blockCounter int
	dataCounter  int
	groupCounter int
	groupStack   []groupFrame
	locale       model.LocaleID
	aborted      bool
}

// groupFrame is one open structural group on the reader's stack. level is only
// meaningful for kind=="section" (the AsciiDoc heading level it opened at).
type groupFrame struct {
	id    string
	kind  string
	level int
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// Line/block classifiers. AsciiDoc block markers sit at column 0.
var (
	reDocTitle   = regexp.MustCompile(`^=[ \t]+(\S.*)$`)
	reHeading    = regexp.MustCompile(`^(={1,6})[ \t]+(\S.*)$`)
	reAttrEntry  = regexp.MustCompile(`^:!?[A-Za-z0-9][A-Za-z0-9_-]*!?:([ \t]|$)`)
	reBlockAttr  = regexp.MustCompile(`^\[.*\]$`)
	reAdmonition = regexp.MustCompile(`^(NOTE|TIP|IMPORTANT|WARNING|CAUTION):[ \t]+(\S.*)$`)
	reUList      = regexp.MustCompile(`^(\*+|-)[ \t]+(\S.*)$`)
	reOList      = regexp.MustCompile(`^(\.+)[ \t]+(\S.*)$`)

	reListingDelim = regexp.MustCompile(`^-{4,}[ \t]*$`)
	reLiteralDelim = regexp.MustCompile(`^\.{4,}[ \t]*$`)
	rePassDelim    = regexp.MustCompile(`^\+{4,}[ \t]*$`)
	reCommentDelim = regexp.MustCompile(`^/{4,}[ \t]*$`)
	reTableDelim   = regexp.MustCompile(`^\|===[ \t]*$`)
	reExampleDelim = regexp.MustCompile(`^={4,}[ \t]*$`)
	reSidebarDelim = regexp.MustCompile(`^\*{4,}[ \t]*$`)
	reQuoteDelim   = regexp.MustCompile(`^_{4,}[ \t]*$`)
	reOpenDelim    = regexp.MustCompile(`^--[ \t]*$`)
)

// NewReader creates a new AsciiDoc reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "asciidoc",
			FormatDisplayName: "AsciiDoc",
			FormatMimeType:    "text/asciidoc",
			FormatExtensions:  []string{".adoc", ".asciidoc", ".adfm", ".asc"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore wires the skeleton store for byte-exact streaming output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) { r.skeletonStore = store }

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/asciidoc"},
		Extensions: []string{".adoc", ".asciidoc", ".adfm", ".asc"},
	}
}

// Open validates the document and stashes it.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("asciidoc: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		if err := r.readContent(ctx, ch); err != nil {
			ch <- model.PartResult{Error: err}
		}
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) error {
	content, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		return fmt.Errorf("asciidoc: reading: %w", err)
	}
	r.source = content
	r.cursor = 0

	r.locale = r.Doc.SourceLocale
	if r.locale.IsEmpty() {
		r.locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "asciidoc",
		Locale:   r.locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/asciidoc",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return nil
	}

	lines := splitLines(content)

	// The whole body lives in a single document group (reading-order root).
	if !r.openGroup(ctx, ch, "document", "document", 0, nil) {
		return nil
	}

	i := r.processHeader(ctx, ch, lines, 0)
	for i < len(lines) && !r.aborted {
		i = r.processBlock(ctx, ch, lines, i)
	}

	r.closeAllGroups(ctx, ch)

	// Flush any trailing skeleton bytes (final newline, trailing blanks).
	r.skelEmitGap(len(r.source))
	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
	return nil
}

// processHeader consumes the optional AsciiDoc document header: a `= Title`
// doctitle (extracted as a level-1 heading) followed by author/revision lines
// and attribute entries up to the first blank line (preserved as skeleton). It
// returns the index of the first body line.
func (r *Reader) processHeader(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i int) int {
	if i >= len(lines) {
		return i
	}
	loc := reDocTitle.FindStringSubmatchIndex(lines[i].text)
	if loc == nil {
		return i // no document header
	}
	first := lines[i]
	contentStart := first.start + loc[2]
	r.emitBlock(ctx, ch, first.start, contentStart, first.contentEnd,
		"doctitle", "title", model.RoleHeading, 1, nil)
	i++

	// Header zone: author, revision, attribute entries, comments — skeleton —
	// until the first blank line (which terminates the header).
	headerStart := -1
	for i < len(lines) && strings.TrimSpace(lines[i].text) != "" {
		if headerStart < 0 {
			headerStart = lines[i].start
		}
		i++
	}
	if headerStart >= 0 {
		end := lines[i-1].contentEnd
		r.emitData(ctx, ch, "document-header", string(r.source[headerStart:end]))
	}
	return i
}

// processBlock dispatches on the block at line i and returns the next line
// index to process.
func (r *Reader) processBlock(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i int) int {
	line := lines[i]
	text := line.text

	switch {
	case strings.TrimSpace(text) == "":
		// Blank line — skeleton (flushed lazily by the next gap).
		return i + 1

	case reCommentDelim.MatchString(text):
		return r.consumeDelimited(ctx, ch, lines, i, "comment-block", reCommentDelim)

	case strings.HasPrefix(text, "//"):
		r.emitData(ctx, ch, "comment", string(r.source[line.start:line.end]))
		return i + 1

	case reListingDelim.MatchString(text):
		return r.consumeDelimited(ctx, ch, lines, i, "listing", reListingDelim)
	case reLiteralDelim.MatchString(text):
		return r.consumeDelimited(ctx, ch, lines, i, "literal", reLiteralDelim)
	case rePassDelim.MatchString(text):
		return r.consumeDelimited(ctx, ch, lines, i, "passthrough", rePassDelim)

	case reTableDelim.MatchString(text):
		if r.cfg.ExtractTableCells {
			return r.processTable(ctx, ch, lines, i)
		}
		return r.consumeDelimited(ctx, ch, lines, i, "table", reTableDelim)

	case reAttrEntry.MatchString(text):
		r.emitData(ctx, ch, "attribute-entry", string(r.source[line.start:line.end]))
		return i + 1

	case reBlockAttr.MatchString(text):
		r.emitData(ctx, ch, "block-attribute", string(r.source[line.start:line.end]))
		return i + 1

	case isBlockTitle(text):
		if r.cfg.ExtractBlockTitles {
			r.emitBlock(ctx, ch, line.start, line.start+1, line.contentEnd,
				"block-title", "block-title", model.RoleCaption, 0, nil)
		} else {
			r.emitData(ctx, ch, "block-title", string(r.source[line.start:line.end]))
		}
		return i + 1

	case reHeading.MatchString(text):
		return r.emitHeading(ctx, ch, line, i)

	case reAdmonition.MatchString(text):
		loc := reAdmonition.FindStringSubmatchIndex(text)
		label := text[loc[2]:loc[3]]
		contentStart := line.start + loc[4]
		r.emitBlock(ctx, ch, line.start, contentStart, line.contentEnd,
			"admonition", "admonition", model.RoleParagraph, 0,
			map[string]string{"asciidoc.admonition": label})
		return i + 1

	case reExampleDelim.MatchString(text), reSidebarDelim.MatchString(text),
		reQuoteDelim.MatchString(text), reOpenDelim.MatchString(text):
		// Structural delimiter for example/sidebar/quote/open blocks: skeleton,
		// but the inner content is still processed as ordinary body.
		r.emitData(ctx, ch, "delimiter", string(r.source[line.start:line.end]))
		return i + 1

	case reUList.MatchString(text), reOList.MatchString(text):
		return r.processList(ctx, ch, lines, i)

	default:
		return r.emitParagraph(ctx, ch, lines, i)
	}
}

// emitHeading emits a section heading block (RoleHeading, level = number of
// leading `=`) and manages the section group stack so sections nest in reading
// order.
func (r *Reader) emitHeading(ctx context.Context, ch chan<- model.PartResult, line srcLine, i int) int {
	loc := reHeading.FindStringSubmatchIndex(line.text)
	level := loc[3] - loc[2] // length of the `=` run
	contentStart := line.start + loc[4]

	r.closeSectionsGE(ctx, ch, level)
	r.emitBlock(ctx, ch, line.start, contentStart, line.contentEnd,
		fmt.Sprintf("heading-%d", level), "heading", model.RoleHeading, level, nil)
	r.openGroup(ctx, ch, "section", "section", level, nil)
	return i + 1
}

// emitParagraph collects consecutive non-blank, non-block-start lines into one
// paragraph block (RoleParagraph). Internal soft line breaks ride inside the
// block text and round-trip via the runs' Data.
func (r *Reader) emitParagraph(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i int) int {
	start := i
	for i < len(lines) {
		if strings.TrimSpace(lines[i].text) == "" || (i > start && r.isBlockStart(lines[i].text)) {
			break
		}
		i++
	}
	contentStart := lines[start].start
	contentEnd := lines[i-1].contentEnd
	r.emitBlock(ctx, ch, contentStart, contentStart, contentEnd,
		fmt.Sprintf("para%d", r.blockCounter+1), "paragraph", model.RoleParagraph, 0, nil)
	return i
}

// processList brackets a run of consecutive list items in a list group and
// emits each item as a RoleListItem block (level = marker depth).
func (r *Reader) processList(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i int) int {
	if !r.openGroup(ctx, ch, "list", "list", 0, nil) {
		return len(lines)
	}
	for i < len(lines) && !r.aborted {
		text := lines[i].text
		var loc []int
		if reUList.MatchString(text) {
			loc = reUList.FindStringSubmatchIndex(text)
		} else if reOList.MatchString(text) {
			loc = reOList.FindStringSubmatchIndex(text)
		} else {
			break
		}
		marker := text[loc[2]:loc[3]]
		level := len(marker)
		if marker == "-" {
			level = 1
		}
		contentStart := lines[i].start + loc[4]
		r.emitBlock(ctx, ch, lines[i].start, contentStart, lines[i].contentEnd,
			fmt.Sprintf("item%d", r.blockCounter+1), "list-item", model.RoleListItem, level, nil)
		i++
	}
	r.closeGroup(ctx, ch)
	return i
}

// processTable parses a `|===` table into a table group of table-row groups of
// cell blocks. Header cells carry RoleTableHeader, body cells RoleTableCell.
// Cell text round-trips byte-exact via the skeleton; the pipes, specs, and
// newlines stay skeleton.
func (r *Reader) processTable(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i int) int {
	// Find the closing delimiter (or run to EOF).
	closeIdx := -1
	for j := i + 1; j < len(lines); j++ {
		if reTableDelim.MatchString(lines[j].text) {
			closeIdx = j
			break
		}
	}
	end := closeIdx
	if end < 0 {
		end = len(lines)
	}

	cells := r.collectTableCells(lines, i+1, end)
	if len(cells) == 0 {
		// No usable cells — fall back to verbatim skeleton.
		next := end + 1
		if closeIdx < 0 {
			next = len(lines)
		}
		r.emitData(ctx, ch, "table", "")
		return next
	}

	// Default column count = cells on the first content row. Header row when a
	// blank line immediately follows the first content row.
	colCount := cells[0].rowCells
	header := cells[0].headerRow

	if !r.openGroup(ctx, ch, "table", "table", 0, nil) {
		return len(lines)
	}
	col := 0
	rowIsHeader := header
	rowOpen := false
	for _, c := range cells {
		if col == 0 {
			props := map[string]string(nil)
			if rowIsHeader {
				props = map[string]string{"header": "true"}
			}
			if !r.openGroup(ctx, ch, "table-row", "table-row", 0, props) {
				return len(lines)
			}
			rowOpen = true
		}
		role := model.RoleTableCell
		if rowIsHeader {
			role = model.RoleTableHeader
		}
		r.emitBlock(ctx, ch, c.cStart, c.cStart, c.cEnd,
			fmt.Sprintf("cell%d", r.blockCounter+1), "table-cell", role, 0,
			map[string]string{"column": strconv.Itoa(col)})
		col++
		if col >= colCount {
			r.closeGroup(ctx, ch) // table-row
			rowOpen = false
			col = 0
			rowIsHeader = false // only the first row may be a header
		}
	}
	if rowOpen {
		r.closeGroup(ctx, ch) // trailing partial row
	}
	r.closeGroup(ctx, ch) // table

	if closeIdx < 0 {
		return len(lines)
	}
	return closeIdx + 1
}

// tableCell is one non-empty cell's content byte range plus per-first-cell
// metadata used for column/header inference.
type tableCell struct {
	cStart, cEnd int
	rowCells     int  // cells on the first content row (only set on cells[0])
	headerRow    bool // first content row is a header (only set on cells[0])
}

// collectTableCells scans the table body lines [from,to) and returns the
// non-empty cells in document order with absolute byte offsets.
func (r *Reader) collectTableCells(lines []srcLine, from, to int) []tableCell {
	var cells []tableCell
	firstContent := -1
	firstRowCells := 0
	for j := from; j < to; j++ {
		text := lines[j].text
		if strings.TrimSpace(text) == "" {
			continue
		}
		if firstContent < 0 {
			firstContent = j
		}
		base := lines[j].start
		n := lineCellCount(text)
		if j == firstContent {
			firstRowCells = n
		}
		for _, seg := range splitCells(text) {
			cs := base + seg.start
			ce := base + seg.end
			if ce <= cs {
				continue // empty cell — stays skeleton
			}
			cells = append(cells, tableCell{cStart: cs, cEnd: ce})
		}
	}
	if len(cells) > 0 {
		cells[0].rowCells = max(firstRowCells, 1)
		// Header row when a blank line immediately follows the first content row.
		if firstContent >= 0 && firstContent+1 < to &&
			strings.TrimSpace(lines[firstContent+1].text) == "" {
			cells[0].headerRow = true
		}
	}
	return cells
}

// cellSeg is the trimmed content byte range of one cell within a line.
type cellSeg struct{ start, end int }

// splitCells splits a PSV table line on `|` separators and returns the trimmed
// content range of each cell (relative to the line). The leading segment before
// the first `|` is ignored.
func splitCells(line string) []cellSeg {
	var out []cellSeg
	i := 0
	n := len(line)
	for i < n {
		if line[i] != '|' {
			i++
			continue
		}
		// Cell content runs from after this `|` to the next `|` or EOL.
		segStart := i + 1
		j := segStart
		for j < n && line[j] != '|' {
			j++
		}
		cs, ce := trimSpaceRange(line, segStart, j)
		out = append(out, cellSeg{start: cs, end: ce})
		i = j
	}
	return out
}

// lineCellCount returns the number of `|`-separated cells on a table line.
func lineCellCount(line string) int {
	return strings.Count(line, "|")
}

// trimSpaceRange returns the [start,end) sub-range of s[start:end] with leading
// and trailing ASCII spaces/tabs removed.
func trimSpaceRange(s string, start, end int) (int, int) {
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return start, end
}

// consumeDelimited consumes a verbatim delimited block (listing / literal /
// passthrough / comment / opaque table) from its opening delimiter at line i to
// the matching closing delimiter, emitting it as a single non-translatable Data
// part. The bytes stay skeleton, so the block round-trips verbatim.
func (r *Reader) consumeDelimited(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i int, name string, closeRE *regexp.Regexp) int {
	startByte := lines[i].start
	j := i + 1
	for j < len(lines) {
		if closeRE.MatchString(lines[j].text) {
			break
		}
		j++
	}
	var endByte int
	var next int
	if j < len(lines) {
		endByte = lines[j].end
		next = j + 1
	} else {
		endByte = len(r.source)
		next = len(lines)
	}
	r.emitData(ctx, ch, name, string(r.source[startByte:endByte]))
	return next
}

// emitBlock emits one translatable block whose source runs are the inline-parsed
// text in [contentStart,contentEnd). The structural prefix [lineStart,
// contentStart) is written to the skeleton verbatim, the content as a Ref.
func (r *Reader) emitBlock(ctx context.Context, ch chan<- model.PartResult,
	lineStart, contentStart, contentEnd int, name, blockType, role string, level int, props map[string]string) bool {

	r.blockCounter++
	id := fmt.Sprintf("tu%d", r.blockCounter)
	text := string(r.source[contentStart:contentEnd])

	block := model.NewBlock(id, text)
	block.Name = name
	block.Type = blockType
	block.SourceLocale = r.locale
	block.Source = parseInline(text)
	if role != "" {
		block.SetSemanticRole(role, level)
	}
	if level > 0 {
		block.Properties["level"] = strconv.Itoa(level)
	}
	maps.Copy(block.Properties, props)

	r.skelEmitGap(lineStart)
	if contentStart > lineStart {
		r.skelText(string(r.source[lineStart:contentStart]))
	}
	r.skelRef(id)
	r.cursor = contentEnd

	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// emitData emits a non-translatable Data part carrying the raw bytes of a
// skeleton construct (so it is visible to downstream tools and recoverable by
// the non-skeleton writer path). The bytes themselves stay in the skeleton.
func (r *Reader) emitData(ctx context.Context, ch chan<- model.PartResult, name, raw string) bool {
	r.dataCounter++
	d := &model.Data{
		ID:         fmt.Sprintf("d%d", r.dataCounter),
		Name:       name,
		Properties: map[string]string{"raw": raw},
	}
	return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d})
}

// --- structural groups ---

func (r *Reader) openGroup(ctx context.Context, ch chan<- model.PartResult, name, typ string, level int, props map[string]string) bool {
	r.groupCounter++
	gid := fmt.Sprintf("g%d", r.groupCounter)
	r.groupStack = append(r.groupStack, groupFrame{id: gid, kind: typ, level: level})
	return r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart,
		Resource: &model.GroupStart{ID: gid, Name: name, Type: typ, Properties: props}})
}

func (r *Reader) closeGroup(ctx context.Context, ch chan<- model.PartResult) bool {
	n := len(r.groupStack)
	if n == 0 {
		return true
	}
	top := r.groupStack[n-1]
	r.groupStack = r.groupStack[:n-1]
	return r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: top.id}})
}

func (r *Reader) closeSectionsGE(ctx context.Context, ch chan<- model.PartResult, level int) {
	for len(r.groupStack) > 0 {
		top := r.groupStack[len(r.groupStack)-1]
		if top.kind != "section" || top.level < level {
			return
		}
		if !r.closeGroup(ctx, ch) {
			return
		}
	}
}

func (r *Reader) closeAllGroups(ctx context.Context, ch chan<- model.PartResult) {
	for len(r.groupStack) > 0 {
		if !r.closeGroup(ctx, ch) {
			return
		}
	}
}

// --- skeleton helpers (markdown-style cursor walk) ---

func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		r.skelBuf.WriteString(s)
	}
}

func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

// skelEmitGap flushes source bytes from the cursor up to pos as skeleton text.
func (r *Reader) skelEmitGap(pos int) {
	if pos > r.cursor {
		r.skelText(string(r.source[r.cursor:pos]))
		r.cursor = pos
	}
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		r.aborted = true
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

// --- line model ---

// srcLine is one physical line: its byte range, the byte where content ends
// (before the terminator), and the content text without the terminator.
type srcLine struct {
	start      int
	contentEnd int
	end        int
	text       string
}

// splitLines splits src into physical lines, recognising LF and CRLF
// terminators and a final unterminated line.
func splitLines(src []byte) []srcLine {
	var lines []srcLine
	i := 0
	n := len(src)
	for i < n {
		j := i
		for j < n && src[j] != '\n' {
			j++
		}
		contentEnd := j
		if contentEnd > i && src[contentEnd-1] == '\r' {
			contentEnd--
		}
		end := j
		if j < n {
			end = j + 1 // include the LF
		}
		lines = append(lines, srcLine{
			start:      i,
			contentEnd: contentEnd,
			end:        end,
			text:       string(src[i:contentEnd]),
		})
		if j >= n {
			break
		}
		i = j + 1
	}
	return lines
}

// isBlockTitle reports whether text is an AsciiDoc block title (`.Title`): a
// leading `.` immediately followed by a non-dot, non-space character.
func isBlockTitle(text string) bool {
	if len(text) < 2 || text[0] != '.' {
		return false
	}
	c := text[1]
	return c != '.' && c != ' ' && c != '\t'
}

// isBlockStart reports whether text begins a non-paragraph block construct,
// terminating an in-progress paragraph.
func (r *Reader) isBlockStart(text string) bool {
	if strings.TrimSpace(text) == "" {
		return true
	}
	if reHeading.MatchString(text) || reUList.MatchString(text) || reOList.MatchString(text) {
		return true
	}
	if isBlockTitle(text) || reAdmonition.MatchString(text) {
		return true
	}
	if reAttrEntry.MatchString(text) || reBlockAttr.MatchString(text) {
		return true
	}
	if strings.HasPrefix(text, "//") {
		return true
	}
	return reListingDelim.MatchString(text) || reLiteralDelim.MatchString(text) ||
		rePassDelim.MatchString(text) || reCommentDelim.MatchString(text) ||
		reTableDelim.MatchString(text) || reExampleDelim.MatchString(text) ||
		reSidebarDelim.MatchString(text) || reQuoteDelim.MatchString(text) ||
		reOpenDelim.MatchString(text)
}
