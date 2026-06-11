// Package transtable implements the Okapi TransTable v1 exchange
// format — a tab-separated bilingual table written by Okapi's
// GenericFilterWriter under the "Translation Table" output preset.
//
// Document shape:
//
//	Line 1 (header): TransTableV1\t<src-locale>\t<trg-locale>
//	Line N (data):   "okpCtx:tu=<id>"\t"<source>"[\t"<target>"]
//	                 or with optional segment suffix:
//	                 "okpCtx:tu=<id>:s=<seg-id>"\t"<source>"[\t"<target>"]
//
// Cells may be optionally surrounded by double quotes. Inside cells,
// the literal sequences `\t` and `\n` are unescaped to tab and
// newline (matching the upstream Okapi TransTableFilter unescape
// pass). Whitespace-only lines are skipped silently.
//
// Rows that share the same `tu=<id>` and carry a `:s=<seg-id>` suffix
// are merged into one segmented text unit. A row without `:s=<seg-id>`
// terminates the current grouping.
package transtable

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Okapi TransTable v1 files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new TransTable v1 reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "transtable",
			FormatDisplayName: "Translation Table",
			FormatMimeType:    "text/x-transtable",
			FormatExtensions:  []string{".txt"},
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
		MIMETypes: []string{"text/x-transtable"},
		// Don't auto-detect — the .txt extension is shared with mosestext
		// and many other text formats; users select transtable explicitly.
		Extensions: []string{},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("transtable: nil document or reader")
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

// rawRow is one parsed non-empty data line.
type rawRow struct {
	tuID    string // tu=<id> portion of the crumb
	segID   string // :s=<seg-id> suffix, "" when absent
	source  string // unescaped source cell, "" when absent
	target  string // unescaped target cell, "" when absent
	hasTrg  bool   // true when a third cell was present (even if empty)
	lineEnd string // "\n", "\r\n", or "\r" (or accumulated from skipped blank lines)
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	br := bufio.NewReader(r.Doc.Reader)

	hdr, headerSkel, lineEnd, headerErr, gotAnyLine := r.readHeader(br)
	if headerErr != nil {
		ch <- model.PartResult{Error: headerErr}
		return
	}

	// Resolve the layer's source locale. Header wins over RawDocument's
	// SourceLocale; falls back to the RawDocument's source locale (then
	// English) when the header is missing/empty.
	sourceLocale := hdr.src
	if sourceLocale.IsEmpty() {
		sourceLocale = r.Doc.SourceLocale
	}
	if sourceLocale.IsEmpty() {
		sourceLocale = model.LocaleEnglish
	}
	targetLocale := hdr.trg
	if targetLocale.IsEmpty() {
		targetLocale = r.Doc.TargetLocale
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "transtable",
		Locale:   sourceLocale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-transtable",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Empty document: no input at all. Emit just LayerStart/End.
	if !gotAnyLine {
		r.skelFlush()
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
		return
	}

	// Record the original header bytes verbatim so the writer can replay
	// them. The writer will overwrite this skeleton position with a
	// freshly-rendered header derived from the writer's locale settings.
	r.skelText(headerSkel + lineEnd)

	// Parse remaining lines into rows, deferring emission until we can
	// group `:s=<seg-id>` segments belonging to the same tu=<id>.
	rows, parseErr := r.parseRows(br)
	if parseErr != nil {
		ch <- model.PartResult{Error: parseErr}
		return
	}

	// Group rows by text unit. Rows without `:s=<seg-id>` are their own
	// text unit; rows with `:s=<seg-id>` and the same `tu=<id>` merge.
	groups := groupRows(rows, r.cfg.AllowSegments)

	for _, g := range groups {
		blk := r.buildBlock(g, sourceLocale, targetLocale)
		// Skeleton: emit one ref per block. The writer reconstructs all
		// of the block's rows (one per source segment) from the block's
		// Source/Targets.
		r.skelRef(blk.ID)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: blk}) {
			return
		}
	}

	r.skelFlush()
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// header is the parsed TransTableV1 header line.
type header struct {
	signature string
	src       model.LocaleID
	trg       model.LocaleID
}

