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
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for MIF (Maker Interchange Format) files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
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

// stringRef records the byte position of a String value and its block association.
type stringRef struct {
	startOffset int // byte offset of the String value content start (after backtick)
	endOffset   int // byte offset of the String value content end (before quote)
	blockIdx    int // which block (0-based)
	stringIdx   int // which string within the block (0-based)
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

	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitErr(ctx, ch, fmt.Errorf("mif: read error: %w", err))
		return
	}
	rawText := string(data)

	stmts := parseMIF(rawText)
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
				refID: fmt.Sprintf("%d:%d", sr.blockIdx, sr.stringIdx),
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
							pgfBlockIdxs = append(pgfBlockIdxs, blockIdx)
							pgfValues = append(pgfValues, ggc.value)
							blockIdx++
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
						blockIdx:  pgfBlockIdxs[i],
						strings:   []string{v},
						searchTag: "PgfNumFormat",
					})
				}
				// Pre-assign blockIdx for each run in EMIT order
				// (matches processContainer's runs loop). Char-only runs
				// also get a blockIdx because processContainer emits a
				// Block for any run with non-empty text.
				runBlockIdxs := make([]int, len(runs))
				for i, run := range runs {
					if run.text == "" {
						runBlockIdxs[i] = -1
						continue
					}
					runBlockIdxs[i] = blockIdx
					blockIdx++
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
								})
							}
						case "String":
							ri, ok := stringToRun[stringPosCursor]
							stringPosCursor++
							if !ok || runEmitted[ri] || runBlockIdxs[ri] < 0 {
								continue
							}
							runEmitted[ri] = true
							strs := runStrings[ri]
							if len(strs) == 0 {
								// Shouldn't happen for a String-driven
								// run, but keep defensive.
								continue
							}
							items = append(items, itemInfo{
								blockIdx:      runBlockIdxs[ri],
								strings:       strs,
								searchTag:     "String",
								inlineChars:   runInlineChars[ri],
								stringsAreRaw: true,
							})
						case "Char":
							owner := charOwnerRun[charPosCursor]
							charPosCursor++
							if owner < 0 || runEmitted[owner] || runBlockIdxs[owner] < 0 {
								continue
							}
							runEmitted[owner] = true
							strs := runStrings[owner]
							if len(strs) == 0 {
								// Char-only run -- emit a CharRewrite
								// item.
								items = append(items, itemInfo{
									blockIdx:        runBlockIdxs[owner],
									searchTag:       "CharRewrite",
									paraCharRewrite: runCharRewrites[owner],
								})
							} else {
								items = append(items, itemInfo{
									blockIdx:      runBlockIdxs[owner],
									strings:       strs,
									searchTag:     "String",
									inlineChars:   runInlineChars[owner],
									stringsAreRaw: true,
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
				blockIdx:  blockIdx,
				strings:   []string{defStmt.value},
				searchTag: "VariableDef",
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
				items = append(items, itemInfo{blockIdx: blockIdx, strings: []string{gc.value}, searchTag: "PgfNumFormat"})
				blockIdx++
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
				walkFramesAndTextLines(child)
			case "TextLine":
				val, ok := firstStringRawValue(child)
				if !ok {
					continue
				}
				items = append(items, itemInfo{
					blockIdx:      blockIdx,
					strings:       []string{val},
					searchTag:     "String",
					stringsAreRaw: true,
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

			if stringInItemIdx == 0 {
				// First String in the item -- write the rendered text
				// into the value slot.
				refs = append(refs, stringRef{
					startOffset: valStart,
					endOffset:   valEnd,
					blockIdx:    it.blockIdx,
					stringIdx:   0,
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

		if stmt.tag == "TextFlow" || stmt.tag == "Tbls" || stmt.tag == "Notes" {
			// Process translatable content inside these containers.
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
			blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), gc.value)
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
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return blockCounter
			}
		}
	}
	return blockCounter
}

// applyCodeFinderWithExtras is applyCodeFinder plus an additional
// list of context-specific patterns appended to the global config
// patterns for THIS block only. Both rule sets feed a single
// applyCodeFinderToSegments call so a second pass doesn't undo the
// first (Segment.Text() drops Ph data, so re-running the splitter
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
	applyCodeFinderToSegments(block.Source, merged)
	for _, segs := range block.Targets {
		applyCodeFinderToSegments(segs, merged)
	}
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
			blockCounter, dataCounter = r.processFramesAndTextLines(ctx, ch, child, blockCounter, dataCounter)
		case "TextLine":
			val, ok := firstStringValue(child)
			if !ok {
				continue
			}
			blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), val)
			block.Name = fmt.Sprintf("textline.%d", blockCounter)
			r.applyCodeFinder(block)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return blockCounter, dataCounter
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
	applyCodeFinderToSegments(block.Source, patterns)
	for _, segs := range block.Targets {
		applyCodeFinderToSegments(segs, patterns)
	}
}

