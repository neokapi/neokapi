// Package encoding provides text encoding detection and conversion utilities.
// It wraps golang.org/x/text encoding support to provide a registry-based
// approach to encoding management for document processing.
package encoding

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// EncoderManager manages text encoding detection and conversion.
type EncoderManager struct {
	encodings map[string]encoding.Encoding
}

// NewEncoderManager creates a new EncoderManager with all supported encodings registered.
func NewEncoderManager() *EncoderManager {
	em := &EncoderManager{
		encodings: make(map[string]encoding.Encoding),
	}
	em.registerDefaults()
	return em
}

// Get returns the encoding for the given name, or an error if not found.
func (em *EncoderManager) Get(name string) (encoding.Encoding, error) {
	key := normalizeEncodingName(name)
	enc, ok := em.encodings[key]
	if !ok {
		return nil, fmt.Errorf("unsupported encoding: %s", name)
	}
	return enc, nil
}

// Register adds an encoding with the given name.
func (em *EncoderManager) Register(name string, enc encoding.Encoding) {
	em.encodings[normalizeEncodingName(name)] = enc
}

// Names returns all registered encoding names.
func (em *EncoderManager) Names() []string {
	names := make([]string, 0, len(em.encodings))
	for name := range em.encodings {
		names = append(names, name)
	}
	return names
}

// Decode reads bytes encoded with the named encoding and returns UTF-8 text.
func (em *EncoderManager) Decode(data []byte, encodingName string) (string, error) {
	if encodingName == "" || normalizeEncodingName(encodingName) == "utf-8" {
		return string(data), nil
	}

	enc, err := em.Get(encodingName)
	if err != nil {
		return "", err
	}

	reader := transform.NewReader(bytes.NewReader(data), enc.NewDecoder())
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("decode %s: %w", encodingName, err)
	}
	return string(decoded), nil
}

// Encode converts UTF-8 text to the named encoding.
func (em *EncoderManager) Encode(text string, encodingName string) ([]byte, error) {
	if encodingName == "" || normalizeEncodingName(encodingName) == "utf-8" {
		return []byte(text), nil
	}

	enc, err := em.Get(encodingName)
	if err != nil {
		return nil, err
	}

	writer := new(bytes.Buffer)
	tw := transform.NewWriter(writer, enc.NewEncoder())
	if _, err := tw.Write([]byte(text)); err != nil {
		return nil, fmt.Errorf("encode %s: %w", encodingName, err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("encode %s close: %w", encodingName, err)
	}
	return writer.Bytes(), nil
}

// Detect attempts to detect the encoding of the given data by examining
// byte order marks (BOMs). Returns the detected encoding name and the
// data without BOM. If no BOM is found, returns "utf-8" as default.
func Detect(data []byte) (encodingName string, content []byte) {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return "utf-8", data[3:]
	}
	if len(data) >= 2 {
		if data[0] == 0xFE && data[1] == 0xFF {
			return "utf-16be", data[2:]
		}
		if data[0] == 0xFF && data[1] == 0xFE {
			return "utf-16le", data[2:]
		}
	}
	return "utf-8", data
}

// ToUTF8 detects the input encoding via BOM and transcodes to UTF-8
// when needed. Returns the transcoded bytes and the detected encoding
// name. When the input is already UTF-8 (with or without BOM), the
// BOM-stripped data is returned unchanged.
//
// This is a convenience wrapper around Detect + EncoderManager.Decode
// that format readers can call in their input-ingestion path so a
// real-world UTF-16 fixture (e.g. a TMX dumped by Trados, a Windows-
// authored .po) doesn't sink the parser the moment it tries to read
// the first multi-byte sequence as UTF-8.
//
// The function is intentionally narrow: it only handles BOM-detected
// encodings. Callers that need prolog/header inspection (e.g. xliff's
// `<?xml encoding="windows-1252"?>` discovery) should layer that on
// top — see core/formats/xliff/reader.go's transcodeToUTF8 for an
// example.
func ToUTF8(data []byte) ([]byte, string, error) {
	enc, stripped := Detect(data)
	if enc == "utf-8" {
		return stripped, enc, nil
	}
	em := NewEncoderManager()
	text, err := em.Decode(stripped, enc)
	if err != nil {
		return nil, enc, err
	}
	return []byte(text), enc, nil
}

func (em *EncoderManager) registerDefaults() {
	// Unicode
	em.Register("utf-8", unicode.UTF8)
	em.Register("utf-16", unicode.UTF16(unicode.BigEndian, unicode.UseBOM))
	em.Register("utf-16be", unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM))
	em.Register("utf-16le", unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM))

	// Western European
	em.Register("iso-8859-1", charmap.ISO8859_1)
	em.Register("latin-1", charmap.ISO8859_1)
	em.Register("iso-8859-2", charmap.ISO8859_2)
	em.Register("iso-8859-3", charmap.ISO8859_3)
	em.Register("iso-8859-4", charmap.ISO8859_4)
	em.Register("iso-8859-5", charmap.ISO8859_5)
	em.Register("iso-8859-6", charmap.ISO8859_6)
	em.Register("iso-8859-7", charmap.ISO8859_7)
	em.Register("iso-8859-8", charmap.ISO8859_8)
	em.Register("iso-8859-9", charmap.ISO8859_9)
	em.Register("iso-8859-10", charmap.ISO8859_10)
	em.Register("iso-8859-13", charmap.ISO8859_13)
	em.Register("iso-8859-14", charmap.ISO8859_14)
	em.Register("iso-8859-15", charmap.ISO8859_15)
	em.Register("iso-8859-16", charmap.ISO8859_16)

	// Windows code pages
	em.Register("windows-1250", charmap.Windows1250)
	em.Register("windows-1251", charmap.Windows1251)
	em.Register("windows-1252", charmap.Windows1252)
	em.Register("windows-1253", charmap.Windows1253)
	em.Register("windows-1254", charmap.Windows1254)
	em.Register("windows-1255", charmap.Windows1255)
	em.Register("windows-1256", charmap.Windows1256)
	em.Register("windows-1257", charmap.Windows1257)
	em.Register("windows-1258", charmap.Windows1258)

	// East Asian
	em.Register("shift_jis", japanese.ShiftJIS)
	em.Register("euc-jp", japanese.EUCJP)
	em.Register("iso-2022-jp", japanese.ISO2022JP)
	em.Register("euc-kr", korean.EUCKR)
	// "gb2312" labels in the wild (HTTP charset, PO Content-Type) carry
	// double-byte EUC-CN data, not the 7-bit HZ escape transport. GBK is a
	// superset of GB2312 and is the correct interpretation for charset=gb2312,
	// matching the WHATWG Encoding Standard and what browsers do.
	em.Register("gb2312", simplifiedchinese.GBK)
	em.Register("euc-cn", simplifiedchinese.GBK)
	em.Register("gbk", simplifiedchinese.GBK)
	em.Register("gb18030", simplifiedchinese.GB18030)
	// HZ-GB-2312 (the 7-bit escape-based transport encoding) stays reachable
	// under its own real name, never via the "gb2312" alias above.
	em.Register("hz-gb-2312", simplifiedchinese.HZGB2312)
	em.Register("hz", simplifiedchinese.HZGB2312)
	em.Register("big5", traditionalchinese.Big5)

	// Other
	em.Register("koi8-r", charmap.KOI8R)
	em.Register("koi8-u", charmap.KOI8U)
	em.Register("macintosh", charmap.Macintosh)
}

// normalizeEncodingName lowercases and strips common variations in encoding names.
func normalizeEncodingName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	return name
}
