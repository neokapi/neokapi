package wiki

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// MediaWiki header pattern: == Title == through ====== Title ======
var mediaWikiHeaderRe = regexp.MustCompile(`^(={2,6})\s*(.+?)\s*(={2,6})\s*$`)

// DokuWiki header pattern: same syntax as MediaWiki (= delimiters)
var dokuWikiHeaderRe = regexp.MustCompile(`^(={2,6})\s*(.+?)\s*(={2,6})\s*$`)

// headerLayoutRe decomposes a matched header line into its byte-exact
// layout: leading `=` delimiters, the whitespace around the title, the
// trailing `=` delimiters, and any whitespace after them. Captured at
// read time and stamped onto the block via storeHeaderLayout so the
// writer can rebuild the line without re-parsing the stored raw source.
//
//	group 1: leading `=` run    (e.g. "===")
//	group 2: whitespace before title
//	group 3: whitespace after title
//	group 4: trailing `=` run   (e.g. "===")
//	group 5: trailing whitespace after the closing delimiters
var headerLayoutRe = regexp.MustCompile(`^(={2,6})(\s*)\S.*?(\s*)(={2,6})(\s*)$`)

// Header layout property keys. headerLevel records the leading delimiter
// length (number of `=`); the *Pre / *Post keys preserve the exact
// surrounding whitespace and the trailing delimiter run so headers
// round-trip byte-for-byte without re-parsing the raw source line.
const (
	headerLevelKey  = "headerLevel"  // leading "=" count, e.g. "3"
	headerPrefixWS  = "headerPreWS"  // whitespace between leading "=" and title
	headerSuffixWS  = "headerPostWS" // whitespace between title and trailing "="
	headerCloseKey  = "headerClose"  // trailing "=" run, e.g. "==="
	headerTrailerWS = "headerEndWS"  // whitespace after the trailing "=" run
)

// storeHeaderLayout records the byte-exact delimiter layout of a matched
// header line on the block. The whitespace captures default to a single
// space when absent so reconstruction always produces well-formed markup,
// matching the layout of the canonical "== Title ==" form.
func storeHeaderLayout(block *model.Block, line string) {
	m := headerLayoutRe.FindStringSubmatch(line)
	if m == nil {
		// Defensive fallback: a line that matched the header recognizer
		// but not the layout decomposer (should not happen). Reconstruct
		// from the recognizer's delimiter groups so output stays valid.
		if hm := mediaWikiHeaderRe.FindStringSubmatch(line); hm != nil {
			block.Properties[headerLevelKey] = strconv.Itoa(len(hm[1]))
			block.Properties[headerPrefixWS] = " "
			block.Properties[headerSuffixWS] = " "
			block.Properties[headerCloseKey] = hm[3]
			block.Properties[headerTrailerWS] = ""
		}
		return
	}
	block.Properties[headerLevelKey] = strconv.Itoa(len(m[1]))
	block.Properties[headerPrefixWS] = m[2]
	block.Properties[headerSuffixWS] = m[3]
	block.Properties[headerCloseKey] = m[4]
	block.Properties[headerTrailerWS] = m[5]
}

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
	// `]]` — closing half of `[[target|alt]]`.
	dokuWikiNamedLinkEndRe = regexp.MustCompile(`\]\]`)
	// `{{...}}` — image / template placeholder. Single-line only,
	// matching Okapi's IMAGE_START.
	dokuWikiImageRe = regexp.MustCompile(`\{\{[^}\r\n]+\}\}`)
	// HTML-style paired inline tags recognised by Okapi WikiPatterns
	// (SUB, SUP, DEL, NOWIKI_TAG); each closer is `</tag>`. We capture the
	// tag literally so the writer round-trips closer bytes verbatim. The
	// inner text remains translatable.
	dokuWikiHTMLCloseRe = map[string]*regexp.Regexp{
		"sub":    regexp.MustCompile(`(?i)</sub>`),
		"sup":    regexp.MustCompile(`(?i)</sup>`),
		"del":    regexp.MustCompile(`(?i)</del>`),
		"nowiki": regexp.MustCompile(`(?i)</nowiki>`),
	}

	// Anchored (`^`) variants of the opener patterns above, used by the
	// inline-run scanner where a match is only accepted at the current
	// position (the historic `loc[0] == 0` guard). Without the anchor,
	// FindStringIndex(text[absStart:]) scans the entire remaining paragraph
	// looking for a match anywhere before reporting "no match at 0", which
	// — repeated per unmatched opener — made a marker-dense paragraph
	// O(n^2) (#608, N2). Anchoring lets the regex bail in O(1) when there
	// is no construct at `absStart`. These are byte-neutral: they match
	// exactly the same constructs the `loc[0] == 0` checks already required.
	// The shared (unanchored) vars are kept for the table/temp-extract scans
	// that legitimately search anywhere (e.g. dokuWikiImageRe at file scope).
	dokuWikiNamedLinkStartAnchoredRe = regexp.MustCompile(`^\[\[[^|\]\r\n]+\|`)
	dokuWikiLinkAnchoredRe           = regexp.MustCompile(`^\[\[[^|\]\r\n]+\]\]`)
	dokuWikiImageAnchoredRe          = regexp.MustCompile(`^\{\{[^}\r\n]+\}\}`)
	dokuWikiMacroAnchoredRe          = regexp.MustCompile(`^~~(?:NOTOC|NOCACHE|INFO:\w*)~~`)
	dokuWikiHTMLOpenAnchoredRe       = regexp.MustCompile(`(?i)^<(sub|sup|del|nowiki)\b[^>]*>`)
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
	// `%%` is the DokuWiki NOWIKI shorthand. Treated as paired
	// markers like the others; emitPaired's `wiki:nowiki` carve-out
	// suppresses recursive tokenisation of the inner text so embedded
	// markers (`~~NOTOC~~`, URLs, …) stay translatable plain text.
	{marker: "%%", codeID: "wiki:nowiki"},
}

// dokuWikiInlineMarkers is the ordered set of inline-construct openers
// scanned by splitDokuWikiInlineRuns. Order is significant: the candidate
// loop breaks earliest-start ties in favour of the marker appearing first
// here (link / image before the symmetric markers), matching the historic
// scan order. The list is kept package-level so the per-paragraph scan can
// maintain a cached next-occurrence offset per marker without re-allocating
// the slice on every call.
var dokuWikiInlineMarkers = []string{"[[", "{{", "**", "__", "''", "//", "<", "~~", "%%"}

// MediaWiki table patterns
var mediaWikiTableStartRe = regexp.MustCompile(`^\{\|`)
var mediaWikiTableEndRe = regexp.MustCompile(`^\|\}`)
var mediaWikiTableRowRe = regexp.MustCompile(`^\|-`)
var mediaWikiTableCellRe = regexp.MustCompile(`^\|(.+)`)
var mediaWikiTableHeaderRe = regexp.MustCompile(`^!(.+)`)

// mediaWikiOkapiTableEndRe mirrors okapi WikiPatterns.TABLE_END
// (`[\^|]\s*\n`) — a `|` or `^` followed by inline whitespace and a
// newline. When this pattern never matches anywhere in the file but a
// `^|`/`^\^` line is present (TABLE_START match — see
// mediaWikiOkapiFirstTableStartLine), the table block runs all the way
// to EOF and its cells are split on every `|`/`^` character
// (TABLE_CELL_PATTERN). MediaWiki Infobox templates
// (`{{Infobox\n|key=value\n...\n}}` followed by paragraphs) hit this
// branch because the infobox lines end in a value (not `|`/`^`), so blank
// lines and HTML comments inside the infobox get folded into the previous
// cell's value via WhitespaceAdjustingEventBuilder.
var mediaWikiOkapiTableEndRe = regexp.MustCompile(`[\^|][ \t]*\r?\n`)

