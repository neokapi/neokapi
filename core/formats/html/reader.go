package html

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
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
	// Obsolete presentational elements that HTML5 §13.2.6.4.7 still
	// classifies as block (so they implicitly close an open <p>) but
	// that Okapi's HtmlFilter treats as raw skeleton bytes — neither
	// wellformedConfiguration.yml nor nonwellformedConfiguration.yml
	// declare them as TEXTUNIT or GROUP, so they fall through as
	// RULE_NOT_FOUND and pass through verbatim. Treating them as
	// inline here mirrors that behaviour: their text content stays
	// inside the surrounding TEXTUNIT instead of being misclassified
	// as a separate bare-text block (which would lose trailing
	// whitespace via trimTrailingWSOfLastTextBlock when the next
	// inline tag follows). The implicit-P-close rule is preserved
	// via pImplicitClosers in tokenreader.go.
	atom.Center: true, atom.Dir: true,
}

// selfClosingElements are void HTML elements.
var selfClosingElements = map[atom.Atom]bool{
	atom.Area: true, atom.Base: true, atom.Br: true, atom.Col: true,
	atom.Embed: true, atom.Hr: true, atom.Img: true, atom.Input: true,
	atom.Link: true, atom.Meta: true, atom.Param: true, atom.Source: true,
	atom.Track: true, atom.Wbr: true,
}

// nonTranslatableElements contain content that is not translatable.
//
// `<noscript>` is included because golang.org/x/net/html tokenises its
// content as a single raw-text TextToken (per the HTML5 "scripting
// enabled" mode default), so the inner markup arrives as bytes rather
// than parsed tokens. Without this flag, pseudo-translation would
// substitute the tag-name and attribute-name letters character-by-
// character (e.g. `<img src="…">` → `<ĩmĝ śŕć="…">`), wrecking the
// noscript fallback. okapi's NekoHTML parses noscript as HTML in
// scripting-disabled mode, which would let us extract the inner `alt`
// as a translatable block — a richer behaviour but one that requires
// sub-parsing the raw text. Treating noscript as opaque mirrors the
// safer default; the trade-off is intentional (parity contract:
// "same semantic config → same results", #557).
var nonTranslatableElements = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true,
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
	"a":    "link:hyperlink",
	"code": "fmt:code", "kbd": "fmt:code", "samp": "fmt:code", "tt": "fmt:code",
	"sub": "fmt:subscript", "sup": "fmt:superscript",
	"mark": "fmt:highlight",
	"br":   "struct:break", "hr": "struct:break",
	"img":    "media:image",
	"button": "ui:button",
}

// Reader implements DataFormatReader for HTML files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	vocab         *model.VocabularyRegistry
	skeletonStore *format.SkeletonStore
}

// SetSkeletonStore sets the skeleton store for tokenizer-based streaming.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
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
		return errors.New("html: nil document or reader")
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

	if r.skeletonStore != nil {
		// Tokenizer path: streaming, no DOM.
		state := newTokenReaderState(r, r.skeletonStore)
		state.run(content, ctx, ch)
	} else {
		// DOM path: existing behavior.
		doc, err := html.Parse(strings.NewReader(string(content)))
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("html: parsing: %w", err)}
			return
		}

		visitor := &readerVisitor{reader: r, ctx: ctx, ch: ch}
		walker := newDOMWalker(r.cfg, visitor)
		walker.walk(doc)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// readerVisitor implements walkVisitor for the reader, building model objects
// and emitting Parts to the channel.
type readerVisitor struct {
	reader *Reader
	ctx    context.Context
	ch     chan<- model.PartResult
}

func (v *readerVisitor) onData(dataID string, n *html.Node, dataName string, props map[string]string) {
	data := &model.Data{
		ID:         dataID,
		Name:       dataName,
		Properties: props,
	}
	v.reader.emit(v.ctx, v.ch, &model.Part{Type: model.PartData, Resource: data})
}

func (v *readerVisitor) onTextBlock(blockID string, n *html.Node) {
	text := n.Data
	if !v.reader.cfg.PreserveWhitespace {
		text = collapseWhitespace(text)
		text = strings.TrimFunc(text, isHTMLWhitespace)
	}
	block := model.NewBlock(blockID, text)
	v.reader.emit(v.ctx, v.ch, &model.Part{Type: model.PartBlock, Resource: block})
}

