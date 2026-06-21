package yaml

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
	yamlv3 "gopkg.in/yaml.v3"
)

// yamlLineRe extracts the 1-based line number yaml.v3 embeds in its parse-error
// messages ("yaml: line 3: …"). yaml.v3 exposes no struct field for the
// position, so RVM diagnostics best-effort it from the message.
var yamlLineRe = regexp.MustCompile(`line (\d+)`)

// Reader implements DataFormatReader for YAML files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
	eol           string       // detected dominant line ending ("\n" or "\r\n")
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new YAML reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "yaml",
			FormatDisplayName: "YAML",
			FormatMimeType:    "application/yaml",
			FormatExtensions:  []string{".yaml", ".yml"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// skelBytes appends raw bytes to the skeleton buffer if active.
func (r *Reader) skelBytes(b []byte) {
	if r.skeletonStore != nil {
		r.skelBuf.Write(b)
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

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/yaml", "text/yaml", "application/x-yaml"},
		Extensions: []string{".yaml", ".yml"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("yaml: nil document or reader")
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

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:         "doc1",
		Name:       r.Doc.URI,
		Format:     "yaml",
		Locale:     locale,
		Encoding:   r.Doc.Encoding,
		MimeType:   "application/yaml",
		Properties: make(map[string]string),
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Bound the whole-input read with the shared safeio byte budget so an
	// unbounded/oversized stream fails with a typed error (identical limit
	// across CLI/server/WASM — see core/safeio).
	content, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("yaml: reading: %w", err)}
		return
	}

	// Detect the source's dominant line-ending convention so the writer
	// can re-emit re-encoded (translated) multi-line scalars with the
	// same convention. This mirrors Okapi's YamlFilter, which records the
	// BOMNewlineEncodingDetector's newline type on open and replays it via
	// getEncoderManager().getLineBreak() in YamlSkeletonWriter — every
	// emitted line break follows the source's convention, never a mix.
	r.eol = detectDominantEOL(content)
	layer.Properties["yaml.eol"] = r.eol

	blockCounter := 0

	// Use a Decoder to support multi-document YAML (--- separators).
	decoder := yamlv3.NewDecoder(strings.NewReader(string(content)))

	// notes accumulates YAML comments in document order so they attach as
	// parity-safe NoteAnnotations to the adjacent translatable block.
	notes := &noteState{}

	if r.skeletonStore != nil {
		// Skeleton mode: collect translatable scalar byte ranges, then
		// build skeleton from raw bytes.
		lineOffsets := buildLineOffsets(content)
		var ranges []scalarRange

		for {
			var node yamlv3.Node
			if err := decoder.Decode(&node); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				r.addSyntaxDiagnostic(content, err)
				ch <- model.PartResult{Error: fmt.Errorf("yaml: parsing: %w", err)}
				return
			}
			r.collectScalarRanges(ctx, ch, &node, nil, &blockCounter, content, lineOffsets, &ranges, nil, false, notes)
		}

		// Build skeleton from raw bytes and collected ranges.
		r.buildSkeleton(content, ranges)
	} else {
		for {
			var node yamlv3.Node
			if err := decoder.Decode(&node); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				r.addSyntaxDiagnostic(content, err)
				ch <- model.PartResult{Error: fmt.Errorf("yaml: parsing: %w", err)}
				return
			}
			r.walkNode(ctx, ch, &node, nil, &blockCounter, nil, notes)
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// addSyntaxDiagnostic records an RVM structure.yaml-syntax diagnostic for a
// decode error, best-effort locating it from the line number yaml.v3 embeds in
// the message (doc-level when no line is parseable). No-op when validation is
// off, so the lenient path is byte-identical.
func (r *Reader) addSyntaxDiagnostic(content []byte, err error) {
	if r.ValidationMode() == format.ValidationOff {
		return
	}
	line := yamlErrorLine(err)
	d := format.Diagnostic{
		Severity: format.SeverityMajor,
		Category: "structure.yaml-syntax",
		Message:  err.Error(),
		Line:     line,
	}
	if line > 0 {
		off := lineStartOffset(content, line)
		d.ByteOffset = off
		d.Snippet = format.SnippetAround(content, off, 0)
	}
	r.AddDiagnostic(d)
}

// yamlErrorLine extracts the 1-based line from a yaml.v3 decode error, unwrapping
// *yaml.TypeError (which carries per-field messages) first. Returns 0 when no
// line is present.
func yamlErrorLine(err error) int {
	msg := err.Error()
	var te *yamlv3.TypeError
	if errors.As(err, &te) && len(te.Errors) > 0 {
		msg = te.Errors[0]
	}
	if m := yamlLineRe.FindStringSubmatch(msg); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// lineStartOffset returns the byte offset of the start of the 1-based line in
// content (clamped to len(content)).
func lineStartOffset(content []byte, line int) int {
	if line <= 1 {
		return 0
	}
	cur := 1
	for i, b := range content {
		if b == '\n' {
			cur++
			if cur == line {
				return i + 1
			}
		}
	}
	return len(content)
}

// scalarRange records the byte range of a translatable scalar in the raw YAML content.
type scalarRange struct {
	start   int    // byte offset of the scalar value representation (including quotes)
	end     int    // byte offset past the scalar
	blockID string // block ID (e.g. "tu1")
	style   yamlv3.Style
}

// noteState accumulates YAML comments in document order so they can be
// attached as NoteAnnotations to the adjacent translatable block.
//
// yaml.v3 records comments on three node fields: HeadComment (the full-line
// `# …` lines ABOVE a mapping entry, stored on the entry's KEY node — or
// directly on a sequence-item / document scalar), LineComment (the trailing
// inline `value # …` comment, stored on the value node), and FootComment
// (the full-line `# …` lines BELOW an entry). The reader previously never
// read any of these fields, so this context was dropped on ingestion (the
// raw `#` text still rides the skeleton, so round-trip was — and stays —
// byte-exact).
//
// Comments are markup that stays verbatim in the skeleton; the note is an
// ingestion-only copy of the comment prose. Annotations are NOT part of the
// parity canonical part stream, so this is parity-safe and needs no opt-out
// flag (treatment B.1 of #928).
type noteState struct {
	// pending holds head/standalone comments awaiting the next block, in
	// document order.
	pending []*model.NoteAnnotation
}

// queueComment cleans a raw yaml.v3 comment field and, if it carries prose,
// queues it to attach to the next block emitted in document order.
func (ns *noteState) queueComment(raw string) {
	if ns == nil {
		return
	}
	if n := newCommentNote(raw); n != nil {
		ns.pending = append(ns.pending, n)
	}
}

// attach drains the queued head/standalone comments onto block (in document
// order) and appends the block's own inline and foot comments. Callers must
// invoke this BEFORE emitting the block's Part so the channel never observes
// a block mutated after send.
func (ns *noteState) attach(block *model.Block, inline, foot string) {
	if ns == nil || block == nil {
		return
	}
	for _, n := range ns.pending {
		block.AddNote(n)
	}
	ns.pending = nil
	if n := newCommentNote(inline); n != nil {
		block.AddNote(n)
	}
	if n := newCommentNote(foot); n != nil {
		block.AddNote(n)
	}
}

// newCommentNote converts a raw yaml.v3 comment field (e.g.
// "# line one\n# line two") into a NoteAnnotation, returning nil for an
// empty or whitespace-only comment.
func newCommentNote(raw string) *model.NoteAnnotation {
	text := cleanComment(raw)
	if text == "" {
		return nil
	}
	return &model.NoteAnnotation{Text: text, From: "yaml"}
}

// cleanComment strips the leading `#` marker and a single following space
// from each line of a raw yaml.v3 comment field and drops blank lines,
// yielding the comment prose. Multi-line comments are rejoined with "\n".
func cleanComment(raw string) string {
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "#")
		trimmed = strings.TrimPrefix(trimmed, " ")
		cleaned = append(cleaned, trimmed)
	}
	return strings.Join(cleaned, "\n")
}

// detectDominantEOL inspects the raw content and returns the source's
// dominant line-ending convention: "\r\n" when CRLF breaks outnumber
// bare LF breaks, otherwise "\n". A file with no line breaks (or pure
// LF) yields "\n", so the LF-source common case is never rewritten.
//
// This mirrors Okapi's BOMNewlineEncodingDetector.getNewlineType(),
// which classifies the whole document by the first / dominant break and
// then YamlSkeletonWriter replays that single convention on every line.
func detectDominantEOL(data []byte) string {
	crlf := 0
	lf := 0
	for i := range data {
		if data[i] == '\n' {
			if i > 0 && data[i-1] == '\r' {
				crlf++
			} else {
				lf++
			}
		}
	}
	if crlf > lf {
		return "\r\n"
	}
	return "\n"
}

// buildLineOffsets returns a slice where lineOffsets[i] is the byte offset of
// the start of line i+1 (1-based, matching yaml.v3 convention).
func buildLineOffsets(data []byte) []int {
	offsets := []int{0} // line 1 starts at offset 0
	for i, b := range data {
		if b == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// lineColToOffset converts 1-based line and column to a byte offset.
func lineColToOffset(lineOffsets []int, line, col int) int {
	if line < 1 || line > len(lineOffsets) {
		return -1
	}
	return lineOffsets[line-1] + col - 1
}

// collectScalarRanges walks the yaml.v3 node tree and collects translatable
// scalar byte ranges while also emitting Part events to the channel. This
// mirrors walkNode but additionally records byte positions for skeleton
// construction.
//
// `visiting` tracks alias targets currently on the recursion stack so
// self-referential anchors (e.g. snakeyaml's beanring fixtures where a
// mapping's value aliases back to its own root) terminate instead of
// looping forever.
func (r *Reader) collectScalarRanges(ctx context.Context, ch chan<- model.PartResult,
	node *yamlv3.Node, path []string, blockCounter *int,
	content []byte, lineOffsets []int, ranges *[]scalarRange,
	visiting map[*yamlv3.Node]bool, insideAlias bool, notes *noteState) {

	switch node.Kind {
	case yamlv3.DocumentNode:
		for _, child := range node.Content {
			r.collectScalarRanges(ctx, ch, child, path, blockCounter, content, lineOffsets, ranges, visiting, insideAlias, notes)
		}

	case yamlv3.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := keyNode.Value
			newPath := append(append([]string{}, path...), key)
			notes.queueComment(keyNode.HeadComment)
			r.collectScalarRanges(ctx, ch, valNode, newPath, blockCounter, content, lineOffsets, ranges, visiting, insideAlias, notes)
			notes.queueComment(keyNode.LineComment)
			notes.queueComment(keyNode.FootComment)
		}

	case yamlv3.SequenceNode:
		for i, child := range node.Content {
			indexPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i))
			r.collectScalarRanges(ctx, ch, child, indexPath, blockCounter, content, lineOffsets, ranges, visiting, insideAlias, notes)
		}

	case yamlv3.ScalarNode:
		// Scalars reached via alias resolution share their source bytes
		// with the original anchor target — we must NOT record a second
		// scalarRange for them, or buildSkeleton would emit the anchor's
		// surrounding skeleton text twice (duplicate keys/values inline)
		// while the alias position (`*id`) stays untouched. Emit the
		// model.Block so callers see the value, but skip the range.
		r.collectScalarRange(ctx, ch, node, path, blockCounter, content, lineOffsets, ranges, insideAlias, notes)

	case yamlv3.AliasNode:
		if node.Alias == nil || visiting[node.Alias] {
			return
		}
		if visiting == nil {
			visiting = map[*yamlv3.Node]bool{}
		}
		visiting[node.Alias] = true
		r.collectScalarRanges(ctx, ch, node.Alias, path, blockCounter, content, lineOffsets, ranges, visiting, true, notes)
		delete(visiting, node.Alias)
	}
}

