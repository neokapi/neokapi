package doxygen

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

// inlineCommands enumerates Doxygen commands that produce inline
// formatting (\e, \a, \b, \c, \p, \em). They mark up the next word.
// The reader keeps them in the extracted text so the writer can
// roundtrip the source verbatim — see processInlineCommands.
//
//nolint:unused // documents the recognised inline-command vocabulary
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
		idx := strings.Index(line, "///<")
		if idx < 0 {
			return false
		}
		before := strings.TrimSpace(line[:idx])
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
		text := strings.Join(texts, "\n")
		block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), text)
		block.Name = fmt.Sprintf("comment.%d", *blockCounter)
		block.Properties["style"] = cb.style
		block.Properties["raw"] = strings.Join(cb.rawLines, "\n")
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
	if cb.style == "javadoc" || cb.style == "qt" {
		return r.buildBlockLayout(cb)
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
			// Blank `///` / `//!` separator line — pass through.
			entries = append(entries, "S:"+raw)
			hasS = true
			continue
		}
		// Match parseLineComments: only non-empty stripped lines
		// contribute to textLines. Look up whether the current
		// textCursor entry is translatable.
		if tl, isTrans := textIdxToTL[textCursor]; isTrans {
			entries = append(entries, "T:"+tl.prefix)
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
			if cb.style == "qt" {
				t = strings.TrimPrefix(t, "/*!")
			} else {
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
		entries = append(entries, "B:"+prefix)
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
		if cmd == "image" {
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
				desc, prefix := r.extractDescAndPrefix(trimmed, cmd, 1)
				if desc != "" {
					current = append(current, translatableLine{text: desc, prefix: prefix})
				}
			}
			continue
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

// extractDescAndPrefix returns the description text after the command
// and its N arguments, plus the prefix substring (command marker +
// args + trailing whitespace) that the writer reattaches on roundtrip
// so command markers like "\brief", "\param x", "\addtogroup mygroup"
// survive in the output instead of being silently dropped.
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
	return desc, line[:idx]
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
