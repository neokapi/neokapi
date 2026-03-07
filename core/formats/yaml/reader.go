package yaml

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
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

	blockCounter := 0

	// Use a Decoder to support multi-document YAML (--- separators).
	decoder := yamlv3.NewDecoder(strings.NewReader(string(content)))
	for {
		var node yamlv3.Node
		if err := decoder.Decode(&node); err != nil {
			if err == io.EOF {
				break
			}
			ch <- model.PartResult{Error: fmt.Errorf("yaml: parsing: %w", err)}
			return
		}
		r.walkNode(ctx, ch, &node, nil, &blockCounter)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) walkNode(ctx context.Context, ch chan<- model.PartResult, node *yamlv3.Node, path []string, blockCounter *int) {
	switch node.Kind {
	case yamlv3.DocumentNode:
		// Multi-document: each document node wraps content
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
		r.emitScalar(ctx, ch, node, path, blockCounter)

	case yamlv3.AliasNode:
		// Resolve aliases by walking the anchor target
		if node.Alias != nil {
			r.walkNode(ctx, ch, node.Alias, path, blockCounter)
		}
	}
}

func (r *Reader) emitScalar(ctx context.Context, ch chan<- model.PartResult, node *yamlv3.Node, path []string, blockCounter *int) {
	isString := node.Tag == "!!str" || node.Tag == ""

	if !isString && !r.cfg.ExtractNonStrings {
		return
	}

	text := node.Value
	if strings.TrimSpace(text) == "" {
		return
	}

	keyPath := strings.Join(path, ".")
	if !r.matchesKeyPath(keyPath) {
		return
	}

	*blockCounter++
	block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), text)
	block.Name = keyPath
	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// matchesKeyPath checks whether a key path matches the configured patterns.
// If no patterns are configured, all paths match.
func (r *Reader) matchesKeyPath(keyPath string) bool {
	if len(r.cfg.KeyPathPatterns) == 0 {
		return true
	}
	for _, pattern := range r.cfg.KeyPathPatterns {
		if matchGlobPath(pattern, keyPath) {
			return true
		}
	}
	return false
}

// matchGlobPath matches a dot-separated key path against a glob pattern.
// Supports * (matches one segment) and ** (matches zero or more segments).
func matchGlobPath(pattern, path string) bool {
	patParts := strings.Split(pattern, ".")
	pathParts := strings.Split(path, ".")
	return matchParts(patParts, pathParts)
}

func matchParts(pat, path []string) bool {
	if len(pat) == 0 {
		return len(path) == 0
	}
	if pat[0] == "**" {
		// ** matches zero or more segments
		// Try matching remaining pattern against every suffix of path
		for i := 0; i <= len(path); i++ {
			if matchParts(pat[1:], path[i:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	if pat[0] == "*" || pat[0] == path[0] {
		return matchParts(pat[1:], path[1:])
	}
	return false
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
