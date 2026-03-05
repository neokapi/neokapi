package json

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for JSON files.
type Reader struct {
	format.BaseFormatReader
	cfg      *Config
	resolver format.SubfilterResolver
	layerSeq int // counter for generating unique child layer IDs
}

// Ensure Reader implements SubfilterAware.
var _ format.SubfilterAware = (*Reader)(nil)

// NewReader creates a new JSON reader.
func NewReader() *Reader {
	cfg := &Config{ExtractArrayStrings: true}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "json",
			FormatDisplayName: "JSON",
			FormatMimeType:    "application/json",
			FormatExtensions:  []string{".json"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format readers.
func (r *Reader) SetSubfilterResolver(resolver format.SubfilterResolver) {
	r.resolver = resolver
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/json"},
		Extensions: []string{".json"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("json: nil document or reader")
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

	// Emit layer start
	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "json",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/json",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Read all content and decode into interface{}
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("json: reading: %w", err)}
		return
	}

	var root any
	if err := json.Unmarshal(content, &root); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("json: parsing: %w", err)}
		return
	}

	blockCounter := 0
	dataCounter := 0
	r.walkValue(ctx, ch, root, "", layer.ID, &blockCounter, &dataCounter)

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// walkValue recursively walks a JSON value and emits Parts.
// The path parameter tracks the key path for naming Blocks (e.g., "root.nested.key").
func (r *Reader) walkValue(ctx context.Context, ch chan<- model.PartResult, value any, path, parentLayerID string, blockCounter, dataCounter *int) {
	switch v := value.(type) {
	case map[string]any:
		r.walkObject(ctx, ch, v, path, parentLayerID, blockCounter, dataCounter)
	case []any:
		r.walkArray(ctx, ch, v, path, parentLayerID, blockCounter, dataCounter)
	case string:
		// Check for subfilter match
		if mapping := r.matchSubfilter(path); mapping != nil && r.resolver != nil {
			r.emitSubfiltered(ctx, ch, v, path, parentLayerID, mapping, blockCounter, dataCounter)
			return
		}
		*blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), v)
		block.Name = path
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	default:
		// Non-string scalar values (numbers, booleans, null) are non-translatable
		*dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", *dataCounter),
			Name: path,
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}
}

// walkObject iterates over the keys of a JSON object in order.
func (r *Reader) walkObject(ctx context.Context, ch chan<- model.PartResult, obj map[string]any, path, parentLayerID string, blockCounter, dataCounter *int) {
	keys := sortedKeys(obj)
	for _, key := range keys {
		childPath := key
		if path != "" {
			childPath = path + "." + key
		}
		r.walkValue(ctx, ch, obj[key], childPath, parentLayerID, blockCounter, dataCounter)
	}
}

// walkArray iterates over the elements of a JSON array.
func (r *Reader) walkArray(ctx context.Context, ch chan<- model.PartResult, arr []any, path, parentLayerID string, blockCounter, dataCounter *int) {
	for i, elem := range arr {
		childPath := path + "[" + strconv.Itoa(i) + "]"
		switch elem.(type) {
		case string:
			if r.cfg.ExtractArrayStrings {
				r.walkValue(ctx, ch, elem, childPath, parentLayerID, blockCounter, dataCounter)
			} else {
				*dataCounter++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", *dataCounter),
					Name: childPath,
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
			}
		default:
			r.walkValue(ctx, ch, elem, childPath, parentLayerID, blockCounter, dataCounter)
		}
	}
}

// matchSubfilter checks if the given key path matches any configured subfilter mapping.
// Returns the first matching mapping, or nil if no match.
func (r *Reader) matchSubfilter(path string) *format.SubfilterMapping {
	for i := range r.cfg.Subfilters {
		sf := &r.cfg.Subfilters[i]
		if matchGlob(sf.Pattern, path) {
			return sf
		}
	}
	return nil
}

// emitSubfiltered emits a child layer with content parsed by the subfilter format reader.
func (r *Reader) emitSubfiltered(ctx context.Context, ch chan<- model.PartResult, content, path, parentLayerID string, mapping *format.SubfilterMapping, blockCounter, dataCounter *int) {
	subReader, err := r.resolver.ResolveReader(mapping.Format)
	if err != nil {
		// Fall back to plain block if subfilter reader is unavailable
		*blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), content)
		block.Name = path
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		return
	}

	r.layerSeq++
	childLayerID := fmt.Sprintf("sf%d", r.layerSeq)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	childLayer := &model.Layer{
		ID:       childLayerID,
		Name:     path,
		Format:   mapping.Format,
		Locale:   locale,
		ParentID: parentLayerID,
		Properties: map[string]string{
			"subfilter.source":  "json",
			"subfilter.keyPath": path,
		},
	}

	// Emit child layer start
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
		return
	}

	// Open sub-reader and emit its parts
	subDoc := &model.RawDocument{
		URI:          path,
		SourceLocale: locale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(content))),
	}
	if err := subReader.Open(ctx, subDoc); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("json: subfilter open for %s: %w", path, err)}
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
		return
	}

	// Read sub-reader parts, skipping the sub-reader's own layer start/end
	// (we already emitted our own child layer boundaries)
	for pr := range subReader.Read(ctx) {
		if pr.Error != nil {
			ch <- model.PartResult{Error: fmt.Errorf("json: subfilter read for %s: %w", path, pr.Error)}
			break
		}
		// Skip the sub-reader's document-level layer events — we provide our own
		if pr.Part.Type == model.PartLayerStart || pr.Part.Type == model.PartLayerEnd {
			if layer, ok := pr.Part.Resource.(*model.Layer); ok && layer.IsRoot() {
				continue
			}
		}
		r.emit(ctx, ch, pr.Part)
	}
	subReader.Close()

	// Emit child layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
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

// matchGlob matches a path against a glob pattern.
// Supports "*" (matches one path segment) and "**" (matches zero or more segments).
// Segments are separated by ".".
func matchGlob(pattern, path string) bool {
	// Use filepath.Match for simple patterns after converting dots to slashes
	patternNorm := strings.ReplaceAll(pattern, ".", "/")
	pathNorm := strings.ReplaceAll(path, ".", "/")
	// Handle array indices: "items[0]" → "items/[0]"
	// filepath.Match doesn't handle brackets specially in our context
	matched, _ := filepath.Match(patternNorm, pathNorm)
	return matched
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}

// sortStrings sorts a slice of strings in ascending order (insertion sort for small slices).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}
