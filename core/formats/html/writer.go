package html

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/net/html"
)

// Writer implements DataFormatWriter for HTML files.
type Writer struct {
	format.BaseFormatWriter
	sourcePath      string
	originalContent []byte
	skeletonStore   *format.SkeletonStore
	cfg             *Config
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// NewWriter creates a new HTML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "html",
		},
		cfg: &Config{},
	}
}

// SetSourcePath sets the path to the original document for re-parse mode.
func (w *Writer) SetSourcePath(path string) {
	w.sourcePath = path
}

// SetOriginalContent sets the original document bytes for re-parse mode.
func (w *Writer) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// Write consumes Parts from a channel and writes reconstructed HTML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all blocks keyed by ID (for the skeleton/reparse modes) plus an
	// ordered event stream of blocks and group brackets (for the semantic
	// block-only mode), and capture source locale.
	blocks := make(map[string]*model.Block)
	var events []*model.Part
	var sourceLocale model.LocaleID
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			switch part.Type {
			case model.PartBlock:
				if b, ok := part.Resource.(*model.Block); ok {
					blocks[b.ID] = b
				}
				events = append(events, part)
			case model.PartGroupStart, model.PartGroupEnd:
				events = append(events, part)
			case model.PartLayerStart:
				if l, ok := part.Resource.(*model.Layer); ok && !l.Locale.IsEmpty() {
					sourceLocale = l.Locale
				}
			}
		}
	}
done:

	// A lang rewrite is needed when a target locale is set and differs from
	// the document's declared source locale.
	needsLangRewrite := !w.Locale.IsEmpty() && !sourceLocale.IsEmpty() && w.Locale != sourceLocale

	// Mode 1: Skeleton store (optimal, byte-exact).
	//
	// lang/xml:lang attribute declarations are stored as typed SkeletonLang
	// entries carrying the source-locale value (#604). The writer retargets
	// them structurally in writeFromSkeleton — no post-serialization regex,
	// no output buffering.
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("html writer: flush skeleton: %w", err)
		}
		return w.writeFromSkeleton(w.skeletonStore, blocks, sourceLocale, needsLangRewrite)
	}

	// Modes 2 and 3 build a golang.org/x/net/html node tree (or render
	// blocks directly), so they rewrite lang/xml:lang structurally on the
	// DOM before html.Render — no post-serialization regex, no buffering.
	if content, err := w.loadOriginalContent(); err != nil {
		return err
	} else if content != nil {
		// Mode 2: Re-parse original content. Lang attributes are rewritten
		// structurally on the DOM (see writerVisitor.onData).
		return w.writeReparse(content, blocks, sourceLocale)
	}
	// Mode 3: Block-only output. Reconstructs HTML from the content model +
	// the structural layer (role-driven semantic export, WS6) — this is the
	// cross-format path (DocLang/Docling/DOCX → clean HTML). Same-format HTML
	// blocks carrying a fragment skeleton keep their captured surrounding
	// markup. No lang attributes are emitted here, so no rewrite is needed.
	return w.writeSemantic(events)
}

// loadOriginalContent returns original content bytes, or nil if unavailable.
func (w *Writer) loadOriginalContent() ([]byte, error) {
	if w.originalContent != nil {
		return w.originalContent, nil
	}
	if w.sourcePath != "" {
		data, err := os.ReadFile(w.sourcePath)
		if err != nil {
			return nil, fmt.Errorf("html writer: read source: %w", err)
		}
		return data, nil
	}
	return nil, nil
}

// writeFromSkeleton reads skeleton entries and fills in block content.
// This produces byte-exact output — only translated text (and, when
// retargeting, lang/xml:lang declarations) differ from the original.
//
// sourceLocale is the document's declared source locale; needsLangRewrite is
// true when the writer targets a different locale. SkeletonLang entries carry
// the original source-locale lang value: when retargeting and the stored
// value is the same LANGUAGE as the source locale (region/script-insensitive,
// mirroring writerVisitor.retargetLangAttr and Okapi's sameLanguageAs), the
// target locale is emitted; otherwise the stored value is emitted verbatim so
// unrelated declarations (e.g. lang="de" in an en→fr document) and the
// no-target case stay byte-exact.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block, sourceLocale model.LocaleID, needsLangRewrite bool) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("html writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.getBlockText(block)
				text = w.substituteBlockRefs(text, blocks)
				text = htmlEncodeBlockText(text, block)
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		case format.SkeletonLang:
			lang := string(entry.Data)
			if needsLangRewrite && sameLanguage(lang, sourceLocale.String()) {
				lang = w.Locale.String()
			}
			if _, err := io.WriteString(w.Output, lang); err != nil {
				return err
			}
		}
	}
	return nil
}

