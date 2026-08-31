package mdx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Reader implements DataFormatReader for MDX (.mdx) files.
//
// MDX is CommonMark Markdown extended with ESM (`import`/`export`), JSX
// elements/fragments, and `{expression}` braces. Goldmark — the parser the
// markdown reader builds on — does not understand any of these: it parses a
// top-level `import …` line or a capitalised `<Component … />` block as an
// ordinary paragraph, which the markdown reader would then extract as
// translatable prose. That both corrupts the construct under translation
// and (for multi-line JSX/imports) breaks the byte round-trip.
//
// The MDX reader therefore PRE-SEGMENTS the document body at the top level
// into opaque MDX regions (ESM / JSX / expressions) and plain-Markdown
// spans (see scanner.go), then:
//
//   - delegates each Markdown span to a fresh markdown.Reader driven over
//     just that span's bytes with its own SkeletonStore, splicing the
//     span's skeleton (text + block refs, with ref IDs remapped) into the
//     MDX skeleton and re-emitting its Blocks; and
//   - emits each opaque MDX region as verbatim skeleton text (plus a Data
//     part for the non-skeleton write path).
//
// Because opaque regions are copied byte-for-byte and Markdown spans
// round-trip through the proven markdown machinery, an untranslated
// read→write reproduces the source exactly. Translatable scope is limited
// to Markdown prose; ESM statements, JSX tags + attributes + children, and
// expressions are never translated (v1 — see the package doc).
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore

	source       []byte
	blockCounter int
	dataCounter  int
	// promoted records elements the translatability table does not classify
	// whose text was taken as translatable, so the inference is reported once
	// each rather than once per paragraph.
	promoted map[string]bool

	// naming composes structural block names across the WHOLE document. One
	// state, shared with every delegated markdown span, so the heading trail
	// carries over a JSX element and two spans cannot both name a paragraph
	// "p" — see markdown.NamingState.
	naming markdown.NamingState
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new MDX reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		FormatName:        "mdx",
		FormatDisplayName: "MDX",
		FormatMimeType:    "text/mdx",
		FormatExtensions:  []string{".mdx"},
		Cfg:               cfg,
		cfg:               cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/mdx"},
		Extensions: []string{".mdx"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("mdx: nil document or reader")
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
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "mdx",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/mdx",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return ctx.Err()
	}

	// Bound the whole-input read with the shared safeio byte budget so an
	// unbounded/oversized stream fails with a typed error (identical limit
	// across CLI/server/WASM — see core/safeio).
	content, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		return fmt.Errorf("mdx: reading: %w", err)
	}
	bom, content := format.SplitBOM(content)
	r.source = content
	r.skelText(bom)
	r.blockCounter = 0
	r.dataCounter = 0
	r.naming.Reset()

	segs := scanSegments(content)
	// Validate the whole scan before emitting anything: a document whose
	// parse ended early must fail, not deliver a fraction of its blocks.
	if err := checkSegments(segs, content); err != nil {
		return err
	}
	for _, seg := range segs {
		span := content[seg.start:seg.end]
		switch seg.kind {
		case segMarkdown:
			if err := r.emitMarkdownSpan(ctx, ch, span, locale); err != nil {
				return err
			}
		case segESM:
			r.emitOpaque(ctx, ch, span, "esm")
		case segJSX:
			if !r.emitJSX(ctx, ch, span, locale) {
				return ctx.Err()
			}
		case segExpr:
			r.emitOpaque(ctx, ch, span, "expression")
		}
	}

	if r.skeletonStore != nil {
		if err := r.skeletonStore.Flush(); err != nil {
			return err
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
	return nil
}

// emitOpaque records a non-translatable MDX region (ESM statement, JSX
// element/fragment, or expression) verbatim. The bytes go to the skeleton
// stream unchanged so the writer reproduces them exactly, and a Data part
// is emitted so the non-skeleton write path can reconstruct the region
// too.
func (r *Reader) emitOpaque(ctx context.Context, ch chan<- model.PartResult, span []byte, name string) {
	r.dataCounter++
	data := &model.Data{
		ID:   fmt.Sprintf("d%d", r.dataCounter),
		Name: "mdx-" + name,
		Properties: map[string]string{
			"content": string(span),
			"kind":    name,
		},
	}
	r.skelText(span)
	r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
}

// spanSkelEntry is one entry from a delegated markdown span's skeleton:
// either verbatim text, or a reference to one of the span's blocks (by
// the markdown reader's original block ID).
type spanSkelEntry struct {
	isRef bool
	text  []byte
	refID string
}

// emitMarkdownSpan splits a plain-Markdown span at GFM table boundaries,
// then dispatches each sub-span: table blocks go to emitOpaque (the
// markdown reader normalises table cell padding for Okapi parity rather
// than preserving it, which would break the MDX byte-faithful round-trip,
// so tables are preserved verbatim and not translated in v1), and
// non-table sub-spans go to emitMarkdownProse for prose extraction.
func (r *Reader) emitMarkdownSpan(ctx context.Context, ch chan<- model.PartResult, span []byte, locale model.LocaleID) error {
	if len(span) == 0 {
		return nil
	}
	for _, sub := range splitMarkdownTables(span) {
		subSpan := span[sub.start:sub.end]
		if sub.isTable {
			if !r.emitTable(ctx, ch, subSpan, locale) {
				return ctx.Err()
			}
			continue
		}
		if err := r.emitMarkdownProse(ctx, ch, subSpan, locale); err != nil {
			return err
		}
	}
	return nil
}

// emitMarkdownProse delegates a table-free Markdown sub-span to a fresh
// markdown.Reader, captures its Blocks and skeleton, and VERIFIES the
// sub-span reconstructs byte-for-byte from that skeleton when untranslated.
//
//   - If it does, the sub-span is spliced into the MDX skeleton (block-ref
//     IDs remapped onto the MDX counter namespace) and its Blocks are
//     emitted as translatable — prose round-trips and translates exactly
//     as `.md`.
//   - If it does NOT (a residual markdown round-trip imperfection not
//     covered by table splitting), the sub-span is emitted as ONE opaque
//     region (verbatim skeleton text + a Data part) with no translatable
//     blocks, keeping the byte-faithful round-trip — the PRIMARY
//     acceptance bar — unconditional.
//
// The markdown reader's LayerStart/LayerEnd parts are dropped — MDX owns
// its document layer. In the non-skeleton fallback path (no MDX skeleton
// store) the markdown reader is still run and its Blocks/Data forwarded.
func (r *Reader) emitMarkdownProse(ctx context.Context, ch chan<- model.PartResult, span []byte, locale model.LocaleID) error {
	if len(span) == 0 {
		return nil
	}

	mdReader := markdown.NewReader()
	// One naming state for the whole MDX document: this is one of several spans
	// of it, and a per-span state would restart the heading trail and hand two
	// spans the same names.
	mdReader.ShareNaming(&r.naming)
	// MDX composes the markdown reader, whose default surfaces code blocks as
	// non-translatable content. MDX-specific content surfacing (code fences, JSX
	// text, table cells) is tracked separately (#928); keep the embedded markdown
	// behaviour unchanged here so code stays skeleton, then let any explicit
	// config override.
	mdReader.MarkdownConfig().SetExtractNonTranslatableContent(false)
	if err := r.cfg.applyTo(mdReader.MarkdownConfig()); err != nil {
		return fmt.Errorf("mdx: applying markdown config: %w", err)
	}

	var subStore *format.SkeletonStore
	if r.skeletonStore != nil {
		var err error
		subStore, err = format.NewSkeletonStore()
		if err != nil {
			return fmt.Errorf("mdx: sub-skeleton store: %w", err)
		}
		defer func() { _ = subStore.Close() }()
		mdReader.SetSkeletonStore(subStore)
	}

	doc := &model.RawDocument{
		URI:          r.Doc.URI,
		Reader:       io.NopCloser(bytes.NewReader(span)),
		SourceLocale: locale,
		Encoding:     r.Doc.Encoding,
	}
	if err := mdReader.Open(ctx, doc); err != nil {
		return fmt.Errorf("mdx: opening markdown span: %w", err)
	}

	// Drain the markdown reader, collecting blocks (keyed by original ID)
	// and data WITHOUT emitting yet — emission is deferred until after the
	// faithfulness check below.
	blocksByOrigID := make(map[string]*model.Block)
	var blocks []*model.Block
	var dataParts []*model.Data
	for pr := range mdReader.Read(ctx) {
		if pr.Error != nil {
			return fmt.Errorf("mdx: markdown span: %w", pr.Error)
		}
		switch pr.Part.Type {
		case model.PartBlock:
			if block, ok := pr.Part.Resource.(*model.Block); ok {
				blocksByOrigID[block.ID] = block
				blocks = append(blocks, block)
			}
		case model.PartData:
			if data, ok := pr.Part.Resource.(*model.Data); ok {
				dataParts = append(dataParts, data)
			}
		default:
			// LayerStart / LayerEnd / Media — drop.
		}
	}

	// Non-skeleton fallback: no faithfulness check possible (the writer's
	// fallback path is best-effort anyway). Emit blocks and data re-ID'd.
	if r.skeletonStore == nil || subStore == nil {
		for _, block := range blocks {
			r.blockCounter++
			block.ID = fmt.Sprintf("tu%d", r.blockCounter)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return ctx.Err()
			}
		}
		for _, data := range dataParts {
			r.dataCounter++
			data.ID = fmt.Sprintf("d%d", r.dataCounter)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return ctx.Err()
			}
		}
		return nil
	}

	// Read the span's skeleton entries.
	if err := subStore.Flush(); err != nil {
		return fmt.Errorf("mdx: flush sub-skeleton: %w", err)
	}
	var entries []spanSkelEntry
	for {
		entry, err := subStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("mdx: read sub-skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			entries = append(entries, spanSkelEntry{text: append([]byte(nil), entry.Data...)})
		case format.SkeletonRef:
			entries = append(entries, spanSkelEntry{isRef: true, refID: string(entry.Data)})
		default:
			// SkeletonLang etc. — markdown never emits these for a plain
			// span; treat as inert text-less entry (skipped on rebuild).
		}
	}

	// Reconstruct the untranslated span from blocks + skeleton and compare
	// to the source bytes. Only splice when byte-identical.
	if ok, div := r.spanReconstructsExactly(span, entries, blocksByOrigID); !ok {
		// Before surrendering the span, try to isolate the blocks that actually
		// diverged. One unsupported construct costing its own block is a very
		// different thing from one costing a whole page, and until now it cost
		// the page: a checklist at the bottom of a release note took every
		// heading and paragraph above it out of the translation.
		if rewritten, quarantined, salvaged := quarantineDivergentBlocks(
			span, entries, blocksByOrigID,
		); salvaged && len(quarantined) < len(blocks) {
			r.recordBlockQuarantine(span, div, len(quarantined), len(blocks))
			entries = rewritten
			for _, block := range blocks {
				if quarantined[block.ID] {
					block.Translatable = false
				}
			}
		} else {

			// Fall back to a single opaque region (verbatim skeleton + Data) so
			// the byte-exact round-trip is preserved regardless of flag.
			//
			// Say so. The fallback costs this span every translatable block it
			// had, and it is the right trade only while somebody can see it being
			// made: a whole document can go opaque for one construct, and the
			// symptom is a page that renders in the source language with no error
			// anywhere. Recorded unconditionally rather than under ValidationMode,
			// because the runs that most need to hear it are ordinary ones.
			r.recordOpaqueFallback(span, div)
			r.emitOpaque(ctx, ch, span, "markdown-opaque")
			// When content surfacing is on, ALSO expose the prose the markdown
			// sub-reader already parsed as non-translatable content (#928,
			// visible to ingestion, skipped by MT). These blocks carry NO
			// skeleton ref — the opaque region above owns the verbatim bytes —
			// so they never affect the round-trip; with the flag off the part
			// stream is unchanged (just the opaque Data above).
			if r.cfg.ExtractNonTranslatableContent() {
				for _, block := range blocks {
					r.blockCounter++
					block.ID = fmt.Sprintf("tu%d", r.blockCounter)
					block.Translatable = false
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return ctx.Err()
					}
				}
			}
			return nil
		}
	}

	// Faithful: emit blocks (re-ID'd) and splice the skeleton with refs
	// remapped to the new IDs. A quarantined block arrives here too, carrying
	// Translatable=false, and its ref is already a literal entry.
	idMap := make(map[string]string, len(blocks))
	for _, block := range blocks {
		orig := block.ID
		r.blockCounter++
		newID := fmt.Sprintf("tu%d", r.blockCounter)
		idMap[orig] = newID
		block.ID = newID
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return ctx.Err()
		}
	}
	for _, entry := range entries {
		if entry.isRef {
			if mapped, ok := idMap[entry.refID]; ok {
				r.skelRef(mapped)
			}
			continue
		}
		r.skelText(entry.text)
	}
	return nil
}

