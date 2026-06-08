package dtd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// codeFinderTagType marks Ph runs extracted by the configurable
// code-finder (e.g. HTML tags inside an entity value), mirroring
// okapi's InlineCodeFinder.TAGTYPE constant ("regxph"). The writer
// uses it to decide which inline-code runs must be entity-escaped.
const codeFinderTagType = "regxph"

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
		return errors.New("dtd: nil document or reader")
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
			// Find the closing `>` while honouring quoted strings —
			// entity values can legitimately contain `>` characters
			// (e.g. `<!ENTITY x "a>b">`), and the naive byte scan would
			// truncate the declaration there.
			entityEnd := indexCloseAngleQuoted(text[pos:])
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

			// Split the value into TextRuns and Ph runs: known entity
			// references (`&amp;` etc.) and NCRs (`&#65;` / `&#x41;`)
			// resolve to characters; unknown named refs (`&test1;`)
			// and parameter-entity refs (`%name;`) become inline Ph
			// codes so they survive pseudo-translation. okapi's
			// DTDFilter does the same — see its hard-coded
			// `&#…;|&#x…;|(&\w*?;)|(%\w*?;)` regex.
			runs := buildEntityValueRuns(value)

			blockCounter++
			blockID := fmt.Sprintf("tu%d", blockCounter)
			block := model.NewBlock(blockID, "")
			block.Name = name
			if len(runs) > 0 {
				block.Source = runs
			}

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
				block.AddNote(&model.NoteAnnotation{
					Text: pendingComment,
				})
				pendingComment = ""
			}

			r.applyCodeFinder(block)

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

// buildEntityValueRuns turns a raw DTD entity value into a Run sequence.
// Standard XML entities (`&amp; &lt; &gt; &quot; &apos;`) and numeric
// character references (`&#NNN;` / `&#xHH;`) are resolved to literal
// characters in TextRuns. Named entity references with non-standard
// names (`&test1;`) and parameter-entity references (`%name;`) become
// Ph runs whose Data carries the original `&name;` / `%name;` form, so
// the writer can emit them verbatim and pseudo-translation skips over
// them as inline codes — mirroring okapi's DTDFilter behaviour.
func buildEntityValueRuns(s string) []model.Run {
	var runs []model.Run
	var text strings.Builder
	flushText := func() {
		if text.Len() > 0 {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text.String()}})
			text.Reset()
		}
	}
	phID := 1
	addPh := func(data string) {
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
			ID:   fmt.Sprintf("e%d", phID),
			Type: "code",
			Data: data,
		}})
		phID++
	}
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '&':
			end := strings.IndexByte(s[i:], ';')
			if end < 0 {
				text.WriteByte('&')
				i++
				continue
			}
			ref := s[i+1 : i+end]
			full := s[i : i+end+1]
			// Numeric character references → resolve to runes.
			if strings.HasPrefix(ref, "#x") || strings.HasPrefix(ref, "#X") {
				if n, err := strconv.ParseInt(ref[2:], 16, 32); err == nil {
					text.WriteRune(rune(n))
					i += end + 1
					continue
				}
			} else if strings.HasPrefix(ref, "#") {
				if n, err := strconv.ParseInt(ref[1:], 10, 32); err == nil {
					text.WriteRune(rune(n))
					i += end + 1
					continue
				}
			}
			switch ref {
			case "amp":
				text.WriteByte('&')
				i += end + 1
				continue
			case "lt":
				text.WriteByte('<')
				i += end + 1
				continue
			case "gt":
				text.WriteByte('>')
				i += end + 1
				continue
			case "quot":
				text.WriteByte('"')
				i += end + 1
				continue
			case "apos":
				text.WriteByte('\'')
				i += end + 1
				continue
			}
			// Unknown named ref → preserve as Ph code.
			flushText()
			addPh(full)
			i += end + 1
		case '%':
			end := strings.IndexByte(s[i:], ';')
			if end < 0 {
				text.WriteByte('%')
				i++
				continue
			}
			flushText()
			addPh(s[i : i+end+1])
			i += end + 1
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			text.WriteString(s[i : i+size])
			i += size
		}
	}
	flushText()
	return runs
}

// indexCloseAngleQuoted returns the index of the `>` that ends a DTD
// declaration starting at s[0], skipping any `>` characters that sit
// inside a single- or double-quoted attribute / entity value. Returns
// -1 when no closing angle is found in s.
func indexCloseAngleQuoted(s string) int {
	var inQuote byte
	for i := range len(s) {
		c := s[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '>':
			return i
		}
	}
	return -1
}

// applyCodeFinder splits the literal-text portions of each source
// segment on configured code-finder regex matches, replacing matched
// substrings with Ph runs so they survive pseudo-translation as opaque
// inline codes. It operates per-TextRun and leaves any existing inline-
// code runs (structural `&entity;` / `%param;` references already lifted
// by buildEntityValueRuns) untouched — mirroring okapi's DTDFilter, which
// calls codeFinder.process(tf) on a TextFragment whose structural refs are
// already codes, so the finder only converts the remaining plain text.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.CodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}
	if len(block.Source) == 0 {
		return
	}
	spanID := 1
	var runs []model.Run
	for _, run := range block.Source {
		if run.Text == nil {
			// Preserve non-text runs (structural references) verbatim.
			runs = append(runs, run)
			continue
		}
		split, used := splitTextRunOnCodeFinder(run.Text.Text, patterns, spanID)
		runs = append(runs, split...)
		spanID += used
	}
	block.SetSourceRuns(runs)
}

// splitTextRunOnCodeFinder breaks a single literal-text string into a Run
// sequence: spans matched by any code-finder pattern become Ph runs (tagged
// with codeFinderTagType), the rest stays as TextRuns. Returns the runs and
// the number of Ph ids consumed (starting at startID).
func splitTextRunOnCodeFinder(text string, patterns []*regexp.Regexp, startID int) ([]model.Run, int) {
	type matchRange struct{ start, end int }
	var matches []matchRange
	for _, re := range patterns {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			matches = append(matches, matchRange{loc[0], loc[1]})
		}
	}
	if len(matches) == 0 {
		return []model.Run{{Text: &model.TextRun{Text: text}}}, 0
	}
	// Insertion sort by start; tiny lists in practice.
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}
	var runs []model.Run
	lastEnd, used := 0, 0
	for _, m := range matches {
		if m.start < lastEnd {
			continue // overlap with prior match
		}
		if m.start > lastEnd {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:m.start]}})
		}
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
			ID: fmt.Sprintf("c%d", startID+used),
			// SubType mirrors okapi's InlineCodeFinder.TAGTYPE
			// ("regxph"). The writer uses it to tell codeFinder-
			// extracted markup (which must be entity-escaped on
			// write — okapi's DTDFilter re-encodes these via
			// encoder.encode(..., TEXT)) apart from structural
			// entity/parameter references (emitted verbatim).
			Type:    "code",
			SubType: codeFinderTagType,
			Data:    text[m.start:m.end],
		}})
		lastEnd = m.end
		used++
	}
	if lastEnd < len(text) {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
	}
	return runs, used
}
