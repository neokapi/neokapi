package doxygen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Doxygen/Javadoc comments in source code.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	tracker       *trailingNewlineTracker

	// lineEnd is the per-file line terminator used when expanding
	// multi-line comment templates. Picked from the first Block's
	// "lineEnding" property (set by the reader from the source bytes)
	// and falls back to "\n" when absent. Without honouring this, a
	// CRLF source like lists.h round-trips with LF inside every
	// comment body and diverges from okapi byte-for-byte.
	lineEnd string
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new Doxygen writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "doxygen",
		},
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// trailingNewlineTracker wraps an io.Writer and records the last byte
// written so the doxygen writer can mirror okapi's "always emit a
// terminating linebreak" behaviour without buffering the full output.
// Okapi's DoxygenFilter reads the source line-by-line and unconditionally
// appends `linebreak` to every line, so the upstream reference always
// terminates with a newline regardless of whether the source did.
type trailingNewlineTracker struct {
	w        io.Writer
	lastByte byte
	hasBytes bool
}

func (t *trailingNewlineTracker) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	if n > 0 {
		t.lastByte = p[n-1]
		t.hasBytes = true
	}
	return n, err
}

// SetOutputWriter overrides the base implementation to wrap the output
// in a trailingNewlineTracker. Required so finalize() can append a
// linebreak only when the writer hasn't already emitted one.
func (w *Writer) SetOutputWriter(out io.Writer) error {
	w.tracker = &trailingNewlineTracker{w: out}
	return w.BaseFormatWriter.SetOutputWriter(w.tracker)
}

// SetOutput overrides the base implementation for the same reason as
// SetOutputWriter. The base implementation creates a *os.File and
// assigns it to Output; we replay that assignment through the tracker.
func (w *Writer) SetOutput(path string) error {
	if err := w.BaseFormatWriter.SetOutput(path); err != nil {
		return err
	}
	w.tracker = &trailingNewlineTracker{w: w.Output}
	w.Output = w.tracker
	return nil
}

// finalize appends a trailing newline if the writer's output stream
// hasn't ended with one yet. Called after the part stream is drained
// (skeleton-driven and no-skeleton paths both invoke it). Mirrors
// okapi's DoxygenFilter behaviour — its line-buffered reader
// concatenates `line + linebreak` for every line, so the merged output
// always carries a terminating linebreak even when the source did
// not. Uses the writer's chosen lineEnd so a CRLF-source file gets
// CRLF padding rather than a stray LF.
func (w *Writer) finalize() error {
	if w.tracker == nil {
		return nil
	}
	if !w.tracker.hasBytes {
		return nil
	}
	if w.tracker.lastByte == '\n' {
		return nil
	}
	_, err := w.tracker.Write([]byte(w.lineSep()))
	return err
}

// Write consumes Parts from a channel and writes reconstructed source.
//
// For multi-section comment groups (a single /*! ... */ comment whose
// reader emitted several Blocks — one per \param/\return/\sa
// section — to honour the spec contract that section commands extract
// as separate Blocks), the writer must emit the whole group's text
// under one comment template, not one comment per Block. Both the
// skeleton-driven and skeleton-less paths therefore buffer the full
// part stream up front and resolve sibling lookups via blocksByID.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeleton(ctx, parts)
	}

	var collected []*model.Part
	blocksByID := make(map[string]*model.Block)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.writeCollected(collected, blocksByID)
			}
			collected = append(collected, part)
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
				}
			}
		}
	}
}

// writeCollected drains the buffered Parts in order, skipping
// non-first blocks of a comment group (they fold into the first
// block's output via writeBlockGrouped → joinedGroupText).
func (w *Writer) writeCollected(collected []*model.Part, blocksByID map[string]*model.Block) error {
	w.pickLineEnd(collected)
	first := true
	for _, part := range collected {
		if part.Type == model.PartBlock {
			block, ok := part.Resource.(*model.Block)
			if !ok {
				return errors.New("doxygen writer: expected Block resource")
			}
			// Subsequent blocks of a multi-section group are folded
			// into the first block's comment-template output.
			if firstID := block.Properties["groupFirstID"]; firstID != "" && firstID != block.ID {
				continue
			}
			if err := w.writeBlockGrouped(block, &first, blocksByID); err != nil {
				return err
			}
			continue
		}
		if err := w.writePart(part, &first); err != nil {
			return err
		}
	}
	return w.finalize()
}