func (v *readerVisitor) onAttributeBlock(blockID string, n *html.Node, attrKey string) {
	value := getAttr(n, attrKey)
	block := &model.Block{
		ID:           blockID,
		Type:         attrKey,
		Translatable: true,
		IsReferent:   true,
		Source:       []model.Run{{Text: &model.TextRun{Text: value}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
	}
	v.reader.emit(v.ctx, v.ch, &model.Part{Type: model.PartBlock, Resource: block})
}

func (v *readerVisitor) onMetaBlock(blockID string, n *html.Node) {
	metaName := strings.ToLower(getAttr(n, "name"))
	content := getAttr(n, "content")
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
	v.reader.emit(v.ctx, v.ch, &model.Part{Type: model.PartBlock, Resource: block})
}

func (v *readerVisitor) onBlockElement(blockID string, n *html.Node, preserveWS bool) {
	// Collect inline content into a Runs builder. This is a second pass
	// over the inline children — the walker already advanced all counters.
	runs := v.collectInlineContent(n, preserveWS)

	hasID := getAttr(n, "id") != ""
	if runs == nil && !hasID {
		return
	}
	if runs == nil {
		runs = []model.Run{}
	}

	block := &model.Block{
		ID:                 blockID,
		Name:               v.reader.blockName(n),
		Type:               v.reader.blockType(n),
		Translatable:       true,
		PreserveWhitespace: preserveWS,
		Source:             runs,
		Targets:            make(map[model.VariantKey]*model.Target),
		Properties:         v.reader.extractBlockProperties(n),
		Skeleton: &model.Skeleton{
			Strategy: model.SkeletonFragmentBased,
			Parts: []model.SkeletonPart{
				&model.SkeletonText{Text: v.reader.renderOpenTag(n)},
				&model.SkeletonRef{ResourceID: blockID, Property: "target"},
				&model.SkeletonText{Text: fmt.Sprintf("</%s>", n.Data)},
			},
		},
	}
	v.reader.emit(v.ctx, v.ch, &model.Part{Type: model.PartBlock, Resource: block})
}

func (v *readerVisitor) onMixedContentBlock(blockID string, parent *html.Node, runStart, runEnd *html.Node, preserveWS bool) {
	// Collect inline content from the run into a Runs slice.
	runs := v.collectMixedRunContent(parent, runStart, runEnd, preserveWS)
	if runs == nil {
		return
	}

	block := &model.Block{
		ID:                 blockID,
		Name:               v.reader.blockName(parent),
		Type:               v.reader.blockType(parent),
		Translatable:       true,
		PreserveWhitespace: preserveWS,
		Source:             runs,
		Targets:            make(map[model.VariantKey]*model.Target),
		Properties:         v.reader.extractBlockProperties(parent),
	}
	v.reader.emit(v.ctx, v.ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// collectInlineContent builds a Runs slice from a block element's inline
// children. This is a pure content-building pass — no counter advancement.
// Returns nil when the collected content is empty.
func (v *readerVisitor) collectInlineContent(n *html.Node, preserveWS bool) []model.Run {
	b := newRunBuilder()
	spanCounter := 0
	v.collectFromNode(n, b, &spanCounter, preserveWS, false)

	runs := b.Runs()
	if !preserveWS {
		runs = collapseWhitespaceRuns(runs)
		runs = trimWhitespaceRuns(runs)
	}

	if len(runs) == 0 {
		return nil
	}
	return runs
}

// collectMixedRunContent builds a Runs slice from a run of inline nodes.
func (v *readerVisitor) collectMixedRunContent(parent *html.Node, runStart, runEnd *html.Node, preserveWS bool) []model.Run {
	b := newRunBuilder()
	spanCounter := 0

	for child := runStart; child != nil && child != runEnd; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			b.AddText(child.Data)
		case html.CommentNode:
			spanCounter++
			b.AddPh(
				strconv.Itoa(spanCounter),
				"code:comment",
				"html:comment",
				"<!--"+child.Data+"-->",
				"", "", model.RunConstraints{},
			)
		case html.ElementNode:
			// Skip extractTranslatableAttributes — walker already handled it.
			v.collectFromNode(child, b, &spanCounter, preserveWS, false)
		}
	}

	runs := b.Runs()
	if !preserveWS {
		runs = collapseWhitespaceRuns(runs)
		runs = trimWhitespaceRuns(runs)
	}

	// Match legacy "text empty AND no spans" early-out (avoid emitting a
	// block that would serialize to nothing translatable).
	hasText := false
	hasNonText := false
	for _, r := range runs {
		if r.Text != nil {
			if r.Text.Text != "" {
				hasText = true
			}
		} else {
			hasNonText = true
		}
	}
	if !hasText && !hasNonText {
		return nil
	}
	return runs
}

// collectFromNode builds Run content from inline children.
func (v *readerVisitor) collectFromNode(n *html.Node, b *runBuilder, spanCounter *int, preserveWS bool, translateNo bool) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			b.AddText(child.Data)

		case html.CommentNode:
			*spanCounter++
			id := strconv.Itoa(*spanCounter)
			b.AddPh(
				id,
				"code:comment",
				"html:comment",
				"<!--"+child.Data+"-->",
				"", "", model.RunConstraints{},
			)

		case html.ElementNode:
			// Note: translatable attributes on inline children are already
			// handled by the walker. We only build fragment content here.

			if nonTranslatableElements[child.DataAtom] {
				*spanCounter++
				id := strconv.Itoa(*spanCounter)
				b.AddPh(
					id,
					"code:markup",
					"html:"+child.Data,
					renderNodeHTML(child),
					"", "", model.RunConstraints{},
				)
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
					*spanCounter++
					id := strconv.Itoa(*spanCounter)
					b.AddPh(
						id,
						"code:markup",
						"html:"+child.Data,
						renderNodeHTML(child),
						"", "", model.RunConstraints{},
					)
					continue
				}

				semType := htmlSemanticType(child.Data)
				subType := "html:" + child.Data

				if selfClosingElements[child.DataAtom] {
					*spanCounter++
					id := strconv.Itoa(*spanCounter)
					info := v.reader.vocab.LookupOrFallback(semType)
					b.AddPh(
						id,
						semType,
						subType,
						v.reader.renderTag(child),
						info.Display.Placeholder,
						info.Equiv,
						model.RunConstraints{
							Deletable:   info.Constraints.Deletable,
							Cloneable:   info.Constraints.Cloneable,
							Reorderable: info.Constraints.Reorderable,
						},
					)
				} else {
					*spanCounter++
					id := strconv.Itoa(*spanCounter)
					info := v.reader.vocab.LookupOrFallback(semType)
					b.AddPcOpen(
						id,
						semType,
						subType,
						v.reader.renderOpenTag(child),
						info.Display.Open,
						info.Equiv,
						model.RunConstraints{
							Deletable:   info.Constraints.Deletable,
							Cloneable:   info.Constraints.Cloneable,
							Reorderable: info.Constraints.Reorderable,
						},
					)
					v.collectFromNode(child, b, spanCounter, preserveWS, childTranslateNo)
					b.AddPcClose(
						id,
						semType,
						subType,
						fmt.Sprintf("</%s>", child.Data),
						info.Equiv,
					)
				}
			}
		}
	}
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

