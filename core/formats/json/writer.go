package json

import (
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

// Writer implements DataFormatWriter for JSON files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	resolver      format.SubfilterResolver
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SubfilterAware, SkeletonStoreConsumer and
// StreamingWriter.
var _ format.SubfilterAware = (*Writer)(nil)
var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.StreamingWriter = (*Writer)(nil)

// StreamingWriter marks this writer as able to consume a streaming skeleton — it
// drains the Part stream then reads the skeleton via store.Next(), which the
// concurrent streaming store serves with synchronization. Pairing it with the
// streaming JSON reader (also a StreamingReader) makes the file runner wire a
// synchronized streaming skeleton store, instead of an unsynchronized buffered
// one the streaming reader and writer would race on.
func (w *Writer) StreamingWriter() bool { return true }

// NewWriter creates a new JSON writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "json",
		},
		cfg: cfg,
	}
}

// Config returns the writer's config for customization.
func (w *Writer) Config() *Config {
	return w.cfg
}

// SetSubfilterResolver sets the resolver for creating sub-format writers.
func (w *Writer) SetSubfilterResolver(resolver format.SubfilterResolver) {
	w.resolver = resolver
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed JSON.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)   // block.ID → block (for skeleton store)
	blocksByPath := make(map[string]*model.Block) // json keypath → block (for token reparse)
	childLayerValues := make(map[string]string)   // layer.Name (key path) → reconstructed string
	var originalJSON []byte

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
					// Index by raw key path for token-based roundtrip.
					key := block.Name
					if kp, ok := block.Properties["json.keypath"]; ok {
						key = kp
					}
					blocksByPath[key] = block
				}
			case model.PartLayerStart:
				if layer, ok := part.Resource.(*model.Layer); ok {
					if layer.IsEmbedded() {
						val, err := w.writeChildLayer(ctx, layer, parts)
						if err != nil {
							return fmt.Errorf("json: writing child layer %s: %w", layer.Name, err)
						}
						childLayerValues[layer.Name] = val
					} else if layer.IsRoot() {
						if raw, ok := layer.Properties["json.original"]; ok {
							originalJSON = []byte(raw)
						}
					}
				}
			}
		}
	}
done:
	// Mode 1: Skeleton store (byte-exact, streaming-friendly).
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("json writer: flush skeleton: %w", err)
		}
		return w.writeFromSkeleton(w.skeletonStore, blocksByID, childLayerValues)
	}

	// Mode 2/3: Re-tokenize original JSON or build from blocks.
	return w.reconstruct(originalJSON, blocksByPath, childLayerValues)
}

// writeChildLayer collects parts until the matching PartLayerEnd and writes them
// through the appropriate sub-format writer.
func (w *Writer) writeChildLayer(ctx context.Context, layer *model.Layer, parts <-chan *model.Part) (string, error) {
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

// writeFromSkeleton reads skeleton entries and fills in block/layer content.
// This produces byte-exact output — only translated text differs from the original.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block, childLayerValues map[string]string) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("json writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			var text string
			quote := byte('"')
			if strings.HasPrefix(refID, "layer:") {
				layerPath := refID[6:]
				text = childLayerValues[layerPath]
			} else if block, ok := blocks[refID]; ok {
				text = w.blockText(block)
				// JSON5 single-quoted source values round-trip with
				// the same delimiter (set by the reader on the block).
				if block.Properties["json.quote"] == "'" {
					quote = '\''
				}
			}
			encoded := escapeJSONStringQuoted(text, w.cfg.EscapeForwardSlashes, quote)
			if _, err := io.WriteString(w.Output, encoded); err != nil {
				return err
			}
		}
	}
	return nil
}

// reconstruct builds the JSON output from collected blocks.
// If originalJSON is available, it does a token-level replacement preserving
// the original formatting. Otherwise, it builds from block paths.
func (w *Writer) reconstruct(originalJSON []byte, blocks map[string]*model.Block, childLayerValues map[string]string) error {
	if originalJSON != nil {
		return w.reconstructFromOriginal(originalJSON, blocks, childLayerValues)
	}
	return w.reconstructFromBlocks(blocks, childLayerValues)
}

