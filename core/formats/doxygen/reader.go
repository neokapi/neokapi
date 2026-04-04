package doxygen

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Doxygen/Javadoc comments in source code.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new Doxygen reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "doxygen",
			FormatDisplayName: "Doxygen Comments",
			FormatMimeType:    "text/x-doxygen-txt",
			FormatExtensions:  []string{".c", ".cpp", ".h", ".java", ".m", ".py"},
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
		MIMETypes:  []string{"text/x-doxygen-txt"},
		Extensions: []string{".c", ".cpp", ".h", ".java", ".m", ".py"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("doxygen: nil document or reader")
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

// commentBlock represents a parsed Doxygen comment group and its surrounding context.
type commentBlock struct {
	// style is the comment style: "triple", "exclamation", "javadoc", "qt", "trailing", "trailing_qt"
	style string
	// prefix holds any code text before a trailing comment on the same line
	prefix string
	// rawLines holds the raw source lines making up this comment (including delimiters)
	rawLines []string
	// textLines holds the extracted translatable text lines (after stripping comment markers)
	textLines []string
}

// nonTranslatableCommands are Doxygen commands whose arguments are not translatable.
var nonTranslatableCommands = map[string]bool{
	"class": true, "file": true, "author": true, "date": true,
	"version": true, "see": true, "since": true, "deprecated": true,
	"namespace": true, "package": true, "defgroup": true, "ingroup": true,
	"addtogroup": true, "name": true, "typedef": true, "enum": true,
	"struct": true, "union": true, "fn": true, "var": true,
	"def": true, "headerfile": true, "page": true, "mainpage": true,
	"subpage": true, "section": true, "subsection": true, "subsubsection": true,
	"paragraph": true, "anchor": true, "ref": true, "copydoc": true,
	"include": true, "dontinclude": true, "line": true, "skip": true,
	"skipline": true, "until": true, "example": true, "dir": true,
	"relates": true, "relatesalso": true, "memberof": true,
	"property": true, "implements": true, "extends": true,
}

// translatableDescCommands are Doxygen commands whose description text IS translatable.
var translatableDescCommands = map[string]bool{
	"brief": true, "details": true, "short": true,
	"param": true, "return": true, "returns": true, "retval": true,
	"throw": true, "throws": true, "exception": true,
	"note": true, "warning": true, "remark": true, "remarks": true,
	"attention": true, "bug": true, "todo": true, "test": true,
	"pre": true, "post": true, "invariant": true,
	"tparam": true, "sa": true,
}

// inlineCommands are Doxygen commands that produce inline formatting.
var inlineCommands = map[string]bool{
	"e": true, "a": true, "b": true, "c": true, "p": true, "em": true,
}

// rawLine holds a line's content and its original line ending.
type rawLine struct {
	content    string
	lineEnding string
}

// splitRawLines splits raw bytes into lines preserving line endings.
func splitRawLines(data []byte) []rawLine {
	remaining := string(data)
	var lines []rawLine
	for len(remaining) > 0 {
		idx := strings.Index(remaining, "\n")
		if idx < 0 {
			lines = append(lines, rawLine{content: remaining})
			break
		}
		lineContent := remaining[:idx]
		ending := "\n"
		if strings.HasSuffix(lineContent, "\r") {
			lineContent = lineContent[:len(lineContent)-1]
			ending = "\r\n"
		}
		lines = append(lines, rawLine{content: lineContent, lineEnding: ending})
		remaining = remaining[idx+1:]
	}
	return lines
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "doxygen",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-doxygen-txt",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	var lines []string
	var rLines []rawLine

	if r.skeletonStore != nil {
		// Read all bytes to preserve line endings for skeleton
		data, err := io.ReadAll(r.Doc.Reader)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("doxygen: reading: %w", err)}
			return
		}
		rLines = splitRawLines(data)
		for _, rl := range rLines {
			lines = append(lines, rl.content)
		}
	} else {
		scanner := bufio.NewScanner(r.Doc.Reader)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
	}

	blockCounter := 0
	dataCounter := 0
	i := 0

	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Check for block comment start: /** or /*!
		if strings.HasPrefix(trimmed, "/**") || strings.HasPrefix(trimmed, "/*!") {
			cb := r.parseBlockComment(lines, i)
			n := len(cb.rawLines)
			r.skelCommentGroup(cb, rLines, i, n, &blockCounter)
			dataCounter++
			r.emitCommentBlock(ctx, ch, cb, &blockCounter, &dataCounter)
			i += n
			continue
		}

		// Check for line comment: /// or //!
		if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "//!") {
			// Check for trailing comment: code ///< text
			if r.isTrailingComment(line) {
				cb := r.parseTrailingLineComment(line)
				r.skelTrailingCommentGroup(cb, rLines, i, &blockCounter)
				dataCounter++
				r.emitCommentBlock(ctx, ch, cb, &blockCounter, &dataCounter)
				i++
				continue
			}

			// Collect consecutive line comments
			cb := r.parseLineComments(lines, i)
			n := len(cb.rawLines)
			r.skelCommentGroup(cb, rLines, i, n, &blockCounter)
			dataCounter++
			r.emitCommentBlock(ctx, ch, cb, &blockCounter, &dataCounter)
			i += n
			continue
		}

		// Check for trailing line comment: code ///< text
		if strings.Contains(line, "///<") {
			idx := strings.Index(line, "///<")
			before := strings.TrimSpace(line[:idx])
			if before != "" {
				cb := r.parseTrailingLineComment(line)
				r.skelTrailingCommentGroup(cb, rLines, i, &blockCounter)
				dataCounter++
				r.emitCommentBlock(ctx, ch, cb, &blockCounter, &dataCounter)
				i++
				continue
			}
		}

		// Check for trailing block comment: code /*!< text */
		if r.isTrailingBlockComment(trimmed) {
			cb := r.parseTrailingBlockComment(line)
			r.skelTrailingCommentGroup(cb, rLines, i, &blockCounter)
			dataCounter++
			r.emitCommentBlock(ctx, ch, cb, &blockCounter, &dataCounter)
			i++
			continue
		}

		// Non-comment line → emit as Data
		r.skelLinesText(rLines, i, 1)
		dataCounter++
		data := &model.Data{
			ID:         fmt.Sprintf("d%d", dataCounter),
			Name:       fmt.Sprintf("code.%d", i+1),
			Properties: map[string]string{"raw": line},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
			return
		}
		i++
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// skelCommentGroup writes skeleton entries for a comment group.
// Translatable groups get a ref (for the first block); non-translatable get text.
func (r *Reader) skelCommentGroup(cb *commentBlock, rLines []rawLine, lineStart, lineCount int, blockCounter *int) {
	if r.skeletonStore == nil {
		return
	}

	translatableLines := r.extractTranslatable(cb.textLines)
	if len(translatableLines) == 0 {
		// Non-translatable: write all lines as skeleton text
		r.skelLinesText(rLines, lineStart, lineCount)
		return
	}

	// Translatable: write a ref for the first block, plus trailing line ending
	nextBlockID := fmt.Sprintf("tu%d", *blockCounter+1)
	r.skelRef(nextBlockID)
	// Write the line ending of the last raw line
	lastIdx := lineStart + lineCount - 1
	if lastIdx < len(rLines) {
		r.skelText(rLines[lastIdx].lineEnding)
	}
}

