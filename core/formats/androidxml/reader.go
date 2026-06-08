package androidxml

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Android string resources
// (res/values/strings.xml).
//
// Output strategy mirrors the native resx/xcstrings readers: the whole document
// is tokenized losslessly and the original bytes are stored on the root Layer so
// the writer can produce byte-faithful output by splicing only changed values.
//
// Translatable entries become Blocks:
//   - <string name="x">…</string>           → one Block (name = x)
//   - <string-array name="x"><item>…</item>  → one Block per item (name = x[i])
//   - <plurals name="x"><item quantity="q">  → one Block per item (name = x[q])
//
// Entries with translatable="false", bare resource references, and everything
// outside the translatable vocabulary (declare-styleable, comments, the prolog)
// round-trip verbatim and are never extracted. Inline %1$s/%d printf arguments,
// <xliff:g> spans, CDATA, and inline styling tags are protected as inline codes.
type Reader struct {
	format.BaseFormatReader
	cfg *Config

	// skeletonStore, when non-nil, captures a byte-exact skeleton of the
	// document interleaving verbatim structure (SkeletonText) with per-block
	// content placeholders (SkeletonRef). This is the path `kapi merge` uses:
	// the source is re-read with no skeleton (reader), the captured skeleton is
	// opened and handed to the WRITER, and the writer splices each block's runs
	// into the SkeletonRef slots — so the androidxml.original layer property is
	// not involved on the merge path. skelBuf accumulates raw bytes between
	// refs; skelFlush writes them out as one SkeletonText entry.
	skeletonStore *format.SkeletonStore
	skelBuf       strings.Builder
}

// Ensure Reader implements SkeletonStoreEmitter so `kapi extract` can capture a
// byte-exact skeleton for `kapi merge`.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new Android string-resources reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "androidxml",
			FormatDisplayName: "Android String Resources",
			FormatExtensions:  []string{".xml"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetConfig applies a new configuration, keeping the typed config field in sync
// with the embedded base so the reader's behaviour reflects callers that swap in
// a fresh *Config via SetConfig (the registry path mutates Config() in place).
func (r *Reader) SetConfig(cfg format.DataFormatConfig) error {
	if err := r.BaseFormatReader.SetConfig(cfg); err != nil {
		return err
	}
	if c, ok := cfg.(*Config); ok {
		r.cfg = c
	}
	return nil
}

// SetSkeletonStore wires a skeleton store so the reader emits a byte-exact
// skeleton (verbatim structure + per-block content refs) instead of relying on
// the androidxml.original layer property. When set, the walk routes every
// non-translatable byte to the skeleton verbatim and replaces each translatable
// element's inner content with a SkeletonRef keyed by the block ID.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// skelText appends verbatim bytes to the pending skeleton-text buffer.
func (r *Reader) skelText(raw string) {
	if r.skeletonStore == nil {
		return
	}
	r.skelBuf.WriteString(raw)
}

// skelTokens appends the verbatim bytes of every token in a span.
func (r *Reader) skelTokens(toks []token) {
	if r.skeletonStore == nil {
		return
	}
	for _, t := range toks {
		r.skelBuf.WriteString(t.raw)
	}
}

// skelRef flushes any pending skeleton text and writes a content placeholder for
// the given block ID. The writer substitutes the block's (target-or-source)
// runs into this slot, re-serialising inline xliff:g/CDATA/printf via
// renderRunsToXML and XML-escaping text via encodeText.
func (r *Reader) skelRef(blockID string) {
	if r.skeletonStore == nil {
		return
	}
	r.skelFlush()
	_ = r.skeletonStore.WriteRef(blockID)
}

// skelFlush writes any buffered verbatim bytes as one SkeletonText entry.
func (r *Reader) skelFlush() {
	if r.skeletonStore == nil {
		return
	}
	if r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText([]byte(r.skelBuf.String()))
		r.skelBuf.Reset()
	}
}

// blockID returns the block ID for the counter value, matching newBlock's
// scheme. Used by the skeleton walk to key refs identically to the blocks the
// same walk emits, and identically to a re-read of the same document at merge
// time (the counter advances in element order in both cases).
func blockID(counter int) string {
	return "tu" + strconv.Itoa(counter)
}

