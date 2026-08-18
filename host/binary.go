package host

import (
	"bytes"
	"io"
	"strings"
)

// The binary-content guard.
//
// The toolbox falls back to plaintext when neither the extension nor a content
// sniff claims a file, which is what keeps `kcat notes`, `kcat CHANGELOG` and
// piped prose working. Applied to a .dmg, a .png or an installer that fell into
// a glob, that fallback is worse than useless: the file is read as text and its
// bytes are printed or converted verbatim — which is how `kcat ~/Downloads/*`
// ends up spraying a disk image at the terminal, and how `kconv --to md` writes
// a Markdown file full of mojibake.
//
// A file only reaches this guard when no format claimed it, so nothing kapi
// understands is affected: a .docx, a .wav, a .png with a media reader or a
// UTF-16 .strings is detected long before the fallback. An explicit --format is
// the escape hatch and short-circuits detection entirely.
//
// The rule is git's: a NUL byte within the first 8000 bytes means binary. It is
// cheap, needs no full read, and has effectively no false positives on text —
// the one exception being UTF-16/32, which announce themselves with a BOM or
// with --encoding.
const binarySniffLen = 8000

// looksBinary reports whether a content prefix is binary rather than text.
func looksBinary(prefix []byte) bool {
	if hasUnicodeBOM(prefix) {
		return false
	}
	return bytes.IndexByte(prefix, 0) >= 0
}

// hasUnicodeBOM reports whether b opens with a Unicode byte-order mark. A
// UTF-16/32 document is full of NUL bytes and is still text; the BOM is how it
// says so. Ordered longest-first, since the UTF-32LE mark starts with the
// UTF-16LE one.
func hasUnicodeBOM(b []byte) bool {
	for _, bom := range [][]byte{
		{0x00, 0x00, 0xFE, 0xFF}, // UTF-32BE
		{0xFF, 0xFE, 0x00, 0x00}, // UTF-32LE
		{0xEF, 0xBB, 0xBF},       // UTF-8
		{0xFE, 0xFF},             // UTF-16BE
		{0xFF, 0xFE},             // UTF-16LE
	} {
		if bytes.HasPrefix(b, bom) {
			return true
		}
	}
	return false
}

// binaryBytes reports whether an in-memory document is binary, honouring an
// --encoding that declares a NUL-bearing Unicode form.
func (a *App) binaryBytes(content []byte) bool {
	if a.encodingAllowsNUL() {
		return false
	}
	return looksBinary(content[:min(len(content), binarySniffLen)])
}

// binaryStream is binaryBytes over a seekable source: it reads a prefix and
// rewinds, so the caller's reader still sees the whole document. A source that
// cannot be rewound is reported as text — refusing a file we failed to inspect
// would turn a seek problem into a wrong diagnosis, and the reader that follows
// reports the real trouble.
func (a *App) binaryStream(content io.ReadSeeker) bool {
	if a.encodingAllowsNUL() {
		return false
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return false
	}
	prefix := make([]byte, binarySniffLen)
	n, err := io.ReadFull(content, prefix)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return false
	}
	return looksBinary(prefix[:n])
}

// encodingAllowsNUL reports whether --encoding names a Unicode form whose
// text legitimately contains NUL bytes, which takes the NUL rule off the table.
func (a *App) encodingAllowsNUL() bool {
	enc := strings.ToUpper(strings.NewReplacer("-", "", "_", "", " ", "").Replace(a.InputEncoding()))
	return strings.HasPrefix(enc, "UTF16") || strings.HasPrefix(enc, "UTF32")
}
