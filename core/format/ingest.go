package format

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"golang.org/x/text/transform"

	"github.com/neokapi/neokapi/core/model"
)

// DecodesInput is implemented by a reader that applies the document's declared
// encoding itself. A plugin-backed reader hands the declaration to its process
// beside the bytes, or beside a path to them, so OpenDocument leaves both as
// the caller supplied them.
type DecodesInput interface {
	// DecodesInput marks the capability. The method is a bare marker: its
	// presence is the signal, and it is never called.
	DecodesInput()
}

// OpenDocument opens doc with r after settling the document's encoding against
// its bytes. It is the one ingestion point for a document a caller reads:
// every native text reader receives UTF-8 from it, whatever charset the file
// holds, and a reader's own Open is reached only through it.
//
// The encoding is settled in this order:
//
//  1. A byte-order mark wins. It is evidence in the file, so a UTF-8 mark keeps
//     the bytes as they are and a UTF-16 mark selects that decoder, whatever
//     doc.Encoding declared.
//  2. A declared encoding other than UTF-8 selects its decoder from the
//     charset registry core/encoding installs.
//  3. With neither, the bytes pass through as UTF-8, and a reader with a
//     heuristic of its own (an XML prolog, a PO header) may still apply it.
//
// On return doc.Encoding names the encoding the document was read as and
// doc.EncodingSettled reports whether a mark or a declaration settled it. When
// the bytes were transcoded, doc.ReaderAt is dropped: the random-access view
// would describe the source bytes, not the UTF-8 the reader consumes.
//
// A reader whose signature declares Binary, and one that implements
// DecodesInput, receive the document as the caller handed it: a ZIP container
// or an image has no charset to decode by.
func OpenDocument(ctx context.Context, r DataFormatReader, doc *model.RawDocument) error {
	if err := settleEncoding(r, doc); err != nil {
		return err
	}
	return r.Open(ctx, doc)
}

// SettleEncoding names the encoding a document is read as, given its first
// bytes and the encoding a caller declared, and reports whether a byte-order
// mark or the declaration settled it. An empty or UTF-8 declaration with no
// mark leaves the document unsettled: the bytes are taken as UTF-8 and a
// reader's own heuristic may still run.
func SettleEncoding(head []byte, declared string) (name string, settled bool) {
	switch {
	case bytes.HasPrefix(head, []byte(UTF8BOM)):
		return "UTF-8", true
	case bytes.HasPrefix(head, []byte{0xFE, 0xFF}):
		return "UTF-16BE", true
	case bytes.HasPrefix(head, []byte{0xFF, 0xFE}):
		return "UTF-16LE", true
	}
	if isUTF8Name(declared) {
		return declared, false
	}
	return declared, true
}

// settleEncoding applies SettleEncoding to doc and wraps doc.Reader in the
// decoder the settled encoding needs.
func settleEncoding(r DataFormatReader, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil || doc.EncodingSettled {
		return nil
	}
	if r.Signature().Binary {
		return nil
	}
	if _, ok := r.(DecodesInput); ok {
		return nil
	}
	br := bufio.NewReader(doc.Reader)
	head, err := br.Peek(len(UTF8BOM))
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read the head of %s: %w", documentName(doc), err)
	}
	name, settled := SettleEncoding(head, doc.Encoding)
	closer := doc.Reader
	doc.Reader = readCloser{Reader: br, Closer: closer}
	if !settled {
		return nil
	}
	doc.Encoding = name
	doc.EncodingSettled = true
	if isUTF8Name(name) {
		return nil
	}
	if EncodingResolver == nil {
		return fmt.Errorf("encoding %q for %s: no charset registry available (core/encoding is not linked)", name, documentName(doc))
	}
	enc, err := EncodingResolver(name)
	if err != nil {
		return fmt.Errorf("encoding for %s: %w", documentName(doc), err)
	}
	doc.Reader = readCloser{Reader: transform.NewReader(br, enc.NewDecoder()), Closer: closer}
	doc.ReaderAt, doc.Size = nil, 0
	return nil
}

// XMLCharsetReader returns the encoding/xml CharsetReader for a document that
// OpenDocument settled: its bytes are UTF-8 whatever charset the prolog names,
// so the input passes through unchanged and the decoder's byte offsets keep
// indexing the content a reader slices its skeleton from. For an unsettled
// document it returns nil, and the decoder refuses a prolog that names another
// charset as it does on its own.
func XMLCharsetReader(doc *model.RawDocument) func(charset string, input io.Reader) (io.Reader, error) {
	if doc == nil || !doc.EncodingSettled {
		return nil
	}
	return func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
}

func documentName(doc *model.RawDocument) string {
	if doc.URI != "" {
		return doc.URI
	}
	return "the document"
}

// readCloser pairs the stream a reader consumes with the source it closes.
type readCloser struct {
	io.Reader
	io.Closer
}