// mediaWikiTempExtractRe mirrors okapi's TEMP_EXTRACT pattern that pulls
// `%%...%%`, `<nowiki>...</nowiki>`, `[[...]]`, and `{{...}}` constructs
// out of the stream BEFORE table-cell tokenisation so that `|` characters
// inside `[[link|alt]]` or `{{Template|param}}` don't get treated as cell
// boundaries. The pattern is reluctant (`.*?`) and single-line — exactly
// matching `%%.*?%%|<nowiki>.*?</nowiki>|\[\[.*?\]\]|\{\{.*?\}\}` from
// WikiPatterns.TEMP_EXTRACT.
var mediaWikiTempExtractRe = regexp.MustCompile(`%%.*?%%|<nowiki>.*?</nowiki>|\[\[.*?\]\]|\{\{.*?\}\}`)

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

// DokuWiki list item: line starts with two-or-more spaces followed by a
// `*` (unordered) or `-` (ordered) marker plus a single space. Mirrors
// Okapi WikiPatterns LIST_ITEM_START_PATTERN (`^ {2,}[\*-] |^>+ `).
// We honour the indented `* ` / `- ` form here; the `>+ ` quote-block
// form has no fixture coverage so we leave it for follow-up. Each
// matching line becomes one Block whose source text is everything after
// the leading delimiter. Without this, `  * a\n  * b\n  * c\n` collapses
// into one paragraph as `* a * b * c` after whitespace collapse, while
// okapi's WikiFilter splits each item into its own LIST_ITEM block.
var dokuWikiListItemRe = regexp.MustCompile(`^( {2,}[\*-] )(.+)$`)

// DokuWiki untranslatable block tags. Each entry's `start` matches a
// line that opens with an `<code lang="php">` / `<file>` / `<html>` /
// `<php>` tag; the matching `end` closes it at the next `</tag>`.
// While the parser is between these markers, every line — including
// blank lines — flows verbatim through the skeleton and contributes
// nothing to translatable text. Mirrors okapi's WikiPatterns CODE_TAG
// / FILE / HTML / PHP block delimiters (all annotated `@Untranslatable`
// upstream).
//
// We anchor each opener with `^\s*` so quoted / escaped occurrences
// inside paragraph prose (e.g. `”%%<code>%%” or ”%%<file>%%”`)
// don't mis-trigger the block. Okapi sidesteps this by running
// TEMP_EXTRACT first — which pulls `%%...%%`, `<nowiki>...</nowiki>`,
// and other non-interpreted spans out of the stream before block
// tokenisation; we approximate the observable outcome by requiring the
// tag at the start of the line.
var dokuWikiUntranslatableBlocks = []struct {
	tag     string
	startRe *regexp.Regexp
	endRe   *regexp.Regexp
	// anyRe matches the opener anywhere in the line (not just at the
	// start). Mirrors okapi's WikiPatterns CODE_TAG / FILE / HTML / PHP
	// FOO_START regex, which the PrefixSuffixTokenizer applies at any
	// position when splitting a block into sub-blocks.
	anyRe *regexp.Regexp
}{
	{tag: "code", startRe: regexp.MustCompile(`(?i)^\s*<code\b[^>]*>`), endRe: regexp.MustCompile(`(?i)</code>`), anyRe: regexp.MustCompile(`(?i)<code\b[^>]*>`)},
	{tag: "file", startRe: regexp.MustCompile(`(?i)^\s*<file\b[^>]*>`), endRe: regexp.MustCompile(`(?i)</file>`), anyRe: regexp.MustCompile(`(?i)<file\b[^>]*>`)},
	{tag: "html", startRe: regexp.MustCompile(`(?i)^\s*<html\b[^>]*>`), endRe: regexp.MustCompile(`(?i)</html>`), anyRe: regexp.MustCompile(`(?i)<html\b[^>]*>`)},
	{tag: "php", startRe: regexp.MustCompile(`(?i)^\s*<php\b[^>]*>`), endRe: regexp.MustCompile(`(?i)</php>`), anyRe: regexp.MustCompile(`(?i)<php\b[^>]*>`)},
}

// dokuWikiTagAttrsRe captures the attribute region of an untranslatable block
// opener so the language / filename hints carried on the tag (DokuWiki's
// `<code php somefile.php>` / `<file php list.php>` forms) can be promoted to
// the surfaced content block's Properties.
var dokuWikiTagAttrsRe = regexp.MustCompile(`(?i)^\s*<(?:code|file|html|php)\b([^>]*)>`)

// dokuWikiTagProps extracts the language and optional name hints from an
// untranslatable block opener line. The first whitespace-delimited token after
// the tag name is the highlight language; an optional second token is the file
// name. Returns nil when the opener carries no attributes.
func dokuWikiTagProps(line string) map[string]string {
	m := dokuWikiTagAttrsRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	fields := strings.Fields(m[1])
	if len(fields) == 0 {
		return nil
	}
	props := map[string]string{"language": fields[0]}
	if len(fields) >= 2 {
		props["name"] = fields[1]
	}
	return props
}

// newDokuWikiCodeBlock builds a non-translatable RoleCode content block (a
// single verbatim run, whitespace-significant, NOT inline-parsed) carrying the
// raw bytes of a DokuWiki tagged or indented code construct. It is surfaced so
// ingestion/LLM consumers see the contextual content while machine translation
// skips it (Translatable=false).
func (r *Reader) newDokuWikiCodeBlock(ps *parseState, text string, props map[string]string) *model.Block {
	ps.blockID++
	block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), text)
	block.Name = "code-block"
	block.Translatable = false
	block.PreserveWhitespace = true
	block.SetSemanticRole(model.RoleCode, 0)
	maps.Copy(block.Properties, props)
	// Promote the DokuWiki highlight language to the canonical code.language
	// convention so cross-format writers (Block.CodeLanguage()) carry it; the
	// open-tag language hint stays in the skeleton for byte-exact round-trip.
	if lang := block.Properties["language"]; lang != "" {
		block.SetCodeLanguage(lang)
		delete(block.Properties, "language")
	}
	return block
}

