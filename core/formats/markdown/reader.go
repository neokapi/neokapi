package markdown

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	xhtml "golang.org/x/net/html"
)

// htmlEntityRE matches a single HTML entity reference: a named entity
// (`&amp;`), a numeric entity (`&#160;`), or a hex entity (`&#xA0;`).
// Used by addTextWithEntities to detect entity references in plain text
// for decoding (mirroring okapi MarkdownFilter line 379, which routes
// TEXT tokens through DecodeUtil.fromPlainTextHTML before adding them
// to the text unit).
var htmlEntityRE = regexp.MustCompile(`&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]+|#[xX][0-9A-Fa-f]+);`)

// addTextWithEntities appends plain text to a runBuilder, decoding any
// HTML entity references inline so the text unit holds the resolved
// character (e.g. `&#39;` → `'`, `&amp;` → `&`). Mirrors okapi
// MarkdownFilter.handleAtomTextUnitToken's
// `DecodeUtil.fromPlainTextHTML(token.getContent())` call
// (MarkdownFilter.java line 379) and the HTML subfilter's
// AbstractMarkupFilter.handleNumericEntity / handleCharacterEntity
// behaviour, which decode entities by default unless
// `preserve_character_entities` is set on the filter config (the
// markdown HTML subfilter config leaves it unset).
//
// On round-trip the decoded character flows through the markdown
// writer's MarkdownEncoder (which does not re-escape) so the entity
// reference is dropped — exactly matching okapi's reference output. The
// `idCounter` parameter is retained for callers that previously relied
// on placeholder ids being bumped per entity; it is now a no-op for
// pure-text input but kept for API stability.
func addTextWithEntities(b *runBuilder, text string, idCounter *int) {
	if text == "" {
		return
	}
	if !strings.Contains(text, "&") || !htmlEntityRE.MatchString(text) {
		b.AddText(text)
		return
	}
	decoded := htmlEntityRE.ReplaceAllStringFunc(text, func(match string) string {
		// xhtml.UnescapeString covers all HTML5 named entities plus
		// decimal/hex numeric character references. If the lookup fails
		// (unknown name) it returns the input unchanged.
		return xhtml.UnescapeString(match)
	})
	b.AddText(decoded)
	_ = idCounter
}

// BlockPropLinePrefix is the per-block property holding the per-line
// continuation prefix for multi-line paragraphs (e.g. `> ` for
// blockquote bodies, indentation for list-item continuations). The
// writer re-emits this prefix after every "\n" in the rendered block
// text so the round-tripped output preserves the original line shape.
// Unset when the paragraph is single-line or its continuation lines
// have inconsistent prefixes.
const BlockPropLinePrefix = "md:line-prefix"

// BlockPropFrontMatterQuote is the per-block property recording the quote
// character ('"' or "'") that wrapped a front matter value in the source.
// The skeleton drops the quotes (the block text is the unquoted value);
// the writer re-applies them — and adds quoting when an unquoted value's
// translation needs it to stay a valid YAML scalar.
const BlockPropFrontMatterQuote = "md:fm-quote"

// Reader implements DataFormatReader for Markdown files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	vocab         *model.VocabularyRegistry
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalescing buffer for skeleton text
	skelCursor    int          // current position in source for skeleton tracking

	source       []byte
	blockCounter int
	dataCounter  int

	// nonTranslatableDepth, when >0, marks every Block emitted as
	// Translatable:false. It is raised while walking a sub-tree whose
	// content is contextual rather than translatable (e.g. a blockquote
	// surfaced as non-translatable content under
	// ExtractNonTranslatableContent). A counter (not a bool) so nested
	// regions restore the outer state correctly on exit.
	nonTranslatableDepth int

	// visibleRefs is the case-sensitive set of reference labels that
	// appear as the displayed text of a shortcut or collapsed LinkRef
	// (`[text]` or `[text][]`). When the matching link reference
	// definition is encountered, its label is extracted as a translatable
	// Block — okapi MarkdownFilter does this so the rendered shortcut and
	// its definition stay in sync. Full LinkRefs (`[anchor][label]`) do
	// NOT make their label visible: the anchor is what's translated, the
	// `[label]` portion stays verbatim.
	visibleRefs map[string]bool

	// usedRefs is the lowercased set of labels referenced by any LinkRef
	// or ImageRef. The link-reference definition's title is extracted as
	// translatable only when its label appears in this set — okapi's
	// behaviour for unused (dead) reference definitions is to treat the
	// title as untranslatable skeleton bytes.
	usedRefs map[string]bool
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new Markdown reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	vocab := model.NewVocabularyRegistry()
	_ = vocab.LoadDefaults()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "markdown",
			FormatDisplayName: "Markdown",
			FormatMimeType:    "text/markdown",
			FormatExtensions:  []string{".md", ".markdown"},
			Cfg:               cfg,
		},
		cfg:   cfg,
		vocab: vocab,
	}
}

// MarkdownConfig returns the reader's markdown-specific config for customization.
func (r *Reader) MarkdownConfig() *Config {
	return r.cfg
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/markdown", "text/x-markdown"},
		Extensions: []string{".md", ".markdown"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("markdown: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		if err := r.readContent(ctx, ch); err != nil {
			ch <- model.PartResult{Error: err}
		}
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) error {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "markdown",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/markdown",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return nil
	}

	// Bound the whole-input read with the shared safeio byte budget so an
	// unbounded/oversized stream fails with a typed error (identical limit
	// across CLI/server/WASM — see core/safeio).
	content, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		return fmt.Errorf("markdown: reading: %w", err)
	}
	r.source = content
	r.skelCursor = 0

	r.blockCounter = 0
	r.dataCounter = 0

	// Handle YAML front matter.
	bodyOffset := r.handleFrontMatter(ctx, ch, content)

	// Parse the markdown body with GFM extensions (tables, strikethrough).
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
	)
	body := content[bodyOffset:]
	doc := md.Parser().Parse(text.NewReader(body))

	r.scanReferenceVisibility(doc)
	r.walkNode(ctx, ch, doc, body, bodyOffset)

	// Flush remaining source bytes as skeleton text.
	if r.skeletonStore != nil && r.skelCursor < len(content) {
		r.skelText(string(content[r.skelCursor:]))
	}
	r.skelFlush()
	if r.skeletonStore != nil {
		if err := r.skeletonStore.Flush(); err != nil {
			return err
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
	return nil
}

// handleFrontMatter detects and processes YAML front matter (--- delimited).
// Returns the byte offset where the markdown body starts.
func (r *Reader) handleFrontMatter(ctx context.Context, ch chan<- model.PartResult, content []byte) int {
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return 0
	}

	// Find closing ---
	var searchStart int
	if len(content) > 4 && content[3] == '\r' {
		searchStart = 5
	} else {
		searchStart = 4
	}
	closingIdx := -1
	for i := searchStart; i < len(content); i++ {
		if content[i] == '-' && i+3 <= len(content) && string(content[i:i+3]) == "---" {
			if i == 0 || content[i-1] == '\n' {
				endIdx := i + 3
				if endIdx >= len(content) || content[endIdx] == '\n' || content[endIdx] == '\r' {
					closingIdx = i
					break
				}
			}
		}
	}
	if closingIdx < 0 {
		return 0
	}

	endOfFrontMatter := closingIdx + 3
	if endOfFrontMatter < len(content) && content[endOfFrontMatter] == '\r' {
		endOfFrontMatter++
	}
	if endOfFrontMatter < len(content) && content[endOfFrontMatter] == '\n' {
		endOfFrontMatter++
	}

	frontMatterRaw := string(content[:endOfFrontMatter])

	if r.cfg.TranslateFrontMatter {
		yamlContent := string(content[searchStart:closingIdx])
		r.skelText(string(content[:searchStart]))
		r.emitFrontMatterBlocks(ctx, ch, yamlContent)
		endMarker := string(content[closingIdx:endOfFrontMatter])
		r.skelText(endMarker)
	} else if r.cfg.ExtractNonTranslatableContent() {
		// Surface the known prose scalars (title/description/summary) as
		// non-translatable content; keys, dates, slugs, and the `---`
		// fences stay skeleton. Byte-identical round-trip to the Data path.
		yamlContent := string(content[searchStart:closingIdx])
		r.skelText(string(content[:searchStart]))
		r.emitFrontMatterContentBlocks(ctx, ch, yamlContent)
		endMarker := string(content[closingIdx:endOfFrontMatter])
		r.skelText(endMarker)
	} else {
		r.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", r.dataCounter),
			Name: "front-matter",
			Properties: map[string]string{
				"content": frontMatterRaw,
			},
		}
		r.skelText(frontMatterRaw)
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}

	r.skelCursor = endOfFrontMatter
	return endOfFrontMatter
}

// frontMatterProseKeys are the front-matter scalar keys whose values are
// human prose worth surfacing as non-translatable content (visible to
// ingestion, skipped by MT) when TranslateFrontMatter is off but
// ExtractNonTranslatableContent is on. Everything else — dates, slugs,
// numbers, tag lists, identifiers — stays opaque skeleton.
var frontMatterProseKeys = map[string]bool{
	"title":       true,
	"description": true,
	"summary":     true,
}

// emitFrontMatterBlocks emits each YAML value as a translatable block.
func (r *Reader) emitFrontMatterBlocks(ctx context.Context, ch chan<- model.PartResult, yaml string) {
	// Optional key allowlist: when FrontMatterKeys is set, only those keys
	// become blocks; everything else stays skeleton.
	allow := map[string]bool{}
	for _, k := range r.cfg.FrontMatterKeys {
		allow[k] = true
	}
	r.emitFrontMatterScalars(ctx, ch, yaml, allow, true)
}

// emitFrontMatterContentBlocks surfaces the known prose front-matter
// scalars (title/description/summary) as non-translatable content blocks,
// keeping every other key — and the `key:` markers themselves — as
// skeleton. The block stream/skeleton layout matches the translatable
// path exactly (the writer restores YAML quoting from block.Type), so the
// round-trip stays byte-exact whether the value is translated or not.
func (r *Reader) emitFrontMatterContentBlocks(ctx context.Context, ch chan<- model.PartResult, yaml string) {
	r.emitFrontMatterScalars(ctx, ch, yaml, frontMatterProseKeys, false)
}

// emitFrontMatterScalars splits YAML front matter into per-key skeleton +
// blocks. A key becomes a Block (with the given translatable flag) when it
// is allowed (empty allow = every scalar, the historical translate-all
// behavior); otherwise the line stays skeleton. The `key:` prefix and any
// leading value whitespace always ride the skeleton so only the scalar
// value travels in the block.
func (r *Reader) emitFrontMatterScalars(ctx context.Context, ch chan<- model.PartResult, yaml string, allow map[string]bool, translatable bool) {
	// strings.Split (not SplitSeq): the trailing empty element after the
	// final newline must be dropped or the skeleton gains a blank line.
	lines := strings.Split(yaml, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	for _, line := range lines {
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			r.skelText(line + "\n")
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		if value == "" || key == "" || (len(allow) > 0 && !allow[key]) {
			r.skelText(line + "\n")
			continue
		}

		unquoted := value
		quote := ""
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			quote = value[:1]
			unquoted = value[1 : len(value)-1]
		}

		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, unquoted)
		block.Name = "fm_" + key
		block.Type = "front-matter"
		block.Translatable = translatable
		block.Properties["key"] = key
		if quote != "" {
			block.Properties[BlockPropFrontMatterQuote] = quote
		}

		prefix := line[:colonIdx+1]
		valuePart := line[colonIdx+1:]
		leadingSpace := ""
		var leadingSpaceSb245 strings.Builder
		for _, c := range valuePart {
			if c == ' ' || c == '\t' {
				leadingSpaceSb245.WriteString(string(c))
			} else {
				break
			}
		}
		leadingSpace += leadingSpaceSb245.String()
		r.skelText(prefix + leadingSpace)
		r.skelRef(blockID)
		r.skelText("\n")

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}
}

// blockRange returns the full byte range of a block node including its prefix
// markers (like "# " for headings, "- " for list items) and trailing newlines.
// For nodes with Lines(), this is the range from the line start to line end.
// The returned range is relative to source (body), not absolute.
func blockRange(node ast.Node, source []byte) (int, int) {
	lines := node.Lines()
	if lines.Len() > 0 {
		first := lines.At(0)
		last := lines.At(lines.Len() - 1)
		return first.Start, last.Stop
	}
	// For container nodes (List, ListItem, Blockquote), compute from children.
	start := len(source)
	end := 0
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		cs, ce := blockRange(child, source)
		if cs < start {
			start = cs
		}
		if ce > end {
			end = ce
		}
	}
	if start >= end {
		return 0, 0
	}
	return start, end
}

func (r *Reader) walkNode(ctx context.Context, ch chan<- model.PartResult, node ast.Node, source []byte, baseOffset int) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Heading:
			r.emitHeading(ctx, ch, n, source, baseOffset)

		case *ast.Paragraph:
			r.emitParagraph(ctx, ch, n, source, baseOffset)

		case *ast.ListItem:
			r.emitListItem(ctx, ch, n, source, baseOffset)

		case *ast.FencedCodeBlock:
			r.emitFencedCodeBlock(ctx, ch, n, source, baseOffset)

		case *ast.CodeBlock:
			r.emitIndentedCodeBlock(ctx, ch, n, source, baseOffset)

		case *ast.HTMLBlock:
			r.emitHTMLBlock(ctx, ch, n, source, baseOffset)

		case *ast.ThematicBreak:
			r.emitThematicBreak(ctx, ch, n, source, baseOffset)

		case *ast.List:
			r.walkNode(ctx, ch, child, source, baseOffset)

		case *ast.Blockquote:
			if r.cfg.TranslateBlockQuotes() {
				r.walkNode(ctx, ch, child, source, baseOffset)
			} else if r.cfg.ExtractNonTranslatableContent() {
				r.emitBlockquoteAsContent(ctx, ch, child, source, baseOffset)
			} else {
				r.emitBlockquoteAsData(ctx, ch, n, source, baseOffset)
			}

		case *ast.LinkReferenceDefinition:
			r.emitLinkReferenceDefinition(ctx, ch, n, source, baseOffset)

		default:
			if child.Kind() == east.KindTable {
				r.emitTable(ctx, ch, child, source, baseOffset)
			} else {
				r.walkNode(ctx, ch, child, source, baseOffset)
			}
		}
	}
}

