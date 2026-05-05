package wiki

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// MediaWiki header pattern: == Title == through ====== Title ======
var mediaWikiHeaderRe = regexp.MustCompile(`^(={2,6})\s*(.+?)\s*(={2,6})\s*$`)

// DokuWiki header pattern: same syntax as MediaWiki (= delimiters)
var dokuWikiHeaderRe = regexp.MustCompile(`^(={2,6})\s*(.+?)\s*(={2,6})\s*$`)

// DokuWiki inline placeholder patterns. These mirror Okapi's WikiPatterns
// (LINK_START, NAMED_LINK_*, IMAGE_*, plus the paired BOLD / ITALIC /
// UNDERLINE / MONOSPACE inline-formatting markers) so the matched
// constructs survive pseudo-translation as opaque inline codes rather
// than being mangled character-by-character. Order matters: NAMED_LINK_*
// must be tried before LINK to ensure `[[target|alt]]` becomes a paired
// (start, translatable text, end) sequence rather than being swallowed
// by the no-pipe placeholder regex.
//
// The patterns intentionally stay single-line (no `(?s)` flag): Okapi
// uses the same single-line semantics, so multi-line `{{...}}` /
// `[[...|...]]` constructs remain as regular text and continue to flow
// through paragraph extraction unchanged.
var (
	// `[[target|` — opening half of `[[target|alt text]]`. The alt text
	// between the opening and closing codes is translatable.
	dokuWikiNamedLinkStartRe = regexp.MustCompile(`\[\[[^|\]\r\n]+\|`)
	// `]]` — closing half of `[[target|alt]]`.
	dokuWikiNamedLinkEndRe = regexp.MustCompile(`\]\]`)
	// `[[target]]` — full placeholder (no pipe, content is not
	// translatable).
	dokuWikiLinkRe = regexp.MustCompile(`\[\[[^|\]\r\n]+\]\]`)
	// `{{...}}` — image / template placeholder. Single-line only,
	// matching Okapi's IMAGE_START.
	dokuWikiImageRe = regexp.MustCompile(`\{\{[^}\r\n]+\}\}`)

	// HTML-style paired inline tags recognised by Okapi WikiPatterns
	// (SUB, SUP, DEL, NOWIKI_TAG). Each opener is `<tag>` (case-insensitive
	// with optional attributes); each closer is `</tag>`. We capture the
	// tag literally so the writer round-trips opener / closer bytes
	// verbatim. The inner text remains translatable.
	dokuWikiHTMLOpenRe  = regexp.MustCompile(`(?i)<(sub|sup|del|nowiki)\b[^>]*>`)
	dokuWikiHTMLCloseRe = map[string]*regexp.Regexp{
		"sub":    regexp.MustCompile(`(?i)</sub>`),
		"sup":    regexp.MustCompile(`(?i)</sup>`),
		"del":    regexp.MustCompile(`(?i)</del>`),
		"nowiki": regexp.MustCompile(`(?i)</nowiki>`),
	}
)

// dokuWikiPaired lists symmetric paired inline markers. Each entry's
// `marker` is both the opening and closing token; we pair the first two
// non-overlapping occurrences on the same paragraph and emit
// PcOpen / TextRun / PcClose so the inner text stays translatable while
// the markers themselves survive pseudo-translation.
//
// Mirrors Okapi WikiPatterns BOLD (`**`), UNDERLINE (`__`), MONOSPACE
// (`”`), and ITALIC (`//`). Italic uses a slightly stricter Okapi
// regex (`(?<!:)//|//(?=\s|$)`) to avoid matching `http://` URLs; we
// approximate that by requiring at least one of the wrappers to NOT
// abut a colon — see splitDokuWikiInlineRuns.
var dokuWikiPaired = []struct {
	marker string
	codeID string // semantic Type used on the run (informational)
}{
	{marker: "**", codeID: "wiki:bold"},
	{marker: "__", codeID: "wiki:underline"},
	{marker: "''", codeID: "wiki:monospace"},
	{marker: "//", codeID: "wiki:italic"},
}

// MediaWiki table patterns
var mediaWikiTableStartRe = regexp.MustCompile(`^\{\|`)
var mediaWikiTableEndRe = regexp.MustCompile(`^\|\}`)
var mediaWikiTableRowRe = regexp.MustCompile(`^\|-`)
var mediaWikiTableCellRe = regexp.MustCompile(`^\|(.+)`)
var mediaWikiTableHeaderRe = regexp.MustCompile(`^!(.+)`)

// DokuWiki table row: ^ Header ^ or | Cell |. Both leading and
// trailing delimiter must be present.
var dokuWikiTableRe = regexp.MustCompile(`^[|^].*[|^]\s*$`)