// pickLineEnd locks the writer's line terminator to the first Block's
// "lineEnding" property. Defaults to "\n" when no Block carries the
// hint. Reading the value once up front matches okapi's behaviour of
// picking a single linebreak per file from DoxygenFilter.detectLineBreak.
func (w *Writer) pickLineEnd(collected []*model.Part) {
	if w.lineEnd != "" {
		return
	}
	for _, part := range collected {
		if part == nil || part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}
		if le := block.Properties["lineEnding"]; le != "" {
			w.lineEnd = le
			return
		}
	}
	w.lineEnd = "\n"
}

// lineSep returns the writer's chosen line terminator, defaulting to
// "\n" when pickLineEnd has not run yet (e.g. unit tests that drive a
// single block through writeBlock directly).
func (w *Writer) lineSep() string {
	if w.lineEnd == "" {
		return "\n"
	}
	return w.lineEnd
}

// writeBlockGrouped writes a single Block, joining sibling blocks of
// the same comment group when present.
func (w *Writer) writeBlockGrouped(block *model.Block, first *bool, blocks map[string]*model.Block) error {
	text := w.joinedGroupText(block, blocks)
	style := block.Properties["style"]
	raw := block.Properties["raw"]
	prefixes := w.joinedGroupPrefixes(block, blocks)
	layout := w.groupLayout(block, blocks)

	if !*first {
		if _, err := fmt.Fprint(w.Output, w.lineSep()); err != nil {
			return err
		}
	}
	*first = false

	return w.writeCommentStyled(text, style, raw, prefixes, layout)
}

// groupLayout returns the per-line layout descriptor for the comment
// group, looked up on the first block of the group (which carries
// the lineLayout property). For ungrouped blocks it returns the
// block's own layout.
func (w *Writer) groupLayout(block *model.Block, blocks map[string]*model.Block) string {
	if layout := block.Properties["lineLayout"]; layout != "" {
		return layout
	}
	firstID := block.Properties["groupFirstID"]
	if firstID == "" {
		return ""
	}
	if first, ok := blocks[firstID]; ok {
		return first.Properties["lineLayout"]
	}
	return ""
}

// writeWithSkeleton collects all blocks, then reconstructs output from skeleton entries.
func (w *Writer) writeWithSkeleton(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
					if w.lineEnd == "" {
						if le := block.Properties["lineEnding"]; le != "" {
							w.lineEnd = le
						}
					}
				}
			}
		}
	}
done:
	if w.lineEnd == "" {
		w.lineEnd = "\n"
	}
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("doxygen writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("doxygen writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.joinedGroupText(block, blocks)
				style := block.Properties["style"]
				raw := block.Properties["raw"]
				prefixes := w.joinedGroupPrefixes(block, blocks)
				layout := w.groupLayout(block, blocks)
				if err := w.writeCommentStyled(text, style, raw, prefixes, layout); err != nil {
					return err
				}
			}
		}
	}
	return w.finalize()
}