// skelTrailingCommentGroup writes skeleton entries for a trailing comment line.
// The prefix goes as skeleton text, the comment part as a ref.
func (r *Reader) skelTrailingCommentGroup(cb *commentBlock, rLines []rawLine, lineStart int, blockCounter *int) {
	if r.skeletonStore == nil {
		return
	}

	translatableLines := r.extractTranslatable(cb.textLines)
	if len(translatableLines) == 0 {
		// Non-translatable: write whole line as skeleton text
		r.skelLinesText(rLines, lineStart, 1)
		return
	}

	// Prefix as skeleton text, ref for block, trailing line ending
	nextBlockID := fmt.Sprintf("tu%d", *blockCounter+1)
	// The prefix is part of the raw line content; write prefix + comment marker
	// For trailing ///<: "code ///< text" — prefix is "code ", marker is "///< "
	// For trailing /*!<: "code /*!< text */" — prefix is "code ", marker includes "/*!< " and " */"
	// The ref will produce just the reconstructed comment part (including prefix/markers)
	// via writeTrailing/writeTrailingQt, so we just need the code prefix
	if cb.prefix != "" {
		r.skelText(cb.prefix)
	}
	r.skelRef(nextBlockID)
	if lineStart < len(rLines) {
		r.skelText(rLines[lineStart].lineEnding)
	}
}

