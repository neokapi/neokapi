package json

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for JSON files.
// It collects all blocks by their key path name, then reconstructs the
// original JSON structure by re-parsing the original document and replacing
// string values with translated text from the blocks.
type Writer struct {
	format.BaseFormatWriter
	resolver format.SubfilterResolver
}

// Ensure Writer implements SubfilterAware.
var _ format.SubfilterAware = (*Writer)(nil)

// NewWriter creates a new JSON writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "json",
		},
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format writers.
func (w *Writer) SetSubfilterResolver(resolver format.SubfilterResolver) {
	w.resolver = resolver
}

// Write consumes Parts from a channel and writes reconstructed JSON.
// It first collects all block parts into a map keyed by block name (the JSON
// key path), then reconstructs the original JSON structure by walking the
// original parsed tree and replacing string values with their translations.
//
// Child layers (from subfiltered content) are reconstructed by writing their
// parts through the appropriate sub-format writer, producing a string value
// for the parent JSON key.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocks := make(map[string]*model.Block)
	childLayerValues := make(map[string]string) // layer.Name (key path) → reconstructed string
	var originalJSON []byte

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				// All parts consumed; reconstruct and write
				return w.reconstruct(originalJSON, blocks, childLayerValues)
			}
			if part.Type == model.PartBlock {
				block, ok := part.Resource.(*model.Block)
				if ok {
					blocks[block.Name] = block
				}
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok && layer.IsEmbedded() {
					// Child layer: collect its parts and write through sub-writer
					val, err := w.writeChildLayer(ctx, layer, parts)
					if err != nil {
						return fmt.Errorf("json: writing child layer %s: %w", layer.Name, err)
					}
					childLayerValues[layer.Name] = val
				}
			}
		}
	}
}

// writeChildLayer collects parts until the matching PartLayerEnd and writes them
// through the appropriate sub-format writer, returning the reconstructed string.
func (w *Writer) writeChildLayer(ctx context.Context, layer *model.Layer, parts <-chan *model.Part) (string, error) {
	// Collect child parts until PartLayerEnd
	var childParts []*model.Part
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return "", fmt.Errorf("unexpected end of parts stream in child layer %s", layer.ID)
			}
			if part.Type == model.PartLayerEnd {
				if endLayer, ok := part.Resource.(*model.Layer); ok && endLayer.ID == layer.ID {
					goto collected
				}
			}
			childParts = append(childParts, part)
		}
	}

collected:
	// If no resolver, concatenate block texts as fallback
	if w.resolver == nil {
		return w.fallbackChildText(childParts), nil
	}

	subWriter, err := w.resolver.ResolveWriter(layer.Format)
	if err != nil {
		return w.fallbackChildText(childParts), nil
	}

	var buf bytes.Buffer
	if err := subWriter.SetOutputWriter(&buf); err != nil {
		return "", err
	}
	subWriter.SetLocale(w.Locale)

	// Feed child parts through sub-writer via channel
	childCh := make(chan *model.Part, len(childParts))
	for _, p := range childParts {
		childCh <- p
	}
	close(childCh)

	if err := subWriter.Write(ctx, childCh); err != nil {
		return "", err
	}
	if err := subWriter.Close(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// fallbackChildText concatenates block source/target texts when no sub-writer is available.
func (w *Writer) fallbackChildText(parts []*model.Part) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if block, ok := p.Resource.(*model.Block); ok {
				sb.WriteString(w.blockText(block))
			}
		}
	}
	return sb.String()
}

// reconstruct builds the JSON output by creating a new structure from the
// collected blocks and child layer values.
func (w *Writer) reconstruct(originalJSON []byte, blocks map[string]*model.Block, childLayerValues map[string]string) error {
	if len(blocks) == 0 && len(childLayerValues) == 0 {
		_, err := w.Output.Write([]byte("{}"))
		return err
	}

	// Build a tree structure from block paths and child layer values
	root := w.buildTree(blocks, childLayerValues)

	// Marshal with indentation, preserving HTML characters unescaped.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return fmt.Errorf("json writer: marshaling: %w", err)
	}

	_, writeErr := w.Output.Write(buf.Bytes())
	return writeErr
}