// scanReferenceVisibility populates r.visibleRefs and r.usedRefs by
// pre-walking every Link, Image, LinkRef and ImageRef node in the
// document. Mirrors the okapi MarkdownParser.preVisitor pass:
//
//   - A reference label is "visible" (and therefore translatable in the
//     matching reference definition) iff at least one shortcut or
//     collapsed LinkRef uses it. Full LinkRefs (`[anchor][label]`) keep
//     the anchor translatable but leave the bracketed label verbatim,
//     so they do NOT mark the label visible.
//   - A reference label is "used" iff any LinkRef or ImageRef points at
//     it, case-folded for matching. Definitions for unused labels keep
//     their title as opaque skeleton bytes.
func (r *Reader) scanReferenceVisibility(doc ast.Node) {
	r.visibleRefs = map[string]bool{}
	r.usedRefs = map[string]bool{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Link:
			if ref := node.Reference; ref != nil {
				label := string(ref.Value)
				r.usedRefs[strings.ToLower(label)] = true
				if ref.Type == ast.ReferenceLinkShortcut || ref.Type == ast.ReferenceLinkCollapsed {
					r.visibleRefs[label] = true
				}
			}
		case *ast.Image:
			if ref := node.Reference; ref != nil {
				label := string(ref.Value)
				r.usedRefs[strings.ToLower(label)] = true
				if ref.Type == ast.ReferenceLinkShortcut || ref.Type == ast.ReferenceLinkCollapsed {
					r.visibleRefs[label] = true
				}
			}
		}
		return ast.WalkContinue, nil
	})
}

// emitLinkReferenceDefinition emits one `[label]: url "title"` block. The
// label and title are extracted as translatable text runs when the
// matching shortcut/collapsed reference is visible (label) or any
// reference uses the label (title); otherwise they are passed through as
// opaque skeleton bytes. Mirrors okapi MarkdownParser.visitReferenceDefinition.
//
// Native goldmark places the AST node's Start at the position of `[`
// (after any leading indent), matching okapi's behaviour of stripping
// 1-3 spaces of indentation from the rendered output.
func (r *Reader) emitLinkReferenceDefinition(ctx context.Context, ch chan<- model.PartResult, n *ast.LinkReferenceDefinition, source []byte, baseOffset int) {
	lines := n.Lines()
	if lines.Len() == 0 {
		return
	}
	first := lines.At(0)
	last := lines.At(lines.Len() - 1)

	defStart := first.Start + baseOffset
	defEnd := last.Stop + baseOffset

	// fullLineStart points at the first byte of the source line that
	// holds the `[label]:` marker — it backs up past any leading
	// CommonMark indent (0–3 spaces). Okapi strips that indent on
	// writeback, so we emit the gap up to defStart but skip it for the
	// definition's own bytes.
	fullLineStart := defStart
	for fullLineStart > 0 && r.source[fullLineStart-1] != '\n' {
		fullLineStart--
	}
	r.skelEmitGap(fullLineStart)
	r.skelCursor = defEnd

	label := string(n.Label)
	labelVisible := r.visibleRefs[label]
	titleUsed := len(n.Title) > 0 && r.usedRefs[strings.ToLower(label)]

	// Reconstruct the URL slice. Reference definitions written with
	// `<url>` angle brackets must preserve that wrapping; goldmark's
	// Destination field is the unwrapped URL, so we sniff the source
	// bytes immediately after the `:` separator to decide.
	urlLiteral := referenceDefinitionURLLiteral(n.Destination, r.source[defStart:defEnd])

	// The simple case: no translatable parts → emit as Data so the
	// non-skeleton write path can still reconstruct the line, and let
	// the skeleton path use the rebuilt literal (so leading indent gets
	// stripped). When skeleton storage is in use this still produces
	// byte-equal output because the rebuild uses the AST-derived label,
	// dest, and title which are exactly the source bytes minus the
	// indent.
	if !labelVisible && !titleUsed {
		r.dataCounter++
		titleOpen, titleClose := titleDelimiters(r.source[defStart:defEnd], string(n.Title))
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", r.dataCounter),
			Name: "link-reference-definition",
			Properties: map[string]string{
				"label":       label,
				"destination": urlLiteral,
				"title":       string(n.Title),
			},
		}
		// Preserve the literal whitespace authored after `]:` so
		// `[l]:  #list` (two spaces) doesn't collapse to `[l]: #list`.
		// Mirrors okapi MarkdownFilter, which round-trips link reference
		// definitions verbatim through skeleton bytes.
		sep := refDefSeparator(r.source[defStart:defEnd])
		r.skelText(buildLinkReferenceDefinitionLiteral(label, urlLiteral, sep, string(n.Title), titleOpen, titleClose))
		// preserve a trailing newline only when source had one (always
		// true for non-EOF defs; goldmark already trimmed the line value
		// so we add it back here).
		if defEnd < len(r.source) && r.source[defEnd] == '\n' {
			r.skelText("\n")
			r.skelCursor = defEnd + 1
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		return
	}

	// Translatable case: emit individual blocks for label and/or title,
	// interleaved with skeleton text containing the structural bytes.
	// Mirrors okapi's `addToQueue(refText, isVisibleRef(refText), REFERENCE)`
	// + `addToQueue(node.getTitle().toString(), isRefTextUsed(refText), REFERENCE)`
	// pattern: each translatable atom becomes its own short text unit so
	// the TM keys cleanly off the source string.
	r.skelText("[")
	if labelVisible {
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, label)
		block.Name = fmt.Sprintf("ref-label%d", r.blockCounter)
		block.Type = "link-reference-label"
		r.skelRef(blockID)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	} else {
		r.skelText(label)
	}
	r.skelText("]:" + refDefSeparator(r.source[defStart:defEnd]) + urlLiteral)
	if titleUsed {
		// Detect title delimiter from source so `'`, `"`, and `(...)`
		// forms all round-trip exactly.
		open, close := titleDelimiters(r.source[defStart:defEnd], string(n.Title))
		r.skelText(" " + open)
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, string(n.Title))
		block.Name = fmt.Sprintf("ref-title%d", r.blockCounter)
		block.Type = "link-reference-title"
		r.skelRef(blockID)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		r.skelText(close)
	} else if len(n.Title) > 0 {
		// Title present but not extracted — emit as skeleton bytes to
		// preserve the original source form.
		open, close := titleDelimiters(r.source[defStart:defEnd], string(n.Title))
		r.skelText(" " + open + string(n.Title) + close)
	}
	if defEnd < len(r.source) && r.source[defEnd] == '\n' {
		r.skelText("\n")
		r.skelCursor = defEnd + 1
	}
}

// buildLinkReferenceDefinitionLiteral builds the source representation
// of `[label]: url "title"` (or with the alternate quote/paren forms).
// Used when the definition has no translatable parts so the writer can
// still reconstruct the line from skeleton bytes alone.
func buildLinkReferenceDefinitionLiteral(label, dest, sep, title, titleOpen, titleClose string) string {
	if sep == "" {
		sep = " "
	}
	var sb strings.Builder
	sb.WriteByte('[')
	sb.WriteString(label)
	sb.WriteString("]:")
	sb.WriteString(sep)
	sb.WriteString(dest)
	if title != "" {
		open, close := titleOpen, titleClose
		if open == "" || close == "" {
			open, close = `"`, `"`
		}
		sb.WriteByte(' ')
		sb.WriteString(open)
		sb.WriteString(title)
		sb.WriteString(close)
	}
	return sb.String()
}

// refDefSeparator returns the literal whitespace bytes between the
// `]:` and the destination URL on the source line. Returns " " when
// the line is malformed (no colon found) so callers always get at
// least the canonical single-space separator.
func refDefSeparator(defLine []byte) string {
	colon := bytes.IndexByte(defLine, ':')
	if colon < 0 {
		return " "
	}
	i := colon + 1
	start := i
	for i < len(defLine) && (defLine[i] == ' ' || defLine[i] == '\t') {
		i++
	}
	if i == start {
		return " "
	}
	return string(defLine[start:i])
}

// referenceDefinitionURLLiteral returns the URL as it appeared in the
// source line (`<url>` or bare `url`). Goldmark's Destination field is
// always the unwrapped URL, so we sniff the bytes immediately after the
// `:` separator for a leading `<` to decide.
func referenceDefinitionURLLiteral(dest []byte, defLine []byte) string {
	d := string(dest)
	colon := bytes.IndexByte(defLine, ':')
	if colon < 0 {
		return d
	}
	// Skip past `:` and any subsequent spaces.
	i := colon + 1
	for i < len(defLine) && (defLine[i] == ' ' || defLine[i] == '\t') {
		i++
	}
	if i < len(defLine) && defLine[i] == '<' {
		return "<" + d + ">"
	}
	return d
}

// titleDelimiters returns the opening and closing characters used to
// wrap the link-reference-definition's title in source. Markdown allows
// `"..."`, `'...'`, and `(...)`. We sniff the bytes immediately before
// the title text to choose; default to double-quotes when the slice
// doesn't contain a recognisable delimiter pair (defensive — should
// never happen for valid CommonMark).
func titleDelimiters(defLine []byte, title string) (string, string) {
	if title == "" {
		return `"`, `"`
	}
	idx := bytes.Index(defLine, []byte(title))
	if idx <= 0 {
		return `"`, `"`
	}
	switch defLine[idx-1] {
	case '\'':
		return "'", "'"
	case '(':
		return "(", ")"
	default:
		return `"`, `"`
	}
}

// skelEmitGap emits any source bytes between the current cursor and the given
// absolute position as skeleton text.
func (r *Reader) skelEmitGap(absPos int) {
	if r.skeletonStore == nil {
		return
	}
	if absPos > r.skelCursor {
		r.skelText(string(r.source[r.skelCursor:absPos]))
		r.skelCursor = absPos
	}
}

// nodeAbsRange returns the absolute byte range of a node's lines content,
// accounting for the baseOffset from the body start.
func nodeAbsRange(node ast.Node, source []byte, baseOffset int) (int, int) {
	s, e := blockRange(node, source)
	return s + baseOffset, e + baseOffset
}

// softBreakContinuation returns the literal source bytes that bridge
// two inline runs across a soft line break: a leading newline plus any
// blockquote (`>` / `> `) or indentation prefix that introduces the
// continuation line. This preserves okapi-parity for paragraphs and
// blockquotes whose hard wraps must round-trip verbatim — okapi's
// MarkdownFilter keeps the literal `\n` (and continuation marker)
// rather than collapsing per CommonMark §6.7. Falls back to a single
// space when the source slice doesn't begin with a newline (defensive
// — should not happen for valid SoftLineBreak Text nodes).
func softBreakContinuation(source []byte, pos int) string {
	if pos < 0 || pos >= len(source) || source[pos] != '\n' {
		return " "
	}
	end := pos + 1
	for end < len(source) {
		c := source[end]
		switch c {
		case ' ', '\t':
			end++
			continue
		case '>':
			end++
			// A single optional space follows `>` per CommonMark §5.1.
			if end < len(source) && source[end] == ' ' {
				end++
			}
			continue
		}
		break
	}
	return string(source[pos:end])
}

// fullNodeAbsRange returns the absolute byte range of a node including
// any prefix characters (like "# " for headings). This scans backward
// from the line start to find the actual start of the markdown line.
func fullNodeAbsRange(node ast.Node, source []byte, baseOffset int) (int, int) {
	lines := node.Lines()
	if lines.Len() == 0 {
		return nodeAbsRange(node, source, baseOffset)
	}
	first := lines.At(0)
	last := lines.At(lines.Len() - 1)

	// Scan backward from line content start to find the beginning of the
	// source line (to capture prefixes like "# ", "- ", "> ", "1. ").
	lineStart := first.Start
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}

	return lineStart + baseOffset, last.Stop + baseOffset
}

// hasOnlyHardBreaks reports whether the inline subtree under node
// contains at least one hard line break and no soft line breaks. Used
// to decide whether BlockPropLinePrefix should fire: the soft-break
// path already bakes the literal `\n` + per-line prefix into the runs
// (see softBreakContinuation), so handing the same prefix to the
// writer would double it. Hard breaks emit a bare "\n" instead and
// therefore still need the property to round-trip blockquote/indent
// continuations. A node with no breaks at all returns false (single
// line — no continuation prefix to apply anywhere).
func hasOnlyHardBreaks(node ast.Node) bool {
	hardSeen := false
	softSeen := false
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := n.(*ast.Text); ok {
			if t.SoftLineBreak() {
				softSeen = true
			}
			if t.HardLineBreak() {
				hardSeen = true
			}
		}
		return ast.WalkContinue, nil
	})
	return hardSeen && !softSeen
}

// detectLinePrefix returns the inter-line prefix shared by every
// continuation line of the given node (e.g. "> " for a blockquote
// paragraph, "  " for an indented continuation inside a list item).
// The prefix is computed from the source bytes between consecutive
// line ranges: for line i+1 to logically continue line i with the
// same paragraph, goldmark's parser strips a per-line prefix that
// sits between the LF that ends line i's content and the start of
// line i+1's content. We take that slice from each pair, then keep
// it only when every gap has the same prefix.
//
// Returns "" when the paragraph has only one line, when the
// continuation lines have inconsistent prefixes (e.g. one line is
// `> ` and another is just empty), or when the gap is empty
// (single-line paragraphs the parser split internally — there's no
// per-line prefix to re-emit).
func detectLinePrefix(node ast.Node, source []byte) string {
	lines := node.Lines()
	if lines.Len() < 2 {
		return ""
	}
	var prefix string
	first := true
	for i := range lines.Len() - 1 {
		curStop := lines.At(i).Stop
		nextStart := lines.At(i + 1).Start
		if curStop > nextStart || nextStart > len(source) {
			return ""
		}
		gap := source[curStop:nextStart]
		// gap typically begins with the LF that ended the previous line,
		// followed by the per-line prefix. Strip the leading LF so the
		// returned prefix is what we want to re-emit AFTER each "\n"
		// in the block's text.
		if len(gap) > 0 && gap[0] == '\n' {
			gap = gap[1:]
		}
		if first {
			prefix = string(gap)
			first = false
		} else if string(gap) != prefix {
			// Inconsistent — bail out and let the writer emit just LFs.
			return ""
		}
	}
	return prefix
}

