package markdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
)

// Writer implements DataFormatWriter for Markdown files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	firstBlock    bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new Markdown writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "markdown",
		},
		cfg:        cfg,
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed Markdown.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)
	var events []*model.Part // blocks + group brackets, in stream order

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
					events = append(events, part)
				}
			case model.PartGroupStart, model.PartGroupEnd:
				events = append(events, part)
			}
		}
	}
done:
	// Wrap the output writer with a per-line trim that mirrors upstream
	// Okapi's MarkdownFilterWriter.trimNonEssentialTrailingSpaces (see
	// MarkdownFilterWriter.java:103-122): on each line break, if the
	// previous line ends in EXACTLY one trailing single space drop that
	// space. The upstream implementation ALSO strips lines made of all
	// spaces, but its skeleton writer (MarkdownSkeletonWriter.java:58
	// appendLinePrefix) re-prepends the per-block line prefix on every
	// line including the now-stripped ones — and the trim doesn't
	// reach those re-prepended bytes because they enter the writer
	// after the next \n. The net effect upstream is that "indent\n"
	// rows survive unchanged. Mirror the net effect here, not the
	// literal Java algorithm: only strip exactly-1-trailing-space.
	// Without this wrap, fixtures like test-html-block-newline.md
	// round-trip with `". \n"` (single trailing space) where okapi
	// emits `".\n"`.
	tw := newTrailSpaceTrimmer(w.Output)
	defer func() { _ = tw.Flush() }()

	// Mode 1: Skeleton store (byte-exact, streaming-friendly).
	if w.skeletonStore != nil {
		if err := w.writeFromSkeleton(w.skeletonStore, blocksByID, tw); err != nil {
			return err
		}
		return tw.Flush()
	}

	// Mode 2: Build from the ordered event stream (cross-format export). Table
	// groups render as GFM tables; everything else renders block-by-block.
	if err := w.writeFromEvents(events, tw); err != nil {
		return err
	}
	return tw.Flush()
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block, out io.Writer) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("markdown writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := out.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				if _, err := io.WriteString(out, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// mdInlineTag maps a canonical inline run Type to its Markdown delimiters
// (open, close). Used by the cross-format semantic export path so inline
// formatting renders as Markdown (**bold**, _italic_) regardless of the source
// format — rather than echoing the source's captured Data (e.g. DocLang's
// "<bold>"). This table is the Markdown format's own projection of the shared
// canonical type vocabulary (core/model/vocabularies); link:hyperlink and
// media:image are handled separately in renderInlineMarkdown because they carry
// attributes. TestMarkdownInlineTagCoverage asserts this table covers every
// attribute-free formatting type the vocabulary defines, so a newly added type
// cannot silently fall through to plain text.
//
// Types with no native Markdown syntax (underline, super/subscript, bidi,
// handwriting) fall back to inline HTML, which Markdown passes through verbatim.
var mdInlineTag = map[string][2]string{
	"fmt:bold":          {"**", "**"},
	"fmt:italic":        {"*", "*"},
	"fmt:code":          {"`", "`"},
	"fmt:strikethrough": {"~~", "~~"},
	"fmt:highlight":     {"<mark>", "</mark>"},
	"fmt:underline":     {"<u>", "</u>"},
	"fmt:superscript":   {"<sup>", "</sup>"},
	"fmt:subscript":     {"<sub>", "</sub>"},
	"fmt:bidi":          {`<bdi dir="rtl">`, "</bdi>"},
	"fmt:handwriting":   {`<span class="handwriting">`, "</span>"},
}

// mdLinkClose builds the Markdown closing syntax for a link or image from its
// run attributes: `](dest)` or `](dest "title")`. destKey is model.AttrHref for
// links, model.AttrSrc for images. A nil/empty attrs map yields `]()`.
func mdLinkClose(attrs map[string]string, destKey string) string {
	dest := attrs[destKey]
	if title := attrs[model.AttrTitle]; title != "" {
		return "](" + dest + ` "` + title + `")`
	}
	return "](" + dest + ")"
}

// renderInlineMarkdown renders a run sequence as Markdown inline content: text
// verbatim, inline formatting from the run type (balanced via a tag stack),
// placeholders as their equivalent text. This is the cross-format projection —
// it never consults a run's Data, so the same Markdown results whatever the
// source format.
//
// Boundary whitespace is trimmed, because Markdown cannot represent it in an
// inline position: a paragraph's, heading's, list item's, blockquote's or table
// cell's leading and trailing whitespace is stripped when the document is read
// back. Emitting it produced text that did not survive a re-read.
//
// That is not a hypothetical. This path drops constructs it has no Markdown
// spelling for — inline HTML, most of all — which is accepted lossiness in
// markup. But dropping a construct at a block's edge PROMOTES the whitespace
// beside it to the boundary, so `<A> 0` rendered as " 0" and read back as "0":
// the loss stopped being confined to markup and started changing the text.
// Block identity is content-derived (AD-036), so the block then hashes to a
// different content key on the second pass and the content-memory matches from
// the first stop hitting (#1603).
//
// Callers rendering content inside a fence (RoleCode) want the untrimmed form —
// there, boundary whitespace is both representable and meaningful.
func renderInlineMarkdown(runs []model.Run) string {
	return strings.TrimSpace(renderInlineMarkdownRaw(runs))
}

// renderInlineMarkdownRaw is renderInlineMarkdown without the boundary trim.
func renderInlineMarkdownRaw(runs []model.Run) string {
	sink := &mdInlineSink{}
	projection.WalkInline(runs, sink)
	sink.flush()
	return sink.sb.String()
}

// mdInlineSink maps the shared inline-run stream (projection.WalkInline) to
// Markdown, owning the open-delimiter stack the paired-code close needs. It
// replaces the writer's former bespoke run loop; WalkInline now handles run
// decoding + plural/select 'other'-branch resolution. Like the old loop it never
// consults a run's Data — the same Markdown results whatever the source format.
type mdInlineSink struct {
	sb   strings.Builder
	open []string // stack of closing delimiters (or "" for dropped tags)
}

func (s *mdInlineSink) Text(t string) { s.sb.WriteString(t) }

func (s *mdInlineSink) Open(r *model.PcOpenRun) {
	switch r.Type {
	case "link:hyperlink":
		// [text](href "title") — the link text is the paired content.
		s.sb.WriteString("[")
		s.open = append(s.open, mdLinkClose(r.Attrs, model.AttrHref))
	case "media:image", "link:image":
		// ![alt](src "title") — the alt text is the paired content.
		s.sb.WriteString("![")
		s.open = append(s.open, mdLinkClose(r.Attrs, model.AttrSrc))
	default:
		if m, ok := mdInlineTag[r.Type]; ok {
			s.sb.WriteString(m[0])
			s.open = append(s.open, m[1])
		} else {
			s.open = append(s.open, "")
		}
	}
}

func (s *mdInlineSink) Close(*model.PcCloseRun) {
	if n := len(s.open); n > 0 {
		c := s.open[n-1]
		s.open = s.open[:n-1]
		s.sb.WriteString(c)
	}
}

func (s *mdInlineSink) Placeholder(r *model.PlaceholderRun) {
	switch r.Type {
	case "media:image", "link:image":
		// Self-closing image (e.g. read from HTML <img>): the alt text lives in
		// the run attributes, not as paired content.
		s.sb.WriteString("![" + r.Attr(model.AttrAlt) + mdLinkClose(r.Attrs, model.AttrSrc))
	default:
		if r.Equiv != "" {
			s.sb.WriteString(r.Equiv)
		}
	}
}

func (s *mdInlineSink) flush() {
	for i := len(s.open) - 1; i >= 0; i-- {
		s.sb.WriteString(s.open[i])
	}
}

// writeFromEvents reconstructs markdown from the ordered block + group event
// stream (cross-format semantic export). A `table` group and its `table-row`
// children render as a GFM table; every other block renders individually.
// Non-table group brackets are transparent — their child blocks render in
// place exactly as the block-only path did.
func (w *Writer) writeFromEvents(events []*model.Part, out io.Writer) error {
	for i := 0; i < len(events); i++ {
		part := events[i]
		switch part.Type {
		case model.PartGroupStart:
			if g, ok := part.Resource.(*model.GroupStart); ok && g.Type == "table" {
				end, rows := w.collectTable(events, i)
				if err := w.writeTable(rows, out); err != nil {
					return err
				}
				i = end
			}
		case model.PartBlock:
			if block, ok := part.Resource.(*model.Block); ok {
				if err := w.writeBlockMarkdown(block, out); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeBlockMarkdown renders one block, role-prefixed, separated from prior
// output by a blank line. Inline formatting renders from each run's vocabulary
// type, not its source-format Data, so the same Markdown results whatever the
// source format.
func (w *Writer) writeBlockMarkdown(block *model.Block, out io.Writer) error {
	// "omml-nor" blocks are the translatable prose spans of an OpenXML equation,
	// surfaced for docx write-back; their text is already in the formula's LaTeX,
	// so skip them in cross-format output to avoid duplication.
	if block.Type == "omml-nor" {
		return nil
	}

	if !w.firstBlock {
		if _, err := fmt.Fprint(out, "\n\n"); err != nil {
			return err
		}
	}
	w.firstBlock = false

	// Structure prefix/suffix, keyed on the normalized semantic role (WS6).
	// SemanticRole drives clean cross-format export (any source → Markdown);
	// it falls back to the format-specific block.Type so same-format
	// round-trips are unchanged.
	role := block.SemanticRole()
	if role == "" {
		role = block.Type
	}

	// Fenced code carries its own leading/trailing whitespace — indentation is
	// the content there. Every other role lands in an inline position, where
	// Markdown strips boundary whitespace on re-read, so the writer trims it
	// rather than emitting text that will not survive (#1603).
	text := renderInlineMarkdown(w.blockRuns(block))
	if role == model.RoleCode {
		text = renderInlineMarkdownRaw(w.blockRuns(block))
	}

	var prefix, suffix string
	switch role {
	case model.RoleTitle:
		prefix = "# "
	case model.RoleHeading:
		if n := block.HeadingLevel(); n > 0 {
			prefix = strings.Repeat("#", n) + " "
		}
	case model.RoleListItem:
		prefix = "- "
	case model.RoleCode:
		// Re-emit the fenced code block's info string (language) so the
		// do-not-translate signal survives cross-format export.
		prefix, suffix = "```"+block.CodeLanguage()+"\n", "\n```"
	case model.RoleCaption:
		prefix, suffix = "*", "*"
	}

	// A paragraph is the switch's implicit default (empty role, no prefix): the
	// rebuild path emits its text at the very start of a line. Block-shaped
	// residues have to be repaired there, or the paragraph re-reads as a
	// different block kind and trip(trip(x)) != trip(x):
	//
	//   - a bare leading marker left behind when a dropped inline construct
	//     exposes it ("#<A>" -> "#", which re-parses as an empty heading) —
	//     backslash-escape it so CommonMark keeps it literal (#1632);
	//   - a multi-line blockquote body whose own "> " line prefix is gone, so
	//     the block splits into a loose paragraph plus a blockquote (#1633);
	//   - a continuation line that forms a GFM table delimiter row or a setext
	//     underline, which promotes the line above it to a table header / the
	//     paragraph to a heading ("|0\n-|" -> a one-cell table) — escape the
	//     bar so it stays literal text (#1651).
	//
	// Blockquote re-marking and leading-marker escaping are mutually exclusive —
	// a blockquote already opens with ">", the one marker we must NOT strip. The
	// interior-bar escape applies only to a plain paragraph: a blockquote's
	// continuation lines carry "> ", which is not a bar.
	if role == "" {
		if bqPrefix, body, isQuote := blockquoteRebuild(block, text); isQuote {
			prefix, text = bqPrefix, body
		} else {
			text = escapeLeadingBlockMarker(text)
			text = escapeInteriorBlockBars(text)
		}
	}

	if _, err := fmt.Fprint(out, prefix, text, suffix); err != nil {
		return err
	}
	return nil
}

// mdCell is one table cell: its rendered text and the column it occupies
// (-1 when the source gave no explicit column index, so it is placed
// sequentially).
type mdCell struct {
	col  int
	text string
}

// mdRow is one accumulated table row.
type mdRow struct {
	header bool
	cells  []mdCell
}

// collectTable walks from the `table` GroupStart at index start to its matching
// GroupEnd, gathering each `table-row` group's cell blocks. It returns the
// index of the matching GroupEnd (so the caller can resume after it) and the
// accumulated rows.
func (w *Writer) collectTable(events []*model.Part, start int) (end int, rows []mdRow) {
	// Group the table's parts into rows of cell blocks with the shared
	// assembler (projection.AssembleTable), then render each cell to GFM here —
	// the writer keeps its locale choice (blockRuns), sparse-column property,
	// and cell escaping; only the row/cell grouping is shared.
	end, asm := projection.AssembleTable(events, start)
	rows = make([]mdRow, 0, len(asm.Rows))
	for _, r := range asm.Rows {
		mr := mdRow{header: r.Header}
		for _, c := range r.Cells {
			col := -1
			if v, ok := c.Block.Properties["column"]; ok {
				n := 0
				if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
					col = n
				}
			}
			text := escapeTableCell(renderInlineMarkdown(w.blockRuns(c.Block)))
			mr.cells = append(mr.cells, mdCell{col: col, text: text})
		}
		rows = append(rows, mr)
	}
	return end, rows
}

// writeTable renders accumulated rows as a GFM table. The header row is the
// first row flagged as a header (or carrying header cells); when none is
// present a blank header is synthesised so the output is valid GFM.
func (w *Writer) writeTable(rows []mdRow, out io.Writer) error {
	if len(rows) == 0 {
		return nil
	}

	numCols := 0
	for _, r := range rows {
		if n := rowWidth(r.cells); n > numCols {
			numCols = n
		}
	}
	if numCols == 0 {
		return nil
	}

	// Pick the header row; everything else is body, in order.
	headerIdx := -1
	for i, r := range rows {
		if r.header {
			headerIdx = i
			break
		}
	}
	var header []string
	var body []mdRow
	if headerIdx >= 0 {
		header = placeCells(rows[headerIdx].cells, numCols)
		for i, r := range rows {
			if i != headerIdx {
				body = append(body, r)
			}
		}
	} else {
		header = make([]string, numCols) // blank header keeps the GFM valid
		body = rows
	}

	if !w.firstBlock {
		if _, err := fmt.Fprint(out, "\n\n"); err != nil {
			return err
		}
	}
	w.firstBlock = false

	var sb strings.Builder
	sb.WriteString(tableRowLine(header))
	sb.WriteByte('\n')
	sep := make([]string, numCols)
	for i := range sep {
		sep[i] = "---"
	}
	sb.WriteString(tableRowLine(sep))
	for _, r := range body {
		sb.WriteByte('\n')
		sb.WriteString(tableRowLine(placeCells(r.cells, numCols)))
	}

	_, err := io.WriteString(out, sb.String())
	return err
}

// rowWidth returns the column count a row occupies: the max explicit column
// index + 1, or the cell count when columns are unlabelled.
func rowWidth(cells []mdCell) int {
	width := 0
	seq := 0
	for _, c := range cells {
		idx := c.col
		if idx < 0 {
			idx = seq
		}
		if idx+1 > width {
			width = idx + 1
		}
		seq = idx + 1
	}
	return width
}

// placeCells lays cells into a fixed-width row by column index, filling unset
// columns with empty strings. Unlabelled cells (col < 0) flow into the next
// free slot.
func placeCells(cells []mdCell, numCols int) []string {
	out := make([]string, numCols)
	seq := 0
	for _, c := range cells {
		idx := c.col
		if idx < 0 {
			idx = seq
		}
		if idx >= 0 && idx < numCols {
			out[idx] = c.text
		}
		seq = idx + 1
	}
	return out
}

// tableRowLine formats one GFM table line: "| a | b | c |".
func tableRowLine(cells []string) string {
	var sb strings.Builder
	sb.WriteByte('|')
	for _, c := range cells {
		sb.WriteByte(' ')
		sb.WriteString(c)
		sb.WriteString(" |")
	}
	return sb.String()
}

// escapeTableCell makes cell text safe inside a GFM table cell: pipes are
// escaped and newlines collapse to <br> so a multi-line value stays on one row.
func escapeTableCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return s
}

// escapeLeadingBlockMarker backslash-escapes the marker that begins text when
// text, emitted verbatim at the start of a line, would OPEN a block-level
// construct — so a paragraph the rebuild path produced does not re-read as a
// heading, list, blockquote, thematic break or fenced code (#1632).
//
// The residue matters because the rebuild path drops inline constructs it has
// no Markdown spelling for (inline HTML most of all). Dropping the HTML in
// "#<A>" leaves the bare text "#", which re-parses as an empty level-1 heading:
// trip(trip(x)) != trip(x). CommonMark reads a backslash-escaped ASCII
// punctuation marker as literal text and the reader preserves the backslash in
// the block text, so the escaped output round-trips and is itself idempotent —
// an already-escaped "\#..." matches none of the cases below and is left alone.
//
// Indented code (a four-space line prefix) is deliberately not covered: the
// rebuild path trims a block's boundary whitespace before this point (#1603),
// so a paragraph can never reach the writer with leading spaces.
func escapeLeadingBlockMarker(text string) string {
	if i, ok := leadingBlockMarkerPos(text); ok {
		return text[:i] + "\\" + text[i:]
	}
	return text
}

// leadingBlockMarkerPos reports the byte index of the marker character to
// backslash-escape so text no longer opens a block construct, and whether any
// such marker was found. For every construct but ordered lists the marker is
// the first byte; for an ordered list it is the "." or ")" after the digits (a
// backslash before a digit is not an escape, so the punctuation is escaped
// instead — "1. x" -> "1\. x").
func leadingBlockMarkerPos(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	switch c := text[0]; c {
	case '#':
		// ATX heading: 1-6 '#' then a space/tab or end of line.
		n := 0
		for n < len(text) && text[n] == '#' {
			n++
		}
		if n <= 6 && atLineBoundary(text, n) {
			return 0, true
		}
	case '>':
		// Blockquote marker.
		return 0, true
	case '-', '+', '*', '_':
		// A run of three or more matching -, _ or * (spaces allowed) is a
		// thematic break; '-', '+' or '*' then a space/tab or end of line is a
		// bullet list ('_' is never a bullet).
		if isThematicBreak(text) {
			return 0, true
		}
		if c != '_' && atLineBoundary(text, 1) {
			return 0, true
		}
	case '`', '~':
		// Fenced code: three or more backticks or tildes.
		n := 0
		for n < len(text) && text[n] == c {
			n++
		}
		if n >= 3 {
			return 0, true
		}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		// Ordered list: 1-9 digits, then '.' or ')', then a space/tab or end.
		n := 0
		for n < len(text) && text[n] >= '0' && text[n] <= '9' {
			n++
		}
		if n <= 9 && n < len(text) && (text[n] == '.' || text[n] == ')') && atLineBoundary(text, n+1) {
			return n, true
		}
	}
	return 0, false
}

// atLineBoundary reports whether position i in text is the end of the first
// line — a space, tab, newline, or the end of the string.
func atLineBoundary(text string, i int) bool {
	if i >= len(text) {
		return true
	}
	switch text[i] {
	case ' ', '\t', '\n':
		return true
	}
	return false
}

// isThematicBreak reports whether text's first line is a CommonMark thematic
// break: three or more matching '-', '_' or '*', separated only by spaces/tabs.
func isThematicBreak(text string) bool {
	line, _, _ := strings.Cut(text, "\n")
	c := line[0]
	if c != '-' && c != '_' && c != '*' {
		return false
	}
	count := 0
	for i := range len(line) {
		switch line[i] {
		case c:
			count++
		case ' ', '\t':
			// allowed between markers
		default:
			return false
		}
	}
	return count >= 3
}

// escapeInteriorBlockBars backslash-escapes any line AFTER the first that, in a
// paragraph the rebuild path emits verbatim, would be read as a GFM table
// delimiter row or a setext underline — promoting the line above it into a
// table header, or the whole paragraph into a heading (#1651). #1632 escapes
// only the LEADING marker of the first line; a table/setext bar is a signal on
// a *later* line (the first line is its header), so it needs this separate pass.
//
// Only the bar line matters: escaping its first hyphen/colon/equals makes the
// line start with a backslash, which is neither a valid delimiter-cell nor a
// setext-bar character, so goldmark parses it as ordinary paragraph text. The
// reader preserves the backslash in the block text, so an already-escaped line
// matches neither grammar and is left alone — the output is idempotent.
func escapeInteriorBlockBars(text string) string {
	if !strings.Contains(text, "\n") {
		return text
	}
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		if isSetextBar(lines[i]) || isTableDelimiterRow(lines[i]) {
			if j := firstBarChar(lines[i]); j >= 0 {
				lines[i] = lines[i][:j] + "\\" + lines[i][j:]
			}
		}
	}
	return strings.Join(lines, "\n")
}

// firstBarChar returns the byte index of the first '-', '=' or ':' in line, or
// -1. That character is the one backslash-escaping neutralises: a delimiter cell
// needs a hyphen and a setext bar is all '=' or all '-', so escaping the first
// such character breaks either grammar.
func firstBarChar(line string) int {
	for i := range len(line) {
		switch line[i] {
		case '-', '=', ':':
			return i
		}
	}
	return -1
}

// isSetextBar reports whether line is a CommonMark setext underline — up to 3
// leading spaces, then a run of only '=' or only '-', then optional trailing
// spaces. Mirrors goldmark's matchesSetextHeadingBar.
func isSetextBar(line string) bool {
	s := strings.TrimRight(strings.TrimLeft(line, " "), " \t")
	if len(line)-len(strings.TrimLeft(line, " ")) > 3 || s == "" {
		return false
	}
	return s == strings.Repeat("=", len(s)) || s == strings.Repeat("-", len(s))
}

// isTableDelimiterRow reports whether line is a GFM table delimiter row — up to
// 3 leading spaces, then cells of only '-'/':'/spaces separated by '|', with at
// least one '-'. Mirrors goldmark's isTableDelim (extension/table.go): every
// byte is a space, '-', '|' or ':', and the line is not made of '-' alone. A
// real delimiter also needs a hyphen (a cell is ":?-+:?"), so require one to
// avoid escaping pipe-only lines that never form a table.
func isTableDelimiterRow(line string) bool {
	if len(line)-len(strings.TrimLeft(line, " ")) > 3 {
		return false
	}
	hasDash := false
	allDash := true
	for i := range len(line) {
		switch line[i] {
		case '-':
			hasDash = true
		case ' ', '\t', '|', ':':
			allDash = false
		default:
			return false
		}
	}
	return hasDash && !allDash
}

// blockquoteRebuild reconstructs a multi-line blockquote body in the rebuild
// path. A blockquote surfaces as an empty-role paragraph whose per-line ">"
// marker lives in one of two places: held in BlockPropLinePrefix with the
// marker stripped from the text (a hard-break body: "a\nb" + prefix "> "), or
// baked into the text on every continuation line (a soft-break body:
// "a\n> b"). Either way the FIRST line's marker is missing in the rebuild path
// — there is no skeleton gap to restore it as RenderBlockContent relies on — so
// the block re-reads as a loose paragraph followed by a blockquote (#1633).
//
// It returns the first-line prefix and the body with continuation markers
// present, reconstructing "> a\n> b". ok is false for any block that is not a
// blockquote (list-item and indented continuations also carry
// BlockPropLinePrefix but must be re-established by their own role prefix, not
// here), leaving those untouched.
func blockquoteRebuild(block *model.Block, text string) (prefix, body string, ok bool) {
	if lp, has := block.Properties[BlockPropLinePrefix]; has && strings.HasPrefix(lp, ">") {
		// Hard-break body: the marker was stripped from the text. Reinsert it
		// after every "\n" exactly as RenderBlockContent (the byte-exact
		// skeleton reference) does, then mark the first line.
		if strings.Contains(text, "\n") {
			text = strings.ReplaceAll(text, "\n", "\n"+lp)
		}
		return lp, text, true
	}
	// Soft-break body: the continuation lines already carry their ">" marker;
	// only the first line lacks one. Recover the marker from the first
	// continuation line so the whole quote is marked identically.
	if m := firstContinuationBlockquoteMarker(text); m != "" {
		return m, text, true
	}
	return "", text, false
}

// firstContinuationBlockquoteMarker returns the blockquote marker (">" plus an
// optional single space) that begins the first continuation line of text, or ""
// when text is single-line or its continuation is not a blockquote line.
func firstContinuationBlockquoteMarker(text string) string {
	nl := strings.IndexByte(text, '\n')
	if nl < 0 || nl+1 >= len(text) || text[nl+1] != '>' {
		return ""
	}
	if nl+2 < len(text) && text[nl+2] == ' ' {
		return "> "
	}
	return ">"
}

// headingLevel returns a block's heading level, preferring the normalized
// structural annotation (WS1) and falling back to the legacy "level" property;
// 0 when neither is present.

// trailSpaceTrimmer is an io.Writer that mirrors upstream Okapi's
// MarkdownFilterWriter trimming algorithm: it buffers bytes per
// physical line and, at every '\n', applies the rule:
//
//   - if the buffered line is made up entirely of spaces, drop them all;
//   - else if it ends with EXACTLY one trailing space, drop that one;
//   - else keep the line intact (preserves ≥2 trailing spaces, the
//     CommonMark hard-break signal, plus the trailing 4-space pattern
//     in fixtures like DirectShape.md's <pre> code).
//
// Carriage returns are preserved verbatim. Flush MUST be called on the
// final write so any unterminated trailing line is also flushed.
type trailSpaceTrimmer struct {
	w   io.Writer
	buf []byte // current physical line being buffered (no trailing \n)
}

func newTrailSpaceTrimmer(w io.Writer) *trailSpaceTrimmer {
	return &trailSpaceTrimmer{w: w}
}

func (t *trailSpaceTrimmer) Write(p []byte) (int, error) {
	for i, c := range p {
		if c == '\n' {
			t.trimBuffered()
			t.buf = append(t.buf, '\n')
			if _, err := t.w.Write(t.buf); err != nil {
				return i, err
			}
			t.buf = t.buf[:0]
			continue
		}
		t.buf = append(t.buf, c)
	}
	return len(p), nil
}

// Flush writes any unterminated trailing line (after the final '\n')
// without applying the trim — okapi's writer leaves the final tail
// alone unless a newline arrives, and we keep that semantics so a
// fixture whose final line legitimately ends in a single space (rare
// in markdown, but possible) round-trips intact.
func (t *trailSpaceTrimmer) Flush() error {
	if len(t.buf) == 0 {
		return nil
	}
	_, err := t.w.Write(t.buf)
	t.buf = t.buf[:0]
	return err
}

func (t *trailSpaceTrimmer) trimBuffered() {
	if len(t.buf) < 2 {
		return
	}
	// Only strip if the line ends in EXACTLY one trailing space — see
	// the comment on the wrap site for why we don't mirror the upstream
	// "all-spaces → empty" branch (the upstream skeleton writer
	// re-prepends the line prefix immediately, so the net effect is
	// "indent\n" rows survive).
	n := len(t.buf)
	if t.buf[n-1] == ' ' && t.buf[n-2] != ' ' {
		t.buf = t.buf[:n-1]
	}
}

// blockText returns the rendered text for a block, preferring the target
// locale's translation if available, falling back to source. Multi-line
// paragraphs whose source carried a per-line continuation prefix (e.g.
// `> ` for blockquote bodies — see BlockPropLinePrefix in reader.go)
// have that prefix re-inserted after every "\n" so blockquotes and
// indented continuations retain their original line shape on round-trip.
// Mirrors okapi MarkdownFilter, whose TextUnit content carries only the
// LFs between lines while its skeleton-driven writer re-emits the
// per-line prefix.
func (w *Writer) blockText(block *model.Block) string {
	runs := w.blockRuns(block)
	if runs == nil {
		return ""
	}
	return RenderBlockContent(block, runs)
}

// RenderBlockContent renders a block's content (the given run sequence —
// source or target) the way the skeleton splice emits it: inline codes
// re-emit their original data, front matter values restore/add YAML
// quoting, and the markdown line-prefix property re-applies to multi-line
// continuations. The MDX reader's byte-faithfulness check uses the same
// function so reader and writer can never disagree about untranslated
// output.
func RenderBlockContent(block *model.Block, runs []model.Run) string {
	rendered := model.RenderRunsWithData(runs)
	if block.Type == "front-matter" {
		// The skeleton carries `key: ` and the newline only; the value —
		// including any quoting — is the block's responsibility. An
		// unchanged, originally-unquoted value renders raw so the
		// untranslated round-trip stays byte-exact whatever the source
		// spelling; quoting is restored (or added when needed) otherwise.
		quote := block.Properties[BlockPropFrontMatterQuote]
		if quote == "" && rendered == model.RenderRunsWithData(block.Source) {
			return rendered
		}
		return frontMatterScalar(rendered, quote)
	}
	if prefix, ok := block.Properties[BlockPropLinePrefix]; ok && prefix != "" && strings.Contains(rendered, "\n") {
		rendered = strings.ReplaceAll(rendered, "\n", "\n"+prefix)
	}
	if block.SemanticRole() == model.RoleTableCell {
		// GFM row splitting sees an unescaped `|` as a cell boundary wherever
		// it appears — even inside a code span — so the reader receives cell
		// text with `\|` already unescaped and every pipe must be re-escaped
		// on the way out. Without this, a cell like `--output-format
		// <json\|text>` round-trips with a bare pipe that splits the cell and
		// leaves an unterminated code span (invalid MDX downstream).
		rendered = strings.ReplaceAll(rendered, "|", "\\|")
	}
	return rendered
}

// frontMatterScalar renders a front matter value, restoring the source's
// quote style and adding quoting when an unquoted value's translation
// would not survive as a YAML plain scalar. Sources that were valid plain
// scalars render byte-identically (the quoting triggers cannot occur in a
// valid plain scalar), so roundtrip output is unchanged for untranslated
// content.
func frontMatterScalar(text, origQuote string) string {
	switch origQuote {
	case "\"":
		return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(text) + "\""
	case "'":
		return "'" + strings.ReplaceAll(text, "'", "''") + "'"
	}
	if frontMatterNeedsQuoting(text) {
		return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(text) + "\""
	}
	return text
}

// frontMatterNeedsQuoting reports whether text cannot stand as a YAML
// plain scalar on a single line.
func frontMatterNeedsQuoting(text string) bool {
	if text == "" {
		return false
	}
	if strings.TrimSpace(text) != text {
		return true
	}
	if strings.Contains(text, ": ") || strings.HasSuffix(text, ":") ||
		strings.Contains(text, " #") || strings.Contains(text, "\n") {
		return true
	}
	switch text[0] {
	case '"', '\'', '[', ']', '{', '}', '>', '|', '&', '*', '!', '%', '@', '`', ',', '#':
		return true
	}
	return strings.HasPrefix(text, "- ")
}

// blockRuns returns the target Run sequence for the configured locale,
// or the source Run sequence if no target is available.
func (w *Writer) blockRuns(block *model.Block) []model.Run {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		runs := block.TargetRuns(w.Locale)
		if len(runs) > 0 {
			return runs
		}
	}
	if len(block.Source) > 0 {
		return block.Source
	}
	return nil
}
