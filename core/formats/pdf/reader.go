package pdf

import (
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
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
		lookback := 512
		if headerStart < lookback {
			lookback = headerStart
		}
		header := string(data[headerStart-lookback : headerStart])

		if strings.Contains(header, "FlateDecode") {
			// Try to decompress
			reader := flate.NewReader(bytes.NewReader(streamData))
			decoded, err := io.ReadAll(reader)
			reader.Close()
			if err == nil {
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

// Regex patterns for PDF text operators
var (
	// Matches (text) Tj operator
	tjPattern = regexp.MustCompile(`\(([^)]*)\)\s*Tj`)
	// Matches (text) ' operator (move to next line and show text)
	tickPattern = regexp.MustCompile(`\(([^)]*)\)\s*'`)
	// Matches TJ array operator: [(text) num (text) ...] TJ
	tjArrayPattern = regexp.MustCompile(`\[([^\]]*)\]\s*TJ`)
	// Matches individual strings within TJ arrays
	tjArrayString = regexp.MustCompile(`\(([^)]*)\)`)
)

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

// extractTextOps extracts text strings from PDF text operators within a BT/ET block.
func extractTextOps(block string) []string {
	var texts []string

	// Handle Tj operator: (text) Tj
	for _, match := range tjPattern.FindAllStringSubmatch(block, -1) {
		if len(match) > 1 {
			text := decodePDFString(match[1])
			if text != "" {
				texts = append(texts, text)
			}
		}
	}

	// Handle ' operator: (text) '
	for _, match := range tickPattern.FindAllStringSubmatch(block, -1) {
		if len(match) > 1 {
			text := decodePDFString(match[1])
			if text != "" {
				texts = append(texts, text)
			}
		}
	}

	// Handle TJ array operator: [(text) num (text)] TJ
	for _, match := range tjArrayPattern.FindAllStringSubmatch(block, -1) {
		if len(match) > 1 {
			arrayContent := match[1]
			for _, strMatch := range tjArrayString.FindAllStringSubmatch(arrayContent, -1) {
				if len(strMatch) > 1 {
					text := decodePDFString(strMatch[1])
					if text != "" {
						texts = append(texts, text)
					}
				}
			}
		}
	}

	return texts
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