// htmlEncodeBlockText post-processes a rendered block's text the same
// way okapi's HtmlSkeletonWriter does before splicing it back into the
// output stream:
//
//   - ASCII double-quotes (`"`) outside `<…>` placeholder spans become
//     `&quot;`. This both matches okapi's text-content escaping (for
//     shortcode-style text like `[vc_column width="1/2"]`) and keeps
//     attribute values well-formed when block text lands inside
//     `attr="…"` via the skeleton.
//   - Inside `<script>…</script>`, `<style>…</style>`, and `<textarea>…
//     </textarea>` placeholder spans, leave `"` alone. These elements'
//     content is CDATA-like (script/style) or user input (textarea); per
//     HTML5 §13.2.5.1 the tokenizer treats `<script>` body as raw text,
//     and `&quot;` inside script source would be syntactically invalid
//     JavaScript. Okapi's HtmlSkeletonWriter (NekoHTML-backed) preserves
//     bare quotes in these contexts.
//   - For blocks that don't preserve whitespace (i.e. not <pre>/<textarea>
//     and not flagged via Config.PreserveWhitespace), runs of HTML
//     whitespace outside `<…>` placeholders collapse to a single space —
//     mirroring okapi's HTML5 whitespace normalisation on text-units that
//     mix text and inline tags.
func htmlEncodeBlockText(s string, block *model.Block) string {
	collapseWS := block != nil && !block.PreserveWhitespace && !attrBlockTypes[block.Type]
	if !collapseWS && !strings.ContainsAny(s, `"`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	depth := 0
	inWS := false
	// rawTextElement is non-empty while we're inside a raw-text element
	// span (`<script>`, `<style>`, `<textarea>`, `<title>`). HTML5
	// §13.2.5.1 (the "script data state" and similar) consume raw bytes
	// until the matching close tag, ignoring `<` for nesting.
	var rawTextElement string
	for i := 0; i < len(s); i++ {
		c := s[i]
		// While inside script/style/textarea/title content, emit bytes
		// verbatim and only check for the matching `</tag>` close.
		if rawTextElement != "" {
			if c == '<' && i+1 < len(s) && s[i+1] == '/' {
				closeTag := "</" + rawTextElement
				if strings.EqualFold(s[i:min(i+len(closeTag), len(s))], closeTag) {
					rawTextElement = ""
					// Fall through to normal `<` handling so the close
					// tag's bytes go through depth tracking.
				} else {
					b.WriteByte(c)
					continue
				}
			} else {
				b.WriteByte(c)
				continue
			}
		}
		switch {
		case c == '<':
			depth++
			inWS = false
			b.WriteByte(c)
			// Detect open tag for a raw-text element so subsequent
			// content (including `"`) passes through untouched.
			if name := scanTagName(s, i+1); name != "" {
				lname := strings.ToLower(name)
				if lname == "script" || lname == "style" || lname == "textarea" || lname == "title" {
					// Find the closing `>` of this open tag, then
					// switch to raw-text mode. Attribute values inside
					// the open tag still need their own quote handling
					// (depth>0 already protects them).
					if closeIdx := strings.IndexByte(s[i:], '>'); closeIdx >= 0 {
						// Copy through the close `>` (including any
						// attribute quotes which are at depth>0).
						end := i + closeIdx
						b.WriteString(s[i+1 : end+1])
						depth = 0
						i = end
						rawTextElement = lname
						inWS = false
						continue
					}
				}
			}
		case c == '>' && depth > 0:
			depth--
			inWS = false
			b.WriteByte(c)
		case depth == 0 && c == '"':
			b.WriteString("&quot;")
			inWS = false
		case depth == 0 && collapseWS && isHTMLWhitespace(rune(c)):
			if !inWS {
				b.WriteByte(' ')
				inWS = true
			}
		default:
			inWS = false
			b.WriteByte(c)
		}
	}
	return b.String()
}

// scanTagName reads an ASCII tag name starting at offset i in s. Returns
// the tag name if the byte at i is a letter, otherwise empty. Stops at
// any non-name character (whitespace, `>`, `/`, `=`, `"`, `'`).
func scanTagName(s string, i int) string {
	if i >= len(s) {
		return ""
	}
	c := s[i]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return ""
	}
	end := i
	for end < len(s) {
		c := s[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			end++
			continue
		}
		break
	}
	return s[i:end]
}

