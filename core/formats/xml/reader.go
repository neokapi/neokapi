package xml

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
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
		return errors.New("xml: nil document or reader")
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
	runs             []model.Run // nil means no content accumulator yet (inline element parent w/o text frame)
	hasRuns          bool        // true once the frame has been initialised as a text accumulator
	spanID           int
	hasContent       bool // true if inline element had any child content
	contentByteStart int  // byte offset where element content begins (after '>'), for skeleton
}

// initRuns marks the frame as a text accumulator. Subsequent addText /
// inline-code events append into frame.runs.
func (f *elementFrame) initRuns() {
	if !f.hasRuns {
		f.runs = nil
		f.hasRuns = true
	}
}

// resetRuns clears the frame's accumulator after its content has been
// emitted (or when the frame is discarded).
func (f *elementFrame) resetRuns() {
	f.runs = nil
	f.hasRuns = false
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

// xmlParseState holds the mutable state for streaming XML parsing in readContentCore.
type xmlParseState struct {
	reader        *Reader
	ctx           context.Context
	ch            chan<- model.PartResult
	content       []byte
	layer         *model.Layer
	contentRanges *[]skelContentRange
	attrRanges    *[]skelAttrRange
	decoder       *xml.Decoder
	blockCounter  int
	dataCounter   int
	spanCounter   int
	stack         []*elementFrame
	wsStack       []bool
}

// findTextFrame returns the nearest non-inline ancestor frame.
func (s *xmlParseState) findTextFrame() *elementFrame {
	for i := len(s.stack) - 1; i >= 0; i-- {
		if !s.stack[i].isInline {
			return s.stack[i]
		}
	}
	return nil
}

// isInExcludedScope checks if any ancestor is excluded (but not inline+excluded).
func (s *xmlParseState) isInExcludedScope() bool {
	for _, f := range s.stack {
		if f.isExcluded {
			return true
		}
	}
	return false
}

// elemPath builds the dot-separated path for the current element stack.
func (s *xmlParseState) elemPath() string {
	var parts []string
	for _, f := range s.stack {
		parts = append(parts, f.name)
	}
	return strings.Join(parts, ".")
}

// isTranslatable checks if the given frame's content is translatable.
func (s *xmlParseState) isTranslatable(frame *elementFrame) bool {
	cfg := s.reader.cfg
	if cfg.ExcludeByDefault {
		return cfg.isIncludedElement(frame.name, frame.attrs)
	}
	if cfg.isExcludedElement(frame.name, frame.attrs) {
		return false
	}
	if len(cfg.TranslatableElements) > 0 {
		for _, e := range cfg.TranslatableElements {
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
func (s *xmlParseState) flushBlock(frame *elementFrame, path string, endTagOffset int) {
	if frame == nil || !frame.hasRuns {
		return
	}

	var finalRuns []model.Run
	if frame.preserveWS || s.contentRanges != nil {
		// In skeleton mode, preserve whitespace as-is for byte-exact roundtrip.
		// The skeleton ref covers the raw bytes, and XML-escaping the decoded
		// text will reproduce the original encoding.
		finalRuns = frame.runs
	} else {
		finalRuns = collapseRunsWhitespace(frame.runs)
	}

	text := model.FlattenRuns(finalRuns)
	if text == "" && !runsHaveInlineCodes(finalRuns) {
		return
	}

	if !s.isTranslatable(frame) {
		// Emit as data part
		s.dataCounter++
		data := &model.Data{
			ID:   "d" + strconv.Itoa(s.dataCounter),
			Name: path,
		}
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartData, Resource: data})
		return
	}

	// Check for subfilter
	if mapping := s.reader.matchSubfilter(path); mapping != nil && s.reader.resolver != nil {
		s.reader.emitSubfiltered(s.ctx, s.ch, text, path, s.layer.ID, mapping, &s.blockCounter, &s.dataCounter)
		frame.resetRuns()
		return
	}

	s.blockCounter++
	blockID := "tu" + strconv.Itoa(s.blockCounter)
	block := &model.Block{
		ID:           blockID,
		Translatable: true,
		Source:       []*model.Segment{model.NewRunsSegment("s1", finalRuns)},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}

	block.Name = path

	// Set block name from ID attribute if available
	idVal := s.reader.cfg.getIDAttribute(frame.name, frame.attrs)
	if idVal != "" {
		block.Name = idVal
	}

	// Set block type
	block.Type = s.reader.cfg.getBlockType(frame.name)

	// Set PreserveWhitespace
	block.PreserveWhitespace = frame.preserveWS

	// Set writable attributes as properties
	writableAttrs := s.reader.cfg.getWritableAttributes(frame.name, frame.attrs)
	for k, v := range writableAttrs {
		block.Properties[k] = v
	}

	s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartBlock, Resource: block})

	// Track content range for skeleton
	if s.contentRanges != nil && frame.contentByteStart > 0 && endTagOffset > 0 {
		// Find the start of the end tag by searching backwards from endTagOffset
		closeStart := findCloseTagStart(s.content, frame.contentByteStart, endTagOffset, frame.name)
		if closeStart >= 0 {
			*s.contentRanges = append(*s.contentRanges, skelContentRange{
				blockID: blockID,
				start:   frame.contentByteStart,
				end:     closeStart,
			})
		}
	}

	frame.resetRuns()
}

