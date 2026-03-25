package html

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// tokenReaderState holds state for the single-pass tokenizer reader.
type tokenReaderState struct {
	reader       *Reader
	store        *format.SkeletonStore
	cfg          *Config
	vocab        *model.VocabularyRegistry
	blockCounter int
	dataCounter  int
}

func newTokenReaderState(r *Reader, store *format.SkeletonStore) *tokenReaderState {
	return &tokenReaderState{
		reader: r,
		store:  store,
		cfg:    r.cfg,
		vocab:  r.vocab,
	}
}

func (s *tokenReaderState) nextBlockID() string {
	s.blockCounter++
	return fmt.Sprintf("tu%d", s.blockCounter)
}

func (s *tokenReaderState) nextDataID() string {
	s.dataCounter++
	return fmt.Sprintf("d%d", s.dataCounter)
}

// run processes HTML content with the tokenizer, writing skeleton data and
// emitting Parts to the channel.
func (s *tokenReaderState) run(content []byte, ctx context.Context, ch chan<- model.PartResult) {
	tokenizer := html.NewTokenizer(bytes.NewReader(content))
	tokenizer.SetMaxBuf(0) // unlimited buffer

	// We process the token stream, maintaining a stack to track nesting.
	// The approach: buffer tokens, classify elements, emit blocks and skeleton data.
	s.processTokenStream(tokenizer, ctx, ch)
}

// knownContainerElements are block-level elements that structurally always
// contain other block-level children (Okapi GROUP elements). These are
// classified as containers unconditionally, without forward scanning, to
// avoid misclassification when the tokenizer buffer has been exhausted by
// a preceding large element (#151).
var knownContainerElements = map[atom.Atom]bool{
	atom.Table: true, atom.Tbody: true, atom.Thead: true,
	atom.Tfoot: true, atom.Tr: true, atom.Colgroup: true,
	atom.Ul: true, atom.Ol: true, atom.Dl: true,
	atom.Select: true, atom.Optgroup: true, atom.Menu: true,
	atom.Details: true, atom.Fieldset: true,
}

// knownLeafElements are block-level elements that structurally cannot contain
// other block-level children and should always be treated as leaf blocks.
// Elements like <li>, <td>, <dd>, <blockquote> are NOT here because they
// can legitimately contain block children (e.g. <li> containing <ul>).
var knownLeafElements = map[atom.Atom]bool{
	atom.P: true, atom.Pre: true,
	atom.H1: true, atom.H2: true, atom.H3: true,
	atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Dt: true,
	atom.Title: true, atom.Caption: true, atom.Figcaption: true,
	atom.Address: true,
}

// elementInfo tracks element nesting during tokenizer processing.
type elementInfo struct {
	tag          string
	a            atom.Atom
	translateNo  bool
	preserveWS   bool
	isBlock      bool
	hasBlockKids bool
}