// substituteBlockRefs replaces every `\x00BLOCK:tuN\x00` sentinel in s
// with the named block's translated text. Reader-side
// rewriteInlineTagWithRefs embeds these sentinels into inline-element
// placeholder data so attribute values get substituted with translations
// at write time. Sentinels survive pseudo-translation because tools treat
// placeholder Data as opaque.
//
// rewriteInlineTagWithRefs always positions sentinels inside HTML
// attribute values (i.e. between the opening and closing `"` of an
// attribute), so the substituted text needs HTML-attribute-value
// encoding: any `"` in the translated text becomes `&#34;` (matching
// okapi's HtmlEncoder NUMERIC_SINGLE_QUOTES default), and bare `&`
// not introducing an existing entity becomes `&amp;`.
func (w *Writer) substituteBlockRefs(s string, blocks map[string]*model.Block) string {
	const sentinel = "\x00BLOCK:"
	if !strings.Contains(s, sentinel) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for {
		i := strings.Index(s, sentinel)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		rest := s[i+len(sentinel):]
		before, after, ok := strings.Cut(rest, "\x00")
		if !ok {
			// Malformed: keep the rest as-is.
			b.WriteString(s[i:])
			return b.String()
		}
		blockID := before
		if blk, ok := blocks[blockID]; ok {
			b.WriteString(encodeForAttributeValue(w.getBlockText(blk)))
		}
		s = after
	}
}

// encodeForAttributeValue escapes a substituted-into-attribute-value
// string so it remains parseable inside `attr="…"`. Only `"` is escaped
// (not `<`, `>`, or `&`) — okapi's HtmlEncoder default NUMERIC_SINGLE_QUOTES
// quote mode emits `&#34;` for embedded double-quotes in attribute
// values and leaves the rest intact, matching how the source is read.
func encodeForAttributeValue(s string) string {
	if !strings.ContainsAny(s, `"`) {
		return s
	}
	return strings.ReplaceAll(s, `"`, "&#34;")
}

// writeReparse re-parses the original HTML, patches translations, and renders.
//
// sourceLocale is the document's declared source locale (may be empty). When
// the writer targets a different locale, lang/xml:lang attributes carrying the
// source locale are rewritten to the target locale structurally on the DOM
// (see writerVisitor.onData) before html.Render — no post-serialization regex.
func (w *Writer) writeReparse(content []byte, blocks map[string]*model.Block, sourceLocale model.LocaleID) error {
	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return fmt.Errorf("html writer: parse original: %w", err)
	}

	visitor := &writerVisitor{writer: w, blocks: blocks, sourceLocale: sourceLocale}
	walker := newDOMWalker(w.cfg, visitor)
	walker.walk(doc)

	return html.Render(w.Output, doc)
}

// writerVisitor implements walkVisitor for the writer, patching DOM nodes
// with translated content.
type writerVisitor struct {
	writer       *Writer
	blocks       map[string]*model.Block
	sourceLocale model.LocaleID
}

func (v *writerVisitor) onData(dataID string, n *html.Node, dataName string, props map[string]string) {
	// lang/xml:lang declarations surface here with props["language"] set
	// (see domWalker.extractLangAttribute). When retargeting to a different
	// locale, rewrite the declaration structurally on the node so the
	// rendered document reports the output language — mirroring okapi's
	// behaviour without touching serialized bytes.
	if props != nil && props["language"] != "" {
		v.retargetLangAttr(n)
	}
	// Other structural elements (doctype, comment, script/style, meta) are
	// preserved as-is in the DOM.
}

// retargetLangAttr rewrites lang/xml:lang attribute values on n from the
// source locale to the writer's target locale. As in the skeleton path's
// SkeletonLang handling, only attributes whose value is the same LANGUAGE as
// the source locale are rewritten, so unrelated declarations (e.g. lang="de"
// in an en→fr document) are preserved.
func (v *writerVisitor) retargetLangAttr(n *html.Node) {
	tgt := v.writer.Locale
	src := v.sourceLocale
	if tgt.IsEmpty() || src.IsEmpty() || tgt == src {
		return
	}
	for i, attr := range n.Attr {
		if attr.Key != "lang" && attr.Key != "xml:lang" {
			continue
		}
		if sameLanguage(attr.Val, src.String()) {
			n.Attr[i].Val = tgt.String()
		}
	}
}

// sameLanguage reports whether two BCP-47 tags share the same primary
// language subtag, ignoring region/script and case. It mirrors Okapi's
// LocaleId.sameLanguageAs, which GenericSkeletonWriter uses to decide whether
// a Property.LANGUAGE value is the document's own (input) language and should
// be retargeted to the output locale. A document-language declaration of
// "en-US" in an en→fr roundtrip is therefore retargeted, while a foreign-
// language inline declaration ("ja") is left untouched.
func sameLanguage(a, b string) bool {
	return strings.EqualFold(primaryLanguageSubtag(a), primaryLanguageSubtag(b))
}

