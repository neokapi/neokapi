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
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new MDX reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "mdx",
			FormatDisplayName: "MDX",
			FormatMimeType:    "text/mdx",
			FormatExtensions:  []string{".mdx"},
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
		return nil
	}

	// Bound the whole-input read with the shared safeio byte budget so an
	// unbounded/oversized stream fails with a typed error (identical limit
	// across CLI/server/WASM — see core/safeio).
	content, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		return fmt.Errorf("mdx: reading: %w", err)
	}
	r.source = content

	segs := scanSegments(content)
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
			r.emitOpaque(ctx, ch, span, "jsx")
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
			r.emitOpaque(ctx, ch, subSpan, "table")
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
				return nil
			}
		}
		for _, data := range dataParts {
			r.dataCounter++
			data.ID = fmt.Sprintf("d%d", r.dataCounter)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return nil
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
	if !r.spanReconstructsExactly(span, entries, blocksByOrigID) {
		// Fall back to a single opaque region for this span.
		r.emitOpaque(ctx, ch, span, "markdown-opaque")
		return nil
	}

	// Faithful: emit blocks (re-ID'd) and splice the skeleton with refs
	// remapped to the new IDs.
	idMap := make(map[string]string, len(blocks))
	for _, block := range blocks {
		orig := block.ID
		r.blockCounter++
		newID := fmt.Sprintf("tu%d", r.blockCounter)
		idMap[orig] = newID
		block.ID = newID
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return nil
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
func (r *Reader) spanReconstructsExactly(span []byte, entries []spanSkelEntry, blocks map[string]*model.Block) bool {
	var buf bytes.Buffer
	for _, entry := range entries {
		if entry.isRef {
			block, ok := blocks[entry.refID]
			if !ok {
				return false
			}
			buf.WriteString(renderBlockSource(block))
			continue
		}
		buf.Write(entry.text)
	}
	return bytes.Equal(buf.Bytes(), span)
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
		_ = r.skeletonStore.WriteText(b)
	}
}

func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		_ = r.skeletonStore.WriteRef(id)
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