// spanReconstructsExactly reports whether replaying the span's skeleton
// entries with each block's source runs reproduces span byte-for-byte.
// This is exactly what the writer does for untranslated output, so a true
// result guarantees the span round-trips faithfully.
// spanDivergence locates the first byte at which a span failed to reconstruct,
// which is the whole diagnostic: a reader that only says "this span did not
// round-trip" leaves the reader of that message exactly where it started.
type spanDivergence struct {
	// offset is the byte offset into the span of the first difference.
	offset int
	// want and got are short excerpts from that offset, source and rebuild.
	want string
	got  string
	// reason names the failure when it is not a byte difference.
	reason string
}

func (r *Reader) spanReconstructsExactly(span []byte, entries []spanSkelEntry, blocks map[string]*model.Block) (bool, spanDivergence) {
	var buf bytes.Buffer
	for _, entry := range entries {
		if entry.isRef {
			block, ok := blocks[entry.refID]
			if !ok {
				return false, spanDivergence{
					offset: buf.Len(),
					want:   excerptAt(span, buf.Len()),
					reason: fmt.Sprintf("skeleton references block %q, which the markdown reader did not emit", entry.refID),
				}
			}
			buf.WriteString(renderBlockSource(block))
			continue
		}
		buf.Write(entry.text)
	}
	rebuilt := buf.Bytes()
	if bytes.Equal(rebuilt, span) {
		return true, spanDivergence{}
	}
	off := firstDifference(rebuilt, span)
	return false, spanDivergence{
		offset: off,
		want:   excerptAt(span, off),
		got:    excerptAt(rebuilt, off),
	}
}

