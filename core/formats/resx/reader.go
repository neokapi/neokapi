package resx

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

// Reader implements DataFormatReader for .NET RESX / .resw files.
//
// Output strategy mirrors the native xcstrings reader: the entire document is
// tokenized losslessly and the original bytes are stored on the root Layer so
// the writer can produce byte-faithful output by splicing only changed <value>
// content. Each translatable string <data> entry becomes one Block whose Name
// is the @name, Source is the decoded <value> text, and note (if any) comes
// from the sibling <comment>. Typed/binary <data> (those carrying @type or
// @mimetype), <resheader>, <metadata>, <assembly>, and the schema are never
// extracted — they round-trip verbatim.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

// Ensure Reader satisfies the skeleton-emitter contract. When a skeleton store
// is wired (kapi extract), the reader streams a byte-exact skeleton — every
// token verbatim except the inner content of each translatable <value>, which
// is replaced by a SkeletonRef the writer fills back in on merge.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// SetSkeletonStore wires a skeleton store so the reader emits byte-exact
// skeleton entries. With a store set, the layer's "resx.original" property is
// not populated — the skeleton supersedes it as the merge source of truth.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// NewReader creates a new RESX reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "resx",
			FormatDisplayName: ".NET RESX",
			FormatMimeType:    "text/microsoft-resx",
			FormatExtensions:  []string{".resx", ".resw"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/microsoft-resx"},
		Extensions: []string{".resx", ".resw"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("resx: nil document or reader")
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
		ch <- model.PartResult{Error: fmt.Errorf("resx: reading: %w", err)}
		return
	}

	toks, err := newTokenizer(string(content)).tokenize()
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("resx: %w", err)}
		return
	}

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:         "doc1",
		Name:       r.Doc.URI,
		Format:     "resx",
		Locale:     locale,
		Encoding:   r.Doc.Encoding,
		MimeType:   "text/microsoft-resx",
		Properties: map[string]string{},
	}
	// Preserve the original document bytes so the writer can produce
	// byte-faithful output, splicing only changed values. unsafe.String shares
	// the backing array — content is not mutated after this point. When a
	// skeleton store is wired the skeleton carries every byte instead, so the
	// (potentially large) original property is omitted — merge replays the
	// skeleton, not the layer property.
	if r.skeletonStore == nil {
		layer.Properties["resx.original"] = unsafe.String(unsafe.SliceData(content), len(content))
	}

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	r.walk(ctx, ch, toks)

	if r.skeletonStore != nil {
		r.emitSkeleton(toks)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// walk scans the top-level token stream, locating string <data> entries and
// emitting one Block per translatable entry. It collects the decoded <value>
// content and any sibling <comment> by scanning the tokens between a <data>
// start tag and its matching </data> end tag.
func (r *Reader) walk(ctx context.Context, ch chan<- model.PartResult, toks []token) {
	blockCounter := 0
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind != tokStartTag || t.name != "data" {
			continue
		}
		// Find the matching </data>.
		end := matchEnd(toks, i, "data")
		if end < 0 {
			continue
		}
		entryToks := toks[i : end+1]

		if r.isTranslatableData(t) {
			blockCounter++
			if !r.emitDataBlock(ctx, ch, t, entryToks, blockCounter) {
				return
			}
		}
		i = end
	}
}

