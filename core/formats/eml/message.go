package eml

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/textproto"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

const (
	maxMessageBytes = 32 << 20
	maxHeaderBytes  = 256 << 10
	maxEntities     = 1000
	maxNesting      = 32
)

type span struct{ start, end int }

type headers struct {
	values    textproto.MIMEHeader
	fields    map[string]span
	separator int
	newline   string
}

type field struct {
	span
	name, text        string
	headers           headers
	charset, transfer string
	params            map[string]string
	boundaries        []string
}

// The offsets address wire bytes. MIME decoders return unfolded or decoded values.
func readHeaders(raw []byte, start, end int) (headers, int, error) {
	h := headers{fields: make(map[string]span)}
	var last string
	pos := start
	for pos < end {
		n := bytes.IndexByte(raw[pos:end], '\n')
		if n < 0 || pos+n-start > maxHeaderBytes {
			return h, 0, errors.New("eml: incomplete headers or header size limit exceeded")
		}
		stop := pos + n
		h.newline = "\n"
		if stop > pos && raw[stop-1] == '\r' {
			stop--
			h.newline = "\r\n"
		}
		line := raw[pos:stop]
		next := pos + n + 1
		if len(line) == 0 {
			h.separator = pos
			pos = next
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			field := h.fields[last]
			field.end = stop
			h.fields[last] = field
		} else {
			colon := bytes.IndexByte(line, ':')
			if colon < 1 {
				return h, 0, errors.New("eml: malformed header field")
			}
			last = textproto.CanonicalMIMEHeaderKey(string(line[:colon]))
			value := colon + 1
			for value < len(line) && (line[value] == ' ' || line[value] == '\t') {
				value++
			}
			h.fields[last] = span{pos + value, stop}
		}
		pos = next
	}
	if pos == start {
		return h, 0, errors.New("eml: missing message headers")
	}
	// A message with headers alone can end after the last field's line ending.
	wire := io.MultiReader(bytes.NewReader(raw[start:pos]), strings.NewReader("\n"))
	values, err := textproto.NewReader(bufio.NewReader(wire)).ReadMIMEHeader()
	if err != nil {
		return h, 0, fmt.Errorf("eml: message headers: %w", err)
	}
	for _, key := range []string{"Subject", "Content-Type", "Content-Transfer-Encoding", "Content-Disposition"} {
		if len(values.Values(key)) > 1 {
			return h, 0, fmt.Errorf("eml: repeated %s header", key)
		}
	}
	h.values = values
	return h, pos, nil
}

func parseMessage(ctx context.Context, raw []byte) ([]field, error) {
	p := messageParser{raw: raw}
	if err := p.entity(ctx, 0, len(raw), "body", nil); err != nil {
		return nil, err
	}
	return p.fields, nil
}

type messageParser struct {
	raw    []byte
	fields []field
	count  int
}

