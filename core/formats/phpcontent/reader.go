package phpcontent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for PHP content files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new PHP content reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
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

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
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
		return errors.New("phpcontent: nil document or reader")
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
	label     string // for heredoc/nowdoc: the delimiter label (e.g. "EOT")
	arrayKey  string // for tokArrayIndex: the key
	directive string // for skip/text directives
	startPos  int    // start position in raw content
	endPos    int    // end position in raw content
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
	r.emitParts(ctx, ch, tokens, content)

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readAll() string {
	if r.skeletonStore != nil {
		// Read raw bytes to preserve exact line endings for skeleton
		data, err := io.ReadAll(r.Doc.Reader)
		if err != nil {
			return ""
		}
		return string(data)
	}
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
	codeStart := 0

	flushCode := func() {
		if codeBuf.Len() > 0 {
			tokens = append(tokens, token{typ: tokCode, value: codeBuf.String(), startPos: codeStart, endPos: i})
			codeBuf.Reset()
		}
		codeStart = i
	}

	for i < n {
		if codeBuf.Len() == 0 {
			codeStart = i
		}
		// Check for comments
		if i+1 < n && content[i] == '/' && content[i+1] == '/' {
			flushCode()
			start := i
			end := strings.Index(content[i:], "\n")
			var comment string
			if end < 0 {
				comment = content[i:]
				i = n
			} else {
				comment = content[i : i+end]
				i += end
			}
			tok := token{typ: tokComment, value: comment, startPos: start, endPos: i}
			// Check for directives
			tok.directive = r.extractDirective(comment)
			tokens = append(tokens, tok)
			continue
		}
		if i+1 < n && content[i] == '/' && content[i+1] == '*' {
			flushCode()
			start := i
			end := strings.Index(content[i+2:], "*/")
			var comment string
			if end < 0 {
				comment = content[i:]
				i = n
			} else {
				comment = content[i : i+2+end+2]
				i = i + 2 + end + 2
			}
			tok := token{typ: tokComment, value: comment, startPos: start, endPos: i}
			tok.directive = r.extractDirective(comment)
			tokens = append(tokens, tok)
			continue
		}
		if i+1 < n && content[i] == '#' && content[i+1] != '[' {
			flushCode()
			start := i
			end := strings.Index(content[i:], "\n")
			var comment string
			if end < 0 {
				comment = content[i:]
				i = n
			} else {
				comment = content[i : i+end]
				i += end
			}
			tok := token{typ: tokComment, value: comment, startPos: start, endPos: i}
			tok.directive = r.extractDirective(comment)
			tokens = append(tokens, tok)
			continue
		}

		// Check for heredoc/nowdoc
		if i+2 < n && content[i] == '<' && content[i+1] == '<' && content[i+2] == '<' {
			flushCode()
			start := i
			tok, adv := r.parseHeredoc(content[i:])
			tok.startPos = start
			tok.endPos = start + adv
			tokens = append(tokens, tok)
			i += adv
			continue
		}

		// Check for single-quoted strings
		if content[i] == '\'' {
			flushCode()
			start := i
			tok, adv := r.parseSingleQuoted(content[i:])
			tok.startPos = start
			tok.endPos = start + adv
			tokens = append(tokens, tok)
			i += adv
			continue
		}

		// Check for double-quoted strings
		if content[i] == '"' {
			flushCode()
			start := i
			tok, adv := r.parseDoubleQuoted(content[i:])
			tok.startPos = start
			tok.endPos = start + adv
			tokens = append(tokens, tok)
			i += adv
			continue
		}

		// Check for concatenation operator
		if content[i] == '.' && (i+1 >= n || content[i+1] != '.' && content[i+1] != '=') {
			flushCode()
			tokens = append(tokens, token{typ: tokConcat, value: ".", startPos: i, endPos: i + 1})
			i++
			continue
		}

		// Check for array index: ['key'] or ["key"]
		if content[i] == '[' {
			if key, adv, ok := r.parseArrayIndex(content[i:]); ok {
				flushCode()
				tokens = append(tokens, token{typ: tokArrayIndex, arrayKey: key, value: content[i : i+adv], startPos: i, endPos: i + adv})
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

// parseSingleQuoted parses a single-quoted string starting at content[0]=='\”.
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
		var nextI int
		if lineEnd < 0 {
			line = content[i:]
			nextI = len(content)
		} else {
			line = content[i : i+lineEnd]
			nextI = i + lineEnd + 1
		}

		// Check if this line is the closing label
		trimmedLine := strings.TrimRight(line, "\r")
		closingLabel := strings.TrimRight(trimmedLine, ";")
		closingLabel = strings.TrimSpace(closingLabel)
		if closingLabel == label {
			// This is the closing label. Consume only up to and
			// including the label characters; leave any trailing
			// `;`, `\r`, `\n`, etc. for the next tokens to pick up
			// as plain code. Doing so lets the non-skeleton writer
			// reconstruct a parseable PHP file by emitting the label
			// itself (which must sit on its own line) and letting
			// the surrounding code tokens supply the trailing
			// statement terminator.
			labelIdx := strings.Index(line, label)
			if labelIdx < 0 {
				// Fallback (should not happen given the equality check above).
				i = nextI
			} else {
				i = lineStart + labelIdx + len(label)
			}
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
			return token{typ: tokString, value: val, quoteType: qtype, label: label}, i
		}

		i = nextI
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
	return token{typ: tokString, value: bodyBuf.String(), quoteType: qtype, label: label}, i
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
func (r *Reader) emitParts(ctx context.Context, ch chan<- model.PartResult, tokens []token, content string) {
	blockCounter := 0
	dataCounter := 0
	skipMode := false
	// When useDirectives is on and extractOutsideDirectives is false,
	// start in skip mode — only _btext_ regions are extracted.
	textMode := true
	if r.cfg.UseDirectives && !r.cfg.ExtractOutsideDirectives {
		textMode = false
	}
	lastArrayKey := ""

	i := 0
	for i < len(tokens) {
		tok := tokens[i]

		// Handle directives
		if tok.typ == tokComment && tok.directive != "" && r.cfg.UseDirectives {
			lower := tok.directive
			if strings.Contains(lower, "_bskip_") {
				skipMode = true
			} else if strings.Contains(lower, "_eskip_") {
				skipMode = false
			} else if strings.Contains(lower, "_btext_") {
				textMode = true
			} else if strings.Contains(lower, "_etext_") {
				textMode = false
			} else if strings.Contains(lower, "_skip_") {
				// Skip next string
				r.skelText(content[tok.startPos:tok.endPos])
				dataCounter++
				if !r.emit(ctx, ch, &model.Part{
					Type:     model.PartData,
					Resource: &model.Data{ID: fmt.Sprintf("d%d", dataCounter), Properties: map[string]string{"comment": tok.value}},
				}) {
					return
				}
				i++
				// Skip the next string token(s) — write their raw content as skeleton text
				nextI := r.skipNextString(tokens, i)
				for k := i; k < nextI; k++ {
					r.skelText(content[tokens[k].startPos:tokens[k].endPos])
				}
				i = nextI
				continue
			}
		}

		// Handle comments as Data
		if tok.typ == tokComment {
			r.skelText(content[tok.startPos:tok.endPos])
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
			r.skelText(content[tok.startPos:tok.endPos])
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
			r.skelText(content[tok.startPos:tok.endPos])
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
			r.skelText(content[tok.startPos:tok.endPos])
			i++
			continue
		}

		// Handle string (possibly concatenated)
		if tok.typ == tokString {
			if skipMode || !textMode {
				r.skelText(content[tok.startPos:tok.endPos])
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

			// Build the segment content as Runs with inline codes.
			runs := r.buildRuns(parts)
			text := model.FlattenRuns(runs)

			// Skip empty or whitespace-only strings
			if strings.TrimSpace(text) == "" {
				// Write the raw content of all skipped tokens as skeleton text
				for k := i; k < j; k++ {
					r.skelText(content[tokens[k].startPos:tokens[k].endPos])
				}
				i = j
				lastArrayKey = ""
				continue
			}

			blockCounter++
			blockID := fmt.Sprintf("tu%d", blockCounter)
			if lastArrayKey != "" {
				blockID = lastArrayKey
			}

			// For skeleton: write the entire raw expression (with quotes, concat)
			// as: prefix text, ref, suffix text.
			// For single string: quote + ref + quote
			// For concatenated strings: first-quote + ref + last-quote (intermediate structure is lost in translation)
			if r.skeletonStore != nil {
				firstTok := parts[0]
				lastTok := parts[len(parts)-1]
				// Write any tokens between i and first string that are not the string itself
				// (whitespace-only code tokens and concat operators before the first string part)
				for k := i; k < j; k++ {
					tkn := tokens[k]
					if tkn.startPos == firstTok.startPos {
						break
					}
					r.skelText(content[tkn.startPos:tkn.endPos])
				}
				// Write the opening quote/delimiter as skeleton text
				r.skelTextStringPrefix(content, firstTok)
				// Write the block reference
				r.skelRef(blockID)
				// Write the closing quote/delimiter as skeleton text
				r.skelTextStringSuffix(content, lastTok)
				// Write any tokens after the last string part
				for k := i; k < j; k++ {
					tkn := tokens[k]
					if tkn.startPos > lastTok.startPos {
						r.skelText(content[tkn.startPos:tkn.endPos])
					}
				}
			}

			block := &model.Block{
				ID:           blockID,
				Translatable: true,
				Source:       runs,
				Targets:      make(map[model.VariantKey]*model.Target),
				Properties:   make(map[string]string),
			}
			if lastArrayKey != "" {
				block.Properties["arrayKey"] = lastArrayKey
			}
			// Record the source string's quote style so the writer can
			// re-emit a parseable PHP string literal when there is no
			// skeleton. Concatenated parts collapse to a single output
			// string in the merged form, so the first part's style
			// drives the output.
			firstStr := parts[0]
			switch firstStr.quoteType {
			case '\'':
				block.Properties["phpQuoteType"] = "single"
			case '"':
				block.Properties["phpQuoteType"] = "double"
			case 'h':
				block.Properties["phpQuoteType"] = "heredoc"
				if firstStr.label != "" {
					block.Properties["phpHeredocLabel"] = firstStr.label
				}
			case 'n':
				block.Properties["phpQuoteType"] = "nowdoc"
				if firstStr.label != "" {
					block.Properties["phpHeredocLabel"] = firstStr.label
				}
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

// buildRuns builds a Run sequence from one or more string tokens, adding inline codes
// for HTML tags, PHP variables, and escape sequences.
func (r *Reader) buildRuns(tokens []token) []model.Run {
	// First, join the string values.
	var combined strings.Builder
	for _, tok := range tokens {
		combined.WriteString(tok.value)
	}
	text := combined.String()

	spanID := 1

	// Only double-quoted strings and heredocs can have PHP
	// variables and escape sequences.
	hasDoubleQuoted := false
	for _, tok := range tokens {
		if tok.quoteType == '"' || tok.quoteType == 'h' {
			hasDoubleQuoted = true
			break
		}
	}

	// Find all inline code positions.
	var matches []inlineCode
	for _, loc := range htmlTagPattern.FindAllStringIndex(text, -1) {
		matches = append(matches, inlineCode{start: loc[0], end: loc[1], data: text[loc[0]:loc[1]], typ: "html"})
	}
	if hasDoubleQuoted {
		for _, loc := range phpVarPattern.FindAllStringIndex(text, -1) {
			matches = append(matches, inlineCode{start: loc[0], end: loc[1], data: text[loc[0]:loc[1]], typ: "php:variable"})
		}
		for _, loc := range escapePattern.FindAllStringIndex(text, -1) {
			matches = append(matches, inlineCode{start: loc[0], end: loc[1], data: text[loc[0]:loc[1]], typ: "php:escape"})
		}
	}

	if len(matches) == 0 {
		if text == "" {
			return nil
		}
		return []model.Run{{Text: &model.TextRun{Text: text}}}
	}

	matches = r.sortAndDedup(matches)

	var runs []model.Run
	lastEnd := 0
	for _, m := range matches {
		if m.start > lastEnd {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:m.start]}})
		}
		// HTML tags map to PcOpen / PcClose / Ph depending on
		// whether the match is an opening, closing, or
		// self-closing element; every other code kind is a
		// self-closing placeholder.
		id := fmt.Sprintf("c%d", spanID)
		spanID++
		lastEnd = m.end
		if m.typ == "html" {
			switch {
			case strings.HasPrefix(m.data, "</"):
				runs = append(runs, model.Run{PcClose: &model.PcCloseRun{ID: id, Type: "html", Data: m.data}})
			case strings.HasSuffix(m.data, "/>"):
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{ID: id, Type: "html", Data: m.data}})
			default:
				runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: "html", Data: m.data}})
			}
		} else {
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{ID: id, Type: m.typ, Data: m.data}})
		}
	}
	if lastEnd < len(text) {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
	}
	return runs
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

