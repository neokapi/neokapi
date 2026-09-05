package markdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
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
	// ctx is the stack of open structural containers on the generative path —
	// lists and block quotes. A list item's marker (`-` vs `1.`) and a quoted
	// paragraph's `> ` prefix are properties of the container, not of the block,
	// so they can only be rendered with the bracket in scope.
	ctx []blockContext
	// prevListItem records that the block just written was an item of the
	// innermost open list, so the next one is separated by a single newline —
	// a tight list — rather than the blank line every other block pair takes.
	prevListItem bool
	// prevQuoteID is the innermost block quote the previous block belonged to.
	// Two blocks in the SAME quote are separated by a quoted blank line, or
	// CommonMark reads them as two adjacent quotations rather than one with two
	// paragraphs.
	prevQuoteID string
}

// blockContext is one open structural container on the generative path.
type blockContext struct {
	groupID string
	kind    string // "ordered-list", "list", or "blockquote"
	counter int    // next ordinal, for an ordered list
}

// listStartOf returns the first ordinal of an ordered list, honouring an
// explicit start (HTML's <ol start>) and defaulting to 1.
func listStartOf(g *model.GroupStart) int {
	if g.Type != "ordered-list" {
		return 0
	}
	if v, ok := g.Properties["start"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

// closeContext pops the container with the given group ID, along with anything
// still open inside it. A malformed stream that ends a group it never started
// leaves the stack untouched rather than unwinding it.
func (w *Writer) closeContext(groupID string) {
	for i, v := range slices.Backward(w.ctx) {
		if v.groupID == groupID {
			w.ctx = w.ctx[:i]
			w.prevListItem = false
			return
		}
	}
}

// innermostQuoteID returns the group ID of the innermost open block quote, or
// "" when the writer is not inside one.
func (w *Writer) innermostQuoteID() string {
	for _, v := range slices.Backward(w.ctx) {
		if v.kind == "blockquote" {
			return v.groupID
		}
	}
	return ""
}

// innermostList returns the innermost open list container, or nil when the
// writer is not inside one.
func (w *Writer) innermostList() *blockContext {
	for i := len(w.ctx) - 1; i >= 0; i-- {
		if k := w.ctx[i].kind; k == "ordered-list" || k == "list" {
			return &w.ctx[i]
		}
	}
	return nil
}

// listDepth counts open list containers, which is how far a nested item indents.
func (w *Writer) listDepth() int {
	n := 0
	for _, c := range w.ctx {
		if c.kind == "ordered-list" || c.kind == "list" {
			n++
		}
	}
	return n
}

// quoteDepth counts open blockquote containers.
func (w *Writer) quoteDepth() int {
	n := 0
	for _, c := range w.ctx {
		if c.kind == "blockquote" {
			n++
		}
	}
	return n
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new Markdown writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		FormatName: "markdown",
		cfg:        cfg,
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed Markdown.
//
// The skeleton path collects, because it renders in skeleton order rather than
// stream order. The generative path does not have to: a table whose width its
// reader declared up front is rendered a row at a time as the rows arrive, so a
// spreadsheet with a million cells costs one row of memory instead of a million
// blocks. Everything else still accumulates — those documents are small, and
// the buffered path is the one the fixtures pin.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var blocksByID map[string]*model.Block
	if w.skeletonStore != nil {
		blocksByID = make(map[string]*model.Block)
	}
	var events []*model.Part // blocks + group brackets, in stream order

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

	var st *streamTable

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			// A table already being streamed consumes everything up to its own
			// GroupEnd.
			if st != nil {
				finished, err := st.consume(part)
				if err != nil {
					return err
				}
				if finished {
					st = nil
				}
				continue
			}
			if blocksByID == nil && isStreamableTable(part) {
				// Render what came before, then stream the table rather than
				// collecting it. Nothing after it needs these events.
				if err := w.writeFromEvents(events, tw); err != nil {
					return err
				}
				events = nil
				st = w.newStreamTable(part, tw)
				continue
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					if blocksByID != nil {
						blocksByID[block.ID] = block
					} else if rendersNothing(block) {
						// Nothing downstream renders it, so there is no reason
						// to hold it until the render skips it. A spreadsheet's
						// shared-string table is 157k of these.
						continue
					}
					events = append(events, part)
				}
			case model.PartGroupStart, model.PartGroupEnd:
				events = append(events, part)
			}
		}
	}
