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
	// deferredStart carries a block-level start tag that processLeafBlock
	// consumed but couldn't process (HTML5 implicit-close pattern: a
	// block-level start inside `<p>` etc. auto-closes the leaf and the
	// block tag is processed at the parent level). The main loop replays
	// it on its next iteration before calling tokenizer.Next().
	deferredStart *deferredStartTag
	// pendingWS buffers pure-whitespace TextTokens at top-level so we can
	// decide later whether to flush them to skeleton or drop them. Okapi
	// strips inter-element whitespace adjacent to a "text-unit" (a run of
	// text + inline tags between two leaf-block boundaries) but preserves
	// whitespace between two structural tags. We buffer pure-WS tokens and
	// drop them when a structural event arrives after a text-unit (or
	// vice-versa).
	pendingWS [][]byte
	// lastTextBlock points at the most recent top-level text-block emitted
	// so we can retroactively trim its trailing whitespace when the next
	// event proves we just exited a text-unit.
	lastTextBlock *model.Block
	// content is the entire input document. forwardScanForBlockChildren
	// uses this for lookahead because tokenizer.Buffered() only returns
	// what is currently in the bufio buffer — after a giant <script>
	// body, the buffered window may stop short of the next `</td>`,
	// causing forwardScan to exhaust its scanner and default to
	// container, mis-classifying TEXTUNIT-typed parents (td/li/dd/…).
	content []byte
	// consumed is the number of bytes of content already consumed by the
	// main tokenizer. It is advanced by len(tokenizer.Raw()) after every
	// Next() call (via next()). golang.org/x/net/html's Raw() returns the
	// exact source bytes of each token, and consecutive Raw() slices
	// concatenate to the input byte-for-byte, so the running sum is an
	// exact, O(1) cursor — replacing the former O(n) bytes.Index scan from
	// byte 0 in remainingContent (#608, N1).
	consumed int
}

// next advances the main tokenizer one token and tracks the consumed-byte
// cursor. Every read from the document tokenizer must go through this so that
// remainingContent can locate the tokenizer's position in O(1). Tokens that
// are replayed from stashed raw bytes (deferredStart/deferredLeafEndTagRaw)
// must NOT be re-counted — they were already consumed (and counted) when first
// read, and replaying them does not call Next().
func (s *tokenReaderState) next(tokenizer *html.Tokenizer) html.TokenType {
	tt := tokenizer.Next()
	s.consumed += len(tokenizer.Raw())
	return tt
}

