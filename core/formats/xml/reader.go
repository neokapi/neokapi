package xml

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for XML files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	resolver      format.SubfilterResolver
	skeletonStore *format.SkeletonStore
	layerSeq      int
}

// Ensure Reader implements SubfilterAware and SkeletonStoreEmitter.
var _ format.SubfilterAware = (*Reader)(nil)
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new XML reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xml",
			FormatDisplayName: "XML",
			FormatMimeType:    "text/xml",
			FormatExtensions:  []string{".xml"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format readers.
func (r *Reader) SetSubfilterResolver(resolver format.SubfilterResolver) {
	r.resolver = resolver
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// SetConfig applies a new configuration.
func (r *Reader) SetConfig(cfg format.DataFormatConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	r.Cfg = cfg
	if c, ok := cfg.(*Config); ok {
		r.cfg = c
	}
	return nil
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/xml", "application/xml"},
		Extensions: []string{".xml"},
		MagicBytes: [][]byte{[]byte("<?xml")},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("xml: nil document or reader")
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

// elementFrame tracks the state for each nested element during parsing.
type elementFrame struct {
	name             string
	attrs            map[string]string
	isInline         bool
	isExcluded       bool
	preserveWS       bool
	frag             *model.Fragment
	spanID           int
	hasContent       bool // true if inline element had any child content
	contentByteStart int  // byte offset where element content begins (after '>'), for skeleton
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "xml",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xml: reading: %w", err)}
		return
	}

	if r.skeletonStore != nil {
		r.readContentSkeleton(ctx, ch, content, layer)
	} else {
		r.readContentSimple(ctx, ch, content, layer)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// skelContentRange records a byte range in the source that corresponds to a block's text content.
type skelContentRange struct {
	blockID string
	start   int // byte offset inclusive
	end     int // byte offset exclusive
}

// skelAttrRange records a byte range for a translatable attribute value.
type skelAttrRange struct {
	blockID string
	start   int // byte offset of attribute value (inside quotes)
	end     int // byte offset after attribute value
}

func (r *Reader) readContentSimple(ctx context.Context, ch chan<- model.PartResult, content []byte, layer *model.Layer) {
	r.readContentCore(ctx, ch, content, layer, nil, nil)
}

func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult, content []byte, layer *model.Layer) {
	// For skeleton mode, we do the normal parse but also track byte positions.
	// After parsing, we write skeleton entries using the collected ranges.
	// The key: only leaf block elements get skeleton refs. Parent containers
	// that don't produce blocks (or produce blocks with only spans/no text)
	// have their content preserved as skeleton text.

	var contentRanges []skelContentRange
	var attrRanges []skelAttrRange
	r.readContentCore(ctx, ch, content, layer, &contentRanges, &attrRanges)

	// Write skeleton entries from the collected ranges.
	r.writeSkeletonEntries(content, contentRanges, attrRanges)
}

func (r *Reader) readContentCore(ctx context.Context, ch chan<- model.PartResult, content []byte, layer *model.Layer,
	contentRanges *[]skelContentRange, attrRanges *[]skelAttrRange) {

	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	blockCounter := 0
	dataCounter := 0
	spanCounter := 0
	var stack []*elementFrame
	var wsStack []bool

	// findTextFrame returns the nearest non-inline ancestor frame.
	findTextFrame := func() *elementFrame {
		for i := len(stack) - 1; i >= 0; i-- {
			if !stack[i].isInline {
				return stack[i]
			}
		}
		return nil
	}

	// isInExcludedScope checks if any ancestor is excluded (but not inline+excluded).
	isInExcludedScope := func() bool {
		for _, f := range stack {
			if f.isExcluded {
				return true
			}
		}
		return false
	}

	// elemPath builds the path for the given stack (including the frames on it).
	elemPath := func() string {
		var parts []string
		for _, f := range stack {
			parts = append(parts, f.name)
		}
		return strings.Join(parts, ".")
	}

	// isTranslatable checks if the given frame's content is translatable.
	isTranslatable := func(frame *elementFrame) bool {
		if r.cfg.ExcludeByDefault {
			return r.cfg.isIncludedElement(frame.name, frame.attrs)
		}
		if r.cfg.isExcludedElement(frame.name, frame.attrs) {
			return false
		}
		if len(r.cfg.TranslatableElements) > 0 {
			for _, e := range r.cfg.TranslatableElements {
				if e == frame.name {
					return true
				}
			}
			return false
		}
		return true
	}

	// flushBlock emits the accumulated text as a block or data part.
	// The frame has already been popped from stack, so we pass the path separately.
	// endTagOffset is the byte offset of the end tag (for skeleton tracking).
	flushBlock := func(frame *elementFrame, path string, endTagOffset int) {
		if frame == nil || frame.frag == nil {
			return
		}

		var finalFrag *model.Fragment
		if frame.preserveWS || contentRanges != nil {
			// In skeleton mode, preserve whitespace as-is for byte-exact roundtrip.
			// The skeleton ref covers the raw bytes, and XML-escaping the decoded
			// text will reproduce the original encoding.
			finalFrag = frame.frag
		} else {
			finalFrag = collapseFragmentWhitespace(frame.frag)
		}

		text := finalFrag.Text()
		if text == "" && !finalFrag.HasSpans() {
			return
		}

		if !isTranslatable(frame) {
			// Emit as data part
			dataCounter++
			data := &model.Data{
				ID:   "d" + strconv.Itoa(dataCounter),
				Name: path,
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
			return
		}

		// Check for subfilter
		if mapping := r.matchSubfilter(path); mapping != nil && r.resolver != nil {
			r.emitSubfiltered(ctx, ch, text, path, layer.ID, mapping, &blockCounter, &dataCounter)
			frame.frag = nil
			return
		}

		blockCounter++
		blockID := "tu" + strconv.Itoa(blockCounter)
		block := &model.Block{
			ID:           blockID,
			Translatable: true,
			Source: []*model.Segment{{
				ID:      "s1",
				Content: finalFrag,
			}},
			Targets:     make(map[model.LocaleID][]*model.Segment),
			Properties:  make(map[string]string),
			Annotations: make(map[string]model.Annotation),
		}

		block.Name = path

		// Set block name from ID attribute if available
		idVal := r.cfg.getIDAttribute(frame.name, frame.attrs)
		if idVal != "" {
			block.Name = idVal
		}

		// Set block type
		block.Type = r.cfg.getBlockType(frame.name)

		// Set PreserveWhitespace
		block.PreserveWhitespace = frame.preserveWS

		// Set writable attributes as properties
		writableAttrs := r.cfg.getWritableAttributes(frame.name, frame.attrs)
		for k, v := range writableAttrs {
			block.Properties[k] = v
		}

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})

		// Track content range for skeleton
		if contentRanges != nil && frame.contentByteStart > 0 && endTagOffset > 0 {
			// Find the start of the end tag by searching backwards from endTagOffset
			closeStart := findCloseTagStart(content, frame.contentByteStart, endTagOffset, frame.name)
			if closeStart >= 0 {
				*contentRanges = append(*contentRanges, skelContentRange{
					blockID: blockID,
					start:   frame.contentByteStart,
					end:     closeStart,
				})
			}
		}

		frame.frag = nil
	}

	for {
		tokOffset := int(decoder.InputOffset())
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xml: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			attrs := make(map[string]string)
			for _, attr := range t.Attr {
				key := attr.Name.Local
				if attr.Name.Space == "xml" || attr.Name.Space == "http://www.w3.org/XML/1998/namespace" {
					key = "xml:" + attr.Name.Local
				} else if attr.Name.Space != "" {
					key = attr.Name.Space + ":" + attr.Name.Local
				}
				attrs[key] = attr.Value
			}

			// Detect xml:lang
			if lang, ok := attrs["xml:lang"]; ok {
				dataCounter++
				data := &model.Data{
					ID:         "d" + strconv.Itoa(dataCounter),
					Name:       t.Name.Local,
					Properties: map[string]string{"language": lang},
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
			}

			isInline := r.cfg.isInlineElement(t.Name.Local)
			isExcluded := r.cfg.isExcludedElement(t.Name.Local, attrs)

			// Check excludeByDefault
			if r.cfg.ExcludeByDefault && !r.cfg.isIncludedElement(t.Name.Local, attrs) {
				isExcluded = true
			}

			// An INCLUDE inside an excluded parent overrides
			if isInExcludedScope() && r.cfg.isIncludedElement(t.Name.Local, attrs) {
				isExcluded = false
			}

			// Check xml:space
			preserveWS := r.cfg.shouldPreserveWhitespace(t.Name.Local)
			if v, ok := attrs["xml:space"]; ok {
				preserveWS = v == "preserve"
			}
			// Inherit from parent
			if len(wsStack) > 0 && wsStack[len(wsStack)-1] {
				preserveWS = true
			}
			wsStack = append(wsStack, preserveWS)

			// Check if inline+excluded (content suppressed but element still inline)
			inlineExcluded := isInline && r.isInlineExcluded(t.Name.Local, attrs)

			// Content starts right after the '>' of the start tag
			contentStart := int(decoder.InputOffset())

			frame := &elementFrame{
				name:             t.Name.Local,
				attrs:            attrs,
				isInline:         isInline,
				isExcluded:       isExcluded || inlineExcluded,
				preserveWS:       preserveWS,
				contentByteStart: contentStart,
			}

			if isInline {
				// Mark parent inline elements as having content
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i].isInline {
						stack[i].hasContent = true
					} else {
						break
					}
				}
				// For inline elements, add opening span to parent's fragment
				parent := findTextFrame()
				if parent != nil && parent.frag != nil && !parent.isExcluded {
					spanCounter++
					parent.frag.AppendSpan(&model.Span{
						SpanType: model.SpanOpening,
						ID:       strconv.Itoa(spanCounter),
						Data:     buildStartTag(t),
						Type:     "fmt:" + t.Name.Local,
					})
					frame.spanID = spanCounter
				}
			} else {
				// Start a new text accumulator for this block element
				frame.frag = model.NewFragment("")
			}

			stack = append(stack, frame)

			// Emit translatable attributes as blocks
			for _, attr := range t.Attr {
				attrName := attr.Name.Local
				if attr.Name.Space == "xml" {
					attrName = "xml:" + attr.Name.Local
				} else if attr.Name.Space != "" {
					attrName = attr.Name.Space + ":" + attr.Name.Local
				}
				if r.cfg.isTranslatableAttribute(t.Name.Local, attrName, attrs) {
					blockCounter++
					blockID := "tu" + strconv.Itoa(blockCounter)
					block := model.NewBlock(blockID, attr.Value)
					block.Name = elemPath() + "@" + attrName
					block.Type = "attribute"
					block.IsReferent = true
					r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})

					// Track attribute range for skeleton
					if attrRanges != nil {
						attrStart, attrEnd := findAttrValueByteRange(content, tokOffset, contentStart, attrName, attr.Value)
						if attrStart >= 0 {
							*attrRanges = append(*attrRanges, skelAttrRange{
								blockID: blockID,
								start:   attrStart,
								end:     attrEnd,
							})
						}
					}
				}
			}

		case xml.EndElement:
			if len(wsStack) > 0 {
				wsStack = wsStack[:len(wsStack)-1]
			}
			if len(stack) == 0 {
				continue
			}
			frame := stack[len(stack)-1]
			// Compute the path before popping
			path := elemPath()
			stack = stack[:len(stack)-1]

			// endTagOffset is the byte offset after the end tag
			endTagOffset := int(decoder.InputOffset())

			if frame.isInline {
				parent := findTextFrame()
				if parent != nil && parent.frag != nil && !parent.isExcluded {
					if !frame.hasContent {
						// Self-closing / empty inline: replace the opening span with a placeholder
						spanID := strconv.Itoa(frame.spanID)
						for i, s := range parent.frag.Spans {
							if s.ID == spanID && s.SpanType == model.SpanOpening {
								parent.frag.Spans[i] = &model.Span{
									SpanType: model.SpanPlaceholder,
									ID:       spanID,
									Data:     s.Data,
									Type:     s.Type,
								}
								break
							}
						}
					} else {
						// Add closing span to parent's fragment
						parent.frag.AppendSpan(&model.Span{
							SpanType: model.SpanClosing,
							ID:       strconv.Itoa(frame.spanID),
							Data:     "</" + t.Name.Local + ">",
							Type:     "fmt:" + t.Name.Local,
						})
					}
				}
			} else {
				// Flush accumulated text as a block
				if !frame.isExcluded {
					flushBlock(frame, path, endTagOffset)
				}
			}

		case xml.CharData:
			text := string(t)

			// If in excluded scope, check what kind
			if isInExcludedScope() {
				// Check if the nearest non-inline ancestor is excluded
				textFrame := findTextFrame()
				if textFrame == nil || textFrame.isExcluded {
					continue
				}
				// The text frame is not excluded, but an inline ancestor is.
				// Skip text from any excluded inline element in the ancestor chain.
				excludedInline := false
				for i := len(stack) - 1; i >= 0; i-- {
					if !stack[i].isInline {
						break
					}
					if stack[i].isExcluded {
						excludedInline = true
						break
					}
				}
				if excludedInline {
					continue
				}
			}

			// Find the frame that should accumulate this text
			textFrame := findTextFrame()

			if textFrame != nil {
				// Mark all inline ancestors as having content
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i].isInline {
						stack[i].hasContent = true
					} else {
						break
					}
				}
				// Accumulate text in the current text frame
				if textFrame.frag == nil {
					textFrame.frag = model.NewFragment("")
				}
				textFrame.frag.AppendText(text)
				continue
			}

			// No parent frame — standalone text (shouldn't normally happen with well-formed XML)
			trimmed := strings.TrimSpace(text)
			if trimmed == "" {
				continue
			}
			path := elemPath()
			blockCounter++
			block := model.NewBlock("tu" + strconv.Itoa(blockCounter), trimmed)
			block.Name = path
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})

		case xml.ProcInst:
			// If we're inside a block element, add as placeholder span
			textFrame := findTextFrame()
			if textFrame != nil && textFrame.frag != nil {
				spanCounter++
				piData := "<?" + t.Target
				if len(t.Inst) > 0 {
					piData += " " + string(t.Inst)
				}
				piData += "?>"
				textFrame.frag.AppendSpan(&model.Span{
					SpanType: model.SpanPlaceholder,
					ID:       strconv.Itoa(spanCounter),
					Data:     piData,
					Type:     "xml:pi",
				})
			} else {
				dataCounter++
				data := &model.Data{
					ID:   "d" + strconv.Itoa(dataCounter),
					Name: "processing-instruction",
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
			}

		case xml.Comment:
			// If we're inside a block element, add as placeholder span
			textFrame := findTextFrame()
			if textFrame != nil && textFrame.frag != nil {
				spanCounter++
				textFrame.frag.AppendSpan(&model.Span{
					SpanType: model.SpanPlaceholder,
					ID:       strconv.Itoa(spanCounter),
					Data:     "<!--" + string(t) + "-->",
					Type:     "xml:comment",
				})
			} else {
				dataCounter++
				data := &model.Data{
					ID:   "d" + strconv.Itoa(dataCounter),
					Name: "comment",
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
			}
		}
	}
}

