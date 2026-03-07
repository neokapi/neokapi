package xml

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for XML files.
type Reader struct {
	format.BaseFormatReader
	cfg      *Config
	resolver format.SubfilterResolver
	layerSeq int
}

// Ensure Reader implements SubfilterAware.
var _ format.SubfilterAware = (*Reader)(nil)

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
	name       string
	attrs      map[string]string
	isInline   bool
	isExcluded bool
	preserveWS bool
	frag       *model.Fragment
	spanID     int
	hasContent bool // true if inline element had any child content
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
	flushBlock := func(frame *elementFrame, path string) {
		if frame == nil || frame.frag == nil {
			return
		}

		var finalFrag *model.Fragment
		if frame.preserveWS {
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
				ID:   fmt.Sprintf("d%d", dataCounter),
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
		block := &model.Block{
			ID:           fmt.Sprintf("tu%d", blockCounter),
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
		frame.frag = nil
	}

	for {
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
					ID:         fmt.Sprintf("d%d", dataCounter),
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

			frame := &elementFrame{
				name:       t.Name.Local,
				attrs:      attrs,
				isInline:   isInline,
				isExcluded: isExcluded || inlineExcluded,
				preserveWS: preserveWS,
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
						ID:       fmt.Sprintf("%d", spanCounter),
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
					block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), attr.Value)
					block.Name = elemPath() + "@" + attrName
					block.Type = "attribute"
					block.IsReferent = true
					r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
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

			if frame.isInline {
				parent := findTextFrame()
				if parent != nil && parent.frag != nil && !parent.isExcluded {
					if !frame.hasContent {
						// Self-closing / empty inline: replace the opening span with a placeholder
						spanID := fmt.Sprintf("%d", frame.spanID)
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
							ID:       fmt.Sprintf("%d", frame.spanID),
							Data:     "</" + t.Name.Local + ">",
							Type:     "fmt:" + t.Name.Local,
						})
					}
				}
			} else {
				// Flush accumulated text as a block
				if !frame.isExcluded {
					flushBlock(frame, path)
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
			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), trimmed)
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
					ID:       fmt.Sprintf("%d", spanCounter),
					Data:     piData,
					Type:     "xml:pi",
				})
			} else {
				dataCounter++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
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
					ID:       fmt.Sprintf("%d", spanCounter),
					Data:     "<!--" + string(t) + "-->",
					Type:     "xml:comment",
				})
			} else {
				dataCounter++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: "comment",
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
			}
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
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
		block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), content)
		block.Name = path
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		return
	}

	r.layerSeq++
	childLayerID := fmt.Sprintf("sf%d", r.layerSeq)

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
