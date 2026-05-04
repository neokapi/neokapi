package yaml

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	yamlv3 "gopkg.in/yaml.v3"
)

// Writer implements DataFormatWriter for YAML files.
type Writer struct {
	format.BaseFormatWriter
	blocks        map[string]*model.Block // key path → block
	blockOrder    []string                // key paths in arrival (= source document) order
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new YAML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "yaml",
		},
		blocks: make(map[string]*model.Block),
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes YAML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block) // block.ID → block (for skeleton store)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					if _, seen := w.blocks[block.Name]; !seen {
						w.blockOrder = append(w.blockOrder, block.Name)
					}
					w.blocks[block.Name] = block
					blocksByID[block.ID] = block
				}
			}
		}
	}
done:
	// Mode 1: Skeleton store (byte-exact, streaming-friendly).
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("yaml writer: flush skeleton: %w", err)
		}
		return w.writeFromSkeleton(w.skeletonStore, blocksByID)
	}

	// Mode 2: Rebuild from blocks (lossy formatting).
	return w.flush()
}

// writeFromSkeleton reads skeleton entries and fills in block content.
// This produces byte-exact output — only translated text differs from the original.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("yaml writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				// If the text is unchanged from source and we have the original
				// raw bytes, use those for byte-exact output.
				if raw, ok := block.Properties["yaml.raw"]; ok && text == block.SourceText() {
					if _, err := io.WriteString(w.Output, raw); err != nil {
						return err
					}
				} else {
					style := block.Properties["yaml.style"]
					indicator := block.Properties["yaml.indicator"]
					indent := block.Properties["yaml.indent"]
					encoded := encodeYAMLScalarWithIndicatorIndent(text, style, indicator, indent)
					if _, err := io.WriteString(w.Output, encoded); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// encodeYAMLScalar encodes a string value using the specified YAML scalar style.
func encodeYAMLScalar(text, style string) string {
	return encodeYAMLScalarWithIndicator(text, style, "")
}

// encodeYAMLScalarWithIndicator is like encodeYAMLScalar but lets the
// caller pass the original block-scalar indicator (`|`, `|-`, `|+`,
// `|2`, `>-`, …) so the chomp / explicit-indent modifier carries
// through on round-trip. Empty indicator falls back to the bare `|` /
// `>` defaults.
func encodeYAMLScalarWithIndicator(text, style, indicator string) string {
	return encodeYAMLScalarWithIndicatorIndent(text, style, indicator, "")
}

// encodeYAMLScalarWithIndicatorIndent extends encodeYAMLScalarWithIndicator
// with the original block-scalar content indent (decimal string, e.g. "12")
// captured by the reader as `yaml.indent`. Empty indent falls back to a
// compact 2-space default suitable for fresh emission.
func encodeYAMLScalarWithIndicatorIndent(text, style, indicator, indent string) string {
	switch style {
	case "double-quoted":
		return encodeDoubleQuoted(text)
	case "single-quoted":
		return encodeSingleQuoted(text)
	case "literal":
		return encodeLiteralBlockWithIndicatorIndent(text, indicator, indent)
	case "folded":
		return encodeFoldedBlockWithIndicatorIndent(text, indicator, indent)
	default:
		// Plain scalar — if the text contains special characters, fall back
		// to the original style (plain).
		return encodePlain(text)
	}
}

// encodeDoubleQuoted encodes a string as a YAML double-quoted scalar.
func encodeDoubleQuoted(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case '\b':
			b.WriteString(`\b`)
		case '\x00':
			b.WriteString(`\0`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// encodeSingleQuoted encodes a string as a YAML single-quoted scalar.
func encodeSingleQuoted(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	b.WriteString(strings.ReplaceAll(s, "'", "''"))
	b.WriteByte('\'')
	return b.String()
}

// encodePlain returns the text as a plain scalar.
func encodePlain(s string) string {
	return s
}

// encodeLiteralBlock encodes text as a literal block scalar (| style)
// with the bare `|` indicator (clip chomp).
func encodeLiteralBlock(s string) string {
	return encodeLiteralBlockWithIndicator(s, "|")
}

// encodeLiteralBlockWithIndicator emits a literal block scalar using
// the given indicator line. Empty indicator falls back to bare `|`.
func encodeLiteralBlockWithIndicator(s, indicator string) string {
	return encodeLiteralBlockWithIndicatorIndent(s, indicator, "")
}

// encodeLiteralBlockWithIndicatorIndent extends
// encodeLiteralBlockWithIndicator with an explicit content indent
// (decimal string). Empty indent falls back to "  " (2 spaces).
func encodeLiteralBlockWithIndicatorIndent(s, indicator, indent string) string {
	if indicator == "" {
		indicator = "|"
	}
	pad := indentPad(indent)
	if !strings.Contains(s, "\n") {
		return indicator + "\n" + pad + s + "\n"
	}
	var b strings.Builder
	b.WriteString(indicator)
	b.WriteByte('\n')
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			continue
		}
		b.WriteString(pad)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// encodeFoldedBlock encodes text as a folded block scalar (> style).
func encodeFoldedBlock(s string) string {
	return encodeFoldedBlockWithIndicator(s, ">")
}

// encodeFoldedBlockWithIndicator emits a folded block scalar using the
// given indicator. Empty indicator falls back to bare `>`.
func encodeFoldedBlockWithIndicator(s, indicator string) string {
	return encodeFoldedBlockWithIndicatorIndent(s, indicator, "")
}

// encodeFoldedBlockWithIndicatorIndent extends
// encodeFoldedBlockWithIndicator with an explicit content indent
// (decimal string). Empty indent falls back to "  " (2 spaces).
func encodeFoldedBlockWithIndicatorIndent(s, indicator, indent string) string {
	if indicator == "" {
		indicator = ">"
	}
	pad := indentPad(indent)
	if !strings.Contains(s, "\n") {
		return indicator + "\n" + pad + s + "\n"
	}
	var b strings.Builder
	b.WriteString(indicator)
	b.WriteByte('\n')
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			continue
		}
		b.WriteString(pad)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// indentPad converts a decimal indent string (e.g. "12") to a space
// string. Empty / unparseable returns the legacy 2-space default.
func indentPad(indent string) string {
	if indent == "" {
		return "  "
	}
	n, err := strconv.Atoi(indent)
	if err != nil || n <= 0 {
		return "  "
	}
	return strings.Repeat(" ", n)
}

func (w *Writer) flush() error {
	if w.Output == nil || len(w.blocks) == 0 {
		return nil
	}

	// Build a yaml.Node tree directly so that mapping key order matches
	// the source document's order. yaml.v3 preserves the slice order of
	// MappingNode.Content (alternating key/value pairs); the previous
	// `map[string]any` approach lost order to Go's randomized map
	// iteration. blockOrder holds the keys in the order they arrived
	// from the reader, which mirrors the source document.
	root := &yamlv3.Node{Kind: yamlv3.MappingNode, Tag: "!!map"}
	for _, name := range w.blockOrder {
		block, ok := w.blocks[name]
		if !ok {
			continue
		}
		text := w.blockText(block)
		root.Content = append(root.Content,
			&yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!str", Value: name},
			&yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!str", Value: text},
		)
	}

	encoder := yamlv3.NewEncoder(w.Output)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return fmt.Errorf("yaml writer: encoding: %w", err)
	}
	return encoder.Close()
}

func (w *Writer) blockText(block *model.Block) string {
	// RenderRunsWithData splices inline-code Data back into the text
	// stream — required when the reader's codeFinder split the value
	// into TextRun + Ph runs. plain SourceText/TargetText drops Ph
	// runs so the placeholders would vanish on round-trip.
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs := block.Targets[w.Locale]
		var b strings.Builder
		for _, seg := range segs {
			b.WriteString(model.RenderRunsWithData(seg.Runs))
		}
		return b.String()
	}
	var b strings.Builder
	for _, seg := range block.Source {
		b.WriteString(model.RenderRunsWithData(seg.Runs))
	}
	return b.String()
}
