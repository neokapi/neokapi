package format

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// UTF8BOM is the UTF-8 encoding of U+FEFF, the byte-order mark.
const UTF8BOM = "\xef\xbb\xbf"

// SplitBOM separates a leading UTF-8 byte-order mark from a document's content,
// returning the mark (nil when absent) and the bytes that follow.
//
// Every text reader must parse `rest`, never `data`. A mark left on the front of
// the content is not inert: it becomes part of the first key, the first cell, or
// the first line, so it lands in the block's name and in the content-derived id
// that content memory and `kapi merge` key on — orphaning the file's memory
// matches and mis-keying its merge overlays. It also defeats prefix tests for
// leading constructs, which is how a document's YAML front matter ends up
// exposed as translatable body text. None of that shows up in a round-trip
// test: the mark is inside the parsed value, so the bytes come back unchanged.
//
// The mark is layout, so it belongs in the skeleton: emit it as the leading
// skeleton text and write-back stays byte-exact. Excel writes one on every CSV
// export, so this is the ordinary case rather than the exotic one.
func SplitBOM(data []byte) (bom, rest []byte) {
	if bytes.HasPrefix(data, []byte(UTF8BOM)) {
		return data[:len(UTF8BOM)], data[len(UTF8BOM):]
	}
	return nil, data
}

// TrimBOM returns data without a leading UTF-8 byte-order mark. Use it where
// the mark has no skeleton to be carried in, for instance when a reader hands
// the bytes to a parser that reconstructs its own output.
func TrimBOM(data []byte) []byte {
	_, rest := SplitBOM(data)
	return rest
}

// SplitBOMReader is [SplitBOM] for a reader a format consumes incrementally: it
// consumes a leading UTF-8 byte-order mark and returns it alongside a reader
// positioned on the first content byte.
//
// A short stream is not an error — a document smaller than the mark simply has
// none.
func SplitBOMReader(rd io.Reader) (bom string, rest io.Reader, err error) {
	br := bufio.NewReader(rd)
	head, perr := br.Peek(len(UTF8BOM))
	if perr != nil && !errors.Is(perr, io.EOF) {
		return "", br, perr
	}
	if string(head) == UTF8BOM {
		if _, derr := br.Discard(len(UTF8BOM)); derr != nil {
			return "", br, derr
		}
		return UTF8BOM, br, nil
	}
	return "", br, nil
}

// TrimBOMString is [TrimBOM] for a string value.
func TrimBOMString(s string) string {
	if len(s) >= len(UTF8BOM) && s[:len(UTF8BOM)] == UTF8BOM {
		return s[len(UTF8BOM):]
	}
	return s
}
