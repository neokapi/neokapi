package pdf

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for PDF files (text extraction).
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new PDF reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "pdf",
			FormatDisplayName: "PDF Text Extraction",
			FormatMimeType:    "application/pdf",
			FormatExtensions:  []string{".pdf"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/pdf"},
		Extensions: []string{".pdf"},
		MagicBytes: [][]byte{[]byte("%PDF-")},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("pdf: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("pdf: reading: %w", err)}
		return
	}

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "pdf",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/pdf",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Extract text from all pages
	pages := extractPages(content)
	blockCounter := 0

	for pageNum, pageText := range pages {
		if pageText == "" {
			continue
		}

		// Emit a child layer for each page
		pageLayer := &model.Layer{
			ID:     fmt.Sprintf("page%d", pageNum+1),
			Name:   fmt.Sprintf("Page %d", pageNum+1),
			Format: "pdf",
			Locale: locale,
			Properties: map[string]string{
				"page-number": strconv.Itoa(pageNum + 1),
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: pageLayer}) {
			return
		}

		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), strings.TrimSpace(pageText))
		block.Name = fmt.Sprintf("page%d", pageNum+1)
		block.Properties["page-number"] = strconv.Itoa(pageNum + 1)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: pageLayer}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// extractPages extracts text content from PDF data, returning text per page.
func extractPages(data []byte) []string {
	// Find all stream content in the PDF
	streams := extractStreams(data)

	var pages []string
	for _, stream := range streams {
		text := extractTextFromStream(stream)
		if text != "" {
			pages = append(pages, text)
		}
	}

	return pages
}

// extractStreams finds and decompresses all streams in the PDF.
func extractStreams(data []byte) [][]byte {
	var streams [][]byte

	// Find all "stream" / "endstream" pairs
	streamStart := []byte("stream")
	streamEnd := []byte("endstream")

	offset := 0
	for offset < len(data) {
		idx := bytes.Index(data[offset:], streamStart)
		if idx < 0 {
			break
		}
		startPos := offset + idx + len(streamStart)

		// Skip \r\n or \n after "stream"
		if startPos < len(data) && data[startPos] == '\r' {
			startPos++
		}
		if startPos < len(data) && data[startPos] == '\n' {
			startPos++
		}

		endIdx := bytes.Index(data[startPos:], streamEnd)
		if endIdx < 0 {
			break
		}
		endPos := startPos + endIdx

		// Trim trailing whitespace before endstream
		streamData := data[startPos:endPos]
		for len(streamData) > 0 && (streamData[len(streamData)-1] == '\r' || streamData[len(streamData)-1] == '\n') {
			streamData = streamData[:len(streamData)-1]
		}

		// Check if the object header preceding this stream mentions FlateDecode
		headerStart := offset + idx
		lookback := min(headerStart, 512)
		header := string(data[headerStart-lookback : headerStart])

		if strings.Contains(header, "FlateDecode") {
			if decoded, ok := flateDecode(streamData); ok {
				streams = append(streams, decoded)
			}
		} else {
			// Use raw stream data
			streams = append(streams, streamData)
		}

		offset = endPos + len(streamEnd)
	}

	return streams
}

// flateDecode inflates a FlateDecode stream. Per ISO 32000-1 §7.4.4.2 a
// FlateDecode stream is the zlib/deflate compressed-data format described in
// RFC 1950 and RFC 1951 — i.e. raw DEFLATE wrapped in a zlib container (a
// two-byte header plus an Adler-32 trailer). zlib.NewReader honours that
// container; compress/flate alone does not, so a zlib stream fed to
// flate.NewReader fails to inflate (the historic native bug behind #510 /
// #616 that dropped every compressed PDF stream). A small minority of broken
// producers emit bare RFC 1951 DEFLATE with no zlib wrapper, so fall back to
// flate.NewReader when the zlib header is absent.
func flateDecode(data []byte) ([]byte, bool) {
	if zr, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
		decoded, rerr := io.ReadAll(zr)
		zr.Close()
		if rerr == nil {
			return decoded, true
		}
	}
	// Fallback: bare DEFLATE (RFC 1951) with no zlib wrapper.
	fr := flate.NewReader(bytes.NewReader(data))
	decoded, err := io.ReadAll(fr)
	fr.Close()
	if err == nil {
		return decoded, true
	}
	return nil, false
}

// extractTextFromStream extracts text from a PDF content stream.
func extractTextFromStream(stream []byte) string {
	s := string(stream)

	// Find text between BT (Begin Text) and ET (End Text) markers
	var allText []string

	btIdx := 0
	for {
		bt := strings.Index(s[btIdx:], "BT")
		if bt < 0 {
			break
		}
		bt += btIdx

		et := strings.Index(s[bt:], "ET")
		if et < 0 {
			break
		}
		et += bt

		textBlock := s[bt : et+2]
		texts := extractTextOps(textBlock)
		if len(texts) > 0 {
			allText = append(allText, texts...)
		}

		btIdx = et + 2
	}

	return strings.Join(allText, " ")
}

