package properties

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
)

// Reader implements DataFormatReader for Java Properties files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new Properties reader.
func NewReader() *Reader {
	cfg := &Config{Separator: "="}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "properties",
			FormatDisplayName: "Java Properties",
			FormatMimeType:    "text/x-java-properties",
			FormatExtensions:  []string{".properties"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/x-java-properties"},
		Extensions: []string{".properties"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("properties: nil document or reader")
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

// logicalLine represents a complete logical line after joining continuations.
type logicalLine struct {
	content   string
	isComment bool
	isBlank   bool
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Emit layer start
	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "properties",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-java-properties",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	lines := r.readLogicalLines()
	blockID := 0
	dataID := 0

	for _, line := range lines {
		if line.isBlank {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "blank",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		if line.isComment {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "comment",
				Properties: map[string]string{
					"comment": line.content,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		// Parse key=value
		key, value, sep := parseProperty(line.content)
		value = decodeUnicodeEscapes(value)

		blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockID), value)
		block.Name = key
		block.Properties["separator"] = sep
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// readLogicalLines reads all lines, joining continuation lines (backslash at EOL).
func (r *Reader) readLogicalLines() []logicalLine {
	scanner := bufio.NewScanner(r.Doc.Reader)
	var lines []logicalLine
	var continuation strings.Builder
	inContinuation := false

	for scanner.Scan() {
		raw := scanner.Text()

		if inContinuation {
			// Continuation: trim leading whitespace
			trimmed := strings.TrimLeft(raw, " \t")
			if hasContinuation(trimmed) {
				continuation.WriteString(trimmed[:len(trimmed)-1])
			} else {
				continuation.WriteString(trimmed)
				lines = append(lines, logicalLine{content: continuation.String()})
				inContinuation = false
				continuation.Reset()
			}
			continue
		}

		// Blank line
		if strings.TrimSpace(raw) == "" {
			lines = append(lines, logicalLine{isBlank: true})
			continue
		}

		// Comment line (# or !)
		trimmed := strings.TrimLeft(raw, " \t")
		if len(trimmed) > 0 && (trimmed[0] == '#' || trimmed[0] == '!') {
			lines = append(lines, logicalLine{content: raw, isComment: true})
			continue
		}

		// Regular or continuation line
		if hasContinuation(raw) {
			continuation.WriteString(raw[:len(raw)-1])
			inContinuation = true
		} else {
			lines = append(lines, logicalLine{content: raw})
		}
	}

	// If the file ends mid-continuation, emit what we have
	if inContinuation {
		lines = append(lines, logicalLine{content: continuation.String()})
	}

	return lines
}

// hasContinuation checks if a line ends with an odd number of backslashes
// (indicating a continuation).
func hasContinuation(line string) bool {
	if len(line) == 0 {
		return false
	}
	count := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

// parseProperty splits a logical line into key, value, and separator.
// Handles key=value, key:value, and key value forms.
func parseProperty(line string) (key, value, sep string) {
	line = strings.TrimLeft(line, " \t")

	// Parse the key: look for unescaped separator (=, :, or whitespace)
	var keyBuf strings.Builder
	i := 0
	for i < len(line) {
		if line[i] == '\\' && i+1 < len(line) {
			// Escaped character in key
			keyBuf.WriteByte(line[i])
			keyBuf.WriteByte(line[i+1])
			i += 2
			continue
		}
		if line[i] == '=' || line[i] == ':' {
			sep = string(line[i])
			key = keyBuf.String()
			value = strings.TrimLeft(line[i+1:], " \t")
			return key, value, sep
		}
		if line[i] == ' ' || line[i] == '\t' {
			key = keyBuf.String()
			// Skip whitespace to find the separator or value
			j := i
			for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
				j++
			}
			if j < len(line) && (line[j] == '=' || line[j] == ':') {
				sep = string(line[j])
				value = strings.TrimLeft(line[j+1:], " \t")
			} else {
				sep = " "
				value = strings.TrimLeft(line[j:], " \t")
				if j < len(line) {
					value = line[j:]
				}
			}
			return key, value, sep
		}
		keyBuf.WriteByte(line[i])
		i++
	}

	// Key only, no value
	key = keyBuf.String()
	sep = "="
	return key, "", sep
}

// decodeUnicodeEscapes processes \uXXXX sequences in a string.
func decodeUnicodeEscapes(s string) string {
	if !strings.Contains(s, "\\u") {
		return s
	}

	var buf strings.Builder
	i := 0
	for i < len(s) {
		if i+5 < len(s) && s[i] == '\\' && s[i+1] == 'u' {
			// Try to parse 4 hex digits
			hex := s[i+2 : i+6]
			r, ok := parseHexRune(hex)
			if ok {
				buf.WriteRune(r)
				i += 6
				continue
			}
		}
		buf.WriteByte(s[i])
		i++
	}
	return buf.String()
}

// parseHexRune converts a 4-character hex string to a rune.
func parseHexRune(hex string) (rune, bool) {
	if len(hex) != 4 {
		return 0, false
	}
	var r rune
	for _, c := range hex {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, false
		}
	}
	if !utf8.ValidRune(r) {
		return 0, false
	}
	return r, true
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
