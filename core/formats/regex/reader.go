package regex

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

// Reader implements DataFormatReader for regex-based text extraction.
// It applies configurable regex rules to extract translatable content
// from arbitrary text formats (Mac .strings, INI, StringInfo, etc.).
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new Regex reader with default configuration.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "regex",
			FormatDisplayName: "Regex Extraction",
			FormatMimeType:    "text/x-regex",
			FormatExtensions:  []string{".ini", ".info", ".rls"},
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
		MIMETypes:  []string{"text/x-regex"},
		Extensions: []string{".ini", ".info", ".rls"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("regex: nil document or reader")
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

// compiledRule is a Rule with its pre-compiled regexp.
type compiledRule struct {
	Rule
	re *regexp.Regexp
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "regex",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-regex",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Read entire input
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitError(ch, fmt.Errorf("regex: reading input: %w", err))
		return
	}
	content := string(data)

	if len(r.cfg.Rules) == 0 {
		// No rules: emit entire content as Data
		if content != "" {
			r.skelText(content)
			r.emitData(ctx, ch, 1, content)
		}
		r.skelFlush()
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
		return
	}

	// Compile rules
	compiled := make([]compiledRule, 0, len(r.cfg.Rules))
	for _, rule := range r.cfg.Rules {
		re, compileErr := regexp.Compile(rule.Pattern)
		if compileErr != nil {
			r.emitError(ch, fmt.Errorf("regex: compiling pattern %q: %w", rule.Pattern, compileErr))
			return
		}
		compiled = append(compiled, compiledRule{Rule: rule, re: re})
	}

	r.extractWithRules(ctx, ch, content, compiled)

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// match represents a regex match with its associated rule.
//
// prefix and suffix are the raw document text on either side of the
// translatable source capture group, sliced directly from the original
// content. They let the writer rebuild output by pure assembly
// (prefix + value + suffix) instead of string-replacing inside the full
// match.
type match struct {
	start  int
	end    int
	groups []string
	rule   compiledRule
	prefix string
	suffix string
}

func (r *Reader) extractWithRules(ctx context.Context, ch chan<- model.PartResult, content string, rules []compiledRule) {
	// Find all matches across all rules and sort by position.
	var matches []match
	for _, cr := range rules {
		allIdx := cr.re.FindAllStringSubmatchIndex(content, -1)
		for _, idx := range allIdx {
			if len(idx) < 2 {
				continue
			}
			groups := extractGroups(content, idx)
			prefix, suffix := splitAroundSourceGroup(content, idx, cr.SourceGroup)
			m := match{
				start:  idx[0],
				end:    idx[1],
				groups: groups,
				rule:   cr,
				prefix: prefix,
				suffix: suffix,
			}
			matches = append(matches, m)
		}
	}

	// Sort matches by start position (stable to preserve rule order for ties)
	sortMatches(matches)

	// Remove overlapping matches (first match wins)
	matches = removeOverlaps(matches)

	blockCounter := 0
	dataCounter := 0
	pos := 0

	for _, m := range matches {
		// Emit non-matching content before this match as Data
		if m.start > pos {
			r.skelText(content[pos:m.start])
			dataCounter++
			if !r.emitData(ctx, ch, dataCounter, content[pos:m.start]) {
				return
			}
		}

		// Extract source text from the configured group
		sourceText := ""
		if m.rule.SourceGroup > 0 && m.rule.SourceGroup < len(m.groups) {
			sourceText = m.groups[m.rule.SourceGroup]
		}

		// Unescape if configured
		sourceText = r.unescape(sourceText)

		// Generate block ID
		blockCounter++
		blockID := fmt.Sprintf("tu%d", blockCounter)

		// For skeleton: the entire match region is represented as a ref
		r.skelRef(blockID)

		block := model.NewBlock(blockID, sourceText)

		// Extract ID/name from group if configured
		if m.rule.IDGroup > 0 && m.rule.IDGroup < len(m.groups) {
			block.Name = m.groups[m.rule.IDGroup]
		}

		// Extract note from group if configured
		if m.rule.NoteGroup > 0 && m.rule.NoteGroup < len(m.groups) {
			noteText := m.groups[m.rule.NoteGroup]
			if noteText != "" {
				block.SetAnno("note", &model.NoteAnnotation{
					Text: noteText,
					From: "regex",
				})
			}
		}

		// Store the raw text on either side of the translatable capture so
		// the writer can rebuild the match by pure assembly:
		//   prefix + escape(value) + suffix
		// No reverse-engineering of the split via string replacement.
		block.Properties["regex.prefix"] = m.prefix
		block.Properties["regex.suffix"] = m.suffix
		block.Properties["regex.sourceGroup"] = strconv.Itoa(m.rule.SourceGroup)

		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}

		pos = m.end
	}

	// Emit trailing non-matching content as Data
	if pos < len(content) {
		r.skelText(content[pos:])
		dataCounter++
		r.emitData(ctx, ch, dataCounter, content[pos:])
	}
}