// skelLinesText writes raw lines (content + line ending) to the skeleton buffer.
func (r *Reader) skelLinesText(rLines []rawLine, start, count int) {
	if r.skeletonStore == nil {
		return
	}
	for j := 0; j < count; j++ {
		r.skelText(rLines[start+j].content + rLines[start+j].lineEnding)
	}
}

// isTrailingComment checks if a line has code followed by ///< comment.
func (r *Reader) isTrailingComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	// If the line starts with ///, it's a regular line comment, not trailing
	if strings.HasPrefix(trimmed, "///") {
		// But if it has "///<", check if there's code before the "///<"
		idx := strings.Index(line, "///<")
		if idx < 0 {
			return false
		}
		before := strings.TrimSpace(line[:idx])
		return before != ""
	}
	return false
}

// isTrailingBlockComment checks if a line has code followed by /*!< text */.
func (r *Reader) isTrailingBlockComment(trimmed string) bool {
	idx := strings.Index(trimmed, "/*!<")
	if idx < 0 {
		return false
	}
	before := strings.TrimSpace(trimmed[:idx])
	return before != "" && strings.Contains(trimmed[idx:], "*/")
}

// parseBlockComment parses a /** ... */ or /*! ... */ block comment starting at line index i.
func (r *Reader) parseBlockComment(lines []string, start int) *commentBlock {
	cb := &commentBlock{}
	trimmedFirst := strings.TrimSpace(lines[start])

	if strings.HasPrefix(trimmedFirst, "/*!") {
		cb.style = "qt"
	} else {
		cb.style = "javadoc"
	}

	// Check if it's a single-line block comment like "/** text */"
	if strings.Contains(trimmedFirst, "*/") {
		cb.rawLines = []string{lines[start]}
		text := trimmedFirst
		// Remove opening delimiter
		if cb.style == "qt" {
			text = strings.TrimPrefix(text, "/*!")
		} else {
			text = strings.TrimPrefix(text, "/**")
		}
		// Remove closing delimiter
		text = strings.TrimSuffix(text, "*/")
		text = strings.TrimSpace(text)
		if text != "" {
			cb.textLines = append(cb.textLines, text)
		}
		return cb
	}

	// Multi-line block comment
	for j := start; j < len(lines); j++ {
		cb.rawLines = append(cb.rawLines, lines[j])
		if j > start && strings.Contains(lines[j], "*/") {
			break
		}
	}

	// Extract text from each line
	for idx, raw := range cb.rawLines {
		text := raw
		if idx == 0 {
			// First line: remove opening delimiter
			trimmed := strings.TrimSpace(text)
			if cb.style == "qt" {
				trimmed = strings.TrimPrefix(trimmed, "/*!")
			} else {
				trimmed = strings.TrimPrefix(trimmed, "/**")
			}
			text = strings.TrimSpace(trimmed)
		} else if idx == len(cb.rawLines)-1 {
			// Last line: remove closing delimiter
			text = strings.TrimSpace(text)
			text = strings.TrimSuffix(text, "*/")
			text = strings.TrimSpace(text)
			text = strings.TrimPrefix(text, "*")
			text = strings.TrimSpace(text)
		} else {
			// Middle lines: remove leading " * "
			text = strings.TrimSpace(text)
			text = strings.TrimPrefix(text, "*")
			text = strings.TrimSpace(text)
		}
		if text != "" {
			cb.textLines = append(cb.textLines, text)
		}
	}

	return cb
}

