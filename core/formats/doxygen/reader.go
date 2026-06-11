package doxygen

import (
	"bytes"
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

// Reader implements DataFormatReader for Doxygen/Javadoc comments in source code.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs

	// lineEnding is the dominant per-file line terminator detected from
	// the source bytes ("\r\n" for CRLF, "\n" for LF). Recorded on every
	// emitted Block so the writer can preserve the source's line-ending
	// convention when expanding multi-line comment templates rather than
	// hardcoding "\n" — without this, CRLF-source files like lists.h
	// lose every interior CR on round-trip (one CR per body line of every
	// translatable comment, e.g. ~15 bytes for the doxygen lists.h fixture).
	lineEnding string
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
	"class": true, "file": true, "see": true,
	"namespace": true, "package": true, "defgroup": true, "ingroup": true,
	"addtogroup": true, "name": true, "typedef": true, "enum": true,
	"struct": true, "union": true, "fn": true, "var": true,
	"def": true, "headerfile": true, "mainpage": true,
	"anchor": true, "copydoc": true,
	"include": true, "dontinclude": true, "line": true, "skip": true,
	"skipline": true, "until": true, "example": true, "dir": true,
	"relates": true, "relatesalso": true, "memberof": true,
	"property": true, "implements": true, "extends": true,
	"snippet": true, "verbinclude": true, "htmlinclude": true,
	"latexinclude": true, "docbookinclude": true, "maninclude": true,
	"rtfinclude": true, "xmlinclude": true, "includelineno": true,
	// Group markers — `@{` opens a member group, `@}` closes one. Their
	// presence on a line means "structural marker", not "translatable
	// prose"; mirrors okapi's PLACEHOLDER classification for them.
	"{": true, "}": true,
	// `\overload` takes a function-declaration LINE arg that okapi
	// declares non-translatable in doxygenConfiguration.yml. Without
	// listing it here the declaration text gets pseudo-translated
	// (special_commands.h `\overload void Test::drawRect(...)`).
	"overload": true,
}

// sectionHeaderCommands map command names to the number of WORD-typed
// non-translatable arguments before the translatable title/text/heading.
// Per okapi's doxygenConfiguration.yml, these PLACEHOLDER commands take
// a leading non-translatable identifier (section name, page id, anchor
// key, ...) followed by translatable display text. Native must keep the
// identifier portion intact while extracting the trailing prose so
// pseudo-translation hits the title without mangling the ID. Without
// this, `\section intro_sec Introduction` either skips translation
// entirely (when the cmd lives in nonTranslatableCommands) or
// pseudo-translates the ID (when treated as inline prose), neither of
// which matches okapi's split.
var sectionHeaderCommands = map[string]int{
	"section":       1,
	"subsection":    1,
	"subsubsection": 1,
	"paragraph":     1,
	"page":          1,
	"subpage":       1,
	"xrefitem":      1, // key non-translatable; heading + list-title translate
	"image":         2, // format + file non-translatable; caption translatable
	// `\par` carries an optional LINE-length title that translates per
	// okapi's doxygenConfiguration.yml — but the (commented-out)
	// PARAGRAPH body parameter is NOT extracted. Treat `\par TITLE` as
	// a section header so the title becomes its own paragraph and the
	// following prose isn't absorbed (special_commands.h `\par User
	// defined paragraph:` followed by `Contents of the paragraph.`
	// stays on two lines like okapi emits).
	"par": 0,
}

// quotedDescCommands are sectionHeaderCommands whose translatable
// description is a quoted string followed by non-translatable trailing
// parameters. Only the text inside the quotes is extracted — everything
// after the closing quote (e.g. `width=10cm` for `\image`) stays in
// the skeleton.
var quotedDescCommands = map[string]bool{
	"image": true,
}

// paragraphBreakCommands enumerates Doxygen commands whose line-start
// occurrence opens a new conditional / language block / list item:
// `\if`, `\else`, `\elseif`, `\ifnot`, `\cond`, `\arg`, `\li`. okapi
// treats these as paragraph-opening structural markers — the prose on
// the line stays its own paragraph and the previous paragraph never
// absorbs across it. `\endif` / `\endcond` are NOT in this set: okapi
// merges them into whichever prose precedes them on the same line via
// WhitespaceCollapse (e.g. `Only included if Cond1 is set. \endif`).
var paragraphBreakCommands = map[string]bool{
	"if": true, "ifnot": true, "else": true, "elseif": true,
	"cond": true,
	// `\arg` and `\li` are list-item markers — each entry is its own
	// paragraph and adjacent items never join via WhitespaceCollapse
	// (special_commands.h `\arg \c AlignLeft …` lines stay separate
	// instead of being collapsed into one mega-line).
	"arg": true, "li": true,
}

// translatableDescCommands are Doxygen commands whose description text IS translatable.
//
// Per okapi's doxygenConfiguration.yml, these are PLACEHOLDER commands
// whose (commented-out) parameter is `length: PARAGRAPH, translatable:
// true`. The text after the command (until the next blank line or
// command) flows as the prose body. Native treats these as "anchor
// + absorb-following-prose" via joinProseLines so multi-line bodies
// roundtrip on a single output line, matching okapi's whitespace
// collapse.
var translatableDescCommands = map[string]bool{
	"brief": true, "details": true, "short": true,
	"param": true, "return": true, "returns": true, "retval": true,
	"throw": true, "throws": true, "exception": true,
	"note": true, "warning": true, "remark": true, "remarks": true,
	"attention": true, "bug": true, "todo": true, "test": true,
	"pre": true, "post": true, "invariant": true,
	"tparam": true, "sa": true,
	// PLACEHOLDER + commented-out PARAGRAPH/translatable arg in
	// okapi's YAML — okapi pseudo-translates the text after these
	// commands too. Without including them here, text like
	// `\copyright GNU Public License.` falls through to prose and
	// gets joined with the previous `\warning` line.
	"copyright": true, "author": true, "authors": true,
	"date": true, "version": true,
	"since": true, "deprecated": true,
}

// The Doxygen inline-formatting commands (\e, \a, \b, \c, \p, \em) mark up the
// next word. The reader keeps them in the extracted text so the writer can
// roundtrip the source verbatim — see processInlineCommands, which deliberately
// leaves the markers in place rather than matching against an enumerated set.

// rawLine holds a line's content and its original line ending.
type rawLine struct {
	content    string
	lineEnding string
}

// detectDominantLineEnding returns the line ending that appears most
// often across rLines. Ties go to "\r\n" so a file that opens with
// CRLF stays CRLF on round-trip even if it ends with a final-line
// without a terminator. Empty input returns "" so callers can fall
// back to a default.
func detectDominantLineEnding(rLines []rawLine) string {
	var crlf, lf int
	for _, rl := range rLines {
		switch rl.lineEnding {
		case "\r\n":
			crlf++
		case "\n":
			lf++
		}
	}
	if crlf == 0 && lf == 0 {
		return ""
	}
	if crlf >= lf {
		return "\r\n"
	}
	return "\n"
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
		r.lineEnding = detectDominantLineEnding(rLines)
	} else {
		// Non-skeleton path: read all bytes ourselves so we can detect
		// CRLF vs LF up front (bufio.Scanner discards line endings).
		// Without this the writer also loses CRLF on no-skeleton round-
		// trips because Block emission feeds the writer's "\n" joiner
		// which silently drops the source's CRs.
		data, err := io.ReadAll(r.Doc.Reader)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("doxygen: reading: %w", err)}
			return
		}
		peeked := splitRawLines(data)
		r.lineEnding = detectDominantLineEnding(peeked)
		for _, rl := range peeked {
			lines = append(lines, rl.content)
		}
	}
	if r.lineEnding == "" {
		r.lineEnding = "\n"
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
			before, _, _ := strings.Cut(line, "///<")
			before = strings.TrimSpace(before)
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

		// Check for Python triple-quoted docstring: """
		if strings.Contains(trimmed, `"""`) {
			cb := r.parseDocstring(lines, i)
			n := len(cb.rawLines)
			r.skelCommentGroup(cb, rLines, i, n, &blockCounter)
			dataCounter++
			r.emitCommentBlock(ctx, ch, cb, &blockCounter, &dataCounter)
			i += n
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
	for j := range count {
		r.skelText(rLines[start+j].content + rLines[start+j].lineEnding)
	}
}

// isTrailingComment checks if a line has code followed by ///< comment.
func (r *Reader) isTrailingComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	// If the line starts with ///, it's a regular line comment, not trailing
	if strings.HasPrefix(trimmed, "///") {
		// But if it has "///<", check if there's code before the "///<"
		before, _, ok := strings.Cut(line, "///<")
		if !ok {
			return false
		}
		before = strings.TrimSpace(before)
		return before != ""
	}
	return false
}