// collectScalarRange checks if a scalar should be extracted and records
// its byte range. When `insideAlias` is true, emits the Block but does
// not append a scalarRange (the alias's source bytes are at the alias
// position, not the original anchor position).
func (r *Reader) collectScalarRange(ctx context.Context, ch chan<- model.PartResult,
	node *yamlv3.Node, path []string, blockCounter *int,
	content []byte, lineOffsets []int, ranges *[]scalarRange,
	insideAlias bool, notes *noteState) {

	// The value node's own head comment (sequence items / document scalars)
	// joins the pending queue, so it attaches to this block when extracted
	// or rides to the next block otherwise. Aliased scalars share the
	// anchor's node — its comments belong to the definition, so skip them
	// to avoid duplicating the note onto the alias block.
	inline, foot := node.LineComment, node.FootComment
	if insideAlias {
		inline, foot = "", ""
	} else {
		notes.queueComment(node.HeadComment)
	}

	isString := node.Tag == "!!str" || node.Tag == ""
	if !isString && !r.cfg.ExtractNonStrings {
		return
	}

	text := node.Value
	if node.Style == yamlv3.FoldedStyle {
		text = foldMoreIndentedRuns(text)
	}
	if strings.TrimSpace(text) == "" {
		return
	}

	keyPath := strings.Join(path, ".")
	if !r.matchesKeyPath(keyPath) {
		return
	}

	// Compute the byte range of this scalar value in the raw content.
	start := lineColToOffset(lineOffsets, node.Line, node.Column)
	if start < 0 || start >= len(content) {
		// Fallback: emit block without span tracking.
		*blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), text)
		block.Name = keyPath
		if r.cfg.UseCodeFinder {
			r.applyCodeFinder(block)
		}
		notes.attach(block, inline, foot)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		return
	}

	// yaml.v3 reports node.Line/Column at the tag indicator (`!`) when
	// the scalar carries an explicit tag (e.g. `! 'value'`, `!!str x`).
	// scanScalarEnd's quoted-string scanners require start to land on
	// the actual scalar character (`'`, `"`, `|`, `>`), so advance past
	// any leading tag handle plus the separating whitespace.
	tagPrefix := scanTagPrefix(content, start)
	start += tagPrefix

	end := scanScalarEnd(content, start, node.Style, node.Value)

	*blockCounter++
	blockID := fmt.Sprintf("tu%d", *blockCounter)

	block := model.NewBlock(blockID, text)
	block.Name = keyPath

	// Store the scalar style so the writer can re-encode in the same style.
	block.Properties["yaml.style"] = scalarStyleName(node.Style)
	// Store the raw original bytes so the writer can reproduce them byte-exact
	// when no translation is applied.
	if start >= 0 && end <= len(content) && start < end {
		block.Properties["yaml.raw"] = string(content[start:end])
	}
	// Record the source's dominant line ending so the writer re-emits
	// multi-line re-encoded (translated) block scalars with the same
	// convention instead of yaml.v3's hardcoded LF. Only set when CRLF;
	// the LF default needs no rewrite and keeps Properties lean.
	if r.eol == "\r\n" {
		block.Properties["yaml.eol"] = r.eol
	}
	// For block scalars (literal `|`, folded `>`), capture the indicator
	// line (`|`, `|-`, `|+`, `|2`, `>-`, …) so the writer can preserve
	// the chomp / explicit-indent indicator on round-trip. Default in
	// the writer is plain `|`/`>` which loses any modifier.
	//
	// Also capture the content's leading-space indent so the writer can
	// re-emit the body at the same column. Without this, the writer
	// hardcodes 2-space indent and a `street: |\n            123…\n…`
	// fixture round-trips to `street: |\n  123…\n…`, diverging from the
	// upstream byte-exact output.
	if node.Style == yamlv3.LiteralStyle || node.Style == yamlv3.FoldedStyle {
		if start < len(content) && (content[start] == '|' || content[start] == '>') {
			j := start
			for j < len(content) && content[j] != '\n' {
				j++
			}
			block.Properties["yaml.indicator"] = string(content[start:j])
			// j is at '\n' (or end). Walk past newline to the first
			// content line and count its leading spaces.
			if j < len(content) && content[j] == '\n' {
				k := j + 1
				indent := 0
				for k < len(content) && content[k] == ' ' {
					indent++
					k++
				}
				if k < len(content) && content[k] != '\n' && indent > 0 {
					block.Properties["yaml.indent"] = strconv.Itoa(indent)
				}
			}
		}
	}

	if r.cfg.UseCodeFinder {
		r.applyCodeFinder(block)
	}

	if !insideAlias {
		*ranges = append(*ranges, scalarRange{
			start:   start,
			end:     end,
			blockID: blockID,
			style:   node.Style,
		})
	}

	notes.attach(block, inline, foot)

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// scanScalarEnd determines the end byte offset of a scalar value in raw YAML.
func scanScalarEnd(content []byte, start int, style yamlv3.Style, value string) int {
	if start >= len(content) {
		return start
	}

	switch {
	case style == yamlv3.DoubleQuotedStyle:
		// Scan past the opening quote to find the closing quote.
		return scanQuotedEnd(content, start, '"')

	case style == yamlv3.SingleQuotedStyle:
		return scanQuotedEnd(content, start, '\'')

	case style == yamlv3.LiteralStyle || style == yamlv3.FoldedStyle:
		return scanBlockScalarEnd(content, start)

	default:
		// Plain scalar: extends to end of line (before comment), or
		// multi-line plain scalar with continuation lines.
		return scanPlainScalarEnd(content, start, value)
	}
}