// reconstructFromOriginal scans the original JSON tokens and replaces string
// values with translated text while preserving all formatting.
func (w *Writer) reconstructFromOriginal(original []byte, blocks map[string]*model.Block, childLayerValues map[string]string) error {
	sc := newScanner(original)
	tokens, err := sc.scan()
	if err != nil {
		return w.reconstructFromBlocks(blocks, childLayerValues)
	}

	var out strings.Builder
	pos := 0
	w.writeTokenValue(&out, tokens, &pos, "", blocks, childLayerValues)

	// Write any trailing whitespace/comments from EOF token
	if pos < len(tokens) && tokens[pos].typ == tokenEOF {
		out.WriteString(tokens[pos].prefix)
	}

	_, writeErr := io.WriteString(w.Output, out.String())
	return writeErr
}

// writeTokenValue writes a JSON value, replacing translatable strings with block text.
func (w *Writer) writeTokenValue(out *strings.Builder, tokens []token, pos *int,
	path string, blocks map[string]*model.Block, childLayerValues map[string]string) {

	if *pos >= len(tokens) {
		return
	}

	tok := tokens[*pos]
	switch tok.typ {
	case tokenObjectStart:
		w.writeTokenObject(out, tokens, pos, path, blocks, childLayerValues)
	case tokenArrayStart:
		w.writeTokenArray(out, tokens, pos, path, blocks, childLayerValues)
	case tokenString:
		// Check if there's a block or child layer value for this path
		if val, ok := childLayerValues[path]; ok {
			out.WriteString(tok.prefix)
			out.WriteString(escapeJSONString(val, w.cfg.EscapeForwardSlashes))
			*pos++
		} else if block, ok := blocks[path]; ok {
			text := w.blockText(block)
			out.WriteString(tok.prefix)
			out.WriteString(escapeJSONString(text, w.cfg.EscapeForwardSlashes))
			*pos++
		} else {
			// Not a translatable string — preserve original raw bytes
			out.WriteString(tok.prefix)
			out.WriteString(tok.raw)
			*pos++
		}
	default:
		// Non-string value — write as-is
		out.WriteString(tok.prefix)
		out.WriteString(tok.raw)
		*pos++
	}
}

// writeTokenObject writes a JSON object, replacing string values.
func (w *Writer) writeTokenObject(out *strings.Builder, tokens []token, pos *int,
	parentPath string, blocks map[string]*model.Block, childLayerValues map[string]string) {

	out.WriteString(tokens[*pos].prefix)
	out.WriteString("{")
	*pos++

	for *pos < len(tokens) {
		tok := tokens[*pos]
		if tok.typ == tokenObjectEnd {
			out.WriteString(tok.prefix)
			out.WriteString("}")
			*pos++
			return
		}
		if tok.typ == tokenComma {
			out.WriteString(tok.prefix)
			out.WriteString(",")
			*pos++
			continue
		}
		if tok.typ == tokenString {
			key := tok.value
			out.WriteString(tok.prefix)
			out.WriteString(tok.raw) // preserve original key formatting
			*pos++

			// Write colon
			if *pos < len(tokens) && tokens[*pos].typ == tokenColon {
				out.WriteString(tokens[*pos].prefix)
				out.WriteString(":")
				*pos++
			}

			childPath := key
			if parentPath != "" {
				childPath = parentPath + "." + key
			}
			w.writeTokenValue(out, tokens, pos, childPath, blocks, childLayerValues)
			continue
		}

		// Unexpected token inside an object — only reachable on malformed input
		// that the lenient scanner accepted. Emit it verbatim and advance so the
		// writer always makes progress (mirrors the reader's walkTokenObject and
		// avoids an infinite loop / hang).
		out.WriteString(tok.prefix)
		out.WriteString(tok.raw)
		*pos++
	}
}

// writeTokenArray writes a JSON array, replacing string values.
func (w *Writer) writeTokenArray(out *strings.Builder, tokens []token, pos *int,
	parentPath string, blocks map[string]*model.Block, childLayerValues map[string]string) {

	out.WriteString(tokens[*pos].prefix)
	out.WriteString("[")
	*pos++

	index := 0
	for *pos < len(tokens) {
		tok := tokens[*pos]
		if tok.typ == tokenArrayEnd {
			out.WriteString(tok.prefix)
			out.WriteString("]")
			*pos++
			return
		}
		if tok.typ == tokenComma {
			out.WriteString(tok.prefix)
			out.WriteString(",")
			*pos++
			continue
		}

		childPath := parentPath + "[" + strconv.Itoa(index) + "]"
		w.writeTokenValue(out, tokens, pos, childPath, blocks, childLayerValues)
		index++
	}
}

