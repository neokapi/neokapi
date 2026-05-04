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
	"github.com/neokapi/neokapi/core/its"
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
	name       string
	// qname is the element's prefix:localname form as it appears in
	// the source (`z:汇集`, `its:rules`, or just `myDoc` when
	// unprefixed). Used by skeleton-tracking code to find the matching
	// closing tag in the source bytes — searching for `</localname`
	// alone misses prefixed elements like `</z:汇集>` and the closing
	// search returns -1 (no skeleton range, content stays untranslated
	// in the round-trip).
	qname      string
	// localName / nsURI capture the element's name decomposition so
	// the ITS resolver can match against namespace-qualified names.
	// `name` keeps the legacy local-name-only form used by every
	// existing predicate to avoid touching the (substantial) call
	// sites in this file.
	localName  string
	nsURI      string
	attrs      map[string]string
	isInline   bool
	isExcluded bool
	// strongExclude is set when the exclusion comes from an ITS
	// `translate="no"` attribute. Strong exclusion propagates to every
	// descendant and drops their text on the floor regardless of
	// whether the descendant frame is itself marked excluded — this is
	// what makes `<its:rules its:translate="no">` correctly suppress
	// `<its:locNote>` text inside it. Distinct from isExcluded because
	// config-driven exclusion (ExcludeByDefault) is overridable by
	// descendant INCLUDE rules and must NOT drop text the same way.
	strongExclude    bool
	preserveWS       bool
	runs             []model.Run // nil means no content accumulator yet (inline element parent w/o text frame)
	hasRuns          bool        // true once the frame has been initialised as a text accumulator
	spanID           int
	hasContent       bool // true if inline element had any child content
	contentByteStart int  // byte offset where element content begins (after '>'), for skeleton
	// ITS locNote (literal text + type + ref) attached to this element,
	// resolved from local its:locNote* attributes and/or matching
	// its:locNoteRule. Surfaced on emitted blocks so writers / tools
	// can carry the note through to translation tooling.
	itsLocNote     string
	itsLocNoteType string
	itsLocNoteRef  string
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

	// Pre-scan for embedded ITS rules so element processing can
	// honor translateRule, withinTextRule, locNoteRule, etc. Rules
	// embedded later in the document override earlier ones (ITS 2.0
	// §5.4 last-rule-wins). External (xlink:href) rule documents are
	// not loaded yet — we surface the references and consume them
	// once a future change adds the resolver.
	itsRules, _, err := its.ExtractRules(content)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xml: parsing ITS rules: %w", err)}
		return
	}
	resolver := its.NewResolver(itsRules)

	if r.skeletonStore != nil {
		r.readContentSkeleton(ctx, ch, content, layer, resolver)
	} else {
		r.readContentSimple(ctx, ch, content, layer, resolver)
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

func (r *Reader) readContentSimple(ctx context.Context, ch chan<- model.PartResult, content []byte, layer *model.Layer, resolver *its.Resolver) {
	_ = r.readContentCore(ctx, ch, content, layer, nil, nil, resolver)
}

func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult, content []byte, layer *model.Layer, resolver *its.Resolver) {
	// For skeleton mode, we do the normal parse but also track byte positions.
	// After parsing, we write skeleton entries using the collected ranges.
	// The key: only leaf block elements get skeleton refs. Parent containers
	// that don't produce blocks (or produce blocks with only spans/no text)
	// have their content preserved as skeleton text.

	var contentRanges []skelContentRange
	var attrRanges []skelAttrRange
	itsRanges := r.readContentCore(ctx, ch, content, layer, &contentRanges, &attrRanges, resolver)

	// Drop any block content range that would overwrite an
	// `<its:rules>` element — those bytes must round-trip verbatim
	// per ITS 2.0 §5. The parent block (typically a <head>) has no
	// translatable text once the rules are stripped, and we want the
	// skeleton text to preserve it untouched.
	if len(itsRanges) > 0 {
		contentRanges = filterContentRangesContainingITSRules(contentRanges, itsRanges)
	}

	// Write skeleton entries from the collected ranges.
	r.writeSkeletonEntries(content, contentRanges, attrRanges)
}