// Signature returns detection metadata for this format. The .xml extension is
// owned by the generic xml format, so Android resources are detected by Sniff
// only — never by extension or MIME (which would collide with generic XML).
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		Sniff: Sniff,
	}
}

// Sniff reports whether the given bytes look like an Android string-resources
// file: a <resources> root carrying at least one translatable element. It is
// deliberately conservative so it does not steal arbitrary XML documents.
func Sniff(data []byte) bool {
	s := string(data)
	if !strings.Contains(s, "<resources") {
		return false
	}
	return strings.Contains(s, "<string ") ||
		strings.Contains(s, "<string>") ||
		strings.Contains(s, "<string-array") ||
		strings.Contains(s, "<plurals")
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("androidxml: nil document or reader")
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

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("androidxml: reading: %w", err)}
		return
	}

	toks, err := newTokenizer(string(content)).tokenize()
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("androidxml: %w", err)}
		return
	}

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:         "doc1",
		Name:       r.Doc.URI,
		Format:     "androidxml",
		Locale:     locale,
		Encoding:   r.Doc.Encoding,
		MimeType:   "application/xml",
		Properties: map[string]string{},
	}
	// Preserve the original document bytes so the writer can produce
	// byte-faithful output, splicing only changed values. unsafe.String shares
	// the backing array — content is not mutated after this point. When a
	// skeleton store is wired (the `kapi extract`/`kapi merge` path), the
	// skeleton carries the verbatim structure instead, so we skip the property —
	// merge discards the layer property anyway, re-reading the source fresh.
	if r.skeletonStore == nil {
		layer.Properties["androidxml.original"] = unsafe.String(unsafe.SliceData(content), len(content))
	}

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	r.walk(ctx, ch, toks)

	// Flush any trailing verbatim bytes after the last block ref.
	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// walk scans the top-level token stream, emitting Blocks for translatable
// <string>, <string-array>, and <plurals> entries. It tracks the most recent
// preceding comment so it can attach it as a translator note.
func (r *Reader) walk(ctx context.Context, ch chan<- model.PartResult, toks []token) {
	counter := 0
	r.walkSpan(ctx, ch, toks, &counter)
}