// applyCodeFinderToSegments rewrites TextRun content in segs so that
// every CodeFinder regex match becomes a Ph (placeholder) run carrying
// the original literal in its Data field. The writer emits Ph.Data
// verbatim via RenderRunsWithData, so inline FrameMaker codes survive
// the round-trip even when target text is generated via pseudo or MT.
//
// Mirrors po.applyCodeFinderToSegments — kept colocated with the mif
// reader to avoid an extra cross-package dependency. The two should
// stay in sync.
func applyCodeFinderToSegments(segs []*model.Segment, patterns []*regexp.Regexp) {
	for _, seg := range segs {
		if seg == nil || len(seg.Runs) == 0 {
			continue
		}
		text := seg.Text()
		var matches [][2]int
		for _, re := range patterns {
			for _, loc := range re.FindAllStringIndex(text, -1) {
				matches = append(matches, [2]int{loc[0], loc[1]})
			}
		}
		if len(matches) == 0 {
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

		var runs []model.Run
		lastEnd := 0
		spanID := 1
		for _, m := range matches {
			if m[0] > lastEnd {
				runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:m[0]]}})
			}
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID:   fmt.Sprintf("c%d", spanID),
				Data: text[m[0]:m[1]],
			}})
			spanID++
			lastEnd = m[1]
		}
		if lastEnd < len(text) {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
		}
		seg.Runs = runs
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
					blockCounter++
					b := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), ggc.value)
					b.Name = fmt.Sprintf("pgf_num_format_inline.%d", blockCounter)
					r.applyCodeFinderWithExtras(b, []*regexp.Regexp{pgfNumFormatLeadingPrefix})
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
						return blockCounter, dataCounter
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
					blockCounter++
					b := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), mt)
					b.Name = fmt.Sprintf("marker.%d", blockCounter)
					r.applyCodeFinder(b)
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
						return blockCounter, dataCounter
					}
				}
			}

			// Split the para's text into runs at inline-code boundaries
			// (Font, Marker, AFrame, XRef, …). Each non-empty run becomes
			// its own translatable Block so the writer can emit the
			// `<String '...'><Font ...><String '...'>` interleaving that
			// okapi's writeParagraph reconstructs from the per-Para
			// TextFragment + inline codes (MIFFilter.java:636-805).
			//
			// Single-run paras (no inline codes between strings) are
			// emitted as before — one Block per Para — so the existing
			// 17 byte-equal MIF fixtures remain unchanged.
			runs := extractParaRuns(child, r.cfg.ExtractHardReturnsAsText)
			var pgfTag string
			for _, gc := range child.children {
				if gc.tag == "PgfTag" {
					pgfTag = gc.value
					break
				}
			}
			for runIdx, run := range runs {
				if run.text == "" {
					continue
				}
				blockCounter++
				block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), run.text)
				block.Name = fmt.Sprintf("para.%d.%d", blockCounter, runIdx)
				if pgfTag != "" {
					block.Properties["pgf_tag"] = pgfTag
				}
				r.applyCodeFinderCtx(block, codeFinderCtx{
					suppressLeadingAnchored: run.precededByInlineCode,
				})
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return blockCounter, dataCounter
				}
			}
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
