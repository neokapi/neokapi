package applestrings

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// fileKind discriminates the two Apple localization file types this format
// handles. The kind is decided from the file extension and, failing that, from
// content sniffing.
type fileKind int

const (
	kindStrings     fileKind = iota // legacy .strings key/value text table
	kindStringsdict                 // .stringsdict plist-XML plural dictionary
)

// Layer property keys used to carry round-trip state from reader to writer.
const (
	propOriginal = "applestrings.original" // original UTF-8 bytes for byte-faithful rewrite
	propKind     = "applestrings.kind"     // "strings" or "stringsdict"
	propEncoding = "applestrings.encoding" // "utf-8", "utf-16le", or "utf-16be"
	propBOM      = "applestrings.bom"      // "1" if the original carried a BOM (UTF-8 case)
)

// Reader implements DataFormatReader for Apple Strings (.strings) and Apple
// Stringsdict (.stringsdict) files.
//
// Output strategy mirrors the native resx/xcstrings readers: the document is
// parsed losslessly and the original (UTF-8) bytes are stored on the root Layer
// so the writer can produce byte-faithful output by splicing only changed
// values. UTF-16 .strings inputs are transcoded to UTF-8 for the model; the
// recorded encoding marker lets the writer reproduce the original byte order.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter so kapi extract can capture a
// byte-exact source skeleton that kapi merge replays through the writer. Without
// this the merge path (which discards layer properties and never re-reads the
// original bytes) could not reproduce the file faithfully.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// SetSkeletonStore sets the skeleton store for streaming skeleton output. When
// non-nil the reader emits a SkeletonText/SkeletonRef stream that stands in for
// the file structure and each translatable value, and skips storing the
// (now-redundant) original-bytes layer property.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// skelText appends non-translatable text to the skeleton buffer when active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		r.skelBuf.WriteString(s)
	}
}

// skelRef flushes buffered text and writes a block reference to the store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore == nil {
		return
	}
	if r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
	_ = r.skeletonStore.WriteRef(id)
}

// skelFlush writes any remaining buffered text to the store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

// NewReader creates a new Apple Strings reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "applestrings",
			FormatDisplayName: "Apple Strings",
			FormatMimeType:    "text/plain",
			FormatExtensions:  []string{".strings", ".stringsdict"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/plain"},
		Extensions: []string{".strings", ".stringsdict"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("applestrings: nil document or reader")
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
	raw, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("applestrings: reading: %w", err)}
		return
	}

	content, enc, hadBOM := decodeToUTF8(raw)
	kind := r.detectKind(content)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	kindStr := "strings"
	if kind == kindStringsdict {
		kindStr = "stringsdict"
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "applestrings",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/plain",
		HasBOM:   hadBOM,
		Properties: map[string]string{
			propKind:     kindStr,
			propEncoding: enc,
		},
	}
	// The original-bytes property and the skeleton stream are two paths to the
	// same byte-exact result. The merge path discards layer properties, so when
	// a skeleton store is wired we omit propOriginal: the writer reconstructs
	// from the skeleton instead (and the property would otherwise be dead state).
	if r.skeletonStore == nil {
		layer.Properties[propOriginal] = content
	}
	if hadBOM {
		layer.Properties[propBOM] = "1"
	}

	// Parse the document up front. Comments that are not attached to any
	// translatable entry — trailing/orphan or superseded comments in a .strings
	// table, and XML comments inside a .stringsdict plist — are surfaced as
	// layer-scoped NoteAnnotations. These MUST be attached before the layer
	// pointer is published on the channel (PartLayerStart): a consumer holds the
	// same pointer, so mutating the layer afterwards would race. Layer
	// annotations are not part of the canonical part stream, so this is
	// parity-safe and leaves the emitted Block/Data/Group stream unchanged (no
	// extraction flag needed). The comment bytes still round-trip in the skeleton.
	var (
		strDoc   *stringsDoc
		dictDoc  *stringsdictDoc
		comments []string
	)
	switch kind {
	case kindStringsdict:
		dictDoc, err = parseStringsdict(content)
		if err == nil {
			comments = dictDoc.commentTexts()
		}
	default:
		strDoc, err = parseStringsFile(content)
		if err == nil {
			comments = strDoc.orphanComments
		}
	}
	if err != nil {
		select {
		case ch <- model.PartResult{Error: err}:
		case <-ctx.Done():
		}
		return
	}
	// Honour the same ExtractComments toggle that governs per-block comment
	// notes: when it is off, no comment surfaces as a note anywhere.
	if r.cfg.ExtractComments {
		if notes := commentNotes(comments); notes != nil {
			layer.SetAnno(model.AnnoNote, notes)
		}
	}

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	switch kind {
	case kindStringsdict:
		if !r.emitStringsdict(ctx, ch, locale, dictDoc) {
			return
		}
	default:
		if !r.emitStrings(ctx, ch, content, locale, strDoc) {
			return
		}
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// detectKind decides whether the content is a .stringsdict plist or a plain
// .strings table. The file extension takes priority; content sniffing is the
// fallback for streams opened without an extension.
func (r *Reader) detectKind(content string) fileKind {
	uri := strings.ToLower(r.Doc.URI)
	switch {
	case strings.HasSuffix(uri, ".stringsdict"):
		return kindStringsdict
	case strings.HasSuffix(uri, ".strings"):
		return kindStrings
	}
	// Content sniff: a plist with the stringsdict marker is a stringsdict.
	if strings.Contains(content, "<plist") && strings.Contains(content, "NSStringLocalizedFormatKey") {
		return kindStringsdict
	}
	if strings.Contains(content, "<?xml") && strings.Contains(content, "<plist") {
		return kindStringsdict
	}
	return kindStrings
}

// commentNotes builds a layer-scoped note collection from comment text that is
// not attached to any translatable entry (superseded/trailing .strings comments
// or XML comments inside a .stringsdict). Empty texts are skipped. Returns nil
// when there is nothing to surface, so the caller avoids setting an empty
// annotation. These are developer/context comments, so they are surfaced as
// semantic metadata (NoteAnnotation), never as translatable content.
func commentNotes(texts []string) *model.Notes {
	var notes *model.Notes
	for _, t := range texts {
		if t == "" {
			continue
		}
		if notes == nil {
			notes = &model.Notes{}
		}
		notes.Items = append(notes.Items, &model.NoteAnnotation{
			Text:      t,
			From:      "developer",
			Annotates: "general",
		})
	}
	return notes
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}
