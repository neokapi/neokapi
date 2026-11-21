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

	"github.com/neokapi/neokapi/core/format"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
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
	// lineEnd reports that a line ending follows the span outside it, so the
	// writer can end an edit on a newline without changing the decoded text.
	lineEnd bool
}

// subjectDecoder decodes RFC 2047 encoded words in any charset the WHATWG
// encoding index names. The mime package handles UTF-8, US-ASCII and
// ISO-8859-1 itself and asks CharsetReader for the rest.
var subjectDecoder = &mime.WordDecoder{CharsetReader: charsetReader}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	enc, err := htmlindex.Get(charset)
	if err != nil || enc == encoding.Replacement {
		return nil, fmt.Errorf("unhandled charset %q", charset)
	}
	return enc.NewDecoder().Reader(input), nil
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

// parseMessage returns the editable fields in message order and the notes a
// reader reports as validation diagnostics: content it left in the skeleton,
// and edits whose effect a mail client may not show.
func parseMessage(ctx context.Context, raw []byte) ([]field, []format.Diagnostic, error) {
	p := messageParser{raw: raw}
	if _, err := p.entity(ctx, 0, len(raw), "body", nil); err != nil {
		return nil, nil, err
	}
	return p.fields, p.notes, nil
}

type messageParser struct {
	raw    []byte
	fields []field
	notes  []format.Diagnostic
	count  int
}

func (p *messageParser) note(category string, offset int, message string) {
	p.notes = append(p.notes, format.Diagnostic{
		Severity: format.SeverityMinor, Category: category, Message: message, ByteOffset: offset,
	})
}

// entity reads one MIME entity and returns its media type.
func (p *messageParser) entity(ctx context.Context, start, end int, path string, boundaries []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p.count++
	if p.count > maxEntities || len(boundaries) > maxNesting {
		return "", errors.New("eml: MIME part count or nesting limit exceeded")
	}
	h, body, err := readHeaders(p.raw, start, end)
	if err != nil {
		return "", err
	}
	if len(boundaries) == 0 {
		if subject, ok := h.fields["Subject"]; ok {
			value, err := subjectDecoder.DecodeHeader(h.values.Get("Subject"))
			switch {
			case err != nil:
				p.note("encoding.eml-subject-charset", subject.start, "The subject is left unchanged: "+err.Error()+".")
			case !utf8.ValidString(value):
				p.note("encoding.eml-subject-8bit", subject.start, "The subject is left unchanged: it holds 8-bit text in no declared charset.")
			default:
				p.fields = append(p.fields, field{span: subject, name: "subject", text: value, headers: h})
			}
		}
		if dkim, ok := h.fields["Dkim-Signature"]; ok {
			p.note("structure.eml-dkim-signature", dkim.start, "The message carries a DKIM signature. An edited subject or body no longer matches it.")
		}
	}
	typ, params, err := parseType(h.values.Get("Content-Type"), "text/plain")
	if err != nil {
		return "", fmt.Errorf("eml: content type: %w", err)
	}
	disposition, dispositionParams, err := parseType(h.values.Get("Content-Disposition"), "inline")
	if err != nil {
		return "", fmt.Errorf("eml: disposition: %w", err)
	}
	if disposition != "inline" || dispositionParams["filename"] != "" || params["name"] != "" {
		return "", nil
	}
	transfer := strings.ToLower(h.values.Get("Content-Transfer-Encoding"))
	switch typ {
	case "multipart/mixed", "multipart/alternative", "multipart/related":
		if transfer != "" && transfer != "7bit" && transfer != "8bit" && transfer != "binary" {
			return "", errors.New("eml: encoded multipart container")
		}
		boundary := params["boundary"]
		if boundary == "" || len(boundary) > 70 || strings.ContainsAny(boundary, "\r\n") {
			return "", errors.New("eml: invalid MIME boundary")
		}
		return typ, p.multipart(ctx, body, end, path, typ, append(boundaries, boundary))
	case "text/plain":
		if strings.EqualFold(params["format"], "flowed") {
			p.note("structure.eml-format-flowed", body, path+" is left unchanged: format=flowed text is not extracted.")
			return typ, nil
		}
	default:
		return typ, nil
	}

	// A line ending that closes the body stays outside the editable raw text.
	// Inside a multipart it introduces the next delimiter. At the top level, a
	// quoted-printable soft line break keeps its line ending.
	lineEnd := false
	trimmed := body + len(trimLineEnd(p.raw[body:end]))
	soft := transfer == "quoted-printable" && bytes.HasSuffix(p.raw[body:trimmed], []byte("="))
	if len(boundaries) > 0 || !soft {
		end, lineEnd = trimmed, trimmed < end
	}
	charset := strings.ToLower(params["charset"])
	text, reason, err := decodeBody(p.raw[body:end], transfer, charset)
	if err != nil {
		return "", fmt.Errorf("eml: %s: %w", path, err)
	}
	switch {
	case reason != "":
		p.note("encoding.eml-body-charset", body, path+" is left unchanged: "+reason+".")
	case text != "":
		p.fields = append(p.fields, field{
			span: span{body, end}, name: path, text: text, headers: h,
			charset: charset, transfer: transfer, params: params,
			boundaries: append([]string(nil), boundaries...), lineEnd: lineEnd,
		})
	}
	return typ, nil
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

func (p *messageParser) multipart(ctx context.Context, start, end int, path, typ string, boundaries []string) error {
	marker := "--" + boundaries[len(boundaries)-1]
	partStart, partNumber := -1, 0
	fields, html := len(p.fields), false
	part := func(partEnd int) error {
		partNumber++
		child, err := p.entity(ctx, partStart, partEnd, fmt.Sprintf("%s.%d", path, partNumber), boundaries)
		html = html || child == "text/html"
		return err
	}
	done := func() {
		if typ == "multipart/alternative" && html && len(p.fields) > fields {
			p.note("structure.eml-html-alternative", start, path+" has an HTML alternative that is left in the original language. A mail client that prefers HTML shows the original body.")
		}
	}
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
				if err := part(pos); err != nil {
					return err
				}
			}
			if final {
				done()
				return nil
			}
			partStart = next
		}
		pos = next
	}
	// End of input closes a multipart whose closing delimiter is missing.
	p.note("structure.eml-missing-delimiter", end, path+" has no closing MIME delimiter. The end of the message closes it.")
	if partStart >= 0 && partStart < end {
		if err := part(end); err != nil {
			return err
		}
	}
	done()
	return nil
}

// decodeBody returns the body text, or the reason it is left unchanged.
func decodeBody(raw []byte, transfer, charset string) (string, string, error) {
	switch charset {
	case "", "us-ascii", "utf-8", "utf8", "iso-8859-1":
	default:
		return "", fmt.Sprintf("charset %q is not supported", charset), nil
	}
	var r io.Reader = bytes.NewReader(raw)
	switch transfer {
	case "", "7bit", "8bit", "binary":
	case "quoted-printable":
		r = quotedprintable.NewReader(r)
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, r)
	default:
		return "", fmt.Sprintf("transfer encoding %q is not supported", transfer), nil
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		return "", "", err
	}
	if charset == "iso-8859-1" {
		decoded, err = charmap.ISO8859_1.NewDecoder().Bytes(decoded)
	}
	if err != nil {
		return "", "", err
	}
	if !utf8.Valid(decoded) {
		if charset == "" {
			return "", "it holds 8-bit text in no declared charset", nil
		}
		return "", fmt.Sprintf("it is not valid %s text", charset), nil
	}
	return strings.ReplaceAll(string(decoded), "\r\n", "\n"), "", nil
}
