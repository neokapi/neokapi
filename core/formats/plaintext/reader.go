package plaintext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	coreenc "github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Why plain text names blocks positionally, and why that is the right answer
// here rather than a gap left open.
//
// core/model/structural.go asks every reader to name a block by its structural
// address — the heading trail, the key path, the natural key the format already
// provides. A plain text file has none of those. It is, by definition, a
// sequence of lines with no headings, no keys, no nesting and no author-visible
// labels. There is nothing in the file for an address to be derived FROM.
//
// The alternatives were considered and are worse:
//
//   - Derive the name from the block's own text. This was tried in this repo
//     and reverted. It fixes deletion and breaks editing — fix a typo and the
//     block becomes a different block and loses its history — and worse,
//     model.ComputeIdentity folds Name into ContextHash, so a content-derived
//     name collapses the two hashes into one and destroys the independent
//     signal that identity matching depends on. Do not repeat it.
//   - Invent a synthetic structure (blank-line "sections", first-line
//     "titles"). That is a guess dressed as structure: it moves for reasons the
//     author never expressed, and is less predictable than the line number it
//     replaces, not more.
//
// So the line number stays, and is read as what it honestly is: a CONTEXT
// signal, not an identity. core/reconcile is built for exactly this — it grades
// content BEFORE context, so a line that shifted because something above it was
// inserted or deleted still matches on its words and keeps its identity and its
// history. Positional naming is acceptable here because the layer that resolves
// identity compensates for it; it is not acceptable in a format that could have
// done better.
//
// Every block also carries the line it starts on as an ADVISORY property, so a
// tool can point at it in the file. Advisory means carried but never hashed into
// identity — otherwise the locator would be the very thing that makes a block
// look like it moved, since it shifts whenever anything above it does.

// propLine is the 1-based line a block starts on — the locator that lets a tool
// point at the block in the file.
//
// Advisory (model.AdvisoryPropertyPrefix), so it is carried but never hashed
// into the block's identity. A locator moves whenever anything above it moves;
// letting it identify a block would make every block below an inserted blank
// line report as moved when nothing about it changed.
const propLine = model.AdvisoryPropertyPrefix + "line"