// scanTagPrefix returns the byte length of a YAML tag prefix at start
// (`!handle ` or `!`+`<verbatim>`+` `), including any trailing
// whitespace separating the tag from the value. Returns 0 when content
// at start does not begin with `!`.
//
// Examples (ret = bytes consumed):
//
//	"! 'value'"        → 2 ("! ")
//	"!str 'value'"     → 5 ("!str ")
//	"!!str 'value'"    → 6 ("!!str ")
//	"!<verbatim> v"    → 12 ("!<verbatim> ")
func scanTagPrefix(content []byte, start int) int {
	if start >= len(content) || content[start] != '!' {
		return 0
	}
	i := start + 1
	// `!<verbatim>` form.
	if i < len(content) && content[i] == '<' {
		for i < len(content) && content[i] != '>' {
			i++
		}
		if i < len(content) {
			i++ // past `>`
		}
	} else {
		// `!`, `!handle`, or `!!handle` — read up to whitespace or EOL.
		for i < len(content) && content[i] != ' ' && content[i] != '\t' &&
			content[i] != '\n' && content[i] != '\r' {
			i++
		}
	}
	// Consume separating whitespace (but not the line terminator).
	for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
		i++
	}
	return i - start
}

// scanQuotedEnd finds the end of a quoted string (handling escapes).
func scanQuotedEnd(content []byte, start int, quote byte) int {
	if start >= len(content) || content[start] != quote {
		return start
	}
	i := start + 1
	for i < len(content) {
		if content[i] == '\\' && quote == '"' {
			i += 2 // skip escape sequence
			continue
		}
		if content[i] == '\'' && quote == '\'' && i+1 < len(content) && content[i+1] == '\'' {
			i += 2 // skip escaped single quote ''
			continue
		}
		if content[i] == quote {
			return i + 1 // past closing quote
		}
		i++
	}
	return i
}

