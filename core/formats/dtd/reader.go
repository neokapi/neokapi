package dtd

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for DTD files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

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

	// Read entire content
	scanner := bufio.NewScanner(r.Doc.Reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var content strings.Builder
	for scanner.Scan() {
		content.WriteString(scanner.Text())
		content.WriteByte('\n')
	}

	text := content.String()
	blockCounter := 0
	dataCounter := 0
	pos := 0

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
			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), resolved)
			block.Name = name

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

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
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

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}