func (r *Reader) emitHeading(ctx context.Context, ch chan<- model.PartResult, n *ast.Heading, source []byte, baseOffset int) {
	r.blockCounter++
	blockID := fmt.Sprintf("tu%d", r.blockCounter)
	textContent := r.extractInlineText(n, source)
	block := model.NewBlock(blockID, textContent)
	block.Name = fmt.Sprintf("heading%d", r.blockCounter)
	block.Type = "heading"
	block.Properties["level"] = strconv.Itoa(n.Level)
	block.SetSemanticRole(model.RoleHeading, n.Level)
	r.addInlineRuns(block, n, source)

	absStart, _ := fullNodeAbsRange(n, source, baseOffset)
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)

	// Emit gap from cursor to the node's full start (includes blank lines before)
	r.skelEmitGap(absStart)
	// Emit prefix (e.g. "# ") as skeleton text
	r.skelText(string(r.source[absStart:lineStart]))
	// Emit block ref for the inline content
	r.skelRef(blockID)
	// Advance cursor past the lines
	r.skelCursor = lineEnd

	// Mirror upstream Okapi: an ATX heading with a closing marker
	// (`### foo ###`) is rendered with the closing marker on its own
	// line. See MarkdownParser.java:544-548 — the visitor emits a
	// newline, then the closing marker, then another newline. Detect
	// the trailing `#+` sequence between lineEnd and the next \n and
	// rewrite ` ###\n` → `\n###\n` in the skeleton.
	if trailEnd, marker, ok := atxClosingMarker(r.source, lineEnd); ok {
		r.skelText("\n" + marker)
		r.skelCursor = trailEnd
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// atxClosingMarker scans `source` from `start` looking for an ATX
// heading's optional trailing `#+` sequence (CommonMark §4.2: the
// closing marker is "an optional sequence of # characters" preceded
// by required whitespace and optionally followed by trailing
// whitespace, terminated by the line's newline). Returns
// (positionBeforeNewline, closingMarker, true) when found, or
// (0, "", false) when the line has no closing marker. The returned
// position points AT the trailing newline so the caller can emit a
// newline + marker pair and advance the skeleton cursor accordingly,
// mirroring upstream MarkdownParser.java:544-548.
func atxClosingMarker(source []byte, start int) (int, string, bool) {
	if start >= len(source) {
		return 0, "", false
	}
	i := start
	// Required whitespace between content and closing marker.
	hasSpace := false
	for i < len(source) && (source[i] == ' ' || source[i] == '\t') {
		i++
		hasSpace = true
	}
	if !hasSpace {
		return 0, "", false
	}
	// One or more '#' characters.
	markerStart := i
	for i < len(source) && source[i] == '#' {
		i++
	}
	if i == markerStart {
		return 0, "", false
	}
	marker := string(source[markerStart:i])
	// Optional trailing whitespace, then the line must end (newline or EOF).
	for i < len(source) && (source[i] == ' ' || source[i] == '\t') {
		i++
	}
	if i < len(source) && source[i] != '\n' {
		return 0, "", false
	}
	return i, marker, true
}

// admonitionHeaderRE matches a MkDocs/Material admonition opener:
//
//	!!! type
//	!!! type "Title"
//	??? type "Title"        (collapsible variant)
//	???+ type "Title"       (collapsible, open-by-default)
//
// Group 1 is the marker (`!!!`, `???`, `???+`). Group 2 is the type
// keyword (`note`, `danger`, `tip`, …). Group 3 (when present) is the
// quoted title — the inner-quote group `4` carries its content.
//
// Mirrors okapi MarkdownParser's admonition recognition: when the line
// has a non-empty quoted title, the keyword stays as skeleton bytes and
// the title is the translatable atom; when there's no title, the keyword
// itself becomes translatable; when the title is `""` the whole header
// line is skeleton.
var admonitionHeaderRE = regexp.MustCompile(`^(!!!|\?\?\?\+?)[ \t]+([A-Za-z][A-Za-z0-9_-]*)([ \t]+"([^"]*)")?[ \t]*$`)

// docusaurusAdmonRE matches a Docusaurus-style `:::keyword` line, with
// or without trailing inline title text.
//
//	:::note
//	:::note Some title text
//	:::                       (closing marker)
//
// Group 1 is the keyword (when present). Group 2 (when present) is the
// trailing title text — extracted as a translatable atom; the keyword
// itself stays as skeleton.
var docusaurusAdmonRE = regexp.MustCompile(`^:::([A-Za-z][A-Za-z0-9_-]*)?(?:[ \t]+(.+))?[ \t]*$`)

// admonitionBodyGaps returns the per-line goldmark-stripped indent of
// each body line (line 1 onwards). The slice's length is lines.Len()-1
// — entry i is the indent of body line i+1 in the paragraph. Lines for
// which the gap can't be reconstructed are omitted (returns nil).
func admonitionBodyGaps(n ast.Node, source []byte) []string {
	lines := n.Lines()
	if lines.Len() < 2 {
		return nil
	}
	out := make([]string, 0, lines.Len()-1)
	for i := range lines.Len() - 1 {
		curStop := lines.At(i).Stop
		nextStart := lines.At(i + 1).Start
		if curStop > nextStart || nextStart > len(source) {
			return nil
		}
		gap := source[curStop:nextStart]
		if len(gap) > 0 && gap[0] == '\n' {
			gap = gap[1:]
		}
		out = append(out, string(gap))
	}
	return out
}

// commonLeadingSpaces returns the longest run of leading whitespace
// (' ' or '\t') shared by every string in xs. Returns "" on an empty
// input or when any string has no leading whitespace.
func commonLeadingSpaces(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	min := xs[0]
	for _, s := range xs[1:] {
		// Walk both strings byte-by-byte; stop at the first divergence
		// or first non-whitespace character.
		i := 0
		for i < len(min) && i < len(s) && min[i] == s[i] && (min[i] == ' ' || min[i] == '\t') {
			i++
		}
		min = min[:i]
		if min == "" {
			return ""
		}
	}
	// Trim any trailing non-whitespace bytes from min (defensive — the
	// loop already guarantees this, but keeps the function correct in
	// the single-element case).
	end := len(min)
	for end > 0 && min[end-1] != ' ' && min[end-1] != '\t' {
		end--
	}
	return min[:end]
}

// emitAdmonition handles a `!!! type "Title"` (or `??? type "Title"`)
// admonition opener that goldmark parsed as a single Paragraph. The
// header line (line 0) is split into skeleton + translatable atoms per
// the rules described on admonitionHeaderRE. The body lines (line 1+)
// are re-emitted as a continuation paragraph block whose per-line
// prefix matches the body's indentation, so the writer round-trips
// the indented body verbatim.
//
// Returns true when the paragraph was recognised and emitted as an
// admonition; false leaves the paragraph for the regular emitParagraph
// path.
func (r *Reader) emitAdmonition(ctx context.Context, ch chan<- model.PartResult, n *ast.Paragraph, source []byte, baseOffset int) bool {
	lines := n.Lines()
	if lines.Len() == 0 {
		return false
	}
	first := lines.At(0)
	headerLine := source[first.Start:first.Stop]
	// Trim a trailing LF before regex matching — the regex expects a
	// "single line" with no terminator.
	headerNoLF := bytes.TrimRight(headerLine, "\n")
	m := admonitionHeaderRE.FindSubmatch(headerNoLF)
	if m == nil {
		return false
	}
	marker := string(m[1])  // !!!, ???, ???+
	keyword := string(m[2]) // note, danger, …
	hasTitleGroup := len(m[3]) > 0
	title := ""
	if len(m[4]) > 0 {
		title = string(m[4])
	}

	headerAbsStart := first.Start + baseOffset
	headerAbsEnd := first.Stop + baseOffset

	r.skelEmitGap(headerAbsStart)

	// Header skeleton + translatable atoms.
	switch {
	case hasTitleGroup && title != "":
		// `!!! type "Title"` → keyword is skeleton, title translates.
		r.skelText(marker + " " + keyword + ` "`)
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, title)
		block.Name = fmt.Sprintf("admon-title%d", r.blockCounter)
		block.Type = "admonition-title"
		r.skelRef(blockID)
		r.skelText(`"`)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})

	case hasTitleGroup && title == "":
		// `!!! type ""` → entire header line is skeleton.
		r.skelText(string(headerNoLF))

	default:
		// `!!! type` (no title) → keyword itself is the translatable atom.
		r.skelText(marker + " ")
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, keyword)
		block.Name = fmt.Sprintf("admon-keyword%d", r.blockCounter)
		block.Type = "admonition-keyword"
		r.skelRef(blockID)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	// Trailing LF of the header line, if any.
	if first.Stop > first.Start && source[first.Stop-1] == '\n' {
		r.skelText("\n")
	}
	r.skelCursor = headerAbsEnd

	// Body lines: lines[1..N]. We emit one block carrying the body's
	// inline text joined by literal LFs (matching emitParagraph's
	// behaviour for soft line breaks), and stash the body indent as the
	// per-line prefix so the writer re-emits each `\n` as `\n<indent>`.
	if lines.Len() < 2 {
		return true
	}

	gaps := admonitionBodyGaps(n, source)
	bodyIndent := commonLeadingSpaces(gaps)
	bodyAbsEnd := lines.At(lines.Len()-1).Stop + baseOffset

	// Emit the body's leading indent (e.g. `    `) as skeleton — the
	// writer re-emits it for line 2+ via the line-prefix property, but
	// line 1's prefix has to come from the skeleton because
	// BlockPropLinePrefix only fires AFTER each "\n" in the rendered
	// block text.
	if bodyIndent != "" {
		r.skelText(bodyIndent)
	}

	r.blockCounter++
	bodyID := fmt.Sprintf("tu%d", r.blockCounter)
	bodyText, perLineExcess := r.extractAdmonitionBodyText(n, source, bodyIndent)
	bodyBlock := model.NewBlock(bodyID, bodyText)
	bodyBlock.Name = fmt.Sprintf("admon-body%d", r.blockCounter)
	bodyBlock.Type = "admonition-body"
	// Convert body text into runs so structural keywords inside the
	// body — fenced-code language tags (`\`\`\` python`) and nested
	// admonition keywords (`!!! danger`, `??? note`, `:::tip`) —
	// become opaque inline placeholders. Without this they'd be
	// pseudo-translated alongside ordinary prose, e.g. `python` →
	// `ƥŷţĥōń`. Mirrors okapi MarkdownParser, which detects these
	// constructs inside admonition bodies and excludes their tag
	// fragments from translation.
	if runs := admonitionBodyRuns(bodyText); runs != nil {
		bodyBlock.Source = runs
	}
	// perLineExcess indicates whether any body line carries indent
	// beyond the outer admonition indent. When every body line sits at
	// exactly the outer indent we can rely on BlockPropLinePrefix alone
	// to round-trip; otherwise the per-line excess is already embedded
	// in bodyText so we must NOT also apply the outer prefix at write
	// time (it would double up). Mirrors okapi MarkdownParser, which
	// extracts each body line at its source column once the outer
	// indent has been stripped.
	if bodyIndent != "" && !perLineExcess {
		bodyBlock.Properties[BlockPropLinePrefix] = bodyIndent
	}
	r.skelRef(bodyID)
	r.skelCursor = bodyAbsEnd

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: bodyBlock})
	return true
}

// admonitionBodyKeywordRE matches structural keyword patterns inside
// an admonition body that must NOT be pseudo-translated:
//
//	!!! note         (any !!!/??? admonition opener; group 1 is the keyword)
//	??? danger
//	``` python       (fenced-code-block info string; group 1 is the language)
//	~~~ go
//	:::tip           (Docusaurus admonition opener)
//
// The whole match — marker + space + keyword — is treated as one
// opaque placeholder so the writer renders the bytes verbatim through
// pseudo-translation. The marker (group 0 prefix) and keyword (group
// 1 capture) round-trip together to keep the construct recognisable.
var admonitionBodyKeywordRE = regexp.MustCompile(`(?:!!!\+?|\?\?\?\+?|\x60{3,}|~{3,}|:::)[ \t]*[A-Za-z][A-Za-z0-9_-]*`)

// admonitionBodyRuns returns a Run sequence for the given admonition
// body text where structural keyword patterns (see
// admonitionBodyKeywordRE) are emitted as opaque placeholders. Returns
// nil when the body text contains no keyword patterns — the caller
// then keeps the default plain-text segment NewBlock provides.
func admonitionBodyRuns(text string) []model.Run {
	matches := admonitionBodyKeywordRE.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	b := newRunBuilder()
	id := 0
	pos := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		if start > pos {
			b.AddText(text[pos:start])
		}
		id++
		b.AddPh(
			strconv.Itoa(id),
			"code:keyword",
			"md:admon-keyword",
			text[start:end],
			"", "",
			false, false, false,
		)
		pos = end
	}
	if pos < len(text) {
		b.AddText(text[pos:])
	}
	return b.Runs()
}

// admonitionBodyStructuralMarkerRE matches a body line whose
// (post-outerIndent-strip) content begins with a structural marker:
// fenced-code fence (`\`\`\“, `~~~`), nested admonition opener
// (`!!!`, `???`), or Docusaurus marker (`:::`). Lines that match are
// NOT joined into the preceding paragraph for indent-doubling purposes
// — they belong to a separate inner block in okapi's sub-parsed view.
var admonitionBodyStructuralMarkerRE = regexp.MustCompile(`^(?:\x60{3,}|~{3,}|!!!\+?|\?\?\?\+?|:::)`)

