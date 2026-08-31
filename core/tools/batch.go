package tools

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// BatchConfig holds configuration for the batch collector tool.
type BatchConfig struct {
	Size int `schema:"title=Batch Size,description=Number of blocks to collect before forwarding as a batch,default=10,min=1"`
}

// ToolName returns the tool name this config applies to.
func (c *BatchConfig) ToolName() string { return "batch" }

// Reset restores default values.
func (c *BatchConfig) Reset() { c.Size = 10 }

// Validate checks configuration validity.
func (c *BatchConfig) Validate() error {
	if c.Size < 1 {
		return errors.New("batch: Size must be >= 1")
	}
	return nil
}

// NewBatchFromConfig creates a batch tool from a config map.
//
// Size is read once at construction, so a flow that asked for batches of 50 got
// batches of 10 — the default — because the step's config was discarded (#1476).
// batch stays Internal (it is pipeline plumbing, not a standalone command), but
// internal and unconfigurable are different things: a flow may name it as a step
// and must be able to size it. Monolingual: no locale to pin.
func NewBatchFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	cfg := &BatchConfig{}
	cfg.Reset()
	if err := applyStepConfig("batch", config, cfg, "", nil); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return NewBatchTool(cfg), nil
}

// NewBatchTool creates a tool that collects blocks into batches.
// Non-block parts pass through immediately. Blocks are buffered and
// forwarded together every Size blocks, with a BatchAnnotation on the
// last block in each batch.
func NewBatchTool(cfg *BatchConfig) tool.Tool {
	if cfg.Size < 1 {
		cfg.Size = 10
	}

	return &batchTool{
		ToolName:        "batch",
		ToolDescription: "Collect blocks into batches for downstream batch processing",
		Cfg:             cfg,
		size:            cfg.Size,
	}
}

type batchTool struct {
	tool.BaseTool
	size int
}

// Process overrides BaseTool.Process to implement batching logic.
func (b *batchTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	var batch []*model.Part

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		for _, p := range batch {
			select {
			case out <- p:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				// Flush remaining blocks on stream end
				return flush()
			}
			if part.Type != model.PartBlock {
				// Non-block parts pass through immediately
				select {
				case out <- part:
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
			batch = append(batch, part)
			if len(batch) >= b.size {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
}
