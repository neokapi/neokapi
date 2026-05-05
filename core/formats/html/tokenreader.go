package html

import (
	"bytes"
	"cmp"
	"context"
	"regexp"
	"slices"
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
	// needsCharsetMeta is set during pre-scan: true when the document
	// contains a <head> element but no <meta charset=…> or
	// <meta http-equiv="content-type" …>. Mirrors okapi's HtmlFilter
	// behaviour, which injects a Content-Type meta directly after
	// <head> in that case so the output declares its UTF-8 encoding
	// in transport headers.
	needsCharsetMeta bool
	// leafBlockTag is the tag name of the leaf block currently being
	// processed by processLeafBlock (e.g. "body", "p"). Empty when no
	// leaf block is active. Used by collectInlineTokens to recognise
	// an end-tag that closes the enclosing leaf block — e.g. an
	// unmatched `<sub>` should not swallow `</body>`. When seen, the
	// raw bytes are stashed in deferredLeafEndTagRaw and the recursion
	// unwinds.
	leafBlockTag string
	// deferredLeafEndTagRaw carries the raw bytes of a leaf block's
	// end-tag that was encountered inside a still-open inline span.
	// processLeafBlock consumes it on its next loop iteration.
	deferredLeafEndTagRaw []byte
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
	return "tu" + strconv.Itoa(s.blockCounter)
}

func (s *tokenReaderState) nextDataID() string {
	s.dataCounter++
	return "d" + strconv.Itoa(s.dataCounter)
}

// run processes HTML content with the tokenizer, writing skeleton data and
// emitting Parts to the channel.
func (s *tokenReaderState) run(content []byte, ctx context.Context, ch chan<- model.PartResult) {
	// Pre-scan: detect whether the document needs an injected
	// Content-Type meta. Okapi's HtmlFilter mirrors this logic — if a
	// <head> exists but no charset declaration is present, the writer
	// inserts <meta http-equiv="Content-Type" …> right after <head>.
	s.needsCharsetMeta = scanNeedsCharsetMeta(content)

	tokenizer := html.NewTokenizer(bytes.NewReader(content))
	tokenizer.SetMaxBuf(0) // unlimited buffer

	// We process the token stream, maintaining a stack to track nesting.
	// The approach: buffer tokens, classify elements, emit blocks and skeleton data.
	s.processTokenStream(tokenizer, ctx, ch)
}