// isTrailingBlockComment checks if a line has code followed by /*!< text */
// or /**< text */. Both Qt-style (/*!<) and Javadoc-style (/**<) trailing
// block markers are recognised.
func (r *Reader) isTrailingBlockComment(trimmed string) bool {
	marker, idx := findTrailingBlockMarker(trimmed)
	if idx < 0 {
		return false
	}
	before := strings.TrimSpace(trimmed[:idx])
	return before != "" && strings.Contains(trimmed[idx+len(marker):], "*/")
}

// findTrailingBlockMarker returns ("/**<" or "/*!<", index) for the first
// trailing block marker present in s, or ("", -1) if none.
func findTrailingBlockMarker(s string) (string, int) {
	idxQt := strings.Index(s, "/*!<")
	idxJd := strings.Index(s, "/**<")
	switch {
	case idxQt < 0 && idxJd < 0:
		return "", -1
	case idxQt < 0:
		return "/**<", idxJd
	case idxJd < 0:
		return "/*!<", idxQt
	case idxQt < idxJd:
		return "/*!<", idxQt
	default:
		return "/**<", idxJd
	}
}

// parseBlockComment parses a /** ... */ or /*! ... */ block comment starting at line index i.
//
// Doxygen also accepts the "after" / "member" marker variants /**< … */
// and /*!< … */ at the start of a block comment (in addition to their
// trailing usage on lines that have code before them, handled by
// parseTrailingBlockComment). When the comment opens with `/**<` or
// `/*!<` and there is no code in front, the entire comment still
// extracts as a normal block comment, but the writer must reproduce
// the `<` so the output stays byte-equal — the cb.style ("qt_member"
// / "javadoc_member") records that.
func (r *Reader) parseBlockComment(lines []string, start int) *commentBlock {
	cb := &commentBlock{}
	trimmedFirst := strings.TrimSpace(lines[start])

	var openMarker string
	switch {
	case strings.HasPrefix(trimmedFirst, "/*!<"):
		cb.style = "qt_member"
		openMarker = "/*!<"
	case strings.HasPrefix(trimmedFirst, "/**<"):
		cb.style = "javadoc_member"
		openMarker = "/**<"
	case strings.HasPrefix(trimmedFirst, "/*!"):
		cb.style = "qt"
		openMarker = "/*!"
	default:
		cb.style = "javadoc"
		openMarker = "/**"
	}

	// Check if it's a single-line block comment like "/** text */"
	if strings.Contains(trimmedFirst, "*/") {
		cb.rawLines = []string{lines[start]}
		text := trimmedFirst
		text = strings.TrimPrefix(text, openMarker)
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

	// Extract text from each line. Middle lines (interior body) are
	// kept even when blank — the empty entry signals a paragraph break
	// to extractTranslatable / joinProseLines, mirroring okapi's
	// BLANK_LINES paragraph splitter so the writer can preserve
	// paragraph boundaries when joining consecutive prose lines.
	for idx, raw := range cb.rawLines {
		text := raw
		isFirst := idx == 0
		isLast := idx == len(cb.rawLines)-1
		switch {
		case isFirst:
			// First line: remove opening delimiter (including the
			// optional `<` member marker, captured in openMarker).
			trimmed := strings.TrimSpace(text)
			trimmed = strings.TrimPrefix(trimmed, openMarker)
			text = strings.TrimSpace(trimmed)
		case isLast:
			// Last line: remove closing delimiter
			text = strings.TrimSpace(text)
			text = strings.TrimSuffix(text, "*/")
			text = strings.TrimSpace(text)
			text = strings.TrimPrefix(text, "*")
			text = strings.TrimSpace(text)
		default:
			// Middle lines: remove leading " * "
			text = strings.TrimSpace(text)
			text = strings.TrimPrefix(text, "*")
			text = strings.TrimSpace(text)
		}
		if text != "" {
			cb.textLines = append(cb.textLines, text)
		} else if !isFirst && !isLast {
			// Preserve blank middle lines as empty textLine entries
			// (paragraph break markers). Skipping the first (delimiter
			// line with no body, e.g. `/**`) and last (closing `*/`)
			// keeps the textLines aligned with the body region only.
			cb.textLines = append(cb.textLines, "")
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
		// Preserve blank `///` / `//!` lines as empty textLine
		// entries so extractTranslatable's BLANK_LINES paragraph
		// splitter can flush groups at paragraph boundaries.
		// Without this joinProseLines bleeds across paragraphs in
		// fixtures like sample.h's constructor block.
		cb.textLines = append(cb.textLines, text)
	}

	return cb
}

// parseTrailingLineComment parses a single line with trailing ///< comment.
func (r *Reader) parseTrailingLineComment(line string) *commentBlock {
	cb := &commentBlock{style: "trailing"}
	cb.rawLines = []string{line}

	before, after, ok := strings.Cut(line, "///<")
	if ok {
		cb.prefix = before
		text := strings.TrimSpace(after)
		if text != "" {
			cb.textLines = append(cb.textLines, text)
		}
	}
	return cb
}

// parseTrailingBlockComment parses a single line with trailing /*!< text */
// or /**< text */. The recognised marker selects the comment style so the
// writer can reproduce the original delimiter on round-trip.
func (r *Reader) parseTrailingBlockComment(line string) *commentBlock {
	marker, idx := findTrailingBlockMarker(line)
	cb := &commentBlock{}
	if marker == "/**<" {
		cb.style = "trailing_javadoc"
	} else {
		cb.style = "trailing_qt"
	}
	cb.rawLines = []string{line}

	if idx >= 0 {
		cb.prefix = line[:idx]
		rest := line[idx+len(marker):]
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

// parseDocstring collects a Python triple-quoted docstring starting at index i.
// Handles both single-line ("""text""") and multi-line ("""...\n...\n""") forms.
func (r *Reader) parseDocstring(lines []string, start int) *commentBlock {
	cb := &commentBlock{style: "docstring"}

	line := lines[start]
	trimmed := strings.TrimSpace(line)

	// Find the opening """
	_, after, ok := strings.Cut(trimmed, `"""`)
	if !ok {
		cb.rawLines = []string{line}
		return cb
	}

	afterOpen := after

	// Check for single-line docstring: """text"""
	before, _, ok := strings.Cut(afterOpen, `"""`)
	if ok {
		cb.rawLines = []string{line}
		text := strings.TrimSpace(before)
		if text != "" {
			cb.textLines = append(cb.textLines, text)
		}
		return cb
	}

	// Multi-line docstring
	cb.rawLines = append(cb.rawLines, line)
	// Text after """ on opening line
	firstText := strings.TrimSpace(afterOpen)
	if firstText != "" {
		cb.textLines = append(cb.textLines, firstText)
	}

	// Continue until closing """
	for j := start + 1; j < len(lines); j++ {
		cb.rawLines = append(cb.rawLines, lines[j])
		lineTrimmed := strings.TrimSpace(lines[j])

		if strings.Contains(lineTrimmed, `"""`) {
			// Text before closing """
			before, _, _ := strings.Cut(lineTrimmed, `"""`)
			beforeClose := strings.TrimSpace(before)
			if beforeClose != "" {
				cb.textLines = append(cb.textLines, beforeClose)
			}
			break
		}
		// Regular content line — preserve blank lines as paragraph separators
		cb.textLines = append(cb.textLines, strings.TrimSpace(lines[j]))
	}

	return cb
}
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

	// Build a per-comment-group line layout that the writer applies to
	// emit raw structural lines (blank `///`, `\code…\endcode` blocks,
	// non-translatable command lines) verbatim while substituting the
	// translatable text portions in place. Without this the writer can
	// only emit canonical `///{text}` lines and silently drops blank
	// `///` separators and excluded code blocks on roundtrip.
	//
	// Each layout entry is one of:
	//   T:<prefix>      consume the next text line, emit `<prefix><text>`
	//                   (the `///` / `//!` / `*` line marker is added by
	//                   the writer per comment style)
	//   S:<raw>         emit `<raw>` verbatim (no comment marker added)
	// Entries are joined with \x01 so prefixes/raw lines containing
	// any character are unambiguous.
	layout := r.buildLineLayout(cb, translatableLines)

	// Emit translatable text as blocks. When a comment group contains
	// multiple translatable sections (e.g. \param a … \param b … \return …)
	// each section becomes its own Block per the spec contract, and we
	// tag every block in the group with groupSize / groupIndex /
	// groupFirstID so the writer can stitch them back into one comment
	// template instead of emitting one comment-per-Block (which would
	// multiply delimiters and silently drop blocks beyond the first
	// skeleton ref).
	groupSize := len(translatableLines)
	firstID := fmt.Sprintf("tu%d", *blockCounter+1)
	for idx, group := range translatableLines {
		*blockCounter++
		texts := make([]string, len(group))
		prefixes := make([]string, len(group))
		hasAnyPrefix := false
		for i, tl := range group {
			texts[i] = tl.text
			prefixes[i] = tl.prefix
			if tl.prefix != "" {
				hasAnyPrefix = true
			}
		}
		// For block-comment styles (/** … */ and /*! … */) AND for
		// line-comment styles (/// and //!), okapi joins consecutive
		// plain-prose lines within a paragraph into a single line
		// separated by spaces, then preserves the original line count
		// by emitting blank `///` / ` * ` lines after. This matches
		// okapi's BLANK_LINES paragraph splitter — only an explicit
		// blank line breaks the join. Mirror that here so the joined
		// text + padding matches the upstream byte sequence.
		if cb.style == "javadoc" || cb.style == "qt" ||
			cb.style == "javadoc_member" || cb.style == "qt_member" ||
			cb.style == "triple" || cb.style == "exclamation" ||
			cb.style == "docstring" {
			texts, prefixes = joinProseLines(texts, prefixes)
		}
		// Build the block's source as a Run sequence rather than a
		// single text string. tokenizedRunsForLines tokenizes inline
		// Doxygen commands (\a x, \param y, \n …) and HTML tags into
		// PlaceholderRuns so they survive pseudo-translation byte-for-
		// byte. Pure-prose lines collapse to a single TextRun, matching
		// the previous behaviour for fixtures with no inline commands.
		runs := tokenizedRunsForLines(texts)
		block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), "")
		block.SetSourceRuns(runs)
		block.Name = fmt.Sprintf("comment.%d", *blockCounter)
		block.Properties["style"] = cb.style
		block.Properties["raw"] = strings.Join(cb.rawLines, "\n")
		// Per-block line-ending hint for the writer. Defaults to "\n"
		// when detection found nothing (e.g. unit tests with synthetic
		// in-memory content); only the non-default CRLF case ships a
		// property so existing fixtures stay LF-clean.
		if r.lineEnding != "" && r.lineEnding != "\n" {
			block.Properties["lineEnding"] = r.lineEnding
		}
		// Store per-line command prefixes so the writer can reattach
		// stripped Doxygen markers (e.g. "\brief ", "\param x ") on
		// roundtrip. Joined with \x00 since prefixes themselves are
		// single-line strings.
		if hasAnyPrefix {
			block.Properties["linePrefixes"] = strings.Join(prefixes, "\x00")
		}
		if groupSize > 1 {
			block.Properties["groupSize"] = strconv.Itoa(groupSize)
			block.Properties["groupIndex"] = strconv.Itoa(idx)
			block.Properties["groupFirstID"] = firstID
		}
		// Only the first block of a group carries the layout; the
		// writer looks it up via groupFirstID.
		if idx == 0 && layout != "" {
			block.Properties["lineLayout"] = layout
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// buildLineLayout produces a layout descriptor for a comment group
// (see emitCommentBlock for format and motivation). Returns "" when
// the layout adds nothing beyond the canonical per-text-line
// emission, so common cases (single-line comments, plain prose
// without structural blanks) skip the templated-render path.
func (r *Reader) buildLineLayout(cb *commentBlock, groups [][]translatableLine) string {
	// Block-comment styles (/** … */ and /*! … */) need their own
	// layout flavour: each raw line carries a delimiter or `*` line
	// marker that the writer must reproduce exactly to preserve
	// indentation / inline-text-on-opening-delimiter / closing-
	// delimiter alignment. Build a raw-with-placeholder layout for
	// those.
	if cb.style == "javadoc" || cb.style == "qt" ||
		cb.style == "javadoc_member" || cb.style == "qt_member" {
		return r.buildBlockLayout(cb)
	}
	if cb.style == "docstring" {
		return r.buildDocstringLayout(cb)
	}
	if cb.style != "triple" && cb.style != "exclamation" {
		return ""
	}
	// Map textLines index → translatableLine so we can quickly
	// answer "did this raw comment line contribute translatable
	// text, and what prefix did we strip?". Keeping this aligned
	// with extractTranslatable's iteration is critical — the layout
	// must reference the same set of textLines that became Blocks.
	textIdxToTL := r.classifyTextLines(cb.textLines)

	// Walk rawLines and emit either a T entry (when this raw line
	// produced a translatable text) or an S entry (passthrough).
	//
	// T entries encode the FULL line prefix including indent, comment
	// marker, and any stripped Doxygen command marker — the writer
	// just emits `<body><translated text>`. Embedding the indent
	// preserves source formatting when other lines in the same comment
	// group force the layout path (e.g. an embedded `\code…\endcode`
	// block). Without this the writer would fall back to a marker-only
	// "/// " prefix and silently strip indentation.
	textCursor := 0 // running index into cb.textLines as we walk rawLines
	var entries []string
	hasS := false
	for _, raw := range cb.rawLines {
		// Determine the line's marker prefix by stripping the comment
		// marker and any indentation. Match the same stripping
		// parseLineComments does so the textCursor stays aligned.
		trimmed := strings.TrimSpace(raw)
		var stripped string
		isTriple := strings.HasPrefix(trimmed, "///")
		isExcl := strings.HasPrefix(trimmed, "//!")
		switch {
		case isTriple:
			stripped = strings.TrimSpace(strings.TrimPrefix(trimmed, "///"))
		case isExcl:
			stripped = strings.TrimSpace(strings.TrimPrefix(trimmed, "//!"))
		default:
			// Should not happen for line-comment styles, but be safe.
			entries = append(entries, "S:"+raw)
			hasS = true
			continue
		}
		if stripped == "" {
			// Blank comment-marker separator line — pass through and
			// advance textCursor to keep alignment with the parser,
			// which appends an empty entry to textLines for these
			// blanks (so extractTranslatable can detect paragraph
			// boundaries).
			entries = append(entries, "S:"+raw)
			hasS = true
			textCursor++
			continue
		}
		// Match parse*Comments: only non-empty stripped lines
		// contribute to textLines. Look up whether the current
		// textCursor entry is translatable.
		if tl, isTrans := textIdxToTL[textCursor]; isTrans {
			// Reconstruct the full per-line prefix verbatim from raw:
			// original indent + comment marker WITH ITS POST-MARKER
			// WHITESPACE (`/// `, `///   `, `//!  `, `## ` …) + the
			// stripped Doxygen command marker (e.g. `\param a `).
			// Capturing the extra whitespace is what preserves source
			// indentation inside list items like `///   -# foo`
			// (3 spaces between `///` and `-#` — naïvely using a
			// canonical `/// ` marker would silently collapse to 1
			// space).
			markerLen := 3 // both `///` and `//!` are 3 chars
			afterMarker := raw[strings.Index(raw, trimmed[:markerLen])+markerLen:]
			ws := afterMarker[:len(afterMarker)-len(strings.TrimLeft(afterMarker, " \t"))]
			marker := trimmed[:markerLen] + ws
			indent := raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))]
			entries = append(entries, "T:"+indent+marker+tl.prefix)
		} else {
			// Non-translatable command line (e.g. \file, \author,
			// \code, \endcode) — pass through verbatim.
			entries = append(entries, "S:"+raw)
			hasS = true
		}
		textCursor++
	}
	// If layout has no S entries the canonical writer path matches
	// the original line for line — no need to attach a template.
	if !hasS {
		return ""
	}
	return strings.Join(entries, "\x01")
}

// buildDocstringLayout produces a layout descriptor for Python triple-
// quoted docstrings. It mirrors okapi's DoxygenFilter conventions:
//
//   - When the opening `"""` line carries translatable text, a T entry
//     consumes the first text line into `indent"""<text>`.
//   - Non-translatable opening text (e.g. `@package docstring`) becomes
//     an S entry (emitted verbatim).
//   - Blank separators between paragraphs get the docstring's indent.
//   - For indented docstrings, okapi writes the closing `"""` at column 0
//     preceded by an extra blank line. The layout encodes this.
//   - For column-0 docstrings, the closing `"""` stays at column 0
//     with no extra blank line.
func (r *Reader) buildDocstringLayout(cb *commentBlock) string {
	if len(cb.rawLines) <= 1 {
		// Single-line docstring: """text""" — canonical writer handles it.
		return ""
	}
	textIdxToTL := r.classifyTextLines(cb.textLines)

	// Determine the docstring's leading indentation from the opening line.
	indent := extractIndent(strings.Join(cb.rawLines, "\n"))

	var entries []string
	textCursor := 0

	// --- Opening line ---
	firstTrimmed := strings.TrimSpace(cb.rawLines[0])
	_, after, _ := strings.Cut(firstTrimmed, `"""`)
	afterOpen := strings.TrimSpace(after)

	if afterOpen != "" {
		if _, isTrans := textIdxToTL[textCursor]; isTrans {
			// Translatable text follows `"""` → T entry so the
			// writer substitutes pseudo-translated text here.
			entries = append(entries, "T:"+indent+`"""`)
		} else {
			// Non-translatable command (e.g. `@package docstring`) →
			// preserve the opening line verbatim.
			entries = append(entries, "S:"+cb.rawLines[0])
		}
		textCursor++
	} else {
		// Opening `"""` with no text on the same line.
		entries = append(entries, "S:"+cb.rawLines[0])
	}

	// --- Body lines (between opening and closing) ---
	for j := 1; j < len(cb.rawLines)-1; j++ {
		contentText := strings.TrimSpace(cb.rawLines[j])
		if contentText == "" {
			// Blank line → paragraph separator. Okapi indents these
			// to match the docstring's leading whitespace.
			entries = append(entries, "S:"+indent)
			textCursor++
			continue
		}
		if _, isTrans := textIdxToTL[textCursor]; isTrans {
			entries = append(entries, "T:"+indent)
		} else {
			entries = append(entries, "S:"+cb.rawLines[j])
		}
		textCursor++
	}

	// --- Closing line ---
	closingLine := cb.rawLines[len(cb.rawLines)-1]
	closingTrimmed := strings.TrimSpace(closingLine)
	before, _, ok := strings.Cut(closingTrimmed, `"""`)

	// Unterminated docstring: the opening `"""` has no matching close
	// before EOF, so the final raw line is genuine body content rather
	// than a closing delimiter. Emit it verbatim and do not synthesize a
	// closing `"""` that the source never had — guarding ci avoids a
	// slice-bounds panic on closingTrimmed[:ci] when ci == -1.
	if !ok {
		entries = append(entries, "S:"+closingLine)
		return strings.Join(entries, "\x01")
	}

	// Check for text before the closing `"""` (rare but valid).
	beforeClose := strings.TrimSpace(before)
	if beforeClose != "" {
		if _, isTrans := textIdxToTL[textCursor]; isTrans {
			entries = append(entries, "T:"+indent)
		} else {
			entries = append(entries, "S:"+closingLine)
		}
	}

	// Okapi convention: indented docstrings get a blank line + column-0
	// closing `"""`. Non-indented docstrings keep the closing as-is.
	if indent != "" {
		entries = append(entries, "S:")       // blank line
		entries = append(entries, "S:"+`"""`) // closing at column 0
	} else {
		entries = append(entries, "S:"+closingLine)
	}

	return strings.Join(entries, "\x01")
}

// buildBlockLayout produces a layout descriptor for /** … */ and
// /*! … */ block comments. Each raw line is encoded as one of:
//
//	B:<linePrefix>      consume next text line, emit `<linePrefix><text>`
//	S:<rawLine>         emit `<rawLine>` verbatim (no text substitution)
//
// linePrefix carries everything in the original line up to (but not
// including) the translatable text — i.e. `/*! `, ` *  `, leading
// indentation, plus any Doxygen command marker (`\brief `,
// `\param x `, `\addtogroup mygroup `) that was stripped during
// extraction. Stitched together with `\n`-separated `\x01` entries.
// Returns "" for single-line block comments where the canonical
// writer matches the original byte-for-byte.
func (r *Reader) buildBlockLayout(cb *commentBlock) string {
	if len(cb.rawLines) <= 1 {
		return ""
	}
	// Walk extractTranslatable's classification to map textLine
	// index → translatable text (post-prefix-strip). Matches the
	// per-cmd handling so the block layout's B: entries land on the
	// same set of textLines that became Block resources.
	textIdxToTL := r.classifyTextLines(cb.textLines)

	// Match parseBlockComment's iteration to know which raw lines
	// produced text vs. which became closing delimiters / blanks.
	var entries []string
	textCursor := 0
	parsedCursor := 0 // index into cb.textLines (post-trim, non-empty only)
	for idx, raw := range cb.rawLines {
		// Compute trimmed text exactly as parseBlockComment does so
		// the parsedCursor lines up with cb.textLines.
		var text string
		if idx == 0 {
			t := strings.TrimSpace(raw)
			switch cb.style {
			case "qt_member":
				t = strings.TrimPrefix(t, "/*!<")
			case "javadoc_member":
				t = strings.TrimPrefix(t, "/**<")
			case "qt":
				t = strings.TrimPrefix(t, "/*!")
			default:
				t = strings.TrimPrefix(t, "/**")
			}
			text = strings.TrimSpace(t)
		} else if idx == len(cb.rawLines)-1 {
			t := strings.TrimSpace(raw)
			t = strings.TrimSuffix(t, "*/")
			t = strings.TrimSpace(t)
			t = strings.TrimPrefix(t, "*")
			text = strings.TrimSpace(t)
		} else {
			t := strings.TrimSpace(raw)
			t = strings.TrimPrefix(t, "*")
			text = strings.TrimSpace(t)
		}
		if text == "" {
			// Pure delimiter / blank line — emit as raw passthrough.
			// Advance parsedCursor on body-blank lines (interior
			// `*` decoration) so it stays aligned with cb.textLines,
			// which now preserves blank entries as paragraph-break
			// markers. The first delimiter line (idx == 0, e.g.
			// `/**`) and last (`*/`) are NOT recorded in textLines,
			// so they don't bump parsedCursor.
			if idx > 0 && idx < len(cb.rawLines)-1 {
				parsedCursor++
			}
			entries = append(entries, "S:"+raw)
			continue
		}
		// Look up whether this textLine survived classification
		// (i.e. became a Block resource).
		tl, isTrans := textIdxToTL[parsedCursor]
		parsedCursor++
		if !isTrans {
			// Text line dropped by extractTranslatable (\file,
			// \class metadata, \code…\endcode body, …) — emit raw
			// passthrough.
			entries = append(entries, "S:"+raw)
			continue
		}
		// Locate where the translatable text begins inside raw —
		// the prefix is everything up to that point (which already
		// includes `/*! `, ` *  `, indent, AND the Doxygen command
		// marker like `\brief ` / `\param x `).
		idxT := strings.Index(raw, tl.text)
		if idxT < 0 {
			entries = append(entries, "S:"+raw)
			continue
		}
		prefix := raw[:idxT]
		// Mirror okapi's WHITESPACE_COLLAPSE on the gap between a
		// command marker and its description: ` *  \brief     `
		// (5-space gap from a padded `\brief     Pretty` source) is
		// normalised to ` *  \brief ` so the roundtrip reproduces
		// okapi's collapsed `\brief Pretty` instead of preserving the
		// source's incidental padding. The leading ` *  ` indent stays
		// intact — only collapse the trailing-whitespace run inside
		// the prefix when it follows a Doxygen command marker (so
		// pure-indent prefixes ` *  ` for plain prose lines aren't
		// touched).
		if trimmed := strings.TrimRight(prefix, " \t"); trimmed != prefix && trimmed != "" {
			if strings.ContainsAny(trimmed, "\\@") {
				prefix = trimmed + " "
			}
		}
		// Capture any trailing content AFTER the translatable text
		// in the raw line so the writer can preserve it verbatim.
		// Typical cases:
		//   - trailing whitespace on ` *     . ` (lists.h line 21)
		//   - closing quote + size indicator on `\image` lines:
		//     `\image latex app.eps "My app" width=10cm` — the
		//     suffix `" width=10cm` stays intact while "My app"
		//     is translated.
		// Encoded as B:<prefix>\x02<suffix>; the writer splits on
		// \x02 and emits prefix + text + suffix.
		//
		// Pure trailing whitespace on an ordinary prose line is NOT
		// significant: okapi's WHITESPACE_COLLAPSE folds the trailing
		// space of a continuation line when joinProseLines merges it
		// into the anchor (` * This is \n * a test.` → `This is a
		// test.`, no trailing space). Only preserve a whitespace-only
		// suffix on a doxygen list-end marker (` *     . ` on lists.h
		// line 21), whose `.` row okapi keeps standalone (see
		// isListEndMarker / joinProseLines), and any non-whitespace
		// suffix (e.g. `\image …  "My app" width=10cm`).
		suffix := raw[idxT+len(tl.text):]
		if strings.TrimRight(suffix, " \t") == "" && !isListEndMarker(tl.text) {
			suffix = ""
		}
		if suffix != "" {
			entries = append(entries, "B:"+prefix+"\x02"+suffix)
		} else {
			entries = append(entries, "B:"+prefix)
		}
		textCursor++
	}
	// Layout serves no purpose if the block is a single-line text
	// case the canonical writer already handles.
	if textCursor == 0 {
		return ""
	}
	return strings.Join(entries, "\x01")
}

// classifyTextLines mirrors extractTranslatable's iteration to
// produce a textLines-index → translatableLine map. Used by both
// buildLineLayout and buildBlockLayout to align raw-line layout
// entries with the textLines that survived as Block resources.
func (r *Reader) classifyTextLines(textLines []string) map[int]translatableLine {
	out := make(map[int]translatableLine)
	var inExclude bool
	for i, line := range textLines {
		trimmed := strings.TrimSpace(line)
		// Blank textLine is a paragraph-break marker (preserved by
		// parseBlockComment) — has no translatable content.
		if trimmed == "" {
			continue
		}
		cmd := r.extractCommand(trimmed)
		if cmd != "" {
			if isExcludeStart(cmd) {
				inExclude = true
				continue
			}
			if isExcludeEnd(cmd) {
				inExclude = false
				continue
			}
		}
		if inExclude {
			continue
		}
		if cmd != "" && nonTranslatableCommands[cmd] {
			if cmd == "addtogroup" || cmd == "defgroup" {
				desc, prefix := r.extractDescAndPrefix(trimmed, cmd, 1)
				if desc != "" {
					out[i] = translatableLine{text: desc, prefix: prefix}
				}
			}
			continue
		}
		if skipArgs, ok := sectionHeaderCommands[cmd]; cmd != "" && ok {
			desc, prefix := r.extractDescAndPrefix(trimmed, cmd, skipArgs)
			if desc != "" {
				out[i] = translatableLine{text: desc, prefix: prefix}
			}
			continue
		}
		if cmd != "" && translatableDescCommands[cmd] {
			desc, prefix := r.extractDescAndPrefix(trimmed, cmd, r.commandArgCount(cmd))
			if desc != "" {
				out[i] = translatableLine{text: desc, prefix: prefix}
			}
			continue
		}
		processed := r.processInlineCommands(trimmed)
		if processed != "" {
			out[i] = translatableLine{text: processed}
		}
	}
	return out
}

// isExcludeStart reports whether cmd starts a Doxygen exclude region
// (matches the in-line list in extractTranslatable).
func isExcludeStart(cmd string) bool {
	switch cmd {
	case "code", "verbatim", "dot", "msc",
		"htmlonly", "latexonly", "xmlonly",
		"manonly", "rtfonly", "docbookonly":
		return true
	}
	return false
}

// isExcludeEnd reports whether cmd ends a Doxygen exclude region.
func isExcludeEnd(cmd string) bool {
	switch cmd {
	case "endcode", "endverbatim", "enddot", "endmsc",
		"endhtmlonly", "endlatexonly", "endxmlonly",
		"endmanonly", "endrtfonly", "enddocbookonly":
		return true
	}
	return false
}

// translatableLine captures one source comment line that contributed
// translatable text. text is the extracted prose; prefix is the
// command marker stripped off so the writer can reattach it
// (e.g. "\brief ", "\param x ", "\addtogroup mygroup ", or "" for
// plain prose lines).
type translatableLine struct {
	text   string
	prefix string
}

// extractTranslatable processes comment text lines and returns groups of translatable text.
// It handles Doxygen commands, code blocks, and inline commands.
//
// Each group is a list of translatableLine entries — one per source
// comment line that contributed prose. The prefix on each entry holds
// the command marker that the reader stripped to surface clean text
// (e.g. "\brief ", "\param x "); the writer reattaches it on
// roundtrip so the output preserves the original command markers
// rather than collapsing them into bare prose.
func (r *Reader) extractTranslatable(textLines []string) [][]translatableLine {
	var groups [][]translatableLine
	var current []translatableLine
	inExclude := false

	for _, line := range textLines {
		trimmed := strings.TrimSpace(line)

		// Blank line in the comment body — mirror okapi's
		// BLANK_LINES paragraph splitter and flush the current group
		// so subsequent prose joining (joinProseLines) doesn't bleed
		// across paragraphs.
		if trimmed == "" {
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
			}
			continue
		}

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

		// Handle non-translatable commands: entire line is metadata
		if cmd != "" && nonTranslatableCommands[cmd] {
			// These commands have arguments that are not translatable.
			// But some like @addtogroup have a description after the group name.
			if cmd == "addtogroup" || cmd == "defgroup" {
				desc, prefix := r.extractDescAndPrefix(trimmed, cmd, 1)
				if desc != "" {
					current = append(current, translatableLine{text: desc, prefix: prefix})
				}
			}
			// Non-translatable command lines act as structural barriers
			// — okapi never absorbs surrounding prose across them. Flush
			// the current paragraph so joinProseLines doesn't bleed
			// adjacent prose lines together. Examples: `\skip`, `\until`,
			// `\skipline`, `\line`, `\dontinclude` between prose lines in
			// the example block of special_commands.h.
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
			}
			continue
		}

		// Handle section-header commands: `\section ID Title`,
		// `\page ID Title`, `\subpage ID Text`, `\ref ID Text`,
		// `\xrefitem KEY Heading "List Title"`. Per okapi's
		// doxygenConfiguration.yml the leading WORD args (ID/KEY) are
		// non-translatable; the trailing text/title IS translatable.
		// Each header is its own paragraph — flush before AND after
		// so joinProseLines doesn't absorb surrounding prose into the
		// section header line.
		if cmd != "" {
			if skipArgs, ok := sectionHeaderCommands[cmd]; ok {
				desc, prefix := r.extractDescAndPrefix(trimmed, cmd, skipArgs)
				if len(current) > 0 {
					groups = append(groups, current)
					current = nil
				}
				if desc != "" {
					groups = append(groups, []translatableLine{{text: desc, prefix: prefix}})
				}
				continue
			}
		}

		// Handle translatable description commands: @param name description
		if cmd != "" && translatableDescCommands[cmd] {
			desc, prefix := r.extractDescAndPrefix(trimmed, cmd, r.commandArgCount(cmd))
			if desc != "" {
				// Flush previous group and start new one for this command
				if len(current) > 0 {
					groups = append(groups, current)
					current = nil
				}
				current = append(current, translatableLine{text: desc, prefix: prefix})
				continue
			}
			continue
		}

		// Handle paragraph-break commands (\if, \else, \elseif, \ifnot,
		// \cond): the line opens a new conditional / language block.
		// Flush the current paragraph before AND after so the marker
		// stands as its own paragraph and adjacent prose lines don't
		// absorb across it.
		if cmd != "" && paragraphBreakCommands[cmd] {
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
			}
			processed := r.processInlineCommands(trimmed)
			if processed != "" {
				groups = append(groups, []translatableLine{{text: processed}})
			}
			continue
		}

		// Handle paragraph-close commands (\endif, \endcond) at line
		// start: okapi merges the marker into the preceding prose line
		// (e.g. ` * \endif` after ` *    Only included if Cond1 is set.`
		// becomes one ` * Only included if Cond1 is set. \endif`), then
		// closes the paragraph so the next line starts fresh. Treat
		// the marker as continuation prose for joinProseLines (empty
		// prefix, falls into prev) AND flush so the next line opens
		// a new group.
		if cmd != "" && (cmd == "endif" || cmd == "endcond") {
			processed := r.processInlineCommands(trimmed)
			if processed != "" {
				current = append(current, translatableLine{text: processed})
			}
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
			}
			continue
		}

		// Handle inline commands by stripping the command prefix
		processed := r.processInlineCommands(trimmed)
		if processed != "" {
			current = append(current, translatableLine{text: processed})
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
			// Recognise the Doxygen group-open / group-close markers
			// (`@{`, `@}`, `\{`, `\}`) as their own commands so they
			// can be flagged non-translatable. Without this the `{`
			// / `}` byte slips past isCommandChar (alphabetic-only)
			// and the line falls through to the prose-extract path,
			// where joinProseLines then absorbs `@{` into the previous
			// paragraph (special_commands.h fixture line 7-8).
			if len(rest) > 0 && (rest[0] == '{' || rest[0] == '}') {
				return rest[:1]
			}
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
// `[` / `]` are NOT included: a `@param[in]` / `@param[out]` qualifier is a
// SEPARATE option block from the command name (`param`), not part of it.
// Including them would fold the qualifier into the command name and bypass
// translatableDescCommands' per-command handling.
func isCommandChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '$'
}

