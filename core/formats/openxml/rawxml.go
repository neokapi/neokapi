// Source-byte replay for the OpenXML readers: a decoder that remembers where in
// the part each token sat, so the skeleton can put a token back exactly as it
// was written.

package openxml

import (
	"bytes"
	"encoding/xml"
	"io"
)

// rawDecoder decodes a part and records the source span of the token it last
// returned.
//
// Go's xml.Decoder is a parser, not a copier: it reports a self-closing element
// as a start element plus a synthetic end element, and it normalises line
// endings inside character data (XML 1.0 §2.11). Re-serialising its tokens
// therefore rewrites `<c r="A1"/>` as `<c r="A1"></c>`, CRLF as LF, `&#39;` as
// `'`, `a='1'` as `a="1"`, and a CDATA section as escaped text. Replaying the
// span a token came from keeps all of it, which is what makes an untranslated
// round trip of a part the reader passed through byte for byte.
//
// The embedded *xml.Decoder carries the rest of the decoder API. Only Token is
// shadowed, and it re-reads InputOffset on every call rather than caching the
// previous end, so a helper that consumed tokens straight off the embedded
// decoder cannot leave the span stale.
type rawDecoder struct {
	*xml.Decoder
	src        *rawSource
	start, end int64
}

// newRawDecoder returns a decoder over a part the caller already holds. src is
// retained and the spans Raw returns alias it, so the caller must not mutate it
// while decoding.
func newRawDecoder(src []byte) *rawDecoder {
	s := &rawSource{buf: src, whole: true}
	return &rawDecoder{Decoder: xml.NewDecoder(bytes.NewReader(src)), src: s}
}

// newRawDecoderString returns a decoder over a fragment the reader holds as a
// string, such as a captured subtree it decodes a second time.
func newRawDecoderString(src string) *rawDecoder {
	return newRawDecoder([]byte(src))
}

// newRawDecoderStream returns a decoder over a part the caller reads rather than
// buffers. It keeps a window of the source, not the part: everything below the
// earliest offset a caller can still ask for is dropped as the parse moves on,
// which is what lets a worksheet of tens of megabytes stream.
func newRawDecoderStream(r io.Reader) *rawDecoder {
	s := &rawSource{r: r}
	return &rawDecoder{Decoder: xml.NewDecoder(s), src: s}
}

// Token reads the next token and records the source span it came from.
func (r *rawDecoder) Token() (xml.Token, error) {
	r.src.release(r.end)
	r.start = r.Decoder.InputOffset()
	tok, err := r.Decoder.Token()
	r.end = r.Decoder.InputOffset()
	if err != nil {
		r.end = r.start
	}
	return tok, err
}

// Raw returns the source bytes the last token came from. The end element Go
// synthesises for a self-closing start covers no source bytes, so Raw returns
// nothing for it and the start element's own span already carries the `/>`.
func (r *rawDecoder) Raw() []byte {
	return r.src.slice(r.start, r.end)
}

// RawString is Raw as a string.
func (r *rawDecoder) RawString() string {
	return string(r.Raw())
}

// Offset reports where the token Token last returned begins. Pair it with Pin
// and From to replay a whole subtree.
func (r *rawDecoder) Offset() int64 {
	return r.start
}

// From returns the source bytes from off through the end of the token Token last
// returned, which spans a subtree when off is the offset of its start element
// and the last token is its end element. Pin off first on a streaming decoder,
// or the window will have moved past it.
func (r *rawDecoder) From(off int64) []byte {
	return r.src.slice(off, r.end)
}

// FromString is From as a string.
func (r *rawDecoder) FromString(off int64) string {
	return string(r.From(off))
}

// Upto returns the source bytes from off up to where the token Token last
// returned begins, which spans an element's content when off is the end of its
// start tag and the last token is its end tag.
func (r *rawDecoder) Upto(off int64) []byte {
	return r.src.slice(off, r.start)
}

// UptoString is Upto as a string.
func (r *rawDecoder) UptoString(off int64) string {
	return string(r.Upto(off))
}

// EndOffset reports where the token Token last returned ends.
func (r *rawDecoder) EndOffset() int64 {
	return r.end
}

// Pin holds the window open from off so a later From can reach back to it.
// Every Pin needs an Unpin, and pins nest.
func (r *rawDecoder) Pin(off int64) {
	r.src.pin(off)
}

// Unpin drops the innermost pin.
func (r *rawDecoder) Unpin() {
	r.src.unpin()
}

// rawSource holds the part bytes a rawDecoder replays from. A buffered part is
// held whole; a streamed one is recorded as the decoder reads it and released as
// the parse moves on.
type rawSource struct {
	r     io.Reader
	buf   []byte
	base  int64   // stream offset of buf[0]
	pins  []int64 // offsets a caller still needs, innermost last
	whole bool    // buf is the entire part and is never released
}

// Read feeds the decoder and records what it hands over.
func (s *rawSource) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.buf = append(s.buf, p[:n]...)
	}
	return n, err
}

func (s *rawSource) pin(off int64) {
	s.pins = append(s.pins, off)
}