func (r *Reader) unescape(s string) string {
	escType := r.cfg.EscapeType
	if escType == "" {
		escType = EscapeNone
	}

	switch escType {
	case EscapeBackslash:
		return unescapeBackslash(s)
	case EscapeDoubleChar:
		return unescapeDoubleChar(s, r.cfg.EscapeChar)
	default:
		return s
	}
}

func unescapeBackslash(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case 'r':
				buf.WriteByte('\r')
			case '\\':
				buf.WriteByte('\\')
			case '"':
				buf.WriteByte('"')
			case '\'':
				buf.WriteByte('\'')
			default:
				// Unknown escape: preserve as-is
				buf.WriteByte('\\')
				buf.WriteByte(next)
			}
			i += 2
		} else {
			buf.WriteByte(s[i])
			i++
		}
	}
	return buf.String()
}

func unescapeDoubleChar(s string, escChar string) string {
	if escChar == "" {
		escChar = "\""
	}
	// Replace doubled escape character with single
	doubled := escChar + escChar
	return strings.ReplaceAll(s, doubled, escChar)
}

func (r *Reader) emitData(ctx context.Context, ch chan<- model.PartResult, counter int, content string) bool {
	d := &model.Data{
		ID:         fmt.Sprintf("d%d", counter),
		Properties: map[string]string{"content": content},
	}
	return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d})
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

func (r *Reader) emitError(ch chan<- model.PartResult, err error) {
	ch <- model.PartResult{Error: err}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

// splitAroundSourceGroup returns the raw document text before and after the
// translatable source capture group within the full match. Offsets come from
// FindAllStringSubmatchIndex, so prefix+group+suffix == content[idx[0]:idx[1]]
// exactly (byte-for-byte), preserving any delimiters, whitespace, or escaped
// characters that surround the captured value.
//
// When the source group is unset, out of range, or did not participate in the
// match (negative offsets), the whole match is treated as prefix and the
// suffix is empty — the writer then emits prefix verbatim, matching the old
// fallback behaviour for matches with no usable capture.
func splitAroundSourceGroup(content string, idx []int, sourceGroup int) (prefix, suffix string) {
	full := content[idx[0]:idx[1]]
	if sourceGroup <= 0 || 2*sourceGroup+1 >= len(idx) {
		return full, ""
	}
	gStart := idx[2*sourceGroup]
	gEnd := idx[2*sourceGroup+1]
	if gStart < 0 || gEnd < 0 {
		return full, ""
	}
	return content[idx[0]:gStart], content[gEnd:idx[1]]
}

// extractGroups extracts the matched groups from submatch indices.
func extractGroups(content string, idx []int) []string {
	numGroups := len(idx) / 2
	groups := make([]string, numGroups)
	for i := range numGroups {
		start := idx[2*i]
		end := idx[2*i+1]
		if start >= 0 && end >= 0 {
			groups[i] = content[start:end]
		}
	}
	return groups
}

// sortMatches sorts matches by start position, breaking ties by rule order.
func sortMatches(matches []match) {
	// Simple insertion sort (typically few matches)
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}
}

// removeOverlaps removes later matches that overlap with earlier ones.
func removeOverlaps(matches []match) []match {
	if len(matches) == 0 {
		return matches
	}
	result := []match{matches[0]}
	for i := 1; i < len(matches); i++ {
		last := result[len(result)-1]
		if matches[i].start >= last.end {
			result = append(result, matches[i])
		}
	}
	return result
}