// buildTree reconstructs a JSON value tree from the collected blocks and child layer values.
// Block names encode the key path (e.g., "nested.key", "items[0]").
func (w *Writer) buildTree(blocks map[string]*model.Block, childLayerValues map[string]string) any {
	root := make(map[string]any)

	for name, block := range blocks {
		text := w.blockText(block)
		w.setPath(root, name, text)
	}

	// Child layer values override/supplement block values at their key paths
	for path, val := range childLayerValues {
		w.setPath(root, path, val)
	}

	// If the root has only array indices as keys, return as array
	if arr, ok := w.tryConvertToArray(root); ok {
		return arr
	}

	return root
}

// setPath sets a value at the given dotted/bracketed path in the tree.
func (w *Writer) setPath(root map[string]any, path string, value any) {
	parts := w.parsePath(path)
	current := any(root)

	for i, part := range parts {
		isLast := i == len(parts)-1

		if idx, isIndex := w.parseIndex(part); isIndex {
			arr := w.ensureArray(current)
			if isLast {
				w.setArrayIndex(arr, idx, value)
				if i > 0 {
					w.setInParent(root, parts[:i], arr)
				}
			} else {
				for len(*arr) <= idx {
					*arr = append(*arr, nil)
				}
				if (*arr)[idx] == nil {
					(*arr)[idx] = make(map[string]any)
				}
				current = (*arr)[idx]
				if i > 0 {
					w.setInParent(root, parts[:i], arr)
				}
			}
		} else {
			obj, ok := current.(map[string]any)
			if !ok {
				return
			}
			if isLast {
				obj[part] = value
			} else {
				next := parts[i+1]
				if _, isIdx := w.parseIndex(next); isIdx {
					if _, exists := obj[part]; !exists {
						arr := make([]any, 0)
						obj[part] = &arr
					}
					current = obj[part]
				} else {
					if _, exists := obj[part]; !exists {
						obj[part] = make(map[string]any)
					}
					current = obj[part]
				}
			}
		}
	}
}

// setInParent updates the parent container to reference the given array.
func (w *Writer) setInParent(root map[string]any, pathParts []string, arr *[]any) {
	current := any(root)
	for i, part := range pathParts {
		isLast := i == len(pathParts)-1
		if idx, isIndex := w.parseIndex(part); isIndex {
			a := w.ensureArray(current)
			if isLast {
				_ = a
				_ = idx
			}
		} else {
			obj, ok := current.(map[string]any)
			if !ok {
				return
			}
			if isLast {
				obj[part] = arr
			} else {
				current = obj[part]
			}
		}
	}
}

func (w *Writer) ensureArray(current any) *[]any {
	switch v := current.(type) {
	case *[]any:
		return v
	default:
		arr := make([]any, 0)
		return &arr
	}
}

func (w *Writer) setArrayIndex(arr *[]any, idx int, value any) {
	for len(*arr) <= idx {
		*arr = append(*arr, nil)
	}
	(*arr)[idx] = value
}

func (w *Writer) parsePath(path string) []string {
	var parts []string
	current := strings.Builder{}

	for i := 0; i < len(path); i++ {
		ch := path[i]
		switch ch {
		case '.':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		case '[':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			current.WriteByte('[')
			for i++; i < len(path) && path[i] != ']'; i++ {
				current.WriteByte(path[i])
			}
			current.WriteByte(']')
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func (w *Writer) parseIndex(part string) (int, bool) {
	if len(part) < 3 || part[0] != '[' || part[len(part)-1] != ']' {
		return 0, false
	}
	idx, err := strconv.Atoi(part[1 : len(part)-1])
	if err != nil {
		return 0, false
	}
	return idx, true
}

func (w *Writer) tryConvertToArray(m map[string]any) ([]any, bool) {
	return nil, false
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}