// skelTextStringPrefix writes the opening quote/delimiter of a string token as skeleton text.
func (r *Reader) skelTextStringPrefix(content string, tok token) {
	switch tok.quoteType {
	case '\'':
		r.skelText("'")
	case '"':
		r.skelText("\"")
	case 'h', 'n':
		// Heredoc/nowdoc: everything from <<< to the first newline after the label.
		// The value starts after that newline.
		raw := content[tok.startPos:tok.endPos]
		idx := strings.Index(raw, "\n")
		if idx >= 0 {
			r.skelText(raw[:idx+1])
		}
	}
}

// skelTextStringSuffix writes the closing quote/delimiter of a string token as skeleton text.
func (r *Reader) skelTextStringSuffix(content string, tok token) {
	switch tok.quoteType {
	case '\'':
		r.skelText("'")
	case '"':
		r.skelText("\"")
	case 'h', 'n':
		// Heredoc/nowdoc: we need everything after the value content.
		// The raw token is: <<<LABEL\n<value>\n<closing-label-line>
		// The value has trailing newline stripped by the parser.
		// Find where the prefix ends (first newline), then skip past the value.
		raw := content[tok.startPos:tok.endPos]
		prefixEnd := strings.Index(raw, "\n")
		if prefixEnd < 0 {
			return
		}
		// After the prefix, the value content starts.
		valueLen := len(tok.value)
		suffixStart := prefixEnd + 1 + valueLen
		if suffixStart <= len(raw) {
			r.skelText(raw[suffixStart:])
		}
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
