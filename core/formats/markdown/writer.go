package markdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Markdown files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	firstBlock    bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new Markdown writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "markdown",
		},
		cfg:        cfg,
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed Markdown.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)
	var orderedBlocks []*model.Block

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
					orderedBlocks = append(orderedBlocks, block)
				}
			}
		}
	}
done:
	// Wrap the output writer with a per-line trim that mirrors upstream
	// Okapi's MarkdownFilterWriter.trimNonEssentialTrailingSpaces (see
	// MarkdownFilterWriter.java:103-122): on each line break, if the
	// previous line ends in EXACTLY one trailing single space drop that
	// space. The upstream implementation ALSO strips lines made of all
	// spaces, but its skeleton writer (MarkdownSkeletonWriter.java:58
	// appendLinePrefix) re-prepends the per-block line prefix on every
	// line including the now-stripped ones — and the trim doesn't
	// reach those re-prepended bytes because they enter the writer
	// after the next \n. The net effect upstream is that "indent\n"
	// rows survive unchanged. Mirror the net effect here, not the
	// literal Java algorithm: only strip exactly-1-trailing-space.
	// Without this wrap, fixtures like test-html-block-newline.md
	// round-trip with `". \n"` (single trailing space) where okapi
	// emits `".\n"`.
	tw := newTrailSpaceTrimmer(w.Output)
	defer func() { _ = tw.Flush() }()

	// Mode 1: Skeleton store (byte-exact, streaming-friendly).
	if w.skeletonStore != nil {
		if err := w.writeFromSkeleton(w.skeletonStore, blocksByID, tw); err != nil {
			return err
		}
		return tw.Flush()
	}

	// Mode 2: Build from blocks (fallback).
	if err := w.writeFromBlocks(orderedBlocks, tw); err != nil {
		return err
	}
	return tw.Flush()
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block, out io.Writer) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("markdown writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := out.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				if _, err := io.WriteString(out, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeFromBlocks reconstructs markdown from blocks without a skeleton store.
func (w *Writer) writeFromBlocks(blocks []*model.Block, out io.Writer) error {
	for _, block := range blocks {
		text := w.blockText(block)

		if !w.firstBlock {
			if _, err := fmt.Fprint(out, "\n\n"); err != nil {
				return err
			}
		}
		w.firstBlock = false

		// Structure prefix, keyed on the normalized semantic role (WS6).
		// SemanticRole drives clean cross-format export (any source → Markdown);
		// it falls back to the format-specific block.Type so same-format
		// round-trips are unchanged.
		role := block.SemanticRole()
		if role == "" {
			role = block.Type
		}
		switch role {
		case model.RoleHeading:
			if n := headingLevel(block); n > 0 {
				if _, err := fmt.Fprint(out, strings.Repeat("#", n)+" "); err != nil {
					return err
				}
			}
		case model.RoleListItem:
			if _, err := fmt.Fprint(out, "- "); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprint(out, text); err != nil {
			return err
		}
	}
	return nil
}

// headingLevel returns a block's heading level, preferring the normalized
// structural annotation (WS1) and falling back to the legacy "level" property;
// 0 when neither is present.
func headingLevel(block *model.Block) int {
	if s, ok := block.Structure(); ok && s != nil && s.Level > 0 {
		return s.Level
	}
	if level, ok := block.Properties["level"]; ok {
		n := 0
		_, _ = fmt.Sscanf(level, "%d", &n)
		return n
	}
	return 0
}

// trailSpaceTrimmer is an io.Writer that mirrors upstream Okapi's
// MarkdownFilterWriter trimming algorithm: it buffers bytes per
// physical line and, at every '\n', applies the rule:
//
//   - if the buffered line is made up entirely of spaces, drop them all;
//   - else if it ends with EXACTLY one trailing space, drop that one;
//   - else keep the line intact (preserves ≥2 trailing spaces, the
//     CommonMark hard-break signal, plus the trailing 4-space pattern
//     in fixtures like DirectShape.md's <pre> code).
//
// Carriage returns are preserved verbatim. Flush MUST be called on the
// final write so any unterminated trailing line is also flushed.
type trailSpaceTrimmer struct {
	w   io.Writer
	buf []byte // current physical line being buffered (no trailing \n)
}

func newTrailSpaceTrimmer(w io.Writer) *trailSpaceTrimmer {
	return &trailSpaceTrimmer{w: w}
}

func (t *trailSpaceTrimmer) Write(p []byte) (int, error) {
	for i, c := range p {
		if c == '\n' {
			t.trimBuffered()
			t.buf = append(t.buf, '\n')
			if _, err := t.w.Write(t.buf); err != nil {
				return i, err
			}
			t.buf = t.buf[:0]
			continue
		}
		t.buf = append(t.buf, c)
	}
	return len(p), nil
}

// Flush writes any unterminated trailing line (after the final '\n')
// without applying the trim — okapi's writer leaves the final tail
// alone unless a newline arrives, and we keep that semantics so a
// fixture whose final line legitimately ends in a single space (rare
// in markdown, but possible) round-trips intact.
func (t *trailSpaceTrimmer) Flush() error {
	if len(t.buf) == 0 {
		return nil
	}
	_, err := t.w.Write(t.buf)
	t.buf = t.buf[:0]
	return err
}

func (t *trailSpaceTrimmer) trimBuffered() {
	if len(t.buf) < 2 {
		return
	}
	// Only strip if the line ends in EXACTLY one trailing space — see
	// the comment on the wrap site for why we don't mirror the upstream
	// "all-spaces → empty" branch (the upstream skeleton writer
	// re-prepends the line prefix immediately, so the net effect is
	// "indent\n" rows survive).
	n := len(t.buf)
	if t.buf[n-1] == ' ' && t.buf[n-2] != ' ' {
		t.buf = t.buf[:n-1]
	}
}

// blockText returns the rendered text for a block, preferring the target
// locale's translation if available, falling back to source. Multi-line
// paragraphs whose source carried a per-line continuation prefix (e.g.
// `> ` for blockquote bodies — see BlockPropLinePrefix in reader.go)
// have that prefix re-inserted after every "\n" so blockquotes and
// indented continuations retain their original line shape on round-trip.
// Mirrors okapi MarkdownFilter, whose TextUnit content carries only the
// LFs between lines while its skeleton-driven writer re-emits the
// per-line prefix.
func (w *Writer) blockText(block *model.Block) string {
	runs := w.blockRuns(block)
	if runs == nil {
		return ""
	}
	return RenderBlockContent(block, runs)
}

// RenderBlockContent renders a block's content (the given run sequence —
// source or target) the way the skeleton splice emits it: inline codes
// re-emit their original data, front matter values restore/add YAML
// quoting, and the markdown line-prefix property re-applies to multi-line
// continuations. The MDX reader's byte-faithfulness check uses the same
// function so reader and writer can never disagree about untranslated
// output.
func RenderBlockContent(block *model.Block, runs []model.Run) string {
	rendered := model.RenderRunsWithData(runs)
	if block.Type == "front-matter" {
		// The skeleton carries `key: ` and the newline only; the value —
		// including any quoting — is the block's responsibility. An
		// unchanged, originally-unquoted value renders raw so the
		// untranslated round-trip stays byte-exact whatever the source
		// spelling; quoting is restored (or added when needed) otherwise.
		quote := block.Properties[BlockPropFrontMatterQuote]
		if quote == "" && rendered == model.RenderRunsWithData(block.Source) {
			return rendered
		}
		return frontMatterScalar(rendered, quote)
	}
	if prefix, ok := block.Properties[BlockPropLinePrefix]; ok && prefix != "" && strings.Contains(rendered, "\n") {
		rendered = strings.ReplaceAll(rendered, "\n", "\n"+prefix)
	}
	return rendered
}

// frontMatterScalar renders a front matter value, restoring the source's
// quote style and adding quoting when an unquoted value's translation
// would not survive as a YAML plain scalar. Sources that were valid plain
// scalars render byte-identically (the quoting triggers cannot occur in a
// valid plain scalar), so roundtrip output is unchanged for untranslated
// content.
func frontMatterScalar(text, origQuote string) string {
	switch origQuote {
	case "\"":
		return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(text) + "\""
	case "'":
		return "'" + strings.ReplaceAll(text, "'", "''") + "'"
	}
	if frontMatterNeedsQuoting(text) {
		return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(text) + "\""
	}
	return text
}

// frontMatterNeedsQuoting reports whether text cannot stand as a YAML
// plain scalar on a single line.
func frontMatterNeedsQuoting(text string) bool {
	if text == "" {
		return false
	}
	if strings.TrimSpace(text) != text {
		return true
	}
	if strings.Contains(text, ": ") || strings.HasSuffix(text, ":") ||
		strings.Contains(text, " #") || strings.Contains(text, "\n") {
		return true
	}
	switch text[0] {
	case '"', '\'', '[', ']', '{', '}', '>', '|', '&', '*', '!', '%', '@', '`', ',', '#':
		return true
	}
	return strings.HasPrefix(text, "- ")
}

// blockRuns returns the target Run sequence for the configured locale,
// or the source Run sequence if no target is available.
func (w *Writer) blockRuns(block *model.Block) []model.Run {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		runs := block.TargetRuns(w.Locale)
		if len(runs) > 0 {
			return runs
		}
	}
	if len(block.Source) > 0 {
		return block.Source
	}
	return nil
}