// DokuWiki "open" table row: starts with `|` or `^` but no trailing
// delimiter. MediaWiki-flavoured fixtures (`{{Infobox\n|key=value\n}}`)
// route through Okapi's WikiFilter as TABLE_START_PATTERN
// (`^\^(?!_\^)|^\|`) blocks where each line becomes one cell. We mirror
// the resulting block shape (one block per `|`-prefixed line) so the
// parity round-trip preserves inter-line `\r\n` separators rather than
// collapsing them into a single space.
var dokuWikiOpenTableRowRe = regexp.MustCompile(`^[|^].`)

// DokuWiki code block: line starts with two-or-more spaces, where the
// first non-space character is not whitespace nor a list-item marker
// (`*` or `-`). Mirrors Okapi WikiPatterns CODE_START
// (`^ {2,}(?!\s|[\*-])`). Lines matching this are non-translatable
// preformatted code that flows verbatim through the skeleton — without
// honouring this, indented sample code in dokuwiki.wiki gets
// pseudo-translated and diverges from the okapi reference.
var dokuWikiCodeStartRe = regexp.MustCompile(`^ {2,}[^\s*-]`)

// MediaWiki image/file link: [[File:...|...|caption]] or [[Image:...|...|caption]]
var mediaWikiImageRe = regexp.MustCompile(`\[\[(?:File|Image):([^]|]+)((?:\|[^]|]*)*)?\]\]`)

