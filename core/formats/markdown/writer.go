package markdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
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
	// pendingDef is the link reference definition whose Data part was the
	// last thing read on the generative path, held until the blocks carrying
	// its label and title have arrived.
	pendingDef *mdRefDefinition
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

	// The output carries the source's own bytes, trailing whitespace included.
	// Upstream okapi's MarkdownFilterWriter.trimNonEssentialTrailingSpaces
	// drops a line's single trailing space, and mirroring that undid what the
	// reader keeps: a wrapped line's trailing space (#2431), a list item's bare
	// marker, an empty ATX heading's space (#2496). MarkdownCanonical trims
	// every line's trailing whitespace on both sides, so parity never depended
	// on it.
	tw := w.Output

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
				if err := w.writeFromEvents(ctx, events, tw); err != nil {
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
			case model.PartData:
				// A link reference definition is the one Data part the
				// generative path writes: the references to it are written as
				// references, so it has to be there to resolve them.
				if d, ok := part.Resource.(*model.Data); ok && blocksByID == nil && d.Name == dataLinkReferenceDefinition {
					events = append(events, part)
				}
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
		return nil
	}

	// Mode 2: Build from the ordered event stream (cross-format export). Table
	// groups render as GFM tables; everything else renders block-by-block.
	if err := w.writeFromEvents(ctx, events, tw); err != nil {
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
	return nil
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block, out io.Writer) error {
	return format.BufferedSkeletonWrite(store, blocks, out, w.renderRef, nil)
}

// renderRef returns the bytes a SkeletonRef contributes for the given block.
// A nil block contributes nothing, matching a map miss.
func (w *Writer) renderRef(block *model.Block) ([]byte, error) {
	if block == nil {
		return nil, nil
	}
	return []byte(w.blockText(block)), nil
}

// The paired codes the markdown reader wraps a link's or image's title in, so
// the title travels as translatable text rather than inside the closing
// skeleton. Named here because the reader spells them and the writer reads
// them back.
const (
	subTypeLinkTitle  = "md:link-title"
	subTypeImageTitle = "md:image-title"
	// subTypeEntity marks a character reference the reader kept as a
	// placeholder carrying its source spelling.
	subTypeEntity = "md:entity"
)

// The sub-types the markdown reader gives a reference-style link's and image's
// codes. Their closing code's Data is the reference's own spelling, from the
// `]` that ends the text to the end of the label, which is what the writer
// puts back: the definition the label names travels as a part of its own.
const (
	subTypeLinkRef  = "md:link-ref"
	subTypeImageRef = "md:image-ref"
)

// isReferenceLink reports whether a paired code is a reference-style link or
// image rather than an inline one.
func isReferenceLink(r *model.PcOpenRun) bool {
	return r.SubType == subTypeLinkRef || r.SubType == subTypeImageRef
}

// referenceCloser returns a reference's closing spelling for a block the
// generative path lays out itself. A label may hold a line ending, and inside
// a container the source spelling carries the next line's prefix with it; the
// label matches with its whitespace collapsed (CommonMark 4.7), so one space
// stands in for the line ending and the prefix after it.
func referenceCloser(data string) string {
	if !strings.ContainsAny(data, "\r\n") {
		return data
	}
	var sb strings.Builder
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c != '\n' && c != '\r' {
			sb.WriteByte(c)
			continue
		}
		sb.WriteByte(' ')
		for i+1 < len(data) && strings.IndexByte(" \t>\r\n", data[i+1]) >= 0 {
			i++
		}
	}
	return sb.String()
}

// refTitleSpelling spells a definition's title between the delimiters that
// need no escape, so the title re-reads as the same text: double quotes,
// single quotes, then parentheses, and double quotes with escapes when the
// title holds all three.
func refTitleSpelling(title string) string {
	switch {
	case !strings.Contains(title, `"`):
		return `"` + title + `"`
	case !strings.Contains(title, "'"):
		return "'" + title + "'"
	case !strings.ContainsAny(title, "()"):
		return "(" + title + ")"
	}
	return `"` + escapeLinkTitle(title, `"`) + `"`
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
	return mdLinkCloseTitled(attrs, destKey, attrs[model.AttrTitle], 0)
}

// mdLinkCloseTitled builds the same closing syntax with an explicit title,
// which is how a translated one reaches the output: the markdown reader offers
// a title as a text run of its own, so the run stream carries the edited title
// and the attribute still carries the source's. quote is the delimiter the
// source spelled the title with, or 0 for none; an empty title is spelled when
// the source spelled one, because the title's quotes take part in how the
// brackets around the link pair: a link whose text holds "[][](0 '" and whose
// closer is "](0" plus a space, two single quotes and ")" is one link, and
// without the empty title the inner brackets form a link of their own.
func mdLinkCloseTitled(attrs map[string]string, destKey, title string, quote byte) string {
	dest := rebuiltDestination(attrs[destKey])
	if title == "" && quote == 0 {
		return "](" + dest + ")"
	}
	if dest == "" {
		// A title with no destination before it is read as the destination,
		// so an empty one is spelled with its brackets.
		dest = "<>"
	}
	open, closer := titleQuotes(quote)
	return "](" + dest + " " + open + escapeLinkTitle(title, closer) + closer + ")"
}

// titleQuotes returns the delimiters a title is spelled with: the source's
// single or double quote or parentheses, double quotes otherwise. Keeping the
// source's delimiter keeps the quotes around the link pairing as they did:
// "[[](0 '](0 '0')')" re-spelled with double quotes read back as a different
// link.
func titleQuotes(quote byte) (string, string) {
	switch quote {
	case '\'':
		return "'", "'"
	case '(':
		return "(", ")"
	}
	return `"`, `"`
}

// rebuiltDestination spells a resolved destination so that the reader reads
// it back as itself: bare where a bare destination ends where the value does,
// and between angle brackets otherwise (spellLinkDestination, which the edit
// path uses for the same question). goldmark resolves "[](( )" to the
// destination "(", which written bare is "[](()", a link to "((" on one pass
// and no link at all on the next (#3198); a destination the source spelled as
// "<b c>" came back as text. An empty destination stays "()", and a value
// with no spelling that reads back, such as one holding a line break, is
// written bare.
func rebuiltDestination(dest string) string {
	if dest == "" {
		return dest
	}
	spelled, err := spellLinkDestination(dest, false)
	if err != nil {
		return dest
	}
	return spelled
}