// scanBlockScalarEnd finds the end of a literal (|) or folded (>) block scalar.
func scanBlockScalarEnd(content []byte, start int) int {
	// The indicator line: |, |+, |-, |2, >+, >-, etc., up to and including its newline.
	i := start
	// Skip the indicator character (| or >)
	if i < len(content) && (content[i] == '|' || content[i] == '>') {
		i++
	}
	// Skip optional chomp/indent indicators and inline comment on indicator line
	for i < len(content) && content[i] != '\n' {
		i++
	}
	if i < len(content) && content[i] == '\n' {
		i++ // past the newline after indicator
	}

	// Determine the indentation of the first content line.
	contentIndent := 0
	j := i
	for j < len(content) && content[j] == ' ' {
		contentIndent++
		j++
	}
	if contentIndent == 0 {
		return i // no content lines
	}

	// Consume all lines that are indented at least contentIndent spaces, or are empty.
	for i < len(content) {
		lineStart := i
		// Check indentation
		spaces := 0
		for i < len(content) && content[i] == ' ' {
			spaces++
			i++
		}
		if i >= len(content) {
			// Trailing spaces at EOF
			return i
		}
		if content[i] == '\n' {
			// Empty line (or line with only spaces) — part of block scalar
			i++
			continue
		}
		if spaces < contentIndent {
			// This line is less indented — not part of the block scalar.
			return lineStart
		}
		// Skip to end of line
		for i < len(content) && content[i] != '\n' {
			i++
		}
		if i < len(content) {
			i++ // past newline
		}
	}
	return i
}