// writeCommentStyled reconstructs a comment using the given style.
//
// linePrefixes carries the per-line Doxygen command markers that the
// reader stripped on extraction (e.g. "\brief ", "\param x ",
// "\addtogroup mygroup "). The comment-style writers reattach them so
// the roundtrip output preserves the original markers instead of
// emitting bare prose. nil or empty prefixes mean "no markers" — the
// writers fall back to plain comment-line emission.
//
// layout, when non-empty, is the per-line layout descriptor produced
// by the reader's buildLineLayout. It lets the writer emit the
// original raw structural lines (blank `///` separators, excluded
// `\code...\endcode` blocks, non-translatable command lines) verbatim
// and substitute translated text into the translatable lines in
// place. Only relevant for line-comment styles (triple / exclamation).
func (w *Writer) writeCommentStyled(text, style, raw string, linePrefixes []string, layout string) error {
	switch style {
	case "triple":
		if layout != "" {
			return w.writeFromLayout(text, layout, "/// ", style)
		}
		return w.writeTripleSlash(text, raw, linePrefixes)
	case "exclamation":
		if layout != "" {
			return w.writeFromLayout(text, layout, "//! ", style)
		}
		return w.writeExclamation(text, raw, linePrefixes)
	case "docstring":
		if layout != "" {
			return w.writeFromLayout(text, layout, "", style)
		}
		return w.writeDocstring(text, raw, linePrefixes)
	case "javadoc":
		if layout != "" {
			return w.writeFromLayout(text, layout, "", style)
		}
		return w.writeJavadoc(text, raw, linePrefixes)
	case "qt":
		if layout != "" {
			return w.writeFromLayout(text, layout, "", style)
		}
		return w.writeQt(text, raw, linePrefixes)
	case "qt_member":
		// /*!< … */ as a stand-alone comment block (the leading
		// member marker `<` was stripped at extraction time so the
		// translatable text doesn't begin with `<`).
		return w.writeQtMember(text, raw, linePrefixes)
	case "javadoc_member":
		// /**< … */ as a stand-alone comment block.
		return w.writeJavadocMember(text, raw, linePrefixes)
	case "trailing":
		return w.writeTrailing(text, raw)
	case "trailing_qt":
		return w.writeTrailingQt(text, raw)
	case "trailing_javadoc":
		return w.writeTrailingJavadoc(text, raw)
	default:
		if layout != "" {
			return w.writeFromLayout(text, layout, "/// ", style)
		}
		return w.writeTripleSlash(text, raw, linePrefixes)
	}
}