// writeSkeletonEntries writes skeleton text and ref entries from the collected ranges.
// It sorts all ranges by start offset, removes overlapping parent ranges, and
// interleaves skeleton text with refs.
func (r *Reader) writeSkeletonEntries(content []byte, contentRanges []skelContentRange, attrRanges []skelAttrRange) {
	// Merge content and attr ranges into a unified sorted list.
	refs := make([]skelRefEntry, 0, len(contentRanges)+len(attrRanges))
	for _, cr := range contentRanges {
		refs = append(refs, skelRefEntry(cr))
	}
	for _, ar := range attrRanges {
		refs = append(refs, skelRefEntry(ar))
	}
	// Sort by start offset.
	slices.SortFunc(refs, func(a, b skelRefEntry) int {
		return a.start - b.start
	})

	// Remove parent ranges that contain child ranges (overlapping nesting).
	// A parent range [pStart, pEnd) that fully contains a child range [cStart, cEnd)
	// should be removed — the child range's ref handles the translatable content,
	// and the structural bytes between/around it are skeleton text.
	refs = removeOverlappingParents(refs)

	pos := 0
	for _, ref := range refs {
		if ref.start > pos {
			_ = r.skeletonStore.WriteText(content[pos:ref.start])
		}
		_ = r.skeletonStore.WriteRef(ref.blockID)
		pos = ref.end
	}
	if pos < len(content) {
		_ = r.skeletonStore.WriteText(content[pos:])
	}
}