// extractAdmonitionBodyText returns the body's text content plus a
// flag indicating whether any body line had indent beyond outerIndent.
//
// Two strategies depending on the body shape:
//
//   - When every body line sits at exactly outerIndent (the simple case
//     for plain-text admonition bodies), the returned text contains
//     just the line content joined with `\n` — the writer re-applies
//     outerIndent on every newline via BlockPropLinePrefix, so the
//     round-trip recovers the original 4-space indent.
//
//   - When some body lines have extra indent (e.g. an inner code block
//     at 8 spaces, or a nested admonition), the returned text embeds
//     the *full* per-line indent verbatim (including outerIndent on
//     every continuation line). The caller must then NOT set
//     BlockPropLinePrefix — doing so would double-apply outerIndent.
//
// Within the second strategy we additionally double the gap of
// "intra-paragraph soft break" pairs — adjacent body lines that both
// sit at outerIndent and don't begin a structural marker. This
// mirrors okapi MarkdownParser, which sub-parses the body and lets
// the writer apply outerIndent twice to such continuation lines (once
// from the per-line gap stored in the TextUnit, once from the writer's
// line-prefix pass).
//
// The hasExcess return flag lets the caller make that choice.
func (r *Reader) extractAdmonitionBodyText(n ast.Node, source []byte, outerIndent string) (text string, hasExcess bool) {
	lines := n.Lines()
	if lines.Len() < 2 {
		return "", false
	}
	gaps := admonitionBodyGaps(n, source)

	// Decide strategy: if any gap exceeds outerIndent, embed full gaps.
	for _, g := range gaps {
		if g != outerIndent {
			hasExcess = true
			break
		}
	}

	// Pre-compute each line's structural-marker status (after stripping
	// outerIndent) so the join loop below can decide whether to double
	// an intra-paragraph gap or keep it verbatim.
	isStructural := make([]bool, lines.Len())
	for i := range lines.Len() {
		line := lines.At(i)
		segment := bytes.TrimRight(source[line.Start:line.Stop], "\n")
		// Strip outerIndent from the content's leading whitespace —
		// some lines (code-block content with extra indent) won't have
		// outerIndent stripped already (gap > outerIndent), but for
		// goldmark the line.Start already points past outerIndent for
		// every body line, so we treat the segment directly.
		isStructural[i] = admonitionBodyStructuralMarkerRE.Match(segment)
	}

	var buf strings.Builder
	for i := 1; i < lines.Len(); i++ {
		if i > 1 {
			buf.WriteByte('\n')
			gapIdx := i - 1
			if hasExcess && gapIdx < len(gaps) {
				gap := gaps[gapIdx]
				// Intra-paragraph soft break: prev and current lines
				// are both plain prose (no structural markers). Double
				// the gap by emitting outerIndent + gap so the writer's
				// line-prefix pass produces (outer + per-line) total
				// indent on writeback. Mirrors okapi's sub-parser
				// behaviour for continuation lines.
				if !isStructural[i-1] && !isStructural[i] && gap == outerIndent {
					buf.WriteString(outerIndent)
				}
				buf.WriteString(gap)
			}
		}
		line := lines.At(i)
		segment := source[line.Start:line.Stop]
		segment = bytes.TrimRight(segment, "\n")
		buf.Write(segment)
	}
	text = buf.String()
	return text, hasExcess
}

// emitDocusaurusAdmonition handles a single-line `:::keyword [title]`
// (or bare `:::` closer) that goldmark parsed as a one-line paragraph.
// Returns true when the paragraph was recognised and emitted as a
// Docusaurus admonition marker; false leaves it for emitParagraph.
//
// Match rules:
//
//	:::            → entire line is skeleton (closing marker)
//	:::tip         → entire line is skeleton (opening marker, no inline title)
//	:::note Title  → `:::note ` is skeleton, `Title` is the translatable atom
func (r *Reader) emitDocusaurusAdmonition(ctx context.Context, ch chan<- model.PartResult, n *ast.Paragraph, source []byte, baseOffset int) bool {
	lines := n.Lines()
	if lines.Len() != 1 {
		return false
	}
	first := lines.At(0)
	lineBytes := source[first.Start:first.Stop]
	lineNoLF := bytes.TrimRight(lineBytes, "\n")
	if !bytes.HasPrefix(lineNoLF, []byte(":::")) {
		return false
	}
	m := docusaurusAdmonRE.FindSubmatch(lineNoLF)
	if m == nil {
		return false
	}

	absStart := first.Start + baseOffset
	absEnd := first.Stop + baseOffset
	r.skelEmitGap(absStart)

	keyword := string(m[1]) // may be ""
	tail := strings.TrimSpace(string(m[2]))

	if tail == "" {
		// `:::` or `:::keyword` with no inline title — entire line is
		// skeleton, no translatable atom.
		r.skelText(string(lineNoLF))
	} else {
		// `:::keyword Title` — keyword + space is skeleton, title is
		// the translatable atom.
		r.skelText(":::" + keyword + " ")
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, tail)
		block.Name = fmt.Sprintf("docu-admon-title%d", r.blockCounter)
		block.Type = "docusaurus-admonition-title"
		r.skelRef(blockID)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	// Trailing LF of the line, if any.
	if first.Stop > first.Start && source[first.Stop-1] == '\n' {
		r.skelText("\n")
	}
	r.skelCursor = absEnd
	return true
}

func (r *Reader) emitParagraph(ctx context.Context, ch chan<- model.PartResult, n *ast.Paragraph, source []byte, baseOffset int) {
	// MkDocs/Material `!!! type "Title"` and `??? type "Title"` opener
	// (with body indented underneath) is a single goldmark Paragraph
	// because admonition syntax isn't part of CommonMark. Detect and
	// route to emitAdmonition so the keyword stays as skeleton and the
	// title + body extract as separate translatable atoms — mirrors
	// okapi MarkdownParser's admonition handling.
	if r.emitAdmonition(ctx, ch, n, source, baseOffset) {
		return
	}
	// Docusaurus `:::note [Title]` (and the bare `:::` closer) is a
	// single-line paragraph in goldmark — same routing.
	if r.emitDocusaurusAdmonition(ctx, ch, n, source, baseOffset) {
		return
	}

	r.blockCounter++
	blockID := fmt.Sprintf("tu%d", r.blockCounter)
	textContent := r.extractInlineText(n, source)
	block := model.NewBlock(blockID, textContent)
	block.Name = fmt.Sprintf("para%d", r.blockCounter)
	r.addInlineRuns(block, n, source)

	// Multi-line paragraphs (e.g. blockquote bodies, indented
	// continuation lines) carry a per-line prefix in source — `> ` for
	// blockquotes, indentation for list-item continuations. Soft-break
	// continuations already have the literal `\n` + prefix baked into
	// the runs by softBreakContinuation; storing the same prefix here
	// too would let the writer prepend it a second time after every
	// "\n", yielding `\n> > ` for blockquotes. Only hand the prefix to
	// the writer when no soft break carries it (i.e. hard breaks, where
	// the run sequence emits a bare "\n" and the writer must reinject
	// the marker). When a paragraph mixes both kinds we still skip the
	// property — the soft-break path covers the more common case and
	// any bare "\n" introduced by a hard break stays unprefixed (an
	// edge case okapi MarkdownFilter doesn't special-case either).
	if hasOnlyHardBreaks(n) {
		if prefix := detectLinePrefix(n, source); prefix != "" {
			block.Properties[BlockPropLinePrefix] = prefix
		}
	}

	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)

	r.skelEmitGap(lineStart)
	r.skelRef(blockID)
	r.skelCursor = lineEnd

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

func (r *Reader) emitListItem(ctx context.Context, ch chan<- model.PartResult, n *ast.ListItem, source []byte, baseOffset int) {
	// Check for nested blocks (sublists, code blocks, HTML blocks).
	// When present, the list item carries both leading inline text
	// (a TextBlock or Paragraph) and one or more nested block-level
	// children. We must still extract the leading inline text as its
	// own translatable block — otherwise "- item two\n  - sublist"
	// loses the "item two" translation. Mirrors okapi
	// MarkdownParser.visitListItem, which emits the inline header line
	// before recursing into nested children.
	// A "mixed" list item is one whose body is more than a single
	// inline text run — anything that needs its own block emit (sublist,
	// code block, HTML block, blockquote, table) or a "loose-list"
	// continuation paragraph (paragraph siblings beyond the first). The
	// simple-emit path below handles only one Paragraph/TextBlock and
	// silently drops everything else, so we route every multi-child or
	// non-paragraph case through emitListItemMixed which walks ALL
	// children. Mirrors okapi MarkdownParser.visitListItem
	// (MarkdownParser.java:1062–1083) which calls
	// visitor.visitChildren(listItem) unconditionally — every child node
	// gets visited regardless of type or count.
	hasNestedBlocks := false
	paragraphLikeCount := 0
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.(type) {
		case *ast.List, *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock, *ast.Blockquote, *ast.Heading, *ast.ThematicBreak:
			hasNestedBlocks = true
		case *ast.Paragraph, *ast.TextBlock:
			paragraphLikeCount++
		default:
			// Tables and other non-inline nodes also need their own
			// block emit; route through the mixed path so walkSingle
			// can dispatch them.
			if child.Kind() == east.KindTable {
				hasNestedBlocks = true
			}
		}
	}
	if paragraphLikeCount > 1 {
		hasNestedBlocks = true
	}

	if hasNestedBlocks {
		r.emitListItemMixed(ctx, ch, n, source, baseOffset)
		return
	}

	// Empty list items (e.g. a bare "5. " on its own line) have no
	// inline children, so blockRange returns (0, 0) — using that as the
	// skeleton position would reset skelCursor to 0 and cause the next
	// emit to re-flush the entire document as skeleton text. There's
	// nothing to translate either, so consume the marker line directly
	// into skeleton text and advance the cursor. Trailing whitespace on
	// the marker line is dropped to mirror okapi MarkdownFilterWriter,
	// which strips the bare marker's trailing space on round-trip.
	if n.FirstChild() == nil {
		// Find the marker line: skip any blank lines from the cursor,
		// then take the next line as the empty marker.
		i := r.skelCursor
		for i < len(r.source) && r.source[i] == '\n' {
			i++
		}
		lineEnd := i
		for lineEnd < len(r.source) && r.source[lineEnd] != '\n' {
			lineEnd++
		}
		if i >= len(r.source) || lineEnd <= i {
			return
		}
		// Emit pending blank lines as-is, then the trimmed marker, then
		// the line terminator (stripping trailing whitespace mirrors
		// okapi's MarkdownFilterWriter behaviour for empty markers).
		if i > r.skelCursor {
			r.skelText(string(r.source[r.skelCursor:i]))
		}
		trimmed := strings.TrimRight(string(r.source[i:lineEnd]), " \t")
		r.skelText(trimmed)
		if lineEnd < len(r.source) && r.source[lineEnd] == '\n' {
			r.skelText("\n")
			lineEnd++
		}
		r.skelCursor = lineEnd
		return
	}

	r.blockCounter++
	blockID := fmt.Sprintf("tu%d", r.blockCounter)
	textContent := r.extractListItemText(n, source)
	block := model.NewBlock(blockID, textContent)
	block.Name = fmt.Sprintf("item%d", r.blockCounter)
	block.Type = "list-item"
	block.SetSemanticRole(model.RoleListItem, 0)

	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if p, ok := child.(*ast.Paragraph); ok {
			r.addInlineRuns(block, p, source)
			if hasOnlyHardBreaks(p) {
				if prefix := detectLinePrefix(p, source); prefix != "" {
					block.Properties[BlockPropLinePrefix] = prefix
				}
			}
			break
		}
		if tb, ok := child.(*ast.TextBlock); ok {
			r.addInlineRuns(block, child, source)
			if hasOnlyHardBreaks(tb) {
				if prefix := detectLinePrefix(tb, source); prefix != "" {
					block.Properties[BlockPropLinePrefix] = prefix
				}
			}
			break
		}
	}

	// For list items: find the text block range and include the list marker prefix
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)
	absStart := lineStart
	// Scan backward to find the list marker
	for absStart > 0 && r.source[absStart-1] != '\n' {
		absStart--
	}

	r.skelEmitGap(absStart)
	// The prefix (e.g. "- " or "1. ") goes as skeleton text
	r.skelText(string(r.source[absStart:lineStart]))
	r.skelRef(blockID)
	r.skelCursor = lineEnd

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// emitListItemMixed handles a list item whose body mixes a leading
// inline text node (TextBlock/Paragraph) with one or more nested
// block-level children (sublist, fenced code, HTML block, …). The
// leading text becomes a list-item block in its own right (so the
// translation lands on the marker line), and remaining children walk
// through walkNode as siblings — that way nested sublists, code
// blocks, etc. emit their own blocks with correct skeleton offsets.
func (r *Reader) emitListItemMixed(ctx context.Context, ch chan<- model.PartResult, n *ast.ListItem, source []byte, baseOffset int) {
	// Locate the leading inline node, if any. Nested children come
	// after it — we walk them after emitting the header.
	var headerNode ast.Node
	first := n.FirstChild()
	if first != nil {
		switch first.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			headerNode = first
		}
	}

	if headerNode != nil {
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		textContent := r.extractInlineText(headerNode, source)
		block := model.NewBlock(blockID, textContent)
		block.Name = fmt.Sprintf("item%d", r.blockCounter)
		block.Type = "list-item"
		block.SetSemanticRole(model.RoleListItem, 0)
		r.addInlineRuns(block, headerNode, source)
		if hasOnlyHardBreaks(headerNode) {
			if prefix := detectLinePrefix(headerNode, source); prefix != "" {
				block.Properties[BlockPropLinePrefix] = prefix
			}
		}

		// Use the header node's line range so we don't accidentally
		// claim bytes belonging to the trailing nested children. Scan
		// backward to capture the list marker prefix ("- ", "1. ").
		lineStart, lineEnd := nodeAbsRange(headerNode, source, baseOffset)
		absStart := lineStart
		for absStart > 0 && r.source[absStart-1] != '\n' {
			absStart--
		}

		r.skelEmitGap(absStart)
		r.skelText(string(r.source[absStart:lineStart]))
		r.skelRef(blockID)
		r.skelCursor = lineEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	// Walk the remaining children. If we emitted a header, skip it on
	// the recursion; otherwise walk all children. We can't reuse
	// walkNode directly (it walks every child), so iterate manually
	// and dispatch via the same switch.
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if child == headerNode {
			continue
		}
		r.walkSingle(ctx, ch, child, source, baseOffset)
	}
}

