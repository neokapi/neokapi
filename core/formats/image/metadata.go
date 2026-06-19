package image

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"html"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/docmeta"
	"github.com/neokapi/neokapi/core/model"
)

// readImageMetadata extracts document metadata from the image at path WITHOUT
// loading the pixel data: it stops scanning at the first image-data chunk (PNG
// IDAT / JPEG SOS) and skips large chunks by seeking. It reads PNG text chunks
// (tEXt/iTXt/zTXt) and embedded XMP, and JPEG APP1 XMP. Translatable fields
// (title, description, keywords, comment) come back as translatable
// docmeta.Entry; the rest (author, copyright, software) as non-translatable.
// Binary EXIF/IPTC are not parsed yet (documented in AD-029).
func readImageMetadata(path, mime string) []docmeta.Entry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	switch mime {
	case "image/png":
		return pngMetadata(f)
	case "image/jpeg":
		return jpegMetadata(f)
	}
	return nil
}

// pngMetadata scans PNG chunks up to the first IDAT (pixel data), reading only
// the small text chunks and seeking past everything else.
func pngMetadata(f *os.File) []docmeta.Entry {
	var sig [8]byte
	if _, err := io.ReadFull(f, sig[:]); err != nil || string(sig[1:4]) != "PNG" {
		return nil
	}
	var entries []docmeta.Entry
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			break
		}
		length := binary.BigEndian.Uint32(hdr[0:4])
		ctype := string(hdr[4:8])
		if ctype == "IDAT" || ctype == "IEND" {
			break // reached pixel data — never read it
		}
		switch ctype {
		case "tEXt", "iTXt", "zTXt":
			if length > 8<<20 {
				return entries // implausibly large text chunk; bail
			}
			data := make([]byte, length)
			if _, err := io.ReadFull(f, data); err != nil {
				return entries
			}
			entries = append(entries, pngTextEntry(ctype, data)...)
			if _, err := f.Seek(4, io.SeekCurrent); err != nil { // skip CRC
				return entries
			}
		default:
			if _, err := f.Seek(int64(length)+4, io.SeekCurrent); err != nil { // data + CRC
				return entries
			}
		}
	}
	return entries
}

// pngTextEntry parses one text chunk into metadata entries (or XMP).
func pngTextEntry(ctype string, data []byte) []docmeta.Entry {
	kw, rest, ok := bytes.Cut(data, []byte{0})
	if !ok {
		return nil
	}
	keyword := string(kw)
	var text string
	switch ctype {
	case "tEXt":
		text = decodeLatin1(rest)
	case "zTXt":
		if len(rest) < 1 {
			return nil
		}
		text = decodeLatin1([]byte(inflate(rest[1:]))) // rest[0] = compression method
	case "iTXt":
		if len(rest) < 2 {
			return nil
		}
		compressed := rest[0] == 1
		p := rest[2:] // skip compression flag + method
		if j := bytes.IndexByte(p, 0); j >= 0 {
			p = p[j+1:] // skip language tag
		}
		if k := bytes.IndexByte(p, 0); k >= 0 {
			p = p[k+1:] // skip translated keyword
		}
		if compressed {
			text = inflate(p)
		} else {
			text = string(p) // UTF-8
		}
	}
	text = strings.TrimSpace(text)
	if keyword == "XML:com.adobe.xmp" {
		return xmpEntries(text)
	}
	return pngKeywordEntry(keyword, text)
}

// pngKeywordEntry maps a standard PNG text keyword to a metadata entry.
func pngKeywordEntry(keyword, text string) []docmeta.Entry {
	if text == "" {
		return nil
	}
	key := "png:" + strings.ToLower(strings.ReplaceAll(keyword, " ", "-"))
	switch keyword {
	case "Title":
		return []docmeta.Entry{{Key: key, Value: text, Translatable: true, Role: model.RoleTitle}}
	case "Description", "Comment":
		return []docmeta.Entry{{Key: key, Value: text, Translatable: true}}
	default: // Author, Copyright, Software, Source, Creation Time, Disclaimer, …
		return []docmeta.Entry{{Key: key, Value: text}}
	}
}

