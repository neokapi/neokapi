package mif

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for MIF (Maker Interchange Format) files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
	scope         extractScope // body-page extraction scope (set per Read)
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new MIF reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "mif",
			FormatDisplayName: "Adobe FrameMaker MIF",
			FormatMimeType:    "application/x-mif",
			FormatExtensions:  []string{".mif"},
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
		MIMETypes:  []string{"application/x-mif", "application/vnd.mif"},
		Extensions: []string{".mif"},
		Sniff: func(data []byte) bool {
			return len(data) >= 9 && string(data[:9]) == "<MIFFile "
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("mif: nil document or reader")
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

// mifStatement represents a parsed MIF statement.
type mifStatement struct {
	tag      string
	value    string
	children []*mifStatement
	raw      string // Original raw text for non-translatable parts.

	// rawValue is the literal source bytes between the opening backtick
	// and closing quote of a value-bearing tag (e.g. `<String 'foo'>`),
	// BEFORE in-string escape decoding. value is the decoded form fed
	// into translatable Blocks. The two differ only when the source uses
	// hex escapes (`\xNN `) that Hexadecimal.toString collapses to a
	// shorter Unicode equivalent (e.g. `\x09 ` -> `\n`). findStringPositions
	// uses rawValue to locate the original `<String>` bytes in rawText,
	// while extractParaRuns feeds value into the translatable run text;
	// the writer's escapeMIF then re-encodes value canonically on output
	// (mirrors okapi MIFEncoder.encode, which never emits the `\xNN`
	// form -- a tab in memory always serializes to `\t`, a LF always to
	// `\n`, regardless of how the source encoded them).
	rawValue string
}

// statementsWith returns the direct children of s whose tag equals
// identity. Mirrors okapi Statement.statementsWith (Statement.java:72-78).
func (s *mifStatement) statementsWith(identity string) []*mifStatement {
	if s == nil {
		return nil
	}
	var out []*mifStatement
	for _, c := range s.children {
		if c.tag == identity {
			out = append(out, c)
		}
	}
	return out
}

// firstStatementWith returns the first direct child of s whose tag equals
// identity, or nil. Mirrors okapi Statement.firstStatementWith
// (Statement.java:65-69).
func (s *mifStatement) firstStatementWith(identity string) *mifStatement {
	for _, c := range s.statementsWith(identity) {
		return c
	}
	return nil
}

// firstLiteral returns the literal value of the first direct child with
// the given tag (its decoded `value`), or "". Used to read scoping keys
// like <ID 8>, <TextRectID 8>, <Unique 12>, <TblID 5>, <PageType BodyPage>.
func (s *mifStatement) firstLiteral(identity string) string {
	if c := s.firstStatementWith(identity); c != nil {
		return c.value
	}
	return ""
}

// extractScope holds the body-page extraction scope computed from a MIF
// document, mirroring okapi's Extracts collector (Extracts.java). It
// records which TextFlows (by 1-based order number), tables (by TblID) and
// anchored frames (by Unique) are reachable from extractable pages.
//
// When pagesSpecified is false the document has no <Page> statements
// (snippet tests, hand-built fixtures); okapi then extracts ALL TextFlows
// and referent tables by default (Extracts.java:208-212), which the reader
// signals by leaving `unscoped` true so emission falls back to extracting
// every container.
type extractScope struct {
	unscoped  bool            // true → extract everything (no pages found)
	textFlows map[string]bool // extractable TextFlow order numbers ("1", "2", …)
	tables    map[string]bool // extractable table ids (TblID)
	frames    map[string]bool // extractable anchored-frame ids (Unique)
}

// pageTypeExtractable mirrors okapi Extracts.pageTypeExtractable
// (Extracts.java:127-132): a page type is in scope iff its category's
// Extract* flag is enabled.
func (r *Reader) pageTypeExtractable(pageType string) bool {
	switch pageType {
	case "LeftMasterPage", "RightMasterPage", "OtherMasterPage":
		return r.cfg.ExtractMasterPages
	case "ReferencePage":
		return r.cfg.ExtractReferencePages
	case "BodyPage":
		return r.cfg.ExtractBodyPages
	case "HiddenPage":
		return r.cfg.ExtractHiddenPages
	}
	return false
}

// textRectIDsFrom collects the <TextRect><ID> values of the given
// statements, recursing into nested <Frame> children. Mirrors okapi
// Extracts.textRectsFrom (Extracts.java:230-252).
func textRectIDsFrom(stmts []*mifStatement, into map[string]bool) {
	var innerFrames []*mifStatement
	for _, s := range stmts {
		for _, tr := range s.statementsWith("TextRect") {
			if id := tr.firstLiteral("ID"); id != "" {
				into[id] = true
			}
		}
		innerFrames = append(innerFrames, s.statementsWith("Frame")...)
	}
	if len(innerFrames) > 0 {
		textRectIDsFrom(innerFrames, into)
	}
}

// frameUniquesFrom collects the <Unique> ids of the given frame statements,
// recursing into nested <Frame> children. Mirrors okapi
// Extracts.addExtractableFrames (Extracts.java:292-308).
func frameUniquesFrom(frames []*mifStatement, into map[string]bool) {
	var inner []*mifStatement
	for _, f := range frames {
		if u := f.firstLiteral("Unique"); u != "" {
			into[u] = true
		}
		inner = append(inner, f.statementsWith("Frame")...)
	}
	if len(inner) > 0 {
		frameUniquesFrom(inner, into)
	}
}

// anchoredFrameTextRects maps each anchored frame's <ID> to the set of
// <TextRect><ID>s it (and its nested frames) owns. Mirrors okapi
// Extracts.anchoredFrameTextRects (Extracts.java:254-275).
func anchoredFrameTextRects(anchoredFrames []*mifStatement) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, f := range anchoredFrames {
		rects := map[string]bool{}
		for _, tr := range f.statementsWith("TextRect") {
			if id := tr.firstLiteral("ID"); id != "" {
				rects[id] = true
			}
		}
		if inner := f.statementsWith("Frame"); len(inner) > 0 {
			textRectIDsFrom(inner, rects)
		}
		if id := f.firstLiteral("ID"); id != "" {
			out[id] = rects
		}
	}
	return out
}

// paraLineReferences collects, across all <Para><ParaLine> of the content
// flows, the literal values of the given inline reference tag (e.g.
// "AFrame", "ATbl"). Mirrors okapi Extracts.anchoredReferences
// (Extracts.java:410-420).
func paraLineReferences(contentFlows []*mifStatement, referenceName string) map[string]bool {
	out := map[string]bool{}
	for _, flow := range contentFlows {
		for _, para := range flow.statementsWith("Para") {
			for _, pl := range para.statementsWith("ParaLine") {
				for _, ref := range pl.statementsWith(referenceName) {
					// The reference value is the first literal child OR the
					// inline value of the statement itself (e.g. <AFrame 12>).
					if ref.value != "" {
						out[ref.value] = true
					}
				}
			}
		}
	}
	return out
}

// tableContentFlows gathers the content flows of the given tables that
// hold translatable Paras: the title content, header cells, and body
// cells. Mirrors okapi Extracts.tableTitleContentFlows +
// tableContentFlowsOf (Extracts.java:377-397).
func tableContentFlows(tables []*mifStatement) []*mifStatement {
	var out []*mifStatement
	for _, tbl := range tables {
		// TblTitle > TblTitleContent
		for _, title := range tbl.statementsWith("TblTitle") {
			out = append(out, title.statementsWith("TblTitleContent")...)
		}
		// TblH/TblBody > Row > Cell > CellContent
		for _, root := range []string{"TblH", "TblBody"} {
			for _, section := range tbl.statementsWith(root) {
				for _, row := range section.statementsWith("Row") {
					for _, cell := range row.statementsWith("Cell") {
						out = append(out, cell.statementsWith("CellContent")...)
					}
				}
			}
		}
	}
	return out
}

// tableReferences gathers AFrame/ATbl references appearing inside the
// content flows of the given tables. Mirrors okapi
// Extracts.tableReferencesOf (Extracts.java:366-375).
func tableReferences(tables []*mifStatement, referenceName string) map[string]bool {
	return paraLineReferences(tableContentFlows(tables), referenceName)
}

// computeExtractScope mirrors okapi Extracts.from (Extracts.java:154-221):
// it scans the document for Pages, AFrames, Tbls and TextFlows, then
// resolves which TextFlows / tables / frames are reachable from the
// extractable (body/master/reference/hidden, per config) pages.
func (r *Reader) computeExtractScope(stmts []*mifStatement) extractScope {
	scope := extractScope{
		textFlows: map[string]bool{},
		tables:    map[string]bool{},
		frames:    map[string]bool{},
	}

	var pages []*mifStatement
	bodyTextRects := map[string]bool{}
	var anchoredFrames []*mifStatement
	var allTables []*mifStatement
	textFlows := map[string]*mifStatement{}
	pagesSpecified := false
	textFlowNumber := 0

	for _, s := range stmts {
		switch s.tag {
		case "Page":
			pagesSpecified = true
			pt := s.firstLiteral("PageType")
			if r.pageTypeExtractable(pt) {
				pages = append(pages, s)
				textRectIDsFrom([]*mifStatement{s}, bodyTextRects)
			}
		case "AFrames":
			anchoredFrames = append(anchoredFrames, s.statementsWith("Frame")...)
		case "Tbls":
			allTables = append(allTables, s.statementsWith("Tbl")...)
		case "TextFlow":
			textFlowNumber++
			textFlows[strconv.Itoa(textFlowNumber)] = s
		}
	}

	// Index anchored tables by TblID.
	anchoredTables := map[string]*mifStatement{}
	for _, tbl := range allTables {
		if id := tbl.firstLiteral("TblID"); id != "" {
			anchoredTables[id] = tbl
		}
	}

	if !pagesSpecified {
		// No page information at all: extract every TextFlow plus referent
		// tables. Snippet documents and hand-built fixtures land here.
		scope.unscoped = true
		return scope
	}

	// Extractable frames are those directly on extractable pages.
	var pageFrames []*mifStatement
	for _, p := range pages {
		pageFrames = append(pageFrames, p.statementsWith("Frame")...)
	}
	frameUniquesFrom(pageFrames, scope.frames)

	// Resolve referent TextFlows/tables/frames by walking the TextRect →
	// TextFlow → AFrame reference chain, mirroring okapi
	// scanForExtractableTextFlowsTablesAndFrames (Extracts.java:310-334).
	r.scanReferents(textFlows, bodyTextRects, anchoredFrames, anchoredTables, &scope)
	return scope
}

// scanReferents iteratively resolves the set of extractable TextFlows,
// tables and anchored frames given the current body TextRect set, mirroring
// okapi Extracts.scanForExtractableTextFlowsTablesAndFrames
// (Extracts.java:310-334). It loops (rather than recursing) until no new
// referent text rects appear.
func (r *Reader) scanReferents(
	textFlows map[string]*mifStatement,
	textRects map[string]bool,
	anchoredFrames []*mifStatement,
	anchoredTables map[string]*mifStatement,
	scope *extractScope,
) {
	afTextRects := anchoredFrameTextRects(anchoredFrames)
	seenTextRects := map[string]bool{}
	for k := range textRects {
		seenTextRects[k] = true
	}
	work := textRects
	for len(work) > 0 {
		// Referent TextFlows: those whose ParaLine has a TextRectID in work.
		referentFlows := map[string]*mifStatement{}
		for num, flow := range textFlows {
			if scope.textFlows[num] {
				continue
			}
			if textFlowReferencesRect(flow, work) {
				referentFlows[num] = flow
				scope.textFlows[num] = true
			}
		}
		// Referent tables: anchored tables referenced (ATbl) by the referent
		// flows.
		var referentFlowList []*mifStatement
		for _, f := range referentFlows {
			referentFlowList = append(referentFlowList, f)
		}
		tblRefs := paraLineReferences(referentFlowList, "ATbl")
		var referentTables []*mifStatement
		for id := range tblRefs {
			if tbl, ok := anchoredTables[id]; ok && !scope.tables[id] {
				scope.tables[id] = true
				referentTables = append(referentTables, tbl)
			}
		}
		// Tables referenced from within referent tables (nested ATbl).
		for id := range tableReferences(referentTables, "ATbl") {
			if tbl, ok := anchoredTables[id]; ok && !scope.tables[id] {
				scope.tables[id] = true
				referentTables = append(referentTables, tbl)
			}
		}
		// Anchored frames referenced (AFrame) by the referent flows + tables.
		frameRefs := paraLineReferences(referentFlowList, "AFrame")
		for id := range tableReferences(referentTables, "AFrame") {
			frameRefs[id] = true
		}
		var newFrames []*mifStatement
		for _, f := range anchoredFrames {
			if frameRefs[f.firstLiteral("ID")] {
				newFrames = append(newFrames, f)
			}
		}
		frameUniquesFrom(newFrames, scope.frames)

		// The referent frames expose new TextRects; loop on any unseen ones.
		next := map[string]bool{}
		for id := range frameRefs {
			for tr := range afTextRects[id] {
				if !seenTextRects[tr] {
					seenTextRects[tr] = true
					next[tr] = true
				}
			}
		}
		work = next
	}
}

// textFlowInScope reports whether the TextFlow with the given 1-based
// order number is in extraction scope. An unscoped document (no Page
// statements) extracts every TextFlow (okapi's no-page default,
// Extracts.java:208-212).
func (s extractScope) textFlowInScope(num int) bool {
	return s.unscoped || s.textFlows[strconv.Itoa(num)]
}

// tableInScope reports whether the table with the given TblID is in scope.
func (s extractScope) tableInScope(tblID string) bool {
	return s.unscoped || s.tables[tblID]
}

// frameInScope reports whether the anchored frame with the given Unique id
// is in scope.
func (s extractScope) frameInScope(unique string) bool {
	return s.unscoped || s.frames[unique]
}

// textFlowReferencesRect reports whether any of the TextFlow's
// Para/ParaLine carries a <TextRectID> in the given set. Mirrors okapi
// Extracts.referentTextFlows (Extracts.java:346-354).
func textFlowReferencesRect(flow *mifStatement, rects map[string]bool) bool {
	for _, para := range flow.statementsWith("Para") {
		for _, pl := range para.statementsWith("ParaLine") {
			if id := pl.firstLiteral("TextRectID"); id != "" && rects[id] {
				return true
			}
		}
	}
	return false
}

// stringRef records the byte position of a String value and its block association.
type stringRef struct {
	startOffset int // byte offset of the String value content start (after backtick)
	endOffset   int // byte offset of the String value content end (before quote)
	blockIdx    int // which block (0-based)
	stringIdx   int // which string within the block (0-based)
	// runOrdinal selects WHICH translatable text-group of the (possibly
	// multi-run) paragraph block this `<String>` slot renders. Since #615
	// composes a whole Para as ONE Block whose runs interleave text and
	// structural inline-code (Ph) placeholders for the `<Font>`/`<AFrame>`/…
	// statements that physically separate the source `<String>` tags, the
	// writer must split the block's text back at those structural boundaries
	// and write each text-group into its own `<String>` slot — mirroring
	// okapi, which serializes one TextUnit across multiple output `<String>`
	// statements with the code data (the `<Font …>` bytes) between them
	// (MIFFilter.processPara, MIFFilter.java:636-811). A value of -1 means
	// "render the whole block" (used by single-value items: VariableDef,
	// PgfNumFormat, TextLine String, MText).
	runOrdinal int
}

// elisionRange records a raw byte range that should be dropped from the
// skeleton output entirely. Used to elide `<Char Foo>` lines whose glyph
// value was inlined into the surrounding String (mirrors okapi's
// MIFFilter.processPara: `<Char>` text gets appended to paraTextBuf and
// the original `<Char>` statement is consumed without re-emission --
// MIFFilter.java:1116-1126), and the wrapper bytes of secondary
// `<String>` tags whose value was merged into the first String's run.
type elisionRange struct {
	startOffset int
	endOffset   int
}

// charRewrite records a `<Char Foo>` raw byte range that should be
// rewritten on output as `<String 'X'>` where X is the Char glyph's
// Unicode equivalent. Used for Char-only paragraphs (Cluster F:
// `<Char Cent>` -> `<String '¢'>`) and Char-adjacent-to-Marker
// cases (Cluster G: `<Char HardSpace>` -> `<String ' '>` on the same
// line as the following Marker). Mirrors okapi's processPara behavior
// where every Char glyph is converted to its literal in paraTextBuf and
// re-emitted as part of a `<String>` (MIFFilter.java:1116-1126 +
// writeParagraph rebuilds the String from paraTextBuf).
type charRewrite struct {
	startOffset int
	endOffset   int
	text        string // synthesized `<String 'X'>` text (with leading indent)
}

// skelOp is a single skeleton-emission operation. The skeleton-build
// loop in readContent merges (refs, elisions, rewrites) into a sorted
// ops slice keyed by start offset, then walks them to produce the
// skeleton stream.
type skelOp struct {
	start, end int
	kind       int // opRef / opElide / opRewrite
	refID      string
	rewriteOut string
}

const (
	opRef     = 0
	opElide   = 1
	opRewrite = 2
)

// charGlyphMap maps `<Char NAME>` to its Unicode literal, mirroring
// okapi's CharLiteralToken (CharLiteralToken.java:40-86). SoftHyphen
// returns "" because okapi explicitly drops it ("we remove those" -- see
// CharLiteralToken.java:48-49). Glyphs not in the map are passed through
// verbatim by extractParaRuns / Char-elision discovery (i.e. they remain
// in the skeleton as raw `<Char>` lines).
var charGlyphMap = map[string]string{
	"Tab":          "\t",
	"HardSpace":    " ",
	"SoftHyphen":   "", // okapi explicitly drops these
	"HardHyphen":   "‑",
	"DiscHyphen":   "\u00ad", // SOFT HYPHEN
	"NoHyphen":     "\u200d", // ZERO WIDTH JOINER
	"Cent":         "¢",
	"Pound":        "£",
	"Yen":          "¥",
	"EnDash":       "–",
	"EmDash":       "—",
	"Dagger":       "†",
	"DoubleDagger": "‡",
	"Bullet":       "•",
	"NumberSpace":  " ",
	"ThinSpace":    " ",
	"EnSpace":      " ",
	"EmSpace":      " ",
	// HardReturn is mapped to "\n" but only when extractHardReturnsAsText
	// is true; handled separately in extractParaRuns and charLiteral.
}

// charLiteral returns the Unicode literal for a `<Char NAME>` glyph,
// mirroring CharLiteralToken.toString. HardReturn returns "\n" only
// when hardReturnsAsText is true (matching okapi's gating in
// MIFFilter.processPara at line 740). Returns ("", false) for unknown
// glyphs so the caller can leave the original `<Char>` statement
// untouched in the skeleton.
func charLiteral(name string, hardReturnsAsText bool) (string, bool) {
	if name == "HardReturn" {
		if hardReturnsAsText {
			return "\n", true
		}
		return "", false
	}
	v, ok := charGlyphMap[name]
	return v, ok
}

