package applestrings

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Encoding markers stored on the Layer so the writer can reproduce the original
// byte order.
const (
	encUTF8    = "utf-8"
	encUTF16LE = "utf-16le"
	encUTF16BE = "utf-16be"
)

// decodeToUTF8 detects the input encoding from a BOM (and a heuristic for
// BOM-less UTF-16) and returns the content as UTF-8, the detected encoding
// marker, and whether a UTF-8 BOM was present. Historically .strings files were
// often UTF-16; modern Xcode emits UTF-8. The binary-plist .strings variant is
// out of scope — such inputs decode as UTF-8 best-effort and round-trip via the
// stored original bytes.
func decodeToUTF8(raw []byte) (content, enc string, hadUTF8BOM bool) {
	switch {
	case len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF:
		// UTF-8 BOM. Keep the BOM in the content so byte-faithful round-trip
		// reproduces it; the lexer skips it during parsing.
		return string(raw), encUTF8, true
	case len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE:
		return decodeUTF16(raw[2:], false), encUTF16LE, false
	case len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF:
		return decodeUTF16(raw[2:], true), encUTF16BE, false
	}
	// No BOM. Heuristic: a UTF-16LE .strings has many NUL bytes in odd
	// positions (ASCII content). If valid UTF-8, treat as UTF-8.
	if looksLikeUTF16LE(raw) {
		return decodeUTF16(raw, false), encUTF16LE, false
	}
	if looksLikeUTF16BE(raw) {
		return decodeUTF16(raw, true), encUTF16BE, false
	}
	return string(raw), encUTF8, false
}

// decodeUTF16 decodes UTF-16 code units (after any BOM was stripped) to a UTF-8
// string. bigEndian selects the byte order.
func decodeUTF16(b []byte, bigEndian bool) string {
	n := len(b) / 2
	u16 := make([]uint16, n)
	for i := range n {
		if bigEndian {
			u16[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			u16[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	return string(utf16.Decode(u16))
}

// encodeFromUTF8 converts UTF-8 content back to the original encoding's bytes,
// re-adding the appropriate BOM for the UTF-16 cases. The UTF-8 case returns the
// bytes unchanged (any UTF-8 BOM is already part of the content).
func encodeFromUTF8(content, enc string) []byte {
	switch enc {
	case encUTF16LE:
		return encodeUTF16([]byte(content), false)
	case encUTF16BE:
		return encodeUTF16([]byte(content), true)
	default:
		return []byte(content)
	}
}

// encodeUTF16 encodes a UTF-8 byte slice as UTF-16 with a leading BOM in the
// requested byte order.
func encodeUTF16(utf8Bytes []byte, bigEndian bool) []byte {
	runes := []rune(string(utf8Bytes))
	u16 := utf16.Encode(runes)
	out := make([]byte, 0, 2+len(u16)*2)
	if bigEndian {
		out = append(out, 0xFE, 0xFF)
	} else {
		out = append(out, 0xFF, 0xFE)
	}
	for _, u := range u16 {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

// looksLikeUTF16LE reports whether a BOM-less buffer is plausibly UTF-16LE: it
// must have even length, contain NUL bytes in odd positions among the first
// chunk (typical for ASCII-heavy localization content), and not be valid UTF-8.
func looksLikeUTF16LE(b []byte) bool {
	if len(b) < 2 || len(b)%2 != 0 {
		return false
	}
	if utf8.Valid(b) && !strings.ContainsRune(string(b), 0) {
		return false
	}
	zerosOdd := 0
	limit := min(len(b), 256)
	for i := 1; i < limit; i += 2 {
		if b[i] == 0 {
			zerosOdd++
		}
	}
	return zerosOdd > limit/8
}

// looksLikeUTF16BE is the big-endian counterpart of looksLikeUTF16LE: NULs land
// in even positions.
func looksLikeUTF16BE(b []byte) bool {
	if len(b) < 2 || len(b)%2 != 0 {
		return false
	}
	if utf8.Valid(b) && !strings.ContainsRune(string(b), 0) {
		return false
	}
	zerosEven := 0
	limit := min(len(b), 256)
	for i := 0; i < limit; i += 2 {
		if b[i] == 0 {
			zerosEven++
		}
	}
	return zerosEven > limit/8
}
