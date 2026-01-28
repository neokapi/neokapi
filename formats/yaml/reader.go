package yaml

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/model"
	yamlv3 "gopkg.in/yaml.v3"
)

// Reader implements DataFormatReader for YAML files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new YAML reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "yaml",
			FormatDisplayName: "YAML",
			FormatMimeType:    "application/yaml",
			FormatExtensions:  []string{".yaml", ".yml"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/yaml", "text/yaml", "application/x-yaml"},
		Extensions: []string{".yaml", ".yml"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("yaml: nil document or reader")
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

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "yaml",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/yaml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("yaml: reading: %w", err)}
		return
	}

	var node yamlv3.Node
	if err := yamlv3.Unmarshal(content, &node); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("yaml: parsing: %w", err)}
		return
	}

	blockCounter := 0
	r.walkNode(ctx, ch, &node, nil, &blockCounter)

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) walkNode(ctx context.Context, ch chan<- model.PartResult, node *yamlv3.Node, path []string, blockCounter *int) {
	switch node.Kind {
	case yamlv3.DocumentNode:
		for _, child := range node.Content {
			r.walkNode(ctx, ch, child, path, blockCounter)
		}

	case yamlv3.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := keyNode.Value
			newPath := append(append([]string{}, path...), key)
			r.walkNode(ctx, ch, valNode, newPath, blockCounter)
		}

	case yamlv3.SequenceNode:
		for i, child := range node.Content {
			indexPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i))
			r.walkNode(ctx, ch, child, indexPath, blockCounter)
		}

	case yamlv3.ScalarNode:
		if node.Tag == "!!str" || node.Tag == "" {
			text := node.Value
			if strings.TrimSpace(text) == "" {
				return
			}
			*blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), text)
			block.Name = strings.Join(path, ".")
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
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