// mifVersionPattern matches the leading decimal of a MIF version
// string, mirroring okapi's Document.Version PATTERN
// `^(\d+\.?\d{0,2})` (Document.java:117). The captured group is the
// numeric prefix used for the >= 8.0 comparison.
var mifVersionPattern = regexp.MustCompile(`^(\d+\.?\d{0,2})`)

// minSupportedMIFVersion mirrors okapi Document.Version
// MIN_SUPPORTED_VERSION (Document.java:118): FrameMaker 8.0 is the
// oldest structure/encoding the filter supports.
const minSupportedMIFVersion = 8.0

// mifFileVersion returns the value of the top-level <MIFFile NN> header,
// and whether one was present. The header is always the first top-level
// statement in a well-formed MIF document.
func mifFileVersion(stmts []*mifStatement) (string, bool) {
	for _, s := range stmts {
		if s.tag == "MIFFile" {
			return s.value, true
		}
	}
	return "", false
}

// validateMIFVersion rejects unsupported MIF document versions, mirroring
// okapi Document.Version.validate (Document.java:125-133). The version
// must start with a decimal number and that number must be >= 8.0;
// otherwise an error matching okapi's OkapiBadFilterInputException message
// ("Unsupported document version: <value>") is returned.
func validateMIFVersion(value string) error {
	const prefix = "mif: Unsupported document version: "
	m := mifVersionPattern.FindString(value)
	if m == "" {
		return errors.New(prefix + value)
	}
	// okapi parses the FULL value string with Double.valueOf, not just the
	// matched prefix, so a year form like "2015" yields 2015.0. Mirror that
	// by parsing the whole value; fall back to the matched prefix only when
	// the full value isn't a clean float (matching Java's tolerant
	// behaviour where the year forms are clean numbers).
	n, err := parseLeadingFloat(value)
	if err != nil {
		n, err = parseLeadingFloat(m)
		if err != nil {
			return errors.New(prefix + value)
		}
	}
	if n < minSupportedMIFVersion {
		return errors.New(prefix + value)
	}
	return nil
}

// parseLeadingFloat parses s as a float, mirroring Java's Double.valueOf
// for the clean numeric version strings MIF uses (`8.00`, `9.00`, `10.0`,
// `2015`). Returns an error for non-numeric input.
func parseLeadingFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// alwaysSkipTags are top-level MIF tags whose content is always non-translatable.
//
// Mirrors okapi MIFFilter.java:54 (TOPSTATEMENTSTOSKIP). AFrames and Page are
// NOT in this set — both are walked by processFramesAndTextLines in okapi
// (MIFFilter.java:395-399) to extract <TextLine> <String> values used in
// graphics frames anchored to FrameMaker pages and paragraphs.
var alwaysSkipTags = map[string]bool{
	"Units":                true,
	"ColorCatalog":         true,
	"ConditionCatalog":     true,
	"BoolCondCatalog":      true,
	"CombinedFontCatalog":  true,
	"FontCatalog":          true,
	"RulingCatalog":        true,
	"TblCatalog":           true,
	"Views":                true,
	"Document":             true,
	"BookComponent":        true,
	"InitialAutoNums":      true,
	"Dictionary":           true,
	"PgfCatalog":           true,
	"ElementDefCatalog":    true,
	"FmtChangeListCatalog": true,
	"DefAttrValuesCatalog": true,
	"AttrCondExprCatalog":  true,
}

