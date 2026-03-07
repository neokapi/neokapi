package html

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// inlineElements are HTML elements treated as inline (Spans, not block boundaries).
var inlineElements = map[atom.Atom]bool{
	atom.A: true, atom.Abbr: true, atom.Acronym: true, atom.B: true,
	atom.Bdo: true, atom.Big: true, atom.Br: true, atom.Button: true,
	atom.Cite: true, atom.Code: true, atom.Del: true, atom.Dfn: true,
	atom.Em: true, atom.Font: true, atom.I: true, atom.Img: true,
	atom.Input: true, atom.Ins: true, atom.Kbd: true, atom.Label: true,
	atom.Q: true, atom.S: true, atom.Samp: true, atom.Small: true,
	atom.Span: true, atom.Strike: true, atom.Strong: true,
	atom.Sub: true, atom.Sup: true, atom.Tt: true,
	atom.U: true, atom.Var: true, atom.Wbr: true,
}

// selfClosingElements are void HTML elements.
var selfClosingElements = map[atom.Atom]bool{
	atom.Area: true, atom.Base: true, atom.Br: true, atom.Col: true,
	atom.Embed: true, atom.Hr: true, atom.Img: true, atom.Input: true,
	atom.Link: true, atom.Meta: true, atom.Param: true, atom.Source: true,
	atom.Track: true, atom.Wbr: true,
}

// nonTranslatableElements contain content that is not translatable.
var nonTranslatableElements = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true,
}

// preserveWhitespaceElements preserve whitespace by default.
var preserveWhitespaceElements = map[atom.Atom]bool{
	atom.Pre: true, atom.Textarea: true,
}

// blockTypeMap maps HTML element names to block types.
var blockTypeMap = map[string]string{
	"p": "paragraph", "pre": "pre", "h1": "heading",
	"h2": "heading", "h3": "heading", "h4": "heading",
	"h5": "heading", "h6": "heading", "li": "listitem",
	"td": "cell", "th": "cell", "dt": "term", "dd": "definition",
	"title": "title", "caption": "caption", "figcaption": "caption",
	"blockquote": "quote", "address": "address",
}

// translatableMetaNames are META name values whose content is translatable.
var translatableMetaNames = map[string]bool{
	"keywords": true, "description": true,
	"twitter:title": true, "twitter:description": true,
	"og:title": true, "og:description": true, "og:site_name": true,
}

// htmlSemanticTypes maps HTML element names to vocabulary semantic types.
var htmlSemanticTypes = map[string]string{
	"b": "fmt:bold", "strong": "fmt:bold",
	"i": "fmt:italic", "em": "fmt:italic",
	"u": "fmt:underline",
	"s": "fmt:strikethrough", "del": "fmt:strikethrough", "strike": "fmt:strikethrough",
	"a":      "link:hyperlink",
	"code":   "fmt:code", "kbd": "fmt:code", "samp": "fmt:code", "tt": "fmt:code",
	"sub":    "fmt:subscript", "sup": "fmt:superscript",
	"mark":   "fmt:highlight",
	"br":     "struct:break", "hr": "struct:break",
	"img":    "media:image",
	"button": "ui:button",
}

// Reader implements DataFormatReader for HTML files.
type Reader struct {
	format.BaseFormatReader
	cfg   *Config
	vocab *model.VocabularyRegistry
}

// NewReader creates a new HTML reader.
func NewReader() *Reader {
	cfg := &Config{}
	vocab := model.NewVocabularyRegistry()
	_ = vocab.LoadDefaults()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "html",
			FormatDisplayName: "HTML",
			FormatMimeType:    "text/html",
			FormatExtensions:  []string{".html", ".htm", ".xhtml"},
			Cfg:               cfg,
		},
		cfg:   cfg,
		vocab: vocab,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/html", "application/xhtml+xml"},
		Extensions: []string{".html", ".htm", ".xhtml"},
		MagicBytes: [][]byte{[]byte("<!DOCTYPE"), []byte("<!doctype"), []byte("<html"), []byte("<HTML")},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("html: nil document or reader")
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
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "html",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/html",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("html: reading: %w", err)}
		return
	}

	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("html: parsing: %w", err)}
		return
	}

	state := &readerState{
		blockCounter: 0,
		dataCounter:  0,
	}

	r.walkNode(ctx, ch, doc, state, false)

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// readerState tracks mutable counters during tree traversal.
type readerState struct {
	blockCounter int
	dataCounter  int
}

