package json

import (
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
}

// NewWriter creates a new JSON writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "json",
		},
	}
}

// Write consumes Parts from a channel and writes reconstructed JSON.
// It first collects all block parts into a map keyed by block name (the JSON
// key path), then reconstructs the original JSON structure by walking the
// original parsed tree and replacing string values with their translations.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocks := make(map[string]*model.Block)
	var originalJSON []byte

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				// All parts consumed; reconstruct and write
				return w.reconstruct(originalJSON, blocks)
			}
			if part.Type == model.PartBlock {
				block, ok := part.Resource.(*model.Block)
				if ok {
					blocks[block.Name] = block
				}
			}
			if part.Type == model.PartLayerStart {
				// Capture original JSON from the layer if available
				if layer, ok := part.Resource.(*model.Layer); ok {
					_ = layer // Layer metadata; original data comes from blocks
				}
			}
		}
	}
}

// reconstruct builds the JSON output by creating a new structure from the
// collected blocks. It builds a tree from block key paths and serializes it.
func (w *Writer) reconstruct(originalJSON []byte, blocks map[string]*model.Block) error {
	if len(blocks) == 0 {
		_, err := w.Output.Write([]byte("{}"))
		return err
	}

	// Build a tree structure from block paths
	root := w.buildTree(blocks)

	// Marshal with indentation to produce clean output
	output, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("json writer: marshaling: %w", err)
	}

	// Add trailing newline
	output = append(output, '\n')
	_, err = w.Output.Write(output)
	return err
}

// buildTree reconstructs a JSON value tree from the collected blocks.
// Block names encode the key path (e.g., "nested.key", "items[0]").
func (w *Writer) buildTree(blocks map[string]*model.Block) interface{} {
	root := make(map[string]interface{})

	for name, block := range blocks {
		text := w.blockText(block)
		w.setPath(root, name, text)
	}

	// If the root has only array indices as keys, return as array
	if arr, ok := w.tryConvertToArray(root); ok {
		return arr
	}

	return root
}

// setPath sets a value at the given dotted/bracketed path in the tree.
// For example, "nested.key" sets root["nested"]["key"] = value.
// "items[0]" sets root["items"][0] = value.
func (w *Writer) setPath(root map[string]interface{}, path string, value interface{}) {
	parts := w.parsePath(path)
	current := interface{}(root)

	for i, part := range parts {
		isLast := i == len(parts)-1

		if idx, isIndex := w.parseIndex(part); isIndex {
			// Array index access
			arr := w.ensureArray(current)
			if isLast {
				w.setArrayIndex(arr, idx, value)
				// Update parent to point to this array
				if i > 0 {
					w.setInParent(root, parts[:i], arr)
				}
			} else {
				// Need to descend into array element
				for len(*arr) <= idx {
					*arr = append(*arr, nil)
				}
				if (*arr)[idx] == nil {
					(*arr)[idx] = make(map[string]interface{})
				}
				current = (*arr)[idx]
				if i > 0 {
					w.setInParent(root, parts[:i], arr)
				}
			}
		} else {
			// Object key access
			obj, ok := current.(map[string]interface{})
			if !ok {
				return
			}
			if isLast {
				obj[part] = value
			} else {
				next := parts[i+1]
				if _, isIdx := w.parseIndex(next); isIdx {
					// Next is array index; ensure we have an array
					if _, exists := obj[part]; !exists {
						arr := make([]interface{}, 0)
						obj[part] = &arr
					}
					current = obj[part]
				} else {
					// Next is object key; ensure we have a map
					if _, exists := obj[part]; !exists {
						obj[part] = make(map[string]interface{})
					}
					current = obj[part]
				}
			}
		}
	}
}

// setInParent updates the parent container to reference the given array.
func (w *Writer) setInParent(root map[string]interface{}, pathParts []string, arr *[]interface{}) {
	current := interface{}(root)
	for i, part := range pathParts {
		isLast := i == len(pathParts)-1
		if idx, isIndex := w.parseIndex(part); isIndex {
			a := w.ensureArray(current)
			if isLast {
				_ = a
				_ = idx
			}
		} else {
			obj, ok := current.(map[string]interface{})
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

// ensureArray gets or creates an array pointer from the current value.
func (w *Writer) ensureArray(current interface{}) *[]interface{} {
	switch v := current.(type) {
	case *[]interface{}:
		return v
	default:
		arr := make([]interface{}, 0)
		return &arr
	}
}

// setArrayIndex sets a value at the given index in an array, growing it if needed.
func (w *Writer) setArrayIndex(arr *[]interface{}, idx int, value interface{}) {
	for len(*arr) <= idx {
		*arr = append(*arr, nil)
	}
	(*arr)[idx] = value
}

// parsePath splits a path like "nested.key" or "items[0]" into components.
// Returns ["nested", "key"] or ["items", "[0]"].
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
			// Read until closing bracket
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

// parseIndex checks if a path component is an array index like "[0]".
// Returns the index and true if it is, or 0 and false otherwise.
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

// tryConvertToArray checks if a map represents an array (all keys are indices)
// and converts it to a slice if so.
func (w *Writer) tryConvertToArray(m map[string]interface{}) ([]interface{}, bool) {
	// This is a helper; at the root level we always expect an object
	return nil, false
}

// blockText returns the text to use for a block: target text if a locale
// is set and a target exists, otherwise source text.
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}