// skipTag returns true if the tag should be skipped based on config.
func (r *Reader) skipTag(tag string) bool {
	if alwaysSkipTags[tag] {
		return true
	}
	switch tag {
	case "MasterPage":
		return !r.cfg.ExtractMasterPages
	case "ReferencePage":
		return !r.cfg.ExtractReferencePages
	case "Page":
		return !r.cfg.ExtractBodyPages
	case "VariableFormats":
		return !r.cfg.ExtractVariables
	case "XRefFormats":
		return !r.cfg.ExtractReferenceFormats
	}
	return false
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitErr(ctx, ch, fmt.Errorf("mif: read error: %w", err))
		return
	}
	rawText := string(data)

	stmts := parseMIF(rawText)

	// Version validation. Mirrors okapi Document.Version.validate
	// (Document.java:111-134): the <MIFFile NN.NN> header must parse to a
	// leading decimal that is >= 8.0. FrameMaker 7 and earlier (`7.00`,
	// `6.00`, …) use an encoding/structure the filter does not support, so
	// okapi throws OkapiBadFilterInputException("Unsupported document
	// version: <value>"). The bare-header snippet tests (e.g.
	// processesSupportedVersions / doesNotProcessUnsupportedVersions) and
	// every fixture pass through here. Newer FrameMaker uses the year form
	// (`2015`), which parses to 2015.0 and is accepted. The check runs
	// before any Part is emitted so an unsupported document yields a single
	// error result (matching okapi throwing during open()).
	if v, ok := mifFileVersion(stmts); ok {
		if err := validateMIFVersion(v); err != nil {
			r.emitErr(ctx, ch, err)
			return
		}
	}

	// Compute the body-page extraction scope (okapi Extracts.from): which
	// TextFlows / tables / anchored frames are reachable from the
	// extractable pages. Done once per document; emitStatements and
	// findStringPositions both consult r.scope so block emission and the
	// skeleton-ref scheme stay in lock-step.
	r.scope = r.computeExtractScope(stmts)

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "mif",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-mif",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	r.emitStatements(ctx, ch, stmts)

	// Build skeleton if needed. We always emit the skeleton (even with no
	// translatable refs) so the writer can reproduce the source verbatim
	// and reach TierByteEqual on fixtures whose translatable surface
	// happens to be empty — without this, the writer falls back to its
	// best-effort no-skeleton path and emits a near-empty stub.
	if r.skeletonStore != nil {
		refs, elisions, rewrites := r.findStringPositions(rawText, stmts)
		// Merge refs + elisions + rewrites into a single ordered op list
		// keyed by start offset. See skelOp / opRef / opElide /
		// opRewrite at package scope.
		ops := make([]skelOp, 0, len(refs)+len(elisions)+len(rewrites))
		for _, sr := range refs {
			ops = append(ops, skelOp{
				start: sr.startOffset,
				end:   sr.endOffset,
				kind:  opRef,
				refID: fmt.Sprintf("%d:%d:%d", sr.blockIdx, sr.stringIdx, sr.runOrdinal),
			})
		}
		for _, e := range elisions {
			ops = append(ops, skelOp{start: e.startOffset, end: e.endOffset, kind: opElide})
		}
		for _, rw := range rewrites {
			ops = append(ops, skelOp{
				start: rw.startOffset, end: rw.endOffset, kind: opRewrite,
				rewriteOut: rw.text,
			})
		}
		// Sort by start offset; for equal starts, refs come before
		// elisions (refs need their value bytes consumed first).
		sortSkelOps(ops)

		skelPos := 0
		for _, op := range ops {
			if op.start > skelPos {
				r.skelText(rawText[skelPos:op.start])
			}
			switch op.kind {
			case opRef:
				r.skelRef(op.refID)
			case opElide:
				// no output; just advance position
			case opRewrite:
				r.skelText(op.rewriteOut)
			}
			if op.end > skelPos {
				skelPos = op.end
			}
		}
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// paraInlineChar records the glyph name of a `<Char NAME>` statement
// that contributed inlined text to a paragraph run. The position of the
// Char inside the paragraph (relative to the surrounding `<String>`
// statements) is tracked via `afterStringSlot`, which is the index into
// the run's stringOffsetIndices slice that the Char appears after
// (-1 = the Char appears before all Strings in the run).
type paraInlineChar struct {
	name            string
	afterStringSlot int
}

// paraCharRewrite records a `<Char NAME>` statement that needs to be
// rewritten on output. Used by Cluster F (Char-only run) and Cluster G
// (Char-followed-by-Marker). The rewrite is applied during the
// monotonic scan in findStringPositions so the search cursor stays
// aligned with String positions.
type paraCharRewrite struct {
	name     string // raw `<Char NAME>` to find
	text     string // glyph value to emit inside `<String 'X'>`
	joinNext bool   // when true, drop the Char line's trailing `\n`+indent so the next sibling joins on the same line
}

// findStringPositions scans raw MIF content for <String `...'> and
// <VariableDef `...'> patterns and associates them with block indices
// based on the parsed statement tree. The block ordering must match the
// order in which emitStatements emits Block parts so that
// writeFromSkeleton's `blockIdx -> w.blocks[blockIdx]` lookup is
// correct.
//
// Returns three slices: (refs, elisions, rewrites).
//   - refs: byte spans where translatable text gets injected by the
//     writer.
//   - elisions: byte spans of `<Char Foo>` lines (and similar) that get
//     dropped from the skeleton because the glyph was inlined into a
//     surrounding String run (Cluster C: HardReturn, Cluster H: inter-
//     String Tab/HardSpace folding).
//   - rewrites: `<Char Foo>` lines that get rewritten as `<String 'X'>`
//     because the Char produced text in a paragraph that has no
//     surrounding String (Cluster F: Char-only Para) or because the
//     Char is immediately followed by a Marker that okapi attaches to
//     a synthesized String (Cluster G: HardSpace before Marker).
func (r *Reader) findStringPositions(rawText string, stmts []*mifStatement) ([]stringRef, []elisionRange, []charRewrite) {
	// Walk the top-level statement list once to enumerate translatable
	// items in emission order. Two kinds participate today:
	//   - Para under TextFlow/Tbls/Notes (each Para → 1 block, may have
	//     multiple <String> children inside its <ParaLine>s)
	//   - VariableDef under VariableFormats (each VariableDef → 1 block,
	//     exactly 1 string)
	// Both share the same `blockIdx:stringIdx` skeleton-ref scheme so the
	// writer can patch them uniformly.
	type itemInfo struct {
		blockIdx        int
		strings         []string // values in order
		searchTag       string   // "String" or "VariableDef" or "CharRewrite"
		inlineChars     []paraInlineChar
		paraCharRewrite []paraCharRewrite // when searchTag == "CharRewrite"
		// stringsAreRaw signals that `strings` contains source-form
		// bytes (post-quote, pre-decode). The search pattern uses them
		// verbatim instead of re-running escapeMIFForSearch. Used by
		// String items where the source may have `\xNN ` hex escapes
		// that decode to a different byte form (e.g. `\x09 ` -> `\n`).
		// PgfNumFormat items keep stringsAreRaw=false because they go
		// through processPgfCatalog with decoded values today; bridging
		// them to rawValue is a separate cluster.
		stringsAreRaw bool
		// runOrdinal: for "String" items that belong to a multi-run Para
		// block, the 0-based index of this run's text-group within the
		// block (so the writer renders only that group's text into the
		// `<String>` slot). -1 = render the whole block (single-value
		// items: VariableDef / PgfNumFormat / TextLine / MText, and Para
		// blocks that contain exactly one translatable run).
		runOrdinal int
		// literalSkeleton marks a "String" item for a NON-extractable run
		// (whitespace / building-block-only): it owns no translatable block,
		// but okapi still serializes the run's pieces into ONE skeleton
		// `<String>` via toMIFString (MIFFilter.java:789) — adjacent
		// `<String>`s merge and inlined `<Char>` glyphs fold in. The first
		// slot's value is rewritten to literalText (source, not translated)
		// rather than referenced to a block; the rest of the merge/inline
		// elisions fire exactly as for an extractable run.
		literalSkeleton bool
		literalText     string // merged source content for literalSkeleton items
	}
	var items []itemInfo
	var rewrites []charRewrite
	blockIdx := 0

	// Mirror exactly the recursion in processContainer +
	// processVariableFormats so the blockIdx of every translatable item
	// here matches the index assigned by emitStatements.
	var walkContainer func(stmt *mifStatement)
	walkContainer = func(stmt *mifStatement) {
		for _, child := range stmt.children {
			if child.tag == "Para" {
				// Pre-assign blockIdx for translatable Blocks in the
				// EMIT order used by processContainer (PgfNumFormat
				// inline → Markers → Runs). The items list itself is
				// later built in SOURCE order so the cursor advances
				// monotonically through the raw text.
				var pgfBlockIdxs []int // for each PgfNumFormat
				var pgfValues []string
				markerBlockIdxs := map[*mifStatement]int{} // by Marker statement
				// Walk pgf overrides first.
				for _, gc := range child.children {
					if gc.tag != "Pgf" {
						continue
					}
					for _, ggc := range gc.children {
						if ggc.tag == "PgfNumFormat" && ggc.value != "" {
							// Mirror processContainer's pgf-inline emit gate:
							// split on hard returns, run CodeFinder (+ the
							// ^[A-Z]: prefix rule), simplify, and emit only when
							// non-whitespace text survives. A PgfNumFormat of
							// only building blocks (the 896 autonumber catalog
							// row: `<n><a><$volnum>…`) yields no block on the
							// emit side, so it must get no item/blockIdx here —
							// otherwise the next `<String>` slot is handed the
							// wrong block and emptied.
							for _, seg := range r.splitFormatValueOnHardReturns(ggc.value) {
								if !r.formatValueEmitsBlock(seg, true) {
									continue
								}
								pgfBlockIdxs = append(pgfBlockIdxs, blockIdx)
								pgfValues = append(pgfValues, seg)
								blockIdx++
							}
						}
					}
				}
				// Pre-assign Marker block indices in source order.
				for _, gc := range child.children {
					if gc.tag != "ParaLine" {
						continue
					}
					for _, lc := range gc.children {
						if lc.tag != "Marker" {
							continue
						}
						if !r.extractMarker(lc) {
							continue
						}
						if markerTextValue(lc) == "" {
							continue
						}
						markerBlockIdxs[lc] = blockIdx
						blockIdx++
					}
				}
				// Mirror processContainer: split the para into runs at
				// inline-code boundaries. Each run with non-empty text
				// gets its own Block (and skeleton ref). Within a single
				// run, multiple `<String>` values still collapse into the
				// run's first String slot — the writer fills slot 0 and
				// elides the rest.
				runs := extractParaRuns(child, r.cfg.ExtractHardReturnsAsText)
				// Emit PgfNumFormat items first (they're at the head of
				// the Para in source order).
				for i, v := range pgfValues {
					items = append(items, itemInfo{
						blockIdx:   pgfBlockIdxs[i],
						strings:    []string{v},
						searchTag:  "PgfNumFormat",
						runOrdinal: -1,
					})
				}
				// Pre-assign blockIdx for each run, matched to the BLOCK
				// processContainer actually emits for this Para.
				//
				// REGRESSION FIX (regressed by 5bacf636, #615): #615 switched
				// processContainer/emitStatements to the inline-code model —
				// one Block per buildParaRuns *unit* (a whole Para, or a
				// hard-return-delimited segment of it), gated by
				// blockHasText() after the CodeFinder + CodeSimplifier run
				// (MIFFilter.processPara, MIFFilter.java:636-811 + tf.hasText()
				// at 781). findStringPositions, however, still assigned ONE
				// blockIdx per extractParaRuns *run* (split at every inline-code
				// boundary) and counted any run with non-empty text — including
				// whitespace-only runs (a lone `\n` from a HardReturn glyph) and
				// code-only runs (e.g. `<String `<$paratext\>'>` whose only
				// content is the `<\$.*?>` building-block code, Parameters.java:
				// 202). Those phantom blockIdxs drift the two manifests apart,
				// so the writer's `w.blocks[blockIdx]` lookup is off-by-N and the
				// translated text lands in the wrong (or no) `<String>` slot.
				//
				// The fix: count blockIdx the way emitStatements does — one per
				// EMITTED buildParaRuns unit. paraRunBlockIdxs groups the
				// extractParaRuns runs into the same hard-return segments
				// buildParaRuns uses and assigns each text-bearing run the
				// blockIdx of its segment's emitted unit (or -1 when the unit is
				// gated out / whitespace-only). All runs of one unit therefore
				// share a single blockIdx, and their `<String>` values coalesce
				// into that one Block's text on output (writer fills the first
				// text slot, elides the rest) — restoring the byte-faithful
				// round-trip while preserving #615's one-TextUnit-per-Para
				// extraction.
				var runBlockIdxs []int
				runBlockIdxs, blockIdx = r.paraRunBlockIdxs(child, runs, blockIdx)
				// runOrdinalOf[ri] = the 0-based index of run ri among the
				// text-bearing runs that share its blockIdx — i.e. which
				// text-group of the merged Para block this run becomes. The
				// writer uses it to render only that group's text into this
				// run's `<String>` slot (multi-run paras serialize one
				// TextUnit across several `<String>` statements, okapi
				// MIFFilter.processPara).
				runOrdinalOf := make([]int, len(runs))
				blockRunCount := map[int]int{}
				for ri := range runs {
					bi := runBlockIdxs[ri]
					if bi < 0 {
						runOrdinalOf[ri] = -1
						continue
					}
					runOrdinalOf[ri] = blockRunCount[bi]
					blockRunCount[bi]++
				}
				// Pre-build the run -> strings mapping AND collect
				// inline Char info per run.
				runStrings := make([][]string, len(runs))
				runInlineChars := make([][]paraInlineChar, len(runs))
				runCharRewrites := make([][]paraCharRewrite, len(runs))
				// Walk ParaLine children to collect allStrings + Char
				// info, then assign each Char to its owning run.
				type charSlot struct {
					name     string
					afterIdx int
					nextTag  string
				}
				var allStrings []string
				var allChars []charSlot
				// Map run-index by absolute String slot (allStrings
				// index): each String belongs to exactly one run via
				// its stringOffsetIndices.
				stringToRun := map[int]int{}
				for ri, run := range runs {
					for _, off := range run.stringOffsetIndices {
						stringToRun[off] = ri
					}
				}
				for _, gc := range child.children {
					if gc.tag != "ParaLine" {
						continue
					}
					for lcIdx, lc := range gc.children {
						switch lc.tag {
						case "String":
							// Use the raw source bytes for skeleton-ref
							// matching: findStringPositions searches rawText
							// for `<String 'rawValue'>` and a decoded value
							// (e.g. `\x09 ` -> `\n`) won't appear verbatim
							// in the source. value (decoded) feeds the
							// translatable Block elsewhere.
							allStrings = append(allStrings, lc.rawValue)
						case "Char":
							nextTag := ""
							if lcIdx+1 < len(gc.children) {
								nextTag = gc.children[lcIdx+1].tag
							}
							allChars = append(allChars, charSlot{
								name:     lc.value,
								afterIdx: len(allStrings) - 1,
								nextTag:  nextTag,
							})
						}
					}
				}
				// Build runStrings.
				for ri, run := range runs {
					var strs []string
					for _, off := range run.stringOffsetIndices {
						if off < len(allStrings) {
							strs = append(strs, allStrings[off])
						}
					}
					runStrings[ri] = strs
				}
				// Assign each Char to its owning run. A Char belongs to
				// the run that consumed it via extractParaRuns -- mirror
				// the same walk to find which run is "current" when the
				// Char is processed.
				charOwnerRun := make([]int, len(allChars))
				for i := range charOwnerRun {
					charOwnerRun[i] = -1
				}
				{
					// Re-walk ParaLine.children mirroring extractParaRuns
					// so that each Char's owner index reflects the run it
					// actually accumulates into. The walk maintains an
					// ephemeral "current run" via curText/curIndices and
					// advances the run-index `ri` at the same boundaries
					// extractParaRuns flushes on (inline-code tags). When
					// Char text accumulates in cur, we know it belongs to
					// the next non-empty run we'll flush -- which equals
					// `runs[ri]` at flush time.
					var curText string
					var curIndices []int
					ri := 0
					stringIdx := 0
					charIdx := 0
					inXRef := false
					var pendingChars []int // Char indexes accumulated since last flush
					flushCur := func() {
						if curText == "" {
							curText = ""
							curIndices = nil
							pendingChars = nil
							return
						}
						for _, ci := range pendingChars {
							charOwnerRun[ci] = ri
						}
						curText = ""
						curIndices = nil
						pendingChars = nil
						ri++
					}
					for _, gc := range child.children {
						if gc.tag != "ParaLine" {
							continue
						}
						for _, lc := range gc.children {
							switch lc.tag {
							case "String":
								if inXRef {
									stringIdx++
									continue
								}
								curText += lc.value
								curIndices = append(curIndices, stringIdx)
								stringIdx++
							case "Char":
								if inXRef {
									charIdx++
									continue
								}
								if lit, ok := charLiteral(lc.value, r.cfg.ExtractHardReturnsAsText); ok {
									curText += lit
									pendingChars = append(pendingChars, charIdx)
								}
								charIdx++
							default:
								flushCur()
								if lc.tag == "XRef" {
									inXRef = true
								} else if lc.tag == "XRefEnd" {
									inXRef = false
								}
							}
						}
					}
					flushCur()
				}
				// Now iterate runs and split inline-vs-rewrite chars.
				for ri, run := range runs {
					if run.text == "" {
						continue
					}
					strs := runStrings[ri]
					var inline []paraInlineChar
					var rewriteChars []paraCharRewrite
					for ci, ch := range allChars {
						if charOwnerRun[ci] != ri {
							continue
						}
						lit, ok := charLiteral(ch.name, r.cfg.ExtractHardReturnsAsText)
						// Always elide the `<Char>` line itself (mirrors
						// okapi's readTag at MIFFilter.java:1527-1532
						// which deletes the just-appended `<Char` from sb
						// before the literal is even read). The lit/ok
						// distinction only controls whether the Char
						// contributes a glyph to a synthesized String run
						// (rewriteChars) -- the source line elision
						// applies regardless.
						if !ok || lit == "" {
							if len(strs) > 0 {
								runSlot := -1
								for i, off := range run.stringOffsetIndices {
									if off <= ch.afterIdx {
										runSlot = i
									}
								}
								inline = append(inline, paraInlineChar{
									name:            ch.name,
									afterStringSlot: runSlot,
								})
							}
							continue
						}
						if len(strs) == 0 {
							rewriteChars = append(rewriteChars, paraCharRewrite{
								name:     ch.name,
								text:     lit,
								joinNext: ch.nextTag == "Marker",
							})
						} else {
							runSlot := -1
							for i, off := range run.stringOffsetIndices {
								if off <= ch.afterIdx {
									runSlot = i
								}
							}
							inline = append(inline, paraInlineChar{
								name:            ch.name,
								afterStringSlot: runSlot,
							})
						}
					}
					runInlineChars[ri] = inline
					runCharRewrites[ri] = rewriteChars
				}
				// Emit items in SOURCE order: walk ParaLine.children
				// sequentially and emit Marker / Run items as they
				// appear. Each Run is emitted ONCE, at the position of
				// its first contributing String or Char.
				runEmitted := make([]bool, len(runs))
				stringPosCursor := 0
				charPosCursor := 0
				for _, gc := range child.children {
					if gc.tag != "ParaLine" {
						continue
					}
					for _, lc := range gc.children {
						switch lc.tag {
						case "Marker":
							if bi, ok := markerBlockIdxs[lc]; ok {
								items = append(items, itemInfo{
									blockIdx:      bi,
									strings:       []string{markerTextRawValue(lc)},
									searchTag:     "MText",
									stringsAreRaw: true,
									runOrdinal:    -1,
								})
							}
						case "String":
							ri, ok := stringToRun[stringPosCursor]
							stringPosCursor++
							if !ok || runEmitted[ri] {
								continue
							}
							strs := runStrings[ri]
							if len(strs) == 0 {
								// Shouldn't happen for a String-driven
								// run, but keep defensive.
								continue
							}
							if runBlockIdxs[ri] < 0 {
								// Non-extractable run (whitespace / building-
								// block-only). okapi still merges its pieces
								// into ONE skeleton `<String>` (toMIFString,
								// MIFFilter.java:789): adjacent `<String>`s
								// coalesce and inlined `<Char>` glyphs fold in.
								// Emit a literal-skeleton item only when there
								// IS something to merge (>1 String, or an inline
								// Char glyph); a lone untouched `<String>` stays
								// verbatim with no item.
								if len(strs) <= 1 && len(runInlineChars[ri]) == 0 {
									continue
								}
								runEmitted[ri] = true
								items = append(items, itemInfo{
									blockIdx:        -1,
									strings:         strs,
									searchTag:       "String",
									inlineChars:     runInlineChars[ri],
									stringsAreRaw:   true,
									runOrdinal:      -1,
									literalSkeleton: true,
									literalText:     runs[ri].text,
								})
								continue
							}
							runEmitted[ri] = true
							items = append(items, itemInfo{
								blockIdx:      runBlockIdxs[ri],
								strings:       strs,
								searchTag:     "String",
								inlineChars:   runInlineChars[ri],
								stringsAreRaw: true,
								runOrdinal:    runOrdinalOf[ri],
							})
						case "Char":
							owner := charOwnerRun[charPosCursor]
							charPosCursor++
							if owner < 0 || runEmitted[owner] {
								continue
							}
							strs := runStrings[owner]
							if len(strs) == 0 {
								// Char-only run -- emit a CharRewrite item.
								// This is a pure skeleton rewrite (a whitespace
								// glyph like <Char HardReturn>/<Char Tab>
								// becomes its own `<String '\n'>` /
								// `<String '\t'>`), so it fires even when the
								// run owns NO translatable block (runBlockIdxs
								// == -1). okapi routes such whitespace-only
								// fragments to the skeleton via toMIFString
								// (MIFFilter.java:789); the rewrite reproduces
								// that without an extracted TextUnit.
								if len(runCharRewrites[owner]) == 0 {
									continue
								}
								runEmitted[owner] = true
								items = append(items, itemInfo{
									blockIdx:        runBlockIdxs[owner],
									searchTag:       "CharRewrite",
									paraCharRewrite: runCharRewrites[owner],
									runOrdinal:      -1,
								})
							} else if runBlockIdxs[owner] < 0 {
								// A text-bearing run that emits no block (gated
								// out): nothing to rewrite or reference.
								continue
							} else {
								runEmitted[owner] = true
								items = append(items, itemInfo{
									blockIdx:      runBlockIdxs[owner],
									strings:       strs,
									searchTag:     "String",
									inlineChars:   runInlineChars[owner],
									stringsAreRaw: true,
									runOrdinal:    runOrdinalOf[owner],
								})
							}
						}
					}
				}
				continue
			}
			if isMIFContainer(child.tag) {
				walkContainer(child)
			}
		}
	}
	walkVariableFormats := func(stmt *mifStatement) {
		for _, child := range stmt.children {
			if child.tag != "VariableFormat" {
				continue
			}
			var defStmt *mifStatement
			for _, gc := range child.children {
				if gc.tag == "VariableDef" {
					defStmt = gc
				}
			}
			if defStmt == nil {
				continue
			}
			items = append(items, itemInfo{
				blockIdx:   blockIdx,
				strings:    []string{defStmt.value},
				searchTag:  "VariableDef",
				runOrdinal: -1,
			})
			blockIdx++
		}
	}
	// PgfCatalog → PgfNumFormat extraction mirrors okapi
	// MIFFilter.java:357-362,1078-1095 with the extractable-PgfTag
	// filter from Extracts.java:449-456. Walked inline so file-order
	// (and blockIdx) tracks emitStatements' processPgfCatalog.
	extractable := extractablePgfTags(stmts)
	walkPgfCatalog := func(stmt *mifStatement) {
		for _, child := range stmt.children {
			if child.tag != "Pgf" {
				continue
			}
			var pgfTag string
			for _, gc := range child.children {
				if gc.tag == "PgfTag" {
					pgfTag = gc.value
					break
				}
			}
			if !extractable[pgfTag] {
				continue
			}
			for _, gc := range child.children {
				if gc.tag != "PgfNumFormat" || gc.value == "" {
					continue
				}
				// Mirror processPgfCatalog's emit gate: split on hard
				// returns, run the CodeFinder (+ the ^[A-Z]: prefix rule),
				// simplify, and emit a block only when non-whitespace text
				// survives (blockHasText). A PgfNumFormat that is only
				// building-block codes (e.g. `<$lastpagenum>`) yields no
				// block on the emit side, so it must get no item/blockIdx
				// here either — otherwise the catalog blockIdx counter
				// drifts and scrambles every later VariableDef/String slot
				// (the Test01 PgfNumFormat regression).
				for _, seg := range r.splitFormatValueOnHardReturns(gc.value) {
					if !r.formatValueEmitsBlock(seg, true) {
						continue
					}
					items = append(items, itemInfo{blockIdx: blockIdx, strings: []string{seg}, searchTag: "PgfNumFormat", runOrdinal: -1})
					blockIdx++
				}
			}
		}
	}
	// Page/AFrames/Frame → TextLine/String extraction mirrors okapi
	// MIFFilter.java:395-399 (top-level dispatch) +
	// 1629-1717 (processPage / processFramesAndTextLines /
	// processTextLine). Each TextLine with a <String> emits one
	// translatable item carrying just that single value. The recursive
	// descent must mirror processFramesAndTextLines so blockIdx stays in
	// lock-step with emitStatements.
	var walkFramesAndTextLines func(stmt *mifStatement)
	walkFramesAndTextLines = func(stmt *mifStatement) {
		for _, child := range stmt.children {
			switch child.tag {
			case "Frame":
				// Mirror processFramesAndTextLines' body-page scope gate
				// (Extracts.frameExtractable) so blockIdx tracks the emit side.
				if !r.scope.frameInScope(child.firstLiteral("Unique")) {
					continue
				}
				walkFramesAndTextLines(child)
			case "TextLine":
				rawVal, ok := firstStringRawValue(child)
				if !ok {
					continue
				}
				decVal, _ := firstStringValue(child)
				// Mirror processTextLine's emit gate: CodeFinder +
				// blockHasText. A TextLine String of only building-block
				// codes / whitespace yields no block, so no item/blockIdx.
				if !r.formatValueEmitsBlock(decVal, false) {
					continue
				}
				items = append(items, itemInfo{
					blockIdx:      blockIdx,
					strings:       []string{rawVal},
					searchTag:     "String",
					stringsAreRaw: true,
					runOrdinal:    -1,
				})
				blockIdx++
			}
		}
	}

	for _, stmt := range stmts {
		if stmt.tag == "MIFFile" {
			continue
		}
		switch stmt.tag {
		case "PgfCatalog":
			walkPgfCatalog(stmt)
		case "TextFlow", "Tbls", "Notes":
			if r.skipTag(stmt.tag) {
				continue
			}
			walkContainer(stmt)
		case "VariableFormats":
			if r.skipTag(stmt.tag) {
				continue
			}
			walkVariableFormats(stmt)
		case "Page":
			if r.skipPage(stmt) {
				continue
			}
			walkFramesAndTextLines(stmt)
		case "AFrames":
			walkFramesAndTextLines(stmt)
		}
	}

	// Now scan the raw text for the matching <Tag `value'> pattern for
	// each item in order.
	var refs []stringRef
	var elisions []elisionRange
	searchFrom := 0

	// findCharLine locates the next `<Char NAME>` line (with leading
	// indentation) at or after `from`. Returns the absolute byte span
	// covering the entire physical line (including the leading `\n` or
	// `\r\n` so the elision drops the line cleanly without stranding
	// whitespace). Returns -1 if no match found, or if the Char is
	// mid-line (in which case the elision is skipped to avoid breaking
	// surrounding content).
	findCharLine := func(from int, name string) (int, int) {
		needle := "<Char " + name + ">"
		idx := strings.Index(rawText[from:], needle)
		if idx < 0 {
			return -1, -1
		}
		abs := from + idx
		// Walk back over leading whitespace.
		start := abs
		for start > from {
			c := rawText[start-1]
			if c == ' ' || c == '\t' {
				start--
				continue
			}
			break
		}
		// At this point start should be at the start of the indentation
		// after a newline. If the preceding byte is `\n` (with optional
		// `\r` before it), include it in the elision so the line is
		// dropped without stranding a bare `\r\n`.
		if start > from && rawText[start-1] == '\n' {
			start--
			if start > from && rawText[start-1] == '\r' {
				start--
			}
		} else {
			// Char is mid-line (no preceding newline). Don't elide --
			// the surrounding context isn't a clean line drop.
			return -1, -1
		}
		end := abs + len(needle)
		// Note: we deliberately do NOT advance past the trailing `\r\n`
		// because the leading `\r\n` is already included via the
		// back-walk. The next line's leading whitespace + content
		// remains intact.
		return start, end
	}

	for _, it := range items {
		if it.searchTag == "CharRewrite" {
			// Char-only run: emit a rewrite per Char glyph. Each
			// rewrite replaces `<Char NAME>` with `<String 'X'>` (and
			// optionally drops the trailing `\n`+indent so the next
			// sibling joins on the same line). Mirrors okapi
			// processPara: paraTextBuf accumulates Char text and is
			// re-emitted as a fresh `<String>` (MIFFilter.java:
			// 739-741, 761).
			for _, rw := range it.paraCharRewrite {
				start, joinEnd := findCharLineForRewrite(rawText, searchFrom, rw.name, rw.joinNext)
				if start < 0 {
					continue
				}
				// Replacement text: indentation + `<String 'X'>`.
				out := buildCharRewriteReplacement(rawText, start, rw.text)
				rewrites = append(rewrites, charRewrite{
					startOffset: start,
					endOffset:   joinEnd,
					text:        out,
				})
				searchFrom = joinEnd
			}
			continue
		}
		if len(it.strings) == 0 {
			continue
		}
		// Locate each String value position monotonically.
		// it.strings carries source-form bytes (rawValue) for String
		// values and decoded values for PgfNumFormat. The flag below
		// records which: source-form skips re-escaping (raw bytes
		// already match the rawText), decoded reapplies escapeMIFForSearch.
		stringPositions := make([][2]int, 0, len(it.strings))
		for stringInItemIdx, expectedVal := range it.strings {
			var encoded string
			if it.stringsAreRaw {
				encoded = expectedVal
			} else {
				encoded = escapeMIFForSearch(expectedVal)
			}
			pattern := "<" + it.searchTag + " `" + encoded + "'>"
			idx := strings.Index(rawText[searchFrom:], pattern)
			if idx < 0 {
				continue
			}
			absIdx := searchFrom + idx
			valStart := absIdx + len("<"+it.searchTag+" `")
			valEnd := valStart + len(encoded)
			stringPositions = append(stringPositions, [2]int{absIdx, valEnd})
			searchFrom = valEnd

			if stringInItemIdx == 0 && it.literalSkeleton {
				// Non-extractable run merged into ONE skeleton `<String>`:
				// rewrite the first slot's value to the merged SOURCE content
				// (re-encoded) rather than referencing a translatable block.
				// The secondary-String elisions + inlined-Char elisions below
				// then collapse the run's remaining `<String>`/`<Char>` lines.
				rewrites = append(rewrites, charRewrite{
					startOffset: valStart,
					endOffset:   valEnd,
					text:        escapeMIFForSearch(it.literalText),
				})
			} else if stringInItemIdx == 0 {
				// First String in the item -- write the rendered text
				// into the value slot. runOrdinal selects which text-group
				// of the (possibly multi-run) Para block this slot renders.
				refs = append(refs, stringRef{
					startOffset: valStart,
					endOffset:   valEnd,
					blockIdx:    it.blockIdx,
					stringIdx:   0,
					runOrdinal:  it.runOrdinal,
				})
			} else if it.searchTag == "String" {
				// Non-first String in the same Para item -- the writer
				// emits empty text for stringIdx>0, but we also need to
				// elide the wrapper bytes between the previous value's
				// `'>` and this value's `'>` so the merged content
				// appears as a single `<String '...'>`.
				prevValEnd := stringPositions[len(stringPositions)-2][1]
				// Determine where the previous String's wrapper ends.
				// MIF wrapper close after the value is `'>` (2 bytes).
				closeStart := prevValEnd
				if closeStart+2 <= len(rawText) && rawText[closeStart] == '\'' && rawText[closeStart+1] == '>' {
					closeStart += 2
				}
				// The current String's wrapper end is just past `'>`.
				curValEnd := valEnd
				if curValEnd+2 <= len(rawText) && rawText[curValEnd] == '\'' && rawText[curValEnd+1] == '>' {
					curValEnd += 2
				}
				// Are the two Strings inside the SAME `<ParaLine>`? If
				// the bytes between them contain `> # end of ParaLine`
				// or `<ParaLine`, they are in different ParaLines and
				// the elision must extend to swallow the wrapping
				// `</ParaLine><ParaLine>` boundary so the output
				// collapses to a single ParaLine (mirrors okapi's
				// processPara unifying paraTextBuf across all ParaLines
				// of one Para -- MIFFilter.java:740-741).
				between := rawText[closeStart:absIdx]
				if strings.Contains(between, "> # end of ParaLine") || strings.Contains(between, "<ParaLine") {
					// Multi-ParaLine merge case. Mirrors okapi
					// MIFFilter.processPara (MIFFilter.java:740-741)
					// which appends ALL paraTextBuf content from every
					// ParaLine into one TextFragment and re-emits one
					// `<String>` -- the second ParaLine wrapper plus
					// its empty `<String>` placeholders never reach the
					// output skeleton.
					//
					// Concretely: keep the FIRST ParaLine and its
					// closing `> # end of ParaLine` line, drop the
					// `<ParaLine ...>` opener of the second AND its
					// closing `> # end of ParaLine` line. Find the END
					// of the first close line, set eraseStart there.
					tailFromClose := rawText[closeStart:]
					firstCloseIdx := strings.Index(tailFromClose, "> # end of ParaLine")
					tail := rawText[curValEnd:]
					closeIdx := strings.Index(tail, "> # end of ParaLine")
					if closeIdx < 0 {
						closeIdx = strings.Index(tail, "\n>")
						if closeIdx >= 0 {
							closeIdx++
						}
					}
					if firstCloseIdx >= 0 && closeIdx >= 0 {
						eraseStart := closeStart + firstCloseIdx + len("> # end of ParaLine")
						// Walk past the first close line's trailing
						// newline so we preserve the line termination
						// for the first close.
						for eraseStart < len(rawText) && rawText[eraseStart] != '\n' {
							eraseStart++
						}
						if eraseStart < len(rawText) && rawText[eraseStart] == '\n' {
							eraseStart++
						}
						eraseEnd := curValEnd + closeIdx + len("> # end of ParaLine")
						// Walk forward past the close line through its
						// trailing newline (so the line is fully
						// removed rather than leaving stray
						// indentation behind).
						for eraseEnd < len(rawText) && rawText[eraseEnd] != '\n' {
							eraseEnd++
						}
						if eraseEnd < len(rawText) && rawText[eraseEnd] == '\n' {
							eraseEnd++
						}
						if eraseEnd > eraseStart {
							// If the elision range contains any
							// `<ElementEnd ...>` lines, those structure-
							// tag boundaries must survive the merge
							// (mirrors okapi MIFFilter.processPara
							// MIFFilter.java:1044-1066 + 1145-1153 where
							// non-extracted statements between Strings
							// fall through "default: skip over" and
							// accumulate in paraCodeBuf -- per MIF
							// Reference §"Element Statements",
							// `<ElementEnd>` is a structure-tag boundary
							// that must not be silently dropped during
							// ParaLine collapse).
							//
							// Per okapi's per-line emission order, the
							// preserved `<ElementEnd>` lines must sit
							// BEFORE the surviving ParaLine close. To
							// achieve that, we shift the elision boundary
							// to drop the FIRST close instead of the
							// second: the second close becomes the
							// surviving close and any preserved
							// `<ElementEnd>` lines naturally precede it.
							if hasElementEndLine(rawText, eraseStart, eraseEnd) {
								// Walk eraseStart back to the START of
								// the first close line so the first
								// close is now part of the elision.
								firstCloseLineStart := eraseStart - 1 // step onto the \n
								for firstCloseLineStart > 0 && rawText[firstCloseLineStart-1] != '\n' {
									firstCloseLineStart--
								}
								// Walk eraseEnd back to the START of the
								// second close line so the second close
								// survives the elision.
								secondCloseLineStart := eraseEnd - 1
								for secondCloseLineStart > 0 && rawText[secondCloseLineStart-1] != '\n' {
									secondCloseLineStart--
								}
								if firstCloseLineStart < secondCloseLineStart {
									eraseStart = firstCloseLineStart
									eraseEnd = secondCloseLineStart
								}
							}
							elisions = append(elisions, splitElisionPreservingElementEnd(rawText, eraseStart, eraseEnd)...)
						}
					}
				} else {
					// Same-ParaLine multi-String. Elide from end of
					// the previous String's value (i.e. starting at
					// the `'>` close) through the wrapper opener
					// `<String \`` of the current String. The current
					// String's `'>` stays so the merged value remains
					// terminated.
					//
					// prevValEnd points just after the value's last
					// byte (before `'>`); start the elision there to
					// drop the previous String's `'>`. The next ref's
					// closing `'>` then serves as the close of the
					// merged String.
					eraseStart := prevValEnd
					eraseEnd := absIdx + len("<"+it.searchTag+" `")
					if eraseEnd < eraseStart {
						eraseEnd = eraseStart
					}
					elisions = append(elisions, elisionRange{
						startOffset: eraseStart,
						endOffset:   eraseEnd,
					})
				}
				// Always emit a ref for stringIdx>0 so the writer
				// consumes the value bytes (writes empty text).
				refs = append(refs, stringRef{
					startOffset: valStart,
					endOffset:   valEnd,
					blockIdx:    it.blockIdx,
					stringIdx:   stringInItemIdx,
				})
			} else {
				// Non-String secondary value (PgfNumFormat etc.) -- no
				// wrapper merging, just a plain value-replacement ref.
				refs = append(refs, stringRef{
					startOffset: valStart,
					endOffset:   valEnd,
					blockIdx:    it.blockIdx,
					stringIdx:   stringInItemIdx,
				})
			}
		}

		// Per-Para inline Char elision (Cluster C / partial G). For each
		// Char glyph that contributed text to this run, find its raw
		// `<Char NAME>` line and add an elision range. The cursor walks
		// monotonically forward; we constrain the search by the next
		// String's position when known. Chars with afterStringSlot==-1
		// appear BEFORE the first String, so we need to search from a
		// position earlier than stringPositions[0][0]. Use the start of
		// the String's enclosing ParaLine line as the back-search
		// anchor (the previous newline boundary).
		if it.searchTag == "String" && len(it.inlineChars) > 0 && len(stringPositions) > 0 {
			firstStringPos := stringPositions[0][0]
			// Walk back to the start of the line containing the first
			// String so we can find any preceding Chars.
			backAnchor := firstStringPos
			for backAnchor > 0 && rawText[backAnchor-1] != '\n' {
				backAnchor--
			}
			// Walk further back over previous lines to find any Char
			// Tab-style lines that precede the first String. Limit the
			// walk to the start of the enclosing ParaLine to avoid
			// crossing into a previous Para's content.
			paraLineOpen := strings.LastIndex(rawText[:firstStringPos], "<ParaLine")
			if paraLineOpen < 0 {
				paraLineOpen = 0
			}
			charSearchFrom := paraLineOpen
			for _, ic := range it.inlineChars {
				// Bound the search to the slot region. afterStringSlot
				// == -1 means before the first String; bound by the
				// first String's position.
				upperBound := len(rawText)
				if ic.afterStringSlot+1 < len(stringPositions) {
					upperBound = stringPositions[ic.afterStringSlot+1][0]
				}
				start, end := findCharLine(charSearchFrom, ic.name)
				if start < 0 || start >= upperBound {
					continue
				}
				elisions = append(elisions, elisionRange{
					startOffset: start,
					endOffset:   end,
				})
				charSearchFrom = end
			}
		}

		// String-followed-by-Marker join (Cluster G partial). Mirrors
		// okapi writeParagraph (MIFFilter.java:636-805) which emits the
		// reconstructed paragraph as a sequence of `<String 'val'>`
		// + inline-code placeholders without inter-tag whitespace --
		// `<String '...'><Marker ` rather than `<String '...'>\n   <Marker `.
		// When the byte just past the LAST String's `'>` (skipping
		// whitespace) is the start of `<Marker `, elide the whitespace
		// so the two tags join on the same output line.
		if it.searchTag == "String" && len(stringPositions) > 0 {
			lastValEnd := stringPositions[len(stringPositions)-1][1]
			// Skip past the closing `'>`.
			cursor := lastValEnd
			if cursor+2 <= len(rawText) && rawText[cursor] == '\'' && rawText[cursor+1] == '>' {
				cursor += 2
			}
			// Skip whitespace (spaces, tabs, newlines).
			scan := cursor
			for scan < len(rawText) {
				c := rawText[scan]
				if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
					scan++
					continue
				}
				break
			}
			// Check if the next tag is `<Marker `.
			if scan < len(rawText) && strings.HasPrefix(rawText[scan:], "<Marker ") {
				if scan > cursor {
					elisions = append(elisions, elisionRange{
						startOffset: cursor,
						endOffset:   scan,
					})
				}
			}
		}
	}

	// Cluster K: FNote/Para bare-`>` ParaLine close rewrite. Mirrors
	// okapi MIFFilter.processPara at MIFFilter.java:1191-1200 which, on
	// every ParaLine close (paraLevel→0), unconditionally appends
	// ` # end of ParaLine\n>` (or inserts `>` before an existing comment
	// if one is present). Source MIF often has the comment already, in
	// which case okapi's insert-`>`-before-comment yields a byte-equal
	// result. But when the source has a BARE `>` ParaLine close (no
	// trailing `# end of ParaLine` comment), okapi rewrites the structure
	// to add the labeled close + a synthesized Para close `>`. Per the
	// MIF Reference (Adobe FrameMaker Parameters/MIF Reference, §
	// "ParaLine Statement"), the `# end of <Tag>` comment is purely
	// cosmetic — but okapi normalizes it. Native must mirror so the byte
	// stream matches.
	//
	// The pattern is: a line containing only whitespace + `>` + `\n`,
	// followed by a line whose `>` close is `> # end of Para`. We elide
	// the bare `>` byte and insert ` # end of ParaLine\n>` immediately
	// after the source's `>` of the `> # end of Para` line. Net result:
	//   source: `>\n   > # end of Para\n`
	//   output: `\n   > # end of ParaLine\n> # end of Para\n`
	rewriteFNoteParaCloses(rawText, &elisions, &rewrites)

	// Cluster: empty multi-ParaLine collapse. Mirrors okapi
	// MIFFilter.processPara handling of consecutive empty ParaLines
	// inside a single Para. okapi's readUntilText (MIFFilter.java:1043-
	// 1066, 1490-1556) drops the `<ParaLine` opener + first whitespace
	// from every NON-FIRST ParaLine in a Para (via readTag's special
	// `Char`/`ParaLine` !storeCharStatement deletion at lines 1527-1532),
	// and drops the bare `>` close char from EVERY ParaLine (lines 1171-
	// 1187 — `paraLevel==1 && !inPgf && !inXRef` falls through without
	// appending). The `>` is then re-inserted only at the LAST
	// `" # end of ParaLine"` occurrence via lastIndexOf
	// (MIFFilter.java:1191-1199). Net effect on consecutive empty
	// ParaLines: first opener kept, last close kept; middle openers and
	// non-last `>` chars elided. Per MIF Reference (Adobe FrameMaker
	// Parameters/MIF Reference, §"ParaLine Statement"), the
	// `# end of <Tag>` comment is cosmetic — but okapi normalizes the
	// surrounding bytes anyway, so native must mirror.
	collapseEmptyMultiParaLines(rawText, &elisions)

	// Cluster: content-bearing multi-ParaLine collapse after a Char
	// rewrite (1188_crlf cluster). When a Para's FIRST ParaLine ends with
	// a `<Char HardReturn>` (rewritten to a synthesized `<String '\n'>`
	// by paraCharRewrite) and is followed by a SECOND ParaLine with its
	// own content, okapi's processPara merges both ParaLines into a
	// single output ParaLine: the second ParaLine wrapper bytes and the
	// first's close line both disappear, leaving only a single `\n`
	// separator between the synthesized String and the next ParaLine's
	// first child statement.
	//
	// This generalises Cluster Q's empty-ParaLine collapse to the case
	// where the first ParaLine carries a HardReturn-derived String.
	// Mirrors okapi MIFFilter.processPara (MIFFilter.java:739-766): the
	// HardReturn flushes the current text unit via addTextUnit and starts
	// a fresh skel via `skel = new GenericSkeleton()`, so the inter-
	// ParaLine wrapper bytes that DID accumulate in paraCodeBuf for the
	// next iteration are reset (paraCodeBuf.setLength(0) at line 772).
	// readUntilText then begins a fresh paraCodeBuf for the second
	// ParaLine and only the `<ParaLine` keyword is deleted (line 1058-
	// 1064) -- the close line of the first ParaLine and the opener of
	// the second never make it to the output. Per MIF Reference §
	// "ParaLine Statement", inter-ParaLine wrapper bytes are cosmetic.
	collapseContentMultiParaLineAfterCharRewrite(rawText, &elisions)

	return refs, elisions, rewrites
}