// parseLineComments collects consecutive /// or //! line comments starting at index i.
func (r *Reader) parseLineComments(lines []string, start int) *commentBlock {
	cb := &commentBlock{}

	firstTrimmed := strings.TrimSpace(lines[start])
	if strings.HasPrefix(firstTrimmed, "//!") {
		cb.style = "exclamation"
	} else {
		cb.style = "triple"
	}

	for j := start; j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])
		isTriple := strings.HasPrefix(trimmed, "///")
		isExcl := strings.HasPrefix(trimmed, "//!")
		if !isTriple && !isExcl {
			break
		}
		// Stop if it's a trailing comment (code before ///<)
		if isTriple && r.isTrailingComment(lines[j]) {
			break
		}
		cb.rawLines = append(cb.rawLines, lines[j])

		// Extract text
		var text string
		if isTriple {
			text = strings.TrimPrefix(trimmed, "///")
		} else {
			text = strings.TrimPrefix(trimmed, "//!")
		}
		text = strings.TrimSpace(text)
		if text != "" {
			cb.textLines = append(cb.textLines, text)
		}
	}

	return cb
}

// parseTrailingLineComment parses a single line with trailing ///< comment.
func (r *Reader) parseTrailingLineComment(line string) *commentBlock {
	cb := &commentBlock{style: "trailing"}
	cb.rawLines = []string{line}

	idx := strings.Index(line, "///<")
	if idx >= 0 {
		cb.prefix = line[:idx]
		text := strings.TrimSpace(line[idx+4:])
		if text != "" {
			cb.textLines = append(cb.textLines, text)
		}
	}
	return cb
}

// parseTrailingBlockComment parses a single line with trailing /*!< text */.
func (r *Reader) parseTrailingBlockComment(line string) *commentBlock {
	cb := &commentBlock{style: "trailing_qt"}
	cb.rawLines = []string{line}

	idx := strings.Index(line, "/*!<")
	if idx >= 0 {
		cb.prefix = line[:idx]
		rest := line[idx+4:]
		endIdx := strings.Index(rest, "*/")
		if endIdx >= 0 {
			rest = rest[:endIdx]
		}
		text := strings.TrimSpace(rest)
		if text != "" {
			cb.textLines = append(cb.textLines, text)
		}
	}
	return cb
}

// emitCommentBlock emits Parts for a parsed comment block.
func (r *Reader) emitCommentBlock(ctx context.Context, ch chan<- model.PartResult, cb *commentBlock, blockCounter, dataCounter *int) {
	// For trailing comments, emit the code prefix as Data first
	if (cb.style == "trailing" || cb.style == "trailing_qt") && cb.prefix != "" {
		*dataCounter++
		data := &model.Data{
			ID:         fmt.Sprintf("d%d", *dataCounter),
			Name:       fmt.Sprintf("code.prefix.%d", *dataCounter),
			Properties: map[string]string{"raw": cb.prefix},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
			return
		}
	}

	// Process text lines: handle Doxygen commands and extract translatable text
	translatableLines := r.extractTranslatable(cb.textLines)

	if len(translatableLines) == 0 {
		// No translatable text — emit entire comment as Data
		*dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", *dataCounter),
			Name: fmt.Sprintf("comment.%d", *dataCounter),
			Properties: map[string]string{
				"style": cb.style,
				"raw":   strings.Join(cb.rawLines, "\n"),
			},
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		return
	}

	// Emit translatable text as blocks
	for _, group := range translatableLines {
		*blockCounter++
		text := strings.Join(group, "\n")
		block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), text)
		block.Name = fmt.Sprintf("comment.%d", *blockCounter)
		block.Properties["style"] = cb.style
		block.Properties["raw"] = strings.Join(cb.rawLines, "\n")

		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// extractTranslatable processes comment text lines and returns groups of translatable text.