// escapeLinkTitle backslash-escapes any double quote in a title that is not
// already escaped, so the title the writer spells between double quotes reads
// back as the same text (CommonMark 6.6 allows an escape anywhere in a title).
// Leaving an escape alone keeps the output idempotent: a title read back from
// this writer carries the backslash and must not gain a second one.
func escapeLinkTitle(title, closer string) string {
	if !strings.Contains(title, closer) {
		return title
	}
	var sb strings.Builder
	escaped := false
	for i := range len(title) {
		c := title[i]
		if c == closer[0] && !escaped {
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
		escaped = c == '\\' && !escaped
	}
	return sb.String()
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
	return trimBlockSpace(renderInline(runs, true))
}

// trimBlockSpace trims the whitespace the parser strips from a paragraph's
// edges: spaces, tabs and line endings. A form feed or a vertical tab is
// content to the parser, so one at a block's edge stays in the text: the
// quote whose first line is a vertical tab, followed by a line holding a
// backslash, lost the line and with it the hard break's place.
func trimBlockSpace(s string) string {
	return strings.Trim(s, " \t\r\n")
}

// renderInlineMarkdownBlock is renderInlineMarkdown for a block that sits
// quoteLevels blockquotes deep, whose continuation lines carry that many
// markers; whose text is raw markup when rawText is set, where a ">" that
// opens a continuation line is content rather than the marker of a quote;
// whose first line writeBlockMarkdown escapes a leading block marker on when
// leadingMarker is set, which it does for every block but a heading or code;
// which is written as one ATX line when heading is set; and whose hard-break
// continuation lines shared the indentation linePrefix, which the reader took
// out of the text and the output puts back.
func renderInlineMarkdownBlock(runs []model.Run, quoteLevels int, rawText, leadingMarker, heading, listItem bool, linePrefix string) string {
	sink := &mdInlineSink{escapeAngle: true, quoteLevels: quoteLevels, rawText: rawText, leadingMarker: leadingMarker, heading: heading, listItem: listItem, linePrefix: linePrefix, droppedAt: -1}
	projection.WalkInline(runs, sink)
	sink.flush()
	return trimBlockSpace(sink.output())
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
	sink := &mdInlineSink{escapeAngle: escapeAngle, droppedAt: -1}
	projection.WalkInline(runs, sink)
	sink.flush()
	return sink.output()
}

// mdInlineSink maps the shared inline-run stream (projection.WalkInline) to
// Markdown, owning the open-delimiter stack the paired-code close needs. It
// replaces the writer's former bespoke run loop; WalkInline now handles run
// decoding + plural/select 'other'-branch resolution. Like the old loop it never
// consults a run's Data — the same Markdown results whatever the source format.
// The one exception is a code the markdown reader sub-typed as a reference
// link or image, whose closing Data is markdown's own spelling of the label.
type mdInlineSink struct {
	sb          strings.Builder
	open        []mdOpenTag // stack of open paired codes, innermost last
	escapeAngle bool        // backslash-escape literal '<' (paragraph text, not code)
	literal     bool        // emit run text only, no markup (fenced code content)
	// pending holds a link's or image's closing markup back until the next run
	// is known, because a title pair follows that close and carries the title
	// the closer has to spell.
	pending *mdPendingClose
	// title collects a title pair's text while one is open.
	title *strings.Builder
	// heldBreak is set by a hard break that opened its own line, and spends
	// itself on the next run: a backslash, when the text after it begins the
	// following line.
	heldBreak bool
	// openingBreak is set by a hard break that opened the block and spelled
	// nothing, so the text after it drops the line break and continuation
	// prefix that would otherwise begin the block's first line.
	openingBreak bool
	// quoteLevels is how many blockquotes deep the block sits, which bounds
	// the markers a continuation line's prefix carries.
	quoteLevels int
	// litDelims holds the offset of every unescaped '*', '_' and '~' the sink
	// wrote from a TEXT run, which output() may have to escape. Delimiters the
	// sink writes as markup are not recorded: those are meant to pair.
	litDelims []int
	// lastClosed is the emphasis or strong pair whose closer the sink wrote
	// last, so the next closer can tell whether it would touch it.
	lastClosed mdClosedEmphasis
	// heldClose is an emphasis closer held back until the byte after it is
	// known (see spellHeldClose).
	heldClose *mdHeldClose
	// pairs holds every emphasis and strong pair the sink wrote, which output()
	// checks against the reader's parser.
	pairs []mdClosedEmphasis
	// markupEnd is the offset just past the last code span fence or
	// strikethrough delimiter the sink wrote, and markupChar its character, so
	// a literal of that character written right after it can be kept apart.
	markupEnd  int
	markupChar byte
	// litTickRuns holds the lengths of the unescaped backtick runs the sink
	// wrote from TEXT runs, which a code span's fence has to differ from.
	litTickRuns map[int]bool
	// bareAngles holds the offset of every unescaped '<' the sink wrote from a
	// TEXT run, which output() escapes where the parser reads a tag or an
	// autolink from it; bareTicks the same for every '`', escaped where the
	// parser reads a code span from it; bareBrackets the same for every '['
	// outside link text, escaped where the text after it completes a link;
	// bareAmps the same for every '&', escaped where the text after it
	// completes a character reference.
	bareAngles   []int
	bareTicks    []int
	bareBrackets []int
	bareAmps     []int
	// shortcutRefs holds the offsets of the brackets of every shortcut
	// reference link the sink wrote, which output() spells as a collapsed
	// reference where the line it opens reads as a link reference definition.
	shortcutRefs []mdClosedEmphasis
	// lastCodeSpan is the code span the sink closed last, for a code span
	// opened right after it to merge into: two fences touching read as one
	// run.
	lastCodeSpan mdCodeSpan
	// droppedAt is the output length right after the last inline construct
	// the sink wrote nothing for (inline HTML), or -1. A ">" that opens a text
	// run there at the start of a line is content: the reader bakes a quote's
	// continuation markers into the run that holds the line ending, so a
	// marker never begins a run of its own.
	droppedAt int
	// rawText is set for a block whose text is markup the reader captured
	// verbatim, which writeBlockMarkdown never recovers as a quote from the
	// markers its continuation lines carry.
	rawText bool
	// leadingMarker is set for a block whose first line writeBlockMarkdown
	// escapes a leading block marker on.
	leadingMarker bool
	// heading is set for a block writeBlockMarkdown writes as one ATX line,
	// its line breaks folded to spaces.
	heading bool
	// listItem is set for a block writeBlockMarkdown writes as a list item,
	// whose continuation lines it escapes any block marker on.
	listItem bool
	// markupMarker is set when a block marker writeBlockMarkdown will escape
	// sits on a delimiter the sink wrote, so the pair has to be re-spelled.
	markupMarker bool
	// linePrefix is the prefix the block's hard-break continuation lines
	// shared, indentation or quote markers, which the reader took out of the
	// text; output() puts it back after every line ending.
	linePrefix string
	// breakSpelled is set by a hard break the sink spelled with a backslash,
	// so the newline after it does not take that backslash for a literal one.
	breakSpelled bool
	// breakSpaces is set by a hard break the sink spelled with two spaces,
	// so the newline after it does not trim them as text's own spaces.
	breakSpaces bool
}

// mdOpenTag is one entry of the open-paired-code stack.
type mdOpenTag struct {
	close   string            // the markup that closes the pair
	at      int               // the offset of an emphasis or strong pair's opener
	attrs   map[string]string // a link's or image's attributes
	destKey string            // model.AttrHref or model.AttrSrc; empty for anything else
	title   bool              // the pair holds the title of the link or image that just closed
	ref     bool              // the pair is a reference-style link or image
}

// mdClosedEmphasis locates an emphasis or strong pair in the output: a run of
// n delimiter characters at openAt and another at closeAt. A zero n is no pair.
type mdClosedEmphasis struct {
	char            byte
	openAt, closeAt int
	n               int
}

// mdCodeSpan is where a closed code span sits in the output: its opener's
// offset, the end of its closing fence, and the fence and padding its content
// was spelled with.
type mdCodeSpan struct {
	openAt, end int
	fence, pad  int
}

// mdHeldClose is an emphasis closer held back until the byte after it is
// known. inner is the pair whose closer it would touch, or a zero pair when
// it touches none; enclosed reports whether a pair of the same character is
// still open around it.
type mdHeldClose struct {
	tag      mdOpenTag
	inner    mdClosedEmphasis
	enclosed bool
}

// mdPendingClose is a link's or image's closing markup, held until the sink
// knows whether a title pair follows it.
type mdPendingClose struct {
	attrs   map[string]string
	destKey string
	// quote is the delimiter the title pair that follows was spelled with in
	// the source, or 0 when none has arrived.
	quote byte
	// data is the closer's spelling in the source, or "" when the run
	// carried none.
	data string
	// titleOpen is the source spelling of the bytes from the "(" after the
	// link's text to the title's opening delimiter, when a title pair
	// follows, or "".
	titleOpen string
}

// isLinkTitlePair reports whether a paired code holds a link's or image's
// title. The markdown reader splits a titled link into the link's own pair and
// a second one around the title text, so the title is offered for translation
// (mirroring okapi's MarkdownFilter). The title belongs inside the closer of
// the pair before it, and spells nothing of its own.
func isLinkTitlePair(r *model.PcOpenRun) bool {
	return r.SubType == subTypeLinkTitle || r.SubType == subTypeImageTitle
}

func (s *mdInlineSink) Text(t string) {
	if s.title != nil {
		s.title.WriteString(t)
		return
	}
	if t != "" {
		s.spellHeldClose(firstRune(t))
	}
	s.flushPending()
	if s.openingBreak {
		s.openingBreak = false
		// With the continuation prefix recorded rather than in the text, the
		// markers are already out of the text and a ">" after the line
		// ending is content.
		levels := s.quoteLevels
		if s.linePrefix != "" {
			levels = 0
		}
		t = dropOpeningBreakPrefix(t, levels)
	}
	s.spellHeldBreak(t)
	if s.escapeAngle {
		if t != "" && t[0] == '>' && s.droppedAt > 0 && s.droppedAt == s.sb.Len() && s.atLineStart() && !s.inCodeSpan() {
			// A ">" opening a text run at the start of a line, right after a
			// construct the sink dropped, is content: the dropped tag was
			// what kept it from being a marker. ">" and a vertical tab over
			// "<A>>" is a quote whose lazy line holds a ">", and written bare
			// the ">" was a marker-only line the block path folded away. At
			// the block's start the leading-marker escape answers instead,
			// and a table cell's ">" opens nothing.
			s.sb.WriteByte('\\')
		}
		s.writeEscapingAngle(t)
		return
	}
	s.sb.WriteString(t)
}

// endsInUnescapedBackslash reports whether out ends in a backslash that is
// itself unescaped, an odd run of them, with any bare carriage returns after
// it: a backslash before "\r\n" spells a hard break as one before "\n" does.
func endsInUnescapedBackslash(out string) bool {
	i := len(out) - 1
	for i >= 0 && out[i] == '\r' {
		i--
	}
	n := 0
	for ; i >= 0 && out[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// inLinkText reports whether a link or image is open, so the text being
// written is its text. A link does not nest, and a bracket escaped there
// changes which "]" the parser pairs the link's own "[" with: the opener is
// dropped after the first "](" that forms no link.
func (s *mdInlineSink) inLinkText() bool {
	for _, t := range s.open {
		if t.destKey != "" || t.ref {
			return true
		}
	}
	return false
}

// lineIsBreakRuns reports whether the output's current line, after the quote
// markers it carries and up to three spaces of indent, is a thematic break
// made of delimiter runs: the closers of pairs, the openers of pairs still
// open, or literal delimiters, which a bare line ending would turn into one.
// A backslash break after such a line is kept whatever wrote the runs: the
// openers of "****" over a backslash break were degraded to text on one pass,
// which kept the backslash, and the pass after, reading the run as text,
// dropped it and escaped the run instead.
func (s *mdInlineSink) lineIsBreakRuns() bool {
	out := s.sb.String()
	start := strings.LastIndexByte(out, '\n') + 1
	line := out[start:]
	content := start + len(blockquoteMarkerPrefixUpTo(line, s.quoteLevels))
	rest := out[content:]
	// CommonMark 4.1 lets a thematic break sit behind up to three spaces: the
	// openers of " ***" over a backslash break and "0**" closed nothing once
	// the backslash went, and the line was a thematic break.
	indent := len(rest) - len(strings.TrimLeft(rest, " "))
	if indent > 3 {
		return false
	}
	rest = rest[indent:]
	if s.heldClose != nil {
		// A closer held until the byte after it is known is on the line too.
		rest += s.heldClose.tag.close
	}
	return rest != "" && isThematicBreak(rest)
}

// checkLevels is the quote depth the parser checks on out run at: the depth
// the sink was told, or, for a block told none whose continuation lines every
// one carry a quote marker, the depth of that marker, since writeBlockMarkdown
// recovers such a block as a quote and marks its first line. Checked at no
// depth, the marked lines open a quote that interrupts the first line's
// paragraph, and the parser could give no verdict, so the pairs went
// unverified and the pass after, reading the block as the quote it had
// become, spelled them differently.
func (s *mdInlineSink) checkLevels(out string) int {
	if s.quoteLevels > 0 || s.rawText {
		return s.quoteLevels
	}
	if m := continuationBlockquoteMarker(out); m != "" {
		return strings.Count(m, ">")
	}
	return 0
}

// checkFrame is the block the parser checks on out read it inside: the depth
// checkLevels gives and whether the block is a list item.
func (s *mdInlineSink) checkFrame(out string) blockFrame {
	return blockFrame{quoteLevels: s.checkLevels(out), listItem: s.listItem}
}

// inBareBracket reports whether the output's last bracket written as text is
// still open: no unescaped "]" follows it.
func (s *mdInlineSink) inBareBracket() bool {
	if len(s.bareBrackets) == 0 {
		return false
	}
	out := s.sb.String()
	for i := s.bareBrackets[len(s.bareBrackets)-1] + 1; i < len(out); i++ {
		switch out[i] {
		case '\\':
			i++
		case ']':
			return false
		}
	}
	return true
}

// inCodeSpan reports whether the innermost open pair is a code span, whose
// content is literal: a backslash inside it is a character, not an escape
// (CommonMark 6.1), so nothing in it is escaped or recorded as a delimiter.
func (s *mdInlineSink) inCodeSpan() bool {
	return len(s.open) > 0 && s.open[len(s.open)-1].close == "`"
}

// spellHeldBreak spends a hard break held from the previous run on the text
// that follows it: a backslash when that text begins the next line, so the
// break's own line is not left empty, and nothing otherwise, because a lone
// backslash mid-line escapes the character after it.
//
// A backslash the text itself ends in is a different matter when a line
// ending follows: a backslash at the end of a line spells a hard break
// (CommonMark 6.7), so a literal one that a break's bare newline would follow
// is escaped to stay a character. The backslash the sink writes to spell a
// break before break spaces is markup and is left alone.
func (s *mdInlineSink) spellHeldBreak(next string) {
	spelled := s.breakSpelled
	s.breakSpelled = false
	if !s.heldBreak {
		if strings.HasPrefix(next, "\n") && !spelled && !s.inCodeSpan() && endsInUnescapedBackslash(s.sb.String()) {
			s.sb.WriteByte('\\')
		}
		return
	}
	s.heldBreak = false
	if strings.HasPrefix(next, "\n") {
		s.sb.WriteByte('\\')
	}
}

// flushPending spells a held-back link or image closer with the title its
// attributes carry, which is the answer whenever no title pair follows. A
// closer the source spelled is kept where it still reads as the same
// destination: the whitespace inside it takes part in how the brackets around
// the link pair, and "[[][](]( 0))" re-spelled as "[[][](](0))" read back as a
// different link.
func (s *mdInlineSink) flushPending() {
	p := s.pending
	if p == nil {
		return
	}
	s.pending = nil
	if p.attrs[model.AttrTitle] == "" {
		if closer, ok := sourceLinkCloser(p.data, p.attrs[p.destKey]); ok {
			s.sb.WriteString(closer)
			return
		}
	}
	s.sb.WriteString(mdLinkCloseTitled(p.attrs, p.destKey, p.attrs[model.AttrTitle], p.quote))
}

// sourceLinkCloser returns the closer a link's source spelled, with a line
// ending inside it collapsed to a space the way referenceCloser does, when
// that spelling reads back as a link to dest with no title.
func sourceLinkCloser(data, dest string) (string, bool) {
	if !strings.HasPrefix(data, "](") || !strings.HasSuffix(data, ")") {
		return "", false
	}
	closer := referenceCloser(data)
	if !linkCloserReadsBack(closer, dest, "") {
		return "", false
	}
	return closer, true
}

// linkCloserReadsBack reports whether closer, written after a link's text,
// reads back as one link to dest with the title given, raw as the source
// spells it between its delimiters.
func linkCloserReadsBack(closer, dest, title string) bool {
	// The link is parsed as a line of a document, with the line ending the
	// parser's destination scan expects after it.
	doc := parseMarkdown([]byte("[x" + closer + "\n"))
	para, ok := doc.FirstChild().(*ast.Paragraph)
	if !ok || doc.ChildCount() != 1 || para.ChildCount() != 1 {
		return false
	}
	link, ok := para.FirstChild().(*ast.Link)
	return ok && string(link.Destination) == dest && string(link.Title) == title
}

// emptyTitleQuote returns the delimiter an empty title was spelled with in a
// link closer's source data, or 0 when the closer carries no empty title: the
// data is "](", the destination as the reader recorded it, bare or between
// angle brackets, then the empty pair and ")".
func emptyTitleQuote(data, dest string) byte {
	if !strings.HasPrefix(data, "](") || !strings.HasSuffix(data, ")") {
		return 0
	}
	inner := data[2 : len(data)-1]
	rest, angled := strings.CutPrefix(inner, "<"+dest+">")
	if !angled {
		rest = strings.TrimPrefix(inner, dest)
	}
	switch strings.TrimLeft(rest, " \t\r\n") {
	case "''":
		return '\''
	case `""`:
		return '"'
	case "()":
		return '('
	}
	return 0
}

// spellPendingTitle spells a held-back closer with the title collected from the
// title pair, and reports whether there was one to spell.
func (s *mdInlineSink) spellPendingTitle() bool {
	if s.title == nil || s.pending == nil {
		s.title = nil
		return false
	}
	p, title := s.pending, s.title.String()
	s.pending, s.title = nil, nil
	// The destination as the source spelled it, with the title the stream
	// carries between the source's delimiters, is kept where it reads back
	// as the link: the whitespace before the destination takes part in how
	// the brackets around the link pair, as it does for a closer with no
	// title.
	if p.titleOpen != "" {
		_, closeDelim := titleQuotes(p.quote)
		closer := "]" + referenceCloser(p.titleOpen) + escapeLinkTitle(title, closeDelim) + closeDelim + ")"
		if linkCloserReadsBack(closer, p.attrs[p.destKey], title) {
			s.sb.WriteString(closer)
			return true
		}
	}
	s.sb.WriteString(mdLinkCloseTitled(p.attrs, p.destKey, title, p.quote))
	return true
}

// writeEscapingAngle writes t, recording every '<' that is not already
// escaped for output() to escape where the parser reads a tag or an autolink
// from it, and backslash-escaping any '[' that opens an EMPTY link text. The
// rebuild path drops inline-HTML constructs, and the residue can abut
// surrounding text into a NEW tag: "<<A>A>" drops the <A> and the leftover
// "<" and "A>" reform "<A>", which re-reads as inline HTML and is dropped
// entirely, losing the whole block (#1652). Escaping the '<' the parser pairs
// stops text from re-parsing as a tag or autolink; the reader preserves the
// backslash, so already-escaped input is left alone and the output is
// idempotent. A '<' the parser leaves as text stays bare: escaped, "\<" is a
// link destination where a bare "<" is not, and "[[](<A 't')" became a link.
// Only Text() (literal run content) is handled — the HTML the sink emits for
// formatting fallbacks (<mark>, <sup>, …) comes through Open/Close, untouched.
//
// The angle escape is what makes the bracket escape necessary. "\<" IS a link
// destination where a bare "<" is not, so escaping the angle in "[](<)" spelled
// a link with no text at all, and a link with no text carries nothing to
// translate: the block was gone (#2526). The same escape turned "[0](< 't')"
// into a link with a title, which the pass after re-spelled. A bracket span
// that a "(" and a "<" follow is escaped too, so the text stays text.
func (s *mdInlineSink) writeEscapingAngle(t string) {
	if s.inCodeSpan() {
		s.sb.WriteString(t)
		return
	}
	if s.markupEnd > 0 && s.markupEnd == s.sb.Len() && t[0] == s.markupChar {
		// A literal backtick or tilde right after a fence or delimiter of the
		// same character would merge with it, so it is escaped and stands as
		// text.
		s.sb.WriteByte('\\')
	}
	escaped := false
	ticks := 0
	for i := range len(t) {
		c := t[i]
		if c == '`' && !escaped {
			ticks++
			s.bareTicks = append(s.bareTicks, s.sb.Len())
		} else if ticks > 0 {
			s.recordTickRun(ticks)
			ticks = 0
		}
		switch {
		case c == '<' && !escaped:
			s.bareAngles = append(s.bareAngles, s.sb.Len())
		case c == '[' && !escaped && !s.inLinkText():
			s.bareBrackets = append(s.bareBrackets, s.sb.Len())
		case c == '&' && !escaped:
			s.bareAmps = append(s.bareAmps, s.sb.Len())
		case c == '\\' && !escaped && lineEndingFollows(t, i+1):
			// A backslash before a line ending spells a hard break (CommonMark
			// 6.7); a literal one inside the text takes a second so it stays a
			// character.
			s.sb.WriteByte('\\')
		case c == '\n':
			s.trimBreakSpaces()
		case (c == '*' || c == '_' || c == '~') && !escaped && !s.inLinkText():
			// A delimiter inside a link's text pairs with nothing outside
			// it, since the brackets are resolved first (CommonMark 6.3),
			// and a reference's text is its label, matched as spelled:
			// "[*]:0" over "*[*]" had the label's asterisk escaped and
			// matched nothing.
			s.litDelims = append(s.litDelims, s.sb.Len())
		}
		s.sb.WriteByte(c)
		escaped = c == '\\' && !escaped
	}
	if ticks > 0 {
		s.recordTickRun(ticks)
	}
}

// trimBreakSpaces leaves one space of the run of two or more spaces or tabs
// the output ends with, before a line ending the text carries: two or more
// before a line ending are a hard break (CommonMark 6.7), which the pass after
// would read in place of the spaces. The run can only come from text, as the
// text on either side of a dropped construct: "0 <A> " and a line ending left
// "0  " once the inline HTML was dropped. Carriage returns before the line
// ending stay where they are.
func (s *mdInlineSink) trimBreakSpaces() {
	if s.breakSpaces {
		s.breakSpaces = false
		return
	}
	if s.inCodeSpan() {
		return
	}
	out := s.sb.String()
	end := len(out)
	for end > 0 && out[end-1] == '\r' {
		end--
	}
	j := end
	for j > 0 && (out[j-1] == ' ' || out[j-1] == '\t') {
		j--
	}
	if end-j < 2 {
		return
	}
	s.sb.Reset()
	s.sb.WriteString(out[:j])
	s.sb.WriteByte(' ')
	s.sb.WriteString(out[end:])
}

// lineEndingFollows reports whether t[i:] begins with a line ending: a newline,
// with any bare carriage returns before it.
func lineEndingFollows(t string, i int) bool {
	for i < len(t) && t[i] == '\r' {
		i++
	}
	return i < len(t) && t[i] == '\n'
}

// recordTickRun notes the length of a literal backtick run the sink wrote.
func (s *mdInlineSink) recordTickRun(n int) {
	if s.litTickRuns == nil {
		s.litTickRuns = map[int]bool{}
	}
	s.litTickRuns[n] = true
}

// opensLinkTextToEscape reports whether s begins with a bracket that has to be
// escaped for the text to stay text where the parser's own reading of the
// output cannot say so: a bracket span followed by "(" and a "<", which the
// angle escape would complete into a link, and angle is the offset in s of
// that "<". The caller asks whether the angle is one the escape takes; one
// left bare completes nothing. Any other bracket is text unless the parser
// forms a link from it, which formedMarkup reports. Escaped on its shape
// alone, the "[](" of "[[](]]", the "[][" of "[[][]]]" and the "[](<" of
// "[[](<]]", each before a reference "[R]", had their inner bracket escaped,
// and the span from the outer bracket was the text of a full reference link
// "[...][R]".
func opensLinkTextToEscape(s string) (angle int, ok bool) {
	// The span is cut as a label is, so an escaped bracket inside it is a
	// character and not its end.
	_, rest, ok := cutLinkLabel(s)
	if !ok || !strings.HasPrefix(rest, "(") {
		return 0, false
	}
	after := strings.TrimLeft(rest[1:], " \t")
	if !strings.HasPrefix(after, "<") {
		return 0, false
	}
	return len(s) - len(after), true
}

func (s *mdInlineSink) Open(r *model.PcOpenRun) {
	if s.literal {
		s.open = append(s.open, mdOpenTag{})
		return
	}
	s.spellHeldBreak("")
	if isLinkTitlePair(r) {
		// The pair spells no markup of its own. With a link's closer still held
		// back, its text is that closer's title; with none (a target whose runs
		// were reordered) the text stays where it is, as text.
		s.open = append(s.open, mdOpenTag{title: true})
		if s.pending != nil {
			s.title = &strings.Builder{}
			// The pair's opening data ends in the delimiter the source spelled
			// the title with.
			if d := r.Data; d != "" {
				s.pending.quote = d[len(d)-1]
				s.pending.titleOpen = d
			}
		}
		return
	}
	s.flushPending()
	switch r.Type {
	case "link:hyperlink":
		// [text](href "title") — the link text is the paired content.
		s.spellHeldClose('[')
		at := s.sb.Len()
		s.sb.WriteString("[")
		s.open = append(s.open, mdOpenTag{attrs: r.Attrs, destKey: model.AttrHref, ref: isReferenceLink(r), at: at})
	case "media:image", "link:image":
		// ![alt](src "title"). The alt text is the paired content, except
		// where a reader does not offer it for translation and records it on
		// the alt attribute instead (markdown's translateImageAlt: false).
		s.spellHeldClose('!')
		s.sb.WriteString("![" + r.Attr(model.AttrAlt))
		s.open = append(s.open, mdOpenTag{attrs: r.Attrs, destKey: model.AttrSrc, ref: isReferenceLink(r)})
	default:
		if m, ok := mdInlineTag[r.Type]; ok {
			s.spellHeldClose(firstRune(m[0]))
			open, close := s.emphasisSpelling(m[0], m[1])
			if open == "`" && s.mergeIntoLastCodeSpan() {
				return
			}
			if open != "" && (open[0] == '~' || open[0] == '`') {
				s.escapeTouchingRun(open[0])
			}
			at := s.sb.Len()
			s.sb.WriteString(open)
			if open != "" && (open[0] == '~' || open[0] == '`') {
				s.markupEnd, s.markupChar = s.sb.Len(), open[0]
			}
			s.open = append(s.open, mdOpenTag{close: close, at: at})
		} else {
			s.open = append(s.open, mdOpenTag{})
			s.droppedAt = s.sb.Len()
		}
	}
}

// emphasisSpelling picks the delimiter for an emphasis or strong code that
// opens where the last one closed. Two asterisk pairs that touch spell a run of
// four ("*a**b*"), which CommonMark reads as one pair around "a**b" rather than
// as the two the block carries, so the block's content changed on the next pass
// (#2499). Underscores read as a pair there because the character before them
// is punctuation, which is what CommonMark 6.2 asks of an intraword "_".
//
// An opener that follows another opener keeps its character: one run of
// asterisks opening nested pairs reads back as those pairs, and an underscore
// after it would make the run punctuation-followed, which an alphanumeric
// before the run then keeps from opening at all.
func (s *mdInlineSink) emphasisSpelling(open, close string) (string, string) {
	if open != "*" && open != "**" {
		return open, close
	}
	b := s.sb.String()
	if b == "" || b[len(b)-1] != '*' {
		return open, close
	}
	if s.lastClosed.char != '*' || s.lastClosed.n == 0 || s.lastClosed.closeAt+s.lastClosed.n != len(b) {
		return open, close
	}
	return strings.ReplaceAll(open, "*", "_"), strings.ReplaceAll(close, "*", "_")
}

func (s *mdInlineSink) Close(r *model.PcCloseRun) {
	s.spellHeldBreak("")
	n := len(s.open)
	if n == 0 {
		s.flushPending()
		return
	}
	tag := s.open[n-1]
	s.open = s.open[:n-1]
	if tag.title {
		s.spellPendingTitle()
		return
	}
	s.flushPending()
	if tag.ref && r != nil && r.Data != "" {
		// A reference link is written back as a reference: its definition
		// is a part of its own and the writer spells that too. Resolving it
		// inline here left the definition's title block to become a
		// paragraph (#2635).
		s.spellHeldClose(']')
		closer := referenceCloser(r.Data)
		if closer == "]" && tag.destKey == model.AttrHref {
			s.shortcutRefs = append(s.shortcutRefs, mdClosedEmphasis{char: ']', openAt: tag.at, closeAt: s.sb.Len()})
		}
		s.sb.WriteString(closer)
		return
	}
	if tag.destKey != "" {
		s.spellHeldClose(']')
		s.pending = &mdPendingClose{attrs: tag.attrs, destKey: tag.destKey}
		if r != nil {
			s.pending.data = r.Data
			s.pending.quote = emptyTitleQuote(r.Data, tag.attrs[tag.destKey])
		}
		return
	}
	if tag.close == "" {
		s.droppedAt = s.sb.Len()
		return
	}
	if tag.close == "`" {
		s.closeCodeSpan(tag.at)
		return
	}
	if tag.close[0] == '~' {
		// An emphasis closer held inside the strikethrough is spelled before
		// the strikethrough closes around it.
		s.spellHeldClose('~')
		s.escapeTouchingRun('~')
		s.pairs = append(s.pairs, mdClosedEmphasis{char: '~', openAt: tag.at, closeAt: s.sb.Len(), n: len(tag.close)})
		s.sb.WriteString(tag.close)
		s.markupEnd, s.markupChar = s.sb.Len(), '~'
		return
	}
	s.spellHeldClose(firstRune(tag.close))
	if isEmphasisDelimiter(tag.close) {
		// An emphasis closer is held until the byte after it is known, which
		// is what CommonMark 6.2's flanking rules ask of a closer, and
		// spellHeldClose then decides the pair's spelling. Two closers of the
		// same character written back-to-back form one run, and the rule of
		// three can keep that run from pairing with the inner opener:
		// "_!*!0*_" rebuilt as "*!*!0**" came back as different pairs around
		// different text (#3106). Pairs opened by one run and closed by one
		// run are the exception: the run reads back as those pairs.
		closed := mdClosedEmphasis{char: tag.close[0], openAt: tag.at, closeAt: s.sb.Len(), n: len(tag.close)}
		h := &mdHeldClose{tag: tag, enclosed: s.enclosedBy(closed.char)}
		if s.lastClosed.n > 0 && s.lastClosed.char == closed.char && s.lastClosed.closeAt+s.lastClosed.n == closed.closeAt {
			if s.lastClosed.openAt == closed.openAt+closed.n {
				s.closedPair(closed)
				s.sb.WriteString(tag.close)
				return
			}
			h.inner = s.lastClosed
		}
		s.heldClose = h
		return
	}
	s.sb.WriteString(tag.close)
}

// closedPair records a pair whose closer is written next.
func (s *mdInlineSink) closedPair(e mdClosedEmphasis) {
	s.lastClosed = e
	s.pairs = append(s.pairs, e)
}

// enclosedBy reports whether an emphasis or strong pair of character c is
// still open on the stack.
func (s *mdInlineSink) enclosedBy(c byte) bool {
	for _, t := range s.open {
		if isEmphasisDelimiter(t.close) && t.close[0] == c {
			return true
		}
	}
	return false
}

// closeCodeSpan closes the code span whose one-backtick opener sits at openAt,
// re-spelling both fences to fit the content between them. CommonMark 6.1
// closes a span on a backtick run exactly as long as the opener, so a fence
// of one backtick around content holding a run of three merged with it and
// the span re-read at different positions on every pass (#3193). The fence is
// one backtick longer than the longest run inside, the way codeFence sizes a
// code block's (#2487), and a padding space keeps it apart from a backtick
// the content begins or ends with. The same space pads content that begins
// and ends with a space, which the re-read would otherwise strip one of.
func (s *mdInlineSink) closeCodeSpan(openAt int) {
	out := s.sb.String()
	content := out[openAt+1:]
	before := out[:openAt]
	fence := codeSpanFence(content, s.litTickRuns)
	pad := ""
	if codeSpanNeedsPadding(content) {
		pad = " "
	}
	if fence == "`" && pad == "" {
		s.sb.WriteByte('`')
		s.markupEnd, s.markupChar = s.sb.Len(), '`'
		s.lastCodeSpan = mdCodeSpan{openAt: openAt, end: s.sb.Len(), fence: 1}
		return
	}
	s.sb.Reset()
	s.sb.WriteString(before)
	s.sb.WriteString(fence)
	s.sb.WriteString(pad)
	s.sb.WriteString(content)
	s.sb.WriteString(pad)
	s.sb.WriteString(fence)
	s.markupEnd, s.markupChar = s.sb.Len(), '`'
	s.lastCodeSpan = mdCodeSpan{openAt: openAt, end: s.sb.Len(), fence: len(fence), pad: len(pad)}
	// A literal delimiter recorded inside the content moved with it.
	shift := len(fence) + len(pad) - 1
	for i, off := range s.litDelims {
		if off > openAt {
			s.litDelims[i] = off + shift
		}
	}
	for i, off := range s.bareAngles {
		if off > openAt {
			s.bareAngles[i] = off + shift
		}
	}
	for i, off := range s.bareTicks {
		if off > openAt {
			s.bareTicks[i] = off + shift
		}
	}
	for i, off := range s.bareBrackets {
		if off > openAt {
			s.bareBrackets[i] = off + shift
		}
	}
	for i, off := range s.bareAmps {
		if off > openAt {
			s.bareAmps[i] = off + shift
		}
	}
}

// mergeIntoLastCodeSpan reopens the code span the output ends with, so the
// span being opened continues it, and reports whether it did. Two code spans
// with nothing between them have no spelling: the closing fence of one and
// the opening fence of the next touch and read as one run. The spans are
// adjacent only because what stood between them was dropped, inline HTML most
// often, and one span over both contents carries the same text. "“<A>`0“"
// followed by inline HTML and "`0`" was written with its fences touching, and
// the pass after read the run of three as an opener.
func (s *mdInlineSink) mergeIntoLastCodeSpan() bool {
	last := s.lastCodeSpan
	if last.end == 0 || last.end != s.sb.Len() {
		return false
	}
	out := s.sb.String()
	content := out[last.openAt+last.fence+last.pad : last.end-last.fence-last.pad]
	s.sb.Reset()
	s.sb.WriteString(out[:last.openAt])
	s.sb.WriteByte('`')
	s.sb.WriteString(content)
	// The offsets recorded inside the content move back with it.
	shift := last.fence + last.pad - 1
	for i, off := range s.litDelims {
		if off > last.openAt {
			s.litDelims[i] = off - shift
		}
	}
	for i, off := range s.bareAngles {
		if off > last.openAt {
			s.bareAngles[i] = off - shift
		}
	}
	for i, off := range s.bareTicks {
		if off > last.openAt {
			s.bareTicks[i] = off - shift
		}
	}
	for i, off := range s.bareBrackets {
		if off > last.openAt {
			s.bareBrackets[i] = off - shift
		}
	}
	for i, off := range s.bareAmps {
		if off > last.openAt {
			s.bareAmps[i] = off - shift
		}
	}
	s.markupEnd, s.lastCodeSpan = 0, mdCodeSpan{}
	s.open = append(s.open, mdOpenTag{close: "`", at: last.openAt})
	return true
}

// escapeTouchingRun escapes every character of an unescaped run of c that
// ends the output, so the code span fence or strikethrough delimiter of c
// written next stands apart from it: a literal backtick before a fence pairs
// with the fence, and a literal tilde beside a "~~" makes a run of three,
// which pairs with nothing. A backslash escapes one character, so each one of
// the run takes its own, or the rest of the run merges with the fence: a
// literal run of two before a one-backtick fence, escaped as one, read as a
// run of three.
func (s *mdInlineSink) escapeTouchingRun(c byte) {
	out := s.sb.String()
	if s.markupEnd == len(out) && s.markupChar == c {
		// The run is markup the sink wrote itself.
		return
	}
	j := len(out)
	for j > 0 && out[j-1] == c {
		j--
	}
	if j == len(out) || backslashEscaped(out, j) {
		return
	}
	s.sb.Reset()
	s.sb.WriteString(out[:j])
	for i := j; i < len(out); i++ {
		s.sb.WriteByte('\\')
		s.sb.WriteByte(out[i])
	}
	// An offset inside the run moves by the backslashes inserted before it,
	// one after it moves by the whole run's.
	shift := func(off int) int {
		switch {
		case off < j:
			return off
		case off >= len(out):
			return off + len(out) - j
		}
		return off + off - j + 1
	}
	// The run is no longer a literal that output() may escape.
	kept := s.litDelims[:0]
	for _, off := range s.litDelims {
		if off >= j {
			continue
		}
		kept = append(kept, off)
	}
	s.litDelims = kept
	for i, off := range s.bareAngles {
		s.bareAngles[i] = shift(off)
	}
	keptTicks := s.bareTicks[:0]
	for _, off := range s.bareTicks {
		if off >= j {
			continue
		}
		keptTicks = append(keptTicks, off)
	}
	s.bareTicks = keptTicks
	for i, off := range s.bareBrackets {
		s.bareBrackets[i] = shift(off)
	}
	for i, off := range s.bareAmps {
		s.bareAmps[i] = shift(off)
	}
	for i := range s.open {
		s.open[i].at = shift(s.open[i].at)
	}
	for i := range s.pairs {
		s.pairs[i].openAt = shift(s.pairs[i].openAt)
		s.pairs[i].closeAt = shift(s.pairs[i].closeAt)
	}
}

// codeSpanFence returns the backtick run that fences a code span around
// content: the shortest run of backticks that no run inside the content
// matches, and that no literal run the block's text holds matches either,
// since CommonMark 6.1 closes a span on a run exactly as long as its opener
// and a literal run before the span would otherwise pair with the opener. The
// fences of other spans are markup already paired and do not count. The
// shortest fence keeps a span that opens a block from spelling a fenced code
// block's opener, which three backticks before a line ending do.
func codeSpanFence(content string, literal map[int]bool) string {
	runs := map[int]bool{}
	for n := range literal {
		runs[n] = true
	}
	n := 0
	for i := range len(content) {
		if content[i] == '`' {
			n++
			continue
		}
		if n > 0 {
			runs[n] = true
		}
		n = 0
	}
	if n > 0 {
		runs[n] = true
	}
	fence := 1
	for runs[fence] {
		fence++
	}
	return strings.Repeat("`", fence)
}

// codeSpanNeedsPadding reports whether a code span's content has to be set
// apart from its fences by a space: when it begins or ends with a backtick,
// which would otherwise join the fence, or when it begins and ends with a
// space and is not all spaces, where CommonMark 6.1 strips one of each on
// re-read.
func codeSpanNeedsPadding(content string) bool {
	if content == "" {
		return false
	}
	if content[0] == '`' || content[len(content)-1] == '`' {
		return true
	}
	// The parser's "space" here is the space character, the tab, and the
	// line endings it turns into spaces first (CommonMark 6.1): a content of
	// spaces around a vertical tab is not all spaces to it, so one space is
	// stripped from each end on re-read unless a pad stands in for it, while
	// one of spaces around a tab or a bare carriage return is.
	return content[0] == ' ' && content[len(content)-1] == ' ' && strings.Trim(content, " \t\r\n") != ""
}

// isEmphasisDelimiter reports whether close is the closer of an emphasis or
// strong pair: a run of one character that CommonMark 6.2 pairs by flanking.
func isEmphasisDelimiter(close string) bool {
	return close != "" && (close[0] == '*' || close[0] == '_') && strings.Trim(close, close[:1]) == ""
}

// spellHeldClose writes the emphasis closer Close held back, now that next, the
// rune that will follow it, is known. Written as it was opened, the closer
// touches the inner pair's closer and the two form one run. One of the pairs
// is re-spelled with underscores instead: the inner one where an underscore
// can open and close in its place, otherwise the outer one, and neither when
// the bytes around them let neither pair, which leaves the run as it was.
//
// The check is CommonMark 6.2 applied to the bytes as they will stand: the
// re-spelled runs on their own, and the run that keeps its character beside
// an underscore, which is punctuation to the flanking rules.
func (s *mdInlineSink) spellHeldClose(next rune) {
	h := s.heldClose
	if h == nil {
		return
	}
	s.heldClose = nil
	out := s.sb.String()
	outer := mdClosedEmphasis{char: h.tag.close[0], openAt: h.tag.at, closeAt: len(out), n: len(h.tag.close)}
	inner := h.inner
	closer := h.tag.close
	if inner.n == 0 {
		if alt, ok := nestedRespelling(out, outer, next, h.enclosed); ok {
			out = respellDelimiters(out, outer, alt)
			outer.char = alt
			closer = strings.Repeat(string(alt), outer.n)
			s.sb.Reset()
			s.sb.WriteString(out)
		}
		s.closedPair(outer)
		s.sb.WriteString(closer)
		return
	}
	switch {
	case pairRespells(out, inner, outer, next, true):
		out = respellDelimiters(out, inner, '_')
		if n := len(s.pairs); n > 0 && s.pairs[n-1] == inner {
			s.pairs[n-1].char = '_'
		}
	case pairRespells(out, inner, outer, next, false):
		out = respellDelimiters(out, outer, '_')
		outer.char = '_'
		closer = strings.Repeat("_", outer.n)
	default:
		s.closedPair(outer)
		s.sb.WriteString(closer)
		return
	}
	s.sb.Reset()
	s.sb.WriteString(out)
	s.closedPair(outer)
	s.sb.WriteString(closer)
}

// nestedRespelling picks the other delimiter character for a pair nested in
// an open pair of the same character when its opener could close as well as
// open. CommonMark 6.2 processes closers left to right, so such an opener
// closes the enclosing pair first: "*!_!0_!*" spelled with asterisks
// throughout read back as an emphasis around the first "!" and literal text
// (#3229). The other character is used where it opens and closes in the same
// places; ok is false when no change is needed or the other character would
// not pair.
func nestedRespelling(out string, e mdClosedEmphasis, next rune, enclosed bool) (byte, bool) {
	if !enclosed {
		return 0, false
	}
	before, after := runeBefore(out, e.openAt), runeAfter(out, e.openAt+e.n)
	if _, closes := delimiterFlanking(e.char, before, after); !closes {
		return 0, false
	}
	alt := byte('_')
	if e.char == '_' {
		alt = '*'
	}
	altOpen, _ := delimiterFlanking(alt, before, after)
	_, altClose := delimiterFlanking(alt, runeBefore(out, e.closeAt), next)
	if !altOpen || !altClose {
		return 0, false
	}
	return alt, true
}

// pairRespells reports whether re-spelling one of two touching pairs with
// underscores lets both pair: the inner one when innerFirst is set, otherwise
// the outer one. out holds everything up to the outer closer, which is not
// written yet, and next is the rune that will follow that closer.
func pairRespells(out string, inner, outer mdClosedEmphasis, next rune, innerFirst bool) bool {
	innerChar, outerChar := inner.char, outer.char
	if innerFirst {
		innerChar = '_'
	} else {
		outerChar = '_'
	}
	// The inner opener and closer, with the outer closer right after the
	// inner one.
	innerOpen, _ := delimiterFlanking(innerChar, runeBefore(out, inner.openAt), runeAfter(out, inner.openAt+inner.n))
	_, innerClose := delimiterFlanking(innerChar, runeBefore(out, inner.closeAt), rune(outerChar))
	// The outer opener as it stands, and the outer closer between the inner
	// closer and whatever follows.
	outerOpen, _ := delimiterFlanking(outerChar, runeBefore(out, outer.openAt), runeAfter(out, outer.openAt+outer.n))
	_, outerClose := delimiterFlanking(outerChar, rune(innerChar), next)
	return innerOpen && innerClose && outerOpen && outerClose
}

// respellDelimiters rewrites both runs of pair e in out with char. A closer
// that is not written yet lies past the end of out and is the caller's to
// spell.
func respellDelimiters(out string, e mdClosedEmphasis, char byte) string {
	b := []byte(out)
	for i := e.openAt; i < e.openAt+e.n && i < len(b); i++ {
		b[i] = char
	}
	for i := e.closeAt; i < e.closeAt+e.n && i < len(b); i++ {
		b[i] = char
	}
	return string(b)
}

// firstRune returns the first rune of s, or a space for an empty string, which
// is what CommonMark 6.2 treats the start and the end of the text as.
func firstRune(s string) rune {
	if s == "" {
		return ' '
	}
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

func (s *mdInlineSink) Placeholder(r *model.PlaceholderRun) {
	if s.literal {
		s.sb.WriteString(r.Equiv)
		return
	}
	if s.title != nil {
		s.title.WriteString(r.Equiv)
		return
	}
	switch {
	case r.Type == "media:image" || r.Type == "link:image":
		s.spellHeldClose('!')
	case r.Equiv != "":
		s.spellHeldClose(firstRune(r.Equiv))
	}
	s.flushPending()
	if r.SubType == subTypeEntity && r.Data != "" {
		// A character reference is spelled as the source spelled it. Its
		// equivalent is the decoded character, and a block that is one
		// reference renders no text of its own, so writing the character
		// turned it into a text block on re-read.
		s.spellHeldBreak(r.Data)
		s.sb.WriteString(r.Data)
		return
	}
	s.spellHeldBreak(r.Equiv)
	switch r.Type {
	case "media:image", "link:image":
		// Self-closing image (e.g. read from HTML <img>): the alt text lives in
		// the run attributes, not as paired content.
		s.sb.WriteString("![" + r.Attr(model.AttrAlt) + mdLinkClose(r.Attrs, model.AttrSrc))
	case "struct:break":
		// A hard break's spelling is markup, so it rides the placeholder and
		// the newline it contributes is the text after it (#1661). On a line
		// whose only content is the break that leaves two newlines, and the
		// blank line ends the paragraph when the output is read back: "0\n\\\n0"
		// came back as two paragraphs (#2448). A backslash gives the line
		// content, and reads back as the same hard break. It is held until the
		// next run confirms a line follows, and a break that opens the block
		// spells nothing: the leading whitespace is trimmed, so there is no
		// blank line to prevent. The same goes for a break while the output
		// holds nothing but the newlines and indents of such breaks: a
		// backslash there survived the trim as the block's first line, the
		// pass after dropped it as a break opening the block, and the line
		// below it moved onto the first line, where a bare "* " is a bullet
		// (#3199).
		opens := trimBlockSpace(s.sb.String()) == ""
		s.openingBreak = opens
		s.heldBreak = !opens && s.atLineStart()
		if !opens && !s.heldBreak && (endsInBreakSpaces(s.sb.String()) || s.inLinkText() || s.inBareBracket() || s.lineIsBreakRuns()) {
			// Two or more spaces before a line ending ARE a hard break, so
			// emitting the break as a bare newline would hand the spaces to the
			// break's spelling and they would stop being content: "a  \\\nb"
			// came back as "a\\nb" on the pass after. A backslash spells the
			// break instead and leaves them where they are (#2529). Inside a
			// link's text the backslash is kept too: the text of a shortcut or
			// collapsed reference is its label, and a label spelled with the
			// break's backslash matches its definition only with it. The same
			// goes for text inside a bare bracket, which the pass after may
			// read as a label: "[", two backslash breaks and "]" matched no
			// definition, and with the first backslash dropped it matched one
			// whose label was a lone backslash. A line of delimiter runs keeps
			// it too: the closers of ">***0" over ">***" and a backslash
			// break are a thematic break once the backslash goes, and escaped
			// they close nothing. After a backslash the
			// break takes its two-space spelling: the parser reads a
			// backslash after an escaped one as text, line ending or not.
			switch {
			case strings.HasSuffix(s.sb.String(), "\\"):
				s.sb.WriteString("  ")
				s.breakSpaces = true
			case len(strings.Trim(r.Data, "\r")) >= 2 && strings.Trim(r.Data, " \t\r") == "" && (s.inLinkText() || s.inBareBracket()):
				// A label matches its definition by its spelling, so a
				// break of spaces inside one keeps its spaces: spelled with
				// a backslash, "[R  " over "]" matched no definition. A
				// tab or a bare carriage return among them is whitespace the
				// label's normalization drops, and a backslash in its place
				// was not.
				s.sb.WriteString(r.Data)
				s.breakSpaces = true
			default:
				s.sb.WriteByte('\\')
			}
			s.breakSpelled = true
		}
		s.sb.WriteString(r.Equiv)
	default:
		if r.Equiv != "" {
			s.sb.WriteString(r.Equiv)
		}
	}
}

// endsInBreakSpaces reports whether out ends in the run of two or more spaces
// or tabs that CommonMark 6.7 reads as a hard break before a line ending. A
// bare carriage return after them joins the line ending the newline spells,
// so the spaces before it are break spaces too.
func endsInBreakSpaces(out string) bool {
	i := len(out) - 1
	for i >= 0 && out[i] == '\r' {
		i--
	}
	n := 0
	for ; i >= 0 && (out[i] == ' ' || out[i] == '\t'); i-- {
		n++
	}
	return n >= 2
}

// atLineStart reports whether the output's current line carries no content
// yet: everything since the last newline is the continuation prefix a
// blockquote or a list item bakes into the runs (#1661), or nothing at all.
// The block's first line carries no such prefix, so a ">" there is content.
func (s *mdInlineSink) atLineStart() bool {
	return lineStartBefore(s.sb.String())
}

// lineStartBefore reports whether out ends on a line that carries no content
// yet, as atLineStart does for the sink's output.
func lineStartBefore(out string) bool {
	marker := false
	for i := len(out) - 1; i >= 0; i-- {
		switch out[i] {
		case '\n':
			return true
		case ' ', '\t', '\r':
		case '>':
			marker = true
		default:
			return false
		}
	}
	return !marker
}

// output returns the rendered Markdown with every literal emphasis delimiter
// escaped that would otherwise pair on the way back in.
//
// A '*' or '_' the reader handed over as TEXT is one goldmark left literal
// where the source stood. The rebuild path can put it somewhere else: an
// emphasis re-spelled from '_' to '*' writes a delimiter beside it, a dropped
// construct takes away what separated two of them, and text re-parses as
// markup. "_*__0_" is one block whose text is "_*_0"; rebuilt as "_*_*0*" it
// came back as two emphasis pairs around different content, and every further
// pass moved again (#2527).
//
// So a literal delimiter is escaped exactly when the OUTPUT would let it pair:
// its run can open or close per CommonMark 6.2, and a run of the same character
// that can answer it sits on the other side. A run that can do neither — the
// "*" of "2 * 3 * 4", an intraword "_" in "foo_bar_baz" — is left alone,
// because escaping it would spell a backslash into text that reads back the
// same without one. An escaped delimiter is never recorded, so the output is
// idempotent.
//
// A literal delimiter written directly beside markup of the same character
// is escaped whatever the merged run could do. "_* `00*0_" is one emphasis
// whose text begins with a literal "*"; spelled with asterisks, the opener and
// the literal formed a run of two followed by a space, which can neither open
// nor close, so the emphasis never opened on re-read and nothing was escaped
// because the merged run could not pair (#3191). Escaped, the literal leaves
// the markup a run of its own. Those escapes go in first, and the pairing
// question is asked of the output as it will stand, with an escaped delimiter
// no part of any run, which is how the reader sees it: asked of the merged
// runs, it gave the pass after a different answer.
//
// The emphasis and strong pairs the sink wrote are then read back through
// the reader's own parser: CommonMark 6.2 pairs delimiters by flanking, by
// the rule of three and in closer order, which the escapes above change too.
// A literal shortens the run it sat in once escaped, and "***0*0*0", a
// literal asterisk before two nested pairs, read back with no pair at all
// once the literal was escaped. So the escaped output is kept where it reads
// back as written, the unescaped output is kept where that one does, and
// otherwise a pair that reads back as another is re-spelled with the other
// character (respellMisreadEmphasis). Pairs with no spelling that reads back,
// which the rebuild's dropped constructs can leave behind, are written as the
// re-read would have them: literal delimiters, escaped where they could pair
// (degradeEmphasis), so the first pass already writes what the second would.
func (s *mdInlineSink) output() string {
	s.reinjectLinePrefix()
	s.foldHeadingLines()
	s.foldOutputBlankLines()
	s.escapeOutputLineMarkers()
	s.collapseDefinitionLookalikes()
	s.escapeOutputLeadingMarker()
	s.escapeFormedMarkup()
	// An angle escaped above can complete a definition's destination where
	// the bare one did not: "[0]:<A>0" is text, and "[0]:\<A>0" a definition
	// whose bracket the block path would escape, reference and all.
	s.collapseDefinitionLookalikes()
	raw := s.sb.String()
	frame := s.checkFrame(raw)
	out, pairs := raw, s.pairs
	var lit []int
	if len(s.litDelims) > 0 {
		out, lit, pairs = escapeDelimiters(out, s.litDelims, mergedDelimiterBytes(out, s.litDelims), pairs)
		out, lit, pairs = escapeDelimiters(out, lit, pairingDelimiterBytes(out), pairs)
	}
	// Literal tildes with no strikethrough beside them can still pair with each
	// other on re-read, so they are checked against the parser as the pairs
	// are.
	if len(pairs) == 0 && len(literalTildes(out, lit)) == 0 {
		return out
	}
	if ok, known, parsed := emphasisReadsBackParsed(out, pairs, frame); !s.markupMarker && (ok || (!known && !parsed)) {
		// A verdict the parser could not give because the output is no
		// single paragraph leaves the output as written. One it could not
		// give because a pair it formed cannot be located is a pairing the
		// sink does not understand, and the pairs are degraded to text below.
		return out
	}
	if out != raw && !s.markupMarker {
		if ok, known := emphasisReadsBack(raw, s.pairs, frame); ok && known {
			return raw
		}
	}
	// A literal tilde pairs with strikethrough markup whatever its run's
	// length, so the literals are escaped before the pairs are re-spelled.
	if tildes := literalTildes(out, lit); len(tildes) > 0 {
		escaped, _, escapedPairs := escapeDelimiters(out, tildes, allOffsets(tildes), pairs)
		if ok, known := emphasisReadsBack(escaped, escapedPairs, frame); ok && known {
			return escaped
		}
	}
	if fixed, ok := respellMisreadEmphasis(out, lit, pairs, frame, s.delimiterConflicts); ok {
		return fixed
	}
	return escapeFormedEmphasis(degradeEmphasis(out, pairs), frame)
}

// escapeFormedEmphasis escapes the delimiters of every emphasis and
// strikethrough the parser still reads from out, which holds no markup any
// more: the pairs were degraded to text, and a literal tilde the degrade left
// bare pairs with a degraded run of any length. "*~!~~0~*~~!~" was degraded
// to text whose first tilde and last two still read as a strikethrough, and
// the pass after escaped them. An escape can let a later run pair instead, so
// the pass repeats until nothing forms.
func escapeFormedEmphasis(out string, frame blockFrame) string {
	for range 8 {
		para, src, shift, ok := parseInlineBlock(out, frame)
		if !ok {
			return out
		}
		var offsets []int
		_ = ast.Walk(para, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			var openLen, closeLen int
			switch {
			case n.Kind() == ast.KindEmphasis:
				openLen = n.(*ast.Emphasis).Level
				closeLen = openLen
			case n.Kind() == east.KindStrikethrough:
				open, close := strikethroughFences(n, src)
				openLen, closeLen = len(open), len(close)
			default:
				return ast.WalkContinue, nil
			}
			start, okStart := inlineContentStart(n, src)
			if !okStart {
				start, okStart = inlineNodeStart(n, src)
			}
			end, okEnd := inlineNodeEnd(n, src)
			if !okStart || !okEnd {
				return ast.WalkContinue, nil
			}
			for i := range openLen {
				offsets = append(offsets, start-shift+i)
			}
			for i := range closeLen {
				offsets = append(offsets, end-closeLen-shift+i)
			}
			return ast.WalkContinue, nil
		})
		if len(offsets) == 0 {
			return out
		}
		slices.Sort(offsets)
		out, _, _ = escapeDelimiters(out, offsets, allOffsets(offsets), nil)
	}
	return out
}

// foldHeadingLines folds a heading's line breaks to spaces as
// singleLineHeading will, so the checks that follow see the line the heading
// will be: "<p", a tab and a line ending, then "0", is a paragraph as it
// stands and an HTML block once folded to "<p 0", and the angle went
// unescaped on the pass that saw the paragraph. The offsets the sink keeps
// move with the folded bytes.
func (s *mdInlineSink) foldHeadingLines() {
	out := s.sb.String()
	if !s.heading || !strings.Contains(out, "\n") {
		return
	}
	spans := headingLineBreakSpans(out)
	s.sb.Reset()
	s.sb.WriteString(foldSpans(out, spans))
	s.remapOffsets(func(off int) int {
		removed := 0
		for _, sp := range spans {
			switch {
			case off >= sp[1]:
				removed += sp[1] - sp[0] - 1
			case off > sp[0]:
				return sp[0] - removed
			}
		}
		return off - removed
	})
}

// reinjectLinePrefix puts the prefix the block's hard-break continuation lines
// shared, indentation or quote markers, back after every line ending, so the
// lines keep their shape and the checks that follow see them as the block
// will have them: "0  ", a break and "    #" is one paragraph whose "#" sits
// four columns in, where it opens nothing. Written at the line's start it was
// escaped there, which a pass that kept the indentation in its text did not
// do. The offsets the sink keeps move with the inserted bytes.
func (s *mdInlineSink) reinjectLinePrefix() {
	out := s.sb.String()
	if s.linePrefix == "" || !strings.Contains(out, "\n") {
		return
	}
	var newlines []int
	for i := range len(out) {
		if out[i] == '\n' {
			newlines = append(newlines, i)
		}
	}
	s.sb.Reset()
	s.sb.WriteString(strings.ReplaceAll(out, "\n", "\n"+s.linePrefix))
	width := len(s.linePrefix)
	s.remapOffsets(func(off int) int { return off + width*countAtMost(newlines, off-1) })
}

// foldOutputBlankLines drops the blank lines from the output before it is
// checked against the parser, as writeBlockMarkdown drops them from the block,
// so the checks see the lines the block will have. "<?0>~00~~" followed by a
// blank line and "0" is one HTML block whose text holds the blank line;
// checked with it in place the text was two paragraphs, no check applied, and
// the literal tildes went unescaped to pair on the pass after. In a quote
// body a continuation line holding nothing but its marker goes the same way
// (foldQuoteMarkerLines): checked with such a line in place the quote held
// two paragraphs and the pairs went unverified. A line prefix that is not a
// quote's says a marker on its line is content, which the block path escapes
// instead. The offsets the sink keeps move with the removed bytes.
func (s *mdInlineSink) foldOutputBlankLines() {
	out := s.sb.String()
	if !strings.Contains(out, "\n") {
		return
	}
	levels := 0
	if s.linePrefix == "" || isQuoteLinePrefix(s.linePrefix) {
		levels = s.checkLevels(out)
	}
	lines := strings.Split(out, "\n")
	type span struct{ start, end int }
	var removed []span
	kept := lines[:0]
	pos := 0
	for i, line := range lines {
		next := pos + len(line) + 1
		blank := isBlankLine(line)
		if !blank && levels > 0 && i > 0 {
			prefix := blockquoteMarkerPrefixUpTo(line, levels)
			blank = prefix != "" && isBlankLine(line[len(prefix):])
		}
		if blank {
			if i < len(lines)-1 {
				removed = append(removed, span{pos, next})
			} else {
				removed = append(removed, span{max(pos-1, 0), pos + len(line)})
			}
		} else {
			kept = append(kept, line)
		}
		pos = next
	}
	if len(removed) == 0 {
		return
	}
	folded := strings.Join(kept, "\n")
	remap := func(off int) int {
		n := 0
		for _, r := range removed {
			if off >= r.end {
				n += r.end - r.start
			} else if off > r.start {
				return r.start - n
			}
		}
		return off - n
	}
	s.sb.Reset()
	s.sb.WriteString(folded)
	s.remapOffsets(remap)
}

// remapOffsets moves every offset the sink keeps through remap, after the
// output's bytes have moved.
func (s *mdInlineSink) remapOffsets(remap func(int) int) {
	for i, off := range s.litDelims {
		s.litDelims[i] = remap(off)
	}
	for i, off := range s.bareAngles {
		s.bareAngles[i] = remap(off)
	}
	for i, off := range s.bareTicks {
		s.bareTicks[i] = remap(off)
	}
	for i, off := range s.bareBrackets {
		s.bareBrackets[i] = remap(off)
	}
	for i, off := range s.bareAmps {
		s.bareAmps[i] = remap(off)
	}
	for i := range s.pairs {
		s.pairs[i].openAt = remap(s.pairs[i].openAt)
		s.pairs[i].closeAt = remap(s.pairs[i].closeAt)
	}
	for i := range s.shortcutRefs {
		s.shortcutRefs[i].openAt = remap(s.shortcutRefs[i].openAt)
		s.shortcutRefs[i].closeAt = remap(s.shortcutRefs[i].closeAt)
	}
	if s.markupEnd > 0 {
		s.markupEnd = remap(s.markupEnd)
	}
}

// isMarkupDelimiter reports whether the byte at off is one of the emphasis
// or strikethrough delimiters the sink wrote, closed or still open.
func (s *mdInlineSink) isMarkupDelimiter(off int) bool {
	for _, p := range s.pairs {
		if (off >= p.openAt && off < p.openAt+p.n) || (off >= p.closeAt && off < p.closeAt+p.n) {
			return true
		}
	}
	for _, tag := range s.open {
		if tag.close != "" && off >= tag.at && off < tag.at+len(tag.close) {
			return true
		}
	}
	return false
}

// escapeOutputLeadingMarker escapes a block marker that opens the output's
// first line, after the whitespace the block path trims, the way
// writeBlockMarkdown will, before the literal delimiters are weighed: the
// escape changes what the run after it can do. "<A>***" over "*0*" is text
// "***", a line ending and an emphasis; the asterisk run could pair with
// nothing as written and stood, the block path escaped the first asterisk as
// a thematic break, and the two left behind closed on the pass after.
func (s *mdInlineSink) escapeOutputLeadingMarker() {
	if !s.leadingMarker {
		return
	}
	out := s.sb.String()
	lead := len(out) - len(strings.TrimLeft(out, " \t\r\n"))
	escaped := escapeLeadingBlockMarker(out[lead:])
	if escaped == out[lead:] {
		return
	}
	at := lead
	for at-lead < len(escaped) && out[at] == escaped[at-lead] {
		at++
	}
	s.sb.Reset()
	s.sb.WriteString(out[:at])
	s.sb.WriteByte('\\')
	s.sb.WriteString(out[at:])
	s.remapOffsets(func(off int) int {
		if off >= at {
			return off + 1
		}
		return off
	})
}

// escapeOutputLineMarkers escapes a block marker that opens a continuation
// line of the output the way writeBlockMarkdown will, before the output is
// checked against the parser, so the checks see a paragraph where the block
// will be one. "<?0>~0~~" and a line holding "#" is one HTML block; checked as
// written, the "#" line was a heading, the text was no single paragraph, no
// check applied, and the literal tildes went unescaped to pair on the pass
// after. A marker that is a delimiter the sink wrote is left to the emphasis
// checks, which then re-spell the pair: the closer of "*" at the start of
// "* 00*" escaped here read as text, and the pass after had no pair to find.
func (s *mdInlineSink) escapeOutputLineMarkers() {
	out := s.sb.String()
	var inserted []int
	for _, pt := range s.lineMarkerPoints(out) {
		if s.isMarkupDelimiter(pt) {
			s.markupMarker = true
			continue
		}
		inserted = append(inserted, pt)
	}
	if len(inserted) == 0 {
		return
	}
	var buf strings.Builder
	prev := 0
	for _, off := range inserted {
		buf.WriteString(out[prev:off])
		buf.WriteByte('\\')
		prev = off
	}
	buf.WriteString(out[prev:])
	s.sb.Reset()
	s.sb.WriteString(buf.String())
	s.remapOffsets(func(off int) int { return off + countAtMost(inserted, off) })
}

// lineMarkerPoints returns, in ascending order, the offsets in out at which
// writeBlockMarkdown will escape a block marker or an underline that opens a
// continuation line. Inside a blockquote the lines that carry every marker
// are the block's own and get the interrupting-marker rule, and a line with
// fewer markers than the quote has levels is lazy for the levels it lacks and
// is escaped on any marker, as escapeLazyQuoteLines does. A block with no
// marker property may still be a quote, which writeBlockMarkdown recovers
// from the markers every one of its continuation lines carries; the quote
// markers of such a block are left alone, and a ">" that opens a
// continuation line of any other block is content. A list item's
// continuation lines are escaped on any marker after the item's content
// column, as escapeItemMarkerLines does: "x" over a bare "*" was written
// unescaped, and the convergence re-read saw an item and an empty one and
// took the item's text without its second line.
func (s *mdInlineSink) lineMarkerPoints(out string) []int {
	if !strings.Contains(out, "\n") {
		return nil
	}
	lines := strings.Split(out, "\n")
	recovered := s.quoteLevels == 0 && !s.rawText && continuationBlockquoteMarker(out) != ""
	var points []int
	pos := len(lines[0]) + 1
	for _, line := range lines[1:] {
		var escaped string
		if s.quoteLevels > 0 {
			prefix := blockquoteMarkerPrefixUpTo(line, s.quoteLevels)
			if strings.Count(prefix, ">") < s.quoteLevels {
				escaped = line[:len(prefix)] + escapeContinuationLineMarkerAt(line[len(prefix):], columnWidth(prefix))
			} else {
				escaped = line[:len(prefix)] + escapeInterruptingBlockMarkerAt(line[len(prefix):], columnWidth(prefix))
			}
		} else if s.listItem {
			escaped = escapeItemMarkerLine(line, len("- "))
		} else {
			escaped = escapeInterruptingBlockMarker(line)
		}
		if escaped != line {
			at := 0
			for at < len(line) && line[at] == escaped[at] {
				at++
			}
			if at < len(line) && (line[at] != '>' || !recovered) {
				points = append(points, pos+at)
			}
		} else if rest := line[len(blockquoteMarkerPrefixUpTo(line, s.quoteLevels)):]; isSetextBar(rest) || isTableDelimiterRow(rest) {
			if j := firstBarChar(rest); j >= 0 {
				points = append(points, pos+len(line)-len(rest)+j)
			}
		}
		pos += len(line) + 1
	}
	return points
}

// delimiterConflicts reports whether a block marker writeBlockMarkdown will
// escape in out sits on a delimiter of one of pairs.
func (s *mdInlineSink) delimiterConflicts(out string, pairs []mdClosedEmphasis) bool {
	for _, pt := range s.lineMarkerPoints(out) {
		for _, p := range pairs {
			if (pt >= p.openAt && pt < p.openAt+p.n) || (pt >= p.closeAt && pt < p.closeAt+p.n) {
				return true
			}
		}
	}
	return false
}

// entityAtStartRE is htmlEntityRE anchored to the start of the text.
var entityAtStartRE = regexp.MustCompile(`^&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]+|#[xX][0-9A-Fa-f]+);`)

// collapseDefinitionLookalikes spells a shortcut reference link that opens a
// line as a collapsed one, "[label][]", where the line as written reads as a
// link reference definition. A reference to a definition is what makes the
// definition's label a block of its own, and escaping the bracket instead
// would lose that block. A definition whose destination line is "[R]:0",
// followed by a line of two single quotes and a digit, is that definition and
// a paragraph that opens with a reference to it, and written as "[R]:0" the
// paragraph was a second definition.
func (s *mdInlineSink) collapseDefinitionLookalikes() {
	for _, ref := range slices.Backward(s.shortcutRefs) {
		out := s.sb.String()
		// A definition opens a block, never a continuation line, which a
		// definition cannot interrupt a paragraph from.
		if ref.openAt >= len(out) || strings.Contains(out[:ref.openAt], "\n") || !lineStartBefore(out[:ref.openAt]) || !readsAsLinkReferenceDefinition(out[ref.openAt:]) {
			continue
		}
		at := ref.closeAt + 1
		s.sb.Reset()
		s.sb.WriteString(out[:at])
		s.sb.WriteString("[]")
		s.sb.WriteString(out[at:])
		shift := func(off int) int {
			if off >= at {
				return off + 2
			}
			return off
		}
		for i, off := range s.litDelims {
			s.litDelims[i] = shift(off)
		}
		for i, off := range s.bareAngles {
			s.bareAngles[i] = shift(off)
		}
		for i, off := range s.bareTicks {
			s.bareTicks[i] = shift(off)
		}
		for i, off := range s.bareBrackets {
			s.bareBrackets[i] = shift(off)
		}
		for i, off := range s.bareAmps {
			s.bareAmps[i] = shift(off)
		}
		for i := range s.pairs {
			s.pairs[i].openAt = shift(s.pairs[i].openAt)
			s.pairs[i].closeAt = shift(s.pairs[i].closeAt)
		}
		if s.markupEnd >= at {
			s.markupEnd += 2
		}
	}
}

// escapeFormedMarkup escapes every recorded literal '<' from which the parser
// reads a tag or an autolink in the output as it stands, and every recorded
// literal '`' in the opening run of a code span it reads, and every one of
// them when the output is not a single paragraph, which is where a '<' can
// open an HTML block. A recorded literal '[' is escaped where the text after
// it, across the runs that were dropped between, completes a link. An escape
// can let a later run pair instead, so the pass repeats until nothing forms.
// The offsets the sink keeps move with the inserted backslashes.
func (s *mdInlineSink) escapeFormedMarkup() {
	for range 8 {
		if len(s.bareAngles) == 0 && len(s.bareTicks) == 0 && len(s.bareBrackets) == 0 && len(s.bareAmps) == 0 {
			return
		}
		out := s.sb.String()
		formed := formedMarkup(out, s.checkFrame(out), s.bareAngles, s.bareTicks, s.bareBrackets)
		for _, off := range s.bareBrackets {
			if off >= len(out) || slices.Contains(formed, off) {
				continue
			}
			if angle, ok := opensLinkTextToEscape(out[off:]); ok && slices.Contains(formed, off+angle) {
				formed = append(formed, off)
			}
		}
		for i, off := range formed {
			formed[i] = bracketEscapePoint(out, off)
		}
		for _, off := range s.bareAmps {
			// The reader reads a character reference by its shape, whatever
			// the name: "&A" and ";" from two runs made "&A;", a block of one
			// reference, which is no block at all.
			if off < len(out) && entityAtStartRE.MatchString(out[off:]) {
				formed = append(formed, off)
			}
		}
		slices.Sort(formed)
		if len(formed) == 0 {
			return
		}
		var buf strings.Builder
		prev := 0
		for _, off := range formed {
			buf.WriteString(out[prev:off])
			buf.WriteByte('\\')
			prev = off
		}
		buf.WriteString(out[prev:])
		s.sb.Reset()
		s.sb.WriteString(buf.String())
		escaped := allOffsets(formed)
		shift := func(off int) int { return off + countAtMost(formed, off) }
		for i, off := range s.litDelims {
			s.litDelims[i] = shift(off)
		}
		for i := range s.pairs {
			s.pairs[i].openAt = shift(s.pairs[i].openAt)
			s.pairs[i].closeAt = shift(s.pairs[i].closeAt)
		}
		s.bareAngles = shiftUnescaped(s.bareAngles, escaped, shift)
		s.bareTicks = shiftUnescaped(s.bareTicks, escaped, shift)
		s.bareBrackets = shiftUnescaped(s.bareBrackets, escaped, shift)
		s.bareAmps = shiftUnescaped(s.bareAmps, escaped, shift)
	}
}

// bracketEscapePoint returns where the escape goes for the bracket at off:
// before the bracket, or before the "]" that closes it when an unescaped "["
// stands before it with no bracket between, where an escaped bracket would
// make the span from that "[" a link label; a label may span a line ending,
// so the search runs back across one. "[R][[](" is a shortcut reference
// followed by text, and with the inner bracket escaped, "[R][\[](", the
// "[\[]" after the reference was a label, which a shortcut reference may not
// be followed by. Any other offset is returned as it is.
func bracketEscapePoint(out string, off int) int {
	if off <= 0 || off >= len(out) || out[off] != '[' {
		return off
	}
	opened := false
	for k := off - 1; k >= 0; k-- {
		c := out[k]
		if (c == '[' || c == ']') && !backslashEscaped(out, k) {
			opened = c == '['
			break
		}
	}
	if !opened {
		return off
	}
	label, _, ok := cutLinkLabel(out[off:])
	if !ok {
		return off
	}
	return off + 1 + len(label)
}

// blockFrame is the block the parser checks read the output inside: how many
// blockquotes deep it sits, and whether it is a list item's text. The checks
// prefix the output with the markers the block path will write, so a line
// reads where it will stand: a tab and "<!A" are four columns in at a line's
// start, text, and two columns into an item written after "- ", where they
// open an HTML block.
type blockFrame struct {
	quoteLevels int
	listItem    bool
}

// parseInlineBlock parses out as the block it will be written in: trimmed of
// the whitespace around it, as the block path trims it, and inside the
// blockquotes and the list item of frame, whose markers the text carries
// after each line ending and the block's prefix supplies for the first line.
// It returns the paragraph the text forms, the bytes it was parsed from and
// the shift to add to an offset in out to find it in those bytes, or ok false
// when the text forms anything else: a form feed before "<!A0" kept it a
// paragraph here and the trim left it an HTML block.
func parseInlineBlock(out string, frame blockFrame) (para ast.Node, src []byte, shift int, ok bool) {
	prefix := strings.Repeat("> ", frame.quoteLevels)
	if frame.listItem {
		prefix += "- "
	}
	lead := len(out) - len(strings.TrimLeft(out, " \t\r\n"))
	src = []byte(prefix + trimBlockSpace(out))
	shift = len(prefix) - lead
	node := parseMarkdown(src)
	for range frame.quoteLevels {
		if node.ChildCount() != 1 {
			return nil, nil, 0, false
		}
		q, isQuote := node.FirstChild().(*ast.Blockquote)
		if !isQuote {
			return nil, nil, 0, false
		}
		node = q
	}
	if frame.listItem {
		if node.ChildCount() != 1 {
			return nil, nil, 0, false
		}
		list, isList := node.FirstChild().(*ast.List)
		if !isList || list.ChildCount() != 1 {
			return nil, nil, 0, false
		}
		node = list.FirstChild()
	}
	if node.ChildCount() != 1 {
		return nil, nil, 0, false
	}
	// A tight list item's text is a text block to the parser, a paragraph
	// in all but name.
	switch p := node.FirstChild().(type) {
	case *ast.Paragraph:
		return p, src, shift, true
	case *ast.TextBlock:
		return p, src, shift, true
	}
	return nil, nil, 0, false
}

// shiftUnescaped drops the offsets just escaped and moves the rest.
func shiftUnescaped(offsets []int, escaped map[int]bool, shift func(int) int) []int {
	var kept []int
	for _, off := range offsets {
		if escaped[off] {
			continue
		}
		kept = append(kept, shift(off))
	}
	return kept
}

// formedMarkup returns, in ascending order, the offsets in angles at which the
// parser reads a tag or an autolink from out, the offsets in ticks that open a
// code span it reads and the offsets in brackets that open a link or an image
// it reads, or all of the angles and ticks when out is not a single paragraph.
func formedMarkup(out string, frame blockFrame, angles, ticks, brackets []int) []int {
	para, src, shift, ok := parseInlineBlock(out, frame)
	if !ok {
		all := slices.Concat(angles, ticks)
		slices.Sort(all)
		return all
	}
	starts := map[int]bool{}
	_ = ast.Walk(para, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			// The link's bracket is where the node starts; the reader handed
			// the bracket over as text, so what closed it into a link arrived
			// in a later run or with a hard break spelled as a soft one.
			if at, ok := inlineNodeStart(v, src); ok {
				starts[at-shift] = true
			}
		case *ast.Image:
			if at, ok := inlineNodeStart(v, src); ok {
				starts[at-shift+1] = true
			}
		case *ast.RawHTML:
			if v.Segments != nil && v.Segments.Len() > 0 {
				starts[v.Segments.At(0).Start-shift] = true
			}
		case *ast.AutoLink:
			if at, ok := inlineNodeStart(v, src); ok {
				starts[at-shift] = true
			}
		case *ast.CodeSpan:
			if at, ok := inlineContentStart(v, src); ok {
				open, _ := codeSpanFences(v, src)
				for i := range len(open) {
					starts[at-shift+i] = true
				}
			}
		}
		return ast.WalkContinue, nil
	})
	var formed []int
	for _, off := range slices.Concat(angles, ticks, brackets) {
		if starts[off] {
			formed = append(formed, off)
		}
	}
	slices.Sort(formed)
	return formed
}

// literalTildes returns the offsets in lit that hold a tilde in out.
func literalTildes(out string, lit []int) []int {
	var tildes []int
	for _, off := range lit {
		if off < len(out) && out[off] == '~' {
			tildes = append(tildes, off)
		}
	}
	return tildes
}

// allOffsets marks every offset given.
func allOffsets(offsets []int) map[int]bool {
	m := make(map[int]bool, len(offsets))
	for _, off := range offsets {
		m[off] = true
	}
	return m
}

// degradeEmphasis writes the delimiters of pairs as the literal text they
// will be on re-read: escaped where their runs could pair, left alone where
// they cannot, which is how output treats a delimiter the reader handed over
// as text.
func degradeEmphasis(out string, pairs []mdClosedEmphasis) string {
	var lit []int
	for _, p := range pairs {
		for i := p.openAt; i < p.openAt+p.n; i++ {
			lit = append(lit, i)
		}
		for i := p.closeAt; i < p.closeAt+p.n; i++ {
			lit = append(lit, i)
		}
	}
	slices.Sort(lit)
	which := pairingDelimiterBytes(out)
	for _, p := range pairs {
		if p.char != '~' {
			continue
		}
		if which == nil {
			which = map[int]bool{}
		}
		for i := p.openAt; i < p.openAt+p.n; i++ {
			which[i] = true
		}
		for i := p.closeAt; i < p.closeAt+p.n; i++ {
			which[i] = true
		}
	}
	out, _, _ = escapeDelimiters(out, lit, which, nil)
	return out
}

// escapeDelimiters writes a backslash before each byte of out whose offset is
// in lit and in which, and returns the result with the offsets of the other
// literal delimiters and of the emphasis pairs moved to where they now sit.
func escapeDelimiters(out string, lit []int, which map[int]bool, pairs []mdClosedEmphasis) (string, []int, []mdClosedEmphasis) {
	if len(which) == 0 {
		return out, lit, pairs
	}
	var buf strings.Builder
	var rest, inserted []int
	prev, n := 0, 0
	for _, off := range lit {
		if !which[off] || backslashEscaped(out, off) {
			rest = append(rest, off+n)
			continue
		}
		buf.WriteString(out[prev:off])
		buf.WriteByte('\\')
		inserted = append(inserted, off)
		prev, n = off, n+1
	}
	if n == 0 {
		return out, lit, pairs
	}
	buf.WriteString(out[prev:])
	shifted := make([]mdClosedEmphasis, len(pairs))
	for i, p := range pairs {
		p.openAt += countAtMost(inserted, p.openAt)
		p.closeAt += countAtMost(inserted, p.closeAt)
		shifted[i] = p
	}
	return buf.String(), rest, shifted
}

// countAtMost returns how many of the ascending offsets are at most off: the
// number of backslashes inserted before the byte that sat at off.
func countAtMost(offsets []int, off int) int {
	n := 0
	for _, o := range offsets {
		if o > off {
			break
		}
		n++
	}
	return n
}

// respellMisreadEmphasis returns out with its emphasis pairs spelled so that
// the reader's parser pairs them as written: the pairs' characters are
// flipped, fewest first, until a spelling reads back. The re-spelled runs may
// let a literal delimiter (one of the offsets in lit) pair where it could not
// before, which the pass after would escape, so each spelling that reads back
// is weighed with its literals escaped too, and the first that reads back
// both ways is taken; failing that, the first that reads back as written. A
// spelling conflicts reports, one with a block marker on a delimiter, is not
// taken. ok is false when none does.
func respellMisreadEmphasis(out string, lit []int, pairs []mdClosedEmphasis, frame blockFrame, conflicts func(string, []mdClosedEmphasis) bool) (string, bool) {
	k := min(len(pairs), 6)
	masks := make([]int, 0, 1<<k)
	for m := 1; m < 1<<k; m++ {
		masks = append(masks, m)
	}
	slices.SortStableFunc(masks, func(a, b int) int { return bits.OnesCount(uint(a)) - bits.OnesCount(uint(b)) })
	first := ""
	for _, m := range masks {
		cand, candPairs := flipEmphasisPairs(out, pairs, m)
		if conflicts(cand, candPairs) {
			// A block marker the block path will escape sits on one of the
			// re-spelled delimiters.
			continue
		}
		escaped, _, escapedPairs := escapeDelimiters(cand, lit, pairingDelimiterBytes(cand), candPairs)
		if escaped != cand {
			// The pass after would escape these literals, so the spelling
			// is judged with them escaped.
			if ok, known := emphasisReadsBack(escaped, escapedPairs, frame); ok && known {
				return escaped, true
			}
		}
		if ok, known := emphasisReadsBack(cand, candPairs, frame); ok && known {
			if escaped == cand {
				return cand, true
			}
			if first == "" {
				first = cand
			}
		}
	}
	if first != "" {
		return first, true
	}
	return out, false
}

// flipEmphasisPairs rewrites the pairs mask selects with the other delimiter
// character.
func flipEmphasisPairs(out string, pairs []mdClosedEmphasis, mask int) (string, []mdClosedEmphasis) {
	b := []byte(out)
	res := slices.Clone(pairs)
	for i := range res {
		if mask&(1<<i) == 0 || res[i].char == '~' {
			continue
		}
		alt := byte('_')
		if res[i].char == '_' {
			alt = '*'
		}
		for j := res[i].openAt; j < res[i].openAt+res[i].n && j < len(b); j++ {
			b[j] = alt
		}
		for j := res[i].closeAt; j < res[i].closeAt+res[i].n && j < len(b); j++ {
			b[j] = alt
		}
		res[i].char = alt
	}
	return string(b), res
}

var (
	errEmphasisUnlocated = errors.New("emphasis node cannot be located")
	errEmphasisMismatch  = errors.New("emphasis pairs differ")
)

// emphasisReadsBack reports whether out, read by the reader's parser, holds
// exactly the emphasis pairs given, each with its delimiters at the offsets
// recorded. known is false when the comparison cannot be made: the output is
// not a single paragraph, or a node's position cannot be resolved.
//
// An output that opens with a block marker is read the way the block path
// will write it, with the marker escaped: "* *0*0*" is a list item as it
// stands, and the escape the block path adds leaves the pair's own opener as
// text.
func emphasisReadsBack(out string, pairs []mdClosedEmphasis, frame blockFrame) (ok, known bool) {
	ok, known, _ = emphasisReadsBackParsed(out, pairs, frame)
	return ok, known
}

// emphasisReadsBackParsed is emphasisReadsBack, reporting as well whether the
// output parsed as the single paragraph the comparison needs: known is false
// without that, and false with it when a pair the parser formed cannot be
// located in the output.
func emphasisReadsBackParsed(out string, pairs []mdClosedEmphasis, frame blockFrame) (ok, known, parsed bool) {
	ok, known, parsed = emphasisReadsBackAsWritten(out, pairs, frame)
	if known {
		return ok, true, parsed
	}
	// The block path trims the output before it escapes a leading marker, so
	// a marker after leading whitespace is a marker here too.
	lead := len(out) - len(strings.TrimLeft(out, " \t\r\n"))
	escaped := out[:lead] + escapeLeadingBlockMarker(out[lead:])
	if escaped == out {
		return false, false, parsed
	}
	at := 0
	for at < len(out) && out[at] == escaped[at] {
		at++
	}
	shifted := slices.Clone(pairs)
	for i := range shifted {
		if shifted[i].openAt >= at {
			shifted[i].openAt++
		}
		if shifted[i].closeAt >= at {
			shifted[i].closeAt++
		}
	}
	ok, known, parsedEscaped := emphasisReadsBackAsWritten(escaped, shifted, frame)
	return ok, known, parsed || parsedEscaped
}

// emphasisReadsBackAsWritten is emphasisReadsBack on out as it stands, read
// inside the block of frame; parsed reports whether out parsed as a single
// paragraph.
func emphasisReadsBackAsWritten(out string, pairs []mdClosedEmphasis, frame blockFrame) (ok, known, parsed bool) {
	para, src, shift, ok := parseInlineBlock(out, frame)
	if !ok {
		return false, false, false
	}
	want := make(map[mdClosedEmphasis]int, len(pairs))
	for _, p := range pairs {
		want[p]++
	}
	got := 0
	err := ast.Walk(para, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var openLen, closeLen int
		switch {
		case n.Kind() == ast.KindEmphasis:
			openLen = n.(*ast.Emphasis).Level
			closeLen = openLen
		case n.Kind() == east.KindStrikethrough:
			open, close := strikethroughFences(n, src)
			openLen, closeLen = len(open), len(close)
		default:
			return ast.WalkContinue, nil
		}
		openAt, okStart := inlineContentStart(n, src)
		if !okStart {
			// A pair whose first child records no position of its own, an
			// empty link say, is located from its neighbours instead.
			openAt, okStart = inlineNodeStart(n, src)
		}
		end, okEnd := inlineNodeEnd(n, src)
		if !okStart || !okEnd || openAt < 0 || openAt >= len(src) {
			return ast.WalkStop, errEmphasisUnlocated
		}
		p := mdClosedEmphasis{char: src[openAt], openAt: openAt - shift, closeAt: end - closeLen - shift, n: openLen}
		if want[p] == 0 {
			return ast.WalkStop, errEmphasisMismatch
		}
		want[p]--
		got++
		return ast.WalkContinue, nil
	})
	switch {
	case errors.Is(err, errEmphasisUnlocated):
		return false, false, true
	case err != nil:
		return false, true, true
	}
	return got == len(pairs), true, true
}