done:
	if st != nil {
		if err := st.finish(); err != nil {
			return err
		}
	}
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
	// A text file ends with a newline. Without one the last block runs into
	// whatever is appended next, and every diff of the output shows a spurious
	// "\ No newline at end of file". Only on the generative path — the skeleton
	// path reproduces the source's own ending, whatever it is.
	if !w.firstBlock {
		if _, err := fmt.Fprint(tw, "\n"); err != nil {
			return err
		}
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
	return strings.TrimSpace(renderInline(runs, true))
}

// renderInlineLiteral renders a run stream as literal text with no Markdown
// markup at all. Inside a fenced code block the content IS the text: emitting
// the inline vocabulary there wraps it in a second layer of markup, so an
// inline-code run inside a fence came out as ```` ```\n`x=1`\n``` ````, and the
// backticks re-read as content.
func renderInlineLiteral(runs []model.Run) string {
	sink := &mdInlineSink{literal: true}
	projection.WalkInline(runs, sink)
	sink.flush()
	return sink.sb.String()
}

func renderInline(runs []model.Run, escapeAngle bool) string {
	sink := &mdInlineSink{escapeAngle: escapeAngle}
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
	sb          strings.Builder
	open        []string // stack of closing delimiters (or "" for dropped tags)
	escapeAngle bool     // backslash-escape literal '<' (paragraph text, not code)
	literal     bool     // emit run text only, no markup (fenced code content)
}

func (s *mdInlineSink) Text(t string) {
	if s.escapeAngle {
		writeEscapingAngle(&s.sb, t)
		return
	}
	s.sb.WriteString(t)
}

// writeEscapingAngle writes t, backslash-escaping any '<' that is not already
// escaped. The rebuild path drops inline-HTML constructs, and the residue can
// abut surrounding text into a NEW tag: "<<A>A>" drops the <A> and the leftover
// "<" and "A>" reform "<A>", which re-reads as inline HTML and is dropped
// entirely, losing the whole block (#1652). Keeping literal '<' escaped stops
// text from re-parsing as a tag or autolink; the reader preserves the backslash,
// so already-escaped input is left alone and the output is idempotent. Only
// Text() (literal run content) is escaped — the HTML the sink emits for
// formatting fallbacks (<mark>, <sup>, …) comes through Open/Close, untouched.
func writeEscapingAngle(sb *strings.Builder, t string) {
	escaped := false
	for i := range len(t) {
		c := t[i]
		if c == '<' && !escaped {
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
		escaped = c == '\\' && !escaped
	}
}

func (s *mdInlineSink) Open(r *model.PcOpenRun) {
	if s.literal {
		s.open = append(s.open, "")
		return
	}
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
	if s.literal {
		s.sb.WriteString(r.Equiv)
		return
	}
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
	for _, v := range slices.Backward(s.open) {
		s.sb.WriteString(v)
	}
}

// writeFromEvents reconstructs markdown from the ordered block + group event
// stream (cross-format semantic export). A `table` group and its `table-row`
// children render as a GFM table; every other block renders individually.
// Non-table group brackets are transparent — their child blocks render in
// place exactly as the block-only path did.
func (w *Writer) writeFromEvents(events []*model.Part, out io.Writer) error {
	var pendingCells []*model.Block
	flushCells := func() error {
		if len(pendingCells) == 0 {
			return nil
		}
		caption, rows := w.assembleFlatCells(pendingCells)
		pendingCells = nil
		if caption != "" {
			if !w.firstBlock {
				if _, err := fmt.Fprint(out, "\n\n"); err != nil {
					return err
				}
			}
			w.firstBlock = false
			if _, err := fmt.Fprint(out, "**"+caption+"**"); err != nil {
				return err
			}
		}
		return w.writeTable(rows, out)
	}

	for i := 0; i < len(events); i++ {
		part := events[i]
		switch part.Type {
		case model.PartGroupStart:
			g, ok := part.Resource.(*model.GroupStart)
			if !ok {
				continue
			}
			if err := flushCells(); err != nil {
				return err
			}
			switch g.Type {
			case "table":
				end, rows := w.collectTable(events, i)
				if err := w.writeTable(rows, out); err != nil {
					return err
				}
				i = end
			case "ordered-list", "list", "blockquote":
				w.ctx = append(w.ctx, blockContext{
					groupID: g.ID,
					kind:    g.Type,
					counter: listStartOf(g),
				})
			}
		case model.PartGroupEnd:
			if err := flushCells(); err != nil {
				return err
			}
			if g, ok := part.Resource.(*model.GroupEnd); ok {
				w.closeContext(g.ID)
			}
		case model.PartBlock:
			block, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}
			// A bare table cell outside any table group means the reader knows
			// each cell's address but has no row container to bracket — a
			// spreadsheet worksheet. Buffer the run and assemble it as one
			// table when it ends, the same fallback core/projection applies.
			if isCellBlock(block) {
				// Cells from a different source part belong to a different
				// grid — a workbook's next worksheet. Flush first, or two
				// sheets merge into one table whose rows come from different
				// grids and whose columns mean different things.
				if n := len(pendingCells); n > 0 &&
					pendingCells[n-1].Properties["partPath"] != block.Properties["partPath"] {
					if err := flushCells(); err != nil {
						return err
					}
				}
				pendingCells = append(pendingCells, block)
				continue
			}
			if err := flushCells(); err != nil {
				return err
			}
			if err := w.writeBlockMarkdown(block, out); err != nil {
				return err
			}
		}
	}
	return flushCells()
}

