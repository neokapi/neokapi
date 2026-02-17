package yaml

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
	yamlv3 "gopkg.in/yaml.v3"
)

// Writer implements DataFormatWriter for YAML files.
type Writer struct {
	format.BaseFormatWriter
	blocks map[string]*model.Block // key path → block
}

// NewWriter creates a new YAML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "yaml",
		},
		blocks: make(map[string]*model.Block),
	}
}

// Write consumes Parts from a channel and writes YAML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.flush()
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					w.blocks[block.Name] = block
				}
			}
		}
	}
}

func (w *Writer) flush() error {
	if w.Output == nil || len(w.blocks) == 0 {
		return nil
	}

	// Build a map structure from blocks
	result := make(map[string]interface{})
	for name, block := range w.blocks {
		text := block.SourceText()
		if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
			text = block.TargetText(w.Locale)
		}
		result[name] = text
	}

	encoder := yamlv3.NewEncoder(w.Output)
	encoder.SetIndent(2)
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("yaml writer: encoding: %w", err)
	}
	return encoder.Close()
}
