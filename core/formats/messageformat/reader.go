package messageformat

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/icu"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// propLine records the 1-based source line a block was read from.
//
// Advisory (model.AdvisoryPropertyPrefix), so it is carried but never hashed
// into the block's identity — and so it never reaches the transfer hash either
// (model.BlockIdentity.RecordHash). A file here is one pattern per line, so a
// line inserted at the top moves every locator below it; letting one identify a
// block would report a file's whole tail as changed on an edit that touched one
// message.
const propLine = model.AdvisoryPropertyPrefix + "line"

// Reader implements DataFormatReader for ICU MessageFormat files.
// Input is treated as one MessageFormat pattern per line, or the whole file
// as a single pattern.
type Reader struct {
	format.BaseFormatReader
	cfg *Config

	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter and StreamingReader.
var (
	_ format.SkeletonStoreEmitter = (*Reader)(nil)
	_ format.StreamingReader      = (*Reader)(nil)
)

// StreamingReader marks this reader as bounded-memory streaming: Open only
// stores the document, and Read parses one MessageFormat message per line via a
// bufio window (never io.ReadAll), holding just the in-progress line. A syntax
// error therefore surfaces on the Read channel (PartResult.Error) rather than
// from Open — the streaming consequence of not pre-parsing the whole file. That
// error is propagated, never swallowed, by the file runner (feedReader) and the
// subfilter driver, which both check PartResult.Error. See [AD-005].
func (r *Reader) StreamingReader() {}

type parsedLine struct {
	lineNum int
	raw     string
	nodes   []node
}

// NewReader creates a new MessageFormat reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		FormatName:        "messageformat",
		FormatDisplayName: "ICU MessageFormat",
		FormatMimeType:    "text/x-messageformat",
		FormatExtensions:  []string{".mf", ".messageformat"},
		Cfg:               cfg,
		cfg:               cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/x-messageformat"},
		Extensions: []string{".mf", ".messageformat"},
	}
}

// Open opens a RawDocument for reading. As a StreamingReader, Open only stores
// the document; parsing happens incrementally in Read (one message per line via
// a bufio window), so a syntax error surfaces on the Read channel rather than
// here.
func (r *Reader) Open(_ context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("messageformat: nil document or reader")
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
		Format:   "messageformat",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-messageformat",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Stream one line at a time via a bufio window so only the in-progress line
	// is held (bounded memory). The read is bounded by the shared safeio byte
	// budget: an unbounded/oversized stream fails with a typed error (identical
	// limit across CLI/server/WASM — see core/safeio). Unlike a bufio.Scanner,
	// ReadString imposes no per-line token cap, so an over-long single line is
	// read (up to the byte budget) rather than rejected.
	bom, body, err := format.SplitBOMReader(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("messageformat: reading: %w", err)}
		return
	}
	r.skelText(bom)
	br := bufio.NewReader(body)
	blockCounter := 0
	contentCounter := 0
	lineNum := 0
	for {
		raw, readErr := br.ReadString('\n')
		if raw != "" {
			content, lineEnding := splitLineEnding(raw)
			lineNum++
			if !r.emitLine(ctx, ch, content, lineEnding, lineNum, &blockCounter, &contentCounter) {
				return
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				r.emitErr(ctx, ch, fmt.Errorf("messageformat: read error: %w", readErr))
				return
			}
			break
		}
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// splitLineEnding separates a raw line read up to and including its terminator
// into content and the preserved line ending ("", "\n", or "\r\n").
func splitLineEnding(raw string) (content, ending string) {
	switch {
	case strings.HasSuffix(raw, "\r\n"):
		return raw[:len(raw)-2], "\r\n"
	case strings.HasSuffix(raw, "\n"):
		return raw[:len(raw)-1], "\n"
	default:
		return raw, ""
	}
}

// emitLine parses one message line and emits its parts (framing prose,
// translatable segments, and skeleton entries). It returns false when the read
// should stop — either the context was cancelled mid-emit or a parse error was
// already surfaced on the channel.
func (r *Reader) emitLine(ctx context.Context, ch chan<- model.PartResult, content, lineEnding string, lineNum int, blockCounter, contentCounter *int) bool {
	// The builder is scoped to this line, not to the document: the line scope
	// already makes every name unique across lines, so only names repeating
	// WITHIN one message need an ordinal. Keeping it here also keeps the
	// reader's memory bounded — a document-wide builder would retain a name per
	// block and this reader streams (see StreamingReader).
	var names model.NameBuilder

	if strings.TrimSpace(content) == "" {
		// Empty line → skeleton text only (a no-op when no skeleton store is
		// wired), no part emission.
		r.skelText(content + lineEnding)
		return true
	}

	nodes, err := parse(content)
	if err != nil {
		r.emitErr(ctx, ch, fmt.Errorf("messageformat: line %d: %w", lineNum, err))
		return false
	}
	pl := parsedLine{lineNum: lineNum, raw: content, nodes: nodes}

	// emitFrames surfaces the literal prose that frames a plural/select branch
	// (the sentence frame around a picker) as non-translatable content blocks —
	// visible to ingestion, skipped by MT. It is a no-op when
	// ExtractNonTranslatableContent is off, keeping the part stream (and thus
	// parity) byte-identical to the prior behavior. The framing text rides only
	// the skeleton's raw line, so these blocks carry no skeleton ref and the
	// write round-trip stays byte-exact.
	emitFrames := func() bool {
		if !r.cfg.ExtractNonTranslatableContent() {
			return true
		}
		for _, lit := range extractLiteralSiblings(pl.nodes) {
			*contentCounter++
			name := names.Name(model.StructuralPath(lineScope(pl.lineNum), "frame"))
			block := newContentBlock(fmt.Sprintf("txt%d", *contentCounter), name, lit, pl.lineNum)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return false
			}
		}
		return true
	}

	// Extract translatable segments from this pattern.
	segments := extractSegments(pl.nodes, "")

	if r.skeletonStore != nil && len(segments) == 1 {
		// Simple case: one block per line, use skeleton ref. A single segment is
		// a branchless leaf, so there is no framing prose to surface and the raw
		// line is not preserved in the skeleton — leave this path untouched.
		*blockCounter++
		blockID := fmt.Sprintf("tu%d", *blockCounter)
		r.skelRef(blockID)
		r.skelText(lineEnding)

		block := r.createBlock(blockID, segments[0], pl, &names)
		return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	if r.skeletonStore != nil {
		// Complex case: multiple segments per line (plural/select). Store the
		// entire line as skeleton text for byte-exact roundtrip.
		r.skelText(pl.raw + lineEnding)
		if !emitFrames() {
			return false
		}
		for _, seg := range segments {
			*blockCounter++
			blockID := fmt.Sprintf("tu%d", *blockCounter)
			block := r.createBlock(blockID, seg, pl, &names)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return false
			}
		}
		return true
	}

	// No skeleton store.
	if !emitFrames() {
		return false
	}
	for _, seg := range segments {
		*blockCounter++
		blockID := fmt.Sprintf("tu%d", *blockCounter)
		block := r.createBlock(blockID, seg, pl, &names)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
	}
	return true
}