// isDrawingMetadata reports whether a block carries a drawing's non-visual
// property (its name, accessibility description, or object title) rather than
// document content. Keyed on the reader's own "element" discriminator, not on
// the block Type, because "property" also covers genuinely visible text — a VML
// textpath string, an mc:AlternateContent fallback — and document metadata.
func isDrawingMetadata(b *model.Block) bool {
	if b.Type != "property" {
		return false
	}
	switch b.Properties["element"] {
	case "drawing-name", "drawing-descr", "drawing-title":
		return true
	}
	return false
}

// isDocMetadata reports whether a block is a document core property — a
// dc:title, dc:creator, cp:keywords extracted from an OPC package's
// docProps/core.xml. Keyed on the part path because "property" as a Type also
// covers visible text (a VML textpath string), and the element names alone
// (title, subject, …) are too generic to claim across formats.
func isDocMetadata(b *model.Block) bool {
	return b.Type == "property" && b.Properties["partPath"] == "docProps/core.xml"
}

// isCellBlock reports whether a block carries a table-cell role.
func isCellBlock(b *model.Block) bool {
	role := b.SemanticRole()
	return role == model.RoleTableCell || role == model.RoleTableHeader
}

// assembleFlatCells groups a run of bare cell blocks into rows on the per-cell
// row hint (projection.PropFlatRow). Cells with no hint collapse into one row —
// best-effort, matching projection.flushFlatCells, since row topology is not
// recoverable from an unhinted block stream. A leading lone-cell row above a
// wider grid is returned as a caption rather than a row.
func (w *Writer) assembleFlatCells(cells []*model.Block) (caption string, rows []mdRow) {
	lastKey, started := "", false
	for _, c := range cells {
		key, hasHint := c.Properties[projection.PropFlatRow]
		if !started || (hasHint && key != lastKey) {
			rows = append(rows, mdRow{})
			lastKey, started = key, true
		}
		col := -1
		if v, ok := c.Properties["column"]; ok {
			n := 0
			if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
				col = n
			}
		}
		text := escapeTableCell(renderInlineMarkdown(w.blockRuns(c)))
		rows[len(rows)-1].cells = append(rows[len(rows)-1].cells, mdCell{col: col, text: text})
	}
	// A lone cell in the first row of a wider grid is the table's title — a
	// worksheet's "Q3 pricing" line above the real columns — not a row of the
	// table. Left in place it would be promoted below and become a header row
	// padded with empty cells, demoting the real header to body. Rendered from
	// the source block, not the assembled cell, so table escaping does not
	// apply to what is now a paragraph.
	if len(rows) > 1 && len(rows[0].cells) == 1 && rows[0].cells[0].text != "" {
		for _, r := range rows[1:] {
			if len(r.cells) > 1 {
				caption = renderInlineMarkdown(w.blockRuns(cells[0]))
				rows = rows[1:]
				break
			}
		}
	}
	// A spreadsheet's first row is its header row far more often than not, and
	// GFM needs a header row regardless — promoting it beats emitting an empty
	// one above real data.
	if len(rows) > 1 {
		rows[0].header = true
	}
	return caption, rows
}

