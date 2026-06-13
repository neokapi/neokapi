package html

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/safeio"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// walkVisitor receives callbacks during DOM traversal. The walker handles
// sequential ID assignment and structural decisions; the visitor handles
// what to do at each translatable point.
type walkVisitor interface {
	// onData is called for non-translatable structural nodes (doctype, comment,
	// script/style elements, meta structure, lang attributes).
	onData(dataID string, n *html.Node, dataName string, props map[string]string)

	// onTextBlock is called for bare text nodes that become blocks.
	onTextBlock(blockID string, n *html.Node)

	// onAttributeBlock is called for translatable attributes (title, alt, etc.).
	onAttributeBlock(blockID string, n *html.Node, attrKey string)

	// onMetaBlock is called for translatable meta content attributes.
	onMetaBlock(blockID string, n *html.Node)

	// onBlockElement is called for block-level elements with inline content
	// that should be emitted as a single block. The walker has already
	// processed translatable attributes on inline children.
	onBlockElement(blockID string, n *html.Node, preserveWS bool)

	// onMixedContentBlock is called for a run of inline/text content within
	// a block element that has both block and inline children. The run spans
	// from runStart up to (but not including) runEnd.
	onMixedContentBlock(blockID string, parent *html.Node, runStart, runEnd *html.Node, preserveWS bool)
}

// domWalker traverses a parsed HTML DOM, assigning sequential block/data IDs
// and calling visitor methods at each translatable point.
type domWalker struct {
	cfg          *Config
	blockCounter int
	dataCounter  int
	visitor      walkVisitor
	// depth bounds the recursive DOM/inline descent so a pathologically nested
	// document degrades to a clean truncation instead of a Go stack-overflow
	// panic. It uses the shared safeio default depth (see core/safeio); valid
	// localization documents nest far below the cap, so the guard never trips
	// for real input and behavior is unchanged.
	depth *safeio.DepthGuard
}

func newDOMWalker(cfg *Config, v walkVisitor) *domWalker {
	return &domWalker{cfg: cfg, visitor: v, depth: safeio.DefaultBudget().DepthGuard()}
}

func (w *domWalker) nextBlockID() string {
	w.blockCounter++
	return fmt.Sprintf("tu%d", w.blockCounter)
}

func (w *domWalker) nextDataID() string {
	w.dataCounter++
	return fmt.Sprintf("d%d", w.dataCounter)
}

// walk traverses the entire document tree.
func (w *domWalker) walk(doc *html.Node) {
	w.walkNode(doc, false)
}

func (w *domWalker) walkNode(n *html.Node, translateNo bool) {
	// Bound recursion depth: a pathologically nested tree is truncated with a
	// clean return rather than overflowing the goroutine stack (see core/safeio).
	if w.depth.Enter() != nil {
		return
	}
	defer w.depth.Leave()

	switch n.Type {
	case html.DocumentNode:
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			w.walkNode(child, translateNo)
		}

	case html.DoctypeNode:
		w.visitor.onData(w.nextDataID(), n, "doctype", nil)

	case html.CommentNode:
		w.visitor.onData(w.nextDataID(), n, "comment", nil)

	case html.ElementNode:
		w.walkElement(n, translateNo)

	case html.TextNode:
		if translateNo {
			return
		}
		text := n.Data
		if !w.cfg.PreserveWhitespace {
			text = collapseWhitespace(text)
			text = strings.TrimFunc(text, isHTMLWhitespace)
		}
		if text != "" {
			w.visitor.onTextBlock(w.nextBlockID(), n)
		}
	}
}

func (w *domWalker) walkElement(n *html.Node, translateNo bool) {
	elemTranslateNo := translateNo
	if tv := getAttr(n, "translate"); tv != "" {
		if tv == "no" {
			elemTranslateNo = true
		} else if tv == "yes" {
			elemTranslateNo = false
		}
	}

	// Non-translatable elements (script, style).
	if nonTranslatableElements[n.DataAtom] {
		w.visitor.onData(w.nextDataID(), n, n.Data, nil)
		return
	}

	// META tag handling.
	if n.DataAtom == atom.Meta {
		w.handleMetaTag(n)
		return
	}

	// lang/xml:lang attributes.
	w.extractLangAttribute(n)

	// Translatable attributes.
	w.extractTranslatableAttributes(n, elemTranslateNo)

	// Block-level element handling.
	if !isInlineElement(n) {
		if elemTranslateNo {
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				w.walkNode(child, elemTranslateNo)
			}
			return
		}

		if w.hasBlockLevelChildren(n) {
			w.processBlockWithMixedContent(n, elemTranslateNo)
			return
		}

		if w.hasAnyContent(n) || getAttr(n, "id") != "" {
			preserveWS := w.cfg.PreserveWhitespace || preserveWhitespaceElements[n.DataAtom]

			// Walk inline content to count spans and advance attribute counters.
			idCounter := 0
			w.walkInlineChildren(n, &idCounter, elemTranslateNo)

			hasID := getAttr(n, "id") != ""
			text := collectPlainText(n, preserveWS)
			if text != "" || idCounter > 0 || hasID {
				w.visitor.onBlockElement(w.nextBlockID(), n, preserveWS)
				return
			}
		}
	}

	// Container element without direct text: recurse into children.
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		w.walkNode(child, elemTranslateNo)
	}
}