// walkNode traverses the HTML tree emitting Parts.
func (r *Reader) walkNode(ctx context.Context, ch chan<- model.PartResult, n *html.Node, state *readerState, translateNo bool) {
	switch n.Type {
	case html.DocumentNode:
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			r.walkNode(ctx, ch, child, state, translateNo)
		}

	case html.DoctypeNode:
		state.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", state.dataCounter),
			Name: "doctype",
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})

	case html.CommentNode:
		state.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", state.dataCounter),
			Name: "comment",
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})

	case html.ElementNode:
		r.walkElement(ctx, ch, n, state, translateNo)

	case html.TextNode:
		if translateNo {
			return
		}
		text := n.Data
		if !r.cfg.PreserveWhitespace {
			text = collapseWhitespace(text)
			text = strings.TrimFunc(text, isHTMLWhitespace)
		}
		if text != "" {
			state.blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", state.blockCounter), text)
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
	}
}

// walkElement handles element nodes with all feature logic.
func (r *Reader) walkElement(ctx context.Context, ch chan<- model.PartResult, n *html.Node, state *readerState, translateNo bool) {
	// Check translate attribute on the element itself.
	elemTranslateNo := translateNo
	if tv := getAttr(n, "translate"); tv != "" {
		if tv == "no" {
			elemTranslateNo = true
		} else if tv == "yes" {
			elemTranslateNo = false
		}
	}

	// Non-translatable elements (script, style) become Data parts.
	if nonTranslatableElements[n.DataAtom] {
		state.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", state.dataCounter),
			Name: n.Data,
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		return
	}

	// META tag handling.
	if n.DataAtom == atom.Meta {
		r.handleMetaTag(ctx, ch, n, state)
		return
	}

	// Extract language/encoding from lang, xml:lang attributes.
	r.extractLangAttribute(ctx, ch, n, state)

	// Extract translatable attributes (title, alt, label, placeholder, value on certain inputs).
	r.extractTranslatableAttributes(ctx, ch, n, state, elemTranslateNo)

	// Block-level element handling.
	if !isInlineElement(n) {
		// Check if this element is excluded via translate="no".
		if elemTranslateNo {
			// Still recurse in case children have translate="yes".
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				r.walkNode(ctx, ch, child, state, elemTranslateNo)
			}
			return
		}

		hasBlockChildren := r.hasBlockLevelChildren(n)

		if hasBlockChildren {
			// Mixed content: process runs of inline content between block elements.
			r.processBlockWithMixedContent(ctx, ch, n, state, elemTranslateNo)
			return
		}

		if r.hasAnyContent(n) || getAttr(n, "id") != "" {
			preserveWS := r.cfg.PreserveWhitespace || preserveWhitespaceElements[n.DataAtom]
			frag := r.collectInlineContent(ctx, ch, n, preserveWS, elemTranslateNo, state)

			hasID := getAttr(n, "id") != ""
			fragOK := frag != nil && !frag.IsEmpty()
			if fragOK || hasID {
				text := ""
				if frag != nil {
					text = frag.Text()
				}
				if text != "" || (frag != nil && frag.HasSpans()) || hasID {
					if frag == nil {
						frag = &model.Fragment{}
					}
					state.blockCounter++
					blockID := fmt.Sprintf("tu%d", state.blockCounter)
					block := &model.Block{
						ID:                 blockID,
						Name:               r.blockName(n),
						Type:               r.blockType(n),
						Translatable:       true,
						PreserveWhitespace: preserveWS,
						Source:             []*model.Segment{{ID: "s1", Content: frag}},
						Targets:            make(map[model.LocaleID][]*model.Segment),
						Properties:         r.extractBlockProperties(n),
						Annotations:        make(map[string]model.Annotation),
						Skeleton: &model.Skeleton{
							Strategy: model.SkeletonFragmentBased,
							Parts: []model.SkeletonPart{
								&model.SkeletonText{Text: r.renderOpenTag(n)},
								&model.SkeletonRef{ResourceID: blockID, Property: "target"},
								&model.SkeletonText{Text: fmt.Sprintf("</%s>", n.Data)},
							},
						},
					}
					r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
					return
				}
			}
		}
	}

	// Container element without direct text: recurse into children.
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		r.walkNode(ctx, ch, child, state, elemTranslateNo)
	}
}