// primaryLanguageSubtag returns the primary language subtag of a BCP-47 tag —
// the portion before the first '-' or '_' separator (BCP-47 §2.2.1).
func primaryLanguageSubtag(tag string) string {
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		return tag[:i]
	}
	return tag
}

func (v *writerVisitor) onTextBlock(blockID string, n *html.Node) {
	if block, ok := v.blocks[blockID]; ok {
		n.Data = v.writer.getBlockText(block)
	}
}

func (v *writerVisitor) onAttributeBlock(blockID string, n *html.Node, attrKey string) {
	if block, ok := v.blocks[blockID]; ok {
		setAttr(n, attrKey, v.writer.getBlockText(block))
	}
}

func (v *writerVisitor) onMetaBlock(blockID string, n *html.Node) {
	if block, ok := v.blocks[blockID]; ok {
		setAttr(n, "content", v.writer.getBlockText(block))
	}
}

func (v *writerVisitor) onBlockElement(blockID string, n *html.Node, preserveWS bool) {
	if block, ok := v.blocks[blockID]; ok {
		v.replaceElementContent(n, block)
	}
}

func (v *writerVisitor) onMixedContentBlock(blockID string, parent *html.Node, runStart, runEnd *html.Node, preserveWS bool) {
	if block, ok := v.blocks[blockID]; ok {
		v.replaceInlineRun(parent, runStart, runEnd, block)
	}
}

// replaceElementContent replaces a block element's children with translated content.
func (v *writerVisitor) replaceElementContent(n *html.Node, block *model.Block) {
	text := v.writer.getBlockText(block)

	for n.FirstChild != nil {
		n.RemoveChild(n.FirstChild)
	}

	nodes, err := html.ParseFragment(strings.NewReader(text), n)
	if err != nil {
		n.AppendChild(&html.Node{Type: html.TextNode, Data: text})
		return
	}
	for _, child := range nodes {
		n.AppendChild(child)
	}
}

// replaceInlineRun replaces a run of inline nodes with translated content.
func (v *writerVisitor) replaceInlineRun(parent *html.Node, runStart, runEnd *html.Node, block *model.Block) {
	text := v.writer.getBlockText(block)

	for runStart != nil && runStart != runEnd {
		next := runStart.NextSibling
		parent.RemoveChild(runStart)
		runStart = next
	}

	nodes, err := html.ParseFragment(strings.NewReader(text), parent)
	if err != nil {
		node := &html.Node{Type: html.TextNode, Data: text}
		parent.InsertBefore(node, runEnd)
		return
	}
	for _, child := range nodes {
		parent.InsertBefore(child, runEnd)
	}
}

// attrBlockTypes are the block.Type values that originate from HTML
// attribute extraction (META content, title=, alt=, …). Okapi normalises
// HTML attribute values by collapsing runs of HTML whitespace to a single
// space; mirror that on the translated render so output bytes match. The
// untranslated path keeps the original raw bytes via the skeleton, so the
// no-translation round-trip stays byte-exact (test attr_double_space).
var attrBlockTypes = map[string]bool{
	"content":     true,
	"title":       true,
	"alt":         true,
	"label":       true,
	"placeholder": true,
	"value":       true,
}

// getBlockText returns the text content to write for a block.
func (w *Writer) getBlockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text := w.renderTargetRuns(block, w.Locale)
		if attrBlockTypes[block.Type] {
			text = collapseWhitespace(text)
		}
		return text
	}
	return w.renderSourceRuns(block)
}

// renderTargetRuns reconstructs the full text from a block's target
// runs, splicing inline-code Data back into the output.
func (w *Writer) renderTargetRuns(block *model.Block, locale model.LocaleID) string {
	runs := block.TargetRuns(locale)
	if len(runs) == 0 {
		return w.renderSourceRuns(block)
	}
	return model.RenderRunsWithData(runs)
}

func (w *Writer) renderSourceRuns(block *model.Block) string {
	return model.RenderRunsWithData(block.Source)
}

// setAttr sets an attribute value on an HTML node, adding it if not present.
func setAttr(n *html.Node, key, val string) {
	for i, attr := range n.Attr {
		if attr.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

// collectPlainText collects plain text from a node's children (for the walker's
// block-emission check), without building spans.
func collectPlainText(n *html.Node, preserveWS bool) string {
	var buf strings.Builder
	collectPlainTextRecur(n, &buf)
	text := buf.String()
	if !preserveWS {
		text = collapseWhitespace(text)
		text = strings.TrimFunc(text, isHTMLWhitespace)
	}
	return text
}

func collectPlainTextRecur(n *html.Node, buf *strings.Builder) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			buf.WriteString(child.Data)
		} else if child.Type == html.ElementNode && isInlineElement(child) {
			collectPlainTextRecur(child, buf)
		}
	}
}
