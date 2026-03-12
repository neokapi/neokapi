package tools

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// LayerProcessorConfig configures the LayerProcessor tool.
type LayerProcessorConfig struct {
	// Pipelines maps format names to tool chains. When a child layer with
	// the given format is encountered, its parts are processed through the
	// corresponding tool chain before being emitted to the output.
	Pipelines map[string][]tool.Tool
}

// ToolName returns the config's tool name.
func (c *LayerProcessorConfig) ToolName() string { return "layer-processor" }

// Reset clears all pipelines.
func (c *LayerProcessorConfig) Reset() { c.Pipelines = nil }

// Validate checks configuration validity.
func (c *LayerProcessorConfig) Validate() error { return nil }

// ApplyMap is not supported for LayerProcessorConfig (configured programmatically).
func (c *LayerProcessorConfig) ApplyMap(values map[string]any) error {
	return fmt.Errorf("layer-processor: programmatic configuration only")
}

// NewLayerProcessorTool creates a LayerProcessor tool that applies format-specific
// tool chains to child layers. Parts belonging to layers whose format has no
// configured pipeline pass through unchanged.
func NewLayerProcessorTool(cfg *LayerProcessorConfig) *LayerProcessor {
	if cfg == nil {
		cfg = &LayerProcessorConfig{}
	}
	return &LayerProcessor{
		BaseTool: tool.BaseTool{
			ToolName:        "layer-processor",
			ToolDescription: "Applies format-specific tool chains to child layers",
			Cfg:             cfg,
		},
		cfg: cfg,
	}
}

// LayerProcessor intercepts child layers and applies format-specific tool chains.
type LayerProcessor struct {
	tool.BaseTool
	cfg *LayerProcessorConfig
}

// Process overrides BaseTool.Process to handle child layers specially.
func (lp *LayerProcessor) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok && layer.IsEmbedded() {
					if err := lp.processChildLayer(ctx, layer, part, in, out); err != nil {
						return err
					}
					continue
				}
			}
			// Pass through non-child-layer parts
			select {
			case out <- part:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// processChildLayer buffers all parts in a child layer, optionally processes them
// through a format-specific pipeline, and emits them to the output.
func (lp *LayerProcessor) processChildLayer(ctx context.Context, layer *model.Layer, startPart *model.Part, in <-chan *model.Part, out chan<- *model.Part) error {
	// Buffer parts until matching PartLayerEnd
	var childParts []*model.Part
	var endPart *model.Part
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return fmt.Errorf("unexpected end of parts stream in child layer %s", layer.ID)
			}
			if part.Type == model.PartLayerEnd {
				if endLayer, ok := part.Resource.(*model.Layer); ok && endLayer.ID == layer.ID {
					endPart = part
					goto collected
				}
			}
			childParts = append(childParts, part)
		}
	}

collected:
	// Look up the pipeline for this layer's format
	pipeline := lp.pipelineFor(layer.Format)

	if pipeline == nil {
		// No pipeline configured — emit start, parts, end unchanged
		return lp.emitParts(ctx, out, startPart, childParts, endPart)
	}

	// Run through the tool chain
	processed, err := lp.runPipeline(ctx, pipeline, childParts)
	if err != nil {
		return fmt.Errorf("layer-processor: processing layer %s (format %s): %w", layer.ID, layer.Format, err)
	}

	return lp.emitParts(ctx, out, startPart, processed, endPart)
}

// pipelineFor returns the tool chain for the given format, or nil.
func (lp *LayerProcessor) pipelineFor(format string) []tool.Tool {
	if lp.cfg.Pipelines == nil {
		return nil
	}
	tools, ok := lp.cfg.Pipelines[format]
	if !ok || len(tools) == 0 {
		return nil
	}
	return tools
}

// runPipeline runs parts through a chain of tools sequentially.
func (lp *LayerProcessor) runPipeline(ctx context.Context, tools []tool.Tool, parts []*model.Part) ([]*model.Part, error) {
	current := parts
	for _, t := range tools {
		inCh := make(chan *model.Part, len(current))
		for _, p := range current {
			inCh <- p
		}
		close(inCh)

		outCh := make(chan *model.Part, len(current)+16)
		if err := t.Process(ctx, inCh, outCh); err != nil {
			return nil, fmt.Errorf("tool %s: %w", t.Name(), err)
		}
		close(outCh)

		var result []*model.Part
		for p := range outCh {
			result = append(result, p)
		}
		current = result
	}
	return current, nil
}

// emitParts sends the start marker, all parts, and the end marker to the output.
func (lp *LayerProcessor) emitParts(ctx context.Context, out chan<- *model.Part, start *model.Part, parts []*model.Part, end *model.Part) error {
	toSend := make([]*model.Part, 0, len(parts)+2)
	toSend = append(toSend, start)
	toSend = append(toSend, parts...)
	toSend = append(toSend, end)

	for _, p := range toSend {
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