func (w *domWalker) handleMetaTag(n *html.Node) {
	httpEquiv := strings.ToLower(getAttr(n, "http-equiv"))
	metaName := strings.ToLower(getAttr(n, "name"))
	content := getAttr(n, "content")
	charset := getAttr(n, "charset")

	if charset != "" {
		w.visitor.onData(w.nextDataID(), n, "meta", map[string]string{"encoding": charset})
		return
	}

	if httpEquiv == "content-type" && content != "" {
		if cs := extractCharset(content); cs != "" {
			w.visitor.onData(w.nextDataID(), n, "meta", map[string]string{"encoding": cs})
			return
		}
	}

	if httpEquiv == "content-language" && content != "" {
		w.visitor.onData(w.nextDataID(), n, "meta", map[string]string{"language": content})
		return
	}

	if content != "" {
		isTranslatable := httpEquiv == "keywords" || translatableMetaNames[metaName]
		if isTranslatable {
			w.visitor.onMetaBlock(w.nextBlockID(), n)
		}
	}

	w.visitor.onData(w.nextDataID(), n, "meta", nil)
}

func (w *domWalker) extractLangAttribute(n *html.Node) {
	lang := getAttr(n, "lang")
	if lang == "" {
		lang = getAttrNS(n, "xml", "lang")
	}
	if lang != "" {
		w.visitor.onData(w.nextDataID(), n, n.Data, map[string]string{"language": lang})
	}
}

func (w *domWalker) extractTranslatableAttributes(n *html.Node, translateNo bool) {
	if translateNo {
		return
	}

	if title := getAttr(n, "title"); title != "" {
		w.visitor.onAttributeBlock(w.nextBlockID(), n, "title")
	}

	if alt := getAttr(n, "alt"); alt != "" {
		// okapi okf_html: per-element rules only — there is no global
		// rule for `alt` (see nonwellformedConfiguration.yml). The
		// element rules cover img, area (no condition), and input
		// (NOT_EQUALS [file, hidden, image, Password]).
		switch n.DataAtom {
		case atom.Img, atom.Area:
			w.visitor.onAttributeBlock(w.nextBlockID(), n, "alt")
		case atom.Input:
			// alt and value share the same NOT_EQUALS exclusion list in
			// okapi okf_html — reuse the helper.
			if isTranslatableInputValue(strings.ToLower(getAttr(n, "type"))) {
				w.visitor.onAttributeBlock(w.nextBlockID(), n, "alt")
			}
		}
	}

	if label := getAttr(n, "label"); label != "" {
		if n.DataAtom == atom.Option {
			w.visitor.onAttributeBlock(w.nextBlockID(), n, "label")
		}
	}

	if ph := getAttr(n, "placeholder"); ph != "" {
		if n.DataAtom == atom.Input || n.DataAtom == atom.Textarea {
			// okapi rule: placeholder NOT_EQUALS 'dummy' — the literal
			// string "dummy" is treated as a placeholder-of-a-placeholder.
			if strings.ToLower(getAttr(n, "type")) != "dummy" {
				w.visitor.onAttributeBlock(w.nextBlockID(), n, "placeholder")
			}
		}
	}

	if val := getAttr(n, "value"); val != "" && n.DataAtom == atom.Input {
		inputType := strings.ToLower(getAttr(n, "type"))
		if isTranslatableInputValue(inputType) {
			w.visitor.onAttributeBlock(w.nextBlockID(), n, "value")
		}
	}

	// `accesskey` extracted on a, area, button, label, legend, textarea
	// (no condition) and input (NOT_EQUALS [file, hidden, image, Password]).
	// See nonwellformedConfiguration.yml lines 81, 135, 180, 222, 265, 277, 339.
	if ak := getAttr(n, "accesskey"); ak != "" {
		switch n.DataAtom {
		case atom.A, atom.Area, atom.Button, atom.Label, atom.Legend, atom.Textarea:
			w.visitor.onAttributeBlock(w.nextBlockID(), n, "accesskey")
		case atom.Input:
			if isTranslatableInputValue(strings.ToLower(getAttr(n, "type"))) {
				w.visitor.onAttributeBlock(w.nextBlockID(), n, "accesskey")
			}
		}
	}
}