// scanNeedsCharsetMeta returns true when content contains a <head>
// element but no <meta charset=…> or <meta http-equiv="content-type" …>.
// The scan is single-pass and case-insensitive on tag/attribute names
// (HTML5) to match okapi's detection.
func scanNeedsCharsetMeta(content []byte) bool {
	tokenizer := html.NewTokenizer(bytes.NewReader(content))
	tokenizer.SetMaxBuf(0)
	hasHead := false
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tagName, hasAttr := tokenizer.TagName()
			a := atom.Lookup(tagName)
			if a == atom.Head {
				hasHead = true
				continue
			}
			if a != atom.Meta {
				continue
			}
			if !hasAttr {
				continue
			}
			attrs := collectTokenAttrs(tokenizer)
			if charset := getTokenAttr(attrs, "charset"); charset != "" {
				return false
			}
			httpEquiv := strings.ToLower(getTokenAttr(attrs, "http-equiv"))
			if httpEquiv == "content-type" {
				return false
			}
		}
	}
	return hasHead
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
	atom.Dt:    true,
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
			preserveWS := false
			if len(stack) > 0 {
				preserveWS = stack[len(stack)-1].preserveWS
			}
			if !translateNo && hasNonWhitespace(text) {
				// Bare text node outside a block context — emit as text block.
				// Trim leading/trailing whitespace from edges that look like
				// source formatting (whitespace runs containing a newline),
				// dropping them from the skeleton entirely; this mirrors
				// okapi's HtmlFilter behavior of joining text directly with
				// adjacent block-level tags. Single-space edges (no newline)
				// are preserved as significant inter-word whitespace inside
				// inline boundaries (e.g. text inside <b>...</b>). When the
				// parent is a preserve-whitespace element (pre/textarea),
				// keep the raw text intact.
				if preserveWS {
					blockID := s.nextBlockID()
					_ = s.store.WriteRef(blockID)
					block := model.NewBlock(blockID, text)
					block.PreserveWhitespace = true
					s.reader.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
				} else {
					mid := trimNewlineEdges(text)
					blockID := s.nextBlockID()
					_ = s.store.WriteRef(blockID)
					block := model.NewBlock(blockID, mid)
					s.reader.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
				}
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

				// Mirror okapi's HtmlFilter: when the document declares
				// no Content-Type meta, inject one immediately after the
				// <head> start tag so the output advertises UTF-8 in
				// transport headers. Inject only once, even on malformed
				// input with multiple <head> openings.
				if a == atom.Head && s.needsCharsetMeta {
					_ = s.store.WriteText([]byte(`<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">`))
					s.reader.emit(ctx, ch, &model.Part{
						Type: model.PartData,
						Resource: &model.Data{
							ID:   s.nextDataID(),
							Name: "meta",
						},
					})
					s.needsCharsetMeta = false
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
// Runs sequence, and emits the block. The start tag raw bytes and closing tag
// raw bytes go into the skeleton; the fragment content is the block reference.
func (s *tokenReaderState) processLeafBlock(tokenizer *html.Tokenizer, tag string, a atom.Atom, attrs []html.Attribute, preserveWS bool, ctx context.Context, ch chan<- model.PartResult) {
	blockID := s.nextBlockID()

	// Start tag already written to skeleton by extractTokenAttrs.
	// NOTE: we defer the skeleton ref write until after content collection
	// so that trimmed leading/trailing whitespace can be written to the
	// skeleton (preserving byte-exact roundtrip).

	// Track the active leaf tag so collectInlineTokens can bail out when
	// it sees an end-tag matching us (e.g. an unmatched `<sub>` should
	// not swallow `</body>`).
	prevLeafTag := s.leafBlockTag
	s.leafBlockTag = tag
	defer func() { s.leafBlockTag = prevLeafTag }()

	// Collect tokens until matching close tag.
	b := newRunBuilder()
	idCounter := 0
	depth := 1

	var closeTagRaw []byte

	for depth > 0 {
		// If a recursive collectInlineTokens already consumed the leaf
		// block's end-tag (e.g. an unmatched inline like
		// `<sub>fox</pub>` swallowing past `</body>`), it leaves the
		// raw bytes here for us.
		if s.deferredLeafEndTagRaw != nil {
			closeTagRaw = s.deferredLeafEndTagRaw
			s.deferredLeafEndTagRaw = nil
			depth = 0
			break
		}
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		tokenRaw := copyBytes(tokenizer.Raw())

		switch tt {
		case html.TextToken:
			addTextWithEntities(b, string(tokenRaw), &idCounter)

		case html.CommentToken:
			idCounter++
			b.AddPh(
				strconv.Itoa(idCounter),
				"code:comment",
				"html:comment",
				string(tokenRaw), // includes <!-- -->
				"", "", model.RunConstraints{},
			)

		case html.StartTagToken:
			childTagName, hasAttr := tokenizer.TagName()
			childTag := string(childTagName)
			childAtom := atom.Lookup(childTagName)
			var childAttrs []html.Attribute
			if hasAttr {
				childAttrs = collectTokenAttrs(tokenizer)
			}

			// Extract translatable attributes on inline children.
			childTransAttrs := s.extractTokenAttrsNoSkeleton(childTag, childAtom, childAttrs, ctx, ch)

			if nonTranslatableElements[childAtom] {
				idCounter++
				// Consume until close, capture raw.
				innerRaw := s.consumeRawUntilClose(tokenizer, childTag)
				b.AddPh(
					strconv.Itoa(idCounter),
					"code:markup",
					"html:"+childTag,
					string(tokenRaw)+string(innerRaw),
					"", "", model.RunConstraints{},
				)
				continue
			}

			childTranslateNo := false
			if tv := getTokenAttr(childAttrs, "translate"); tv == "no" {
				childTranslateNo = true
			}

			if isInlineAtom(childAtom) {
				if childTranslateNo {
					// Whole inline is non-translatable: consume as placeholder.
					idCounter++
					innerRaw := s.consumeRawUntilClose(tokenizer, childTag)
					b.AddPh(
						strconv.Itoa(idCounter),
						"code:markup",
						"html:"+childTag,
						string(tokenRaw)+string(innerRaw),
						"", "", model.RunConstraints{},
					)
					continue
				}

				semType := htmlSemanticType(childTag)
				subType := "html:" + childTag

				if selfClosingElements[childAtom] {
					idCounter++
					info := s.vocab.LookupOrFallback(semType)
					b.AddPh(
						strconv.Itoa(idCounter),
						semType,
						subType,
						string(rewriteInlineTagWithRefs(tokenRaw, childTransAttrs)),
						info.Display.Placeholder,
						info.Equiv,
						model.RunConstraints{
							Deletable:   info.Constraints.Deletable,
							Cloneable:   info.Constraints.Cloneable,
							Reorderable: info.Constraints.Reorderable,
						},
					)
				} else {
					idCounter++
					spanID := strconv.Itoa(idCounter)
					info := s.vocab.LookupOrFallback(semType)
					b.AddPcOpen(
						spanID,
						semType,
						subType,
						string(rewriteInlineTagWithRefs(tokenRaw, childTransAttrs)),
						info.Display.Open,
						info.Equiv,
						model.RunConstraints{
							Deletable:   info.Constraints.Deletable,
							Cloneable:   info.Constraints.Cloneable,
							Reorderable: info.Constraints.Reorderable,
						},
					)
					// Recursively collect inline content.
					s.collectInlineTokens(tokenizer, childTag, b, &idCounter, info, ctx, ch)
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
				continue
			}
			// Stray end-tag inside a leaf block — preserve the original
			// bytes as an inline-code placeholder so they round-trip
			// verbatim. Mirrors okapi's HtmlFilter, which captures any
			// markup that doesn't match a translatable text path as an
			// inline `<code>` (e.g. `</br></br>` is preserved literally
			// in merged_codes.html). Per the HTML5 parsing algorithm an
			// end-tag for a void element like `</br>` is reinterpreted
			// as `<br>`, but our extraction keeps the original close-tag
			// bytes so the merge step writes them back exactly as the
			// source had them.
			endSemType := htmlSemanticType(endTag)
			endSubType := "html:" + endTag
			idCounter++
			endInfo := s.vocab.LookupOrFallback(endSemType)
			b.AddPh(
				strconv.Itoa(idCounter),
				endSemType,
				endSubType,
				string(tokenRaw),
				endInfo.Display.Placeholder,
				endInfo.Equiv,
				model.RunConstraints{
					Deletable:   endInfo.Constraints.Deletable,
					Cloneable:   endInfo.Constraints.Cloneable,
					Reorderable: endInfo.Constraints.Reorderable,
				},
			)

		case html.SelfClosingTagToken:
			childTagName, hasAttr := tokenizer.TagName()
			childTag := string(childTagName)
			childAtom := atom.Lookup(childTagName)
			var childAttrs []html.Attribute
			if hasAttr {
				childAttrs = collectTokenAttrs(tokenizer)
			}

			childTransAttrs := s.extractTokenAttrsNoSkeleton(childTag, childAtom, childAttrs, ctx, ch)

			semType := htmlSemanticType(childTag)
			subType := "html:" + childTag
			idCounter++
			info := s.vocab.LookupOrFallback(semType)
			b.AddPh(
				strconv.Itoa(idCounter),
				semType,
				subType,
				string(rewriteInlineTagWithRefs(tokenRaw, childTransAttrs)),
				info.Display.Placeholder,
				info.Equiv,
				model.RunConstraints{
					Deletable:   info.Constraints.Deletable,
					Cloneable:   info.Constraints.Cloneable,
					Reorderable: info.Constraints.Reorderable,
				},
			)
		}
	}

	// In skeleton mode, skip whitespace normalization entirely.
	// The skeleton ref will be filled from the fragment's raw text,
	// preserving original whitespace for byte-exact roundtrip.
	// The block's PreserveWhitespace flag tells downstream tools whether
	// they need to normalize the text for translation.
	//
	// NOTE: collapseWhitespaceRuns / trimWhitespaceRuns are applied in
	// the DOM-based reader path (reader.go) which does not use skeleton.
	//
	// Exception 1: peel leading/trailing whitespace runs that look like
	// source formatting (containing a newline) off the very first and
	// last text runs. Mirrors okapi's HtmlFilter behavior — text inside
	// a leaf block has its inter-element edge whitespace dropped on
	// extraction, and the writer joins translated text directly with the
	// element's tags (e.g. <body>\nText...</body> → <body>Pseudo(Text)...</body>).
	//
	// Exception 2: collapse internal whitespace runs that contain a
	// newline (i.e. source line breaks within translatable text) to a
	// single space. Mirrors okapi's HtmlFilter behavior — multi-line
	// source paragraphs come out single-line. Pure space/tab whitespace
	// runs without a newline are preserved (okapi keeps multi-space
	// formatting inside `<title>` and similar; matching that exactly
	// is too strict for a normalizer-friendly fix).
	if !preserveWS {
		s.peelEdgeNewlinesFromRuns(b)
		s.collapseInternalNewlineRuns(b)
	}

	_ = s.store.WriteRef(blockID)

	// Emit block if it has content.
	hasID := getTokenAttr(attrs, "id") != ""
	if !b.IsEmpty() || hasID {
		block := &model.Block{
			ID:                 blockID,
			Name:               blockNameFromToken(tag, attrs),
			Type:               blockTypeFromTag(tag),
			Translatable:       true,
			PreserveWhitespace: preserveWS,
			Source:             []*model.Segment{model.NewRunsSegment("s1", b.Runs())},
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

// collectInlineTokens recursively collects inline content into a runBuilder
// until the matching close tag for parentTag is found.
func (s *tokenReaderState) collectInlineTokens(tokenizer *html.Tokenizer, parentTag string, b *runBuilder, idCounter *int, parentInfo *model.SpanTypeInfo, ctx context.Context, ch chan<- model.PartResult) {
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			return
		}
		tokenRaw := copyBytes(tokenizer.Raw())

		switch tt {
		case html.TextToken:
			addTextWithEntities(b, string(tokenRaw), idCounter)

		case html.CommentToken:
			*idCounter++
			b.AddPh(
				strconv.Itoa(*idCounter),
				"code:comment",
				"html:comment",
				string(tokenRaw),
				"", "", model.RunConstraints{},
			)

		case html.StartTagToken:
			childTagName, hasAttr := tokenizer.TagName()
			childTag := string(childTagName)
			childAtom := atom.Lookup(childTagName)
			var childAttrs []html.Attribute
			if hasAttr {
				childAttrs = collectTokenAttrs(tokenizer)
			}

			if nonTranslatableElements[childAtom] {
				*idCounter++
				innerRaw := s.consumeRawUntilClose(tokenizer, childTag)
				b.AddPh(
					strconv.Itoa(*idCounter),
					"code:markup",
					"html:"+childTag,
					string(tokenRaw)+string(innerRaw),
					"", "", model.RunConstraints{},
				)
				continue
			}

			childTranslateNo := getTokenAttr(childAttrs, "translate") == "no"
			if childTranslateNo {
				*idCounter++
				innerRaw := s.consumeRawUntilClose(tokenizer, childTag)
				b.AddPh(
					strconv.Itoa(*idCounter),
					"code:markup",
					"html:"+childTag,
					string(tokenRaw)+string(innerRaw),
					"", "", model.RunConstraints{},
				)
				continue
			}

			// Extract translatable attributes on inline children so the
			// writer can substitute translated values into the placeholder
			// data via the BLOCK sentinel.
			childTransAttrs := s.extractTokenAttrsNoSkeleton(childTag, childAtom, childAttrs, ctx, ch)

			if isInlineAtom(childAtom) {
				semType := htmlSemanticType(childTag)
				subType := "html:" + childTag

				if selfClosingElements[childAtom] {
					*idCounter++
					info := s.vocab.LookupOrFallback(semType)
					b.AddPh(
						strconv.Itoa(*idCounter),
						semType,
						subType,
						string(rewriteInlineTagWithRefs(tokenRaw, childTransAttrs)),
						info.Display.Placeholder,
						info.Equiv,
						model.RunConstraints{
							Deletable:   info.Constraints.Deletable,
							Cloneable:   info.Constraints.Cloneable,
							Reorderable: info.Constraints.Reorderable,
						},
					)
				} else {
					*idCounter++
					spanID := strconv.Itoa(*idCounter)
					info := s.vocab.LookupOrFallback(semType)
					b.AddPcOpen(
						spanID,
						semType,
						subType,
						string(rewriteInlineTagWithRefs(tokenRaw, childTransAttrs)),
						info.Display.Open,
						info.Equiv,
						model.RunConstraints{
							Deletable:   info.Constraints.Deletable,
							Cloneable:   info.Constraints.Cloneable,
							Reorderable: info.Constraints.Reorderable,
						},
					)
					s.collectInlineTokens(tokenizer, childTag, b, idCounter, info, ctx, ch)
					// If the inner recursion bailed because it saw the
					// leaf block's end-tag, propagate the unwind so we
					// don't keep swallowing post-block skeleton.
					if s.deferredLeafEndTagRaw != nil {
						return
					}
				}
			}

		case html.EndTagToken:
			endTagName, _ := tokenizer.TagName()
			endTag := string(endTagName)
			if endTag == parentTag {
				// Emit closing run. Use the same span ID as the matching
				// opener so the pair can be reassembled downstream.
				semType := htmlSemanticType(parentTag)
				subType := "html:" + parentTag
				b.AddPcClose(
					findOpeningRunID(b, semType),
					semType,
					subType,
					string(tokenRaw),
					parentInfo.Equiv,
				)
				return
			}
			// End-tag for the enclosing leaf block (e.g. `</body>` while
			// still inside an unmatched `<sub>fox</pub>`). Stash the raw
			// close-tag bytes so processLeafBlock picks them up, and
			// unwind without consuming the close in the inline span.
			// Mirrors okapi's HtmlFilter: a missing inline close auto-
			// closes at the leaf-block boundary rather than swallowing
			// trailing skeleton.
			if s.leafBlockTag != "" && endTag == s.leafBlockTag {
				s.deferredLeafEndTagRaw = tokenRaw
				return
			}
			// Stray end-tag inside an inline run — same treatment as
			// processLeafBlock: preserve the original close-tag bytes
			// as a Ph so they survive the round-trip verbatim.
			endSemType := htmlSemanticType(endTag)
			endSubType := "html:" + endTag
			*idCounter++
			endInfo := s.vocab.LookupOrFallback(endSemType)
			b.AddPh(
				strconv.Itoa(*idCounter),
				endSemType,
				endSubType,
				string(tokenRaw),
				endInfo.Display.Placeholder,
				endInfo.Equiv,
				model.RunConstraints{
					Deletable:   endInfo.Constraints.Deletable,
					Cloneable:   endInfo.Constraints.Cloneable,
					Reorderable: endInfo.Constraints.Reorderable,
				},
			)

		case html.SelfClosingTagToken:
			childTagName, hasAttr := tokenizer.TagName()
			childTag := string(childTagName)
			childAtom := atom.Lookup(childTagName)
			var childAttrs []html.Attribute
			if hasAttr {
				childAttrs = collectTokenAttrs(tokenizer)
			}

			childTransAttrs := s.extractTokenAttrsNoSkeleton(childTag, childAtom, childAttrs, ctx, ch)

			semType := htmlSemanticType(childTag)
			subType := "html:" + childTag
			*idCounter++
			info := s.vocab.LookupOrFallback(semType)
			b.AddPh(
				strconv.Itoa(*idCounter),
				semType,
				subType,
				string(rewriteInlineTagWithRefs(tokenRaw, childTransAttrs)),
				info.Display.Placeholder,
				info.Equiv,
				model.RunConstraints{
					Deletable:   info.Constraints.Deletable,
					Cloneable:   info.Constraints.Cloneable,
					Reorderable: info.Constraints.Reorderable,
				},
			)
		}
	}
}

// findOpeningRunID finds the ID of the last unmatched PcOpen run whose Type
// matches semType. Mirrors the legacy findOpeningSpanID but walks Runs.
func findOpeningRunID(b *runBuilder, semType string) string {
	openCount := make(map[string]int)
	for i := len(b.runs) - 1; i >= 0; i-- {
		r := b.runs[i]
		switch {
		case r.PcClose != nil && r.PcClose.Type == semType:
			openCount[r.PcClose.ID]++
		case r.PcOpen != nil && r.PcOpen.Type == semType:
			if openCount[r.PcOpen.ID] > 0 {
				openCount[r.PcOpen.ID]--
			} else {
				return r.PcOpen.ID
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
			} else if !selfClosingElements[a] {
				depth++
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
				Source:       []*model.Segment{model.NewRunsSegment("s1", []model.Run{{Text: &model.TextRun{Text: content}}})},
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
// Used for inline elements inside leaf blocks. Returns the list of (key, value, blockID)
// tuples emitted, so the caller can rewrite the inline tag's raw bytes to embed
// block-id markers in place of the attribute values; the writer then substitutes
// those markers with translated text. Without this rewrite the inline tag would
// emit its source attribute values verbatim.
func (s *tokenReaderState) extractTokenAttrsNoSkeleton(tag string, a atom.Atom, attrs []html.Attribute, ctx context.Context, ch chan<- model.PartResult) []transAttrEntry {
	var out []transAttrEntry
	if title := getTokenAttr(attrs, "title"); title != "" {
		id := s.nextBlockID()
		s.emitAttrBlock(id, "title", title, ctx, ch)
		out = append(out, transAttrEntry{"title", title, id})
	}
	if alt := getTokenAttr(attrs, "alt"); alt != "" {
		if a == atom.Img || a == atom.Input || a == atom.Area {
			id := s.nextBlockID()
			s.emitAttrBlock(id, "alt", alt, ctx, ch)
			out = append(out, transAttrEntry{"alt", alt, id})
		}
	}
	if label := getTokenAttr(attrs, "label"); label != "" && a == atom.Option {
		id := s.nextBlockID()
		s.emitAttrBlock(id, "label", label, ctx, ch)
		out = append(out, transAttrEntry{"label", label, id})
	}
	if ph := getTokenAttr(attrs, "placeholder"); ph != "" {
		if a == atom.Input || a == atom.Textarea {
			id := s.nextBlockID()
			s.emitAttrBlock(id, "placeholder", ph, ctx, ch)
			out = append(out, transAttrEntry{"placeholder", ph, id})
		}
	}
	if val := getTokenAttr(attrs, "value"); val != "" && a == atom.Input {
		inputType := strings.ToLower(getTokenAttr(attrs, "type"))
		if isTranslatableInputValue(inputType) {
			id := s.nextBlockID()
			s.emitAttrBlock(id, "value", val, ctx, ch)
			out = append(out, transAttrEntry{"value", val, id})
		}
	}
	return out
}

// rewriteInlineTagWithRefs rewrites raw inline-tag bytes to replace each
// translatable attribute value with a `\x00BLOCK:tuN\x00` sentinel. The
// html writer detects these sentinels in placeholder data and substitutes
// each with the corresponding block's translated text. NUL bytes don't
// appear in well-formed HTML, so they're a safe in-band signal.
func rewriteInlineTagWithRefs(raw []byte, transAttrs []transAttrEntry) []byte {
	if len(transAttrs) == 0 {
		return raw
	}
	type replacement struct {
		offset  int
		length  int
		blockID string
	}
	repls := make([]replacement, 0, len(transAttrs))
	for _, a := range transAttrs {
		offset := findAttrValueOffset(raw, a.key)
		if offset >= 0 {
			repls = append(repls, replacement{offset, len(a.value), a.blockID})
		}
	}
	if len(repls) == 0 {
		return raw
	}
	slices.SortFunc(repls, func(a, b replacement) int {
		return cmp.Compare(a.offset, b.offset)
	})
	var buf bytes.Buffer
	pos := 0
	for _, r := range repls {
		buf.Write(raw[pos:r.offset])
		buf.WriteString(blockRefSentinelStart)
		buf.WriteString(r.blockID)
		buf.WriteString(blockRefSentinelEnd)
		pos = r.offset + r.length
	}
	buf.Write(raw[pos:])
	return buf.Bytes()
}

// blockRefSentinelStart marks the start of a `\x00BLOCK:tuN\x00`-style
// ID embedded in placeholder data; the writer substitutes each with the
// named block's translated text before emitting bytes. The terminator
// is a single NUL byte so we can distinguish ID end from start.
const blockRefSentinelStart = "\x00BLOCK:"
const blockRefSentinelEnd = "\x00"

func (s *tokenReaderState) emitAttrBlock(blockID, attrKey, value string, ctx context.Context, ch chan<- model.PartResult) {
	block := &model.Block{
		ID:           blockID,
		Type:         attrKey,
		Translatable: true,
		IsReferent:   true,
		Source:       []*model.Segment{model.NewRunsSegment("s1", []model.Run{{Text: &model.TextRun{Text: value}}})},
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

	repls := make([]replacement, 0, len(attrs))
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
	slices.SortFunc(repls, func(a, b replacement) int {
		return cmp.Compare(a.offset, b.offset)
	})

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
	keyBytes := []byte(strings.ToLower(attrKey))

	idx := 0
	for {
		pos := indexBytesInsensitive(raw[idx:], keyBytes)
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

// indexBytesInsensitive finds the first occurrence of the lowercase needle in
// haystack using case-insensitive comparison, without allocating a lowercase
// copy of haystack.
func indexBytesInsensitive(haystack, needle []byte) int {
	nl := len(needle)
	hl := len(haystack)
	if nl == 0 {
		return 0
	}
	if nl > hl {
		return -1
	}
	for i := 0; i <= hl-nl; i++ {
		if bytes.EqualFold(haystack[i:i+nl], needle) {
			return i
		}
	}
	return -1
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
	attrs := make([]html.Attribute, 0, 8) // preallocate; typical HTML elements have <10 attributes
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

// getTokenAttr performs a linear scan for the attribute key. Although this is
// called multiple times per token (up to ~8 calls for input elements), a map
// would not help: typical HTML elements have fewer than 10 attributes, so the
// linear scan is faster than building a map and amortizing the hash overhead.
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

// splitEdgeWhitespace splits s into (leading, mid, trailing) where leading
// is the run of HTML whitespace at the start, trailing the run at the end,
// and mid the substring between (which is guaranteed to start and end with
// non-whitespace). Used to peel inter-element whitespace off bare text
// nodes before emitting them as block content, mirroring okapi's
// HtmlFilter behavior.
func splitEdgeWhitespace(s string) (leading, mid, trailing string) {
	i := 0
	for i < len(s) && isHTMLWhitespaceByte(s[i]) {
		i++
	}
	if i == len(s) {
		return s, "", ""
	}
	j := len(s)
	for j > i && isHTMLWhitespaceByte(s[j-1]) {
		j--
	}
	return s[:i], s[i:j], s[j:]
}

// htmlEntityRE matches a single HTML entity reference: a named entity
// (`&amp;`), a numeric entity (`&#160;`), or a hex entity (`&#xA0;`).
// Used by addTextWithEntities to peel entities out of bare text into
// inline placeholder runs so they survive pseudo-translation as
// opaque codes (rather than getting their letters substituted to
// `&ĺţ;` etc.).
var htmlEntityRE = regexp.MustCompile(`&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]+|#[xX][0-9A-Fa-f]+);`)

// addTextWithEntities adds raw HTML text to a runBuilder, splitting
// out HTML entity references as inline placeholders so they don't get
// pseudo-translated character by character. The entity's source bytes
// (`&amp;`, `&#160;`) become the placeholder Data and are written
// back verbatim, mirroring okapi's HtmlFilter behavior of treating
// entities as opaque inline codes.
func addTextWithEntities(b *runBuilder, text string, idCounter *int) {
	if text == "" {
		return
	}
	matches := htmlEntityRE.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		b.AddText(text)
		return
	}
	pos := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		if start > pos {
			b.AddText(text[pos:start])
		}
		*idCounter++
		b.AddPh(
			strconv.Itoa(*idCounter),
			"code:entity",
			"html:entity",
			text[start:end],
			"", "", model.RunConstraints{},
		)
		pos = end
	}
	if pos < len(text) {
		b.AddText(text[pos:])
	}
}

// containsNewline reports whether s contains any \r or \n character.
func containsNewline(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			return true
		}
	}
	return false
}

// collapseInternalNewlineRuns collapses any whitespace run inside a
// TextRun that contains a newline character to a single space. Mirrors
// okapi's HtmlFilter behavior of joining multi-line source paragraphs
// onto a single line of translatable text.
func (s *tokenReaderState) collapseInternalNewlineRuns(b *runBuilder) {
	for i := range b.runs {
		if b.runs[i].Text == nil {
			continue
		}
		t := b.runs[i].Text.Text
		if !containsNewline(t) {
			continue
		}
		b.runs[i].Text.Text = collapseWhitespaceRunsContainingNewline(t)
	}
}

// collapseWhitespaceRunsContainingNewline scans s and replaces every
// maximal run of HTML whitespace that contains at least one newline
// with a single space. Pure-space runs (no newline) pass through.
func collapseWhitespaceRunsContainingNewline(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if !isHTMLWhitespaceByte(s[i]) {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i
		hasNL := false
		for j < len(s) && isHTMLWhitespaceByte(s[j]) {
			if s[j] == '\n' || s[j] == '\r' {
				hasNL = true
			}
			j++
		}
		if hasNL {
			b.WriteByte(' ')
		} else {
			b.WriteString(s[i:j])
		}
		i = j
	}
	return b.String()
}

// peelEdgeNewlinesFromRuns trims leading whitespace from the first text
// run and trailing whitespace from the last text run if those runs
// contain at least one newline. Mirrors okapi's HtmlFilter "drop
// non-significant edge whitespace inside translatable units" behavior.
// Runs that consist entirely of newline-bearing whitespace are dropped
// in their entirety. Edge runs preceded/followed by an inline-code run
// are not eligible for trimming since the whitespace is significant
// inter-word formatting.
func (s *tokenReaderState) peelEdgeNewlinesFromRuns(b *runBuilder) {
	if len(b.runs) == 0 {
		return
	}
	// Leading: walk forward; while runs[start] is a TextRun whose run
	// contains a newline, peel newline-bearing leading whitespace.
	for len(b.runs) > 0 && b.runs[0].Text != nil {
		t := b.runs[0].Text.Text
		if !containsNewline(t) {
			break
		}
		i := 0
		for i < len(t) && isHTMLWhitespaceByte(t[i]) {
			i++
		}
		if i == len(t) {
			b.runs = b.runs[1:]
			continue
		}
		b.runs[0].Text.Text = t[i:]
		break
	}
	// Trailing: symmetric.
	for len(b.runs) > 0 && b.runs[len(b.runs)-1].Text != nil {
		idx := len(b.runs) - 1
		t := b.runs[idx].Text.Text
		if !containsNewline(t) {
			break
		}
		j := len(t)
		for j > 0 && isHTMLWhitespaceByte(t[j-1]) {
			j--
		}
		if j == 0 {
			b.runs = b.runs[:idx]
			continue
		}
		b.runs[idx].Text.Text = t[:j]
		break
	}
}

// trimNewlineEdges trims leading and trailing whitespace runs from s only
// if those runs contain at least one newline. Single-space edges (no
// newline) are preserved as significant inter-word whitespace adjacent to
// inline element boundaries. When the entire string is whitespace, returns
// s unchanged.
func trimNewlineEdges(s string) string {
	leading, mid, trailing := splitEdgeWhitespace(s)
	if mid == "" {
		return s
	}
	leadingHasNL := strings.ContainsAny(leading, "\r\n")
	trailingHasNL := strings.ContainsAny(trailing, "\r\n")
	if !leadingHasNL {
		mid = leading + mid
	}
	if !trailingHasNL {
		mid = mid + trailing
	}
	return mid
}

func isHTMLWhitespaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
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