// handleMetaTag processes <meta> tags for translatable content, language, and encoding.
func (r *Reader) handleMetaTag(ctx context.Context, ch chan<- model.PartResult, n *html.Node, state *readerState) {
	httpEquiv := strings.ToLower(getAttr(n, "http-equiv"))
	metaName := strings.ToLower(getAttr(n, "name"))
	content := getAttr(n, "content")
	charset := getAttr(n, "charset")

	// charset attribute → encoding Data part.
	if charset != "" {
		state.dataCounter++
		data := &model.Data{
			ID:         fmt.Sprintf("d%d", state.dataCounter),
			Name:       "meta",
			Properties: map[string]string{"encoding": charset},
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		return
	}

	// http-equiv="Content-Type" with charset → encoding Data part.
	if httpEquiv == "content-type" && content != "" {
		if cs := extractCharset(content); cs != "" {
			state.dataCounter++
			data := &model.Data{
				ID:         fmt.Sprintf("d%d", state.dataCounter),
				Name:       "meta",
				Properties: map[string]string{"encoding": cs},
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
			return
		}
	}

	// http-equiv="Content-Language" → language Data part.
	if httpEquiv == "content-language" && content != "" {
		state.dataCounter++
		data := &model.Data{
			ID:         fmt.Sprintf("d%d", state.dataCounter),
			Name:       "meta",
			Properties: map[string]string{"language": content},
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		return
	}

	// Translatable META content: keywords, description, twitter:*, og:*.
	if content != "" {
		isTranslatable := false
		if httpEquiv == "keywords" || translatableMetaNames[metaName] {
			isTranslatable = true
		}
		if httpEquiv == "keywords" {
			isTranslatable = true
		}

		if isTranslatable {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			block := &model.Block{
				ID:           blockID,
				Name:         metaName,
				Type:         "content",
				Translatable: true,
				IsReferent:   true,
				Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(content)}},
				Targets:      make(map[model.LocaleID][]*model.Segment),
				Properties:   make(map[string]string),
				Annotations:  make(map[string]model.Annotation),
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
	}

	// Emit a Data part for the meta tag structure.
	state.dataCounter++
	data := &model.Data{
		ID:   fmt.Sprintf("d%d", state.dataCounter),
		Name: "meta",
	}
	r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
}

// extractTranslatableAttributes extracts translatable attribute values as blocks.
func (r *Reader) extractTranslatableAttributes(ctx context.Context, ch chan<- model.PartResult, n *html.Node, state *readerState, translateNo bool) {
	if translateNo {
		return
	}

	// title attribute is translatable on all elements.
	if title := getAttr(n, "title"); title != "" {
		state.blockCounter++
		blockID := fmt.Sprintf("tu%d", state.blockCounter)
		block := &model.Block{
			ID:           blockID,
			Type:         "title",
			Translatable: true,
			IsReferent:   true,
			Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(title)}},
			Targets:      make(map[model.LocaleID][]*model.Segment),
			Properties:   make(map[string]string),
			Annotations:  make(map[string]model.Annotation),
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	// alt attribute is translatable on img, input, area.
	if alt := getAttr(n, "alt"); alt != "" {
		if n.DataAtom == atom.Img || n.DataAtom == atom.Input || n.DataAtom == atom.Area {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			block := &model.Block{
				ID:           blockID,
				Type:         "alt",
				Translatable: true,
				IsReferent:   true,
				Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(alt)}},
				Targets:      make(map[model.LocaleID][]*model.Segment),
				Properties:   make(map[string]string),
				Annotations:  make(map[string]model.Annotation),
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
	}

	// label attribute on option.
	if label := getAttr(n, "label"); label != "" {
		if n.DataAtom == atom.Option {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			block := &model.Block{
				ID:           blockID,
				Type:         "label",
				Translatable: true,
				IsReferent:   true,
				Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(label)}},
				Targets:      make(map[model.LocaleID][]*model.Segment),
				Properties:   make(map[string]string),
				Annotations:  make(map[string]model.Annotation),
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
	}

	// placeholder attribute on input.
	if ph := getAttr(n, "placeholder"); ph != "" {
		if n.DataAtom == atom.Input || n.DataAtom == atom.Textarea {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			block := &model.Block{
				ID:           blockID,
				Type:         "placeholder",
				Translatable: true,
				IsReferent:   true,
				Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(ph)}},
				Targets:      make(map[model.LocaleID][]*model.Segment),
				Properties:   make(map[string]string),
				Annotations:  make(map[string]model.Annotation),
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
	}

	// value attribute on input (type-dependent).
	if val := getAttr(n, "value"); val != "" && n.DataAtom == atom.Input {
		inputType := strings.ToLower(getAttr(n, "type"))
		// value is translatable for submit, button, reset and generic types (not file, hidden, radio, checkbox, image).
		if isTranslatableInputValue(inputType) {
			state.blockCounter++
			blockID := fmt.Sprintf("tu%d", state.blockCounter)
			block := &model.Block{
				ID:           blockID,
				Type:         "value",
				Translatable: true,
				IsReferent:   true,
				Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(val)}},
				Targets:      make(map[model.LocaleID][]*model.Segment),
				Properties:   make(map[string]string),
				Annotations:  make(map[string]model.Annotation),
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
	}
}

// isTranslatableInputValue returns true if the input type has a translatable value attribute.
func isTranslatableInputValue(inputType string) bool {
	switch inputType {
	case "file", "hidden", "radio", "checkbox", "image":
		return false
	case "submit", "button", "reset":
		return true
	default:
		// For other/unknown types, extract value.
		return true
	}
}

// extractLangAttribute emits Data parts for lang/xml:lang attributes.
func (r *Reader) extractLangAttribute(ctx context.Context, ch chan<- model.PartResult, n *html.Node, state *readerState) {
	lang := getAttr(n, "lang")
	if lang == "" {
		lang = getAttrNS(n, "xml", "lang")
	}
	if lang != "" {
		state.dataCounter++
		data := &model.Data{
			ID:         fmt.Sprintf("d%d", state.dataCounter),
			Name:       n.Data,
			Properties: map[string]string{"language": lang},
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}
}

// collectInlineContent collects all text and inline elements from a block element
// into a single Fragment with Spans using sequential IDs and semantic types.
func (r *Reader) collectInlineContent(ctx context.Context, ch chan<- model.PartResult, n *html.Node, preserveWS bool, translateNo bool, state *readerState) *model.Fragment {
	frag := &model.Fragment{}
	spanCounter := 0
	r.collectFromNode(ctx, ch, n, frag, &spanCounter, preserveWS, translateNo, state)

	// Apply whitespace collapsing to the fragment text if needed.
	if !preserveWS {
		frag.CodedText = collapseWhitespaceCodedText(frag.CodedText)
		frag.CodedText = trimCodedText(frag.CodedText)
	}

	return frag
}

func (r *Reader) collectFromNode(ctx context.Context, ch chan<- model.PartResult, n *html.Node, frag *model.Fragment, spanCounter *int, preserveWS bool, translateNo bool, state *readerState) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			frag.AppendText(child.Data)

		case html.CommentNode:
			*spanCounter++
			id := strconv.Itoa(*spanCounter)
			frag.AppendSpan(&model.Span{
				SpanType: model.SpanPlaceholder,
				Type:     "code:comment",
				SubType:  "html:comment",
				ID:       id,
				Data:     "<!--" + child.Data + "-->",
			})

		case html.ElementNode:
			// Extract translatable attributes from inline elements inside blocks.
			r.extractTranslatableAttributes(ctx, ch, child, state, translateNo)

			// Non-translatable elements inside a block become placeholder spans.
			if nonTranslatableElements[child.DataAtom] {
				*spanCounter++
				id := strconv.Itoa(*spanCounter)
				frag.AppendSpan(&model.Span{
					SpanType: model.SpanPlaceholder,
					Type:     "code:markup",
					SubType:  "html:" + child.Data,
					ID:       id,
					Data:     renderNodeHTML(child),
				})
				continue
			}

			// Check translate attribute on inline elements.
			childTranslateNo := translateNo
			if tv := getAttr(child, "translate"); tv != "" {
				if tv == "no" {
					childTranslateNo = true
				} else if tv == "yes" {
					childTranslateNo = false
				}
			}

			if isInlineElement(child) {
				// If this inline element has translate="no" and the parent context is translatable,
				// emit the entire subtree as a placeholder span — unless a descendant has translate="yes".
				if childTranslateNo && !translateNo && !hasDescendantTranslateYes(child) {
					*spanCounter++
					id := strconv.Itoa(*spanCounter)
					frag.AppendSpan(&model.Span{
						SpanType: model.SpanPlaceholder,
						Type:     "code:markup",
						SubType:  "html:" + child.Data,
						ID:       id,
						Data:     renderNodeHTML(child),
					})
					continue
				}

				semType := htmlSemanticType(child.Data)
				subType := "html:" + child.Data

				if selfClosingElements[child.DataAtom] {
					*spanCounter++
					id := strconv.Itoa(*spanCounter)
					info := r.vocab.LookupOrFallback(semType)
					frag.AppendSpan(&model.Span{
						SpanType:    model.SpanPlaceholder,
						Type:        semType,
						SubType:     subType,
						ID:          id,
						Data:        r.renderTag(child),
						DisplayText: info.Display.Placeholder,
						EquivText:   info.Equiv,
						Deletable:   info.Constraints.Deletable,
						Cloneable:   info.Constraints.Cloneable,
						CanReorder:  info.Constraints.Reorderable,
					})
				} else {
					*spanCounter++
					id := strconv.Itoa(*spanCounter)
					info := r.vocab.LookupOrFallback(semType)
					frag.AppendSpan(&model.Span{
						SpanType:    model.SpanOpening,
						Type:        semType,
						SubType:     subType,
						ID:          id,
						Data:        r.renderOpenTag(child),
						DisplayText: info.Display.Open,
						EquivText:   info.Equiv,
						Deletable:   info.Constraints.Deletable,
						Cloneable:   info.Constraints.Cloneable,
						CanReorder:  info.Constraints.Reorderable,
					})
					r.collectFromNode(ctx, ch, child, frag, spanCounter, preserveWS, childTranslateNo, state)
					frag.AppendSpan(&model.Span{
						SpanType:    model.SpanClosing,
						Type:        semType,
						SubType:     subType,
						ID:          id,
						Data:        fmt.Sprintf("</%s>", child.Data),
						DisplayText: info.Display.Close,
						EquivText:   info.Equiv,
						Deletable:   info.Constraints.Deletable,
						Cloneable:   info.Constraints.Cloneable,
						CanReorder:  info.Constraints.Reorderable,
					})
				}
			} else {
				// Non-inline element inside a block (e.g., <ul> inside <p>):
				// End current block content and let the parent walkNode handle it.
				// We don't recurse here — the parent's walkNode will handle children.
			}
		}
	}
}

// hasBlockLevelChildren returns true if the node has any non-inline element children.
func (r *Reader) hasBlockLevelChildren(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && !isInlineElement(child) && !nonTranslatableElements[child.DataAtom] {
			return true
		}
	}
	return false
}

