package properties

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for Java Properties files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

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

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
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
	// rawLines holds the raw text of each physical line (without line endings)
	// for skeleton reconstruction. For simple lines this has one entry;
	// for continuation lines it has multiple entries.
	rawLines []string
	// lineEndings holds the line ending ("\n", "\r\n", or "") for each physical line.
	lineEndings []string
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
			// Skeleton: write the blank line's raw text + line ending
			r.skelText(r.rawLineText(line))
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
			// Skeleton: write the comment line's raw text + line ending
			r.skelText(r.rawLineText(line))
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
		value = decodeValueEscapes(value)
		if r.cfg.UseJavaEscapes {
			value = decodeJavaEscapes(value)
		}

		blockID++
		blockIDStr := fmt.Sprintf("tu%d", blockID)

		// Skeleton: write key+separator prefix as text, value as ref, line ending as text
		if r.skeletonStore != nil {
			r.skelPropertyLine(line, blockIDStr)
		}

		block := model.NewBlock(blockIDStr, value)
		block.Name = key
		block.Properties["separator"] = sep
		// Store raw value for byte-exact skeleton reconstruction
		if r.skeletonStore != nil {
			block.Properties["rawValue"] = r.rawValueText(line)
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	r.skelFlush()

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// readLogicalLines reads all lines, joining continuation lines (backslash at EOL).
// It preserves raw line text and line endings for skeleton reconstruction.
func (r *Reader) readLogicalLines() []logicalLine {
	br := bufio.NewReader(r.Doc.Reader)
	var lines []logicalLine
	var continuation strings.Builder
	var contRawLines []string
	var contLineEndings []string
	inContinuation := false

	for {
		rawLine, err := br.ReadString('\n')
		if rawLine == "" && err != nil {
			break
		}

		// Split content from line ending
		content := rawLine
		lineEnding := ""
		if strings.HasSuffix(content, "\r\n") {
			content = content[:len(content)-2]
			lineEnding = "\r\n"
		} else if strings.HasSuffix(content, "\n") {
			content = content[:len(content)-1]
			lineEnding = "\n"
		}

		if inContinuation {
			contRawLines = append(contRawLines, content)
			contLineEndings = append(contLineEndings, lineEnding)
			// Continuation: trim leading whitespace for logical content
			trimmed := strings.TrimLeft(content, " \t")
			if hasContinuation(trimmed) {
				continuation.WriteString(trimmed[:len(trimmed)-1])
			} else {
				continuation.WriteString(trimmed)
				lines = append(lines, logicalLine{
					content:     continuation.String(),
					rawLines:    contRawLines,
					lineEndings: contLineEndings,
				})
				inContinuation = false
				continuation.Reset()
				contRawLines = nil
				contLineEndings = nil
			}
			if err != nil {
				break
			}
			continue
		}

		// Blank line
		if strings.TrimSpace(content) == "" {
			lines = append(lines, logicalLine{
				isBlank:     true,
				rawLines:    []string{content},
				lineEndings: []string{lineEnding},
			})
			if err != nil {
				break
			}
			continue
		}

		// Comment line (# or !)
		trimmed := strings.TrimLeft(content, " \t")
		if len(trimmed) > 0 && (trimmed[0] == '#' || trimmed[0] == '!') {
			lines = append(lines, logicalLine{
				content:     content,
				isComment:   true,
				rawLines:    []string{content},
				lineEndings: []string{lineEnding},
			})
			if err != nil {
				break
			}
			continue
		}

		// Regular or continuation line
		if hasContinuation(content) {
			continuation.WriteString(content[:len(content)-1])
			contRawLines = []string{content}
			contLineEndings = []string{lineEnding}
			inContinuation = true
		} else {
			lines = append(lines, logicalLine{
				content:     content,
				rawLines:    []string{content},
				lineEndings: []string{lineEnding},
			})
		}

		if err == io.EOF {
			break
		}
	}

	// If the file ends mid-continuation, emit what we have
	if inContinuation {
		lines = append(lines, logicalLine{
			content:     continuation.String(),
			rawLines:    contRawLines,
			lineEndings: contLineEndings,
		})
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

// decodeValueEscapes processes Java properties escape sequences in a value string:
// \uXXXX (unicode), \n (newline), \t (tab), \r (CR), \\ (backslash).
// Unknown escape sequences (e.g. \w) are kept as-is.
func decodeValueEscapes(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}

	var buf strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case 'u':
				if i+5 < len(s) {
					hex := s[i+2 : i+6]
					r, ok := parseHexRune(hex)
					if ok {
						buf.WriteRune(r)
						i += 6
						continue
					}
				}
				// Invalid \u sequence — keep as-is
				buf.WriteByte(s[i])
				i++
			case 'n':
				buf.WriteByte('\n')
				i += 2
			case 't':
				buf.WriteByte('\t')
				i += 2
			case 'r':
				buf.WriteByte('\r')
				i += 2
			case '\\':
				buf.WriteByte('\\')
				i += 2
			default:
				// Unknown escape — keep as-is (e.g. \w -> \w)
				buf.WriteByte(s[i])
				buf.WriteByte(next)
				i += 2
			}
			continue
		}
		buf.WriteByte(s[i])
		i++
	}
	return buf.String()
}