// scanPlainScalarEnd finds the end of a plain (unquoted) scalar.
func scanPlainScalarEnd(content []byte, start int, value string) int {
	i := start

	// Detect flow context by scanning backwards for unmatched { or [.
	inFlow := isInFlowContext(content, start)

	if inFlow {
		// In flow context, plain scalars are terminated by , } ] or newline.
		for i < len(content) {
			ch := content[i]
			if ch == ',' || ch == '}' || ch == ']' || ch == '\n' {
				break
			}
			i++
		}
		// Trim trailing whitespace
		for i > start && content[i-1] == ' ' {
			i--
		}
		return i
	}

	// Block context: extends to end of line (before comment).
	lineEnd := i
	for lineEnd < len(content) && content[lineEnd] != '\n' {
		lineEnd++
	}

	// A trailing carriage return is part of the CRLF line break, not the
	// scalar value (YAML 1.2 §5.4: a CR immediately preceding the LF is
	// consumed as the line break and never appears in the parsed value).
	// Excluding it here keeps the `\r` in the skeleton so the writer
	// reproduces the source's CRLF around the (possibly translated)
	// scalar, instead of smuggling the CR into the scalar range where it
	// is lost the moment the scalar is re-encoded.
	commentScanEnd := lineEnd
	if commentScanEnd > i && content[commentScanEnd-1] == '\r' {
		commentScanEnd--
	}

	// Trim trailing comment: find " #" pattern
	effectiveEnd := commentScanEnd
	for j := i; j < commentScanEnd; j++ {
		if j+1 < commentScanEnd && content[j] == ' ' && content[j+1] == '#' {
			effectiveEnd = j
			break
		}
	}

	// Decide whether the scalar continues onto subsequent lines.
	//
	// Plain scalars in YAML may carry continuation lines whose
	// content is folded into single spaces in the parsed value — so
	// `strings.Contains(value, "\n")` is *not* a reliable continuation
	// detector. Instead, check whether the next line is indented
	// strictly deeper than the key's column. When it isn't, the
	// scalar is single-line and we trim trailing whitespace.
	startCol := scalarStartColumn(content, start)
	hasContinuation := false
	if lineEnd < len(content) && content[lineEnd] == '\n' {
		next := lineEnd + 1
		nextIndent := 0
		for next < len(content) && content[next] == ' ' {
			nextIndent++
			next++
		}
		if next < len(content) && content[next] != '\n' && nextIndent > startCol {
			hasContinuation = true
		}
	}
	if !hasContinuation {
		end := effectiveEnd
		for end > i && content[end-1] == ' ' {
			end--
		}
		return end
	}

	// Multi-line plain scalar: include continuation lines.
	//
	// Continuations come in two forms in plain YAML scalars:
	//   1. Hard line breaks: the parsed value contains literal `\n`
	//      between lines (rare for plain scalars, but possible).
	//   2. Folded continuations: subsequent lines indented MORE than
	//      the key column. yaml.v3 folds these into single spaces in
	//      the parsed value, so `value` carries no `\n` for them —
	//      the previous `\n`-counting heuristic under-consumed, left
	//      continuation bytes in the skeleton, and the writer
	//      re-emitted them AFTER the substituted translation, causing
	//      duplicate content.
	//
	// Use indentation-based detection so both cases work: walk
	// forward while subsequent lines are either blank or indented
	// strictly deeper than the key column. Stop at the first line
	// indented at-or-below that column.
	i = lineEnd
	if i < len(content) {
		i++ // past newline
	}
	for i < len(content) {
		lineStart := i
		// Measure indent of this line.
		indent := 0
		for i < len(content) && content[i] == ' ' {
			indent++
			i++
		}
		if i >= len(content) {
			// EOF inside trailing indent — include it.
			i = lineStart
			break
		}
		if content[i] == '\n' {
			// Empty line — part of plain scalar continuation if the
			// next non-empty line is indented enough; consume and
			// keep scanning.
			i++
			continue
		}
		if indent <= startCol {
			// Less- or equal-indented than scalar start — not a
			// continuation. Rewind to start of this line.
			i = lineStart
			break
		}
		// Continuation line — consume to end-of-line.
		for i < len(content) && content[i] != '\n' {
			i++
		}
		if i < len(content) {
			i++ // past newline
		}
	}
	if !strings.HasSuffix(value, "\n") && i > 0 && content[i-1] == '\n' {
		i--
	}
	return i
}