// walkSpan walks one token span (the whole document or the inner content of a
// container element), threading a shared block counter so ids stay unique across
// nested spans.
func (r *Reader) walkSpan(ctx context.Context, ch chan<- model.PartResult, toks []token, counter *int) {
	pendingComment := "" // most recent comment seen since the last non-trivial token

	for i := 0; i < len(toks); i++ {
		t := toks[i]

		switch t.kind {
		case tokComment:
			pendingComment = commentText(t.raw)
			r.skelText(t.raw)
			continue
		case tokText:
			// Whitespace-only text between a comment and its entry keeps the
			// comment pending; any other text clears it.
			if strings.TrimSpace(t.raw) != "" {
				pendingComment = ""
			}
			r.skelText(t.raw)
			continue
		case tokStartTag:
			switch t.name {
			case "string":
				end := matchEnd(toks, i, "string")
				if end < 0 {
					// Unbalanced — keep the lone start tag verbatim.
					r.skelText(t.raw)
					continue
				}
				if r.isTranslatable(t) && !r.isReferenceValue(toks[i+1:end]) {
					*counter++
					// Skeleton: start tag verbatim, a ref for the inner content,
					// then the end tag verbatim.
					r.skelText(t.raw)
					r.skelRef(blockID(*counter))
					r.skelText(toks[end].raw)
					if !r.emitString(ctx, ch, t, toks[i+1:end], pendingComment, *counter) {
						return
					}
				} else {
					// Non-translatable <string> (translatable="false" or a bare
					// resource reference): the whole entry round-trips verbatim.
					r.skelTokens(toks[i : end+1])
				}
				pendingComment = ""
				i = end
			case "string-array":
				end := matchEnd(toks, i, "string-array")
				if end < 0 {
					r.skelText(t.raw)
					continue
				}
				if r.isTranslatable(t) {
					// Container start tag verbatim; emitArray emits per-item
					// structure + refs; the matching end tag is verbatim.
					r.skelText(t.raw)
					if !r.emitArray(ctx, ch, t, toks[i+1:end], pendingComment, counter) {
						return
					}
					r.skelText(toks[end].raw)
				} else {
					r.skelTokens(toks[i : end+1])
				}
				pendingComment = ""
				i = end
			case "plurals":
				end := matchEnd(toks, i, "plurals")
				if end < 0 {
					r.skelText(t.raw)
					continue
				}
				if r.isTranslatable(t) {
					r.skelText(t.raw)
					if !r.emitPlurals(ctx, ch, t, toks[i+1:end], pendingComment, counter) {
						return
					}
					r.skelText(toks[end].raw)
				} else {
					r.skelTokens(toks[i : end+1])
				}
				pendingComment = ""
				i = end
			case "resources":
				// The document container: descend into its children. (A nested
				// <resources> is not valid Android, but recursing is harmless.)
				end := matchEnd(toks, i, "resources")
				pendingComment = ""
				if end > i {
					// Container start tag is verbatim structure; recurse for the
					// inner span; the matching end tag is verbatim too.
					r.skelText(t.raw)
					r.walkSpan(ctx, ch, toks[i+1:end], counter)
					r.skelText(toks[end].raw)
					i = end
				} else {
					r.skelText(t.raw)
				}
			default:
				// Any other element (declare-styleable, color, dimen, style, …)
				// is non-translatable structure: round-trip its whole span
				// verbatim and clear the pending comment.
				end := matchEnd(toks, i, t.name)
				pendingComment = ""
				if end > i {
					r.skelTokens(toks[i : end+1])
					i = end
				} else {
					r.skelText(t.raw)
				}
			}
		case tokSelfClose:
			pendingComment = ""
			r.skelText(t.raw)
		default:
			// End tags, CDATA, PIs, the prolog/doctype, and any other top-level
			// token are verbatim structure. (Well-formed Android resources keep
			// these between entries, e.g. the <?xml ?> prolog and stray
			// whitespace handled above; this is a safety net for anything else.)
			r.skelText(t.raw)
		}
	}
}

// isTranslatable reports whether an entry start tag is translatable, honouring
// translatable="false".
func (r *Reader) isTranslatable(start token) bool {
	if !r.cfg.SkipNonTranslatable {
		return true
	}
	if v, ok := start.attrValue("translatable"); ok && v == "false" {
		return false
	}
	return true
}

// isReferenceValue reports whether a <string>'s inner span is a bare resource
// reference (the config gate is applied here).
func (r *Reader) isReferenceValue(innerToks []token) bool {
	if !r.cfg.SkipResourceReferences {
		return false
	}
	// Only a single text token can be a reference; markup means real content.
	var text strings.Builder
	for _, t := range innerToks {
		switch t.kind {
		case tokText:
			text.WriteString(decodeEntities(t.raw))
		default:
			return false
		}
	}
	return isResourceReference(text.String())
}