// newContentBlock builds a non-translatable content block carrying literal
// prose that frames a plural/select branch. The text rides as a single verbatim
// run (no inline placeholder parse) with PreserveWhitespace set so the
// surrounding spacing stays faithful; being plain prose, it carries no semantic
// role. Translatable=false keeps it out of the MT payload while remaining
// visible to ingestion.
func newContentBlock(id, name, text string, lineNum int) *model.Block {
	block := &model.Block{
		ID:                 id,
		Translatable:       false,
		PreserveWhitespace: true,
		Source:             []model.Run{{Text: &model.TextRun{Text: text}}},
		Targets:            make(map[model.VariantKey]*model.Target),
		Properties:         make(map[string]string),
		Name:               name,
	}
	block.Properties[propLine] = strconv.Itoa(lineNum)
	return block
}

// lineScope names the message a block belongs to.
//
// A MessageFormat file has no keys: it is one pattern per line, and the line is
// the only structure the author controls. So unlike PO or properties, this
// format has no natural key to name blocks by, and the line ordinal stays — the
// same positional gap the prose formats have (see
// core/formats/identity_conformance_test.go). What the scope does fix is
// collision: a branch path like `count.one` repeats on every line that picks on
// `count`, and naming two blocks alike hands them one identity downstream.
func lineScope(lineNum int) string {
	return fmt.Sprintf("line.%d", lineNum)
}

// createBlock builds a Block from a segment, optionally with inline spans
// for argument placeholders.
func (r *Reader) createBlock(id string, seg segment, pl parsedLine, names *model.NameBuilder) *model.Block {
	// Find the nodes that correspond to this segment
	branchNodes := findBranchNodes(pl.nodes, seg.path)

	name := names.Name(model.StructuralPath(lineScope(pl.lineNum), seg.path))

	if branchNodes != nil && nodesHavePlaceholders(branchNodes) {
		return r.createBlockWithRuns(id, name, seg, branchNodes)
	}

	block := model.NewBlock(id, seg.text)
	block.Name = name
	block.Properties[propLine] = strconv.Itoa(pl.lineNum)
	if seg.path != "" {
		block.Properties["path"] = seg.path
	}
	return block
}

// createBlockWithRuns creates a Block with inline placeholder runs for
// argument references like {name}, {0,date,short}, and #.
func (r *Reader) createBlockWithRuns(id, name string, seg segment, nodes []node) *model.Block {
	var runs []model.Run
	spanID := 0

	for _, n := range nodes {
		switch n.Type {
		case icu.NodeText:
			runs = append(runs, model.Run{Text: &model.TextRun{Text: n.Text}})
		case icu.NodeHash:
			spanID++
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				Type: "icu:number",
				ID:   fmt.Sprintf("h%d", spanID),
				Data: "#",
				Disp: "#",
			}})
		case icu.NodeArg:
			spanID++
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				Type: "icu:argument",
				ID:   fmt.Sprintf("p%d", spanID),
				Data: n.Text,
				Disp: n.Text,
			}})
		}
	}

	block := &model.Block{
		ID:           id,
		Translatable: true,
		Source:       runs,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
		Name:         name,
	}
	if seg.path != "" {
		block.Properties["path"] = seg.path
	}
	return block
}

// findBranchNodes navigates the parsed tree to find the nodes for a given
// dot-delimited path (e.g., "count.one" or "gender.male.count.other").
func findBranchNodes(nodes []node, path string) []node {
	if path == "" {
		return nodes
	}

	argName, rest, ok := strings.Cut(path, ".")
	if !ok {
		return nodes
	}

	for _, n := range nodes {
		if n.Type.Picker() && n.ArgName == argName {
			// Find the branch
			keyword, subRest, hasMore := strings.Cut(rest, ".")
			for _, br := range n.Branches {
				if br.Keyword == keyword {
					if !hasMore {
						return br.Body
					}
					return findBranchNodes(br.Body, subRest)
				}
			}
		}
	}
	return nodes
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
			r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		r.skeletonStore.WriteRef(id)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		r.skeletonStore.WriteText(r.skelBuf.Bytes())
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

// emitErr surfaces a parse or read error on the Part channel. Streaming readers
// report errors here (not from Open); the file runner and the subfilter driver
// both check PartResult.Error, so the error is never swallowed.
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
