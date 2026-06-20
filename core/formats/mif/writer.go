package mif

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for MIF files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	wroteVersion  bool
	blocks        []*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new MIF writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName:       "mif",
			RequiresSkeleton: true,
		},
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed MIF.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		// Collect all blocks, then write from skeleton
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case part, ok := <-parts:
				if !ok {
					return w.writeFromSkeleton()
				}
				if part.Type == model.PartBlock {
					if block, ok := part.Resource.(*model.Block); ok {
						w.blocks = append(w.blocks, block)
					}
				}
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if err := w.writePart(part); err != nil {
				return err
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("mif writer: flush skeleton: %w", err)
	}

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("mif writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			// Ref ID is "blockIdx:stringIdx:runOrdinal".
			refID := string(entry.Data)
			parts := strings.SplitN(refID, ":", 3)
			if len(parts) < 2 {
				continue
			}
			blockIdx, err := strconv.Atoi(parts[0])
			if err != nil || blockIdx < 0 || blockIdx >= len(w.blocks) {
				continue
			}
			stringIdx, err := strconv.Atoi(parts[1])
			if err != nil {
				continue
			}
			// runOrdinal selects which text-group of a multi-run Para block
			// to render into this `<String>` slot. -1 (or a missing third
			// field, for older single-value items) renders the whole block.
			runOrdinal := -1
			if len(parts) == 3 {
				if ro, err := strconv.Atoi(parts[2]); err == nil {
					runOrdinal = ro
				}
			}

			block := w.blocks[blockIdx]
			runs := block.Source
			if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
				runs = block.TargetRuns(w.Locale)
			}

			// stringIdx>0 entries are the secondary `<String>`s of a single
			// run whose values were merged into the run's first `<String>`
			// (the wrapper bytes between them are elided by the reader). They
			// emit no text — the run's whole text-group lives in slot 0.
			if stringIdx != 0 {
				continue
			}

			// A Para is composed as ONE Block whose runs interleave text and
			// structural inline-code (Ph) placeholders for the `<Font>` /
			// `<AFrame>` / … statements that physically separate the source
			// `<String>` tags. The reader emits one ref per text-group with
			// its runOrdinal so we render only that group's text here, leaving
			// the structural statements in the skeleton between slots —
			// mirroring okapi, which serializes one TextUnit across several
			// `<String>` outputs (MIFFilter.processPara, MIFFilter.java:636-811).
			var text string
			if runOrdinal < 0 {
				text = renderSegments(runs)
			} else {
				text = renderRunGroup(runs, runOrdinal)
			}
			// Re-wrap with the leading/trailing boundary content that
			// simplifyBlockCodes trimmed from the extracted unit (it is
			// non-translatable and okapi keeps it in the output `<String>`).
			// The lead attaches to the first text-group; the trail to the
			// last. For whole-block (-1) renders both attach to the one slot.
			lead, trail := blockTrim(block)
			if lead != "" || trail != "" {
				lastGroup := runGroupCount(runs) - 1
				if runOrdinal < 0 {
					text = lead + text + trail
				} else {
					if runOrdinal == 0 {
						text = lead + text
					}
					if runOrdinal == lastGroup {
						text += trail
					}
				}
			}
			if _, err := io.WriteString(w.Output, escapeMIF(text)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		return w.writeBlock(part)
	case model.PartData:
		return w.writeData(part)
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("mif writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Escape the text for MIF string format.
	escaped := escapeMIF(text)

	pgfTag := block.Properties["pgf_tag"]
	if pgfTag == "" {
		pgfTag = "Body"
	}

	if _, err := fmt.Fprintf(w.Output, " <Para\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "  <PgfTag `%s'>\n", pgfTag); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "  <ParaLine\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "   <String `%s'>\n", escaped); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "  >\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, " >\n"); err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return errors.New("mif writer: expected Data resource")
	}

	if data.Properties["tag"] == "MIFFile" {
		version := data.Properties["version"]
		if version == "" {
			version = "2015"
		}
		if _, err := fmt.Fprintf(w.Output, "<MIFFile %s>\n", version); err != nil {
			return err
		}
		w.wroteVersion = true
		return nil
	}

	raw := data.Properties["raw"]
	if raw != "" {
		if _, err := fmt.Fprint(w.Output, raw); err != nil {
			return err
		}
	}
	return nil
}

// renderSegments renders a run sequence using RenderRunsWithData so
// inline-code Ph runs emit their captured literal. Used by
// writeFromSkeleton to fill String/VariableDef refs without losing the
// FrameMaker building blocks the reader extracted via the CodeFinder.
func renderSegments(runs []model.Run) string {
	return model.RenderRunsWithData(runs)
}

// blockTrim returns the leading and trailing boundary content that
// simplifyBlockCodes removed from the block during extraction (recorded in
// block.Properties[simplifyBlockTrim] as "lead\x00trail"). These are
// non-translatable bytes (boundary whitespace + leading/trailing inline-code
// building blocks) that okapi keeps in the output `<String>`; the writer
// re-wraps the translated core with them so the round-trip stays byte-exact.
func blockTrim(block *model.Block) (lead, trail string) {
	if block == nil || block.Properties == nil {
		return "", ""
	}
	v, ok := block.Properties[simplifyBlockTrim]
	if !ok {
		return "", ""
	}
	l, t, _ := strings.Cut(v, "\x00")
	return l, t
}

// paraGroup is the rendered content of one structural text-group of a Para
// block (the runs between two STRUCTURAL empty-Data inline-code boundaries),
// plus whether that group carries extractable (non-whitespace) text.
type paraGroup struct {
	text        string
	extractable bool
}

// paraGroups splits a Para block's runs into structural text-groups. A
// STRUCTURAL inline-code (empty-Data Ph, synthesized by buildParaRuns for the
// `<Font>`/`<AFrame>`/`<Marker>`/… statements that physically separate source
// `<String>` tags) is a group boundary; the statement bytes stay in the
// skeleton. Building-block inline codes (Ph with Data, from the CodeFinder)
// belong INSIDE the group's `<String>` value, so their Data is rendered.
//
// A group is "extractable" when it carries non-whitespace text — those are
// the groups that received a translatable ref (runOrdinal). Whitespace-only
// or building-block-only groups are non-extractable: they own no ref (their
// `<String>` stays in the skeleton or is rewritten from a Char glyph), so the
// reader's runOrdinal numbering skips them. renderRunGroup therefore indexes
// into the EXTRACTABLE groups only, keeping the writer's group numbering in
// lock-step with the reader's runOrdinal (which counts ref-producing runs).
func paraGroups(runs []model.Run) []paraGroup {
	var groups []paraGroup
	cur := paraGroup{}
	flush := func() {
		groups = append(groups, cur)
		cur = paraGroup{}
	}
	for _, run := range runs {
		if run.Ph != nil && run.Ph.Data == "" {
			flush()
			continue
		}
		switch {
		case run.Text != nil:
			cur.text += run.Text.Text
			if hasNonWhitespace(run.Text.Text) {
				cur.extractable = true
			}
		case run.Ph != nil:
			cur.text += run.Ph.Data
		}
	}
	flush()
	return groups
}

// runGroupCount returns the number of EXTRACTABLE text-groups in a block.
func runGroupCount(runs []model.Run) int {
	n := 0
	for _, g := range paraGroups(runs) {
		if g.extractable {
			n++
		}
	}
	return n
}

// renderRunGroup renders the ordinal-th EXTRACTABLE text-group of a Para
// block (text + building-block code Data; structural boundaries excluded).
func renderRunGroup(runs []model.Run, ordinal int) string {
	idx := 0
	for _, g := range paraGroups(runs) {
		if !g.extractable {
			continue
		}
		if idx == ordinal {
			return g.text
		}
		idx++
	}
	return ""
}

// escapeMIF escapes special characters for MIF string values.
//
// The default branch must emit each rune as its UTF-8 byte sequence,
// not a single byte cast — `byte(r)` truncates any rune above U+00FF
// (e.g. pseudo-translated `ĺ` 0x013A → 0x3A = ':') and produces silent
// data corruption. Use a string append so the runtime writes the full
// UTF-8 encoding.
func escapeMIF(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		switch r {
		case '`':
			out.WriteString("\\`")
		case '\'':
			out.WriteString("\\'")
		case '\\':
			out.WriteString("\\\\")
		case '>':
			out.WriteString("\\>")
		case '\t':
			out.WriteString("\\t")
		case '\n':
			// Newlines in MIF strings should be represented with Char HardReturn.
			// For simplicity, we keep the newline in the string.
			out.WriteString("\\n")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
