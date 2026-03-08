package phpcontent

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for PHP content files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new PHP content reader.
func NewReader() *Reader {
	cfg := &Config{UseDirectives: true}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "phpcontent",
			FormatDisplayName: "PHP Content",
			FormatMimeType:    "application/x-php",
			FormatExtensions:  []string{".php", ".phpcnt"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/x-php"},
		Extensions: []string{".php", ".phpcnt"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("phpcontent: nil document or reader")
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

// token types produced by the lexer.
type tokenType int

const (
	tokString      tokenType = iota // a string literal (single-quoted, double-quoted, heredoc, nowdoc)
	tokConcat                       // the . operator
	tokComment                      // a comment (// or /* */)
	tokCode                         // non-string PHP code
	tokArrayIndex                   // array index key value
	tokSkipComment                  // a comment containing a skip directive
)

type token struct {
	typ       tokenType
	value     string // raw string value (with quotes stripped for strings)
	quoteType byte   // '"' or '\'' or 'h' (heredoc) or 'n' (nowdoc)
	arrayKey  string // for tokArrayIndex: the key
	directive string // for skip/text directives
}

// inlineCode represents a matched inline code within a string.
type inlineCode struct {
	start int
	end   int
	data  string
	typ   string
}

// htmlTagPattern matches HTML tags inside strings.
var htmlTagPattern = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*(?:\s+[^>]*)*/?>`)

// phpVarPattern matches PHP variables like $var, $var_name, $obj->prop.
var phpVarPattern = regexp.MustCompile(`\$[a-zA-Z_]\w*(?:->[\w]+)*`)

// escapePattern matches PHP escape sequences in double-quoted strings.
var escapePattern = regexp.MustCompile(`\\[nrtv\\$"efx0-7]`)

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "phpcontent",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-php",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content := r.readAll()
	tokens := r.tokenize(content)
	r.emitParts(ctx, ch, tokens)

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readAll() string {
	scanner := bufio.NewScanner(r.Doc.Reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var buf strings.Builder
	first := true
	for scanner.Scan() {
		if !first {
			buf.WriteByte('\n')
		}
		buf.WriteString(scanner.Text())
		first = false
	}
	return buf.String()
}

// tokenize performs lexical analysis on the PHP content and returns a sequence of tokens.
func (r *Reader) tokenize(content string) []token {
	var tokens []token
	i := 0
	n := len(content)
	var codeBuf strings.Builder

	flushCode := func() {
		if codeBuf.Len() > 0 {
			tokens = append(tokens, token{typ: tokCode, value: codeBuf.String()})
			codeBuf.Reset()
		}
	}

	for i < n {
		// Check for comments
		if i+1 < n && content[i] == '/' && content[i+1] == '/' {
			flushCode()
			end := strings.Index(content[i:], "\n")
			var comment string
			if end < 0 {
				comment = content[i:]
				i = n
			} else {
				comment = content[i : i+end]
				i = i + end
			}
			tok := token{typ: tokComment, value: comment}
			// Check for directives
			tok.directive = r.extractDirective(comment)
			tokens = append(tokens, tok)
			continue
		}
		if i+1 < n && content[i] == '/' && content[i+1] == '*' {
			flushCode()
			end := strings.Index(content[i+2:], "*/")
			var comment string
			if end < 0 {
				comment = content[i:]
				i = n
			} else {
				comment = content[i : i+2+end+2]
				i = i + 2 + end + 2
			}
			tok := token{typ: tokComment, value: comment}
			tok.directive = r.extractDirective(comment)
			tokens = append(tokens, tok)
			continue
		}
		if i+1 < n && content[i] == '#' && content[i+1] != '[' {
			flushCode()
			end := strings.Index(content[i:], "\n")
			var comment string
			if end < 0 {
				comment = content[i:]
				i = n
			} else {
				comment = content[i : i+end]
				i = i + end
			}
			tok := token{typ: tokComment, value: comment}
			tok.directive = r.extractDirective(comment)
			tokens = append(tokens, tok)
			continue
		}

		// Check for heredoc/nowdoc
		if i+2 < n && content[i] == '<' && content[i+1] == '<' && content[i+2] == '<' {
			flushCode()
			tok, adv := r.parseHeredoc(content[i:])
			tokens = append(tokens, tok)
			i += adv
			continue
		}

		// Check for single-quoted strings
		if content[i] == '\'' {
			flushCode()
			tok, adv := r.parseSingleQuoted(content[i:])
			tokens = append(tokens, tok)
			i += adv
			continue
		}

		// Check for double-quoted strings
		if content[i] == '"' {
			flushCode()
			tok, adv := r.parseDoubleQuoted(content[i:])
			tokens = append(tokens, tok)
			i += adv
			continue
		}

		// Check for concatenation operator
		if content[i] == '.' && (i+1 >= n || content[i+1] != '.' && content[i+1] != '=') {
			flushCode()
			tokens = append(tokens, token{typ: tokConcat, value: "."})
			i++
			continue
		}

		// Check for array index: ['key'] or ["key"]
		if content[i] == '[' {
			if key, adv, ok := r.parseArrayIndex(content[i:]); ok {
				flushCode()
				tokens = append(tokens, token{typ: tokArrayIndex, arrayKey: key, value: content[i : i+adv]})
				i += adv
				continue
			}
		}

		codeBuf.WriteByte(content[i])
		i++
	}

	flushCode()
	return tokens
}

// parseSingleQuoted parses a single-quoted string starting at content[0]=='\''.
func (r *Reader) parseSingleQuoted(content string) (token, int) {
	var buf strings.Builder
	i := 1 // skip opening quote
	for i < len(content) {
		if content[i] == '\\' && i+1 < len(content) {
			if content[i+1] == '\'' || content[i+1] == '\\' {
				buf.WriteByte(content[i+1])
				i += 2
				continue
			}
			buf.WriteByte(content[i])
			i++
			continue
		}
		if content[i] == '\'' {
			i++ // skip closing quote
			return token{typ: tokString, value: buf.String(), quoteType: '\''}, i
		}
		buf.WriteByte(content[i])
		i++
	}
	return token{typ: tokString, value: buf.String(), quoteType: '\''}, i
}

// parseDoubleQuoted parses a double-quoted string starting at content[0]=='"'.
func (r *Reader) parseDoubleQuoted(content string) (token, int) {
	var buf strings.Builder
	i := 1 // skip opening quote
	for i < len(content) {
		if content[i] == '\\' && i+1 < len(content) {
			// Keep escape sequences as-is for now; they'll become inline codes
			buf.WriteByte(content[i])
			buf.WriteByte(content[i+1])
			i += 2
			continue
		}
		if content[i] == '"' {
			i++ // skip closing quote
			return token{typ: tokString, value: buf.String(), quoteType: '"'}, i
		}
		buf.WriteByte(content[i])
		i++
	}
	return token{typ: tokString, value: buf.String(), quoteType: '"'}, i
}

// parseHeredoc parses a heredoc or nowdoc starting at <<<.
func (r *Reader) parseHeredoc(content string) (token, int) {
	// Find the label after <<<
	i := 3 // skip <<<
	// skip optional whitespace
	for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
		i++
	}

	isNowdoc := false
	isQuotedHeredoc := false
	var label string

	if i < len(content) && content[i] == '\'' {
		// Nowdoc: <<<'LABEL'
		isNowdoc = true
		i++ // skip opening quote
		end := strings.IndexByte(content[i:], '\'')
		if end < 0 {
			return token{typ: tokCode, value: content}, len(content)
		}
		label = content[i : i+end]
		i += end + 1 // skip closing quote
	} else if i < len(content) && content[i] == '"' {
		// Quoted heredoc: <<<"LABEL"
		isQuotedHeredoc = true
		i++ // skip opening quote
		end := strings.IndexByte(content[i:], '"')
		if end < 0 {
			return token{typ: tokCode, value: content}, len(content)
		}
		label = content[i : i+end]
		i += end + 1 // skip closing quote
	} else {
		// Regular heredoc: <<<LABEL
		start := i
		for i < len(content) && content[i] != '\n' && content[i] != '\r' && content[i] != ';' {
			i++
		}
		label = strings.TrimSpace(content[start:i])
	}

	// Skip to next line
	if i < len(content) && content[i] == '\r' {
		i++
	}
	if i < len(content) && content[i] == '\n' {
		i++
	}

	// Find the closing label
	var bodyBuf strings.Builder
	for i < len(content) {
		lineStart := i
		// Read to end of line
		lineEnd := strings.IndexByte(content[i:], '\n')
		var line string
		if lineEnd < 0 {
			line = content[i:]
			i = len(content)
		} else {
			line = content[i : i+lineEnd]
			i = i + lineEnd + 1
		}

		// Check if this line is the closing label
		trimmedLine := strings.TrimRight(line, "\r")
		closingLabel := strings.TrimRight(trimmedLine, ";")
		closingLabel = strings.TrimSpace(closingLabel)
		if closingLabel == label {
			// This is the closing label
			_ = lineStart
			qtype := byte('h')
			if isNowdoc {
				qtype = 'n'
			} else if isQuotedHeredoc {
				qtype = 'h'
			}
			val := bodyBuf.String()
			// Remove trailing newline if present
			if strings.HasSuffix(val, "\r\n") {
				val = val[:len(val)-2]
			} else if strings.HasSuffix(val, "\n") {
				val = val[:len(val)-1]
			}
			return token{typ: tokString, value: val, quoteType: qtype}, i
		}

		bodyBuf.WriteString(line)
		if lineEnd >= 0 {
			bodyBuf.WriteByte('\n')
		}
	}

	// Unterminated heredoc - return what we have
	qtype := byte('h')
	if isNowdoc {
		qtype = 'n'
	}
	return token{typ: tokString, value: bodyBuf.String(), quoteType: qtype}, i
}

// parseArrayIndex tries to parse ['key'] or ["key"] at content[0]=='['.
func (r *Reader) parseArrayIndex(content string) (string, int, bool) {
	if len(content) < 4 { // minimum: ['k']
		return "", 0, false
	}

	i := 1 // skip [
	// skip whitespace
	for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
		i++
	}

	if i >= len(content) {
		return "", 0, false
	}

	quote := content[i]
	if quote != '\'' && quote != '"' {
		return "", 0, false
	}

	i++ // skip opening quote
	start := i
	for i < len(content) && content[i] != quote {
		if content[i] == '\\' {
			i++ // skip escaped char
		}
		i++
	}
	if i >= len(content) {
		return "", 0, false
	}

	key := content[start:i]
	i++ // skip closing quote

	// skip whitespace
	for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
		i++
	}

	if i >= len(content) || content[i] != ']' {
		return "", 0, false
	}
	i++ // skip ]
	return key, i, true
}

// extractDirective extracts a skip/text directive from a comment, if any.
func (r *Reader) extractDirective(comment string) string {
	lower := strings.ToLower(comment)
	if strings.Contains(lower, "_skip_") || strings.Contains(lower, "_bskip_") ||
		strings.Contains(lower, "_eskip_") || strings.Contains(lower, "_btext_") ||
		strings.Contains(lower, "_etext_") {
		return lower
	}
	return ""
}

// emitParts processes the token stream and emits Parts.
func (r *Reader) emitParts(ctx context.Context, ch chan<- model.PartResult, tokens []token) {
	blockCounter := 0
	dataCounter := 0
	skipMode := false
	lastArrayKey := ""

	i := 0
	for i < len(tokens) {
		tok := tokens[i]

		// Handle directives
		if tok.typ == tokComment && tok.directive != "" && r.cfg.UseDirectives {
			lower := tok.directive
			if strings.Contains(lower, "_bskip_") {
				skipMode = true
			} else if strings.Contains(lower, "_eskip_") || strings.Contains(lower, "_etext_") {
				skipMode = false
			} else if strings.Contains(lower, "_skip_") {
				// Skip next string
				dataCounter++
				if !r.emit(ctx, ch, &model.Part{
					Type:     model.PartData,
					Resource: &model.Data{ID: fmt.Sprintf("d%d", dataCounter), Properties: map[string]string{"comment": tok.value}},
				}) {
					return
				}
				i++
				// Skip the next string token(s)
				i = r.skipNextString(tokens, i)
				continue
			}
			// _btext_ just continues normal extraction mode (no special handling needed)
		}

		// Handle comments as Data
		if tok.typ == tokComment {
			dataCounter++
			if !r.emit(ctx, ch, &model.Part{
				Type:     model.PartData,
				Resource: &model.Data{ID: fmt.Sprintf("d%d", dataCounter), Properties: map[string]string{"comment": tok.value}},
			}) {
				return
			}
			i++
			continue
		}

		// Handle code as Data
		if tok.typ == tokCode {
			dataCounter++
			if !r.emit(ctx, ch, &model.Part{
				Type:     model.PartData,
				Resource: &model.Data{ID: fmt.Sprintf("d%d", dataCounter), Properties: map[string]string{"code": tok.value}},
			}) {
				return
			}
			i++
			continue
		}

		// Handle array index
		if tok.typ == tokArrayIndex {
			lastArrayKey = tok.arrayKey
			dataCounter++
			if !r.emit(ctx, ch, &model.Part{
				Type:     model.PartData,
				Resource: &model.Data{ID: fmt.Sprintf("d%d", dataCounter), Properties: map[string]string{"arrayIndex": tok.value}},
			}) {
				return
			}
			i++
			continue
		}

		// Handle concatenation operator
		if tok.typ == tokConcat {
			i++
			continue
		}

		// Handle string (possibly concatenated)
		if tok.typ == tokString {
			if skipMode {
				dataCounter++
				if !r.emit(ctx, ch, &model.Part{
					Type:     model.PartData,
					Resource: &model.Data{ID: fmt.Sprintf("d%d", dataCounter), Properties: map[string]string{"skipped": tok.value}},
				}) {
					return
				}
				i++
				continue
			}

			// Collect concatenated strings (skip whitespace-only code tokens around concat)
			var parts []token
			parts = append(parts, tok)
			j := i + 1
			for j < len(tokens) {
				// Skip whitespace-only code tokens
				k := j
				for k < len(tokens) && tokens[k].typ == tokCode && isWhitespaceOnly(tokens[k].value) {
					k++
				}
				if k < len(tokens) && tokens[k].typ == tokConcat {
					// Found concat, now look for next string (skipping whitespace)
					m := k + 1
					for m < len(tokens) && tokens[m].typ == tokCode && isWhitespaceOnly(tokens[m].value) {
						m++
					}
					if m < len(tokens) && tokens[m].typ == tokString {
						parts = append(parts, tokens[m])
						j = m + 1
						continue
					}
				}
				break
			}

			// Build the fragment with inline codes
			frag := r.buildFragment(parts)
			text := frag.Text()

			// Skip empty or whitespace-only strings
			if strings.TrimSpace(text) == "" {
				i = j
				lastArrayKey = ""
				continue
			}

			blockCounter++
			blockID := fmt.Sprintf("tu%d", blockCounter)
			if lastArrayKey != "" {
				blockID = lastArrayKey
			}

			block := &model.Block{
				ID:           blockID,
				Translatable: true,
				Source:       []*model.Segment{{ID: "s1", Content: frag}},
				Targets:      make(map[model.LocaleID][]*model.Segment),
				Properties:   make(map[string]string),
				Annotations:  make(map[string]model.Annotation),
			}
			if lastArrayKey != "" {
				block.Properties["arrayKey"] = lastArrayKey
			}

			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}

			i = j
			lastArrayKey = ""
			continue
		}

		i++
	}
}

// skipNextString advances past the next string token (including concatenations).
func (r *Reader) skipNextString(tokens []token, i int) int {
	// Skip whitespace/code tokens to find next string
	for i < len(tokens) && tokens[i].typ != tokString {
		i++
	}
	if i >= len(tokens) {
		return i
	}
	i++ // skip the string

	// Skip any concatenated strings (with whitespace tolerance)
	for i < len(tokens) {
		k := i
		for k < len(tokens) && tokens[k].typ == tokCode && isWhitespaceOnly(tokens[k].value) {
			k++
		}
		if k < len(tokens) && tokens[k].typ == tokConcat {
			m := k + 1
			for m < len(tokens) && tokens[m].typ == tokCode && isWhitespaceOnly(tokens[m].value) {
				m++
			}
			if m < len(tokens) && tokens[m].typ == tokString {
				i = m + 1
				continue
			}
		}
		break
	}
	return i
}

// buildFragment builds a Fragment from one or more string tokens, adding inline codes
// for HTML tags, PHP variables, and escape sequences.
func (r *Reader) buildFragment(tokens []token) *model.Fragment {
	// First, join the string values
	var combined strings.Builder
	for _, tok := range tokens {
		combined.WriteString(tok.value)
	}
	text := combined.String()

	frag := &model.Fragment{}
	spanID := 1

	// Determine if we need to process inline codes
	// (only double-quoted strings and heredocs can have variables and escape sequences)
	hasDoubleQuoted := false
	for _, tok := range tokens {
		if tok.quoteType == '"' || tok.quoteType == 'h' {
			hasDoubleQuoted = true
			break
		}
	}

	// Find all inline code positions
	var matches []inlineCode

	// HTML tags
	for _, loc := range htmlTagPattern.FindAllStringIndex(text, -1) {
		matches = append(matches, inlineCode{start: loc[0], end: loc[1], data: text[loc[0]:loc[1]], typ: "html"})
	}

	// PHP variables (only in double-quoted or heredoc context)
	if hasDoubleQuoted {
		for _, loc := range phpVarPattern.FindAllStringIndex(text, -1) {
			matches = append(matches, inlineCode{start: loc[0], end: loc[1], data: text[loc[0]:loc[1]], typ: "php:variable"})
		}

		// Escape sequences
		for _, loc := range escapePattern.FindAllStringIndex(text, -1) {
			matches = append(matches, inlineCode{start: loc[0], end: loc[1], data: text[loc[0]:loc[1]], typ: "php:escape"})
		}
	}

	if len(matches) == 0 {
		frag.CodedText = text
		return frag
	}

	// Sort matches by position, remove overlaps
	matches = r.sortAndDedup(matches)

	lastEnd := 0
	for _, m := range matches {
		if m.start > lastEnd {
			frag.AppendText(text[lastEnd:m.start])
		}

		spanType := model.SpanPlaceholder
		if m.typ == "html" {
			data := m.data
			if strings.HasPrefix(data, "</") {
				spanType = model.SpanClosing
			} else if strings.HasSuffix(data, "/>") {
				spanType = model.SpanPlaceholder
			} else {
				spanType = model.SpanOpening
			}
		}

		frag.AppendSpan(&model.Span{
			ID:       fmt.Sprintf("c%d", spanID),
			SpanType: spanType,
			Type:     m.typ,
			Data:     m.data,
		})
		spanID++
		lastEnd = m.end
	}
	if lastEnd < len(text) {
		frag.AppendText(text[lastEnd:])
	}

	return frag
}

// sortAndDedup sorts code matches by position and removes overlapping matches.
func (r *Reader) sortAndDedup(matches []inlineCode) []inlineCode {
	if len(matches) <= 1 {
		return matches
	}

	// Simple insertion sort (small slices) by start position
	for i := 1; i < len(matches); i++ {
		key := matches[i]
		j := i - 1
		for j >= 0 && matches[j].start > key.start {
			matches[j+1] = matches[j]
			j--
		}
		matches[j+1] = key
	}

	// Remove overlaps: keep earlier match
	var result []inlineCode
	lastEnd := 0
	for _, m := range matches {
		if m.start >= lastEnd {
			result = append(result, m)
			lastEnd = m.end
		}
	}
	return result
}

// isWhitespaceOnly returns true if the string contains only whitespace characters.
func isWhitespaceOnly(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}