// Reader implements DataFormatReader for Wiki files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new wiki reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "wiki",
			FormatDisplayName: "Wiki",
			FormatMimeType:    "text/x-wiki",
			FormatExtensions:  []string{".wiki", ".mediawiki"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/x-wiki"},
		Extensions: []string{".wiki", ".mediawiki"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("wiki: nil document or reader")
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

// rawLine holds a line's content and its original line ending.
type rawLine struct {
	content    string
	lineEnding string
}

// splitRawLines splits raw bytes into lines preserving line endings.
func splitRawLines(data []byte) []rawLine {
	remaining := string(data)
	var lines []rawLine
	for len(remaining) > 0 {
		idx := strings.Index(remaining, "\n")
		if idx < 0 {
			lines = append(lines, rawLine{content: remaining})
			break
		}
		lineContent := remaining[:idx]
		ending := "\n"
		if strings.HasSuffix(lineContent, "\r") {
			lineContent = lineContent[:len(lineContent)-1]
			ending = "\r\n"
		}
		lines = append(lines, rawLine{content: lineContent, lineEnding: ending})
		remaining = remaining[idx+1:]
	}
	return lines
}

// parseState holds mutable state during parsing.
type parseState struct {
	blockID       int
	dataID        int
	paraLines     []string
	paraLineIdxes []int // indices into rLines for skeleton tracking
}

func (ps *parseState) flushParagraph(ctx context.Context, r *Reader, ch chan<- model.PartResult, rLines []rawLine) bool {
	if len(ps.paraLines) == 0 {
		return true
	}
	// Per the DokuWiki paragraph contract — and the upstream WikiFilter
	// behaviour — adjacent non-blank lines belong to one paragraph and
	// the embedded soft line breaks collapse to a single space. Okapi
	// joins lines with the source's line ending and then runs
	// WhitespaceAdjustingEventBuilder to collapse interior runs of
	// whitespace (between non-whitespace runs) to a single space. We
	// mirror the observable outcome directly: join with `\n`, then
	// collapse interior whitespace runs — unless the caller has opted
	// into PreserveWhitespace. Tracked under #522.
	text := strings.Join(ps.paraLines, "\n")
	if !r.cfg.PreserveWhitespace {
		text = collapseInteriorWhitespace(text)
	}
	paraIdxes := ps.paraLineIdxes
	ps.paraLines = nil
	ps.paraLineIdxes = nil
	if strings.TrimSpace(text) == "" {
		return true
	}

	// Skeleton ref strategy: emit a single ref for the paragraph and
	// trail the last source line ending as skeleton text. We compute
	// it once up front so the early-return paths below can reuse it.
	emitSkeletonRef := func(blockID string) {
		if r.skeletonStore != nil && len(rLines) > 0 {
			r.skelRef(blockID)
			if len(paraIdxes) > 0 {
				lastIdx := paraIdxes[len(paraIdxes)-1]
				if lastIdx < len(rLines) {
					r.skelText(rLines[lastIdx].lineEnding)
				}
			}
		}
	}

	// DokuWiki image syntax recognition (#521). Only apply for the
	// DokuWiki variant — the upstream WikiFilter is DokuWiki-only and
	// the `{{…}}` construct does not exist as inline syntax in
	// MediaWiki (which uses `[[File:…]]`, handled by the dedicated
	// MediaWiki image path).
	if r.cfg.Variant == VariantDokuWiki && dokuWikiImageRe.MatchString(text) {
		return ps.emitDokuWikiParagraphWithImages(ctx, r, ch, text, emitSkeletonRef)
	}

	ps.blockID++
	blockID := fmt.Sprintf("tu%d", ps.blockID)
	emitSkeletonRef(blockID)
	block := model.NewBlock(blockID, text)
	tokenizeDokuWikiInlineCodes(block)
	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// emitDokuWikiParagraphWithImages emits any image captions as their own
// translatable Blocks (in document order) followed by the surrounding
// paragraph as a single Block whose source carries the images as inline
// PlaceholderRuns. When the trimmed paragraph is exactly one bare image
// (`{{img.png}}` with no caption and no surrounding text), no Block is
// emitted at all — the image is non-translatable and there is no
// surrounding text to carry it as inline content.
//
// Mirrors okapi WikiFilter's IMAGE handling: the `{{…}}` construct is
// pulled out by TEMP_EXTRACT before block / text-unit splitting and
// reinjected as an inline code, with `IMAGE_CAPTION_PATTERN` extracting
// the caption text as a translatable PropertyTextUnitPlaceholder.
func (ps *parseState) emitDokuWikiParagraphWithImages(
	ctx context.Context, r *Reader, ch chan<- model.PartResult,
	text string, emitSkeletonRef func(string),
) bool {
	// Find every image construct and its position.
	matches := dokuWikiImageRe.FindAllStringIndex(text, -1)

	// Special case: trimmed paragraph is exactly one bare image
	// (no caption, no surrounding text) → emit no translatable Block.
	if len(matches) == 1 {
		startMatch := matches[0][0]
		endMatch := matches[0][1]
		// Check if everything outside the match is whitespace.
		if strings.TrimSpace(text[:startMatch]) == "" && strings.TrimSpace(text[endMatch:]) == "" {
			imgRaw := text[startMatch:endMatch]
			// Only suppress when the image has no caption — otherwise
			// the caption is translatable and must surface.
			if _, caption := splitDokuWikiImage(imgRaw); caption == "" {
				// Attribute the bare image bytes to the skeleton when
				// active so byte-exact roundtrips still reconstruct
				// the document.
				if r.skeletonStore != nil {
					r.skelText(text)
				}
				return true
			}
		}
	}

	// Pass 1: emit a Block for each image's caption, in document order.
	// The okapi WikiFilter emits the caption TextUnit before the
	// surrounding paragraph TextUnit; mirror that ordering so block
	// indexes line up across implementations.
	for _, m := range matches {
		imgRaw := text[m[0]:m[1]]
		_, caption := splitDokuWikiImage(imgRaw)
		caption = strings.TrimSpace(caption)
		if caption == "" {
			continue
		}
		ps.blockID++
		captionBlock := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), caption)
		captionBlock.Name = "image-caption"
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: captionBlock}) {
			return false
		}
	}

	// Pass 2: build the surrounding paragraph as Runs, replacing each
	// image with an inline PlaceholderRun so SourceText() returns only
	// the translatable text spans.
	ps.blockID++
	blockID := fmt.Sprintf("tu%d", ps.blockID)
	emitSkeletonRef(blockID)

	runs := make([]model.Run, 0, len(matches)*2+1)
	cursor := 0
	for i, m := range matches {
		if m[0] > cursor {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[cursor:m[0]]}})
		}
		imgRaw := text[m[0]:m[1]]
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
			ID:    fmt.Sprintf("ph%d", i+1),
			Type:  "image",
			Data:  imgRaw,
			Equiv: imgRaw,
		}})
		cursor = m[1]
	}
	if cursor < len(text) {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: text[cursor:]}})
	}

	block := model.NewRunsBlock(blockID, runs)
	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// splitDokuWikiImage splits a `{{name|caption}}` construct into its
// name and caption components. Returns the caption as the empty string
// when the construct has no `|`.
func splitDokuWikiImage(raw string) (name, caption string) {
	// Strip the `{{` prefix and `}}` suffix.
	inner := strings.TrimSuffix(strings.TrimPrefix(raw, "{{"), "}}")
	if pipe := strings.Index(inner, "|"); pipe >= 0 {
		return inner[:pipe], inner[pipe+1:]
	}
	return inner, ""
}