// delimRun is one maximal run of the same emphasis delimiter character in the
// rendered output, with what CommonMark 6.2 lets it do there.
type delimRun struct {
	char       byte
	start, end int
	canOpen    bool
	canClose   bool
}

// pairingDelimiterBytes reports, for every byte offset in out, whether that
// byte belongs to a delimiter run that could pair with another run of the same
// character: one that can open sits before one that can close.
func pairingDelimiterBytes(out string) map[int]bool {
	runs := scanDelimiterRuns(out)
	pairs := map[byte]bool{}
	for _, c := range []byte{'*', '_'} {
		opened := false
		for _, r := range runs {
			if r.char != c {
				continue
			}
			if opened && r.canClose {
				pairs[c] = true
				break
			}
			if r.canOpen {
				opened = true
			}
		}
	}
	if len(pairs) == 0 {
		return nil
	}
	marked := map[int]bool{}
	for _, r := range runs {
		if !pairs[r.char] || (!r.canOpen && !r.canClose) {
			continue
		}
		for i := r.start; i < r.end; i++ {
			marked[i] = true
		}
	}
	return marked
}

// mergedDelimiterBytes reports the literal delimiter bytes (those at the
// offsets in lit) that sit in a run holding markup delimiters too: a run the
// reader would see as one where the sink meant two things.
func mergedDelimiterBytes(out string, lit []int) map[int]bool {
	literal := make(map[int]bool, len(lit))
	for _, off := range lit {
		literal[off] = true
	}
	var merged map[int]bool
	for _, r := range scanDelimiterRuns(out) {
		n := 0
		for i := r.start; i < r.end; i++ {
			if literal[i] {
				n++
			}
		}
		if n == 0 || n == r.end-r.start {
			continue
		}
		if merged == nil {
			merged = map[int]bool{}
		}
		for i := r.start; i < r.end; i++ {
			if literal[i] {
				merged[i] = true
			}
		}
	}
	return merged
}

