package mif

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

// Reader implements DataFormatReader for MIF (Maker Interchange Format) files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new MIF reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "mif",
			FormatDisplayName: "Adobe FrameMaker MIF",
			FormatMimeType:    "application/x-mif",
			FormatExtensions:  []string{".mif"},
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
		MIMETypes:  []string{"application/x-mif", "application/vnd.mif"},
		Extensions: []string{".mif"},
		Sniff: func(data []byte) bool {
			return len(data) >= 9 && string(data[:9]) == "<MIFFile "
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("mif: nil document or reader")
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

// mifStatement represents a parsed MIF statement.
type mifStatement struct {
	tag      string
	value    string
	children []*mifStatement
	raw      string // Original raw text for non-translatable parts.
}

// stringRef records the byte position of a String value and its block association.
type stringRef struct {
	startOffset int // byte offset of the String value content start (after backtick)
	endOffset   int // byte offset of the String value content end (before quote)
	blockIdx    int // which block (0-based)
	stringIdx   int // which string within the block (0-based)
}

// alwaysSkipTags are top-level MIF tags whose content is always non-translatable.
var alwaysSkipTags = map[string]bool{
	"Units":                true,
	"ColorCatalog":         true,
	"ConditionCatalog":     true,
	"BoolCondCatalog":      true,
	"CombinedFontCatalog":  true,
	"FontCatalog":          true,
	"RulingCatalog":        true,
	"TblCatalog":           true,
	"Views":                true,
	"Document":             true,
	"BookComponent":        true,
	"InitialAutoNums":      true,
	"Dictionary":           true,
	"PgfCatalog":           true,
	"ElementDefCatalog":    true,
	"FmtChangeListCatalog": true,
	"DefAttrValuesCatalog": true,
	"AttrCondExprCatalog":  true,
	"AFrames":              true,
}

// skipTag returns true if the tag should be skipped based on config.
func (r *Reader) skipTag(tag string) bool {
	if alwaysSkipTags[tag] {
		return true
	}
	switch tag {
	case "MasterPage":
		return !r.cfg.ExtractMasterPages
	case "ReferencePage":
		return !r.cfg.ExtractReferencePages
	case "Page":
		return !r.cfg.ExtractBodyPages
	case "VariableFormats":
		return !r.cfg.ExtractVariables
	case "XRefFormats":
		return !r.cfg.ExtractReferenceFormats
	}
	return false
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "mif",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-mif",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitErr(ctx, ch, fmt.Errorf("mif: read error: %w", err))
		return
	}
	rawText := string(data)

	stmts := parseMIF(rawText)
	r.emitStatements(ctx, ch, stmts)

	// Build skeleton if needed
	if r.skeletonStore != nil {
		refs := r.findStringPositions(rawText, stmts)
		if len(refs) > 0 {
			skelPos := 0
			for _, sr := range refs {
				if sr.startOffset > skelPos {
					r.skelText(rawText[skelPos:sr.startOffset])
				}
				refID := fmt.Sprintf("%d:%d", sr.blockIdx, sr.stringIdx)
				r.skelRef(refID)
				skelPos = sr.endOffset
			}
			if skelPos < len(rawText) {
				r.skelText(rawText[skelPos:])
			}
			r.skelFlush()
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// findStringPositions scans raw MIF content for <String `...'> patterns and associates
// them with block indices based on the parsed statement tree.
func (r *Reader) findStringPositions(rawText string, stmts []*mifStatement) []stringRef {
	// First, figure out which paragraphs are translatable and in which order
	// (matching the same logic as emitStatements/processContainer).
	var translatableParas []*mifStatement
	for _, stmt := range stmts {
		if r.skipTag(stmt.tag) || stmt.tag == "MIFFile" {
			continue
		}
		if stmt.tag == "TextFlow" || stmt.tag == "Tbls" || stmt.tag == "Notes" {
			collectTranslatableParas(stmt, &translatableParas)
		}
	}

	// For each translatable para, find its String children and count them
	type paraStringInfo struct {
		blockIdx int
		strings  []string // the string values in order
	}
	var paraInfos []paraStringInfo
	blockIdx := 0
	for _, para := range translatableParas {
		text := extractParaText(para)
		if strings.TrimSpace(text) == "" {
			continue
		}
		var strs []string
		for _, child := range para.children {
			if child.tag == "ParaLine" {
				for _, lc := range child.children {
					if lc.tag == "String" {
						strs = append(strs, lc.value)
					}
				}
			}
		}
		paraInfos = append(paraInfos, paraStringInfo{blockIdx: blockIdx, strings: strs})
		blockIdx++
	}

	// Now scan the raw text for <String `...'> patterns and match them to the para infos
	var refs []stringRef
	paraIdx := 0
	stringInParaIdx := 0
	searchFrom := 0

	for paraIdx < len(paraInfos) {
		pi := paraInfos[paraIdx]
		if stringInParaIdx >= len(pi.strings) {
			paraIdx++
			stringInParaIdx = 0
			continue
		}

		expectedVal := pi.strings[stringInParaIdx]
		// Find the next <String `expectedVal'> in rawText starting from searchFrom
		pattern := "<String `" + escapeMIFForSearch(expectedVal) + "'>"
		idx := strings.Index(rawText[searchFrom:], pattern)
		if idx < 0 {
			// Try without exact match (shouldn't happen with well-formed MIF)
			stringInParaIdx++
			continue
		}

		absIdx := searchFrom + idx
		// The value starts after "<String `" (9 bytes for "<String `")
		valStart := absIdx + len("<String `")
		valEnd := valStart + len(escapeMIFForSearch(expectedVal))

		refs = append(refs, stringRef{
			startOffset: valStart,
			endOffset:   valEnd,
			blockIdx:    pi.blockIdx,
			stringIdx:   stringInParaIdx,
		})

		searchFrom = valEnd
		stringInParaIdx++
	}

	return refs
}

// escapeMIFForSearch converts a parsed string value back to its MIF-escaped form for searching.
func escapeMIFForSearch(s string) string {
	// The value was unquoted by unquoteMIF, so the raw form in the file is the original.
	// Since parseMIF uses unquoteMIF which just strips backtick/quote delimiters,
	// the value already has MIF escapes resolved... but actually unquoteMIF only strips
	// the delimiters, it doesn't unescape. So the value should match raw content.
	return s
}

// collectTranslatableParas collects Para statements that contain translatable text.
func collectTranslatableParas(stmt *mifStatement, result *[]*mifStatement) {
	for _, child := range stmt.children {
		if child.tag == "Para" {
			text := extractParaText(child)
			if strings.TrimSpace(text) != "" {
				*result = append(*result, child)
			}
		} else if child.tag == "TextFlow" || child.tag == "Notes" || child.tag == "Tbls" ||
			child.tag == "Cell" || child.tag == "CellContent" || child.tag == "Row" ||
			child.tag == "TblBody" || child.tag == "Tbl" {
			collectTranslatableParas(child, result)
		}
	}
}

func (r *Reader) emitStatements(ctx context.Context, ch chan<- model.PartResult, stmts []*mifStatement) {
	blockCounter := 0
	dataCounter := 0

	for _, stmt := range stmts {
		if r.skipTag(stmt.tag) {
			// Emit as non-translatable data.
			dataCounter++
			d := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "mif." + stmt.tag,
				Properties: map[string]string{
					"tag": stmt.tag,
					"raw": stmt.raw,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
				return
			}
			continue
		}

		if stmt.tag == "MIFFile" {
			// Emit version line as data.
			dataCounter++
			d := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "mif.version",
				Properties: map[string]string{
					"tag":     "MIFFile",
					"version": stmt.value,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
				return
			}
			continue
		}

		if stmt.tag == "TextFlow" || stmt.tag == "Tbls" || stmt.tag == "Notes" {
			// Process translatable content inside these containers.
			blockCounter, dataCounter = r.processContainer(ctx, ch, stmt, blockCounter, dataCounter)
			continue
		}

		// Default: emit as data.
		dataCounter++
		d := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: "mif." + stmt.tag,
			Properties: map[string]string{
				"tag": stmt.tag,
				"raw": stmt.raw,
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
			return
		}
	}
}

// processContainer recursively processes a MIF container for translatable strings.
func (r *Reader) processContainer(ctx context.Context, ch chan<- model.PartResult, stmt *mifStatement, blockCounter, dataCounter int) (int, int) {
	for _, child := range stmt.children {
		if child.tag == "Para" {
			text := extractParaTextImpl(child, r.cfg.ExtractHardReturnsAsText)
			if strings.TrimSpace(text) != "" {
				blockCounter++
				block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
				block.Name = fmt.Sprintf("para.%d", blockCounter)

				// Extract paragraph tag if present.
				for _, gc := range child.children {
					if gc.tag == "PgfTag" {
						block.Properties["pgf_tag"] = gc.value
						break
					}
				}

				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return blockCounter, dataCounter
				}
			}
		} else if child.tag == "TextFlow" || child.tag == "Notes" || child.tag == "Tbls" || child.tag == "Cell" || child.tag == "CellContent" || child.tag == "Row" || child.tag == "TblBody" || child.tag == "Tbl" {
			blockCounter, dataCounter = r.processContainer(ctx, ch, child, blockCounter, dataCounter)
		}
	}
	return blockCounter, dataCounter
}

// extractParaText extracts translatable text from a Para statement.
// This standalone version always treats hard returns as text (used by findStringPositions
// and collectTranslatableParas where config doesn't affect presence/absence of text).
func extractParaText(para *mifStatement) string {
	return extractParaTextImpl(para, true)
}

// extractParaTextImpl extracts translatable text with configurable hard return handling.
func extractParaTextImpl(para *mifStatement, hardReturnsAsText bool) string {
	var texts []string
	for _, child := range para.children {
		if child.tag == "ParaLine" {
			for _, lc := range child.children {
				switch lc.tag {
				case "String":
					texts = append(texts, lc.value)
				case "Char":
					switch lc.value {
					case "HardReturn":
						if hardReturnsAsText {
							texts = append(texts, "\n")
						}
					case "Tab":
						texts = append(texts, "\t")
					case "HardSpace":
						texts = append(texts, "\u00A0")
					case "SoftHyphen":
						texts = append(texts, "\u00AD")
					case "EnSpace":
						texts = append(texts, "\u2002")
					case "EmSpace":
						texts = append(texts, "\u2003")
					case "ThinSpace":
						texts = append(texts, "\u2009")
					}
				}
			}
		}
	}
	return strings.Join(texts, "")
}

// parseMIF parses a MIF document into a list of top-level statements.
//
// MIF lines come in three flavours:
//   - whole-line comment: a line whose first non-space char is `#`
//     ("# Options:", "# end of Color"); always trailing on a `>` line too
//     (e.g. " > # end of Color")
//   - single-line statement: "<TagName value>" optionally followed by
//     a "# trailing comment" (e.g. "<MIFFile 9.00> # Generated by FrameMaker")
//   - multi-line container: opens with "<TagName" and closes on a later
//     line whose first non-space char is `>` (followed by an optional
//     "# end of TagName" comment)
//
// The lexer here is line-oriented — it handles all three cases by
// trimming the trailing "# ..." comment before classifying the line.
func parseMIF(content string) []*mifStatement {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var stmts []*mifStatement
	var stack []*mifStatement
	var rawBuilder strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		// Whole-line comment ("# Options:", "# End of MIFFile") —
		// preserved as raw on the surrounding context but has no
		// structural meaning.
		if strings.HasPrefix(trimmed, "#") {
			if len(stack) > 0 {
				stack[len(stack)-1].raw += line + "\n"
			} else {
				rawBuilder.WriteString(line + "\n")
			}
			continue
		}

		// Strip a trailing "# comment" from the structural part before
		// classifying. MIF allows trailing comments after `>` on close
		// lines (" > # end of Color") and after the closing `>` of a
		// single-line statement ("<MIFFile 9.00> # Generated by …").
		structural := stripTrailingComment(trimmed)

		if structural == ">" {
			// End of current statement.
			if len(stack) > 0 {
				current := stack[len(stack)-1]
				current.raw += line + "\n"
				stack = stack[:len(stack)-1]
				if len(stack) > 0 {
					parent := stack[len(stack)-1]
					parent.children = append(parent.children, current)
					parent.raw += current.raw
				} else {
					stmts = append(stmts, current)
				}
			}
			continue
		}

		if strings.HasPrefix(structural, "<") {
			// Start of a new statement.
			tag, value := parseTagLine(structural)
			stmt := &mifStatement{
				tag:   tag,
				value: value,
				raw:   line + "\n",
			}

			// Check if this is a single-line statement (ends with >).
			// Heuristic: MIF closes a single-line statement with a `>`
			// at the end of the structural part. The earlier
			// `!strings.HasSuffix(structural, " >")` guard preserved
			// the multi-line opener case where the opening `<Tag` line
			// has no value and no children on the same line — but real
			// MIF never opens a container that way; container openers
			// look like "<TagName" (no trailing `>`) or "<TagName ".
			// Treat any trimmed-line ending in `>` as single-line.
			if strings.HasSuffix(structural, ">") {
				inner := structural[1 : len(structural)-1] // remove < and >
				tag, after, hasVal := strings.Cut(inner, " ")
				stmt.tag = tag
				if hasVal {
					stmt.value = unquoteMIF(after)
				}
				if len(stack) > 0 {
					parent := stack[len(stack)-1]
					parent.children = append(parent.children, stmt)
					parent.raw += line + "\n"
				} else {
					stmts = append(stmts, stmt)
				}
				continue
			}

			stack = append(stack, stmt)
			continue
		}

		// Non-statement line (comment or other content).
		if len(stack) > 0 {
			stack[len(stack)-1].raw += line + "\n"
		} else {
			rawBuilder.WriteString(line + "\n")
		}
	}

	// Flush any unclosed statements.
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, current)
			parent.raw += current.raw
		} else {
			stmts = append(stmts, current)
		}
	}

	return stmts
}