// hasElementEndLine reports whether rawText[start:end) contains at least
// one `<ElementEnd ...>` statement at the start of its physical line
// (i.e. preceded only by indent whitespace then a newline). Used to
// decide whether the multi-ParaLine merge needs to preserve structure-
// tag markers (per MIF Reference §"Element Statements").
func hasElementEndLine(rawText string, start, end int) bool {
	const tag = "<ElementEnd "
	if start < 0 || end > len(rawText) || start >= end {
		return false
	}
	scan := start
	for scan < end {
		idx := strings.Index(rawText[scan:end], tag)
		if idx < 0 {
			return false
		}
		abs := scan + idx
		// Walk back over leading whitespace.
		ls := abs
		for ls > start {
			c := rawText[ls-1]
			if c == ' ' || c == '\t' {
				ls--
				continue
			}
			break
		}
		if ls == start || rawText[ls-1] == '\n' {
			return true
		}
		scan = abs + len(tag)
	}
	return false
}

// splitElisionPreservingElementEnd returns one or more elision ranges
// covering [start, end) that skip over (i.e. preserve in the skeleton)
// every `<ElementEnd ...>` line that falls within the input range. The
// returned ranges always cover [start, end) minus the bytes spanning each
// preserved `<ElementEnd>` physical line (its leading indent through the
// trailing newline). When no `<ElementEnd>` lines are found, the result
// is a single range identical to [start, end).
//
// This mirrors okapi MIFFilter.processPara (MIFFilter.java:1044-1066 +
// 1145-1153): when collapsing multi-ParaLine paragraphs, non-extracted
// statements between ParaLines (notably `<ElementBegin>` / `<ElementEnd>`
// structure tag markers per MIF Reference §"Element Statements") fall
// through the "default: skip over" branch and accumulate in paraCodeBuf,
// so they survive into the merged paragraph's output.
func splitElisionPreservingElementEnd(rawText string, start, end int) []elisionRange {
	const tag = "<ElementEnd "
	if start < 0 || end > len(rawText) || start >= end {
		return nil
	}
	var out []elisionRange
	cursor := start
	scan := start
	for scan < end {
		idx := strings.Index(rawText[scan:end], tag)
		if idx < 0 {
			break
		}
		abs := scan + idx
		// Walk back over leading whitespace to find the line start.
		lineStart := abs
		for lineStart > start {
			c := rawText[lineStart-1]
			if c == ' ' || c == '\t' {
				lineStart--
				continue
			}
			break
		}
		// Require the preceding char (if any) to be a newline, i.e. the
		// `<ElementEnd>` is at the start of its physical line. Mid-line
		// occurrences are not safe to split out.
		if lineStart > start && rawText[lineStart-1] != '\n' {
			scan = abs + len(tag)
			continue
		}
		// Walk forward to the end of the line (include the trailing \n).
		lineEnd := abs + len(tag)
		for lineEnd < end && rawText[lineEnd] != '\n' {
			lineEnd++
		}
		if lineEnd < end && rawText[lineEnd] == '\n' {
			lineEnd++
		}
		// Emit an elision for [cursor, lineStart) -- the preceding chunk
		// up to (but not including) the preserved ElementEnd line.
		if lineStart > cursor {
			out = append(out, elisionRange{startOffset: cursor, endOffset: lineStart})
		}
		// Skip the preserved ElementEnd line; advance cursor and scan
		// past it.
		cursor = lineEnd
		scan = lineEnd
	}
	if cursor < end {
		out = append(out, elisionRange{startOffset: cursor, endOffset: end})
	}
	if len(out) == 0 {
		// No content to elide (entire range was preserved). Fall back to
		// the original range so the caller's invariant (eraseEnd >
		// eraseStart implies at least one elision) holds for empty
		// inputs; in practice this branch isn't reached because we only
		// call this when end > start.
		return []elisionRange{{startOffset: start, endOffset: end}}
	}
	return out
}

// collapseEmptyMultiParaLines scans rawText for runs of two or more
// adjacent empty ParaLine wrappers inside the same Para and emits the
// elision ops needed to mirror okapi's processPara normalization
// (MIFFilter.java:1043-1066, 1171-1199). For each such run:
//   - drop `<ParaLine` + the trailing first whitespace char (the `\r`
//     in CRLF or `\n` in LF) from every non-first ParaLine opener
//   - drop the bare `>` close char from every non-last ParaLine close
//
// The `<ParaLine` opener and ParaLine close lines that bracket the run
// keep their indentation and trailing newline characters so the
// surrounding lines stay correctly framed.
func collapseEmptyMultiParaLines(rawText string, elisions *[]elisionRange) {
	const openTag = "<ParaLine"
	const closeMark = "> # end of ParaLine"
	n := len(rawText)
	i := 0
	for i < n {
		// Find the next ParaLine opener that is followed only by a
		// matching empty close (no String/Char/Marker between them).
		idx := strings.Index(rawText[i:], openTag)
		if idx < 0 {
			break
		}
		first := i + idx
		// Confirm this is a multi-line opener: the only chars from `<`
		// to end-of-line after `<ParaLine` are whitespace.
		afterOpen := first + len(openTag)
		if afterOpen >= n {
			break
		}
		// Reject inline `<ParaLine ...>` style (single-line); we want
		// the multi-line form where the line ends right after the tag.
		// Walk over optional whitespace then expect \r or \n.
		j := afterOpen
		for j < n && (rawText[j] == ' ' || rawText[j] == '\t') {
			j++
		}
		if j >= n || (rawText[j] != '\r' && rawText[j] != '\n') {
			i = afterOpen
			continue
		}
		// Walk a run of consecutive empty ParaLines starting at `first`.
		// runStarts[k] = byte offset of the k-th `<ParaLine` opener.
		// runCloseGt[k] = byte offset of the bare `>` of the k-th close.
		var runStarts []int
		var runCloseGt []int
		cursor := first
		for cursor < n {
			// Confirm `<ParaLine` at cursor.
			if cursor+len(openTag) > n || rawText[cursor:cursor+len(openTag)] != openTag {
				break
			}
			// Find end of opener line (\n).
			lineEnd := cursor + len(openTag)
			for lineEnd < n && rawText[lineEnd] != '\n' {
				if rawText[lineEnd] != '\r' && rawText[lineEnd] != ' ' && rawText[lineEnd] != '\t' {
					// Non-multi-line opener (e.g. `<ParaLine ...>`).
					lineEnd = -1
					break
				}
				lineEnd++
			}
			if lineEnd < 0 || lineEnd >= n {
				break
			}
			// Now scan past the opener line. The next non-blank line
			// must be a close: `[ws]> # end of ParaLine[ws]\n` with no
			// intervening child statements.
			k := lineEnd + 1
			// Skip leading whitespace on the close line.
			for k < n && (rawText[k] == ' ' || rawText[k] == '\t') {
				k++
			}
			if k >= n || rawText[k] != '>' {
				break
			}
			gtPos := k
			// Confirm the `>` is followed by ` # end of ParaLine`.
			if k+len(closeMark) > n {
				break
			}
			if rawText[k:k+len(closeMark)] != closeMark {
				break
			}
			// Walk to end of close line.
			closeLineEnd := k + len(closeMark)
			for closeLineEnd < n && rawText[closeLineEnd] != '\n' {
				closeLineEnd++
			}
			if closeLineEnd >= n {
				break
			}
			closeLineEnd++ // include the \n
			runStarts = append(runStarts, cursor)
			runCloseGt = append(runCloseGt, gtPos)
			// Advance to potential next ParaLine opener: skip indent
			// after the close line.
			next := closeLineEnd
			for next < n && (rawText[next] == ' ' || rawText[next] == '\t') {
				next++
			}
			if next+len(openTag) > n || rawText[next:next+len(openTag)] != openTag {
				break
			}
			cursor = next
		}
		if len(runStarts) >= 2 {
			// Apply elisions:
			//   - Non-first opener: drop `<ParaLine` + the first
			//     trailing whitespace byte (`\r` in CRLF, or `\n` in
			//     LF). The remaining `\n` plus indent of the close line
			//     stays so the close line still terminates cleanly.
			//   - Non-last close: drop the bare `>` byte.
			for k := 1; k < len(runStarts); k++ {
				op := runStarts[k]
				end := op + len(openTag)
				// Consume the first whitespace char after `<ParaLine`
				// (the `\r` of CRLF, or the `\n` of LF, or a space/tab).
				if end < n {
					c := rawText[end]
					if c == '\r' || c == '\n' || c == ' ' || c == '\t' {
						end++
					}
				}
				*elisions = append(*elisions, elisionRange{
					startOffset: op,
					endOffset:   end,
				})
			}
			for k := range len(runCloseGt) - 1 {
				gt := runCloseGt[k]
				*elisions = append(*elisions, elisionRange{
					startOffset: gt,
					endOffset:   gt + 1,
				})
			}
			// Advance past the run.
			i = runCloseGt[len(runCloseGt)-1] + 1
			continue
		}
		i = afterOpen
	}
}

