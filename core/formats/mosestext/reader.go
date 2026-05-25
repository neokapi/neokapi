package mosestext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Moses Text files.
// Each non-empty line becomes a translatable Block (text unit).
// Empty lines become Data parts.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new Moses Text reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "mosestext",
			FormatDisplayName: "Moses Text",
			FormatMimeType:    "text/x-mosestext",
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
		MIMETypes:  []string{"text/x-mosestext"},
		Extensions: []string{}, // Don't auto-detect .txt as mosestext
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("mosestext: nil document or reader")
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
		Format:   "mosestext",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-mosestext",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	if r.skeletonStore != nil {
		r.readLinesSkeleton(ctx, ch)
	} else {
		r.readLinesNormal(ctx, ch)
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// rawLine is one physical line scanned from the document, carrying its
// content (without the terminator) and the exact terminator that
// followed it ("", "\n", "\r\n", or "\r"). The terminator is preserved
// so the skeleton path can reconstruct the input byte-for-byte.
type rawLine struct {
	content string
	ending  string
}

// entry is one Moses InlineText text unit assembled from one or more
// physical lines, mirroring the grouping in MosesTextFilter.next():
//   - a plain (non-mrk) line is its own entry; or
//   - an `<mrk mtype="seg">…</mrk>` annotation forms one entry whose body
//     is the text between the markers (possibly spanning several lines,
//     joined with "\n" as upstream does).
//
// markerStart / markerEnd hold the literal `<mrk …>` / `</mrk>` tags
// (empty for a plain line) so the skeleton path can replay them verbatim.
type entry struct {
	body        string
	markerStart string
	markerEnd   string
	// skel is the trailing skeleton text after the entry's content
	// placeholder — line terminators (and, for mrk entries, the closing
	// tag is emitted via markerEnd, with the terminator appended here).
	skel string
}

// readLinesNormal reads all lines without skeleton tracking.
func (r *Reader) readLinesNormal(ctx context.Context, ch chan<- model.PartResult) {
	lines := r.scanRawLines()
	entries := groupEntries(lines)

	blockCounter := 0
	dataCounter := 0

	for _, e := range entries {
		if e.body == "" && e.markerStart == "" && e.markerEnd == "" {
			dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: fmt.Sprintf("empty-line%d", dataCounter),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		blockCounter++
		block := r.newBlock(blockCounter, e.body)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// readLinesSkeleton reads lines while recording skeleton entries for byte-exact roundtrip.
func (r *Reader) readLinesSkeleton(ctx context.Context, ch chan<- model.PartResult) {
	lines := r.scanRawLines()
	entries := groupEntries(lines)

	blockCounter := 0
	dataCounter := 0

	for _, e := range entries {
		if e.body == "" && e.markerStart == "" && e.markerEnd == "" {
			// Empty line is non-translatable data.
			r.skelText(e.skel)
			dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: fmt.Sprintf("empty-line%d", dataCounter),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		blockCounter++
		blockIDStr := fmt.Sprintf("tu%d", blockCounter)
		r.skelText(e.markerStart)
		r.skelRef(blockIDStr)
		r.skelText(e.markerEnd + e.skel)
		block := r.newBlock(blockCounter, e.body)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// newBlock builds a translatable Block from a decoded Moses InlineText
// entry body. The body is decoded through decodeInlineText (entities,
// `<g>`/`<x>`/`<bx>`/`<ex>` codes, `<lb/>` line breaks) into a Run
// sequence; a configured code finder, if any, further carves the
// resulting plain text into placeholder runs.
func (r *Reader) newBlock(counter int, body string) *model.Block {
	// The code finder is a native-only, opt-in feature that carves
	// verbatim placeholder runs out of the raw line — it deliberately
	// does NOT entity-decode or parse Moses InlineText markup (its
	// contract is verbatim preservation, with the writer replaying
	// everything as-is). When it is active, skip InlineText decoding and
	// keep the raw body so the carved Data stays byte-exact.
	if len(r.cfg.GetCodeFinderPatterns()) > 0 {
		block := model.NewBlock(fmt.Sprintf("tu%d", counter), body)
		block.Name = fmt.Sprintf("line%d", counter)
		block.PreserveWhitespace = true
		r.applyCodeFinder(block)
		return block
	}

	// Default mode: decode the Moses InlineText (pseudo-XLIFF) surface —
	// XML entities, <g>/<x>/<bx>/<ex> codes, and <lb/> line breaks —
	// matching Okapi's MosesTextFilter.fromPseudoXLIFF. The encode marker
	// tells the writer to re-encode the body on output for a byte-exact
	// round trip.
	runs := decodeInlineText(body)
	block := model.NewRunsBlock(fmt.Sprintf("tu%d", counter), runs)
	block.Name = fmt.Sprintf("line%d", counter)
	block.PreserveWhitespace = true
	block.Properties[propEncode] = encodeInlineTextValue
	return block
}

// groupEntries folds a sequence of physical lines into Moses InlineText
// entries, mirroring the entry-grouping loop in MosesTextFilter.next().
// A blank line becomes an empty entry (a Data part downstream); a plain
// line becomes a one-line entry; an `<mrk mtype="seg">…</mrk>`
// annotation becomes one entry whose body spans every line up to the
// closing `</mrk>`.
func groupEntries(lines []rawLine) []entry {
	var entries []entry
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		if ln.content == "" {
			entries = append(entries, entry{skel: ln.ending})
			continue
		}

		// Detect the start of an `<mrk mtype="seg">` segment.
		if loc := startSegment.FindStringIndex(ln.content); loc != nil && loc[0] == 0 {
			marker := ln.content[:loc[1]]
			rest := ln.content[loc[1]:]
			var sb strings.Builder
			ending := ln.ending
			// Same line closes the segment?
			if strings.HasSuffix(rest, endSegment) {
				sb.WriteString(strings.TrimSuffix(rest, endSegment))
			} else {
				sb.WriteString(rest)
				sb.WriteString("\n")
				// Continuation lines until one ends with </mrk>.
				for i+1 < len(lines) {
					i++
					cont := lines[i]
					ending = cont.ending
					if strings.HasSuffix(cont.content, endSegment) {
						sb.WriteString(strings.TrimSuffix(cont.content, endSegment))
						break
					}
					sb.WriteString(cont.content)
					sb.WriteString("\n")
				}
			}
			entries = append(entries, entry{
				body:        sb.String(),
				markerStart: marker,
				markerEnd:   endSegment,
				skel:        ending,
			})
			continue
		}

		entries = append(entries, entry{body: ln.content, skel: ln.ending})
	}
	return entries
}

// applyCodeFinder rewrites a block's source runs so that any region
// matching the configured code-finder regexes becomes a placeholder run
// (Ph) instead of a translatable text run. The Data captured on each Ph
// is the original matched text — the writer replays it verbatim via
// model.RenderRunsWithData.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.GetCodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}
	if len(block.Source) == 0 {
		return
	}
	// Skip sources that already carry inline codes from the Moses
	// InlineText decode — re-running the code finder over the
	// flattened text would discard those PcOpen/PcClose/Ph runs.
	for _, run := range block.Source {
		if run.Text == nil {
			return
		}
	}
	text := model.RunsText(block.Source)
	type matchRange struct{ start, end int }
	var matches []matchRange
	for _, re := range patterns {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			matches = append(matches, matchRange{loc[0], loc[1]})
		}
	}
	if len(matches) == 0 {
		return
	}
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}
	var runs []model.Run
	lastEnd := 0
	spanID := 1
	for _, m := range matches {
		if m.start < lastEnd {
			continue // skip overlapping match
		}
		if m.start > lastEnd {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:m.start]}})
		}
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
			ID:   fmt.Sprintf("c%d", spanID),
			Type: "code",
			Data: text[m.start:m.end],
		}})
		lastEnd = m.end
		spanID++
	}
	if lastEnd < len(text) {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: text[lastEnd:]}})
	}
	block.SetSourceRuns(runs)
}

// scanRawLines reads the whole document and splits it into physical
// lines, preserving the exact terminator (CR, LF, or CRLF) that ended
// each line. This mirrors the line splitting of Java's BufferedReader
// (which MosesTextFilter relies on) — any of CR, LF, or CRLF terminates
// a line — while retaining the original terminator bytes so the
// skeleton path can reconstruct the input verbatim. A trailing
// terminator does not introduce a phantom empty line.
func (r *Reader) scanRawLines() []rawLine {
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil || len(data) == 0 {
		return nil
	}
	s := string(data)

	var lines []rawLine
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\n' && c != '\r' {
			continue
		}
		ending := string(c)
		if c == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			ending = "\r\n"
		}
		lines = append(lines, rawLine{content: s[start:i], ending: ending})
		if ending == "\r\n" {
			i++ // skip the LF of a CRLF pair
		}
		start = i + 1
	}
	// Trailing content with no terminator forms a final line. A trailing
	// terminator (start == len(s)) does not create a phantom empty line.
	if start < len(s) {
		lines = append(lines, rawLine{content: s[start:], ending: ""})
	}
	return lines
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