// extractDescAndPrefix returns the description text after the command
// and its N arguments, plus the prefix substring (command marker +
// args + trailing whitespace) that the writer reattaches on roundtrip
// so command markers like "\brief", "\param x", "\addtogroup mygroup"
// survive in the output instead of being silently dropped.
//
// Whitespace inside the prefix (between command, args, and the
// description) is collapsed to single spaces, mirroring okapi's
// WhitespaceAdjustingEventBuilder.collapseWhitespace which folds
// `\s+` between non-whitespace tokens to a single space. Without
// this, `\brief     Pretty` would round-trip with the original
// 5 spaces while okapi emits `\brief Pretty`.
func (r *Reader) extractDescAndPrefix(line, cmd string, skipArgs int) (string, string) {
	desc := r.extractDescriptionAfterCommand(line, cmd, skipArgs)
	if desc == "" {
		return "", ""
	}
	// Slice the prefix as the substring of `line` before `desc`.
	idx := strings.LastIndex(line, desc)
	if idx <= 0 {
		return desc, ""
	}
	prefix := line[:idx]
	prefix = collapseInteriorWhitespace(prefix)
	// Trailing whitespace on the prefix (the gap between the command
	// and the description) collapses to a single space too — okapi
	// glues `\brief Pretty` regardless of how many spaces were between
	// `\brief` and `Pretty` in the source. The collapseInteriorWhitespace
	// pattern misses this case because there's no non-whitespace char
	// to match after the gap (the description string isn't part of the
	// prefix).
	if trimmed := strings.TrimRight(prefix, " \t"); trimmed != prefix && trimmed != "" {
		prefix = trimmed + " "
	}
	return desc, prefix
}