func (s *tokenReaderState) processTokenStream(tokenizer *html.Tokenizer, ctx context.Context, ch chan<- model.PartResult) {
	var stack []elementInfo
	translateNo := false

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if err := ctx.Err(); err != nil {
			return
		}

		raw := copyBytes(tokenizer.Raw())

		switch tt {
		case html.DoctypeToken:
			_ = s.store.WriteText(raw)
			s.reader.emit(ctx, ch, &model.Part{
				Type: model.PartData,
				Resource: &model.Data{
					ID:   s.nextDataID(),
					Name: "doctype",
				},
			})

		case html.CommentToken:
			// Check if we're inside a block element (leaf).
			// If so, this will be handled during block content collection.
			// At top level or inside containers, it's non-translatable.
			_ = s.store.WriteText(raw)
			s.reader.emit(ctx, ch, &model.Part{
				Type: model.PartData,
				Resource: &model.Data{
					ID:   s.nextDataID(),
					Name: "comment",
				},
			})

		case html.TextToken:
			text := string(raw)
			if !translateNo && hasNonWhitespace(text) {
				// Bare text node outside a block context — emit as text block.
				// In skeleton mode, preserve raw text for byte-exact roundtrip.
				// The block's PreserveWhitespace flag tells downstream tools
				// whether to normalize for translation.
				blockID := s.nextBlockID()
				_ = s.store.WriteRef(blockID)

				block := model.NewBlock(blockID, text)
				s.reader.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
			} else {
				_ = s.store.WriteText(raw)
			}

		case html.StartTagToken:
			tagName, hasAttr := tokenizer.TagName()
			tag := string(tagName)
			a := atom.Lookup(tagName)

			var attrs []html.Attribute
			if hasAttr {
				attrs = collectTokenAttrs(tokenizer)
			}

			info := elementInfo{
				tag:         tag,
				a:           a,
				translateNo: translateNo,
			}

			// Check translate attribute.
			if tv := getTokenAttr(attrs, "translate"); tv != "" {
				if tv == "no" {
					info.translateNo = true
				} else if tv == "yes" {
					info.translateNo = false
				}
			}

			// Non-translatable elements (script, style).
			if nonTranslatableElements[a] {
				_ = s.store.WriteText(raw)
				s.reader.emit(ctx, ch, &model.Part{
					Type: model.PartData,
					Resource: &model.Data{
						ID:   s.nextDataID(),
						Name: tag,
					},
				})
				// Consume all content until closing tag.
				s.consumeUntilClose(tokenizer, tag, ctx, ch)
				stack = append(stack, info)
				continue
			}

			// Check if void element.
			if selfClosingElements[a] {
				// Handle META tags.
				if a == atom.Meta {
					s.handleMetaToken(raw, attrs, ctx, ch)
					continue
				}

				// Extract lang attribute.
				s.extractLangFromToken(raw, tag, attrs, ctx, ch)

				// Extract translatable attributes.
				if !info.translateNo {
					s.extractTokenAttrs(raw, tag, a, attrs, ctx, ch)
				} else {
					_ = s.store.WriteText(raw)
				}
				continue
			}

			isInline := inlineElements[a]

			if !isInline {
				// Block-level element.
				info.isBlock = true
				info.preserveWS = s.cfg.PreserveWhitespace || preserveWhitespaceElements[a]

				// Extract lang attribute.
				s.extractLangFromToken(nil, tag, attrs, ctx, ch)

				// Extract translatable attributes and write start tag
				// to skeleton (with attr refs if needed).
				if !info.translateNo {
					s.extractTokenAttrs(raw, tag, a, attrs, ctx, ch)
				} else {
					_ = s.store.WriteText(raw)
				}

				if info.translateNo {
					stack = append(stack, info)
					translateNo = info.translateNo
					continue
				}

				// Classify: leaf block or container.
				// Known containers/leaves skip the forward scan entirely (#151).
				var hasBlockKids bool
				if knownContainerElements[a] {
					hasBlockKids = true
				} else if !knownLeafElements[a] {
					// Ambiguous element (e.g. <div>): use forward scan.
					// If buffer is exhausted, forwardScan defaults to container
					// (safe: avoids losing structural tags).
					remaining := tokenizer.Buffered()
					hasBlockKids = s.forwardScanForBlockChildren(remaining, tag)
				}
				info.hasBlockKids = hasBlockKids

				if hasBlockKids {
					// Container: start tag already written to skeleton above.
					stack = append(stack, info)
					translateNo = info.translateNo
					continue
				}

				// Leaf block: collect content until closing tag,
				// build fragment, emit as block.
				// Start tag already written to skeleton above.
				s.processLeafBlock(tokenizer, tag, a, attrs, info.preserveWS, ctx, ch)
				info.isBlock = false // mark as already processed
				stack = append(stack, info)
				translateNo = info.translateNo
				continue
			}

			// Inline element at top level — handled in text flow.
			_ = s.store.WriteText(raw)
			stack = append(stack, info)
			translateNo = info.translateNo

		case html.EndTagToken:
			tagName, _ := tokenizer.TagName()
			tag := string(tagName)

			// Pop stack.
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				if top.tag == tag {
					stack = stack[:len(stack)-1]
				}
			}

			_ = s.store.WriteText(raw)

			// Restore translateNo from parent.
			if len(stack) > 0 {
				translateNo = stack[len(stack)-1].translateNo
			} else {
				translateNo = false
			}

		case html.SelfClosingTagToken:
			tagName, hasAttr := tokenizer.TagName()
			tag := string(tagName)
			a := atom.Lookup(tagName)

			var attrs []html.Attribute
			if hasAttr {
				attrs = collectTokenAttrs(tokenizer)
			}

			if a == atom.Meta {
				s.handleMetaToken(raw, attrs, ctx, ch)
				continue
			}

			s.extractLangFromToken(raw, tag, attrs, ctx, ch)
			if !translateNo {
				s.extractTokenAttrs(raw, tag, a, attrs, ctx, ch)
			} else {
				_ = s.store.WriteText(raw)
			}
		}
	}
}