// quarantineDivergentBlocks salvages a span that did not reconstruct, by
// isolating the blocks that failed instead of surrendering the whole span.
//
// It replays the skeleton against the source with a cursor. A literal entry has
// to match at the cursor; a block's rendered source has to match at the cursor.
// A block that does not is QUARANTINED: its ref becomes a literal entry holding
// the source bytes it actually occupied, so those bytes still round-trip while
// every other block in the span keeps its translatability.
//
// The result is byte-exact by construction — each emitted entry is either an
// entry that matched the source at its position or a verbatim slice of it, and
// the cursor walks the span exactly once. What can go wrong is attribution, not
// output: if a landmark resynchronises early, a block may take a neighbour's
// bytes with it and be reported as divergent when its neighbour was. That
// yields more untranslatable content, never wrong content, and if the cursor
// cannot be reconciled the caller falls back to the whole-span behaviour.
func quarantineDivergentBlocks(
	span []byte, entries []spanSkelEntry, blocks map[string]*model.Block,
) ([]spanSkelEntry, map[string]bool, bool) {
	out := make([]spanSkelEntry, 0, len(entries))
	quarantined := make(map[string]bool)
	cursor := 0

	for i, entry := range entries {
		if !entry.isRef {
			// The skeleton's own literal text diverging means the reader built
			// a skeleton that does not describe this source. Nothing to salvage
			// block by block.
			if !bytes.HasPrefix(span[cursor:], entry.text) {
				return nil, nil, false
			}
			cursor += len(entry.text)
			out = append(out, entry)
			continue
		}
		block, ok := blocks[entry.refID]
		if !ok {
			return nil, nil, false
		}
		rendered := []byte(renderBlockSource(block))
		if bytes.HasPrefix(span[cursor:], rendered) {
			cursor += len(rendered)
			out = append(out, entry)
			continue
		}
		end, ok := resyncToLandmark(span, cursor, entries[i+1:])
		if !ok {
			return nil, nil, false
		}
		quarantined[entry.refID] = true
		out = append(out, spanSkelEntry{text: append([]byte(nil), span[cursor:end]...)})
		cursor = end
	}

	if cursor != len(span) || len(quarantined) == 0 {
		return nil, nil, false
	}
	return out, quarantined, true
}