// writeBlockMarkdown renders one block, role-prefixed, separated from prior
// output by a blank line. Inline formatting renders from each run's vocabulary
// type, not its source-format Data, so the same Markdown results whatever the
// source format.
func (w *Writer) writeBlockMarkdown(block *model.Block, out io.Writer) error {

	if rendersNothing(block) {
		return nil
	}
	// A drawing's name, alt text and object title are graphic metadata, not
	// document flow. They are surfaced as their own blocks so an ingestion
	// consumer and the editor can see them, but rendering each as a paragraph
	// turns one image into four stray lines — a name, a filename, and the alt
	// text twice, once per non-visual-properties element. The alt text reaches
	// the output through the image run's own attributes instead.
	if isDrawingMetadata(block) {
		return nil
	}
	// A document's core properties — title, author, keywords, category — are
	// metadata about the file, not positions in its flow. They are real
	// translation units (a document title is translated), but rendering them as
	// paragraphs appends the author's name and keyword list after the last real
	// paragraph as if the document said them. Generative-path only.
	if isDocMetadata(block) {
		return nil
	}

	role0 := block.SemanticRole()
	if role0 == "" {
		role0 = block.Type
	}
	isItem := role0 == model.RoleListItem
	quoteID := w.innermostQuoteID()

	if !w.firstBlock {
		// Consecutive items of one list are a tight list: one newline, not the
		// blank line every other block pair takes. A blank line between items
		// makes CommonMark render each `<li>` as its own paragraph, which is a
		// different document.
		sep := "\n\n"
		switch {
		case isItem && w.prevListItem:
			sep = "\n"
		case quoteID != "" && quoteID == w.prevQuoteID:
			// Blank line INSIDE the quote, not around it.
			sep = "\n" + strings.TrimRight(strings.Repeat("> ", w.quoteDepth()), " ") + "\n"
		}
		if _, err := fmt.Fprint(out, sep); err != nil {
			return err
		}
	}
	w.firstBlock = false
	w.prevListItem = isItem
	w.prevQuoteID = quoteID

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
		text = renderInlineLiteral(w.blockRuns(block))
	}

	var prefix, suffix string
	switch role {
	case model.RoleTitle:
		prefix = "# "
		text = singleLineHeading(text)
	case model.RoleHeading:
		if n := block.HeadingLevel(); n > 0 {
			prefix = strings.Repeat("#", n) + " "
			text = singleLineHeading(text)
		}
	case model.RoleListItem:
		// The marker belongs to the enclosing list, not the item: an ordered
		// list numbers its items, an unordered one bullets them, and a nested
		// list indents. With no list bracket in scope (a reader that emits bare
		// items, or a single item quoted out of context) a bullet is the safe
		// reading — it is what the writer has always emitted.
		prefix = "- "
		if l := w.innermostList(); l != nil {
			if l.kind == "ordered-list" {
				prefix = strconv.Itoa(l.counter) + ". "
				l.counter++
			}
			if d := w.listDepth(); d > 1 {
				prefix = strings.Repeat("  ", d-1) + prefix
			}
		}
	case model.RoleCode:
		// Re-emit the fenced code block's info string (language) so the
		// do-not-translate signal survives cross-format export.
		prefix, suffix = "```"+block.CodeLanguage()+"\n", "\n```"
	case model.RoleCaption:
		prefix, suffix = "*", "*"
	}

	// A block the switch left with no prefix or suffix lands as bare text at the
	// very start of a line — a paragraph (empty role) but also any block whose
	// non-empty Type matched no case, most importantly "html-text" (a run of
	// inline HTML plus text). Block-shaped residues have to be repaired there, or
	// the text re-reads as a different block kind — or as nothing, losing the
	// block — and trip(trip(x)) != trip(x):
	//
	//   - a bare leading marker left behind when a dropped inline construct
	//     exposes it ("#<A>" -> "#", which re-parses as an empty heading;
	//     "<A>\n*" -> "*", an empty bullet that re-reads as no block at all) —
	//     backslash-escape it so CommonMark keeps it literal (#1632). Gating this
	//     on the empty role alone missed html-text blocks (#1657);
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
	if prefix == "" && suffix == "" {
		if bqPrefix, body, isQuote := blockquoteRebuild(block, text); isQuote {
			prefix, text = bqPrefix, body
		} else {
			text = escapeLeadingBlockMarker(text)
			text = escapeInteriorBlockBars(text)
		}
	}

	// A block inside a <blockquote> bracket is quoted regardless of its own
	// role: the marker goes on every line, including continuation lines, or
	// the quotation ends after the first one.
	if q := w.quoteDepth(); q > 0 {
		marker := strings.Repeat("> ", q)
		body := prefix + text + suffix
		lines := strings.Split(body, "\n")
		for i, ln := range lines {
			lines[i] = marker + ln
		}
		prefix, text, suffix = "", strings.Join(lines, "\n"), ""
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
	case '`':
		// Backtick fenced-code opener: three or more backticks whose info string
		// — the rest of the first line — contains NO backtick. A backtick later
		// on the line disqualifies the fence (CommonMark forbids a backtick in a
		// backtick-fence info string), so the run is ordinary text and must be
		// left alone: escaping only its first backtick would turn the remaining
		// two into a code-span opener and change the content (#1657).
		n := 0
		for n < len(text) && text[n] == '`' {
			n++
		}
		if n >= 3 {
			rest := text[n:]
			if i := strings.IndexByte(rest, '\n'); i >= 0 {
				rest = rest[:i]
			}
			if !strings.Contains(rest, "`") {
				return 0, true
			}
		}
	case '~':
		// Tilde fenced-code opener: three or more tildes. A tilde is not a
		// code-span delimiter, so escaping the first is safe and needs no
		// info-string guard.
		n := 0
		for n < len(text) && text[n] == '~' {
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

// headingLineBreakRE matches one line break inside heading text, with the
// whitespace around it and the blockquote marker a soft break bakes into the
// continuation line.
var headingLineBreakRE = regexp.MustCompile(`[ \t]*\r?\n[ \t>]*`)

// singleLineHeading folds heading text onto one line. The rebuild path writes
// every heading as ATX, and an ATX heading ends at its newline, so a heading
// whose text spans lines (a multi-line setext heading, or a heading from a
// format that allows a break inside one) re-read as a heading followed by a
// paragraph, and the block count changed (#1659). Each break becomes one
// space: the heading keeps its words, and a second rebuild is a fixed point.
func singleLineHeading(text string) string {
	if !strings.Contains(text, "\n") {
		return text
	}
	return headingLineBreakRE.ReplaceAllString(text, " ")
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
	// Soft-break body: the continuation lines carry their ">" marker, except
	// a lazy continuation line (CommonMark 5.1), which has none; only the
	// first line always lacks one. Recover the marker from the first marked
	// continuation line so the quote opens with it: "> a\nb\n> c" re-reads as
	// one blockquote with a lazy line, where a body judged by its first
	// continuation line alone rebuilt as a paragraph plus a quote (#2434).
	if m := continuationBlockquoteMarker(text); m != "" {
		return m, text, true
	}
	return "", text, false
}

// continuationBlockquoteMarker returns the blockquote marker (">" plus an
// optional single space) that begins the first continuation line of text
// carrying one, or "" when text is single-line or no continuation line is a
// blockquote line.
func continuationBlockquoteMarker(text string) string {
	for nl := strings.IndexByte(text, '\n'); nl >= 0; {
		line := text[nl+1:]
		if strings.HasPrefix(line, "> ") {
			return "> "
		}
		if strings.HasPrefix(line, ">") {
			return ">"
		}
		next := strings.IndexByte(line, '\n')
		if next < 0 {
			return ""
		}
		nl += 1 + next
	}
	return ""
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
	if role := block.SemanticRole(); role == model.RoleTableCell || role == model.RoleTableHeader {
		// GFM row splitting sees an unescaped `|` as a cell boundary wherever
		// it appears, even inside a code span, so every pipe leaving a cell
		// is escaped, and one that already is must not be escaped again. The
		// reader hands back both: a code span's `\|` arrives without its
		// backslash (the parser drops it) and a plain `\|` arrives with it,
		// and a translated cell may spell either. Without the escape, a cell
		// like `--output-format <json\|text>` came back with a bare pipe that
		// split the cell and left an unterminated code span (invalid MDX
		// downstream); with an unconditional one, a plain `\|` came back
		// doubled (#1661).
		rendered = escapeCellPipes(rendered)
	}
	return rendered
}

// escapeCellPipes backslash-escapes every `|` in table-cell text whose
// preceding byte is not already a backslash, which is the rule the GFM row
// splitter applies when it decides whether a pipe belongs to the cell.
func escapeCellPipes(text string) string {
	if !strings.Contains(text, "|") {
		return text
	}
	var sb strings.Builder
	sb.Grow(len(text) + 4)
	for i := range len(text) {
		if text[i] == '|' && (i == 0 || text[i-1] != '\\') {
			sb.WriteByte('\\')
		}
		sb.WriteByte(text[i])
	}
	return sb.String()
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