// processLeafBlock collects tokens until the element's closing tag, builds a
// Fragment, and emits the block. The start tag raw bytes and closing tag raw
// bytes go into the skeleton; the fragment content is the block reference.
func (s *tokenReaderState) processLeafBlock(tokenizer *html.Tokenizer, tag string, a atom.Atom, attrs []html.Attribute, preserveWS bool, ctx context.Context, ch chan<- model.PartResult) {
	blockID := s.nextBlockID()

	// Start tag already written to skeleton by extractTokenAttrs.
	// NOTE: we defer the skeleton ref write until after content collection
	// so that trimmed leading/trailing whitespace can be written to the
	// skeleton (preserving byte-exact roundtrip).

	// Collect tokens until matching close tag.
	frag := &model.Fragment{}
	spanCounter := 0
	depth := 1

	var closeTagRaw []byte

	for depth > 0 {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		tokenRaw := copyBytes(tokenizer.Raw())

		switch tt {
		case html.TextToken:
			frag.AppendText(string(tokenRaw))

		case html.CommentToken:
			spanCounter++
			frag.AppendSpan(&model.Span{
				SpanType: model.SpanPlaceholder,
				Type:     "code:comment",
				SubType:  "html:comment",
				ID:       strconv.Itoa(spanCounter),
				Data:     string(tokenRaw), // includes <!-- -->
			})

		case html.StartTagToken:
			childTagName, hasAttr := tokenizer.TagName()
			childTag := string(childTagName)
			childAtom := atom.Lookup(childTagName)
			var childAttrs []html.Attribute
			if hasAttr {
				childAttrs = collectTokenAttrs(tokenizer)
			}

			// Extract translatable attributes on inline children.
			s.extractTokenAttrsNoSkeleton(childTag, childAtom, childAttrs, ctx, ch)

			if nonTranslatableElements[childAtom] {
				spanCounter++
				// Consume until close, capture raw.
				innerRaw := s.consumeRawUntilClose(tokenizer, childTag)
				frag.AppendSpan(&model.Span{
					SpanType: model.SpanPlaceholder,
					Type:     "code:markup",
					SubType:  "html:" + childTag,
					ID:       strconv.Itoa(spanCounter),
					Data:     string(tokenRaw) + string(innerRaw),
				})
				continue
			}

			childTranslateNo := false
			if tv := getTokenAttr(childAttrs, "translate"); tv == "no" {
				childTranslateNo = true
			}

			if isInlineAtom(childAtom) {
				if childTranslateNo {
					// Whole inline is non-translatable: consume as placeholder.
					spanCounter++
					innerRaw := s.consumeRawUntilClose(tokenizer, childTag)
					frag.AppendSpan(&model.Span{
						SpanType: model.SpanPlaceholder,
						Type:     "code:markup",
						SubType:  "html:" + childTag,
						ID:       strconv.Itoa(spanCounter),
						Data:     string(tokenRaw) + string(innerRaw),
					})
					continue
				}

				semType := htmlSemanticType(childTag)
				subType := "html:" + childTag

				if selfClosingElements[childAtom] {
					spanCounter++
					info := s.vocab.LookupOrFallback(semType)
					frag.AppendSpan(&model.Span{
						SpanType:    model.SpanPlaceholder,
						Type:        semType,
						SubType:     subType,
						ID:          strconv.Itoa(spanCounter),
						Data:        string(tokenRaw),
						DisplayText: info.Display.Placeholder,
						EquivText:   info.Equiv,
						Deletable:   info.Constraints.Deletable,
						Cloneable:   info.Constraints.Cloneable,
						CanReorder:  info.Constraints.Reorderable,
					})
				} else {
					spanCounter++
					spanID := strconv.Itoa(spanCounter)
					info := s.vocab.LookupOrFallback(semType)
					frag.AppendSpan(&model.Span{
						SpanType:    model.SpanOpening,
						Type:        semType,
						SubType:     subType,
						ID:          spanID,
						Data:        string(tokenRaw),
						DisplayText: info.Display.Open,
						EquivText:   info.Equiv,
						Deletable:   info.Constraints.Deletable,
						Cloneable:   info.Constraints.Cloneable,
						CanReorder:  info.Constraints.Reorderable,
					})
					// Recursively collect inline content.
					s.collectInlineTokens(tokenizer, childTag, frag, &spanCounter, info)
				}
			} else {
				// Nested block element inside a "leaf" — shouldn't happen
				// if forward scan is correct, but handle gracefully.
				depth++
			}

		case html.EndTagToken:
			endTagName, _ := tokenizer.TagName()
			endTag := string(endTagName)
			if endTag == tag {
				depth--
				if depth == 0 {
					closeTagRaw = tokenRaw
				}
			}

		case html.SelfClosingTagToken:
			childTagName, hasAttr := tokenizer.TagName()
			childTag := string(childTagName)
			childAtom := atom.Lookup(childTagName)
			var childAttrs []html.Attribute
			if hasAttr {
				childAttrs = collectTokenAttrs(tokenizer)
			}

			s.extractTokenAttrsNoSkeleton(childTag, childAtom, childAttrs, ctx, ch)

			semType := htmlSemanticType(childTag)
			subType := "html:" + childTag
			spanCounter++
			info := s.vocab.LookupOrFallback(semType)
			frag.AppendSpan(&model.Span{
				SpanType:    model.SpanPlaceholder,
				Type:        semType,
				SubType:     subType,
				ID:          strconv.Itoa(spanCounter),
				Data:        string(tokenRaw),
				DisplayText: info.Display.Placeholder,
				EquivText:   info.Equiv,
				Deletable:   info.Constraints.Deletable,
				Cloneable:   info.Constraints.Cloneable,
				CanReorder:  info.Constraints.Reorderable,
			})
		}
	}

	// In skeleton mode, skip whitespace normalization entirely.
	// The skeleton ref will be filled from the fragment's raw text,
	// preserving original whitespace for byte-exact roundtrip.
	// The block's PreserveWhitespace flag tells downstream tools whether
	// they need to normalize the text for translation.
	//
	// NOTE: collapseWhitespaceCodedText / trimCodedText are applied in
	// the DOM-based reader path (reader.go) which does not use skeleton.

	_ = s.store.WriteRef(blockID)

	// Emit block if it has content.
	hasID := getTokenAttr(attrs, "id") != ""
	if !frag.IsEmpty() || hasID {
		block := &model.Block{
			ID:                 blockID,
			Name:               blockNameFromToken(tag, attrs),
			Type:               blockTypeFromTag(tag),
			Translatable:       true,
			PreserveWhitespace: preserveWS,
			Source:             []*model.Segment{{ID: "s1", Content: frag}},
			Targets:            make(map[model.LocaleID][]*model.Segment),
			Properties:         extractBlockPropsFromToken(attrs),
			Annotations:        make(map[string]model.Annotation),
		}
		s.reader.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	// Write close tag to skeleton.
	if closeTagRaw != nil {
		_ = s.store.WriteText(closeTagRaw)
	}
}

// collectInlineTokens recursively collects inline content into a Fragment
// until the matching close tag for parentTag is found.
func (s *tokenReaderState) collectInlineTokens(tokenizer *html.Tokenizer, parentTag string, frag *model.Fragment, spanCounter *int, parentInfo *model.SpanTypeInfo) {
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			return
		}
		tokenRaw := copyBytes(tokenizer.Raw())

		switch tt {
		case html.TextToken:
			frag.AppendText(string(tokenRaw))

		case html.CommentToken:
			*spanCounter++
			frag.AppendSpan(&model.Span{
				SpanType: model.SpanPlaceholder,
				Type:     "code:comment",
				SubType:  "html:comment",
				ID:       strconv.Itoa(*spanCounter),
				Data:     string(tokenRaw),
			})

		case html.StartTagToken:
			childTagName, hasAttr := tokenizer.TagName()
			childTag := string(childTagName)
			childAtom := atom.Lookup(childTagName)
			var childAttrs []html.Attribute
			if hasAttr {
				childAttrs = collectTokenAttrs(tokenizer)
			}

			if nonTranslatableElements[childAtom] {
				*spanCounter++
				innerRaw := s.consumeRawUntilClose(tokenizer, childTag)
				frag.AppendSpan(&model.Span{
					SpanType: model.SpanPlaceholder,
					Type:     "code:markup",
					SubType:  "html:" + childTag,
					ID:       strconv.Itoa(*spanCounter),
					Data:     string(tokenRaw) + string(innerRaw),
				})
				continue
			}

			childTranslateNo := getTokenAttr(childAttrs, "translate") == "no"
			if childTranslateNo {
				*spanCounter++
				innerRaw := s.consumeRawUntilClose(tokenizer, childTag)
				frag.AppendSpan(&model.Span{
					SpanType: model.SpanPlaceholder,
					Type:     "code:markup",
					SubType:  "html:" + childTag,
					ID:       strconv.Itoa(*spanCounter),
					Data:     string(tokenRaw) + string(innerRaw),
				})
				continue
			}

			if isInlineAtom(childAtom) {
				semType := htmlSemanticType(childTag)
				subType := "html:" + childTag

				if selfClosingElements[childAtom] {
					*spanCounter++
					info := s.vocab.LookupOrFallback(semType)
					frag.AppendSpan(&model.Span{
						SpanType:    model.SpanPlaceholder,
						Type:        semType,
						SubType:     subType,
						ID:          strconv.Itoa(*spanCounter),
						Data:        string(tokenRaw),
						DisplayText: info.Display.Placeholder,
						EquivText:   info.Equiv,
						Deletable:   info.Constraints.Deletable,
						Cloneable:   info.Constraints.Cloneable,
						CanReorder:  info.Constraints.Reorderable,
					})
				} else {
					*spanCounter++
					spanID := strconv.Itoa(*spanCounter)
					info := s.vocab.LookupOrFallback(semType)
					frag.AppendSpan(&model.Span{
						SpanType:    model.SpanOpening,
						Type:        semType,
						SubType:     subType,
						ID:          spanID,
						Data:        string(tokenRaw),
						DisplayText: info.Display.Open,
						EquivText:   info.Equiv,
						Deletable:   info.Constraints.Deletable,
						Cloneable:   info.Constraints.Cloneable,
						CanReorder:  info.Constraints.Reorderable,
					})
					s.collectInlineTokens(tokenizer, childTag, frag, spanCounter, info)
				}
			}

		case html.EndTagToken:
			endTagName, _ := tokenizer.TagName()
			endTag := string(endTagName)
			if endTag == parentTag {
				// Emit closing span.
				semType := htmlSemanticType(parentTag)
				subType := "html:" + parentTag
				// Use the same span ID as the opening (current value of counter).
				// Actually, get the ID from the parentInfo context — it was the
				// counter value when opening was created.
				frag.AppendSpan(&model.Span{
					SpanType:    model.SpanClosing,
					Type:        semType,
					SubType:     subType,
					ID:          findOpeningSpanID(frag, semType),
					Data:        string(tokenRaw),
					DisplayText: parentInfo.Display.Close,
					EquivText:   parentInfo.Equiv,
					Deletable:   parentInfo.Constraints.Deletable,
					Cloneable:   parentInfo.Constraints.Cloneable,
					CanReorder:  parentInfo.Constraints.Reorderable,
				})
				return
			}

		case html.SelfClosingTagToken:
			childTagName, hasAttr := tokenizer.TagName()
			childTag := string(childTagName)
			if hasAttr {
				collectTokenAttrs(tokenizer) // consume attrs
			}

			semType := htmlSemanticType(childTag)
			subType := "html:" + childTag
			*spanCounter++
			info := s.vocab.LookupOrFallback(semType)
			frag.AppendSpan(&model.Span{
				SpanType:    model.SpanPlaceholder,
				Type:        semType,
				SubType:     subType,
				ID:          strconv.Itoa(*spanCounter),
				Data:        string(tokenRaw),
				DisplayText: info.Display.Placeholder,
				EquivText:   info.Equiv,
				Deletable:   info.Constraints.Deletable,
				Cloneable:   info.Constraints.Cloneable,
				CanReorder:  info.Constraints.Reorderable,
			})
		}
	}
}