// writeFromLayout emits a comment group using the per-line layout
// descriptor. Tags:
//
//	T:<prefix>   consume next text line, emit `<prefix><text>` — used
//	             for /// and //! comment groups. The prefix carries
//	             the original indent + comment marker + any stripped
//	             Doxygen command marker, so the writer doesn't add a
//	             marker of its own.
//	B:<prefix>   consume next text line, emit `<prefix><text>` — used
//	             for /** … */ and /*! … */ block comments where each
//	             raw line embeds its own delimiter / `*` line marker
//	             in the prefix.
//	S:<raw>      emit `<raw>` verbatim.
//
// marker is preserved as a no-op argument for backward compatibility
// with callers; T entries no longer consult it (the layout encodes
// the full line prefix). style identifies the comment shape ("qt",
// "javadoc", …) so the layout walker can apply okapi's per-style
// quirks — currently the Qt-block blank-then-canonical-body indent
// transfer (see the inline comment further down for details).
func (w *Writer) writeFromLayout(text, layout, _ string, style string) error {
	textLines := strings.Split(text, "\n")
	textCursor := 0
	entries := strings.Split(layout, "\x01")

	// First pass: identify padding T/B entries (those that would
	// consume an empty text-line). okapi's whitespace-collapse model
	// treats the whole comment as one fluid TextUnit and pushes any
	// "leftover" line slots — produced by joinProseLines collapsing
	// continuation prose into the anchor line — to the END of the
	// comment block, not inline with the prose. Mirror that: skip
	// padding entries during the in-order walk and emit the
	// equivalent canonical-blank lines just before the final closing
	// delimiter (or at the end for line-comment styles with no
	// delimiter).
	//
	// T entries (line-comment styles) emit `///` (bare marker, no
	// trailing space) for padding. B entries (block-comment styles)
	// emit the indent + `*` padding as okapi does (e.g. ` * ` for
	// javadoc — keep the body as-is).
	type entryPlan struct {
		tag    byte
		body   string
		text   string
		suffix string // optional trailing whitespace captured from raw (B entries only)
		isPad  bool   // T/B-entry whose text is empty (padding)
		canon  string // canonical bare comment marker for padding lines
	}
	plans := make([]entryPlan, 0, len(entries))
	for _, entry := range entries {
		if len(entry) < 2 {
			plans = append(plans, entryPlan{})
			continue
		}
		tag := entry[0]
		body := entry[2:]
		// B entries may carry a trailing-whitespace suffix from the
		// raw source line, encoded as `<prefix>\x02<suffix>`. Split
		// it back out so the writer can emit prefix + text + suffix
		// and preserve okapi's trailing-whitespace behaviour on lines
		// like ` *     . ` (lists.h line 21 — list-end `.` anchor with
		// a trailing space).
		suffix := ""
		if tag == 'B' {
			if i := strings.IndexByte(body, '\x02'); i >= 0 {
				suffix = body[i+1:]
				body = body[:i]
			}
		}
		ep := entryPlan{tag: tag, body: body, suffix: suffix}
		if tag == 'T' || tag == 'B' {
			line := ""
			if textCursor < len(textLines) {
				line = textLines[textCursor]
			}
			textCursor++
			ep.text = line
			if line == "" {
				ep.isPad = true
				if tag == 'T' {
					// `///` style — padding emits bare marker, no trailing space.
					ep.canon = strings.TrimRight(body, " \t")
				} else {
					// `/** */` / `/*! */` style — padding emits the body
					// as-is (includes the ` * ` line marker with its space).
					ep.canon = body
				}
			}
		}
		plans = append(plans, ep)
	}

	// Normalise B-style padding to the body indent of the FIRST
	// continuation B entry (a ` * `-style line, not the opening
	// `/**`/`/*!` delimiter line which carries `/`/`!` characters in
	// its prefix). okapi computes a paragraph-level baseline indent and
	// uses it for padding; without this, lists.h paragraph 2's
	// tail-pad emerges as ` *            ` (the 12-space indent of the
	// absorbed `More info about the click event.` line) where okapi
	// emits a flat ` *  `. Skipping the opener prevents `/** ` /
	// `/*! ` from contaminating the pad style for special_commands.h
	// blocks whose opening line carries inline body content.
	canonicalBPad := ""
	for _, p := range plans {
		if p.tag != 'B' || p.isPad || p.body == "" {
			continue
		}
		if !isStarBodyPrefix(p.body) {
			continue
		}
		canonicalBPad = p.body
		break
	}
	if canonicalBPad != "" {
		for i := range plans {
			if plans[i].isPad && plans[i].tag == 'B' {
				plans[i].canon = canonicalBPad
			}
		}
	}

	// okapi quirk: in /*! Qt-style blocks whose canonical body indent is
	// `<marker>  ` (i.e. ` *  ` — two spaces between `*` and content),
	// a blank ` *` separator immediately followed by a prose line whose
	// prefix EXACTLY equals the canonical body pad gets the body indent
	// transferred to the blank line. Source lines:
	//
	//	 *
	//	 *  More text here.
	//
	// emit as:
	//
	//	 *
	//	 *More text here.
	//
	// The behaviour is specific to /*! blocks with a 2-space body pad
	// (lists.h block 1) — Javadoc /** blocks and /*! blocks with a
	// 1-space body pad (special_commands.h) preserve source layout
	// per the okapi reference. Restricting on style="qt" + canonical
	// body pad ending in two trailing spaces keeps other fixtures
	// byte-equal.
	if style == "qt" && canonicalBPad != "" && strings.HasSuffix(canonicalBPad, "  ") {
		marker := strings.TrimRight(canonicalBPad, " \t")
		for i := 0; i+1 < len(plans); i++ {
			cur, next := &plans[i], &plans[i+1]
			if cur.tag != 'S' || next.tag != 'B' {
				continue
			}
			// The S entry must encode a bare `*`-marker line (e.g.
			// ` *` for ` *  ` canonical) — no trailing whitespace.
			if cur.body != marker {
				continue
			}
			// The B entry must use the canonical body pad EXACTLY —
			// over- or under-indented prose lines are NOT subject to
			// the transfer (lists.h block 2 has 5-space and 3-space
			// post-blank prose lines that okapi preserves verbatim).
			if next.body != canonicalBPad {
				continue
			}
			// Padding entries don't participate (the canonical-pad
			// rewrite already handles them). Only first-line-of-
			// paragraph prose triggers the transfer.
			if next.isPad {
				continue
			}
			cur.body = canonicalBPad // ` *` → ` *  `
			next.body = marker       // ` *  ` → ` *`
		}
	}

	// Decide where padding lands: just before the LAST S-entry whose
	// body looks like a comment closer (`*/` or anything that's NOT
	// purely whitespace+comment-marker). For triple/excl layouts this
	// usually means "no closer" — the loop falls through and padding
	// lands after the final entry.
	tailInsertAt := len(plans)
	for i := len(plans) - 1; i >= 0; i-- {
		if plans[i].tag == 'S' && strings.Contains(plans[i].body, "*/") {
			tailInsertAt = i
			break
		}
	}

	// Count padding entries to relocate.
	var paddingCanons []string
	for i := range plans {
		if plans[i].isPad {
			paddingCanons = append(paddingCanons, plans[i].canon)
		}
	}

	// Second pass: emit. Padding T entries are dropped from their
	// original position; their canonical-blank counterparts get
	// emitted at tailInsertAt instead.
	first := true
	nl := w.lineSep()
	emit := func(s string) error {
		if !first {
			if _, err := fmt.Fprint(w.Output, nl); err != nil {
				return err
			}
		}
		first = false
		if _, err := fmt.Fprint(w.Output, s); err != nil {
			return err
		}
		return nil
	}
	for i, ep := range plans {
		if i == tailInsertAt {
			for _, c := range paddingCanons {
				if err := emit(c); err != nil {
					return err
				}
			}
		}
		if len(ep.body) == 0 && ep.tag == 0 {
			// Empty entry placeholder — preserve the blank line that
			// the strings.Split on \x01 produced for malformed input.
			if err := emit(""); err != nil {
				return err
			}
			continue
		}
		switch ep.tag {
		case 'T', 'B':
			if ep.isPad {
				continue // padding emitted at tail
			}
			if err := emit(ep.body + ep.text + ep.suffix); err != nil {
				return err
			}
		case 'S':
			if err := emit(ep.body); err != nil {
				return err
			}
		}
	}
	if tailInsertAt == len(plans) {
		// No closer found — append padding at the very end.
		for _, c := range paddingCanons {
			if err := emit(c); err != nil {
				return err
			}
		}
	}
	return nil
}

