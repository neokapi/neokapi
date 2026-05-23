package applestrings

import (
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
	cfg *Config
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
			propOriginal: content,
			propKind:     kindStr,
			propEncoding: enc,
		},
	}
	if hadBOM {
		layer.Properties[propBOM] = "1"
	}

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	switch kind {
	case kindStringsdict:
		if !r.emitStringsdict(ctx, ch, content, locale) {
			return
		}
	default:
		if !r.emitStrings(ctx, ch, content, locale) {
			return
		}
	}

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

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}
