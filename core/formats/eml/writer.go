package eml

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/text/encoding/charmap"
)

type Writer struct {
	format.BaseFormatWriter
	store    *format.SkeletonStore
	original []byte
}

func NewWriter() *Writer {
	return &Writer{FormatName: name, RequiresSkeleton: true, Binary: true}
}

func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) { w.store = store }
func (w *Writer) SetOriginalContent(raw []byte)                { w.original = raw }

type replacement struct {
	span
	data []byte
}

func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocks := map[string]*model.Block{}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.render(ctx, blocks)
			}
			if block, ok := part.Resource.(*model.Block); ok {
				blocks[block.ID] = block
			}
		}
	}
}

func (w *Writer) source() ([]byte, error) {
	if w.store == nil {
		if len(w.original) == 0 {
			return nil, errors.New("eml: write requires the original message or its skeleton")
		}
		if len(w.original) > maxMessageBytes {
			return nil, errors.New("eml: message size limit exceeded")
		}
		return w.original, nil
	}
	if err := w.store.Flush(); err != nil {
		return nil, err
	}
	var raw bytes.Buffer
	for {
		entry, err := w.store.Next()
		if errors.Is(err, io.EOF) {
			return raw.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		data := entry.Data
		switch entry.Type {
		case format.SkeletonText:
		case format.SkeletonOriginal:
			var ok bool
			_, data, ok = format.DecodeSkeletonPair(data)
			if !ok {
				return nil, errors.New("eml: invalid skeleton value")
			}
		case format.SkeletonRef:
			continue
		default:
			return nil, errors.New("eml: invalid skeleton entry")
		}
		if len(data) > maxMessageBytes-raw.Len() {
			return nil, errors.New("eml: message size limit exceeded")
		}
		raw.Write(data)
	}
}

func (w *Writer) render(ctx context.Context, blocks map[string]*model.Block) error {
	raw, err := w.source()
	if err != nil {
		return err
	}
	fields, err := parseMessage(ctx, raw)
	if err != nil {
		return err
	}
	var edits []replacement
	for _, f := range fields {
		b := blocks[f.name]
		if b == nil {
			continue
		}
		text := b.SourceText()
		if !w.Locale.IsEmpty() && b.HasTarget(w.Locale) {
			text = b.TargetText(w.Locale)
		} else if b.Name == "" && text == "" {
			// A skeleton-only merge supplies a block ID and its available target.
			continue
		}
		if text == f.text {
			continue
		}
		if len(text) > maxMessageBytes {
			return errors.New("eml: edited text exceeds message size limit")
		}
		if !utf8.ValidString(text) {
			return fmt.Errorf("eml: %s edit is not UTF-8", f.name)
		}
		if f.name == "subject" {
			subject, err := encodeSubject(text, f.headers.newline)
			if err != nil {
				return err
			}
			edits = append(edits, replacement{f.span, []byte(subject)})
		} else {
			body, err := f.bodyEdits(text)
			if err != nil {
				return err
			}
			edits = append(edits, body...)
		}
	}
	slices.SortStableFunc(edits, func(a, b replacement) int { return cmp.Compare(a.start, b.start) })
	size := int64(len(raw))
	for _, edit := range edits {
		size += int64(len(edit.data)) - int64(edit.end-edit.start)
	}
	if size > maxMessageBytes {
		return errors.New("eml: edited message exceeds size limit")
	}
	pos := 0
	for _, edit := range edits {
		if _, err := w.Output.Write(raw[pos:edit.start]); err != nil {
			return err
		}
		if _, err := w.Output.Write(edit.data); err != nil {
			return err
		}
		pos = edit.end
	}
	_, err = w.Output.Write(raw[pos:])
	return err
}

func encodeSubject(text, newline string) (string, error) {
	for _, r := range text {
		if r == '\r' || r == '\n' || (r < ' ' && r != '\t') || r == 127 {
			return "", errors.New("eml: subject contains a control character")
		}
	}
	if strings.TrimSpace(text) == text && isASCII(text) && !strings.Contains(text, "=?") && len(text) <= 980 {
		return text, nil
	}
	// Folding between encoded words also preserves literal RFC 2047 syntax and long ASCII words.
	var words []string
	for len(text) > 0 {
		n := min(len(text), 42)
		for n < len(text) && !utf8.RuneStart(text[n]) {
			n--
		}
		words = append(words, "=?UTF-8?B?"+base64.StdEncoding.EncodeToString([]byte(text[:n]))+"?=")
		text = text[n:]
	}
	return strings.Join(words, newline+" "), nil
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func (f field) bodyEdits(text string) ([]replacement, error) {
	newline := f.headers.newline
	wire := []byte(strings.ReplaceAll(text, "\n", newline))
	var edits []replacement
	header := func(key, value string) {
		if old, ok := f.headers.fields[key]; ok {
			edits = append(edits, replacement{old, []byte(value)})
		} else {
			edits = append(edits, replacement{span{f.headers.separator, f.headers.separator}, []byte(key + ": " + value + newline)})
		}
	}
	charset := f.charset
	if charset == "iso-8859-1" {
		if encoded, err := charmap.ISO8859_1.NewEncoder().Bytes(wire); err == nil {
			wire = encoded
		} else {
			charset = "utf-8"
		}
	} else if !isASCII(text) {
		charset = "utf-8"
	}
	if charset != f.charset {
		f.params["charset"] = charset
		header("Content-Type", mime.FormatMediaType("text/plain", f.params))
	}
	transfer := f.transfer
	switch transfer {
	case "base64":
	case "quoted-printable":
		var buf bytes.Buffer
		qp := quotedprintable.NewWriter(&buf)
		if _, err := qp.Write(wire); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
		encoded := buf.Bytes()
		// quotedprintable.Writer canonicalizes isolated CR and LF.
		if strings.Contains(text, "\r") || collides(encoded, f.boundaries) {
			transfer = "base64"
		} else {
			wire = bytes.ReplaceAll(encoded, []byte("\r\n"), []byte(newline))
		}
	default:
		if (!isASCII(string(wire)) && (transfer == "" || transfer == "7bit")) || strings.HasSuffix(text, "\n") || !safeBodyLines(wire) || collides(wire, f.boundaries) {
			transfer = "base64"
		}
	}
	if transfer == "base64" {
		wire = foldBase64(wire, newline)
	}
	if transfer != f.transfer {
		header("Content-Transfer-Encoding", transfer)
	}
	if len(edits) > 0 && f.name == "body" && f.headers.values.Get("MIME-Version") == "" {
		header("MIME-Version", "1.0")
	}
	return append(edits, replacement{f.span, wire}), nil
}

func safeBodyLines(wire []byte) bool {
	for line := range bytes.SplitSeq(wire, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if len(line) > 998 {
			return false
		}
		for _, b := range line {
			if b < ' ' && b != '\t' || b == 127 {
				return false
			}
		}
	}
	return true
}

func collides(wire []byte, boundaries []string) bool {
	for line := range bytes.SplitSeq(wire, []byte("\n")) {
		line = bytes.TrimRight(line, "\r \t")
		for _, b := range boundaries {
			if string(line) == "--"+b || string(line) == "--"+b+"--" {
				return true
			}
		}
	}
	return false
}

func foldBase64(wire []byte, newline string) []byte {
	encoded := base64.StdEncoding.EncodeToString(wire)
	var out bytes.Buffer
	for len(encoded) > 76 {
		out.WriteString(encoded[:76])
		out.WriteString(newline)
		encoded = encoded[76:]
	}
	out.WriteString(encoded)
	return out.Bytes()
}