// emitTranslatableAttrs emits translatable attributes as blocks and tracks skeleton ranges.
func (s *xmlParseState) emitTranslatableAttrs(elem xml.StartElement, tokOffset, contentStart int) {
	for _, attr := range elem.Attr {
		attrName := attr.Name.Local
		if attr.Name.Space == "xml" {
			attrName = "xml:" + attr.Name.Local
		} else if attr.Name.Space != "" {
			attrName = attr.Name.Space + ":" + attr.Name.Local
		}
		if s.reader.cfg.isTranslatableAttribute(elem.Name.Local, attrName, s.stack[len(s.stack)-1].attrs) {
			s.blockCounter++
			blockID := "tu" + strconv.Itoa(s.blockCounter)
			block := model.NewBlock(blockID, attr.Value)
			block.Name = s.elemPath() + "@" + attrName
			block.Type = "attribute"
			block.IsReferent = true
			s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartBlock, Resource: block})

			// Track attribute range for skeleton
			if s.attrRanges != nil {
				attrStart, attrEnd := findAttrValueByteRange(s.content, tokOffset, contentStart, attrName, attr.Value)
				if attrStart >= 0 {
					*s.attrRanges = append(*s.attrRanges, skelAttrRange{
						blockID: blockID,
						start:   attrStart,
						end:     attrEnd,
					})
				}
			}
		}
	}
}

// handleStartElement processes an xml.StartElement token.
func (s *xmlParseState) handleStartElement(t xml.StartElement, tokOffset int) {
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
		s.dataCounter++
		data := &model.Data{
			ID:         "d" + strconv.Itoa(s.dataCounter),
			Name:       t.Name.Local,
			Properties: map[string]string{"language": lang},
		}
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartData, Resource: data})
	}

	isInline := s.reader.cfg.isInlineElement(t.Name.Local)
	isExcluded := s.reader.cfg.isExcludedElement(t.Name.Local, attrs)

	// Check excludeByDefault
	if s.reader.cfg.ExcludeByDefault && !s.reader.cfg.isIncludedElement(t.Name.Local, attrs) {
		isExcluded = true
	}

	// An INCLUDE inside an excluded parent overrides
	if s.isInExcludedScope() && s.reader.cfg.isIncludedElement(t.Name.Local, attrs) {
		isExcluded = false
	}

	// Check xml:space
	preserveWS := s.reader.cfg.shouldPreserveWhitespace(t.Name.Local)
	if v, ok := attrs["xml:space"]; ok {
		preserveWS = v == "preserve"
	}
	// Inherit from parent
	if len(s.wsStack) > 0 && s.wsStack[len(s.wsStack)-1] {
		preserveWS = true
	}
	s.wsStack = append(s.wsStack, preserveWS)

	// Check if inline+excluded (content suppressed but element still inline)
	inlineExcluded := isInline && s.reader.isInlineExcluded(t.Name.Local, attrs)

	// Content starts right after the '>' of the start tag
	contentStart := int(s.decoder.InputOffset())

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
		for i := len(s.stack) - 1; i >= 0; i-- {
			if s.stack[i].isInline {
				s.stack[i].hasContent = true
			} else {
				break
			}
		}
		// For inline elements, add opening span to parent's fragment
		parent := s.findTextFrame()
		if parent != nil && parent.hasRuns && !parent.isExcluded {
			s.spanCounter++
			id := strconv.Itoa(s.spanCounter)
			parent.runs = append(parent.runs, model.Run{PcOpen: &model.PcOpenRun{
				ID:   id,
				Type: "fmt:" + t.Name.Local,
				Data: buildStartTag(t),
			}})
			frame.spanID = s.spanCounter
		}
	} else {
		// Start a new text accumulator for this block element.
		frame.initRuns()
	}

	s.stack = append(s.stack, frame)

	// Emit translatable attributes as blocks
	s.emitTranslatableAttrs(t, tokOffset, contentStart)
}