// collapseContentMultiParaLineAfterCharRewrite scans rawText for the
// pattern of a `<Char HardReturn>` line (which paraCharRewrite turns
// into a synthesized `<String '\n'>`) immediately followed by an
// inter-ParaLine wrapper boundary (`> # end of ParaLine` close line
// then `<ParaLine` open line) where the next ParaLine carries its own
// content. For each such match, it emits an elision range that drops
// the entire boundary except for a single `\n` byte separator.
//
// Concretely, for source bytes (after `<Char HardReturn>'s closing `>`):
//
//	\r\n  > # end of ParaLine\r\n  <ParaLine\r\n   <NextTag...
//
// the elision spans from the `\r` right after `>` through (and
// including) the `\r` of `<ParaLine\r\n`. What survives in the output
// is `\n   <NextTag...` -- the LF after `<ParaLine\r` becomes the
// single-`\n` separator that mirrors okapi's `skel = new GenericSkeleton`
// reset semantics at MIFFilter.java:764.
//
// LF-only line endings are handled symmetrically: the elision then
// drops `\n  > # end of ParaLine\n  <ParaLine` (no `\r` bytes), again
// preserving the trailing `\n` that follows `<ParaLine`.
func collapseContentMultiParaLineAfterCharRewrite(rawText string, elisions *[]elisionRange) {
	const charTag = "<Char HardReturn>"
	const closeMark = "> # end of ParaLine"
	const openTag = "<ParaLine"
	n := len(rawText)
	i := 0
	for i < n {
		idx := strings.Index(rawText[i:], charTag)
		if idx < 0 {
			return
		}
		abs := i + idx
		// Advance i for the next iteration regardless of whether we
		// emit an elision.
		i = abs + len(charTag)
		// Require that `<Char HardReturn>` is line-anchored: walk back
		// over leading whitespace and require a preceding newline.
		ls := abs
		for ls > 0 {
			c := rawText[ls-1]
			if c == ' ' || c == '\t' {
				ls--
				continue
			}
			break
		}
		if ls == 0 || rawText[ls-1] != '\n' {
			continue
		}
		// Walk backwards to the enclosing `<ParaLine` opener and require
		// that no `<String` appears between the opener and the
		// `<Char HardReturn>`. If a String IS present, the multi-String-
		// multi-ParaLine merge at line 953 already handles the boundary
		// collapse; firing this pass on top of that would over-elide.
		paraLineStart := strings.LastIndex(rawText[:abs], openTag)
		if paraLineStart < 0 {
			continue
		}
		if strings.Contains(rawText[paraLineStart:abs], "<String ") ||
			strings.Contains(rawText[paraLineStart:abs], "<String\t") ||
			strings.Contains(rawText[paraLineStart:abs], "<String`") {
			continue
		}
		// Position right after `>` of `<Char HardReturn>`.
		p := abs + len(charTag)
		eraseStart := p
		// Match `\r?\n` after the Char close.
		if p < n && rawText[p] == '\r' {
			p++
		}
		if p >= n || rawText[p] != '\n' {
			continue
		}
		p++
		// Match `[ws]> # end of ParaLine`.
		for p < n && (rawText[p] == ' ' || rawText[p] == '\t') {
			p++
		}
		if p+len(closeMark) > n || rawText[p:p+len(closeMark)] != closeMark {
			continue
		}
		p += len(closeMark)
		// Match `\r?\n` after the close.
		if p < n && rawText[p] == '\r' {
			p++
		}
		if p >= n || rawText[p] != '\n' {
			continue
		}
		p++
		// Match `[ws]<ParaLine` (opener of next ParaLine).
		for p < n && (rawText[p] == ' ' || rawText[p] == '\t') {
			p++
		}
		if p+len(openTag) > n || rawText[p:p+len(openTag)] != openTag {
			continue
		}
		p += len(openTag)
		// The opener may have a trailing space before its newline
		// (e.g. `<ParaLine \n`); skip any space/tab.
		for p < n && (rawText[p] == ' ' || rawText[p] == '\t') {
			p++
		}
		// Match `\r?\n` after the opener; preserve the final `\n` byte
		// as the separator in the output.
		if p < n && rawText[p] == '\r' {
			// Erase up to AND INCLUDING the `\r`; the `\n` survives.
			p++
		} else if p < n && rawText[p] == '\n' {
			// LF-only: erase up to (but NOT including) the `\n`.
		} else {
			continue
		}
		eraseEnd := p
		// Verify the next ParaLine has content (i.e. there is at least
		// one `<` tag before the next `> # end of ParaLine`). This
		// keeps the elision conservative -- if the second ParaLine is
		// empty, the existing collapseEmptyMultiParaLines pass handles
		// it (and applying both would over-elide).
		nextLineStart := p
		if nextLineStart < n && rawText[nextLineStart] == '\n' {
			nextLineStart++
		}
		// Scan forward to the matching close of the next ParaLine.
		scan := nextLineStart
		foundContent := false
		for scan < n {
			c := rawText[scan]
			switch c {
			case ' ', '\t', '\r', '\n':
				scan++
				continue
			case '<':
				// Found content (a tag) before close -- this ParaLine
				// has content.
				foundContent = true
			case '>':
				// Bare `>` (close of empty ParaLine) -- no content.
			default:
				// Some other char -- treat as no content for safety.
			}
			break
		}
		if !foundContent {
			continue
		}
		if eraseEnd > eraseStart {
			*elisions = append(*elisions, elisionRange{
				startOffset: eraseStart,
				endOffset:   eraseEnd,
			})
		}
		// Advance i past the elision.
		i = eraseEnd
	}
}

// rewriteFNoteParaCloses scans rawText for the bare-`>` ParaLine close
// pattern (typically inside `<FNote>` blocks) and emits the
// elision + rewrite ops needed to mirror okapi's processPara
// (MIFFilter.java:1191-1200) ParaLine close synthesis. See Cluster K
// note in findStringPositions for the per-byte rewrite contract.
func rewriteFNoteParaCloses(rawText string, elisions *[]elisionRange, rewrites *[]charRewrite) {
	// Match: \n<whitespace>>\n<whitespace>> # end of Para\n
	// Walk byte-by-byte to avoid pulling in regexp dependency.
	n := len(rawText)
	for i := 0; i < n; i++ {
		if rawText[i] != '\n' {
			continue
		}
		// Check for bare `>` line: <whitespace>>\n
		j := i + 1
		for j < n && (rawText[j] == ' ' || rawText[j] == '\t') {
			j++
		}
		if j >= n || rawText[j] != '>' {
			continue
		}
		bareGtPos := j
		j++
		if j >= n || rawText[j] != '\n' {
			continue
		}
		// Now check the next line is `<whitespace>> # end of Para\n`.
		k := j + 1
		for k < n && (rawText[k] == ' ' || rawText[k] == '\t') {
			k++
		}
		if k >= n || rawText[k] != '>' {
			continue
		}
		paraGtPos := k
		const endOfPara = " # end of Para"
		if k+1+len(endOfPara) > n {
			continue
		}
		if rawText[k+1:k+1+len(endOfPara)] != endOfPara {
			continue
		}
		// Verify it's exactly `> # end of Para` and not a longer name
		// like `> # end of ParaLine` -- the next byte after must be
		// `\r` or `\n` (not a letter).
		after := k + 1 + len(endOfPara)
		if after < n && rawText[after] != '\n' && rawText[after] != '\r' {
			continue
		}
		// Emit elision: drop the bare `>` byte.
		*elisions = append(*elisions, elisionRange{
			startOffset: bareGtPos,
			endOffset:   bareGtPos + 1,
		})
		// Emit rewrite: insert ` # end of ParaLine\n>` right after the
		// source's `>` of the `> # end of Para` line. start==end means
		// pure insertion at that offset.
		insertAt := paraGtPos + 1
		*rewrites = append(*rewrites, charRewrite{
			startOffset: insertAt,
			endOffset:   insertAt,
			text:        " # end of ParaLine\n>",
		})
		// Advance to skip over the matched region.
		i = after
	}
}

// findCharLineForRewrite locates the next `<Char NAME>` line at or
// after `from` and returns (start, joinEnd) where:
//   - start is the byte offset of the first character of indentation
//     for the line containing `<Char NAME>`. The leading newline is
//     NOT included so the rewrite's replacement preserves line
//     alignment with surrounding ParaLine children.
//   - joinEnd is the byte offset just past the closing `>` of
//     `<Char NAME>`; when joinNext=true it is extended through any
//     trailing whitespace + newline so the sibling tag on the next
//     source line joins on the same output line as the synthesized
//     `<String>`.
//
// Returns (-1, -1) if no clean line-anchored match is found.
func findCharLineForRewrite(rawText string, from int, name string, joinNext bool) (start, joinEnd int) {
	needle := "<Char " + name + ">"
	idx := strings.Index(rawText[from:], needle)
	if idx < 0 {
		return -1, -1
	}
	abs := from + idx
	// Walk back over leading whitespace.
	start = abs
	for start > from {
		c := rawText[start-1]
		if c == ' ' || c == '\t' {
			start--
			continue
		}
		break
	}
	// At this point `start` should be just after a newline.
	if start <= from || rawText[start-1] != '\n' {
		// Char is mid-line -- bail.
		return -1, -1
	}
	end := abs + len(needle)
	joinEnd = end
	if joinNext {
		// Extend through trailing whitespace + newline so the next
		// sibling joins on the same output line.
		for joinEnd < len(rawText) {
			c := rawText[joinEnd]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
				joinEnd++
				continue
			}
			break
		}
	}
	return start, joinEnd
}

// buildCharRewriteReplacement builds the replacement text for a
// `<Char NAME>` rewrite. The output is `<indent><String 'X'>` where
// `<indent>` is the leading whitespace at `start` (from the call to
// findCharLineForRewrite), and X is the MIF-escaped glyph value.
//
// The trailing newline (if any) at the end of the original `<Char>`
// line is preserved by the caller (rewrite range ends at the close
// `>` so the original `\r?\n` after it stays in the skeleton). When
// joinNext was set, the caller's range extends past the trailing
// whitespace -- in that case the next sibling tag joins on the same
// output line.
func buildCharRewriteReplacement(rawText string, start int, glyphText string) string {
	end := start
	for end < len(rawText) && (rawText[end] == ' ' || rawText[end] == '\t') {
		end++
	}
	indent := rawText[start:end]
	return indent + "<String `" + escapeMIFForSearch(glyphText) + "'>"
}

// sortSkelOps sorts skeleton ops in-place by start offset. Refs come
// before elisions when starts are equal so the writer consumes the
// value bytes before any wrapper-elision step jumps past them. The sort
// is stable so ops sharing a (start, kind) keep their insertion order,
// matching the previous hand-rolled stable insertion sort exactly.
func sortSkelOps(ops []skelOp) {
	slices.SortStableFunc(ops, func(a, b skelOp) int {
		if c := cmp.Compare(a.start, b.start); c != 0 {
			return c
		}
		return cmp.Compare(a.kind, b.kind)
	})
}

// escapeMIFForSearch re-encodes a parsed value back to the MIF in-string
// escape form so we can locate it in the rawText scan. Mirrors
// writer.escapeMIF — kept colocated with the reader since both
// scanners and the writer must agree.
func escapeMIFForSearch(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '`':
			b.WriteString("\\`")
		case '\'':
			b.WriteString("\\'")
		case '\\':
			b.WriteString("\\\\")
		case '>':
			b.WriteString("\\>")
		case '\t':
			b.WriteString("\\t")
		case '\n':
			b.WriteString("\\n")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (r *Reader) emitStatements(ctx context.Context, ch chan<- model.PartResult, stmts []*mifStatement) {
	blockCounter := 0
	dataCounter := 0
	textFlowNumber := 0

	emitData := func(stmt *mifStatement) bool {
		dataCounter++
		d := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: "mif." + stmt.tag,
			Properties: map[string]string{
				"tag": stmt.tag,
				"raw": stmt.raw,
			},
		}
		return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d})
	}

	for _, stmt := range stmts {
		// PgfCatalog is walked first to emit translatable
		// <PgfNumFormat> blocks (okapi MIFFilter.java:357-362,1078-1095);
		// raw PgfCatalog text then flows through the alwaysSkipTags
		// branch below as Data.
		if stmt.tag == "PgfCatalog" {
			blockCounter = r.processPgfCatalog(ctx, ch, stmt, extractablePgfTags(stmts), blockCounter)
		}
		if r.skipTag(stmt.tag) {
			// Emit as non-translatable data.
			dataCounter++
			d := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "mif." + stmt.tag,
				Properties: map[string]string{
					"tag": stmt.tag,
					"raw": stmt.raw,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
				return
			}
			continue
		}

		if stmt.tag == "MIFFile" {
			// Emit version line as data.
			dataCounter++
			d := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "mif.version",
				Properties: map[string]string{
					"tag":     "MIFFile",
					"version": stmt.value,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
				return
			}
			continue
		}

		if stmt.tag == "TextFlow" {
			// Body-page scoping (okapi Extracts.textFlowExtractable,
			// MIFFilter.java:568-577): a TextFlow is extracted only when its
			// 1-based order number is reachable from an extractable page. Out
			// of scope → emit as non-translatable Data so the writer can
			// reproduce it verbatim.
			textFlowNumber++
			if !r.scope.textFlowInScope(textFlowNumber) {
				if !emitData(stmt) {
					return
				}
				continue
			}
			blockCounter, dataCounter = r.processContainer(ctx, ch, stmt, blockCounter, dataCounter)
			continue
		}

		if stmt.tag == "Tbls" || stmt.tag == "Notes" {
			// Process translatable content inside these containers. Tbls
			// children (Tbl) are filtered per-TblID inside processContainer.
			blockCounter, dataCounter = r.processContainer(ctx, ch, stmt, blockCounter, dataCounter)
			continue
		}

		if stmt.tag == "VariableFormats" {
			// Extract each <VariableDef `...'> as a translatable block, so
			// FrameMaker variable text round-trips through the
			// pseudo-translate pipeline (matches okapi's MIFFilter
			// behaviour). The skeleton-store ref scheme uses the same
			// blockIdx:stringIdx form as Para/String — see
			// findStringPositions.
			blockCounter, dataCounter = r.processVariableFormats(ctx, ch, stmt, blockCounter, dataCounter)
			continue
		}

		if stmt.tag == "Page" {
			// FrameMaker pages can carry translatable strings via direct
			// <TextLine> children and via <Frame> children (each holding
			// nested <TextLine> with <String>). Mirrors okapi
			// MIFFilter.java:395 (processPage) + 1629-1644 + 1663-1673
			// (processFramesAndTextLines). The PageType-based skip lives
			// inside processPage to match the okapi gating.
			if r.skipPage(stmt) {
				dataCounter++
				d := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: "mif.Page",
					Properties: map[string]string{
						"tag": "Page",
						"raw": stmt.raw,
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
					return
				}
				continue
			}
			blockCounter, dataCounter = r.processFramesAndTextLines(ctx, ch, stmt, blockCounter, dataCounter)
			continue
		}

		if stmt.tag == "AFrames" {
			// Top-level anchored-frame container — mirrors okapi
			// MIFFilter.java:398-400 (processFramesAndTextLines on
			// AFrames). Walks <Frame> children recursively and emits one
			// Block per <TextLine><String>.
			blockCounter, dataCounter = r.processFramesAndTextLines(ctx, ch, stmt, blockCounter, dataCounter)
			continue
		}

		// Default: emit as data.
		dataCounter++
		d := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: "mif." + stmt.tag,
			Properties: map[string]string{
				"tag": stmt.tag,
				"raw": stmt.raw,
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
			return
		}
	}
}

// extractablePgfTags returns the set of <PgfTag> names whose PgfCatalog
// <PgfNumFormat> entry should be extracted as translatable. Mirrors
// okapi Extracts.java:449-456: a tag is in the set iff some Para in
// extractable TextFlow/Tbls has it, has non-empty <PgfNumString>, and
// has NO inline <Pgf><PgfNumFormat> override — the catalog text is the
// active numbering string in that case.
func extractablePgfTags(stmts []*mifStatement) map[string]bool {
	out := map[string]bool{}
	var visit func(stmt *mifStatement)
	visit = func(stmt *mifStatement) {
		for _, child := range stmt.children {
			if child.tag == "Para" {
				var pgfTag string
				hasNumString, inlineHasNumFormat := false, false
				for _, gc := range child.children {
					switch gc.tag {
					case "PgfTag":
						pgfTag = gc.value
					case "PgfNumString":
						hasNumString = true
					case "Pgf":
						for _, ggc := range gc.children {
							if ggc.tag == "PgfNumFormat" {
								inlineHasNumFormat = true
							}
						}
					}
				}
				if pgfTag != "" && hasNumString && !inlineHasNumFormat {
					out[pgfTag] = true
				}
				continue
			}
			if isMIFContainer(child.tag) {
				visit(child)
			}
		}
	}
	for _, stmt := range stmts {
		switch stmt.tag {
		case "TextFlow", "Tbls", "Notes":
			visit(stmt)
		}
	}
	return out
}