// jpegMetadata scans JPEG marker segments up to SOS (scan data), extracting XMP
// from the APP1 segment. APP segments are bounded (≤64 KiB), so reading them is
// not the pixel payload.
func jpegMetadata(f *os.File) []docmeta.Entry {
	r := bufio.NewReader(f)
	var soi [2]byte
	if _, err := io.ReadFull(r, soi[:]); err != nil || soi[0] != 0xFF || soi[1] != 0xD8 {
		return nil
	}
	const xmpPrefix = "http://ns.adobe.com/xap/1.0/\x00"
	var entries []docmeta.Entry
	for {
		b, err := r.ReadByte()
		if err != nil {
			break
		}
		if b != 0xFF {
			continue
		}
		marker, err := r.ReadByte()
		for err == nil && marker == 0xFF { // skip fill bytes
			marker, err = r.ReadByte()
		}
		if err != nil {
			break
		}
		if marker == 0xDA || marker == 0xD9 { // SOS (scan) or EOI — stop before pixels
			break
		}
		if marker >= 0xD0 && marker <= 0xD7 { // RSTn — no length
			continue
		}
		var lb [2]byte
		if _, err := io.ReadFull(r, lb[:]); err != nil {
			break
		}
		length := int(binary.BigEndian.Uint16(lb[:])) - 2
		if length < 0 {
			break
		}
		if marker == 0xE1 { // APP1 — EXIF or XMP
			data := make([]byte, length)
			if _, err := io.ReadFull(r, data); err != nil {
				break
			}
			if bytes.HasPrefix(data, []byte(xmpPrefix)) {
				entries = append(entries, xmpEntries(string(data[len(xmpPrefix):]))...)
			}
			continue
		}
		if _, err := r.Discard(length); err != nil {
			break
		}
	}
	return entries
}

var (
	reXMPTitle   = regexp.MustCompile(`(?s)<dc:title>.*?<rdf:li[^>]*>(.*?)</rdf:li>`)
	reXMPDesc    = regexp.MustCompile(`(?s)<dc:description>.*?<rdf:li[^>]*>(.*?)</rdf:li>`)
	reXMPCreator = regexp.MustCompile(`(?s)<dc:creator>.*?<rdf:li[^>]*>(.*?)</rdf:li>`)
	reXMPSubject = regexp.MustCompile(`(?s)<dc:subject>(.*?)</dc:subject>`)
	reXMPLi      = regexp.MustCompile(`<rdf:li[^>]*>(.*?)</rdf:li>`)
)

// xmpEntries pulls the localizable Dublin-Core fields out of an XMP packet.
// dc:title/description are language-alternative text (translatable); dc:subject
// is a keyword bag (translatable); dc:creator is the author (non-translatable).
func xmpEntries(xml string) []docmeta.Entry {
	var out []docmeta.Entry
	if m := reXMPTitle.FindStringSubmatch(xml); m != nil {
		if v := strings.TrimSpace(html.UnescapeString(m[1])); v != "" {
			out = append(out, docmeta.Entry{Key: "xmp:dc:title", Value: v, Translatable: true, Role: model.RoleTitle})
		}
	}
	if m := reXMPDesc.FindStringSubmatch(xml); m != nil {
		if v := strings.TrimSpace(html.UnescapeString(m[1])); v != "" {
			out = append(out, docmeta.Entry{Key: "xmp:dc:description", Value: v, Translatable: true})
		}
	}
	if m := reXMPSubject.FindStringSubmatch(xml); m != nil {
		var kws []string
		for _, li := range reXMPLi.FindAllStringSubmatch(m[1], -1) {
			if v := strings.TrimSpace(html.UnescapeString(li[1])); v != "" {
				kws = append(kws, v)
			}
		}
		if len(kws) > 0 {
			out = append(out, docmeta.Entry{Key: "xmp:dc:subject", Value: strings.Join(kws, ", "), Translatable: true})
		}
	}
	if m := reXMPCreator.FindStringSubmatch(xml); m != nil {
		if v := strings.TrimSpace(html.UnescapeString(m[1])); v != "" {
			out = append(out, docmeta.Entry{Key: "xmp:dc:creator", Value: v})
		}
	}
	return out
}

func decodeLatin1(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		sb.WriteRune(rune(c))
	}
	return sb.String()
}

func inflate(b []byte) string {
	zr, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return ""
	}
	defer func() { _ = zr.Close() }()
	out, err := io.ReadAll(io.LimitReader(zr, 4<<20))
	if err != nil {
		return ""
	}
	return string(out)
}
