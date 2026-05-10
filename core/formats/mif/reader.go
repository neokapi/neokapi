package mif

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
}

// stringRef records the byte position of a String value and its block association.
type stringRef struct {
	startOffset int // byte offset of the String value content start (after backtick)
	endOffset   int // byte offset of the String value content end (before quote)
	blockIdx    int // which block (0-based)
	stringIdx   int // which string within the block (0-based)
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
		refs := r.findStringPositions(rawText, stmts)
		skelPos := 0
		for _, sr := range refs {
			if sr.startOffset > skelPos {
				r.skelText(rawText[skelPos:sr.startOffset])
			}
			refID := fmt.Sprintf("%d:%d", sr.blockIdx, sr.stringIdx)
			r.skelRef(refID)
			skelPos = sr.endOffset
		}
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// findStringPositions scans raw MIF content for <String `...'> and
// <VariableDef `...'> patterns and associates them with block indices
// based on the parsed statement tree. The block ordering must match the
// order in which emitStatements emits Block parts so that
// writeFromSkeleton's `blockIdx -> w.blocks[blockIdx]` lookup is
// correct.
func (r *Reader) findStringPositions(rawText string, stmts []*mifStatement) []stringRef {
	// Walk the top-level statement list once to enumerate translatable
	// items in emission order. Two kinds participate today:
	//   - Para under TextFlow/Tbls/Notes (each Para → 1 block, may have
	//     multiple <String> children inside its <ParaLine>s)
	//   - VariableDef under VariableFormats (each VariableDef → 1 block,
	//     exactly 1 string)
	// Both share the same `blockIdx:stringIdx` skeleton-ref scheme so the
	// writer can patch them uniformly.
	type itemInfo struct {
		blockIdx  int
		strings   []string // values in order
		searchTag string   // "String" or "VariableDef"
	}
	var items []itemInfo
	blockIdx := 0

	// Mirror exactly the recursion in processContainer +
	// processVariableFormats so the blockIdx of every translatable item
	// here matches the index assigned by emitStatements.
	var walkContainer func(stmt *mifStatement)
	walkContainer = func(stmt *mifStatement) {
		for _, child := range stmt.children {
			if child.tag == "Para" {
				// Inline <Pgf><PgfNumFormat> override comes BEFORE the
				// para text in the source file, so emit it first.
				// Mirrors okapi MIFFilter.java:1078-1112: when an inline
				// PgfNumFormat is non-empty, okapi extracts it as a
				// translatable text unit (as referent when
				// extractPgfNumFormatsInline=false, as a paraTextBuf
				// merge when true). Either way it IS extracted; native
				// emits it as a standalone Block before the Para's text.
				for _, gc := range child.children {
					if gc.tag != "Pgf" {
						continue
					}
					for _, ggc := range gc.children {
						if ggc.tag == "PgfNumFormat" && ggc.value != "" {
							items = append(items, itemInfo{
								blockIdx:  blockIdx,
								strings:   []string{ggc.value},
								searchTag: "PgfNumFormat",
							})
							blockIdx++
						}
					}
				}
				// <Marker> entries inside ParaLines emit standalone
				// MText blocks (interleaved with the para text in source
				// order). Mirrors okapi MIFFilter.java:1128-1133 +
				// processMarker (829-883) — markers with MTypeName
				// 'Index' (when ExtractIndexMarkers=true) or 'Hypertext'
				// (when ExtractLinks=true) become translatable referent
				// units.
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
						items = append(items, itemInfo{
							blockIdx:  blockIdx,
							strings:   []string{mt},
							searchTag: "MText",
						})
						blockIdx++
					}
				}
				// Mirror processContainer: split the para into runs at
				// inline-code boundaries. Each run with non-empty text
				// gets its own Block (and skeleton ref). Within a single
				// run, multiple `<String>` values still collapse into the
				// run's first String slot — the writer fills slot 0 and
				// elides the rest.
				runs := extractParaRuns(child, true)
				// Collect every <String> value in source order so we can
				// pick out the indices each run should occupy.
				var allStrings []string
				for _, gc := range child.children {
					if gc.tag == "ParaLine" {
						for _, lc := range gc.children {
							if lc.tag == "String" {
								allStrings = append(allStrings, lc.value)
							}
						}
					}
				}
				for _, run := range runs {
					if strings.TrimSpace(run.text) == "" {
						continue
					}
					strs := make([]string, 0, len(run.stringOffsetIndices))
					for _, off := range run.stringOffsetIndices {
						if off < len(allStrings) {
							strs = append(strs, allStrings[off])
						}
					}
					if len(strs) == 0 {
						// Run made entirely of Char-translated text; the
						// skeleton has no <String> position to anchor to,
						// so emit nothing here. The Para wrapper itself
						// stays in skeleton text and the writer's data
						// path will not fire — matches okapi's behavior
						// for inline-only Char content.
						blockIdx++
						continue
					}
					items = append(items, itemInfo{
						blockIdx:  blockIdx,
						strings:   strs,
						searchTag: "String",
					})
					blockIdx++
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
				val, ok := firstStringValue(child)
				if !ok {
					continue
				}
				items = append(items, itemInfo{
					blockIdx:  blockIdx,
					strings:   []string{val},
					searchTag: "String",
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
	itemIdx := 0
	stringInItemIdx := 0
	searchFrom := 0

	for itemIdx < len(items) {
		it := items[itemIdx]
		if stringInItemIdx >= len(it.strings) {
			itemIdx++
			stringInItemIdx = 0
			continue
		}

		expectedVal := it.strings[stringInItemIdx]
		pattern := "<" + it.searchTag + " `" + escapeMIFForSearch(expectedVal) + "'>"
		idx := strings.Index(rawText[searchFrom:], pattern)
		if idx < 0 {
			// Skip — should not happen with well-formed MIF.
			stringInItemIdx++
			continue
		}

		absIdx := searchFrom + idx
		valStart := absIdx + len("<"+it.searchTag+" `")
		valEnd := valStart + len(escapeMIFForSearch(expectedVal))

		// For non-first <String> refs inside a multi-ParaLine <Para>, widen
		// the ref so it swallows the entire surrounding `<ParaLine ...
		// > # end of ParaLine\n` block. The native reader merges every
		// String inside one Para into a single Block (matching okapi's
		// per-Para text unit), so the writer emits the merged translated
		// text into the FIRST String only and writes nothing for
		// stringIdx>0 — but without this widening, the surrounding empty
		// `<ParaLine><String \`'>` skeleton would still leak through.
		// okapi's MIFFilter (writeParagraph) collapses the multi-ParaLine
		// shape into a single ParaLine on output, so we mirror that by
		// dropping the trailing ParaLine wrappers from the skeleton.
		swallowStart, swallowEnd := valStart, valEnd
		if it.searchTag == "String" && stringInItemIdx > 0 {
			swallowStart, swallowEnd = expandToEnclosingParaLine(rawText, absIdx, valEnd)
		}

		refs = append(refs, stringRef{
			startOffset: swallowStart,
			endOffset:   swallowEnd,
			blockIdx:    it.blockIdx,
			stringIdx:   stringInItemIdx,
		})

		searchFrom = valEnd
		stringInItemIdx++
	}

	return refs
}

// expandToEnclosingParaLine returns a [start, end) byte span that covers
// the entire `<ParaLine … > # end of ParaLine\n` block surrounding the
// String at [stringTagStart, valEnd). `stringTagStart` is the byte offset
// of the opening `<` of the `<String …>` tag; `valEnd` is the offset just
// past the value (before the closing backquote). The expansion includes
// any leading whitespace/newline before `<ParaLine` and the trailing
// newline after the `> # end of ParaLine` closer so the writer's
// stringIdx>0 elision drops the wrapper cleanly without leaving stray
// blank lines.
func expandToEnclosingParaLine(rawText string, stringTagStart, valEnd int) (int, int) {
	// Walk backwards to find the most recent `<ParaLine` opener.
	openIdx := strings.LastIndex(rawText[:stringTagStart], "<ParaLine")
	if openIdx < 0 {
		return stringTagStart, valEnd
	}
	// Include any whitespace + newline preceding `<ParaLine` so the line
	// disappears entirely (otherwise we leak indentation + `\n`).
	start := openIdx
	for start > 0 {
		c := rawText[start-1]
		if c == ' ' || c == '\t' {
			start--
			continue
		}
		if c == '\n' {
			start--
			break
		}
		break
	}

	// Walk forwards from valEnd to find the matching `> # end of ParaLine`
	// closer (or a bare `>` close at the same nesting). For wrapper-only
	// secondary ParaLines this is the next `> # end of ParaLine` line.
	tail := rawText[valEnd:]
	closeIdx := strings.Index(tail, "> # end of ParaLine")
	if closeIdx < 0 {
		// Defensive: no comment marker — fall back to plain `>` line.
		closeIdx = strings.Index(tail, "\n>")
		if closeIdx < 0 {
			return start, valEnd
		}
		closeIdx++ // skip the leading `\n`
	}
	end := valEnd + closeIdx
	// Advance past the rest of the closer line up to (but NOT including)
	// the trailing newline. The newline stays in the skeleton so it serves
	// as the line break between the surviving first ParaLine's closer and
	// whatever follows (e.g. `> # end of Para`).
	for end < len(rawText) && rawText[end] != '\n' {
		end++
	}
	return start, end
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
// pseudo-translated. The rule is in okapi's default codeFinder rule
// list but is intentionally omitted from the native default rule list
// (config.go) because applying it to ordinary <String> text would
// split and lose the leading capital — empirically the bridge does NOT
// apply the leading-letter rule to text-flow strings (Test01.mif's
// `<String P:Body>` reference output is `<String 'Ƥ:ßōďŷ'>`, not
// `<String 'P:ßōďŷ'>`). It DOES apply it to <PgfNumFormat> values
// inside <PgfCatalog> (Test02-v9.mif's `<PgfNumFormat 'T:Table <n+\>:'>`
// reference is `'T:Ţàƀĺē <n+\>:'`, with `T` preserved). Apply it
// contextually here.
var pgfNumFormatLeadingPrefix = regexp.MustCompile(`^[A-Z]:`)

// applyCodeFinder splits each TextRun in the block into text +
// placeholder runs whenever a CodeFinder pattern matches. This keeps
// FrameMaker building blocks (`<$lastpagenum\>`, `<n+\>`, `<$tblsheetnum\>`,
// …) from being pseudo-translated character by character — the
// pseudo-translate step only transforms text runs.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.GetCodeFinderPatterns()
	if len(patterns) == 0 || block == nil {
		return
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
				if strings.TrimSpace(run.text) == "" {
					continue
				}
				blockCounter++
				block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), run.text)
				block.Name = fmt.Sprintf("para.%d.%d", blockCounter, runIdx)
				if pgfTag != "" {
					block.Properties["pgf_tag"] = pgfTag
				}
				r.applyCodeFinder(block)
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
type paraTextRun struct {
	text                string
	stringOffsetIndices []int
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

	flush := func() {
		if cur.text == "" {
			cur = paraTextRun{}
			return
		}
		runs = append(runs, cur)
		cur = paraTextRun{}
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
				switch lc.value {
				case "HardReturn":
					if hardReturnsAsText {
						cur.text += "\n"
					}
				case "Tab":
					cur.text += "\t"
				case "HardSpace":
					cur.text += "\u00A0"
				case "SoftHyphen":
					cur.text += "\u00AD"
				case "EnSpace":
					cur.text += "\u2002"
				case "EmSpace":
					cur.text += "\u2003"
				case "ThinSpace":
					cur.text += "\u2009"
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
	var rawBuilder strings.Builder

	popStack := func(line string) {
		if len(stack) == 0 {
			return
		}
		current := stack[len(stack)-1]
		current.raw += line + "\n"
		stack = stack[:len(stack)-1]
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, current)
			parent.raw += current.raw
		} else {
			stmts = append(stmts, current)
		}
	}

	pushSingleLine := func(line, tagSrc string) {
		// tagSrc is the in-tag content WITHOUT surrounding `<` `>`,
		// i.e. just `Tag value` or `Tag` — used to derive tag/value.
		tag, after, hasVal := strings.Cut(tagSrc, " ")
		stmt := &mifStatement{
			tag: tag,
			raw: line + "\n",
		}
		if hasVal {
			stmt.value = unquoteMIF(after)
		}
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, stmt)
			parent.raw += line + "\n"
		} else {
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
				raw:   line + "\n",
			}
			stack = append(stack, stmt)
			continue
		}

		// Non-statement line (comment or other content).
		if len(stack) > 0 {
			stack[len(stack)-1].raw += line + "\n"
		} else {
			rawBuilder.WriteString(line + "\n")
		}
	}

	// Flush any unclosed statements (defensive — a well-formed MIF will
	// have already popped them above).
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, current)
			parent.raw += current.raw
		} else {
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

// unescapeMIFString decodes the in-string MIF escape sequences. The
// inverse of escapeMIF in writer.go. Anything outside the recognised
// set passes through verbatim — robust against partial sequences in
// hand-written fixtures.
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
		default:
			// Unknown escape — keep both bytes so encoder/decoder
			// round-trip remains lossless.
			b.WriteByte(c)
		}
	}
	return b.String()
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
