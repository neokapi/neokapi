package properties

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
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
	cfg := &Config{
		Separator:              "=",
		ExtractOnlyMatchingKey: true,
		KeyCondition:           ".*text.*",
		CommentsAreNotes:       true,
		EscapeExtendedChars:    true,
	}
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
		return errors.New("properties: nil document or reader")
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
	var pendingNote string // accumulated comment text for commentsAreNotes

	// Localization-directive state machine. The stack tracks nested
	// `#_btext` / `#_bskip` blocks (true = extract, false = skip);
	// nextOverride is set by the single-line `#_text` / `#_skip`
	// directives and consumed by the very next key=value entry.
	directiveStack := []bool{}
	var nextOverride *bool
	currentExtract := func() bool {
		if len(directiveStack) == 0 {
			return true
		}
		return directiveStack[len(directiveStack)-1]
	}

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
			// Blank line resets pending notes
			pendingNote = ""
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
			// Localization directives — okapi-compatible. Recognised
			// before the comments-as-notes accumulation so directive
			// markers don't leak into translator notes.
			switch directiveKind(line.content) {
			case dirBText:
				directiveStack = append(directiveStack, true)
				continue
			case dirBSkip:
				directiveStack = append(directiveStack, false)
				continue
			case dirEText, dirESkip:
				if len(directiveStack) > 0 {
					directiveStack = directiveStack[:len(directiveStack)-1]
				}
				continue
			case dirText:
				v := true
				nextOverride = &v
				continue
			case dirSkip:
				v := false
				nextOverride = &v
				continue
			}
			// Accumulate comment text for notes if enabled
			if r.cfg.CommentsAreNotes {
				// Extract comment text (strip leading # or ! and whitespace)
				commentText := extractCommentText(line.content)
				if pendingNote != "" {
					pendingNote += "\n" + commentText
				} else {
					pendingNote = commentText
				}
			}
			continue
		}

		// Parse key=value
		key, value, sep := parseProperty(line.content)
		value = decodeValueEscapes(value)
		if r.cfg.UseJavaEscapes {
			value = decodeJavaEscapes(value)
		}

		// Determine extraction. Single-line `#_text`/`#_skip`
		// directives override the surrounding block state for exactly
		// one entry; otherwise fall back to the current block state and
		// then the legacy KeyCondition pattern.
		extract := currentExtract()
		if nextOverride != nil {
			extract = *nextOverride
			nextOverride = nil
		}
		if extract && !r.cfg.shouldExtractKey(key) {
			extract = false
		}
		if !extract {
			// Not extracted: emit as skeleton
			r.skelText(r.rawLineText(line))
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "skipped-entry",
				Properties: map[string]string{
					"key": key,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			pendingNote = ""
			continue
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

		// Attach pending comment as note
		if pendingNote != "" {
			block.Properties["note"] = pendingNote
			pendingNote = ""
		}

		// Apply inline code finder
		if r.cfg.UseCodeFinder {
			r.applyCodeFinder(block)
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
	// Bound the streamed read with the shared safeio byte budget so an
	// unbounded/oversized stream fails with a typed error (identical limit
	// across CLI/server/WASM — see core/safeio).
	br := bufio.NewReader(safeio.DefaultBudget().Reader(r.Doc.Reader))
	var lines []logicalLine
	var continuation strings.Builder
	var contRawLines []string
	var contLineEndings []string
	inContinuation := false

	for {
		content, lineEnding, eof := readPhysicalLine(br)
		if content == "" && lineEnding == "" && eof {
			break
		}
		// The trailing physical line of the file (no terminator) is
		// signalled by an empty lineEnding; the loop must still process
		// its content before stopping on the next iteration.

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
			if eof {
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
			if eof {
				break
			}
			continue
		}

		// Comment line (# or !) — and optionally ; or // when ExtraComments is enabled
		trimmed := strings.TrimLeft(content, " \t")
		if len(trimmed) > 0 && r.isCommentStart(trimmed) {
			lines = append(lines, logicalLine{
				content:     content,
				isComment:   true,
				rawLines:    []string{content},
				lineEndings: []string{lineEnding},
			})
			if eof {
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

		if eof {
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

// readPhysicalLine reads one physical line from br, recognising LF ("\n"),
// CR ("\r") and CRLF ("\r\n") as line terminators — matching upstream
// Okapi's PropertiesFilter, which reads via BufferedReader.readLine()
// (LF/CR/CRLF per the Java contract). It returns the line content (without
// the terminator), the terminator bytes that followed it (or "" for the
// final unterminated line), and eof=true once no further bytes remain.
//
// Using bufio.ReadString('\n') alone would collapse bare-CR-separated
// lines (classic-Mac / Java Properties \r terminators) into one logical
// line; this byte-level scan splits them correctly while preserving the
// exact terminator for byte-faithful skeleton reconstruction.
func readPhysicalLine(br *bufio.Reader) (content, ending string, eof bool) {
	var buf strings.Builder
	for {
		b, err := br.ReadByte()
		if err != nil {
			// EOF (or read error): whatever we have is the final line.
			return buf.String(), "", true
		}
		switch b {
		case '\n':
			return buf.String(), "\n", false
		case '\r':
			// Peek for a following LF to recognise CRLF; otherwise the
			// CR alone terminates the line.
			next, err := br.ReadByte()
			if err != nil {
				return buf.String(), "\r", true
			}
			if next == '\n' {
				return buf.String(), "\r\n", false
			}
			// Not part of a CRLF — push the byte back for the next read.
			_ = br.UnreadByte()
			return buf.String(), "\r", false
		default:
			buf.WriteByte(b)
		}
	}
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
			r |= c - '0'
		case c >= 'a' && c <= 'f':
			r |= c - 'a' + 10
		case c >= 'A' && c <= 'F':
			r |= c - 'A' + 10
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

// extractCommentText strips the comment marker and leading whitespace from a comment line.
// directiveKind classifies an okapi-compatible localization directive
// embedded in a comment line. Returns dirNone when the comment is not a
// directive. Recognised forms (case-sensitive): `#_text`, `#_skip`,
// `#_btext`, `#_etext`, `#_bskip`, `#_eskip`. Whitespace before/after
// the marker is allowed.
type directiveType int

const (
	dirNone directiveType = iota
	dirText
	dirSkip
	dirBText
	dirEText
	dirBSkip
	dirESkip
)

func directiveKind(content string) directiveType {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 {
		return dirNone
	}
	// Strip the comment marker (# or ! ; not extending to ExtraComments —
	// okapi only recognises directives on `#` lines).
	if trimmed[0] != '#' {
		return dirNone
	}
	body := strings.TrimSpace(trimmed[1:])
	switch body {
	case "_text":
		return dirText
	case "_skip":
		return dirSkip
	case "_btext":
		return dirBText
	case "_etext":
		return dirEText
	case "_bskip":
		return dirBSkip
	case "_eskip":
		return dirESkip
	}
	return dirNone
}

func extractCommentText(content string) string {
	trimmed := strings.TrimLeft(content, " \t")
	if len(trimmed) == 0 {
		return ""
	}
	if trimmed[0] == '#' || trimmed[0] == '!' || trimmed[0] == ';' {
		return strings.TrimLeft(trimmed[1:], " \t")
	}
	if len(trimmed) >= 2 && trimmed[0] == '/' && trimmed[1] == '/' {
		return strings.TrimLeft(trimmed[2:], " \t")
	}
	return trimmed
}

// isCommentStart checks if a trimmed line starts with a comment marker.
// Standard markers are # and !. When ExtraComments is enabled, ; and // are also recognized.
func (r *Reader) isCommentStart(trimmed string) bool {
	if trimmed[0] == '#' || trimmed[0] == '!' {
		return true
	}
	if r.cfg.ExtraComments {
		if trimmed[0] == ';' {
			return true
		}
		if len(trimmed) >= 2 && trimmed[0] == '/' && trimmed[1] == '/' {
			return true
		}
	}
	return false
}

// applyCodeFinder applies code finder patterns to a block's fragments.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.CodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}

	if len(block.Source) == 0 {
		return
	}
	text := model.RunsText(block.Source)

	type matchRange struct {
		start, end int
	}
	var matches []matchRange
	for _, re := range patterns {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			matches = append(matches, matchRange{loc[0], loc[1]})
		}
	}
	if len(matches) == 0 {
		return
	}

	// Sort matches by start position
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	var runs []model.Run
	lastEnd := 0
	spanID := 1
	for _, m := range matches {
		if m.start > lastEnd {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:m.start]}})
		}
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
			ID:   fmt.Sprintf("c%d", spanID),
			Type: "code",
			Data: text[m.start:m.end],
		}})
		lastEnd = m.end
		spanID++
	}
	if lastEnd < len(text) {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
	}
	block.SetSourceRuns(runs)
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