// processBlockWithMixedContent handles block elements that have both block and inline children.
// It creates blocks for runs of inline/text content between block elements, then recurses into block elements.
func (r *Reader) processBlockWithMixedContent(ctx context.Context, ch chan<- model.PartResult, n *html.Node, state *readerState, translateNo bool) {
	preserveWS := r.cfg.PreserveWhitespace || preserveWhitespaceElements[n.DataAtom]

	// Process children, grouping consecutive inline/text nodes into blocks.
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode || (child.Type == html.ElementNode && isInlineElement(child)) {
			// Collect a run of inline content.
			frag := &model.Fragment{}
			spanCounter := 0
			for child != nil && (child.Type == html.TextNode ||
				child.Type == html.CommentNode ||
				(child.Type == html.ElementNode && isInlineElement(child))) {
				switch child.Type {
				case html.TextNode:
					frag.AppendText(child.Data)
				case html.CommentNode:
					spanCounter++
					frag.AppendSpan(&model.Span{
						SpanType: model.SpanPlaceholder,
						Type:     "code:comment",
						SubType:  "html:comment",
						ID:       strconv.Itoa(spanCounter),
						Data:     "<!--" + child.Data + "-->",
					})
				case html.ElementNode:
					r.extractTranslatableAttributes(ctx, ch, child, state, translateNo)
					r.collectFromNode(ctx, ch, child, frag, &spanCounter, preserveWS, translateNo, state)
				}
				child = child.NextSibling
			}

			if !preserveWS {
				frag.CodedText = collapseWhitespaceCodedText(frag.CodedText)
				frag.CodedText = trimCodedText(frag.CodedText)
			}
			text := frag.Text()
			if text != "" || frag.HasSpans() {
				state.blockCounter++
				blockID := fmt.Sprintf("tu%d", state.blockCounter)
				block := &model.Block{
					ID:                 blockID,
					Name:               r.blockName(n),
					Type:               r.blockType(n),
					Translatable:       true,
					PreserveWhitespace: preserveWS,
					Source:             []*model.Segment{{ID: "s1", Content: frag}},
					Targets:            make(map[model.LocaleID][]*model.Segment),
					Properties:         r.extractBlockProperties(n),
					Annotations:        make(map[string]model.Annotation),
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
			}

			// child has been advanced past the inline run; if nil, break.
			if child == nil {
				break
			}
			// Fall through to process this block-level child below.
		}

		// Block-level element child: recurse via walkNode.
		r.walkNode(ctx, ch, child, state, translateNo)
	}
}

