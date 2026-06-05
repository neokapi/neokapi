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
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for ICU MessageFormat files.
// Input is treated as one MessageFormat pattern per line, or the whole file
// as a single pattern.
type Reader struct {
	format.BaseFormatReader
	cfg *Config

	// parsedLines stores the parsed node trees per line, used by the writer
	// to reconstruct the original pattern structure.
	parsedLines   []parsedLine
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
	lineEndings   []string     // preserved line endings per parsedLine
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

type parsedLine struct {
	lineNum int
	raw     string
	nodes   []node
}

// NewReader creates a new MessageFormat reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "messageformat",
			FormatDisplayName: "ICU MessageFormat",
			FormatMimeType:    "text/x-messageformat",
			FormatExtensions:  []string{".mf", ".messageformat"},
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
		MIMETypes:  []string{"text/x-messageformat"},
		Extensions: []string{".mf", ".messageformat"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("messageformat: nil document or reader")
	}

	r.parsedLines = nil
	r.lineEndings = nil

	if r.skeletonStore != nil {
		return r.openWithSkeleton(ctx, doc)
	}

	// Pre-parse all lines to detect errors early (like CHOICE format).
	scanner := bufio.NewScanner(doc.Reader)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		if strings.TrimSpace(raw) == "" {
			r.parsedLines = append(r.parsedLines, parsedLine{lineNum: lineNum, raw: raw})
			continue
		}
		nodes, err := parse(raw)
		if err != nil {
			return fmt.Errorf("messageformat: line %d: %w", lineNum, err)
		}
		r.parsedLines = append(r.parsedLines, parsedLine{lineNum: lineNum, raw: raw, nodes: nodes})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("messageformat: read error: %w", err)
	}

	r.Doc = doc
	return nil
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

func (r *Reader) openWithSkeleton(_ context.Context, doc *model.RawDocument) error {
	data, err := io.ReadAll(doc.Reader)
	if err != nil {
		return fmt.Errorf("messageformat: read error: %w", err)
	}

	rLines := splitRawLines(data)
	lineNum := 0
	for _, rl := range rLines {
		lineNum++
		raw := rl.content
		r.lineEndings = append(r.lineEndings, rl.lineEnding)
		if strings.TrimSpace(raw) == "" {
			r.parsedLines = append(r.parsedLines, parsedLine{lineNum: lineNum, raw: raw})
			continue
		}
		nodes, err := parse(raw)
		if err != nil {
			return fmt.Errorf("messageformat: line %d: %w", lineNum, err)
		}
		r.parsedLines = append(r.parsedLines, parsedLine{lineNum: lineNum, raw: raw, nodes: nodes})
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

	blockCounter := 0

	for i, pl := range r.parsedLines {
		lineEnding := ""
		if i < len(r.lineEndings) {
			lineEnding = r.lineEndings[i]
		}

		if pl.nodes == nil {
			// Empty line → skeleton text, skip part emission
			r.skelText(pl.raw + lineEnding)
			continue
		}

		// Extract translatable segments from this pattern
		segments := extractSegments(pl.nodes, "")

		if r.skeletonStore != nil && len(segments) == 1 {
			// Simple case: one block per line, use skeleton ref
			blockCounter++
			blockID := fmt.Sprintf("tu%d", blockCounter)
			r.skelRef(blockID)
			r.skelText(lineEnding)

			block := r.createBlock(blockID, segments[0], pl)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		} else if r.skeletonStore != nil {
			// Complex case: multiple segments per line (plural/select).
			// Store entire line as skeleton text for byte-exact roundtrip.
			r.skelText(pl.raw + lineEnding)

			for _, seg := range segments {
				blockCounter++
				blockID := fmt.Sprintf("tu%d", blockCounter)
				block := r.createBlock(blockID, seg, pl)
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
		} else {
			for _, seg := range segments {
				blockCounter++
				blockID := fmt.Sprintf("tu%d", blockCounter)
				block := r.createBlock(blockID, seg, pl)
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			}
		}
	}

	r.skelFlush()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// createBlock builds a Block from a segment, optionally with inline spans
// for argument placeholders.
func (r *Reader) createBlock(id string, seg segment, pl parsedLine) *model.Block {
	// Find the nodes that correspond to this segment
	branchNodes := findBranchNodes(pl.nodes, seg.path)

	if branchNodes != nil && nodesHavePlaceholders(branchNodes) {
		return r.createBlockWithRuns(id, seg, branchNodes)
	}

	block := model.NewBlock(id, seg.text)
	block.Name = seg.path
	if seg.path == "" {
		block.Name = fmt.Sprintf("line.%d", pl.lineNum)
	}
	block.Properties["line"] = strconv.Itoa(pl.lineNum)
	if seg.path != "" {
		block.Properties["path"] = seg.path
	}
	return block
}

// createBlockWithRuns creates a Block with inline placeholder runs for
// argument references like {name}, {0,date,short}, and #.
func (r *Reader) createBlockWithRuns(id string, seg segment, nodes []node) *model.Block {
	var runs []model.Run
	spanID := 0

	for _, n := range nodes {
		switch n.typ {
		case nodeText:
			runs = append(runs, model.Run{Text: &model.TextRun{Text: n.text}})
		case nodeHash:
			spanID++
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				Type: "icu:number",
				ID:   fmt.Sprintf("h%d", spanID),
				Data: "#",
				Disp: "#",
			}})
		case nodeArg:
			spanID++
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				Type: "icu:argument",
				ID:   fmt.Sprintf("p%d", spanID),
				Data: n.text,
				Disp: n.text,
			}})
		}
	}

	block := &model.Block{
		ID:           id,
		Translatable: true,
		Source:       runs,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
		Name:         seg.path,
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
		if (n.typ == nodePlural || n.typ == nodeSelect || n.typ == nodeSelectOrd) && n.argName == argName {
			// Find the branch
			keyword, subRest, hasMore := strings.Cut(rest, ".")
			for _, br := range n.branches {
				if br.keyword == keyword {
					if !hasMore {
						return br.body
					}
					return findBranchNodes(br.body, subRest)
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