// isHTMLWhitespace returns true for HTML whitespace characters (space, tab, newline, carriage return, form feed).
// Unlike unicode.IsSpace, this does NOT include non-breaking space (\u00A0).
func isHTMLWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
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
	_ = html.Render(&buf, n)
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
	for part := range strings.SplitSeq(contentType, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "charset=") {
			return strings.TrimSpace(part[8:])
		}
	}
	return ""
}

// htmlSemanticType returns the vocabulary semantic type for an HTML element name.
func htmlSemanticType(element string) string {
	if t, ok := htmlSemanticTypes[strings.ToLower(element)]; ok {
		return t
	}
	return "code:markup"
}

// isTranslatableInputValue returns true if the input type has a translatable value attribute.
//
// Mirrors the okapi okf_html `nonwellformedConfiguration.yml` rule for the
// `input` element:
//
//	value: [type, NOT_EQUALS, [file, hidden, image, Password]]
//
// (HtmlFilter.java extends AbstractMarkupFilter; the YAML rule above is the
// authoritative source. See
// okapi/filters/html/src/main/resources/net/sf/okapi/filters/html/nonwellformedConfiguration.yml:260-267
// for the full input-element rule block.)
//
// `radio` and `checkbox` were previously excluded here, but okapi extracts
// their `value` attributes as translatable — typically used for user-visible
// labels. The exclusion list matches okapi byte-for-byte: file, hidden,
// image, password. The okapi YAML uses the literal "Password" but
// okf_html lowercases the type before matching, so the comparison is
// effectively case-insensitive (callers already lowercase inputType).
func isTranslatableInputValue(inputType string) bool {
	switch inputType {
	case "file", "hidden", "image", "password":
		return false
	default:
		return true
	}
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