// skelRefEntry is a unified skeleton reference used in writeSkeletonEntries.
type skelRefEntry struct {
	blockID string
	start   int
	end     int
}

// removeOverlappingParents filters out ranges that fully contain other ranges.
// Refs must be sorted by start offset. Uses a stack to achieve O(n) complexity.
func removeOverlappingParents(refs []skelRefEntry) []skelRefEntry {
	if len(refs) <= 1 {
		return refs
	}
	// Mark ranges that are parents (fully contain another range) for removal.
	// Since refs are sorted by start, a parent always appears before or at the
	// same position as its children. We use a stack of indices whose end has not
	// yet been passed. When a new ref starts inside a stacked ref, the stacked
	// ref is a parent and is marked for removal.
	remove := make([]bool, len(refs))
	// stack holds indices of refs whose end we haven't passed yet.
	stack := make([]int, 0, 8)
	for i := range refs {
		// Pop entries that end before the current ref starts — they can't be
		// parents of this ref.
		for len(stack) > 0 && refs[stack[len(stack)-1]].end <= refs[i].start {
			stack = stack[:len(stack)-1]
		}
		// Any remaining entries on the stack started at or before refs[i].start
		// and end after it — check if they fully contain refs[i].
		for _, si := range stack {
			if refs[si].start <= refs[i].start && refs[i].end <= refs[si].end {
				remove[si] = true
			}
		}
		stack = append(stack, i)
	}
	result := make([]skelRefEntry, 0, len(refs))
	for i, ref := range refs {
		if !remove[i] {
			result = append(result, ref)
		}
	}
	return result
}