// hasAnyContent returns true if the node contains any text or inline element content,
// checking recursively through inline elements.
func (r *Reader) hasAnyContent(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			if strings.TrimFunc(child.Data, isHTMLWhitespace) != "" {
				return true
			}
		}
		if child.Type == html.ElementNode {
			if isInlineElement(child) {
				if selfClosingElements[child.DataAtom] {
					return true
				}
				if r.hasAnyContent(child) {
					return true
				}
			}
		}
	}
	return false
}

// hasPlaceholderContent returns true if node has only self-closing inline children (no text).
func (r *Reader) hasPlaceholderContent(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && isInlineElement(child) && selfClosingElements[child.DataAtom] {
			return true
		}
	}
	return false
}

// hasDescendantTranslateYes checks if any descendant element has translate="yes".
func hasDescendantTranslateYes(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			if getAttr(child, "translate") == "yes" {
				return true
			}
			if hasDescendantTranslateYes(child) {
				return true
			}
		}
	}
	return false
}

// htmlSemanticType returns the vocabulary semantic type for an HTML element name.
func htmlSemanticType(element string) string {
	if t, ok := htmlSemanticTypes[strings.ToLower(element)]; ok {
		return t
	}
	return "code:markup"
}

// blockType returns the block type for an element.
func (r *Reader) blockType(n *html.Node) string {
	if t, ok := blockTypeMap[strings.ToLower(n.Data)]; ok {
		return t
	}
	return ""
}