// tokenizeDokuWikiInlineCodes walks the block's first source segment and
// rewrites its Run sequence so DokuWiki link / image markup survives as
// opaque inline codes (placeholders) rather than translatable text.
//
// Constructs handled (mirrors Okapi WikiPatterns):
//   - `[[target]]`           → single PlaceholderRun (LINK_START with
//     placeholder=true).
//   - `[[target|alt text]]`  → PcOpen / TextRun(alt text) / PcClose
//     paired sequence. The alt text stays translatable while the link
//     target round-trips verbatim — matches Okapi's NAMED_LINK_START /
//     NAMED_LINK_END pair.
//   - `{{anything}}`         → single PlaceholderRun (IMAGE_START with
//     placeholder=true).
//
// Without this pass, pseudo-translation rewrites the URL-bearing parts
// of the markup (`[[doku>DokuWiki]]` → `[[ďōķũ>ĎōķũŴĩķĩ]]`), driving the
// wiki parity round-trip away from byte parity with the okapi reference.
func tokenizeDokuWikiInlineCodes(b *model.Block) {
	if b == nil || len(b.Source) == 0 {
		return
	}
	for _, seg := range b.Source {
		if len(seg.Runs) == 0 {
			continue
		}
		text := seg.Text()
		// Cheap fast-path: skip the regex scan when the segment has
		// none of the inline opener characters our tokeniser knows
		// about (`[`, `{`, `<`, plus the symmetric markers `*`, `_`,
		// `'`, `/`).
		if !strings.ContainsAny(text, "[{<*_'/") {
			continue
		}
		runs, changed := splitDokuWikiInlineRuns(text)
		if changed {
			seg.SetRuns(runs)
		}
	}
}