// Reader implements DataFormatReader for plain text files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new plain text reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		FormatName:        "plaintext",
		FormatDisplayName: "Plain Text",
		FormatMimeType:    "text/plain",
		FormatExtensions:  []string{".txt", ".text"},
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
		MIMETypes:  []string{"text/plain"},
		Extensions: []string{".txt", ".text"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("plaintext: nil document or reader")
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

	// Emit layer start
	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "plaintext",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/plain",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Buffer + transcode upfront so UTF-16-with-BOM fixtures (e.g.
	// BOM_MacUTF16withBOM2.txt) get split on '\n' as UTF-8 instead of
	// snagging the high byte of each UTF-16 codepoint.
	raw, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("plaintext: reading: %w", err)}
		return
	}
	utf8Bytes, _, err := coreenc.ToUTF8(raw)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("plaintext: transcoding to UTF-8: %w", err)}
		return
	}

	// ToUTF8 strips the mark along with the encoding it announces; the skeleton
	// carries it back so write-back stays byte-exact.
	if bom, _ := format.SplitBOM(raw); len(bom) > 0 {
		r.skelText(string(bom))
	}

	switch {
	case r.cfg.SpliceLines:
		r.readSpliced(ctx, ch, string(utf8Bytes))
	case r.cfg.ParagraphMode():
		r.readByParagraph(ctx, ch, string(utf8Bytes))
	default:
		r.readByLine(ctx, ch, string(utf8Bytes))
	}

	r.skelFlush()

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readByLine(ctx context.Context, ch chan<- model.PartResult, text string) {
	blockID := 0
	lineNum := 0
	remaining := text

	for len(remaining) > 0 {
		lineNum++

		content, lineEnding, rest := nextPlainLine(remaining)
		remaining = rest

		if content == "" {
			r.skelText(lineEnding)
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", lineNum),
				Name: fmt.Sprintf("line%d", lineNum),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		blockID++
		blockIDStr := fmt.Sprintf("tu%d", blockID)
		r.skelRef(blockIDStr)
		r.skelText(lineEnding)
		block := model.NewBlock(blockIDStr, content)
		block.Name = fmt.Sprintf("line%d", lineNum)
		block.Properties[propLine] = strconv.Itoa(lineNum)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// nextPlainLine peels one line off the front of text, returning the
// content, the line-ending bytes (\n, \r\n, \r, or "" at EOF), and the
// remaining unread text. Recognises bare \r as a line terminator so
// Mac-classic / UTF-16-derived fixtures don't get mashed into one
// gigantic block.
func nextPlainLine(text string) (content, ending, rest string) {
	for i := range len(text) {
		switch text[i] {
		case '\n':
			return text[:i], "\n", text[i+1:]
		case '\r':
			if i+1 < len(text) && text[i+1] == '\n' {
				return text[:i], "\r\n", text[i+2:]
			}
			return text[:i], "\r", text[i+1:]
		}
	}
	return text, "", ""
}

// readSpliced implements splice mode (the behavior of the retired
// splicedlines format): lines ending with the configured marker are
// continuation lines, joined with the following line(s) into a single
// Block whose logical text separates the pieces with '\n'. The markers
// themselves never enter block text; the writer re-adds them (with the
// original line endings) from the block properties recorded here, so the
// skeleton round-trip is byte-exact.
func (r *Reader) readSpliced(ctx context.Context, ch chan<- model.PartResult, text string) {
	marker := r.cfg.Marker()

	type splicedLine struct {
		content    string // line content, continuation marker stripped
		lineEnding string
	}

	var accumulated []splicedLine
	blockID := 0
	dataID := 0
	// hadTrailingSplicer is set when the EOF flush deals with an
	// accumulator whose final raw line ended in the marker (and thus had
	// it stripped). The writer reads the resulting block property to add
	// the marker back on emit, matching Okapi's okf_splicedlines
	// round-trip behavior (SplicedLinesFilterTest#testTrailingBackslash).
	hadTrailingSplicer := false

	flushBlock := func() bool {
		if len(accumulated) == 0 {
			return true
		}
		pieces := make([]string, len(accumulated))
		for i, sl := range accumulated {
			pieces[i] = sl.content
		}
		joined := strings.Join(pieces, "\n")

		if strings.TrimSpace(joined) == "" {
			// Whitespace-only group: reconstruct the raw bytes (including
			// any stripped markers) as skeleton and emit a Data part.
			for i, sl := range accumulated {
				if i < len(accumulated)-1 || hadTrailingSplicer {
					r.skelText(sl.content + marker + sl.lineEnding)
				} else {
					r.skelText(sl.content + sl.lineEnding)
				}
			}
			accumulated = nil
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("empty.%d", dataID),
			}
			return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		}

		blockID++
		blockIDStr := fmt.Sprintf("tu%d", blockID)
		numLines := len(accumulated)

		// Skeleton: the block ref carries the joined content (markers and
		// continuation endings restored by the writer); the last line's
		// ending stays in the skeleton.
		r.skelRef(blockIDStr)
		r.skelText(accumulated[numLines-1].lineEnding)

		block := model.NewBlock(blockIDStr, joined)
		block.Name = fmt.Sprintf("block%d", blockID)
		block.Properties["continued"] = strconv.Itoa(numLines)
		block.Properties["splice-marker"] = marker
		if hadTrailingSplicer {
			block.Properties["trailing-splicer"] = "true"
		}
		// Store the continuation line endings so the writer can reconstruct.
		if numLines > 1 {
			endings := make([]string, 0, numLines-1)
			for i := range numLines - 1 {
				endings = append(endings, accumulated[i].lineEnding)
			}
			block.Properties["continuation-endings"] = strings.Join(endings, "|")
		}

		accumulated = nil
		return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	remaining := text
	for len(remaining) > 0 {
		content, lineEnding, rest := nextPlainLine(remaining)
		remaining = rest

		if stripped, ok := strings.CutSuffix(content, marker); ok {
			// Continuation line: strip the marker and accumulate.
			accumulated = append(accumulated, splicedLine{content: stripped, lineEnding: lineEnding})
			continue
		}
		// Non-continuation: add to accumulator and flush.
		accumulated = append(accumulated, splicedLine{content: content, lineEnding: lineEnding})
		if !flushBlock() {
			return
		}
	}

	// The accumulator can only be non-empty here if the input ended
	// mid-continuation (every non-continuation flushes immediately).
	if len(accumulated) > 0 {
		hadTrailingSplicer = true
		if !flushBlock() {
			return
		}
	}
}

func (r *Reader) readByParagraph(ctx context.Context, ch chan<- model.PartResult, text string) {
	if r.skeletonStore != nil {
		r.readByParagraphSkeleton(ctx, ch, text)
		return
	}

	paragraphs := strings.Split(text, "\n\n")
	blockID := 0

	for i, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockID), para)
		block.Name = fmt.Sprintf("para%d", blockID)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}

		// Emit separator data between paragraphs (not after last)
		if i < len(paragraphs)-1 {
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", i+1),
				Name: "paragraph-separator",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
	}
}