// emitSkeleton streams a byte-exact skeleton of the token stream to the
// skeleton store. Every token is written verbatim except the inner character
// data of a translatable <data>/<value> element, which is replaced by a
// SkeletonRef carrying the block ID. The writer (writeFromSkeleton) replays the
// text entries verbatim and re-encodes the resolved block value at each ref, so
// an untranslated round-trip is byte-identical and a translated one changes only
// the <value> inner bytes.
//
// Block IDs must match those assigned by walk(): the counter advances only for
// translatable <data> entries, in document order, producing the same
// "tu"+counter scheme. emitSkeleton mirrors walk() exactly to keep them aligned.
//
// Known limitation: a <value> whose content is a CDATA section round-trips
// through the block as decoded plain text (collectText unwraps CDATA), so the
// ref re-encodes it as escaped character data — the CDATA wrapper is not
// preserved. Every other shape (plain text, entity-escaped text, .NET
// placeholders, multi-line, empty) is byte-exact.
func (r *Reader) emitSkeleton(toks []token) {
	pending := 0 // start index of the not-yet-flushed verbatim run
	flush := func(end int) {
		var b strings.Builder
		for _, t := range toks[pending:end] {
			b.WriteString(t.raw)
		}
		_ = r.skeletonStore.WriteText([]byte(b.String()))
		pending = end
	}

	blockCounter := 0
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind != tokStartTag || t.name != "data" {
			continue
		}
		end := matchEnd(toks, i, "data")
		if end < 0 {
			continue
		}
		entryToks := toks[i : end+1]

		if r.isTranslatableData(t) {
			blockCounter++
			// Only emit a ref when the entry actually produced a Block: a
			// <data> with no inner <value> span yields no Block (walk's
			// emitDataBlock returns early on a missing <value>), so leave the
			// whole entry as verbatim skeleton text.
			valStart, valEnd := locateChild(entryToks, "value")
			if valStart >= 0 {
				// Flush everything up to and including the <value> start tag
				// (entry-relative valStart maps to toks index i+valStart).
				flush(i + valStart + 1)
				_ = r.skeletonStore.WriteRef("tu" + strconv.Itoa(blockCounter))
				// Resume verbatim emission at the </value> end tag, skipping
				// the original inner-content tokens the ref now stands for.
				pending = i + valEnd
			}
		}
		i = end
	}
	flush(len(toks))
}

// isTranslatableData decides whether a <data> start tag denotes a translatable
// string resource. Typed/binary resources (those with @type or @mimetype) and
// designer name-reference entries (@name starting with '>') are not.
func (r *Reader) isTranslatableData(start token) bool {
	if _, ok := start.attrValue("type"); ok {
		return false
	}
	if _, ok := start.attrValue("mimetype"); ok {
		return false
	}
	name, ok := start.attrValue("name")
	if !ok {
		return false
	}
	if r.cfg.SkipNameDataReferences {
		if c, ok := firstRune(name); ok && c == '>' {
			return false
		}
	}
	return true
}