// It handles Doxygen commands, code blocks, and inline commands.
func (r *Reader) extractTranslatable(textLines []string) [][]string {
	var groups [][]string
	var current []string
	inExclude := false

	for _, line := range textLines {
		trimmed := strings.TrimSpace(line)

		// Check for block exclude commands (e.g. @code, \code)
		cmd := r.extractCommand(trimmed)
		if cmd != "" {
			if cmd == "code" || cmd == "verbatim" || cmd == "dot" || cmd == "msc" ||
				cmd == "htmlonly" || cmd == "latexonly" || cmd == "xmlonly" ||
				cmd == "manonly" || cmd == "rtfonly" || cmd == "docbookonly" {
				// Flush current group
				if len(current) > 0 {
					groups = append(groups, current)
					current = nil
				}
				inExclude = true
				continue
			}
			if cmd == "endcode" || cmd == "endverbatim" || cmd == "enddot" || cmd == "endmsc" ||
				cmd == "endhtmlonly" || cmd == "endlatexonly" || cmd == "endxmlonly" ||
				cmd == "endmanonly" || cmd == "endrtfonly" || cmd == "enddocbookonly" {
				inExclude = false
				continue
			}
		}

		if inExclude {
			continue
		}

		// Handle image command: @image format file [caption]
		if cmd == "image" {
			// The image command itself is not translatable
			continue
		}

		// Handle non-translatable commands: entire line is metadata
		if cmd != "" && nonTranslatableCommands[cmd] {
			// These commands have arguments that are not translatable.
			// But some like @addtogroup have a description after the group name.
			if cmd == "addtogroup" || cmd == "defgroup" {
				desc := r.extractDescriptionAfterCommand(trimmed, cmd, 1)
				if desc != "" {
					current = append(current, desc)
				}
			}
			continue
		}

		// Handle translatable description commands: @param name description
		if cmd != "" && translatableDescCommands[cmd] {
			desc := r.extractDescriptionAfterCommand(trimmed, cmd, r.commandArgCount(cmd))
			if desc != "" {
				// Flush previous group and start new one for this command
				if len(current) > 0 {
					groups = append(groups, current)
					current = nil
				}
				current = append(current, desc)
				continue
			}
			continue
		}

		// Handle inline commands by stripping the command prefix
		processed := r.processInlineCommands(trimmed)
		if processed != "" {
			current = append(current, processed)
		}
	}

	if len(current) > 0 {
		groups = append(groups, current)
	}

	return groups
}

// extractCommand extracts the Doxygen command name from a line, if any.
// Supports both @command and \command syntax.
func (r *Reader) extractCommand(line string) string {
	for i, ch := range line {
		if ch == '@' || ch == '\\' {
			rest := line[i+1:]
			// Extract the command word
			end := 0
			for end < len(rest) && isCommandChar(rest[end]) {
				end++
			}
			if end > 0 {
				return rest[:end]
			}
		}
		// Only look for commands at the start of the line (after whitespace)
		if ch != ' ' && ch != '\t' {
			break
		}
	}
	return ""
}

// isCommandChar returns true if the byte is valid in a Doxygen command name.
func isCommandChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '$' || b == '[' || b == ']'
}

// extractDescriptionAfterCommand extracts the description text after a command and its N arguments.
func (r *Reader) extractDescriptionAfterCommand(line, cmd string, skipArgs int) string {
	// Find the command in the line
	idx := strings.Index(line, "@"+cmd)
	if idx < 0 {
		idx = strings.Index(line, "\\"+cmd)
	}
	if idx < 0 {
		return ""
	}
	rest := line[idx+1+len(cmd):]
	rest = strings.TrimSpace(rest)

	// Skip the specified number of arguments
	for i := 0; i < skipArgs && rest != ""; i++ {
		// Skip quoted argument
		if rest[0] == '"' {
			endQuote := strings.Index(rest[1:], "\"")
			if endQuote >= 0 {
				rest = rest[endQuote+2:]
			} else {
				return ""
			}
		} else {
			// Skip whitespace-delimited argument
			spaceIdx := strings.IndexAny(rest, " \t")
			if spaceIdx >= 0 {
				rest = rest[spaceIdx:]
			} else {
				return ""
			}
		}
		rest = strings.TrimSpace(rest)
	}

	return rest
}

// commandArgCount returns the number of non-description arguments a command takes.
func (r *Reader) commandArgCount(cmd string) int {
	switch cmd {
	case "param", "tparam", "throw", "throws", "exception", "retval":
		return 1 // param name
	default:
		return 0
	}
}

// processInlineCommands strips inline Doxygen commands (@e, @a, @b, etc.)
// and returns the text with the command markers removed.
func (r *Reader) processInlineCommands(line string) string {
	result := line
	for _, prefix := range []string{"@", "\\"} {
		for cmd := range inlineCommands {
			marker := prefix + cmd + " "
			result = strings.ReplaceAll(result, marker, "")
		}
	}
	return result
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