// reconstructFromBlocks builds JSON from scratch when no original is available.
// This is the fallback path — it won't preserve original formatting.
func (w *Writer) reconstructFromBlocks(blocks map[string]*model.Block, childLayerValues map[string]string) error {
	if len(blocks) == 0 && len(childLayerValues) == 0 {
		_, err := w.Output.Write([]byte("{}"))
		return err
	}

	root := w.buildTree(blocks, childLayerValues)

	var buf strings.Builder
	w.writeJSON(&buf, root, 0)
	buf.WriteString("\n")

	_, writeErr := io.WriteString(w.Output, buf.String())
	return writeErr
}

// buildTree reconstructs a JSON value tree from blocks and child layer values.
func (w *Writer) buildTree(blocks map[string]*model.Block, childLayerValues map[string]string) any {
	root := newOrderedMap()

	for name, block := range blocks {
		text := w.blockText(block)
		setNestedPath(root, name, text)
	}
	for path, val := range childLayerValues {
		setNestedPath(root, path, val)
	}

	return root
}

// writeJSON writes a JSON value with indentation.
func (w *Writer) writeJSON(buf *strings.Builder, value any, indent int) {
	switch v := value.(type) {
	case *orderedMap:
		buf.WriteString("{\n")
		for i, key := range v.keys {
			w.writeIndent(buf, indent+1)
			buf.WriteString(escapeJSONString(key, w.cfg.EscapeForwardSlashes))
			buf.WriteString(": ")
			w.writeJSON(buf, v.values[key], indent+1)
			if i < len(v.keys)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		w.writeIndent(buf, indent)
		buf.WriteString("}")
	case *[]any:
		w.writeJSON(buf, *v, indent)
	case []any:
		buf.WriteString("[\n")
		for i, elem := range v {
			w.writeIndent(buf, indent+1)
			w.writeJSON(buf, elem, indent+1)
			if i < len(v)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		w.writeIndent(buf, indent)
		buf.WriteString("]")
	case string:
		buf.WriteString(escapeJSONString(v, w.cfg.EscapeForwardSlashes))
	default:
		buf.WriteString(fmt.Sprintf("%v", v))
	}
}

func (w *Writer) writeIndent(buf *strings.Builder, level int) {
	for range level {
		buf.WriteString("  ")
	}
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}

// orderedMap preserves key insertion order.
type orderedMap struct {
	keys   []string
	values map[string]any
}

func newOrderedMap() *orderedMap {
	return &orderedMap{values: make(map[string]any)}
}

func (m *orderedMap) set(key string, val any) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = val
}

func (m *orderedMap) get(key string) (any, bool) {
	v, ok := m.values[key]
	return v, ok
}

// setNestedPath sets a value at the given dotted/bracketed path in an orderedMap.
func setNestedPath(root *orderedMap, path string, value any) {
	parts := parsePath(path)
	if len(parts) == 0 {
		return
	}

	current := any(root)
	for i, part := range parts {
		isLast := i == len(parts)-1

		if idx, isIndex := parseIndex(part); isIndex {
			arr, ok := current.(*[]any)
			if !ok {
				var a []any
				arr = &a
			}
			for len(*arr) <= idx {
				*arr = append(*arr, nil)
			}
			if isLast {
				(*arr)[idx] = value
			} else {
				if (*arr)[idx] == nil {
					nextPart := parts[i+1]
					if _, nextIsIndex := parseIndex(nextPart); nextIsIndex {
						var a []any
						(*arr)[idx] = &a
					} else {
						(*arr)[idx] = newOrderedMap()
					}
				}
				current = (*arr)[idx]
			}
		} else {
			obj, ok := current.(*orderedMap)
			if !ok {
				return
			}
			if isLast {
				obj.set(part, value)
			} else {
				existing, exists := obj.get(part)
				if !exists {
					nextPart := parts[i+1]
					if _, nextIsIndex := parseIndex(nextPart); nextIsIndex {
						var a []any
						obj.set(part, &a)
						current = &a
					} else {
						child := newOrderedMap()
						obj.set(part, child)
						current = child
					}
				} else {
					current = existing
				}
			}
		}
	}
}

func parsePath(path string) []string {
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

func parseIndex(part string) (int, bool) {
	if len(part) < 3 || part[0] != '[' || part[len(part)-1] != ']' {
		return 0, false
	}
	idx, err := strconv.Atoi(part[1 : len(part)-1])
	if err != nil {
		return 0, false
	}
	return idx, true
}
