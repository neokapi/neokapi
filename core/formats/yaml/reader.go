package yaml

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	yamlv3 "gopkg.in/yaml.v3"
)

// Reader implements DataFormatReader for YAML files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
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

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("yaml: reading: %w", err)}
		return
	}

	blockCounter := 0

	// Use a Decoder to support multi-document YAML (--- separators).
	decoder := yamlv3.NewDecoder(strings.NewReader(string(content)))

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
				ch <- model.PartResult{Error: fmt.Errorf("yaml: parsing: %w", err)}
				return
			}
			r.collectScalarRanges(ctx, ch, &node, nil, &blockCounter, content, lineOffsets, &ranges, nil)
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
				ch <- model.PartResult{Error: fmt.Errorf("yaml: parsing: %w", err)}
				return
			}
			r.walkNode(ctx, ch, &node, nil, &blockCounter, nil)
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// scalarRange records the byte range of a translatable scalar in the raw YAML content.
type scalarRange struct {
	start   int    // byte offset of the scalar value representation (including quotes)
	end     int    // byte offset past the scalar
	blockID string // block ID (e.g. "tu1")
	style   yamlv3.Style
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
	visiting map[*yamlv3.Node]bool) {

	switch node.Kind {
	case yamlv3.DocumentNode:
		for _, child := range node.Content {
			r.collectScalarRanges(ctx, ch, child, path, blockCounter, content, lineOffsets, ranges, visiting)
		}

	case yamlv3.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := keyNode.Value
			newPath := append(append([]string{}, path...), key)
			r.collectScalarRanges(ctx, ch, valNode, newPath, blockCounter, content, lineOffsets, ranges, visiting)
		}

	case yamlv3.SequenceNode:
		for i, child := range node.Content {
			indexPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i))
			r.collectScalarRanges(ctx, ch, child, indexPath, blockCounter, content, lineOffsets, ranges, visiting)
		}

	case yamlv3.ScalarNode:
		r.collectScalarRange(ctx, ch, node, path, blockCounter, content, lineOffsets, ranges)

	case yamlv3.AliasNode:
		if node.Alias == nil || visiting[node.Alias] {
			return
		}
		if visiting == nil {
			visiting = map[*yamlv3.Node]bool{}
		}
		visiting[node.Alias] = true
		r.collectScalarRanges(ctx, ch, node.Alias, path, blockCounter, content, lineOffsets, ranges, visiting)
		delete(visiting, node.Alias)
	}
}

// collectScalarRange checks if a scalar should be extracted and records
// its byte range.
func (r *Reader) collectScalarRange(ctx context.Context, ch chan<- model.PartResult,
	node *yamlv3.Node, path []string, blockCounter *int,
	content []byte, lineOffsets []int, ranges *[]scalarRange) {

	isString := node.Tag == "!!str" || node.Tag == ""
	if !isString && !r.cfg.ExtractNonStrings {
		return
	}

	text := node.Value
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
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		return
	}

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

	if r.cfg.UseCodeFinder {
		r.applyCodeFinder(block)
	}

	*ranges = append(*ranges, scalarRange{
		start:   start,
		end:     end,
		blockID: blockID,
		style:   node.Style,
	})

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

	// Trim trailing comment: find " #" pattern
	effectiveEnd := lineEnd
	for j := i; j < lineEnd; j++ {
		if j+1 < lineEnd && content[j] == ' ' && content[j+1] == '#' {
			effectiveEnd = j
			break
		}
	}

	// For single-line plain scalars, trim trailing whitespace.
	if !strings.Contains(value, "\n") {
		end := effectiveEnd
		for end > i && content[end-1] == ' ' {
			end--
		}
		return end
	}

	// Multi-line plain scalar: include continuation lines.
	i = lineEnd
	if i < len(content) {
		i++ // past newline
	}
	valueLines := strings.Count(value, "\n")
	for line := 0; line < valueLines && i < len(content); line++ {
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
	for i := 1; i < len(ranges); i++ {
		for j := i; j > 0 && ranges[j].start < ranges[j-1].start; j-- {
			ranges[j], ranges[j-1] = ranges[j-1], ranges[j]
		}
	}

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
func (r *Reader) walkNode(ctx context.Context, ch chan<- model.PartResult, node *yamlv3.Node, path []string, blockCounter *int, visiting map[*yamlv3.Node]bool) {
	switch node.Kind {
	case yamlv3.DocumentNode:
		// Multi-document: each document node wraps content
		for _, child := range node.Content {
			r.walkNode(ctx, ch, child, path, blockCounter, visiting)
		}

	case yamlv3.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := keyNode.Value
			newPath := append(append([]string{}, path...), key)
			r.walkNode(ctx, ch, valNode, newPath, blockCounter, visiting)
		}

	case yamlv3.SequenceNode:
		for i, child := range node.Content {
			indexPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i))
			r.walkNode(ctx, ch, child, indexPath, blockCounter, visiting)
		}

	case yamlv3.ScalarNode:
		r.emitScalar(ctx, ch, node, path, blockCounter)

	case yamlv3.AliasNode:
		if node.Alias == nil || visiting[node.Alias] {
			return
		}
		if visiting == nil {
			visiting = map[*yamlv3.Node]bool{}
		}
		visiting[node.Alias] = true
		r.walkNode(ctx, ch, node.Alias, path, blockCounter, visiting)
		delete(visiting, node.Alias)
	}
}

func (r *Reader) emitScalar(ctx context.Context, ch chan<- model.PartResult, node *yamlv3.Node, path []string, blockCounter *int) {
	isString := node.Tag == "!!str" || node.Tag == ""

	if !isString && !r.cfg.ExtractNonStrings {
		return
	}

	text := node.Value
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
	patterns := r.cfg.GetCodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}

	for _, seg := range block.Source {
		if len(seg.Runs) == 0 {
			continue
		}
		text := seg.Text()

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
			continue
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
		seg.SetRuns(runs)
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