// findCloseTagStart finds the byte offset where the closing tag starts (the '<' of '</tag>')
// by searching backwards from endOffset. endOffset is after the '>' of the end tag.
func findCloseTagStart(data []byte, searchStart, endOffset int, tagName string) int {
	if endOffset > len(data) {
		endOffset = len(data)
	}
	closeTag := []byte("</" + tagName)
	segment := data[searchStart:endOffset]
	idx := bytes.LastIndex(segment, closeTag)
	if idx < 0 {
		return -1
	}
	return searchStart + idx
}

// findAttrValueByteRange finds the byte range of an attribute value within a start tag.
// It searches for attrName="value" pattern in the raw bytes between tagStart and tagEnd.
// Returns (start, end) offsets in the content, where start is the first byte of the value
// and end is after the last byte of the value (inside the quotes).
func findAttrValueByteRange(content []byte, tagStart, tagEnd int, attrName, attrValue string) (int, int) {
	if tagEnd > len(content) {
		tagEnd = len(content)
	}
	segment := content[tagStart:tagEnd]

	// Search for the attribute name followed by = and quoted value.
	attrBytes := []byte(attrName)
	idx := 0
	for {
		pos := bytes.Index(segment[idx:], attrBytes)
		if pos < 0 {
			return -1, -1
		}
		pos += idx

		// Skip past the attribute name.
		afterName := pos + len(attrBytes)
		if afterName >= len(segment) {
			return -1, -1
		}

		// Skip whitespace.
		for afterName < len(segment) && (segment[afterName] == ' ' || segment[afterName] == '\t' || segment[afterName] == '\n' || segment[afterName] == '\r') {
			afterName++
		}
		if afterName >= len(segment) || segment[afterName] != '=' {
			idx = pos + 1
			continue
		}
		afterName++ // skip '='

		// Skip whitespace after '='.
		for afterName < len(segment) && (segment[afterName] == ' ' || segment[afterName] == '\t' || segment[afterName] == '\n' || segment[afterName] == '\r') {
			afterName++
		}
		if afterName >= len(segment) {
			return -1, -1
		}

		quote := segment[afterName]
		if quote != '"' && quote != '\'' {
			idx = pos + 1
			continue
		}
		valueStart := afterName + 1

		// Find closing quote.
		valueEnd := bytes.IndexByte(segment[valueStart:], quote)
		if valueEnd < 0 {
			return -1, -1
		}
		valueEnd += valueStart

		return tagStart + valueStart, tagStart + valueEnd
	}
}

