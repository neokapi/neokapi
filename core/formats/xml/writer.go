package xml

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for XML files.
type Writer struct {
	format.BaseFormatWriter
	resolver format.SubfilterResolver
}

// Ensure Writer implements SubfilterAware.
var _ format.SubfilterAware = (*Writer)(nil)

// NewWriter creates a new XML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xml",
		},
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format writers.
func (w *Writer) SetSubfilterResolver(resolver format.SubfilterResolver) {
	w.resolver = resolver
}

// Write consumes Parts from a channel and writes reconstructed XML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if part.Type == model.PartBlock {
				block, ok := part.Resource.(*model.Block)
				if !ok {
					continue
				}
				text := w.blockText(block)
				if _, err := fmt.Fprint(w.Output, text); err != nil {
					return err
				}
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok && layer.IsEmbedded() {
					val, err := w.writeChildLayer(ctx, layer, parts)
					if err != nil {
						return fmt.Errorf("xml: writing child layer %s: %w", layer.Name, err)
					}
					if _, err := fmt.Fprint(w.Output, val); err != nil {
						return err
					}
				}
			}
		}
	}
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

// fallbackChildText concatenates block texts when no sub-writer is available.
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

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}