// handleEndElement processes an xml.EndElement token.
func (s *xmlParseState) handleEndElement(t xml.EndElement) {
	if len(s.wsStack) > 0 {
		s.wsStack = s.wsStack[:len(s.wsStack)-1]
	}
	if len(s.stack) == 0 {
		return
	}
	frame := s.stack[len(s.stack)-1]
	// Compute the path before popping
	path := s.elemPath()
	s.stack = s.stack[:len(s.stack)-1]

	// endTagOffset is the byte offset after the end tag
	endTagOffset := int(s.decoder.InputOffset())

	if frame.isInline {
		parent := s.findTextFrame()
		if parent != nil && parent.hasRuns && !parent.isExcluded {
			if !frame.hasContent {
				// Self-closing / empty inline: replace the opening run with a Ph.
				spanID := strconv.Itoa(frame.spanID)
				for i, r := range parent.runs {
					if r.PcOpen != nil && r.PcOpen.ID == spanID {
						parent.runs[i] = model.Run{Ph: &model.PlaceholderRun{
							ID:   spanID,
							Type: r.PcOpen.Type,
							Data: r.PcOpen.Data,
						}}
						break
					}
				}
			} else {
				// Add closing run to parent's accumulator.
				parent.runs = append(parent.runs, model.Run{PcClose: &model.PcCloseRun{
					ID:   strconv.Itoa(frame.spanID),
					Type: "fmt:" + t.Name.Local,
					Data: "</" + t.Name.Local + ">",
				}})
			}
		}
	} else if !frame.isExcluded {
		// Flush accumulated text as a block
		s.flushBlock(frame, path, endTagOffset)
	}
}

// handleCharData processes an xml.CharData token.
func (s *xmlParseState) handleCharData(t xml.CharData) {
	text := string(t)

	// If in excluded scope, check what kind
	if s.isInExcludedScope() {
		// Check if the nearest non-inline ancestor is excluded
		textFrame := s.findTextFrame()
		if textFrame == nil || textFrame.isExcluded {
			return
		}
		// The text frame is not excluded, but an inline ancestor is.
		// Skip text from any excluded inline element in the ancestor chain.
		for i := len(s.stack) - 1; i >= 0; i-- {
			if !s.stack[i].isInline {
				break
			}
			if s.stack[i].isExcluded {
				return
			}
		}
	}

	// Find the frame that should accumulate this text
	textFrame := s.findTextFrame()

	if textFrame != nil {
		// Mark all inline ancestors as having content
		for i := len(s.stack) - 1; i >= 0; i-- {
			if s.stack[i].isInline {
				s.stack[i].hasContent = true
			} else {
				break
			}
		}
		// Accumulate text in the current text frame.
		textFrame.initRuns()
		textFrame.runs = appendTextRun(textFrame.runs, text)
		return
	}

	// No parent frame — standalone text (shouldn't normally happen with well-formed XML)
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	path := s.elemPath()
	s.blockCounter++
	block := model.NewBlock("tu"+strconv.Itoa(s.blockCounter), trimmed)
	block.Name = path
	s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// handleProcInst processes an xml.ProcInst token.
func (s *xmlParseState) handleProcInst(t xml.ProcInst) {
	textFrame := s.findTextFrame()
	if textFrame != nil && textFrame.hasRuns {
		s.spanCounter++
		piData := "<?" + t.Target
		if len(t.Inst) > 0 {
			piData += " " + string(t.Inst)
		}
		piData += "?>"
		textFrame.runs = append(textFrame.runs, model.Run{Ph: &model.PlaceholderRun{
			ID:   strconv.Itoa(s.spanCounter),
			Type: "xml:pi",
			Data: piData,
		}})
	} else {
		s.dataCounter++
		data := &model.Data{
			ID:   "d" + strconv.Itoa(s.dataCounter),
			Name: "processing-instruction",
		}
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartData, Resource: data})
	}
}