func (w *domWalker) processBlockWithMixedContent(n *html.Node, translateNo bool) {
	preserveWS := w.cfg.PreserveWhitespace || preserveWhitespaceElements[n.DataAtom]

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode || (child.Type == html.ElementNode && isInlineElement(child)) {
			var textBuf strings.Builder
			idCounter := 0
			runStart := child
			// Collect the inline elements in this run so the standalone-inline
			// rule (documented below) can decide how the run is emitted.
			var inlineKids []*html.Node
			for child != nil && (child.Type == html.TextNode ||
				child.Type == html.CommentNode ||
				(child.Type == html.ElementNode && isInlineElement(child))) {
				switch child.Type {
				case html.TextNode:
					textBuf.WriteString(child.Data)
				case html.CommentNode:
					idCounter++
				case html.ElementNode:
					w.extractTranslatableAttributes(child, translateNo)
					w.walkInlineChildren(child, &idCounter, translateNo)
					inlineKids = append(inlineKids, child)
				}
				child = child.NextSibling
			}

			text := textBuf.String()
			if !preserveWS {
				text = collapseWhitespace(text)
				text = strings.TrimFunc(text, isHTMLWhitespace)
			}
			// hasDirectText: the run carries direct (sibling-level) non-
			// whitespace text gluing its inline elements into one unit.
			hasDirectText := text != ""

			// Standalone-inline rule.
			//
			// A "mixed-content run" is the maximal sequence of text + inline
			// elements between two block-level boundaries inside a block
			// CONTAINER (an element that also has block-level children, e.g.
			// <body>, <div>, <section> with sibling <p>/<h1>/...). How the
			// run becomes translatable blocks depends on whether it has any
			// direct (sibling-level) non-whitespace text:
			//
			//  - The run HAS direct text (e.g. "Lead <b>bold</b> tail"):
			//    inline elements are glued to surrounding text, so they stay
			//    inline runs inside ONE combined block. This is the original
			//    behavior — an inline element WITHIN a block's text never
			//    becomes its own block.
			//
			//  - The run has NO direct text — only standalone inline elements
			//    separated by whitespace (e.g. "<button>Pay now</button>
			//    <a>Need help?</a>"): each inline element that carries
			//    translatable content becomes its OWN block. The whitespace
			//    between them is not translatable text, so there is nothing
			//    gluing the elements into a single unit. This makes the
			//    read-only DOM path agree with the authoritative tokenizer
			//    path (which emits one block per such standalone inline
			//    element) so word-count / segment-count / kgrep see the same
			//    translatable blocks that pseudo-translate localizes (#721).
			//
			// A bare comment-only run (idCounter > 0, no text, no inline kids)
			// keeps the combined-block path so the comment placeholder still
			// round-trips.
			switch {
			case hasDirectText || len(inlineKids) == 0:
				if text != "" || idCounter > 0 {
					w.visitor.onMixedContentBlock(w.nextBlockID(), n, runStart, child, preserveWS)
				}
			default:
				for _, inl := range inlineKids {
					if w.inlineHasTranslatableContent(inl) {
						w.visitor.onBlockElement(w.nextBlockID(), inl, preserveWS)
					}
				}
			}

			if child == nil {
				break
			}
		}

		w.walkNode(child, translateNo)
	}
}

// inlineHasTranslatableContent reports whether a standalone inline element
// holds content worth emitting as its own block. Unlike onBlockElement's emit
// decision for block-level elements (which also fires on a bare `id` anchor),
// a standalone inline element must carry actual text/inline-markup content
// (hasAnyContent). An empty inline element with only an `id` (e.g.
// `<span id="x"></span>`) is not translatable and is not emitted — matching
// the authoritative tokenizer path, which never emits an empty inline as a
// block.
func (w *domWalker) inlineHasTranslatableContent(n *html.Node) bool {
	return w.hasAnyContent(n)
}

// walkInlineChildren traverses inline content, counting spans and advancing
// attribute block counters. This mirrors collectFromNode's traversal order.
func (w *domWalker) walkInlineChildren(n *html.Node, idCounter *int, translateNo bool) {
	// Bound the inline-element recursion (e.g. deeply nested <b><b>…) on the
	// same shared depth guard as walkNode (see core/safeio).
	if w.depth.Enter() != nil {
		return
	}
	defer w.depth.Leave()

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.CommentNode:
			*idCounter++

		case html.ElementNode:
			w.extractTranslatableAttributes(child, translateNo)

			if nonTranslatableElements[child.DataAtom] {
				*idCounter++
				continue
			}

			childTranslateNo := translateNo
			if tv := getAttr(child, "translate"); tv != "" {
				if tv == "no" {
					childTranslateNo = true
				} else if tv == "yes" {
					childTranslateNo = false
				}
			}

			if isInlineElement(child) {
				if childTranslateNo && !translateNo && !hasDescendantTranslateYes(child) {
					*idCounter++
					continue
				}

				if selfClosingElements[child.DataAtom] {
					*idCounter++
				} else {
					*idCounter++ // opening
					w.walkInlineChildren(child, idCounter, childTranslateNo)
					*idCounter++ // closing
				}
			}
		}
	}
}

// hasBlockLevelChildren returns true if the node has any non-inline element children.
func (w *domWalker) hasBlockLevelChildren(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && !isInlineElement(child) && !nonTranslatableElements[child.DataAtom] {
			return true
		}
	}
	return false
}

// hasAnyContent returns true if the node contains any text or inline element content.
func (w *domWalker) hasAnyContent(n *html.Node) bool {
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
				if w.hasAnyContent(child) {
					return true
				}
			}
		}
	}
	return false
}