// splitDokuWikiInlineRuns scans `text` left-to-right, emitting:
//   - TextRun for plain text spans;
//   - PlaceholderRun for `[[target]]` and `{{...}}` constructs;
//   - PcOpen / TextRun / PcClose triples for `[[target|alt]]` and
//     paired DokuWiki inline-formatting markers (`**bold**`,
//     `//italic//`, `__underline__`, `”monospace”`).
//
// The returned bool reports whether any inline construct matched (when
// false the caller can keep the original single TextRun).
//
// `lastEmit` tracks the leftmost byte not yet copied into a run so that
// non-matching `[[` / `{{` openers (e.g. `{{Infobox\n|...` whose closing
// `}}` lands in a different paragraph) survive verbatim — losing those
// openers regressed mediawiki.wiki against the okapi reference.
func splitDokuWikiInlineRuns(text string) ([]model.Run, bool) {
	var runs []model.Run
	var idCounter int
	lastEmit := 0
	scan := 0
	changed := false
	flushTextUpTo := func(end int) {
		if end > lastEmit {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEmit:end]}})
			lastEmit = end
		}
	}
	emitPaired := func(absStart, openEnd, closeStart, closeEnd int, codeType, opener, closer string) {
		flushTextUpTo(absStart)
		altText := text[openEnd:closeStart]
		idCounter++
		openID := fmt.Sprintf("ph%d", idCounter)
		runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
			ID:   openID,
			Type: codeType,
			Data: opener,
		}})
		if altText != "" {
			// Recurse so nested inline constructs (e.g. `**__bold+
			// underline__**`) get tokenised too.
			inner, innerChanged := splitDokuWikiInlineRuns(altText)
			if innerChanged {
				runs = append(runs, inner...)
			} else {
				runs = append(runs, model.Run{Text: &model.TextRun{Text: altText}})
			}
		}
		runs = append(runs, model.Run{PcClose: &model.PcCloseRun{
			ID:   openID,
			Type: codeType,
			Data: closer,
		}})
		lastEmit = closeEnd
		scan = lastEmit
		changed = true
	}
	for scan < len(text) {
		// Find the next inline construct from `scan`. We rank by
		// earliest start offset so left-to-right precedence wins; ties
		// break in favour of multi-char openers (link / image > paired
		// markers), but in practice the openers don't collide.
		//
		// `<` is a candidate opener for the HTML-style paired tags
		// (`<sub>`, `<sup>`, `<del>`, `<nowiki>`); the tag-name regex
		// validates the match, so plain `<file>` / `<files>` / `<` in
		// translatable text fall through to the default text run.
		bestStart := -1
		var bestKind string
		for _, m := range []string{"[[", "{{", "**", "__", "''", "//", "<"} {
			if idx := strings.Index(text[scan:], m); idx >= 0 {
				if bestStart < 0 || idx < bestStart {
					bestStart = idx
					bestKind = m
				}
			}
		}
		if bestStart < 0 {
			break
		}
		absStart := scan + bestStart

		matched := false

		// HTML-style paired tag (case-insensitive sub/sup/del/nowiki).
		if bestKind == "<" {
			if openLoc := dokuWikiHTMLOpenRe.FindStringSubmatchIndex(text[absStart:]); openLoc != nil && openLoc[0] == 0 {
				tag := strings.ToLower(text[absStart+openLoc[2] : absStart+openLoc[3]])
				if closeRe, ok := dokuWikiHTMLCloseRe[tag]; ok {
					rest := text[absStart+openLoc[1]:]
					if closeLoc := closeRe.FindStringIndex(rest); closeLoc != nil {
						emitPaired(
							absStart,
							absStart+openLoc[1],
							absStart+openLoc[1]+closeLoc[0],
							absStart+openLoc[1]+closeLoc[1],
							"wiki:"+tag,
							text[absStart:absStart+openLoc[1]],
							rest[closeLoc[0]:closeLoc[1]],
						)
						matched = true
					}
				}
			}
		}

		// Try image `{{...}}` first when that's the leading token.
		if bestKind == "{{" {
			if loc := dokuWikiImageRe.FindStringIndex(text[absStart:]); loc != nil && loc[0] == 0 {
				flushTextUpTo(absStart)
				idCounter++
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
					ID:   fmt.Sprintf("ph%d", idCounter),
					Type: "wiki:image",
					Data: text[absStart : absStart+loc[1]],
				}})
				lastEmit = absStart + loc[1]
				scan = lastEmit
				changed = true
				matched = true
			}
		}

		if !matched && bestKind == "[[" {
			// Try named link `[[target|alt]]`.
			if startLoc := dokuWikiNamedLinkStartRe.FindStringIndex(text[absStart:]); startLoc != nil && startLoc[0] == 0 {
				rest := text[absStart+startLoc[1]:]
				if endLoc := dokuWikiNamedLinkEndRe.FindStringIndex(rest); endLoc != nil {
					emitPaired(
						absStart,
						absStart+startLoc[1],
						absStart+startLoc[1]+endLoc[0],
						absStart+startLoc[1]+endLoc[1],
						"wiki:link",
						text[absStart:absStart+startLoc[1]],
						rest[endLoc[0]:endLoc[1]],
					)
					matched = true
				}
			}
			// Fall back to bare `[[target]]` placeholder.
			if !matched {
				if loc := dokuWikiLinkRe.FindStringIndex(text[absStart:]); loc != nil && loc[0] == 0 {
					flushTextUpTo(absStart)
					idCounter++
					runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
						ID:   fmt.Sprintf("ph%d", idCounter),
						Type: "wiki:link",
						Data: text[absStart : absStart+loc[1]],
					}})
					lastEmit = absStart + loc[1]
					scan = lastEmit
					changed = true
					matched = true
				}
			}
		}

		// Paired symmetric inline markers (`**`, `__`, `''`, `//`).
		if !matched {
			for _, p := range dokuWikiPaired {
				if p.marker != bestKind {
					continue
				}
				openLen := len(p.marker)
				// Italic guard: skip openers immediately preceded by `:`
				// (so `http://` doesn't open italic). Mirrors Okapi's
				// `(?<!:)//` lookbehind.
				if p.marker == "//" && absStart > 0 && text[absStart-1] == ':' {
					break
				}
				// Find the matching closer in the remainder of the
				// paragraph. Closer must NOT immediately follow the
				// opener (no zero-length pairs) and, for italic, must
				// be followed by whitespace or end-of-string per
				// Okapi's `//(?=\s|$)`.
				rest := text[absStart+openLen:]
				closeRel := -1
				idx := 0
				for {
					found := strings.Index(rest[idx:], p.marker)
					if found < 0 {
						break
					}
					candidate := idx + found
					if p.marker == "//" {
						after := candidate + openLen
						if after < len(rest) {
							c := rest[after]
							if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
								idx = candidate + openLen
								continue
							}
						}
					}
					closeRel = candidate
					break
				}
				if closeRel <= 0 {
					break
				}
				closeStart := absStart + openLen + closeRel
				closeEnd := closeStart + openLen
				emitPaired(absStart, absStart+openLen, closeStart, closeEnd, p.codeID, text[absStart:absStart+openLen], text[closeStart:closeEnd])
				matched = true
				break
			}
		}

		if !matched {
			// Opener has no closer on the same paragraph — leave the
			// opener characters in place (they'll get folded into the
			// next text flush) and resume scanning past them. For the
			// `<` candidate (which can be a non-tag character) advance
			// only one byte so we don't accidentally swallow the next
			// real opener.
			step := len(bestKind)
			if bestKind == "<" {
				step = 1
			}
			scan = absStart + step
		}
	}
	if !changed {
		return nil, false
	}
	flushTextUpTo(len(text))
	return runs, true
}

