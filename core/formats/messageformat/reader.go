package messageformat

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for ICU MessageFormat files.
// Input is treated as one MessageFormat pattern per line, or the whole file
// as a single pattern.
type Reader struct {
	format.BaseFormatReader
	cfg *Config

	// parsedLines stores the parsed node trees per line, used by the writer
	// to reconstruct the original pattern structure.
	parsedLines []parsedLine
}

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
		return fmt.Errorf("messageformat: nil document or reader")
	}

	// Pre-parse all lines to detect errors early (like CHOICE format).
	scanner := bufio.NewScanner(doc.Reader)
	lineNum := 0
	r.parsedLines = nil
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		if strings.TrimSpace(raw) == "" {
			r.parsedLines = append(r.parsedLines, parsedLine{lineNum: lineNum, raw: raw})
			continue
		}
		nodes, err := parse(raw)
		if err != nil {
			return fmt.Errorf("messageformat: line %d: %s", lineNum, err.Error())
		}
		r.parsedLines = append(r.parsedLines, parsedLine{lineNum: lineNum, raw: raw, nodes: nodes})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("messageformat: read error: %w", err)
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

	for _, pl := range r.parsedLines {
		if pl.nodes == nil {
			// Empty line → skip or emit as data
			continue
		}

		// Extract translatable segments from this pattern
		segments := extractSegments(pl.nodes, "")

		for _, seg := range segments {
			blockCounter++
			blockID := fmt.Sprintf("tu%d", blockCounter)

			block := r.createBlock(blockID, seg, pl)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// createBlock builds a Block from a segment, optionally with inline spans
// for argument placeholders.
func (r *Reader) createBlock(id string, seg segment, pl parsedLine) *model.Block {
	// Find the nodes that correspond to this segment
	branchNodes := findBranchNodes(pl.nodes, seg.path)

	if branchNodes != nil && nodesHavePlaceholders(branchNodes) {
		return r.createBlockWithSpans(id, seg, branchNodes)
	}

	block := model.NewBlock(id, seg.text)
	block.Name = seg.path
	if seg.path == "" {
		block.Name = fmt.Sprintf("line.%d", pl.lineNum)
	}
	block.Properties["line"] = fmt.Sprintf("%d", pl.lineNum)
	if seg.path != "" {
		block.Properties["path"] = seg.path
	}
	return block
}

// createBlockWithSpans creates a Block with inline placeholder spans for
// argument references like {name}, {0,date,short}, and #.
func (r *Reader) createBlockWithSpans(id string, seg segment, nodes []node) *model.Block {
	frag := &model.Fragment{}
	spanID := 0

	for _, n := range nodes {
		switch n.typ {
		case nodeText:
			frag.AppendText(n.text)
		case nodeHash:
			spanID++
			frag.AppendSpan(&model.Span{
				SpanType:    model.SpanPlaceholder,
				Type:        "icu:number",
				ID:          fmt.Sprintf("h%d", spanID),
				Data:        "#",
				DisplayText: "#",
			})
		case nodeArg:
			spanID++
			frag.AppendSpan(&model.Span{
				SpanType:    model.SpanPlaceholder,
				Type:        "icu:argument",
				ID:          fmt.Sprintf("p%d", spanID),
				Data:        n.text,
				DisplayText: n.text,
			})
		}
	}

	block := &model.Block{
		ID:           id,
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: frag}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
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

	parts := strings.SplitN(path, ".", 2)
	if len(parts) < 2 {
		return nodes
	}
	argName := parts[0]
	rest := parts[1]

	for _, n := range nodes {
		if (n.typ == nodePlural || n.typ == nodeSelect || n.typ == nodeSelectOrd) && n.argName == argName {
			// Find the branch
			branchParts := strings.SplitN(rest, ".", 2)
			keyword := branchParts[0]
			for _, br := range n.branches {
				if br.keyword == keyword {
					if len(branchParts) == 1 {
						return br.body
					}
					return findBranchNodes(br.body, branchParts[1])
				}
			}
		}
	}
	return nodes
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
