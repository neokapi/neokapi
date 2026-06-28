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

	// reCellSpec matches an AsciiDoc table cell SPAN spec preceding a `|`:
	// `N+` (colspan N), `.M+` (rowspan M), `N.M+` (both). Alignment/style/repeat
	// operators are not parsed here.
	reCellSpec = regexp.MustCompile(`^(\d+)?(?:\.(\d+))?\+$`)

	// reColsAttr captures the value of a table `cols=` attribute, quoted or bare
	// (e.g. cols=3, cols="1,1,1", cols="3*"). reHeaderOpt matches the header
	// option in either the `%header` shorthand or `options="header"` long form.
	reColsAttr  = regexp.MustCompile(`cols\s*=\s*(?:"([^"]*)"|([^,\s\]]+))`)
	reHeaderOpt = regexp.MustCompile(`%header\b|options\s*=\s*"?[^"\]]*\bheader\b`)
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
		return r.consumeVerbatimContent(ctx, ch, lines, i, "listing", "----", reListingDelim)
	case reLiteralDelim.MatchString(text):
		return r.consumeVerbatimContent(ctx, ch, lines, i, "literal", "....", reLiteralDelim)
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
		// A block-attribute line applies to the block that immediately follows.
		// When that block is a table, capture its `cols=` / `%header` so the table
		// parses with the right column count and header even when each cell is on
		// its own line (no blank-line row boundary to infer the width from). The
		// attribute bytes still ride the skeleton for a byte-exact round-trip.
		if r.cfg.ExtractTableCells && i+1 < len(lines) && reTableDelim.MatchString(lines[i+1].text) {
			cols, header := parseTableAttrs(text)
			r.emitData(ctx, ch, "block-attribute", string(r.source[line.start:line.end]))
			return r.processTableWith(ctx, ch, lines, i+1, cols, header)
		}
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

	case text[0] == ' ' || text[0] == '\t':
		// A line whose first character is whitespace is an AsciiDoc literal
		// paragraph (verbatim) — surface it as non-translatable content, not prose.
		return r.emitIndentedLiteral(ctx, ch, lines, i)

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

// emitIndentedLiteral collects a contiguous run of leading-whitespace lines into
// one literal-paragraph block. AsciiDoc treats an indented paragraph as verbatim
// literal content, so it is surfaced as non-translatable content (RoleCode), not
// translatable prose — the leading indent is significant and stays in the body.
func (r *Reader) emitIndentedLiteral(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i int) int {
	start := i
	for i < len(lines) {
		t := lines[i].text
		if strings.TrimSpace(t) == "" || len(t) == 0 || (t[0] != ' ' && t[0] != '\t') {
			break
		}
		i++
	}
	contentStart := lines[start].start
	contentEnd := lines[i-1].contentEnd
	r.emitContentBlock(ctx, ch, contentStart, contentStart, contentEnd,
		fmt.Sprintf("literal%d", r.blockCounter+1), "literal-paragraph", model.RoleCode, nil)
	return i
}

