package format

import "io"

// CaptureReader wraps an io.Reader, retaining the bytes read so far in a sliding
// window. It lets a streaming walk driven by an offset-reporting decoder — e.g.
// encoding/xml's Decoder, via InputOffset() — resolve byte ranges back to their
// raw source bytes without holding the whole document in memory (no io.ReadAll).
//
// The decoder consumes bytes through Read; Slice(start,end) returns the raw
// source for an absolute byte range, and DiscardTo(off) drops everything below
// an absolute offset once its skeleton has been emitted. The retained window is
// therefore bounded by (the decoder's read-ahead, ~one bufio chunk) + (the span
// between the last discard frontier and the current parse position) — not the
// document size.
//
// Byte offsets are absolute in the underlying stream, matching the semantics of
// encoding/xml's Decoder.InputOffset(). The reader is single-goroutine: the
// decoder pulls from it on the same goroutine that calls Slice/DiscardTo.
type CaptureReader struct {
	src  io.Reader
	buf  []byte // retained window; buf[0] is at absolute offset base
	base int64  // absolute offset of buf[0]
}

// NewCaptureReader returns a CaptureReader over src. The decoder should be built
// directly over the returned value (e.g. xml.NewDecoder(cr)) so every byte the
// decoder consumes flows through Read and is captured.
func NewCaptureReader(src io.Reader) *CaptureReader {
	return &CaptureReader{src: src}
}

// Read implements io.Reader, appending the bytes delivered to the caller into
// the retained window. CaptureReader deliberately does not implement
// io.ByteReader, so encoding/xml wraps it in a bufio.Reader and reads ahead in
// bounded chunks (keeping the window past InputOffset() small).
func (c *CaptureReader) Read(p []byte) (int, error) {
	n, err := c.src.Read(p)
	if n > 0 {
		c.buf = append(c.buf, p[:n]...)
	}
	return n, err
}

// Slice returns the raw source bytes for the absolute range [start,end). The
// range must lie within the retained window: start must be at or after the last
// DiscardTo offset, and end must be at or before the number of bytes read so
// far. It reports false if the range is outside the window.
//
// The returned slice aliases the internal buffer and is only valid until the
// next Read or DiscardTo call; copy it (e.g. via string(...)) before advancing
// the decoder.
func (c *CaptureReader) Slice(start, end int64) ([]byte, bool) {
	lo := start - c.base
	hi := end - c.base
	if lo < 0 || hi > int64(len(c.buf)) || lo > hi {
		return nil, false
	}
	return c.buf[lo:hi], true
}

// DiscardTo drops retained bytes below the absolute offset off, freeing memory.
// Offsets below the current base are ignored. It compacts in place, reusing the
// backing array so capacity stays bounded to the peak window size.
func (c *CaptureReader) DiscardTo(off int64) {
	if off <= c.base {
		return
	}
	drop := off - c.base
	if drop >= int64(len(c.buf)) {
		c.buf = c.buf[:0]
		c.base = off
		return
	}
	n := copy(c.buf, c.buf[drop:])
	c.buf = c.buf[:n]
	c.base += drop
}

// Base returns the absolute offset of the oldest retained byte (the last
// DiscardTo frontier). Exposed for tests/assertions on the window bound.
func (c *CaptureReader) Base() int64 { return c.base }

// Window returns the number of bytes currently retained. Exposed for
// tests/assertions that the window stays bounded (does not grow with the doc).
func (c *CaptureReader) Window() int { return len(c.buf) }