// resyncToLandmark returns where the next literal skeleton entry begins in span
// at or after cursor — the boundary of the block being quarantined. With no
// literal entry left, the block runs to the end of the span.
func resyncToLandmark(span []byte, cursor int, rest []spanSkelEntry) (int, bool) {
	for _, e := range rest {
		if e.isRef || len(e.text) == 0 {
			continue
		}
		idx := bytes.Index(span[cursor:], e.text)
		if idx < 0 {
			return 0, false
		}
		return cursor + idx, true
	}
	return len(span), true
}

// recordBlockQuarantine reports the salvaged case: some blocks lost their
// translatability, the rest kept it. Distinct from the whole-span fallback
// because the loss is bounded, and worth saying anyway — a block that stays in
// the source language is invisible in the output.
func (r *Reader) recordBlockQuarantine(span []byte, div spanDivergence, lost, total int) {
	line, col := format.LineColumn(r.source, r.spanOffset(span)+div.offset)
	detail := div.reason
	if detail == "" {
		detail = fmt.Sprintf("source has %q, rebuild has %q", div.want, div.got)
	}
	r.AddDiagnostic(format.Diagnostic{
		Severity: format.SeverityMajor,
		Category: "structure.markdown-block-opaque",
		Message: fmt.Sprintf(
			"%d of %d blocks in this span did not reconstruct byte-for-byte and "+
				"will stay in the source language; the rest still translate: %s",
			lost, total, detail),
		Line:       line,
		Column:     col,
		ByteOffset: r.spanOffset(span) + div.offset,
	})
}