// collapseInteriorWhitespace replaces `\s+` runs surrounded by non-
// whitespace with a single space, leaving leading and trailing
// whitespace intact. Matches okapi's WHITESPACE_COLLAPSE pattern
// `(?<=\S)\s+(?=\S)` for prefix normalization.
var interiorWhitespacePattern = regexp.MustCompile(`(\S)[ \t]+(\S)`)

func collapseInteriorWhitespace(s string) string {
	for {
		next := interiorWhitespacePattern.ReplaceAllString(s, "$1 $2")
		if next == s {
			return s
		}
		s = next
	}
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
	// Doxygen accepts an `[option]` qualifier directly after the
	// command name (no whitespace in between), e.g. `@param[in]`,
	// `@param[out]`, `@param[in,out]`. The qualifier is part of the
	// command marker — not an argument — so skip it before counting
	// args. Without this, `@param[out] dest` skips `[out]` as the
	// "param name" arg and leaves `dest <description>` as the desc,
	// which then folds the parameter name into the translatable text.
	if len(rest) > 0 && rest[0] == '[' {
		if endBracket := strings.IndexByte(rest, ']'); endBracket >= 0 {
			rest = rest[endBracket+1:]
		}
	}
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

	// For \image, the description is only the quoted caption — any
	// trailing size indicator (e.g. width=10cm) is non-translatable.
	// Extract just the text inside the quotes so the writer preserves
	// the size parameter verbatim.
	if quotedDescCommands[cmd] && len(rest) > 0 && rest[0] == '"' {
		endQuote := strings.Index(rest[1:], "\"")
		if endQuote >= 0 {
			return rest[1 : endQuote+1]
		}
		return ""
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

// processInlineCommands handles inline Doxygen commands (@e, @a, @b,
// \c, \p, \em). The reader keeps the markers attached to the
// extracted text rather than stripping them, so the writer can
// roundtrip the source verbatim and translators can decide whether
// to keep, drop, or relocate the inline emphasis. The spec
// `inline_doxygen_commands` feature only requires the marked-up
// word to remain extractable (it asserts containment, not equality);
// keeping the markers also preserves the original byte sequence on
// roundtrip.
func (r *Reader) processInlineCommands(line string) string {
	return line
}

// argTakingCommandsPattern lists the Doxygen commands whose first
// argument is the parameter NAME (a code identifier, never translated).
// Matched first so the regex engine eats `\param x` / `\throws E` /
// `\retval RC` / `\cond LABEL` / `\if LABEL` etc. as one inline-code
// chunk — okapi appends the argument to the placeholder Code (see
// DoxygenFilter.parseParameters → code.append(extractor.parameter()))
// and pseudo-translation must not touch the identifier.
//
// `\cond LABEL`, `\if LABEL`, `\ifnot LABEL` carry a section-label
// WORD that okapi marks `translatable: false` in
// doxygenConfiguration.yml. Without including them here the labels
// (`TEST`, `DEV`, `Cond1`, …) get pseudo-translated alongside the
// surrounding prose — visible in special_commands.h's `@cond TEST`
// blocks.
//
// `\ref` is treated as an inline placeholder with one mandatory WORD
// (anchor name) followed by an optional quoted text. Match `\ref name`
// here so the anchor name is protected from pseudo-translation; any
// trailing `"display text"` stays as TextRun and translates normally.
//
// `\subpage` follows the same shape and frequently appears inside list
// items (`- \subpage intro`) where the line-leading character is `-`,
// not the command marker — extractCommand can't detect it as a section
// header in that position, so the inline pattern is the right place to
// protect the anchor name.
const argTakingCommandsPattern = `[\\@](?:param|tparam|throws?|exception|retval|cond|if|ifnot|elseif|ref|subpage|page|section|subsection|subsubsection|paragraph)\s+\w+`

// inlineCodePattern matches Doxygen special commands (\cmd, @cmd) and
// HTML-like tags (<tag>, </tag>) that appear inline within translatable
// text. Mirrors okapi DoxygenPatterns.DOXYGEN_COMMAND so the same
// substrings get protected as inline codes during pseudo-translation.
//
//	[\\@](?:param|...)\s+\w+            — \param x, \throws E (eat arg)
//	[\\@]\w+(?:[\[\(\{].*?[\]\)\}])?   — \cmd, @cmd, \cmd[arg], @cmd{x}
//	</?[a-zA-Z][^>]*>                   — <tag>, </tag>, <tag attr="...">
//	@[{}]                               — @{ , @} (group toggles)
//
// The pattern is applied to translatable lines; matches become
// PlaceholderRuns whose Data carries the original substring verbatim,
// while non-matching segments stay as TextRuns. Pseudo-translation only
// substitutes TextRun characters, so the protected commands round-trip
// byte-for-byte.
var inlineCodePattern = regexp.MustCompile(
	argTakingCommandsPattern +
		`|[\\@]\w+(?:[\[\(\{][^\]\)\}]*[\]\)\}])?` +
		`|</?[a-zA-Z][^>]*>` +
		`|@[{}]`)

// tokenizeInlineCodes splits a translatable text line into a sequence
// of Runs: TextRuns for prose, PlaceholderRuns for inline Doxygen
// commands (\cmd / @cmd) and HTML tags. The returned runs flatten back
// to the input via concatenating each run's text/Data, so the writer
// can reconstruct the original byte sequence.
//
// idStart is the next placeholder ID to allocate; the function returns
// the next free ID alongside the runs so callers can keep IDs unique
// across an entire Block's text.
func tokenizeInlineCodes(line string, idStart int) ([]model.Run, int) {
	if line == "" {
		return nil, idStart
	}
	matches := inlineCodePattern.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return []model.Run{{Text: &model.TextRun{Text: line}}}, idStart
	}
	out := make([]model.Run, 0, 2*len(matches)+1)
	cursor := 0
	id := idStart
	for _, m := range matches {
		match := line[m[0]:m[1]]
		// Mirror okapi DoxygenFilter.parseCommentChunk lines 432-438 —
		// the regex matches any `\word` / `@word` substring, but okapi
		// only protects KNOWN commands. Unknown commands like
		// `\invalid` (sample.h) get treated as plain text and pseudo-
		// translated. Without this check, native protects every
		// `\word` and the unknown-command bytes round-trip verbatim,
		// diverging from okapi by ~7 bytes per occurrence.
		if !isProtectedInlineMatch(match) {
			continue
		}
		if m[0] > cursor {
			out = append(out, model.Run{Text: &model.TextRun{Text: line[cursor:m[0]]}})
		}
		out = append(out, model.Run{Ph: &model.PlaceholderRun{
			ID:   strconv.Itoa(id),
			Type: "doxygen:cmd",
			Data: match,
		}})
		id++
		cursor = m[1]
	}
	if cursor < len(line) {
		out = append(out, model.Run{Text: &model.TextRun{Text: line[cursor:]}})
	}
	if len(out) == 0 {
		// Every regex match was an unknown command — emit the whole
		// line as a single TextRun (the unknown commands flow through
		// as plain text and get pseudo-translated like everything else).
		return []model.Run{{Text: &model.TextRun{Text: line}}}, idStart
	}
	return out, id
}

// isProtectedInlineMatch reports whether a regex match from
// inlineCodePattern names a known Doxygen command (or HTML tag, or
// group toggle) that should round-trip as a verbatim placeholder. For
// unknown commands like `\invalid`, okapi falls back to treating the
// substring as plain text — this function mirrors that gate.
func isProtectedInlineMatch(match string) bool {
	if len(match) == 0 {
		return false
	}
	// HTML-style tag — keep protecting these, okapi treats them as
	// inline placeholders unconditionally via the same regex branch.
	if match[0] == '<' {
		return true
	}
	// Group toggles `@{` / `@}` — always protect, okapi has them as
	// dedicated structural markers.
	if match == "@{" || match == "@}" || match == "\\{" || match == "\\}" {
		return true
	}
	// `\cmd` / `@cmd` — extract the command name (without the leading
	// `\`/`@` and any trailing `[arg]` / `{arg}` / `(arg)` suffix) and
	// check it against the known-Doxygen-command set.
	if match[0] != '\\' && match[0] != '@' {
		return false
	}
	body := match[1:]
	// Trim any bracket-suffix so `\cmd{arg}` is keyed on `cmd`.
	for i, b := range body {
		if b == '[' || b == '(' || b == '{' {
			body = body[:i]
			break
		}
	}
	// arg-taking commands are formatted as e.g. `\param x` or
	// `\throws E` (the regex captures `\param x` as one token via
	// argTakingCommandsPattern). Strip everything after the first
	// whitespace so the lookup keys on the command name only.
	if idx := strings.IndexAny(body, " \t"); idx >= 0 {
		body = body[:idx]
	}
	return knownDoxygenCommands[body]
}

// knownDoxygenCommands enumerates Doxygen command names that okapi's
// doxygenConfiguration.yml registers (from
// okapi/filters/doxygen/src/main/resources/.../doxygenConfiguration.yml).
// Used by isProtectedInlineMatch to decide whether a `\word` /
// `@word` substring should be protected as a verbatim placeholder
// (known) or pseudo-translated as plain text (unknown). Mirrors
// okapi's DoxygenFilter.parseCommentChunk fallback.
var knownDoxygenCommands = map[string]bool{
	"a": true, "addindex": true, "addtogroup": true, "anchor": true,
	"arg": true, "attention": true, "author": true, "authors": true,
	"b": true, "blockquote": true, "body": true, "br": true,
	"brief": true, "bug": true, "c": true, "callergraph": true,
	"callgraph": true, "caption": true, "category": true, "center": true,
	"cite": true, "class": true, "code": true, "cond": true,
	"copybrief": true, "copydetails": true, "copydoc": true,
	"copyright": true, "date": true, "dd": true, "def": true,
	"defgroup": true, "deprecated": true, "description": true,
	"details": true, "dfn": true, "dir": true, "div": true,
	"dl": true, "dontinclude": true, "dot": true, "dotfile": true,
	"dt": true, "e": true, "else": true, "elseif": true, "em": true,
	"endcode": true, "endcond": true, "enddot": true, "endhtmlonly": true,
	"endif": true, "endinternal": true, "endlatexonly": true,
	"endlink": true, "endmanonly": true, "endmsc": true,
	"endrtfonly": true, "endverbatim": true, "endxmlonly": true,
	"enum": true, "example": true, "exception": true, "extends": true,
	"file": true, "fn": true, "form": true, "headerfile": true,
	"hideinitializer": true, "hr": true, "htmlinclude": true,
	"htmlonly": true, "i": true, "if": true, "ifnot": true,
	"image": true, "img": true, "implements": true, "include": true,
	"includelineno": true, "ingroup": true, "inheritdoc": true,
	"input": true, "interface": true, "internal": true, "invariant": true,
	"item": true, "kbd": true, "latexonly": true, "li": true,
	"line": true, "link": true, "list": true, "listheader": true,
	"mainpage": true, "manonly": true, "memberof": true, "meta": true,
	"msc": true, "mscfile": true, "multicol": true, "n": true,
	"name": true, "namespace": true, "nosubgrouping": true, "note": true,
	"ol": true, "overload": true, "p": true, "package": true,
	"page": true, "par": true, "para": true, "paragraph": true,
	"param": true, "paramref": true, "permission": true,
	"post": true, "pre": true, "private": true, "privatesection": true,
	"property": true, "protected": true, "protectedsection": true,
	"protocol": true, "public": true, "publicsection": true,
	"ref": true, "related": true, "relatedalso": true, "relates": true,
	"relatesalso": true, "remark": true, "remarks": true, "result": true,
	"return": true, "returns": true, "retval": true, "rtfonly": true,
	"sa": true, "section": true, "see": true, "seealso": true,
	"short": true, "showinitializer": true, "since": true, "skip": true,
	"skipline": true, "small": true, "snippet": true, "span": true,
	"strong": true, "struct": true, "sub": true, "subpage": true,
	"subsection": true, "subsubsection": true, "summary": true,
	"sup": true, "table": true, "tableofcontents": true, "td": true,
	"term": true, "test": true, "th": true, "throw": true,
	"throws": true, "todo": true, "tparam": true, "tr": true,
	"tt": true, "typedef": true, "typeparam": true, "typeparamref": true,
	"ul": true, "union": true, "until": true, "value": true,
	"var": true, "verbatim": true, "verbinclude": true, "version": true,
	"warning": true, "weakgroup": true, "xmlonly": true, "xrefitem": true,
}

// listItemPattern matches okapi's LIST_ITEM_PREFIX (`- `, `-# `,
// `+ `, `*# `, `1. `, etc.). When a translatable line starts with one
// of these markers it counts as a list item, and okapi never joins it
// with surrounding prose — the line stays on its own row.
var listItemPattern = regexp.MustCompile(`^(?:[-+*]#?|\d+\.)\s+`)

// isListItem reports whether a translatable text line opens with a
// Doxygen list-item marker.
//
// A lone `.` (optionally followed by whitespace) is Doxygen's
// explicit list-end marker — okapi tokenises it as its own row and
// never joins it into the surrounding paragraph. Without recognising
// it here, joinProseLines absorbs the `.` into the preceding list
// item (e.g. lists.h line 20 ` *     . ` gets folded into ` *     -
// sub sub item 2` and emerges as `sub sub item 2 . The dot above…`).
//
// HTML list items (`<li>`, `<li>foo`) are equally non-joinable. okapi
// tokenises HTML markup via the same LIST_ITEM_PREFIX_PATTERN
// alternation; lists.h's `<li>mouse events` rows would otherwise be
// glued into one long paragraph by joinProseLines.
func isListItem(s string) bool {
	if listItemPattern.MatchString(s) {
		return true
	}
	if isListEndMarker(s) {
		return true
	}
	return isHTMLListItem(s)
}

// isListEndMarker reports whether s is the doxygen list-end marker —
// a lone `.` with optional trailing whitespace (e.g. ".", ". ", ".\t").
func isListEndMarker(s string) bool {
	if s == "" || s[0] != '.' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}

// isHTMLListItem reports whether s opens with `<li>` (case-insensitive,
// optional attributes inside the tag).
func isHTMLListItem(s string) bool {
	if len(s) < 4 || s[0] != '<' {
		return false
	}
	// Match `<li` followed by whitespace, `>`, or `/`.
	if (s[1] == 'l' || s[1] == 'L') && (s[2] == 'i' || s[2] == 'I') {
		c := s[3]
		return c == '>' || c == ' ' || c == '\t' || c == '/'
	}
	return false
}

// joinProseLines mirrors okapi's "join consecutive plain-prose lines
// in a paragraph with a single space, padding to preserve the original
// line count" behaviour (DoxygenFilter.parsePlainText splits on
// BLANK_LINES_PATTERN, then collapses the remaining whitespace inside
// each paragraph). Within one paragraph, the first translatable line
// is the anchor — be it plain prose, a list item, or a command-anchor
// like `\param a description` / `\section sec title`. Each anchor
// absorbs the trailing plain-prose lines (those whose prefix is empty
// AND that don't open with a list marker) into its slot with single-
// space separators, leaving the absorbed slots empty. The writer then
// emits a canonical comment-marker line (no trailing prose) for each
// empty slot to preserve the source's line count.
//
// Lines whose body opens with a list-item marker (`- `, `-# `,
// `1. `, …) are treated as their own rows — okapi's tokenizer
// (LIST_ITEM_PREFIX_PATTERN) chunks list items independently from the
// surrounding paragraph, so joining would erase the list structure.
func joinProseLines(texts, prefixes []string) ([]string, []string) {
	if len(texts) <= 1 {
		return texts, prefixes
	}
	if len(texts) != len(prefixes) {
		// Defensive: prefix slice always tracks texts 1:1 in
		// emitCommentBlock; bail out if invariant violated.
		return texts, prefixes
	}
	out := make([]string, len(texts))
	outPx := make([]string, len(prefixes))
	copy(out, texts)
	copy(outPx, prefixes)
	i := 0
	for i < len(out) {
		// Skip already-empty slots (left behind by a previous merge
		// pass when a paragraph is processed twice — defensive).
		if out[i] == "" {
			i++
			continue
		}
		// Doxygen's lone `.` list-end marker doesn't absorb following
		// prose — okapi keeps the `.` row on its own line and lets the
		// trailing prose start a fresh paragraph (lists.h L21
		// ` *     . Ţĥē ďōţ àƀōvē…` vs reference ` *     . ` followed
		// by separate ` *     Ţĥē ďōţ…`). Plain `-`/`-#`/`<li>` list
		// items still absorb their continuation prose (e.g. sample.h's
		// ` *  -# mouse click event\n *     More info about…` collapses
		// to a single row), so the gate is narrower than `isListItem`.
		if isListEndMarker(out[i]) {
			i++
			continue
		}
		j := i + 1
		// Continue absorbing trailing plain-prose lines until we hit
		// a non-prose marker (non-empty prefix), a blank, or a list
		// item (which starts its own row).
		for j < len(out) && outPx[j] == "" && out[j] != "" && !isListItem(out[j]) {
			out[i] = out[i] + " " + out[j]
			out[j] = ""
			j++
		}
		if j == i+1 {
			// No absorption — advance by 1 to keep making progress.
			i++
		} else {
			i = j
		}
	}
	return out, outPx
}

// tokenizedRunsForLines builds the Run sequence for a group's text by
// tokenizing each line with tokenizeInlineCodes and joining adjacent
// lines with a literal "\n" TextRun. This is the inline-code-aware
// counterpart of strings.Join(texts, "\n") used previously.
func tokenizedRunsForLines(lines []string) []model.Run {
	var runs []model.Run
	id := 1
	for i, line := range lines {
		if i > 0 {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: "\n"}})
		}
		lineRuns, next := tokenizeInlineCodes(line, id)
		runs = append(runs, lineRuns...)
		id = next
	}
	return runs
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