// processPgfCatalog emits one Block per non-empty <PgfNumFormat> child
// of every extractable <Pgf>. Mirrors okapi MIFFilter.java:357-362
// (inPgfCatalog state) and 1078-1095 (PgfNumFormat → translatable when
// inPgfCatalog). Surrounding raw bytes flow through the skeleton store.
func (r *Reader) processPgfCatalog(ctx context.Context, ch chan<- model.PartResult, stmt *mifStatement, extractable map[string]bool, blockCounter int) int {
	for _, child := range stmt.children {
		if child.tag != "Pgf" {
			continue
		}
		var pgfTag string
		for _, gc := range child.children {
			if gc.tag == "PgfTag" {
				pgfTag = gc.value
				break
			}
		}
		if !extractable[pgfTag] {
			continue
		}
		for _, gc := range child.children {
			if gc.tag != "PgfNumFormat" || gc.value == "" {
				continue
			}
			// Hard-return splitting (okapi processFormats / processPara for
			// the inPgfCatalog case): with ExtractHardReturnsAsText off a
			// '\n' in the catalog PgfNumFormat starts a new TextUnit.
			for _, seg := range r.splitFormatValueOnHardReturns(gc.value) {
				blockCounter++
				block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), seg)
				block.Name = fmt.Sprintf("pgf_num_format.%d", blockCounter)
				block.Properties["pgf_tag"] = pgfTag
				// PgfNumFormat values get the additional ^[A-Z]: rule
				// (pgfNumFormatLeadingPrefix). This protects auto-number type
				// prefixes like `T:`, `C:`, `H:` while leaving regular
				// <String>-context text alone. Both rule sets are applied in
				// a single pass so the global codeFinder placeholders (e.g.
				// `<n+>`, `<$lastpagenum>`) coexist with the leading prefix
				// placeholder without one pass discarding the other.
				r.applyCodeFinderWithExtras(block, []*regexp.Regexp{pgfNumFormatLeadingPrefix})
				simplifyBlockCodes(block)
				if !blockHasText(block) {
					// All building-block codes / whitespace: okapi yields no
					// TextUnit for this PgfNumFormat (tf.hasText() gate).
					continue
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return blockCounter
				}
			}
		}
	}
	return blockCounter
}

// applyCodeFinderWithExtras is applyCodeFinder plus an additional
// list of context-specific patterns appended to the global config
// patterns for THIS block only. Both rule sets feed a single
// applyCodeFinderToBlock call so a second pass doesn't undo the
// first (flattening to text drops Ph data, so re-running the splitter
// after a previous pass would lose the earlier placeholders).
func (r *Reader) applyCodeFinderWithExtras(block *model.Block, extras []*regexp.Regexp) {
	if block == nil {
		return
	}
	patterns := r.cfg.GetCodeFinderPatterns()
	merged := make([]*regexp.Regexp, 0, len(patterns)+len(extras))
	merged = append(merged, patterns...)
	merged = append(merged, extras...)
	if len(merged) == 0 {
		return
	}
	applyCodeFinderToBlock(block, merged)
}

// skipPage reports whether a <Page> statement should be treated as a
// non-translatable Data blob. Mirrors okapi
// Extracts.java:127-132 (pageTypeExtractable) — true iff the page's
// <PageType> value is in a category whose Extract* config flag is false.
// Pages with no <PageType> are processed as if extractable, matching the
// okapi default-include behaviour.
func (r *Reader) skipPage(stmt *mifStatement) bool {
	for _, c := range stmt.children {
		if c.tag != "PageType" {
			continue
		}
		switch c.value {
		case "BodyPage":
			return !r.cfg.ExtractBodyPages
		case "ReferencePage":
			return !r.cfg.ExtractReferencePages
		case "HiddenPage":
			return !r.cfg.ExtractHiddenPages
		case "LeftMasterPage", "RightMasterPage", "OtherMasterPage":
			return !r.cfg.ExtractMasterPages
		}
		return false
	}
	return false
}

// processFramesAndTextLines walks a <Page>/<AFrames>/<Frame> subtree and
// emits one translatable Block per <TextLine> that holds a <String>.
// Mirrors okapi MIFFilter.java:1663-1717:
//   - Walks for direct <Frame> and <TextLine> children
//   - <Frame> recurses (processFrame → processFramesAndTextLines)
//   - <TextLine> with a <String> child becomes one TextUnit
//
// The skeleton-ref scheme uses the same `blockIdx:stringIdx` form as
// Para/String. Each TextLine has at most one String (the okapi
// processTextLine code stops after the first String it encounters), so
// stringIdx is always 0 here.
func (r *Reader) processFramesAndTextLines(ctx context.Context, ch chan<- model.PartResult, stmt *mifStatement, blockCounter, dataCounter int) (int, int) {
	for _, child := range stmt.children {
		switch child.tag {
		case "Frame":
			// Body-page scoping (okapi processFrame +
			// Extracts.frameExtractable, MIFFilter.java:1646-1661): an
			// anchored <Frame> is walked only when its <Unique> id is in the
			// reachable-frame set. Page-direct frames are added to that set
			// in computeExtractScope, so body-page frames pass; anchored
			// frames not referenced from extractable content are skipped.
			if !r.scope.frameInScope(child.firstLiteral("Unique")) {
				continue
			}
			blockCounter, dataCounter = r.processFramesAndTextLines(ctx, ch, child, blockCounter, dataCounter)
		case "TextLine":
			val, ok := firstStringValue(child)
			if !ok {
				continue
			}
			// Hard-return splitting (okapi processTextLine,
			// MIFFilter.java:1683-1714): with ExtractHardReturnsAsText off, a
			// '\n' inside the TextLine string starts a new TextUnit.
			for _, seg := range r.splitFormatValueOnHardReturns(val) {
				blockCounter++
				block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), seg)
				block.Name = fmt.Sprintf("textline.%d", blockCounter)
				r.applyCodeFinder(block)
				simplifyBlockCodes(block)
				if !blockHasText(block) {
					continue
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return blockCounter, dataCounter
				}
			}
		}
	}
	return blockCounter, dataCounter
}

// firstStringValue returns the value of the first <String> direct child
// of stmt (typically a <TextLine>), and whether one was found. Mirrors
// the single-String-per-TextLine model used by okapi processTextLine.
func firstStringValue(stmt *mifStatement) (string, bool) {
	for _, c := range stmt.children {
		if c.tag == "String" {
			return c.value, true
		}
	}
	return "", false
}

// firstStringRawValue is firstStringValue but returns the source-form
// bytes (rawValue) used by findStringPositions to match against
// rawText. Use this whenever the returned value flows into
// findStringPositions' itemInfo.strings; use firstStringValue when the
// decoded text feeds a translatable Block (extractParaRuns / processContainer
// don't pass through here today — TextLine emits one block per String).
func firstStringRawValue(stmt *mifStatement) (string, bool) {
	for _, c := range stmt.children {
		if c.tag == "String" {
			return c.rawValue, true
		}
	}
	return "", false
}

// extractMarker reports whether a <Marker> statement should be
// extracted as translatable (its <MText> value becomes a Block).
// Mirrors okapi processMarker (MIFFilter.java:842-857): only Index
// markers (when ExtractIndexMarkers) and Hypertext markers (when
// ExtractLinks) are extracted.
func (r *Reader) extractMarker(stmt *mifStatement) bool {
	for _, c := range stmt.children {
		if c.tag != "MTypeName" {
			continue
		}
		switch c.value {
		case "Index":
			return r.cfg.ExtractIndexMarkers
		case "Hypertext":
			return r.cfg.ExtractLinks
		}
		return false
	}
	return false
}

// markerTextValue returns the <MText> child's value of a <Marker>, or
// "" if missing. Mirrors okapi MIFFilter.java:860 (readUntil("MText;")).
func markerTextValue(stmt *mifStatement) string {
	for _, c := range stmt.children {
		if c.tag == "MText" {
			return c.value
		}
	}
	return ""
}

// markerTextRawValue returns the source-form bytes of the MText
// child, used by findStringPositions to locate the original
// `<MText '...'>` line in rawText. Differs from markerTextValue when
// the MText body contains `\xNN ` hex escapes that Hexadecimal.toString
// collapses to a different Unicode form on decode.
func markerTextRawValue(stmt *mifStatement) string {
	for _, c := range stmt.children {
		if c.tag == "MText" {
			return c.rawValue
		}
	}
	return ""
}

// processVariableFormats walks the <VariableFormats> block and emits one
// Block per <VariableDef `...'> child, mirroring okapi's MIFFilter which
// extracts the variable text as translatable. The block carries the
// owning <VariableName> as a property so the writer/UI can show useful
// context, but the writer round-trip uses the skeleton-ref scheme to
// patch only the VariableDef value verbatim.
func (r *Reader) processVariableFormats(ctx context.Context, ch chan<- model.PartResult, stmt *mifStatement, blockCounter, dataCounter int) (int, int) {
	for _, child := range stmt.children {
		if child.tag != "VariableFormat" {
			continue
		}
		var name string
		var defStmt *mifStatement
		for _, gc := range child.children {
			switch gc.tag {
			case "VariableName":
				name = gc.value
			case "VariableDef":
				defStmt = gc
			}
		}
		if defStmt == nil {
			continue
		}
		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), defStmt.value)
		block.Name = fmt.Sprintf("variable.%d", blockCounter)
		if name != "" {
			block.Properties["variable_name"] = name
		}
		r.applyCodeFinder(block)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return blockCounter, dataCounter
		}
	}
	return blockCounter, dataCounter
}

// pgfNumFormatLeadingPrefix matches the okapi codeFinder rule
// `^[A-Z]{1}:` (Parameters.java:196) that protects FrameMaker
// auto-numbering type prefixes like `T:`, `C:`, `H:` from being
// pseudo-translated. The same rule ships in the global default rule
// list (config.go) and is suppressed at apply-time when the block's
// text was preceded by an inline-code statement in its ParaLine
// (paraTextRun.precededByInlineCode). PgfNumFormat blocks do not flow
// through paraTextRun, so apply it here unconditionally — mirrors
// Java's checkInlineCodes call from MIFFilter.java:1098 inside the
// extractedReferent / inPgfCatalog branch.
var pgfNumFormatLeadingPrefix = regexp.MustCompile(`^[A-Z]:`)

// simplifyBlockCodes removes leading and trailing inline-code (Ph) runs
// from the block's first source segment, then strips whitespace that
// becomes leading/trailing once those boundary codes are gone. Mirrors
// okapi's CodeSimplifier / TextUnitSimplification (MIFFilter.java:237-239,
// 935-963) which pulls leading/trailing placeholder codes out of every
// extracted TextUnit (their data is folded into the surrounding skeleton).
//
// This is what reduces a PgfNumFormat referent like `T:Table <n+>: ` to
// "Table <code>:" — the leading auto-number type-prefix code (`T:`,
// matched by the ^[A-Z]: rule) and the trailing `\t`/space are simplified
// away, leaving only the interior building-block code.
// blockHasText reports whether the block's source carries at least one
// non-whitespace text character (inline codes don't count). Mirrors okapi
// tf.hasText() applied after CodeSimplifier: a unit that simplifies to only
// codes / whitespace is not emitted.
func blockHasText(block *model.Block) bool {
	if block == nil {
		return false
	}
	for _, run := range block.SourceRuns() {
		if run.Text != nil && hasNonWhitespace(run.Text.Text) {
			return true
		}
	}
	return false
}

// simplifyBlockTrim is the property key under which simplifyBlockCodes
// records, before it discards them, the rendered leading and trailing
// content it removes from the block's first segment (joined by a NUL).
// okapi routes such boundary codes/whitespace into the surrounding
// skeleton (CodeSimplifier folds leading/trailing placeholder codes out of
// the TextUnit, MIFFilter.java:935-963) so they STILL appear in the output
// `<String>` — they are non-translatable, so the writer re-wraps the
// translated core with them on round-trip. Without this the trimmed bytes
// (e.g. a leading `<Char Tab>` glyph, or a trailing `<$paratext>` building
// block) would vanish from the output, breaking byte-faithful round-trip.
const simplifyBlockTrim = "mif_simplify_trim"

func simplifyBlockCodes(block *model.Block) {
	if block == nil || len(block.Source) == 0 {
		return
	}
	// Work on a flattened rune+code stream so the iterative leading/
	// trailing simplification mirrors okapi CodeSimplifier.removeLeadingTrailingCodes
	// exactly: on iteration 1 a boundary whitespace is only removable when it
	// is adjacent to a boundary code that is also being removed; from
	// iteration 2 onward (only reached when iteration 1 removed something)
	// boundary whitespace is removable on its own. This keeps "Custom: "
	// (no code → no iteration 2 → trailing space kept) distinct from
	// "Table <code>: " (leading T: code removed → iteration 2 → trailing
	// space trimmed).
	type tok struct {
		isCode bool
		text   string // for non-code: a single whitespace run boundary handled char-by-char
		run    model.Run
	}
	// Flatten into per-rune text tokens and per-code tokens so boundary
	// whitespace can be removed one char at a time.
	var toks []tok
	for _, run := range block.Source {
		if run.Text != nil {
			for _, r := range run.Text.Text {
				toks = append(toks, tok{text: string(r)})
			}
			continue
		}
		toks = append(toks, tok{isCode: true, run: run})
	}
	origToks := append([]tok(nil), toks...)

	// Whitespace per Java's Character.isWhitespace, which okapi
	// CodeSimplifier uses: spaces, tabs, and line breaks all count. A hard
	// return that becomes leading/trailing once a boundary code is removed
	// is therefore simplified away too (the 1188_crlf "Pa<tab>ra\n 2." case).
	isWS := func(s string) bool {
		return s == " " || s == "\t" || s == "\n" || s == "\r"
	}

	iteration := 0
	for {
		iteration++
		removed := false

		// Leading: collect leading codes; whitespace only after a code on
		// iteration 1, freely from iteration 2.
		lead := 0
		sawCode := false
		for lead < len(toks) {
			t := toks[lead]
			if t.isCode {
				sawCode = true
				lead++
				continue
			}
			if isWS(t.text) && (sawCode || iteration > 1) {
				lead++
				continue
			}
			break
		}
		if lead > 0 {
			toks = toks[lead:]
			removed = true
		}

		// Trailing: symmetric.
		tail := len(toks)
		sawCode = false
		for tail > 0 {
			t := toks[tail-1]
			if t.isCode {
				sawCode = true
				tail--
				continue
			}
			if isWS(t.text) && (sawCode || iteration > 1) {
				tail--
				continue
			}
			break
		}
		if tail < len(toks) {
			toks = toks[:tail]
			removed = true
		}

		if !removed {
			break
		}
		if len(toks) == 0 {
			break
		}
	}

	// Capture the leading/trailing tokens that were removed so the writer
	// can re-wrap the translated core with them on round-trip (they are
	// non-translatable boundary whitespace/codes that okapi keeps in the
	// output `<String>`). The surviving `toks` is a contiguous middle slice
	// of origToks; everything before/after it was trimmed.
	renderToks := func(ts []tok) string {
		var b strings.Builder
		for _, t := range ts {
			if t.isCode {
				if t.run.Ph != nil {
					b.WriteString(t.run.Ph.Data)
				}
				continue
			}
			b.WriteString(t.text)
		}
		return b.String()
	}
	leadCount := 0
	if len(toks) > 0 {
		// Find the start index of the surviving slice within origToks by
		// locating the first surviving token. Because trimming only removes
		// from the ends, len(origToks)-len(toks) split between lead and tail.
		// Reconstruct lead length: scan origToks for the position where the
		// surviving middle begins (the trimming loop removed a prefix then a
		// suffix iteratively, so the survivors are origToks[lead:lead+len(toks)]).
		// We recompute lead by matching the surviving content length.
		for leadCount <= len(origToks)-len(toks) {
			match := true
			for k := range toks {
				o := origToks[leadCount+k]
				s := toks[k]
				if o.isCode != s.isCode || o.text != s.text {
					match = false
					break
				}
			}
			if match {
				break
			}
			leadCount++
		}
	} else {
		leadCount = len(origToks)
	}
	isStructuralPh := func(t tok) bool {
		return t.isCode && t.run.Ph != nil && t.run.Ph.Data == ""
	}
	// The trimmed leading region is origToks[:leadCount]. Anything before
	// the LAST structural inline-code (empty-Data Ph, i.e. a <Font>/<AFrame>
	// boundary) in that region belongs to an EARLIER text-group — a separate
	// output `<String>` slot (or a whitespace-only run handled by a Char
	// rewrite). Only the bytes AFTER that boundary are this surviving group's
	// own leading content that the writer must restore. Without this bound a
	// leading `<Char HardReturn>`-derived `\n` (which becomes its own
	// `<String '\n'>` via paraCharRewrite) would also be duplicated into the
	// next group's String (the 1188_crlf cluster).
	leadFrom := 0
	for i := range leadCount {
		if isStructuralPh(origToks[i]) {
			leadFrom = i + 1
		}
	}
	leadStr := renderToks(origToks[leadFrom:leadCount])

	trailStart := leadCount + len(toks)
	if trailStart > len(origToks) {
		trailStart = len(origToks)
	}
	// Symmetric: the trailing trim stops at the FIRST structural inline-code
	// in the trailing region; content at/after it belongs to a later group.
	trailEnd := len(origToks)
	for i := trailStart; i < len(origToks); i++ {
		if isStructuralPh(origToks[i]) {
			trailEnd = i
			break
		}
	}
	trailStr := renderToks(origToks[trailStart:trailEnd])
	if leadStr != "" || trailStr != "" {
		if block.Properties == nil {
			block.Properties = map[string]string{}
		}
		block.Properties[simplifyBlockTrim] = leadStr + "\x00" + trailStr
	}

	// Rebuild runs, coalescing consecutive text tokens.
	var out []model.Run
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, model.Run{Text: &model.TextRun{Text: buf.String()}})
			buf.Reset()
		}
	}
	for _, t := range toks {
		if t.isCode {
			flush()
			out = append(out, t.run)
			continue
		}
		buf.WriteString(t.text)
	}
	flush()
	if len(out) == 0 {
		out = []model.Run{{Text: &model.TextRun{Text: ""}}}
	}
	block.SetSourceRuns(out)
}

// applyCodeFinder splits each TextRun in the block into text +
// placeholder runs whenever a CodeFinder pattern matches. This keeps
// FrameMaker building blocks (`<$lastpagenum\>`, `<n+\>`, `<$tblsheetnum\>`,
// …) from being pseudo-translated character by character — the
// pseudo-translate step only transforms text runs.
func (r *Reader) applyCodeFinder(block *model.Block) {
	r.applyCodeFinderCtx(block, codeFinderCtx{})
}

// codeFinderCtx carries per-block context that adjusts which CodeFinder
// rules are eligible for this block.
//
// suppressLeadingAnchored: drop rules anchored at `^` when applying to
// this block. Mirrors okapi's behaviour where the InlineCodeFinder
// processes a TextFragment's coded-text representation — when the
// fragment has a leading inline-code (e.g. a Font/Marker code created
// by paraCodeBuf in MIFFilter.processPara, MIFFilter.java:693-734),
// the coded text begins with a marker character (U+E101..U+E103) and
// `^[A-Z]:` cannot match at offset 0. Native runs split at code
// boundaries instead of carrying an in-text marker, so the equivalent
// gating must be applied explicitly via this flag.
type codeFinderCtx struct {
	suppressLeadingAnchored bool
}

// splitFormatValueOnHardReturns splits a format/PgfNumFormat value into
// hard-return-delimited segments. When ExtractHardReturnsAsText is true (or
// the value has no newline) the whole value is one segment; otherwise each
// '\n' starts a new segment, mirroring okapi's per-'\n' TextUnit flushing
// for PgfNumFormat referents (MIFFilter.java:1578-1607). Empty segments are
// kept here (the caller drops blocks that simplify to no text).
func (r *Reader) splitFormatValueOnHardReturns(value string) []string {
	if r.cfg.ExtractHardReturnsAsText || !strings.Contains(value, "\n") {
		return []string{value}
	}
	return strings.Split(value, "\n")
}