// blockName returns the block name for an element, incorporating id attribute.
func (r *Reader) blockName(n *html.Node) string {
	if id := getAttr(n, "id"); id != "" {
		return id + "-id"
	}
	return n.Data
}

// extractBlockProperties returns properties from the element's attributes.
func (r *Reader) extractBlockProperties(n *html.Node) map[string]string {
	props := make(map[string]string)
	if id := getAttr(n, "id"); id != "" {
		props["id"] = id
	}
	if dir := getAttr(n, "dir"); dir != "" {
		props["dir"] = dir
	}
	return props
}

// collapseWhitespace collapses runs of HTML whitespace into single spaces.
func collapseWhitespace(s string) string {
	var buf strings.Builder
	inSpace := false
	for _, r := range s {
		if isHTMLWhitespace(r) {
			if !inSpace {
				buf.WriteRune(' ')
				inSpace = true
			}
		} else {
			buf.WriteRune(r)
			inSpace = false
		}
	}
	return buf.String()
}

// collapseWhitespaceCodedText collapses HTML whitespace in coded text, preserving span markers.
func collapseWhitespaceCodedText(s string) string {
	var buf strings.Builder
	inSpace := false
	for _, r := range s {
		if r == model.MarkerOpening || r == model.MarkerClosing || r == model.MarkerPlaceholder {
			buf.WriteRune(r)
			// Don't change space state for markers.
			continue
		}
		if isHTMLWhitespace(r) {
			if !inSpace {
				buf.WriteRune(' ')
				inSpace = true
			}
		} else {
			buf.WriteRune(r)
			inSpace = false
		}
	}
	return buf.String()
}

