package yaml

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	yamlv3 "gopkg.in/yaml.v3"
)

// Writer implements DataFormatWriter for YAML files.
type Writer struct {
	format.BaseFormatWriter
	blocks        map[string]*model.Block // key path → block
	blockOrder    []string                // key paths in document order
	skeletonStore *format.SkeletonStore
	originalBytes []byte // raw original YAML, captured from LayerStart
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
			switch part.Type {
			case model.PartLayerStart:
				if layer, ok := part.Resource.(*model.Layer); ok && layer != nil {
					if raw, ok := layer.Properties["yaml.original"]; ok && raw != "" {
						w.originalBytes = []byte(raw)
					}
				}
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					if _, exists := w.blocks[block.Name]; !exists {
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
					encoded := encodeYAMLScalar(text, style)
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
	switch style {
	case "double-quoted":
		return encodeDoubleQuoted(text)
	case "single-quoted":
		return encodeSingleQuoted(text)
	case "literal":
		// For literal block scalars, we can't re-encode inline since the
		// skeleton already contains the indicator and structure. Just return
		// the text as-is since the skeleton handles the surrounding structure.
		return encodeLiteralBlock(text)
	case "folded":
		return encodeFoldedBlock(text)
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

// encodeLiteralBlock encodes text as a literal block scalar (| style).
// The indicator line and indent are part of the skeleton text, so we only
// return the content lines with proper indentation.
func encodeLiteralBlock(s string) string {
	// For block scalars, the skeleton ref replaces the entire scalar
	// representation including the indicator. So we need to produce the
	// full block scalar representation.
	return encodeLiteralBlockFull(s)
}

// encodeLiteralBlockFull produces a full literal block scalar representation.
func encodeLiteralBlockFull(s string) string {
	if !strings.Contains(s, "\n") {
		// Single line — use literal style with clip chomp
		return "|\n  " + s + "\n"
	}
	var b strings.Builder
	b.WriteString("|\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			// Trailing newline in value produces empty last split element
			continue
		}
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// encodeFoldedBlock produces a full folded block scalar representation.
func encodeFoldedBlock(s string) string {
	if !strings.Contains(s, "\n") {
		return ">\n  " + s + "\n"
	}
	var b strings.Builder
	b.WriteString(">\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			continue
		}
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func (w *Writer) flush() error {
	if w.Output == nil || len(w.blocks) == 0 {
		return nil
	}

	// Preferred path: rebuild from the original YAML AST so mapping
	// key order, nesting, and unmodified-value bytes are preserved.
	if len(w.originalBytes) > 0 {
		if err := w.flushFromOriginal(); err == nil {
			return nil
		}
		// Fall through to flat fallback on parse / structural errors.
	}

	// Fallback: flat key→value emit. The order is the document order
	// captured during channel consumption (w.blockOrder), so callers
	// at least get deterministic output even when we can't reconstruct
	// the original tree.
	for _, name := range w.blockOrder {
		block := w.blocks[name]
		if block == nil {
			continue
		}
		text := w.blockText(block)
		// Encode the value using yaml.v3 for proper escaping.
		encoded, err := yamlv3.Marshal(text)
		if err != nil {
			return fmt.Errorf("yaml writer: encoding %q: %w", name, err)
		}
		// yamlv3.Marshal of a string emits it followed by a newline.
		// Trim that — we want "key: value\n", not "key: value\n\n".
		encStr := strings.TrimRight(string(encoded), "\n")
		if _, err := fmt.Fprintf(w.Output, "%s: %s\n", name, encStr); err != nil {
			return err
		}
	}
	return nil
}

// flushFromOriginal re-parses the original YAML bytes, walks the AST
// substituting translated scalar values keyed by their dotted key path,
// and re-emits the document. This preserves mapping key order, nesting,
// and (where the value is unchanged) original byte representation.
func (w *Writer) flushFromOriginal() error {
	decoder := yamlv3.NewDecoder(strings.NewReader(string(w.originalBytes)))
	encoder := yamlv3.NewEncoder(w.Output)
	encoder.SetIndent(2)

	for {
		var node yamlv3.Node
		if err := decoder.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("yaml writer: re-parse original: %w", err)
		}
		w.substituteNode(&node, nil)
		if err := encoder.Encode(&node); err != nil {
			return fmt.Errorf("yaml writer: encode: %w", err)
		}
	}
	return encoder.Close()
}

// substituteNode walks the AST and replaces translatable scalar values
// using w.blocks[<dotted-path>]. Mirrors the reader's walkNode logic.
func (w *Writer) substituteNode(node *yamlv3.Node, path []string) {
	switch node.Kind {
	case yamlv3.DocumentNode:
		for _, child := range node.Content {
			w.substituteNode(child, path)
		}
	case yamlv3.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			newPath := append(append([]string{}, path...), keyNode.Value)
			w.substituteNode(valNode, newPath)
		}
	case yamlv3.SequenceNode:
		for i, child := range node.Content {
			indexPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i))
			w.substituteNode(child, indexPath)
		}
	case yamlv3.ScalarNode:
		keyPath := strings.Join(path, ".")
		if block, ok := w.blocks[keyPath]; ok {
			text := w.blockText(block)
			// Preserve the original scalar style so the encoder picks
			// the same representation.
			node.Value = text
			// If the text contains characters that would invalidate the
			// existing style (e.g. a newline in a plain scalar), let the
			// encoder choose by clearing the style.
			if needsStyleReset(text, node.Style) {
				node.Style = 0
			}
		}
	case yamlv3.AliasNode:
		// Aliases reference an anchor; do not rewrite the alias itself.
		// The anchored target node is reachable through its declaration
		// and will be substituted there.
	}
}

// needsStyleReset reports whether the existing yaml.v3 scalar style
// can no longer represent the given text (e.g. a plain scalar can't
// contain a newline). In those cases the encoder should pick a style.
func needsStyleReset(text string, style yamlv3.Style) bool {
	switch style {
	case yamlv3.DoubleQuotedStyle, yamlv3.SingleQuotedStyle:
		return false
	case yamlv3.LiteralStyle, yamlv3.FoldedStyle:
		return false
	default:
		// Plain or unset — newlines or leading/trailing spaces force a re-style.
		return strings.ContainsAny(text, "\n\r")
	}
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}