// scalarStartColumn returns the 0-based indent threshold a follow-on
// line must exceed to count as a plain-scalar continuation.
//
// For a mapping-value scalar (`key: value` or `- key: value`) the
// threshold is the column of `key` — sibling keys at that column close
// the scalar.
//
// For a list-item-value scalar (`- value`) the threshold is the column
// of `-` — sibling list items at that column close the scalar.
//
// Without the mapping-vs-list distinction a `- key: value\n  sibkey: …`
// pattern (mapping inside a sequence) would mis-classify the sibling
// `sibkey` line as a continuation of the first value, swallowing it
// from the skeleton and producing concatenated output on round-trip.
func scalarStartColumn(content []byte, start int) int {
	// Walk back to the start of the line.
	lineStart := start
	for lineStart > 0 && content[lineStart-1] != '\n' {
		lineStart--
	}
	// Skip leading whitespace.
	col := 0
	for lineStart+col < len(content) && (content[lineStart+col] == ' ' || content[lineStart+col] == '\t') {
		col++
	}
	// If the line is a list item (`- ` after the indent), and a `:` lives
	// between the dash and the scalar value, the scalar is a mapping
	// value embedded in a sequence — the threshold is the key's column
	// (right after `- `), not the dash's column.
	if lineStart+col+1 < len(content) && content[lineStart+col] == '-' && content[lineStart+col+1] == ' ' {
		keyCol := col + 2
		// Look for `:` between key and scalar start (which is at `start`,
		// already past tag prefix). Presence of `:` => mapping value.
		hasColon := false
		for i := lineStart + keyCol; i < start && i < len(content) && content[i] != '\n'; i++ {
			if content[i] == ':' {
				hasColon = true
				break
			}
		}
		if hasColon {
			return keyCol
		}
	}
	return col
}

// isInFlowContext checks if a position is inside a YAML flow context (within { } or [ ]).
func isInFlowContext(content []byte, pos int) bool {
	depth := 0
	for i := pos - 1; i >= 0; i-- {
		switch content[i] {
		case '}', ']':
			depth++
		case '{', '[':
			if depth > 0 {
				depth--
			} else {
				return true
			}
		case '\n':
			// In block context, flow indicators on previous lines don't count
			// unless we're inside a nested flow. If depth > 0 we're still inside
			// a flow started on a previous line.
			if depth == 0 {
				return false
			}
		}
	}
	return false
}

