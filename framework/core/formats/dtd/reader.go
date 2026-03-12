package dtd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for DTD files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new DTD reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "dtd",
			FormatDisplayName: "DTD",
			FormatMimeType:    "application/xml-dtd",
			FormatExtensions:  []string{".dtd"},
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
		MIMETypes:  []string{"application/xml-dtd"},
		Extensions: []string{".dtd"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("dtd: nil document or reader")
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

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "dtd",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/xml-dtd",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Read entire content — use io.ReadAll to preserve raw bytes (including line endings)
	rawContent, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("dtd: reading: %w", err)}
		return
	}

	text := string(rawContent)
	blockCounter := 0
	dataCounter := 0
	pos := 0
	skelPos := 0 // tracks how far we've written to skeleton

	// Track pending comment for attachment to next entity
	var pendingComment string

	for pos < len(text) {
		// Skip whitespace
		start := pos
		for pos < len(text) && (text[pos] == ' ' || text[pos] == '\t' || text[pos] == '\n' || text[pos] == '\r') {
			pos++
		}

		if pos >= len(text) {
			// Trailing whitespace as data
			if pos > start {
				r.skelText(text[skelPos:pos])
				skelPos = pos
				dataCounter++
				d := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: "whitespace",
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d})
			}
			break
		}

		// Check for comment
		if strings.HasPrefix(text[pos:], "<!--") {
			endIdx := strings.Index(text[pos:], "-->")
			if endIdx == -1 {
				// Unclosed comment — treat rest as data
				r.skelText(text[skelPos:])
				skelPos = len(text)
				dataCounter++
				d := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: "comment",
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d})
				break
			}
			commentText := text[pos+4 : pos+endIdx]
			commentText = strings.TrimSpace(commentText)
			pendingComment = commentText
			pos += endIdx + 3
			continue
		}

		// Check for <!ENTITY
		if strings.HasPrefix(text[pos:], "<!ENTITY") {
			entityEnd := strings.IndexByte(text[pos:], '>')
			if entityEnd == -1 {
				break
			}
			entityDecl := text[pos : pos+entityEnd+1]
			entityStart := pos
			pos += entityEnd + 1

			name, value, ok := parseEntityDecl(entityDecl)
			if !ok {
				// Non-entity declaration (ELEMENT, ATTLIST, etc.) — emit as data
				dataCounter++
				d := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: "declaration",
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
					return
				}
				pendingComment = ""
				continue
			}

			// Resolve character references and standard XML entities in value
			resolved := resolveReferences(value)

			blockCounter++
			blockID := fmt.Sprintf("tu%d", blockCounter)
			block := model.NewBlock(blockID, resolved)
			block.Name = name

			// For skeleton: find the value position within the entity declaration
			// The value is the quoted string after the entity name
			if r.skeletonStore != nil {
				valueStart, valueEnd := findEntityValuePos(text[entityStart:pos])
				if valueStart >= 0 {
					// Write skeleton text up to the value start
					r.skelText(text[skelPos : entityStart+valueStart])
					// Write skeleton ref for the value
					r.skelRef(blockID)
					// Update skelPos to after the value
					skelPos = entityStart + valueEnd
				}
			}

			// Attach pending comment as note
			if pendingComment != "" {
				block.Annotations["note"] = &model.NoteAnnotation{
					Text: pendingComment,
				}
				pendingComment = ""
			}

			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// Check for other <! declarations (ELEMENT, ATTLIST, etc.)
		if strings.HasPrefix(text[pos:], "<!") {
			endIdx := strings.IndexByte(text[pos:], '>')
			if endIdx == -1 {
				break
			}
			pos += endIdx + 1
			dataCounter++
			d := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "declaration",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
				return
			}
			pendingComment = ""
			continue
		}

		// Check for processing instructions
		if strings.HasPrefix(text[pos:], "<?") {
			endIdx := strings.Index(text[pos:], "?>")
			if endIdx == -1 {
				break
			}
			pos += endIdx + 2
			dataCounter++
			d := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "pi",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
				return
			}
			continue
		}

		// Skip any other character
		pos++
	}

	// Flush any remaining skeleton text
	if r.skeletonStore != nil && skelPos < len(text) {
		r.skelText(text[skelPos:])
	}
	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// findEntityValuePos finds the byte offset of the entity value content