// blockLinePrefixes returns the per-line command markers stored on
// the block (split on \x00), or nil if the block has none.
func blockLinePrefixes(block *model.Block) []string {
	if block == nil {
		return nil
	}
	raw := block.Properties["linePrefixes"]
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\x00")
}

// joinedGroupPrefixes returns the concatenated linePrefixes for every
// sibling block in a multi-section comment group, in section order.
// For ungrouped blocks this returns just the block's own prefixes.
func (w *Writer) joinedGroupPrefixes(block *model.Block, blocks map[string]*model.Block) []string {
	sizeStr := block.Properties["groupSize"]
	if sizeStr == "" {
		return blockLinePrefixes(block)
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size <= 1 {
		return blockLinePrefixes(block)
	}
	firstID := block.Properties["groupFirstID"]
	if firstID == "" || !strings.HasPrefix(firstID, "tu") {
		return blockLinePrefixes(block)
	}
	startNum, err := strconv.Atoi(firstID[2:])
	if err != nil {
		return blockLinePrefixes(block)
	}
	var all []string
	hasAny := false
	for i := range size {
		id := fmt.Sprintf("tu%d", startNum+i)
		sib, ok := blocks[id]
		if !ok {
			continue
		}
		px := blockLinePrefixes(sib)
		// Each sibling's text contributes len(px) lines (or 1 if
		// no prefixes recorded but the block has text).
		if len(px) == 0 {
			n := 1
			if t := w.blockText(sib); t != "" {
				n = strings.Count(t, "\n") + 1
			}
			for range n {
				all = append(all, "")
			}
		} else {
			all = append(all, px...)
			for _, p := range px {
				if p != "" {
					hasAny = true
				}
			}
		}
	}
	if !hasAny {
		return nil
	}
	return all
}

// blockText returns target or source text for a block. Inline-code
// runs (PlaceholderRun) emit their original Data verbatim so protected
// Doxygen commands (\a x, \param y, \n …) and HTML tags round-trip
// byte-for-byte even after pseudo-translation has run against the
// surrounding TextRuns.
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		if runs := block.TargetRuns(w.Locale); runs != nil {
			return runsTextWithCodes(runs)
		}
		return block.TargetText(w.Locale)
	}
	return runsTextWithCodes(block.Source)
}