// decodeJavaEscapes additionally decodes \: \= \# \! in property values.
// This is controlled by the useJavaEscapes config option.
func decodeJavaEscapes(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}

	var buf strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case ':', '=', '#', '!':
				buf.WriteByte(next)
				i += 2
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

// rawLineText reconstructs the full raw text (with line endings) for a non-property line.
func (r *Reader) rawLineText(line logicalLine) string {
	var sb strings.Builder
	for i, raw := range line.rawLines {
		sb.WriteString(raw)
		if i < len(line.lineEndings) {
			sb.WriteString(line.lineEndings[i])
		}
	}
	return sb.String()
}

// rawValueText extracts the raw value portion from a property line's raw lines.
// For single lines: returns the text after key+separator.
// For continuation lines: returns the multi-line raw value with continuation markers.
func (r *Reader) rawValueText(line logicalLine) string {
	if len(line.rawLines) == 0 {
		return ""
	}
	// Find the value start position in the first raw line
	first := line.rawLines[0]
	_, _, sep := parseProperty(first)
	_ = sep
	// Find separator position in the raw line
	trimmed := strings.TrimLeft(first, " \t")
	offset := len(first) - len(trimmed)
	i := offset
	for i < len(first) {
		if first[i] == '\\' && i+1 < len(first) {
			i += 2
			continue
		}
		if first[i] == '=' || first[i] == ':' {
			// Value starts after separator and optional whitespace
			valStart := i + 1
			for valStart < len(first) && (first[valStart] == ' ' || first[valStart] == '\t') {
				valStart++
			}
			if len(line.rawLines) == 1 {
				return first[valStart:]
			}
			// Multi-line: reconstruct the raw value across continuation lines
			var sb strings.Builder
			sb.WriteString(first[valStart:])
			for j := 1; j < len(line.rawLines); j++ {
				sb.WriteString(line.lineEndings[j-1])
				sb.WriteString(line.rawLines[j])
			}
			return sb.String()
		}
		if first[i] == ' ' || first[i] == '\t' {
			// Space separator
			j := i
			for j < len(first) && (first[j] == ' ' || first[j] == '\t') {
				j++
			}
			if j < len(first) && (first[j] == '=' || first[j] == ':') {
				valStart := j + 1
				for valStart < len(first) && (first[valStart] == ' ' || first[valStart] == '\t') {
					valStart++
				}
				if len(line.rawLines) == 1 {
					return first[valStart:]
				}
				var sb strings.Builder
				sb.WriteString(first[valStart:])
				for k := 1; k < len(line.rawLines); k++ {
					sb.WriteString(line.lineEndings[k-1])
					sb.WriteString(line.rawLines[k])
				}
				return sb.String()
			}
			// Space is the separator; value starts at j
			if len(line.rawLines) == 1 {
				return first[j:]
			}
			var sb strings.Builder
			sb.WriteString(first[j:])
			for k := 1; k < len(line.rawLines); k++ {
				sb.WriteString(line.lineEndings[k-1])
				sb.WriteString(line.rawLines[k])
			}
			return sb.String()
		}
		i++
	}
	return ""
}

// skelPropertyLine writes skeleton entries for a key=value property line.
// The key+separator prefix goes as skeleton text, the value as a ref,
// and the trailing line ending as skeleton text.
func (r *Reader) skelPropertyLine(line logicalLine, blockID string) {
	if len(line.rawLines) == 0 {
		return
	}
	// Find where the value starts in the first raw line
	first := line.rawLines[0]
	prefix := r.rawKeyPrefix(first)
	r.skelText(prefix)
	r.skelRef(blockID)
	// Write trailing line ending (last physical line's ending)
	if len(line.lineEndings) > 0 {
		lastEnding := line.lineEndings[len(line.lineEndings)-1]
		r.skelText(lastEnding)
	}
}

// rawKeyPrefix returns the key+separator+whitespace prefix of a raw property line,
// i.e., everything before the value starts.
func (r *Reader) rawKeyPrefix(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	offset := len(line) - len(trimmed)
	i := offset
	for i < len(line) {
		if line[i] == '\\' && i+1 < len(line) {
			i += 2
			continue
		}
		if line[i] == '=' || line[i] == ':' {
			valStart := i + 1
			for valStart < len(line) && (line[valStart] == ' ' || line[valStart] == '\t') {
				valStart++
			}
			return line[:valStart]
		}
		if line[i] == ' ' || line[i] == '\t' {
			j := i
			for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
				j++
			}
			if j < len(line) && (line[j] == '=' || line[j] == ':') {
				valStart := j + 1
				for valStart < len(line) && (line[valStart] == ' ' || line[valStart] == '\t') {
					valStart++
				}
				return line[:valStart]
			}
			// Space is the separator
			return line[:j]
		}
		i++
	}
	// Key only, no value
	return line
}

// skelText appends text to the skeleton buffer if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		r.skelBuf.WriteString(s)
	}
}

// skelRef flushes buffered text and writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
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
