package arb

import (
	"bytes"
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

// Reader implements DataFormatReader for Flutter Application Resource Bundle
// (.arb) files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new ARB reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "arb",
			FormatDisplayName: "Flutter ARB",
			FormatMimeType:    "application/json",
			FormatExtensions:  []string{".arb"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/json"},
		Extensions: []string{".arb"},
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output. When
// set, the reader emits a byte-exact skeleton (every structural byte as Text,
// each translatable message value as a Ref) instead of stashing the original
// document bytes on the layer.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// skelText appends text to the skeleton buffer if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil {
		r.skelBuf.WriteString(s)
	}
}

// skelRef flushes buffered text and writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

// skelToken appends a token's prefix and raw bytes to the skeleton buffer.
func (r *Reader) skelToken(tok token) {
	if r.skeletonStore != nil {
		r.skelBuf.WriteString(tok.prefix)
		r.skelBuf.WriteString(tok.raw)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("arb: nil document or reader")
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

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("arb: reading: %w", err)}
		return
	}

	cat, err := parseCatalog(content)
	if err != nil {
		ch <- model.PartResult{Error: err}
		return
	}

	// ARB is monolingual: the file's locale comes from "@@locale", falling back
	// to the document's declared source locale, then English. The message value
	// is treated as content in this locale.
	locale := model.LocaleID(cat.locale)
	if locale.IsEmpty() {
		locale = r.Doc.SourceLocale
	}
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "arb",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/json",
		Properties: map[string]string{
			"arb.locale": cat.locale,
		},
	}
	// Preserve the original document bytes so the writer can produce
	// byte-faithful output, splicing only changed values. unsafe.String shares
	// the backing array — content is not mutated after this point.
	//
	// When a skeleton store is wired (the kapi merge path) the writer rebuilds
	// the document from the skeleton instead, so we skip the original-bytes
	// property — mirroring the native JSON reader.
	if r.skeletonStore == nil {
		layer.Properties["arb.original"] = unsafe.String(unsafe.SliceData(content), len(content))
	}

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Build a key→block-ID map so the skeleton token walk can emit the same
	// "tu<n>" IDs the catalog walk assigns. Message keys are emitted in
	// cat.keyOrder (document order), so both walks agree on the counter.
	blockIDByKey := make(map[string]string, len(cat.keyOrder))
	blockCounter := 0
	for _, key := range cat.keyOrder {
		res := cat.resources[key]
		blockCounter++
		block := r.blockFor(res, locale, blockCounter)
		blockIDByKey[key] = block.ID
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	// Emit the byte-exact skeleton: a token walk that writes every structural
	// byte verbatim and stands a Ref in for each translatable message value.
	if r.skeletonStore != nil {
		r.emitSkeleton(content, blockIDByKey)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// emitSkeleton walks the document's token stream (mirroring rewrite.go's
// walkTop) and streams a byte-exact skeleton to the store. Every token's
// prefix+raw bytes are written as skeleton Text, except each translatable
// message value, where a Ref to the value's block stands in for its serialized
// bytes. Non-translatable keys ("@<id>" attribute objects and "@@<global>"
// metadata) and all structure/whitespace become Text.
//
// On any tokenizer error the skeleton path is left incomplete and the writer's
// EntriesWritten check (used by the merge wiring) lets the caller fall back to
// the non-skeleton writer. Real .arb files always tokenize — parseCatalog above
// would have already failed on malformed input before we reach here.
func (r *Reader) emitSkeleton(content []byte, blockIDByKey map[string]string) {
	sc := newScanner(content)
	tokens, err := sc.scan()
	if err != nil {
		return
	}
	pos := 0
	r.skeletonTop(tokens, &pos, blockIDByKey)
	// Trailing whitespace lives on the EOF token's prefix.
	if pos < len(tokens) && tokens[pos].typ == tokEOF {
		r.skelText(tokens[pos].prefix)
	}
	r.skelFlush()
}

// skeletonTop walks the flat top-level object, emitting skeleton entries.
func (r *Reader) skeletonTop(tokens []token, pos *int, blockIDByKey map[string]string) {
	if *pos >= len(tokens) || tokens[*pos].typ != tokObjectStart {
		// Not the expected shape — copy whatever remains verbatim so the
		// skeleton still reproduces the bytes (the writer would then emit no
		// refs, equivalent to the original).
		for *pos < len(tokens) && tokens[*pos].typ != tokEOF {
			r.skelToken(tokens[*pos])
			*pos++
		}
		return
	}
	r.skelToken(tokens[*pos]) // {
	*pos++
	for *pos < len(tokens) {
		tok := tokens[*pos]
		switch tok.typ {
		case tokObjectEnd:
			r.skelToken(tok)
			*pos++
			return
		case tokComma:
			r.skelToken(tok)
			*pos++
			continue
		case tokString:
			key := tok.value
			r.skelToken(tok) // key
			*pos++
			// Colon.
			if *pos < len(tokens) && tokens[*pos].typ == tokColon {
				r.skelToken(tokens[*pos])
				*pos++
			}
			if id, ok := blockIDByKey[key]; ok && !strings.HasPrefix(key, "@") {
				// Translatable message value — stand a Ref in for it.
				r.skeletonRefValue(tokens, pos, id)
			} else {
				// "@<id>" / "@@<global>" / unknown key — copy its value verbatim.
				r.skeletonCopyValue(tokens, pos)
			}
		default:
			// Unexpected token at object level — copy verbatim.
			r.skelToken(tok)
			*pos++
		}
	}
}

// skeletonRefValue writes a Ref for a string message value (its prefix as Text,
// then the Ref in place of the raw quoted bytes). A non-string value (invalid
// ARB) is copied verbatim defensively.
func (r *Reader) skeletonRefValue(tokens []token, pos *int, blockID string) {
	if *pos < len(tokens) && tokens[*pos].typ == tokString {
		r.skelText(tokens[*pos].prefix)
		r.skelRef(blockID)
		*pos++
		return
	}
	r.skeletonCopyValue(tokens, pos)
}

// skeletonCopyValue copies an arbitrary JSON value (scalar/object/array) to the
// skeleton verbatim, keeping nested structure balanced.
func (r *Reader) skeletonCopyValue(tokens []token, pos *int) {
	if *pos >= len(tokens) {
		return
	}
	tok := tokens[*pos]
	switch tok.typ {
	case tokObjectStart, tokArrayStart:
		r.skelToken(tok)
		*pos++
		depth := 1
		for depth > 0 && *pos < len(tokens) {
			t := tokens[*pos]
			if t.typ == tokEOF {
				return
			}
			r.skelToken(t)
			*pos++
			switch t.typ {
			case tokObjectStart, tokArrayStart:
				depth++
			case tokObjectEnd, tokArrayEnd:
				depth--
			}
		}
	default:
		r.skelToken(tok)
		*pos++
	}
}

// blockFor builds a Block for one ARB resource. The message value is carried as
// source content; ICU placeholders/plural/select constructs are protected as
// opaque inline placeholder runs. The sibling "@<id>" description becomes a
// developer note.
func (r *Reader) blockFor(res *resource, locale model.LocaleID, counter int) *model.Block {
	runs := runsFromValue(res.value)

	block := &model.Block{
		ID:           "tu" + strconv.Itoa(counter),
		Name:         res.id,
		Translatable: true,
		SourceLocale: locale,
		Source:       runs,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
	}
	block.Properties["arb.key"] = res.id

	if r.cfg.DescriptionNotes {
		if res.description != "" {
			block.AddNote(&model.NoteAnnotation{
				Text:      res.description,
				From:      "developer",
				Annotates: "general",
			})
		}
		// Per-placeholder example/description hints become additional developer
		// notes (in document order). The placeholders' structural fields (type,
		// format, …) are not surfaced — they stay in the byte-faithful skeleton.
		for _, ph := range res.placeholders {
			if text := placeholderNoteText(ph); text != "" {
				block.AddNote(&model.NoteAnnotation{
					Text:      text,
					From:      "developer",
					Annotates: "general",
				})
			}
		}
	}

	return block
}

// placeholderNoteText renders a placeholder's example/description hint as a
// single developer-note line, e.g. "count: number of items (example: 42)".
// It returns "" when the placeholder carries neither a description nor an
// example (so the reader emits no note for it).
func placeholderNoteText(ph placeholderHint) string {
	if ph.description == "" && ph.example == "" {
		return ""
	}
	text := ph.name
	if ph.description != "" {
		text += ": " + ph.description
	}
	if ph.example != "" {
		text += " (example: " + ph.example + ")"
	}
	return text
}

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