func (r *Reader) applyCodeFinderCtx(block *model.Block, ctx codeFinderCtx) {
	patterns := r.cfg.GetCodeFinderPatterns()
	if len(patterns) == 0 || block == nil {
		return
	}
	if ctx.suppressLeadingAnchored {
		filtered := patterns[:0:0]
		for _, p := range patterns {
			if strings.HasPrefix(p.String(), "^") {
				continue
			}
			filtered = append(filtered, p)
		}
		patterns = filtered
		if len(patterns) == 0 {
			return
		}
	}
	applyCodeFinderToBlock(block, patterns)
}

// applyCodeFinderToRuns rewrites TextRun content in runs so that every
// CodeFinder regex match becomes a Ph (placeholder) run carrying the
// original literal in its Data field. The writer emits Ph.Data verbatim
// via RenderRunsWithData, so inline FrameMaker codes survive the
// round-trip even when target text is generated via pseudo or MT.
//
// Mirrors po.applyCodeFinderToSegments — kept colocated with the mif
// reader to avoid an extra cross-package dependency. The two should
// stay in sync.
func applyCodeFinderToRuns(runs []model.Run, patterns []*regexp.Regexp) []model.Run {
	if len(runs) == 0 {
		return runs
	}
	// Process per-run so existing inline-code (Ph) runs produced by
	// buildParaRuns survive: only TextRun content is split. spanID is
	// shared across the whole run sequence so generated placeholder ids
	// stay unique.
	spanID := 1
	var out []model.Run
	for _, run := range runs {
		if run.Text == nil {
			out = append(out, run)
			continue
		}
		text := run.Text.Text
		var matches [][2]int
		for _, re := range patterns {
			for _, loc := range re.FindAllStringIndex(text, -1) {
				matches = append(matches, [2]int{loc[0], loc[1]})
			}
		}
		if len(matches) == 0 {
			out = append(out, run)
			continue
		}
		// Sort matches by start, drop overlaps (keep the earlier match).
		for i := 1; i < len(matches); i++ {
			for j := i; j > 0 && matches[j][0] < matches[j-1][0]; j-- {
				matches[j], matches[j-1] = matches[j-1], matches[j]
			}
		}
		merged := matches[:0]
		for _, m := range matches {
			if len(merged) > 0 && m[0] < merged[len(merged)-1][1] {
				continue
			}
			merged = append(merged, m)
		}
		matches = merged

		lastEnd := 0
		for _, m := range matches {
			if m[0] > lastEnd {
				out = append(out, model.Run{Text: &model.TextRun{Text: text[lastEnd:m[0]]}})
			}
			out = append(out, model.Run{Ph: &model.PlaceholderRun{
				ID:   fmt.Sprintf("c%d", spanID),
				Data: text[m[0]:m[1]],
			}})
			spanID++
			lastEnd = m[1]
		}
		if lastEnd < len(text) {
			out = append(out, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
		}
	}
	return out
}

// applyCodeFinderToBlock applies applyCodeFinderToRuns to a block's
// source runs and every committed target's runs.
func applyCodeFinderToBlock(block *model.Block, patterns []*regexp.Regexp) {
	block.SetSourceRuns(applyCodeFinderToRuns(block.Source, patterns))
	for _, loc := range block.TargetLocales() {
		block.SetTargetRuns(loc, applyCodeFinderToRuns(block.TargetRuns(loc), patterns))
	}
}

// processContainer recursively processes a MIF container for translatable strings.
func (r *Reader) processContainer(ctx context.Context, ch chan<- model.PartResult, stmt *mifStatement, blockCounter, dataCounter int) (int, int) {
	for _, child := range stmt.children {
		if child.tag == "Para" {
			// Inline <Pgf><PgfNumFormat> override is extracted as a
			// standalone translatable Block, emitted BEFORE the Para
			// text so the blockIdx ordering matches the source-file
			// scan order used by findStringPositions. Mirrors okapi
			// MIFFilter.java:1078-1112 where a non-empty inline
			// PgfNumFormat always yields a translatable unit (as
			// referent when extractPgfNumFormatsInline=false, as a
			// paraTextBuf merge when true).
			for _, gc := range child.children {
				if gc.tag != "Pgf" {
					continue
				}
				for _, ggc := range gc.children {
					if ggc.tag != "PgfNumFormat" || ggc.value == "" {
						continue
					}
					// Hard-return splitting: when ExtractHardReturnsAsText is
					// false a '\n' in the PgfNumFormat referent starts a new
					// TextUnit (okapi MIFFilter.java:1578-1607).
					for _, seg := range r.splitFormatValueOnHardReturns(ggc.value) {
						blockCounter++
						b := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), seg)
						b.Name = fmt.Sprintf("pgf_num_format_inline.%d", blockCounter)
						r.applyCodeFinderWithExtras(b, []*regexp.Regexp{pgfNumFormatLeadingPrefix})
						simplifyBlockCodes(b)
						if !blockHasText(b) {
							continue
						}
						if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
							return blockCounter, dataCounter
						}
					}
				}
			}

			// <Marker> MText extraction inside ParaLines, mirroring
			// okapi MIFFilter.java:1128-1133 + processMarker (829-883).
			// Index markers (when ExtractIndexMarkers=true) and
			// Hypertext markers (when ExtractLinks=true) become
			// translatable referent units. Emitted before the Para text
			// because <Marker> always appears before the surrounding
			// <String> in source order — keeping the emit order matched
			// to the file order keeps findStringPositions' linear scan
			// monotonic.
			for _, gc := range child.children {
				if gc.tag != "ParaLine" {
					continue
				}
				for _, lc := range gc.children {
					if lc.tag != "Marker" {
						continue
					}
					if !r.extractMarker(lc) {
						continue
					}
					mt := markerTextValue(lc)
					if mt == "" {
						continue
					}
					// Hard-return splitting (okapi addReferentTextUnits,
					// MIFFilter.java:893-921): with ExtractHardReturnsAsText
					// off a '\n' in the marker text starts a new referent unit.
					for _, seg := range r.splitFormatValueOnHardReturns(mt) {
						blockCounter++
						b := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), seg)
						b.Name = fmt.Sprintf("marker.%d", blockCounter)
						r.applyCodeFinder(b)
						simplifyBlockCodes(b)
						if !blockHasText(b) {
							continue
						}
						if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
							return blockCounter, dataCounter
						}
					}
				}
			}

			// Build the paragraph as ONE translatable Block whose source
			// runs interleave text and inline-code (Ph) runs, mirroring
			// okapi's processPara which composes a single TextFragment per
			// paragraph with inline Codes (MIFFilter.java:636-811). A
			// paragraph with no non-whitespace text (only codes / glyph
			// whitespace) yields no Block — okapi's tf.hasText() gate
			// (MIFFilter.java:781).
			units := buildParaRuns(child, r.cfg.ExtractHardReturnsAsText)
			if len(units) == 0 {
				continue
			}
			var pgfTag string
			for _, gc := range child.children {
				if gc.tag == "PgfTag" {
					pgfTag = gc.value
					break
				}
			}
			for _, pr := range units {
				blockCounter++
				block := model.NewRunsBlock(fmt.Sprintf("tu%d", blockCounter), pr.runs)
				block.Name = fmt.Sprintf("para.%d", blockCounter)
				if pgfTag != "" {
					block.Properties["pgf_tag"] = pgfTag
				}
				// The leading-anchored codeFinder rules (`^[A-Z]:`) only
				// match when the paragraph's first run is text. Okapi gates
				// the same way: a leading inline Code pushes the `^` anchor
				// past offset 0, so when the first run is a Ph code we
				// suppress those rules.
				leadingCode := len(pr.runs) > 0 && pr.runs[0].Ph != nil
				r.applyCodeFinderCtx(block, codeFinderCtx{
					suppressLeadingAnchored: leadingCode,
				})
				simplifyBlockCodes(block)
				if !blockHasText(block) {
					continue
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return blockCounter, dataCounter
				}
			}
		} else if child.tag == "Tbl" {
			// Body-page scoping (okapi Extracts.tableExtractable,
			// MIFFilter.java:543-559): a table is walked only when its TblID
			// is referenced from an extractable text flow / table chain.
			if !r.scope.tableInScope(child.firstLiteral("TblID")) {
				continue
			}
			blockCounter, dataCounter = r.processContainer(ctx, ch, child, blockCounter, dataCounter)
		} else if isMIFContainer(child.tag) {
			blockCounter, dataCounter = r.processContainer(ctx, ch, child, blockCounter, dataCounter)
		}
	}
	return blockCounter, dataCounter
}

// isMIFContainer reports whether tag is a structural MIF wrapper that
// the reader walks through to find Para children. Kept in one place so
// processContainer (emit-side) and findStringPositions (skeleton-ref
// side) stay in lock-step — drift between the two manifests as a
// blockIdx misalignment that scrambles translated output across String
// slots.
func isMIFContainer(tag string) bool {
	switch tag {
	case "TextFlow", "Notes", "Tbls", "Tbl",
		"TblBody", "TblH", "TblF",
		"TblTitle", "TblTitleContent",
		"Row", "Cell", "CellContent",
		// FNote (footnote container) holds <Para>; Footnote kept for
		// older fixtures using the long form. Both must be walked so
		// the contained Para text reaches processContainer.
		"FNote", "Footnote":
		return true
	}
	return false
}

// paraTextRun describes one translatable text run inside a Para -- the
// text that accumulates between (or before/after) inline-code statements
// inside the para's ParaLines. Each run becomes one Block on emit and
// one entry in findStringPositions' items list.
//
// stringOffsetIndices captures the sequential index (0-based, within
// the para) of every `<String>` whose value contributes to this run.
// findStringPositions uses these to know which `<String>` raw-text
// positions to coalesce into a single skeleton ref. For example, a
// para like `<String 'a'><String 'b'><Font...><String 'c'>` produces two
// runs: ["ab", indices 0,1] and ["c", index 2].
//
// precededByInlineCode reports whether an inline-code statement (Font,
// Marker, AFrame, XRef, …) appeared in the same Para before this run's
// text accumulated. Mirrors okapi's TextFragment composition in
// MIFFilter.processPara (MIFFilter.java:693-733): when paraCodeBuf has
// content at the time text is appended, the resulting TF starts with a
// leading code marker character, so codeFinder rules anchored at `^`
// (like `^[A-Z]:`) cannot match. Native runs are split at code
// boundaries — we don't model the leading code as an in-run marker —
// so we track the "preceded by code" condition explicitly to gate the
// leading-letter rule.
type paraTextRun struct {
	text                 string
	stringOffsetIndices  []int
	precededByInlineCode bool
}

// extractParaRuns walks a Para's ParaLines and returns the sequence of
// translatable text runs split at inline-code boundaries (Font, Marker,
// AFrame, ...). Mirrors okapi's processPara/readUntilText behavior
// (MIFFilter.java:636-805 + 1027-1175): consecutive `<String>` and
// inlinable `<Char>` content accumulates into one TextFragment until an
// inline-code statement (Font/Marker/AFrame/etc.) is encountered, at
// which point the current TextFragment closes and a new one starts
// after the code.
//
// Within an `<XRef>...<XRefEnd>` pair, ALL content (including Strings)
// is part of the XRef inline code -- no text run accumulates. The first
// String after `<XRefEnd>` starts a fresh run.
//
// The returned slice always has at least one element. Empty runs (no
// translatable text) are dropped from the output.
func extractParaRuns(para *mifStatement, hardReturnsAsText bool) []paraTextRun {
	var runs []paraTextRun
	var cur paraTextRun
	stringIdx := 0
	inXRef := false
	pendingPrecededByCode := false

	flush := func() {
		if cur.text == "" {
			cur = paraTextRun{precededByInlineCode: pendingPrecededByCode}
			return
		}
		runs = append(runs, cur)
		cur = paraTextRun{precededByInlineCode: pendingPrecededByCode}
	}

	for _, child := range para.children {
		if child.tag != "ParaLine" {
			continue
		}
		for _, lc := range child.children {
			switch {
			case lc.tag == "String":
				if inXRef {
					stringIdx++
					continue
				}
				cur.text += lc.value
				cur.stringOffsetIndices = append(cur.stringOffsetIndices, stringIdx)
				stringIdx++
			case lc.tag == "Char":
				if inXRef {
					continue
				}
				if lit, ok := charLiteral(lc.value, hardReturnsAsText); ok {
					cur.text += lit
				}
			default:
				// Any other ParaLine child statement is treated as an
				// inline code: it closes the current text run and starts
				// a new one. Mirrors okapi's default branch in
				// readUntilText (MIFFilter.java:1144-1153) which calls
				// skipOverContent + flips significant=true for any tag
				// that isn't ParaLine/Pgf/String/Char/Marker — the next
				// text appended via paraTextBuf becomes a fresh String
				// in the writer's reconstructed paragraph.
				//
				// XRef is special: while inXRef is true, no Strings
				// contribute to text runs. Track entry/exit explicitly.
				flush()
				pendingPrecededByCode = true
				cur.precededByInlineCode = true
				if lc.tag == "XRef" {
					inXRef = true
				} else if lc.tag == "XRefEnd" {
					inXRef = false
				}
			}
		}
	}
	flush()
	return runs
}