// emitDokuWikiSkelCode surfaces `text` as a RoleCode content block on the
// skeleton path: the body rides a skeleton ref (so the open/close markers stay
// skeleton and the round-trip is byte-exact). An empty body emits nothing.
func (r *Reader) emitDokuWikiSkelCode(ctx context.Context, ch chan<- model.PartResult,
	ps *parseState, text string, props map[string]string) bool {
	if text == "" {
		return true
	}
	block := r.newDokuWikiCodeBlock(ps, text, props)
	r.skelRef(block.ID)
	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// emitDokuWikiCode surfaces `text` as a RoleCode content block on the
// no-skeleton path (no skeleton ref; the writer reconstructs normalized
// output). An empty body emits nothing.
func (r *Reader) emitDokuWikiCode(ctx context.Context, ch chan<- model.PartResult,
	ps *parseState, text string, props map[string]string) bool {
	if text == "" {
		return true
	}
	block := r.newDokuWikiCodeBlock(ps, text, props)
	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// matchDokuWikiUntranslatableOpener reports whether `line` opens an
// untranslatable block tag (`<code>` / `<file>` / `<html>` / `<php>`).
// The returned end-pattern is used by the line-based parser to detect
// the matching closer on a later line. Returns nil when no block
// opener is present.
func matchDokuWikiUntranslatableOpener(line string) (closeRe *regexp.Regexp, tag string) {
	for _, b := range dokuWikiUntranslatableBlocks {
		if loc := b.startRe.FindStringIndex(line); loc != nil {
			return b.endRe, b.tag
		}
	}
	return nil, ""
}

// findDokuWikiUntranslatableInLine locates the earliest mid-line
// untranslatable block opener (`<code>` / `<file>` / `<html>` / `<php>`)
// within a regular text line. It returns the byte offset where the
// opener begins (`start`), the matching close pattern, and ok=true when
// a real DokuWiki block tag is present.
//
// This mirrors upstream okapi: an untranslatable block delimiter such as
// `<file>` cuts off the surrounding text unit even when it appears in the
// middle of a line. The text before the opener becomes its own TU; the
// opener and everything after it (until the matching closer, or EOF for
// an unclosed block) is non-translatable skeleton — exactly the shape
// WikiFilter.parseBlocks produces for an `@Untranslatable` block.
// Compare WikiFilterTest#testSimilarHtmlTags: `This is <file> a test.`
// yields the TU "This is" while `<files>` (not a real tag, defeated by
// the `\b` word boundary) passes through untouched.
//
// Occurrences inside okapi's TEMP_EXTRACT-protected spans
// (`%%…%%`, `<nowiki>…</nowiki>`, `[[…]]`, `{{…}}`) are skipped, matching
// the upstream behaviour of pulling those spans out before block
// tokenisation so an escaped `%%<file>%%` does not trigger a block.
func findDokuWikiUntranslatableInLine(line string) (start int, closeRe *regexp.Regexp, ok bool) {
	bestStart := -1
	var bestClose *regexp.Regexp
	for _, b := range dokuWikiUntranslatableBlocks {
		loc := b.anyRe.FindStringIndex(line)
		if loc == nil {
			continue
		}
		if dokuWikiPositionProtected(line, loc[0]) {
			continue
		}
		if bestStart < 0 || loc[0] < bestStart {
			bestStart = loc[0]
			bestClose = b.endRe
		}
	}
	if bestStart < 0 {
		return 0, nil, false
	}
	return bestStart, bestClose, true
}

// dokuWikiPositionProtected reports whether the byte offset pos in line
// falls inside a TEMP_EXTRACT-protected span (`%%…%%`,
// `<nowiki>…</nowiki>`, `[[…]]`, `{{…}}`). Such spans are pulled out of
// the stream by okapi before block tokenisation, so an opener inside one
// must not be treated as a block delimiter.
func dokuWikiPositionProtected(line string, pos int) bool {
	for _, loc := range mediaWikiTempExtractRe.FindAllStringIndex(line, -1) {
		if pos >= loc[0] && pos < loc[1] {
			return true
		}
	}
	return false
}

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
	// trailingLineEnding returns the line ending of the paragraph's
	// last source line, or "" when no skeleton tracking is active /
	// the paragraph has no recorded indices. Used by early-return
	// paths that route bare paragraph bytes to skeleton without
	// emitting a ref — they still need to terminate the line so the
	// next blank-line `\r\n` lands on a fresh line in the writer.
	trailingLineEnding := func() string {
		if r.skeletonStore == nil || len(rLines) == 0 || len(paraIdxes) == 0 {
			return ""
		}
		lastIdx := paraIdxes[len(paraIdxes)-1]
		if lastIdx >= len(rLines) {
			return ""
		}
		return rLines[lastIdx].lineEnding
	}

	// DokuWiki image syntax recognition (#521). Only apply for the
	// DokuWiki variant — the upstream WikiFilter is DokuWiki-only and
	// the `{{…}}` construct does not exist as inline syntax in
	// MediaWiki (which uses `[[File:…]]`, handled by the dedicated
	// MediaWiki image path).
	if r.cfg.Variant == VariantDokuWiki && dokuWikiImageRe.MatchString(text) {
		return ps.emitDokuWikiParagraphWithImages(ctx, r, ch, text, emitSkeletonRef, trailingLineEnding())
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
	text string, emitSkeletonRef func(string), trailingLineEnding string,
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
				// the document. Append the source line ending too so
				// the bare image line still terminates before the
				// blank-line skeleton chunk that the caller emits next.
				if r.skeletonStore != nil {
					r.skelText(text)
					if trailingLineEnding != "" {
						r.skelText(trailingLineEnding)
					}
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
	// the translatable text spans. Tokenise the whole paragraph first
	// — that way a wrapping NAMED_LINK (`[[url|…]]` whose alt text is
	// the image) splits cleanly into PcOpen / inner / PcClose, with
	// the URL preserved in PcOpen.Data instead of pseudo-translating
	// as plain text. The image substitution then walks the run list
	// and rewrites any TextRun whose contents include a `{{…}}`
	// match into a TextRun + Ph + … sequence, so the placeholder for
	// the image still carries its untranslated bytes.
	ps.blockID++
	blockID := fmt.Sprintf("tu%d", ps.blockID)
	emitSkeletonRef(blockID)

	runs, changed := splitDokuWikiInlineRuns(text)
	if !changed {
		runs = []model.Run{{Text: &model.TextRun{Text: text}}}
	}
	runs = replaceDokuWikiImagesInRuns(runs)

	block := model.NewRunsBlock(blockID, runs)
	return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// replaceDokuWikiImagesInRuns walks `runs` and rewrites each TextRun
// whose contents contain one or more `{{…}}` image matches into a
// sequence of TextRun + PlaceholderRun (or PcOpen / TextRun(caption)
// / PcClose for captioned images) runs. Non-text runs pass through
// unchanged so PcOpen / PcClose markers (e.g. NAMED_LINK wrappers
// around an image) keep their captured Data — the writer renders
// them via RenderRunsWithData and the round-trip stays byte stable.
// Mirrors the okapi WikiPatterns IMAGE_START / IMAGE_END inline-code
// emission, including the IMAGE_CAPTION_PATTERN property hook that
// surfaces the caption as translatable text embedded inside the
// inline code.
func replaceDokuWikiImagesInRuns(runs []model.Run) []model.Run {
	out := make([]model.Run, 0, len(runs))
	idCounter := 0
	for _, run := range runs {
		if run.Text == nil {
			out = append(out, run)
			continue
		}
		text := run.Text.Text
		matches := dokuWikiImageRe.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			out = append(out, run)
			continue
		}
		cursor := 0
		for _, m := range matches {
			if m[0] > cursor {
				out = append(out, model.Run{Text: &model.TextRun{Text: text[cursor:m[0]]}})
			}
			imgRaw := text[m[0]:m[1]]
			if _, caption := splitDokuWikiImage(imgRaw); strings.TrimSpace(caption) != "" {
				inner := strings.TrimSuffix(strings.TrimPrefix(imgRaw, "{{"), "}}")
				pipe := strings.Index(inner, "|")
				openerEnd := m[0] + 2 + pipe + 1
				closerStart := m[1] - 2
				idCounter++
				openID := fmt.Sprintf("phimg%d", idCounter)
				out = append(out, model.Run{PcOpen: &model.PcOpenRun{
					ID:   openID,
					Type: "wiki:image",
					Data: text[m[0]:openerEnd],
				}})
				captionText := text[openerEnd:closerStart]
				if captionText != "" {
					out = append(out, model.Run{Text: &model.TextRun{Text: captionText}})
				}
				out = append(out, model.Run{PcClose: &model.PcCloseRun{
					ID:   openID,
					Type: "wiki:image",
					Data: text[closerStart:m[1]],
				}})
			} else {
				idCounter++
				out = append(out, model.Run{Ph: &model.PlaceholderRun{
					ID:    fmt.Sprintf("phimg%d", idCounter),
					Type:  "image",
					Data:  imgRaw,
					Equiv: imgRaw,
				}})
			}
			cursor = m[1]
		}
		if cursor < len(text) {
			out = append(out, model.Run{Text: &model.TextRun{Text: text[cursor:]}})
		}
	}
	return out
}

// splitDokuWikiImage splits a `{{name|caption}}` construct into its
// name and caption components. Returns the caption as the empty string
// when the construct has no `|`.
func splitDokuWikiImage(raw string) (name, caption string) {
	// Strip the `{{` prefix and `}}` suffix.
	inner := strings.TrimSuffix(strings.TrimPrefix(raw, "{{"), "}}")
	if before, after, ok := strings.Cut(inner, "|"); ok {
		return before, after
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
	text := model.RunsText(b.Source)
	// Cheap fast-path: skip the regex scan when the block has
	// none of the inline opener characters our tokeniser knows
	// about (`[`, `{`, `<`, plus the symmetric markers `*`, `_`,
	// `'`, `/`).
	if !strings.ContainsAny(text, "[{<*_'/~%") {
		return
	}
	runs, changed := splitDokuWikiInlineRuns(text)
	if changed {
		b.Source = runs
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
			// `<nowiki>` (and the `%%…%%` shorthand) suppresses
			// inline-code recognition for its contents — okapi's
			// parseInlineCodes flips its `enabled` flag off when it
			// enters a noWiki span. Mirror that by emitting the raw
			// text run as plain text, with no recursive tokenisation,
			// so embedded markers like `~~NOTOC~~` stay translatable.
			if codeType == "wiki:nowiki" {
				runs = append(runs, model.Run{Text: &model.TextRun{Text: altText}})
			} else {
				// Recurse so nested inline constructs (e.g. `**__bold+
				// underline__**`) get tokenised too.
				inner, innerChanged := splitDokuWikiInlineRuns(altText)
				if innerChanged {
					runs = append(runs, inner...)
				} else {
					runs = append(runs, model.Run{Text: &model.TextRun{Text: altText}})
				}
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
	// markerNext caches, per opener, the absolute offset of its next
	// occurrence at-or-after the current `scan` position (-1 = none left).
	// Re-Indexing all 9 markers from `scan` on every iteration is O(n) per
	// loop and the loop advances by as little as one byte on an unmatched
	// opener, so a marker-dense paragraph (`** ** ** …`, `// // // …`) was
	// O(n²) in paragraph length (#608, N2). Because markers are fixed
	// substrings, any cached offset that is still >= `scan` remains the
	// true next occurrence; we only re-search a marker once `scan` passes
	// its cached offset, making the total work linear in paragraph length.
	markerNext := make([]int, len(dokuWikiInlineMarkers))
	for i, m := range dokuWikiInlineMarkers {
		if idx := strings.Index(text, m); idx >= 0 {
			markerNext[i] = idx
		} else {
			markerNext[i] = -1
		}
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
		// `~~` opens DokuWiki macro placeholders such as
		// `~~NOTOC~~` / `~~NOCACHE~~` / `~~INFO:<word>~~` — the
		// macro match validates the full token.
		bestStart := -1
		var bestKind string
		for i, m := range dokuWikiInlineMarkers {
			// Refresh any cached offset that `scan` has advanced past.
			if markerNext[i] >= 0 && markerNext[i] < scan {
				if idx := strings.Index(text[scan:], m); idx >= 0 {
					markerNext[i] = scan + idx
				} else {
					markerNext[i] = -1
				}
			}
			if markerNext[i] < 0 {
				continue
			}
			rel := markerNext[i] - scan
			if bestStart < 0 || rel < bestStart {
				bestStart = rel
				bestKind = m
			}
		}
		if bestStart < 0 {
			break
		}
		absStart := scan + bestStart

		matched := false

		// HTML-style paired tag (case-insensitive sub/sup/del/nowiki).
		if bestKind == "<" {
			if openLoc := dokuWikiHTMLOpenAnchoredRe.FindStringSubmatchIndex(text[absStart:]); openLoc != nil {
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
		// Bare images (no caption) emit a single Ph; images with a
		// caption split into PcOpen / TextRun(caption) / PcClose so
		// the caption text gets pseudo-translated like other inline
		// strings while the image filename stays in the opener Data.
		// Mirrors okapi WikiPatterns IMAGE_START with the
		// IMAGE_CAPTION_PATTERN property: the caption surfaces as a
		// PropertyTextUnitPlaceholder, i.e. translatable text
		// embedded in an opaque inline code.
		if bestKind == "{{" {
			if loc := dokuWikiImageAnchoredRe.FindStringIndex(text[absStart:]); loc != nil {
				imgRaw := text[absStart : absStart+loc[1]]
				if name, caption := splitDokuWikiImage(imgRaw); strings.TrimSpace(caption) != "" {
					// Reconstruct the opener (`{{` + leading whitespace
					// + name + `|`) so the writer round-trips spacing.
					inner := strings.TrimSuffix(strings.TrimPrefix(imgRaw, "{{"), "}}")
					pipe := strings.Index(inner, "|")
					_ = name // kept for documentation; opener slice is
					//        // computed from `pipe` to preserve spacing.
					openerEnd := absStart + 2 + pipe + 1
					closerStart := absStart + loc[1] - 2
					emitPaired(
						absStart,
						openerEnd,
						closerStart,
						absStart+loc[1],
						"wiki:image",
						text[absStart:openerEnd],
						text[closerStart:absStart+loc[1]],
					)
					matched = true
				} else {
					flushTextUpTo(absStart)
					idCounter++
					runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
						ID:   fmt.Sprintf("ph%d", idCounter),
						Type: "wiki:image",
						Data: imgRaw,
					}})
					lastEmit = absStart + loc[1]
					scan = lastEmit
					changed = true
					matched = true
				}
			}
		}

		if !matched && bestKind == "[[" {
			// Try named link `[[target|alt]]`.
			if startLoc := dokuWikiNamedLinkStartAnchoredRe.FindStringIndex(text[absStart:]); startLoc != nil {
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
				if loc := dokuWikiLinkAnchoredRe.FindStringIndex(text[absStart:]); loc != nil {
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

		// DokuWiki macro (`~~NOTOC~~` / `~~NOCACHE~~` /
		// `~~INFO:<word>~~`). Single placeholder run.
		if !matched && bestKind == "~~" {
			if loc := dokuWikiMacroAnchoredRe.FindStringIndex(text[absStart:]); loc != nil {
				flushTextUpTo(absStart)
				idCounter++
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
					ID:   fmt.Sprintf("ph%d", idCounter),
					Type: "wiki:macro",
					Data: text[absStart : absStart+loc[1]],
				}})
				lastEmit = absStart + loc[1]
				scan = lastEmit
				changed = true
				matched = true
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
		// Okapi WikiFilter shape detection (variant-agnostic in upstream):
		// when the file has at least one `^|`/`^\^` line (TABLE_START
		// match) but TABLE_END (`[\^|]\s*\n`) never matches anywhere, the
		// entire body from the first table-start line through EOF is one
		// giant table block whose cells are split on every `|`/`^`
		// character. Mirrors WikiFilter.parseBlocks taking the
		// `t.toString()` of the unbalanced TABLE_START token (which
		// extends to EOS when no suffix is found, per
		// PrefixSuffixTokenizer / Token). Without this branch, MediaWiki
		// Infobox templates round-trip through the line-based paragraph
		// path and preserve interior blank lines / HTML comments
		// verbatim — the okapi reference folds them into the previous
		// cell via WhitespaceAdjustingEventBuilder. Detection runs for
		// both variants because okapi's WikiFilter applies the same
		// TABLE_START/TABLE_END logic regardless of dialect; the
		// dokuwiki.wiki fixture sidesteps this branch because every
		// `^ Header ^` row matches TABLE_END.
		if firstTableLine := mediaWikiOkapiFirstTableStartLine(rLines); firstTableLine >= 0 &&
			!mediaWikiOkapiHasTableEnd(rLines) {
			r.readMediaWikiInfobox(ctx, ch, rLines, ps, firstTableLine)
		} else if r.cfg.Variant == VariantDokuWiki {
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
			storeHeaderLayout(block, line)
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
			storeHeaderLayout(block, line)
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

// mediaWikiOkapiFirstTableStartLine returns the index of the first line
// matching okapi WikiPatterns TABLE_START (`^\|` or `^\^` not followed by
// `_^`). Returns -1 when no line matches.
func mediaWikiOkapiFirstTableStartLine(rLines []rawLine) int {
	for i, rl := range rLines {
		s := rl.content
		if len(s) == 0 {
			continue
		}
		if s[0] == '|' {
			return i
		}
		if s[0] == '^' {
			// Reject `^_^` start to mirror `^\^(?!_\^)`.
			if !(len(s) >= 3 && s[1] == '_' && s[2] == '^') {
				return i
			}
		}
	}
	return -1
}

// mediaWikiOkapiHasTableEnd reports whether the rLines content contains
// any okapi-style TABLE_END match (`[\^|][ \t]*\r?\n` — a `|`/`^` followed
// by inline whitespace and a newline). Joined view is checked rather than
// each line individually so we mirror the multi-line regex semantics of
// `[\^|]\s*\n` against the whole document body.
func mediaWikiOkapiHasTableEnd(rLines []rawLine) bool {
	var sb strings.Builder
	for _, rl := range rLines {
		sb.WriteString(rl.content)
		sb.WriteString(rl.lineEnding)
	}
	return mediaWikiOkapiTableEndRe.MatchString(sb.String())
}

// readMediaWikiInfobox handles the okapi WikiFilter "unbalanced
// TABLE_START → EOF" shape: prefix lines (before the first `|`/`^` line)
// route through the regular line-based MediaWiki parser; the rest of the
// file is treated as one giant table whose cells are split on every
// `|`/`^` character (after extracting `[[...]]` / `{{...}}` /
// `<nowiki>...</nowiki>` / `%%...%%` regions so their interior `|`s don't
// split a link or template). Each non-blank cell becomes one Block whose
// source text mirrors WhitespaceAdjustingEventBuilder: leading/trailing
// whitespace runs flow to the skeleton, interior whitespace runs collapse
// to a single space.
func (r *Reader) readMediaWikiInfobox(ctx context.Context, ch chan<- model.PartResult,
	rLines []rawLine, ps *parseState, firstTableLine int) {

	// Drive prefix lines through the variant's per-line parser so
	// headers / bare paragraphs / blank-line separators behave identically.
	// We slice rLines and route to the same line-based helper a top-level
	// call would have used (minus the Infobox detection branch, which by
	// construction can't re-fire on the prefix slice — the firstTableLine
	// row is excluded).
	if firstTableLine > 0 {
		if r.cfg.Variant == VariantDokuWiki {
			r.readDokuWikiLines(ctx, ch, rLines[:firstTableLine], ps)
		} else {
			r.readMediaWikiLinesNoInfobox(ctx, ch, rLines[:firstTableLine], ps)
		}
	}

	// Re-assemble the giant table body verbatim (content + line ending
	// per row). The trailing rLines entry has an empty lineEnding when the
	// file lacks a final newline, so the assembled string preserves the
	// source byte stream exactly.
	var bodyB strings.Builder
	for i := firstTableLine; i < len(rLines); i++ {
		bodyB.WriteString(rLines[i].content)
		bodyB.WriteString(rLines[i].lineEnding)
	}
	body := bodyB.String()

	// TEMP_EXTRACT: pull out `%%...%%` / `<nowiki>...</nowiki>` / `[[...]]`
	// / `{{...}}` regions (single-line, reluctant) and replace each with a
	// placeholder rune so cell-splitting on `|`/`^` doesn't slice through
	// the middle of a link target or template parameter list.
	const placeholder = '￼'
	var extracted []string
	post := mediaWikiTempExtractRe.ReplaceAllStringFunc(body, func(m string) string {
		extracted = append(extracted, m)
		return string(placeholder)
	})

	// Restore extracted spans in segment text — each placeholder rune
	// consumes the next saved capture in document order. extractedIdx is
	// closed-over so successive emitCell invocations advance through the
	// shared list, mirroring okapi's `extracted.pop()` in
	// WikiFilter.replaceExtracted.
	extractedIdx := 0
	restoreExtracted := func(s string) string {
		if extractedIdx >= len(extracted) || !strings.ContainsRune(s, placeholder) {
			return s
		}
		var b strings.Builder
		b.Grow(len(s))
		for _, ch := range s {
			if ch == placeholder && extractedIdx < len(extracted) {
				b.WriteString(extracted[extractedIdx])
				extractedIdx++
				continue
			}
			b.WriteRune(ch)
		}
		return b.String()
	}

	// Walk the post-extract string, splitting on every `|`/`^` character.
	// Each `|`/`^` is a cell delimiter (skeleton); the run between two
	// delimiters (or between end of last delimiter and EOF) is a cell.
	// Mirrors okapi DelimiterTokenizer(TABLE_CELL_PATTERN, body) behaviour
	// where the first segment is the prefix before the first delimiter.
	emitCell := func(seg string) bool {
		// Apply WhitespaceAdjustingEventBuilder: split off leading and
		// trailing whitespace runs (skeleton), collapse interior whitespace
		// runs (between non-WS chars) to single spaces (cell text). Empty
		// or whitespace-only cells degenerate into pure skeleton.
		seg = restoreExtracted(seg)
		if seg == "" {
			return true
		}
		// Front whitespace run
		front := 0
		for front < len(seg) && isWikiSpace(rune(seg[front])) {
			front++
		}
		if front == len(seg) {
			// All whitespace → skeleton.
			r.skelText(seg)
			return true
		}
		// Back whitespace run
		back := len(seg)
		for back > front && isWikiSpace(rune(seg[back-1])) {
			back--
		}
		body := seg[front:back]
		// Collapse interior whitespace runs (between non-WS chars) to a
		// single space. Mirrors WHITESPACE_COLLAPSE = `(?<=\S)\s+(?=\S)`.
		collapsed := collapseInteriorWhitespace(body)
		if seg[:front] != "" {
			r.skelText(seg[:front])
		}
		ps.blockID++
		blockID := fmt.Sprintf("tu%d", ps.blockID)
		r.skelRef(blockID)
		block := model.NewBlock(blockID, collapsed)
		block.Name = "infobox-cell"
		// Run inline-code tokenisation so embedded `[[link|alt]]` /
		// `{{template|param}}` survive pseudo-translation as opaque
		// codes rather than being mangled character-by-character.
		tokenizeDokuWikiInlineCodes(block)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
		if seg[back:] != "" {
			r.skelText(seg[back:])
		}
		return true
	}

	// Iterate post-extract string. The character set is ASCII for the
	// delimiters (`|`/`^`) and the placeholder is U+FFFC (multi-byte UTF-8),
	// so byte-level scanning is safe — we only key off the two ASCII bytes.
	start := 0
	for i := range len(post) {
		c := post[i]
		if c != '|' && c != '^' {
			continue
		}
		// Emit segment [start, i) as a cell, then `c` as skeleton.
		if !emitCell(post[start:i]) {
			return
		}
		r.skelText(string(c))
		start = i + 1
	}
	// Trailing segment after last delimiter (or whole body if no
	// delimiter — but the caller guarantees at least one `|`/`^` in the
	// firstTableLine row, so the loop fires).
	if start < len(post) {
		if !emitCell(post[start:]) {
			return
		}
	}
}

// readMediaWikiLinesNoInfobox runs the line-based MediaWiki parser
// without the Infobox (unbalanced TABLE_START → EOF) detection — used to
// process the prefix portion of an Infobox file (everything before the
// first `^|`/`^\^` line). Mirrors readMediaWikiLines exactly minus the
// detection branch at the top, so prefix headers / paragraphs / blank
// lines round-trip identically.
func (r *Reader) readMediaWikiLinesNoInfobox(ctx context.Context, ch chan<- model.PartResult,
	rLines []rawLine, ps *parseState) {
	inTable := false
	for i, rl := range rLines {
		line := rl.content

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
			storeHeaderLayout(block, line)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}
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
		if mediaWikiTableEndRe.MatchString(line) {
			inTable = false
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}
		if inTable && mediaWikiTableRowRe.MatchString(line) {
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}
		if inTable && mediaWikiTableHeaderRe.MatchString(line) {
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
		if mediaWikiImageRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			r.extractImageCaptions(ctx, ch, line, ps)
			continue
		}
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
		ps.paraLines = append(ps.paraLines, line)
		ps.paraLineIdxes = append(ps.paraLineIdxes, i)
	}
	ps.flushParagraph(ctx, r, ch, rLines)
}

// handleMidLineUntranslatable handles a regular-text line that contains
// an untranslatable block opener (`<file>` / `<code>` / `<html>` /
// `<php>`) somewhere after its first column. The text before the opener
// is appended to the current paragraph and the paragraph is flushed as a
// translatable Block; the opener and everything after it on this line is
// non-translatable. If the matching closer is also present after the
// opener on the same line the block is closed immediately; otherwise the
// caller enters verbatim mode (signalled by a non-nil returned close
// pattern) until the closer is seen on a later line.
//
// `lineEnding` is the source line ending (skeleton mode only; "" for the
// no-skeleton scanner path). It returns the pattern that closes the block
// (nil when the block closed inline) and ok=false only when emission was
// cancelled.
func (r *Reader) handleMidLineUntranslatable(
	ctx context.Context, ch chan<- model.PartResult, ps *parseState,
	line, lineEnding string, start int, closeRe *regexp.Regexp, rLines []rawLine,
) (open *regexp.Regexp, ok bool) {
	before := line[:start]
	rest := line[start:]

	// Append the leading text to the current paragraph and flush so the
	// pre-tag prose becomes its own translatable Block (mirrors okapi's
	// parseTextUnits over the first DelimiterTokenizer token, which trims
	// the boundary whitespace adjacent to the cut). The whitespace that
	// abutted the tag is dropped from the source so `This is <file>…`
	// yields the TU "This is" rather than "This is ".
	if trimmedBefore := strings.TrimRight(before, " \t"); trimmedBefore != "" {
		ps.paraLines = append(ps.paraLines, trimmedBefore)
		if r.skeletonStore != nil {
			ps.paraLineIdxes = append(ps.paraLineIdxes, len(rLines)) // sentinel: no own line ending
		}
		// In skeleton mode the trimmed-off boundary whitespace must still
		// reach the writer so the line round-trips byte-for-byte; prepend
		// it to the non-translatable remainder below.
		if r.skeletonStore != nil {
			rest = before[len(trimmedBefore):] + rest
		}
	} else if r.skeletonStore != nil && before != "" {
		// All-whitespace prefix (e.g. leading indent) — keep it verbatim.
		rest = before + rest
	}
	if !ps.flushParagraph(ctx, r, ch, rLines) {
		return nil, false
	}

	// The opener and the remainder of the line are non-translatable. In
	// skeleton mode they must be reproduced verbatim; in the no-skeleton
	// mode a single Data part marks the structural separator.
	if r.skeletonStore != nil {
		r.skelText(rest + lineEnding)
	}
	if !ps.emitData(ctx, r, ch) {
		return nil, false
	}

	// Closed inline on the same line? Then no block stays open.
	if closeRe.MatchString(rest) {
		return nil, true
	}
	return closeRe, true
}

func (r *Reader) readDokuWiki(ctx context.Context, ch chan<- model.PartResult,
	scanner *bufio.Scanner, ps *parseState) {

	// untranslatableEnd tracks an open `<code>` / `<file>` / `<html>` /
	// `<php>` block. While non-nil, every line bypasses the regular
	// classifiers and is consumed verbatim until a line containing the
	// matching closer (e.g. `</code>`) is seen.
	var untranslatableEnd *regexp.Regexp

	// tagBody accumulates the verbatim body of an open tagged block when
	// ExtractNonTranslatableContent() is on; tagProps carries the lang/name
	// hints from the opener. codeBuf accumulates consecutive indented code
	// lines into a single RoleCode block. All are nil in the opt-out
	// (flag-off) mode, where behaviour is byte-identical to before.
	var (
		tagBody  *strings.Builder
		tagProps map[string]string
		codeBuf  *strings.Builder
	)
	// flushCode surfaces any accumulated indented-code block before the
	// next non-code construct is emitted. No-op (and zero stream change)
	// when not accumulating.
	flushCode := func() bool {
		if codeBuf == nil {
			return true
		}
		text := codeBuf.String()
		codeBuf = nil
		return r.emitDokuWikiCode(ctx, ch, ps, text, nil)
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Inside an untranslatable block. With surfacing on, accumulate
		// the verbatim body and emit it as one RoleCode block at the
		// closer; otherwise emit a Data part per line so the no-skeleton
		// path still surfaces structural separators.
		if untranslatableEnd != nil {
			if tagBody != nil {
				if untranslatableEnd.MatchString(line) {
					if !r.emitDokuWikiCode(ctx, ch, ps, tagBody.String(), tagProps) {
						return
					}
					tagBody = nil
					tagProps = nil
					untranslatableEnd = nil
				} else {
					if tagBody.Len() > 0 {
						tagBody.WriteByte('\n')
					}
					tagBody.WriteString(line)
				}
				continue
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			if untranslatableEnd.MatchString(line) {
				untranslatableEnd = nil
			}
			continue
		}

		// Open an untranslatable block when the line introduces one. With
		// surfacing on, a multi-line block's body is accumulated for a
		// RoleCode block (the opener/closer markers are dropped on this
		// normalized path); otherwise the opener is a Data marker and
		// subsequent lines route through the branch above until the
		// closer is seen.
		if endRe, _ := matchDokuWikiUntranslatableOpener(line); endRe != nil {
			if !flushCode() {
				return
			}
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			if r.cfg.ExtractNonTranslatableContent() && !endRe.MatchString(line) {
				untranslatableEnd = endRe
				tagBody = &strings.Builder{}
				tagProps = dokuWikiTagProps(line)
				continue
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			// Same line may also contain the closer (e.g.
			// `<code>foo</code>`); bail out of the block immediately
			// in that case.
			if !endRe.MatchString(line) {
				untranslatableEnd = endRe
			}
			continue
		}

		// Check for header
		if m := dokuWikiHeaderRe.FindStringSubmatch(line); m != nil {
			if !flushCode() {
				return
			}
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			ps.blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), strings.TrimSpace(m[2]))
			block.Name = "header"
			storeHeaderLayout(block, line)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// DokuWiki table row (well-formed: leading + trailing delimiter)
		if dokuWikiTableRe.MatchString(line) {
			if !flushCode() {
				return
			}
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
			if !flushCode() {
				return
			}
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

		// Code block (indented two-or-more spaces). With surfacing on,
		// consecutive indented lines accumulate into one RoleCode block;
		// otherwise each line is a Data part so it stays out of the
		// translatable surface while the no-skeleton write path still
		// serialises a structural separator.
		if dokuWikiCodeStartRe.MatchString(line) {
			if r.cfg.ExtractNonTranslatableContent() {
				if codeBuf == nil {
					if !ps.flushParagraph(ctx, r, ch, nil) {
						return
					}
					codeBuf = &strings.Builder{}
				}
				if codeBuf.Len() > 0 {
					codeBuf.WriteByte('\n')
				}
				codeBuf.WriteString(line)
				continue
			}
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// DokuWiki list item — each `  * <text>` / `  - <text>` line
		// becomes its own Block. Mirrors okapi's
		// LIST_ITEM_START_PATTERN behaviour (each item is a separate
		// translatable text unit). Without this, consecutive items get
		// folded into one paragraph and whitespace-collapsed to
		// `* a * b * c`.
		if m := dokuWikiListItemRe.FindStringSubmatch(line); m != nil {
			if !flushCode() {
				return
			}
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			itemText := strings.TrimSpace(m[2])
			if !r.cfg.PreserveWhitespace {
				itemText = collapseInteriorWhitespace(itemText)
			}
			if itemText == "" {
				if !ps.emitData(ctx, r, ch) {
					return
				}
				continue
			}
			ps.blockID++
			block := model.NewBlock(fmt.Sprintf("tu%d", ps.blockID), itemText)
			block.Name = "list-item"
			tokenizeDokuWikiInlineCodes(block)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// Blank line separates paragraphs
		if strings.TrimSpace(line) == "" {
			if !flushCode() {
				return
			}
			if !ps.flushParagraph(ctx, r, ch, nil) {
				return
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// Mid-line untranslatable block opener (`<file>` / `<code>` /
		// `<html>` / `<php>`): cut the text unit off at the opener.
		if start, closeRe, ok := findDokuWikiUntranslatableInLine(line); ok {
			if !flushCode() {
				return
			}
			open, cont := r.handleMidLineUntranslatable(ctx, ch, ps, line, "", start, closeRe, nil)
			if !cont {
				return
			}
			untranslatableEnd = open
			continue
		}

		// Regular text -- accumulate into paragraph
		if !flushCode() {
			return
		}
		ps.paraLines = append(ps.paraLines, line)
	}

	// Flush remaining code / paragraph
	if !flushCode() {
		return
	}
	ps.flushParagraph(ctx, r, ch, nil)
}

func (r *Reader) readDokuWikiLines(ctx context.Context, ch chan<- model.PartResult,
	rLines []rawLine, ps *parseState) {

	// untranslatableEnd: see readDokuWiki for rationale. Skeleton-mode
	// equivalent — each verbatim line goes straight to the skeleton
	// buffer so the round-trip writer reproduces the block bytes
	// exactly. Used only in the opt-out (flag-off) mode; with surfacing
	// on, tagged and indented code regions are consumed by look-ahead so
	// the body rides a single ref while the markers stay skeleton.
	var untranslatableEnd *regexp.Regexp

	// resumeAt skips lines already consumed by a look-ahead (the body and
	// closer of a tagged block, or the trailing lines of an indented code
	// run). isIndentedCode mirrors the per-line precedence: a tag opener
	// (handled earlier) is never folded into an indented code run.
	resumeAt := -1
	isIndentedCode := func(s string) bool {
		if !dokuWikiCodeStartRe.MatchString(s) {
			return false
		}
		endRe, _ := matchDokuWikiUntranslatableOpener(s)
		return endRe == nil
	}

	for i, rl := range rLines {
		if i < resumeAt {
			continue
		}
		line := rl.content

		if untranslatableEnd != nil {
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			if untranslatableEnd.MatchString(line) {
				untranslatableEnd = nil
			}
			continue
		}

		if endRe, _ := matchDokuWikiUntranslatableOpener(line); endRe != nil {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.skelText(rl.content + rl.lineEnding)
			if r.cfg.ExtractNonTranslatableContent() && !endRe.MatchString(line) {
				// Surface the multi-line body as one non-translatable
				// RoleCode block riding a ref; opener/closer markers
				// stay in the skeleton so the round-trip is byte-exact.
				j := i + 1
				for j < len(rLines) && !endRe.MatchString(rLines[j].content) {
					j++
				}
				var buf strings.Builder
				for k := i + 1; k < j && k < len(rLines); k++ {
					buf.WriteString(rLines[k].content + rLines[k].lineEnding)
				}
				if !r.emitDokuWikiSkelCode(ctx, ch, ps, buf.String(), dokuWikiTagProps(line)) {
					return
				}
				if j < len(rLines) {
					r.skelText(rLines[j].content + rLines[j].lineEnding)
				}
				resumeAt = j + 1
				continue
			}
			if !ps.emitData(ctx, r, ch) {
				return
			}
			if !endRe.MatchString(line) {
				untranslatableEnd = endRe
			}
			continue
		}

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
			storeHeaderLayout(block, line)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			continue
		}

		// DokuWiki table row (well-formed). Interleave skeleton chunks
		// and cell refs so pseudo'd cell content gets spliced back into
		// the row at write time. The leading and trailing `|` / `^`
		// delimiters, plus the inter-cell whitespace, all live in the
		// skeleton; only the trimmed cell body becomes a ref. Empty
		// cells (`||` / `|^` with nothing between) flush as raw
		// skeleton text so the row's column count still matches.
		if dokuWikiTableRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			r.emitDokuWikiTableRowSkeleton(ctx, ch, line, rl.lineEnding, ps)
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

		// Code block. With surfacing on, consecutive indented lines are
		// gathered into one non-translatable RoleCode block whose verbatim
		// body rides a single ref (byte-exact round-trip). Otherwise the
		// entire raw line (including its indent) goes to skeleton so the
		// round-trip writer outputs it verbatim with no pseudo pass
		// touching the contents.
		if dokuWikiCodeStartRe.MatchString(line) {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			if r.cfg.ExtractNonTranslatableContent() {
				var buf strings.Builder
				j := i
				for j < len(rLines) && isIndentedCode(rLines[j].content) {
					buf.WriteString(rLines[j].content + rLines[j].lineEnding)
					j++
				}
				if !r.emitDokuWikiSkelCode(ctx, ch, ps, buf.String(), nil) {
					return
				}
				resumeAt = j
				continue
			}
			r.skelText(rl.content + rl.lineEnding)
			if !ps.emitData(ctx, r, ch) {
				return
			}
			continue
		}

		// DokuWiki list item — see readDokuWiki for rationale. Skeleton
		// shape: leading `  * ` / `  - ` delimiter → skeleton text,
		// translatable item body → ref, trailing whitespace + line
		// ending → skeleton text. Round-trip writer reconstructs
		// `  * <pseudo'd text>\r\n` verbatim with the cell content
		// slotted in.
		if m := dokuWikiListItemRe.FindStringSubmatch(line); m != nil {
			if !ps.flushParagraph(ctx, r, ch, rLines) {
				return
			}
			leadSkel := m[1]
			body := m[2]
			trailLen := len(body) - len(strings.TrimRight(body, " \t"))
			trailSkel := body[len(body)-trailLen:] + rl.lineEnding
			itemText := strings.TrimSpace(body)
			if !r.cfg.PreserveWhitespace {
				itemText = collapseInteriorWhitespace(itemText)
			}
			if itemText == "" {
				r.skelText(rl.content + rl.lineEnding)
				if !ps.emitData(ctx, r, ch) {
					return
				}
				continue
			}
			r.skelText(leadSkel)
			ps.blockID++
			blockID := fmt.Sprintf("tu%d", ps.blockID)
			r.skelRef(blockID)
			r.skelText(trailSkel)
			block := model.NewBlock(blockID, itemText)
			block.Name = "list-item"
			tokenizeDokuWikiInlineCodes(block)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
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

		// Mid-line untranslatable block opener — see readDokuWiki. The
		// pre-tag prose becomes a ref; the opener + remainder of the line
		// (plus its line ending) goes to skeleton verbatim so the
		// round-trip writer reproduces the block bytes exactly.
		if start, closeRe, ok := findDokuWikiUntranslatableInLine(line); ok {
			open, cont := r.handleMidLineUntranslatable(ctx, ch, ps, line, rl.lineEnding, start, closeRe, rLines)
			if !cont {
				return
			}
			untranslatableEnd = open
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
	if slices.Contains(params, s) {
		return true
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

// emitDokuWikiTableRowSkeleton walks `line` (a well-formed dokuwiki
// table row matching `^[|^].*[|^]\s*$`) and emits an interleaved
// skeleton sequence: cell-delimiter bytes go straight to skeleton text,
// each non-empty trimmed cell body becomes one Block+skelRef so the
// pseudo-translated text gets spliced back into the row at write time.
//
// Without this routing the entire raw line is committed to skeleton
// up-front and the per-cell Blocks float free in the TM but never
// reach the writer — the round-trip output keeps the original English
// cells while the okapi reference shows pseudo'd ones.
func (r *Reader) emitDokuWikiTableRowSkeleton(ctx context.Context, ch chan<- model.PartResult,
	line, lineEnding string, ps *parseState) {

	i := 0
	n := len(line)
	for i < n {
		// Cell delimiter (`|` or `^`).
		if line[i] != '|' && line[i] != '^' {
			// Defensive: should never happen for a row matched by
			// dokuWikiTableRe. Push remainder to skeleton and bail.
			r.skelText(line[i:])
			break
		}
		// Emit the delimiter, then look ahead for the next delimiter
		// to determine the cell span. Skip over `[[…]]` and `{{…}}`
		// constructs whose interior `|` characters are inline-code
		// arguments (named-link target separators, image captions),
		// not cell separators. Mirrors okapi's TEMP_EXTRACT pre-pass
		// which pulls these spans out before TABLE_CELL_PATTERN
		// (`[\^|]`) splitting runs.
		j := i + 1
		for j < n {
			c := line[j]
			if c == '|' || c == '^' {
				break
			}
			if c == '[' && j+1 < n && line[j+1] == '[' {
				if k := strings.Index(line[j:], "]]"); k >= 0 {
					j += k + 2
					continue
				}
			}
			if c == '{' && j+1 < n && line[j+1] == '{' {
				if k := strings.Index(line[j:], "}}"); k >= 0 {
					j += k + 2
					continue
				}
			}
			j++
		}
		// `cellSpan` is the bytes between this delimiter and the next
		// (or EOL when no more delimiters appear). The leading
		// delimiter char goes to skeleton; the cell body is the
		// remainder, with leading and trailing whitespace also routed
		// to skeleton so the pseudo'd cell content slots back into
		// the original column width.
		r.skelText(line[i : i+1])
		cellSpan := line[i+1 : j]
		// All-whitespace (or empty) cell: no translatable body, just
		// route the run to skeleton. Avoids leadLen+trailLen overlap
		// when TrimLeft and TrimRight both consume the entire span.
		if strings.TrimSpace(cellSpan) == "" {
			r.skelText(cellSpan)
			i = j
			continue
		}
		leadLen := len(cellSpan) - len(strings.TrimLeft(cellSpan, " \t"))
		trailLen := len(cellSpan) - len(strings.TrimRight(cellSpan, " \t"))
		body := cellSpan[leadLen : len(cellSpan)-trailLen]
		if leadLen > 0 {
			r.skelText(cellSpan[:leadLen])
		}
		if body != "" {
			ps.blockID++
			blockID := fmt.Sprintf("tu%d", ps.blockID)
			r.skelRef(blockID)
			block := model.NewBlock(blockID, body)
			block.Name = "table-cell"
			tokenizeDokuWikiInlineCodes(block)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
		if trailLen > 0 {
			r.skelText(cellSpan[len(cellSpan)-trailLen:])
		}
		i = j
	}
	r.skelText(lineEnding)
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
//
// One DokuWiki-specific exception: an interior whitespace run that
// starts immediately after a `\\` (LINEBREAK marker) and contains a
// `\n` collapses to a single `\n` rather than a single space. Okapi's
// WikiPatterns LINEBREAK_START_PATTERN (`\\{2,}(?:\s+|$)`) extracts
// `\\` plus its following whitespace as an inline placeholder before
// WhitespaceAdjustingEventBuilder runs, so the marker's bytes — and
// any embedded line break — round-trip verbatim. Without this carve-out
// the dokuwiki paragraph
//
//	... line\\
//	or followed by\\ a whitespace ...
//
// would join into one line as `... line\\ or followed by\\ a whitespace
// ...`, dropping the linebreak that the `\\\n` marker should preserve.
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
		} else if isDokuWikiLineBreakRun(runes, i, j) {
			// `\\<whitespace>` LINEBREAK marker: preserve a single `\n`
			// when the run contains one (mirrors okapi's inline-marker
			// extraction described in the function comment). Falls through
			// to single-space collapse otherwise so `\\ ` round-trips.
			b.WriteByte('\n')
		} else {
			// Interior whitespace run between non-whitespace — collapse
			// to a single space.
			b.WriteByte(' ')
		}
		i = j
	}
	return b.String()
}

// isDokuWikiLineBreakRun reports whether the whitespace run runes[i:j]
// is the trailing whitespace of a `\\\s+` DokuWiki LINEBREAK marker AND
// contains at least one `\n`. The caller uses the answer to decide
// whether to collapse the run to ` ` or to `\n`.
func isDokuWikiLineBreakRun(runes []rune, i, j int) bool {
	// Need at least two preceding `\` runes to form `\\`.
	if i < 2 || runes[i-1] != '\\' || runes[i-2] != '\\' {
		return false
	}
	for k := i; k < j; k++ {
		if runes[k] == '\n' {
			return true
		}
	}
	return false
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