// scalarStyleName returns a string identifier for the yaml scalar style.
func scalarStyleName(style yamlv3.Style) string {
	switch style {
	case yamlv3.DoubleQuotedStyle:
		return "double-quoted"
	case yamlv3.SingleQuotedStyle:
		return "single-quoted"
	case yamlv3.LiteralStyle:
		return "literal"
	case yamlv3.FoldedStyle:
		return "folded"
	case yamlv3.FlowStyle:
		return "flow"
	default:
		return "plain"
	}
}

// buildSkeleton constructs skeleton entries from raw bytes and sorted
// scalar byte ranges.
func (r *Reader) buildSkeleton(content []byte, ranges []scalarRange) {
	// Sort by start offset (they should already be in order from tree walk).
	// SortStableFunc preserves the prior insertion-sort's stable ordering for
	// any equal-start entries, keeping output byte-identical.
	slices.SortStableFunc(ranges, func(a, b scalarRange) int {
		return cmp.Compare(a.start, b.start)
	})

	pos := 0
	for _, sr := range ranges {
		if sr.start > pos {
			r.skelBytes(content[pos:sr.start])
		}
		r.skelRef(sr.blockID)
		pos = sr.end
	}
	// Trailing content
	if pos < len(content) {
		r.skelBytes(content[pos:])
	}
	r.skelFlush()
}

// walkNode emits scalar Parts from the parsed YAML tree.
//
// `visiting` tracks alias targets currently on the recursion stack so
// self-referential anchors terminate instead of looping forever (see
// collectScalarRanges for the same pattern).
func (r *Reader) walkNode(ctx context.Context, ch chan<- model.PartResult, node *yamlv3.Node, path []string, blockCounter *int, visiting map[*yamlv3.Node]bool, notes *noteState) {
	switch node.Kind {
	case yamlv3.DocumentNode:
		// Multi-document: each document node wraps content
		for _, child := range node.Content {
			r.walkNode(ctx, ch, child, path, blockCounter, visiting, notes)
		}

	case yamlv3.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := keyNode.Value
			newPath := append(append([]string{}, path...), key)
			notes.queueComment(keyNode.HeadComment)
			r.walkNode(ctx, ch, valNode, newPath, blockCounter, visiting, notes)
			notes.queueComment(keyNode.LineComment)
			notes.queueComment(keyNode.FootComment)
		}

	case yamlv3.SequenceNode:
		for i, child := range node.Content {
			indexPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i))
			r.walkNode(ctx, ch, child, indexPath, blockCounter, visiting, notes)
		}

	case yamlv3.ScalarNode:
		r.emitScalar(ctx, ch, node, path, blockCounter, notes)

	case yamlv3.AliasNode:
		if node.Alias == nil || visiting[node.Alias] {
			return
		}
		if visiting == nil {
			visiting = map[*yamlv3.Node]bool{}
		}
		visiting[node.Alias] = true
		r.walkNode(ctx, ch, node.Alias, path, blockCounter, visiting, notes)
		delete(visiting, node.Alias)
	}
}

func (r *Reader) emitScalar(ctx context.Context, ch chan<- model.PartResult, node *yamlv3.Node, path []string, blockCounter *int, notes *noteState) {
	// The value node's own head comment (sequence items / document scalars)
	// joins the pending queue, so it attaches to this block when extracted
	// or rides to the next block otherwise.
	notes.queueComment(node.HeadComment)

	isString := node.Tag == "!!str" || node.Tag == ""

	if !isString && !r.cfg.ExtractNonStrings {
		return
	}

	text := node.Value
	if node.Style == yamlv3.FoldedStyle {
		text = foldMoreIndentedRuns(text)
	}
	if strings.TrimSpace(text) == "" {
		return
	}

	keyPath := strings.Join(path, ".")
	if !r.matchesKeyPath(keyPath) {
		return
	}

	*blockCounter++
	block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), text)
	block.Name = keyPath
	if r.cfg.UseCodeFinder {
		r.applyCodeFinder(block)
	}
	notes.attach(block, node.LineComment, node.FootComment)
	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// matchesKeyPath checks whether a key path matches the configured patterns.
// If no patterns are configured, all paths match.
func (r *Reader) matchesKeyPath(keyPath string) bool {
	if len(r.cfg.KeyPathPatterns) == 0 {
		return true
	}
	for _, pattern := range r.cfg.KeyPathPatterns {
		if matchGlobPath(pattern, keyPath) {
			return true
		}
	}
	return false
}