func (p *messageParser) entity(ctx context.Context, start, end int, path string, boundaries []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.count++
	if p.count > maxEntities || len(boundaries) > maxNesting {
		return errors.New("eml: MIME part count or nesting limit exceeded")
	}
	h, body, err := readHeaders(p.raw, start, end)
	if err != nil {
		return err
	}
	if len(boundaries) == 0 {
		if subject, ok := h.fields["Subject"]; ok {
			value, err := new(mime.WordDecoder).DecodeHeader(h.values.Get("Subject"))
			if err != nil {
				return fmt.Errorf("eml: subject: %w", err)
			}
			if !utf8.ValidString(value) {
				return errors.New("eml: invalid subject encoding")
			}
			p.fields = append(p.fields, field{span: subject, name: "subject", text: value, headers: h})
		}
	}
	typ, params, err := parseType(h.values.Get("Content-Type"), "text/plain")
	if err != nil {
		return fmt.Errorf("eml: content type: %w", err)
	}
	disposition, dispositionParams, err := parseType(h.values.Get("Content-Disposition"), "inline")
	if err != nil {
		return fmt.Errorf("eml: disposition: %w", err)
	}
	if disposition != "inline" || dispositionParams["filename"] != "" || params["name"] != "" {
		return nil
	}
	transfer := strings.ToLower(h.values.Get("Content-Transfer-Encoding"))
	switch typ {
	case "multipart/mixed", "multipart/alternative", "multipart/related":
		if transfer != "" && transfer != "7bit" && transfer != "8bit" && transfer != "binary" {
			return errors.New("eml: encoded multipart container")
		}
		boundary := params["boundary"]
		if boundary == "" || len(boundary) > 70 || strings.ContainsAny(boundary, "\r\n") {
			return errors.New("eml: invalid MIME boundary")
		}
		return p.multipart(ctx, body, end, path, append(boundaries, boundary))
	case "text/plain":
		if strings.EqualFold(params["format"], "flowed") {
			return nil
		}
	default:
		return nil
	}

	// The newline introducing a delimiter belongs to the multipart container.
	// Keep a top-level terminal line ending outside the editable raw text, too.
	if len(boundaries) > 0 || transfer != "quoted-printable" {
		end = body + len(trimLineEnd(p.raw[body:end]))
	}
	charset := strings.ToLower(params["charset"])
	text, supported, err := decodeBody(p.raw[body:end], transfer, charset)
	if err != nil {
		return fmt.Errorf("eml: %s: %w", path, err)
	}
	if supported && text != "" {
		p.fields = append(p.fields, field{
			span: span{body, end}, name: path, text: text, headers: h,
			charset: charset, transfer: transfer, params: params,
			boundaries: append([]string(nil), boundaries...),
		})
	}
	return nil
}

func parseType(value, fallback string) (string, map[string]string, error) {
	if value == "" {
		value = fallback
	}
	return mime.ParseMediaType(value)
}

func trimLineEnd(line []byte) []byte {
	if bytes.HasSuffix(line, []byte("\n")) {
		return bytes.TrimSuffix(line[:len(line)-1], []byte("\r"))
	}
	return line
}

func (p *messageParser) multipart(ctx context.Context, start, end int, path string, boundaries []string) error {
	marker := "--" + boundaries[len(boundaries)-1]
	partStart, partNumber := -1, 0
	for pos := start; pos < end; {
		if err := ctx.Err(); err != nil {
			return err
		}
		next := end
		if n := bytes.IndexByte(p.raw[pos:end], '\n'); n >= 0 {
			next = pos + n + 1
		}
		line := bytes.TrimRight(trimLineEnd(p.raw[pos:next]), " \t")
		final := string(line) == marker+"--"
		if final || string(line) == marker {
			if partStart >= 0 {
				partNumber++
				if err := p.entity(ctx, partStart, pos, fmt.Sprintf("%s.%d", path, partNumber), boundaries); err != nil {
					return err
				}
			}
			if final {
				return nil
			}
			partStart = next
		}
		pos = next
	}
	return fmt.Errorf("eml: %s: missing closing MIME delimiter", path)
}

func decodeBody(raw []byte, transfer, charset string) (string, bool, error) {
	switch charset {
	case "", "us-ascii", "utf-8", "iso-8859-1":
	default:
		return "", false, nil
	}
	var r io.Reader = bytes.NewReader(raw)
	switch transfer {
	case "", "7bit", "8bit", "binary":
	case "quoted-printable":
		r = quotedprintable.NewReader(r)
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, r)
	default:
		return "", false, nil
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		return "", false, err
	}
	if charset == "iso-8859-1" {
		decoded, err = charmap.ISO8859_1.NewDecoder().Bytes(decoded)
	}
	if err != nil {
		return "", false, err
	}
	if !utf8.Valid(decoded) {
		return "", false, errors.New("invalid text encoding")
	}
	return strings.ReplaceAll(string(decoded), "\r\n", "\n"), true, nil
}