func (s *rawSource) unpin() {
	if len(s.pins) > 0 {
		s.pins = s.pins[:len(s.pins)-1]
	}
}

// release drops the recorded bytes below off, unless a pin still reaches under
// it. Pins nest, so the outermost one is the lowest.
func (s *rawSource) release(off int64) {
	if s.whole {
		return
	}
	if len(s.pins) > 0 && s.pins[0] < off {
		off = s.pins[0]
	}
	if off <= s.base {
		return
	}
	n := off - s.base
	if n >= int64(len(s.buf)) {
		s.buf = s.buf[:0]
	} else {
		s.buf = s.buf[:copy(s.buf, s.buf[n:])]
	}
	s.base = off
}

func (s *rawSource) slice(from, to int64) []byte {
	if from < s.base || from >= to || to > s.base+int64(len(s.buf)) {
		return nil
	}
	return s.buf[from-s.base : to-s.base]
}

// rawAttrValueSpan locates the value of the idx'th attribute of a start tag
// inside its own source bytes, so a caller replacing one attribute value can
// keep every other byte of the tag: the attribute order, the quote characters,
// the spacing, and the self-closing slash.
//
// idx counts attributes the way encoding/xml reports them, which is document
// order including xmlns declarations. The returned range covers the value
// between the quotes.
func rawAttrValueSpan(raw []byte, idx int) (start, end int, ok bool) {
	i := 0
	if i < len(raw) && raw[i] == '<' {
		i++
	}
	// Element name.
	for i < len(raw) && !isXMLSpace(raw[i]) && raw[i] != '>' && raw[i] != '/' {
		i++
	}
	n := 0
	for i < len(raw) {
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || raw[i] == '>' || raw[i] == '/' {
			return 0, 0, false
		}
		for i < len(raw) && !isXMLSpace(raw[i]) && raw[i] != '=' {
			i++
		}
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || raw[i] != '=' {
			return 0, 0, false
		}
		i++
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || (raw[i] != '"' && raw[i] != '\'') {
			return 0, 0, false
		}
		quote := raw[i]
		i++
		vs := i
		for i < len(raw) && raw[i] != quote {
			i++
		}
		if i >= len(raw) {
			return 0, 0, false
		}
		if n == idx {
			return vs, i, true
		}
		i++
		n++
	}
	return 0, 0, false
}

// rawAttr locates one attribute inside a start tag's own source bytes.
type rawAttr struct {
	name string
	// start and end bracket the whole attribute including the whitespace that
	// separates it from what precedes it, so removing the span leaves a
	// well-formed tag.
	start, end int
	// valueStart and valueEnd bracket the value between its quotes.
	valueStart, valueEnd int
}

// scanRawAttrs lists a start tag's attributes in document order, with the span
// each occupies in the tag's own bytes. A caller rewriting one attribute keeps
// every other byte that way: the attribute order, the quote characters, the
// spacing and the self-closing slash.
//
// It reports nothing for a tag it cannot walk to the end, so a caller falls
// back to replaying the tag whole rather than corrupting it.
func scanRawAttrs(raw []byte) ([]rawAttr, bool) {
	i := 0
	if i >= len(raw) || raw[i] != '<' {
		return nil, false
	}
	i++
	for i < len(raw) && !isXMLSpace(raw[i]) && raw[i] != '>' && raw[i] != '/' {
		i++
	}
	var attrs []rawAttr
	for {
		sep := i
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) {
			return nil, false
		}
		if raw[i] == '>' || raw[i] == '/' {
			return attrs, true
		}
		nameStart := i
		for i < len(raw) && !isXMLSpace(raw[i]) && raw[i] != '=' {
			i++
		}
		name := string(raw[nameStart:i])
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || raw[i] != '=' {
			return nil, false
		}
		i++
		for i < len(raw) && isXMLSpace(raw[i]) {
			i++
		}
		if i >= len(raw) || (raw[i] != '"' && raw[i] != '\'') {
			return nil, false
		}
		quote := raw[i]
		i++
		valueStart := i
		for i < len(raw) && raw[i] != quote {
			i++
		}
		if i >= len(raw) {
			return nil, false
		}
		valueEnd := i
		i++
		attrs = append(attrs, rawAttr{
			name: name, start: sep, end: i,
			valueStart: valueStart, valueEnd: valueEnd,
		})
	}
}

// rawTagAttrInsertPoint reports where a new attribute belongs in a start tag:
// just before the `/>` or `>` that ends it.
func rawTagAttrInsertPoint(raw []byte, attrs []rawAttr) (int, bool) {
	i := 0
	if len(attrs) > 0 {
		i = attrs[len(attrs)-1].end
	} else {
		if i >= len(raw) || raw[i] != '<' {
			return 0, false
		}
		i++
		for i < len(raw) && !isXMLSpace(raw[i]) && raw[i] != '>' && raw[i] != '/' {
			i++
		}
	}
	for i < len(raw) && isXMLSpace(raw[i]) {
		i++
	}
	if i >= len(raw) || (raw[i] != '>' && raw[i] != '/') {
		return 0, false
	}
	return i, true
}

func isXMLSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}