// findOpeningSpanID finds the ID of the last unmatched opening span with the given type.
func findOpeningSpanID(frag *model.Fragment, semType string) string {
	// Walk backwards through spans to find the matching opening span.
	openCount := make(map[string]int)
	for i := len(frag.Spans) - 1; i >= 0; i-- {
		sp := frag.Spans[i]
		if sp.Type != semType {
			continue
		}
		switch sp.SpanType {
		case model.SpanClosing:
			openCount[sp.ID]++
		case model.SpanOpening:
			if openCount[sp.ID] > 0 {
				openCount[sp.ID]--
			} else {
				return sp.ID
			}
		}
	}
	return "1"
}

// forwardScanForBlockChildren scans remaining buffered content to check if the
// current element has block-level children. Returns true if any direct child
// is a block-level start tag.
//
// When the buffer is exhausted before finding the closing tag (ErrorToken),
// we default to true (container). This is the safe choice: treating a leaf
// as a container emits its text as bare text blocks (still translatable),
// while treating a container as a leaf loses structural tags entirely (#151).
func (s *tokenReaderState) forwardScanForBlockChildren(remaining []byte, parentTag string) bool {
	if len(remaining) == 0 {
		return true // buffer exhausted — assume container to avoid losing structure
	}
	scanner := html.NewTokenizer(bytes.NewReader(remaining))
	scanner.SetMaxBuf(0)
	depth := 0

	for {
		tt := scanner.Next()
		if tt == html.ErrorToken {
			return true // buffer exhausted — assume container
		}

		switch tt {
		case html.StartTagToken:
			tagName, _ := scanner.TagName()
			a := atom.Lookup(tagName)

			if depth == 0 {
				// Direct child: block-level if not inline and not script/style.
				if !inlineElements[a] && !nonTranslatableElements[a] {
					return true
				}
				if inlineElements[a] && !selfClosingElements[a] {
					depth++
				}
			} else {
				if !selfClosingElements[a] {
					depth++
				}
			}

		case html.EndTagToken:
			tagName, _ := scanner.TagName()
			endTag := string(tagName)
			if depth > 0 {
				depth--
			} else if endTag == parentTag {
				return false
			}

		case html.SelfClosingTagToken:
			// Self-closing at depth 0: not a block child.
		}
	}
}