func (ps *parseState) emitData(ctx context.Context, r *Reader, ch chan<- model.PartResult) bool {
	ps.dataID++
	data := &model.Data{ID: fmt.Sprintf("d%d", ps.dataID)}
	return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Emit layer start
	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "wiki",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-wiki",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	var rLines []rawLine

	if r.skeletonStore != nil {
		data, err := io.ReadAll(r.Doc.Reader)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("wiki: reading: %w", err)}
			return
		}
		rLines = splitRawLines(data)
		ps := &parseState{}
		if r.cfg.Variant == VariantDokuWiki {
			r.readDokuWikiLines(ctx, ch, rLines, ps)
		} else {
			r.readMediaWikiLines(ctx, ch, rLines, ps)
		}
		r.skelFlush()
	} else {
		scanner := bufio.NewScanner(r.Doc.Reader)
		ps := &parseState{}
		if r.cfg.Variant == VariantDokuWiki {
			r.readDokuWiki(ctx, ch, scanner, ps)
		} else {
			r.readMediaWiki(ctx, ch, scanner, ps)
		}
		if err := scanner.Err(); err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("wiki: reading: %w", err)}
		}
	}

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readMediaWiki(ctx context.Context, ch chan<- model.PartResult,
	scanner *bufio.Scanner, ps *parseState) {

	inTable := false

	for scanner.Scan() {
		line := scanner.Text()

		// Check for header
		if m := mediaWikiHeaderRe.FindStringSubmatch(line); m != nil {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			ps.blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), strings.TrimSpace(m[2]))
			block.Name = "header"
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// Table start
		if mediaWikiTableStartRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			inTable = true
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table end
		if mediaWikiTableEndRe.MatchString(line) {
			inTable = false
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table row separator
		if inTable && mediaWikiTableRowRe.MatchString(line) {
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table header cells
		if inTable && mediaWikiTableHeaderRe.MatchString(line) {
			m := mediaWikiTableHeaderRe.FindStringSubmatch(line)
			cells := splitTableCells(m[1], "!!")
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				if cell == "" {
					continue
				}
				ps.blockID++
				block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
				block.Name = "table-header"
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
			continue
		}

		// Table data cells
		if inTable && mediaWikiTableCellRe.MatchString(line) {
			m := mediaWikiTableCellRe.FindStringSubmatch(line)
			cells := splitTableCells(m[1], "||")
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				if cell == "" {
					continue
				}
				ps.blockID++
				block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
				block.Name = "table-cell"
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
			continue
		}

		// Image/file links with captions
		if mediaWikiImageRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			r.extractImageCaptions(ctx, ch, line, ps)
			continue
		}

		// Blank line separates paragraphs
		if strings.TrimSpace(line) == "" {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Regular text line -- accumulate into paragraph
		ps.paraLines = append(ps.paraLines, line)
	}

	// Flush remaining paragraph
	ps.flushParagraph(ctx, r, ch, nil)
}

func (r *Reader) readMediaWikiLines(ctx context.Context, ch chan<- model.PartResult,
	rLines []rawLine, ps *parseState) {

	inTable := false

	for i, rl := range rLines {
		line := rl.content

		// Check for header
		if m := mediaWikiHeaderRe.FindStringSubmatch(line); m != nil {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			ps.blockID++
			blockID := fmt.Sprintf("tu%d", ps.blockID)
			r.skelRef(blockID)
			r.skelText(rl.lineEnding)
			block := model.NewBlock(blockID, strings.TrimSpace(m[2]))
			block.Name = "header"
			block.Properties["raw"] = line
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// Table start
		if mediaWikiTableStartRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			inTable = true
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table end
		if mediaWikiTableEndRe.MatchString(line) {
			inTable = false
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table row separator
		if inTable && mediaWikiTableRowRe.MatchString(line) {
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Table header cells
		if inTable && mediaWikiTableHeaderRe.MatchString(line) {
			// Store entire line as skeleton text (table reconstruction is complex)
			r.skelText(rl.content + rl.lineEnding)
			m := mediaWikiTableHeaderRe.FindStringSubmatch(line)
			cells := splitTableCells(m[1], "!!")
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				if cell == "" {
					continue
				}
				ps.blockID++
				block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
				block.Name = "table-header"
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
			continue
		}

		// Table data cells
		if inTable && mediaWikiTableCellRe.MatchString(line) {
			r.skelText(rl.content + rl.lineEnding)
			m := mediaWikiTableCellRe.FindStringSubmatch(line)
			cells := splitTableCells(m[1], "||")
			for _, cell := range cells {
				cell = strings.TrimSpace(cell)
				if cell == "" {
					continue
				}
				ps.blockID++
				block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
				block.Name = "table-cell"
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
			continue
		}

		// Image/file links with captions
		if mediaWikiImageRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			r.extractImageCaptions(ctx, ch, line, ps)
			continue
		}

		// Blank line separates paragraphs
		if strings.TrimSpace(line) == "" {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Regular text line -- accumulate into paragraph
		ps.paraLines = append(ps.paraLines, line)
		ps.paraLineIdxes = append(ps.paraLineIdxes, i)
	}

	// Flush remaining paragraph
	ps.flushParagraph(ctx, r, ch, rLines)
}

func (r *Reader) readDokuWiki(ctx context.Context, ch chan<- model.PartResult,
	scanner *bufio.Scanner, ps *parseState) {

	for scanner.Scan() {
		line := scanner.Text()

		// Check for header
		if m := dokuWikiHeaderRe.FindStringSubmatch(line); m != nil {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			ps.blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), strings.TrimSpace(m[2]))
			block.Name = "header"
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// DokuWiki table row (well-formed: leading + trailing delimiter)
		if dokuWikiTableRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			r.extractDokuWikiTableCells(ctx, ch, line, ps)
			continue
		}

		// DokuWiki "open" table row (leading `|` or `^` only). Each
		// line becomes a single Block whose source text is everything
		// after the opener delimiter. Mirrors okapi's TABLE_START
		// recognition path so multi-line `{{Infobox\n|key=value\n}}`
		// MediaWiki templates surface as one cell per line rather than
		// being joined into a single space-collapsed paragraph. The
		// extra cell whitespace is collapsed (mirroring okapi's
		// WhitespaceAdjustingEventBuilder).
		if dokuWikiOpenTableRowRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			cell := strings.TrimSpace(line[1:])
			if !r.cfg.PreserveWhitespace {
				cell = collapseInteriorWhitespace(cell)
			}
			if cell != "" {
				ps.blockID++
				block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
				block.Name = "table-cell"
				tokenizeDokuWikiInlineCodes(block)
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
			continue
		}

		// Code block (indented two-or-more spaces) — emit a Data part
		// per line so the line stays out of the translatable surface
		// while the no-skeleton write path still serialises a
		// structural separator.
		if dokuWikiCodeStartRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Blank line separates paragraphs
		if strings.TrimSpace(line) == "" {
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Regular text -- accumulate into paragraph
		ps.paraLines = append(ps.paraLines, line)
	}

	// Flush remaining paragraph
	ps.flushParagraph(ctx, r, ch, nil)
}

func (r *Reader) readDokuWikiLines(ctx context.Context, ch chan<- model.PartResult,
	rLines []rawLine, ps *parseState) {

	for i, rl := range rLines {
		line := rl.content

		// Check for header
		if m := dokuWikiHeaderRe.FindStringSubmatch(line); m != nil {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			ps.blockID++
			blockID := fmt.Sprintf("tu%d", ps.blockID)
			r.skelRef(blockID)
			r.skelText(rl.lineEnding)
			block := model.NewBlock(blockID, strings.TrimSpace(m[2]))
			block.Name = "header"
			block.Properties["raw"] = line
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// DokuWiki table row (well-formed)
		if dokuWikiTableRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			r.extractDokuWikiTableCells(ctx, ch, line, ps)
			continue
		}

		// DokuWiki "open" table row — see readDokuWiki for rationale.
		// Skeleton splits the line into: leading delimiter + space →
		// skeleton text, the cell content → ref, trailing whitespace
		// + line ending → skeleton text. That way the round-trip
		// writer reconstructs `|key = value\r\n` verbatim with the
		// pseudo'd cell content slotted into the middle.
		if dokuWikiOpenTableRowRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			cell := strings.TrimSpace(line[1:])
			if !r.cfg.PreserveWhitespace {
				cell = collapseInteriorWhitespace(cell)
			}
			if cell == "" {
				r.skelText(rl.content + rl.lineEnding)
				if !ps.emitData(ctx, r, ch) {
					return
				}
				continue
			}
			// Compute leading and trailing skeleton chunks so
			// arbitrary spacing inside `|   key = value   ` survives.
			body := line[1:]
			leadLen := len(body) - len(strings.TrimLeft(body, " \t"))
			trailLen := len(body) - len(strings.TrimRight(body, " \t"))
			leadSkel := line[:1+leadLen]
			trailSkel := body[len(body)-trailLen:] + rl.lineEnding
			r.skelText(leadSkel)
			ps.blockID++
			blockID := fmt.Sprintf("tu%d", ps.blockID)
			r.skelRef(blockID)
			r.skelText(trailSkel)
			block := model.NewBlock(blockID, cell)
			block.Name = "table-cell"
			tokenizeDokuWikiInlineCodes(block)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// Code block — same rationale as readDokuWiki above. The
		// entire raw line (including its indent) goes to skeleton, so
		// the round-trip writer outputs it verbatim with no pseudo
		// pass touching the contents.
		if dokuWikiCodeStartRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Blank line separates paragraphs
		if strings.TrimSpace(line) == "" {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Regular text -- accumulate into paragraph
		ps.paraLines = append(ps.paraLines, line)
		ps.paraLineIdxes = append(ps.paraLineIdxes, i)
	}

	// Flush remaining paragraph
	ps.flushParagraph(ctx, r, ch, rLines)
}

func (r *Reader) extractImageCaptions(ctx context.Context, ch chan<- model.PartResult,
	line string, ps *parseState) {

	matches := mediaWikiImageRe.FindAllStringSubmatch(line, -1)
	for _, m := range matches {
		if len(m) < 3 || m[2] == "" {
			continue
		}
		// m[2] contains |param1|param2|...|caption
		// The last pipe-separated segment is typically the caption.
		parts := strings.Split(m[2], "|")
		// Skip the first empty element (leading |)
		var caption string
		for i := len(parts) - 1; i >= 0; i-- {
			seg := strings.TrimSpace(parts[i])
			if seg == "" {
				continue
			}
			// Skip known MediaWiki image parameters
			lower := strings.ToLower(seg)
			if isMediaWikiImageParam(lower) {
				continue
			}
			caption = seg
			break
		}
		if caption != "" {
			ps.blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), caption)
			block.Name = "image-caption"
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
	}

	// Also emit any text outside the image link
	remainder := mediaWikiImageRe.ReplaceAllString(line, "")
	remainder = strings.TrimSpace(remainder)
	if remainder != "" {
		ps.blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), remainder)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}
}

func isMediaWikiImageParam(s string) bool {
	params := []string{
		"thumb", "thumbnail", "frame", "frameless", "border",
		"left", "right", "center", "none",
		"baseline", "sub", "super", "top", "text-top", "middle", "bottom", "text-bottom",
		"upright",
	}
	for _, p := range params {
		if s == p {
			return true
		}
	}
	// Prefixed params like "link=..."
	if strings.HasPrefix(s, "link=") || strings.HasPrefix(s, "alt=") || strings.HasPrefix(s, "page=") {
		return true
	}
	// Size spec like "200px" or "200x300px"
	if strings.HasSuffix(s, "px") {
		return true
	}
	return false
}

func (r *Reader) extractDokuWikiTableCells(ctx context.Context, ch chan<- model.PartResult,
	line string, ps *parseState) {

	// Remove leading/trailing | or ^
	trimmed := line
	if len(trimmed) > 0 && (trimmed[0] == '|' || trimmed[0] == '^') {
		trimmed = trimmed[1:]
	}
	if len(trimmed) > 0 && (trimmed[len(trimmed)-1] == '|' || trimmed[len(trimmed)-1] == '^') {
		trimmed = trimmed[:len(trimmed)-1]
	}

	// Split on | and ^
	var cells []string
	var current strings.Builder
	for _, c := range trimmed {
		if c == '|' || c == '^' {
			cells = append(cells, current.String())
			current.Reset()
		} else {
			current.WriteRune(c)
		}
	}
	cells = append(cells, current.String())

	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		ps.blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), cell)
		block.Name = "table-cell"
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}
}