// emitDataBlock emits one Block for a string <data> entry. The entryToks slice
// spans the <data> start tag through its </data> end tag.
func (r *Reader) emitDataBlock(ctx context.Context, ch chan<- model.PartResult,
	start token, entryToks []token, counter int) bool {

	name, _ := start.attrValue("name")
	valueText, hasValue := childElementText(entryToks, "value")
	if !hasValue {
		// A <data> with no <value> child is malformed/empty; skip emission but
		// keep the bytes (the writer copies it verbatim).
		return true
	}

	block := &model.Block{
		ID:           "tu" + strconv.Itoa(counter),
		Name:         name,
		Translatable: true,
		SourceLocale: r.docLocale(),
		Source:       buildValueRuns(valueText),
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}

	// xml:space="preserve" means the surrounding whitespace inside <value> is
	// significant. RESX strings are commonly declared with it; record the flag
	// so downstream tools treat the content faithfully.
	if space, ok := start.attrValue("xml:space"); ok && space == "preserve" {
		block.PreserveWhitespace = true
	}

	// Sibling <comment> → translator note.
	if comment, ok := childElementText(entryToks, "comment"); ok && r.cfg.ExtractComments {
		if strings.TrimSpace(comment) != "" {
			block.Annotations["note"] = &model.NoteAnnotation{
				Text:      comment,
				From:      "developer",
				Annotates: "general",
			}
		}
	}

	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
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

// matchEnd returns the index of the end tag matching the start tag at
// toks[startIdx], honouring nesting of same-named elements. Returns -1 if no
// match is found. A self-closing start tag matches itself.
func matchEnd(toks []token, startIdx int, name string) int {
	if toks[startIdx].kind == tokSelfClose {
		return startIdx
	}
	depth := 0
	for i := startIdx; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.kind == tokStartTag && t.name == name:
			depth++
		case t.kind == tokEndTag && t.name == name:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// childElementText returns the decoded text content of the first direct-ish
// child element with the given local name inside entryToks (the token span of a
// <data> entry, inclusive of its own start/end tags). It returns the
// concatenated character data between the child's start and end tags with XML
// entities resolved and CDATA unwrapped, plus whether the element was present.
//
// The scan is depth-aware: it only matches a child whose start tag sits exactly
// one level below <data>, so a <value> nested inside some other element would
// not be mistaken for the entry's own <value>. In practice RESX <data> children
// (<value>, <comment>) are always at depth 1.
func childElementText(entryToks []token, name string) (string, bool) {
	// entryToks[0] is the <data> start tag, entryToks[len-1] its </data>.
	depth := 0
	for i := range entryToks {
		t := entryToks[i]
		switch t.kind {
		case tokStartTag:
			depth++
			if depth == 2 && t.name == name {
				// Collect text up to the matching end tag at this depth.
				return collectText(entryToks, i, name), true
			}
		case tokSelfClose:
			if depth == 1 && t.name == name {
				return "", true
			}
		case tokEndTag:
			depth--
		}
	}
	return "", false
}

// collectText concatenates the decoded character data of the element starting
// at entryToks[startIdx] up to its matching end tag. CDATA sections are
// included verbatim (their inner bytes), and text is entity-decoded.
func collectText(entryToks []token, startIdx int, name string) string {
	var b strings.Builder
	depth := 0
	for i := startIdx; i < len(entryToks); i++ {
		t := entryToks[i]
		switch t.kind {
		case tokStartTag:
			if t.name == name {
				depth++
				continue
			}
		case tokEndTag:
			if t.name == name {
				depth--
				if depth == 0 {
					return b.String()
				}
			}
		case tokText:
			if depth >= 1 {
				b.WriteString(decodeEntities(t.raw))
			}
		case tokCDATA:
			if depth >= 1 {
				inner := strings.TrimSuffix(strings.TrimPrefix(t.raw, "<![CDATA["), "]]>")
				b.WriteString(inner)
			}
		}
	}
	return b.String()
}

// buildValueRuns splits decoded <value> text into a Run sequence, lifting .NET
// composite-format placeholders ({0}, {1:t}, {0,-10}) into inline Ph codes so
// they survive pseudo-translation and AI translation as opaque tokens — the
// same protection Okapi's okf_xml-resx config applies via its codeFinder rule
// (\{[^}]+?\}). Doubled braces ({{ and }}) are literal and stay as text. The
// remaining literal text becomes TextRuns.
func buildValueRuns(s string) []model.Run {
	if s == "" {
		return []model.Run{{Text: &model.TextRun{Text: ""}}}
	}
	var runs []model.Run
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text.String()}})
			text.Reset()
		}
	}
	phID := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '{' {
			// Escaped brace "{{" → literal '{'.
			if i+1 < len(s) && s[i+1] == '{' {
				text.WriteByte('{')
				i += 2
				continue
			}
			// Find the closing '}' (placeholders do not nest in .NET format).
			end := strings.IndexByte(s[i:], '}')
			if end > 0 {
				placeholder := s[i : i+end+1]
				flush()
				phID++
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
					ID:    "p" + strconv.Itoa(phID),
					Type:  "code",
					Data:  placeholder,
					Equiv: placeholder,
				}})
				i += end + 1
				continue
			}
			text.WriteByte('{')
			i++
			continue
		}
		if c == '}' && i+1 < len(s) && s[i+1] == '}' {
			// Escaped brace "}}" → literal '}'.
			text.WriteByte('}')
			i += 2
			continue
		}
		text.WriteByte(c)
		i++
	}
	flush()
	if len(runs) == 0 {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: ""}})
	}
	return runs
}