// runsTextWithCodes flattens a run sequence to the literal text
// the doxygen writer should emit: TextRuns contribute their Text
// verbatim, PlaceholderRuns contribute their Data (the original
// Doxygen command / HTML tag substring captured at extraction time).
// Other run kinds fall back to their plain-text projection.
func runsTextWithCodes(runs []model.Run) string {
	var sb strings.Builder
	for _, run := range runs {
		switch {
		case run.Text != nil:
			sb.WriteString(run.Text.Text)
		case run.Ph != nil:
			sb.WriteString(run.Ph.Data)
		default:
			// Conservative fallback for unsupported run kinds.
			sb.WriteString(model.RunsText([]model.Run{run}))
		}
	}
	return sb.String()
}

// joinedGroupText returns the text for a single comment template,
// gathering sibling Blocks that belong to the same comment-group when
// the reader split a multi-section comment (e.g. \param a … \param b …
// \return … inside one /*! … */) into multiple Blocks. Each section's
// text is joined with a newline so the comment-style writers (writeQt,
// writeJavadoc, …) emit one line per section under the comment's
// shared delimiters. For ungrouped Blocks this returns blockText
// unchanged.
func (w *Writer) joinedGroupText(block *model.Block, blocks map[string]*model.Block) string {
	sizeStr := block.Properties["groupSize"]
	if sizeStr == "" {
		return w.blockText(block)
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size <= 1 {
		return w.blockText(block)
	}
	firstID := block.Properties["groupFirstID"]
	if firstID == "" || !strings.HasPrefix(firstID, "tu") {
		return w.blockText(block)
	}
	startNum, err := strconv.Atoi(firstID[2:])
	if err != nil {
		return w.blockText(block)
	}
	parts := make([]string, 0, size)
	for i := range size {
		id := fmt.Sprintf("tu%d", startNum+i)
		sib, ok := blocks[id]
		if !ok {
			continue
		}
		parts = append(parts, w.blockText(sib))
	}
	return strings.Join(parts, "\n")
}

func (w *Writer) writePart(part *model.Part, first *bool) error {
	switch part.Type {
	case model.PartData:
		return w.writeData(part, first)
	case model.PartBlock:
		return w.writeBlock(part, first)
	default:
		return nil
	}
}

func (w *Writer) writeData(part *model.Part, first *bool) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return errors.New("doxygen writer: expected Data resource")
	}

	raw := data.Properties["raw"]
	if raw == "" {
		return nil
	}

	if !*first {
		if _, err := fmt.Fprint(w.Output, w.lineSep()); err != nil {
			return err
		}
	}
	*first = false

	_, err := fmt.Fprint(w.Output, raw)
	return err
}

func (w *Writer) writeBlock(part *model.Part, first *bool) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("doxygen writer: expected Block resource")
	}

	// Pick line ending lazily for callers that bypass writeCollected
	// (e.g. unit tests driving a single Block straight to writeBlock).
	if w.lineEnd == "" {
		if le := block.Properties["lineEnding"]; le != "" {
			w.lineEnd = le
		}
	}

	text := w.blockText(block)

	style := block.Properties["style"]
	raw := block.Properties["raw"]
	prefixes := blockLinePrefixes(block)
	layout := block.Properties["lineLayout"]

	if !*first {
		if _, err := fmt.Fprint(w.Output, w.lineSep()); err != nil {
			return err
		}
	}
	*first = false

	return w.writeCommentStyled(text, style, raw, prefixes, layout)
}