// walkSingle dispatches a single AST child through the same switch
// walkNode uses. Extracted so emitListItemMixed can iterate the
// children manually while skipping the header node it already emitted.
func (r *Reader) walkSingle(ctx context.Context, ch chan<- model.PartResult, child ast.Node, source []byte, baseOffset int) {
	switch n := child.(type) {
	case *ast.Heading:
		r.emitHeading(ctx, ch, n, source, baseOffset)
	case *ast.Paragraph:
		r.emitParagraph(ctx, ch, n, source, baseOffset)
	case *ast.ListItem:
		r.emitListItem(ctx, ch, n, source, baseOffset)
	case *ast.FencedCodeBlock:
		r.emitFencedCodeBlock(ctx, ch, n, source, baseOffset)
	case *ast.CodeBlock:
		r.emitIndentedCodeBlock(ctx, ch, n, source, baseOffset)
	case *ast.HTMLBlock:
		r.emitHTMLBlock(ctx, ch, n, source, baseOffset)
	case *ast.ThematicBreak:
		r.emitThematicBreak(ctx, ch, n, source, baseOffset)
	case *ast.List:
		r.walkNode(ctx, ch, child, source, baseOffset)
	case *ast.Blockquote:
		if r.cfg.TranslateBlockQuotes() {
			r.walkNode(ctx, ch, child, source, baseOffset)
		} else if r.cfg.ExtractNonTranslatableContent() {
			r.emitBlockquoteAsContent(ctx, ch, child, source, baseOffset)
		} else {
			r.emitBlockquoteAsData(ctx, ch, n, source, baseOffset)
		}
	case *ast.LinkReferenceDefinition:
		r.emitLinkReferenceDefinition(ctx, ch, n, source, baseOffset)
	default:
		if child.Kind() == east.KindTable {
			r.emitTable(ctx, ch, child, source, baseOffset)
		} else {
			r.walkNode(ctx, ch, child, source, baseOffset)
		}
	}
}

func (r *Reader) emitFencedCodeBlock(ctx context.Context, ch chan<- model.PartResult, n *ast.FencedCodeBlock, source []byte, baseOffset int) {
	// Mirror upstream MarkdownParser.java:413-424 (FencedCodeBlock
	// visitor) which iterates the content lines and skips any line
	// matching NEWLINE_ONLY_PATTERN ("blank line") — okapi's
	// round-tripped fenced code blocks therefore drop blank lines from
	// inside the fences. Without this, fixtures like example2.md round-
	// trip with a stray empty line where the source has one inside a
	// JS fenced block.
	content := extractRawLinesSkipBlanks(n, source)
	lang := ""
	if l := n.Language(source); l != nil {
		lang = string(l)
	}

	// For fenced code blocks, we need the full range including the ``` fences.
	// The Lines() only contain the code content, not the fence lines.
	// We need to find the fence lines in the source.
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)

	// Scan backward from the code content to find the opening fence
	fenceStart := lineStart
	for fenceStart > 0 && r.source[fenceStart-1] != '\n' {
		fenceStart--
	}
	// Go one more line back to find the opening fence line itself
	if fenceStart > 0 {
		// fenceStart is at the start of the first code content line,
		// the opening ``` is the line before
		openFenceStart := fenceStart
		if openFenceStart > 0 {
			openFenceStart--
			for openFenceStart > 0 && r.source[openFenceStart-1] != '\n' {
				openFenceStart--
			}
		}
		fenceStart = openFenceStart
	}

	// Find the closing fence after the content
	fenceEnd := lineEnd
	if fenceEnd < len(r.source) && r.source[fenceEnd] == '\n' {
		fenceEnd++
	}
	// The closing ``` line
	closeFenceEnd := fenceEnd
	for closeFenceEnd < len(r.source) && r.source[closeFenceEnd] != '\n' {
		closeFenceEnd++
	}
	if closeFenceEnd < len(r.source) && r.source[closeFenceEnd] == '\n' {
		closeFenceEnd++
	}

	if r.cfg.TranslateCodeBlocks {
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, content)
		block.Name = fmt.Sprintf("code%d", r.blockCounter)
		block.Type = "code-block"
		// Normalized structure layer (WS1) so cross-format export (e.g. DocLang
		// <code> + the recommended Linguist language <label>) carries the role
		// and language, not just the markdown-local skeleton fence.
		block.SetSemanticRole(model.RoleCode, 0)
		if lang != "" {
			block.Properties["language"] = lang
			block.SetCodeLanguage(lang)
		}

		r.skelEmitGap(fenceStart)
		// Opening fence as skeleton text
		r.skelText(string(r.source[fenceStart:lineStart]))
		r.skelRef(blockID)
		// Closing fence as skeleton text
		r.skelText(string(r.source[lineEnd:closeFenceEnd]))
		r.skelCursor = closeFenceEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	} else if r.cfg.ExtractNonTranslatableContent() {
		// Surface the code as non-translatable RoleCode content (visible to
		// ingestion, skipped by MT); fences stay skeleton, body rides a ref.
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, content)
		block.Name = fmt.Sprintf("code%d", r.blockCounter)
		block.Type = "code-block"
		block.Translatable = false
		block.PreserveWhitespace = true
		block.SetSemanticRole(model.RoleCode, 0)
		if lang != "" {
			block.Properties["language"] = lang
		}

		r.skelEmitGap(fenceStart)
		r.skelText(string(r.source[fenceStart:lineStart]))
		r.skelRef(blockID)
		r.skelText(string(r.source[lineEnd:closeFenceEnd]))
		r.skelCursor = closeFenceEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	} else {
		r.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", r.dataCounter),
			Name: "code-block",
			Properties: map[string]string{
				"content": content,
			},
		}
		if lang != "" {
			data.Properties["language"] = lang
		}

		r.skelEmitGap(fenceStart)
		r.skelText(string(r.source[fenceStart:closeFenceEnd]))
		r.skelCursor = closeFenceEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}
}

func (r *Reader) emitIndentedCodeBlock(ctx context.Context, ch chan<- model.PartResult, n *ast.CodeBlock, source []byte, baseOffset int) {
	content := r.extractRawLines(n, source)

	// For indented code blocks, the Lines() include the indented content.
	// Scan backward to find the real start including indentation.
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)
	absStart := lineStart
	for absStart > 0 && r.source[absStart-1] != '\n' {
		absStart--
	}

	if r.cfg.TranslateCodeBlocks {
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		// Use the literal source slice from the first line content
		// (after the 4-space indent already emitted as skeleton) to the
		// last line's end. This preserves per-line indentation on
		// continuation lines (goldmark's Lines().Value() strips the
		// 4-space prefix uniformly, so a "    code\n         deeper"
		// indented block would lose the 5-space relative indent on
		// "deeper"). The source slice carries those bytes verbatim,
		// including any blank lines between content lines.
		blockContent := string(r.source[lineStart:lineEnd])
		// okapi MarkdownFilterWriter re-indents blank lines within an
		// indented code block to the same prefix as content lines. The
		// CommonMark source typically writes blank lines unindented
		// ("...\n\n    next line"), but okapi's text-unit content
		// carries the prefix on every line — including the blanks — so
		// the round-tripped output has a "    \n    next line"
		// rectangle. Mirror that here so the block's text round-trips
		// byte-equal with okapi.
		prefix := string(r.source[absStart:lineStart])
		if prefix != "" && strings.Contains(blockContent, "\n\n") {
			blockContent = strings.ReplaceAll(blockContent, "\n\n", "\n"+prefix+"\n")
		}
		block := model.NewBlock(blockID, blockContent)
		block.Name = fmt.Sprintf("code%d", r.blockCounter)
		block.Type = "code-block"
		block.SetSemanticRole(model.RoleCode, 0) // WS1 role (indented code has no language)

		r.skelEmitGap(absStart)
		r.skelText(prefix)
		r.skelRef(blockID)
		r.skelCursor = lineEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	} else if r.cfg.ExtractNonTranslatableContent() {
		// Surface as non-translatable RoleCode content; the 4-space indent stays
		// skeleton, the verbatim body rides a ref (byte-exact round-trip).
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		prefix := string(r.source[absStart:lineStart])
		blockContent := string(r.source[lineStart:lineEnd])
		block := model.NewBlock(blockID, blockContent)
		block.Name = fmt.Sprintf("code%d", r.blockCounter)
		block.Type = "code-block"
		block.Translatable = false
		block.PreserveWhitespace = true
		block.SetSemanticRole(model.RoleCode, 0)

		r.skelEmitGap(absStart)
		r.skelText(prefix)
		r.skelRef(blockID)
		r.skelCursor = lineEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	} else {
		r.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", r.dataCounter),
			Name: "code-block",
			Properties: map[string]string{
				"content": content,
			},
		}

		r.skelEmitGap(absStart)
		r.skelText(string(r.source[absStart:lineEnd]))
		r.skelCursor = lineEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}
}

func (r *Reader) emitHTMLBlock(ctx context.Context, ch chan<- model.PartResult, n *ast.HTMLBlock, source []byte, baseOffset int) {
	content := r.extractRawLines(n, source)
	lineStart, lineEnd := nodeAbsRange(n, source, baseOffset)

	// HTML blocks also have a ClosureLine that may contain the closing tag
	if n.HasClosure() {
		cl := n.ClosureLine
		closureEnd := cl.Stop + baseOffset
		if closureEnd > lineEnd {
			content += string(cl.Value(source))
			lineEnd = closureEnd
		}
	}

	if r.cfg.TranslateHTMLBlocks {
		r.blockCounter++
		blockID := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(blockID, content)
		block.Name = fmt.Sprintf("html%d", r.blockCounter)
		block.Type = "html-block"

		r.skelEmitGap(lineStart)
		r.skelRef(blockID)
		r.skelCursor = lineEnd

		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		return
	}

	// Default path: route HTML block through the in-process HTML subfilter
	// (text content + select translatable attributes become Blocks; tags
	// remain skeleton). Mirrors okapi MarkdownFilter.processByHtmlFilter
	// (MarkdownFilter.java line 604) which delegates HTML blocks to
	// HtmlFilter via okf_html@for_markdown.fprm so only inline content is
	// extracted while tag structure passes through verbatim.
	r.skelEmitGap(lineStart)
	r.processHTMLBlockSubfilter(ctx, ch, r.source[lineStart:lineEnd])
	r.skelCursor = lineEnd
}

// htmlExcludedElements are elements whose content (and any nested
// content) is not translated. Mirrors okf_html@for_markdown.fprm's
// EXCLUDE rule for math, script, style, stylesheet.
var htmlExcludedElements = map[string]bool{
	"math":       true,
	"script":     true,
	"style":      true,
	"stylesheet": true,
}

// htmlTranslatableAttrs maps a tag name to the set of attributes whose
// values should be extracted as translatable Blocks. Mirrors the
// translatableAttributes map in okf_html@for_markdown.fprm. We cover
// only the high-impact attrs (alt, title, value) on the elements that
// appear in upstream markdown fixtures; the full okapi rule set is
// larger but parity-tested fixtures don't exercise it.
var htmlTranslatableAttrs = map[string]map[string]bool{
	"a":      {"title": true},
	"abbr":   {"title": true},
	"area":   {"alt": true, "title": true},
	"applet": {"alt": true, "title": true},
	"img":    {"alt": true, "title": true},
}

// processHTMLBlockSubfilter tokenizes the raw bytes of an HTML block and
// emits one Block per translatable text run + one Block per translatable
// attribute value, preserving tag structure (and the verbatim bytes of
// any non-translatable text such as whitespace, content inside excluded
// elements, or entity references) as skeleton text. Mirrors okapi's
// processByHtmlFilter path which routes HTML block content through an
// HtmlFilter subfilter rather than translating the whole block as one
// opaque text unit.
func (r *Reader) processHTMLBlockSubfilter(ctx context.Context, ch chan<- model.PartResult, content []byte) {
	z := xhtml.NewTokenizer(bytes.NewReader(content))
	z.SetMaxBuf(0)

	extractMath := r.cfg.ExtractNonTranslatableContent()
	excludeDepth := 0 // >0 when inside script/style — text becomes skeleton
	mathDepth := 0    // >0 when accumulating a <math> body as RoleFormula content
	var mathBuf strings.Builder
	// flushMath emits the accumulated <math> body as a non-translatable
	// RoleFormula content block (visible to ingestion, skipped by MT); a
	// whitespace-only body rides the skeleton verbatim so round-trip stays
	// byte-exact.
	flushMath := func() {
		body := mathBuf.String()
		mathBuf.Reset()
		if hasNonWhitespace([]byte(body)) {
			r.emitMathFormulaBlock(ctx, ch, body)
		} else {
			r.skelText(body)
		}
	}
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			// Unterminated <math> at EOF: flush whatever accumulated so the
			// bytes still round-trip rather than vanishing.
			if mathDepth > 0 {
				flushMath()
			}
			return
		}
		raw := z.Raw()
		// Copy raw because subsequent Next() calls invalidate the slice.
		rawBytes := append([]byte(nil), raw...)

		// While inside a surfaced <math> element, every token's bytes
		// accumulate into the formula body until the matching close. The
		// opening <math…> and closing </math> tags themselves stay
		// skeleton (delimiters), so only the body rides the content block.
		if mathDepth > 0 {
			if tt == xhtml.StartTagToken || tt == xhtml.EndTagToken {
				tagBytes, _ := z.TagName()
				if strings.EqualFold(string(tagBytes), "math") {
					if tt == xhtml.StartTagToken {
						mathDepth++
						mathBuf.Write(rawBytes)
						continue
					}
					// End </math>.
					mathDepth--
					if mathDepth == 0 {
						flushMath()
						r.skelText(string(rawBytes))
						continue
					}
					mathBuf.Write(rawBytes)
					continue
				}
			}
			mathBuf.Write(rawBytes)
			continue
		}

		switch tt {
		case xhtml.TextToken:
			if excludeDepth > 0 || !hasNonWhitespace(rawBytes) {
				r.skelText(string(rawBytes))
				continue
			}
			r.emitHTMLSubfilterTextBlock(ctx, ch, rawBytes)

		case xhtml.StartTagToken, xhtml.SelfClosingTagToken, xhtml.EndTagToken:
			tagBytes, _ := z.TagName()
			tag := strings.ToLower(string(tagBytes))
			if tt == xhtml.EndTagToken {
				if htmlExcludedElements[tag] && excludeDepth > 0 {
					excludeDepth--
				}
				r.skelText(string(rawBytes))
				continue
			}
			// Open <math>: surface the MathML body as a RoleFormula content
			// block. The tag stays skeleton; the body accumulates until the
			// matching close. When extraction is off, fall through to the
			// generic excluded-element path (body → skeleton).
			if tt == xhtml.StartTagToken && tag == "math" && extractMath {
				r.skelText(string(rawBytes))
				mathDepth = 1
				continue
			}
			// Start or self-closing tag: maybe extract attributes.
			if tt == xhtml.StartTagToken && htmlExcludedElements[tag] {
				excludeDepth++
				r.skelText(string(rawBytes))
				continue
			}
			r.emitHTMLSubfilterTag(ctx, ch, tag, rawBytes)

		case xhtml.CommentToken:
			// `<![CDATA[...]]>` arrives here (html.Tokenizer reports it
			// as a comment token). Mirror okapi's HTML_CDATA_PAT path
			// which extracts the CDATA payload as a translatable text
			// block while keeping the `<![CDATA[` and `]]>` markers as
			// skeleton bytes.
			if open, body, close, ok := splitCDATA(rawBytes); ok && excludeDepth == 0 {
				r.skelText(string(open))
				if hasNonWhitespace(body) {
					r.emitHTMLSubfilterTextBlock(ctx, ch, body)
				} else {
					r.skelText(string(body))
				}
				r.skelText(string(close))
				continue
			}
			r.skelText(string(rawBytes))

		default:
			// Doctype, etc. — pass through verbatim.
			r.skelText(string(rawBytes))
		}
	}
}