// isInlineExcluded checks if an inline element is also excluded.
func (r *Reader) isInlineExcluded(name string, attrs map[string]string) bool {
	for _, rule := range r.cfg.ElementRules {
		if rule.Matches(name) && rule.HasRule(RuleInline) && rule.HasRule(RuleExclude) {
			if rule.Condition != nil {
				return rule.Condition.Evaluate(attrs)
			}
			return true
		}
	}
	return false
}

// buildStartTag reconstructs the start tag XML string from a StartElement.
func buildStartTag(se xml.StartElement) string {
	var buf strings.Builder
	buf.WriteByte('<')
	buf.WriteString(se.Name.Local)
	for _, attr := range se.Attr {
		buf.WriteByte(' ')
		if attr.Name.Space != "" {
			buf.WriteString(attr.Name.Space)
			buf.WriteByte(':')
		}
		buf.WriteString(attr.Name.Local)
		buf.WriteString(`="`)
		buf.WriteString(attr.Value)
		buf.WriteByte('"')
	}
	buf.WriteByte('>')
	return buf.String()
}

// isMarkerRune returns true if the rune is a span marker character.
func isMarkerRune(r rune) bool {
	return r == model.MarkerOpening || r == model.MarkerClosing || r == model.MarkerPlaceholder
}