// extractTextOps extracts the text shown by the text-showing operators (Tj,
// TJ, ', ") inside a BT/ET block.
//
// It walks the block as a PDF token stream rather than matching regexes, so it
// honours the literal-string grammar from ISO 32000-1 §7.3.4.2: a string runs
// from the opening "(" to its balanced closing ")", where "\(", "\)" and "\\"
// are escaped delimiters and unescaped parens nest. The earlier regex
// (`\(([^)]*)\)`) stopped at the first ")" — escaped or not — so a literal like
// `(Cost: \(US\$50\) \\ \101OK)` never matched and the run was dropped (#510 /
// #616). The scanner instead collects every string literal (and hex string)
// and flushes the pending operands when it reaches a text-showing operator,
// which covers both the standalone `(text) Tj` form and the `[(t) num (t)] TJ`
// kerning-array form uniformly.
func extractTextOps(block string) []string {
	var texts []string
	var pending strings.Builder // accumulates strings until an operator flushes them

	flush := func() {
		if pending.Len() > 0 {
			texts = append(texts, pending.String())
			pending.Reset()
		}
	}

	i := 0
	n := len(block)
	for i < n {
		c := block[i]
		switch {
		case c == '(':
			// Literal string: scan to the balanced closing paren.
			lit, next := scanLiteralString(block, i)
			pending.WriteString(decodePDFString(lit))
			i = next
		case c == '<' && i+1 < n && block[i+1] != '<':
			// Hex string: <48656C6C6F>. (Skip "<<" dictionary openers.)
			hexVal, next := scanHexString(block, i)
			pending.WriteString(hexVal)
			i = next
		case c == 'T' && i+1 < n && block[i+1] == 'j':
			// Tj — show string.
			flush()
			i += 2
		case c == 'T' && i+1 < n && block[i+1] == 'J':
			// TJ — show array of strings with kerning.
			flush()
			i += 2
		case c == '\'':
			// ' — move to next line and show string.
			flush()
			i++
		case c == '"':
			// " — set spacing, move to next line, show string.
			flush()
			i++
		default:
			i++
		}
	}
	flush()

	if len(texts) == 0 {
		return nil
	}
	return texts
}

// scanLiteralString parses a PDF literal string starting at the "(" at start.
// It returns the raw inner bytes (escapes still encoded) and the index just
// past the closing ")". Per ISO 32000-1 §7.3.4.2: "\(", "\)" and "\\" are
// escaped and unescaped parentheses must balance. If the string is unbalanced
// (truncated stream) it consumes to the end of the block.
func scanLiteralString(s string, start int) (string, int) {
	var buf strings.Builder
	depth := 0
	i := start
	for i < len(s) {
		c := s[i]
		switch c {
		case '\\':
			// Keep the backslash and the escaped byte verbatim; decodePDFString
			// resolves the escape later.
			buf.WriteByte(c)
			if i+1 < len(s) {
				buf.WriteByte(s[i+1])
				i += 2
			} else {
				i++
			}
			continue
		case '(':
			depth++
			if depth > 1 {
				buf.WriteByte(c)
			}
		case ')':
			depth--
			if depth == 0 {
				return buf.String(), i + 1
			}
			buf.WriteByte(c)
		default:
			buf.WriteByte(c)
		}
		i++
	}
	return buf.String(), i
}

// scanHexString parses a PDF hexadecimal string starting at the "<" at start
// (ISO 32000-1 §7.3.4.3). Each pair of hex digits is one byte; a trailing odd
// digit is padded with 0. Whitespace inside the angle brackets is ignored. It
// returns the decoded bytes as a string and the index just past the closing
// ">". These bytes are font-code units, not necessarily Unicode — without a
// ToUnicode CMap they are only meaningful for single-byte ASCII fonts, so the
// result is filtered to printable characters to avoid surfacing glyph-index
// garbage.
func scanHexString(s string, start int) (string, int) {
	i := start + 1
	var hexDigits []byte
	for i < len(s) && s[i] != '>' {
		c := s[i]
		if isHexDigit(c) {
			hexDigits = append(hexDigits, c)
		}
		i++
	}
	if i < len(s) {
		i++ // consume '>'
	}
	if len(hexDigits)%2 == 1 {
		hexDigits = append(hexDigits, '0')
	}
	var buf strings.Builder
	for j := 0; j+1 < len(hexDigits); j += 2 {
		hi := hexValue(hexDigits[j])
		lo := hexValue(hexDigits[j+1])
		b := hi<<4 | lo
		// Only keep printable ASCII / common whitespace; glyph-index bytes
		// from CID fonts are not Unicode and would be garbage here.
		if b == '\t' || b == '\n' || b == '\r' || (b >= 0x20 && b < 0x7f) {
			buf.WriteByte(b)
		}
	}
	return buf.String(), i
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexValue(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// decodePDFString decodes a PDF string, handling common escape sequences.
func decodePDFString(s string) string {
	var buf strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case 'b':
				buf.WriteByte('\b')
			case 'f':
				buf.WriteByte('\f')
			case '(':
				buf.WriteByte('(')
			case ')':
				buf.WriteByte(')')
			case '\\':
				buf.WriteByte('\\')
			default:
				// Octal escape
				if s[i] >= '0' && s[i] <= '7' {
					octal := string(s[i])
					i++
					var octalSb324 strings.Builder
					for j := 0; j < 2 && i < len(s) && s[i] >= '0' && s[i] <= '7'; j++ {
						octalSb324.WriteString(string(s[i]))
						i++
					}
					octal += octalSb324.String()
					if val, err := strconv.ParseUint(octal, 8, 8); err == nil {
						buf.WriteByte(byte(val))
					}
					continue
				}
				buf.WriteByte(s[i])
			}
		} else {
			buf.WriteByte(s[i])
		}
		i++
	}
	return buf.String()
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