// readByParagraphSkeleton handles paragraph mode with skeleton store active.
// It scans line by line to preserve exact line endings, grouping non-empty
// lines into paragraphs separated by blank lines.
func (r *Reader) readByParagraphSkeleton(ctx context.Context, ch chan<- model.PartResult, text string) {
	// Split into raw lines preserving line endings.
	type rawLine struct {
		content    string
		lineEnding string
	}
	var lines []rawLine
	remaining := text
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

	// Group lines into paragraphs. A paragraph is a sequence of non-empty lines.
	// Between paragraphs we track the exact separator bytes (line endings of
	// the last content line + empty line endings).
	type paragraph struct {
		text            string // joined content (internal newlines use \n)
		lastLineEnding  string // line ending after the last line of the paragraph
		internalEndings string // "|"-joined endings of all lines but the last, when any is not "\n"
	}

	var paragraphs []paragraph
	// separatorsBetween[i] = separator text between paragraph i and i+1
	var separatorsBetween []string
	var leadingSep strings.Builder
	var curLines []rawLine

	flushParagraph := func() {
		if len(curLines) == 0 {
			return
		}
		var sb strings.Builder
		for i, l := range curLines {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(l.content)
		}
		lastEnding := curLines[len(curLines)-1].lineEnding
		// Record the internal line endings when any differ from plain
		// "\n" (e.g. CRLF), so the writer can restore them byte-exact.
		internal := ""
		if len(curLines) > 1 {
			endings := make([]string, 0, len(curLines)-1)
			nonLF := false
			for _, l := range curLines[:len(curLines)-1] {
				endings = append(endings, l.lineEnding)
				if l.lineEnding != "\n" {
					nonLF = true
				}
			}
			if nonLF {
				internal = strings.Join(endings, "|")
			}
		}
		paragraphs = append(paragraphs, paragraph{text: sb.String(), lastLineEnding: lastEnding, internalEndings: internal})
		curLines = nil
	}

	var sepBuf strings.Builder
	seenContent := false

	for _, l := range lines {
		if l.content == "" {
			if len(curLines) > 0 {
				// End of a paragraph: flush it, start accumulating separator
				flushParagraph()
				// The last line ending of the paragraph was captured in flushParagraph.
				// The empty line's ending is part of the separator.
				sepBuf.WriteString(l.lineEnding)
			} else if !seenContent {
				leadingSep.WriteString(l.lineEnding)
			} else {
				sepBuf.WriteString(l.lineEnding)
			}
		} else {
			seenContent = true
			if sepBuf.Len() > 0 {
				separatorsBetween = append(separatorsBetween, sepBuf.String())
				sepBuf.Reset()
			}
			curLines = append(curLines, l)
		}
	}
	flushParagraph()
	trailingSep := sepBuf.String()

	// Emit parts with skeleton entries
	blockID := 0
	dataID := 0

	// Leading empty lines
	if leadingSep.Len() > 0 {
		r.skelText(leadingSep.String())
	}

	for i, para := range paragraphs {
		blockID++
		blockIDStr := fmt.Sprintf("tu%d", blockID)

		r.skelRef(blockIDStr)
		// Write the line ending after this paragraph's last line
		r.skelText(para.lastLineEnding)

		block := model.NewBlock(blockIDStr, para.text)
		block.Name = fmt.Sprintf("para%d", blockID)
		if para.internalEndings != "" {
			block.Properties["line-endings"] = para.internalEndings
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}

		// Emit separator between paragraphs
		if i < len(separatorsBetween) {
			r.skelText(separatorsBetween[i])

			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "paragraph-separator",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
	}

	if trailingSep != "" {
		r.skelText(trailingSep)
	}
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

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