// (without quotes) within an entity declaration string.
// Returns (start, end) where start is after the opening quote and end is before the closing quote.
func findEntityValuePos(decl string) (int, int) {
	// Find the quote character after the entity name
	i := len("<!ENTITY")
	// Skip whitespace
	for i < len(decl) && (decl[i] == ' ' || decl[i] == '\t') {
		i++
	}
	// Skip entity name
	for i < len(decl) && decl[i] != ' ' && decl[i] != '\t' && decl[i] != '"' && decl[i] != '\'' {
		i++
	}
	// Skip whitespace before quote
	for i < len(decl) && (decl[i] == ' ' || decl[i] == '\t') {
		i++
	}
	if i >= len(decl) {
		return -1, -1
	}
	quote := decl[i]
	if quote != '"' && quote != '\'' {
		return -1, -1
	}
	valueStart := i + 1
	valueEnd := strings.IndexByte(decl[valueStart:], quote)
	if valueEnd == -1 {
		return -1, -1
	}
	return valueStart, valueStart + valueEnd
}

// parseEntityDecl parses an ENTITY declaration and returns (name, value, ok).
// Returns ok=false for parameter entities (% prefix) or system/public entities.
func parseEntityDecl(decl string) (string, string, bool) {
	// Remove <!ENTITY and trailing >
	inner := strings.TrimPrefix(decl, "<!ENTITY")
	inner = strings.TrimSuffix(inner, ">")
	inner = strings.TrimSpace(inner)

	if len(inner) == 0 {
		return "", "", false
	}

	// Skip parameter entities (% name)
	if inner[0] == '%' {
		return "", "", false
	}

	// Extract entity name
	nameEnd := strings.IndexAny(inner, " \t\n\r")
	if nameEnd == -1 {
		return "", "", false
	}
	name := inner[:nameEnd]
	rest := strings.TrimSpace(inner[nameEnd:])

	// Check for SYSTEM or PUBLIC (external entities — not translatable)
	upper := strings.ToUpper(rest)
	if strings.HasPrefix(upper, "SYSTEM") || strings.HasPrefix(upper, "PUBLIC") {
		return "", "", false
	}

	// Extract quoted value
	if len(rest) == 0 {
		return "", "", false
	}

	quote := rest[0]
	if quote != '"' && quote != '\'' {
		return "", "", false
	}

	endQuote := strings.IndexByte(rest[1:], quote)
	if endQuote == -1 {
		return "", "", false
	}
	value := rest[1 : 1+endQuote]

	return name, value, true
}

// resolveReferences resolves numeric character references (&#65; &#x41;) and
// standard XML entity references (&amp; &lt; &gt; &quot; &apos;) in the value.
// Unknown entity references like &ent1; are left as-is.
func resolveReferences(s string) string {
	var buf strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '&' {
			end := strings.IndexByte(s[i:], ';')
			if end == -1 {
				buf.WriteByte(s[i])
				i++
				continue
			}
			ref := s[i+1 : i+end]
			if strings.HasPrefix(ref, "#x") || strings.HasPrefix(ref, "#X") {
				// Hex NCR
				hexStr := ref[2:]
				if n, err := strconv.ParseInt(hexStr, 16, 32); err == nil {
					buf.WriteRune(rune(n))
				} else {
					buf.WriteString(s[i : i+end+1])
				}
			} else if strings.HasPrefix(ref, "#") {
				// Decimal NCR
				decStr := ref[1:]
				if n, err := strconv.ParseInt(decStr, 10, 32); err == nil {
					buf.WriteRune(rune(n))
				} else {
					buf.WriteString(s[i : i+end+1])
				}
			} else {
				// Named entity reference
				switch ref {
				case "amp":
					buf.WriteByte('&')
				case "lt":
					buf.WriteByte('<')
				case "gt":
					buf.WriteByte('>')
				case "quot":
					buf.WriteByte('"')
				case "apos":
					buf.WriteByte('\'')
				default:
					// Leave unknown entity references as-is
					buf.WriteString(s[i : i+end+1])
				}
			}
			i += end + 1
		} else if s[i] == '%' {
			// Parameter entity references (%name;)
			end := strings.IndexByte(s[i:], ';')
			if end == -1 {
				buf.WriteByte(s[i])
				i++
				continue
			}
			// Leave parameter entity references as-is
			buf.WriteString(s[i : i+end+1])
			i += end + 1
		} else {
			// Regular character — handle multi-byte UTF-8
			_, size := utf8.DecodeRuneInString(s[i:])
			buf.WriteString(s[i : i+size])
			i += size
		}
	}
	return buf.String()
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
