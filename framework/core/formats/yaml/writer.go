package yaml

import (
	"context"
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
		if err == io.EOF {
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

	// Build a map structure from blocks
	result := make(map[string]any)
	for name, block := range w.blocks {
		text := w.blockText(block)
		result[name] = text
	}

	encoder := yamlv3.NewEncoder(w.Output)
	encoder.SetIndent(2)
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("yaml writer: encoding: %w", err)
	}
	return encoder.Close()
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}