// filterContentRangesContainingITSRules drops content ranges that
// fully contain any `<its:rules>` byte range. ITS rules elements are
// document metadata and must round-trip verbatim; if we let a parent
// block range cover them, the writer would substitute the rendered
// block text and lose the rules.
func filterContentRangesContainingITSRules(ranges []skelContentRange, itsRanges []skelByteRange) []skelContentRange {
	out := ranges[:0]
	for _, cr := range ranges {
		drop := false
		for _, ir := range itsRanges {
			if cr.start <= ir.start && ir.end <= cr.end {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, cr)
		}
	}
	return out
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

	// itsResolver, when non-nil, evaluates ITS rules (translateRule,
	// withinTextRule, locNoteRule, …) against each element. Built
	// from the document's <its:rules> blocks during the pre-pass.
	itsResolver *its.Resolver

	// itsRulesRanges tracks the byte ranges of `<its:rules>` elements
	// encountered during streaming. The pre-pass already extracted the
	// rules; we skip them in the streaming reader so they don't pollute
	// the parent element's text accumulator. Their bytes still belong
	// in the output (per ITS 2.0 — rules are document-level metadata
	// that round-trip). After parsing, writeSkeletonEntries drops any
	// content range that fully contains an itsRulesRange so the
	// surrounding skeleton text preserves the rules verbatim.
	itsRulesRanges []skelByteRange
}

// skelByteRange is a half-open [start, end) byte range in the source.
type skelByteRange struct {
	start int
	end   int
}