// consumeUntilClose consumes tokens until the matching closing tag, writing
// all raw bytes to skeleton.
func (s *tokenReaderState) consumeUntilClose(tokenizer *html.Tokenizer, tag string, ctx context.Context, ch chan<- model.PartResult) {
	depth := 1
	for depth > 0 {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			return
		}
		raw := copyBytes(tokenizer.Raw())
		_ = s.store.WriteText(raw)

		switch tt {
		case html.StartTagToken:
			tagName, _ := tokenizer.TagName()
			if string(tagName) == tag {
				depth++
			}
		case html.EndTagToken:
			tagName, _ := tokenizer.TagName()
			if string(tagName) == tag {
				depth--
			}
		}
	}
}

// consumeRawUntilClose consumes tokens until the matching close tag and
// returns all raw bytes concatenated.
func (s *tokenReaderState) consumeRawUntilClose(tokenizer *html.Tokenizer, tag string) []byte {
	var buf bytes.Buffer
	depth := 1
	for depth > 0 {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		buf.Write(tokenizer.Raw())

		switch tt {
		case html.StartTagToken:
			tagName, _ := tokenizer.TagName()
			if string(tagName) == tag {
				depth++
			}
		case html.EndTagToken:
			tagName, _ := tokenizer.TagName()
			if string(tagName) == tag {
				depth--
			}
		}
	}
	return buf.Bytes()
}