// scanDelimiterRuns finds the maximal runs of '*' and '_' in out and applies
// CommonMark 6.2: a run is left-flanking when it is not followed by whitespace
// and either is not followed by punctuation or is preceded by whitespace or
// punctuation, right-flanking is the mirror of that, and '_' additionally may
// open or close only where it is not intraword. A backslash-escaped delimiter
// is text (CommonMark 2.4): it belongs to no run and ends the one before it.
func scanDelimiterRuns(out string) []delimRun {
	var runs []delimRun
	for i := 0; i < len(out); {
		c := out[i]
		if (c != '*' && c != '_') || backslashEscaped(out, i) {
			i++
			continue
		}
		j := i
		for j < len(out) && out[j] == c && !backslashEscaped(out, j) {
			j++
		}
		r := delimRun{char: c, start: i, end: j}
		r.canOpen, r.canClose = delimiterFlanking(c, runeBefore(out, i), runeAfter(out, j))
		runs = append(runs, r)
		i = j
	}
	return runs
}

// delimiterFlanking applies CommonMark 6.2 to a run of c with the given runes
// on either side: whether the run can open and whether it can close.
func delimiterFlanking(c byte, before, after rune) (canOpen, canClose bool) {
	beforeSpace, beforePunct := unicode.IsSpace(before), isMarkdownPunct(before)
	afterSpace, afterPunct := unicode.IsSpace(after), isMarkdownPunct(after)
	left := !afterSpace && (!afterPunct || beforeSpace || beforePunct)
	right := !beforeSpace && (!beforePunct || afterSpace || afterPunct)
	if c == '_' {
		return left && (!right || beforePunct), right && (!left || afterPunct)
	}
	return left, right
}