// recordOpaqueFallback reports a span that lost every translatable block to the
// round-trip guard, locating the divergence so the cause is a lookup rather
// than a bisect.
func (r *Reader) recordOpaqueFallback(span []byte, div spanDivergence) {
	line, col := format.LineColumn(r.source, r.spanOffset(span)+div.offset)
	detail := div.reason
	if detail == "" {
		detail = fmt.Sprintf("source has %q, rebuild has %q", div.want, div.got)
	}
	r.AddDiagnostic(format.Diagnostic{
		Severity: format.SeverityMajor,
		Category: "structure.markdown-span-opaque",
		Message: "this span did not reconstruct byte-for-byte, so it carries no " +
			"translatable content and will stay in the source language: " + detail,
		Line:       line,
		Column:     col,
		ByteOffset: r.spanOffset(span) + div.offset,
		Snippet:    div.want,
	})
}

// spanOffset locates a span within the document, so a diagnostic reports a
// position in the file rather than in a fragment nobody can see.
func (r *Reader) spanOffset(span []byte) int {
	if len(r.source) == 0 || len(span) == 0 {
		return 0
	}
	if i := bytes.Index(r.source, span); i >= 0 {
		return i
	}
	return 0
}

// firstDifference returns the index of the first differing byte, or the length
// of the shorter slice when one is a prefix of the other.
func firstDifference(a, b []byte) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// excerptAt returns a short, single-line excerpt of b starting at off, so a
// diagnostic can show what diverged without printing a document.
func excerptAt(b []byte, off int) string {
	if off < 0 || off > len(b) {
		return ""
	}
	end := min(off+48, len(b))
	// %q at the call site escapes newlines already.
	return string(b[off:end])
}

// renderBlockSource renders a block's source runs the way the writer does
// for untranslated output, including the markdown line-prefix property so
// multi-line blockquote/list continuations reconstruct exactly.
func renderBlockSource(block *model.Block) string {
	if len(block.Source) == 0 {
		return ""
	}
	return markdown.RenderBlockContent(block, block.Source)
}

// --- Skeleton helpers (mirror the markdown reader's coalescing pattern) ---

func (r *Reader) skelText(b []byte) {
	if r.skeletonStore != nil && len(b) > 0 {
		r.skeletonStore.WriteText(b)
	}
}

func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		r.skeletonStore.WriteRef(id)
	}
}

// emit sends a part to the channel, honouring context cancellation.
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