// handleMetaToken handles META tags for encoding, language, and translatable content.
func (s *tokenReaderState) handleMetaToken(raw []byte, attrs []html.Attribute, ctx context.Context, ch chan<- model.PartResult) {
	httpEquiv := strings.ToLower(getTokenAttr(attrs, "http-equiv"))
	metaName := strings.ToLower(getTokenAttr(attrs, "name"))
	content := getTokenAttr(attrs, "content")
	charset := getTokenAttr(attrs, "charset")

	if charset != "" {
		_ = s.store.WriteText(raw)
		s.reader.emit(ctx, ch, &model.Part{
			Type: model.PartData,
			Resource: &model.Data{
				ID:         s.nextDataID(),
				Name:       "meta",
				Properties: map[string]string{"encoding": charset},
			},
		})
		return
	}

	if httpEquiv == "content-type" && content != "" {
		if cs := extractCharset(content); cs != "" {
			_ = s.store.WriteText(raw)
			s.reader.emit(ctx, ch, &model.Part{
				Type: model.PartData,
				Resource: &model.Data{
					ID:         s.nextDataID(),
					Name:       "meta",
					Properties: map[string]string{"encoding": cs},
				},
			})
			return
		}
	}

	if httpEquiv == "content-language" && content != "" {
		_ = s.store.WriteText(raw)
		s.reader.emit(ctx, ch, &model.Part{
			Type: model.PartData,
			Resource: &model.Data{
				ID:         s.nextDataID(),
				Name:       "meta",
				Properties: map[string]string{"language": content},
			},
		})
		return
	}

	if content != "" {
		isTranslatable := httpEquiv == "keywords" || translatableMetaNames[metaName]
		if isTranslatable {
			// Translatable meta: write skeleton with ref for content attribute.
			blockID := s.nextBlockID()
			s.writeAttrRefSkeleton(raw, "content", content, blockID)

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
			s.reader.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})

			// Also emit as data.
			s.reader.emit(ctx, ch, &model.Part{
				Type:     model.PartData,
				Resource: &model.Data{ID: s.nextDataID(), Name: "meta"},
			})
			return
		}
	}

	_ = s.store.WriteText(raw)
	s.reader.emit(ctx, ch, &model.Part{
		Type:     model.PartData,
		Resource: &model.Data{ID: s.nextDataID(), Name: "meta"},
	})
}

// extractLangFromToken extracts lang/xml:lang attributes.
func (s *tokenReaderState) extractLangFromToken(raw []byte, tag string, attrs []html.Attribute, ctx context.Context, ch chan<- model.PartResult) {
	lang := getTokenAttr(attrs, "lang")
	if lang == "" {
		lang = getTokenAttrNS(attrs, "xml", "lang")
	}
	if lang != "" {
		s.reader.emit(ctx, ch, &model.Part{
			Type: model.PartData,
			Resource: &model.Data{
				ID:         s.nextDataID(),
				Name:       tag,
				Properties: map[string]string{"language": lang},
			},
		})
	}
}