// collapseFragmentWhitespace applies whitespace collapsing to a fragment,
// preserving span markers and their positions.
func collapseFragmentWhitespace(f *model.Fragment) *model.Fragment {
	if f == nil {
		return nil
	}
	result := &model.Fragment{
		Spans: f.Spans,
	}
	var buf strings.Builder
	inSpace := false
	started := false

	for _, r := range f.CodedText {
		if isMarkerRune(r) {
			if inSpace && started {
				buf.WriteByte(' ')
				inSpace = false
			}
			buf.WriteRune(r)
			started = true
		} else if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if started {
				inSpace = true
			}
		} else {
			if inSpace {
				buf.WriteByte(' ')
				inSpace = false
			}
			buf.WriteRune(r)
			started = true
		}
	}
	result.CodedText = buf.String()
	return result
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// matchSubfilter checks if the given element path matches any configured subfilter mapping.
func (r *Reader) matchSubfilter(path string) *format.SubfilterMapping {
	for i := range r.cfg.Subfilters {
		sf := &r.cfg.Subfilters[i]
		if matchGlob(sf.Pattern, path) {
			return sf
		}
	}
	return nil
}

// emitSubfiltered emits a child layer with content parsed by the subfilter format reader.
func (r *Reader) emitSubfiltered(ctx context.Context, ch chan<- model.PartResult, content, path, parentLayerID string, mapping *format.SubfilterMapping, blockCounter, dataCounter *int) {
	subReader, err := r.resolver.ResolveReader(mapping.Format)
	if err != nil {
		// Fall back to plain block
		*blockCounter++
		block := model.NewBlock("tu" + strconv.Itoa(*blockCounter), content)
		block.Name = path
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		return
	}

	r.layerSeq++
	childLayerID := "sf" + strconv.Itoa(r.layerSeq)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	childLayer := &model.Layer{
		ID:       childLayerID,
		Name:     path,
		Format:   mapping.Format,
		Locale:   locale,
		ParentID: parentLayerID,
		Properties: map[string]string{
			"subfilter.source":      "xml",
			"subfilter.elementPath": path,
		},
	}

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
		return
	}

	subDoc := &model.RawDocument{
		URI:          path,
		SourceLocale: locale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(content))),
	}
	if err := subReader.Open(ctx, subDoc); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xml: subfilter open for %s: %w", path, err)}
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
		return
	}

	for pr := range subReader.Read(ctx) {
		if pr.Error != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xml: subfilter read for %s: %w", path, pr.Error)}
			break
		}
		// Skip the sub-reader's document-level layer events
		if pr.Part.Type == model.PartLayerStart || pr.Part.Type == model.PartLayerEnd {
			if l, ok := pr.Part.Resource.(*model.Layer); ok && l.IsRoot() {
				continue
			}
		}
		r.emit(ctx, ch, pr.Part)
	}
	subReader.Close()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
}

// matchGlob matches a path against a glob pattern using dot-separated segments.
func matchGlob(pattern, path string) bool {
	patternNorm := strings.ReplaceAll(pattern, ".", "/")
	pathNorm := strings.ReplaceAll(path, ".", "/")
	matched, _ := filepath.Match(patternNorm, pathNorm)
	return matched
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