// runeBefore and runeAfter give the rune on each side of a delimiter run. The
// start and the end of the text stand in as whitespace, the way CommonMark 6.2
// asks. Both decode the way the parser does: from the nearest byte at or
// before the position that starts a UTF-8 sequence, so a stray continuation
// byte is read with the character before it. Before "*" in a digit, the byte
// 0xAA and "*", the parser sees the digit, and the run closes; decoded on its
// own the byte is the replacement character, a symbol, and the run closed
// nothing.
func runeBefore(s string, i int) rune {
	if i <= 0 {
		return ' '
	}
	return runeAtOrBefore(s, i-1)
}

func runeAfter(s string, i int) rune {
	if i >= len(s) {
		return ' '
	}
	return runeAtOrBefore(s, i)
}

// runeAtOrBefore decodes the rune whose UTF-8 sequence starts at the nearest
// start byte at or before i.
func runeAtOrBefore(s string, i int) rune {
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r
}

// isMarkdownPunct answers CommonMark's "punctuation character", which covers
// the Unicode symbol categories as well as the punctuation ones.
func isMarkdownPunct(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func (s *mdInlineSink) flush() {
	s.heldBreak = false
	s.spellHeldClose(' ')
	s.spellPendingTitle()
	s.flushPending()
	for _, tag := range slices.Backward(s.open) {
		switch {
		case tag.title:
		case tag.destKey != "":
			s.sb.WriteString(mdLinkClose(tag.attrs, tag.destKey))
		default:
			if isEmphasisDelimiter(tag.close) {
				s.closedPair(mdClosedEmphasis{char: tag.close[0], openAt: tag.at, closeAt: s.sb.Len(), n: len(tag.close)})
			}
			s.sb.WriteString(tag.close)
		}
	}
	s.open = nil
}

// writeFromEvents reconstructs markdown from the ordered block + group event
// stream (cross-format semantic export). A `table` group and its `table-row`
// children render as a GFM table; every other block renders individually.
// Non-table group brackets are transparent — their child blocks render in
// place exactly as the block-only path did.
func (w *Writer) writeFromEvents(ctx context.Context, events []*model.Part, out io.Writer) error {
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
		case model.PartData:
			d, ok := part.Resource.(*model.Data)
			if !ok || d.Name != dataLinkReferenceDefinition {
				continue
			}
			if err := w.flushDefinition(out); err != nil {
				return err
			}
			if err := flushCells(); err != nil {
				return err
			}
			w.pendingDef = &mdRefDefinition{
				label:       d.Properties["label"],
				destination: d.Properties["destination"],
				title:       d.Properties["title"],
			}
		case model.PartGroupStart:
			g, ok := part.Resource.(*model.GroupStart)
			if !ok {
				continue
			}
			if err := w.flushDefinition(out); err != nil {
				return err
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
			if err := w.flushDefinition(out); err != nil {
				return err
			}
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
			if w.pendingDef != nil && w.pendingDef.take(block, w.blockRuns(block)) {
				continue
			}
			if err := w.flushDefinition(out); err != nil {
				return err
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
			if err := w.writeBlockMarkdown(ctx, block, out); err != nil {
				return err
			}
		}
	}
	if err := w.flushDefinition(out); err != nil {
		return err
	}
	return flushCells()
}

// rendersOnlyDelimiters reports whether runs render as emphasis delimiters
// around nothing: every text run is whitespace, every placeholder has no
// equivalent text, and every code is an emphasis the writer spells as
// delimiters or inline HTML it drops. A link or an image keeps its block,
// since its brackets read back as the link they are.
func rendersOnlyDelimiters(runs []model.Run) bool {
	codes := 0
	for _, r := range runs {
		switch r.Kind() {
		case model.RunKindText:
			if strings.TrimSpace(r.Text.Text) != "" {
				return false
			}
		case model.RunKindPh:
			if r.Ph.Equiv != "" {
				return false
			}
		case model.RunKindPcOpen:
			if _, emphasis := mdInlineTag[r.PcOpen.Type]; !emphasis && r.PcOpen.Type != "fmt:html" {
				return false
			}
			codes++
		case model.RunKindPcClose:
		default:
			return false
		}
	}
	return codes > 0
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
		text := escapeTableCell(renderInlineMarkdown(w.cellRuns(c)))
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
				caption = renderInlineMarkdown(w.cellRuns(cells[0]))
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
func (w *Writer) writeBlockMarkdown(ctx context.Context, block *model.Block, out io.Writer) error {

	if rendersNothing(block) {
		return nil
	}
	// A drawing's name, alt text and object title are graphic metadata, not
	// document flow. They are surfaced as their own blocks so an ingestion
	// consumer and the editor can see them, but rendering each as a paragraph
	// turns one image into four stray lines — a name, a filename, and the alt
	// text twice, once per non-visual-properties element. The alt text reaches
	// the output through the image run's own attributes instead.
	// A document's core properties — title, author, keywords, category — are
	// metadata about the file in the same way: real translation units (a
	// document title is translated), but rendering them as paragraphs appends
	// the author's name and keyword list after the last real paragraph as if
	// the document said them. Generative-path only; projection.IsMetadata is
	// the one rule every generative writer and the preview share.
	if projection.IsMetadata(block) {
		return nil
	}

	role0 := block.SemanticRole()
	if role0 == "" {
		role0 = block.Type
	}
	switch role0 {
	case "", model.RoleParagraph, model.RoleHeading, model.RoleTitle, model.RoleListItem:
		if rendersOnlyDelimiters(w.blockRuns(block)) {
			// A block whose runs hold no text and whose codes are emphasis
			// around inline HTML renders as bare delimiters once the HTML is
			// dropped: "*<A>*" before a heading was written as "**", a
			// paragraph of two asterisks the pass after read as text, and
			// "# *<A>*" as "# **", a heading with text.
			return nil
		}
	}
	if err := w.separateBlock(role0 == model.RoleListItem, out); err != nil {
		return err
	}

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
	text, quoteLevels := w.inlineBlockText(block)
	text = w.convergeInlineText(ctx, block, text, quoteLevels)
	if block.Type == "docusaurus-admonition-title" || block.Type == "admonition-title" {
		// An admonition's title is text to the reader, and the rebuild
		// writes it as a paragraph, where a bracket span can be a reference
		// to one of the document's definitions: "::: [0]" over a definition
		// of "0" came back as a paragraph holding a reference, and the
		// label's block with it.
		text = escapeBareBrackets(text)
	}
	if role == model.RoleCode {
		text = renderInlineLiteral(w.blockRuns(block))
	}

	var prefix, suffix string
	switch role {
	case model.RoleTitle:
		prefix = "# "
		text = escapeHeadingClosingSequence(singleLineHeading(text))
	case model.RoleHeading:
		if n := block.HeadingLevel(); n > 0 {
			prefix = strings.Repeat("#", n) + " "
			text = escapeHeadingClosingSequence(singleLineHeading(text))
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
		// do-not-translate signal survives cross-format export. The content
		// carries the line ending of its last line and the suffix opens with
		// one, so one of them goes or every pass adds a blank line before the
		// closing fence.
		text = strings.TrimSuffix(text, "\n")
		info := block.CodeLanguage()
		fence := codeFence(text, info)
		// An info string that begins with the fence's own character would
		// lengthen the fence: "~~~" and "~x" written together open a fence of
		// four that the closing fence of three does not close. A space
		// between them is not part of the info string (CommonMark 4.5).
		sep := ""
		if info != "" && info[0] == fence[0] {
			sep = " "
		}
		prefix, suffix = fence+sep+info+"\n", "\n"+fence
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
	//
	// A list item's text lands after `- `, and a rebuilt blockquote's body after
	// `> `. Both are block-content positions, where the same markers open the
	// same constructs: "* <A0A>#" reached the writer as an item whose text is a
	// bare "#", was written as "- #", and re-read as an item holding an empty
	// heading, which carries no content — the item was gone (#2469).
	switch {
	case prefix == "" && suffix == "":
		text = foldBlankLines(text)
		if bqPrefix, body, isQuote := w.blockquoteRebuild(block, text); isQuote {
			levels := strings.Count(bqPrefix, ">")
			prefix, text = bqPrefix, escapeLazyQuoteLines(escapeLeadingBlockMarker(foldQuoteMarkerLines(body, levels)), levels)
		} else {
			text = escapeBlockMarkerLines(body)
			text = escapeInteriorBlockBars(text)
		}
	case role == model.RoleListItem:
		// An item's continuation lines are in block-content position like a
		// paragraph's, so a line that forms a setext underline or a GFM
		// delimiter row promotes the line above it the same way.
		text = escapeInteriorBlockBars(escapeItemMarkerLines(foldBlankLines(text), len(prefix)))
		if first, _, _ := strings.Cut(text, "\n"); isThematicBreak(prefix + first) {
			// "- --" is three dashes with spaces between, a thematic break
			// (CommonMark 4.1), so an item whose text is a dash run escapes
			// the first of them.
			text = "\\" + text
		}
	}

	if _, err := fmt.Fprint(out, w.quoteBody(prefix+text+suffix)); err != nil {
		return err
	}
	return nil
}

// separateBlock writes the separator between the previous block and the one
// about to be written, and records the one about to be written as previous.
func (w *Writer) separateBlock(isItem bool, out io.Writer) error {
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
	return nil
}

// quoteBody marks a block's rendered body for the block quotes it sits in. A
// block inside a <blockquote> bracket is quoted regardless of its own role:
// the marker goes on every line, including continuation lines, or the
// quotation ends after the first one.
func (w *Writer) quoteBody(body string) string {
	q := w.quoteDepth()
	if q == 0 {
		return body
	}
	marker := strings.Repeat("> ", q)
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		lines[i] = marker + ln
	}
	return strings.Join(lines, "\n")
}

// mdRefDefinition is a link reference definition the generative path holds
// back until the blocks that carry its translatable label and title have
// arrived, so the definition is written once, with their text in place.
type mdRefDefinition struct {
	label, destination, title string
}

// take fills the definition from one of its own blocks and reports whether
// the block was one: the reader emits a visible label and a used title as
// blocks right after the definition's Data part.
func (d *mdRefDefinition) take(block *model.Block, runs []model.Run) bool {
	switch block.Type {
	case blockTypeRefLabel:
		if text := model.RenderRunsWithData(runs); text != "" {
			d.label = text
		}
	case blockTypeRefTitle:
		d.title = model.RenderRunsWithData(runs)
	default:
		return false
	}
	return true
}

// flushDefinition writes the definition held back, if any, as a block of its
// own: `[label]: destination "title"`.
func (w *Writer) flushDefinition(out io.Writer) error {
	def := w.pendingDef
	if def == nil {
		return nil
	}
	w.pendingDef = nil
	if err := w.separateBlock(false, out); err != nil {
		return err
	}
	dest := definitionDestination(def.destination, def.title != "")
	text := "[" + def.label + "]: " + dest
	if def.title != "" {
		// The parser takes a backslash at the end of a bare destination
		// together with the space after it, so a title after such a
		// destination needs a second space to be read at all.
		sep := " "
		if endsInUnescapedBackslash(dest) {
			sep = "  "
		}
		text += sep + refTitleSpelling(def.title)
	}
	_, err := fmt.Fprint(out, w.quoteBody(text))
	return err
}

// definitionDestination spells a rebuilt definition's destination: as the
// reader recorded it, or between angle brackets when a title follows and the
// destination holds a control character, which a bare destination cannot
// carry past a title on the same line: "[R]:\n[R]\x06\n'0'0" was rebuilt as
// "[R]: [R]\x06 \"0\"" and read back without its title. A destination holding
// an angle bracket cannot go between them and stays as recorded, and so does
// one ending in an unescaped backslash, which would escape the closing
// bracket: a backslash, a control byte and a backslash before two spaces and
// a title was spelled between brackets that never closed, and the definition
// was a paragraph. Bare, flushDefinition separates it from the title by two
// spaces.
func definitionDestination(dest string, titled bool) string {
	if dest == "" {
		// An empty destination is spelled by its brackets alone.
		return "<>"
	}
	if !titled || strings.ContainsAny(dest, "<>") || endsInUnescapedBackslash(dest) {
		return dest
	}
	for i := range len(dest) {
		if dest[i] < ' ' {
			return "<" + dest + ">"
		}
	}
	return dest
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
			text := escapeTableCell(renderInlineMarkdown(w.cellRuns(c.Block)))
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

// escapeTableCell makes cell text safe inside a GFM table cell: an unescaped
// pipe is escaped and newlines collapse to <br> so a multi-line value stays on
// one row. A pipe the source already escaped is left as it is, or the cell
// gained a backslash on every pass and after the first one its text read as an
// escaped backslash followed by a column separator (#2501).
func escapeTableCell(s string) string {
	s = escapeCellPipes(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return s
}

// escapeBareBrackets writes a backslash before every unescaped "[" in text,
// so no span of it reads as a link or a reference.
func escapeBareBrackets(text string) string {
	var b strings.Builder
	for i := range len(text) {
		if text[i] == '[' && !backslashEscaped(text, i) {
			b.WriteByte('\\')
		}
		b.WriteByte(text[i])
	}
	return b.String()
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

// escapeHeadingClosingSequence backslash-escapes a run of hashes that ends a
// rebuilt heading's text, so it stays the heading's content rather than
// becoming the closing sequence CommonMark 4.2 reads there.
//
// The residue matters because the rebuild path drops inline constructs it has
// no Markdown spelling for. "# <A>#" leaves the bare text "#", written as
// "# #", where the hash is a closing sequence and the heading has no content
// at all — the block was gone (#2484). This is the heading's form of #2469:
// the marker sits at the end of the line rather than at its start.
//
// A hash not preceded by whitespace is content already ("# C#"), and an
// escaped run matches nothing, so the escape is idempotent. A bare carriage
// return is whitespace inside the line to the parser, like a space or a tab.
func escapeHeadingClosingSequence(text string) string {
	end := len(strings.TrimRight(text, " \t\r"))
	i := end
	for i > 0 && text[i-1] == '#' {
		i--
	}
	if i == end {
		return text
	}
	if i > 0 && text[i-1] != ' ' && text[i-1] != '\t' && text[i-1] != '\r' {
		return text
	}
	return text[:i] + "\\" + text[i:]
}

// escapeBlockMarkerLines applies escapeLeadingBlockMarker to every line of a
// block the rebuild path emits verbatim. Each line of such a block starts in
// block-content position, so a marker on the second line opens a construct as
// surely as one on the first: "<div>0\n# 0" is a single HTML block whose text
// is "0\n# 0" (CommonMark 4.6 runs the block to the blank line), the rebuild
// path has no spelling for the HTML and writes the text as a paragraph, and the
// second line read back as an ATX heading — one block became two (#2470).
//
// A blockquote body is excluded at the call site: its continuation lines carry
// the ">" marker the rebuild restores, which is the one marker that belongs
// there. A list item's continuation lines carry their indent, so they begin
// with a space and match no case.
func escapeBlockMarkerLines(text string) string {
	if !strings.Contains(text, "\n") {
		return escapeLeadingBlockMarker(text)
	}
	// The first line's marker is judged on the whole text: a link reference
	// definition's label may span lines.
	lines := strings.Split(escapeLeadingBlockMarker(text), "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = escapeInterruptingBlockMarker(lines[i])
	}
	return strings.Join(lines, "\n")
}

// escapeContinuationLineMarkers applies escapeLeadingBlockMarker to every line
// of text after the first.
func escapeContinuationLineMarkers(text string) string {
	if !strings.Contains(text, "\n") {
		return text
	}
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = escapeLeadingBlockMarker(lines[i])
	}
	return strings.Join(lines, "\n")
}

// escapeItemMarkerLines applies escapeLeadingBlockMarker to every line of a
// rebuilt list item whose content starts at column. The item's continuation
// lines are written after its first, at the column of the text they carry,
// and the parser ends the item's paragraph at any block marker there, a bullet
// with nothing after it included: "- 0\n*\n0" read back as an item, an empty
// list and a paragraph, where the item's text had been "0\n*\n0". A line
// indented past the item's column is inside the item, where a marker sits
// after up to three more spaces: "*    0\n    # 0" is one item whose lazy
// line carries four spaces, and rebuilt after "- " the same line is a heading.
func escapeItemMarkerLines(text string, column int) string {
	if !strings.Contains(text, "\n") {
		return escapeLeadingBlockMarker(text)
	}
	lines := strings.Split(escapeLeadingBlockMarker(text), "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = escapeItemMarkerLine(lines[i], column)
	}
	return strings.Join(lines, "\n")
}

// escapeItemMarkerLine escapes a block marker after the indentation of a
// rebuilt item's continuation line, where the indentation is at most three
// columns past the item's content column. A tab advances to the next tab
// stop of four (CommonMark 2.2): a line of a tab and ">0" sits four columns
// in, inside an item whose content starts at column two, where the ">" opens
// a quote. A setext underline or a table delimiter row there is escaped the
// same way: a tab and "=" after a quoted item's lazy lines was text in the
// quote, and rebuilt inside the item alone it underlined the lines above.
func escapeItemMarkerLine(line string, column int) string {
	indent, col := 0, 0
	for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
		if line[indent] == '\t' {
			col += 4 - col%4
		} else {
			col++
		}
		indent++
	}
	if col > column+3 {
		return line
	}
	rest := line[indent:]
	if i, ok := blockMarkerPos(rest); ok && rest[i] != '[' {
		return line[:indent+i] + "\\" + rest[i:]
	}
	if isSetextBar(rest) || isTableDelimiterRow(rest) {
		if j := firstBarChar(rest); j >= 0 {
			return line[:indent+j] + "\\" + rest[j:]
		}
	}
	return line
}

// escapeContinuationLineMarkerAt escapes any block marker that opens a lazy
// continuation line whose first byte sits at column, where the parser
// measures the indentation before a marker from (blockIndentAt): a nested
// quote's line with one marker of two is lazy for the inner quote, and a
// space and a tab after its ">" put a "#" two columns in, a heading. A link
// reference definition is the exception: a definition cannot interrupt a
// paragraph (CommonMark 4.7), so a line that reads as one is the paragraph's
// text, and its bracket may open a reference link whose definition's label is
// a block of its own. "[0]:0" over a quote whose lazy line is "[0]:0" has
// that line as a reference and text, and escaped as a definition lookalike
// the reference was gone, and the label block with it.
func escapeContinuationLineMarkerAt(line string, column int) string {
	off, ok := blockIndentAt(line, column)
	if !ok {
		return line
	}
	if i, found := blockMarkerPos(line[off:]); found && line[off+i] != '[' {
		return line[:off+i] + "\\" + line[off+i:]
	}
	return line
}

// codeFence returns the backtick run that opens and closes a rebuilt fenced
// code block: three, or one more than the longest run of backticks that starts
// a line of the content.
//
// CommonMark 4.5 closes a fence on a run at least as long as the opener, so a
// three-backtick opener around content holding a three-backtick line ended the
// block after its first line: the rest became a paragraph and a stray opener
// was left at the end (#2487). The fence is backticks, so a tilde run in the
// content closes nothing and does not count, except when the info string holds
// a backtick, which a backtick fence's info string may not: the fence is
// tildes then, and the tilde runs in the content are the ones that count.
func codeFence(text, info string) string {
	char := byte('`')
	if strings.Contains(info, "`") {
		char = '~'
	}
	longest := 0
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimLeft(line, " ")
		n := 0
		for n < len(line) && line[n] == char {
			n++
		}
		longest = max(longest, n)
	}
	return strings.Repeat(string(char), max(3, longest+1))
}

// foldBlankLines drops the blank lines inside a block the rebuild path emits as
// flow content, so the block stays one block.
//
// A blank line ends a paragraph, and the rebuild path leaves one behind
// wherever it drops a construct that occupied a line of its own: the inline
// HTML in "a\n<a>\na" is dropped, which is accepted lossiness, and the empty
// line it left split the block in two (#2503). Fenced code keeps its blank
// lines: indentation and spacing are the content there, and it reaches the
// writer with a prefix and a suffix rather than through this path.
func foldBlankLines(text string) string {
	if !strings.Contains(text, "\n") {
		return text
	}
	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if isBlankLine(line) {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n")
}

// isBlankLine reports whether the parser reads line as a blank line: one of
// spaces and tabs only (CommonMark 2.1), with the carriage return of a CRLF
// line ending. A form feed or a vertical tab is content, so a line holding
// one keeps the paragraph going: "<A\n\f\nA>" is one paragraph, and folded
// to "<A\nA>" it was an inline tag and no block at all.
func isBlankLine(line string) bool {
	return strings.Trim(line, " \t\r") == ""
}

// foldQuoteMarkerLines drops a rebuilt blockquote body's continuation lines
// that hold nothing but their marker, the way foldBlankLines drops a
// paragraph's blank lines: a line a dropped construct leaves empty ends the
// quote's paragraph on re-read, and ">00\n><A>\n0" came back as two blocks.
func foldQuoteMarkerLines(body string, levels int) string {
	if !strings.Contains(body, "\n") {
		return body
	}
	lines := strings.Split(body, "\n")
	kept := lines[:1]
	for _, line := range lines[1:] {
		prefix := blockquoteMarkerPrefixUpTo(line, levels)
		if isBlankLine(line[len(prefix):]) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// escapeLazyQuoteLines escapes the block markers on a rebuilt blockquote body's
// LAZY continuation lines — the ones CommonMark 5.1 lets a paragraph run onto
// without repeating the ">".
//
// A marked continuation line is left alone: it carries the ">" the rebuild
// restores, which is the marker that belongs there, and escaping it would break
// the quote into loose paragraphs. A lazy line carries none, so a marker on it
// opens a construct as surely as one at the start of a block: ">0\n#\\\n0" is
// one block whose text is "0\n#\n0", and the bare "#" on line two read back as
// a heading, so one block became two (#2485). A lazy line that forms a GFM
// delimiter row or a setext underline promotes the line above it the same way,
// which is what escapeInteriorBlockBars does for a plain paragraph.
//
// On a lazy line the parser ends the quote's paragraph at any block marker,
// a list marker with nothing after it included, where CommonMark 5.2 would
// let a paragraph run on. So every marker on a lazy line is escaped, where a
// plain paragraph's continuation line escapes the interrupting ones alone.
//
// levels is the quote's depth, which bounds the markers a continuation line
// carries as its prefix: a ">" beyond them is content, the way the rebuild
// path leaves one when it drops the inline HTML that stood between two.
func escapeLazyQuoteLines(body string, levels int) string {
	if !strings.Contains(body, "\n") {
		return body
	}
	lines := strings.Split(body, "\n")
	for i := 1; i < len(lines); i++ {
		// A marked line keeps its ">" and the escape applies to what follows
		// it, which is the same block-content position the marker opens.
		// A line with fewer markers than the quote has levels is lazy for
		// the levels it lacks, and the parser ends the paragraph at any
		// marker there as it does on a line with none.
		prefix := blockquoteMarkerPrefixUpTo(lines[i], levels)
		rest := lines[i][len(prefix):]
		if strings.Count(prefix, ">") < levels {
			rest = escapeContinuationLineMarkerAt(rest, columnWidth(prefix))
		} else {
			rest = escapeInterruptingBlockMarkerAt(rest, columnWidth(prefix))
		}
		if isSetextBar(rest) || isTableDelimiterRow(rest) {
			if j := firstBarChar(rest); j >= 0 {
				rest = rest[:j] + "\\" + rest[j:]
			}
		}
		lines[i] = prefix + rest
	}
	return strings.Join(lines, "\n")
}

// escapeInterruptingBlockMarker escapes the marker that begins a CONTINUATION
// line of a rebuilt block, which is a narrower set than the one that opens a
// block at the start of one: CommonMark 5.2 lets a list interrupt a paragraph
// only when the item carries content and, for an ordered list, only when it
// starts at 1.
//
// Escaping a marker that cannot interrupt changes the document. "[R]:\n0)" is a
// paragraph, because an unmatched ")" is not a link destination; escaping the
// list marker spells "0\)", which IS one, so the paragraph became a link
// reference definition and the block was gone.
func escapeInterruptingBlockMarker(line string) string {
	return escapeInterruptingBlockMarkerAt(line, 0)
}

// escapeInterruptingBlockMarkerAt is escapeInterruptingBlockMarker for a line
// whose first byte sits at column, where the parser measures the indentation
// before a marker from: inside a blockquote the content starts after the
// marker, so a space and a tab before "#" are two columns in after "> " and
// open a heading, where at a line's start they are four and the "#" is text.
func escapeInterruptingBlockMarkerAt(line string, column int) string {
	off, ok := blockIndentAt(line, column)
	if !ok {
		return line
	}
	i, found := blockMarkerPos(line[off:])
	if !found || !interruptsAParagraph(line[off:]) {
		return line
	}
	return line[:off+i] + "\\" + line[off+i:]
}

// blockIndentAt returns the length of the indentation that can precede a
// block marker on a line whose first byte sits at column, measured as the
// parser measures it: spaces and tabs, a tab reaching the next stop of four
// (CommonMark 2.2), and at most three columns of it (CommonMark 4.2). ok is
// false where the line is indented code.
func blockIndentAt(text string, column int) (int, bool) {
	n, col := 0, column
	for n < len(text) && (text[n] == ' ' || text[n] == '\t') {
		if text[n] == '\t' {
			col += 4 - col%4
		} else {
			col++
		}
		if col-column > 3 {
			return 0, false
		}
		n++
	}
	return n, true
}

// columnWidth returns the column a line's content starts at after prefix: its
// bytes, with a tab reaching the next stop of four.
func columnWidth(prefix string) int {
	col := 0
	for i := range len(prefix) {
		if prefix[i] == '\t' {
			col += 4 - col%4
		} else {
			col++
		}
	}
	return col
}

// interruptsAParagraph reports whether the block marker leadingBlockMarkerPos
// found at the start of line opens a construct that CommonMark lets interrupt a
// paragraph. A heading, a blockquote, a thematic break and a fence all do,
// whatever follows them.
func interruptsAParagraph(line string) bool {
	line = line[blockIndent(line):]
	if line == "" {
		return false
	}
	switch c := line[0]; {
	case c == '#', c == '>', c == '`', c == '~':
		return true
	case c == '-' || c == '+' || c == '*' || c == '_':
		return isThematicBreak(line) || (c != '_' && listContentFollows(line, 1))
	case c >= '0' && c <= '9':
		n := 0
		for n < len(line) && line[n] >= '0' && line[n] <= '9' {
			n++
		}
		if strings.TrimLeft(line[:n], "0") != "1" {
			return false
		}
		return listContentFollows(line, n+1)
	}
	return false
}

// listContentFollows reports whether a list marker ending just before i is
// followed by a space or tab and then content, which CommonMark 5.2 requires
// before the item can interrupt a paragraph. Content is anything but the
// parser's whitespace: a form feed or a vertical tab after the marker is an
// item's content, and the line interrupts.
func listContentFollows(line string, i int) bool {
	if i >= len(line) || (line[i] != ' ' && line[i] != '\t') {
		return false
	}
	return strings.Trim(line[i:], " \t\r\n") != ""
}

// leadingBlockMarkerPos reports the byte index of the marker character to
// backslash-escape so text no longer opens a block construct, and whether any
// such marker was found. For every construct but ordered lists the marker is
// the first byte; for an ordered list it is the "." or ")" after the digits (a
// backslash before a digit is not an escape, so the punctuation is escaped
// instead — "1. x" -> "1\. x").
func leadingBlockMarkerPos(text string) (int, bool) {
	off := blockIndent(text)
	i, ok := blockMarkerPos(text[off:])
	return off + i, ok
}

// blockIndent returns the length of the indentation that can precede a block
// marker: CommonMark allows up to three spaces there, and a fourth makes the
// line indented code. A marker the escape tested at byte zero alone was missed
// when the line carried any of it, and " # 0" opened a heading on the way back
// in (#2504).
func blockIndent(text string) int {
	n := 0
	for n < 3 && n < len(text) && text[n] == ' ' {
		n++
	}
	return n
}

// blockMarkerPos answers leadingBlockMarkerPos for a line whose indentation has
// already been stepped over.
func blockMarkerPos(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	switch c := text[0]; c {
	case '[':
		// Link reference definition: the definition is the whole line and
		// carries nothing to translate, so a paragraph that reads as one on the
		// way back in has lost its block (#2526).
		if readsAsLinkReferenceDefinition(text) {
			return 0, true
		}
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
	case ':':
		// A Docusaurus admonition opens on ":::" and closes on it too; a
		// title that is ":::" was written as a paragraph and read back as the
		// closer, and the block was gone.
		if strings.HasPrefix(text, ":::") && (len(text) == 3 || text[3] != ':') {
			return 0, true
		}
	case '!', '?':
		// A MkDocs admonition opens on "!!! type" or "??? type".
		if (strings.HasPrefix(text, "!!!") || strings.HasPrefix(text, "???")) && atLineBoundary(strings.TrimPrefix(text[3:], "+"), 0) {
			return 0, true
		}
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

// readsAsLinkReferenceDefinition reports whether text, emitted at the start of
// a block, opens a link reference definition (CommonMark 4.7) rather than a
// paragraph: a bracketed label, a colon, a destination, and then nothing but an
// optional title.
//
// The rebuild path escapes a literal '<' so it cannot open inline HTML, and
// "\<" IS a destination where a bare "<" is not: "[a]:<" came back as "[a]:\<",
// which reads as a definition, and a definition holds no translatable text, so
// the block was gone (#2526).
func readsAsLinkReferenceDefinition(text string) bool {
	label, rest, ok := cutLinkLabel(text)
	// A label needs a character that is not whitespace, by the parser's own
	// set: a vertical tab or a form feed is one.
	if !ok || strings.Trim(label, " \t\n\r") == "" || !strings.HasPrefix(rest, ":") {
		return false
	}
	return definitionTailIsADestination(rest[1:])
}

// cutLinkLabel splits a leading "[label]" off text, reporting the label's
// content and what follows the "]". A backslash escapes the bracket after it,
// a label may span lines (CommonMark 4.7), and a blank line ends it.
func cutLinkLabel(text string) (label, rest string, ok bool) {
	if text == "" || text[0] != '[' {
		return "", "", false
	}
	for i := 1; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case '\n':
			// A blank line ends the label.
			if rest := strings.TrimLeft(text[i+1:], " \t\r"); rest == "" || rest[0] == '\n' {
				return "", "", false
			}
		case '[':
			// A label holds no unescaped bracket (CommonMark 6.3), so
			// "[[]:x" opens no definition; it is the text of a link or text.
			return "", "", false
		case ']':
			return text[1:i], text[i+1:], true
		}
	}
	return "", "", false
}

// definitionTailIsADestination reports whether what follows a definition's
// colon is a destination with nothing but an optional title after it. Anything
// else on the line — the ")" of "[R]:\n0)", which closes more parentheses than
// the destination opened — leaves the line a paragraph. So does a title that
// is not closed, or that is followed by anything but whitespace (CommonMark
// 4.7): "[R]:0 \"" is a paragraph, and one whose "[R]" is a reference link
// when a definition of R exists, so escaping its bracket cost the label its
// use and the definition its block.
// isASCIIPunct reports whether c is an ASCII punctuation character, the set a
// backslash escapes (CommonMark 2.4).
func isASCIIPunct(c byte) bool {
	return (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~')
}

func definitionTailIsADestination(s string) bool {
	i, ok := skipDefinitionSpace(s, 0)
	if !ok {
		return false
	}
	i, ok = skipDefinitionDestination(s, i)
	if !ok {
		return false
	}
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r') {
		i++
	}
	if i >= len(s) || s[i] == '\n' {
		return true
	}
	var closer byte
	switch s[i] {
	case '"':
		closer = '"'
	case '\'':
		closer = '\''
	case '(':
		closer = ')'
	default:
		return false
	}
	j := i + 1
	for j < len(s) && s[j] != closer {
		if s[j] == '\\' {
			j++
		}
		j++
	}
	if j >= len(s) {
		return false
	}
	for j++; j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\r'); j++ {
	}
	return j >= len(s) || s[j] == '\n'
}

// skipDefinitionSpace steps over the whitespace CommonMark allows before a
// definition's destination: spaces and tabs, with at most one line ending. ok
// is false when the whitespace runs to the end, which leaves no destination.
func skipDefinitionSpace(s string, i int) (int, bool) {
	crossed := false
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\r':
			i++
		case '\n':
			if crossed {
				return i, false
			}
			crossed = true
			i++
		default:
			return i, true
		}
	}
	return i, false
}

// skipDefinitionDestination steps over a link destination starting at i,
// reporting the offset just past it. It follows goldmark's
// parseLinkDestination: angle brackets closed on the same line, or a run of
// non-whitespace bytes that ends at a ")" closing more parentheses than it
// opened. A control character other than a space, a tab or a line ending
// stays in the destination, as it does for the parser.
func skipDefinitionDestination(s string, i int) (int, bool) {
	if i >= len(s) {
		return i, false
	}
	if s[i] == '<' {
		for j := i + 1; j < len(s); j++ {
			switch s[j] {
			case '\\':
				if j+1 < len(s) && isASCIIPunct(s[j+1]) {
					j++
				}
			case '\n':
				return i, false
			case '>':
				return j + 1, true
			}
		}
		return i, false
	}
	depth, j := 0, i
	for j < len(s) {
		c := s[j]
		if c == '\\' && j+1 < len(s) && isASCIIPunct(s[j+1]) {
			// A backslash escapes ASCII punctuation only (CommonMark 2.4);
			// before anything else it is a character of the destination, and
			// a space after it ends the destination as any space does.
			j += 2
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			if depth == 0 {
				break
			}
			depth--
		}
		j++
	}
	return j, j > i
}

// atLineBoundary reports whether position i in text is the end of the first
// line — a space, tab, line ending, or the end of the string.
//
// A bare carriage return counts. goldmark ends a line on "\n" alone, so a "\r"
// stays inside the line, where it is whitespace like any other and "#\r0" opens
// an ATX heading whose content is "0". Testing for a space, a tab and a newline
// alone missed that, and a document written with the classic Mac line ending
// split on the rebuild path (#2528).
func atLineBoundary(text string, i int) bool {
	if i >= len(text) {
		return true
	}
	switch text[i] {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// isThematicBreak reports whether text's first line is a CommonMark thematic
// break: three or more matching '-', '_' or '*', separated only by whitespace.
// A carriage return counts as whitespace there, the way goldmark reads one that
// sits inside a line (#2528).
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
		case ' ', '\t', '\r':
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
// spaces. Mirrors goldmark's matchesSetextHeadingBar. The parser counts the
// indent up to a bare carriage return and then reads on past it, so only the
// spaces before one count and the return itself is left out.
func isSetextBar(line string) bool {
	if indentBeforeCarriageReturn(line) > 3 {
		return false
	}
	line = strings.ReplaceAll(line, "\r", "")
	s := strings.TrimRight(strings.TrimLeft(line, " "), " \t")
	if s == "" {
		return false
	}
	return s == strings.Repeat("=", len(s)) || s == strings.Repeat("-", len(s))
}

// isTableDelimiterRow reports whether line is a GFM table delimiter row — up to
// 3 leading spaces, then cells of only '-'/':'/spaces separated by '|', with at
// least one '-'. Mirrors goldmark's isTableDelim (extension/table.go): every
// byte is a space, '-', '|' or ':', and the line is not made of '-' alone. A
// real delimiter also needs a hyphen (a cell is ":?-+:?"), so require one to
// avoid escaping pipe-only lines that never form a table. A bare carriage
// return is whitespace inside the line to the parser, which counts the indent
// up to it and reads on past it: "\r-" is a row, and so is "\r    -".
func isTableDelimiterRow(line string) bool {
	if indentBeforeCarriageReturn(line) > 3 {
		return false
	}
	line = strings.ReplaceAll(line, "\r", "")
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

// indentBeforeCarriageReturn returns the count of spaces that open line, up to
// the first byte that is not a space; a bare carriage return ends the count the
// way any other byte does.
func indentBeforeCarriageReturn(line string) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	return n
}

// headingLineBreakSpans returns, in ascending order, the spans of heading
// text that each line break takes with it: the whitespace before the line
// ending, the blockquote markers a soft break bakes into the continuation
// line, and the whitespace after them. The markers are the ones the parser
// reads (blockquoteMarkerPrefix): a ">" four or more columns in is content,
// so a tab-indented ">" on a setext heading's second line stays in the
// heading, where a fold that took every ">" after the break left "a" and a
// tab and ">" as "a".
func headingLineBreakSpans(text string) [][2]int {
	var spans [][2]int
	for i := 0; i < len(text); i++ {
		if text[i] != '\n' {
			continue
		}
		start := i
		for start > 0 && (text[start-1] == ' ' || text[start-1] == '\t' || text[start-1] == '\r') {
			start--
		}
		end := i + 1
		end += len(blockquoteMarkerPrefix(text[end:]))
		for end < len(text) && (text[end] == ' ' || text[end] == '\t') {
			end++
		}
		// A line of whitespace between two breaks joins them into one
		// span, so the heading gets one space, not one per line.
		if n := len(spans); n > 0 && start <= spans[n-1][1] {
			spans[n-1][1] = end
		} else {
			spans = append(spans, [2]int{start, end})
		}
		i = end - 1
	}
	return spans
}

// foldSpans replaces each span of text with one space.
func foldSpans(text string, spans [][2]int) string {
	if len(spans) == 0 {
		return text
	}
	var b strings.Builder
	prev := 0
	for _, sp := range spans {
		b.WriteString(text[prev:sp[0]])
		b.WriteByte(' ')
		prev = sp[1]
	}
	b.WriteString(text[prev:])
	return b.String()
}

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
	return foldSpans(text, headingLineBreakSpans(text))
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
func (w *Writer) blockquoteRebuild(block *model.Block, text string) (prefix, body string, ok bool) {
	if lp, has := block.Properties[BlockPropLinePrefix]; has && lp != "" {
		if !isQuoteLinePrefix(lp) {
			// The continuation prefix is known and is not a quote marker, so
			// a ">" the text carries after it is content. The block's first
			// line may still open a quote its continuation lines run on
			// lazily, and a line of that content opening with a marker is
			// escaped here, where it is known to be content, before the body
			// is folded by its markers.
			if m, has := block.Properties[BlockPropQuoteMarker]; has && m != "" && w.quoteDepth() == 0 {
				return quoteMarkerPrefix(m), escapeContinuationLineMarkers(text), true
			}
			return "", text, false
		}
		// Hard-break body: the marker was stripped from the text, and the
		// sink has put it back after every "\n" exactly as RenderBlockContent
		// (the byte-exact skeleton reference) does. The first line takes the
		// marker its own line carried, and at most one space: the
		// continuation prefix may carry the indentation of the lines below,
		// and ">a" over ">     0" has the prefix ">     ", which put the first
		// line's text four columns in, where it was indented code inside the
		// quote. A carriage return after the marker is whitespace the line
		// does not need.
		first := lp
		if m, has := block.Properties[BlockPropQuoteMarker]; has && m != "" {
			first = m
		}
		return quoteMarkerPrefix(first), text, true
	}
	// The reader records the marker the block's own first line carried, which
	// is the only source for a quote with no continuation line at all: ">> a"
	// arrived as a one-line paragraph and came back as "a", the whole quote
	// gone (#2464). A block already inside a <blockquote> bracket is marked by
	// that bracket below, so its own marker would double it.
	if m, has := block.Properties[BlockPropQuoteMarker]; has && m != "" && w.quoteDepth() == 0 {
		return quoteMarkerPrefix(m), text, true
	}
	// Soft-break body: the continuation lines carry their ">" marker, except a
	// lazy continuation line (CommonMark 5.1), which has none; only the first
	// line always lacks one. Recover the marker from the first marked
	// continuation line so the quote opens with it: "> a\nb\n> c" re-reads as
	// one blockquote with a lazy line, where a body judged by its first
	// continuation line alone rebuilt as a paragraph plus a quote (#2434).
	//
	// A block whose text is raw markup is excluded: its ">" is content, and
	// re-marking it moved the quote onto the block's own first line and split
	// it in two on the pass after ("<p>.\n- <\n>", #2505). A markdown block
	// that really is a quote body carries the marker property above; the guess
	// serves a block that arrives from another format with none.
	if rawTextBlock(block) {
		return "", text, false
	}
	if m := continuationBlockquoteMarker(text); m != "" {
		return m, text, true
	}
	return "", text, false
}

// quoteMarkerPrefix is the marker a rebuilt quote's first line takes: the
// markers the block's own line carried, with the spaces after them where they
// are at most the one the marker takes and the three of indent CommonMark 5.1
// allows, and one space otherwise. The recorded marker may carry the
// indentation of a container the rebuild does not restore, and a bare
// carriage return after it is whitespace the line does not need: ">\t\t0"
// inside a quoted list item is the item's paragraph, and written with its
// marker as recorded it was indented code inside the quote.
func quoteMarkerPrefix(m string) string {
	m = strings.TrimRight(m, "\r")
	trimmed := strings.TrimRight(m, " \t")
	if trail := m[len(trimmed):]; trail == "" || (trail == strings.Repeat(" ", len(trail)) && len(trail) <= 4) {
		return m
	}
	return trimmed + " "
}

// inlineBlockText renders a block's runs as inline Markdown with the sink told
// what the block is: its quote depth, whether its text is raw markup, whether
// its first line gets the leading-marker escape, whether it is written as one
// ATX line, and the continuation prefix a hard-break body had recorded rather
// than in its text. It returns the text and the quote depth it was checked at.
func (w *Writer) inlineBlockText(block *model.Block) (string, int) {
	leadingMarker, heading := true, false
	switch block.SemanticRole() {
	case model.RoleTitle, model.RoleHeading:
		leadingMarker, heading = false, true
	case model.RoleCode, model.RoleCaption:
		leadingMarker = false
	}
	// A hard-break body has its continuation prefix recorded rather than in
	// its text; the sink puts it back before it checks its output. A prefix
	// of quote markers also sets the depth the output is checked at.
	linePrefix := block.Properties[BlockPropLinePrefix]
	quoteLevels := blockQuoteLevels(block)
	if isQuoteLinePrefix(linePrefix) {
		quoteLevels = strings.Count(linePrefix, ">")
	}
	text := renderInlineMarkdownBlock(w.blockRuns(block), quoteLevels, rawTextBlock(block), leadingMarker, heading, block.SemanticRole() == model.RoleListItem, linePrefix)
	// A block with no marker whose continuation lines every one carry a quote
	// marker is written as a quote (blockquoteRebuild), and its text is
	// checked at that depth.
	if quoteLevels == 0 && !rawTextBlock(block) {
		if m := continuationBlockquoteMarker(text); m != "" {
			quoteLevels = strings.Count(m, ">")
		}
	}
	return text, quoteLevels
}

// convergeInlineText returns a paragraph's or list item's rendered text as
// the spelling its own re-read settles on: the text is read back with the
// reader, inside the quote it will be written in, rendered again from the
// block that read gives, and the pass is repeated, up to twice, until a pass
// renders what it read. The sink spells markup from the parser's rules and
// checks the spelling against the parser, but a re-read hands the next pass
// a different run structure for the same text where a construct was dropped
// or a delimiter re-spelled, and that pass can spell it differently again:
// ">____!" and a backslash break over ">_!__" was written with its literal
// underscore bare, and the pass after, holding the inner pair as "_", escaped
// it. Text with no markup character is left as it is, as is one whose re-read
// is not one block of the same role, and one holding a reference link, which
// resolves against definitions a re-read of the block alone does not have.
func (w *Writer) convergeInlineText(ctx context.Context, block *model.Block, text string, quoteLevels int) string {
	role := block.SemanticRole()
	if role != "" && role != model.RoleParagraph && role != model.RoleListItem {
		return text
	}
	if !strings.ContainsAny(text, "*_~`<[&\\") {
		return text
	}
	for _, r := range w.blockRuns(block) {
		// A reference link resolves against the document's definitions,
		// which a re-read of the block alone does not have.
		if r.PcOpen != nil && isReferenceLink(r.PcOpen) {
			return text
		}
	}
	prefix := strings.Repeat("> ", quoteLevels)
	if role == model.RoleListItem {
		if quoteLevels > 0 {
			return text
		}
		prefix = "- "
	}
	for range 2 {
		rb := readSingleBlock(ctx, prefix+text+"\n")
		if rb == nil || rb.SemanticRole() != role {
			return text
		}
		next, _ := w.inlineBlockText(rb)
		if next == text {
			return text
		}
		text = next
	}
	return text
}

// readSingleBlock reads src with the markdown reader and returns the one
// block it holds, or nil when the read fails or yields any other number.
func readSingleBlock(ctx context.Context, src string) *model.Block {
	r := NewReader()
	doc := &model.RawDocument{
		URI:          "rebuild://pass",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(strings.NewReader(src)),
	}
	if err := r.Open(ctx, doc); err != nil {
		return nil
	}
	defer r.Close()
	var found *model.Block
	for res := range r.Read(ctx) {
		if res.Error != nil {
			return nil
		}
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		b, ok := res.Part.Resource.(*model.Block)
		if !ok {
			continue
		}
		if found != nil {
			return nil
		}
		found = b
	}
	return found
}

// isQuoteLinePrefix reports whether a continuation prefix the reader recorded
// is a blockquote's: a ">" after at most three spaces of indent (CommonMark
// 5.1). " >" over a hard break is the marker of the quote, and judged on its
// first byte alone it was indentation with a ">" of content after it.
func isQuoteLinePrefix(lp string) bool {
	indent := len(lp) - len(strings.TrimLeft(lp, " "))
	return indent <= 3 && strings.HasPrefix(lp[indent:], ">")
}

// blockQuoteLevels returns how many quote markers a block's text carries after
// each line ending: those of the marker its first line carried, except when
// the block relies on the writer to restore its continuation prefix, where the
// line endings in the text are bare and a ">" after one is content.
func blockQuoteLevels(block *model.Block) int {
	if _, has := block.Properties[BlockPropLinePrefix]; has {
		return 0
	}
	return strings.Count(block.Properties[BlockPropQuoteMarker], ">")
}

// rawTextBlock reports whether a block's text is markup the reader captured
// verbatim, where a leading marker is content rather than structure.
func rawTextBlock(block *model.Block) bool {
	return block.Type == "html-text" || block.Type == "html-block"
}

// continuationBlockquoteMarker returns the blockquote marker a text's
// continuation lines carry, or "" when text is single-line, any of them
// carries none, or none carries content after its marker.
//
// Every line must be marked. A paragraph whose text merely holds a ">" line —
// which is what the rebuild path leaves when it drops an inline construct that
// occupied one — was re-marked as a quote, and the ">" it gained on its own
// first line split it in two on the pass after (#2503's neighbour). A markdown
// quote body carries the marker property instead, so this answers only for a
// block that arrives from another format. A line of a soft-break quote body
// carries content after its marker, so lines that are markers alone are
// content: a vertical tab over a dropped tag and ">" was re-marked as a
// quote, the marker-only line folded away, and the quote of a vertical tab
// was no block.
func continuationBlockquoteMarker(text string) string {
	marker := ""
	content := false
	for nl := strings.IndexByte(text, '\n'); nl >= 0; {
		line := text[nl+1:]
		if next := strings.IndexByte(line, '\n'); next >= 0 {
			line = line[:next]
		}
		m := blockquoteMarkerPrefix(line)
		if m == "" {
			return ""
		}
		if marker == "" {
			marker = m
		}
		if strings.Trim(line[len(m):], " \t\r") != "" {
			content = true
		}
		next := strings.IndexByte(text[nl+1:], '\n')
		if next < 0 {
			break
		}
		nl += 1 + next
	}
	if !content {
		return ""
	}
	return marker
}

// blockquoteMarkerPrefix returns the whole blockquote marker sequence that
// opens line: every ">" the line begins with, each carrying the optional
// single space that follows it and the up-to-three spaces of indent
// CommonMark 5.1 allows before it. "" when the line opens no quote.
//
// The sequence is what the line spells, not one level of it: ">> a\n>> b"
// arrived with ">> " on its continuation line and was rebuilt as ">a\n>> b",
// a one-level quote holding a lazy line (#2464). An indented marker counts
// too: " >0" continues a quote, and a recovery testing only column 0 missed
// it. Four or more columns of indent, with a tab reaching the next stop of
// four, put the ">" in indented-code position, where it is content: a
// paragraph's tab-indented continuation line keeps its ">" (CommonMark 4.8).
func blockquoteMarkerPrefix(line string) string {
	return blockquoteMarkerPrefixUpTo(line, -1)
}

// blockquoteMarkerPrefixUpTo is blockquoteMarkerPrefix bounded to levels
// markers; a negative levels takes every marker the line opens with.
func blockquoteMarkerPrefixUpTo(line string, levels int) string {
	i, col, indent := 0, 0, 0
	for n := 0; levels < 0 || n < levels; n++ {
		j, c, ind := i, col, indent
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			next := c + 1
			if line[j] == '\t' {
				next = c + 4 - c%4
			}
			ind += next - c
			c = next
			j++
		}
		if ind > 3 || j >= len(line) || line[j] != '>' {
			return line[:i]
		}
		j++
		c++
		indent = 0
		if j < len(line) && line[j] == ' ' {
			j++
			c++
		} else if j < len(line) && line[j] == '\t' {
			// The marker takes one column of the tab; the rest is indent.
			next := c + 4 - c%4
			indent = next - c - 1
			c = next
			j++
		}
		i, col = j, c
	}
	return line[:i]
}

// dropOpeningBreakPrefix removes from the text after a hard break that opened
// the block the line ending and the blockquote markers, up to levels of them,
// that introduced the next line. The break spelled nothing, so that line is the
// block's first and its markers are supplied by the block's own prefix; a ">"
// beyond the block's depth is content.
func dropOpeningBreakPrefix(t string, levels int) string {
	if !strings.HasPrefix(t, "\n") {
		return t
	}
	rest := t[1:]
	return rest[len(blockquoteMarkerPrefixUpTo(rest, levels)):]
}

// headingLevel returns a block's heading level, preferring the normalized
// structural annotation (WS1) and falling back to the legacy "level" property;
// 0 when neither is present.

// blockText returns the rendered text for a block, preferring the target
// locale's translation if available, falling back to source. Multi-line
// paragraphs whose source carried a per-line continuation prefix (e.g.
// `> ` for blockquote bodies — see BlockPropLinePrefix in reader.go)
// have that prefix re-inserted after every "\n" so blockquotes and
// indented continuations retain their original line shape on round-trip.
// Mirrors okapi MarkdownFilter, whose TextUnit content carries only the
// LFs between lines while its skeleton-driven writer re-emits the
// per-line prefix.
//
// A translation is rendered by renderInlineText. A source an edit rewrote is
// rendered by renderEditedSource, which escapes the markup the edit added; a
// source nobody edited is written as it was read.
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		if runs := block.TargetRuns(w.Locale); len(runs) > 0 {
			return finishBlockContent(block, renderInlineText(block, runs))
		}
	}
	source := format.AuthoritativeRuns(block)
	if len(source) == 0 {
		return ""
	}
	if read, edited := block.SourceAsRead(); edited {
		return finishBlockContent(block, renderEditedSource(block, read))
	}
	return finishBlockContent(block, model.RenderRunsWithData(source))
}

// RenderBlockContent renders a block's content (the given run sequence —
// source or target) the way the skeleton splice emits it: inline codes
// re-emit their original data, front matter values restore/add YAML
// quoting, and the markdown line-prefix property re-applies to multi-line
// continuations. The MDX reader's byte-faithfulness check uses the same
// function so reader and writer can never disagree about untranslated
// output.
func RenderBlockContent(block *model.Block, runs []model.Run) string {
	return finishBlockContent(block, model.RenderRunsWithData(runs))
}

// renderInlineText renders a translation's runs as RenderRunsWithData does,
// escaping the complete inline constructs CommonMark would read from its text:
// raw HTML, an autolink, a character reference and a complete inline link or
// image. A translation is text, and inline codes travel as their own runs. The
// reader turns every such construct into an inline code, so a translation
// that copies the source's text keeps its bytes. An edited source is rendered
// by renderEditedSource instead, which also has the block as read to compare
// with.
//
// Text inside a code span is literal, as is the content of a code block, a
// math block, front matter and the raw HTML the HTML subfilter reads; none of
// it is escaped.
func renderInlineText(block *model.Block, runs []model.Run) string {
	switch block.Type {
	case "front-matter", "code-block", "math", "html-block", "html-text", "html-attr":
		return model.RenderRunsWithData(runs)
	}
	if block.SemanticRole() == model.RoleCode {
		return model.RenderRunsWithData(runs)
	}
	var b strings.Builder
	var at []int
	codeDepth := 0
	model.RenderRunsWith(&b, runs, &model.RunRenderer{
		Text: func(b *strings.Builder, t string) {
			if codeDepth == 0 {
				for i := range len(t) {
					if t[i] == '<' || t[i] == '&' || t[i] == '[' {
						at = append(at, b.Len()+i)
					}
				}
			}
			b.WriteString(t)
		},
		Code: func(r model.Run) {
			switch opens, closes := codeSpanRun(r); {
			case opens:
				codeDepth++
			case closes && codeDepth > 0:
				codeDepth--
			}
		},
	})
	if len(at) == 0 {
		return b.String()
	}
	out := b.String()
	var esc strings.Builder
	esc.Grow(len(out) + len(at))
	prev := 0
	for _, i := range at {
		if !opensInlineMarkup(out[i:]) || backslashEscaped(out, i) {
			continue
		}
		esc.WriteString(out[prev:i])
		esc.WriteByte('\\')
		prev = i
	}
	esc.WriteString(out[prev:])
	return esc.String()
}

// inlineMarkupRE matches, at the start of a string, the inline constructs
// CommonMark reads from '<', '&' or '[': raw HTML (an open or closing tag, a
// comment, a processing instruction, a declaration, a CDATA section; spec
// 6.6), a URI or email autolink (6.5), an entity or numeric character
// reference (2.5), and a complete inline link, whose destination and optional
// title are closed by ')' (6.3). Brackets that open no link, such as "[1]" or
// a link left unclosed, stay as they are.
var inlineMarkupRE = regexp.MustCompile(`^(?:` +
	`<[A-Za-z][A-Za-z0-9-]*(?:\s+[A-Za-z_:][A-Za-z0-9_.:-]*(?:\s*=\s*(?:[^"'=<>` + "`" + `\x00-\x20]+|'[^']*'|"[^"]*"))?)*\s*/?>` +
	`|</[A-Za-z][A-Za-z0-9-]*\s*>` +
	`|<!--(?:>|->|[\s\S]*?-->)` +
	`|<\?[\s\S]*?\?>` +
	`|<![A-Za-z][^>]*>` +
	`|<!\[CDATA\[[\s\S]*?\]\]>` +
	`|<[A-Za-z][A-Za-z0-9.+-]{1,31}:[^<>\x00-\x20]*>` +
	`|<[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*>` +
	`|&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]{1,7}|#[xX][0-9A-Fa-f]{1,6});` +
	`|\[[^\[\]]*\]\(\s*(?:<[^<>\n]*>|[^\s()<>]*(?:\([^\s()]*\)[^\s()<>]*)*)(?:\s+(?:"[^"]*"|'[^']*'|\([^()]*\)))?\s*\)` +
	`)`)

// opensInlineMarkup reports whether s begins with a construct inlineMarkupRE
// names.
func opensInlineMarkup(s string) bool {
	return inlineMarkupRE.MatchString(s)
}

// backslashEscaped reports whether the byte at i is already escaped: preceded
// by an odd number of backslashes.
func backslashEscaped[S string | []byte](s S, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// finishBlockContent applies the block-level spelling the skeleton splice
// needs to a block's rendered runs.
func finishBlockContent(block *model.Block, rendered string) string {
	if block.Type == "front-matter" {
		// The skeleton carries `key: ` and the newline only; the value —
		// including any quoting — is the block's responsibility. An
		// unchanged, originally-unquoted value renders raw so the
		// untranslated round-trip stays byte-exact whatever the source
		// spelling; quoting is restored (or added when needed) otherwise.
		quote := block.Properties[BlockPropFrontMatterQuote]
		read, _ := block.SourceAsRead()
		if quote == "" && rendered == model.RenderRunsWithData(read) {
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
// splitter applies when it decides whether a pipe belongs to the cell. Inside
// a code span every pipe takes a backslash whatever precedes it: the splitter
// takes the backslash of each `\|` there out of the content, where a backslash
// is a character, so content that is two backslashes and a pipe is spelled
// with three, and written with two it came back with one.
func escapeCellPipes(text string) string {
	if !strings.Contains(text, "|") {
		return text
	}
	var sb strings.Builder
	sb.Grow(len(text) + 4)
	fence := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		// Inside a span a backslash is a character, so a backtick after one
		// closes the span as any other does.
		if c == '`' && (fence > 0 || !backslashEscaped(text, i)) {
			n := 1
			for i+n < len(text) && text[i+n] == '`' {
				n++
			}
			switch {
			case fence == 0 && backtickRunFollows(text[i+n:], n):
				// A run with no run of its length after it opens no span
				// and is text (CommonMark 6.1).
				fence = n
			case fence == n:
				fence = 0
			}
			sb.WriteString(text[i : i+n])
			i += n - 1
			continue
		}
		if c == '|' && (fence > 0 || i == 0 || text[i-1] != '\\') {
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// backtickRunFollows reports whether text, the content after a span's
// opening run, holds a run of exactly n backticks; a backslash before one is
// a character there.
func backtickRunFollows(text string, n int) bool {
	for i := 0; i < len(text); i++ {
		if text[i] != '`' {
			continue
		}
		j := i
		for j < len(text) && text[j] == '`' {
			j++
		}
		if j-i == n {
			return true
		}
		i = j - 1
	}
	return false
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
	if source := format.AuthoritativeRuns(block); len(source) > 0 {
		return source
	}
	return nil
}

// cellRuns picks a table cell's runs: the cell's formatted display when the
// reader stamped one (a spreadsheet value cell shows its number through its
// format), else the locale-chosen runs.
func (w *Writer) cellRuns(block *model.Block) []model.Run {
	if display, ok := block.Properties[model.PropCellDisplay]; ok {
		return []model.Run{{Text: &model.TextRun{Text: display}}}
	}
	// A character reference stays a placeholder: the sink spells one the
	// markdown reader read as the source spelled it, and one from another
	// format as its characters, where a cell of one reference written as its
	// characters came back as a text block the source did not have.
	return w.blockRuns(block)
}