func splitTableCells(content, separator string) []string {
	return strings.Split(content, separator)
}

// collapseInteriorWhitespace mirrors the okapi WhitespaceAdjustingEventBuilder
// behaviour applied to wiki text units: runs of whitespace flanked by
// non-whitespace runs collapse to a single space. Whitespace at the
// start or end of the input is preserved (the upstream filter peels
// surrounding whitespace into the skeleton — we leave it in the text
// rather than splitting because the wiki Block model already trims for
// extraction). Equivalent to Java's `(?<=\S)\s+(?=\S)` → " " replacement.
func collapseInteriorWhitespace(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	runes := []rune(s)
	i := 0
	// Pass 1: copy any leading whitespace verbatim.
	for i < len(runes) && isWikiSpace(runes[i]) {
		b.WriteRune(runes[i])
		i++
	}
	// Pass 2: walk the body, collapsing interior whitespace runs.
	for i < len(runes) {
		r := runes[i]
		if !isWikiSpace(r) {
			b.WriteRune(r)
			i++
			continue
		}
		// Start of a whitespace run inside the body. Look ahead.
		j := i
		for j < len(runes) && isWikiSpace(runes[j]) {
			j++
		}
		if j == len(runes) {
			// Trailing whitespace — preserve verbatim.
			for k := i; k < j; k++ {
				b.WriteRune(runes[k])
			}
		} else {
			// Interior whitespace run between non-whitespace — collapse
			// to a single space.
			b.WriteByte(' ')
		}
		i = j
	}
	return b.String()
}

// isWikiSpace mirrors Java's \s for ASCII (space, tab, CR, LF, FF, VT).
// Wiki documents are typically ASCII for whitespace so we don't extend
// this to Unicode whitespace classes.
func isWikiSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

// skelText appends text to the skeleton buffer if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		r.skelBuf.WriteString(s)
	}
}

// skelRef flushes buffered text and writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
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