// extractTokenAttrs extracts translatable attributes (title, alt, etc.) from a token.
// If raw is not nil, it writes the tag raw bytes to skeleton (with attr refs as needed).
func (s *tokenReaderState) extractTokenAttrs(raw []byte, tag string, a atom.Atom, attrs []html.Attribute, ctx context.Context, ch chan<- model.PartResult) {
	// Collect which attributes are translatable.
	var transAttrs []transAttrEntry

	if title := getTokenAttr(attrs, "title"); title != "" {
		id := s.nextBlockID()
		transAttrs = append(transAttrs, transAttrEntry{"title", title, id})
		s.emitAttrBlock(id, "title", title, ctx, ch)
	}

	if alt := getTokenAttr(attrs, "alt"); alt != "" {
		if a == atom.Img || a == atom.Input || a == atom.Area {
			id := s.nextBlockID()
			transAttrs = append(transAttrs, transAttrEntry{"alt", alt, id})
			s.emitAttrBlock(id, "alt", alt, ctx, ch)
		}
	}

	if label := getTokenAttr(attrs, "label"); label != "" {
		if a == atom.Option {
			id := s.nextBlockID()
			transAttrs = append(transAttrs, transAttrEntry{"label", label, id})
			s.emitAttrBlock(id, "label", label, ctx, ch)
		}
	}

	if ph := getTokenAttr(attrs, "placeholder"); ph != "" {
		if a == atom.Input || a == atom.Textarea {
			id := s.nextBlockID()
			transAttrs = append(transAttrs, transAttrEntry{"placeholder", ph, id})
			s.emitAttrBlock(id, "placeholder", ph, ctx, ch)
		}
	}

	if val := getTokenAttr(attrs, "value"); val != "" && a == atom.Input {
		inputType := strings.ToLower(getTokenAttr(attrs, "type"))
		if isTranslatableInputValue(inputType) {
			id := s.nextBlockID()
			transAttrs = append(transAttrs, transAttrEntry{"value", val, id})
			s.emitAttrBlock(id, "value", val, ctx, ch)
		}
	}

	// Write skeleton data.
	if raw != nil {
		if len(transAttrs) == 0 {
			_ = s.store.WriteText(raw)
		} else {
			s.writeMultiAttrRefSkeleton(raw, transAttrs)
		}
	}
}

// extractTokenAttrsNoSkeleton extracts translatable attributes without writing skeleton.
// Used for inline elements inside leaf blocks.
func (s *tokenReaderState) extractTokenAttrsNoSkeleton(tag string, a atom.Atom, attrs []html.Attribute, ctx context.Context, ch chan<- model.PartResult) {
	if title := getTokenAttr(attrs, "title"); title != "" {
		s.emitAttrBlock(s.nextBlockID(), "title", title, ctx, ch)
	}
	if alt := getTokenAttr(attrs, "alt"); alt != "" {
		if a == atom.Img || a == atom.Input || a == atom.Area {
			s.emitAttrBlock(s.nextBlockID(), "alt", alt, ctx, ch)
		}
	}
	if label := getTokenAttr(attrs, "label"); label != "" && a == atom.Option {
		s.emitAttrBlock(s.nextBlockID(), "label", label, ctx, ch)
	}
	if ph := getTokenAttr(attrs, "placeholder"); ph != "" {
		if a == atom.Input || a == atom.Textarea {
			s.emitAttrBlock(s.nextBlockID(), "placeholder", ph, ctx, ch)
		}
	}
	if val := getTokenAttr(attrs, "value"); val != "" && a == atom.Input {
		inputType := strings.ToLower(getTokenAttr(attrs, "type"))
		if isTranslatableInputValue(inputType) {
			s.emitAttrBlock(s.nextBlockID(), "value", val, ctx, ch)
		}
	}
}

func (s *tokenReaderState) emitAttrBlock(blockID, attrKey, value string, ctx context.Context, ch chan<- model.PartResult) {
	block := &model.Block{
		ID:           blockID,
		Type:         attrKey,
		Translatable: true,
		IsReferent:   true,
		Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(value)}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}
	s.reader.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// writeAttrRefSkeleton writes a tag's raw bytes to skeleton, replacing one
// attribute value with a block reference.
func (s *tokenReaderState) writeAttrRefSkeleton(raw []byte, attrKey, attrValue, blockID string) {
	offset := findAttrValueOffset(raw, attrKey)
	if offset < 0 {
		// Fallback: write whole tag then ref.
		_ = s.store.WriteText(raw)
		return
	}

	// Write up to the attribute value.
	_ = s.store.WriteText(raw[:offset])
	_ = s.store.WriteRef(blockID)
	// Write after the attribute value.
	_ = s.store.WriteText(raw[offset+len(attrValue):])
}