// processList brackets a run of consecutive list items in a list group and
// emits each item as a RoleListItem block (level = marker depth).
func (r *Reader) processList(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i int) int {
	// Record whether this list is ordered (`.`/numbered) or unordered (`*`/`-`)
	// on the group — the one structural signal a downstream consumer (and an
	// ordered/unordered round-trip) needs that the bare item depth does not carry.
	listProps := map[string]string{"ordered": "false"}
	if i < len(lines) && reOList.MatchString(lines[i].text) {
		listProps["ordered"] = "true"
	}
	if !r.openGroup(ctx, ch, "list", "list", 0, listProps) {
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
	return r.processTableWith(ctx, ch, lines, i, 0, false)
}

// processTableWith is processTable with optional column/header overrides parsed
// from a preceding `[cols=N,%header]` block attribute. attrCols > 0 sets the
// column count (so per-line cells group into rows of N); attrHeader promotes the
// first row to a header. When both are zero/false the table is inferred from its
// content (cells on the first row; header when a blank line follows it).
func (r *Reader) processTableWith(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i, attrCols int, attrHeader bool) int {
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
	// blank line immediately follows the first content row. An explicit
	// `[cols=N]` / `%header` attribute overrides the inference.
	colCount := cells[0].rowCells
	header := cells[0].headerRow
	if attrCols > 0 {
		colCount = attrCols
	}
	if attrHeader {
		header = true
	}

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
		props := map[string]string{"column": strconv.Itoa(col)}
		span := 1
		if c.colspan > 1 {
			props["colspan"] = strconv.Itoa(c.colspan)
			span = c.colspan
		}
		if c.rowspan > 1 {
			props["rowspan"] = strconv.Itoa(c.rowspan)
		}
		r.emitBlock(ctx, ch, c.cStart, c.cStart, c.cEnd,
			fmt.Sprintf("cell%d", r.blockCounter+1), "table-cell", role, 0, props)
		col += span
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

// parseTableAttrs extracts the column count and header flag from a table
// block-attribute line (e.g. `[%header,cols=3]`). cols is 0 when no parseable
// `cols=` is present (the caller then infers from content).
func parseTableAttrs(line string) (cols int, header bool) {
	if m := reColsAttr.FindStringSubmatch(line); m != nil {
		val := m[1]
		if val == "" {
			val = m[2]
		}
		cols = colsCount(val)
	}
	header = reHeaderOpt.MatchString(line)
	return cols, header
}

// colsCount derives the logical column count from a `cols=` value: a plain
// integer (`3`), a comma list of column specs (`1,1,1` → 3), or the repeat form
// (`3*` / `3*<spec>` → 3). Returns 0 when it cannot be determined.
func colsCount(spec string) int {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0
	}
	if star := strings.IndexByte(spec, '*'); star > 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(spec[:star])); err == nil && n > 0 {
			return n
		}
	}
	if strings.Contains(spec, ",") {
		return len(strings.Split(spec, ","))
	}
	if n, err := strconv.Atoi(spec); err == nil && n > 0 {
		return n
	}
	return 0
}

// tableCell is one non-empty cell's content byte range plus per-first-cell
// metadata used for column/header inference.
type tableCell struct {
	cStart, cEnd     int
	colspan, rowspan int  // merged-cell extents (0 = default 1)
	rowCells         int  // cells on the first content row (only set on cells[0])
	headerRow        bool // first content row is a header (only set on cells[0])
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
			cells = append(cells, tableCell{cStart: cs, cEnd: ce, colspan: seg.colspan, rowspan: seg.rowspan})
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

// cellSeg is the trimmed content byte range of one cell within a line, plus any
// colspan/rowspan parsed from the cell's span spec (0 = default 1).
type cellSeg struct {
	start, end       int
	colspan, rowspan int
}

// splitCells splits a PSV table line on `|` separators and returns the trimmed
// content range of each cell (relative to the line). A span spec immediately
// preceding a `|` (`2+`, `.3+`, `2.3+`) — whether in the leading segment or as
// the trailing token of the previous cell — attaches to the cell that `|` opens.
func splitCells(line string) []cellSeg {
	var out []cellSeg
	n := len(line)
	firstPipe := strings.IndexByte(line, '|')
	if firstPipe < 0 {
		return out
	}
	// A span spec in the leading segment binds to the first cell.
	pendCol, pendRow := parseCellSpec(line[:firstPipe])
	for i := firstPipe; i >= 0 && i < n; {
		segStart := i + 1
		j := segStart
		for j < n && line[j] != '|' {
			j++
		}
		// When another `|` follows, a span spec may be the trailing token of this
		// cell's text — it belongs to the NEXT cell, so strip it from this one.
		rawEnd := j
		nextCol, nextRow := 0, 0
		if j < n {
			if specStart, c, rr := trailingCellSpec(line, segStart, j); specStart >= 0 {
				rawEnd = specStart
				nextCol, nextRow = c, rr
			}
		}
		cs, ce := trimSpaceRange(line, segStart, rawEnd)
		if ce > cs {
			out = append(out, cellSeg{start: cs, end: ce, colspan: pendCol, rowspan: pendRow})
		}
		pendCol, pendRow = nextCol, nextRow
		i = j
	}
	return out
}

// parseCellSpec parses an AsciiDoc cell span spec token (e.g. "2+", ".3+",
// "2.3+"); returns (0,0) when s is not a span spec. A bare "+" means span 1.
func parseCellSpec(s string) (col, row int) {
	m := reCellSpec.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, 0
	}
	col, row = 1, 1
	if m[1] != "" {
		col, _ = strconv.Atoi(m[1])
	}
	if m[2] != "" {
		row, _ = strconv.Atoi(m[2])
	}
	return col, row
}