// remainingContent returns the input bytes that have not yet been
// processed by the tokenizer, with a fallback to tokenizer.Buffered().
//
// The tokenizer's current position is tracked exactly and cheaply by the
// consumed-byte cursor (s.consumed), advanced by len(Raw()) after every
// Next() in next(). golang.org/x/net/html's Raw() returns the verbatim
// source of each token and consecutive tokens' Raw() slices concatenate to
// the input, so s.consumed is precisely the offset of the byte following the
// last-read token — equivalent to where tokenizer.Buffered() begins, but in
// O(1) rather than the former O(n) bytes.Index scan from byte 0 (#608, N1).
//
// When the cursor is unavailable (content not saved, e.g. older callers /
// tests) we fall back to tokenizer.Buffered(): only the bytes currently in
// the bufio window, which is still a safe input for forward-scan lookahead.
func (s *tokenReaderState) remainingContent(tokenizer *html.Tokenizer) []byte {
	if len(s.content) == 0 {
		return tokenizer.Buffered()
	}
	if s.consumed < 0 || s.consumed > len(s.content) {
		// Defensive: cursor out of range (should not happen). Fall back
		// to Buffered() rather than panic on a bad slice.
		return tokenizer.Buffered()
	}
	return s.content[s.consumed:]
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

// flushPendingWS writes any buffered pure-WS tokens to skeleton and clears
// the buffer. Used when the surrounding context is structural-to-structural
// (e.g. between two leaf-block tags with no text-unit in between).
func (s *tokenReaderState) flushPendingWS() {
	for _, ws := range s.pendingWS {
		_ = s.store.WriteText(ws)
	}
	s.pendingWS = nil
}

// dropPendingWS discards buffered pure-WS tokens. Used when adjacent to a
// text-unit (Okapi-style trimming).
func (s *tokenReaderState) dropPendingWS() {
	s.pendingWS = nil
}

// trimTrailingWSOfLastTextBlock retroactively trims trailing HTML whitespace
// from the most recent top-level text-block. Called when a structural event
// follows a text-unit, so any trailing whitespace inside the unit (embedded
// in the last text-block's content) should be dropped to match Okapi.
func (s *tokenReaderState) trimTrailingWSOfLastTextBlock() {
	if s.lastTextBlock == nil {
		return
	}
	runs := s.lastTextBlock.Source
	if len(runs) == 0 {
		return
	}
	last := &runs[len(runs)-1]
	if last.Text == nil {
		return
	}
	last.Text.Text = strings.TrimRightFunc(last.Text.Text, isHTMLWhitespace)
}

// onStructuralEvent is called immediately before writing a top-level
// structural event (block-level tag, comment, doctype, script/style) to
// the skeleton. It applies Okapi's text-unit-adjacency trimming: if we
// just exited a text-unit, drop pendingWS and trim trailing whitespace of
// the last text-block; otherwise flush pendingWS (it sits between two
// structural events and is preserved).
func (s *tokenReaderState) onStructuralEvent() {
	if s.lastTextBlock != nil {
		s.trimTrailingWSOfLastTextBlock()
		s.dropPendingWS()
		s.lastTextBlock = nil
		return
	}
	s.flushPendingWS()
}

// onInlineEvent is called immediately before writing a top-level inline
// tag (e.g. <b>, </b>) to the skeleton. Inline tags are part of the
// text-unit, so any pendingWS is intra-unit and must be flushed.
func (s *tokenReaderState) onInlineEvent() {
	s.flushPendingWS()
}

// run processes HTML content with the tokenizer, writing skeleton data and
// emitting Parts to the channel.
func (s *tokenReaderState) run(content []byte, ctx context.Context, ch chan<- model.PartResult) {
	// Pre-scan: detect whether the document needs an injected
	// Content-Type meta. Okapi's HtmlFilter mirrors this logic — if a
	// <head> exists but no charset declaration is present, the writer
	// inserts <meta http-equiv="Content-Type" …> right after <head>.
	s.needsCharsetMeta = scanNeedsCharsetMeta(content)

	// Save full content for forwardScanForBlockChildren lookahead.
	s.content = content

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
//
// `head` is included so empty `<head></head>` doesn't get classified as
// a leaf block — leaf-block processing trims leading-newline whitespace
// from the runs, which would drop the source's `<head>\n</head>` newline
// and emit `<head><meta>...</head>` instead of okapi's `<head><meta>...\n</head>`.
var knownContainerElements = map[atom.Atom]bool{
	atom.Head:  true,
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

// pImplicitClosers lists the block-level start tags that implicitly close
// an open `<p>` per HTML5 §13.2.6.4.7 ("in body" insertion mode). When
// processLeafBlock is collecting `<p>` content and sees one of these, it
// stops collecting and bounces the start tag back to the main loop so the
// element is processed at the parent scope (mirrors the auto-close NekoHTML
// performs and that okapi's HtmlFilter relies on).
//
// HTML5 §13.2.6.4.7 enumerates the start tags that, when seen in "in body"
// insertion mode while a `<p>` is in button scope, first emit an end tag
// for the open `<p>`: address, article, aside, blockquote, center, details,
// dialog, dir, div, dl, fieldset, figcaption, figure, footer, h1-h6, header,
// hgroup, main, menu, nav, ol, p, plaintext, pre, section, summary, ul, hr,
// form, table, listing, xmp.
var pImplicitClosers = map[atom.Atom]bool{
	atom.Address:    true,
	atom.Article:    true,
	atom.Aside:      true,
	atom.Blockquote: true,
	atom.Center:     true,
	atom.Details:    true,
	atom.Dialog:     true,
	atom.Dir:        true,
	atom.Div:        true,
	atom.Dl:         true,
	atom.Fieldset:   true,
	atom.Figcaption: true,
	atom.Figure:     true,
	atom.Footer:     true,
	atom.Form:       true,
	atom.H1:         true, atom.H2: true, atom.H3: true,
	atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Header:  true,
	atom.Hgroup:  true,
	atom.Hr:      true,
	atom.Listing: true,
	atom.Main:    true,
	atom.Menu:    true,
	atom.Nav:     true,
	atom.Ol:      true,
	atom.P:       true,
	atom.Pre:     true,
	atom.Section: true,
	atom.Summary: true,
	atom.Table:   true,
	atom.Ul:      true,
	atom.Xmp:     true,
}

// implicitlyClosesLeaf reports whether childAtom (a block-level start tag
// just seen inside the leaf currently being collected by processLeafBlock)
// auto-closes that leaf per HTML5 parsing rules. Currently only `<p>` has
// implicit-close handling; the other knownLeafElements (`<title>`, `<pre>`,
// `<h1-6>`, etc.) cannot legally contain block children and historically
// emitted their bytes inline anyway.
func implicitlyClosesLeaf(leafAtom, childAtom atom.Atom) bool {
	if leafAtom == atom.P {
		return pImplicitClosers[childAtom]
	}
	return false
}

// synthesizeOrphanInlineClose appends a PcClose run with empty Data for an
// inline element that was implicitly closed by an HTML5-illegal block-level
// child (adoption agency). Empty Data means the writer renders nothing for
// the close (mirrors NekoHTML, which closes the inline silently and reopens
// it later if the next legal context permits). Keeps run pairs balanced for
// downstream tools that assume PcOpen/PcClose come in pairs.
func (s *tokenReaderState) synthesizeOrphanInlineClose(b *runBuilder, parentTag string, parentInfo *model.SpanTypeInfo) {
	semType := htmlSemanticType(parentTag)
	subType := "html:" + parentTag
	equiv := ""
	if parentInfo != nil {
		equiv = parentInfo.Equiv
	}
	b.AddPcClose(
		findOpeningRunID(b, semType),
		semType,
		subType,
		"",
		equiv,
	)
}

// deferredStartTag carries the pre-fetched fields of a start tag that the
// main loop should replay without calling tokenizer.Next() / TagName() /
// collectTokenAttrs again. Used by processLeafBlock to bounce an
// HTML5-implicit-close block-level start back to the outer scope.
type deferredStartTag struct {
	raw   []byte
	tag   string
	a     atom.Atom
	attrs []html.Attribute
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
	defer func() {
		// Flush any whitespace still buffered at end-of-document. The
		// document boundary is treated like a structural event for trailing
		// whitespace inside the last text-block.
		s.onStructuralEvent()
	}()

	for {
		// Replay any start tag deferred by processLeafBlock (HTML5
		// implicit-close: e.g. <p>...<table>... auto-closes <p> and
		// processes <table> at the outer scope).
		if s.deferredStart != nil {
			d := s.deferredStart
			s.deferredStart = nil
			s.processStartTag(tokenizer, d.raw, d.tag, d.a, d.attrs, &stack, &translateNo, ctx, ch)
			continue
		}

		tt := s.next(tokenizer)
		if tt == html.ErrorToken {
			break
		}
		if err := ctx.Err(); err != nil {
			return
		}

		raw := copyBytes(tokenizer.Raw())

		switch tt {
		case html.DoctypeToken:
			s.onStructuralEvent()
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
			s.onStructuralEvent()
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
				// Bare text node outside a block context — emit as text
				// block. Drop the leading HTML whitespace bytes that
				// precede the first non-whitespace rune; okapi's html
				// filter trims that prefix off the start of the text-unit.
				// Pending pure-WS buffered before this text-block is also
				// adjacent to a text-unit, so drop it. Inside preserve-
				// whitespace elements (pre/textarea), keep the raw text
				// intact (and don't touch pendingWS).
				//
				// HTML entities (`&amp;`, `&nbsp;`, …) are peeled into
				// inline placeholder runs so they survive pseudo-translation
				// as opaque codes rather than having their letters
				// substituted (e.g. `&amp;` → `&àmƥ;`). This mirrors okapi's
				// HtmlFilter, which treats entity references as inline
				// `<code>` pairs. Same logic as addTextWithEntities in the
				// leaf-block / inline-collection paths.
				if preserveWS {
					blockID := s.nextBlockID()
					_ = s.store.WriteRef(blockID)
					block := buildBlockWithEntities(blockID, text)
					block.PreserveWhitespace = true
					s.reader.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
				} else {
					s.dropPendingWS()
					body := text
					// Only trim leading whitespace at the start of a new
					// text-unit. When `lastTextBlock` is set, this text is
					// the continuation of an existing unit (split across an
					// inline boundary like `</i>`), and the leading space is
					// intra-unit content — okapi preserves it. Mirrors
					// HtmlFilter's behaviour of trimming the unit's leading
					// whitespace once and treating subsequent text-runs as
					// part of the same unit.
					if s.lastTextBlock == nil {
						body = trimLeadingHTMLWhitespace(text)
					}
					blockID := s.nextBlockID()
					_ = s.store.WriteRef(blockID)
					block := buildBlockWithEntities(blockID, body)
					s.reader.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
					s.lastTextBlock = block
				}
			} else {
				// Pure-whitespace text token at top level: buffer it. The
				// next non-WS event decides whether to flush (between two
				// structural tags) or drop (adjacent to a text-unit).
				s.pendingWS = append(s.pendingWS, raw)
			}

		case html.StartTagToken:
			tagName, hasAttr := tokenizer.TagName()
			tag := string(tagName)
			a := atom.Lookup(tagName)

			var attrs []html.Attribute
			if hasAttr {
				attrs = collectTokenAttrs(tokenizer)
			}

			s.processStartTag(tokenizer, raw, tag, a, attrs, &stack, &translateNo, ctx, ch)

		case html.EndTagToken:
			tagName, _ := tokenizer.TagName()
			tag := string(tagName)
			endAtom := atom.Lookup(tagName)

			// Pop stack.
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				if top.tag == tag {
					stack = stack[:len(stack)-1]
				}
			}

			// Inline end tag is part of the text-unit; structural end tag
			// (container close) ends it.
			if inlineElements[endAtom] {
				s.onInlineEvent()
			} else {
				s.onStructuralEvent()
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
				s.onStructuralEvent()
				s.handleMetaToken(raw, attrs, ctx, ch)
				continue
			}

			if inlineElements[a] {
				s.onInlineEvent()
			} else {
				s.onStructuralEvent()
			}

			s.extractLangFromToken(raw, tag, attrs, ctx, ch)
			if !translateNo {
				s.extractTokenAttrs(raw, tag, a, attrs, ctx, ch)
			} else {
				// translate="no": no translatable attributes are extracted,
				// but the language declaration is still spliced out as a
				// typed SkeletonLang entry so the writer can retarget it.
				s.writeStartTagSkeleton(raw, nil, langAttrKeys(attrs))
			}
		}
	}
}

// processStartTag handles a single StartTagToken. Extracted from the main
// loop so it can be replayed by the deferred-start mechanism (HTML5
// implicit-close: a block-level start inside `<p>` etc. auto-closes the
// leaf and the block tag is processed at the parent level).
func (s *tokenReaderState) processStartTag(tokenizer *html.Tokenizer, raw []byte, tag string, a atom.Atom, attrs []html.Attribute, stack *[]elementInfo, translateNo *bool, ctx context.Context, ch chan<- model.PartResult) {
	info := elementInfo{
		tag:         tag,
		a:           a,
		translateNo: *translateNo,
	}

	if tv := getTokenAttr(attrs, "translate"); tv != "" {
		if tv == "no" {
			info.translateNo = true
		} else if tv == "yes" {
			info.translateNo = false
		}
	}

	if nonTranslatableElements[a] {
		s.onStructuralEvent()
		_ = s.store.WriteText(raw)
		s.reader.emit(ctx, ch, &model.Part{
			Type: model.PartData,
			Resource: &model.Data{
				ID:   s.nextDataID(),
				Name: tag,
			},
		})
		s.consumeUntilClose(tokenizer, tag, ctx, ch)
		*stack = append(*stack, info)
		return
	}

	if selfClosingElements[a] {
		if a == atom.Meta {
			s.onStructuralEvent()
			s.handleMetaToken(raw, attrs, ctx, ch)
			return
		}
		if inlineElements[a] {
			s.onInlineEvent()
		} else {
			s.onStructuralEvent()
		}
		s.extractLangFromToken(raw, tag, attrs, ctx, ch)
		if !info.translateNo {
			s.extractTokenAttrs(raw, tag, a, attrs, ctx, ch)
		} else {
			s.writeStartTagSkeleton(raw, nil, langAttrKeys(attrs))
		}
		return
	}

	isInline := inlineElements[a]

	if !isInline {
		info.isBlock = true
		info.preserveWS = s.cfg.PreserveWhitespace || preserveWhitespaceElements[a]

		s.onStructuralEvent()
		s.extractLangFromToken(nil, tag, attrs, ctx, ch)

		if !info.translateNo {
			s.extractTokenAttrs(raw, tag, a, attrs, ctx, ch)
		} else {
			s.writeStartTagSkeleton(raw, nil, langAttrKeys(attrs))
		}

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
			*stack = append(*stack, info)
			*translateNo = info.translateNo
			return
		}

		var hasBlockKids bool
		if knownContainerElements[a] {
			hasBlockKids = true
		} else if !knownLeafElements[a] {
			// Prefer scanning the full remaining content rather than
			// just tokenizer.Buffered(): after a giant <script> body
			// the buffered window may stop short of the next `</td>`,
			// causing forwardScan to exhaust and default to container,
			// which mis-classifies TEXTUNIT-typed parents and loses
			// translatable inline content (e.g. dropped whitespace
			// between text and `<input>` because the parent is a
			// container, not a leaf — its inline child becomes a
			// bare-text block whose trailing WS is then trimmed by
			// trimTrailingWSOfLastTextBlock when the input self-
			// closing tag follows).
			remaining := s.remainingContent(tokenizer)
			hasBlockKids = s.forwardScanForBlockChildren(remaining, tag)
		}
		info.hasBlockKids = hasBlockKids

		if hasBlockKids {
			*stack = append(*stack, info)
			*translateNo = info.translateNo
			return
		}

		s.processLeafBlock(tokenizer, tag, a, attrs, info.preserveWS, ctx, ch)
		info.isBlock = false
		*stack = append(*stack, info)
		*translateNo = info.translateNo
		return
	}

	s.onInlineEvent()
	if !info.translateNo {
		s.extractTokenAttrs(raw, tag, a, attrs, ctx, ch)
	} else {
		s.writeStartTagSkeleton(raw, nil, langAttrKeys(attrs))
	}
	*stack = append(*stack, info)
	*translateNo = info.translateNo
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
			break
		}
		tt := s.next(tokenizer)
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

			if isInlineAtom(childAtom) || childAtom == 0 {
				// Unknown elements (atom == 0, e.g. `<exclude>`) are
				// treated as inline placeholders so their tags
				// survive verbatim. Mirrors okapi's HtmlFilter, which
				// captures unrecognised markup as inline `<code>` pairs.
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
					// If the recursion saw a block-level start tag that
					// HTML5-implicitly closed this inline (and therefore
					// also this leaf, since leaves can't legally contain
					// block children), unwind the leaf so the main loop
					// can replay the deferred start. See the inline
					// implicit-close branch in collectInlineTokens for
					// the spec citation.
					if s.deferredStart != nil {
						goto leafClosed
					}
				}
			} else if implicitlyClosesLeaf(a, childAtom) {
				// HTML5 implicit close: a block-level start tag inside a
				// leaf (e.g. `<p><table>`) auto-closes the leaf. Defer
				// the start back to the main loop and end this leaf
				// without a close tag (the writer omits it cleanly).
				// Mirrors NekoHTML's behaviour, which okapi relies on.
				s.deferredStart = &deferredStartTag{
					raw:   tokenRaw,
					tag:   childTag,
					a:     childAtom,
					attrs: childAttrs,
				}
				goto leafClosed
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
			// For truly-unknown elements (atom == 0, e.g. </pub>), okapi's
			// HtmlFilter swallows any whitespace immediately following the
			// orphan close tag — peel it from the next text token to match.
			// Known-element stray closes (</br>, </font>, …) keep their
			// trailing whitespace because okapi treats them as recognised
			// inline codes with normal spacing.
			if atom.Lookup(endTagName) == 0 {
				b.MarkDropNextLeadingSpace()
			}

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
leafClosed:

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
			Source:             b.Runs(),
			Targets:            make(map[model.VariantKey]*model.Target),
			Properties:         extractBlockPropsFromToken(attrs),
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
		tt := s.next(tokenizer)
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

			if isInlineAtom(childAtom) || childAtom == 0 {
				// Unknown elements (atom == 0, e.g. `<exclude>`) are
				// treated as inline placeholders so their start/end
				// tags survive the round-trip verbatim. Mirrors okapi's
				// HtmlFilter, which preserves any unrecognised markup
				// inside translatable text as inline `<code>` pairs.
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
					// If the inner recursion saw a block-level start tag
					// (HTML5 adoption agency: e.g. `<a><h3>` implicitly
					// closes the `<a>` and the enclosing leaf), unwind
					// this frame too so processLeafBlock can flush.
					if s.deferredStart != nil {
						s.synthesizeOrphanInlineClose(b, parentTag, parentInfo)
						return
					}
				}
			} else {
				// Block-level start tag inside an inline span. HTML5
				// §13.2.6.4.7 "in body" insertion mode + adoption agency:
				// any flow-content block start (`<h1-6>`, `<p>`, `<table>`,
				// `<div>`, …) implicitly closes the open phrasing-content
				// element (`<a>`, `<font>`, `<b>`, …) and the enclosing
				// leaf block. NekoHTML follows this; without the unwind
				// our reader silently dropped the block start tag and
				// turned its end tag into a stray-Ph (e.g.
				// `<a><h3>title</h3></a>` lost the `<h3>` opener and
				// emitted an orphan `</h3>` placeholder).
				//
				// Defer the block start back to the main loop and
				// synthesize an empty close for this inline run so the
				// emitted runs stay balanced (the close has empty Data
				// so the writer renders nothing — matches NekoHTML's
				// "close-and-reopen" effect for the immediate output).
				s.deferredStart = &deferredStartTag{
					raw:   tokenRaw,
					tag:   childTag,
					a:     childAtom,
					attrs: childAttrs,
				}
				s.synthesizeOrphanInlineClose(b, parentTag, parentInfo)
				return
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
			// Same okapi parity rule as processLeafBlock: drop one leading
			// whitespace char from the next text token whenever we just
			// consumed an unknown orphan close (atom == 0). simple_subscript
			// uses </pub>; anything else (</br>, </font>, …) keeps its
			// surrounding spacing intact.
			if atom.Lookup(endTagName) == 0 {
				b.MarkDropNextLeadingSpace()
			}

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
			// An end-tag matching parentTag closes the parent regardless
			// of nested-inline depth: HTML5 §13.2.6.4.7 "in body" insertion
			// mode treats unclosed inline phrasing-content (e.g. an open
			// `<font>` when `</td>` arrives) as implicitly closed by the
			// adoption agency algorithm — the `</td>` does belong to the
			// parent, not to the open inline. Without this, sources with
			// unclosed inline tags (`<td><font>text </td>`) cause the
			// scanner to keep walking past `</td>`, exhaust the buffer,
			// and misclassify the parent as a container — collapsing the
			// translatable text into a sequence of raw skeleton tags.
			if endTag == parentTag {
				return false
			}
			if depth > 0 {
				depth--
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
		tt := s.next(tokenizer)
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
		tt := s.next(tokenizer)
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
		// `<meta http-equiv="Content-Language" content="en">` declares the
		// document language exactly like lang/xml:lang. Okapi's HTML filter
		// normalizes this content attribute to Property.LANGUAGE
		// (HtmlFilter.normalizeAttributeName) and GenericSkeletonWriter
		// retargets it to the output locale, so splice the content value out
		// as a SkeletonLang entry rather than writing it verbatim — otherwise
		// the declaration would keep the stale source locale on translation.
		s.writeStartTagSkeleton(raw, nil, []string{"content"})
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
			s.writeAttrRefSkeleton(raw, "content", blockID)

			block := &model.Block{
				ID:           blockID,
				Name:         metaName,
				Type:         "content",
				Translatable: true,
				IsReferent:   true,
				Source:       []model.Run{{Text: &model.TextRun{Text: content}}},
				Targets:      make(map[model.VariantKey]*model.Target),
				Properties:   make(map[string]string),
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

// extractLangFromToken extracts lang/xml:lang attributes, emitting a Data
// part carrying the declared language. The skeleton write for the start tag
// happens elsewhere (extractTokenAttrs / writeStartTagSkeleton); those paths
// splice the lang value out as a typed SkeletonLang entry so the writer can
// retarget the document language structurally — see langAttrKeys.
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

// langAttrKeys returns the literal attribute key forms ("lang" and/or
// "xml:lang") of language declarations carrying a non-empty value on a tag,
// or nil when none is present. Each returned key is the form that appears in
// the raw start-tag bytes, so writeStartTagSkeleton can locate it with
// findAttrValueRange and splice the value out as a SkeletonLang entry.
//
// An XHTML element may legitimately carry BOTH lang= and xml:lang= (HTML5
// §3.2.6.1: "the lang and xml:lang attributes [...] in a conforming document
// have the same value"). Both must be spliced and retargeted together; the
// upstream Okapi HTML filter normalizes both to the single Property.LANGUAGE
// (HtmlFilter.normalizeAttributeName), and GenericSkeletonWriter retargets
// every language property to the output locale, so omitting one would leave a
// stale source-locale declaration (the W3CHTMHLTest1 divergence: native
// emitted xml:lang="en" lang="fr").
func langAttrKeys(attrs []html.Attribute) []string {
	var keys []string
	if getTokenAttr(attrs, "lang") != "" {
		keys = append(keys, "lang")
	}
	if getTokenAttrNS(attrs, "xml", "lang") != "" {
		keys = append(keys, "xml:lang")
	}
	return keys
}

// extractTokenAttrs extracts translatable attributes (title, alt, etc.) from a token.
// If raw is not nil, it writes the tag raw bytes to skeleton (with attr refs as needed).
func (s *tokenReaderState) extractTokenAttrs(raw []byte, tag string, a atom.Atom, attrs []html.Attribute, ctx context.Context, ch chan<- model.PartResult) {
	// Collect which attributes are translatable.
	var transAttrs []transAttrEntry

	if title := getTokenAttrLastNonEmpty(attrs, "title"); title != "" {
		id := s.nextBlockID()
		transAttrs = append(transAttrs, transAttrEntry{"title", title, id})
		s.emitAttrBlock(id, "title", title, ctx, ch)
	}

	if alt := getTokenAttrLastNonEmpty(attrs, "alt"); alt != "" {
		// okapi okf_html: alt is per-element only — img, area (no
		// condition) and input (NOT_EQUALS [file, hidden, image,
		// Password]). See nonwellformedConfiguration.yml lines 41
		// (img), 81 (area), 263 (input).
		switch a {
		case atom.Img, atom.Area:
			id := s.nextBlockID()
			transAttrs = append(transAttrs, transAttrEntry{"alt", alt, id})
			s.emitAttrBlock(id, "alt", alt, ctx, ch)
		case atom.Input:
			if isTranslatableInputValue(strings.ToLower(getTokenAttr(attrs, "type"))) {
				id := s.nextBlockID()
				transAttrs = append(transAttrs, transAttrEntry{"alt", alt, id})
				s.emitAttrBlock(id, "alt", alt, ctx, ch)
			}
		}
	}

	if label := getTokenAttrLastNonEmpty(attrs, "label"); label != "" {
		if a == atom.Option {
			id := s.nextBlockID()
			transAttrs = append(transAttrs, transAttrEntry{"label", label, id})
			s.emitAttrBlock(id, "label", label, ctx, ch)
		}
	}

	if ph := getTokenAttrLastNonEmpty(attrs, "placeholder"); ph != "" {
		if a == atom.Input || a == atom.Textarea {
			id := s.nextBlockID()
			transAttrs = append(transAttrs, transAttrEntry{"placeholder", ph, id})
			s.emitAttrBlock(id, "placeholder", ph, ctx, ch)
		}
	}

	if val := getTokenAttrLastNonEmpty(attrs, "value"); val != "" && a == atom.Input {
		inputType := strings.ToLower(getTokenAttr(attrs, "type"))
		if isTranslatableInputValue(inputType) {
			id := s.nextBlockID()
			transAttrs = append(transAttrs, transAttrEntry{"value", val, id})
			s.emitAttrBlock(id, "value", val, ctx, ch)
		}
	}

	// `accesskey`: per-element on a, area, button, label, legend, textarea
	// (no condition) and input (NOT_EQUALS [file, hidden, image, Password]).
	// See nonwellformedConfiguration.yml lines 81, 135, 180, 222, 265, 277, 339.
	if ak := getTokenAttrLastNonEmpty(attrs, "accesskey"); ak != "" {
		emit := false
		switch a {
		case atom.A, atom.Area, atom.Button, atom.Label, atom.Legend, atom.Textarea:
			emit = true
		case atom.Input:
			emit = isTranslatableInputValue(strings.ToLower(getTokenAttr(attrs, "type")))
		}
		if emit {
			id := s.nextBlockID()
			transAttrs = append(transAttrs, transAttrEntry{"accesskey", ak, id})
			s.emitAttrBlock(id, "accesskey", ak, ctx, ch)
		}
	}

	// Write skeleton data. The language attribute (if any) is spliced out as
	// a typed SkeletonLang entry alongside the translatable-attribute refs in
	// a single offset-sorted pass, so the writer can retarget the document
	// language structurally instead of rewriting serialized bytes (#604).
	if raw != nil {
		s.writeStartTagSkeleton(raw, transAttrs, langAttrKeys(attrs))
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
	if title := getTokenAttrLastNonEmpty(attrs, "title"); title != "" {
		id := s.nextBlockID()
		s.emitAttrBlock(id, "title", title, ctx, ch)
		out = append(out, transAttrEntry{"title", title, id})
	}
	if alt := getTokenAttrLastNonEmpty(attrs, "alt"); alt != "" {
		// Same okapi okf_html rule as extractTokenAttrs.
		switch a {
		case atom.Img, atom.Area:
			id := s.nextBlockID()
			s.emitAttrBlock(id, "alt", alt, ctx, ch)
			out = append(out, transAttrEntry{"alt", alt, id})
		case atom.Input:
			if isTranslatableInputValue(strings.ToLower(getTokenAttr(attrs, "type"))) {
				id := s.nextBlockID()
				s.emitAttrBlock(id, "alt", alt, ctx, ch)
				out = append(out, transAttrEntry{"alt", alt, id})
			}
		}
	}
	if label := getTokenAttrLastNonEmpty(attrs, "label"); label != "" && a == atom.Option {
		id := s.nextBlockID()
		s.emitAttrBlock(id, "label", label, ctx, ch)
		out = append(out, transAttrEntry{"label", label, id})
	}
	if ph := getTokenAttrLastNonEmpty(attrs, "placeholder"); ph != "" {
		if a == atom.Input || a == atom.Textarea {
			id := s.nextBlockID()
			s.emitAttrBlock(id, "placeholder", ph, ctx, ch)
			out = append(out, transAttrEntry{"placeholder", ph, id})
		}
	}
	if val := getTokenAttrLastNonEmpty(attrs, "value"); val != "" && a == atom.Input {
		inputType := strings.ToLower(getTokenAttr(attrs, "type"))
		if isTranslatableInputValue(inputType) {
			id := s.nextBlockID()
			s.emitAttrBlock(id, "value", val, ctx, ch)
			out = append(out, transAttrEntry{"value", val, id})
		}
	}

	// Same accesskey rule as extractTokenAttrs.
	if ak := getTokenAttrLastNonEmpty(attrs, "accesskey"); ak != "" {
		emit := false
		switch a {
		case atom.A, atom.Area, atom.Button, atom.Label, atom.Legend, atom.Textarea:
			emit = true
		case atom.Input:
			emit = isTranslatableInputValue(strings.ToLower(getTokenAttr(attrs, "type")))
		}
		if emit {
			id := s.nextBlockID()
			s.emitAttrBlock(id, "accesskey", ak, ctx, ch)
			out = append(out, transAttrEntry{"accesskey", ak, id})
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
		offset, length := findAttrValueRange(raw, a.key)
		if offset >= 0 {
			repls = append(repls, replacement{offset, length, a.blockID})
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
		Source:       []model.Run{{Text: &model.TextRun{Text: value}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
	}
	s.reader.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// writeAttrRefSkeleton writes a tag's raw bytes to skeleton, replacing one
// attribute value with a block reference. The raw byte range of the value is
// looked up afresh — using the decoded value's len here would be wrong for
// entity-bearing values whose decoded length differs from their raw byte
// length, and would corrupt the skeleton tail.
func (s *tokenReaderState) writeAttrRefSkeleton(raw []byte, attrKey, blockID string) {
	offset, length := findAttrValueRange(raw, attrKey)
	if offset < 0 {
		// Fallback: write whole tag then ref.
		_ = s.store.WriteText(raw)
		return
	}

	// Write up to the attribute value.
	_ = s.store.WriteText(raw[:offset])
	_ = s.store.WriteRef(blockID)
	// Write after the attribute value.
	_ = s.store.WriteText(raw[offset+length:])
}

// skelSpliceKind distinguishes how a spliced-out attribute-value byte range
// is re-emitted by the writer: as a translatable block reference, or as a
// language-attribute value to retarget structurally.
type skelSpliceKind int

const (
	spliceRef  skelSpliceKind = iota // -> SkeletonRef(payload = blockID)
	spliceLang                       // -> SkeletonLang(payload = source lang value)
)

// skelSplice describes one byte range inside a raw start tag that is replaced
// by a typed skeleton entry (rather than written as opaque SkeletonText).
type skelSplice struct {
	offset  int
	length  int
	kind    skelSpliceKind
	payload string // blockID for spliceRef, source lang value for spliceLang
}

// writeStartTagSkeleton writes a start tag's raw bytes to skeleton, splicing
// out translatable attribute values (as SkeletonRef) and the language
// attribute value (as SkeletonLang) so the writer can substitute or retarget
// them structurally. Both splice kinds share a single offset-sorted pass so
// that lang and translatable attributes on the SAME tag interleave correctly.
//
// langKeys is nil when no lang/xml:lang declaration is present (most tags);
// pass the literal key forms returned by langAttrKeys otherwise. When an
// element carries both lang= and xml:lang= (XHTML), both are spliced so the
// writer retargets every language declaration consistently.
func (s *tokenReaderState) writeStartTagSkeleton(raw []byte, transAttrs []transAttrEntry, langKeys []string) {
	splices := make([]skelSplice, 0, len(transAttrs)+len(langKeys))
	for _, a := range transAttrs {
		offset, length := findAttrValueRange(raw, a.key)
		if offset >= 0 {
			splices = append(splices, skelSplice{offset, length, spliceRef, a.blockID})
		}
	}
	for _, langKey := range langKeys {
		offset, length := findAttrValueRange(raw, langKey)
		if offset >= 0 {
			splices = append(splices, skelSplice{offset, length, spliceLang, string(raw[offset : offset+length])})
		}
	}

	if len(splices) == 0 {
		_ = s.store.WriteText(raw)
		return
	}

	// Sort by offset (ascending) so lang and translatable splices on the
	// same tag are emitted in document order.
	slices.SortFunc(splices, func(a, b skelSplice) int {
		return cmp.Compare(a.offset, b.offset)
	})

	pos := 0
	for _, sp := range splices {
		_ = s.store.WriteText(raw[pos:sp.offset])
		switch sp.kind {
		case spliceRef:
			_ = s.store.WriteRef(sp.payload)
		case spliceLang:
			_ = s.store.WriteLang(sp.payload)
		}
		pos = sp.offset + sp.length
	}
	_ = s.store.WriteText(raw[pos:])
}

// findAttrValueRange returns the byte offset and length of an attribute's value
// in raw tag bytes (i.e., the slice raw[off:off+length] is the original
// undecoded attribute-value text between the surrounding quotes, or the bare
// unquoted token). Returns -1, 0 if the attribute can't be located.
//
// Skeleton writers must use this length (not the decoded attribute string's
// len) when stitching skeleton text around the value — otherwise entity-bearing
// values like `&#x20000;` (1 decoded rune, 9 raw bytes) yield mismatched
// before/after slices that duplicate or drop trailing raw bytes.
// findAttrValueRange returns the (offset, length) of the value slot for
// attribute `attrKey` in raw. When the attribute appears multiple times
// (tag-soup duplicates like `alt="" alt="real"`), it returns the LAST
// occurrence whose value is non-empty — mirroring getTokenAttrLastNonEmpty
// so the substitute-translation pass writes into the same slot the
// extraction pass picked. Falls back to the first occurrence when every
// duplicate is empty.
func findAttrValueRange(raw []byte, attrKey string) (int, int) {
	keyBytes := []byte(strings.ToLower(attrKey))

	firstStart, firstLen := -1, 0
	bestStart, bestLen := -1, 0

	idx := 0
	for {
		pos := indexBytesInsensitive(raw[idx:], keyBytes)
		if pos < 0 {
			break
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
			break
		}

		var start, end int
		quote := raw[eqPos]
		if quote == '"' || quote == '\'' {
			start = eqPos + 1
			end = start
			for end < len(raw) && raw[end] != quote {
				end++
			}
			idx = end + 1
		} else {
			// Unquoted attribute value: terminate at whitespace, '>', or '/'.
			start = eqPos
			end = start
			for end < len(raw) {
				c := raw[end]
				if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' || c == '/' {
					break
				}
				end++
			}
			idx = end
		}

		if firstStart < 0 {
			firstStart, firstLen = start, end-start
		}
		if end > start {
			// Non-empty value — prefer this slot (last non-empty wins).
			bestStart, bestLen = start, end-start
		}
	}

	if bestStart >= 0 {
		return bestStart, bestLen
	}
	return firstStart, firstLen
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

// getTokenAttrLastNonEmpty returns the value of the LAST attribute named
// `key` with a non-empty value. When all occurrences are empty, falls back
// to the first occurrence (so callers see "" for "no real value present").
//
// Tag-soup HTML — common in scraped pages — frequently emits duplicate
// translatable attributes (e.g. `<img alt="" alt="Real text" />`) where
// the meaningful content sits in the second slot. HTML5 §13.2.2.1 says
// duplicates after the first are tokenizer errors, but golang.org/x/net/html
// surfaces all of them in the parsed Attribute slice. Okapi's NekoHTML
// preserves the meaningful (non-empty) attribute when extracting a
// translation unit. Use this helper for translatable attributes (alt,
// title, placeholder, value, label, accesskey) so the extraction picks
// the same slot Okapi did. The skeleton-write side still preserves the
// raw bytes of all duplicates verbatim — only the extraction target
// changes.
func getTokenAttrLastNonEmpty(attrs []html.Attribute, key string) string {
	out := ""
	found := false
	for _, a := range attrs {
		if a.Key == key {
			if !found {
				out = a.Val
				found = true
			}
			if a.Val != "" {
				out = a.Val
			}
		}
	}
	return out
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

// htmlEntityRE matches a single HTML entity reference: a named entity
// (`&amp;`), a numeric entity (`&#160;`), or a hex entity (`&#xA0;`).
// Used by addTextWithEntities to peel entities out of bare text into
// inline placeholder runs so they survive pseudo-translation as
// opaque codes (rather than getting their letters substituted to
// `&ĺţ;` etc.).
var htmlEntityRE = regexp.MustCompile(`&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]+|#[xX][0-9A-Fa-f]+);`)

// buildBlockWithEntities wraps NewBlock so bare-text blocks (the
// processTokenStream top-level path) get the same entity peeling as
// leaf-block / inline-collection paths. Without this a `<td>` whose
// content exceeds the tokenizer buffer (so forwardScanForBlockChildren
// returns the safe-default true and treats td as a container) would
// have its inner text emitted via NewBlock(text), and entities like
// `&amp;` would survive pseudo-translation as `&àmƥ;` because pseudo
// substitutes letters rune-by-rune. Mirrors okapi's HtmlFilter, which
// always wraps entity references as opaque inline codes regardless of
// whether the surrounding container is a leaf block or not.
func buildBlockWithEntities(blockID, text string) *model.Block {
	matches := htmlEntityRE.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return model.NewBlock(blockID, text)
	}
	b := newRunBuilder()
	idCounter := 0
	pos := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		if start > pos {
			b.AddText(text[pos:start])
		}
		idCounter++
		b.AddPh(
			strconv.Itoa(idCounter),
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
	return &model.Block{
		ID:           blockID,
		Translatable: true,
		Source:       b.runs,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
	}
}

// addTextWithEntities adds raw HTML text to a runBuilder, splitting
// out HTML entity references as inline placeholders so they don't get
// pseudo-translated character by character. The entity's source bytes
// (`&amp;`, `&#160;`) become the placeholder Data and are written
// back verbatim, mirroring okapi's HtmlFilter behavior of treating
// entities as opaque inline codes.
func addTextWithEntities(b *runBuilder, text string, idCounter *int) {
	if b.dropNextLeadingSpace {
		b.dropNextLeadingSpace = false
		text = trimLeadingHTMLWhitespace(text)
	}
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
	for i := range len(s) {
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