// writeMultiAttrRefSkeleton writes a tag's raw bytes to skeleton, replacing
// multiple attribute values with block references.
func (s *tokenReaderState) writeMultiAttrRefSkeleton(raw []byte, attrs []transAttrEntry) {
	type replacement struct {
		offset  int
		length  int
		blockID string
	}

	var repls []replacement
	for _, a := range attrs {
		offset := findAttrValueOffset(raw, a.key)
		if offset >= 0 {
			repls = append(repls, replacement{offset, len(a.value), a.blockID})
		}
	}

	if len(repls) == 0 {
		_ = s.store.WriteText(raw)
		return
	}

	// Sort by offset (ascending).
	for i := range repls {
		for j := i + 1; j < len(repls); j++ {
			if repls[j].offset < repls[i].offset {
				repls[i], repls[j] = repls[j], repls[i]
			}
		}
	}

	pos := 0
	for _, r := range repls {
		_ = s.store.WriteText(raw[pos:r.offset])
		_ = s.store.WriteRef(r.blockID)
		pos = r.offset + r.length
	}
	_ = s.store.WriteText(raw[pos:])
}

// findAttrValueOffset finds the byte offset of an attribute's value in raw tag bytes.
// Returns -1 if not found.
func findAttrValueOffset(raw []byte, attrKey string) int {
	lower := bytes.ToLower(raw)
	keyBytes := []byte(strings.ToLower(attrKey))

	idx := 0
	for {
		pos := bytes.Index(lower[idx:], keyBytes)
		if pos < 0 {
			return -1
		}
		pos += idx

		// Ensure it's preceded by whitespace.
		if pos > 0 && raw[pos-1] != ' ' && raw[pos-1] != '\t' && raw[pos-1] != '\n' && raw[pos-1] != '\r' {
			idx = pos + 1
			continue
		}

		// Find = after key.
		eqPos := pos + len(keyBytes)
		for eqPos < len(raw) && (raw[eqPos] == ' ' || raw[eqPos] == '\t') {
			eqPos++
		}
		if eqPos >= len(raw) || raw[eqPos] != '=' {
			idx = pos + 1
			continue
		}
		eqPos++ // skip =

		// Skip whitespace after =.
		for eqPos < len(raw) && (raw[eqPos] == ' ' || raw[eqPos] == '\t') {
			eqPos++
		}
		if eqPos >= len(raw) {
			return -1
		}

		// Check for quote.
		quote := raw[eqPos]
		if quote == '"' || quote == '\'' {
			return eqPos + 1 // value starts after the quote
		}
		// Unquoted attribute value.
		return eqPos
	}
}

// transAttrEntry holds a translatable attribute found during token processing.
type transAttrEntry struct {
	key     string
	value   string
	blockID string
}

// Helper functions for tokenizer-based processing.

func copyBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

func collectTokenAttrs(tokenizer *html.Tokenizer) []html.Attribute {
	var attrs []html.Attribute
	for {
		key, val, more := tokenizer.TagAttr()
		if len(key) > 0 {
			attrs = append(attrs, html.Attribute{Key: string(key), Val: string(val)})
		}
		if !more {
			break
		}
	}
	return attrs
}

func getTokenAttr(attrs []html.Attribute, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func getTokenAttrNS(attrs []html.Attribute, ns, key string) string {
	combined := ns + ":" + key
	for _, a := range attrs {
		if a.Key == combined {
			return a.Val
		}
		if a.Namespace == ns && a.Key == key {
			return a.Val
		}
	}
	return ""
}

func isInlineAtom(a atom.Atom) bool {
	return inlineElements[a]
}

func hasNonWhitespace(s string) bool {
	for _, r := range s {
		if !isHTMLWhitespace(r) {
			return true
		}
	}
	return false
}

func blockTypeFromTag(tag string) string {
	if t, ok := blockTypeMap[strings.ToLower(tag)]; ok {
		return t
	}
	return ""
}

func blockNameFromToken(tag string, attrs []html.Attribute) string {
	if id := getTokenAttr(attrs, "id"); id != "" {
		return id + "-id"
	}
	return tag
}

func extractBlockPropsFromToken(attrs []html.Attribute) map[string]string {
	props := make(map[string]string)
	if id := getTokenAttr(attrs, "id"); id != "" {
		props["id"] = id
	}
	if dir := getTokenAttr(attrs, "dir"); dir != "" {
		props["dir"] = dir
	}
	return props
}