// readHeader reads the first non-empty line and parses it as the
// `TransTableV1\t<src>\t<trg>` header. Whitespace-only lines preceding
// the header are skipped silently. Returns the parsed header, the
// header line's verbatim text (sans line ending), the line ending, an
// error if the header is malformed, and a flag indicating whether any
// line was read at all (so an empty document can short-circuit).
func (r *Reader) readHeader(br *bufio.Reader) (header, string, string, error, bool) {
	for {
		raw, err := br.ReadString('\n')
		if raw == "" && err != nil {
			if err == io.EOF {
				return header{}, "", "", nil, false
			}
			return header{}, "", "", fmt.Errorf("transtable: reading header: %w", err), false
		}
		content, lineEnd := splitLineEnd(raw)
		if strings.TrimSpace(content) == "" {
			// Whitespace-only — skip silently per upstream contract.
			if err == io.EOF {
				return header{}, "", "", nil, false
			}
			continue
		}
		cells := strings.Split(content, "\t")
		canon := make([]string, len(cells))
		for i, c := range cells {
			canon[i] = unescape(stripQuotes(c))
		}
		if len(canon) < 1 || canon[0] != "TransTableV1" {
			return header{}, content, lineEnd, fmt.Errorf("transtable: invalid signature %q (expected TransTableV1)", canon[0]), true
		}
		h := header{signature: canon[0]}
		if len(canon) > 1 {
			h.src = model.LocaleID(canon[1])
		}
		if len(canon) > 2 {
			h.trg = model.LocaleID(canon[2])
		}
		return h, content, lineEnd, nil, true
	}
}

// parseRows reads the remainder of the document, returning one rawRow
// per non-empty data line. Whitespace-only lines are skipped silently;
// they do not generate Data parts and do not break a segment group
// (per the upstream `testSegmentedWithTarget` contract).
func (r *Reader) parseRows(br *bufio.Reader) ([]rawRow, error) {
	var rows []rawRow
	for {
		raw, err := br.ReadString('\n')
		if raw == "" && err != nil {
			if err == io.EOF {
				return rows, nil
			}
			return rows, fmt.Errorf("transtable: reading: %w", err)
		}
		content, lineEnd := splitLineEnd(raw)

		if strings.TrimSpace(content) == "" {
			// Skip silently. We don't emit Data for blank lines (the
			// upstream contract is "absorbed silently"). The skeleton
			// stream still receives the bytes through the active text
			// buffer so byte-exact roundtrip is achievable when the
			// writer chooses to honor it.
			r.skelText(content + lineEnd)
			if err == io.EOF {
				return rows, nil
			}
			continue
		}

		row, perr := parseRow(content, lineEnd)
		if perr != nil {
			return rows, fmt.Errorf("transtable: line %q: %w", content, perr)
		}
		rows = append(rows, row)

		if err == io.EOF {
			return rows, nil
		}
	}
}

// parseRow splits one data line into its constituent cells and
// extracts the tu/seg ids from the crumb.
func parseRow(content, lineEnd string) (rawRow, error) {
	row := rawRow{lineEnd: lineEnd}

	// Split into at most three cells.
	cells := strings.SplitN(content, "\t", 3)
	if len(cells) == 0 {
		return row, errors.New("empty row")
	}

	crumb := unescape(stripQuotes(cells[0]))
	tuID, segID, ok := parseCrumb(crumb)
	if !ok {
		return row, fmt.Errorf("invalid crumb %q (expected okpCtx:tu=<id>[:s=<seg-id>])", crumb)
	}
	row.tuID = tuID
	row.segID = segID

	if len(cells) >= 2 {
		row.source = unescape(stripQuotes(cells[1]))
	}
	if len(cells) >= 3 {
		row.target = unescape(stripQuotes(cells[2]))
		row.hasTrg = true
	}
	return row, nil
}

// parseCrumb extracts `tu=<id>` and the optional `:s=<seg-id>` from an
// `okpCtx:tu=<id>[:s=<seg-id>]` crumb.
func parseCrumb(crumb string) (tuID, segID string, ok bool) {
	const prefix = "okpCtx:tu="
	if !strings.HasPrefix(crumb, prefix) {
		return "", "", false
	}
	rest := crumb[len(prefix):]
	if rest == "" {
		return "", "", false
	}
	if before, after, ok0 := strings.Cut(rest, ":s="); ok0 {
		return before, after, true
	}
	return rest, "", true
}

// rowGroup is a sequence of rows that belong to one text unit.
type rowGroup struct {
	tuID string
	rows []rawRow
}