// linePrefixAt returns the prefix to reattach for the i-th text line.
// Returns "" when no prefix was recorded for that line.
func linePrefixAt(prefixes []string, i int) string {
	if i < 0 || i >= len(prefixes) {
		return ""
	}
	return prefixes[i]
}

// writeTripleSlash writes text as /// line comments, preserving indentation from the original.
func (w *Writer) writeTripleSlash(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	lines := strings.Split(text, "\n")
	nl := w.lineSep()
	for i, line := range lines {
		if i > 0 {
			if _, err := fmt.Fprint(w.Output, nl); err != nil {
				return err
			}
		}
		px := linePrefixAt(prefixes, i)
		// Padding line (joinProseLines left this slot empty): okapi
		// emits the bare comment marker without a trailing space, so
		// strip the trailing space from the standard `/// ` marker.
		marker := "/// "
		if line == "" && px == "" {
			marker = "///"
		}
		if _, err := fmt.Fprintf(w.Output, "%s%s%s%s", indent, marker, px, line); err != nil {
			return err
		}
	}
	return nil
}

// writeDocstring writes text as a Python triple-quoted docstring.
func (w *Writer) writeDocstring(text, raw string, _ []string) error {
	indent := extractIndent(raw)
	rawLines := strings.Split(raw, "\n")
	nl := w.lineSep()

	// Single-line docstring: """text"""
	if len(rawLines) == 1 {
		_, err := fmt.Fprintf(w.Output, `%s"""%s"""`, indent, text)
		return err
	}

	// Multi-line: emit opening """ + text on same line if original had it
	trimmedFirst := strings.TrimSpace(rawLines[0])
	idx := strings.Index(trimmedFirst, `"""`)
	afterOpen := strings.TrimSpace(trimmedFirst[idx+3:])
	if afterOpen != "" {
		// Text follows opening """
		if _, err := fmt.Fprintf(w.Output, `%s"""%s`, indent, text); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w.Output, `%s"""`, indent); err != nil {
			return err
		}
	}

	// Middle content lines — not used when layout drives reconstruction;
	// this path handles the no-layout fallback.
	if afterOpen == "" {
		textLines := strings.Split(text, "\n")
		for _, tl := range textLines {
			if _, err := fmt.Fprint(w.Output, nl); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w.Output, "%s%s", indent, tl); err != nil {
				return err
			}
		}
	}

	// Closing """
	if _, err := fmt.Fprint(w.Output, nl); err != nil {
		return err
	}
	// Closing line indentation from the raw
	lastRaw := rawLines[len(rawLines)-1]
	closingIndent := lastRaw[:len(lastRaw)-len(strings.TrimLeft(lastRaw, " \t"))]
	_, err := fmt.Fprintf(w.Output, `%s"""`, closingIndent)
	return err
}

// writeExclamation writes text as //! line comments.
func (w *Writer) writeExclamation(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	lines := strings.Split(text, "\n")
	nl := w.lineSep()
	for i, line := range lines {
		if i > 0 {
			if _, err := fmt.Fprint(w.Output, nl); err != nil {
				return err
			}
		}
		px := linePrefixAt(prefixes, i)
		// Same padding rule as writeTripleSlash — bare marker, no trailing space.
		marker := "//! "
		if line == "" && px == "" {
			marker = "//!"
		}
		if _, err := fmt.Fprintf(w.Output, "%s%s%s%s", indent, marker, px, line); err != nil {
			return err
		}
	}
	return nil
}