// emitHTMLSubfilterTextBlock emits one translatable Block whose source
// text is the raw token bytes with HTML entity references decoded
// (e.g. `&#39;` → `'`, `&amp;` → `&`). Mirrors okapi's HTML subfilter
// path (AbstractMarkupFilter.handleNumericEntity / handleCharacterEntity)
// which decodes entities before adding them to the text unit, except
// when `preserve_character_entities` is set on the subfilter config —
// the markdown HTML subfilter config (okf_html@for_markdown.fprm) leaves
// it unset, so decoding is the default. The decoded text passes through
// the markdown writer (MarkdownEncoder, no re-escape) unchanged, so the
// entity reference is dropped on round-trip just like Java does.
// The skeleton stream gets a single Ref so the writer substitutes the
// translation in place.
func (r *Reader) emitHTMLSubfilterTextBlock(ctx context.Context, ch chan<- model.PartResult, text []byte) {
	r.blockCounter++
	blockID := fmt.Sprintf("tu%d", r.blockCounter)
	decoded := xhtml.UnescapeString(string(text))
	block := model.NewBlock(blockID, decoded)
	block.Name = fmt.Sprintf("html%d", r.blockCounter)
	block.Type = "html-text"

	r.skelRef(blockID)
	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// emitMathFormulaBlock emits the verbatim MathML body of an HTML-block
// <math> element as a non-translatable RoleFormula content block: visible
// to ingestion, skipped by MT. The body is a single verbatim run (no
// inline parse, whitespace preserved) and rides the skeleton via a ref so
// the <math>…</math> delimiters round-trip byte-exact.
func (r *Reader) emitMathFormulaBlock(ctx context.Context, ch chan<- model.PartResult, body string) {
	r.blockCounter++
	blockID := fmt.Sprintf("tu%d", r.blockCounter)
	block := model.NewBlock(blockID, body)
	block.Name = fmt.Sprintf("math%d", r.blockCounter)
	block.Type = "math"
	block.Translatable = false
	block.PreserveWhitespace = true
	block.SetSemanticRole(model.RoleFormula, 0)
	r.skelRef(blockID)
	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// emitHTMLSubfilterTag writes a start/self-closing tag's raw bytes to
// skeleton, splitting around any translatable attribute values to emit
// Block refs in their place. Attribute names that aren't translatable
// for the given tag pass through verbatim.
func (r *Reader) emitHTMLSubfilterTag(ctx context.Context, ch chan<- model.PartResult, tag string, raw []byte) {
	transAttrs := htmlTranslatableAttrs[tag]
	if len(transAttrs) == 0 {
		r.skelText(string(raw))
		return
	}
	// Find each attribute value's position inside `raw` and split around
	// translatable ones. We re-scan the raw tag bytes because the html
	// Tokenizer's TagAttr returns decoded values without offsets.
	cursor := 0
	for _, m := range htmlAttrRE.FindAllSubmatchIndex(raw, -1) {
		// Group 1: attribute name; group 3: double-quoted value;
		// group 4: single-quoted value; group 5: unquoted value.
		nameStart, nameEnd := m[2], m[3]
		name := strings.ToLower(string(raw[nameStart:nameEnd]))
		if !transAttrs[name] {
			continue
		}
		var valStart, valEnd int
		switch {
		case m[6] >= 0:
			valStart, valEnd = m[6], m[7]
		case m[8] >= 0:
			valStart, valEnd = m[8], m[9]
		case m[10] >= 0:
			valStart, valEnd = m[10], m[11]
		default:
			continue
		}
		if valStart <= valEnd && valStart < len(raw) {
			value := raw[valStart:valEnd]
			if !hasNonWhitespace(value) {
				continue
			}
			// Emit raw bytes up to the value start (includes attr name +
			// `=` + opening quote).
			if valStart > cursor {
				r.skelText(string(raw[cursor:valStart]))
			}
			r.blockCounter++
			blockID := fmt.Sprintf("tu%d", r.blockCounter)
			block := model.NewBlock(blockID, string(value))
			block.Name = fmt.Sprintf("html-attr%d", r.blockCounter)
			block.Type = "html-attr"
			block.Properties["attr"] = name
			r.skelRef(blockID)
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
			cursor = valEnd
		}
	}
	if cursor < len(raw) {
		r.skelText(string(raw[cursor:]))
	}
}

// htmlAttrRE matches a single HTML attribute inside a tag. Group 1 is
// the attribute name; one of groups 4/5/6 holds the value (double,
// single, or unquoted). The pattern intentionally tolerates messy real-
// world attributes including bare names and unquoted values. It does
// NOT match across `>` because the tokenizer always feeds a complete
// tag (including any quoted-string contents).
var htmlAttrRE = regexp.MustCompile(`([a-zA-Z_:][-a-zA-Z0-9_:.]*)(\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+)))?`)

// hasNonWhitespace reports whether b contains any byte that is not a
// space, tab, CR, or LF. Used to decide whether a TextToken or
// attribute value warrants extraction (pure whitespace stays skeleton).
func hasNonWhitespace(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n':
		default:
			return true
		}
	}
	return false
}

// splitCDATA recognises a `<![CDATA[...]]>` token and returns its
// opening (`<![CDATA[`), payload, and closing (`]]>`) byte slices.
// Used by the HTML-block subfilter to route only the CDATA payload
// through the translation channel — mirrors okapi MarkdownFilter's
// HTML_CDATA_PAT handling (MarkdownFilter.java line 296).
func splitCDATA(raw []byte) (open, body, close []byte, ok bool) {
	const startMarker = "<![CDATA["
	const endMarker = "]]>"
	if !bytes.HasPrefix(raw, []byte(startMarker)) || !bytes.HasSuffix(raw, []byte(endMarker)) {
		return nil, nil, nil, false
	}
	return raw[:len(startMarker)], raw[len(startMarker) : len(raw)-len(endMarker)], raw[len(raw)-len(endMarker):], true
}

func (r *Reader) emitThematicBreak(ctx context.Context, ch chan<- model.PartResult, n *ast.ThematicBreak, source []byte, baseOffset int) {
	r.dataCounter++
	data := &model.Data{
		ID:   fmt.Sprintf("d%d", r.dataCounter),
		Name: "thematic-break",
	}
	// ThematicBreak has no Lines(). The break text (e.g. "---\n") is in the
	// gap between the previous and next nodes. Since thematic break is Data
	// (non-translatable), we don't need a ref — skelEmitGap from the next
	// node will capture this gap as skeleton text.
	r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
}

// emitBlockquoteAsContent surfaces a non-translatable blockquote
// (TranslateBlockQuotes off) as contextual content rather than an opaque
// Data part: it walks the blockquote's children exactly like the
// translatable path — one Block per paragraph/list-item, the `>` marker
// and continuation prefixes preserved in skeleton — but every emitted
// Block is demoted to Translatable:false (visible to ingestion, skipped
// by MT). Gated by the caller on ExtractNonTranslatableContent so the
// flag-off path keeps emitting the byte-identical Data part.
func (r *Reader) emitBlockquoteAsContent(ctx context.Context, ch chan<- model.PartResult, node ast.Node, source []byte, baseOffset int) {
	r.nonTranslatableDepth++
	r.walkNode(ctx, ch, node, source, baseOffset)
	r.nonTranslatableDepth--
}

func (r *Reader) emitBlockquoteAsData(ctx context.Context, ch chan<- model.PartResult, n *ast.Blockquote, source []byte, baseOffset int) {
	r.dataCounter++
	absStart, absEnd := nodeAbsRange(n, source, baseOffset)
	// Scan backward to include the > prefix
	for absStart > 0 && r.source[absStart-1] != '\n' {
		absStart--
	}
	rawContent := string(r.source[absStart:absEnd])
	data := &model.Data{
		ID:   fmt.Sprintf("d%d", r.dataCounter),
		Name: "blockquote",
		Properties: map[string]string{
			"content": rawContent,
		},
	}
	r.skelEmitGap(absStart)
	r.skelText(rawContent)
	r.skelCursor = absEnd
	r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
}

func (r *Reader) emitTable(ctx context.Context, ch chan<- model.PartResult, node ast.Node, source []byte, baseOffset int) {
	// Build each row as `| cell | cell |\n` with single-space padding
	// around each cell. Mirrors upstream MarkdownParser.java:770-806 —
	// the TableCell visitor unconditionally emits `"| "` before the
	// cell content and `" "` after, regardless of how the source
	// padded it (so `| data   |   foo |` collapses to `| data | foo |`).
	// The header separator row is preserved verbatim from source
	// because okapi's TableSeparator visitor emits the node's text
	// verbatim (line 808-820 in MarkdownParser.java).
	absStart, absEnd := nodeAbsRange(node, source, baseOffset)
	tableLineStart := absStart
	for tableLineStart > 0 && r.source[tableLineStart-1] != '\n' {
		tableLineStart--
	}
	r.skelEmitGap(tableLineStart)
	// Walk forward from absEnd through the trailing pipe(s) and final
	// newline of the last row so the cursor lands at the start of the
	// next block instead of leaving stray ` |\n` bytes for the next gap.
	cursorEnd := absEnd
	for cursorEnd < len(r.source) && r.source[cursorEnd] != '\n' {
		cursorEnd++
	}
	if cursorEnd < len(r.source) && r.source[cursorEnd] == '\n' {
		cursorEnd++
	}
	r.skelCursor = cursorEnd

	// Find the source separator line (`| --- | --- |`) so we can emit
	// it verbatim — okapi's TableSeparator visitor preserves the
	// separator's exact source bytes (alignment colons, dash count)
	// rather than synthesizing it. Goldmark doesn't expose the
	// separator as an AST child, so locate it from source by scanning
	// the line that immediately follows the header row.
	separatorLine := r.tableSeparatorLine(node, baseOffset)

	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		if row.Kind() != east.KindTableHeader && row.Kind() != east.KindTableRow {
			continue
		}
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			if cell.Kind() != east.KindTableCell {
				continue
			}
			r.skelText("| ")
			cellText := r.extractInlineText(cell, source)
			if strings.TrimSpace(cellText) == "" {
				// Empty cell: emit just a space to give an "| |" pair.
				r.skelText(" ")
				continue
			}
			r.blockCounter++
			blockID := fmt.Sprintf("tu%d", r.blockCounter)
			block := model.NewBlock(blockID, cellText)
			block.Name = fmt.Sprintf("cell%d", r.blockCounter)
			block.Type = "table-cell"
			r.addInlineRuns(block, cell, source)
			r.skelRef(blockID)
			r.skelText(" ")
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
		r.skelText("|\n")
		// Inject the separator immediately after the header row.
		if row.Kind() == east.KindTableHeader && separatorLine != "" {
			r.skelText(separatorLine)
		}
	}
}

// tableSeparatorLine returns the source line that holds the GFM table
// separator (e.g. "| --- | --- |\n") sandwiched between the header
// row and the first body row. Returns "" when the line can't be
// located (defensive — well-formed GFM tables always have one). Uses
// r.source (the full document) plus baseOffset because nodeAbsRange's
// "absolute" positions index into r.source, not into the body slice
// goldmark sees (which strips any leading YAML front matter).
func (r *Reader) tableSeparatorLine(table ast.Node, baseOffset int) string {
	header := table.FirstChild()
	if header == nil || header.Kind() != east.KindTableHeader {
		return ""
	}
	_, headerEnd := nodeAbsRange(header, r.source[baseOffset:], baseOffset)
	i := headerEnd
	for i < len(r.source) && r.source[i] != '\n' {
		i++
	}
	if i >= len(r.source) {
		return ""
	}
	i++ // skip header row's newline
	start := i
	for i < len(r.source) && r.source[i] != '\n' {
		i++
	}
	if i >= len(r.source) {
		return ""
	}
	i++ // include the trailing newline
	return string(r.source[start:i])
}

// --- Skeleton store helpers ---

func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil {
		r.skelBuf.WriteString(s)
	}
}

func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

// --- Inline text extraction ---

func (r *Reader) extractInlineText(node ast.Node, source []byte) string {
	var buf strings.Builder
	r.collectInlineText(&buf, node, source)
	return buf.String()
}

func (r *Reader) collectInlineText(buf *strings.Builder, node ast.Node, source []byte) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			buf.Write(n.Segment.Value(source))
			if n.SoftLineBreak() {
				buf.WriteString(softBreakContinuation(source, n.Segment.Stop))
			}
			if n.HardLineBreak() {
				buf.WriteByte('\n')
			}
		case *ast.String:
			buf.Write(n.Value)
		case *ast.CodeSpan:
			for gc := n.FirstChild(); gc != nil; gc = gc.NextSibling() {
				if t, ok := gc.(*ast.Text); ok {
					buf.Write(t.Segment.Value(source))
				}
			}
		case *ast.Image:
			if r.cfg.TranslateImageAlt() {
				r.collectInlineText(buf, child, source)
			}
		case *ast.AutoLink:
			buf.Write(n.URL(source))
		default:
			r.collectInlineText(buf, child, source)
		}
	}
}

