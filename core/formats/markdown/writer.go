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
func renderInlineMarkdown(runs []model.Run) string {
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
	text := renderInlineMarkdown(w.blockRuns(block))

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
	var prefix, suffix string
	switch role {
	case model.RoleTitle:
		prefix = "# "
	case model.RoleHeading:
		if n := headingLevel(block); n > 0 {
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

// headingLevel returns a block's heading level, preferring the normalized
// structural annotation (WS1) and falling back to the legacy "level" property;
// 0 when neither is present.
func headingLevel(block *model.Block) int {
	if s, ok := block.Structure(); ok && s != nil && s.Level > 0 {
		return s.Level
	}
	if level, ok := block.Properties["level"]; ok {
		n := 0
		_, _ = fmt.Sscanf(level, "%d", &n)
		return n
	}
	return 0
}

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
