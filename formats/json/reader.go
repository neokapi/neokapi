package json

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for JSON files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

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

	var root interface{}
	if err := json.Unmarshal(content, &root); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("json: parsing: %w", err)}
		return
	}

	blockCounter := 0
	dataCounter := 0
	r.walkValue(ctx, ch, root, "", &blockCounter, &dataCounter)

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// walkValue recursively walks a JSON value and emits Parts.
// The path parameter tracks the key path for naming Blocks (e.g., "root.nested.key").
func (r *Reader) walkValue(ctx context.Context, ch chan<- model.PartResult, value interface{}, path string, blockCounter, dataCounter *int) {
	switch v := value.(type) {
	case map[string]interface{}:
		r.walkObject(ctx, ch, v, path, blockCounter, dataCounter)
	case []interface{}:
		r.walkArray(ctx, ch, v, path, blockCounter, dataCounter)
	case string:
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
// Since json.Unmarshal into map[string]interface{} does not preserve order,
// the key iteration order is non-deterministic. For reproducible ordering,
// we sort keys. However, the writer uses a re-parse approach, so key order
// in the output is determined by the original document, not the parts stream.
func (r *Reader) walkObject(ctx context.Context, ch chan<- model.PartResult, obj map[string]interface{}, path string, blockCounter, dataCounter *int) {
	// Use sorted keys for deterministic part ordering.
	keys := sortedKeys(obj)
	for _, key := range keys {
		childPath := key
		if path != "" {
			childPath = path + "." + key
		}
		r.walkValue(ctx, ch, obj[key], childPath, blockCounter, dataCounter)
	}
}

// walkArray iterates over the elements of a JSON array.
func (r *Reader) walkArray(ctx context.Context, ch chan<- model.PartResult, arr []interface{}, path string, blockCounter, dataCounter *int) {
	for i, elem := range arr {
		childPath := path + "[" + strconv.Itoa(i) + "]"
		switch elem.(type) {
		case string:
			if r.cfg.ExtractArrayStrings {
				r.walkValue(ctx, ch, elem, childPath, blockCounter, dataCounter)
			} else {
				*dataCounter++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", *dataCounter),
					Name: childPath,
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
			}
		default:
			r.walkValue(ctx, ch, elem, childPath, blockCounter, dataCounter)
		}
	}
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

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Sort for deterministic ordering
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