func (r *Reader) extractListItemText(item *ast.ListItem, source []byte) string {
	var buf strings.Builder
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			r.collectInlineText(&buf, child, source)
		case *ast.Text:
			buf.Write(n.Segment.Value(source))
		default:
			r.collectInlineText(&buf, child, source)
		}
	}
	return buf.String()
}

func (r *Reader) extractRawLines(node ast.Node, source []byte) string {
	var buf strings.Builder
	lines := node.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}
	return buf.String()
}

// extractRawLinesSkipBlanks is the same as extractRawLines but skips
// any line that, after stripping leading whitespace and the trailing
// `\n`, is empty. Mirrors upstream MarkdownParser.java:417's
// NEWLINE_ONLY_PATTERN check inside the FencedCodeBlock visitor.
func extractRawLinesSkipBlanks(node ast.Node, source []byte) string {
	var buf strings.Builder
	lines := node.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		v := line.Value(source)
		if isBlankLine(v) {
			continue
		}
		buf.Write(v)
	}
	return buf.String()
}

func isBlankLine(line []byte) bool {
	for _, c := range line {
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	return true
}

// --- Inline run building ---

func (r *Reader) addInlineRuns(block *model.Block, node ast.Node, source []byte) {
	b := newRunBuilder()
	idCounter := 0
	r.buildCodedRuns(b, node, source, &idCounter)
	if b.HasInlineCodes() {
		block.Source = b.Runs()
	}
}

func (r *Reader) buildCodedRuns(b *runBuilder, node ast.Node, source []byte, idCounter *int) {
	// excludeStack tracks open RawHTML tags whose content is non-
	// translatable (math, script, style — see htmlExcludedElements).
	// While the stack is non-empty, every sibling node — Text, RawHTML,
	// even nested elements — is folded into one accumulating opaque
	// PlaceholderRun so the original bytes round-trip verbatim through
	// pseudo-translation. Mirrors okapi's HTML-subfilter EXCLUDE
	// handling for inline RawHTML segments inside Markdown paragraphs.
	var excludeStack []string
	var excludeBuf strings.Builder
	flushExclude := func() {
		if excludeBuf.Len() == 0 {
			return
		}
		*idCounter++
		id := strconv.Itoa(*idCounter)
		data := excludeBuf.String()
		b.AddPcOpen(id, "fmt:html", "md:html-inline", data, "", "", false, false, false)
		b.AddPcClose(id, "fmt:html", "md:html-inline", "", "")
		excludeBuf.Reset()
	}

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		// While inside an excluded RawHTML element, accumulate every
		// child's source bytes into the opaque placeholder.
		if len(excludeStack) > 0 {
			r.appendNodeRawBytes(&excludeBuf, child, source)
			if rh, ok := child.(*ast.RawHTML); ok {
				switch rawHTMLTagKind(rh, source) {
				case rawHTMLOpenExcluded:
					excludeStack = append(excludeStack, "")
				case rawHTMLClose:
					excludeStack = excludeStack[:len(excludeStack)-1]
					if len(excludeStack) == 0 {
						flushExclude()
					}
				}
			}
			continue
		}
		switch n := child.(type) {
		case *ast.Text:
			addTextWithEntities(b, string(n.Segment.Value(source)), idCounter)
			if n.SoftLineBreak() {
				b.AddText(softBreakContinuation(source, n.Segment.Stop))
			}
			if n.HardLineBreak() {
				b.AddText("\n")
			}
		case *ast.String:
			addTextWithEntities(b, string(n.Value), idCounter)

		case *ast.Emphasis:
			r.buildEmphasisRuns(b, n, source, idCounter)

		case *ast.CodeSpan:
			r.buildCodeSpanRuns(b, n, source, idCounter)

		case *ast.Link:
			r.buildLinkRuns(b, n, source, idCounter)

		case *ast.Image:
			r.buildImageRuns(b, n, source, idCounter)

		case *ast.AutoLink:
			r.buildAutoLinkRuns(b, n, source, idCounter)

		case *ast.RawHTML:
			if rawHTMLTagKind(n, source) == rawHTMLOpenExcluded {
				r.appendNodeRawBytes(&excludeBuf, n, source)
				excludeStack = append(excludeStack, "")
				continue
			}
			r.buildRawHTMLRuns(b, n, source, idCounter)

		default:
			if child.Kind() == east.KindStrikethrough {
				r.buildStrikethroughRuns(b, child, source, idCounter)
			} else {
				r.buildCodedRuns(b, child, source, idCounter)
			}
		}
	}
	flushExclude()
}

type rawHTMLKind int

const (
	rawHTMLOther rawHTMLKind = iota
	rawHTMLOpenExcluded
	rawHTMLClose
)

// rawHTMLTagKind classifies a goldmark RawHTML inline node so the
// paragraph builder can detect openings and closings of excluded
// elements (math, script, style). Self-closing tags and non-tag
// fragments (comments, processing instructions) report rawHTMLOther.
func rawHTMLTagKind(n *ast.RawHTML, source []byte) rawHTMLKind {
	var raw bytes.Buffer
	for i := range n.Segments.Len() {
		seg := n.Segments.At(i)
		raw.Write(seg.Value(source))
	}
	s := raw.Bytes()
	if len(s) < 2 || s[0] != '<' {
		return rawHTMLOther
	}
	closing := false
	idx := 1
	if s[1] == '/' {
		closing = true
		idx = 2
	}
	end := idx
	for end < len(s) {
		c := s[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == ':' {
			end++
			continue
		}
		break
	}
	tag := strings.ToLower(string(s[idx:end]))
	if tag == "" {
		return rawHTMLOther
	}
	if closing {
		if htmlExcludedElements[tag] {
			return rawHTMLClose
		}
		return rawHTMLOther
	}
	// Self-closing tags don't open a region.
	if bytes.HasSuffix(bytes.TrimSpace(s), []byte("/>")) {
		return rawHTMLOther
	}
	if htmlExcludedElements[tag] {
		return rawHTMLOpenExcluded
	}
	return rawHTMLOther
}

// appendNodeRawBytes writes a node's source bytes to dst. For Text and
// RawHTML nodes we use the segment ranges; everything else falls back
// to the node's Lines() span (paragraph descendants typically expose
// the same ranges via Lines() at the leaf level).
func (r *Reader) appendNodeRawBytes(dst *strings.Builder, n ast.Node, source []byte) {
	switch v := n.(type) {
	case *ast.Text:
		dst.Write(v.Segment.Value(source))
		if v.SoftLineBreak() {
			dst.WriteString(softBreakContinuation(source, v.Segment.Stop))
		}
		if v.HardLineBreak() {
			dst.WriteByte('\n')
		}
	case *ast.RawHTML:
		for i := range v.Segments.Len() {
			seg := v.Segments.At(i)
			dst.Write(seg.Value(source))
		}
	default:
		// Best-effort: use Lines() ranges so nested inline nodes
		// (Emphasis, Link, Image with text inside math) still
		// contribute their source bytes verbatim. Excluded markup
		// doesn't typically nest non-RawHTML children, but this guards
		// against malformed input.
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			dst.Write(seg.Value(source))
		}
	}
}