// isHTMLWhitespace returns true for HTML whitespace characters (space, tab, newline, carriage return, form feed).
// Unlike unicode.IsSpace, this does NOT include non-breaking space (\u00A0).
func isHTMLWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

// trimCodedText trims leading and trailing HTML whitespace from coded text,
// preserving span markers at the boundaries.
func trimCodedText(s string) string {
	runes := []rune(s)
	start := 0
	end := len(runes)
	// Trim leading whitespace (skip markers).
	for start < end {
		r := runes[start]
		if r == model.MarkerOpening || r == model.MarkerClosing || r == model.MarkerPlaceholder {
			break
		}
		if isHTMLWhitespace(r) {
			start++
		} else {
			break
		}
	}
	// Trim trailing whitespace (skip markers).
	for end > start {
		r := runes[end-1]
		if r == model.MarkerOpening || r == model.MarkerClosing || r == model.MarkerPlaceholder {
			break
		}
		if isHTMLWhitespace(r) {
			end--
		} else {
			break
		}
	}
	return string(runes[start:end])
}

func (r *Reader) renderOpenTag(n *html.Node) string {
	var buf strings.Builder
	buf.WriteString("<")
	buf.WriteString(n.Data)
	for _, attr := range n.Attr {
		buf.WriteString(" ")
		if attr.Namespace != "" {
			buf.WriteString(attr.Namespace)
			buf.WriteString(":")
		}
		buf.WriteString(attr.Key)
		buf.WriteString(`="`)
		buf.WriteString(html.EscapeString(attr.Val))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	return buf.String()
}

func (r *Reader) renderTag(n *html.Node) string {
	var buf strings.Builder
	buf.WriteString("<")
	buf.WriteString(n.Data)
	for _, attr := range n.Attr {
		buf.WriteString(" ")
		if attr.Namespace != "" {
			buf.WriteString(attr.Namespace)
			buf.WriteString(":")
		}
		buf.WriteString(attr.Key)
		buf.WriteString(`="`)
		buf.WriteString(html.EscapeString(attr.Val))
		buf.WriteString(`"`)
	}
	if selfClosingElements[n.DataAtom] {
		buf.WriteString("/>")
	} else {
		buf.WriteString(">")
	}
	return buf.String()
}

// renderNodeHTML renders an element and all its children to HTML string.
func renderNodeHTML(n *html.Node) string {
	var buf strings.Builder
	html.Render(&buf, n)
	return buf.String()
}

func isInlineElement(n *html.Node) bool {
	return n.Type == html.ElementNode && inlineElements[n.DataAtom]
}

// getAttr returns the value of the named attribute, or empty string.
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// getAttrNS returns the value of a namespaced attribute.
// Go's HTML parser stores xml:lang as key="xml:lang" (no namespace),
// so we also check for the combined "ns:key" form.
func getAttrNS(n *html.Node, ns, key string) string {
	combined := ns + ":" + key
	for _, attr := range n.Attr {
		if attr.Namespace == ns && attr.Key == key {
			return attr.Val
		}
		if attr.Key == combined {
			return attr.Val
		}
	}
	return ""
}

// extractCharset extracts charset from a Content-Type string.
func extractCharset(contentType string) string {
	for _, part := range strings.Split(contentType, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "charset=") {
			return strings.TrimSpace(part[8:])
		}
	}
	return ""
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

// Ensure regexp import is used.
var _ = (*regexp.Regexp)(nil)