// hasNonWhitespace reports whether s contains at least one non-whitespace
// rune. Mirrors okapi TextFragment.hasText() (TextFragment.java:1194-1212),
// which decides whether a paragraph fragment yields a TextUnit: inline
// codes never count, and whitespace-only text (a lone Tab/ThinSpace glyph)
// does not either.
func hasNonWhitespace(s string) bool {
	for _, r := range s {
		if r == '\u00a0' {
			// NBSP (FrameMaker HardSpace): Java's Character.isWhitespace treats
			// U+00A0 as whitespace, but unicode.IsSpace does not, so handle it
			// explicitly to match okapi's hasText() classification.
			continue
		}
		if !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// paraRunsResult is the output of buildParaRuns: the trimmed ordered run
// sequence for one paragraph plus whether it carries extractable text.
type paraRunsResult struct {
	runs    []model.Run // text + Ph runs, leading/trailing codes & whitespace trimmed
	hasText bool        // true when the paragraph yields a TextUnit
}

// inlineCodeOrdinal is a monotonic id generator shared across the inline
// codes of a single paragraph. Okapi's GenericContent renders codes as
// <1/>, <2/>, … by their assigned id; the id is consumed even when a
// leading code is later trimmed (e.g. testEmptyFTag's leading <AFrame 1>
// is dropped but the next kept code still renders as <2/>). buildParaRuns
// assigns ids in source order so the rendered placeholder numbers match
// okapi.
type inlineCodeOrdinal struct {
	n int
}

func (o *inlineCodeOrdinal) next() int {
	o.n++
	return o.n
}

// paraRunBlockIdxs assigns each extractParaRuns run the blockIdx of the
// Block that processContainer/emitStatements actually emits for it, and
// returns the advanced blockIdx counter. It is the skeleton-side mirror of
// processContainer's emit loop (reader.go processContainer + buildParaRuns):
// one block per EMITTED buildParaRuns unit, where a unit is a Para (or, when
// ExtractHardReturnsAsText is false, a hard-return-delimited segment of it)
// whose source survives the CodeFinder + CodeSimplifier and still carries
// non-whitespace text (okapi tf.hasText() gate, MIFFilter.java:781).
//
// Runs that fall in a unit that emits no block (whitespace-only, or only
// inline-code building blocks like `<$paratext>` / `<n+>`) get -1: they
// produce no skeleton ref, so their original `<String>` bytes stay in the
// skeleton verbatim — matching okapi, which routes such fragments to the
// skeleton via toMIFString (MIFFilter.java:789) rather than extracting them.
//
// extractParaRuns splits at every inline-code boundary; buildParaRuns merges
// across those boundaries and only splits at hard returns. So consecutive
// runs map to ONE shared blockIdx (the merged unit) until a hard-return
// boundary starts the next unit — keeping blockIdx in lock-step with the
// emit side and preventing the off-by-N drift introduced by 5bacf636 (#615).
// formatValueEmitsBlock reports whether a single (already hard-return-split)
// format value — a <PgfNumFormat> or <TextLine>'s <String> — would survive
// the emit-side gate and produce a translatable Block. It mirrors the
// processPgfCatalog / processTextLine pipeline: CodeFinder (plus the ^[A-Z]:
// PgfNumFormat prefix rule when withPgfPrefix is set) → simplifyBlockCodes →
// blockHasText (okapi tf.hasText() after CodeSimplifier). Used by
// findStringPositions so its catalog/textline blockIdx counter skips
// building-block-only values exactly as emitStatements does.
func (r *Reader) formatValueEmitsBlock(value string, withPgfPrefix bool) bool {
	block := model.NewBlock("probe", value)
	if withPgfPrefix {
		r.applyCodeFinderWithExtras(block, []*regexp.Regexp{pgfNumFormatLeadingPrefix})
	} else {
		r.applyCodeFinder(block)
	}
	simplifyBlockCodes(block)
	return blockHasText(block)
}

func (r *Reader) paraRunBlockIdxs(para *mifStatement, runs []paraTextRun, blockIdx int) ([]int, int) {
	hardReturnsAsText := r.cfg.ExtractHardReturnsAsText
	out := make([]int, len(runs))
	for i := range out {
		out[i] = -1
	}

	// Assign each run to a hard-return segment index. When
	// hardReturnsAsText is true (the default) there are no splits, so every
	// run belongs to segment 0. When false, a run whose text contains a
	// '\n' (a HardReturn / hard-return escape that buildParaRuns splits on)
	// closes the current segment AFTER that run, mirroring buildParaRuns'
	// split-on-'\n' behaviour.
	runSeg := make([]int, len(runs))
	seg := 0
	for i, run := range runs {
		runSeg[i] = seg
		if !hardReturnsAsText && strings.Contains(run.text, "\n") {
			seg++
		}
	}

	// Determine, per segment, whether processContainer emits a block — by
	// running the exact same gate (buildParaRuns → CodeFinder → simplify →
	// blockHasText). buildParaRuns returns one result per segment that has
	// non-whitespace text, in source order; we re-apply the CodeFinder gate
	// to discover which of those actually survive (e.g. a `<$paratext>`-only
	// unit yields a buildParaRuns result but is dropped by blockHasText).
	units := buildParaRuns(para, hardReturnsAsText)
	emits := make([]bool, len(units))
	for ui, pr := range units {
		block := model.NewRunsBlock("probe", pr.runs)
		leadingCode := len(pr.runs) > 0 && pr.runs[0].Ph != nil
		r.applyCodeFinderCtx(block, codeFinderCtx{suppressLeadingAnchored: leadingCode})
		simplifyBlockCodes(block)
		emits[ui] = blockHasText(block)
	}

	// buildParaRuns only yields a result for segments that contain
	// non-whitespace text; whitespace-only segments are skipped entirely.
	// Walk the segments in order and pair each text-bearing segment with the
	// next buildParaRuns unit so segment→unit indices line up.
	segHasText := map[int]bool{}
	for i, run := range runs {
		if hasNonWhitespace(run.text) {
			segHasText[runSeg[i]] = true
		}
	}
	maxSeg := 0
	for _, s := range runSeg {
		if s > maxSeg {
			maxSeg = s
		}
	}
	segBlockIdx := make(map[int]int)
	unitCursor := 0
	for s := 0; s <= maxSeg; s++ {
		if !segHasText[s] {
			segBlockIdx[s] = -1
			continue
		}
		if unitCursor >= len(units) {
			segBlockIdx[s] = -1
			continue
		}
		if emits[unitCursor] {
			segBlockIdx[s] = blockIdx
			blockIdx++
		} else {
			segBlockIdx[s] = -1
		}
		unitCursor++
	}

	for i, run := range runs {
		// A run only gets a ref if it carries non-whitespace text that
		// SURVIVES the CodeFinder AND its segment emits a block. A run whose
		// only content is building-block codes (e.g. a `<String '<$pagenum>'>`
		// run between Fonts) collapses to codes-only after applyCodeFinder, so
		// it owns no output text-group — its `<String>` must stay in the
		// skeleton verbatim (okapi keeps `<$pagenum>` unchanged). Using plain
		// hasNonWhitespace here would mis-count `<$pagenum>` as text (the `<`,
		// letters, `>` are non-space) and hand it a phantom run-group, emptying
		// the slot (the 893 `<$pagenum>` / `<$paratext>` cluster).
		if !r.runHasExtractableText(run.text) {
			out[i] = -1
			continue
		}
		out[i] = segBlockIdx[runSeg[i]]
	}
	return out, blockIdx
}

// runHasExtractableText reports whether a single text run, after the
// CodeFinder masks FrameMaker building blocks into inline codes, still
// carries non-whitespace text. Mirrors the per-run contribution to okapi's
// tf.hasText() gate: a run of only `<$pagenum>`/`<n+>`-style codes (and
// whitespace) contributes no extractable text.
func (r *Reader) runHasExtractableText(text string) bool {
	if !hasNonWhitespace(text) {
		return false
	}
	block := model.NewBlock("probe", text)
	r.applyCodeFinder(block)
	return blockHasText(block)
}

// buildParaRuns walks a Para's ParaLines and produces ONE ordered Run
// sequence for the whole paragraph, mirroring okapi's processPara which
// builds a single TextFragment per paragraph with interspersed inline
// Codes (MIFFilter.java:636-811). This is the inline-code model: a
// paragraph is ONE translatable unit, not one-unit-per-text-run.
//
// Rules, grounded in processPara + readUntilText (MIFFilter.java:1027-1175):
//   - <String> values append to the current text run.
//   - glyph <Char> (Tab→"\t", ThinSpace→" ", HardSpace→" ", …)
//     append to the current text run as TEXT, not as a code
//     (CharLiteralToken.java). SoftHyphen and unknown glyphs contribute
//     nothing.
//   - any other ParaLine child statement (Font, AFrame, Dummy, Var, …) is
//     an inline code: it flushes the current text run and becomes a Ph run.
//   - leading codes and leading whitespace-only text are dropped (okapi
//     routes them to the skeleton via the `first` branch + paraSkelBuf,
//     MIFFilter.java:693-711).
//   - trailing codes and trailing whitespace-only text are dropped (okapi
//     leaves them in paraCodeBuf/paraSkelBuf at loop end → skeleton,
//     MIFFilter.java:800-805).
//   - within <XRef>…<XRefEnd> ALL content is part of the XRef code; no
//     text accumulates.
//   - if no non-whitespace text survives, the paragraph yields no unit
//     (okapi's tf.hasText() gate at MIFFilter.java:781).
//
// When hardReturnsAsText is false, a hard return ("\n", produced by a
// <Char HardReturn> or a `\x09`/`\n` escape) splits the paragraph into
// SEPARATE units at each newline, mirroring okapi's processPara
// (MIFFilter.java:739-766) which flushes the current TextFragment as its
// own TextUnit at every '\n'. The result is therefore a slice: one entry
// per hard-return-delimited segment (whitespace-only segments dropped).
func buildParaRuns(para *mifStatement, hardReturnsAsText bool) []paraRunsResult {
	// First pass: emit raw runs in source order (text and code), trimming
	// only the XRef-internal content.
	type rawRun struct {
		text   string // non-empty for a text run
		isCode bool
		ord    int // inline-code ordinal for code runs
	}
	var raw []rawRun
	var ord inlineCodeOrdinal
	inXRef := false

	appendText := func(s string) {
		if s == "" {
			return
		}
		if len(raw) > 0 && !raw[len(raw)-1].isCode {
			raw[len(raw)-1].text += s
			return
		}
		raw = append(raw, rawRun{text: s})
	}

	for _, child := range para.children {
		if child.tag != "ParaLine" {
			continue
		}
		for _, lc := range child.children {
			switch {
			case lc.tag == "String":
				if inXRef {
					continue
				}
				appendText(lc.value)
			case lc.tag == "Char":
				if inXRef {
					continue
				}
				if lit, ok := charLiteral(lc.value, hardReturnsAsText); ok {
					appendText(lit)
				}
			default:
				// Inline code. The XRef…XRefEnd span is a single code in
				// okapi (the whole reference is opaque); emit one code at
				// XRef entry and swallow everything until XRefEnd.
				if lc.tag == "XRef" {
					if !inXRef {
						raw = append(raw, rawRun{isCode: true, ord: ord.next()})
					}
					inXRef = true
					continue
				}
				if lc.tag == "XRefEnd" {
					inXRef = false
					continue
				}
				if inXRef {
					continue
				}
				raw = append(raw, rawRun{isCode: true, ord: ord.next()})
			}
		}
	}

	// Split the raw run list into hard-return-delimited segments. When
	// hardReturnsAsText is true there is exactly one segment (newlines stay
	// as text); when false, each '\n' inside a text run starts a new
	// segment (okapi processPara, MIFFilter.java:739-766).
	type segRun struct {
		isCode bool
		ord    int
		text   string
	}
	var segments [][]segRun
	cur := []segRun{}
	for _, rr := range raw {
		if rr.isCode {
			cur = append(cur, segRun{isCode: true, ord: rr.ord})
			continue
		}
		if hardReturnsAsText || !strings.Contains(rr.text, "\n") {
			cur = append(cur, segRun{text: rr.text})
			continue
		}
		parts := strings.Split(rr.text, "\n")
		for i, p := range parts {
			if p != "" {
				cur = append(cur, segRun{text: p})
			}
			if i < len(parts)-1 {
				// Newline boundary: close the current segment.
				segments = append(segments, cur)
				cur = []segRun{}
			}
		}
	}
	segments = append(segments, cur)

	// Build one paraRunsResult per segment that has non-whitespace text.
	var results []paraRunsResult
	for _, seg := range segments {
		anyText := false
		for _, sr := range seg {
			if !sr.isCode && hasNonWhitespace(sr.text) {
				anyText = true
				break
			}
		}
		if !anyText {
			continue
		}
		// Emit runs in source order. Leading/trailing code + adjacent-
		// whitespace trimming happens afterwards in simplifyBlockCodes,
		// matching okapi's order (codeFinder + CodeSimplifier run AFTER the
		// TextFragment is composed). Trimming here would discard codes
		// before their ordinals are assigned and over-trim whitespace okapi
		// keeps (e.g. the trailing space in "1000 Main Street ").
		var runs []model.Run
		for _, sr := range seg {
			if sr.isCode {
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
					ID:    strconv.Itoa(sr.ord),
					Equiv: fmt.Sprintf("<%d/>", sr.ord),
				}})
				continue
			}
			if sr.text == "" {
				continue
			}
			runs = append(runs, model.Run{Text: &model.TextRun{Text: sr.text}})
		}
		results = append(results, paraRunsResult{runs: runs, hasText: true})
	}
	return results
}

// parseMIF parses a MIF document into a list of top-level statements.
//
// MIF lines come in three shapes:
//
//  1. Single-line statement: <Tag value>            (closes on same line)
//  2. Single-line with comment: <Tag value> # comment
//  3. Multi-line opener: <Tag                       (children follow)
//
// The closer for (3) is a `>` token, optionally followed by `#` comment
// text on the same line. The parser must recognise both the inline `>`
// in (1)/(2) and the closer-with-comment so containers like
// VariableFormats / VariableFormat actually pop the stack.
func parseMIF(content string) []*mifStatement {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var stmts []*mifStatement
	var stack []*mifStatement

	// raw is only ever read for top-level statements (emitStatements +
	// the non-skeleton writer.go fallback). Rather than copying each
	// child's bytes up into every ancestor on pop — which is O(file ×
	// tree-depth) memory and O(n²) CPU as the growing parent string is
	// re-copied per `+=` — we accumulate the top-level statement's raw
	// text exactly once, line by line in document order, into a single
	// builder for the currently-open root statement. The reconstructed
	// byte stream is identical (every line is re-emitted as line+"\n",
	// blank lines skipped); only the copy count changes.
	var rootBuilder strings.Builder

	// appendLine records line+"\n" into the root statement's raw text.
	// When the stack is empty there is no open multi-line top-level
	// statement, so the line belongs to no root and is dropped (matching
	// the old behaviour where such lines went only to a discarded
	// package-local builder).
	appendLine := func(line string) {
		if len(stack) > 0 {
			rootBuilder.WriteString(line)
			rootBuilder.WriteByte('\n')
		}
	}

	popStack := func(line string) {
		if len(stack) == 0 {
			return
		}
		current := stack[len(stack)-1]
		appendLine(line)
		stack = stack[:len(stack)-1]
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, current)
		} else {
			// Closing a top-level multi-line statement: finalize its raw.
			current.raw = rootBuilder.String()
			rootBuilder.Reset()
			stmts = append(stmts, current)
		}
	}

	pushSingleLine := func(line, tagSrc string) {
		// tagSrc is the in-tag content WITHOUT surrounding `<` `>`,
		// i.e. just `Tag value` or `Tag` — used to derive tag/value.
		tag, after, hasVal := strings.Cut(tagSrc, " ")
		stmt := &mifStatement{tag: tag}
		if hasVal {
			stmt.value = unquoteMIF(after)
			stmt.rawValue = unquoteMIFRaw(after)
		}
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, stmt)
			appendLine(line)
		} else {
			// Single-line top-level statement: its raw is just this line.
			stmt.raw = line + "\n"
			stmts = append(stmts, stmt)
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		// Closer line: `>` possibly followed by `# comment`.
		if isCloserLine(trimmed) {
			popStack(line)
			continue
		}

		if strings.HasPrefix(trimmed, "<") {
			// Find the unescaped `>` that closes the tag (if any) on
			// this line. This handles `<Tag value>` and
			// `<Tag value> # comment` uniformly.
			if closeIdx := findInlineClose(trimmed); closeIdx >= 0 {
				inner := trimmed[1:closeIdx]
				pushSingleLine(line, inner)
				continue
			}

			// Multi-line opener: tag spans lines, children follow.
			tag, value := parseTagLine(trimmed)
			stmt := &mifStatement{
				tag:   tag,
				value: value,
			}
			// Record the opener into the root builder. For a nested
			// opener the stack is already non-empty so appendLine works;
			// for a new top-level opener the stack is still empty here, so
			// write the opener directly (the root builder was just reset
			// by the previous top-level pop).
			if len(stack) > 0 {
				appendLine(line)
			} else {
				rootBuilder.WriteString(line)
				rootBuilder.WriteByte('\n')
			}
			stack = append(stack, stmt)
			continue
		}

		// Non-statement line (comment or other content).
		appendLine(line)
	}

	// Flush any unclosed statements (defensive — a well-formed MIF will
	// have already popped them above).
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, current)
		} else {
			current.raw = rootBuilder.String()
			rootBuilder.Reset()
			stmts = append(stmts, current)
		}
	}

	return stmts
}

// isCloserLine reports whether trimmed is a stack-closer line. The
// canonical form is `>` alone or `> # ...comment`.
func isCloserLine(trimmed string) bool {
	if trimmed == ">" {
		return true
	}
	if !strings.HasPrefix(trimmed, ">") {
		return false
	}
	rest := strings.TrimSpace(trimmed[1:])
	return strings.HasPrefix(rest, "#")
}

// findInlineClose returns the index of the `>` that closes a tag on a
// `<Tag …>` line, or -1 when the tag continues on subsequent lines. The
// `>` is recognised when:
//   - it is unescaped (preceding rune is not `\`)
//   - it sits at end-of-line (with optional trailing whitespace) OR is
//     followed by `# …` comment text on the same line
//
// String-literal regions (`…'`) are skipped so a `>` inside a backtick-
// quoted MIF value (e.g. `<VariableDef '<$daynum\>'>`) doesn't mis-close
// the tag. MIF escapes inline `>` inside such strings as `\>`, so we
// also bail past `\>` defensively.
func findInlineClose(s string) int {
	inString := false
	for i := 1; i < len(s); i++ {
		c := s[i]
		// Track entry/exit of MIF string literals: opens on `\``, closes
		// on `'`. The opening backtick is never escaped.
		if !inString && c == '`' {
			inString = true
			continue
		}
		if inString && c == '\'' {
			// Backslash-escaped quote stays inside the string.
			if i > 0 && s[i-1] == '\\' {
				continue
			}
			inString = false
			continue
		}
		if inString {
			continue
		}
		if c != '>' {
			continue
		}
		// Backslash-escaped `>` is not a closer.
		if i > 0 && s[i-1] == '\\' {
			continue
		}
		// What follows? Whitespace+EOL or whitespace+`#` is a real
		// closer; anything else (an alphanumeric run, etc.) means the
		// `>` is a literal and we keep scanning.
		rest := strings.TrimLeft(s[i+1:], " \t")
		if rest == "" || strings.HasPrefix(rest, "#") {
			return i
		}
	}
	return -1
}

// parseTagLine parses a MIF line like "<TagName value" or "<TagName".
func parseTagLine(line string) (string, string) {
	// Remove leading '<'.
	line = strings.TrimPrefix(line, "<")
	// Remove trailing '>' if present (single-line statement).
	line = strings.TrimSuffix(line, ">")
	line = strings.TrimSpace(line)

	tag, after, hasVal := strings.Cut(line, " ")
	var value string
	if hasVal {
		value = unquoteMIF(strings.TrimSpace(after))
	}
	return tag, value
}

// unquoteMIF strips the MIF backtick…quote delimiters and decodes the
// in-string escapes (`\>` → `>`, `\\` → `\`, `\t` → tab, `\n` →
// newline, `\\` → `\`, `\q` → `'`, `\Q` → “ ` “). The writer's
// escapeMIF re-encodes the same set on output, so values round-trip
// faithfully when no translation transforms them.
func unquoteMIF(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '\'' {
		s = s[1 : len(s)-1]
	}
	return unescapeMIFString(s)
}

// unquoteMIFRaw strips the MIF backtick…quote delimiters but preserves
// the body bytes verbatim (no escape decoding). Mirrors what
// findStringPositions needs to locate the exact source bytes for a
// `<String>` / `<PgfNumFormat>` / `<VariableDef>` value in rawText.
// Differs from unquoteMIF only when the source used `\xNN ` hex escapes
// that Hexadecimal.toString collapses to a different Unicode form on
// decode -- escapeMIF re-encodes the decoded form canonically (`\t`,
// `\n`, …), so the raw source bytes can't be reconstructed from value
// alone (MIFFilter.java:1804-1819 readHexa).
func unquoteMIFRaw(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '\'' {
		s = s[1 : len(s)-1]
	}
	return s
}

// unescapeMIFString decodes the in-string MIF escape sequences. The
// inverse of escapeMIF in writer.go. Anything outside the recognised
// set passes through verbatim — robust against partial sequences in
// hand-written fixtures.
//
// The `\xNN ` (2-hex-digit + mandatory trailing space) form mirrors
// okapi MIFFilter.readHexa (MIFFilter.java:1804-1819) which calls
// Hexadecimal.toString to map the integer value to a Unicode equivalent:
// `\x08` -> tab, `\x09` -> LF, `\x11` -> NBSP (U+00A0), `\x10` -> figure
// space (U+2007), etc. The trailing space byte IS consumed (per the
// `readExtraSpace=true` argument okapi passes for `\x`). Values not in
// the table fall through to literal `\xNN ` preservation so unknown
// codes survive round-trip.
func unescapeMIFString(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		next := s[i+1]
		switch next {
		case '\\', '`', '\'', '>':
			b.WriteByte(next)
			i++
		case 't':
			b.WriteByte('\t')
			i++
		case 'n':
			b.WriteByte('\n')
			i++
		case 'q':
			b.WriteByte('\'')
			i++
		case 'Q':
			b.WriteByte('`')
			i++
		case 'x':
			// `\xNN ` — 2 hex digits plus a mandatory trailing space
			// (consumed). Mirrors MIFFilter.readHexa with
			// readExtraSpace=true (MIFFilter.java:1813-1819).
			if i+4 < len(s) && isHexDigit(s[i+2]) && isHexDigit(s[i+3]) && s[i+4] == ' ' {
				v := (hexDigitValue(s[i+2]) << 4) | hexDigitValue(s[i+3])
				if lit, ok := hexadecimalLiteral(v); ok {
					b.WriteString(lit)
					i += 4
					continue
				}
			}
			// Unknown / malformed `\xNN` — preserve bytes verbatim.
			b.WriteByte(c)
		default:
			// Unknown escape — keep both bytes so encoder/decoder
			// round-trip remains lossless.
			b.WriteByte(c)
		}
	}
	return b.String()
}

// hexadecimalLiteral mirrors okapi's Hexadecimal.toString
// (Hexadecimal.java:43-84) — maps the integer value of a `\xNN ` MIF
// escape to its Unicode literal. Values not in this table return
// (zero, false) so the caller can preserve the source bytes verbatim
// (okapi logs a warning and emits the `\xNN ` form wrapped in inline-code
// brackets; native preserves the raw form which matches byte-equal
// when no translation transforms it).
func hexadecimalLiteral(v int) (string, bool) {
	switch v {
	case 0x04:
		return "\u00ad", true // Discretionary hyphen (U+00AD)
	case 0x05:
		return "\u200d", true // No hyphen / ZWJ (U+200D)
	case 0x06:
		return "", true // Removed entirely per okapi
	case 0x08:
		return "\t", true // Tab
	case 0x09:
		return "\n", true // Forced return / line break
	case 0x10:
		return " ", true // Numeric / figure space
	case 0x11:
		return " ", true // Non-breaking space
	case 0x12:
		return " ", true // Thin space
	case 0x13:
		return " ", true // En space
	case 0x14:
		return " ", true // Em space
	case 0x15:
		return "‑", true // Non-breaking / hard hyphen
	}
	return "", false
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func hexDigitValue(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10
	}
	return 0
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

func (r *Reader) emitErr(ctx context.Context, ch chan<- model.PartResult, err error) {
	select {
	case ch <- model.PartResult{Error: err}:
	case <-ctx.Done():
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