// groupRows merges consecutive rows that share a tu=<id> and carry a
// `:s=<seg-id>` suffix. A row without `:s=<seg-id>` is always its own
// group, regardless of its tu=<id>. When allowSegments is false every
// row is its own group.
func groupRows(rows []rawRow, allowSegments bool) []rowGroup {
	var groups []rowGroup
	for _, row := range rows {
		if !allowSegments || row.segID == "" {
			groups = append(groups, rowGroup{tuID: row.tuID, rows: []rawRow{row}})
			continue
		}
		// segID present: append to the previous group iff it has the
		// same tu=<id> and was itself a segmented row (segID != "").
		if n := len(groups); n > 0 && groups[n-1].tuID == row.tuID && len(groups[n-1].rows) > 0 && groups[n-1].rows[0].segID != "" {
			groups[n-1].rows = append(groups[n-1].rows, row)
			continue
		}
		groups = append(groups, rowGroup{tuID: row.tuID, rows: []rawRow{row}})
	}
	return groups
}

// buildBlock turns one rowGroup into a translatable Block. Each row in
// the group is one former structural segment. In the Run model the
// block holds a single Run sequence per side (source / target); the
// per-row segmentation rides as a stand-off segmentation overlay whose
// Spans carry the segment ids and run-index boundaries (AD-017).
func (r *Reader) buildBlock(g rowGroup, sourceLocale, targetLocale model.LocaleID) *model.Block {
	blk := &model.Block{
		ID:           g.tuID,
		Name:         "tu" + g.tuID,
		Translatable: true,
		SourceLocale: sourceLocale,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
	}
	blk.Properties["tu_id"] = g.tuID

	var (
		srcRuns []model.Run
		trgRuns []model.Run
		spans   []model.Span
		srcPos  int
		trgPos  int
	)
	hasAnyTarget := false
	for i, row := range g.rows {
		segID := row.segID
		if segID == "" {
			segID = fmt.Sprintf("s%d", i+1)
		}

		// Each row contributes exactly one source TextRun. Record the
		// segment span over the source runs.
		srcRun := model.Run{Text: &model.TextRun{Text: row.source}}
		srcRuns = append(srcRuns, srcRun)
		srcEnd := srcPos + 1

		// Each row also contributes one target TextRun (empty when the
		// row had no third cell) so source/target segment spans stay
		// index-aligned, matching the former per-segment alignment.
		trgRun := model.Run{Text: &model.TextRun{Text: row.target}}
		trgRuns = append(trgRuns, trgRun)
		trgEnd := trgPos + 1
		if row.hasTrg {
			hasAnyTarget = true
		}

		spans = append(spans, model.Span{
			ID:    segID,
			Range: model.RunRange{StartRun: srcPos, EndRun: srcEnd},
		})
		srcPos = srcEnd
		trgPos = trgEnd
	}

	blk.Source = srcRuns
	blk.SetSegmentation(nil, spans)

	if hasAnyTarget && !targetLocale.IsEmpty() {
		blk.SetTargetRuns(targetLocale, trgRuns)
		key := model.Variant(targetLocale)
		blk.SetSegmentation(&key, spans)
	}
	return blk
}

// splitLineEnd separates a raw line read from bufio.Reader into its
// content (no terminator) and the line terminator that followed it.
func splitLineEnd(raw string) (content, lineEnd string) {
	switch {
	case strings.HasSuffix(raw, "\r\n"):
		return raw[:len(raw)-2], "\r\n"
	case strings.HasSuffix(raw, "\n"):
		return raw[:len(raw)-1], "\n"
	case strings.HasSuffix(raw, "\r"):
		return raw[:len(raw)-1], "\r"
	default:
		return raw, ""
	}
}

// stripQuotes removes a single pair of surrounding double quotes from
// s, if present. Inner quotes are left alone — the upstream filter
// does not perform CSV-style quote escaping on the content cells.
func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// unescape replaces the literal sequences `\t` and `\n` with tab and
// newline. Mirrors the upstream Okapi TransTableFilter unescape pass.
func unescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 't':
				b.WriteByte('\t')
				i++
				continue
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// escape is the inverse of unescape — used by the writer to produce
// safe TransTable cells.
func escape(s string) string {
	if !strings.ContainsAny(s, "\t\n\\") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := range len(s) {
		switch s[i] {
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteByte(s[i])
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

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