// writeJavadoc writes text as a /** ... */ block comment.
func (w *Writer) writeJavadoc(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	rawLines := strings.Split(raw, "\n")
	nl := w.lineSep()

	// Single-line block comment
	if len(rawLines) == 1 {
		px := linePrefixAt(prefixes, 0)
		_, err := fmt.Fprintf(w.Output, "%s/** %s%s */", indent, px, text)
		return err
	}

	// Multi-line block comment
	lines := strings.Split(text, "\n")
	if _, err := fmt.Fprintf(w.Output, "%s/**", indent); err != nil {
		return err
	}
	for i, line := range lines {
		px := linePrefixAt(prefixes, i)
		if _, err := fmt.Fprintf(w.Output, "%s%s * %s%s", nl, indent, px, line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w.Output, "%s%s */", nl, indent)
	return err
}

// writeQt writes text as a /*! ... */ block comment.
func (w *Writer) writeQt(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	rawLines := strings.Split(raw, "\n")
	nl := w.lineSep()

	// Single-line block comment
	if len(rawLines) == 1 {
		px := linePrefixAt(prefixes, 0)
		_, err := fmt.Fprintf(w.Output, "%s/*! %s%s */", indent, px, text)
		return err
	}

	// Multi-line block comment
	lines := strings.Split(text, "\n")
	if _, err := fmt.Fprintf(w.Output, "%s/*!", indent); err != nil {
		return err
	}
	for i, line := range lines {
		px := linePrefixAt(prefixes, i)
		if _, err := fmt.Fprintf(w.Output, "%s%s  %s%s", nl, indent, px, line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w.Output, "%s%s*/", nl, indent)
	return err
}

// writeQtMember writes text as a stand-alone /*!< ... */ block
// comment (Doxygen's "after" / member documentation marker placed at
// the start of the comment rather than after preceding code).
func (w *Writer) writeQtMember(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	rawLines := strings.Split(raw, "\n")
	nl := w.lineSep()
	if len(rawLines) == 1 {
		px := linePrefixAt(prefixes, 0)
		_, err := fmt.Fprintf(w.Output, "%s/*!< %s%s */", indent, px, text)
		return err
	}
	lines := strings.Split(text, "\n")
	if _, err := fmt.Fprintf(w.Output, "%s/*!<", indent); err != nil {
		return err
	}
	for i, line := range lines {
		px := linePrefixAt(prefixes, i)
		if _, err := fmt.Fprintf(w.Output, "%s%s  %s%s", nl, indent, px, line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w.Output, "%s%s*/", nl, indent)
	return err
}

// writeJavadocMember writes text as a stand-alone /**< ... */ block
// comment.
func (w *Writer) writeJavadocMember(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	rawLines := strings.Split(raw, "\n")
	nl := w.lineSep()
	if len(rawLines) == 1 {
		px := linePrefixAt(prefixes, 0)
		_, err := fmt.Fprintf(w.Output, "%s/**< %s%s */", indent, px, text)
		return err
	}
	lines := strings.Split(text, "\n")
	if _, err := fmt.Fprintf(w.Output, "%s/**<", indent); err != nil {
		return err
	}
	for i, line := range lines {
		px := linePrefixAt(prefixes, i)
		if _, err := fmt.Fprintf(w.Output, "%s%s * %s%s", nl, indent, px, line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w.Output, "%s%s */", nl, indent)
	return err
}

// writeTrailing writes text as a trailing ///< comment.
func (w *Writer) writeTrailing(text, _ string) error {
	_, err := fmt.Fprintf(w.Output, "///< %s", text)
	return err
}

// writeTrailingQt writes text as a trailing /*!< ... */ comment.
func (w *Writer) writeTrailingQt(text, _ string) error {
	_, err := fmt.Fprintf(w.Output, "/*!< %s */", text)
	return err
}

// writeTrailingJavadoc writes text as a trailing /**< ... */ comment.
func (w *Writer) writeTrailingJavadoc(text, _ string) error {
	_, err := fmt.Fprintf(w.Output, "/**< %s */", text)
	return err
}

// isStarBodyPrefix reports whether body looks like a continuation
// `*`-style line prefix (leading whitespace, then `*`, then optional
// whitespace) — the canonical block-comment line shape. Returns false
// for opener lines whose prefix contains `/` (e.g. `/** `, `/*! `).
func isStarBodyPrefix(body string) bool {
	i := 0
	for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
		i++
	}
	return i < len(body) && body[i] == '*'
}

// extractIndent returns the leading whitespace from the first line of raw text.
func extractIndent(raw string) string {
	firstLine := raw
	if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
		firstLine = raw[:idx]
	}
	trimmed := strings.TrimLeft(firstLine, " \t")
	return firstLine[:len(firstLine)-len(trimmed)]
}