// handleComment processes an xml.Comment token.
func (s *xmlParseState) handleComment(t xml.Comment) {
	textFrame := s.findTextFrame()
	if textFrame != nil && textFrame.hasRuns {
		s.spanCounter++
		textFrame.runs = append(textFrame.runs, model.Run{Ph: &model.PlaceholderRun{
			ID:   strconv.Itoa(s.spanCounter),
			Type: "xml:comment",
			Data: "<!--" + string(t) + "-->",
		}})
	} else {
		s.dataCounter++
		data := &model.Data{
			ID:   "d" + strconv.Itoa(s.dataCounter),
			Name: "comment",
		}
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartData, Resource: data})
	}
}

func (r *Reader) readContentCore(ctx context.Context, ch chan<- model.PartResult, content []byte, layer *model.Layer,
	contentRanges *[]skelContentRange, attrRanges *[]skelAttrRange) {

	s := &xmlParseState{
		reader:        r,
		ctx:           ctx,
		ch:            ch,
		content:       content,
		layer:         layer,
		contentRanges: contentRanges,
		attrRanges:    attrRanges,
		decoder:       xml.NewDecoder(strings.NewReader(string(content))),
	}

	for {
		tokOffset := int(s.decoder.InputOffset())
		tok, err := s.decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xml: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			s.handleStartElement(t, tokOffset)
		case xml.EndElement:
			s.handleEndElement(t)
		case xml.CharData:
			s.handleCharData(t)
		case xml.ProcInst:
			s.handleProcInst(t)
		case xml.Comment:
			s.handleComment(t)
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

// appendTextRun appends plain text to a run slice, coalescing with
// the previous run if it is also a TextRun.
func appendTextRun(runs []model.Run, text string) []model.Run {
	if text == "" {
		return runs
	}
	if n := len(runs); n > 0 && runs[n-1].Text != nil {
		runs[n-1].Text.Text += text
		return runs
	}
	return append(runs, model.Run{Text: &model.TextRun{Text: text}})
}

// runsHaveInlineCodes reports whether the run slice contains any
// non-text run. Used by flushBlock to decide whether a segment with
// no flattened text is still worth emitting (e.g. a <br/> run alone).
func runsHaveInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// collapseRunsWhitespace applies whitespace collapsing to a run
// sequence, preserving inline-code runs and their positions.
// Mirrors the legacy collapseFragmentWhitespace semantics.
func collapseRunsWhitespace(runs []model.Run) []model.Run {
	if len(runs) == 0 {
		return runs
	}
	// Walk TextRuns and collapse whitespace across the entire
	// sequence. Track "in space" across run boundaries so an inline
	// code between two whitespace-padded text runs doesn't suppress
	// the single space we want to emit.
	out := make([]model.Run, 0, len(runs))
	inSpace := false
	started := false
	for _, r := range runs {
		if r.Text == nil {
			// Non-text run: if we have a pending space and any text
			// has already been emitted, flush a single space first.
			if inSpace && started {
				out = appendTextRun(out, " ")
				inSpace = false
			}
			started = true
			out = append(out, r)
			continue
		}
		var buf strings.Builder
		for _, ch := range r.Text.Text {
			if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
				if started {
					inSpace = true
				}
			} else {
				if inSpace {
					buf.WriteByte(' ')
					inSpace = false
				}
				buf.WriteRune(ch)
				started = true
			}
		}
		if buf.Len() > 0 {
			out = appendTextRun(out, buf.String())
		}
	}
	return out
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
		block := model.NewBlock("tu"+strconv.Itoa(*blockCounter), content)
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