// emitString emits one Block for a <string> entry.
//
// Android allows two <string> entries to share a @name when they carry distinct
// @product qualifiers (e.g. product="tablet" vs product="default"); the build
// selects one at packaging time. To keep such siblings addressable and
// byte-faithful, a product-qualified entry's Block.Name is suffixed "name@product"
// (mirroring how plurals/arrays use name[quantity]/name[index]), and the product
// is recorded in Properties so the writer can match the exact element.
func (r *Reader) emitString(ctx context.Context, ch chan<- model.PartResult,
	start token, innerToks []token, comment string, counter int) bool {

	name, _ := start.attrValue("name")
	blockName := name
	if product, ok := start.attrValue("product"); ok && product != "" {
		blockName = name + "@" + product
	}
	block := r.newBlock(counter, blockName, buildRuns(innerToks))
	r.applyEntryProps(block, start, "string")
	r.applyComment(block, comment)
	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// emitArray emits one Block per <item> in a <string-array> entry.
func (r *Reader) emitArray(ctx context.Context, ch chan<- model.PartResult,
	start token, innerToks []token, comment string, counter *int) bool {

	name, _ := start.attrValue("name")
	idx := 0
	for i := 0; i < len(innerToks); i++ {
		t := innerToks[i]
		if t.kind != tokStartTag || t.name != "item" {
			// Inter-item structure (whitespace, comments, anything non-item)
			// round-trips verbatim in the skeleton.
			r.skelText(t.raw)
			continue
		}
		end := matchEndInner(innerToks, i, "item")
		if end < 0 {
			r.skelText(t.raw)
			continue
		}
		*counter++
		// Skeleton: <item ...> verbatim, a ref for the item's inner content,
		// then </item> verbatim.
		r.skelText(t.raw)
		r.skelRef(blockID(*counter))
		r.skelText(innerToks[end].raw)
		blockName := name + "[" + strconv.Itoa(idx) + "]"
		block := r.newBlock(*counter, blockName, buildRuns(innerToks[i+1:end]))
		r.applyEntryProps(block, start, "string-array")
		block.Properties["androidxml.arrayName"] = name
		block.Properties["androidxml.index"] = strconv.Itoa(idx)
		// Only the first item carries the entry comment as a note.
		if idx == 0 {
			r.applyComment(block, comment)
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
		idx++
		i = end
	}
	return true
}

// emitPlurals emits one Block per <item quantity="…"> in a <plurals> entry.
func (r *Reader) emitPlurals(ctx context.Context, ch chan<- model.PartResult,
	start token, innerToks []token, comment string, counter *int) bool {

	name, _ := start.attrValue("name")
	first := true
	for i := 0; i < len(innerToks); i++ {
		t := innerToks[i]
		if t.kind != tokStartTag || t.name != "item" {
			r.skelText(t.raw)
			continue
		}
		end := matchEndInner(innerToks, i, "item")
		if end < 0 {
			r.skelText(t.raw)
			continue
		}
		quantity, _ := t.attrValue("quantity")
		*counter++
		// Skeleton: <item quantity="…"> verbatim, a ref for the inner content,
		// then </item> verbatim.
		r.skelText(t.raw)
		r.skelRef(blockID(*counter))
		r.skelText(innerToks[end].raw)
		blockName := name + "[" + quantity + "]"
		block := r.newBlock(*counter, blockName, buildRuns(innerToks[i+1:end]))
		r.applyEntryProps(block, start, "plurals")
		block.Properties["androidxml.pluralsName"] = name
		block.Properties["androidxml.quantity"] = quantity
		if first {
			r.applyComment(block, comment)
			first = false
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
		i = end
	}
	return true
}

// newBlock constructs a translatable Block from a run sequence.
func (r *Reader) newBlock(counter int, name string, runs []model.Run) *model.Block {
	return &model.Block{
		ID:           "tu" + strconv.Itoa(counter),
		Name:         name,
		Translatable: true,
		SourceLocale: r.docLocale(),
		Source:       runs,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
	}
}

// applyEntryProps records the entry kind on the Block so the writer and tooling
// can identify it. The raw @name and any @product qualifier are stored so the
// writer can match the exact source element (two same-@name entries are
// distinguished by @product — see emitString).
func (r *Reader) applyEntryProps(b *model.Block, start token, kind string) {
	b.Properties["androidxml.kind"] = kind
	if name, ok := start.attrValue("name"); ok {
		b.Properties["androidxml.name"] = name
	}
	if product, ok := start.attrValue("product"); ok && product != "" {
		b.Properties["androidxml.product"] = product
	}
}

// applyComment attaches a preceding XML comment as a developer note when comment
// extraction is enabled and the comment carries text.
func (r *Reader) applyComment(b *model.Block, comment string) {
	if !r.cfg.ExtractComments {
		return
	}
	if strings.TrimSpace(comment) == "" {
		return
	}
	b.SetAnno("note", &model.NoteAnnotation{
		Text:      strings.TrimSpace(comment),
		From:      "developer",
		Annotates: "general",
	})
}

func (r *Reader) docLocale() model.LocaleID {
	if !r.Doc.SourceLocale.IsEmpty() {
		return r.Doc.SourceLocale
	}
	return model.LocaleEnglish
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// commentText extracts the inner text of an XML comment token (between "<!--"
// and "-->").
func commentText(raw string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(raw, "<!--"), "-->")
	return inner
}