// itsContext builds an ElementContext from the current parse stack
// for the ITS resolver. Reuses the existing path-stack rather than
// maintaining a parallel structure.
func (s *xmlParseState) itsContext(thisName its.NameMatch, thisAttrs []its.Attribute) *its.ElementContext {
	path := make([]its.NameMatch, 0, len(s.stack)+1)
	for _, f := range s.stack {
		path = append(path, its.NameMatch{
			NamespaceURI: f.nsURI,
			Local:        f.localName,
		})
	}
	path = append(path, thisName)
	return &its.ElementContext{Path: path, Attributes: thisAttrs}
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

// isWhitespaceOnly returns true when s contains only ASCII whitespace
// (space, tab, CR, LF). Used by flushBlock to skip emitting blocks
// for elements whose only "text" is inter-element whitespace around
// excluded children — those bytes belong in skeleton, not in a block
// content range.
func isWhitespaceOnly(s string) bool {
	for i := range len(s) {
		switch s[i] {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return false
		}
	}
	return true
}

// hasStrongExcludeAncestor returns true if any frame on the stack is
// strongly excluded (via ITS `translate="no"`). Used by character-data
// handling to drop text inside `<its:rules its:translate="no">` even
// when the immediate text frame is unmarked — the text frame's own
// `isExcluded=false` is correct (no INCLUDE override needed) but the
// ancestor's strong exclusion still applies.
func (s *xmlParseState) hasStrongExcludeAncestor() bool {
	for _, f := range s.stack {
		if f.strongExclude {
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

	// Whitespace-only text without inline codes isn't translatable.
	// This case fires when an element wraps an excluded subtree
	// (e.g. `<info>\n  <its:rules its:translate="no">...</its:rules>\n </info>`)
	// — the only "text" left in the parent's frame is the
	// inter-element whitespace. Emitting a block here would attach a
	// content range covering the entire `<info>` interior, and the
	// writer would then replace the excluded subtree's bytes with the
	// joined whitespace on merge, dropping the structural content.
	// Letting it fall through keeps the whole interior in skeleton
	// text where the writer preserves it verbatim. preserveWS skips
	// this filter — when whitespace is explicitly significant we
	// want the block.
	if !frame.preserveWS && !runsHaveInlineCodes(finalRuns) && isWhitespaceOnly(text) {
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

	// Attach ITS-resolved metadata that was captured at element-start
	// (locNote text, term flag, …). The frame carries the resolved
	// values so flushBlock doesn't need to re-evaluate.
	if frame.itsLocNote != "" {
		block.Properties["locNote"] = frame.itsLocNote
		if frame.itsLocNoteType != "" {
			block.Properties["locNoteType"] = frame.itsLocNoteType
		}
	}
	if frame.itsLocNoteRef != "" {
		block.Properties["locNoteRef"] = frame.itsLocNoteRef
	}

	s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartBlock, Resource: block})

	// Track content range for skeleton
	if s.contentRanges != nil && frame.contentByteStart > 0 && endTagOffset > 0 {
		// Find the start of the end tag by searching backwards from
		// endTagOffset. Use the source qname (`z:汇集`) when available
		// — searching for the local name alone (`汇集`) misses the
		// `z:` prefix in `</z:汇集>` and the search returns -1, so
		// the block's content range is dropped from skeleton and the
		// translation never gets substituted into the output.
		closeName := frame.qname
		if closeName == "" {
			closeName = frame.name
		}
		closeStart := findCloseTagStart(s.content, frame.contentByteStart, endTagOffset, closeName)
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
	// Build the ITS context once for this element so attribute-targeted
	// rules (`<its:translateRule selector="//@alt"/>`) can be evaluated.
	thisName := its.NameMatch{NamespaceURI: elem.Name.Space, Local: elem.Name.Local}
	itsAttrs := buildITSAttributes(elem.Attr)
	itsCtx := s.itsContext(thisName, itsAttrs)

	for _, attr := range elem.Attr {
		attrName := attr.Name.Local
		if attr.Name.Space == "xml" {
			attrName = "xml:" + attr.Name.Local
		} else if attr.Name.Space != "" {
			attrName = attr.Name.Space + ":" + attr.Name.Local
		}
		// ITS attribute rules win over cfg defaults (translate="yes"
		// promotes a non-translatable attribute; translate="no"
		// suppresses one cfg would have emitted). Attributes in the
		// ITS namespace itself never become content blocks — they are
		// metadata.
		if attr.Name.Space == its.NamespaceURI {
			continue
		}
		translate := s.itsAttrTranslate(itsCtx, attr)
		var include bool
		switch translate {
		case its.Yes:
			include = true
		case its.No:
			include = false
		default:
			include = s.reader.cfg.isTranslatableAttribute(elem.Name.Local, attrName, s.stack[len(s.stack)-1].attrs)
		}
		if !include {
			continue
		}
		s.blockCounter++
		blockID := "tu" + strconv.Itoa(s.blockCounter)
		block := model.NewBlock(blockID, attr.Value)
		block.Name = s.elemPath() + "@" + attrName
		block.Type = "attribute"
		block.IsReferent = true
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartBlock, Resource: block})

		// Track attribute range for skeleton
		if s.attrRanges != nil {
			// Skip skeleton attr ranges when this element will be
			// rendered as an inline placeholder inside a parent block.
			// The parent's content range already covers the attribute's
			// bytes, and the writer would emit overlapping refs
			// otherwise. We still emit the block so future writers can
			// surface it (e.g. for human review), but it won't round-
			// trip into the inline tag yet — that requires the writer
			// to expand inline placeholders against referent blocks,
			// which lives outside the scope of this change.
			if s.elementWillRenderInline(elem) {
				continue
			}
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

// elementWillRenderInline reports whether the element being processed
// will end up as inline content of a parent text-bearing frame. Used
// by emitTranslatableAttrs to suppress overlapping skeleton ranges
// for attributes that the parent block's content range already covers.
func (s *xmlParseState) elementWillRenderInline(elem xml.StartElement) bool {
	// The frame for this element is already pushed (handleStartElement
	// pushes before calling emitTranslatableAttrs). The frame is inline
	// iff its isInline flag is true AND a non-inline ancestor exists
	// further up the stack to absorb the inline content.
	if len(s.stack) == 0 {
		return false
	}
	top := s.stack[len(s.stack)-1]
	if !top.isInline {
		return false
	}
	for i := len(s.stack) - 2; i >= 0; i-- {
		if !s.stack[i].isInline {
			return s.stack[i].hasRuns
		}
	}
	return false
}

// itsAttrTranslate returns the ITS translate decision for one
// attribute on the element described by ctx, or its.Unset when the
// resolver has no opinion (no matching rule and no local override).
func (s *xmlParseState) itsAttrTranslate(ctx *its.ElementContext, attr xml.Attr) its.Tristate {
	if s.itsResolver == nil {
		return its.Unset
	}
	resolved := s.itsResolver.ResolveAttribute(
		ctx,
		its.NameMatch{NamespaceURI: attr.Name.Space, Local: attr.Name.Local},
		nil,
	)
	return resolved.Translate
}

// buildITSAttributes converts an xml.StartElement attribute slice
// into the ITS resolver's Attribute slice for predicate evaluation.
// Namespace-qualified attributes carry their URI; unqualified
// attributes pass through with empty NamespaceURI.
func buildITSAttributes(attrs []xml.Attr) []its.Attribute {
	out := make([]its.Attribute, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, its.Attribute{
			Name:  its.NameMatch{NamespaceURI: a.Name.Space, Local: a.Name.Local},
			Value: a.Value,
		})
	}
	return out
}

// handleStartElement processes an xml.StartElement token.
func (s *xmlParseState) handleStartElement(t xml.StartElement, tokOffset int) {
	// `<its:rules>` is document-level metadata (per W3C ITS 2.0 §5).
	// The pre-pass already extracted every rule it carries, so we skip
	// the element entirely here — pushing a frame would pollute the
	// parent's text accumulator (whitespace + comments around the
	// rules block would land in the parent's runs). Recording the
	// byte range lets writeSkeletonEntries preserve the rules verbatim
	// in skeleton text by suppressing any content range that would
	// otherwise overwrite it.
	if t.Name.Space == its.NamespaceURI && t.Name.Local == "rules" {
		if err := s.decoder.Skip(); err != nil {
			s.ch <- model.PartResult{Error: fmt.Errorf("xml: skipping its:rules: %w", err)}
			return
		}
		end := int(s.decoder.InputOffset())
		s.itsRulesRanges = append(s.itsRulesRanges, skelByteRange{start: tokOffset, end: end})
		return
	}

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

	// ITS resolution combines global rules (translateRule,
	// withinTextRule, locNoteRule, …) with the element's own local
	// attributes per ITS 2.0 §5.4 precedence (locals win, last rule
	// wins among rules). Inheritance for translate / preserveSpace
	// is handled by the parent stack (we read parent.strongExclude
	// below); rules that match the element override inheritance.
	strongExclude := false
	if len(s.stack) > 0 && s.stack[len(s.stack)-1].strongExclude {
		strongExclude = true
	}
	itsAttrs := buildITSAttributes(t.Attr)
	thisName := its.NameMatch{NamespaceURI: t.Name.Space, Local: t.Name.Local}
	itsCtx := s.itsContext(thisName, itsAttrs)
	localITS := its.LocalAttributesFrom(attrs)
	resolved := s.itsResolver.ResolveElement(itsCtx, &localITS)
	if resolved.Translate == its.No {
		strongExclude = true
		isExcluded = true
	} else if resolved.Translate == its.Yes {
		strongExclude = false
		isExcluded = false
	}
	if resolved.WithinText == its.Yes {
		isInline = true
	} else if resolved.WithinText == its.No {
		isInline = false
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
		qname:            extractElementQName(s.content, tokOffset, t.Name.Local),
		localName:        t.Name.Local,
		nsURI:            t.Name.Space,
		attrs:            attrs,
		isInline:         isInline,
		isExcluded:       isExcluded || inlineExcluded,
		strongExclude:    strongExclude,
		preserveWS:       preserveWS,
		contentByteStart: contentStart,
		itsLocNote:       resolved.LocNoteText,
		itsLocNoteType:   string(resolved.LocNoteType),
		itsLocNoteRef:    resolved.LocNoteRef,
	}

	if isInline {
		// When the inline element is excluded by an ITS translate="no"
		// rule, capture the entire subtree (including children, text,
		// markup) as a single opaque Ph using the source bytes. The
		// text inside isn't translatable; the structure inside should
		// round-trip verbatim. Without this, descendant text gets
		// dropped (hasStrongExcludeAncestor) and inline child markup
		// gets emitted into the parent's runs without the surrounding
		// excluded text — producing `<del><img/></del>` instead of
		// `<del> the icon <img/></del>`.
		if strongExclude {
			parent := s.findTextFrame()
			// Skip the entire subtree on the decoder. Decoder.Skip
			// consumes through the matching end element.
			if err := s.decoder.Skip(); err != nil {
				s.ch <- model.PartResult{Error: fmt.Errorf("xml: skipping excluded inline %s: %w", t.Name.Local, err)}
				return
			}
			endOffset := int(s.decoder.InputOffset())
			// Capture verbatim source bytes for `<tag ...>...</tag>`
			// (or `<tag .../>` for self-closing).
			subtree := ""
			if tokOffset >= 0 && endOffset > tokOffset && endOffset <= len(s.content) {
				subtree = string(s.content[tokOffset:endOffset])
			}
			if parent != nil && parent.hasRuns && !parent.isExcluded {
				// Mark parent inline ancestors as having content.
				for i := len(s.stack) - 1; i >= 0; i-- {
					if s.stack[i].isInline {
						s.stack[i].hasContent = true
					} else {
						break
					}
				}
				s.spanCounter++
				parent.runs = append(parent.runs, model.Run{Ph: &model.PlaceholderRun{
					ID:   strconv.Itoa(s.spanCounter),
					Type: "fmt:" + t.Name.Local,
					Data: subtree,
				}})
			}
			// Pop the wsStack push from above (no end-element will
			// fire for this skipped element).
			if len(s.wsStack) > 0 {
				s.wsStack = s.wsStack[:len(s.wsStack)-1]
			}
			return
		}
		// Mark parent inline elements as having content
		for i := len(s.stack) - 1; i >= 0; i-- {
			if s.stack[i].isInline {
				s.stack[i].hasContent = true
			} else {
				break
			}
		}
		// For inline elements, add opening span to parent's fragment.
		// Use the original source bytes for the start tag rather than
		// reconstructing from xml.StartElement: Go's encoding/xml
		// decoder unescapes attribute entities and replaces namespace
		// prefixes with URIs. Reconstructing produces invalid XML
		// (`<b http://www.w3.org/2005/11/its:translate="no">`) and
		// loses entity escaping (`<img attr1="&=amp"/>`).
		parent := s.findTextFrame()
		if parent != nil && parent.hasRuns && !parent.isExcluded {
			s.spanCounter++
			id := strconv.Itoa(s.spanCounter)
			parent.runs = append(parent.runs, model.Run{PcOpen: &model.PcOpenRun{
				ID:   id,
				Type: "fmt:" + t.Name.Local,
				Data: s.startTagBytes(t, tokOffset, contentStart),
			}})
			frame.spanID = s.spanCounter
		}
	} else {
		// Non-inline element. When marked translate="no" inside a
		// text-bearing parent block, treat it the same way as an
		// inline+excluded element: capture the entire subtree as a
		// single Ph in the parent's runs using verbatim source bytes.
		// Without this, the child's content range never tracks
		// (excluded frames don't flush blocks), the parent's content
		// range covers the child's bytes but parent's translation
		// has no placeholder for them, and the child element is
		// dropped from output entirely.
		if strongExclude {
			parent := s.findTextFrame()
			if parent != nil && parent.hasRuns && !parent.isExcluded {
				if err := s.decoder.Skip(); err != nil {
					s.ch <- model.PartResult{Error: fmt.Errorf("xml: skipping excluded %s: %w", t.Name.Local, err)}
					return
				}
				endOffset := int(s.decoder.InputOffset())
				subtree := ""
				if tokOffset >= 0 && endOffset > tokOffset && endOffset <= len(s.content) {
					subtree = string(s.content[tokOffset:endOffset])
				}
				s.spanCounter++
				parent.runs = append(parent.runs, model.Run{Ph: &model.PlaceholderRun{
					ID:   strconv.Itoa(s.spanCounter),
					Type: "fmt:" + t.Name.Local,
					Data: subtree,
				}})
				if len(s.wsStack) > 0 {
					s.wsStack = s.wsStack[:len(s.wsStack)-1]
				}
				return
			}
		}
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
				// Rewrite the captured start-tag (`<tag attrs>`) into the
				// self-closing form (`<tag attrs/>`) so the writer emits
				// the same shape the source had — preserving the empty
				// element shape okapi readers expect from `<img/>`-style
				// inline placeholders.
				spanID := strconv.Itoa(frame.spanID)
				for i, r := range parent.runs {
					if r.PcOpen != nil && r.PcOpen.ID == spanID {
						parent.runs[i] = model.Run{Ph: &model.PlaceholderRun{
							ID:   spanID,
							Type: r.PcOpen.Type,
							Data: selfCloseStartTag(r.PcOpen.Data),
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

	// Strongly excluded subtree (e.g. `<its:rules its:translate="no">`)
	// drops every descendant's text — descendants do not get an
	// INCLUDE override, the exclusion is intentional ITS metadata.
	if s.hasStrongExcludeAncestor() {
		return
	}

	// If in excluded scope (config-driven, e.g. ExcludeByDefault), the
	// existing logic stands: drop text only when the immediate text
	// frame is itself excluded. Descendant INCLUDE overrides re-enable
	// extraction by clearing isExcluded on the relevant frame.
	if s.isInExcludedScope() {
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
	contentRanges *[]skelContentRange, attrRanges *[]skelAttrRange, resolver *its.Resolver) []skelByteRange {

	s := &xmlParseState{
		reader:        r,
		ctx:           ctx,
		ch:            ch,
		content:       content,
		layer:         layer,
		contentRanges: contentRanges,
		attrRanges:    attrRanges,
		decoder:       xml.NewDecoder(strings.NewReader(string(content))),
		itsResolver:   resolver,
	}

	for {
		tokOffset := int(s.decoder.InputOffset())
		tok, err := s.decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xml: parsing: %w", err)}
			return nil
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
	return s.itsRulesRanges
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

// extractElementQName returns the prefix:localname form of the element
// whose start tag begins at tokOffset. Falls back to localName when the
// source bytes are unavailable or unparseable. Used to find matching
// close tags in source bytes — `</prefix:localname>` won't be found if
// we search for just `</localname>`.
func extractElementQName(content []byte, tokOffset int, localName string) string {
	if tokOffset < 0 || tokOffset >= len(content) {
		return localName
	}
	// Source must begin with '<' followed by the qname. Walk forward
	// until whitespace or '>' or '/'.
	if content[tokOffset] != '<' {
		return localName
	}
	end := tokOffset + 1
	for end < len(content) {
		c := content[end]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '>' || c == '/' {
			break
		}
		end++
	}
	qname := string(content[tokOffset+1 : end])
	if qname == "" {
		return localName
	}
	return qname
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

// selfCloseStartTag rewrites an open-form start tag (`<tag attrs>`)
// into the self-closing form (`<tag attrs/>`). Used when an inline
// element turns out to have no content so the captured open-tag bytes
// can stand in for the original `<tag attrs/>` source.
func selfCloseStartTag(s string) string {
	if !strings.HasSuffix(s, ">") || strings.HasSuffix(s, "/>") {
		return s
	}
	return s[:len(s)-1] + "/>"
}

// buildStartTag reconstructs the start tag XML string from a StartElement.
//
// This is a fallback used only when source bytes are unavailable (e.g.
// programmatically-constructed events). The reconstruction loses two
// pieces of information the parser strips:
//
//   - Namespace prefixes are replaced with the resolved URI in
//     attr.Name.Space (e.g. `its:translate` becomes `http://www.w3.org/...:translate`),
//     producing invalid XML.
//   - Attribute values come back entity-decoded (`&amp;` → `&`,
//     `&quot;` → `"`), so re-emitting them verbatim breaks XML.
//
// Inline-element start tags use s.startTagBytes instead, which copies
// the original source bytes and preserves both prefixes and entities.
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

// startTagBytes returns the original source bytes for an element's
// start tag in open form (`<tag attrs>`). This preserves namespace
// prefixes (the parser replaces them with URIs in attr.Name.Space)
// and entity-escaped attribute values (the parser unescapes them).
//
// When source bytes are unavailable or the byte range is invalid,
// falls back to buildStartTag with its known limitations.
//
// Self-closing source (`<tag attrs/>`) is normalized to open form
// (`<tag attrs>`) so the writer's selfCloseStartTag transform works
// uniformly on the captured bytes.
func (s *xmlParseState) startTagBytes(t xml.StartElement, tokOffset, contentStart int) string {
	if tokOffset < 0 || contentStart <= tokOffset || contentStart > len(s.content) {
		return buildStartTag(t)
	}
	raw := s.content[tokOffset:contentStart]
	// Normalize self-closing form (`<tag/>` or `<tag />`) to open form
	// (`<tag>`) so the writer's open/close inline-code shape is
	// consistent. The writer rewrites empty inlines back to self-close
	// via selfCloseStartTag — that path expects an open-form tag.
	if n := len(raw); n >= 2 && raw[n-1] == '>' {
		j := n - 2
		// Skip optional whitespace between attributes and `/>`
		for j > 0 && (raw[j] == ' ' || raw[j] == '\t' || raw[j] == '\r' || raw[j] == '\n') {
			j--
		}
		if raw[j] == '/' {
			open := make([]byte, 0, n-1)
			open = append(open, raw[:j]...)
			open = append(open, '>')
			return string(open)
		}
	}
	return string(raw)
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