func (r *Reader) buildEmphasisRuns(b *runBuilder, n *ast.Emphasis, source []byte, idCounter *int) {
	delim := emphasisDelimiter(n, source)
	var semType, subType, data string
	if n.Level == 2 {
		semType = "fmt:bold"
		subType = "md:strong"
		data = strings.Repeat(string(delim), 2)
	} else {
		semType = "fmt:italic"
		subType = "md:emphasis"
		data = string(delim)
	}
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback(semType)
	b.AddPcOpen(id, semType, subType, data, info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	r.buildCodedRuns(b, n, source, idCounter)
	b.AddPcClose(id, semType, subType, data, info.Equiv)
}

// emphasisDelimiter returns the byte ('*' or '_') used as the
// emphasis marker in the source. goldmark's ast.Emphasis only carries
// the delimiter level (1 or 2), not which character was used, so we
// look at the source bytes immediately before the first child node.
// Defaults to '*' when the offset can't be located (e.g. nested or
// programmatically-built nodes).
func emphasisDelimiter(n *ast.Emphasis, source []byte) byte {
	first := n.FirstChild()
	if first == nil {
		return '*'
	}
	// Inline text nodes carry their source range via Segment.
	if t, ok := first.(*ast.Text); ok {
		start := t.Segment.Start - n.Level
		if start >= 0 && start < len(source) {
			c := source[start]
			if c == '*' || c == '_' {
				return c
			}
		}
	}
	return '*'
}

func (r *Reader) buildCodeSpanRuns(b *runBuilder, n *ast.CodeSpan, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback("fmt:code")
	// CommonMark §6.1: a code span's opening and closing fences are a
	// run of backticks; the length must avoid any backtick run in the
	// content. The fence is also followed by an optional padding space
	// when the content begins/ends with a backtick. Reconstruct the
	// exact fence + padding from source so `` ` `` (literal backtick
	// surrounded by 2-backtick fences with single padding space)
	// round-trips intact instead of collapsing to ``` (3 backticks).
	openMarker, closeMarker := codeSpanFences(n, source)
	b.AddPcOpen(id, "fmt:code", "md:code", openMarker, info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	for gc := n.FirstChild(); gc != nil; gc = gc.NextSibling() {
		if t, ok := gc.(*ast.Text); ok {
			b.AddText(string(t.Segment.Value(source)))
		}
	}
	b.AddPcClose(id, "fmt:code", "md:code", closeMarker, info.Equiv)
}

// codeSpanFences returns the opening and closing markers (each a run
// of “ ` “ plus an optional padding space) for the given CodeSpan as
// they appear in source. Falls back to "`" / "`" when boundaries
// can't be determined (defensive — should not happen for a well-formed
// CodeSpan).
func codeSpanFences(n *ast.CodeSpan, source []byte) (string, string) {
	first := n.FirstChild()
	last := n.LastChild()
	if first == nil || last == nil {
		return "`", "`"
	}
	firstText, ok1 := first.(*ast.Text)
	lastText, ok2 := last.(*ast.Text)
	if !ok1 || !ok2 {
		return "`", "`"
	}
	contentStart := firstText.Segment.Start
	contentEnd := lastText.Segment.Stop
	if contentStart >= len(source) || contentEnd > len(source) || contentStart > contentEnd {
		return "`", "`"
	}
	// Walk back from contentStart to capture optional padding space + backticks.
	openEnd := contentStart
	openStart := contentStart
	if openStart > 0 && source[openStart-1] == ' ' {
		openStart--
	}
	for openStart > 0 && source[openStart-1] == '`' {
		openStart--
	}
	// If we backed up over a space but found no backticks, undo the space.
	if openStart < openEnd && (openEnd-openStart == 1 && source[openStart] == ' ') {
		openStart++
	}
	open := string(source[openStart:openEnd])
	// Walk forward from contentEnd to capture optional padding space + backticks.
	closeStart := contentEnd
	closeEnd := contentEnd
	if closeEnd < len(source) && source[closeEnd] == ' ' {
		closeEnd++
	}
	for closeEnd < len(source) && source[closeEnd] == '`' {
		closeEnd++
	}
	if closeEnd > closeStart && (closeEnd-closeStart == 1 && source[closeStart] == ' ') {
		closeEnd--
	}
	close := string(source[closeStart:closeEnd])
	if open == "" {
		open = "`"
	}
	if close == "" {
		close = "`"
	}
	return open, close
}

func (r *Reader) buildLinkRuns(b *runBuilder, n *ast.Link, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback("link:hyperlink")

	// Reference-style links (`[text]`, `[text][]`, `[text][label]`)
	// must round-trip in their reference form. Goldmark resolves the
	// destination eagerly so the AST always carries the URL, but we
	// detect the reference form via n.Reference and rebuild the closing
	// marker as `]`, `][]`, or `][label]` accordingly. Mirrors okapi
	// MarkdownParser.visitRefLink which emits the LINK_REF tokens
	// verbatim from the source markers.
	if n.Reference != nil {
		closing := referenceCloseMarker(n.Reference)
		b.AddPcOpen(id, "link:hyperlink", "md:link-ref", "[", info.Display.Open, info.Equiv,
			info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
		r.buildCodedRuns(b, n, source, idCounter)
		b.AddPcClose(id, "link:hyperlink", "md:link-ref", closing, info.Equiv)
		return
	}

	destLiteral := linkDestinationLiteral(n.Destination, source)

	b.AddPcOpen(id, "link:hyperlink", "md:link", "[", info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	r.buildCodedRuns(b, n, source, idCounter)

	// When the link has a title, split the closing marker so the title
	// becomes a translatable text run between two paired codes:
	//   pc-open `[` → link text → pc-close `]` →
	//   pc-open `](url "` → title text → pc-close `")`
	// This mirrors okapi's MarkdownFilter behaviour, which extracts the
	// link/image title as a translatable string. Without the split the
	// title would round-trip untranslated as part of the closing skeleton.
	if len(n.Title) > 0 {
		b.AddPcClose(id, "link:hyperlink", "md:link", "]", info.Equiv)
		*idCounter++
		titleID := strconv.Itoa(*idCounter)
		b.AddPcOpen(titleID, "link:hyperlink", "md:link-title",
			"("+destLiteral+` "`, "", "", false, false, false)
		b.AddText(string(n.Title))
		b.AddPcClose(titleID, "link:hyperlink", "md:link-title", `")`, "")
		return
	}

	b.AddPcClose(id, "link:hyperlink", "md:link", "]("+destLiteral+")", info.Equiv)
}

func (r *Reader) buildImageRuns(b *runBuilder, n *ast.Image, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback("link:image")

	// Reference-style images (`![alt]`, `![alt][]`, `![alt][label]`)
	// follow the same preserve-reference-form rule as buildLinkRuns: the
	// closing marker becomes `]`, `][]`, or `][label]` instead of the
	// resolved `](url)`. Note that for `![][label]` (empty alt) we still
	// produce an empty pc-pair around no inline content, matching the
	// shape okapi MarkdownParser uses for IMAGE_REF nodes whose text is
	// undefined.
	if n.Reference != nil {
		closing := referenceCloseMarker(n.Reference)
		b.AddPcOpen(id, "link:image", "md:image-ref", "![", info.Display.Open, info.Equiv,
			info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
		if r.cfg.TranslateImageAlt() {
			r.buildCodedRuns(b, n, source, idCounter)
		}
		b.AddPcClose(id, "link:image", "md:image-ref", closing, info.Equiv)
		return
	}

	destLiteral := linkDestinationLiteral(n.Destination, source)

	b.AddPcOpen(id, "link:image", "md:image", "![", info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	if r.cfg.TranslateImageAlt() {
		r.buildCodedRuns(b, n, source, idCounter)
	}

	// Same title-splitting trick as buildLinkRuns above so image titles
	// are extracted as translatable text rather than baked into the
	// closing-data skeleton. See buildLinkRuns for the rationale.
	if len(n.Title) > 0 {
		b.AddPcClose(id, "link:image", "md:image", "]", info.Equiv)
		*idCounter++
		titleID := strconv.Itoa(*idCounter)
		b.AddPcOpen(titleID, "link:image", "md:image-title",
			"("+destLiteral+` "`, "", "", false, false, false)
		b.AddText(string(n.Title))
		b.AddPcClose(titleID, "link:image", "md:image-title", `")`, "")
		return
	}

	b.AddPcClose(id, "link:image", "md:image", "]("+destLiteral+")", info.Equiv)
}

// referenceCloseMarker returns the closing-marker bytes for a
// reference-style link or image, given the resolved Reference info from
// goldmark's AST. The three CommonMark forms map as follows:
//
//	[text]            (Shortcut)  → "]"
//	[text][]          (Collapsed) → "][]"
//	[text][label]     (Full)      → "][label]"
//
// The label preserves the source casing exactly via Reference.Value, so
// the round-trip output matches the original form even when the link
// text was case-folded by translation.
func referenceCloseMarker(ref *ast.ReferenceLink) string {
	switch ref.Type {
	case ast.ReferenceLinkShortcut:
		return "]"
	case ast.ReferenceLinkCollapsed:
		return "][]"
	default: // ReferenceLinkFull
		return "][" + string(ref.Value) + "]"
	}
}

// linkDestinationLiteral returns the destination URL as it appeared in
// the source: bare (`http://example.com`) or wrapped in angle brackets
// (`<http://example.com>`). goldmark's ast.Link/Image carries only the
// resolved Destination string; we peek at the source bytes for the
// inline-link form so round-trips preserve angle-bracket-wrapped URLs
// (e.g. `[Link](<https://...> "title")`).
func linkDestinationLiteral(dest []byte, source []byte) string {
	d := string(dest)
	if d == "" {
		// Empty destination keeps its angle-wrapped form when the
		// source authored `](<>)` — the bare `]()` form is a separate
		// syntactic choice the parser collapses to the same empty
		// string. Preserving the source's choice keeps round-trip
		// faithfulness for placeholder/anchor links.
		if bytes.Contains(source, []byte("](<>)")) {
			return "<>"
		}
		return d
	}
	// Match the destination only in inline-link context: the `](<dest`
	// substring uniquely belongs to the inline `[text](<url>)` form.
	// Searching for the bare `<dest>` would also match autolinks
	// (`<http://...>`) elsewhere in the document, which would wrongly
	// upgrade a bare-form inline link to angle-wrapped just because the
	// same URL appears somewhere as an autolink.
	if bytes.Contains(source, []byte("](<"+d+">")) {
		return "<" + d + ">"
	}
	return d
}

func (r *Reader) buildAutoLinkRuns(b *runBuilder, n *ast.AutoLink, source []byte, idCounter *int) {
	url := string(n.URL(source))
	// Standard CommonMark `<url>` autolinks are opaque non-translatable
	// inline codes — okapi MarkdownParser emits AUTO_LINK / MAIL_LINK
	// tokens with translatable=false, so the `<url>` travels through
	// pseudo translation as verbatim bytes. Goldmark's linkify
	// extension also surfaces bare-URL matches (e.g. `https://...`
	// without angle brackets) as AutoLink nodes, but okapi (which uses
	// flexmark's matching set of extensions) treats those bare URLs as
	// plain TEXT and DOES pseudo-translate them. Mirror that split:
	//   - source had `<url>` → opaque placeholder
	//   - source had bare URL (linkify) → plain text
	if hasAngleBrackets(n, source) {
		*idCounter++
		id := strconv.Itoa(*idCounter)
		info := r.vocab.LookupOrFallback("link:hyperlink")
		b.AddPh(id, "link:hyperlink", "md:autolink", "<"+url+">",
			info.Display.Open, info.Equiv,
			info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
		return
	}
	b.AddText(url)
}

// hasAngleBrackets reports whether the AutoLink's source representation
// includes the surrounding `<...>` markers (standard CommonMark §6.5)
// rather than the bare URL form recognised by goldmark's linkify
// extension. Goldmark stores the AutoLink's URL segment in a private
// `value *Text` field with no public accessor and doesn't add it as a
// child node, so we infer the source position from the previous
// sibling's text segment: the AutoLink starts immediately after that
// segment's end, and a `<` byte there means the standard form.
func hasAngleBrackets(n *ast.AutoLink, source []byte) bool {
	prev := n.PreviousSibling()
	if prev == nil {
		return false
	}
	t, ok := prev.(*ast.Text)
	if !ok {
		return false
	}
	pos := t.Segment.Stop
	return pos < len(source) && source[pos] == '<'
}

func (r *Reader) buildRawHTMLRuns(b *runBuilder, n *ast.RawHTML, source []byte, idCounter *int) {
	var htmlContent strings.Builder
	for i := range n.Segments.Len() {
		seg := n.Segments.At(i)
		htmlContent.Write(seg.Value(source))
	}
	tag := htmlContent.String()

	// Mirrors okapi HTML-subfilter ATTRIBUTE_TRANS handling for inline
	// RawHTML tags inside Markdown paragraphs: when a tag carries a
	// translatable attribute (img alt, a title, …), split the raw bytes
	// around the value so the value lands in the runs as a TextRun while
	// the surrounding tag bytes stay opaque PlaceholderRun data.
	if extracted := r.emitInlineHTMLWithAttrs(b, tag, idCounter); extracted {
		return
	}

	// `<![CDATA[...]]>` payload is translatable text per okapi
	// MarkdownFilter HTML_CDATA_PAT — split the markers off as opaque
	// codes around the inner text run.
	if open, body, close, ok := splitCDATA([]byte(tag)); ok && hasNonWhitespace(body) {
		*idCounter++
		id := strconv.Itoa(*idCounter)
		b.AddPcOpen(id, "fmt:html", "md:html-cdata", string(open), "", "", false, false, false)
		b.AddText(string(body))
		b.AddPcClose(id, "fmt:html", "md:html-cdata", string(close), "")
		return
	}

	// Raw inline HTML has no vocabulary entry, so emit with empty
	// display/equiv and zero-valued (all-false) RunConstraints.
	*idCounter++
	id := strconv.Itoa(*idCounter)
	b.AddPcOpen(id, "fmt:html", "md:html-inline", tag, "", "", false, false, false)
	b.AddPcClose(id, "fmt:html", "md:html-inline", "", "")
}

// emitInlineHTMLWithAttrs detects translatable attributes on an inline
// HTML tag and splits the raw bytes so attribute values become inline
// TextRuns surrounded by opaque PlaceholderRuns. Returns false for
// closing tags, comments, processing instructions, or tags that carry
// no translatable attributes — in those cases the caller falls back to
// emitting the whole tag as one opaque placeholder.
func (r *Reader) emitInlineHTMLWithAttrs(b *runBuilder, tag string, idCounter *int) bool {
	raw := []byte(tag)
	if len(raw) < 2 || raw[0] != '<' || raw[1] == '/' || raw[1] == '!' || raw[1] == '?' {
		return false
	}
	// Locate tag name to look up its translatable-attribute set.
	idx := 1
	end := idx
	for end < len(raw) {
		c := raw[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == ':' {
			end++
			continue
		}
		break
	}
	tagName := strings.ToLower(string(raw[idx:end]))
	transAttrs := htmlTranslatableAttrs[tagName]
	if len(transAttrs) == 0 {
		return false
	}

	type valueRange struct {
		start, end int
	}
	var values []valueRange
	for _, m := range htmlAttrRE.FindAllSubmatchIndex(raw, -1) {
		nameStart, nameEnd := m[2], m[3]
		if nameStart < end {
			continue // skip the tag name itself
		}
		name := strings.ToLower(string(raw[nameStart:nameEnd]))
		if !transAttrs[name] {
			continue
		}
		var vs, ve int
		switch {
		case m[6] >= 0:
			vs, ve = m[6], m[7]
		case m[8] >= 0:
			vs, ve = m[8], m[9]
		case m[10] >= 0:
			vs, ve = m[10], m[11]
		default:
			continue
		}
		if vs < 0 || vs >= ve || !hasNonWhitespace(raw[vs:ve]) {
			continue
		}
		values = append(values, valueRange{vs, ve})
	}
	if len(values) == 0 {
		return false
	}
	// Emit a sequence of PlaceholderRun(opaque-bytes) + TextRun(value)
	// segments. The closing PlaceholderRun matches the last open id so the
	// runs form a balanced open/close pair around the value sequence.
	cursor := 0
	*idCounter++
	id := strconv.Itoa(*idCounter)
	for i, v := range values {
		preData := string(raw[cursor:v.start])
		// Open code carrying the bytes leading up to the value (or
		// between values). The first iteration's data includes the tag
		// name + leading attrs; subsequent iterations carry the bytes
		// between attributes.
		if i == 0 {
			b.AddPcOpen(id, "fmt:html", "md:html-inline", preData, "", "", false, false, false)
		} else {
			*idCounter++
			placeholderID := strconv.Itoa(*idCounter)
			b.AddPh(placeholderID, "code:html", "md:html-inline-bytes", preData, "", "", false, false, false)
		}
		b.AddText(string(raw[v.start:v.end]))
		cursor = v.end
	}
	tail := string(raw[cursor:])
	b.AddPcClose(id, "fmt:html", "md:html-inline", tail, "")
	return true
}

func (r *Reader) buildStrikethroughRuns(b *runBuilder, node ast.Node, source []byte, idCounter *int) {
	*idCounter++
	id := strconv.Itoa(*idCounter)
	info := r.vocab.LookupOrFallback("fmt:strike")
	// Goldmark's GFM strikethrough accepts both `~` (subscript-style)
	// and `~~` (standard) delimiters. Detect the actual marker length
	// from source so `H~2~O` (single tildes) round-trips intact rather
	// than getting normalized to `H~~2~~O`. Mirrors okapi
	// MarkdownParser which preserves the source delimiter verbatim.
	openMarker, closeMarker := strikethroughFences(node, source)
	b.AddPcOpen(id, "fmt:strike", "md:strikethrough", openMarker, info.Display.Open, info.Equiv,
		info.Constraints.Deletable, info.Constraints.Cloneable, info.Constraints.Reorderable)
	r.buildCodedRuns(b, node, source, idCounter)
	b.AddPcClose(id, "fmt:strike", "md:strikethrough", closeMarker, info.Equiv)
}

// strikethroughFences returns the source-literal opening/closing
// `~` runs that delimit the strikethrough node. Falls back to `~~` /
// `~~` when boundaries can't be determined.
func strikethroughFences(node ast.Node, source []byte) (string, string) {
	first := node.FirstChild()
	last := node.LastChild()
	if first == nil || last == nil {
		return "~~", "~~"
	}
	firstText, ok1 := first.(*ast.Text)
	lastText, ok2 := last.(*ast.Text)
	if !ok1 || !ok2 {
		return "~~", "~~"
	}
	contentStart := firstText.Segment.Start
	contentEnd := lastText.Segment.Stop
	if contentStart > len(source) || contentEnd > len(source) || contentStart > contentEnd {
		return "~~", "~~"
	}
	openStart := contentStart
	for openStart > 0 && source[openStart-1] == '~' {
		openStart--
	}
	closeEnd := contentEnd
	for closeEnd < len(source) && source[closeEnd] == '~' {
		closeEnd++
	}
	open := string(source[openStart:contentStart])
	close := string(source[contentEnd:closeEnd])
	if open == "" {
		open = "~~"
	}
	if close == "" {
		close = "~~"
	}
	return open, close
}

// --- Emit helper ---

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	// Within a non-translatable sub-tree (e.g. a blockquote surfaced as
	// contextual content), demote every Block to non-translatable. The
	// block still rides the skeleton via its ref, so round-trip is
	// unchanged — only the MT/translation contract flips.
	if part.Type == model.PartBlock && r.nonTranslatableDepth > 0 {
		if block, ok := part.Resource.(*model.Block); ok {
			block.Translatable = false
		}
	}
	// Apply inline code finder to blocks if enabled
	if part.Type == model.PartBlock && r.cfg.UseCodeFinder {
		if block, ok := part.Resource.(*model.Block); ok {
			r.applyCodeFinder(block)
		}
	}
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// applyCodeFinder applies code finder patterns to a block's fragments.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.CodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}

	if len(block.Source) == 0 {
		return
	}
	text := model.RunsText(block.Source)

	type matchRange struct {
		start, end int
	}
	var matches []matchRange
	for _, re := range patterns {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			matches = append(matches, matchRange{loc[0], loc[1]})
		}
	}
	if len(matches) == 0 {
		return
	}

	// Sort matches by start position
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	var newRuns []model.Run
	lastEnd := 0
	spanID := 1
	for _, m := range matches {
		if m.start > lastEnd {
			newRuns = append(newRuns, model.Run{Text: &model.TextRun{Text: text[lastEnd:m.start]}})
		}
		newRuns = append(newRuns, model.Run{Ph: &model.PlaceholderRun{
			ID:   fmt.Sprintf("c%d", spanID),
			Type: "code",
			Data: text[m.start:m.end],
		}})
		lastEnd = m.end
		spanID++
	}
	if lastEnd < len(text) {
		newRuns = append(newRuns, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
	}
	block.SetSourceRuns(newRuns)
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