// trailingCellSpec reports whether the last whitespace-delimited token of
// line[from:to] is a cell span spec. It returns the spec's start offset (so the
// caller can trim it off the current cell) and the parsed col/row, or (-1,0,0).
func trailingCellSpec(line string, from, to int) (specStart, col, row int) {
	end := to
	for end > from && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}
	start := end
	for start > from && line[start-1] != ' ' && line[start-1] != '\t' {
		start--
	}
	if c, r := parseCellSpec(line[start:end]); c != 0 {
		return start, c, r
	}
	return -1, 0, 0
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

// consumeVerbatimContent consumes a listing/literal delimited block and surfaces
// its BODY as a non-translatable content block (RoleCode) rather than burying it
// in opaque skeleton, while keeping the open/close delimiters as skeleton so the
// block round-trips byte-exact. fence is the canonical delimiter re-synthesized
// on the normalized (no-skeleton) writer path. Unterminated or empty blocks fall
// back to the verbatim-skeleton path.
func (r *Reader) consumeVerbatimContent(ctx context.Context, ch chan<- model.PartResult, lines []srcLine, i int, name, fence string, closeRE *regexp.Regexp) int {
	j := i + 1
	for j < len(lines) && !closeRE.MatchString(lines[j].text) {
		j++
	}
	if j >= len(lines) {
		return r.consumeDelimited(ctx, ch, lines, i, name, closeRE) // unterminated
	}
	bodyStart := lines[i].end
	bodyEnd := lines[j].start
	if bodyEnd <= bodyStart {
		return r.consumeDelimited(ctx, ch, lines, i, name, closeRE) // empty body
	}
	r.emitContentBlock(ctx, ch, bodyStart, bodyStart, bodyEnd,
		name, name, model.RoleCode, map[string]string{"asciidoc.fence": fence})
	return j + 1
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
	// Promote table-cell span props to the typed structure annotation so
	// geometry/structure consumers (and DocLang export) see the merged extents.
	if s, ok := block.Structure(); ok && s != nil {
		if v := block.Properties["colspan"]; v != "" {
			s.ColSpan, _ = strconv.Atoi(v)
		}
		if v := block.Properties["rowspan"]; v != "" {
			s.RowSpan, _ = strconv.Atoi(v)
		}
	}

	r.skelEmitGap(lineStart)
	if contentStart > lineStart {
		r.skelText(string(r.source[lineStart:contentStart]))
	}
	r.skelRef(id)
	r.cursor = contentEnd

	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// emitContentBlock emits a NON-translatable content block: the bytes in
// [contentStart,contentEnd) are surfaced as first-class, role-tagged content (a
// single verbatim run, whitespace-significant, NOT inline-parsed) rather than
// buried in opaque skeleton — so an ingestion/LLM consumer sees the contextual
// content (code, literal text) while MT still skips it (Translatable=false). The
// structural prefix [lineStart,contentStart) stays skeleton and the body rides as
// a Ref, so the byte-exact round-trip is preserved.
func (r *Reader) emitContentBlock(ctx context.Context, ch chan<- model.PartResult,
	lineStart, contentStart, contentEnd int, name, blockType, role string, props map[string]string) bool {

	r.blockCounter++
	id := fmt.Sprintf("tu%d", r.blockCounter)
	text := string(r.source[contentStart:contentEnd])

	block := model.NewBlock(id, text) // default Source is a single verbatim run
	block.Name = name
	block.Type = blockType
	block.SourceLocale = r.locale
	block.Translatable = false
	block.PreserveWhitespace = true
	if role != "" {
		block.SetSemanticRole(role, 0)
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