// parseTagLine parses a MIF line like "<TagName value" or "<TagName".
func parseTagLine(line string) (string, string) {
	// Remove leading '<'.
	line = strings.TrimPrefix(line, "<")
	// Remove trailing '>' if present (single-line statement).
	line = strings.TrimSuffix(line, ">")
	line = strings.TrimSpace(line)

	tag, after, hasVal := strings.Cut(line, " ")
	var value string
	if hasVal {
		value = unquoteMIF(strings.TrimSpace(after))
	}
	return tag, value
}

// stripTrailingComment trims a trailing "# …" MIF inline comment from a
// structural line, taking care to ignore `#` characters inside backtick-
// quoted string values (e.g. `<String `width: #ffffff'>`). Returns the
// structural prefix with trailing whitespace removed.
func stripTrailingComment(s string) string {
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '`':
			inQuote = true
		case '\'':
			if inQuote {
				inQuote = false
			}
		case '\\':
			// Skip escaped char inside or outside quotes — handles
			// "\\`" / "\\'" / "\\>" sequences emitted by MIFEncoder.
			if i+1 < len(s) {
				i++
			}
		case '#':
			if !inQuote {
				return strings.TrimRight(s[:i], " \t")
			}
		}
	}
	return s
}

// unquoteMIF removes MIF backtick-single-quote delimiters from a string value.
func unquoteMIF(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
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

func (r *Reader) emitErr(ctx context.Context, ch chan<- model.PartResult, err error) {
	select {
	case ch <- model.PartResult{Error: err}:
	case <-ctx.Done():
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