// matchGlobPath matches a dot-separated key path against a glob pattern.
// Supports * (matches one segment) and ** (matches zero or more segments).
func matchGlobPath(pattern, path string) bool {
	patParts := strings.Split(pattern, ".")
	pathParts := strings.Split(path, ".")
	return matchParts(patParts, pathParts)
}

func matchParts(pat, path []string) bool {
	if len(pat) == 0 {
		return len(path) == 0
	}
	if pat[0] == "**" {
		// ** matches zero or more segments
		// Try matching remaining pattern against every suffix of path
		for i := 0; i <= len(path); i++ {
			if matchParts(pat[1:], path[i:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	if pat[0] == "*" || pat[0] == path[0] {
		return matchParts(pat[1:], path[1:])
	}
	return false
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// applyCodeFinder applies code finder patterns to a block's segments,
// rewriting their Run sequences with placeholder runs at matched
// positions.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.CodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}

	if len(block.Source) == 0 {
		return
	}
	text := model.RunsText(block.Source)

	// Collect all match ranges
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

	// Rebuild fragment with coded text markers
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
	block.Source = runs
}

// foldMoreIndentedRuns mirrors snakeyaml's (Okapi reference parser)
// behavior of folding consecutive more-indented lines in a `>` (folded)
// scalar with single spaces, instead of preserving their newlines per
// YAML 1.2 §8.1.3. yaml.v3 implements the spec correctly and yields a
// value where more-indented lines keep their `\n` separators (e.g.
// `"    L3\n    L4"`); snakeyaml flattens them to `"    L3 L4"`.
//
// The okapi YAML filter feeds its parser through a custom javacc
// grammar (net.sf.okapi.filters.yaml.parser.IndentedBlock) that always
// folds adjacent content lines with a space — including more-indented
// runs — and only inserts a newline when a "skeleton" (blank) line
// comes between them. The okapi tokenizer also strips the leading
// indentation of each tokenized line (the parent indent stays in
// skeleton), so when okapi joins two more-indented lines the second
// line contributes only its non-whitespace content. To round-trip
// byte-equal with okapi's writer, we pre-fold the value here so the
// writer's encoder sees the same shape snakeyaml would have produced
// and emits it back identically.
//
// Rule: between any two non-empty value lines separated by EXACTLY one
// `\n`, where BOTH lines start with whitespace (more-indented),
// replace the `\n` plus the second line's leading whitespace with a
// single space. The first line's leading whitespace is preserved so
// the writer can re-emit the more-indented content at the right
// column. Runs of `\n\n` or longer (paragraph boundaries) are
// preserved as-is — those represent source blank lines, which
// snakeyaml emits as separate text spans linked by `lb` codes.
func foldMoreIndentedRuns(s string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	n := len(s)
	for i < n {
		// Walk to the next `\n`.
		j := i
		for j < n && s[j] != '\n' {
			j++
		}
		b.WriteString(s[i:j])
		if j >= n {
			return b.String()
		}
		// Count consecutive `\n` starting at j.
		k := j
		for k < n && s[k] == '\n' {
			k++
		}
		runLen := k - j
		if runLen == 1 {
			prev := s[i:j]
			// Find the next line's text bounds and its leading
			// whitespace length.
			lineStart := k
			ws := 0
			for lineStart+ws < n && (s[lineStart+ws] == ' ' || s[lineStart+ws] == '\t') {
				ws++
			}
			textStart := lineStart + ws
			textEnd := textStart
			for textEnd < n && s[textEnd] != '\n' {
				textEnd++
			}
			nextHasContent := textEnd > textStart
			if len(prev) > 0 && nextHasContent && isMoreIndentedLine(prev) && ws > 0 {
				// Fold: replace the single `\n` + leading whitespace
				// with one space, then continue scanning from the
				// non-whitespace start of the second line. The first
				// line's existing indent is preserved so the writer
				// re-emits the more-indented column unchanged.
				b.WriteByte(' ')
				i = textStart
				continue
			}
		}
		// Preserve the `\n` run as-is.
		b.WriteString(s[j:k])
		i = k
	}
	return b.String()
}

// isMoreIndentedLine reports whether a value line starts with
// whitespace (space or tab), marking it as a "more-indented" line in
// folded-scalar terms. yaml.v3 strips the block-scalar content indent
// before returning the value; whatever leading whitespace remains is
// "more than the content indent" and counts as more-indented.
func isMoreIndentedLine(line string) bool {
	if line == "" {
		return false
	}
	c := line[0]
	return c == ' ' || c == '\t'
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
